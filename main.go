// Command tmsentinel is the Phase 0 prototype of a stateless, FDA-free
// Time Machine monitoring CLI. It answers: is my Time Machine backup
// really running — and if not, why not?
//
// Phase 0 scope: prove the FDA-free core. It runs
//
//	log show --style ndjson --info --last 7d \
//	  --predicate 'subsystem == "com.apple.TimeMachine"'
//
// as a subprocess (the --info level is required: backupd logs its
// lifecycle signals — BackupDispatching etc. — at Info, not Default), pairs backup start and completion events
// (idle -> running -> success/failed), and reports the timestamp and age
// of the last successful backup. Nothing is persisted; every run derives
// state fresh from the event stream.
//
// Read-only: it never starts, stops, or configures backups.
//
// Exit codes (stable contract):
//
//	0  last successful backup within the freshness threshold
//	2  last successful backup older than the threshold, or none found
//	1  internal error (subprocess failure, bad flags, export write error)
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	exitFresh = 0
	exitError = 1
	exitStale = 2

	logBinaryPath = "/usr/bin/log"
	logWindow     = "7d"
	logPredicate  = `subsystem == "com.apple.TimeMachine"`

	// timestampLayout matches log show --style ndjson timestamps such as
	// "2026-10-19 13:24:07.365874+0200"; the fraction is optional in parse.
	timestampLayout = "2006-01-02 15:04:05.999999-0700"

	defaultThresholdHours = 48.0

	// runningGrace bounds how long a start-without-completion session is
	// considered running before it is reported as hung. Heuristic; to be
	// tuned against the export corpus in later phases.
	runningGrace = 6 * time.Hour
)

// eventKind classifies a Unified Log record by its meaning for backups.
type eventKind int

const (
	kindNone    eventKind = iota // parsed record, no backup signal
	kindStart                    // backup started
	kindSuccess                  // backup completed successfully
	kindFailed                   // backup attempt failed
)

func (k eventKind) String() string {
	switch k {
	case kindStart:
		return "start"
	case kindSuccess:
		return "success"
	case kindFailed:
		return "failed"
	default:
		return "none"
	}
}

// logEvent is a parsed record placed on the timeline.
type logEvent struct {
	ts      time.Time
	kind    eventKind
	message string
	process string
}

// logRecord is the subset of the log show --style ndjson schema we need.
// All fields are pointers: the parser must be robust against missing or
// null fields, never crash, and never silently guess.
type logRecord struct {
	Timestamp        *string `json:"timestamp"`
	EventMessage     *string `json:"eventMessage"`
	EventType        *string `json:"eventType"`
	Category         *string `json:"category"`
	ProcessImagePath *string `json:"processImagePath"`
	// Finished is set on the ndjson footer line ({"count":N,"finished":1})
	// that log show appends; such lines are not records.
	Finished *int `json:"finished"`
}

// Classification. Verified on this system 2026-10-23 (macOS, local APFS
// destination, complete run captured):
//
//	start:    category BackupDispatching, "Backup requested to next
//	          destination in rotation. ..."
//	success:  category BackupDispatching, "Backup succeeded\nElapsed
//	          time: About N minutes"
//	noise:    "fs_snapshot_list failed" fires during healthy runs
//	          (and in idle maintenance) — not a failure signal
//	noise:    "Starting backup pass N" (FileProtection) marks inner
//	          passes of one running backup, not session starts
//
// Classic patterns documented for older macOS releases are kept and
// marked unverified here; their status is tracked in the README.
var (
	startPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)starting (automatic|manual) backup`), // classic, unverified here
	}
	successPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)backup succeeded`), // verified on this system
		regexp.MustCompile(`(?i)completed backup`), // classic, unverified here
		regexp.MustCompile(`(?i)backup completed`), // classic, unverified here
		regexp.MustCompile(`(?i)backup finished`),  // classic, unverified here
	}
	failedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)backup failed`),             // expected shape, unverified here
		regexp.MustCompile(`(?i)failed to complete backup`), // classic, unverified here
		regexp.MustCompile(`(?i)backup aborted`),            // classic, unverified here
		regexp.MustCompile(`(?i)failed to mount (backup )?destination`),
	}
	// noisePatterns fire inside healthy backup runs; they must never be
	// mistaken for session starts or failures.
	noisePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)starting backup pass`),    // verified on this system
		regexp.MustCompile(`(?i)fs_snapshot_list failed`), // verified on this system
	}
)

func classifyMessage(msg string) eventKind {
	for _, re := range noisePatterns {
		if re.MatchString(msg) {
			return kindNone
		}
	}
	for _, re := range startPatterns {
		if re.MatchString(msg) {
			return kindStart
		}
	}
	for _, re := range successPatterns {
		if re.MatchString(msg) {
			return kindSuccess
		}
	}
	for _, re := range failedPatterns {
		if re.MatchString(msg) {
			return kindFailed
		}
	}
	return kindNone
}

