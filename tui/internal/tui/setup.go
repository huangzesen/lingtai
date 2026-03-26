package tui

import (
	"strings"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SetupDoneMsg is emitted when API key setup completes.
type SetupDoneMsg struct{}

// SetupModel handles API key configuration.
type SetupModel struct {
	input      textinput.Model
	globalDir  string
	done       bool
	err        error
	currentKey string // masked display of existing key
	width      int
	height     int
}

func NewSetupModel(globalDir string) SetupModel {
	ti := textinput.New()
	ti.Placeholder = i18n.T("setup.prompt_api_key")
	ti.Focus()
	ti.CharLimit = 128
	ti.Width = 50

	// Load existing key for masked display
	var masked string
	cfg, err := config.LoadConfig(globalDir)
	if err == nil && cfg.MiniMaxAPIKey != "" {
		key := cfg.MiniMaxAPIKey
		if len(key) > 8 {
			masked = key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
		} else {
			masked = strings.Repeat("*", len(key))
		}
	}

	return SetupModel{
		input:      ti,
		globalDir:  globalDir,
		currentKey: masked,
	}
}

func (m SetupModel) Init() tea.Cmd { return textinput.Blink }

func (m SetupModel) Update(msg tea.Msg) (SetupModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			key := m.input.Value()
			if key == "" && m.currentKey != "" {
				// Existing key is fine, proceed without changes
				m.done = true
				return m, func() tea.Msg { return SetupDoneMsg{} }
			}
			if key == "" {
				return m, nil
			}
			cfg := config.Config{MiniMaxAPIKey: key}
			if err := config.SaveConfig(m.globalDir, cfg); err != nil {
				m.err = err
				return m, nil
			}
			m.done = true
			return m, func() tea.Msg { return SetupDoneMsg{} }
		case "esc":
			return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m SetupModel) View() string {
	var b strings.Builder

	// Title bar
	title := StyleTitle.Render("  " + i18n.T("app.title") + " — " + i18n.T("setup.title"))
	escHint := StyleSubtle.Render("[esc] " + i18n.T("setup.back"))
	padding := m.width - lipgloss.Width(title) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(title + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(title + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	// Show current key if set
	if m.currentKey != "" {
		b.WriteString("  " + i18n.TF("setup.current_key", m.currentKey) + "\n\n")
	}

	// Input
	b.WriteString("  " + i18n.T("setup.api_key_label") + " " + m.input.View() + "\n\n")

	// Error
	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
		b.WriteString("  " + errStyle.Render(i18n.TF("setup.error", m.err.Error())) + "\n\n")
	}

	// Hints
	if m.currentKey != "" {
		b.WriteString(StyleSubtle.Render("  [Enter] " + i18n.T("setup.keep_or_save") + "    [Esc] " + i18n.T("setup.back")) + "\n")
	} else {
		b.WriteString(StyleSubtle.Render("  [Enter] " + i18n.T("setup.save") + "    [Esc] " + i18n.T("setup.back")) + "\n")
	}

	return b.String()
}

func (m SetupModel) Done() bool { return m.done }
