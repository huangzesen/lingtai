package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Keys map[string]string `json:"keys,omitempty"` // provider → key, e.g. {"minimax": "xxx", "gemini": "xxx"}
}

func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".lingtai")
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
	if err := json.Unmarshal(data, &cfg); err == nil && cfg.Keys != nil {
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
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)
}

func NeedsSetup(dir string) bool {
	cfg, err := LoadConfig(dir)
	if err != nil {
		return true
	}
	return len(cfg.Keys) == 0
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
