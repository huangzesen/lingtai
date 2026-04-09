package preset

import (
	"fmt"
	"os"
	"path/filepath"
)

// Recipe names. A recipe is a named pair of {greet.md, comment.md} applied
// to the orchestrator at setup time to shape its initial and ongoing
// disposition. Not to be confused with preset (LLM/capabilities template).
const (
	RecipeGreeter  = "greeter"
	RecipePlain    = "plain"
	RecipeAdaptive = "adaptive"
	RecipeTutorial = "tutorial"
	RecipeCustom   = "custom"
)

// BundledRecipes returns the four built-in recipe names in picker display order.
// Adaptive is first (recommended default). Custom is not included — it is
// handled separately in the picker because it requires a user-supplied directory.
func BundledRecipes() []string {
	return []string{RecipeAdaptive, RecipeGreeter, RecipePlain, RecipeTutorial}
}

// RecipeDir returns the absolute directory for a bundled recipe under the
// global config dir: <globalDir>/recipes/<name>/.
func RecipeDir(globalDir, name string) string {
	return filepath.Join(globalDir, "recipes", name)
}

// ResolveGreetPath returns the absolute path to the greet file for a recipe
// directory and language, applying the per-lang fallback rule:
//  1. <recipeDir>/<lang>/greet.md
//  2. <recipeDir>/greet.md
//  3. empty string (no greet)
//
// recipeDir can be either a bundled recipe directory (from RecipeDir) or a
// user-supplied custom directory. The rule is identical for both.
func ResolveGreetPath(recipeDir, lang string) string {
	return resolveRecipeFile(recipeDir, lang, "greet.md")
}

// ResolveCommentPath returns the absolute path to the comment file for a recipe
// directory and language, using the same fallback rule as ResolveGreetPath.
func ResolveCommentPath(recipeDir, lang string) string {
	return resolveRecipeFile(recipeDir, lang, "comment.md")
}

func resolveRecipeFile(recipeDir, lang, filename string) string {
	if recipeDir == "" {
		return ""
	}
	// 1. Try lang-specific: <dir>/<lang>/<filename>
	if lang != "" {
		langPath := filepath.Join(recipeDir, lang, filename)
		if info, err := os.Stat(langPath); err == nil && !info.IsDir() {
			return langPath
		}
	}
	// 2. Try root: <dir>/<filename>
	rootPath := filepath.Join(recipeDir, filename)
	if info, err := os.Stat(rootPath); err == nil && !info.IsDir() {
		return rootPath
	}
	// 3. No match
	return ""
}

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

// ValidateCustomDir checks that a user-supplied custom recipe folder exists and
// is a directory. Returns a human-readable error on failure.
//
// Empty folders are accepted — they behave like the plain recipe. The only
// precondition is that the path refers to an existing directory.
func ValidateCustomDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("custom recipe folder path is empty")
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("custom recipe folder does not exist: %q", dir)
		}
		return fmt.Errorf("cannot access custom recipe folder: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("custom recipe path is not a directory: %q", dir)
	}
	return nil
}

// ProjectLocalRecipeDir returns <projectRoot>/.lingtai-recipe/ if it exists and
// is a directory, otherwise empty string. Used to pre-fill the custom path
// input in /setup and forward-compatible with agora network export.
func ProjectLocalRecipeDir(projectRoot string) string {
	p := filepath.Join(projectRoot, ".lingtai-recipe")
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return ""
	}
	return p
}
