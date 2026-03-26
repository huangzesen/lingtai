package tui

import (
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type SetupModel struct {
	input     textinput.Model
	globalDir string
	done      bool
	err       error
}

func NewSetupModel(globalDir string) SetupModel {
	ti := textinput.New()
	ti.Placeholder = "Enter your MiniMax API key"
	ti.Focus()
	ti.CharLimit = 128
	ti.Width = 50
	return SetupModel{input: ti, globalDir: globalDir}
}

func (m SetupModel) Init() tea.Cmd { return textinput.Blink }

type setupDoneMsg struct{}

func (m SetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			key := m.input.Value()
			if key == "" {
				return m, nil
			}
			cfg := config.Config{MiniMaxAPIKey: key}
			if err := config.SaveConfig(m.globalDir, cfg); err != nil {
				m.err = err
				return m, nil
			}
			m.done = true
			return m, func() tea.Msg { return setupDoneMsg{} }
		case "esc", "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m SetupModel) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#48bb78")).Render("灵台 — Setup")
	s := "\n" + title + "\n\n"
	s += "  MiniMax API Key: " + m.input.View() + "\n\n"
	if m.err != nil {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("#e53e3e")).Render("  Error: "+m.err.Error()) + "\n\n"
	}
	s += StyleSubtle.Render("  [Enter] Save    [Esc] Quit") + "\n"
	return s
}

func (m SetupModel) Done() bool { return m.done }
