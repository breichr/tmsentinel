// Package cli implements the subcommand dispatch, output, and the
// exit-code contract (0-5, stable, documented in the README):
//
//	0  last successful backup fresh (running counts as fresh, with hint)
//	1  internal error (subprocess failure, bad flags, write error)
//	2  last successful backup older than threshold, or none in window
//	3  no destination configured
//	4  destination configured but unreachable
//	5  reserved for --json consumers that want running:true explicitly;
//	   check itself never exits 5
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/breichr/tmsentinel/destinations"
	"github.com/breichr/tmsentinel/event"
	"github.com/breichr/tmsentinel/freshness"
	"github.com/breichr/tmsentinel/state"
)

// Version is the semantic version of the tool.
const Version = "0.1.0"

// Exit codes: a stable, documented contract. New states get new codes
// >= 6, major version only.
const (
	ExitFresh         = 0
	ExitError         = 1
	ExitStale         = 2
	ExitNoDestination = 3
	ExitUnreachable   = 4
	// ExitRunning is reserved for --json consumers that want to ask for
	// the running state explicitly. check itself exits 0 with a hint.
	ExitRunning = 5
)

const defaultThresholdHours = 48.0

// Sources bundles the external inputs, injected so tests never run
// subprocesses. Mount is the mount-point checker (statfs on darwin).
type Sources struct {
	Log   event.Source
	Dest  destinations.Source
	Mount destinations.MountChecker
}

// Run dispatches a subcommand and returns the exit code. It performs no
// system changes; all queries are read-only.
func Run(args []string, stdout, stderr io.Writer, now time.Time) int {
	src := Sources{
		Log:   event.DefaultUnifiedLog(),
		Dest:  destinations.DefaultTMUtil(),
		Mount: destinations.StatfsMounted,
	}
	return run(src, args, stdout, stderr, now)
}

// run dispatches with injected sources; tests use this entry point and
// never run subprocesses.
func run(src Sources, args []string, stdout, stderr io.Writer, now time.Time) int {
	if len(args) == 0 {
		usage(stderr)
		return ExitError
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "check":
		return runCheck(src, rest, stdout, stderr, now)
	case "destinations":
		return runDestinations(src, rest, stdout, stderr)
	case "export":
		return runExport(src, rest, stdout, stderr, now)
	case "version":
		fmt.Fprintf(stdout, "tmsentinel %s\n", Version)
		return ExitFresh
	case "help", "-h", "--help":
		usage(stdout)
		return ExitFresh
	default:
		fmt.Fprintf(stderr, "tmsentinel: unknown subcommand %q\n", cmd)
		usage(stderr)
		return ExitError
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `tmsentinel — is my Time Machine backup really running?

Usage:
  tmsentinel check [--json] [--threshold HOURS]   freshness check for schedulers
  tmsentinel destinations [--json]               list configured destinations
  tmsentinel export FILE                         write raw log output (corpus)
  tmsentinel version

Exit codes: 0 fresh, 1 internal error, 2 stale, 3 no destination,
4 destination unreachable. (5 reserved: running, for --json consumers.)
`)
}

// checkReport is the stable --json schema. Fields are additive only;
// never renamed or removed.
type checkReport struct {
	State           string        `json:"state"`
	LastSuccessAgeH *float64      `json:"last_success_age_h"`
	ThresholdH      float64       `json:"threshold_h"`
	Destinations    []destSummary `json:"destinations"`
	Diagnosis       struct{}      `json:"diagnosis"`
	Running         bool          `json:"running"`
	ConfigLoaded    bool          `json:"config_loaded"`
}

type destSummary struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	MountPoint string `json:"mount_point,omitempty"`
	Reachable  bool   `json:"reachable"`
}

func runCheck(src Sources, args []string, stdout, stderr io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	threshold := fs.Float64("threshold", defaultThresholdHours, "freshness threshold in hours (default 48)")
	if err := fs.Parse(args); err != nil {
		return ExitError
	}
	if *threshold <= 0 {
		fmt.Fprintf(stderr, "tmsentinel: --threshold must be positive (got %v)\n", *threshold)
		return ExitError
	}
	thresholdDur := time.Duration(*threshold * float64(time.Hour))

	dests, code := fetchDestinations(src, stderr)
	if code != ExitFresh {
		return code
	}
	if len(dests) == 0 {
		return emitCheck(stdout, stderr, checkReport{
			State:        "none",
			ThresholdH:   *threshold,
			Destinations: []destSummary{},
		}, "no destination configured", ExitNoDestination, *jsonOut)
	}

	raw, err := src.Log.Show()
	if err != nil {
		fmt.Fprintf(stderr, "tmsentinel: %v\n", err)
		return ExitError
	}
	st := event.Parse(raw)
	logUnparsed(st, stderr)

	rep := state.Derive(st.Events, now)
	running := rep.Running()

	var agePtr *float64
	if rep.HasSuccess {
		age := freshness.AgeHours(rep.LastSuccess, now)
		agePtr = &age
	}

	summaries := make([]destSummary, 0, len(dests))
	anyReachable := false
	for _, d := range dests {
		r := destinations.Reachable(d, src.Mount)
		anyReachable = anyReachable || r
		summaries = append(summaries, destSummary{Name: d.Name, Kind: d.Kind, MountPoint: d.MountPoint, Reachable: r})
	}

	report := checkReport{
		State:           rep.LastState(),
		LastSuccessAgeH: agePtr,
		ThresholdH:      *threshold,
		Destinations:    summaries,
		Running:         running,
		ConfigLoaded:    false, // no config file yet (Phase 2)
	}

	// Precedence: running (hint) and fresh succeed before staleness;
	// unreachable destinations sharpen a stale result.
	switch {
	case running:
		return emitCheck(stdout, stderr, report, "backup running", ExitFresh, *jsonOut)
	case rep.HasSuccess && freshness.Fresh(rep.LastSuccess, thresholdDur, now):
		return emitCheck(stdout, stderr, report, "fresh", ExitFresh, *jsonOut)
	case !anyReachable:
		return emitCheck(stdout, stderr, report, "destination unreachable", ExitUnreachable, *jsonOut)
	default:
		if rep.HasSuccess {
			return emitCheck(stdout, stderr, report, "stale, older than threshold", ExitStale, *jsonOut)
		}
		return emitCheck(stdout, stderr, report, "no successful backup in window", ExitStale, *jsonOut)
	}
}

