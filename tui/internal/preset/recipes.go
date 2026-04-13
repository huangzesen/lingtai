package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Recipe names. A recipe is a named pair of {greet.md, comment.md} applied
// to the orchestrator at setup time to shape its initial and ongoing
// disposition. Not to be confused with preset (LLM/capabilities template).
const (
	RecipeCustom   = "custom"
	RecipeImported = "imported"
	RecipeAgora    = "agora" // from ~/lingtai-agora/recipes/
)

// AgoraRecipe holds a discovered recipe from ~/lingtai-agora/recipes/.
type AgoraRecipe struct {
	Info RecipeInfo // from recipe.json
	Dir  string     // absolute path to the recipe directory
}

// ScanAgoraRecipes returns all valid recipes found under ~/lingtai-agora/recipes/.
// Each subdirectory must contain a valid recipe.json with a non-empty name.
// Returns nil if directory doesn't exist or is empty.
func ScanAgoraRecipes(lang string) []AgoraRecipe {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	recipesDir := filepath.Join(home, "lingtai-agora", "recipes")
	entries, err := os.ReadDir(recipesDir)
	if err != nil {
		return nil
	}
	var recipes []AgoraRecipe
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "" || e.Name()[0] == '.' {
			continue
		}
		dir := filepath.Join(recipesDir, e.Name())
		info, err := LoadRecipeInfo(dir, lang)
		if err != nil {
			continue // skip dirs without valid recipe.json
		}
		recipes = append(recipes, AgoraRecipe{Info: info, Dir: dir})
	}
	return recipes
}

// DiscoveredRecipe holds a recipe found by scanning a category directory.
type DiscoveredRecipe struct {
	ID   string     // directory name (e.g. "greeter", "tutorial")
	Info RecipeInfo // from recipe.json
	Dir  string     // absolute path to the recipe directory
}

// RecipeCategories defines the display order of built-in recipe categories.
var RecipeCategories = []string{"recommended", "intrinsic", "examples"}

// ScanCategory returns all valid recipes found under <globalDir>/recipes/<category>/.
// Each subdirectory must contain a valid recipe.json with a non-empty name.
// Results are sorted alphabetically by ID (directory name).
func ScanCategory(globalDir, category, lang string) []DiscoveredRecipe {
	catDir := filepath.Join(globalDir, "recipes", category)
	entries, err := os.ReadDir(catDir)
	if err != nil {
		return nil
	}
	var recipes []DiscoveredRecipe
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "" || e.Name()[0] == '.' {
			continue
		}
		dir := filepath.Join(catDir, e.Name())
		info, err := LoadRecipeInfo(dir, lang)
		if err != nil {
			continue
		}
		recipes = append(recipes, DiscoveredRecipe{ID: e.Name(), Info: info, Dir: dir})
	}
	sort.Slice(recipes, func(i, j int) bool {
		return recipes[i].ID < recipes[j].ID
	})
	return recipes
}

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

// RecipeDir returns the absolute directory for a discovered recipe by searching
// all category subdirectories under <globalDir>/recipes/. Returns empty string
// if the recipe is not found.
func RecipeDir(globalDir, name string) string {
	recipesRoot := filepath.Join(globalDir, "recipes")
	entries, err := os.ReadDir(recipesRoot)
	if err != nil {
		return ""
	}
	for _, cat := range entries {
		if !cat.IsDir() || cat.Name()[0] == '.' {
			continue
		}
		candidate := filepath.Join(recipesRoot, cat.Name(), name)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
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

// ResolveCovenantPath returns the absolute path to the covenant file for a
// recipe directory and language, using the same fallback rule as ResolveGreetPath.
// Returns empty string if the recipe does not provide a covenant override.
func ResolveCovenantPath(recipeDir, lang string) string {
	return resolveRecipeFile(recipeDir, lang, "covenant.md")
}

// ResolveProceduresPath returns the absolute path to the procedures file for a
// recipe directory and language, using the same fallback rule as ResolveGreetPath.
// Returns empty string if the recipe does not provide a procedures override.
func ResolveProceduresPath(recipeDir, lang string) string {
	return resolveRecipeFile(recipeDir, lang, "procedures.md")
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
