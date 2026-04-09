package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// MigrateLegacyLanguage moves Language from config.json to tui_config.json if needed.
func MigrateLegacyLanguage(globalDir string) {
	cfg, err := LoadConfig(globalDir)
	if err != nil || cfg.Language == "" {
		return
	}
	tc := LoadTUIConfig(globalDir)
	if tc.Language == "en" || tc.Language == "" {
		// Only migrate if tui_config hasn't been explicitly set
		tcPath := filepath.Join(globalDir, "tui_config.json")
		if _, err := os.Stat(tcPath); os.IsNotExist(err) {
			tc.Language = cfg.Language
			SaveTUIConfig(globalDir, tc)
		}
	}
}

// GlobalDirName is the name of the global config directory under $HOME.
const GlobalDirName = ".lingtai-tui"

type Config struct {
	Keys     map[string]string `json:"keys,omitempty"`     // provider → key, e.g. {"minimax": "xxx"}
	Language string            `json:"language,omitempty"` // deprecated: use TUIConfig.Language
}

// TUIConfig holds global TUI preferences at ~/.lingtai-tui/tui_config.json.
type TUIConfig struct {
	Language       string `json:"language"`
	MailPageSize   int    `json:"mail_page_size"`
	Greeting       bool   `json:"greeting"`
	Theme          string `json:"theme,omitempty"` // theme name: "ink-dark" (default), etc.
	Insights       bool   `json:"insights"`
}

// DefaultTUIConfig returns sensible defaults.
func DefaultTUIConfig() TUIConfig {
	return TUIConfig{
		Language:     "en",
		MailPageSize: 100,
		Greeting:     true,
		Insights:     false,
	}
}

// LoadTUIConfig reads ~/.lingtai-tui/tui_config.json.
func LoadTUIConfig(globalDir string) TUIConfig {
	data, err := os.ReadFile(filepath.Join(globalDir, "tui_config.json"))
	if err != nil {
		return DefaultTUIConfig()
	}
	var tc TUIConfig
	if err := json.Unmarshal(data, &tc); err != nil {
		return DefaultTUIConfig()
	}
	if tc.Language == "" {
		tc.Language = "en"
	}
	if tc.MailPageSize > 0 && tc.MailPageSize < 100 {
		tc.MailPageSize = 100 // migrate old values below minimum
	}
	// Greeting defaults to true when absent from JSON.
	if !strings.Contains(string(data), `"greeting"`) {
		tc.Greeting = true
	}
	// Insights defaults to false when absent from JSON.
	// No override needed — zero value of bool is false.
	return tc
}

// SaveTUIConfig writes ~/.lingtai-tui/tui_config.json.
func SaveTUIConfig(globalDir string, tc TUIConfig) error {
	data, err := json.MarshalIndent(tc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(globalDir, "tui_config.json"), data, 0o644)
}

func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, GlobalDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func LoadConfig(dir string) (Config, error) {
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func SaveConfig(dir string, cfg Config) error {
	os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		return err
	}
	return WriteEnvFile(dir, cfg)
}

// EnvFilePath returns the path to the global .env file.
func EnvFilePath(globalDir string) string {
	return filepath.Join(globalDir, ".env")
}

// WriteEnvFile writes API keys from config to ~/.lingtai-tui/.env.
// This file is loaded by agents at boot via env_file in init.json.
func WriteEnvFile(globalDir string, cfg Config) error {
	var lines []string
	for provider, key := range cfg.Keys {
		if key == "" {
			continue
		}
		envKey := providerToEnvKey(provider)
		lines = append(lines, envKey+"="+key)
	}
	path := EnvFilePath(globalDir)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// providerToEnvKey maps provider name to environment variable name.
func providerToEnvKey(provider string) string {
	switch provider {
	case "minimax":
		return "MINIMAX_API_KEY"
	default:
		return "LLM_API_KEY"
	}
}


