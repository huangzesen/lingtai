package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/process"
)

type AgentsModel struct {
	agents     []fs.AgentNode
	cursor     int
	baseDir    string
	lingtaiCmd string
	message    string
}

func NewAgentsModel(baseDir, lingtaiCmd string) AgentsModel {
	return AgentsModel{baseDir: baseDir, lingtaiCmd: lingtaiCmd}
}

type agentRefreshMsg struct{ agents []fs.AgentNode }

func (m AgentsModel) refreshAgents() tea.Msg {
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

func (m AgentsModel) Init() tea.Cmd { return m.refreshAgents }

func (m AgentsModel) Update(msg tea.Msg) (AgentsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case agentRefreshMsg:
		m.agents = msg.agents
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
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
					m.message = fmt.Sprintf("sent sleep to %s", a.AgentName)
				}
			}
		case "k":
			if m.cursor < len(m.agents) {
				a := m.agents[m.cursor]
				if !a.IsHuman {
					fs.TouchSignal(a.WorkingDir, fs.SignalSuspend)
					m.message = fmt.Sprintf("sent suspend to %s", a.AgentName)
				}
			}
		case "i":
			if m.cursor < len(m.agents) {
				a := m.agents[m.cursor]
				if !a.IsHuman {
					fs.TouchSignal(a.WorkingDir, fs.SignalInterrupt)
					m.message = fmt.Sprintf("sent interrupt to %s", a.AgentName)
				}
			}
		case "r":
			if m.cursor < len(m.agents) && m.lingtaiCmd != "" {
				a := m.agents[m.cursor]
				if !a.IsHuman && !a.Alive {
					process.LaunchAgent(m.lingtaiCmd, a.WorkingDir)
					m.message = fmt.Sprintf("reviving %s", a.AgentName)
				}
			}
		}
	}
	return m, nil
}

func (m AgentsModel) View() string {
	var b strings.Builder
	title := StyleTitle.Render("灵台 — Agents")
	b.WriteString("\n  " + title + "              [tab] Mail\n\n")
	header := fmt.Sprintf("  %-3s %-15s %-12s %-8s", "", "NAME", "STATE", "ALIVE")
	b.WriteString(StyleSubtle.Render(header) + "\n")
	for i, a := range m.agents {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		stateStyle := lipgloss.NewStyle().Foreground(StateColor(a.State))
		aliveStr := "●"
		if !a.Alive {
			aliveStr = "○"
		}
		line := fmt.Sprintf("%s%-15s %s  %s", cursor, a.AgentName, stateStyle.Render(fmt.Sprintf("%-12s", a.State)), aliveStr)
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(StyleSubtle.Render("  [s]leep  [k]ill  [i]nterrupt  [r]evive") + "\n")
	if m.message != "" {
		b.WriteString("\n  " + m.message + "\n")
	}
	return b.String()
}
