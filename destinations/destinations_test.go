package destinations

import (
	"strings"
	"testing"
)

// [real] anonymized: tmutil destinationinfo output captured on this
// system 2026-10-23, destination mounted.
const realInfoMounted = `====================================================
Name          : TimeMachine
Kind          : Local
Mount Point   : /Volumes/TimeMachine
ID            : 8F11C3F6-D15E-42D4-91B2-DD75A321E475
`

// [real] anonymized: same destination, not mounted (no Mount Point line,
// as observed 2026-10-23 before the SSD was connected).
const realInfoUnmounted = `====================================================
Name          : TimeMachine
Kind          : Local
ID            : 8F11C3F6-D15E-42D4-91B2-DD75A321E475
`

// [synthetic] two destinations, unknown extra line, block without a
// Name — parser must skip what it does not understand, never guess.
const synTwoDest = `====================================================
Name          : First
Kind          : Local
Mount Point   : /Volumes/First
ID            : AAAA
Some Future Field : unknown value
====================================================
Name          : Second
Kind          : Network
ID            : BBBB
====================================================
Kind          : Orphan
ID            : CCCC
`

func TestParseInfoSingle(t *testing.T) {
	dests := ParseInfo(realInfoMounted)
	if len(dests) != 1 {
		t.Fatalf("dests = %d, want 1", len(dests))
	}
	d := dests[0]
	if d.Name != "TimeMachine" || d.Kind != "Local" || d.MountPoint != "/Volumes/TimeMachine" || d.ID != "8F11C3F6-D15E-42D4-91B2-DD75A321E475" {
		t.Errorf("parsed destination = %+v", d)
	}

	dests = ParseInfo(realInfoUnmounted)
	if len(dests) != 1 || dests[0].MountPoint != "" {
		t.Errorf("unmounted: dests = %+v, want one without mount point", dests)
	}
}

func TestParseInfoRobustness(t *testing.T) {
	dests := ParseInfo(synTwoDest)
	if len(dests) != 2 {
		t.Fatalf("dests = %d, want 2 (orphan block skipped)", len(dests))
	}
	if dests[0].Name != "First" || dests[0].Kind != "Local" || dests[0].MountPoint != "/Volumes/First" || dests[0].ID != "AAAA" {
		t.Errorf("first = %+v", dests[0])
	}
	if dests[1].Name != "Second" || dests[1].Kind != "Network" || dests[1].MountPoint != "" || dests[1].ID != "BBBB" {
		t.Errorf("second = %+v", dests[1])
	}

	if got := ParseInfo(""); len(got) != 0 {
		t.Errorf("empty input: dests = %d, want 0", len(got))
	}
	if got := ParseInfo("some random text\nwithout structure"); len(got) != 0 {
		t.Errorf("garbage input: dests = %d, want 0", len(got))
	}
}

func TestReachable(t *testing.T) {
	mounted := func(mp string) bool { return mp == "/up" }
	d := Destination{Name: "X", Kind: "Local", MountPoint: "/up"}
	if !Reachable(d, mounted) {
		t.Error("mounted destination reported unreachable")
	}
	d.MountPoint = "/down"
	if Reachable(d, mounted) {
		t.Error("unmounted destination reported reachable")
	}
	d.MountPoint = ""
	if Reachable(d, mounted) {
		t.Error("destination without mount point reported reachable")
	}
	if Reachable(d, nil) {
		t.Error("nil checker reported reachable")
	}
}

func TestAnyReachable(t *testing.T) {
	mounted := func(mp string) bool { return mp == "/up" }
	if AnyReachable([]Destination{{MountPoint: "/down"}, {MountPoint: "/up"}}, mounted) != true {
		t.Error("want true with one reachable")
	}
	if AnyReachable([]Destination{{MountPoint: "/down"}}, mounted) != false {
		t.Error("want false with none reachable")
	}
	if AnyReachable(nil, mounted) != false {
		t.Error("want false with no destinations")
	}
}

func TestDefaultCheckerOnRealVolume(t *testing.T) {
	// StatfsMounted is the production checker; on darwin it must not
	// panic and must answer for the root filesystem.
	if !StatfsMounted("/") {
		t.Error("root filesystem reported not mounted")
	}
	if StatfsMounted("/nonexistent-volume-xyz") {
		t.Error("nonexistent path reported mounted")
	}
}

func TestTMUtilArgs(t *testing.T) {
	tm := DefaultTMUtil()
	if tm.Binary == "" || !strings.HasSuffix(tm.Binary, "tmutil") {
		t.Errorf("default binary = %q", tm.Binary)
	}
}
