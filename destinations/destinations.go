// Package destinations reads Time Machine destinations from
// tmutil destinationinfo (read-only, FDA-free). It is the only package
// besides event that runs a subprocess.
package destinations

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Destination is one configured Time Machine destination.
type Destination struct {
	Name       string
	Kind       string // "Local", "Network", ...
	ID         string
	MountPoint string
}

// Source provides raw tmutil destinationinfo output. Implementations
// are injected so tests never run subprocesses.
type Source interface {
	Info() ([]byte, error)
}

// TMUtil runs tmutil destinationinfo as a subprocess.
type TMUtil struct {
	Binary string // absolute path; default "/usr/bin/tmutil"
}

// DefaultTMUtil returns the production source.
func DefaultTMUtil() TMUtil {
	return TMUtil{Binary: "/usr/bin/tmutil"}
}

// Info executes the read-only destinationinfo query.
func (t TMUtil) Info() ([]byte, error) {
	bin := t.Binary
	if bin == "" {
		bin = "/usr/bin/tmutil"
	}
	cmd := exec.Command(bin, "destinationinfo")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tmutil destinationinfo failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

var fieldRe = regexp.MustCompile(`^([A-Za-z ]+?)\s*:\s*(.*)$`)

// ParseInfo parses tmutil destinationinfo text output. It is defensive:
// unknown lines and blocks without a Name are skipped, never guessed.
func ParseInfo(text string) []Destination {
	var dests []Destination
	var cur *Destination
	// header lines of each block look like "====..."
	isBlockHead := func(line string) bool {
		return strings.HasPrefix(strings.TrimSpace(line), "====")
	}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if isBlockHead(line) {
			if cur != nil && cur.Name != "" {
				dests = append(dests, *cur)
			}
			cur = &Destination{}
			continue
		}
		if cur == nil {
			continue
		}
		m := fieldRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, val := m[1], strings.TrimSpace(m[2])
		switch key {
		case "Name":
			cur.Name = val
		case "Kind":
			cur.Kind = val
		case "ID":
			cur.ID = val
		case "Mount Point":
			cur.MountPoint = val
		}
	}
	if cur != nil && cur.Name != "" {
		dests = append(dests, *cur)
	}
	return dests
}

// MountChecker reports whether a mount point is currently mounted and
// visible via VFS. Injected for tests; the default implementation uses
// statfs (filesystem metadata, no volume content access, FDA-free).
type MountChecker func(mountPoint string) bool

// Reachable reports whether the destination's mount point is mounted.
// Destinations without a mount point are reported unreachable; a
// mounted point whose statfs succeeds counts as reachable.
func Reachable(d Destination, mounted MountChecker) bool {
	if d.MountPoint == "" || mounted == nil {
		return false
	}
	return mounted(d.MountPoint)
}

// AnyReachable reports whether at least one destination is reachable.
func AnyReachable(dests []Destination, mounted MountChecker) bool {
	for _, d := range dests {
		if Reachable(d, mounted) {
			return true
		}
	}
	return false
}