func emitCheck(stdout, stderr io.Writer, rep checkReport, result string, code int, jsonOut bool) int {
	if jsonOut {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "tmsentinel: json: %v\n", err)
			return ExitError
		}
		fmt.Fprintln(stdout, string(b))
		if code != ExitFresh {
			fmt.Fprintf(stderr, "tmsentinel: %s\n", result)
		}
		return code
	}
	fmt.Fprintf(stdout, "state: %s\n", rep.State)
	if rep.LastSuccessAgeH != nil {
		fmt.Fprintf(stdout, "last successful backup: %.1f h ago\n", *rep.LastSuccessAgeH)
	} else {
		fmt.Fprintf(stdout, "last successful backup: none in window\n")
	}
	fmt.Fprintf(stdout, "threshold: %.1f h\n", rep.ThresholdH)
	fmt.Fprintf(stdout, "destinations: %d\n", len(rep.Destinations))
	for _, d := range rep.Destinations {
		state := "unreachable"
		if d.Reachable {
			state = "reachable"
		}
		fmt.Fprintf(stdout, "  %s (%s, %s)\n", d.Name, d.Kind, state)
	}
	if rep.Running {
		fmt.Fprintf(stdout, "hint: backup is running right now\n")
	}
	fmt.Fprintf(stdout, "result: %s (exit %d)\n", result, code)
	return code
}

func fetchDestinations(src Sources, stderr io.Writer) ([]destinations.Destination, int) {
	raw, err := src.Dest.Info()
	if err != nil {
		fmt.Fprintf(stderr, "tmsentinel: %v\n", err)
		return nil, ExitError
	}
	return destinations.ParseInfo(string(raw)), ExitFresh
}

func runDestinations(src Sources, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("destinations", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitError
	}
	dests, code := fetchDestinations(src, stderr)
	if code != ExitFresh {
		return code
	}
	if len(dests) == 0 {
		if *jsonOut {
			fmt.Fprintln(stdout, "[]")
		} else {
			fmt.Fprintln(stdout, "no destinations configured")
		}
		return ExitNoDestination
	}
	if *jsonOut {
		b, err := json.MarshalIndent(dests, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "tmsentinel: json: %v\n", err)
			return ExitError
		}
		fmt.Fprintln(stdout, string(b))
		return ExitFresh
	}
	for _, d := range dests {
		reach := "unreachable"
		if destinations.Reachable(d, src.Mount) {
			reach = "reachable"
		}
		mp := d.MountPoint
		if mp == "" {
			mp = "-"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", d.Name, d.Kind, mp, reach)
	}
	return ExitFresh
}

func runExport(src Sources, args []string, stdout, stderr io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return ExitError
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "tmsentinel: export requires exactly one file argument\n")
		return ExitError
	}
	path := fs.Arg(0)
	raw, err := src.Log.Show()
	if err != nil {
		fmt.Fprintf(stderr, "tmsentinel: %v\n", err)
		return ExitError
	}
	st := event.Parse(raw)
	logUnparsed(st, stderr)
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(stderr, "tmsentinel: export: %v\n", err)
		return ExitError
	}
	defer f.Close()
	cmdLine := event.DefaultUnifiedLog().CommandLine()
	if ap, ok := src.Log.(interface{ CommandLine() string }); ok {
		cmdLine = ap.CommandLine()
	}
	if err := event.Export(f, raw, cmdLine, now); err != nil {
		fmt.Fprintf(stderr, "tmsentinel: export: %v\n", err)
		return ExitError
	}
	fmt.Fprintf(stdout, "exported %d parsed / %d unparsed records to %s\n",
		st.Parsed, st.Unparsed, path)
	return ExitFresh
}

func logUnparsed(st event.Result, stderr io.Writer) {
	for _, line := range st.UnparsedLines {
		fmt.Fprintf(stderr, "tmsentinel: unparsed record: %s\n", line)
	}
	fmt.Fprintf(stderr, "tmsentinel: %d parsed / %d unparsed records (%d footer lines)\n",
		st.Parsed, st.Unparsed, st.Footer)
}
