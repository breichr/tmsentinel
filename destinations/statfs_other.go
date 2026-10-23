//go:build !darwin

package destinations

// StatfsMounted is the default MountChecker on non-macOS platforms:
// unsupported, always false. Live checks only run on the target system.
func StatfsMounted(mountPoint string) bool {
	return false
}
