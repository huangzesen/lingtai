package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MiniMaxAPIKey != "" {
		t.Error("expected empty api key")
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{MiniMaxAPIKey: "test-key-123"}
	if err := SaveConfig(dir, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.MiniMaxAPIKey != "test-key-123" {
		t.Errorf("api_key = %q, want %q", loaded.MiniMaxAPIKey, "test-key-123")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); os.IsNotExist(err) {
		t.Error("config.json not created")
	}
}

func TestNeedsSetup(t *testing.T) {
	dir := t.TempDir()
	if !NeedsSetup(dir) {
		t.Error("should need setup when no config.json")
	}
	SaveConfig(dir, Config{MiniMaxAPIKey: "key"})
	if NeedsSetup(dir) {
		t.Error("should not need setup after saving config")
	}
}
