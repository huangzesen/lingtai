package process

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/preset"
)

func InitProject(lingtaiDir string) error {
	if err := os.MkdirAll(lingtaiDir, 0o755); err != nil {
		return fmt.Errorf("create .lingtai: %w", err)
	}
	humanDir := filepath.Join(lingtaiDir, "human")
	manifestPath := filepath.Join(humanDir, ".agent.json")
	if _, err := os.Stat(manifestPath); err == nil {
		return nil
	}
	for _, sub := range []string{
		"mailbox/inbox",
		"mailbox/sent",
		"mailbox/archive",
	} {
		if err := os.MkdirAll(filepath.Join(humanDir, sub), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", sub, err)
		}
	}
	absPath, _ := filepath.Abs(humanDir)
	manifest := map[string]interface{}{
		"agent_name": "human",
		"address":    absPath,
		"admin":      nil,
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	contactsPath := filepath.Join(humanDir, "mailbox", "contacts.json")
	if err := os.WriteFile(contactsPath, []byte("[]"), 0o644); err != nil {
		return fmt.Errorf("write contacts: %w", err)
	}
	// TUI asset directory — viz data, topology snapshots, NOT agent state
	tuiAssetDir := filepath.Join(lingtaiDir, ".tui-asset")
	if err := os.MkdirAll(tuiAssetDir, 0o755); err != nil {
		return fmt.Errorf("create .tui-asset: %w", err)
	}
	// Bundled skills — write to .lingtai/.skills/ (shared across all agents)
	preset.PopulateBundledSkills(lingtaiDir)
	return nil
}

// resolvePython returns the Python executable for an agent.
// Priority: agent init.json venv_path → fallbackCmd.
func resolvePython(agentDir, fallbackCmd string) string {
	initPath := filepath.Join(agentDir, "init.json")
	data, err := os.ReadFile(initPath)
	if err == nil {
		var init map[string]interface{}
		if json.Unmarshal(data, &init) == nil {
			if vp, ok := init["venv_path"].(string); ok && vp != "" {
				python := config.VenvPython(vp)
				if _, err := os.Stat(python); err == nil {
					return python
				}
			}
		}
	}
	return fallbackCmd
}

// LaunchAgent starts an agent process. lingtaiCmd is the global fallback Python;
// the agent's init.json venv_path is tried first.
func LaunchAgent(lingtaiCmd, agentDir string) (*exec.Cmd, error) {
	fs.CleanSignals(agentDir)
	python := resolvePython(agentDir, lingtaiCmd)
	cmd := exec.Command(python, "-m", "lingtai", "run", agentDir)
	// Redirect agent output to a log file instead of the TUI terminal
	logPath := filepath.Join(agentDir, "logs")
	os.MkdirAll(logPath, 0o755)
	logFile, err := os.OpenFile(filepath.Join(logPath, "agent.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launch agent: %w", err)
	}
	return cmd, nil
}
