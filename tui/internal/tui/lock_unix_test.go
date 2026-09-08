//go:build !windows

package tui

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestWaitForLockClearDoesNotUnlinkHeldLockInode is the POSIX regression from
// #886: a lock held past the wait deadline must not be unlinked, and the
// pathname must keep naming the same inode.
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

// TestRemoveAgentLockIfOwnedRemovesUnlockedCurrentInode: a stale, unlocked
// lock file is safe to remove.
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

// TestRemoveAgentLockIfOwnedLeavesHeldLockInode: a lock held by a live
// descriptor must be left untouched on disk, at the same inode.
func TestRemoveAgentLockIfOwnedLeavesHeldLockInode(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".agent.lock")
	holder, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if removeAgentLockIfOwned(path) {
		t.Fatalf("removeAgentLockIfOwned(%q) = true for held lock", path)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("held lock path was removed: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Fatalf("held lock path inode changed")
	}
}

// TestWaitForLockClearSkipsReplacedInode: when the pathname has been replaced
// by a fresh inode that a newcomer actively holds, the deadline must expire
// without unlinking that live fresh lease — exactly the #886 hazard that an
// unconditional unlink of the pathname would enable.
func TestWaitForLockClearSkipsReplacedInode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".agent.lock")

	// Old holder: inode A, lock held for the whole poll.
	oldHolder, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer oldHolder.Close()
	if err := syscall.Flock(int(oldHolder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("lock old inode: %v", err)
	}
	defer syscall.Flock(int(oldHolder.Fd()), syscall.LOCK_UN)
	oldInfo, err := oldHolder.Stat()
	if err != nil {
		t.Fatal(err)
	}

	// Newcomer unlinks the path and replaces it with inode B, holding it.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	newHolder, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer newHolder.Close()
	if err := syscall.Flock(int(newHolder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("lock new inode: %v", err)
	}
	defer syscall.Flock(int(newHolder.Fd()), syscall.LOCK_UN)
	newInfo, err := newHolder.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(oldInfo, newInfo) {
		t.Fatalf("replacement didn't create a distinct inode")
	}

	// Timeout cleanup must leave the newcomer's live inode B in place.
	origAttempts, origInterval := lockWaitAttempts, lockWaitInterval
	lockWaitAttempts, lockWaitInterval = 1, 0
	defer func() {
		lockWaitAttempts, lockWaitInterval = origAttempts, origInterval
	}()
	waitForLockClear(dir)

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("newcomer lock path was removed: %v", err)
	}
	if !os.SameFile(newInfo, after) {
		t.Fatalf("newcomer lock inode changed or was replaced")
	}
}

// TestWaitForLockClearReturnsQuicklyForUnlockedLock keeps the happy path
// covered with a fast poll (no 60s stall when no lock exists).
func TestWaitForLockClearReturnsQuicklyForUnlockedLock(t *testing.T) {
	dir := t.TempDir()
	origAttempts, origInterval := lockWaitAttempts, lockWaitInterval
	lockWaitAttempts, lockWaitInterval = 2, time.Millisecond
	defer func() {
		lockWaitAttempts, lockWaitInterval = origAttempts, origInterval
	}()

	waitForLockClear(dir)
}
