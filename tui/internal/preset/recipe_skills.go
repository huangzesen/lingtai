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

	// 3. Agora networks (try networks/ first, fall back to legacy projects/)
	home, err := os.UserHomeDir()
	if err == nil {
		agoraRoot := filepath.Join(home, "lingtai-agora", "networks")
		entries, readErr := os.ReadDir(agoraRoot)
		if readErr != nil {
			// Fallback: try legacy projects/ path
			agoraRoot = filepath.Join(home, "lingtai-agora", "projects")
			entries, readErr = os.ReadDir(agoraRoot)
		}
		if readErr == nil {
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

	// 4. Agora standalone recipes (~/lingtai-agora/recipes/)
	if home, err := os.UserHomeDir(); err == nil {
		agoraRecipesDir := filepath.Join(home, "lingtai-agora", "recipes")
		if entries, err := os.ReadDir(agoraRecipesDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() || e.Name() == "" || e.Name()[0] == '.' {
					continue
				}
				recipeDir := filepath.Join(agoraRecipesDir, e.Name())
				linkRecipeDir(skillsDir, recipeDir, e.Name(), lang, claimed)
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
		if e.Name() == "" || e.Name()[0] == '.' {
			continue
		}
		info, err := os.Stat(filepath.Join(skillsRoot, e.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
		skillName := e.Name()
		resolved := ResolveSkillDir(recipeDir, skillName, lang)
		if resolved == "" {
			continue
		}

		// Compute symlink name. The resolved dir might be a fallback lang
		// (e.g., zh/ when user asked for wen), so derive the suffix from the
		// actual resolved path, not the requested lang.
		base := filepath.Join(recipeDir, "skills", skillName)
		var linkName string
		if resolved == base {
			// Root fallback — no lang suffix
			linkName = fmt.Sprintf("%s-%s", recipeName, skillName)
		} else {
			// Lang-specific — suffix is the directory name under the skill
			resolvedLang := filepath.Base(resolved)
			linkName = fmt.Sprintf("%s-%s-%s", recipeName, skillName, resolvedLang)
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

		if err := os.Symlink(resolved, linkPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create recipe skill symlink %q: %v\n", linkName, err)
		}
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
		if e.Type()&os.ModeSymlink == 0 {
			continue // not a symlink — leave it alone
		}
		path := filepath.Join(skillsDir, e.Name())
		// Check if target exists (os.Stat follows symlinks)
		if _, err := os.Stat(path); err != nil {
			// Broken symlink — remove
			os.Remove(path)
		}
	}
}
