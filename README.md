# tmsentinel

A stateless, FDA-free Time Machine monitoring CLI written in Go. It
answers one question: is my Time Machine backup really running — and
if not, why not?

Read-only, no daemon, no GUI, no state file: the "last successful
backup" state is derived fresh from the Unified Log on every run.

Status: **Phase 1 — MVP: `check`, `destinations`** (Phase 0 complete,
Gate 1 passed — see "Phase 0 results").

## Usage

```sh
go build -o tmsentinel .
./tmsentinel check [--json] [--threshold HOURS]   # freshness check, exit code for schedulers
./tmsentinel destinations [--json]                # destinations, kind, reachability
./tmsentinel export FILE                           # raw-data export (corpus/fixtures)
./tmsentinel version
```

`check` derives the state fresh from the Unified Log (read-only, no
daemon, no state file) and checks the destination via
`tmutil destinationinfo` (FDA-free). The underlying query:

```
/usr/bin/log show --style ndjson --info --last 7d \
  --predicate 'subsystem == "com.apple.TimeMachine"'
```

(`--info` is mandatory: backupd logs its lifecycle signals at Info
level — see "Phase 0 results".)

### Exit codes (contract)

| Code | Meaning |
| --- | --- |
| 0 | Last backup successful and fresh; a running backup counts as healthy (0 with a hint) |
| 1 | Internal error (subprocess, flags, write failure) |
| 2 | Last backup older than the threshold — or no successful backup in the window |
| 3 | No destination configured |
| 4 | Destination configured but unreachable |
| 5 | Reserved: running backup, for `--json` consumers only (field `running: true`); `check` itself exits 0 with a hint |

Codes 0–5 are stable. New states get new codes >= 6, major version only.

### Flags and JSON

- `check --threshold` (hours, default 48), `check --json`,
  `destinations --json`
- `check --json` schema (additive only, never renamed): `state`,
  `last_success_age_h`, `threshold_h`, `destinations[]` (name, kind,
  mount point, reachable), `diagnosis{}`, `running`, `config_loaded`
- No configuration file required; `config_loaded: false` until
  configuration arrives in Phase 2

### Export format

`tmsentinel export FILE` writes header lines starting with `#`
(captured_at, command), followed by the unchanged NDJSON records.
The parser skips `#` lines and the `log show` footer
(`{"count":N,"finished":1}`). Unparsed lines are logged to stderr with
their raw content, followed by a summary (`n parsed / n unparsed
records`) — nothing is silently dropped.

## Installation

### Homebrew (tap)

```sh
brew install breichr/tmsentinel
```

(Tap: `github.com/breichr/homebrew-tmsentinel`; the formula source is
kept as a template in `contrib/homebrew/`.)

### Direct download

