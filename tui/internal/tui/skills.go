package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

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

// SkillsModel is a two-panel view: skill list (left) + SKILL.md content (right).
type SkillsModel struct {
	skillsDir string // .lingtai/.skills/
	width     int
	height    int

	skills   []skillEntry
	problems []skillProblem
	cursor   int

	// Right panel viewport for SKILL.md content
	viewport viewport.Model
	ready    bool
}

func NewSkillsModel(projectDir string) SkillsModel {
	return SkillsModel{
		skillsDir: filepath.Join(projectDir, ".skills"),
	}
}

// skillsLoadMsg carries scan results.
type skillsLoadMsg struct {
	skills   []skillEntry
	problems []skillProblem
}

const (
	skillsHeaderLines = 2
	skillsFooterLines = 2
)

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
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
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

// ── Tea lifecycle ───────────────────────────────────────────────────

func (m SkillsModel) loadData() tea.Msg {
	skills, problems := scanSkills(m.skillsDir)
	return skillsLoadMsg{skills: skills, problems: problems}
}

func (m SkillsModel) Init() tea.Cmd { return m.loadData }

func (m SkillsModel) Update(msg tea.Msg) (SkillsModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpHeight := m.height - skillsHeaderLines - skillsFooterLines
		if vpHeight < 1 {
			vpHeight = 1
		}
		if !m.ready {
			m.viewport = viewport.New()
			m.viewport.SetWidth(m.width)
			m.viewport.SetHeight(vpHeight)
			m.ready = true
		} else {
			m.viewport.SetWidth(m.width)
			m.viewport.SetHeight(vpHeight)
		}
		m.syncViewportContent()

	case skillsLoadMsg:
		m.skills = msg.skills
		m.problems = msg.problems
		if m.cursor >= len(m.skills) {
			m.cursor = max(0, len(m.skills)-1)
		}
		m.syncViewportContent()

	case tea.MouseWheelMsg:
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.syncViewportContent()
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.skills)-1 {
				m.cursor++
				m.syncViewportContent()
			}
			return m, nil
		case "r":
			return m, m.loadData
		default:
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// ── Rendering ───────────────────────────────────────────────────────

func (m *SkillsModel) syncViewportContent() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.renderBody())
}

func (m SkillsModel) renderBody() string {
	leftW := m.width / 3
	if leftW < 25 {
		leftW = 25
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW := m.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}
	// Safety
	if leftW+1+rightW > m.width && m.width > 1 {
		rightW = m.width - leftW - 1
		if rightW < 0 {
			rightW = 0
		}
	}

	leftContent := m.renderLeft(leftW)
	rightContent := m.renderRight(rightW)

	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")

	// Pad to equal length
	vpHeight := m.height - skillsHeaderLines - skillsFooterLines
	if vpHeight < 1 {
		vpHeight = 1
	}
	for len(leftLines) < vpHeight {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < vpHeight {
		rightLines = append(rightLines, "")
	}
	for len(leftLines) < len(rightLines) {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < len(leftLines) {
		rightLines = append(rightLines, "")
	}

	sep := lipgloss.NewStyle().Foreground(ColorTextFaint).Render("│")

	var body strings.Builder
	for i := 0; i < len(leftLines); i++ {
		l := padToWidth(leftLines[i], leftW)
		body.WriteString(l + sep + rightLines[i] + "\n")
	}
	return strings.TrimRight(body.String(), "\n")
}

func (m SkillsModel) renderLeft(maxW int) string {
	nameStyle := lipgloss.NewStyle().Foreground(ColorText)
	selectedStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	versionStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b"))
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+sectionStyle.Render(i18n.T("skills.installed")))
	lines = append(lines, "")

	if len(m.skills) == 0 && len(m.problems) == 0 {
		lines = append(lines, "  "+StyleFaint.Render(i18n.T("skills.none")))
	}

	for i, sk := range m.skills {
		marker := "  "
		style := nameStyle
		if i == m.cursor {
			marker = "> "
			style = selectedStyle
		}
		ver := ""
		if sk.Version != "" {
			ver = " " + versionStyle.Render(sk.Version)
		}
		lines = append(lines, "  "+marker+style.Render(sk.Name)+ver)
	}

	if len(m.problems) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+warnStyle.Render(i18n.T("skills.problems")))
		lines = append(lines, "")
		for _, p := range m.problems {
			lines = append(lines, "  "+warnStyle.Render("  "+p.Folder))
			lines = append(lines, "  "+StyleFaint.Render("  "+p.Reason))
		}
	}

	return strings.Join(lines, "\n")
}

func (m SkillsModel) renderRight(maxW int) string {
	if len(m.skills) == 0 {
		return "\n  " + StyleFaint.Render(i18n.T("skills.select_hint"))
	}
	if m.cursor >= len(m.skills) {
		return ""
	}

	sk := m.skills[m.cursor]

	// Wrap body text to fit the right panel
	wrapped := lipgloss.NewStyle().Width(maxW - 2).Render(sk.Body)

	var lines []string
	lines = append(lines, "")
	for _, line := range strings.Split(wrapped, "\n") {
		lines = append(lines, " "+line)
	}
	return strings.Join(lines, "\n")
}

func (m SkillsModel) View() string {
	title := StyleTitle.Render("  "+i18n.T("skills.title")) + "\n" + strings.Repeat("\u2500", m.width)

	scrollHint := ""
	if m.ready && !m.viewport.AtBottom() {
		scrollHint = " " + RuneBullet + " pgup/pgdn scroll"
	}
	footer := strings.Repeat("\u2500", m.width) + "\n" +
		StyleFaint.Render("  "+i18n.T("hints.skills_nav")+scrollHint)

	return title + "\n" + m.viewport.View() + "\n" + footer
}
