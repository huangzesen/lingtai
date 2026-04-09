# Recipe-Shipped Skills Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow recipes to ship skills that are automatically symlinked into `.lingtai/.skills/` on TUI startup, with i18n resolution, collision detection, and stale symlink pruning.

**Architecture:** New functions in `preset` package (`ResolveSkillDir`, `LinkRecipeSkills`, `PruneStaleSkillSymlinks`), a fix to `scanSkills()` in `tui/skills.go` to follow symlinks, call sites in `main.go` and `launcher.go`, and a new `lingtai-recipe` bundled skill.

**Tech Stack:** Go, `os.Symlink`, `os.Lstat`, `os.Stat`, existing `preset` and `tui` packages.

---

## File Structure

| File | Responsibility |
|---|---|
| `tui/internal/preset/recipes.go` | Add `ResolveSkillDir()` — i18n resolution for recipe skill directories |
| `tui/internal/preset/recipe_skills.go` | **New.** `LinkRecipeSkills()` and `PruneStaleSkillSymlinks()` — symlink lifecycle |
| `tui/internal/preset/recipe_skills_test.go` | **New.** Tests for `ResolveSkillDir`, `LinkRecipeSkills`, `PruneStaleSkillSymlinks` |
| `tui/internal/tui/skills.go` | Fix `scanSkills()` to follow symlinks |
| `tui/internal/tui/skills_test.go` | **New.** Test that `scanSkills()` discovers symlinked skill directories |
| `tui/main.go` | Call `LinkRecipeSkills` + `PruneStaleSkillSymlinks` after `PopulateBundledSkills` |
| `tui/internal/process/launcher.go` | Same call site addition |
| `tui/internal/preset/skills/lingtai-recipe/en/SKILL.md` | **New.** Self-documenting recipe skill (English) |
| `tui/internal/preset/skills/lingtai-recipe/zh/SKILL.md` | **New.** Self-documenting recipe skill (Chinese) |

---

### Task 1: Add `ResolveSkillDir` to recipes.go

**Files:**
- Modify: `tui/internal/preset/recipes.go`
- Modify: `tui/internal/preset/recipes_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `tui/internal/preset/recipes_test.go`:

```go
func TestResolveSkillDir_LangSpecific(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "my-skill", "en")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644)

	got := ResolveSkillDir(dir, "my-skill", "en")
	if got != skillDir {
		t.Errorf("ResolveSkillDir lang-specific = %q, want %q", got, skillDir)
	}
}

func TestResolveSkillDir_FallbackToRoot(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644)

	got := ResolveSkillDir(dir, "my-skill", "zh")
	if got != skillDir {
		t.Errorf("ResolveSkillDir fallback = %q, want %q", got, skillDir)
	}
}

func TestResolveSkillDir_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "skills", "my-skill"), 0o755)
	// No SKILL.md anywhere

	got := ResolveSkillDir(dir, "my-skill", "en")
	if got != "" {
		t.Errorf("ResolveSkillDir no match = %q, want empty", got)
	}
}

func TestResolveSkillDir_EmptyRecipeDir(t *testing.T) {
	got := ResolveSkillDir("", "my-skill", "en")
	if got != "" {
		t.Errorf("ResolveSkillDir empty recipeDir = %q, want empty", got)
	}
}

