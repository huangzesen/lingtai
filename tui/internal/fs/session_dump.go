// tui/internal/fs/session_dump.go
package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// renderMailEntry renders a mail SessionEntry to markdown.
func renderMailEntry(e SessionEntry) string {
	ts := ""
	if t, err := time.Parse(time.RFC3339, e.Ts); err == nil {
		ts = t.UTC().Format("15:04")
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**%s** %s → %s", e.From, ts, e.To))
	if e.Subject != "" {
		b.WriteString(" │ Re: " + e.Subject)
	}
	b.WriteString("\n" + e.Body + "\n")
	if len(e.Attachments) > 0 {
		b.WriteString("Attachments:\n")
		for i, att := range e.Attachments {
			b.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, att))
		}
	}
	return b.String()
}

// renderEventEntry renders a thinking/tool/diary SessionEntry to markdown.
func renderEventEntry(e SessionEntry) string {
	return fmt.Sprintf("[%s] %s\n", e.Type, e.Body)
}

// renderInsightEntry renders an insight SessionEntry to markdown.
func renderInsightEntry(e SessionEntry) string {
	var b strings.Builder
	b.WriteString("---\n")
	if e.Question != "" {
		b.WriteString("/btw › " + e.Question + "\n")
	} else {
		b.WriteString("★ insight\n")
	}
	b.WriteString(e.Body + "\n")
	b.WriteString("---\n")
	return b.String()
}
