package tui

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"

	"github.com/anthropics/lingtai-tui/i18n"
)

// MarkdownEntry is a single item in the markdown viewer's left panel.
type MarkdownEntry struct {
	Label   string // display name shown in list
	Group   string // section header (entries with same group are grouped)
	Path    string // absolute path to file (read on selection)
	Content string // pre-built content (used instead of Path if non-empty)
}

// MarkdownViewerCloseMsg is sent when the user exits the viewer.
type MarkdownViewerCloseMsg struct{}

// MarkdownViewerModel is a two-panel view: entry list (left) + rendered
// markdown content (right). It is a standalone tea.Model — callers build
// the entry list and pass it in.
type MarkdownViewerModel struct {
	entries []MarkdownEntry
	title   string
	width   int
	height  int
	cursor  int

	viewport viewport.Model
	ready    bool
}

const (
	mdvHeaderLines = 2
	mdvFooterLines = 2
)

// NewMarkdownViewer creates a viewer with the given entries and title.
func NewMarkdownViewer(entries []MarkdownEntry, title string) MarkdownViewerModel {
	return MarkdownViewerModel{
		entries: entries,
		title:   title,
	}
}

func (m MarkdownViewerModel) Init() tea.Cmd { return nil }

func (m MarkdownViewerModel) Update(msg tea.Msg) (MarkdownViewerModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpHeight := m.height - mdvHeaderLines - mdvFooterLines
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
		m.syncContent()

	case tea.MouseWheelMsg:
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return MarkdownViewerCloseMsg{} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.syncContent()
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.syncContent()
			}
			return m, nil
		default:
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *MarkdownViewerModel) syncContent() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.renderBody())
}

func (m MarkdownViewerModel) renderBody() string {
	leftW := m.width / 3
	if leftW < 25 {
		leftW = 25
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW := m.width - leftW - 2 // -1 separator, -1 left padding
	if rightW < 20 {
		rightW = 20
	}
	if leftW+2+rightW > m.width && m.width > 2 {
		rightW = m.width - leftW - 2
		if rightW < 0 {
			rightW = 0
		}
	}

	leftContent := m.renderLeft(leftW)
	rightContent := m.renderRight(rightW)

	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")

	vpHeight := m.height - mdvHeaderLines - mdvFooterLines
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
		body.WriteString(l + sep + " " + rightLines[i] + "\n")
	}
	return strings.TrimRight(body.String(), "\n")
}

func (m MarkdownViewerModel) renderLeft(maxW int) string {
	selectedStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(ColorText)
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b"))

	problemsGroup := i18n.T("skills.problems")

	var lines []string
	lastGroup := ""

	for i, e := range m.entries {
		if e.Group != lastGroup {
			if lastGroup != "" {
				lines = append(lines, "")
			}
			gs := sectionStyle
			if e.Group == problemsGroup || e.Group == "Problems" {
				gs = warnStyle
			}
			lines = append(lines, "  "+gs.Render(e.Group))
			lines = append(lines, "")
			lastGroup = e.Group
		}

		marker := "  "
		style := normalStyle
		if e.Group == problemsGroup || e.Group == "Problems" {
			style = warnStyle
		}
		if i == m.cursor {
			marker = "> "
			style = selectedStyle
		}
		lines = append(lines, "  "+marker+style.Render(e.Label))
	}

	if len(m.entries) == 0 {
		lines = append(lines, "  "+StyleFaint.Render("(empty)"))
	}

	return strings.Join(lines, "\n")
}

func (m MarkdownViewerModel) renderRight(maxW int) string {
	if len(m.entries) == 0 || m.cursor >= len(m.entries) {
		return "\n  " + StyleFaint.Render("(no content)")
	}

	e := m.entries[m.cursor]

	var raw string
	if e.Content != "" {
		raw = e.Content
	} else if e.Path != "" {
		data, err := os.ReadFile(e.Path)
		if err != nil {
			return "\n  " + StyleFaint.Render("(file not found)")
		}
		raw = string(data)
	} else {
		return "\n  " + StyleFaint.Render("(no content)")
	}

	// Strip YAML frontmatter if present
	if loc := fmRe.FindStringIndex(raw); loc != nil {
		raw = raw[loc[1]:]
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "\n  " + StyleFaint.Render("(empty)")
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(ActiveTheme().GlamourStyle),
		glamour.WithWordWrap(maxW-2),
	)
	if err == nil {
		if rendered, rerr := r.Render(raw); rerr == nil {
			return "\n" + rendered
		}
	}

	wrapped := lipgloss.NewStyle().Width(maxW - 2).Render(raw)
	var lines []string
	lines = append(lines, "")
	for _, line := range strings.Split(wrapped, "\n") {
		lines = append(lines, " "+line)
	}
	return strings.Join(lines, "\n")
}

func (m MarkdownViewerModel) View() string {
	title := StyleTitle.Render("  "+m.title) + "\n" + strings.Repeat("\u2500", m.width)

	scrollHint := ""
	if m.ready && !m.viewport.AtBottom() {
		scrollHint = " " + RuneBullet + " pgup/pgdn scroll"
	}
	footer := strings.Repeat("\u2500", m.width) + "\n" +
		StyleFaint.Render("  ↑↓ "+i18n.T("welcome.select_lang")+"  [Esc] "+i18n.T("firstrun.back")+scrollHint)

	return title + "\n" + PaintViewportBG(m.viewport.View(), m.width) + "\n" + footer
}
