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

// ProjectHash returns the first 12 hex chars of SHA-256(projectPath).
func ProjectHash(projectPath string) string {
	sum := sha256.Sum256([]byte(projectPath))
	return hex.EncodeToString(sum[:])[:12]
}

// briefHistoryDir returns <base>/brief/projects/<hash>/history/.
func briefHistoryDir(base, hash string) string {
	return filepath.Join(base, "brief", "projects", hash, "history")
}

// BriefFilePath returns <base>/brief/projects/<hash>/brief.md for a project.
func BriefFilePath(base, projectPath string) string {
	hash := ProjectHash(projectPath)
	return filepath.Join(base, "brief", "projects", hash, "brief.md")
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

// renderHourMarkdown renders a slice of entries for one hour into a markdown document.
func renderHourMarkdown(entries []SessionEntry, hour time.Time) string {
	var b strings.Builder
	nextHour := hour.Add(time.Hour)
	b.WriteString(fmt.Sprintf("# Session — %s %s–%s UTC\n\n",
		hour.Format("2006-01-02"),
		hour.Format("15:04"),
		nextHour.Format("15:04"),
	))
	for _, e := range entries {
		switch e.Type {
		case "mail":
			b.WriteString(renderMailEntry(e))
		case "insight":
			b.WriteString(renderInsightEntry(e))
		default:
			b.WriteString(renderEventEntry(e))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// dumpCompletedHour renders entries for the given hour to markdown and writes
// to historyDir/YYYY-MM-DD-HH.md — but only if the content differs from the
// existing file (or the file does not exist). Empty entries produce no file.
func dumpCompletedHour(entries []SessionEntry, hour time.Time, historyDir string) {
	if len(entries) == 0 {
		return
	}
	content := renderHourMarkdown(entries, hour)
	filename := hour.Format("2006-01-02-15") + ".md"
	path := filepath.Join(historyDir, filename)

	// Compare with existing file.
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		return // identical — no rewrite
	}

	os.MkdirAll(historyDir, 0o755)
	os.WriteFile(path, []byte(content), 0o644)
}

// DumpAllHours groups entries by hour and writes each hour's markdown to
// historyDir. Unlike the runtime dumpHours (capped at 24h), this processes
// all entries — used by the migration to backfill full history.
func DumpAllHours(entries []SessionEntry, historyDir string) {
	if len(entries) == 0 {
		return
	}

	// Group entries by truncated hour.
	hours := make(map[time.Time][]SessionEntry)
	for _, e := range entries {
		t, err := time.Parse(time.RFC3339, e.Ts)
		if err != nil {
			continue
		}
		h := t.Truncate(time.Hour)
		hours[h] = append(hours[h], e)
	}

	for hour, hourEntries := range hours {
		dumpCompletedHour(hourEntries, hour, historyDir)
	}
}
