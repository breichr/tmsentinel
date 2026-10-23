// Package state derives backup sessions from the event stream.
// Pure logic: idle -> running -> success/failed/hung; no I/O, no clock
// (now is injected), nothing persisted.
package state

import (
	"sort"
	"time"

	"github.com/breichr/tmsentinel/event"
)

// RunningGrace bounds how long a start-without-completion session is
// considered running before it is reported as hung. Heuristic; to be
// tuned against the export corpus in later phases.
const RunningGrace = 6 * time.Hour

// Session is one derived backup run: a start event closed by a
// completion event, or left open (start without completion).
type Session struct {
	Start   time.Time
	End     time.Time // zero for an open session
	Outcome string    // "success", "failed", "hung", "running"
}

// Report is the derived state of the window. Pure derivation from
// the event stream; nothing is persisted.
type Report struct {
	Sessions       []Session
	LastSuccess    time.Time
	HasSuccess     bool
	OrphanFailures int
	LastFailure    time.Time
	HasFailure     bool
}

// LastState describes the state of the most recent backup in the
// window. One of: success, failed, running, hung — or none when no
// backup session was detected at all.
func (r Report) LastState() string {
	if len(r.Sessions) == 0 {
		return "none"
	}
	return r.Sessions[len(r.Sessions)-1].Outcome
}

// Running reports whether a backup session is open at the end of the
// event stream.
func (r Report) Running() bool {
	return r.LastState() == "running"
}

// Derive pairs start and completion events into sessions and derives
// the freshness facts of the window. Pure; now is injected. Input is
// sorted chronologically first, so callers may pass unsorted events.
func Derive(events []event.Event, now time.Time) Report {
	sorted := make([]event.Event, len(events))
	copy(sorted, events)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Ts.Before(sorted[j].Ts) })
	var rep Report
	var open *Session
	close := func(end time.Time, outcome string) {
		open.End = end
		open.Outcome = outcome
		rep.Sessions = append(rep.Sessions, *open)
		open = nil
	}
	for _, ev := range sorted {
		switch ev.Kind {
		case event.KindStart:
			if open != nil {
				// a new start while one is open: the previous run has no
				// completion event -> start without completion
				close(ev.Ts, "hung")
			}
			s := Session{Start: ev.Ts}
			open = &s
		case event.KindSuccess:
			if !rep.HasSuccess || ev.Ts.After(rep.LastSuccess) {
				rep.LastSuccess = ev.Ts
			}
			rep.HasSuccess = true
			if open != nil {
				close(ev.Ts, "success")
			}
		case event.KindFailed:
			if !rep.HasFailure || ev.Ts.After(rep.LastFailure) {
				rep.LastFailure = ev.Ts
			}
			rep.HasFailure = true
			if open != nil {
				close(ev.Ts, "failed")
			} else {
				rep.OrphanFailures++
			}
		}
	}
	if open != nil {
		// window ended with an open session: running, or hung if the start
		// is older than the grace period
		if now.Sub(open.Start) <= RunningGrace {
			close(now, "running")
		} else {
			close(now, "hung")
		}
	}
	return rep
}
