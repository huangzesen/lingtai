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
	if cfg.Keys != nil && len(cfg.Keys) > 0 {
		t.Error("expected empty keys")
	}
}

func TestSaveAndLoadConfig_NewFormat(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Keys: map[string]string{
		"minimax": "test-minimax-key",
		"gemini":  "test-gemini-key",
	}}
	if err := SaveConfig(dir, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Keys == nil {
		t.Fatal("Keys is nil after load")
	}
	if loaded.Keys["minimax"] != "test-minimax-key" {
		t.Errorf("Keys[minimax] = %q, want %q", loaded.Keys["minimax"], "test-minimax-key")
	}
	if loaded.Keys["gemini"] != "test-gemini-key" {
		t.Errorf("Keys[gemini] = %q, want %q", loaded.Keys["gemini"], "test-gemini-key")
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); os.IsNotExist(err) {
		t.Error("config.json not created")
	}
}

func TestNeedsSetup_WithKeys(t *testing.T) {
	dir := t.TempDir()
	if !NeedsSetup(dir) {
		t.Error("should need setup when no config.json")
	}
	SaveConfig(dir, Config{Keys: map[string]string{"minimax": "key"}})
	if NeedsSetup(dir) {
		t.Error("should not need setup after saving config with keys")
	}
}

func TestNeedsSetup_EmptyKeys(t *testing.T) {
	dir := t.TempDir()
	// Empty keys map should also trigger needs setup
	SaveConfig(dir, Config{Keys: map[string]string{}})
	if !NeedsSetup(dir) {
		t.Error("should need setup with empty keys map")
	}
}

func TestLoadConfig_LegacyMigration(t *testing.T) {
	dir := t.TempDir()
	// Write legacy format directly
	legacyJSON := `{"minimax_api_key": "legacy-minimax-key"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(legacyJSON), 0644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	// Load should migrate
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("load after migration: %v", err)
	}
	if cfg.Keys == nil {
		t.Fatal("Keys should be initialized after migration")
	}
	if cfg.Keys["minimax"] != "legacy-minimax-key" {
		t.Errorf("Keys[minimax] = %q, want %q (migrated)", cfg.Keys["minimax"], "legacy-minimax-key")
	}
}

func TestMigrateConfig(t *testing.T) {
	cfg := &Config{}
	MigrateConfig(cfg, "legacy-key")
	if cfg.Keys == nil {
		t.Fatal("Keys should be initialized")
	}
	if cfg.Keys["minimax"] != "legacy-key" {
		t.Errorf("Keys[minimax] = %q, want %q", cfg.Keys["minimax"], "legacy-key")
	}
}

func TestMigrateConfig_Empty(t *testing.T) {
	cfg := &Config{Keys: map[string]string{"gemini": "existing-gemini"}}
	MigrateConfig(cfg, "legacy-minimax")
	// Should not overwrite existing keys
	if cfg.Keys["minimax"] != "legacy-minimax" {
		t.Errorf("Keys[minimax] = %q, want %q", cfg.Keys["minimax"], "legacy-minimax")
	}
	if cfg.Keys["gemini"] != "existing-gemini" {
		t.Errorf("Keys[gemini] = %q, should not change", cfg.Keys["gemini"])
	}
}
