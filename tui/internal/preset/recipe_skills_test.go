package preset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkRecipeSkills_BundledRecipe(t *testing.T) {
	globalDir := t.TempDir()
	recipeDir := filepath.Join(globalDir, "recipes", "adaptive", "skills", "discovery", "en")
	os.MkdirAll(recipeDir, 0o755)
	os.WriteFile(filepath.Join(recipeDir, "SKILL.md"), []byte("---\nname: discovery\ndescription: test\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "en", "")

	linkPath := filepath.Join(skillsDir, "adaptive-discovery-en")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink, got %v", info.Mode())
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != recipeDir {
		t.Errorf("symlink target = %q, want %q", target, recipeDir)
	}
}

func TestLinkRecipeSkills_RootFallback(t *testing.T) {
	globalDir := t.TempDir()
	skillRoot := filepath.Join(globalDir, "recipes", "plain", "skills", "helper")
	os.MkdirAll(skillRoot, 0o755)
	os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: helper\ndescription: test\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "zh", "")

	linkPath := filepath.Join(skillsDir, "plain-helper")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("symlink not created for root fallback: %v", err)
	}
}

func TestLinkRecipeSkills_CustomRecipe(t *testing.T) {
	globalDir := t.TempDir()

	customRecipe := filepath.Join(t.TempDir(), "my-recipe")
	os.MkdirAll(customRecipe, 0o755)
	skillDir := filepath.Join(customRecipe, "skills", "custom-skill", "en")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: custom-skill\ndescription: test\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "en", customRecipe)

	linkPath := filepath.Join(skillsDir, "my-recipe-custom-skill-en")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("custom recipe symlink not created: %v", err)
	}
}

func TestLinkRecipeSkills_CollisionSkipsBundledWins(t *testing.T) {
	globalDir := t.TempDir()
	bundledSkill := filepath.Join(globalDir, "recipes", "adaptive", "skills", "guide", "en")
	os.MkdirAll(bundledSkill, 0o755)
	os.WriteFile(filepath.Join(bundledSkill, "SKILL.md"), []byte("---\nname: guide\ndescription: bundled\n---\n"), 0o644)

	customDir := filepath.Join(t.TempDir(), "adaptive")
	os.MkdirAll(customDir, 0o755)
	customSkill := filepath.Join(customDir, "skills", "guide", "en")
	os.MkdirAll(customSkill, 0o755)
	os.WriteFile(filepath.Join(customSkill, "SKILL.md"), []byte("---\nname: guide\ndescription: custom\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "en", customDir)

	linkPath := filepath.Join(skillsDir, "adaptive-guide-en")
	target, _ := os.Readlink(linkPath)
	if target != bundledSkill {
		t.Errorf("collision: expected bundled to win, got target %q, want %q", target, bundledSkill)
	}
}

func TestLinkRecipeSkills_Idempotent(t *testing.T) {
	globalDir := t.TempDir()
	recipeDir := filepath.Join(globalDir, "recipes", "adaptive", "skills", "discovery", "en")
	os.MkdirAll(recipeDir, 0o755)
	os.WriteFile(filepath.Join(recipeDir, "SKILL.md"), []byte("---\nname: discovery\ndescription: test\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "en", "")
	LinkRecipeSkills(lingtaiDir, globalDir, "en", "")

	linkPath := filepath.Join(skillsDir, "adaptive-discovery-en")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("symlink missing after idempotent call: %v", err)
	}
}

func TestPruneStaleSkillSymlinks_RemovesBroken(t *testing.T) {
	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	brokenLink := filepath.Join(skillsDir, "stale-skill-en")
	os.Symlink("/nonexistent/path", brokenLink)

	realSkill := filepath.Join(skillsDir, "real-skill")
	os.MkdirAll(realSkill, 0o755)
	os.WriteFile(filepath.Join(realSkill, "SKILL.md"), []byte("x"), 0o644)

	PruneStaleSkillSymlinks(lingtaiDir)

	if _, err := os.Lstat(brokenLink); !os.IsNotExist(err) {
		t.Errorf("broken symlink should have been removed")
	}
	if _, err := os.Stat(filepath.Join(realSkill, "SKILL.md")); err != nil {
		t.Errorf("real skill should not be touched: %v", err)
	}
}

func TestPruneStaleSkillSymlinks_KeepsValidSymlinks(t *testing.T) {
	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	targetDir := t.TempDir()
	os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte("x"), 0o644)

	validLink := filepath.Join(skillsDir, "valid-skill-en")
	os.Symlink(targetDir, validLink)

	PruneStaleSkillSymlinks(lingtaiDir)

	if _, err := os.Lstat(validLink); err != nil {
		t.Errorf("valid symlink should be kept: %v", err)
	}
}
