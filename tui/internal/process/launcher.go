package process

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
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
	return nil
}

func LaunchAgent(lingtaiCmd, agentDir string) (*exec.Cmd, error) {
	fs.CleanSignals(agentDir)
	cmd := exec.Command(lingtaiCmd, "-m", "lingtai", "run", agentDir)
	// Redirect agent output to a log file instead of the TUI terminal
	logPath := filepath.Join(agentDir, "logs")
	os.MkdirAll(logPath, 0o755)
	logFile, err := os.OpenFile(filepath.Join(logPath, "agent.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	// Inject API keys from ~/.lingtai/config.json into subprocess env
	cmd.Env = os.Environ()
	if globalDir, err := config.GlobalDir(); err == nil {
		if cfg, err := config.LoadConfig(globalDir); err == nil {
			for provider, key := range cfg.Keys {
				if key == "" {
					continue
				}
				envKey := providerToEnvKey(provider)
				cmd.Env = append(cmd.Env, envKey+"="+key)
			}
		}
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launch agent: %w", err)
	}
	return cmd, nil
}

// providerToEnvKey maps provider name to environment variable name
func providerToEnvKey(provider string) string {
	switch provider {
	case "minimax":
		return "MINIMAX_API_KEY"
	case "gemini":
		return "GEMINI_API_KEY"
	default:
		return "LLM_API_KEY"
	}
}