func classify(rec *logRecord) eventKind {
	// BackupDispatching (category, from backupd) carries the verified
	// start ("Backup requested ...") and completion ("Backup succeeded
	// ...") signals. Other processes (e.g. tmutil itself) log to this
	// subsystem too, so match on the message content, not category alone.
	if rec.EventMessage != nil {
		lower := strings.ToLower(*rec.EventMessage)
		if rec.Category != nil && strings.EqualFold(*rec.Category, "BackupDispatching") {
			if strings.Contains(lower, "backup requested") {
				return kindStart
			}
			if strings.Contains(lower, "backup succeeded") {
				return kindSuccess
			}
		}
		return classifyMessage(*rec.EventMessage)
	}
	return kindNone
}

// parseStats summarizes one parse run.
type parseStats struct {
	lines      int      // non-empty input lines seen
	parsed     int      // lines decoded and placed on the timeline
	unparsed   int      // lines that could not be decoded or timestamped
	comments   int      // '#' header lines (export metadata)
	footer     int      // log show ndjson footer lines (known non-records)
	unparsedLn []string // raw content of unparsed lines
	events     []logEvent
}

// parseNDJSON parses raw log show --style ndjson output line by line.
// It is pure: no I/O, no clock. Every unparsed line is collected with its
// raw content so callers can log it; records are never silently dropped.
func parseNDJSON(raw []byte) parseStats {
	var st parseStats
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		st.lines++
		if strings.HasPrefix(line, "#") {
			// export header comment (see exportRaw)
			st.comments++
			continue
		}
		var rec logRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			st.unparsed++
			st.unparsedLn = append(st.unparsedLn, line)
			continue
		}
		if rec.Timestamp == nil {
			if rec.Finished != nil {
				// footer line of log show --style ndjson output
				st.footer++
				continue
			}
			st.unparsed++
			st.unparsedLn = append(st.unparsedLn, line)
			continue
		}
		ts, err := time.Parse(timestampLayout, *rec.Timestamp)
		if err != nil {
			st.unparsed++
			st.unparsedLn = append(st.unparsedLn, line)
			continue
		}
		ev := logEvent{ts: ts, kind: classify(&rec)}
		if rec.EventMessage != nil {
			ev.message = *rec.EventMessage
		}
		if rec.ProcessImagePath != nil {
			ev.process = *rec.ProcessImagePath
		}
		st.parsed++
		st.events = append(st.events, ev)
	}
	sort.SliceStable(st.events, func(i, j int) bool { return st.events[i].ts.Before(st.events[j].ts) })
	return st
}

// backupSession is one derived backup run: a start event closed by a
// completion event, or left open (start without completion).
type backupSession struct {
	start   time.Time
	end     time.Time // zero for an open session
	outcome string    // "success", "failed", "hung", "running"
}

// statusReport is the derived state of the window. Pure derivation from
// the event stream; nothing is persisted.
type statusReport struct {
	sessions       []backupSession
	lastSuccess    time.Time
	hasSuccess     bool
	orphanFailures int
	lastFailure    time.Time
	hasFailure     bool
}

// deriveState pairs start and completion events into sessions and derives
// the freshness facts of the window. Pure; now is injected.
func deriveState(events []logEvent, now time.Time) statusReport {
	var rep statusReport
	var open *backupSession
	close := func(end time.Time, outcome string) {
		open.end = end
		open.outcome = outcome
		rep.sessions = append(rep.sessions, *open)
		open = nil
	}
	for _, ev := range events {
		switch ev.kind {
		case kindStart:
			if open != nil {
				// a new start while one is open: the previous run has no
				// completion event -> start without completion
				close(ev.ts, "hung")
			}
			s := backupSession{start: ev.ts}
			open = &s
		case kindSuccess:
			if !rep.hasSuccess || ev.ts.After(rep.lastSuccess) {
				rep.lastSuccess = ev.ts
			}
			rep.hasSuccess = true
			if open != nil {
				close(ev.ts, "success")
			}
		case kindFailed:
			if !rep.hasFailure || ev.ts.After(rep.lastFailure) {
				rep.lastFailure = ev.ts
			}
			rep.hasFailure = true
			if open != nil {
				close(ev.ts, "failed")
			} else {
				rep.orphanFailures++
			}
		}
	}
	if open != nil {
		// window ended with an open session: running, or hung if the start
		// is older than the grace period
		if now.Sub(open.start) <= runningGrace {
			close(now, "running")
		} else {
			close(now, "hung")
		}
	}
	return rep
}

// lastState describes the state of the most recent backup in the window.
// One of: success, failed, running, hung — or none when no backup session
// was detected at all.
func (r statusReport) lastState() string {
	if len(r.sessions) == 0 {
		return "none"
	}
	return r.sessions[len(r.sessions)-1].outcome
}

// exitCode applies the freshness threshold. Contract:
// 0 fresh success, 2 stale (older than threshold, or no success in window).
func exitCode(r statusReport, threshold time.Duration, now time.Time) int {
	if !r.hasSuccess {
		return exitStale
	}
	if now.Sub(r.lastSuccess) <= threshold {
		return exitFresh
	}
	return exitStale
}

