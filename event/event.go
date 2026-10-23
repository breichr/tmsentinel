// Package event parses the Unified Log stream of the Time Machine
// subsystem. It is the only package that shells out to the log
// binary (together with destinations, which runs tmutil).
//
// The parser is defensive by contract: missing or null fields never
// crash it, and unparsed lines are collected with their raw content
// instead of being dropped or guessed.
package event

import (
	"bufio"
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
)

// timestampLayout matches log show --style ndjson timestamps such as
// "2026-10-19 13:24:07.365874+0200"; the fraction is optional in parse.
const timestampLayout = "2006-01-02 15:04:05.999999-0700"

// Kind classifies a Unified Log record by its meaning for backups.
type Kind int

const (
	KindNone    Kind = iota // parsed record, no backup signal
	KindStart               // backup started
	KindSuccess             // backup completed successfully
	KindFailed              // backup attempt failed
)

func (k Kind) String() string {
	switch k {
	case KindStart:
		return "start"
	case KindSuccess:
		return "success"
	case KindFailed:
		return "failed"
	default:
		return "none"
	}
}

// Event is a parsed record placed on the timeline.
type Event struct {
	Ts      time.Time
	Kind    Kind
	Message string
	Process string
}

// record is the subset of the log show --style ndjson schema we need.
// All fields are pointers: the parser must be robust against missing or
// null fields, never crash, and never silently guess.
type record struct {
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

// ClassifyMessage maps an eventMessage to a Kind using the pattern
// catalogs above.
func ClassifyMessage(msg string) Kind {
	for _, re := range noisePatterns {
		if re.MatchString(msg) {
			return KindNone
		}
	}
	for _, re := range startPatterns {
		if re.MatchString(msg) {
			return KindStart
		}
	}
	for _, re := range successPatterns {
		if re.MatchString(msg) {
			return KindSuccess
		}
	}
	for _, re := range failedPatterns {
		if re.MatchString(msg) {
			return KindFailed
		}
	}
	return KindNone
}

func classify(rec *record) Kind {
	// BackupDispatching (category, from backupd) carries the verified
	// start ("Backup requested ...") and completion ("Backup succeeded
	// ...") signals. Other processes (e.g. tmutil itself) log to this
	// subsystem too, so match on the message content, not category alone.
	if rec.EventMessage != nil {
		lower := strings.ToLower(*rec.EventMessage)
		if rec.Category != nil && strings.EqualFold(*rec.Category, "BackupDispatching") {
			if strings.Contains(lower, "backup requested") {
				return KindStart
			}
			if strings.Contains(lower, "backup succeeded") {
				return KindSuccess
			}
		}
		return ClassifyMessage(*rec.EventMessage)
	}
	return KindNone
}

// Result summarizes one parse run.
type Result struct {
	Lines         int      // non-empty input lines seen
	Parsed        int      // lines decoded and placed on the timeline
	Unparsed      int      // lines that could not be decoded or timestamped
	Comments      int      // '#' header lines (export metadata)
	Footer        int      // log show ndjson footer lines (known non-records)
	UnparsedLines []string // raw content of unparsed lines
	Events        []Event
}

// Parse parses raw log show --style ndjson output line by line.
// It is pure: no I/O, no clock. Every unparsed line is collected with its
// raw content so callers can log it; records are never silently dropped.
func Parse(raw []byte) Result {
	var st Result
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		st.Lines++
		if strings.HasPrefix(line, "#") {
			// export header comment (see Export)
			st.Comments++
			continue
		}
		var rec record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			st.Unparsed++
			st.UnparsedLines = append(st.UnparsedLines, line)
			continue
		}
		if rec.Timestamp == nil {
			if rec.Finished != nil {
				// footer line of log show --style ndjson output
				st.Footer++
				continue
			}
			st.Unparsed++
			st.UnparsedLines = append(st.UnparsedLines, line)
			continue
		}
		ts, err := time.Parse(timestampLayout, *rec.Timestamp)
		if err != nil {
			st.Unparsed++
			st.UnparsedLines = append(st.UnparsedLines, line)
			continue
		}
		ev := Event{Ts: ts, Kind: classify(&rec)}
		if rec.EventMessage != nil {
			ev.Message = *rec.EventMessage
		}
		if rec.ProcessImagePath != nil {
			ev.Process = *rec.ProcessImagePath
		}
		st.Parsed++
		st.Events = append(st.Events, ev)
	}
	sort.SliceStable(st.Events, func(i, j int) bool { return st.Events[i].Ts.Before(st.Events[j].Ts) })
	return st
}
