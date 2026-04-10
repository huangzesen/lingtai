package secretary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGreetContent(t *testing.T) {
	content := GreetContent()
	if content == "" {
		t.Fatal("GreetContent() returned empty string")
	}
	if !strings.Contains(content, "secretary") {
		t.Error("greet content should mention 'secretary'")
	}
}

func TestRecipeDir(t *testing.T) {
	tmpDir := t.TempDir()
	dir, err := RecipeDir(tmpDir)
	if err != nil {
		t.Fatalf("RecipeDir() error: %v", err)
	}
	if dir == "" {
		t.Fatal("RecipeDir() returned empty string")
	}

	// Verify key files exist
	for _, file := range []string{
		"greet.md",
		"comment.md",
		"covenant.md",
		"skills/briefing/SKILL.md",
	} {
		path := filepath.Join(dir, file)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist: %v", file, err)
		}
	}
}

func TestDirectoryPaths(t *testing.T) {
	base := "/home/user/.lingtai-tui"
	if dir := ProjectDir(base); dir != base+"/secretary" {
		t.Errorf("ProjectDir = %q, want %s/secretary", dir, base)
	}
	if dir := LingtaiDir(base); dir != base+"/secretary/.lingtai" {
		t.Errorf("LingtaiDir = %q, want %s/secretary/.lingtai", dir, base)
	}
	if dir := AgentDir(base); dir != base+"/secretary/.lingtai/secretary" {
		t.Errorf("AgentDir = %q, want %s/secretary/.lingtai/secretary", dir, base)
	}
}

func TestPopulateAssetsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dir, err := RecipeDir(tmpDir)
	if err != nil {
		t.Fatalf("first RecipeDir() error: %v", err)
	}

	// Call again — should overwrite without error
	dir2, err := RecipeDir(tmpDir)
	if err != nil {
		t.Fatalf("second RecipeDir() error: %v", err)
	}
	if dir != dir2 {
		t.Errorf("RecipeDir changed: %q vs %q", dir, dir2)
	}
}
