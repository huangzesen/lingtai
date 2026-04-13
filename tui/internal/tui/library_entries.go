package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// libraryFile matches the JSON schema of library/library.json written by agents.
type libraryFile struct {
	Version int            `json:"version"`
	Entries []libraryEntry `json:"entries"`
}

type libraryEntry struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	Content       string `json:"content"`
	Supplementary string `json:"supplementary"`
	CreatedAt     string `json:"created_at"`
}

// buildLibraryEntries scans all agent directories under lingtaiDir for
// library/library.json files and returns MarkdownEntry items grouped by
// agent name. Each library entry becomes one sidebar item whose content
// is the entry's markdown content (with metadata header).
func buildLibraryEntries(lingtaiDir string) []MarkdownEntry {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return nil
	}

	var result []MarkdownEntry

	// Collect agents that have libraries, sorted by name
	type agentLib struct {
		name    string
		entries []libraryEntry
	}
	var agents []agentLib

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		libPath := filepath.Join(lingtaiDir, entry.Name(), "library", "library.json")
		data, err := os.ReadFile(libPath)
		if err != nil {
			continue
		}
		var lib libraryFile
		if json.Unmarshal(data, &lib) != nil || len(lib.Entries) == 0 {
			continue
		}
		agents = append(agents, agentLib{name: entry.Name(), entries: lib.Entries})
	}

	sort.Slice(agents, func(i, j int) bool {
		return agents[i].name < agents[j].name
	})

	for _, ag := range agents {
		// Sort entries by created_at descending (newest first)
		sort.Slice(ag.entries, func(i, j int) bool {
			return ag.entries[i].CreatedAt > ag.entries[j].CreatedAt
		})
		for _, le := range ag.entries {
			label := le.Title
			if label == "" {
				label = le.ID
			}
			// Truncate long titles for sidebar
			if len(label) > 30 {
				label = label[:27] + "..."
			}

			// Build the right-panel content as markdown
			var md strings.Builder
			md.WriteString("# " + le.Title + "\n\n")
			if le.Summary != "" {
				md.WriteString("> " + le.Summary + "\n\n")
			}
			if le.CreatedAt != "" {
				if t, err := time.Parse(time.RFC3339Nano, le.CreatedAt); err == nil {
					md.WriteString(fmt.Sprintf("*%s* · `%s`\n\n", t.Format("2006-01-02 15:04"), le.ID))
				} else {
					md.WriteString(fmt.Sprintf("`%s`\n\n", le.ID))
				}
			}
			md.WriteString("---\n\n")
			md.WriteString(le.Content)
			if le.Supplementary != "" {
				md.WriteString("\n\n---\n\n## Supplementary\n\n" + le.Supplementary)
			}

			result = append(result, MarkdownEntry{
				Label:   label,
				Group:   ag.name,
				Content: md.String(),
			})
		}
	}

	return result
}
