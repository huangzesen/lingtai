package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// migrateRenamePadCodexLibrary renames agent filesystem paths and
// rewrites init.json capability keys:
//
// Filesystem:
//   - system/memory.md → system/pad.md
//   - system/memory_append.json → system/pad_append.json
//   - library/ → codex/ (including library.json → codex.json)
//   - .skills/ → .library/
//
// init.json:
//   - top-level "memory" field → "pad"
//   - top-level "memory_file" field → "pad_file"
//   - capabilities key "library" → "codex" (knowledge archive)
//   - capabilities key "skills" → "library" (skill library)
//   - capability config "library_limit" → "codex_limit"
//
// Runs on each agent directory found under lingtaiDir.
// Also renames .lingtai/.skills/ → .lingtai/.library/ at the network level.
func migrateRenamePadCodexLibrary(lingtaiDir string) error {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return nil
	}

	didWork := false

	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' || e.Name() == "human" {
			continue
		}
		agentDir := filepath.Join(lingtaiDir, e.Name())

		// 1. system/memory.md → system/pad.md
		if safeRename(
			filepath.Join(agentDir, "system", "memory.md"),
			filepath.Join(agentDir, "system", "pad.md"),
			"system/memory.md → system/pad.md",
		) {
			didWork = true
		}

		// 2. system/memory_append.json → system/pad_append.json
		if safeRename(
			filepath.Join(agentDir, "system", "memory_append.json"),
			filepath.Join(agentDir, "system", "pad_append.json"),
			"system/memory_append.json → system/pad_append.json",
		) {
			didWork = true
		}

		// 3. library/ → codex/
		oldLibDir := filepath.Join(agentDir, "library")
		newCodexDir := filepath.Join(agentDir, "codex")
		if safeRename(oldLibDir, newCodexDir, "library/ → codex/") {
			didWork = true
			// Rename library.json → codex.json inside
			safeRename(
				filepath.Join(newCodexDir, "library.json"),
				filepath.Join(newCodexDir, "codex.json"),
				"library.json → codex.json",
			)
		}

		// 4. .skills/ → .library/ (agent-level, if it exists)
		if safeRename(
			filepath.Join(agentDir, ".skills"),
			filepath.Join(agentDir, ".library"),
			".skills → .library (agent-level)",
		) {
			didWork = true
		}

		// 5. Rewrite init.json capability keys and config fields
		if rewriteInitJSON(filepath.Join(agentDir, "init.json")) {
			didWork = true
		}
	}

	// 6. .lingtai/.skills/ → .lingtai/.library/ (network-level)
	if safeRename(
		filepath.Join(lingtaiDir, ".skills"),
		filepath.Join(lingtaiDir, ".library"),
		".skills → .library (network-level)",
	) {
		didWork = true
	}

	// Only show the banner + blocking prompt if we actually migrated
	// something. Fresh projects (meta.Version == 0 → replays all
	// migrations including this one) had nothing to rename and shouldn't
	// see the scary "your agents need a refresh" warning.
	if !didWork {
		return nil
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("  ║  Migration complete: memory→pad, library→codex, skills→library  ║")
	fmt.Println("  ║                                                              ║")
	fmt.Println("  ║  Your agents' files and init.json have been updated.         ║")
	fmt.Println("  ║  Running agents still have old tool names in their session.  ║")
	fmt.Println("  ║                                                              ║")
	fmt.Println("  ║  After the TUI starts, type:  /refresh all                   ║")
	fmt.Println("  ║  This restarts all agents with the new tool names.           ║")
	fmt.Println("  ╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Print("  Press [Enter] to continue...")
	fmt.Scanln()

	return nil
}

// rewriteInitJSON rewrites capability names and config fields in an
// agent's init.json. Handles both list and map capability formats.
// Returns true if any changes were written to disk.
func rewriteInitJSON(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false // no init.json — nothing to do
	}

	var init map[string]interface{}
	if json.Unmarshal(data, &init) != nil {
		return false // corrupt — skip
	}

	changed := false

	// Rename top-level "memory" → "pad"
	if v, ok := init["memory"]; ok {
		init["pad"] = v
		delete(init, "memory")
		changed = true
	}

	// Rename top-level "memory_file" → "pad_file"
	if v, ok := init["memory_file"]; ok {
		init["pad_file"] = v
		delete(init, "memory_file")
		changed = true
	}

	// Rename capability keys inside manifest.capabilities
	manifest, _ := init["manifest"].(map[string]interface{})
	if manifest != nil {
		caps, _ := manifest["capabilities"].(map[string]interface{})
		if caps != nil {
			// "library" (old knowledge archive) → "codex"
			if v, ok := caps["library"]; ok {
				caps["codex"] = v
				delete(caps, "library")
				changed = true

				// Rename library_limit → codex_limit inside the config
				if cfg, ok := caps["codex"].(map[string]interface{}); ok {
					if lim, ok := cfg["library_limit"]; ok {
						cfg["codex_limit"] = lim
						delete(cfg, "library_limit")
					}
				}
			}

			// "skills" (old skill library) → "library"
			if v, ok := caps["skills"]; ok {
				caps["library"] = v
				delete(caps, "skills")
				changed = true
			}
		}
	}

	if !changed {
		return false
	}

	out, err := json.MarshalIndent(init, "", "  ")
	if err != nil {
		return false
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fmt.Printf("  warning: failed to rewrite %s: %v\n", path, err)
		return false
	}
	fmt.Printf("  migrated init.json capability keys\n")
	return true
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
