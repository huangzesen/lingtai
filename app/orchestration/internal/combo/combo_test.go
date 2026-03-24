package combo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	// Use a temp dir as home
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	c := Combo{
		Name:   "test-combo",
		Model:  map[string]interface{}{"provider": "gemini", "model": "gemini-2.5-pro"},
		Config: map[string]interface{}{"agent_name": "alice", "agent_port": float64(8501)},
		Env:    map[string]string{"GEMINI_API_KEY": "sk-test"},
	}

	if err := Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file permissions
	path := filepath.Join(Dir(), "test-combo.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600 permissions, got %o", perm)
	}

	// Load
	loaded, err := Load("test-combo")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != "test-combo" {
		t.Errorf("Name: got %q, want %q", loaded.Name, "test-combo")
	}
	if loaded.Env["GEMINI_API_KEY"] != "sk-test" {
		t.Errorf("Env: got %q", loaded.Env["GEMINI_API_KEY"])
	}
}

func TestList(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	// Empty list
	combos, err := List()
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(combos) != 0 {
		t.Errorf("expected 0 combos, got %d", len(combos))
	}

	// Save two combos
	Save(Combo{Name: "beta", Model: map[string]interface{}{"provider": "openai"}})
	Save(Combo{Name: "alpha", Model: map[string]interface{}{"provider": "gemini"}})

	combos, err = List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(combos) != 2 {
		t.Fatalf("expected 2 combos, got %d", len(combos))
	}
	// Should be sorted by name
	if combos[0].Name != "alpha" {
		t.Errorf("expected first combo 'alpha', got %q", combos[0].Name)
	}
	if combos[1].Name != "beta" {
		t.Errorf("expected second combo 'beta', got %q", combos[1].Name)
	}
}

func TestLoadNotFound(t *testing.T) {
	tmp := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", origHome)

	_, err := Load("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent combo")
	}
}
