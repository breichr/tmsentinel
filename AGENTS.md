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

- event: Unified-Log parser + event model. Shells out to `log`
  (UnifiedLog source) and writes the export corpus format.
- state: backup state machine (idle -> running -> success/failed/hung),
  pure, sorts its input.
- freshness: age logic, thresholds, injected clock, pure.
- destinations: tmutil destinationinfo subprocess + parser + mount-point
  reachability (statfs). Second subprocess package.
- cli: subcommand dispatch (check, destinations, export, version),
  output, exit-code contract.
- reason: failure fingerprints -> plain-text diagnosis, pure (Phase 3).
- notify: healthchecks ping, macOS notification via osascript, exit
  code adapter (Phase 2).

Only event and destinations run subprocesses. Everything else does no
I/O; subprocess sources are injected as interfaces, so no test ever
runs `log` or `tmutil`.

## Testing

- Fixtures are anonymized real `log show` exports in testdata/,
  produced by `tmsentinel export`. Golden-file
  tests. Synthetic fixtures must be marked `synthetic` in a header
  comment.
- No test calls `log`, `tmutil`, or the network. Recorded outputs only.
- `go build ./...`, `go vet ./...`, `go test ./...` must pass.
- CI must run without macOS privileges; live smoke tests carry
  //go:build darwin.

## Releases

- Releases are cut locally, never in CI: the Developer ID certificate
  and the notary keychain profile (`caskvet-notary`) stay on the
  machine and never enter GitHub secrets. CI only builds and tests.
- Process per version: tag `vX.Y.Z` matching `cli.Version` ->
  `scripts/release.sh` (build darwin/arm64 + amd64, codesign with
  Developer ID Application, hardened runtime, timestamp; ditto zips;
  checksums.txt) -> `scripts/notarize.sh` (notarytool, wait for
  Accepted) -> `gh release create` with the zips and checksums.txt.
- Archives are zips, not tar.gz: notarytool accepts no tar.gz. Zip
  without --keepParent so the archive contains only the binary.
- After each release, update the formula template in
  contrib/homebrew/ (version, URLs, sha256 from checksums.txt) and
  copy it by hand into the tap repository breichr/homebrew-tmsentinel.
- Install docs use the fully qualified path:
  `brew install breichr/tmsentinel/tmsentinel` (the shorthand does
  not resolve on current brew).

## Definition of done

- All checks pass, new parsing covered by a fixture including the
  missing-field fallback.
- `tmsentinel check` works with zero config after every change.
- README updated for user-visible changes.
- Current phase: 1 (MVP: check, destinations). Do not work ahead of
  the active gate.
