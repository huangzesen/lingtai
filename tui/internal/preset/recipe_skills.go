package preset

import (
	"fmt"
	"os"
	"path/filepath"
)

// LinkRecipeSkills creates grouped symlinks in <lingtaiDir>/.skills/ for
// skills from the active recipe, custom recipe, and agora recipes.
// Called on every TUI startup after PopulateBundledSkills().
//
// Each recipe's skills are placed under .skills/<recipe-name>/ as a group
// folder. Individual skills within that group are symlinked to the
// lang-resolved skill directory in the recipe.
//
// Only the active bundled recipe's skills are linked — inactive bundled
// recipes do not contribute skills. Custom and agora recipes are always linked.
func LinkRecipeSkills(lingtaiDir, globalDir, lang, activeRecipe, customDir string) {
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	os.MkdirAll(skillsDir, 0o755)

	// Track all recipe group dirs we create/update so we can detect stale ones.
	activeGroups := make(map[string]bool)
	activeGroups["intrinsic"] = true // managed by PopulateBundledSkills
	activeGroups["custom"] = true    // agent-created, never touched

	// 1. Active bundled/example recipe only
	if activeRecipe != "" {
		recipeDir := RecipeDir(globalDir, activeRecipe)
		if recipeDir != "" {
			if info, err := os.Stat(recipeDir); err == nil && info.IsDir() {
				linkRecipeDir(skillsDir, recipeDir, activeRecipe, lang)
				activeGroups[activeRecipe] = true
			}
		}
	}

	// Clean up group dirs from previously active recipes that are no longer
	// active. Scan all category subdirs under recipes/.
	recipesRoot := filepath.Join(globalDir, "recipes")
	if cats, err := os.ReadDir(recipesRoot); err == nil {
		for _, cat := range cats {
			if !cat.IsDir() || cat.Name()[0] == '.' {
				continue
			}
			catDir := filepath.Join(recipesRoot, cat.Name())
			if entries, err := os.ReadDir(catDir); err == nil {
				for _, e := range entries {
					if !e.IsDir() || e.Name() == activeRecipe {
						continue
					}
					staleGroup := filepath.Join(skillsDir, e.Name())
					if _, err := os.Stat(staleGroup); err == nil {
						os.RemoveAll(staleGroup)
					}
				}
			}
		}
	}

	// 2. Custom recipe (if set)
	if customDir != "" {
		recipeName := filepath.Base(customDir)
		linkRecipeDir(skillsDir, customDir, recipeName, lang)
		activeGroups[recipeName] = true
	}

	// 3. Agora networks (try networks/ first, fall back to legacy projects/)
	home, err := os.UserHomeDir()
	if err == nil {
		agoraRoot := filepath.Join(home, "lingtai-agora", "networks")
		entries, readErr := os.ReadDir(agoraRoot)
		if readErr != nil {
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
					linkRecipeDir(skillsDir, recipeDir, e.Name(), lang)
					activeGroups[e.Name()] = true
				}
			}
		}
	}

	// 4. Clean up stale group dirs that are no longer active.
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if activeGroups[name] || isHidden(name) {
				continue
			}
			p := filepath.Join(skillsDir, name)
			info, err := os.Lstat(p)
			if err != nil {
				continue
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				os.RemoveAll(p)
			}
		}
	}
}

// linkRecipeDir creates .skills/<recipeName>/ as a real directory and
// symlinks each resolved skill into it.
func linkRecipeDir(skillsDir, recipeDir, recipeName, lang string) {
	skillsRoot := filepath.Join(recipeDir, "skills")
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return // no skills/ directory — normal for most recipes
	}

	groupDir := filepath.Join(skillsDir, recipeName)
	os.MkdirAll(groupDir, 0o755)

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

		linkPath := filepath.Join(groupDir, skillName)

		// Check if symlink already exists and points to the correct target
		if existing, err := os.Readlink(linkPath); err == nil {
			if existing == resolved {
				continue // already correct
			}
			os.Remove(linkPath) // wrong target — recreate
		} else {
			// Not a symlink — remove whatever is there
			os.RemoveAll(linkPath)
		}

		if err := os.Symlink(resolved, linkPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to create recipe skill symlink %q: %v\n", linkPath, err)
		}
	}
}

// PruneStaleSkillSymlinks scans <lingtaiDir>/.skills/ and removes any
// symlinks whose target no longer exists. Works recursively — handles
// both flat legacy symlinks and grouped symlinks inside recipe folders.
func PruneStaleSkillSymlinks(lingtaiDir string) {
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	pruneDir(skillsDir)
}

func pruneDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.Type()&os.ModeSymlink != 0 {
			// Symlink — check if target exists
			if _, err := os.Stat(path); err != nil {
				os.Remove(path) // broken symlink
			}
		} else if e.IsDir() && e.Name() != "." && !isHidden(e.Name()) {
			// Real directory — recurse (for group folders)
			pruneDir(path)
		}
	}
}

func isHidden(name string) bool {
	return len(name) > 0 && name[0] == '.'
}
