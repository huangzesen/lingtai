package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/anthropics/lingtai-tui/i18n"
)

// skillEntry holds parsed metadata for one skill.
type skillEntry struct {
	Name        string
	Description string
	Version     string
	Path        string // absolute path to SKILL.md
	Body        string // raw content of SKILL.md (loaded on select)
}

// skillProblem describes a broken skill folder.
type skillProblem struct {
	Folder string
	Reason string
}

// ── Frontmatter parser ──────────────────────────────────────────────

var (
	fmRe = regexp.MustCompile(`(?s)\A---\s*\n(.*?\n)---\s*\n`)
	kvRe = regexp.MustCompile(`(?m)^(\w[\w-]*)\s*:\s*(.+)$`)
)

func parseFrontmatter(text string) map[string]string {
	m := fmRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	result := make(map[string]string)
	for _, kv := range kvRe.FindAllStringSubmatch(m[1], -1) {
		result[kv[1]] = strings.TrimSpace(kv[2])
	}
	return result
}

// ── Scan ────────────────────────────────────────────────────────────

func scanSkills(skillsDir string) ([]skillEntry, []skillProblem) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, nil
	}

	var skills []skillEntry
	var problems []skillProblem

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		// Use os.Stat (follows symlinks) instead of e.IsDir() so that
		// symlinked skill directories from recipes are discovered.
		info, err := os.Stat(filepath.Join(skillsDir, e.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			problems = append(problems, skillProblem{Folder: e.Name(), Reason: "missing SKILL.md"})
			continue
		}
		text := string(data)
		fm := parseFrontmatter(text)
		if fm == nil {
			problems = append(problems, skillProblem{Folder: e.Name(), Reason: "invalid frontmatter"})
			continue
		}
		name := fm["name"]
		desc := fm["description"]
		if name == "" {
			problems = append(problems, skillProblem{Folder: e.Name(), Reason: "missing name"})
			continue
		}
		if desc == "" {
			problems = append(problems, skillProblem{Folder: e.Name(), Reason: "missing description"})
			continue
		}
		skills = append(skills, skillEntry{
			Name:        name,
			Description: desc,
			Version:     fm["version"],
			Path:        skillFile,
			Body:        text,
		})
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, problems
}

// buildSkillEntries converts scan results into MarkdownEntry items for the
// markdown viewer. Bundled (non-symlink) skills go under "Skills", symlinked
// skills under "Imported", and broken folders under "Problems".
func buildSkillEntries(skillsDir string, skills []skillEntry, problems []skillProblem) []MarkdownEntry {
	var entries []MarkdownEntry

	var bundled, imported []skillEntry
	for _, sk := range skills {
		dir := filepath.Dir(sk.Path)
		info, err := os.Lstat(dir)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			imported = append(imported, sk)
		} else {
			bundled = append(bundled, sk)
		}
	}

	for _, sk := range bundled {
		label := sk.Name
		if sk.Version != "" {
			label += " " + sk.Version
		}
		entries = append(entries, MarkdownEntry{
			Label: label,
			Group: i18n.T("skills.installed"),
			Path:  sk.Path,
		})
	}

	for _, sk := range imported {
		label := sk.Name
		if sk.Version != "" {
			label += " " + sk.Version
		}
		entries = append(entries, MarkdownEntry{
			Label: label,
			Group: i18n.T("recipe.imported"),
			Path:  sk.Path,
		})
	}

	for _, p := range problems {
		entries = append(entries, MarkdownEntry{
			Label:   p.Folder,
			Group:   i18n.T("skills.problems"),
			Content: p.Reason,
		})
	}

	return entries
}
