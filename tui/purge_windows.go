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
	// Optional dir filter from os.Args[2]
	var filterDir string
	if len(os.Args) > 2 {
		filterDir, _ = filepath.Abs(os.Args[2])
	}

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
		dir   string
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
				agentDir := ""
				agent := "unknown"
				if idx := strings.Index(cmdline, "lingtai run "); idx >= 0 {
					agentDir = cmdline[idx+len("lingtai run "):]
					agentDir = strings.TrimSpace(strings.Split(agentDir, " ")[0])
					agent = filepath.Base(agentDir)
				}

				// Filter by dir if specified
				if filterDir != "" {
					lingtaiPrefix := filepath.Join(filterDir, ".lingtai") + string(filepath.Separator)
					if !strings.HasPrefix(agentDir, lingtaiPrefix) {
						cmdline = ""
						pid = ""
						continue
					}
				}

				procs = append(procs, proc{pid: pid, agent: agent, dir: agentDir})
			}
			cmdline = ""
			pid = ""
		}
	}

	if len(procs) == 0 {
		if filterDir != "" {
			fmt.Printf("No lingtai processes found in %s.\n", filterDir)
		} else {
			fmt.Println("No lingtai processes found.")
		}
		return
	}

	scope := "ALL"
	if filterDir != "" {
		scope = filterDir
	}
	fmt.Printf("%-8s %-30s %s\n", "PID", "AGENT", "DIRECTORY")
	for _, p := range procs {
		fmt.Printf("%-8s %-30s %s\n", p.pid, p.agent, p.dir)
	}
	fmt.Printf("\n%d process(es) in %s. Kill all? [y/N] ", len(procs), scope)

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
