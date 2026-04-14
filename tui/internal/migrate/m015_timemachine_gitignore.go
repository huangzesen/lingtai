package migrate

import (
	"os"
	"path/filepath"
)

const timeMachineGitignore = `# Time Machine — auto-generated
**/.git/
*.lock
*.heartbeat
*.pid
.status.json
*.pyc
__pycache__/
logs/
history/
.portal/
`

// migrateTimeMachineGitignore creates .lingtai/.gitignore for the
// network-level time machine. No-op if .gitignore already exists.
func migrateTimeMachineGitignore(lingtaiDir string) error {
	gitignorePath := filepath.Join(lingtaiDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		return nil
	}
	return os.WriteFile(gitignorePath, []byte(timeMachineGitignore), 0o644)
}
