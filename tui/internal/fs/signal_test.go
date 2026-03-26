package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTouchSignal(t *testing.T) {
	dir := t.TempDir()
	if err := TouchSignal(dir, SignalSleep); err != nil {
		t.Fatalf("touch signal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sleep")); os.IsNotExist(err) {
		t.Error(".sleep file not created")
	}
}

func TestCleanSignals(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".sleep"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, ".suspend"), nil, 0o644)
	CleanSignals(dir)
	if _, err := os.Stat(filepath.Join(dir, ".sleep")); !os.IsNotExist(err) {
		t.Error(".sleep should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, ".suspend")); !os.IsNotExist(err) {
		t.Error(".suspend should be removed")
	}
}

func TestHasSignal(t *testing.T) {
	dir := t.TempDir()
	if HasSignal(dir, SignalSleep) {
		t.Error("should not have signal before touch")
	}
	os.WriteFile(filepath.Join(dir, ".sleep"), nil, 0o644)
	if !HasSignal(dir, SignalSleep) {
		t.Error("should have signal after touch")
	}
}
