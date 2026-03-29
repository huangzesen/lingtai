package fs

import (
	"os"
	"path/filepath"
	"time"
)

type Signal string

const (
	SignalSleep     Signal = ".sleep"
	SignalSuspend   Signal = ".suspend"
	SignalInterrupt Signal = ".interrupt"
)

func TouchSignal(dir string, sig Signal) error {
	return os.WriteFile(filepath.Join(dir, string(sig)), nil, 0o644)
}

func HasSignal(dir string, sig Signal) bool {
	_, err := os.Stat(filepath.Join(dir, string(sig)))
	return err == nil
}

func CleanSignals(dir string) {
	for _, sig := range []Signal{SignalSleep, SignalSuspend, SignalInterrupt} {
		os.Remove(filepath.Join(dir, string(sig)))
	}
}

// SuspendAndWait sends a suspend signal and waits for the agent to die.
// Returns after the agent stops heartbeating or after timeout.
func SuspendAndWait(dir string, timeout time.Duration) {
	if !IsAlive(dir, 2.0) {
		return
	}
	TouchSignal(dir, SignalSuspend)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if !IsAlive(dir, 2.0) {
			return
		}
	}
}