func TestResolveSkillDir_EmptyLang(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644)

	got := ResolveSkillDir(dir, "my-skill", "")
	if got != skillDir {
		t.Errorf("ResolveSkillDir empty lang = %q, want %q", got, skillDir)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd tui && go test ./internal/preset/ -run TestResolveSkillDir -v`
Expected: FAIL — `ResolveSkillDir` undefined.

- [ ] **Step 3: Implement `ResolveSkillDir`**

Add to `tui/internal/preset/recipes.go`:

```go
// ResolveSkillDir returns the absolute path to a skill directory within a
// recipe, applying the per-lang fallback rule:
//  1. <recipeDir>/skills/<skillName>/<lang>/SKILL.md exists → return that dir
//  2. <recipeDir>/skills/<skillName>/SKILL.md exists → return that dir
//  3. empty string (no match)
func ResolveSkillDir(recipeDir, skillName, lang string) string {
	if recipeDir == "" {
		return ""
	}
	base := filepath.Join(recipeDir, "skills", skillName)
	// 1. Try lang-specific
	if lang != "" {
		langDir := filepath.Join(base, lang)
		if info, err := os.Stat(filepath.Join(langDir, "SKILL.md")); err == nil && !info.IsDir() {
			return langDir
		}
	}
	// 2. Try root
	if info, err := os.Stat(filepath.Join(base, "SKILL.md")); err == nil && !info.IsDir() {
		return base
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd tui && go test ./internal/preset/ -run TestResolveSkillDir -v`
Expected: PASS (all 5 tests).

- [ ] **Step 5: Commit**

```bash
git add tui/internal/preset/recipes.go tui/internal/preset/recipes_test.go
git commit -m "feat(preset): add ResolveSkillDir for recipe skill i18n resolution"
```

---

### Task 2: Implement `LinkRecipeSkills` and `PruneStaleSkillSymlinks`

**Files:**
- Create: `tui/internal/preset/recipe_skills.go`
- Create: `tui/internal/preset/recipe_skills_test.go`

- [ ] **Step 1: Write the failing tests**

Create `tui/internal/preset/recipe_skills_test.go`:

```go
package preset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkRecipeSkills_BundledRecipe(t *testing.T) {
	// Set up a fake globalDir with a recipe that has a skill
	globalDir := t.TempDir()
	recipeDir := filepath.Join(globalDir, "recipes", "adaptive", "skills", "discovery", "en")
	os.MkdirAll(recipeDir, 0o755)
	os.WriteFile(filepath.Join(recipeDir, "SKILL.md"), []byte("---\nname: discovery\ndescription: test\n---\n"), 0o644)

	// Set up lingtaiDir with .skills/
	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "en", "")

	// Check symlink exists
	linkPath := filepath.Join(skillsDir, "adaptive-discovery-en")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("symlink not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected symlink, got %v", info.Mode())
	}

	// Check symlink target
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

	// Root fallback: no lang suffix
	linkPath := filepath.Join(skillsDir, "plain-helper")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("symlink not created for root fallback: %v", err)
	}
}

func TestLinkRecipeSkills_CustomRecipe(t *testing.T) {
	globalDir := t.TempDir()
	// No bundled recipe skills

	customDir := t.TempDir()
	// Rename customDir to have a predictable basename
	customRecipe := filepath.Join(filepath.Dir(customDir), "my-recipe")
	os.Rename(customDir, customRecipe)
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
	// Bundled recipe with a skill
	bundledSkill := filepath.Join(globalDir, "recipes", "adaptive", "skills", "guide", "en")
	os.MkdirAll(bundledSkill, 0o755)
	os.WriteFile(filepath.Join(bundledSkill, "SKILL.md"), []byte("---\nname: guide\ndescription: bundled\n---\n"), 0o644)

	// Custom recipe dir also named "adaptive" (collision)
	customDir := filepath.Join(t.TempDir(), "adaptive")
	os.MkdirAll(customDir, 0o755)
	customSkill := filepath.Join(customDir, "skills", "guide", "en")
	os.MkdirAll(customSkill, 0o755)
	os.WriteFile(filepath.Join(customSkill, "SKILL.md"), []byte("---\nname: guide\ndescription: custom\n---\n"), 0o644)

	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	LinkRecipeSkills(lingtaiDir, globalDir, "en", customDir)

	// Bundled should win — symlink target is the bundled path
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

	// Run twice — should not error
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

	// Create a broken symlink
	brokenLink := filepath.Join(skillsDir, "stale-skill-en")
	os.Symlink("/nonexistent/path", brokenLink)

	// Create a valid non-symlink directory (should not be touched)
	realSkill := filepath.Join(skillsDir, "real-skill")
	os.MkdirAll(realSkill, 0o755)
	os.WriteFile(filepath.Join(realSkill, "SKILL.md"), []byte("x"), 0o644)

	PruneStaleSkillSymlinks(lingtaiDir)

	// Broken symlink removed
	if _, err := os.Lstat(brokenLink); !os.IsNotExist(err) {
		t.Errorf("broken symlink should have been removed")
	}
	// Real skill untouched
	if _, err := os.Stat(filepath.Join(realSkill, "SKILL.md")); err != nil {
		t.Errorf("real skill should not be touched: %v", err)
	}
}

