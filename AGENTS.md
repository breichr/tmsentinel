# tmsentinel

Stateless, FDA-free Time Machine monitoring CLI in Go. Answers: is my
backup really running — and if not, why not? Read-only, no daemon, no GUI.

## Non-negotiables

- Stateless: no database, no state file, no daemon. "Last successful
  backup" is derived from the Unified Log on every run.
- Read-only: never starts, stops, or configures backups.
- FDA-free: no Full Disk Access. `tmutil latestbackup`/`listbackups`
  only behind an optional FDA flag.
- Exit codes 0-5 are a stable contract (see README). New states get
  new codes >= 6, major version only.
- Log access via `log show --style ndjson` subprocess, never
  OSLogStore. Parse defensively against missing fields; log unparsed
  records, never guess.
- No fingerprint without a test fixture in testdata/.

## Packages

Phase 0 is a single-file prototype in the module root (package main).
The target package layout for Phase 1+:

- event: Unified-Log parser + event model. Only package that shells
  out to `log`.
- state: backup state machine (idle -> running -> success/failed), pure.
- freshness: age logic, thresholds, injected clock, pure.
- reason: failure fingerprints -> plain-text diagnosis, pure.
- notify: healthchecks ping, macOS notification via osascript, exit
  code adapter. Only package besides event that runs subprocesses.
- cli: subcommand dispatch, output, config.

Everything else does no I/O. In the prototype the same separation is
kept functionally: parsing, state derivation, and freshness logic are
pure functions; only runLogShow performs process I/O.

## Testing

- Fixtures are anonymized real `log show` exports in testdata/,
  produced by `--export` (later `tmsentinel export`). Golden-file
  tests. Synthetic fixtures must be marked `synthetic` in a header
  comment.
- No test calls `log`, `tmutil`, or the network. Recorded outputs only.
- `go build ./...`, `go vet ./...`, `go test ./...` must pass.
- CI must run without macOS privileges; live smoke tests carry
  //go:build darwin.

## Definition of done

- All checks pass, new parsing covered by a fixture including the
  missing-field fallback.
- `tmsentinel check` works with zero config after every change.
- README updated for user-visible changes.
- Current phase: 0 (proof of the FDA-free core). Do not work ahead of
  the active gate.
