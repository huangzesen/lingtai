//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func purgeMain() {
	// Use taskkill to find and kill all python processes running "lingtai run"
	out, err := exec.Command("wmic", "process", "where",
		"commandline like '%lingtai run%'", "get", "processid").Output()
	if err != nil {
		// Fallback: try tasklist
		out, err = exec.Command("tasklist", "/FI", "IMAGENAME eq python.exe", "/FO", "CSV").Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error listing processes: %v\n", err)
			os.Exit(1)
		}
	}

	// Parse PIDs and kill
	killed := 0
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "ProcessId") {
			continue
		}
		// taskkill by PID
		cmd := exec.Command("taskkill", "/F", "/PID", line)
		if cmd.Run() == nil {
			killed++
		}
	}

	if killed == 0 {
		fmt.Println("No lingtai processes found.")
	} else {
		fmt.Printf("Purged %d process(es).\n", killed)
	}
}
