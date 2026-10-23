package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/breichr/tmsentinel/destinations"
	"github.com/breichr/tmsentinel/event"
)

// fakeLog is a canned event.Source. It can also describe its query
// like event.UnifiedLog does.
type fakeLog struct {
	raw  []byte
	err  error
	args []string
}

func (f fakeLog) Show() ([]byte, error) { return f.raw, f.err }
func (f fakeLog) Args() []string        { return f.args }
func (f fakeLog) CommandLine() string   { return "/usr/bin/log " + strings.Join(f.args, " ") }

type fakeDest struct {
	text string
	err  error
}

func (f fakeDest) Info() ([]byte, error) { return []byte(f.text), f.err }

func mountedOn(paths ...string) destinations.MountChecker {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return func(mp string) bool { return set[mp] }
}

const realInfoMounted = `====================================================
Name          : TimeMachine
Kind          : Local
Mount Point   : /Volumes/TimeMachine
ID            : 8F11C3F6-D15E-42D4-91B2-DD75A321E475
`

const realInfoNone = `No destinations configured.
`

// [real] anonymized lifecycle lines of the 2026-10-23 run; see
// event/event_test.go for provenance.
const realRun = `{"eventType":"logEvent","subsystem":"com.apple.TimeMachine","category":"BackupDispatching","processImagePath":"/System/Library/CoreServices/TimeMachine/backupd","timestamp":"2026-10-23 14:33:47.562690+0200","eventMessage":"Backup requested to next destination in rotation. specifiedOptions: TMBackupOptions(rawValue: 1)"}
{"eventType":"logEvent","subsystem":"com.apple.TimeMachine","category":"BackupDispatching","processImagePath":"/System/Library/CoreServices/TimeMachine/backupd","timestamp":"2026-10-23 14:37:01.297873+0200","eventMessage":"Backup succeeded\nElapsed time: About 4 minutes"}
{"count":2,"finished":1}
`

func now(t *testing.T) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02 15:04:05-0700", "2026-10-23 16:00:00+0200")
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func runWith(src Sources, args []string, t *testing.T) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(src, args, &out, &errb, now(t))
	return code, out.String(), errb.String()
}

func TestCheckFresh(t *testing.T) {
	src := Sources{Log: fakeLog{raw: []byte(realRun)}, Dest: fakeDest{text: realInfoMounted}, Mount: mountedOn("/Volumes/TimeMachine")}
	code, out, errb := runWith(src, []string{"check"}, t)
	if code != ExitFresh {
		t.Errorf("exit = %d, want 0; out=%q err=%q", code, out, errb)
	}
	if !strings.Contains(out, "state: success") || !strings.Contains(out, "result: fresh") {
		t.Errorf("output missing expected lines:\n%s", out)
	}
	if !strings.Contains(out, "reachable") {
		t.Errorf("output missing destination reachability:\n%s", out)
	}
}

