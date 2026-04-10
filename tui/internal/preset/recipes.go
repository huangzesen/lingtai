package preset

import (
	"encoding/json"
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
	RecipeImported = "imported"
)

// RecipeInfo holds the metadata from a recipe's recipe.json manifest.
type RecipeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// LoadRecipeInfo reads recipe.json from a recipe directory, resolved via the
// standard i18n fallback (<lang>/recipe.json → recipe.json). Returns an error
// if the file is not found, unparseable, or has an empty name.
func LoadRecipeInfo(recipeDir, lang string) (RecipeInfo, error) {
	if recipeDir == "" {
		return RecipeInfo{}, fmt.Errorf("empty recipe directory")
	}
	path := resolveRecipeFile(recipeDir, lang, "recipe.json")
	if path == "" {
		return RecipeInfo{}, fmt.Errorf("recipe.json not found in %s", recipeDir)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RecipeInfo{}, fmt.Errorf("read recipe.json: %w", err)
	}
	var info RecipeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return RecipeInfo{}, fmt.Errorf("parse recipe.json: %w", err)
	}
	if info.Name == "" {
		return RecipeInfo{}, fmt.Errorf("recipe.json has empty name in %s", recipeDir)
	}
	return info, nil
}

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

// langFallbackChain returns the ordered list of languages to try for a given
// lang. The rule is simple: try <lang> first, then root. Root is mandatory
// and serves as the universal fallback for all languages.
func langFallbackChain(lang string) []string {
	if lang == "" {
		return []string{""}
	}
	return []string{lang, ""}
}

func resolveRecipeFile(recipeDir, lang, filename string) string {
	if recipeDir == "" {
		return ""
	}
	for _, l := range langFallbackChain(lang) {
		var path string
		if l == "" {
			path = filepath.Join(recipeDir, filename)
		} else {
			path = filepath.Join(recipeDir, l, filename)
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// ResolveSkillDir returns the absolute path to a skill directory within a
// recipe, applying the lang fallback chain (wen → zh → en → root):
//
//	<recipeDir>/skills/<skillName>/<lang>/SKILL.md → that dir
//	... fallback langs ...
//	<recipeDir>/skills/<skillName>/SKILL.md → that dir
//	empty string (no match)
func ResolveSkillDir(recipeDir, skillName, lang string) string {
	if recipeDir == "" {
		return ""
	}
	base := filepath.Join(recipeDir, "skills", skillName)
	for _, l := range langFallbackChain(lang) {
		var dir string
		if l == "" {
			dir = base
		} else {
			dir = filepath.Join(base, l)
		}
		if info, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil && !info.IsDir() {
			return dir
		}
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
