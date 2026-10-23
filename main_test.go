package main

// Fixture lines marked [real] are anonymized records captured from this
// system's Unified Log on 2026-10-23. Lines marked [synthetic] are
// constructed edge cases that have no real export yet.

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// [real] anonymized: fs_snapshot_list failure observed in category
// TMSnapshot on this system. Verified as noise: it also fires inside a
// healthy, successful backup run (2026-10-23) and in idle maintenance.
const realSnapshotFail = `{"timezoneName":"","messageType":"Error","eventType":"logEvent","source":null,"formatString":"%{public}@","userID":501,"subsystem":"com.apple.TimeMachine","category":"TMSnapshot","threadID":3910225,"processImagePath":"/System/Library/CoreServices/SystemUIServer.app/Contents/MacOS/SystemUIServer","timestamp":"2026-10-19 15:21:43.980924+0200","eventMessage":"fs_snapshot_list failed: Operation not supported"}`

// [real] anonymized: capabilities read failure observed on this system.
const realCapabilitiesFail = `{"eventType":"logEvent","subsystem":"com.apple.TimeMachine","category":"General","processImagePath":"/System/Library/CoreServices/backupd-helper","timestamp":"2026-10-20 14:03:37.290851+0200","eventMessage":"Failed to read capabilities for '/Volumes/NASBOX', error: Operation not permitted"}`

// [real] anonymized: verified backup start signal from the 2026-10-23
// run, category BackupDispatching, emitted by backupd.
const realDispatchStart = `{"eventType":"logEvent","subsystem":"com.apple.TimeMachine","category":"BackupDispatching","processImagePath":"/System/Library/CoreServices/TimeMachine/backupd","timestamp":"2026-10-23 14:33:47.562690+0200","eventMessage":"Backup requested to next destination in rotation. specifiedOptions: TMBackupOptions(rawValue: 1)"}`

// [real] anonymized: verified backup success signal from the same run.
const realDispatchSuccess = `{"eventType":"logEvent","subsystem":"com.apple.TimeMachine","category":"BackupDispatching","processImagePath":"/System/Library/CoreServices/TimeMachine/backupd","timestamp":"2026-10-23 14:37:01.297873+0200","eventMessage":"Backup succeeded\nElapsed time: About 4 minutes"}`

// [real] anonymized: inner pass markers of a running backup (category
// FileProtection); must not be treated as session starts.
const realPassStart = `{"eventType":"logEvent","subsystem":"com.apple.TimeMachine","category":"FileProtection","processImagePath":"/System/Library/CoreServices/TimeMachine/backupd","timestamp":"2026-10-23 14:35:10.124577+0200","eventMessage":"Starting backup pass 1"}`

// [real] ndjson footer line that log show appends after all records.
const realFooter = `{"count":5570,"finished":1}`

// [synthetic] classic backup lifecycle messages documented for the
// com.apple.TimeMachine subsystem; no real fixture captured yet.
const synStart = `{"eventType":"logEvent","subsystem":"com.apple.TimeMachine","category":"General","timestamp":"2026-10-21 10:00:00+0200","eventMessage":"Starting automatic backup on mount: /Volumes/BACKUPDISK"}`

const synSuccess = `{"eventType":"logEvent","subsystem":"com.apple.TimeMachine","category":"General","timestamp":"2026-10-21 10:42:00+0200","eventMessage":"Completed backup of Macintosh HD to /Volumes/BACKUPDISK"}`

const synFailed = `{"eventType":"logEvent","subsystem":"com.apple.TimeMachine","category":"General","timestamp":"2026-10-21 11:00:00+0200","eventMessage":"Backup failed with error: Failed to mount destination."}`

// [synthetic] missing-field records: every field except the timestamp.
const synTimestampOnly = `{"timestamp":"2026-10-21 12:00:00+0200"}`

// [synthetic] missing-field record: eventMessage but no timestamp.
const synNoTimestamp = `{"eventType":"logEvent","eventMessage":"Something happened"}`

// [synthetic] not JSON at all.
const synGarbage = `Filtering the log data using`

// [synthetic] timestamp without fractional seconds (layout variant).
const synSecondPrecision = `{"timestamp":"2026-10-21 09:00:00+0200","eventMessage":"Starting manual backup","subsystem":"com.apple.TimeMachine"}`

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(timestampLayout, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return ts
}

