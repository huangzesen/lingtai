//go:build !windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func purgeMain() {
	// Optional dir filter from os.Args[2]
	var filterDir string
	if len(os.Args) > 2 {
		filterDir, _ = filepath.Abs(os.Args[2])
	}

	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running ps: %v\n", err)
		os.Exit(1)
	}

	type proc struct {
		pid   int
		agent string
		dir   string
	}

	var procs []proc
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "lingtai run") || strings.Contains(line, "grep") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil || pid == os.Getpid() {
			continue
		}

		var agentDir string
		for i, f := range fields {
			if f == "run" && i+1 < len(fields) {
				agentDir = fields[i+1]
				break
			}
		}

		// Filter by dir if specified
		if filterDir != "" {
			lingtaiPrefix := filepath.Join(filterDir, ".lingtai") + string(filepath.Separator)
			if !strings.HasPrefix(agentDir, lingtaiPrefix) {
				continue
			}
		}

		procs = append(procs, proc{
			pid:   pid,
			agent: filepath.Base(agentDir),
			dir:   agentDir,
		})
	}

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

	// SIGTERM first
	for _, p := range procs {
		if proc, err := os.FindProcess(p.pid); err == nil {
			proc.Signal(syscall.SIGTERM)
		}
	}
	time.Sleep(2 * time.Second)

	// SIGKILL survivors
	killed := 0
	for _, p := range procs {
		if proc, err := os.FindProcess(p.pid); err == nil {
			if proc.Signal(syscall.Signal(0)) == nil {
				proc.Signal(syscall.SIGKILL)
			}
		}
		killed++
	}

	fmt.Printf("Purged %d process(es).\n", killed)
}
