// tui/internal/fs/session_dump.go
package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// projectHash returns the first 12 hex chars of SHA-256(projectPath).
func projectHash(projectPath string) string {
	sum := sha256.Sum256([]byte(projectPath))
	return hex.EncodeToString(sum[:])[:12]
}

// briefHistoryDir returns ~/.lingtai-tui/brief/<hash>/history/.
func briefHistoryDir(hash string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lingtai-tui", "brief", hash, "history")
}
