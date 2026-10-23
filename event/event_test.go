package event

// Fixture lines marked [real] are anonymized records captured from this
// system's Unified Log on 2026-10-23. Lines marked [synthetic] are
// constructed edge cases that have no real export yet.

import (
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

// [synthetic] missing-field record: every field except the timestamp.
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

func TestParseNDJSONRealisticCapture(t *testing.T) {
	raw := synStart + "\n" + realSnapshotFail + "\n" + realCapabilitiesFail + "\n" + realFooter + "\n"
	st := Parse([]byte(raw))
	if st.Lines != 4 {
		t.Errorf("lines = %d, want 4", st.Lines)
	}
	if st.Parsed != 3 {
		t.Errorf("parsed = %d, want 3", st.Parsed)
	}
	if st.Unparsed != 0 {
		t.Errorf("unparsed = %d, want 0; unparsed: %v", st.Unparsed, st.UnparsedLines)
	}
	if st.Footer != 1 {
		t.Errorf("footer = %d, want 1", st.Footer)
	}
	if len(st.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(st.Events))
	}
	// events are sorted chronologically regardless of input order:
	// snapshot fail (10-19) and capabilities fail (10-20) are verified
	// noise (KindNone), start (10-21) matches the classic pattern
	want := []Kind{KindNone, KindNone, KindStart}
	for i, k := range want {
		if st.Events[i].Kind != k {
			t.Errorf("event[%d].kind = %v, want %v", i, st.Events[i].Kind, k)
		}
	}
}

func TestParseNDJSONMissingFields(t *testing.T) {
	// missing eventMessage/process: still parsed, classified as none
	st := Parse([]byte(synTimestampOnly + "\n"))
	if st.Parsed != 1 || st.Unparsed != 0 || len(st.Events) != 1 {
		t.Fatalf("timestamp-only record: parsed=%d unparsed=%d events=%d", st.Parsed, st.Unparsed, len(st.Events))
	}
	if st.Events[0].Kind != KindNone {
		t.Errorf("timestamp-only kind = %v, want none", st.Events[0].Kind)
	}

	// missing timestamp (and not the footer): unparsed, raw kept
	st = Parse([]byte(synNoTimestamp + "\n"))
	if st.Parsed != 0 || st.Unparsed != 1 {
		t.Fatalf("no-timestamp record: parsed=%d unparsed=%d", st.Parsed, st.Unparsed)
	}
	if len(st.UnparsedLines) != 1 || st.UnparsedLines[0] != synNoTimestamp {
		t.Errorf("unparsed raw = %q, want original line", st.UnparsedLines)
	}

	// null timestamp field
	withNull := `{"timestamp":null,"eventMessage":"x"}`
	st = Parse([]byte(withNull + "\n"))
	if st.Unparsed != 1 {
		t.Errorf("null timestamp: unparsed = %d, want 1", st.Unparsed)
	}

	// non-JSON garbage: unparsed, raw kept
	st = Parse([]byte(synGarbage + "\n"))
	if st.Unparsed != 1 || st.UnparsedLines[0] != synGarbage {
		t.Errorf("garbage: unparsed=%d raw=%v", st.Unparsed, st.UnparsedLines)
	}

	// empty lines are ignored entirely
	st = Parse([]byte("\n\n" + realFooter + "\n\n"))
	if st.Lines != 1 || st.Footer != 1 {
		t.Errorf("empty lines: lines=%d footer=%d", st.Lines, st.Footer)
	}
}

func TestParseTimestampPrecision(t *testing.T) {
	st := Parse([]byte(synSecondPrecision + "\n"))
	if st.Parsed != 1 {
		t.Fatalf("second-precision timestamp not parsed: %+v", st)
	}
	want := mustParseTime(t, "2026-10-21 09:00:00+0200")
	if !st.Events[0].Ts.Equal(want) {
		t.Errorf("ts = %v, want %v", st.Events[0].Ts, want)
	}
}

