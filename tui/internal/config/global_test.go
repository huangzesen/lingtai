package config

import (
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
		"custom":  "test-custom-key",
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
	if loaded.Keys["custom"] != "test-custom-key" {
		t.Errorf("Keys[custom] = %q, want %q", loaded.Keys["custom"], "test-custom-key")
	}
}


