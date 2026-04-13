package migrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anthropics/lingtai-tui/internal/preset"
)

// migrateSkillsGroups restructures .skills/ from flat layout to grouped layout.
//
// Old layout: .skills/<skill-name>/ (all flat — bundled, recipe, custom mixed)
// New layout: .skills/intrinsic/ (symlink), .skills/<recipe>/ (group dirs),
//             .skills/custom/ (agent-created)
//
// This migration:
// 1. Removes flat bundled skill dirs (they'll be re-created as .skills/intrinsic/ on next launch)
// 2. Moves flat non-symlink, non-bundled skill dirs into .skills/custom/
// 3. Removes flat legacy recipe symlinks (they'll be re-created as grouped on next launch)
func migrateSkillsGroups(lingtaiDir string) error {
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	if _, err := os.Stat(skillsDir); err != nil {
		return nil // no .skills/ — nothing to do
	}

	bundled := preset.BundledSkillNames()

	// Skills removed or renamed in past TUI versions — delete, don't migrate.
	stale := map[string]bool{
		"lingtai-agora": true, // renamed to lingtai-export-network in v0.4.40
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	var toCustom []string   // non-symlink, non-bundled dirs → move to custom/
	var toRemove []string   // flat bundled dirs or legacy flat symlinks → remove

	for _, e := range entries {
		name := e.Name()
		if name == "intrinsic" || name == "custom" || name[0] == '.' {
			continue
		}

		p := filepath.Join(skillsDir, name)
		info, err := os.Lstat(p)
		if err != nil {
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// Legacy flat symlink (e.g., "adaptive-discovery-en", "tutorial-guide-en")
			// Will be re-created as grouped symlinks on next launch.
			toRemove = append(toRemove, p)
			continue
		}

		if !info.IsDir() {
			continue
		}

		if bundled[name] || stale[name] {
			// Flat bundled or stale skill dir — remove
			toRemove = append(toRemove, p)
		} else {
			// Custom/agent-created skill — move to .skills/custom/
			toCustom = append(toCustom, name)
		}
	}

	// Move custom skills
	if len(toCustom) > 0 {
		customDir := filepath.Join(skillsDir, "custom")
		os.MkdirAll(customDir, 0o755)
		for _, name := range toCustom {
			src := filepath.Join(skillsDir, name)
			dst := filepath.Join(customDir, name)
			if _, err := os.Stat(dst); err == nil {
				// Already exists in custom/ — skip
				continue
			}
			if err := os.Rename(src, dst); err != nil {
				fmt.Printf("  warning: could not move skill %q to custom/: %v\n", name, err)
			} else {
				fmt.Printf("  migrated skill %q → custom/%s\n", name, name)
			}
		}
	}

	// Remove legacy entries
	for _, p := range toRemove {
		os.RemoveAll(p)
	}

	return nil
}