func TestClassifyMessages(t *testing.T) {
	cases := []struct {
		msg  string
		want Kind
	}{
		// verified on this system (2026-10-23 run)
		{"Backup succeeded\nElapsed time: About 4 minutes", KindSuccess},
		{"Starting backup pass 1", KindNone},
		{"Starting backup pass 2", KindNone},
		{"fs_snapshot_list failed: Operation not supported", KindNone},
		{"Failed to read capabilities for '/Volumes/NASBOX', error: Operation not permitted", KindNone},
		{"com.apple.backupd.sandbox.xpc: connection invalid", KindNone},
		{"Bonjour resolution not possible for 'smb://user@.../NASBOX'", KindNone},
		// classic patterns, unverified on this system
		{"Starting automatic backup on mount: /Volumes/DISK", KindStart},
		{"Starting manual backup", KindStart},
		{"Completed backup of Macintosh HD", KindSuccess},
		{"Backup completed successfully", KindSuccess},
		{"backup finished in 42 seconds", KindSuccess},
		{"Backup failed with error: mounted volume error", KindFailed},
		{"Failed to mount backup destination.", KindFailed},
		{"", KindNone},
	}
	for _, c := range cases {
		if got := ClassifyMessage(c.msg); got != c.want {
			t.Errorf("ClassifyMessage(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

func TestClassifyDispatchCategory(t *testing.T) {
	// verified BackupDispatching records (real, anonymized); sorted
	// chronologically: start (14:33), pass marker (14:35), success (14:37)
	st := Parse([]byte(realDispatchStart + "\n" + realDispatchSuccess + "\n" + realPassStart + "\n"))
	if len(st.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(st.Events))
	}
	if st.Events[0].Kind != KindStart {
		t.Errorf("dispatch start kind = %v, want start", st.Events[0].Kind)
	}
	if st.Events[1].Kind != KindNone {
		t.Errorf("pass marker kind = %v, want none", st.Events[1].Kind)
	}
	if st.Events[2].Kind != KindSuccess {
		t.Errorf("dispatch success kind = %v, want success", st.Events[2].Kind)
	}
}

// TestGoldenPhase0Corpus runs the parser against the anonymized real
// export captured on 2026-10-23 (testdata/phase0-2026-10-23-anonymized.ndjson):
// a default-level window with zero backup sessions and 62 verified
// noise signals. Golden values must change only deliberately, and only
// with a new fixture.
func TestGoldenPhase0Corpus(t *testing.T) {
	raw, err := os.ReadFile("../testdata/phase0-2026-10-23-anonymized.ndjson")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	st := Parse(raw)
	if st.Parsed != 5570 || st.Unparsed != 0 || st.Footer != 1 || st.Comments != 4 {
		t.Errorf("parse stats = parsed %d / unparsed %d / footer %d / comments %d, want 5570/0/1/4",
			st.Parsed, st.Unparsed, st.Footer, st.Comments)
	}
	var starts, successes int
	for _, ev := range st.Events {
		switch ev.Kind {
		case KindStart:
			starts++
		case KindSuccess:
			successes++
		}
	}
	if starts != 0 || successes != 0 {
		t.Errorf("default-level window: starts=%d successes=%d, want 0/0 (lifecycle is Info-level)", starts, successes)
	}
}

// TestGoldenCompleteRun runs the parser against the anonymized real
// export of the 2026-10-23 backup (testdata/phase0-2026-10-23-run.ndjson):
// the Gate 1 evidence — exactly one start and one success event.
func TestGoldenCompleteRun(t *testing.T) {
	raw, err := os.ReadFile("../testdata/phase0-2026-10-23-run.ndjson")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	st := Parse(raw)
	if st.Unparsed != 0 {
		t.Errorf("unparsed = %d, want 0; %v", st.Unparsed, st.UnparsedLines)
	}
	if st.Footer != 1 || st.Comments != 5 {
		t.Errorf("footer = %d, comments = %d, want 1/5", st.Footer, st.Comments)
	}
	if st.Parsed != 292 {
		t.Errorf("parsed = %d, want 292", st.Parsed)
	}
	wantStart := mustParseTime(t, "2026-10-23 14:33:47.562690+0200")
	wantSuccess := mustParseTime(t, "2026-10-23 14:37:01.297873+0200")
	var gotStart, gotSuccess int
	for _, ev := range st.Events {
		switch ev.Kind {
		case KindStart:
			gotStart++
			if !ev.Ts.Equal(wantStart) {
				t.Errorf("start event at %v, want %v", ev.Ts, wantStart)
			}
		case KindSuccess:
			gotSuccess++
			if !ev.Ts.Equal(wantSuccess) {
				t.Errorf("success event at %v, want %v", ev.Ts, wantSuccess)
			}
		}
	}
	if gotStart != 1 || gotSuccess != 1 {
		t.Errorf("start/success events = %d/%d, want 1/1", gotStart, gotSuccess)
	}
}
