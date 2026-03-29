//go:build !windows

package tui

import (
	"os"
	"syscall"
)

// tryLock attempts a non-blocking flock on the lock file. Returns true if lock
// was acquired (meaning no other process holds it), and releases immediately.
func tryLock(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return true // can't open → assume not locked
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return false // locked by another process
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return true
}
