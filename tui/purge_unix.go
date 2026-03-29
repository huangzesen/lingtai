//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func purgeMain() {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running ps: %v\n", err)
		os.Exit(1)
	}

	var pids []int
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
		pids = append(pids, pid)
	}

	if len(pids) == 0 {
		fmt.Println("No lingtai processes found.")
		return
	}

	fmt.Printf("Found %d lingtai process(es). Killing...\n", len(pids))

	for _, pid := range pids {
		if p, err := os.FindProcess(pid); err == nil {
			p.Signal(syscall.SIGTERM)
		}
	}
	time.Sleep(2 * time.Second)

	// SIGKILL survivors
	killed := 0
	for _, pid := range pids {
		if p, err := os.FindProcess(pid); err == nil {
			if p.Signal(syscall.Signal(0)) == nil {
				p.Signal(syscall.SIGKILL)
			}
		}
		killed++
	}

	fmt.Printf("Purged %d process(es).\n", killed)
}
