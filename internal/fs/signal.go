package fs

import (
	"os"
	"path/filepath"
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