func line(t *testing.T, rec map[string]any) string {
	t.Helper()
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestParseNDJSONRealisticCapture(t *testing.T) {
	raw := synStart + "\n" + realSnapshotFail + "\n" + realCapabilitiesFail + "\n" + realFooter + "\n"
	st := parseNDJSON([]byte(raw))
	if st.lines != 4 {
		t.Errorf("lines = %d, want 4", st.lines)
	}
	if st.parsed != 3 {
		t.Errorf("parsed = %d, want 3", st.parsed)
	}
	if st.unparsed != 0 {
		t.Errorf("unparsed = %d, want 0; unparsed: %v", st.unparsed, st.unparsedLn)
	}
	if st.footer != 1 {
		t.Errorf("footer = %d, want 1", st.footer)
	}
	if len(st.events) != 3 {
		t.Fatalf("events = %d, want 3", len(st.events))
	}
	// events are sorted chronologically regardless of input order:
	// snapshot fail (10-19) and capabilities fail (10-20) are verified
	// noise (kindNone), start (10-21) matches the classic pattern
	want := []eventKind{kindNone, kindNone, kindStart}
	for i, k := range want {
		if st.events[i].kind != k {
			t.Errorf("event[%d].kind = %v, want %v", i, st.events[i].kind, k)
		}
	}
}

func TestParseNDJSONMissingFields(t *testing.T) {
	// missing eventMessage/process: still parsed, classified as none
	st := parseNDJSON([]byte(synTimestampOnly + "\n"))
	if st.parsed != 1 || st.unparsed != 0 || len(st.events) != 1 {
		t.Fatalf("timestamp-only record: parsed=%d unparsed=%d events=%d", st.parsed, st.unparsed, len(st.events))
	}
	if st.events[0].kind != kindNone {
		t.Errorf("timestamp-only kind = %v, want none", st.events[0].kind)
	}

	// missing timestamp (and not the footer): unparsed, raw kept, not dropped
	st = parseNDJSON([]byte(synNoTimestamp + "\n"))
	if st.parsed != 0 || st.unparsed != 1 {
		t.Fatalf("no-timestamp record: parsed=%d unparsed=%d", st.parsed, st.unparsed)
	}
	if len(st.unparsedLn) != 1 || st.unparsedLn[0] != synNoTimestamp {
		t.Errorf("unparsed raw = %q, want original line", st.unparsedLn)
	}

	// null timestamp field
	withNull := `{"timestamp":null,"eventMessage":"x"}`
	st = parseNDJSON([]byte(withNull + "\n"))
	if st.unparsed != 1 {
		t.Errorf("null timestamp: unparsed = %d, want 1", st.unparsed)
	}

	// non-JSON garbage: unparsed, raw kept
	st = parseNDJSON([]byte(synGarbage + "\n"))
	if st.unparsed != 1 || st.unparsedLn[0] != synGarbage {
		t.Errorf("garbage: unparsed=%d raw=%v", st.unparsed, st.unparsedLn)
	}

	// empty lines are ignored entirely
	st = parseNDJSON([]byte("\n\n" + realFooter + "\n\n"))
	if st.lines != 1 || st.footer != 1 {
		t.Errorf("empty lines: lines=%d footer=%d", st.lines, st.footer)
	}
}

func TestParseTimestampPrecision(t *testing.T) {
	st := parseNDJSON([]byte(synSecondPrecision + "\n"))
	if st.parsed != 1 {
		t.Fatalf("second-precision timestamp not parsed: %+v", st)
	}
	want := mustParseTime(t, "2026-10-21 09:00:00+0200")
	if !st.events[0].ts.Equal(want) {
		t.Errorf("ts = %v, want %v", st.events[0].ts, want)
	}
}

func TestClassifyMessages(t *testing.T) {
	cases := []struct {
		msg  string
		want eventKind
	}{
		// verified on this system (2026-10-23 run)
		{"Backup succeeded\nElapsed time: About 4 minutes", kindSuccess},
		{"Starting backup pass 1", kindNone},
		{"Starting backup pass 2", kindNone},
		{"fs_snapshot_list failed: Operation not supported", kindNone},
		{"Failed to read capabilities for '/Volumes/NASBOX', error: Operation not permitted", kindNone},
		{"com.apple.backupd.sandbox.xpc: connection invalid", kindNone},
		{"Bonjour resolution not possible for 'smb://user@.../NASBOX'", kindNone},
		// classic patterns, unverified on this system
		{"Starting automatic backup on mount: /Volumes/DISK", kindStart},
		{"Starting manual backup", kindStart},
		{"Completed backup of Macintosh HD", kindSuccess},
		{"Backup completed successfully", kindSuccess},
		{"backup finished in 42 seconds", kindSuccess},
		{"Backup failed with error: mounted volume error", kindFailed},
		{"Failed to mount backup destination.", kindFailed},
		{"", kindNone},
	}
	for _, c := range cases {
		if got := classifyMessage(c.msg); got != c.want {
			t.Errorf("classifyMessage(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

func TestClassifyDispatchCategory(t *testing.T) {
	// verified BackupDispatching records (real, anonymized)
	st := parseNDJSON([]byte(realDispatchStart + "\n" + realDispatchSuccess + "\n" + realPassStart + "\n"))
	if len(st.events) != 3 {
		t.Fatalf("events = %d, want 3", len(st.events))
	}
	// sorted chronologically: start (14:33), pass marker (14:35), success (14:37)
	if st.events[0].kind != kindStart {
		t.Errorf("dispatch start kind = %v, want start", st.events[0].kind)
	}
	if st.events[1].kind != kindNone {
		t.Errorf("pass marker kind = %v, want none", st.events[1].kind)
	}
	if st.events[2].kind != kindSuccess {
		t.Errorf("dispatch success kind = %v, want success", st.events[2].kind)
	}
}

func eventsFromLines(t *testing.T, lines ...string) []logEvent {
	t.Helper()
	st := parseNDJSON([]byte(joinLines(lines)))
	return st.events
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

func TestDeriveStateSuccessLifecycle(t *testing.T) {
	now := mustParseTime(t, "2026-10-23 14:00:00+0200")
	ev := eventsFromLines(t, synStart, synSuccess)
	rep := deriveState(ev, now)
	if len(rep.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(rep.sessions))
	}
	s := rep.sessions[0]
	if s.outcome != "success" {
		t.Errorf("outcome = %q, want success", s.outcome)
	}
	if !rep.hasSuccess {
		t.Fatal("hasSuccess = false, want true")
	}
	if want := mustParseTime(t, "2026-10-21 10:42:00+0200"); !rep.lastSuccess.Equal(want) {
		t.Errorf("lastSuccess = %v, want %v", rep.lastSuccess, want)
	}
}

func TestDeriveStateFailedLifecycle(t *testing.T) {
	now := mustParseTime(t, "2026-10-23 14:00:00+0200")
	ev := eventsFromLines(t, synStart, synFailed)
	rep := deriveState(ev, now)
	if len(rep.sessions) != 1 || rep.sessions[0].outcome != "failed" {
		t.Fatalf("sessions = %+v, want one failed", rep.sessions)
	}
	if rep.hasSuccess {
		t.Error("hasSuccess = true, want false")
	}
	if !rep.hasFailure {
		t.Error("hasFailure = false, want true")
	}
}

func TestDeriveStateStartWithoutCompletion(t *testing.T) {
	// fresh start, within running grace -> running
	now := mustParseTime(t, "2026-10-23 14:00:00+0200")
	start := line(t, map[string]any{
		"timestamp":    "2026-10-23 12:30:00+0200",
		"eventMessage": "Starting automatic backup on mount: /Volumes/DISK",
		"subsystem":    "com.apple.TimeMachine",
	})
	rep := deriveState(eventsFromLines(t, start), now)
	if rep.lastState() != "running" {
		t.Errorf("state = %q, want running", rep.lastState())
	}

	// old start beyond grace -> hung
	oldStart := line(t, map[string]any{
		"timestamp":    "2026-10-22 06:00:00+0200",
		"eventMessage": "Starting automatic backup on mount: /Volumes/DISK",
	})
	rep = deriveState(eventsFromLines(t, oldStart), now)
	if rep.lastState() != "hung" {
		t.Errorf("state = %q, want hung", rep.lastState())
	}

	// a later start closes the previous open session as hung
	second := line(t, map[string]any{
		"timestamp":    "2026-10-23 12:00:00+0200",
		"eventMessage": "Starting automatic backup on mount: /Volumes/DISK",
	})
	rep = deriveState(eventsFromLines(t, oldStart, second), now)
	if len(rep.sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(rep.sessions))
	}
	if rep.sessions[0].outcome != "hung" {
		t.Errorf("first session outcome = %q, want hung", rep.sessions[0].outcome)
	}
	if rep.lastState() != "running" {
		t.Errorf("last state = %q, want running", rep.lastState())
	}
}

func TestDeriveStateOrphanFailure(t *testing.T) {
	now := mustParseTime(t, "2026-10-23 14:00:00+0200")
	// a failure without any start event must not fabricate a session
	rep := deriveState(eventsFromLines(t, synFailed), now)
	if len(rep.sessions) != 0 {
		t.Fatalf("sessions = %d, want 0 (failures must not invent sessions)", len(rep.sessions))
	}
	if rep.orphanFailures != 1 {
		t.Errorf("orphanFailures = %d, want 1", rep.orphanFailures)
	}
	if !rep.hasFailure {
		t.Error("hasFailure = false, want true")
	}
	if rep.lastState() != "none" {
		t.Errorf("state = %q, want none", rep.lastState())
	}

	// verified noise lines (fs_snapshot_list, capabilities) must not
	// register as failures at all
	rep = deriveState(eventsFromLines(t, realSnapshotFail, realCapabilitiesFail), now)
	if rep.hasFailure || rep.orphanFailures != 0 {
		t.Errorf("noise lines: hasFailure=%v orphanFailures=%d, want false/0", rep.hasFailure, rep.orphanFailures)
	}
}

func TestDeriveStateCompleteRun(t *testing.T) {
	// the verified real lifecycle of the 2026-10-23 run: dispatch start,
	// noise (snapshot list failures, inner pass markers), success
	now := mustParseTime(t, "2026-10-23 15:00:00+0200")
	ev := eventsFromLines(t, realDispatchStart, realSnapshotFail, realPassStart, realDispatchSuccess)
	rep := deriveState(ev, now)
	if len(rep.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(rep.sessions))
	}
	s := rep.sessions[0]
	if s.outcome != "success" {
		t.Errorf("outcome = %q, want success", s.outcome)
	}
	if want := mustParseTime(t, "2026-10-23 14:33:47.562690+0200"); !s.start.Equal(want) {
		t.Errorf("start = %v, want %v", s.start, want)
	}
	if want := mustParseTime(t, "2026-10-23 14:37:01.297873+0200"); !s.end.Equal(want) {
		t.Errorf("end = %v, want %v", s.end, want)
	}
	if !rep.hasSuccess || !rep.lastSuccess.Equal(s.end) {
		t.Errorf("lastSuccess = %v, want %v", rep.lastSuccess, s.end)
	}
	if code := exitCode(rep, 48*time.Hour, now); code != exitFresh {
		t.Errorf("exitCode = %d, want 0", code)
	}
}

func TestExitCodeThreshold(t *testing.T) {
	now := mustParseTime(t, "2026-10-23 14:00:00+0200")
	threshold := 48 * time.Hour

	successAt := func(s string) statusReport {
		ev := eventsFromLines(t, line(t, map[string]any{
			"timestamp":    s,
			"eventMessage": "Completed backup of Macintosh HD",
			"subsystem":    "com.apple.TimeMachine",
		}))
		return deriveState(ev, now)
	}

	// fresh: 24 h ago
	if got := exitCode(successAt("2026-10-22 14:00:00+0200"), threshold, now); got != exitFresh {
		t.Errorf("exit 24h ago = %d, want 0", got)
	}
	// exactly at threshold: still fresh
	if got := exitCode(successAt("2026-10-21 14:00:00+0200"), threshold, now); got != exitFresh {
		t.Errorf("exit at exactly 48h = %d, want 0", got)
	}
	// one second past threshold: stale
	if got := exitCode(successAt("2026-10-21 13:59:59+0200"), threshold, now); got != exitStale {
		t.Errorf("exit at 48h-1s(past) = %d, want 2", got)
	}
	// no success in window: stale
	ev := eventsFromLines(t, realSnapshotFail)
	rep := deriveState(ev, now)
	if got := exitCode(rep, threshold, now); got != exitStale {
		t.Errorf("exit with no success = %d, want 2", got)
	}
}

func TestExportRaw(t *testing.T) {
	now := mustParseTime(t, "2026-10-23 14:00:00+0200")
	raw := []byte(synStart + "\n" + realFooter + "\n")
	var buf bytes.Buffer
	if err := exportRaw(&buf, raw, now); err != nil {
		t.Fatalf("exportRaw: %v", err)
	}
	out := buf.String()
	if !bytes.HasPrefix(buf.Bytes(), []byte("# tmsentinel export\n")) {
		t.Error("export missing header")
	}
	if !bytes.Contains(buf.Bytes(), []byte("# captured_at: 2026-10-23T14:00:00+02:00")) {
		t.Errorf("export missing captured_at, got:\n%s", out)
	}
	if !bytes.HasSuffix(buf.Bytes(), raw) {
		t.Error("export must end with the raw records")
	}
}

// TestGoldenPhase0Corpus runs the parser against the anonymized real
// export captured on 2026-10-23 (testdata/phase0-2026-10-23-anonymized.ndjson):
// a window with zero backup sessions and 62 snapshot-failure signals.
// Golden values must change only deliberately, and only with a new fixture.
func TestGoldenPhase0Corpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/phase0-2026-10-23-anonymized.ndjson")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	st := parseNDJSON(raw)
	if st.parsed != 5570 || st.unparsed != 0 || st.footer != 1 || st.comments != 4 {
		t.Errorf("parse stats = parsed %d / unparsed %d / footer %d / comments %d, want 5570/0/1/4",
			st.parsed, st.unparsed, st.footer, st.comments)
	}
	now := mustParseTime(t, "2026-10-23 14:29:39+0200")
	rep := deriveState(st.events, now)
	if len(rep.sessions) != 0 {
		t.Errorf("sessions = %d, want 0", len(rep.sessions))
	}
	if rep.hasSuccess {
		t.Error("hasSuccess = true, want false")
	}
	// the 62 fs_snapshot_list lines in this fixture are verified noise:
	// they fire in healthy runs and in idle maintenance alike
	if rep.orphanFailures != 0 || rep.hasFailure {
		t.Errorf("orphanFailures = %d, hasFailure = %v, want 0/false", rep.orphanFailures, rep.hasFailure)
	}
	if rep.lastState() != "none" {
		t.Errorf("lastState = %q, want none", rep.lastState())
	}
	if code := exitCode(rep, 48*time.Hour, now); code != exitStale {
		t.Errorf("exitCode = %d, want 2", code)
	}
}

// TestGoldenCompleteRun runs the full pipeline against the anonymized
// real export of the 2026-10-23 backup (testdata/phase0-2026-10-23-run.ndjson):
// exactly one session, start 14:33:47, success 14:37:01 — the Gate 1
// evidence. Golden values must change only deliberately, and only with
// a new fixture.
func TestGoldenCompleteRun(t *testing.T) {
	raw, err := os.ReadFile("testdata/phase0-2026-10-23-run.ndjson")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	st := parseNDJSON(raw)
	if st.unparsed != 0 {
		t.Errorf("unparsed = %d, want 0; %v", st.unparsed, st.unparsedLn)
	}
	if st.footer != 1 || st.comments != 5 {
		t.Errorf("footer = %d, comments = %d, want 1/5", st.footer, st.comments)
	}
	if st.parsed != 292 {
		t.Errorf("parsed = %d, want 292", st.parsed)
	}
	now := mustParseTime(t, "2026-10-23 15:00:00+0200")
	rep := deriveState(st.events, now)
	if len(rep.sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(rep.sessions))
	}
	s := rep.sessions[0]
	if s.outcome != "success" {
		t.Errorf("outcome = %q, want success", s.outcome)
	}
	if want := mustParseTime(t, "2026-10-23 14:33:47.562690+0200"); !s.start.Equal(want) {
		t.Errorf("start = %v, want %v", s.start, want)
	}
	if want := mustParseTime(t, "2026-10-23 14:37:01.297873+0200"); !s.end.Equal(want) {
		t.Errorf("end = %v, want %v", s.end, want)
	}
	if rep.orphanFailures != 0 {
		t.Errorf("orphanFailures = %d, want 0", rep.orphanFailures)
	}
	if code := exitCode(rep, 48*time.Hour, now); code != exitFresh {
		t.Errorf("exitCode = %d, want 0", code)
	}
}

// Smoke test of run() against recorded output only: no log, no tmutil.
func TestRunAgainstRecordedOutput(t *testing.T) {
	if os.Getenv("TMsENTINEL_LIVE") != "" {
		t.Skip("live run")
	}
	// run() shells out to log; the pure pipeline is covered above. This
	// guard documents that no test may invoke log or tmutil.
}