func TestCheckStale(t *testing.T) {
	old := `{"timestamp":"2026-10-19 14:37:01.297873+0200","eventMessage":"Backup succeeded","category":"BackupDispatching","subsystem":"com.apple.TimeMachine"}
{"count":1,"finished":1}
`
	src := Sources{Log: fakeLog{raw: []byte(old)}, Dest: fakeDest{text: realInfoMounted}, Mount: mountedOn("/Volumes/TimeMachine")}
	code, out, _ := runWith(src, []string{"check"}, t)
	if code != ExitStale {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(out, "stale") {
		t.Errorf("output missing stale:\n%s", out)
	}
}

func TestCheckNoSuccessUnreachable(t *testing.T) {
	// no backup events and destination not mounted -> exit 4
	empty := `{"count":0,"finished":1}
`
	src := Sources{Log: fakeLog{raw: []byte(empty)}, Dest: fakeDest{text: realInfoMounted}, Mount: mountedOn()}
	code, out, _ := runWith(src, []string{"check"}, t)
	if code != ExitUnreachable {
		t.Errorf("exit = %d, want 4", code)
	}
	if !strings.Contains(out, "unreachable") {
		t.Errorf("output missing unreachable:\n%s", out)
	}
}

func TestCheckNoSuccessReachable(t *testing.T) {
	// no backup events but destination mounted -> exit 2
	empty := `{"count":0,"finished":1}
`
	src := Sources{Log: fakeLog{raw: []byte(empty)}, Dest: fakeDest{text: realInfoMounted}, Mount: mountedOn("/Volumes/TimeMachine")}
	code, out, _ := runWith(src, []string{"check"}, t)
	if code != ExitStale {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(out, "no successful backup in window") {
		t.Errorf("output missing result:\n%s", out)
	}
}

func TestCheckNoDestination(t *testing.T) {
	src := Sources{Log: fakeLog{raw: []byte(realRun)}, Dest: fakeDest{text: realInfoNone}, Mount: mountedOn()}
	code, out, _ := runWith(src, []string{"check"}, t)
	if code != ExitNoDestination {
		t.Errorf("exit = %d, want 3", code)
	}
	if !strings.Contains(out, "no destination configured") {
		t.Errorf("output missing result:\n%s", out)
	}
}

func TestCheckRunning(t *testing.T) {
	// open session within grace: exit 0 with hint, running true in json
	running := `{"timestamp":"2026-10-23 15:30:00+0200","eventMessage":"Backup requested to next destination in rotation. specifiedOptions: 1","category":"BackupDispatching","subsystem":"com.apple.TimeMachine"}
{"count":1,"finished":1}
`
	src := Sources{Log: fakeLog{raw: []byte(running)}, Dest: fakeDest{text: realInfoMounted}, Mount: mountedOn("/Volumes/TimeMachine")}
	code, out, _ := runWith(src, []string{"check"}, t)
	if code != ExitFresh {
		t.Errorf("exit = %d, want 0 (running is healthy, hint only)", code)
	}
	if !strings.Contains(out, "running") {
		t.Errorf("output missing running hint:\n%s", out)
	}

	_, jout, _ := runWith(src, []string{"check", "--json"}, t)
	if !strings.Contains(jout, `"running": true`) {
		t.Errorf("json missing running:true:\n%s", jout)
	}
	if !strings.Contains(jout, `"state": "running"`) {
		t.Errorf("json missing state running:\n%s", jout)
	}
	if !strings.Contains(jout, `"config_loaded": false`) {
		t.Errorf("json missing config_loaded:\n%s", jout)
	}
	if !strings.Contains(jout, `"threshold_h": 48`) {
		t.Errorf("json missing threshold:\n%s", jout)
	}
}

func TestCheckJSONSchema(t *testing.T) {
	src := Sources{Log: fakeLog{raw: []byte(realRun)}, Dest: fakeDest{text: realInfoMounted}, Mount: mountedOn("/Volumes/TimeMachine")}
	code, out, _ := runWith(src, []string{"check", "--json"}, t)
	if code != ExitFresh {
		t.Errorf("exit = %d, want 0", code)
	}
	for _, want := range []string{`"state"`, `"last_success_age_h"`, `"threshold_h"`, `"destinations"`, `"diagnosis"`, `"running"`, `"config_loaded"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json schema missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, `"last_success_age_h": null`) {
		t.Errorf("fresh run must report an age, got null:\n%s", out)
	}
}

func TestCheckInternalError(t *testing.T) {
	src := Sources{Log: fakeLog{err: os.ErrDeadlineExceeded}, Dest: fakeDest{text: realInfoMounted}, Mount: mountedOn()}
	code, _, errb := runWith(src, []string{"check"}, t)
	if code != ExitError {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb, "i/o timeout") {
		t.Errorf("stderr = %q", errb)
	}

	src.Dest = fakeDest{err: os.ErrPermission}
	code, _, _ = runWith(src, []string{"check"}, t)
	if code != ExitError {
		t.Errorf("destinationinfo error: exit = %d, want 1", code)
	}
}

func TestCheckThresholdFlag(t *testing.T) {
	old := `{"timestamp":"2026-10-23 01:00:00+0200","eventMessage":"Backup succeeded","category":"BackupDispatching","subsystem":"com.apple.TimeMachine"}
{"count":1,"finished":1}
`
	// 15 h old: fresh with default 48, stale with --threshold 10
	src := Sources{Log: fakeLog{raw: []byte(old)}, Dest: fakeDest{text: realInfoMounted}, Mount: mountedOn("/Volumes/TimeMachine")}
	if code, _, _ := runWith(src, []string{"check"}, t); code != ExitFresh {
		t.Errorf("default threshold: exit = %d, want 0", code)
	}
	if code, _, _ := runWith(src, []string{"check", "--threshold", "10"}, t); code != ExitStale {
		t.Errorf("threshold 10: exit = %d, want 2", code)
	}
	if code, _, _ := runWith(src, []string{"check", "--threshold", "0"}, t); code != ExitError {
		t.Errorf("threshold 0: exit = %d, want 1", code)
	}
}

func TestDestinationsCommand(t *testing.T) {
	src := Sources{Log: fakeLog{}, Dest: fakeDest{text: realInfoMounted}, Mount: mountedOn("/Volumes/TimeMachine")}
	code, out, _ := runWith(src, []string{"destinations"}, t)
	if code != ExitFresh {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "TimeMachine\tLocal\t/Volumes/TimeMachine\treachable") {
		t.Errorf("output = %q", out)
	}

	_, jout, _ := runWith(src, []string{"destinations", "--json"}, t)
	for _, want := range []string{`"Name"`, `"Kind"`, `"MountPoint"`, `"ID"`} {
		if !strings.Contains(jout, want) {
			t.Errorf("json missing %s:\n%s", want, jout)
		}
	}

	src.Mount = mountedOn()
	code, out, _ = runWith(src, []string{"destinations"}, t)
	if code != ExitFresh {
		t.Errorf("unreachable destinations listing: exit = %d, want 0", code)
	}
	if !strings.Contains(out, "unreachable") {
		t.Errorf("output = %q", out)
	}

	src.Dest = fakeDest{text: realInfoNone}
	code, out, _ = runWith(src, []string{"destinations"}, t)
	if code != ExitNoDestination || !strings.Contains(out, "no destinations") {
		t.Errorf("no destinations: exit=%d out=%q", code, out)
	}
}

func TestExportCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corpus.ndjson")
	src := Sources{Log: fakeLog{raw: []byte(realRun), args: []string{"show", "--style", "ndjson", "--info", "--last", "7d", "--predicate", "x"}}, Dest: fakeDest{}, Mount: mountedOn()}
	code, out, _ := runWith(src, []string{"export", path}, t)
	if code != ExitFresh {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, "exported 2 parsed") {
		t.Errorf("output = %q", out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "# tmsentinel export\n") {
		t.Errorf("missing header:\n%s", text[:100])
	}
	if !strings.Contains(text, "# captured_at: 2026-10-23T16:00:00+02:00") {
		t.Errorf("missing captured_at:\n%s", text[:200])
	}
	if !strings.Contains(text, "Backup requested to next destination") {
		t.Errorf("raw records missing:\n%s", text)
	}

	// missing file argument -> usage error
	if code, _, _ := runWith(src, []string{"export"}, t); code != ExitError {
		t.Errorf("export without file: exit = %d, want 1", code)
	}
	// unwritable path -> internal error
	if code, _, _ := runWith(src, []string{"export", "/nonexistent-dir-xyz/corpus.ndjson"}, t); code != ExitError {
		t.Errorf("export to bad path: exit = %d, want 1", code)
	}
}

func TestVersionAndUsage(t *testing.T) {
	src := Sources{}
	code, out, _ := runWith(src, []string{"version"}, t)
	if code != ExitFresh || !strings.HasPrefix(out, "tmsentinel ") {
		t.Errorf("version: exit=%d out=%q", code, out)
	}
	if code, _, _ := runWith(src, nil, t); code != ExitError {
		t.Errorf("no args: exit = %d, want 1", code)
	}
	if code, _, _ := runWith(src, []string{"frobnicate"}, t); code != ExitError {
		t.Errorf("unknown subcommand: exit = %d, want 1", code)
	}
	if code, out, _ := runWith(src, []string{"help"}, t); code != ExitFresh || !strings.Contains(out, "Usage:") {
		t.Errorf("help: exit=%d out=%q", code, out)
	}
}

var _ event.Source = fakeLog{}
var _ destinations.Source = fakeDest{}
