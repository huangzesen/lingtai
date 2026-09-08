//go:build !windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anthropics/lingtai-tui/internal/processscan"
)

const (
	purgeTermGrace      = 2 * time.Second
	purgeVerifyDeadline = 2 * time.Second
	purgeVerifyInterval = 100 * time.Millisecond
)

var unixSignalProcess = func(pid int, sig syscall.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(sig)
}

type purgeResult struct {
	purged int
	failed int
}

func purgeMain() {
	// Optional dir filter from os.Args[2]
	var filterDir string
	if len(os.Args) > 2 {
		filterDir, _ = filepath.Abs(os.Args[2])
	}

	found, err := processscan.FindAllAgentProcesses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running ps: %v\n", err)
		os.Exit(1)
	}
	procs := purgeProcsFromAgentProcesses(found, filterDir, os.Getpid())

	if len(procs) == 0 {
		if filterDir != "" {
			fmt.Printf("No lingtai processes found in %s.\n", filterDir)
		} else {
			fmt.Println("No lingtai processes found.")
		}
		return
	}

	// List matching processes
	scope := "ALL"
	if filterDir != "" {
		scope = filterDir
	}
	fmt.Printf("%-8s %-30s %s\n", "PID", "AGENT", "DIRECTORY")
	for _, p := range procs {
		fmt.Printf("%-8d %-30s %s\n", p.pid, p.agent, p.dir)
	}
	fmt.Printf("\n%d process(es) in %s. Kill all? [y/N] ", len(procs), scope)

	// Wait for confirmation
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted.")
		return
	}

	result := purgeUnixProcesses(procs, purgeTermGrace)
	if result.failed > 0 {
		fmt.Printf("Purged %d process(es). Failed %d process(es).\n", result.purged, result.failed)
		return
	}
	fmt.Printf("Purged %d process(es).\n", result.purged)
}

func purgeUnixProcesses(procs []purgeProc, termGrace time.Duration) purgeResult {
	for _, p := range procs {
		_ = unixSignalProcess(p.pid, syscall.SIGTERM)
	}
	time.Sleep(termGrace)

	var result purgeResult
	for _, p := range procs {
		if !unixProcessAlive(p.pid) {
			result.purged++
			continue
		}
		if err := unixSignalProcess(p.pid, syscall.SIGKILL); err != nil {
			result.failed++
			continue
		}
		if waitForUnixProcessExit(p.pid, purgeVerifyDeadline, purgeVerifyInterval) {
			result.purged++
		} else {
			result.failed++
		}
	}
	return result
}

func unixProcessAlive(pid int) bool {
	return unixSignalProcess(pid, syscall.Signal(0)) == nil
}

func waitForUnixProcessExit(pid int, deadline, interval time.Duration) bool {
	end := time.Now().Add(deadline)
	for {
		if !unixProcessAlive(pid) {
			return true
		}
		if !time.Now().Before(end) {
			return false
		}
		time.Sleep(interval)
	}
}
