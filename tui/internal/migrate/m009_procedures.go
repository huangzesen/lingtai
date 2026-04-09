package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// migrateProcedures seeds system/procedures.md for each existing agent that
// lacks one. Content is copied from the global procedures asset matching the
// agent's configured language.
func migrateProcedures(lingtaiDir string) error {
	globalDir := globalTUIDir()
	if globalDir == "" {
		return nil // can't resolve global dir — skip silently
	}

	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		agentDir := filepath.Join(lingtaiDir, entry.Name())

		// Skip if system/procedures.md already exists
		proceduresFile := filepath.Join(agentDir, "system", "procedures.md")
		if _, err := os.Stat(proceduresFile); err == nil {
			continue
		}

		// Read agent language from init.json
		lang := agentLanguage(agentDir)

		// Read procedures content from global asset
		src := filepath.Join(globalDir, "procedures", lang, "procedures.md")
		content, err := os.ReadFile(src)
		if err != nil {
			// Try English fallback
			src = filepath.Join(globalDir, "procedures", "en", "procedures.md")
			content, err = os.ReadFile(src)
			if err != nil {
				continue // no procedures asset available
			}
		}

		// Ensure system/ directory exists
		systemDir := filepath.Join(agentDir, "system")
		os.MkdirAll(systemDir, 0o755)

		os.WriteFile(proceduresFile, content, 0o644)
	}
	return nil
}

// agentLanguage reads the language field from an agent's init.json manifest.
// Returns "en" if unreadable or missing.
func agentLanguage(agentDir string) string {
	data, err := os.ReadFile(filepath.Join(agentDir, "init.json"))
	if err != nil {
		return "en"
	}
	var raw map[string]interface{}
	if json.Unmarshal(data, &raw) != nil {
		return "en"
	}
	manifest, _ := raw["manifest"].(map[string]interface{})
	if manifest == nil {
		return "en"
	}
	if lang, ok := manifest["language"].(string); ok && lang != "" {
		return lang
	}
	return "en"
}

// globalTUIDir returns ~/.lingtai-tui or "" if home dir is unresolvable.
func globalTUIDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".lingtai-tui")
}
