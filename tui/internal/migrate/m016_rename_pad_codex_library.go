package migrate

import (
	"fmt"
	"os"
	"path/filepath"
)

// migrateRenamePadCodexLibrary renames agent filesystem paths:
//
//   - system/memory.md → system/pad.md
//   - library/ → codex/ (including library.json → codex.json)
//   - .skills/ → .library/
//
// Runs on each agent directory found under lingtaiDir.
// Also renames .lingtai/.skills/ → .lingtai/.library/ at the network level.
func migrateRenamePadCodexLibrary(lingtaiDir string) error {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' || e.Name() == "human" {
			continue
		}
		agentDir := filepath.Join(lingtaiDir, e.Name())

		// 1. system/memory.md → system/pad.md
		safeRename(
			filepath.Join(agentDir, "system", "memory.md"),
			filepath.Join(agentDir, "system", "pad.md"),
			"system/memory.md → system/pad.md",
		)

		// 2. system/memory_append.json → system/pad_append.json
		safeRename(
			filepath.Join(agentDir, "system", "memory_append.json"),
			filepath.Join(agentDir, "system", "pad_append.json"),
			"system/memory_append.json → system/pad_append.json",
		)

		// 3. library/ → codex/
		oldLibDir := filepath.Join(agentDir, "library")
		newCodexDir := filepath.Join(agentDir, "codex")
		if safeRename(oldLibDir, newCodexDir, "library/ → codex/") {
			// Rename library.json → codex.json inside
			safeRename(
				filepath.Join(newCodexDir, "library.json"),
				filepath.Join(newCodexDir, "codex.json"),
				"library.json → codex.json",
			)
		}

		// 4. .skills/ → .library/ (agent-level, if it exists)
		safeRename(
			filepath.Join(agentDir, ".skills"),
			filepath.Join(agentDir, ".library"),
			".skills → .library (agent-level)",
		)
	}

	// 5. .lingtai/.skills/ → .lingtai/.library/ (network-level)
	safeRename(
		filepath.Join(lingtaiDir, ".skills"),
		filepath.Join(lingtaiDir, ".library"),
		".skills → .library (network-level)",
	)

	return nil
}

// safeRename renames src → dst if src exists and dst does not.
// Returns true if the rename happened.
func safeRename(src, dst, label string) bool {
	if _, err := os.Stat(src); err != nil {
		return false // source doesn't exist
	}
	if _, err := os.Lstat(dst); err == nil {
		fmt.Printf("  warning: %s skipped — destination already exists\n", label)
		return false
	}
	if err := os.Rename(src, dst); err != nil {
		fmt.Printf("  warning: %s failed: %v\n", label, err)
		return false
	}
	fmt.Printf("  migrated %s\n", label)
	return true
}
