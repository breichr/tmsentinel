# Gate 2 notes — two-week self-use log

Window: 2026-10-23 to 2026-11-06 (started with hourly backups to the
local APFS SSD and the 6-hourly corpus export via cron).

Gate 2 questions (roadmap):

1. Do I use tmsentinel myself daily after two weeks?
2. Does it cover at least one situation tmwatch (and the healthchecks
   script) do not — e.g. running backup, destination-offline vs.
   staleness?

Fill the tables as you go. Verdict at the bottom, after the window.
If a backup fails or hangs, capture its exit code and output the same
day — the corpus export runs every 6 h automatically.

## Daily use

| Date | Used? | Where (prompt/alias/launchd)? | Notes |
| --- | --- | --- | --- |
| (example) 2026-10-23 | yes | shell prompt | check reported 0.3 h, exit 0, matched menu bar |

## Incidents and observations

| Date | Situation | tmsentinel said | tmwatch / script said | Menu bar reality | Verdict |
| --- | --- | --- | --- | --- | --- |
| (example) 2026-10-23 | SSD unplugged all day | exit 4, destination unreachable | — | "last backup X days ago" | tmsentinel named the cause, others only the age |

## Coverage wins over tmwatch (Gate 2 evidence)

| Capability | Observed on | Evidence |
| --- | --- | --- |
| destination offline vs. stale (exit 4 vs. 2) | | |
| running backup detected (hint, exit 0) | | |
| hung backup (start without completion) | | |
| anything else | | |

## Weaknesses (things that annoyed me or were wrong)

| Date | Weakness | Severity | Follow-up phase |
| --- | --- | --- | --- |

## Verdict (fill after 2026-11-06)

- [ ] Used daily
- [ ] At least one situation covered that tmwatch does not cover
- Result: pass / fail, and what follows from it
