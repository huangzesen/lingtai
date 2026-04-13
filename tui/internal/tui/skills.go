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
	Group       string // group folder name (e.g., "intrinsic", "custom", recipe name)
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

// scanSkills scans skillsDir recursively for skill folders.
//
// A directory with SKILL.md is a skill folder (leaf).
// A directory containing only subdirectories (no loose files) is a group folder.
// A directory with loose files but no SKILL.md is corrupted.
func scanSkills(skillsDir string) ([]skillEntry, []skillProblem) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, nil
	}

	var skills []skillEntry
	var problems []skillProblem

	for _, e := range entries {
		if isHiddenEntry(e.Name()) {
			continue
		}
		// Follow symlinks for stat
		childPath := filepath.Join(skillsDir, e.Name())
		info, err := os.Stat(childPath)
		if err != nil || !info.IsDir() {
			continue
		}

		skillFile := filepath.Join(childPath, "SKILL.md")
		if fileExists(skillFile) {
			// Flat skill (legacy or ungrouped)
			sk, prob := parseSkillFile(skillFile, e.Name(), "")
			if sk != nil {
				skills = append(skills, *sk)
			}
			if prob != nil {
				problems = append(problems, *prob)
			}
			continue
		}

		// No SKILL.md — check if it's a valid group folder
		scanGroup(childPath, e.Name(), &skills, &problems)
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, problems
}

// scanGroup scans a group folder recursively.
func scanGroup(dir, group string, skills *[]skillEntry, problems *[]skillProblem) {
	children, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Check for loose files (corruption check)
	for _, c := range children {
		if isHiddenEntry(c.Name()) {
			continue
		}
		childPath := filepath.Join(dir, c.Name())
		info, err := os.Stat(childPath)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			*problems = append(*problems, skillProblem{
				Folder: group,
				Reason: "not a skill (no SKILL.md) and has loose files",
			})
			return
		}
	}

	// All children are directories — recurse
	for _, c := range children {
		if isHiddenEntry(c.Name()) {
			continue
		}
		childPath := filepath.Join(dir, c.Name())
		info, err := os.Stat(childPath)
		if err != nil || !info.IsDir() {
			continue
		}

		skillFile := filepath.Join(childPath, "SKILL.md")
		if fileExists(skillFile) {
			sk, prob := parseSkillFile(skillFile, c.Name(), group)
			if sk != nil {
				*skills = append(*skills, *sk)
			}
			if prob != nil {
				*problems = append(*problems, *prob)
			}
			continue
		}

		// Nested group — recurse deeper
		nestedGroup := group + "/" + c.Name()
		scanGroup(childPath, nestedGroup, skills, problems)
	}
}

func parseSkillFile(skillFile, folderName, group string) (*skillEntry, *skillProblem) {
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, &skillProblem{Folder: folderName, Reason: "cannot read SKILL.md"}
	}
	text := string(data)
	fm := parseFrontmatter(text)
	if fm == nil {
		return nil, &skillProblem{Folder: folderName, Reason: "invalid frontmatter"}
	}
	name := fm["name"]
	desc := fm["description"]
	if name == "" {
		return nil, &skillProblem{Folder: folderName, Reason: "missing name"}
	}
	if desc == "" {
		return nil, &skillProblem{Folder: folderName, Reason: "missing description"}
	}
	return &skillEntry{
		Name:        name,
		Description: desc,
		Version:     fm["version"],
		Path:        skillFile,
		Body:        text,
		Group:       group,
	}, nil
}

// buildSkillEntries converts scan results into MarkdownEntry items for the
// markdown viewer. Skills are grouped by their Group field (folder name).
// "intrinsic" is always shown last. Empty group means ungrouped (legacy).
//
// For intrinsic skills with i18n variants (SKILL-{lang}.md), the viewer
// concatenates SKILL.md + SKILL-{lang}.md (falling back to SKILL-en.md).
func buildSkillEntries(skillsDir, lang string, skills []skillEntry, problems []skillProblem) []MarkdownEntry {
	// Collect groups in order: custom first, then recipe groups, intrinsic last.
	type groupBucket struct {
		name   string
		skills []skillEntry
	}

	groupMap := make(map[string]*groupBucket)
	var groupOrder []string

	for _, sk := range skills {
		g := sk.Group
		if g == "" {
			g = "custom" // legacy ungrouped skills → custom
		}
		if _, exists := groupMap[g]; !exists {
			groupMap[g] = &groupBucket{name: g}
			groupOrder = append(groupOrder, g)
		}
		groupMap[g].skills = append(groupMap[g].skills, sk)
	}

	// Sort groups: custom first, intrinsic last, everything else alphabetical in between
	sort.SliceStable(groupOrder, func(i, j int) bool {
		a, b := groupOrder[i], groupOrder[j]
		if a == "custom" {
			return true
		}
		if b == "custom" {
			return false
		}
		if a == "intrinsic" {
			return false
		}
		if b == "intrinsic" {
			return true
		}
		return a < b
	})

	var entries []MarkdownEntry
	for _, g := range groupOrder {
		bucket := groupMap[g]
		for _, sk := range bucket.skills {
			label := sk.Name
			if sk.Version != "" {
				label += " " + sk.Version
			}
			entry := MarkdownEntry{
				Label: label,
				Group: bucket.name,
				Path:  sk.Path,
			}
			// For intrinsic skills, concat SKILL.md + SKILL-{lang}.md for display.
			if bucket.name == "intrinsic" {
				entry.Content = concatSkillI18n(sk.Path, lang)
			}
			entries = append(entries, entry)
		}
	}

	if len(problems) > 0 {
		for _, p := range problems {
			entries = append(entries, MarkdownEntry{
				Label:   p.Folder,
				Group:   i18n.T("skills.problems"),
				Content: p.Reason,
			})
		}
	}

	return entries
}

// concatSkillI18n reads SKILL.md and appends the best SKILL-{lang}.md
// variant for display. Falls back to SKILL-en.md if the requested lang
// variant does not exist. If no lang variant exists at all, returns just
// the SKILL.md content.
func concatSkillI18n(skillMdPath, lang string) string {
	base, err := os.ReadFile(skillMdPath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(skillMdPath)

	// Try SKILL-{lang}.md, fall back to SKILL-en.md
	langFile := filepath.Join(dir, "SKILL-"+lang+".md")
	data, err := os.ReadFile(langFile)
	if err != nil && lang != "en" {
		langFile = filepath.Join(dir, "SKILL-en.md")
		data, err = os.ReadFile(langFile)
	}
	if err != nil {
		return string(base) // no lang variant — just show SKILL.md
	}

	return string(base) + "\n---\n\n" + string(data)
}

func isHiddenEntry(name string) bool {
	return len(name) > 0 && name[0] == '.'
}
