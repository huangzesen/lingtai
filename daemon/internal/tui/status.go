package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lingtai-daemon/internal/i18n"
	"lingtai-daemon/internal/manage"
)

// StatusModel shows all agents in this project's .lingtai/.
type StatusModel struct {
	spirits    []statusEntry
	lingtaiDir string
	width      int
	height     int
	cursor     int
	errMsg     string // shown once after agent start failure
}

type statusEntry struct {
	spirit    manage.Spirit
	comboName string
}

// StatusTransitionMsg signals a view transition from Status.
type StatusTransitionMsg struct {
	Target View
}

func NewStatus(lingtaiDir string) StatusModel {
	m := StatusModel{lingtaiDir: lingtaiDir}
	m.scan()
	return m
}

func (m *StatusModel) scan() {
	m.spirits = nil
	spirits := manage.ScanSpirits(m.lingtaiDir)
	for _, s := range spirits {
		comboName := readComboName(filepath.Join(m.lingtaiDir, s.Name, "combo.json"))
		m.spirits = append(m.spirits, statusEntry{spirit: s, comboName: comboName})
	}
}

func readComboName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "—"
	}
	var c struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(data, &c) != nil || c.Name == "" {
		return "—"
	}
	return c.Name
}

func (m StatusModel) Init() tea.Cmd {
	return nil
}

func (m StatusModel) Update(msg tea.Msg) (StatusModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.errMsg = "" // clear error on any key
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "s", "S":
			return m, func() tea.Msg { return StatusTransitionMsg{Target: ViewWizard} }
		case "enter":
			return m, func() tea.Msg { return StatusTransitionMsg{Target: ViewChat} }
		case "k", "K":
			// Kill all agents
			for _, e := range m.spirits {
				if e.spirit.Alive {
					if proc, err := os.FindProcess(e.spirit.PID); err == nil {
						proc.Signal(os.Interrupt)
					}
				}
			}
			m.scan()
		case "r", "R":
			m.scan()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m StatusModel) View() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")).Render("  " + i18n.S("title"))
	b.WriteString("\n" + title + "\n\n")

	if len(m.spirits) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Faint(true).Render(i18n.S("no_spirits")) + "\n")
	} else {
		// Header
		header := fmt.Sprintf("  %-3s %-18s %-10s %-18s %-7s", "", i18n.S("name"), i18n.S("status"), i18n.S("combo"), i18n.S("port"))
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render(header) + "\n")

		for _, e := range m.spirits {
			icon := "●"
			status := i18n.S("running")
			statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // green
			if !e.spirit.Alive {
				icon = "✗"
				status = i18n.S("dead")
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
			}
			nameStr := e.spirit.Name
			portStr := fmt.Sprintf(":%d", e.spirit.Port)

			line := fmt.Sprintf("  %s %-18s %s %-18s %-7s",
				statusStyle.Render(icon),
				nameStr,
				statusStyle.Render(fmt.Sprintf("%-10s", status)),
				lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render(e.comboName),
				lipgloss.NewStyle().Faint(true).Render(portStr),
			)
			b.WriteString(line + "\n")
		}
	}

	if m.errMsg != "" {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(m.errMsg) + "\n")
	}

	b.WriteString("\n")
	help := lipgloss.NewStyle().Faint(true).Render("  " + i18n.S("status_help"))
	b.WriteString(help + "\n")

	return b.String()
}
