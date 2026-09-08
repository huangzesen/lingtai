//go:build !windows

package tui

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestWaitForLockClearDoesNotUnlinkHeldLockInode(t *testing.T) {
	dir := t.TempDir()
	lockFile := filepath.Join(dir, ".agent.lock")
	holder, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)

	before, err := os.Stat(lockFile)
	if err != nil {
		t.Fatal(err)
	}
	origAttempts, origInterval := lockWaitAttempts, lockWaitInterval
	lockWaitAttempts, lockWaitInterval = 1, 0
	defer func() {
		lockWaitAttempts, lockWaitInterval = origAttempts, origInterval
	}()

	waitForLockClear(dir)

	after, err := os.Stat(lockFile)
	if err != nil {
		t.Fatalf("held lock path was removed after timeout: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatalf("held lock path inode changed after timeout")
	}
}

func TestRemoveAgentLockIfOwnedRemovesUnlockedCurrentInode(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".agent.lock")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !removeAgentLockIfOwned(path) {
		t.Fatalf("removeAgentLockIfOwned(%q) = false for unlocked lock file", path)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file still exists after safe removal: %v", err)
	}
}

func TestWaitForLockClearReturnsQuicklyForUnlockedLock(t *testing.T) {
	dir := t.TempDir()
	origAttempts, origInterval := lockWaitAttempts, lockWaitInterval
	lockWaitAttempts, lockWaitInterval = 2, time.Millisecond
	defer func() {
		lockWaitAttempts, lockWaitInterval = origAttempts, origInterval
	}()

	waitForLockClear(dir)
}
