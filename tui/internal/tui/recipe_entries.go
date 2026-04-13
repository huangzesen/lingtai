package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/lingtai-tui/i18n"
)

// buildRecipeEntries scans a recipe directory and returns MarkdownEntry items
// for the markdown viewer. Discovers all lang variants of greet.md, comment.md,
// covenant.md, procedures.md, and skills.
func buildRecipeEntries(recipeDir string) []MarkdownEntry {
	if recipeDir == "" {
		return nil
	}
	var entries []MarkdownEntry

	addFile := func(filename, group string) {
		// Root version
		rootPath := filepath.Join(recipeDir, filename)
		if info, err := os.Stat(rootPath); err == nil && !info.IsDir() {
			entries = append(entries, MarkdownEntry{
				Label: filename,
				Group: group,
				Path:  rootPath,
			})
		}
		// Lang variants
		dirEntries, err := os.ReadDir(recipeDir)
		if err != nil {
			return
		}
		for _, e := range dirEntries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "skills" {
				continue
			}
			langPath := filepath.Join(recipeDir, e.Name(), filename)
			if info, err := os.Stat(langPath); err == nil && !info.IsDir() {
				entries = append(entries, MarkdownEntry{
					Label: filename + " (" + e.Name() + ")",
					Group: group,
					Path:  langPath,
				})
			}
		}
	}

	addFile("greet.md", "greet.md")
	addFile("comment.md", "comment.md")
	addFile("recipe.json", "recipe.json")

	// Skills
	skillsRoot := filepath.Join(recipeDir, "skills")
	skillDirs, err := os.ReadDir(skillsRoot)
	if err == nil {
		for _, sd := range skillDirs {
			if !sd.IsDir() || strings.HasPrefix(sd.Name(), ".") {
				continue
			}
			skillName := sd.Name()
			rootSkill := filepath.Join(skillsRoot, skillName, "SKILL.md")
			if info, err := os.Stat(rootSkill); err == nil && !info.IsDir() {
				entries = append(entries, MarkdownEntry{
					Label: skillName + "/SKILL.md",
					Group: i18n.T("skills.title"),
					Path:  rootSkill,
				})
			}
			langDirs, err := os.ReadDir(filepath.Join(skillsRoot, skillName))
			if err != nil {
				continue
			}
			for _, ld := range langDirs {
				if !ld.IsDir() || strings.HasPrefix(ld.Name(), ".") {
					continue
				}
				langSkill := filepath.Join(skillsRoot, skillName, ld.Name(), "SKILL.md")
				if info, err := os.Stat(langSkill); err == nil && !info.IsDir() {
					entries = append(entries, MarkdownEntry{
						Label: skillName + "/SKILL.md (" + ld.Name() + ")",
						Group: i18n.T("skills.title"),
						Path:  langSkill,
					})
				}
			}
		}
	}

	// Optional overrides (only shown if they exist)
	addFile("covenant.md", "covenant.md")
	addFile("procedures.md", "procedures.md")

	return entries
}
