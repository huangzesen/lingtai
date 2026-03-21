package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ModelConfig holds LLM provider settings.
type ModelConfig struct {
	Provider  string       `json:"provider"`
	Model     string       `json:"model"`
	APIKeyEnv string       `json:"api_key_env"`
	BaseURL   string       `json:"base_url,omitempty"`
	Vision    *ModelConfig `json:"vision,omitempty"`
	WebSearch *ModelConfig `json:"web_search,omitempty"`
}

// IMAPConfig holds IMAP addon settings (passed through to Python).
type IMAPConfig map[string]interface{}

// TelegramConfig holds Telegram addon settings (passed through to Python).
type TelegramConfig map[string]interface{}

// Config is the top-level daemon configuration.
type Config struct {
	Model      ModelConfig    `json:"-"` // resolved from "model" field
	IMAP       IMAPConfig     `json:"imap,omitempty"`
	Telegram   TelegramConfig `json:"telegram,omitempty"`
	CLI        bool           `json:"cli"`
	AgentName  string         `json:"agent_name"`
	BaseDir    string         `json:"base_dir"`
	BashPolicy string         `json:"bash_policy,omitempty"`
	MaxTurns   int            `json:"max_turns"`
	AgentPort  int            `json:"agent_port"`
	CLIPort    int            `json:"cli_port,omitempty"`
	Covenant   string         `json:"covenant,omitempty"`

	// Internal
	ConfigDir string `json:"-"` // directory containing config.json
}

// Load reads and validates a config.json file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config not found: %s", path)
	}

	absPath, _ := filepath.Abs(path)
	configDir := filepath.Dir(absPath)

	// Load .env from config directory
	LoadDotenv(configDir)

	// Parse into raw map first to handle the "model" field polymorphism
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}

	// Parse everything except "model" into Config struct
	cfg := &Config{
		AgentName: "orchestrator",
		BaseDir:   "~/.lingtai",
		MaxTurns:  50,
		AgentPort: 8501,
		CLI:       false,
		ConfigDir: configDir,
	}
	// Re-unmarshal to get defaults overridden
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Apply defaults for zero values
	if cfg.AgentName == "" {
		cfg.AgentName = "orchestrator"
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = "~/.lingtai"
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 50
	}
	if cfg.AgentPort == 0 {
		cfg.AgentPort = 8501
	}
	if cfg.CLIPort == 0 {
		cfg.CLIPort = cfg.AgentPort + 1
	}

	// Expand ~ in base_dir
	if strings.HasPrefix(cfg.BaseDir, "~") {
		home, _ := os.UserHomeDir()
		cfg.BaseDir = filepath.Join(home, cfg.BaseDir[1:])
	}

	// Resolve model config
	modelRaw, ok := raw["model"]
	if !ok {
		return nil, fmt.Errorf("'model' field is required in config.json")
	}

	// Try as string (file path) first
	var modelPath string
	if err := json.Unmarshal(modelRaw, &modelPath); err == nil {
		// It's a string — load from file
		fullPath := filepath.Join(configDir, modelPath)
		modelData, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("model config not found: %s", fullPath)
		}
		if err := json.Unmarshal(modelData, &cfg.Model); err != nil {
			return nil, fmt.Errorf("invalid model config: %w", err)
		}
	} else {
		// Try as inline object
		if err := json.Unmarshal(modelRaw, &cfg.Model); err != nil {
			return nil, fmt.Errorf("'model' must be a file path or inline object: %w", err)
		}
	}

	if cfg.Model.Provider == "" {
		return nil, fmt.Errorf("model.provider is required")
	}

	return cfg, nil
}

// LoadDotenv loads a .env file from the given directory into os.Environ.
// Existing env vars are not overwritten (setenv only if not already set).
func LoadDotenv(dir string) {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, "'\"")
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}

// ResolveEnvVar looks up an environment variable by name.
// Returns an error if the variable is not set.
func ResolveEnvVar(name string) (string, error) {
	val, ok := os.LookupEnv(name)
	if !ok || val == "" {
		return "", fmt.Errorf("environment variable %q is not set — add it to your environment or .env file", name)
	}
	return val, nil
}

// WorkingDir returns the agent's working directory: {base_dir}/{agent_name}
func (c *Config) WorkingDir() string {
	return filepath.Join(c.BaseDir, c.AgentName)
}
