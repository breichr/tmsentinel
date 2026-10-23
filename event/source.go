package event

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Subsystem is the Unified Log subsystem Time Machine logs to.
const Subsystem = `com.apple.TimeMachine`

// Source provides raw log show output. Implementations are injected so
// tests never run subprocesses.
type Source interface {
	Show() ([]byte, error)
}

// UnifiedLog runs the log show query as a subprocess.
// Window must be a log show --last duration (default "7d", minimum 24h
// per architecture decision; never below the freshness threshold's
// look-ahead).
type UnifiedLog struct {
	Binary string // absolute path; launchd-safe, default "/usr/bin/log"
	Window string // log show --last window, default "7d"
}

// DefaultUnifiedLog returns the production source.
func DefaultUnifiedLog() UnifiedLog {
	return UnifiedLog{Binary: "/usr/bin/log", Window: "7d"}
}

// Args returns the full argument list of the query.
func (u UnifiedLog) Args() []string {
	bin := u.Binary
	if bin == "" {
		bin = "/usr/bin/log"
	}
	win := u.Window
	if win == "" {
		win = "7d"
	}
	// --info is mandatory: backupd logs its lifecycle signals
	// (BackupDispatching etc.) at Info level, not Default.
	return []string{"show", "--style", "ndjson", "--info", "--last", win,
		"--predicate", `subsystem == "` + Subsystem + `"`}
}

// Show executes the Unified Log query. It performs no system changes;
// log show is read-only.
func (u UnifiedLog) Show() ([]byte, error) {
	bin := u.Binary
	if bin == "" {
		bin = "/usr/bin/log"
	}
	cmd := exec.Command(bin, u.Args()...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("log show failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Export writes raw log show output with a metadata header to w.
// It is the corpus capture format: the header makes files
// self-describing so the export subcommand keeps the same format.
// commandLine describes the query that produced raw (see
// UnifiedLog.CommandLine).
func Export(w io.Writer, raw []byte, commandLine string, now time.Time) error {
	fmt.Fprintf(w, "# tmsentinel export\n")
	fmt.Fprintf(w, "# captured_at: %s\n", now.Format(time.RFC3339))
	fmt.Fprintf(w, "# command: %s\n", commandLine)
	fmt.Fprintf(w, "# records_below: raw log show ndjson, one per line\n")
	_, err := w.Write(raw)
	return err
}

// CommandLine returns the full query command including the binary.
func (u UnifiedLog) CommandLine() string {
	bin := u.Binary
	if bin == "" {
		bin = "/usr/bin/log"
	}
	return bin + " " + strings.Join(u.Args(), " ")
}
