package preset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkRecipeSkills_ActiveBundledRecipe(t *testing.T) {
	globalDir := t.TempDir()
	recipeDir := filepath.Join(globalDir, "recipes", "intrinsic", "adaptive", "skills", "discovery", "en")
	os.MkdirAll(recipeDir, 0o755)
	os.WriteFile(filepath.Join(recipeDir, "SKILL.md"), []byte("---\nname: discovery\ndescription: test\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "en", "adaptive", "")

	linkPath := filepath.Join(skillsDir, "adaptive", "discovery")
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

func TestLinkRecipeSkills_InactiveBundledRecipeNotLinked(t *testing.T) {
	globalDir := t.TempDir()
	// tutorial has skills but is NOT the active recipe
	tutorialSkill := filepath.Join(globalDir, "recipes", "examples", "tutorial", "skills", "guide", "en")
	os.MkdirAll(tutorialSkill, 0o755)
	os.WriteFile(filepath.Join(tutorialSkill, "SKILL.md"), []byte("---\nname: guide\ndescription: test\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	// Active recipe is "greeter", not "tutorial"
	LinkRecipeSkills(lingtaiDir, globalDir, "en", "greeter", "")

	linkPath := filepath.Join(skillsDir, "tutorial", "guide")
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("inactive bundled recipe skills should NOT be linked, but found: %v", linkPath)
	}
}

func TestLinkRecipeSkills_RootFallback(t *testing.T) {
	globalDir := t.TempDir()
	skillRoot := filepath.Join(globalDir, "recipes", "intrinsic", "plain", "skills", "helper")
	os.MkdirAll(skillRoot, 0o755)
	os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: helper\ndescription: test\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "zh", "plain", "")

	linkPath := filepath.Join(skillsDir, "plain", "helper")
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

	LinkRecipeSkills(lingtaiDir, globalDir, "en", "", customRecipe)

	linkPath := filepath.Join(skillsDir, "my-recipe", "custom-skill")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("custom recipe symlink not created: %v", err)
	}
}

func TestLinkRecipeSkills_Idempotent(t *testing.T) {
	globalDir := t.TempDir()
	recipeDir := filepath.Join(globalDir, "recipes", "intrinsic", "adaptive", "skills", "discovery", "en")
	os.MkdirAll(recipeDir, 0o755)
	os.WriteFile(filepath.Join(recipeDir, "SKILL.md"), []byte("---\nname: discovery\ndescription: test\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "en", "adaptive", "")
	LinkRecipeSkills(lingtaiDir, globalDir, "en", "adaptive", "")

	linkPath := filepath.Join(skillsDir, "adaptive", "discovery")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("symlink missing after idempotent call: %v", err)
	}
}

func TestLinkRecipeSkills_SwitchRecipeCleansOld(t *testing.T) {
	globalDir := t.TempDir()

	// Set up two bundled recipes with skills
	tutorialSkill := filepath.Join(globalDir, "recipes", "examples", "tutorial", "skills", "guide", "en")
	os.MkdirAll(tutorialSkill, 0o755)
	os.WriteFile(filepath.Join(tutorialSkill, "SKILL.md"), []byte("---\nname: guide\ndescription: test\n---\n"), 0o644)

	adaptiveSkill := filepath.Join(globalDir, "recipes", "intrinsic", "adaptive", "skills", "discovery", "en")
	os.MkdirAll(adaptiveSkill, 0o755)
	os.WriteFile(filepath.Join(adaptiveSkill, "SKILL.md"), []byte("---\nname: discovery\ndescription: test\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	// First launch with tutorial active
	LinkRecipeSkills(lingtaiDir, globalDir, "en", "tutorial", "")
	if _, err := os.Lstat(filepath.Join(skillsDir, "tutorial", "guide")); err != nil {
		t.Fatalf("tutorial skill not linked: %v", err)
	}

	// Switch to adaptive
	LinkRecipeSkills(lingtaiDir, globalDir, "en", "adaptive", "")

	// Tutorial group should be cleaned up
	if _, err := os.Stat(filepath.Join(skillsDir, "tutorial")); !os.IsNotExist(err) {
		t.Errorf("old tutorial group should be removed after switching to adaptive")
	}
	// Adaptive should be linked
	if _, err := os.Lstat(filepath.Join(skillsDir, "adaptive", "discovery")); err != nil {
		t.Fatalf("adaptive skill not linked: %v", err)
	}
}

func TestPruneStaleSkillSymlinks_RemovesBroken(t *testing.T) {
	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	brokenLink := filepath.Join(skillsDir, "stale-skill")
	os.Symlink("/nonexistent/path", brokenLink)

	groupDir := filepath.Join(skillsDir, "old-recipe")
	os.MkdirAll(groupDir, 0o755)
	brokenGroupLink := filepath.Join(groupDir, "dead-skill")
	os.Symlink("/nonexistent/path", brokenGroupLink)

	realSkill := filepath.Join(skillsDir, "custom", "real-skill")
	os.MkdirAll(realSkill, 0o755)
	os.WriteFile(filepath.Join(realSkill, "SKILL.md"), []byte("x"), 0o644)

	PruneStaleSkillSymlinks(lingtaiDir)

	if _, err := os.Lstat(brokenLink); !os.IsNotExist(err) {
		t.Errorf("broken flat symlink should have been removed")
	}
	if _, err := os.Lstat(brokenGroupLink); !os.IsNotExist(err) {
		t.Errorf("broken grouped symlink should have been removed")
	}
	if _, err := os.Stat(filepath.Join(realSkill, "SKILL.md")); err != nil {
		t.Errorf("real skill should not be touched: %v", err)
	}
}

func TestPruneStaleSkillSymlinks_KeepsValidSymlinks(t *testing.T) {
	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	groupDir := filepath.Join(skillsDir, "my-recipe")
	os.MkdirAll(groupDir, 0o755)

	targetDir := t.TempDir()
	os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte("x"), 0o644)

	validLink := filepath.Join(groupDir, "valid-skill")
	os.Symlink(targetDir, validLink)

	PruneStaleSkillSymlinks(lingtaiDir)

	if _, err := os.Lstat(validLink); err != nil {
		t.Errorf("valid symlink should be kept: %v", err)
	}
}
