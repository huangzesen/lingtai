//go:build windows

package tui

import (
	"os"

	"golang.org/x/sys/windows"
)

// tryLock attempts a non-blocking exclusive lock on the lock file. Returns
// true if the lock was acquired, and releases it immediately.
func tryLock(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return true // can't open -> assume not locked
	}
	defer f.Close()
	var overlapped windows.Overlapped
	err = windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		^uint32(0),
		^uint32(0),
		&overlapped,
	)
	if err != nil {
		return false
	}
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, ^uint32(0), ^uint32(0), &overlapped)
	return true
}

// removeAgentLockIfOwned removes path only when no other process holds the
// lock. Windows open/delete sharing makes the POSIX unlink-inode race a
// non-issue here, and a live holder's open handle normally blocks deletion
// anyway; tryLock probes with the same LockFileEx the agent's holder competes
// on. Returns true when the path no longer needs removal.
func removeAgentLockIfOwned(path string) bool {
	if !tryLock(path) {
		return false
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false
	}
	return true
}
