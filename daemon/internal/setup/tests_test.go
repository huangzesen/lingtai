package setup

import (
	"os"
	"path/filepath"
	"testing"

	"stoai-daemon/internal/config"
)

func TestTestEnvVar_Set(t *testing.T) {
	t.Setenv("TEST_SETUP_VAR", "value")
	result := TestEnvVar("TEST_SETUP_VAR")
	if !result.OK {
		t.Error("expected OK for set env var")
	}
}

func TestTestEnvVar_Missing(t *testing.T) {
	result := TestEnvVar("DEFINITELY_NOT_SET_XYZ")
	if result.OK {
		t.Error("expected failure for missing env var")
	}
}

func TestWizardConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Simulate what the wizard writes
	modelJSON := `{"provider": "minimax", "model": "test-model", "api_key_env": "TEST_KEY"}`
	os.WriteFile(filepath.Join(dir, "model.json"), []byte(modelJSON), 0644)

	configJSON := `{
		"model": "model.json",
		"agent_name": "wizard-test",
		"agent_port": 9000,
		"base_dir": "` + dir + `/agents"
	}`
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(configJSON), 0644)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}
	if cfg.AgentName != "wizard-test" {
		t.Errorf("agent_name: got %q, want %q", cfg.AgentName, "wizard-test")
	}
	if cfg.Model.Provider != "minimax" {
		t.Errorf("model.provider: got %q, want %q", cfg.Model.Provider, "minimax")
	}
	if cfg.AgentPort != 9000 {
		t.Errorf("agent_port: got %d, want %d", cfg.AgentPort, 9000)
	}
}
