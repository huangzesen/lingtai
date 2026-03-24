package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Basic(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{
		"model": {"provider": "minimax", "model": "test", "api_key_env": "K"},
		"agent_id": "abc123",
		"agent_name": "myagent"
	}`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentID != "abc123" {
		t.Errorf("got %q, want %q", cfg.AgentID, "abc123")
	}
	if cfg.AgentName != "myagent" {
		t.Errorf("got %q, want %q", cfg.AgentName, "myagent")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{
		"model": {"provider": "minimax", "model": "test", "api_key_env": "K"},
		"agent_id": "abc123"
	}`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentPort != 8501 {
		t.Errorf("agent_port default: got %d, want %d", cfg.AgentPort, 8501)
	}
	if cfg.MaxTurns != 50 {
		t.Errorf("max_turns default: got %d, want %d", cfg.MaxTurns, 50)
	}
	if cfg.CLI != false {
		t.Error("cli default should be false")
	}
}

func TestLoadConfig_MissingAgentID(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{
		"model": {"provider": "minimax", "model": "test", "api_key_env": "K"}
	}`), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Error("expected error for missing agent_id")
	}
}

func TestLoadConfig_ModelFromFile(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.json")
	os.WriteFile(modelPath, []byte(`{
		"provider": "openai", "model": "gpt-4o", "api_key_env": "OAI_KEY"
	}`), 0644)
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{"model": "model.json", "agent_id": "abc123"}`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model.Provider != "openai" {
		t.Errorf("got provider %q, want %q", cfg.Model.Provider, "openai")
	}
}

func TestLoadConfig_ModelInline(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{
		"model": {"provider": "anthropic", "model": "claude", "api_key_env": "ANT"},
		"agent_id": "abc123"
	}`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model.Provider != "anthropic" {
		t.Errorf("got provider %q, want %q", cfg.Model.Provider, "anthropic")
	}
}

func TestLoadConfig_MissingModel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{"agent_id": "abc123"}`), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Error("expected error for missing model")
	}
}

func TestLoadDotenv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("TEST_DAEMON_VAR=hello123\n"), 0644)

	os.Unsetenv("TEST_DAEMON_VAR")
	LoadDotenv(dir)
	if v := os.Getenv("TEST_DAEMON_VAR"); v != "hello123" {
		t.Errorf("got %q, want %q", v, "hello123")
	}
	os.Unsetenv("TEST_DAEMON_VAR") // cleanup
}

func TestResolveEnvVar(t *testing.T) {
	os.Setenv("TEST_RESOLVE_KEY", "secret")
	defer os.Unsetenv("TEST_RESOLVE_KEY")

	val, err := ResolveEnvVar("TEST_RESOLVE_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "secret" {
		t.Errorf("got %q, want %q", val, "secret")
	}
}

func TestResolveEnvVar_Missing(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR")
	_, err := ResolveEnvVar("NONEXISTENT_VAR")
	if err == nil {
		t.Error("expected error for missing env var")
	}
}

func TestDisplayName(t *testing.T) {
	cfg := &Config{AgentID: "abc123", AgentName: "alice"}
	if cfg.DisplayName() != "alice" {
		t.Errorf("got %q, want %q", cfg.DisplayName(), "alice")
	}
	cfg.AgentName = ""
	if cfg.DisplayName() != "abc123" {
		t.Errorf("got %q, want %q", cfg.DisplayName(), "abc123")
	}
}
