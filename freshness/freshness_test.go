package freshness

import (
	"testing"
	"time"
)

func at(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02T15:04:05Z07:00", s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return ts
}

func TestAgeHours(t *testing.T) {
	now := at(t, "2026-10-23T14:00:00+02:00")
	then := at(t, "2026-10-21T14:00:00+02:00")
	if got := AgeHours(then, now); got != 48 {
		t.Errorf("AgeHours = %v, want 48", got)
	}
	half := at(t, "2026-10-23T13:30:00+02:00")
	if got := AgeHours(half, now); got != 0.5 {
		t.Errorf("AgeHours = %v, want 0.5", got)
	}
}

func TestFreshThresholdBoundaries(t *testing.T) {
	// mandatory boundary cases (test strategy 03)
	now := at(t, "2026-10-23T14:00:00+02:00")
	threshold := 48 * time.Hour

	// exactly at threshold: fresh
	if !Fresh(at(t, "2026-10-21T14:00:00+02:00"), threshold, now) {
		t.Error("exactly 48 h: want fresh")
	}
	// one second past threshold: not fresh
	if Fresh(at(t, "2026-10-21T13:59:59+02:00"), threshold, now) {
		t.Error("48 h - 1 s (older): want not fresh")
	}
	// one second before threshold: fresh
	if !Fresh(at(t, "2026-10-21T14:00:01+02:00"), threshold, now) {
		t.Error("one second newer than threshold: want fresh")
	}
}
