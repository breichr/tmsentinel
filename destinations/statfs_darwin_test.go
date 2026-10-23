//go:build darwin

package destinations

import "testing"

// Live smoke test of the production mount checker: per test strategy,
// live checks carry //go:build darwin and never run in CI.
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
