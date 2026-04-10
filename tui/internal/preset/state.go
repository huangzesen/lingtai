package preset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RecipeState is the content of .lingtai/.tui-asset/.recipe — a TUI-only
// project-level file tracking the currently selected launch recipe.
// The Python kernel ignores this file.
type RecipeState struct {
	Recipe    string `json:"recipe"`               // one of the six recipe names (adaptive, greeter, plain, tutorial, custom, imported)
	CustomDir string `json:"custom_dir,omitempty"` // set when Recipe == RecipeCustom or RecipeImported
}

// recipeStatePath returns the absolute path to .lingtai/.tui-asset/.recipe.
func recipeStatePath(lingtaiDir string) string {
	return filepath.Join(lingtaiDir, ".tui-asset", ".recipe")
}

// LoadRecipeState reads .lingtai/.tui-asset/.recipe. Returns a zero-value
// RecipeState and nil error if the file does not exist — this is the expected
// state for legacy projects and freshly-cleaned (post-nirvana) projects.
// Returns an error only on actual I/O or JSON parse failure.
func LoadRecipeState(lingtaiDir string) (RecipeState, error) {
	var state RecipeState
	data, err := os.ReadFile(recipeStatePath(lingtaiDir))
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("read recipe state: %w", err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("parse recipe state: %w", err)
	}
	return state, nil
}

// SaveRecipeState atomically writes .lingtai/.tui-asset/.recipe. Creates the
// .tui-asset/ directory if it does not already exist.
func SaveRecipeState(lingtaiDir string, state RecipeState) error {
	dir := filepath.Join(lingtaiDir, ".tui-asset")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .tui-asset dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal recipe state: %w", err)
	}
	// Atomic write: temp file + rename
	tmpPath := recipeStatePath(lingtaiDir) + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		os.Remove(tmpPath) // best-effort cleanup of partial write
		return fmt.Errorf("write recipe state tmp: %w", err)
	}
	if err := os.Rename(tmpPath, recipeStatePath(lingtaiDir)); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename recipe state: %w", err)
	}
	return nil
}