func TestPruneStaleSkillSymlinks_KeepsValidSymlinks(t *testing.T) {
	lingtaiDir := t.TempDir()
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	// Create a valid symlink target
	targetDir := t.TempDir()
	os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte("x"), 0o644)

	validLink := filepath.Join(skillsDir, "valid-skill-en")
	os.Symlink(targetDir, validLink)

	PruneStaleSkillSymlinks(lingtaiDir)

	// Valid symlink kept
	if _, err := os.Lstat(validLink); err != nil {
		t.Errorf("valid symlink should be kept: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd tui && go test ./internal/preset/ -run "TestLinkRecipeSkills|TestPruneStale" -v`
Expected: FAIL — `LinkRecipeSkills` and `PruneStaleSkillSymlinks` undefined.

- [ ] **Step 3: Implement `LinkRecipeSkills` and `PruneStaleSkillSymlinks`**

Create `tui/internal/preset/recipe_skills.go`:

```go
package preset

import (
	"fmt"
	"os"
	"path/filepath"
)

// LinkRecipeSkills creates symlinks in <lingtaiDir>/.skills/ for every skill
// found in every known recipe directory. Called on every TUI startup after
// PopulateBundledSkills().
//
// All recipes' skills are linked simultaneously — switching recipes only
// affects greet.md/comment.md, not skill availability. Bundled recipes are
// linked first and win on name collisions.
//
// Symlink naming: <recipe-dirname>-<skill-name>-<lang> (lang-specific) or
// <recipe-dirname>-<skill-name> (root fallback).
func LinkRecipeSkills(lingtaiDir, globalDir, lang, customDir string) {
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	// Track claimed symlink names for collision detection.
	// Bundled recipes are processed first and win collisions.
	claimed := make(map[string]string) // symlink name → recipe that claimed it

	// 1. Bundled recipes
	recipesRoot := filepath.Join(globalDir, "recipes")
	if entries, err := os.ReadDir(recipesRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			recipeDir := filepath.Join(recipesRoot, e.Name())
			linkRecipeDir(skillsDir, recipeDir, e.Name(), lang, claimed)
		}
	}

	// 2. Custom recipe (if set)
	if customDir != "" {
		recipeName := filepath.Base(customDir)
		linkRecipeDir(skillsDir, customDir, recipeName, lang, claimed)
	}

	// 3. Agora projects
	home, err := os.UserHomeDir()
	if err == nil {
		agoraRoot := filepath.Join(home, "lingtai-agora", "projects")
		if entries, err := os.ReadDir(agoraRoot); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				recipeDir := filepath.Join(agoraRoot, e.Name(), ".lingtai-recipe")
				if info, err := os.Stat(recipeDir); err == nil && info.IsDir() {
					linkRecipeDir(skillsDir, recipeDir, e.Name(), lang, claimed)
				}
			}
		}
	}
}

// linkRecipeDir symlinks all skills from a single recipe directory into skillsDir.
func linkRecipeDir(skillsDir, recipeDir, recipeName, lang string, claimed map[string]string) {
	skillsRoot := filepath.Join(recipeDir, "skills")
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return // no skills/ directory — normal for most recipes
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "" || e.Name()[0] == '.' {
			continue
		}
		skillName := e.Name()
		resolved := ResolveSkillDir(recipeDir, skillName, lang)
		if resolved == "" {
			continue
		}

		// Compute symlink name
		var linkName string
		langDir := filepath.Join(recipeDir, "skills", skillName, lang)
		if resolved == langDir {
			linkName = fmt.Sprintf("%s-%s-%s", recipeName, skillName, lang)
		} else {
			linkName = fmt.Sprintf("%s-%s", recipeName, skillName)
		}

		// Collision detection: first writer wins
		if owner, exists := claimed[linkName]; exists {
			if owner != recipeName {
				fmt.Fprintf(os.Stderr, "warning: recipe skill %q from %q collides with %q — skipped\n", linkName, recipeName, owner)
			}
			continue
		}
		claimed[linkName] = recipeName

		linkPath := filepath.Join(skillsDir, linkName)

		// Check if symlink already exists and points to the correct target
		if existing, err := os.Readlink(linkPath); err == nil {
			if existing == resolved {
				continue // already correct, skip
			}
			// Wrong target (e.g., lang changed) — remove and recreate
			os.Remove(linkPath)
		} else {
			// Not a symlink — might be a regular dir from PopulateBundledSkills
			// or a broken state. Remove if it exists.
			os.Remove(linkPath)
		}

		os.Symlink(resolved, linkPath)
	}
}

// PruneStaleSkillSymlinks scans <lingtaiDir>/.skills/ and removes any
// symlinks whose target no longer exists. Non-symlink entries (bundled
// skills written by PopulateBundledSkills) are never touched.
func PruneStaleSkillSymlinks(lingtaiDir string) {
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(skillsDir, e.Name())
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue // not a symlink — leave it alone
		}
		// Check if target exists
		if _, err := os.Stat(path); err != nil {
			// Broken symlink — remove
			os.Remove(path)
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd tui && go test ./internal/preset/ -run "TestLinkRecipeSkills|TestPruneStale" -v`
Expected: PASS (all 7 tests).

- [ ] **Step 5: Commit**

```bash
git add tui/internal/preset/recipe_skills.go tui/internal/preset/recipe_skills_test.go
git commit -m "feat(preset): add LinkRecipeSkills and PruneStaleSkillSymlinks"
```

---

### Task 3: Fix `scanSkills()` to follow symlinks

**Files:**
- Modify: `tui/internal/tui/skills.go:86-98`
- Create: `tui/internal/tui/skills_test.go`

- [ ] **Step 1: Write the failing test**

Create `tui/internal/tui/skills_test.go`:

```go
package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSkills_FollowsSymlinks(t *testing.T) {
	// Create a real skill directory somewhere
	targetDir := t.TempDir()
	os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte("---\nname: symlinked-skill\ndescription: A symlinked skill\nversion: 1.0.0\n---\nBody here.\n"), 0o644)

	// Create .skills/ with a symlink to it
	skillsDir := filepath.Join(t.TempDir(), ".skills")
	os.MkdirAll(skillsDir, 0o755)
	os.Symlink(targetDir, filepath.Join(skillsDir, "test-skill-en"))

	// Also create a regular (non-symlinked) skill for comparison
	regularDir := filepath.Join(skillsDir, "regular-skill")
	os.MkdirAll(regularDir, 0o755)
	os.WriteFile(filepath.Join(regularDir, "SKILL.md"), []byte("---\nname: regular-skill\ndescription: A regular skill\nversion: 1.0.0\n---\nBody.\n"), 0o644)

	skills, problems := scanSkills(skillsDir)
	if len(problems) != 0 {
		t.Errorf("unexpected problems: %v", problems)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Skills are sorted by name
	names := []string{skills[0].Name, skills[1].Name}
	if names[0] != "regular-skill" || names[1] != "symlinked-skill" {
		t.Errorf("skill names = %v, want [regular-skill, symlinked-skill]", names)
	}
}

func TestScanSkills_SkipsBrokenSymlinks(t *testing.T) {
	skillsDir := filepath.Join(t.TempDir(), ".skills")
	os.MkdirAll(skillsDir, 0o755)

	// Broken symlink
	os.Symlink("/nonexistent", filepath.Join(skillsDir, "broken-skill"))

	skills, problems := scanSkills(skillsDir)
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
	// Broken symlinks are silently skipped, not reported as problems
	// (PruneStaleSkillSymlinks handles cleanup separately)
	if len(problems) != 0 {
		t.Errorf("expected 0 problems, got %d", len(problems))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd tui && go test ./internal/tui/ -run "TestScanSkills" -v`
Expected: FAIL — `TestScanSkills_FollowsSymlinks` finds only 1 skill (the regular one; the symlinked one is skipped by the current `e.IsDir()` check).

- [ ] **Step 3: Fix `scanSkills()` to follow symlinks**

In `tui/internal/tui/skills.go`, replace lines 95-98:

```go
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
```

With:

```go
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		// Use os.Stat (follows symlinks) instead of e.IsDir() so that
		// symlinked skill directories from recipes are discovered.
		info, err := os.Stat(filepath.Join(skillsDir, e.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd tui && go test ./internal/tui/ -run "TestScanSkills" -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add tui/internal/tui/skills.go tui/internal/tui/skills_test.go
git commit -m "fix(tui): scanSkills follows symlinks for recipe skills"
```

---

### Task 4: Wire call sites in `main.go` and `launcher.go`

**Files:**
- Modify: `tui/main.go:184`
- Modify: `tui/internal/process/launcher.go:52`

- [ ] **Step 1: Add calls in `main.go`**

In `tui/main.go`, after line 184 (`preset.PopulateBundledSkills(lingtaiDir)`), add:

```go
	// Recipe skills — symlink skills from all known recipes into .lingtai/.skills/.
	// Runs after PopulateBundledSkills so bundled skills (regular dirs) are already
	// in place, and after LoadTUIConfig so we have the user's language.
	// Note: at this point tuiCfg may not yet be loaded (it's loaded at line 200).
	// We read recipe state and lang here; for first-run, recipe skills are picked
	// up on the next TUI launch after the wizard completes.
```

Wait — `tuiCfg` is loaded at line 200, *after* line 184. We need to either move the call below line 200, or read the lang separately. Let me check if we can read the lang from the TUI config file directly.

Actually, the cleanest approach: place the `LinkRecipeSkills` call after line 201 (`i18n.SetLang(tuiCfg.Language)`), inside the `!needsFirstRun` block at line 205 (since on first run there's no recipe state yet). But bundled recipe skills should still be linked on first run... 

The best approach: place it after line 201 unconditionally (we always have `tuiCfg.Language` by then). On first run, `LoadRecipeState` returns zero-value (no custom dir), so only bundled recipe skills get linked — which is correct.

In `tui/main.go`, after line 201 (`i18n.SetLang(tuiCfg.Language)`), add:

```go
	// Recipe skills — symlink skills from all known recipes into .lingtai/.skills/.
	// On first run, no custom recipe is set yet, so only bundled recipe skills are
	// linked. Custom/agora recipe skills are picked up on the next launch.
	if recipeState, err := preset.LoadRecipeState(lingtaiDir); err == nil {
		preset.LinkRecipeSkills(lingtaiDir, globalDir, tuiCfg.Language, recipeState.CustomDir)
	}
	preset.PruneStaleSkillSymlinks(lingtaiDir)
```

- [ ] **Step 2: Add calls in `launcher.go`**

In `tui/internal/process/launcher.go`, after line 52 (`preset.PopulateBundledSkills(lingtaiDir)`), add:

```go
	// Recipe skills — symlink from all recipes. On InitProject we don't have
	// lang/recipe state yet, so pass defaults. The main.go call site handles
	// the full linking with actual config values.
```

Actually, `launcher.go:52` is inside `InitProject()` which runs once when `.lingtai/` is first created. At that point there's no recipe state and no TUI config loaded yet. The `main.go` call site (after line 201) handles the real linking. We should **not** add `LinkRecipeSkills` to `launcher.go` — it would run without lang/recipe info and duplicate work.

So: only add the call in `main.go`.

- [ ] **Step 3: Build to verify**

Run: `cd tui && go build -o /dev/null .`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add tui/main.go
git commit -m "feat(tui): wire LinkRecipeSkills and PruneStaleSkillSymlinks on startup"
```

---

### Task 5: Create `lingtai-recipe` bundled skill (English)

**Files:**
- Create: `tui/internal/preset/skills/lingtai-recipe/en/SKILL.md`

- [ ] **Step 1: Create the English SKILL.md**

Create `tui/internal/preset/skills/lingtai-recipe/en/SKILL.md`:

```markdown
---
name: lingtai-recipe
description: Guide for creating and understanding launch recipes — the mechanism that shapes how an orchestrator greets users, what behavioral constraints it follows, and what skills it ships. Use when the human asks about recipes, wants to create a custom recipe, or needs to understand how recipes work.
version: 1.0.0
---

# lingtai-recipe: Creating Launch Recipes

A **launch recipe** is a named directory that shapes an orchestrator's first-contact behavior, ongoing constraints, and available skills. Every lingtai project uses a recipe — selected during `/setup` or inherited from a published network via `/agora`.

## Recipe Directory Structure

```
my-recipe/
  en/
    greet.md              # First message to new users
    comment.md            # Persistent behavioral instructions
  zh/
    greet.md
    comment.md
  skills/                 # Optional: recipe-shipped skills
    my-skill/
      en/
        SKILL.md
        scripts/          # Optional helper scripts
        assets/           # Optional assets
      zh/
        SKILL.md
```

## The Three Components

### 1. `greet.md` — First Contact

The first message the orchestrator sends when a new user opens the TUI. Written from the orchestrator's perspective (first person).

**Purpose:** Set the tone, introduce the network, tell the user what they can do, offer guidance.

**Placeholders** (substituted at setup time):

| Placeholder | Value |
|---|---|
| `{{time}}` | Current date and time (2006-01-02 15:04) |
| `{{addr}}` | Human's email address in the network |
| `{{lang}}` | Language code (en, zh, wen) |
| `{{location}}` | Human's geographic location (City, Region, Country) |
| `{{soul_delay}}` | Soul cycle interval in seconds |

**Example:**

```markdown
Welcome to the OpenClaw Explainer Network! It's {{time}}.

I'm the lead orchestrator of a team of 10 agents. Type /cpr all
to wake everyone up, then tell me what you'd like to explore.
```

**Rules:**
- Keep it short (5-10 sentences max)
- Be proactive — introduce yourself, don't wait to be asked
- Always remind users to `/cpr all` to wake the full team (if the network has multiple agents)
- Use `{{time}}` and `{{location}}` to make the greeting feel alive

### 2. `comment.md` — Ongoing Behavioral Constraints

Injected into the orchestrator's system prompt on every turn. The persistent playbook.

**Purpose:** Define what topics to cover, how to delegate, constraints, tone. Think of it as a covenant extension specific to this recipe.

**Rules:**
- No placeholders — this is static text
- Keep it focused and concise — it's injected every turn, so every token counts
- Reference skills by name if the recipe ships skills (the agent can load them on demand)

### 3. `skills/` — Recipe-Shipped Skills

Optional. Skills that travel with the recipe and are automatically symlinked into `.lingtai/.skills/` when the TUI starts.

Each skill follows the standard SKILL.md contract:

```markdown
---
name: my-skill-name
description: One-line description of what this skill does
version: 1.0.0
---

# Skill content here...
```

**i18n:** Each skill can have language-specific versions. The TUI resolves:
1. `skills/<name>/<lang>/SKILL.md` — language-specific (preferred)
2. `skills/<name>/SKILL.md` — root fallback (language-agnostic)

**Symlink naming:** The TUI creates symlinks in `.lingtai/.skills/` named `<recipe>-<skill>-<lang>` (lang-specific) or `<recipe>-<skill>` (root fallback). This prevents collisions across recipes.

**Scripts and assets:** Place them alongside `SKILL.md` in the same language directory. They are self-contained per language.

## i18n Fallback Rules

All recipe files use the same resolution pattern:

1. Try `<recipe>/<lang>/<file>` — language-specific version
2. Try `<recipe>/<file>` — root fallback
3. Skip if neither exists

This applies to `greet.md`, `comment.md`, and skill directories.

## Recipe Types

| Type | Location | When Linked |
|---|---|---|
| Bundled | `~/.lingtai-tui/recipes/<name>/` | Always (shipped with TUI) |
| Custom | User-specified directory | When set via `/setup` |
| Agora | `<project>/.lingtai-recipe/` | When agora project exists |

All types follow the same directory structure and rules.

## How to Create a Custom Recipe

1. Create a directory with the structure above
2. Write at least a `greet.md` (comment.md and skills/ are optional)
3. In the TUI, run `/setup`, select "Custom" recipe, and enter the path to your directory
4. The orchestrator will restart and use your recipe

## How to Publish a Recipe

When you run `/agora publish`, the publishing flow includes a step to create a launch recipe at `.lingtai-recipe/` in the project root. This recipe travels with the published network and is automatically used by recipients who clone it.

## Testing

Point `/setup`'s custom recipe picker at your directory. The orchestrator restarts with your greet, comment, and skills immediately. Iterate until satisfied, then publish.
```

- [ ] **Step 2: Verify the file is well-formed**

Run: `head -5 tui/internal/preset/skills/lingtai-recipe/en/SKILL.md`
Expected: Shows the YAML frontmatter with name, description, version.

- [ ] **Step 3: Commit**

```bash
git add tui/internal/preset/skills/lingtai-recipe/en/SKILL.md
git commit -m "feat(skills): add lingtai-recipe self-documenting skill (English)"
```

---

### Task 6: Create `lingtai-recipe` bundled skill (Chinese)

**Files:**
- Create: `tui/internal/preset/skills/lingtai-recipe/zh/SKILL.md`

- [ ] **Step 1: Create the Chinese SKILL.md**

Create `tui/internal/preset/skills/lingtai-recipe/zh/SKILL.md`:

```markdown
---
name: lingtai-recipe
description: 创建和理解启动配方的指南——配方决定了调度器如何问候用户、遵循什么行为约束、以及附带哪些技能。当用户询问配方、想创建自定义配方、或需要理解配方工作原理时使用。
version: 1.0.0
---

# lingtai-recipe：创建启动配方

**启动配方**是一个命名目录，用于塑造调度器的首次接触行为、持续约束和可用技能。每个灵台项目都使用一个配方——在 `/setup` 中选择，或通过 `/agora` 从已发布网络继承。

## 配方目录结构

```
my-recipe/
  en/
    greet.md              # 给新用户的第一条消息
    comment.md            # 持久行为指令
  zh/
    greet.md
    comment.md
  skills/                 # 可选：配方附带技能
    my-skill/
      en/
        SKILL.md
        scripts/          # 可选辅助脚本
        assets/           # 可选资源
      zh/
        SKILL.md
```

## 三个组件

### 1. `greet.md` — 首次接触

调度器在新用户打开 TUI 时发送的第一条消息。以调度器的视角（第一人称）撰写。

**用途：** 设定基调，介绍网络，告诉用户能做什么，提供引导。

**占位符**（在设置时替换）：

| 占位符 | 值 |
|---|---|
| `{{time}}` | 当前日期和时间 |
| `{{addr}}` | 用户在网络中的邮箱地址 |
| `{{lang}}` | 语言代码（en、zh、wen） |
| `{{location}}` | 用户地理位置（城市、地区、国家） |
| `{{soul_delay}}` | 灵魂循环间隔（秒） |

**规则：**
- 保持简短（最多 5-10 句）
- 主动介绍自己，不要等用户提问
- 始终提醒用户使用 `/cpr all` 唤醒全部团队
- 使用 `{{time}}` 和 `{{location}}` 让问候更生动

### 2. `comment.md` — 持续行为约束

在每个回合注入调度器系统提示。持久的行为手册。

**用途：** 定义涵盖的主题、委派方式、约束、语气。

**规则：**
- 无占位符——这是静态文本
- 保持精简——每个回合都会注入，每个 token 都算数
- 如果配方附带技能，通过名称引用它们

### 3. `skills/` — 配方附带技能

可选。随配方一起分发的技能，TUI 启动时自动链接到 `.lingtai/.skills/`。

每个技能遵循标准 SKILL.md 格式：

```markdown
---
name: 技能名称
description: 一行描述
version: 1.0.0
---
```

**国际化：** 每个技能可有语言特定版本：
1. `skills/<name>/<lang>/SKILL.md` — 语言特定版本（优先）
2. `skills/<name>/SKILL.md` — 根目录回退

**链接命名：** TUI 在 `.lingtai/.skills/` 中创建名为 `<配方名>-<技能名>-<语言>` 或 `<配方名>-<技能名>` 的符号链接。

## 国际化回退规则

所有配方文件使用相同的解析模式：
1. 尝试 `<recipe>/<lang>/<file>` — 语言特定版本
2. 尝试 `<recipe>/<file>` — 根目录回退
3. 两者都不存在则跳过

## 如何创建自定义配方

1. 按上述结构创建目录
2. 至少编写一个 `greet.md`（comment.md 和 skills/ 可选）
3. 在 TUI 中运行 `/setup`，选择「自定义」配方，输入目录路径
4. 调度器会重启并使用你的配方

## 如何发布配方

运行 `/agora publish` 时，发布流程包含在项目根目录创建 `.lingtai-recipe/` 启动配方的步骤。该配方随网络一起发布，克隆者会自动使用。
```

- [ ] **Step 2: Commit**

```bash
git add tui/internal/preset/skills/lingtai-recipe/zh/SKILL.md
git commit -m "feat(skills): add lingtai-recipe self-documenting skill (Chinese)"
```

---

### Task 7: Integration test — full build and verify

**Files:**
- No new files.

- [ ] **Step 1: Run all preset tests**

Run: `cd tui && go test ./internal/preset/ -v`
Expected: All tests pass, including new `ResolveSkillDir`, `LinkRecipeSkills`, and `PruneStaleSkillSymlinks` tests.

- [ ] **Step 2: Run all tui tests**

Run: `cd tui && go test ./internal/tui/ -v -count=1`
Expected: All tests pass, including new `scanSkills` symlink tests.

- [ ] **Step 3: Build the binary**

Run: `cd tui && make build`
Expected: Build succeeds, binary at `tui/bin/lingtai-tui`.

- [ ] **Step 4: Verify the new skill is embedded**

Run: `cd tui && go run . --help 2>&1 | head -1`
Expected: Shows version/help output (binary runs without crash).

Verify the skill file is in the embed:
Run: `ls tui/internal/preset/skills/lingtai-recipe/en/SKILL.md`
Expected: File exists.

- [ ] **Step 5: Final commit (if any fixups needed)**

Only if previous steps required changes. Otherwise skip.
