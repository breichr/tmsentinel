package state

import (
	"testing"
	"time"

	"github.com/breichr/tmsentinel/event"
)

func at(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02 15:04:05.999999-0700", s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return ts
}

func ev(ts time.Time, kind event.Kind) event.Event {
	return event.Event{Ts: ts, Kind: kind}
}

func TestDeriveSuccessLifecycle(t *testing.T) {
	now := at(t, "2026-10-23 15:00:00+0200")
	events := []event.Event{
		ev(at(t, "2026-10-23 14:33:47+0200"), event.KindStart),
		ev(at(t, "2026-10-23 14:37:01+0200"), event.KindSuccess),
	}
	rep := Derive(events, now)
	if len(rep.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(rep.Sessions))
	}
	if rep.Sessions[0].Outcome != "success" {
		t.Errorf("outcome = %q, want success", rep.Sessions[0].Outcome)
	}
	if !rep.HasSuccess {
		t.Fatal("hasSuccess = false, want true")
	}
	if want := at(t, "2026-10-23 14:37:01+0200"); !rep.LastSuccess.Equal(want) {
		t.Errorf("lastSuccess = %v, want %v", rep.LastSuccess, want)
	}
	if rep.LastState() != "success" || rep.Running() {
		t.Errorf("lastState = %q running = %v, want success/false", rep.LastState(), rep.Running())
	}
}

func TestDeriveFailedLifecycle(t *testing.T) {
	now := at(t, "2026-10-23 15:00:00+0200")
	events := []event.Event{
		ev(at(t, "2026-10-23 10:00:00+0200"), event.KindStart),
		ev(at(t, "2026-10-23 10:05:00+0200"), event.KindFailed),
	}
	rep := Derive(events, now)
	if len(rep.Sessions) != 1 || rep.Sessions[0].Outcome != "failed" {
		t.Fatalf("sessions = %+v, want one failed", rep.Sessions)
	}
	if rep.HasSuccess {
		t.Error("hasSuccess = true, want false")
	}
	if !rep.HasFailure {
		t.Error("hasFailure = false, want true")
	}
	if rep.OrphanFailures != 0 {
		t.Errorf("orphanFailures = %d, want 0", rep.OrphanFailures)
	}
}

func TestDeriveStartWithoutCompletion(t *testing.T) {
	// fresh start, within running grace -> running
	now := at(t, "2026-10-23 14:00:00+0200")
	rep := Derive([]event.Event{ev(at(t, "2026-10-23 12:30:00+0200"), event.KindStart)}, now)
	if rep.LastState() != "running" {
		t.Errorf("state = %q, want running", rep.LastState())
	}
	if !rep.Running() {
		t.Error("running = false, want true")
	}

	// old start beyond grace -> hung
	oldStart := at(t, "2026-10-22 06:00:00+0200")
	rep = Derive([]event.Event{ev(oldStart, event.KindStart)}, now)
	if rep.LastState() != "hung" {
		t.Errorf("state = %q, want hung", rep.LastState())
	}

	// a later start closes the previous open session as hung
	second := at(t, "2026-10-23 12:00:00+0200")
	rep = Derive([]event.Event{ev(oldStart, event.KindStart), ev(second, event.KindStart)}, now)
	if len(rep.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(rep.Sessions))
	}
	if rep.Sessions[0].Outcome != "hung" {
		t.Errorf("first session outcome = %q, want hung", rep.Sessions[0].Outcome)
	}
	if rep.LastState() != "running" {
		t.Errorf("last state = %q, want running", rep.LastState())
	}
}

func TestDeriveOrphanFailure(t *testing.T) {
	now := at(t, "2026-10-23 15:00:00+0200")
	// a failure without any start event must not fabricate a session
	rep := Derive([]event.Event{ev(at(t, "2026-10-23 09:00:00+0200"), event.KindFailed)}, now)
	if len(rep.Sessions) != 0 {
		t.Fatalf("sessions = %d, want 0 (failures must not invent sessions)", len(rep.Sessions))
	}
	if rep.OrphanFailures != 1 {
		t.Errorf("orphanFailures = %d, want 1", rep.OrphanFailures)
	}
	if !rep.HasFailure {
		t.Error("hasFailure = false, want true")
	}
	if rep.LastState() != "none" {
		t.Errorf("state = %q, want none", rep.LastState())
	}
}

func TestDeriveEmpty(t *testing.T) {
	now := at(t, "2026-10-23 15:00:00+0200")
	rep := Derive(nil, now)
	if rep.LastState() != "none" || rep.HasSuccess || rep.HasFailure || rep.Running() {
		t.Errorf("empty window: state=%q hasSuccess=%v hasFailure=%v running=%v, want none/false/false/false",
			rep.LastState(), rep.HasSuccess, rep.HasFailure, rep.Running())
	}
}

func TestDeriveOutOfOrderEvents(t *testing.T) {
	// Derive sorts its input, so unsorted slices derive the same state
	now := at(t, "2026-10-23 15:00:00+0200")
	sorted := []event.Event{
		ev(at(t, "2026-10-23 10:00:00+0200"), event.KindStart),
		ev(at(t, "2026-10-23 10:30:00+0200"), event.KindSuccess),
	}
	shuffled := []event.Event{sorted[1], sorted[0]}
	for name, in := range map[string][]event.Event{"sorted": sorted, "shuffled": shuffled} {
		rep := Derive(in, now)
		if rep.LastState() != "success" {
			t.Errorf("%s: lastState = %q, want success", name, rep.LastState())
		}
		if !rep.LastSuccess.Equal(sorted[1].Ts) {
			t.Errorf("%s: lastSuccess = %v, want %v", name, rep.LastSuccess, sorted[1].Ts)
		}
	}
}