func logArgs() []string {
	return []string{"show", "--style", "ndjson", "--info", "--last", logWindow, "--predicate", logPredicate}
}

// runLogShow executes the Unified Log query as a subprocess. It performs
// no system changes; log show is read-only.
func runLogShow() ([]byte, error) {
	cmd := exec.Command(logBinaryPath, logArgs()...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("log show failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// exportRaw writes raw log show output with a metadata header to path.
// It is the corpus start: the header makes files self-describing so the
// later `tmsentinel export` subcommand keeps the same format.
func exportRaw(w io.Writer, raw []byte, now time.Time) error {
	fmt.Fprintf(w, "# tmsentinel export\n")
	fmt.Fprintf(w, "# captured_at: %s\n", now.Format(time.RFC3339))
	fmt.Fprintf(w, "# command: %s %s\n", logBinaryPath, strings.Join(logArgs(), " "))
	fmt.Fprintf(w, "# records_below: raw log show ndjson, one per line\n")
	_, err := w.Write(raw)
	return err
}

var errUsage = errors.New("usage error")

func run(stdout, stderr io.Writer, now time.Time) int {
	threshold := flag.Float64("threshold", defaultThresholdHours, "freshness threshold in hours (default 48)")
	exportPath := flag.String("export", "", "write raw log show output to this file (corpus capture)")
	flag.CommandLine.SetOutput(stderr)
	flag.Parse()
	if *threshold <= 0 {
		fmt.Fprintf(stderr, "tmsentinel: --threshold must be positive (got %v)\n", *threshold)
		return exitError
	}

	raw, err := runLogShow()
	if err != nil {
		fmt.Fprintf(stderr, "tmsentinel: %v\n", err)
		return exitError
	}

	st := parseNDJSON(raw)
	for _, line := range st.unparsedLn {
		fmt.Fprintf(stderr, "tmsentinel: unparsed record: %s\n", line)
	}
	fmt.Fprintf(stderr, "tmsentinel: %d parsed / %d unparsed records (%d footer lines)\n",
		st.parsed, st.unparsed, st.footer)
	rep := deriveState(st.events, now)
	thresholdDur := time.Duration(*threshold * float64(time.Hour))
	code := exitCode(rep, thresholdDur, now)

	fmt.Fprintf(stdout, "Time Machine status (unified log, last %s)\n", logWindow)
	if rep.hasSuccess {
		age := now.Sub(rep.lastSuccess)
		fmt.Fprintf(stdout, "  last successful backup : %s (%.1f h ago)\n",
			rep.lastSuccess.Format("2006-01-02 15:04:05 MST"), age.Hours())
	} else {
		fmt.Fprintf(stdout, "  last successful backup : none in window\n")
	}
	fmt.Fprintf(stdout, "  backups detected       : %d\n", len(rep.sessions))
	state := rep.lastState()
	switch state {
	case "none":
		fmt.Fprintf(stdout, "  last backup state      : no backup session detected")
		if rep.hasFailure {
			fmt.Fprintf(stdout, " (%d failure signals, last: %s)",
				rep.orphanFailures, rep.lastFailure.Format("2006-01-02 15:04:05 MST"))
		}
		fmt.Fprintln(stdout)
	case "running":
		last := rep.sessions[len(rep.sessions)-1]
		fmt.Fprintf(stdout, "  last backup state      : running (started %s, %.1f h ago)\n",
			last.start.Format("2006-01-02 15:04:05 MST"), now.Sub(last.start).Hours())
	case "hung":
		last := rep.sessions[len(rep.sessions)-1]
		fmt.Fprintf(stdout, "  last backup state      : hung, start without completion (started %s, %.1f h ago)\n",
			last.start.Format("2006-01-02 15:04:05 MST"), now.Sub(last.start).Hours())
	default:
		fmt.Fprintf(stdout, "  last backup state      : %s\n", state)
	}
	fmt.Fprintf(stdout, "  threshold              : %.1f h\n", *threshold)

	if *exportPath != "" {
		f, err := os.Create(*exportPath)
		if err != nil {
			fmt.Fprintf(stderr, "tmsentinel: export: %v\n", err)
			return exitError
		}
		if err := exportRaw(f, raw, now); err != nil {
			f.Close()
			fmt.Fprintf(stderr, "tmsentinel: export: %v\n", err)
			return exitError
		}
		if err := f.Close(); err != nil {
			fmt.Fprintf(stderr, "tmsentinel: export: %v\n", err)
			return exitError
		}
		fmt.Fprintf(stdout, "  export                 : %s\n", *exportPath)
	}

	switch code {
	case exitFresh:
		fmt.Fprintln(stdout, "result: fresh (exit 0)")
	default:
		if rep.hasSuccess {
			fmt.Fprintln(stdout, "result: stale, older than threshold (exit 2)")
		} else {
			fmt.Fprintln(stdout, "result: no successful backup in window (exit 2)")
		}
	}
	return code
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr, time.Now()))
}
