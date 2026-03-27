package tui

import (
	"strings"

	"github.com/anthropics/lingtai-tui/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PaletteSelectMsg is sent when the user selects a command from the palette.
type PaletteSelectMsg struct {
	Command string
	Args    string // optional argument (e.g. "/rename foo" → Args="foo")
}

// Command represents a slash command in the palette.
type Command struct {
	Name        string // e.g. "manage"
	Description string // i18n key for description
}

// PaletteModel is the command palette overlay.
type PaletteModel struct {
	commands []Command
	filtered []Command
	cursor   int
	filter   string
	width    int
}

func NewPaletteModel() PaletteModel {
	cmds := DefaultCommands()
	return PaletteModel{
		commands: cmds,
		filtered: cmds,
	}
}

// DefaultCommands returns all slash commands.
func DefaultCommands() []Command {
	return []Command{
		{Name: "sleep", Description: "palette.sleep"},
		{Name: "sleep-all", Description: "palette.sleep_all"},
		{Name: "suspend", Description: "palette.suspend"},
		{Name: "suspend-all", Description: "palette.suspend_all"},
		{Name: "cpr", Description: "palette.cpr"},
		{Name: "nickname", Description: "palette.nickname"},
		{Name: "rename", Description: "palette.rename"},
		{Name: "lang", Description: "palette.lang"},
		{Name: "refresh", Description: "palette.refresh"},
		{Name: "manage", Description: "palette.manage"},
		{Name: "viz", Description: "palette.viz"},
		{Name: "setup", Description: "palette.setup"},
		{Name: "settings", Description: "palette.settings"},
		{Name: "presets", Description: "palette.presets"},
		{Name: "help", Description: "palette.help"},
		{Name: "quit", Description: "palette.quit"},
	}
}

func (m PaletteModel) Init() tea.Cmd { return nil }

func (m PaletteModel) Update(msg tea.Msg) (PaletteModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil
		case "enter":
			if m.cursor < len(m.filtered) {
				cmd := m.filtered[m.cursor]
				return m, func() tea.Msg {
					return PaletteSelectMsg{Command: cmd.Name}
				}
			}
			return m, nil
		}
	}
	return m, nil
}

// SetFilter updates the filter string and refilters commands using fuzzy matching.
// filter should be the text after "/" (e.g., "man" from "/man").
func (m *PaletteModel) SetFilter(filter string) {
	m.filter = filter
	m.filtered = nil
	if filter == "" {
		m.filtered = m.commands
		m.cursor = 0
		return
	}

	filterLower := strings.ToLower(filter)
	for _, cmd := range m.commands {
		if fuzzyMatch(cmd.Name, filterLower) {
			m.filtered = append(m.filtered, cmd)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// fuzzyMatch checks for substring containment first, then character-sequence matching.
func fuzzyMatch(cmd, filter string) bool {
	cmdLower := strings.ToLower(cmd)
	if strings.Contains(cmdLower, filter) {
		return true
	}
	si := 0
	for _, c := range cmdLower {
		if si < len(filter) && c == rune(filter[si]) {
			si++
		}
	}
	return si == len(filter)
}

// LineCount returns the terminal lines the palette occupies (0 if empty).
func (m PaletteModel) LineCount() int {
	if len(m.filtered) == 0 {
		return 0
	}
	return len(m.filtered) + 2 // border top + commands + border bottom
}

func (m PaletteModel) View() string {
	if len(m.filtered) == 0 {
		return ""
	}

	var b strings.Builder
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSubtle).
		Padding(0, 1)

	for idx, cmd := range m.filtered {
		cursor := "  "
		if idx == m.cursor {
			cursor = "> "
		}
		name := "/" + cmd.Name
		desc := i18n.T(cmd.Description)
		line := cursor + lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render(padRight(name, 12)) + StyleSubtle.Render(desc)
		b.WriteString(line)
		if idx < len(m.filtered)-1 {
			b.WriteString("\n")
		}
	}

	return border.Render(b.String())
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s + " "
	}
	return s + strings.Repeat(" ", width-len(s))
}
