package migrate

import (
	"os"
	"path/filepath"
)

// migrateAgoraRename renames ~/lingtai-agora/projects/ to
// ~/lingtai-agora/networks/ if the old path exists and the new path
// does not. This is a one-time migration for the /export command rename.
func migrateAgoraRename(_ string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil // can't resolve home — skip silently
	}
	oldDir := filepath.Join(home, "lingtai-agora", "projects")
	newDir := filepath.Join(home, "lingtai-agora", "networks")

	if _, err := os.Stat(oldDir); err != nil {
		return nil // old dir doesn't exist — nothing to do
	}
	if _, err := os.Stat(newDir); err == nil {
		return nil // new dir already exists — don't clobber
	}

	return os.Rename(oldDir, newDir)
}
