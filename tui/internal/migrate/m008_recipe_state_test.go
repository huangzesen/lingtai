package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeGlobalTUIConfig writes a minimal tui_config.json with the greeting field.
func writeGlobalTUIConfig(t *testing.T, dir string, greeting *bool) {
	t.Helper()
	obj := map[string]interface{}{
		"language":       "en",
		"mail_page_size": 100,
	}
	if greeting != nil {
		obj["greeting"] = *greeting
	}
	data, _ := json.Marshal(obj)
	os.WriteFile(filepath.Join(dir, "tui_config.json"), data, 0o644)
}

// withTempHome replaces $HOME so the migration reads our fake tui_config.json.
func withTempHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	globalDir := filepath.Join(tmpHome, ".lingtai-tui")
	os.MkdirAll(globalDir, 0o755)
	return globalDir
}

func TestMigrateRecipeState_FreshDefaults(t *testing.T) {
	globalDir := withTempHome(t)
	writeGlobalTUIConfig(t, globalDir, nil)

	lingtaiDir := t.TempDir()
	orchDir := filepath.Join(lingtaiDir, "orch")
	os.MkdirAll(orchDir, 0o755)
	os.WriteFile(filepath.Join(orchDir, "init.json"), []byte(`{"manifest":{}}`), 0o644)
	os.WriteFile(filepath.Join(orchDir, ".agent.json"), []byte(`{"admin":{}}`), 0o644)

	if err := migrateRecipeState(lingtaiDir); err != nil {
		t.Fatalf("migrateRecipeState err = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(lingtaiDir, ".tui-asset", ".recipe"))
	if err != nil {
		t.Fatalf("recipe file not created: %v", err)
	}
	if !strings.Contains(string(data), `"greeter"`) {
		t.Errorf(".recipe contents = %s, want greeter", data)
	}
}

func TestMigrateRecipeState_GreetingFalse(t *testing.T) {
	globalDir := withTempHome(t)
	falseVal := false
	writeGlobalTUIConfig(t, globalDir, &falseVal)

	lingtaiDir := t.TempDir()
	orchDir := filepath.Join(lingtaiDir, "orch")
	os.MkdirAll(orchDir, 0o755)
	os.WriteFile(filepath.Join(orchDir, "init.json"), []byte(`{"manifest":{}}`), 0o644)
	os.WriteFile(filepath.Join(orchDir, ".agent.json"), []byte(`{"admin":{}}`), 0o644)

	if err := migrateRecipeState(lingtaiDir); err != nil {
		t.Fatalf("migrateRecipeState err = %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(lingtaiDir, ".tui-asset", ".recipe"))
	if !strings.Contains(string(data), `"plain"`) {
		t.Errorf(".recipe = %s, want plain", data)
	}
}

func TestMigrateRecipeState_TutorialSurvivor(t *testing.T) {
	globalDir := withTempHome(t)
	writeGlobalTUIConfig(t, globalDir, nil)

	lingtaiDir := t.TempDir()
	orchDir := filepath.Join(lingtaiDir, "tutorial")
	os.MkdirAll(orchDir, 0o755)
	os.WriteFile(filepath.Join(orchDir, "init.json"),
		[]byte(`{"manifest":{},"comment_file":"/home/user/.lingtai-tui/tutorial/tutorial.md"}`),
		0o644)
	os.WriteFile(filepath.Join(orchDir, ".agent.json"), []byte(`{"admin":{}}`), 0o644)

	if err := migrateRecipeState(lingtaiDir); err != nil {
		t.Fatalf("migrateRecipeState err = %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(lingtaiDir, ".tui-asset", ".recipe"))
	if !strings.Contains(string(data), `"tutorial"`) {
		t.Errorf(".recipe = %s, want tutorial", data)
	}
}

func TestMigrateRecipeState_AlreadyMigrated(t *testing.T) {
	_ = withTempHome(t)
	lingtaiDir := t.TempDir()
	tuiAsset := filepath.Join(lingtaiDir, ".tui-asset")
	os.MkdirAll(tuiAsset, 0o755)
	existing := []byte(`{"recipe": "plain"}`)
	os.WriteFile(filepath.Join(tuiAsset, ".recipe"), existing, 0o644)

	if err := migrateRecipeState(lingtaiDir); err != nil {
		t.Fatalf("migrateRecipeState err = %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tuiAsset, ".recipe"))
	if string(data) != string(existing) {
		t.Errorf("existing .recipe was modified: got %s, want %s", data, existing)
	}
}
