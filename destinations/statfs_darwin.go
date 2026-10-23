//go:build darwin

package destinations

import "syscall"

// StatfsMounted is the default MountChecker on macOS: statfs reads VFS
// metadata for the mount point without accessing volume contents, so it
// works without Full Disk Access.
func StatfsMounted(mountPoint string) bool {
	var st syscall.Statfs_t
	if err := syscall.Statfs(mountPoint, &st); err != nil {
		return false
	}
	return true
}