Unpack the release archive for `darwin-arm64` or `darwin-amd64` from the
[GitHub releases page](https://github.com/breichr/tmsentinel/releases),
put the binary into `~/.local/bin` (or similar), make it executable.

### From source

```sh
go build -o ~/.local/bin/tmsentinel .
```

Standard library only, no external dependencies; macOS only (the
`statfs` reachability check and `log`/`tmutil` are darwin-specific).

## Examples

```sh
# For launchd/cron/healthchecks.io: only the exit code matters
tmsentinel check

# Nothing fresh? Inspect the destinations
tmsentinel destinations

# Your own threshold
tmsentinel check --threshold 24

# Raw data for bug reports and test fixtures
tmsentinel export report.ndjson
```

## Phase 0 results

Verified 2026-10-23 on the target Mac (macOS, arm64), without Full Disk
Access. Important for interpretation: at the start of the available
log window the backup destination was **unreachable** on this system
(`destinationinfo` showed a local destination "TimeMachine", volume
not mounted; additionally the Time Machine menu process probed an SMB
destination `NASBOX` that could not be resolved via Bonjour). In the
7-day window **not a single backup ran**.

### tmutil verification without Full Disk Access

**`tmutil destinationinfo` — works without FDA (exit 0):**

```
====================================================
Name          : TimeMachine
Kind          : Local
ID            : 8F11C3F6-D15E-42D4-91B2-DD75A321E475
```

**`tmutil status` — works without FDA (exit 0):**

```
Backup session status:
{
    ClientID = "com.apple.backupd";
    Percent = "-1";
    Running = 0;
}
```

**`tmutil latestbackup` — FDA binding confirmed (destination mounted):**

```
tmutil: latestbackup requires Full Disk Access privileges.
To allow this operation, select Full Disk Access in the Privacy
tab of the Security and Privacy preference pane, and add Terminal
to the list of applications which are allowed Full Disk Access.
```

(exit 80). In the first verification run (destination not mounted) the
command did **not** fail on FDA as expected, but earlier on mounting
the destination (`Code=18 "Failed to mount destination."`,
notably with exit 0) — the FDA-protected operation was never reached.
With the destination mounted, the FDA error occurs exactly as
expected: **the FDA binding is proven.** Even a plain
`ls /Volumes/TimeMachine/` fails without FDA with "Operation not
permitted" — the destination itself stays FDA-protected, the Unified
Log does not.

### Prototype runs against real data (7 days)

Two runs, two system states:

**Run 1 — destination unreachable (14:29):**

```
tmsentinel: 5572 parsed / 0 unparsed records (1 footer lines)
Time Machine status (unified log, last 7d)
  last successful backup : none in window
  backups detected       : 0
  last backup state      : no backup session detected
  threshold              : 48.0 h
result: no successful backup in window (exit 2)
```

Cross-check: `tmutil status` reported `Running = 0`, the destination
volume was not mounted, the NAS was not resolvable via Bonjour — no
source showed a backup. Consistent.

**Run 2 — backup SSD connected, one complete run (15:0x):**

```
tmsentinel: 13038 parsed / 0 unparsed records (1 footer lines)
Time Machine status (unified log, last 7d)
  last successful backup : 2026-10-23 14:37:01 CEST (0.3 h ago)
  backups detected       : 1
  last backup state      : success
  threshold              : 48.0 h
result: fresh (exit 0)
```

Cross-check against reality: Time Machine started automatically when
the SSD was connected (`tmutil status`: `Running = 1` from 14:33:49,
phase PreparingSourceVolumes) and reported `Running = 0` afterwards.
The Unified Log shows the complete run: dispatch 14:33:47, "Backup
succeeded — Elapsed time: About 4 minutes" at 14:37:01 — the reported
age (0.3 h) and timestamp match reality. The previous backup on the
destination dated from Oct 14 (reference mount
`2026-10-14-170330.backup`); the window in between was backup-free,
exactly as run 1 reported.

### Key technical finding: the Info level is mandatory

The first prototype ran without `--info` and saw **not a single**
lifecycle event. Cause: `backupd` logs all lifecycle signals
(`BackupDispatching`, `BackupDestination`, `TMSession`, …) at **Info**
level, which `log show` does not return by default. The final query
therefore is:

```
/usr/bin/log show --style ndjson --info --last 7d \
  --predicate 'subsystem == "com.apple.TimeMachine"'
```

Second finding: `fs_snapshot_list failed: Operation not supported` is
**not** a failure signal — it fires during a healthy, successful backup
(3× at 14:33:51, success completion) as well as during idle maintenance
(62× over the backup-free week). The classifier treats it as noise
accordingly; likewise `Starting backup pass N` (inner passes of a
running backup, category FileProtection).

Side finding: the Unified Log store of this system reaches back only
to 2026-10-19 13:24 (a 30-day query returns practically the same lines
as the 7-day query). For corpus capture this means: captures are
short-lived, run `export` regularly.

## Gate 1 report

**Gate 1 question: does the Unified Log reliably deliver the start and
completion of every backup without FDA?**

Answer: **Yes — proven on a real, complete backup run on 2026-10-23 on
this Mac, entirely without FDA.** Both start and completion appear in
category `BackupDispatching` of subsystem `com.apple.TimeMachine`
(Info level) with clear, machine-readable messages.

Per backup in the 7-day window:

| Backup | Start event | Completion event |
| --- | --- | --- |
| 2026-10-23, 14:33:47 → 14:37:01 (local, APFS SSD) | yes — `BackupDispatching`: "Backup requested to next destination in rotation" | yes — `BackupDispatching`: "Backup succeeded — Elapsed time: About 4 minutes" |

Earlier backups (most recent Oct 14) lie outside the log window (the
store retains only ~4 days) and are therefore not verifiable — no claim
about them, positive or negative.

Also proven:

- `log show --style ndjson --info` on subsystem
  `com.apple.TimeMachine` works **without FDA** from the command line;
  13,038 records readable in the 7-day window, all fields public (no
  `<private>`).
- The **absence** of backups is equally observable: run 1 correctly
  reported "no backup in window" while the destination was unreachable
  — the tool's use case, FDA-free.
- The FDA boundary lies exactly where the project expected it:
  `tmutil latestbackup` (exit 80) and even `ls` on the destination fail
  without FDA; the Unified Log does not.

Open questions (documented, not speculatively solved in code):

1. There is currently **no network Time Machine destination** on this
   Mac (local, APFS SSD). The SMB variant of `BackupDispatching` is
   therefore **not applicable here** — an open question for later or
   for other systems, once a network destination exists.
2. What does the **failure** completion event of this macOS version
   look like (expected: category `BackupDispatching`, message with
   "failed")? Classification currently only covers classic patterns,
   unverified on this system.
3. No real **hung** backup (start without completion) occurred in the
   window; the state machine handles it (hung after a 6 h grace
   period), but without a fixture it remains untested against reality.

Gate 1: **passed** (proven locally; the SMB variant is not verifiable
on this system for lack of a network destination, documented as an
open edge condition).

### Prototype pattern catalog with verification status

| Pattern | Classification | Status |
| --- | --- | --- |
| Category `BackupDispatching` + "Backup requested to next destination in rotation" | start | **proven** (real run 2026-10-23) |
| Category `BackupDispatching` + "Backup succeeded" | success | **proven** (real run 2026-10-23) |
| `Starting backup pass N` (FileProtection) | none (inner pass) | **proven** — deliberately not a session start |
| `fs_snapshot_list failed` | none (noise) | **proven** — also fires in healthy runs |
| `Failed to read capabilities`, `Bonjour resolution not possible`, XPC noise | none (noise) | **proven** |
| `Starting (automatic\|manual) backup` | start | classically documented, **unverified** on this system |
| `Completed backup`, `Backup completed`, `Backup finished` | success | classically documented, **unverified** on this system |
| `Backup failed`, `failed to complete backup`, `Backup aborted`, `Failed to mount (backup )?destination` | failed | classically documented, **unverified** on this system |

### Corpus start

Two anonymized real exports as golden-file tests:

- `testdata/phase0-2026-10-23-anonymized.ndjson` — backup-free window
  (default level, 5,570 records): state "no backup", exit 2.
- `testdata/phase0-2026-10-23-run.ndjson` — complete real run (Info
  level, 292 records): exactly one session, start 14:33:47, success
  14:37:01, exit 0 — the Gate 1 evidence.

Continuation: run `export` regularly until the corpus also contains an
SMB run (success and abort) and a hung backup (roadmap).

## Definition of done (Phase 0)

- `go build ./...`, `go vet ./...`, `go test ./...` pass — unit tests
  for parser, classification, state machine, threshold logic, and
  export with sample NDJSON lines, including lines with missing fields
  (no timestamp, missing fields, garbage lines, footer, comment lines).
- Read-only: the program never starts or stops backups and changes
  nothing on the system.
- No Phase 1 work (`check`/`destinations` subcommands) — that phase
  has since been completed.
