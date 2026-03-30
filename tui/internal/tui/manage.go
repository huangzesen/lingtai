package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/process"
)

type agentRefreshMsg struct{ agents []fs.AgentNode }

// ManageModel is the /manage view — agent list with lifecycle signals.
type ManageModel struct {
	agents     []fs.AgentNode
	cursor     int
	baseDir    string
	lingtaiCmd string
	message    string
	width      int
	height     int
}

func NewManageModel(baseDir, lingtaiCmd string) ManageModel {
	return ManageModel{baseDir: baseDir, lingtaiCmd: lingtaiCmd}
}

func (m ManageModel) refreshAgents() tea.Msg {
	agents, _ := fs.DiscoverAgents(m.baseDir)
	for i := range agents {
		if agents[i].IsHuman {
			agents[i].Alive = true
		} else {
			agents[i].Alive = fs.IsAlive(agents[i].WorkingDir, 2.0)
		}
	}
	return agentRefreshMsg{agents: agents}
}

func (m ManageModel) Init() tea.Cmd {
	return tea.Batch(m.refreshAgents, tickEvery(time.Second))
}

func (m ManageModel) Update(msg tea.Msg) (ManageModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case agentRefreshMsg:
		m.agents = msg.agents
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refreshAgents, tickEvery(time.Second))

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		case "s":
			if m.cursor < len(m.agents) {
				a := m.agents[m.cursor]
				if !a.IsHuman {
					fs.TouchSignal(a.WorkingDir, fs.SignalSleep)
					m.message = i18n.TF("manage.sent_sleep", a.AgentName)
				}
			}
		case "k":
			if m.cursor < len(m.agents) {
				a := m.agents[m.cursor]
				if !a.IsHuman {
					fs.TouchSignal(a.WorkingDir, fs.SignalSuspend)
					m.message = i18n.TF("manage.sent_suspend", a.AgentName)
				}
			}
		case "i":
			if m.cursor < len(m.agents) {
				a := m.agents[m.cursor]
				if !a.IsHuman {
					fs.TouchSignal(a.WorkingDir, fs.SignalInterrupt)
					m.message = i18n.TF("manage.sent_interrupt", a.AgentName)
				}
			}
		case "r":
			if m.cursor < len(m.agents) && m.lingtaiCmd != "" {
				a := m.agents[m.cursor]
				if !a.IsHuman && !a.Alive {
					process.LaunchAgent(m.lingtaiCmd, a.WorkingDir)
					m.message = i18n.TF("manage.reviving", a.AgentName)
				}
			}
		}
	}
	return m, nil
}

func (m ManageModel) View() string {
	var b strings.Builder

	// Title bar
	title := StyleTitle.Render(i18n.T("app.title")) + " " + StyleAccent.Render(RuneBullet) + " " + StyleTitle.Render(i18n.T("manage.title"))
	escHint := StyleAccent.Render("[esc] ") + StyleSubtle.Render(i18n.T("manage.back"))
	padding := m.width - lipgloss.Width(title) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(title + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(title + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	// Header
	header := fmt.Sprintf("  %-3s %-15s %-12s %-8s",
		"",
		i18n.T("manage.header_name"),
		i18n.T("manage.header_state"),
		i18n.T("manage.header_alive"),
	)
	b.WriteString(StyleSubtle.Render(header) + "\n")

	// Agent list
	for i, a := range m.agents {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		stateStyle := lipgloss.NewStyle().Foreground(StateColor(a.State))
		aliveStr := lipgloss.NewStyle().Foreground(ColorAgent).Render("●")
		if !a.Alive {
			aliveStr = lipgloss.NewStyle().Foreground(ColorTextFaint).Render("○")
		}
		line := fmt.Sprintf("%s%-15s %s  %s",
			cursor,
			a.AgentName,
			stateStyle.Render(fmt.Sprintf("%-12s", a.State)),
			aliveStr,
		)
		b.WriteString(line + "\n")
	}

	// Footer
	b.WriteString("\n" + strings.Repeat("─", m.width) + "\n")
	hints := fmt.Sprintf("  [s]%s  [k]%s  [i]%s  [r]%s",
		i18n.T("manage.sleep"),
		i18n.T("manage.kill"),
		i18n.T("manage.interrupt"),
		i18n.T("manage.revive"),
	)
	b.WriteString(StyleFaint.Render(hints) + "\n")
	b.WriteString(StyleFaint.Render(fmt.Sprintf("  ↑↓ %s  [esc] %s",
		i18n.T("manage.select"), i18n.T("manage.back"))) + "\n")

	if m.message != "" {
		b.WriteString("\n  " + m.message + "\n")
	}

	return b.String()
}
