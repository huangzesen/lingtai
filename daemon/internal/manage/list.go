//go:build !windows

package manage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"lingtai-daemon/internal/i18n"
)

// Spirit represents a running (or stale) agent.
type Spirit struct {
	Name    string
	PID     int
	Port    int
	Config  string
	Started time.Time
	Alive   bool
}

// ScanSpirits scans base_dir for agent.pid files.
func ScanSpirits(baseDir string) []Spirit {
	var spirits []Spirit
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return spirits
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pidPath := filepath.Join(baseDir, entry.Name(), "agent.pid")
		data, err := os.ReadFile(pidPath)
		if err != nil {
			continue
		}
		var info struct {
			PID     int    `json:"pid"`
			Port    int    `json:"port"`
			Config  string `json:"config"`
			Started string `json:"started"`
		}
		if json.Unmarshal(data, &info) != nil {
			continue
		}
		started, _ := time.Parse(time.RFC3339, info.Started)
		spirits = append(spirits, Spirit{
			Name:    entry.Name(),
			PID:     info.PID,
			Port:    info.Port,
			Config:  info.Config,
			Started: started,
			Alive:   isAlive(info.PID),
		})
	}
	return spirits
}

// isAlive checks if a process with the given PID is running.
func isAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Use Signal(0) to check.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// FormatTable renders spirits as a colored table string.
func FormatTable(spirits []Spirit) string {
	if len(spirits) == 0 {
		return fmt.Sprintf("  %s\n", i18n.S("no_spirits"))
	}

	header := fmt.Sprintf(
		" \033[1m%-20s %-8s %-6s %-12s %s\033[0m\n",
		i18n.S("name"), i18n.S("pid"), i18n.S("port"),
		i18n.S("uptime"), i18n.S("status"),
	)
	result := header
	for _, s := range spirits {
		uptime := ""
		status := ""
		if s.Alive {
			uptime = formatDuration(time.Since(s.Started))
			status = fmt.Sprintf("\033[32m● %s\033[0m", i18n.S("running"))
		} else {
			uptime = "—"
			status = fmt.Sprintf("\033[31m✗ %s\033[0m", i18n.S("dead"))
		}
		result += fmt.Sprintf(
			" %-20s %-8d %-6d %-12s %s\n",
			s.Name, s.PID, s.Port, uptime, status,
		)
	}
	result += fmt.Sprintf("\n  \033[2mStop with: kill <PID>\033[0m\n")
	return result
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
