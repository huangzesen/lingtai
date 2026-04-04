package migrate

import (
	"os"
	"path/filepath"
)

// migrateCharacterToLingtai renames system/character.md → system/lingtai.md
// in every agent working directory under .lingtai/.
func migrateCharacterToLingtai(lingtaiDir string) error {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		oldPath := filepath.Join(lingtaiDir, entry.Name(), "system", "character.md")
		newPath := filepath.Join(lingtaiDir, entry.Name(), "system", "lingtai.md")

		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			continue // no character.md to migrate
		}
		if _, err := os.Stat(newPath); err == nil {
			continue // lingtai.md already exists
		}

		if err := os.Rename(oldPath, newPath); err != nil {
			return err
		}
	}
	return nil
}
