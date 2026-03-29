//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func purgeMain() {
	out, err := exec.Command("wmic", "process", "where",
		"commandline like '%lingtai run%'",
		"get", "processid,commandline", "/format:list").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing processes: %v\n", err)
		os.Exit(1)
	}

	type proc struct {
		pid   string
		agent string
	}

	var procs []proc
	var cmdline, pid string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "CommandLine=") {
			cmdline = strings.TrimPrefix(line, "CommandLine=")
		}
		if strings.HasPrefix(line, "ProcessId=") {
			pid = strings.TrimPrefix(line, "ProcessId=")
			if cmdline != "" && strings.Contains(cmdline, "lingtai run") {
				agent := "unknown"
				if idx := strings.Index(cmdline, "lingtai run "); idx >= 0 {
					dir := cmdline[idx+len("lingtai run "):]
					dir = strings.TrimSpace(strings.Split(dir, " ")[0])
					agent = filepath.Base(dir)
				}
				procs = append(procs, proc{pid: pid, agent: agent})
			}
			cmdline = ""
			pid = ""
		}
	}

	if len(procs) == 0 {
		fmt.Println("No lingtai processes found.")
		return
	}

	fmt.Printf("%-8s %s\n", "PID", "AGENT")
	for _, p := range procs {
		fmt.Printf("%-8s %s\n", p.pid, p.agent)
	}
	fmt.Printf("\n%d process(es) found. Kill all? [y/N] ", len(procs))

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted.")
		return
	}

	killed := 0
	for _, p := range procs {
		cmd := exec.Command("taskkill", "/F", "/PID", p.pid)
		if cmd.Run() == nil {
			killed++
		}
	}

	fmt.Printf("Purged %d process(es).\n", killed)
}
