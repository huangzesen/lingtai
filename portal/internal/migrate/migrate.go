package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CurrentVersion is the latest migration version compiled into this binary.
const CurrentVersion = 3

type metaFile struct {
	Version int `json:"version"`
}

// Migration represents a single versioned migration step.
type Migration struct {
	Version int
	Name    string
	Fn      func(lingtaiDir string) error
}

// migrations is the ordered list of all migrations. Append-only.
var migrations = []Migration{
	{Version: 1, Name: "topology-to-portal", Fn: migrateTopologyToPortal},
	{Version: 2, Name: "tape-normalize", Fn: migrateTapeNormalize},
	{Version: 3, Name: "character-to-lingtai", Fn: migrateCharacterToLingtai},
}

// Run executes all pending migrations on the given .lingtai/ directory.
// It reads the current version from meta.json (or assumes 0 if missing),
// runs migrations sequentially, and writes the new version atomically.
func Run(lingtaiDir string) error {
	metaPath := filepath.Join(lingtaiDir, "meta.json")

	current := 0
	if data, err := os.ReadFile(metaPath); err == nil {
		var m metaFile
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("parse meta.json: %w", err)
		}
		current = m.Version
	}

	if current > CurrentVersion {
		return fmt.Errorf(
			"data version %d is newer than this binary supports (%d); upgrade lingtai-portal",
			current, CurrentVersion,
		)
	}

	if current == CurrentVersion {
		return nil // already up to date
	}

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}
		if err := m.Fn(lingtaiDir); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Name, err)
		}
	}

	// Write new version atomically (write temp + rename)
	newMeta, _ := json.Marshal(metaFile{Version: CurrentVersion})
	tmpPath := metaPath + ".tmp"
	if err := os.WriteFile(tmpPath, newMeta, 0o644); err != nil {
		return fmt.Errorf("write meta.json.tmp: %w", err)
	}
	if err := os.Rename(tmpPath, metaPath); err != nil {
		return fmt.Errorf("rename meta.json: %w", err)
	}

	return nil
}
