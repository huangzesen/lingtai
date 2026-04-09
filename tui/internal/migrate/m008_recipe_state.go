package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// migrateRecipeState creates .lingtai/.tui-asset/.recipe for legacy projects
// that predate the launch recipe system. No-op if the file already exists.
//
// Recipe inference:
//  1. If any agent's init.json.comment_file contains "tutorial" → tutorial
//  2. Else global tui_config.json.greeting == false → plain
//  3. Otherwise → greeter
//
// init.json is NOT modified. Only .recipe is written.
func migrateRecipeState(lingtaiDir string) error {
	recipePath := filepath.Join(lingtaiDir, ".tui-asset", ".recipe")
	if _, err := os.Stat(recipePath); err == nil {
		return nil // already migrated
	}

	recipe := inferLegacyRecipe(lingtaiDir)

	tuiAsset := filepath.Join(lingtaiDir, ".tui-asset")
	if err := os.MkdirAll(tuiAsset, 0o755); err != nil {
		return fmt.Errorf("create .tui-asset: %w", err)
	}
	payload := fmt.Sprintf("{\n  \"recipe\": %q\n}", recipe)
	return os.WriteFile(recipePath, []byte(payload), 0o644)
}

// inferLegacyRecipe returns the best-guess recipe name for a legacy project.
func inferLegacyRecipe(lingtaiDir string) string {
	if hasLegacyTutorialAgent(lingtaiDir) {
		return "tutorial"
	}
	if greeting := readLegacyGreeting(); greeting != nil && !*greeting {
		return "plain"
	}
	return "greeter"
}

// hasLegacyTutorialAgent returns true if any agent directory under lingtaiDir
// has an init.json whose comment_file contains "tutorial".
func hasLegacyTutorialAgent(lingtaiDir string) bool {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		initPath := filepath.Join(lingtaiDir, entry.Name(), "init.json")
		data, err := os.ReadFile(initPath)
		if err != nil {
			continue
		}
		var init map[string]interface{}
		if err := json.Unmarshal(data, &init); err != nil {
			continue
		}
		if commentFile, ok := init["comment_file"].(string); ok {
			if strings.Contains(commentFile, "tutorial") {
				return true
			}
		}
	}
	return false
}

// readLegacyGreeting reads the global tui_config.json.greeting field.
// Returns nil if the file is missing, unreadable, or the field is absent.
// Uses raw JSON to avoid importing the config package.
func readLegacyGreeting() *bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	path := filepath.Join(home, ".lingtai-tui", "tui_config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	raw, ok := obj["greeting"]
	if !ok {
		return nil
	}
	b, ok := raw.(bool)
	if !ok {
		return nil
	}
	return &b
}
