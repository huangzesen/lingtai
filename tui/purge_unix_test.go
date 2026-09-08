//go:build !windows

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPurgeMainFailsLoudOnScanError(t *testing.T) {
	if os.Getenv("LINGTAI_TEST_PURGE_SCAN_FAIL") == "1" {
		os.Args = []string{"lingtai-tui", "purge"}
		purgeMain()
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^TestPurgeMainFailsLoudOnScanError$")
	cmd.Env = append(os.Environ(),
		"LINGTAI_TEST_PURGE_SCAN_FAIL=1",
		"PATH="+t.TempDir(), // ps is unreachable, so the scan command fails
	)
	cmd.Stdin = strings.NewReader("n\n") // never reached; keeps a regression from hanging on the kill prompt
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected nonzero exit when ps is unavailable, got err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if code := exitErr.ExitCode(); code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error running ps") {
		t.Fatalf("stderr = %q, want it to report the ps failure", stderr.String())
	}
	if strings.Contains(stdout.String(), "No lingtai processes found") {
		t.Fatalf("stdout = %q, scan failure must not masquerade as an empty process list", stdout.String())
	}
}

func TestPurgeUnixProcessesReportsObservedOutcomes(t *testing.T) {
	origSignalProcess := unixSignalProcess
	defer func() {
		unixSignalProcess = origSignalProcess
	}()

	alive := map[int]bool{
		101: true,
		202: true,
		303: true,
	}
	unixSignalProcess = func(pid int, sig syscall.Signal) error {
		switch sig {
		case syscall.SIGTERM:
			if pid == 101 {
				alive[pid] = false
			}
			return nil
		case syscall.SIGKILL:
			if pid == 303 {
				return errors.New("permission denied")
			}
			alive[pid] = false
			return nil
		case syscall.Signal(0):
			if alive[pid] {
				return nil
			}
			return syscall.ESRCH
		default:
			return nil
		}
	}

	result := purgeUnixProcesses([]purgeProc{
		{pid: 101},
		{pid: 202},
		{pid: 303},
	}, 0)

	if result.purged != 2 || result.failed != 1 {
		t.Fatalf("purge result = %+v, want purged=2 failed=1", result)
	}
	if waitForUnixProcessExit(303, 0, time.Millisecond) {
		t.Fatalf("waitForUnixProcessExit reported a still-signalable process as exited")
	}
}
