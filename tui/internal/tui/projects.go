package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
)

// projectEntry holds a registered project and its loaded details.
type projectEntry struct {
	Path    string
	Name    string         // basename of the project directory
	Network fs.Network     // loaded on select
	Current bool           // true if this is the TUI's current project
}

// ProjectsModel is a two-panel view: project list (left) + agent details (right).
type ProjectsModel struct {
	globalDir  string
	projectDir string // current TUI project's .lingtai/ directory
	width      int
	height     int

	projects []projectEntry
	cursor   int

	// Right panel viewport
	viewport viewport.Model
	ready    bool
}

func NewProjectsModel(globalDir, projectDir string) ProjectsModel {
	return ProjectsModel{
		globalDir:  globalDir,
		projectDir: projectDir,
	}
}

// projectsLoadMsg carries the loaded project list.
type projectsLoadMsg struct {
	projects []projectEntry
}

const (
	projectsHeaderLines = 2
	projectsFooterLines = 2
)

func (m ProjectsModel) loadData() tea.Msg {
	paths := config.LoadAndPrune(m.globalDir)
	currentProject := filepath.Dir(m.projectDir) // .lingtai/ → parent

	var projects []projectEntry
	for _, p := range paths {
		entry := projectEntry{
			Path:    p,
			Name:    filepath.Base(p),
			Current: p == currentProject,
		}
		// Load network info for each project
		lingtaiDir := filepath.Join(p, ".lingtai")
		net, _ := fs.BuildNetwork(lingtaiDir)
		entry.Network = net
		projects = append(projects, entry)
	}
	return projectsLoadMsg{projects: projects}
}

func (m ProjectsModel) Init() tea.Cmd { return m.loadData }

func (m ProjectsModel) Update(msg tea.Msg) (ProjectsModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpHeight := m.height - projectsHeaderLines - projectsFooterLines
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

	case projectsLoadMsg:
		m.projects = msg.projects
		if m.cursor >= len(m.projects) {
			m.cursor = max(0, len(m.projects)-1)
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
			if m.cursor < len(m.projects)-1 {
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

func (m *ProjectsModel) syncViewportContent() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.renderBody())
}

func (m ProjectsModel) renderBody() string {
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

	vpHeight := m.height - projectsHeaderLines - projectsFooterLines
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

func (m ProjectsModel) renderLeft(maxW int) string {
	nameStyle := lipgloss.NewStyle().Foreground(ColorText)
	selectedStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	currentStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+sectionStyle.Render(i18n.T("projects.registered")))
	lines = append(lines, "")

	if len(m.projects) == 0 {
		lines = append(lines, "  "+StyleFaint.Render(i18n.T("projects.none")))
	}

	for i, proj := range m.projects {
		marker := "  "
		style := nameStyle
		if i == m.cursor {
			marker = "> "
			style = selectedStyle
		}
		name := proj.Name
		suffix := ""
		if proj.Current {
			suffix = " " + currentStyle.Render(i18n.T("projects.current"))
		}
		lines = append(lines, "  "+marker+style.Render(name)+suffix)
	}

	return strings.Join(lines, "\n")
}

func (m ProjectsModel) renderRight(maxW int) string {
	if len(m.projects) == 0 {
		return "\n  " + StyleFaint.Render(i18n.T("projects.select_hint"))
	}
	if m.cursor >= len(m.projects) {
		return ""
	}

	proj := m.projects[m.cursor]

	labelStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	var lines []string

	// Path
	lines = append(lines, "")
	lines = append(lines, "  "+labelStyle.Render(i18n.T("projects.path")+": ")+valueStyle.Render(proj.Path))
	lines = append(lines, "")

	// Agent list
	lines = append(lines, "  "+sectionStyle.Render(i18n.T("projects.section_agents")))
	lines = append(lines, "")

	net := proj.Network
	if len(net.Nodes) == 0 {
		lines = append(lines, "  "+StyleFaint.Render("  ──"))
	} else {
		for _, n := range net.Nodes {
			name := n.AgentName
			if n.Nickname != "" {
				name = n.Nickname
			}
			if name == "" {
				name = "(unknown)"
			}
			state := n.State
			if state == "" {
				state = "──"
			}
			stateRendered := lipgloss.NewStyle().Foreground(StateColor(strings.ToUpper(state))).Render(state)
			if n.IsHuman {
				name = "human"
				stateRendered = lipgloss.NewStyle().Foreground(StateColor("ACTIVE")).Render("ACTIVE")
			}
			lines = append(lines, fmt.Sprintf("  %-20s %s", valueStyle.Render(name), stateRendered))
		}
	}

	// Network stats
	stats := net.Stats
	lines = append(lines, "")
	lines = append(lines, "  "+sectionStyle.Render(i18n.T("projects.section_network")))
	lines = append(lines, "")

	var stateParts []string
	if stats.Active > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("ACTIVE"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.active"), stats.Active)))
	}
	if stats.Idle > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("IDLE"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.idle"), stats.Idle)))
	}
	if stats.Stuck > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("STUCK"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.stuck"), stats.Stuck)))
	}
	if stats.Asleep > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("ASLEEP"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.asleep"), stats.Asleep)))
	}
	if stats.Suspended > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("SUSPENDED"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.suspended"), stats.Suspended)))
	}
	if len(stateParts) > 0 {
		lines = append(lines, "  "+strings.Join(stateParts, "  "))
	} else {
		lines = append(lines, "  "+StyleFaint.Render("──"))
	}

	// Mail count
	if stats.TotalMails > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.total_mails")+": ")+valueStyle.Render(fmt.Sprintf("%d", stats.TotalMails)))
	}

	return strings.Join(lines, "\n")
}

func (m ProjectsModel) View() string {
	title := StyleTitle.Render("  "+i18n.T("projects.title")) + "\n" + strings.Repeat("\u2500", m.width)

	scrollHint := ""
	if m.ready && !m.viewport.AtBottom() {
		scrollHint = " " + RuneBullet + " pgup/pgdn scroll"
	}
	footer := strings.Repeat("\u2500", m.width) + "\n" +
		StyleFaint.Render("  "+i18n.T("hints.projects_nav")+scrollHint)

	return title + "\n" + m.viewport.View() + "\n" + footer
}
