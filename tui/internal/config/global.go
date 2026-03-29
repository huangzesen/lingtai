package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// GlobalDirName is the name of the global config directory under $HOME.
const GlobalDirName = ".lingtai-tui"

type Config struct {
	Keys     map[string]string `json:"keys,omitempty"`     // provider → key, e.g. {"minimax": "xxx"}
	Language string            `json:"language,omitempty"` // TUI language: "en", "zh", "wen"
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
	// Try new format first
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err == nil && (cfg.Keys != nil || cfg.Language != "") {
		return cfg, nil
	}
	// Fallback: try legacy format (minimax_api_key)
	var legacy struct {
		MiniMaxAPIKey string `json:"minimax_api_key,omitempty"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return Config{}, err
	}
	MigrateConfig(&cfg, legacy.MiniMaxAPIKey)
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

func NeedsSetup(dir string) bool {
	cfg, err := LoadConfig(dir)
	if err != nil {
		return true
	}
	return len(cfg.Keys) == 0
}

// TutorialDone returns true if the user has completed or skipped the tutorial.
func TutorialDone(globalDir string) bool {
	_, err := os.Stat(filepath.Join(globalDir, ".tutorial"))
	return err == nil
}

// MarkTutorialDone writes a .tutorial marker to the global dir.
func MarkTutorialDone(globalDir string) {
	os.WriteFile(filepath.Join(globalDir, ".tutorial"), []byte("done\n"), 0o644)
}

// MigrateConfig migrates legacy config (minimax_api_key) to new format (keys).
func MigrateConfig(cfg *Config, legacyKey string) {
	if legacyKey != "" && cfg.Keys == nil {
		cfg.Keys = make(map[string]string)
	}
	if legacyKey != "" && cfg.Keys != nil && cfg.Keys["minimax"] == "" {
		cfg.Keys["minimax"] = legacyKey
	}
}
