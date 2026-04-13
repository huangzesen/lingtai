package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
)

// buildSecretaryBriefs constructs the markdown entry list for the briefs viewer.
// Group 1: This project — profile.md + journal.md
// Group 2: Other projects — journal.md for each other project hash
func buildSecretaryBriefs(globalDir, projectDir string) []MarkdownEntry {
	briefBase := filepath.Join(globalDir, "brief")
	projectPath := filepath.Dir(projectDir) // projectDir is .lingtai/, parent is the project
	thisHash := fs.ProjectHash(projectPath)

	var entries []MarkdownEntry

	// Profile (universal)
	profilePath := filepath.Join(briefBase, "profile.md")
	if _, err := os.Stat(profilePath); err == nil {
		entries = append(entries, MarkdownEntry{
			Label: "profile.md",
			Group: "This Project",
			Path:  profilePath,
		})
	} else {
		entries = append(entries, MarkdownEntry{
			Label:   "profile.md",
			Group:   "This Project",
			Content: "*No profile yet — the secretary has not run a briefing cycle.*",
		})
	}

	// This project's journal
	thisJournal := filepath.Join(briefBase, "projects", thisHash, "journal.md")
	if _, err := os.Stat(thisJournal); err == nil {
		entries = append(entries, MarkdownEntry{
			Label: "journal.md",
			Group: "This Project",
			Path:  thisJournal,
		})
	} else {
		entries = append(entries, MarkdownEntry{
			Label:   "journal.md",
			Group:   "This Project",
			Content: "*No journal yet — the secretary has not run a briefing cycle.*",
		})
	}

	// Other projects' journals — only show projects that still exist.
	// LoadAndPrune is the authoritative check: it reads registry.jsonl,
	// removes entries whose .lingtai/ is gone, rewrites the file, and
	// returns surviving paths. We hash each to build a live-hash set.
	liveHashes := make(map[string]bool)
	for _, p := range config.LoadAndPrune(globalDir) {
		liveHashes[fs.ProjectHash(p)] = true
	}

	projectsDir := filepath.Join(briefBase, "projects")
	dirEntries, err := os.ReadDir(projectsDir)
	if err == nil {
		for _, d := range dirEntries {
			if !d.IsDir() || d.Name() == thisHash {
				continue
			}
			if !liveHashes[d.Name()] {
				continue // project no longer registered — skip
			}
			journalPath := filepath.Join(projectsDir, d.Name(), "journal.md")
			if _, err := os.Stat(journalPath); err == nil {
				// Try to show a friendlier label — use first line of journal if possible
				label := d.Name() + "/journal.md"
				if data, err := os.ReadFile(journalPath); err == nil {
					if first := firstNonEmptyLine(string(data)); first != "" {
						label = strings.TrimPrefix(first, "# ")
						if len(label) > 40 {
							label = label[:37] + "..."
						}
					}
				}
				entries = append(entries, MarkdownEntry{
					Label: label,
					Group: "Other Projects",
					Path:  journalPath,
				})
			}
		}
	}

	return entries
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
