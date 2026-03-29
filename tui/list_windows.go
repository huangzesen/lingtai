//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func listMain() {
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
		pid     string
		agent   string
		dir     string
		project string
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

				project := ""
				if idx := strings.Index(agentDir, `\.lingtai\`); idx >= 0 {
					project = agentDir[:idx]
				}

				procs = append(procs, proc{pid: pid, agent: agent, dir: agentDir, project: project})
			}
			cmdline = ""
			pid = ""
		}
	}

	if len(procs) == 0 {
		if filterDir != "" {
			fmt.Printf("No lingtai processes running in %s.\n", filterDir)
		} else {
			fmt.Println("No lingtai processes running.")
		}
		return
	}

	// Detect phantoms: processes running under a dir that has no .lingtai/
	phantomDirs := map[string]bool{}
	if filterDir != "" {
		lingtaiDir := filepath.Join(filterDir, ".lingtai")
		if _, err := os.Stat(lingtaiDir); os.IsNotExist(err) {
			phantomDirs[filterDir] = true
		}
	} else {
		seen := map[string]bool{}
		for _, p := range procs {
			if p.project != "" && !seen[p.project] {
				seen[p.project] = true
				lingtaiDir := filepath.Join(p.project, ".lingtai")
				if _, err := os.Stat(lingtaiDir); os.IsNotExist(err) {
					phantomDirs[p.project] = true
				}
			}
		}
	}

	fmt.Printf("%-8s %-30s %s\n", "PID", "AGENT", "PROJECT")
	for _, p := range procs {
		label := ""
		if phantomDirs[p.project] {
			label = " [PHANTOM]"
		}
		fmt.Printf("%-8s %-30s %s%s\n", p.pid, p.agent, p.project, label)
	}
	fmt.Printf("\n%d process(es) running.\n", len(procs))

	if len(phantomDirs) > 0 {
		fmt.Println()
		for dir := range phantomDirs {
			fmt.Printf("WARNING: %s\\.lingtai\\ no longer exists — processes are phantoms.\n", dir)
		}
		if filterDir != "" {
			fmt.Printf("Run 'lingtai-tui purge %s' to kill them.\n", filterDir)
		} else {
			fmt.Println("Run 'lingtai-tui purge <dir>' to kill phantoms in a specific directory.")
		}
	}
}
