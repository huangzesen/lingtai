package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	MiniMaxAPIKey string `json:"minimax_api_key,omitempty"`
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
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)
}

func NeedsSetup(dir string) bool {
	cfg, err := LoadConfig(dir)
	if err != nil {
		return true
	}
	return cfg.MiniMaxAPIKey == ""
}
