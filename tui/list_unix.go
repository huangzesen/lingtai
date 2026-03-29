//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func listMain() {
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
		pid     int
		agent   string
		project string
		dir     string
		elapsed string
	}

	var procs []proc
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "lingtai run") || strings.Contains(line, "grep") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil || pid == os.Getpid() {
			continue
		}

		// Parse agent dir from command args: ... lingtai run <dir>
		var agentDir string
		for i, f := range fields {
			if f == "run" && i+1 < len(fields) {
				agentDir = fields[i+1]
				break
			}
		}

		// If filtering by dir, only include processes under <dir>/.lingtai/
		if filterDir != "" {
			lingtaiPrefix := filepath.Join(filterDir, ".lingtai") + string(filepath.Separator)
			if !strings.HasPrefix(agentDir, lingtaiPrefix) {
				continue
			}
		}

		agent := filepath.Base(agentDir)
		project := ""
		// Walk up to find .lingtai parent
		if idx := strings.Index(agentDir, "/.lingtai/"); idx >= 0 {
			project = agentDir[:idx]
		}

		// Get process start time from /proc or ps
		elapsed := fields[9] // ps aux column 10 is START

		procs = append(procs, proc{pid: pid, agent: agent, project: project, dir: agentDir, elapsed: elapsed})
	}

	if len(procs) == 0 {
		if filterDir != "" {
			fmt.Printf("No lingtai processes running in %s.\n", filterDir)
		} else {
			fmt.Println("No lingtai processes running.")
		}
		return
	}

	// Also try to get elapsed time via ps -o etimes
	etimes := map[int]string{}
	pidStrs := make([]string, len(procs))
	for i, p := range procs {
		pidStrs[i] = strconv.Itoa(p.pid)
	}
	if out2, err := exec.Command("ps", "-o", "pid=,etimes=", "-p", strings.Join(pidStrs, ",")).Output(); err == nil {
		for _, line := range strings.Split(string(out2), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 {
				if pid, err := strconv.Atoi(fields[0]); err == nil {
					if secs, err := strconv.Atoi(fields[1]); err == nil {
						d := time.Duration(secs) * time.Second
						if d >= 24*time.Hour {
							etimes[pid] = fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
						} else if d >= time.Hour {
							etimes[pid] = fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
						} else {
							etimes[pid] = fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
						}
					}
				}
			}
		}
	}

	// Detect phantoms: processes running under a dir that has no .lingtai/
	phantomDirs := map[string]bool{}
	if filterDir != "" {
		lingtaiDir := filepath.Join(filterDir, ".lingtai")
		if _, err := os.Stat(lingtaiDir); os.IsNotExist(err) {
			phantomDirs[filterDir] = true
		}
	} else {
		// Check each unique project dir
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

	fmt.Printf("%-8s %-12s %-30s %s\n", "PID", "UPTIME", "AGENT", "PROJECT")
	for _, p := range procs {
		up := p.elapsed
		if e, ok := etimes[p.pid]; ok {
			up = e
		}
		label := ""
		if phantomDirs[p.project] {
			label = " [PHANTOM]"
		}
		fmt.Printf("%-8d %-12s %-30s %s%s\n", p.pid, up, p.agent, p.project, label)
	}
	fmt.Printf("\n%d process(es) running.\n", len(procs))

	if len(phantomDirs) > 0 {
		fmt.Println()
		for dir := range phantomDirs {
			fmt.Printf("WARNING: %s/.lingtai/ no longer exists — processes are phantoms.\n", dir)
		}
		if filterDir != "" {
			fmt.Printf("Run 'lingtai-tui purge %s' to kill them.\n", filterDir)
		} else {
			fmt.Println("Run 'lingtai-tui purge <dir>' to kill phantoms in a specific directory.")
		}
	}
}
