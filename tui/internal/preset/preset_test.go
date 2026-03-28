package preset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withTempPresets(t *testing.T, fn func()) {
	t.Helper()
	orig := os.Getenv("HOME")
	tmp := t.TempDir()
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", orig)
	fn()
}

func TestList_EmptyDir(t *testing.T) {
	withTempPresets(t, func() {
		presets, err := List()
		if err != nil {
			t.Fatalf("List() error: %v", err)
		}
		if len(presets) != 0 {
			t.Errorf("expected 0 presets, got %d", len(presets))
		}
	})
}

func TestSaveAndLoad_Roundtrip(t *testing.T) {
	withTempPresets(t, func() {
		p := DefaultPreset()
		if err := Save(p); err != nil {
			t.Fatalf("Save() error: %v", err)
		}
		loaded, err := Load(p.Name)
		if err != nil {
			t.Fatalf("Load() error: %v", err)
		}
		if loaded.Name != p.Name {
			t.Errorf("name = %q, want %q", loaded.Name, p.Name)
		}
		if loaded.Description != p.Description {
			t.Errorf("description = %q, want %q", loaded.Description, p.Description)
		}
	})
}

func TestEnsureDefault_CreatesBuiltinPresets(t *testing.T) {
	withTempPresets(t, func() {
		if err := EnsureDefault(); err != nil {
			t.Fatalf("EnsureDefault() error: %v", err)
		}
		presets, _ := List()
		if len(presets) != 2 {
			t.Fatalf("expected 2 presets, got %d", len(presets))
		}
		names := map[string]bool{}
		for _, p := range presets {
			names[p.Name] = true
		}
		for _, want := range []string{"minimax", "custom"} {
			if !names[want] {
				t.Errorf("missing preset %q", want)
			}
		}
	})
}

func TestGenerateInitJSON_ProducesValidJSON(t *testing.T) {
	withTempPresets(t, func() {
		p := DefaultPreset()
		tmpDir := t.TempDir()
		lingtaiDir := filepath.Join(tmpDir, ".lingtai")
		os.MkdirAll(lingtaiDir, 0o755)

		globalDir := filepath.Join(tmpDir, ".lingtai-global")
		Bootstrap(globalDir)
		if err := GenerateInitJSON(p, "test-agent", "test-agent", lingtaiDir, globalDir); err != nil {
			t.Fatalf("GenerateInitJSON() error: %v", err)
		}

		// Check init.json exists and is valid
		initPath := filepath.Join(lingtaiDir, "test-agent", "init.json")
		data, err := os.ReadFile(initPath)
		if err != nil {
			t.Fatalf("read init.json: %v", err)
		}
		var initJSON map[string]interface{}
		if err := json.Unmarshal(data, &initJSON); err != nil {
			t.Fatalf("parse init.json: %v", err)
		}

		// Check required fields
		manifest, ok := initJSON["manifest"].(map[string]interface{})
		if !ok {
			t.Fatal("manifest not a map")
		}
		for _, key := range []string{"agent_name", "language", "llm", "capabilities", "admin", "streaming", "max_turns"} {
			if _, exists := manifest[key]; !exists {
				t.Errorf("manifest missing key %q", key)
			}
		}
		if manifest["agent_name"] != "test-agent" {
			t.Errorf("agent_name = %v, want %q", manifest["agent_name"], "test-agent")
		}

		// Check .agent.json exists
		agentPath := filepath.Join(lingtaiDir, "test-agent", ".agent.json")
		if _, err := os.Stat(agentPath); err != nil {
			t.Errorf(".agent.json not created: %v", err)
		}
	})
}

func TestDelete_RemovesFile(t *testing.T) {
	withTempPresets(t, func() {
		p := DefaultPreset()
		Save(p)
		if err := Delete(p.Name); err != nil {
			t.Fatalf("Delete() error: %v", err)
		}
		presets, _ := List()
		if len(presets) != 0 {
			t.Errorf("expected 0 presets after delete, got %d", len(presets))
		}
	})
}

func TestHasAny(t *testing.T) {
	withTempPresets(t, func() {
		if HasAny() {
			t.Error("HasAny() = true, want false on empty dir")
		}
		Save(DefaultPreset())
		if !HasAny() {
			t.Error("HasAny() = false, want true after save")
		}
	})
}

func TestMigrateAddonTemplates(t *testing.T) {
	tmp := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", orig)

	Bootstrap(tmp)

	for _, addon := range []string{"imap", "telegram"} {
		path := filepath.Join(tmp, "addons", addon, "example", "config.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s example not created: %v", addon, err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("%s example is invalid JSON: %v\nContent: %s", addon, err, string(data))
		}
		if len(parsed) == 0 {
			t.Errorf("%s example is empty", addon)
		}
		preview := string(data)
		if len(preview) > 80 {
			preview = preview[:80]
		}
		t.Logf("%s: %d keys — %s", addon, len(parsed), preview)
	}
}
