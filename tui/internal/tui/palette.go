package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/charmbracelet/x/ansi"
)

// PaletteSelectMsg is sent when the user selects a command from the palette.
type PaletteSelectMsg struct {
	Command string
	Args    string // optional argument (e.g. "/rename foo" → Args="foo")
}

// Command represents a slash command in the palette.
type Command struct {
	Name        string // e.g. "manage"
	Description string // i18n key for short description (shown in palette)
	Detail      string // i18n key for detailed description (shown in greeting, commands.json)
}

// PaletteModel is the command palette overlay.
type PaletteModel struct {
	commands    []Command
	filtered    []Command
	cursor      int
	filter      string
	width       int
	maxHeight   int
	windowStart int
	bounded     bool
}

func NewPaletteModel() PaletteModel {
	cmds := DefaultCommands()
	return PaletteModel{
		commands:  cmds,
		filtered:  cmds,
		maxHeight: len(cmds) + 2,
	}
}

// SetSize bounds the palette to the already-budgeted Mail child width and the
// maximum total palette height, including its two border rows.
func (m *PaletteModel) SetSize(width, maxHeight int) {
	m.width = max(0, width)
	m.maxHeight = max(0, maxHeight)
	m.bounded = true
	m.clampWindow()
}

func (m PaletteModel) visibleCapacity() int {
	capacity := m.maxHeight - 2
	if capacity < 0 {
		return 0
	}
	return min(capacity, len(m.filtered))
}

func (m *PaletteModel) clampWindow() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.windowStart = 0
		return
	}
	m.cursor = min(max(m.cursor, 0), len(m.filtered)-1)
	capacity := m.visibleCapacity()
	if capacity == 0 {
		m.windowStart = 0
		return
	}
	maxStart := len(m.filtered) - capacity
	m.windowStart = min(max(m.windowStart, 0), maxStart)
	if m.cursor < m.windowStart {
		m.windowStart = m.cursor
	} else if m.cursor >= m.windowStart+capacity {
		m.windowStart = m.cursor - capacity + 1
	}
}

func (m PaletteModel) visibleWindow() (int, int) {
	capacity := m.visibleCapacity()
	if capacity == 0 {
		return 0, 0
	}
	start := min(max(m.windowStart, 0), len(m.filtered)-capacity)
	return start, start + capacity
}

func (m PaletteModel) canRender() bool {
	if len(m.filtered) == 0 || m.visibleCapacity() == 0 {
		return false
	}
	return !m.bounded || m.width >= 4
}

// DefaultCommands returns all slash commands.
func DefaultCommands() []Command {
	return []Command{
		{Name: "btw", Description: "palette.btw", Detail: "cmd.btw"},
		{Name: "sleep", Description: "palette.sleep", Detail: "cmd.sleep"},
		{Name: "suspend", Description: "palette.suspend", Detail: "cmd.suspend"},
		{Name: "cpr", Description: "palette.cpr", Detail: "cmd.cpr"},
		{Name: "clear", Description: "palette.clear", Detail: "cmd.clear"},
		{Name: "refresh", Description: "palette.refresh", Detail: "cmd.refresh"},
		{Name: "doctor", Description: "palette.doctor", Detail: "cmd.doctor"},
		{Name: "update", Description: "palette.update", Detail: "cmd.update"},
		{Name: "update-tui", Description: "palette.update_tui", Detail: "cmd.update_tui"},
		{Name: "mcp", Description: "palette.mcp", Detail: "cmd.mcp"},
		{Name: "setup", Description: "palette.setup", Detail: "cmd.setup"},
		{Name: "settings", Description: "palette.settings", Detail: "cmd.settings"},
		{Name: "kanban", Description: "palette.kanban", Detail: "cmd.kanban"},
		{Name: "daemons", Description: "palette.daemons", Detail: "cmd.daemons"},
		{Name: "notification", Description: "palette.notification", Detail: "cmd.notification"},
		{Name: "taskcard", Description: "palette.taskcard", Detail: "cmd.taskcard"},
		{Name: "goal", Description: "palette.goal", Detail: "cmd.goal"},
		{Name: "projects", Description: "palette.projects", Detail: "cmd.projects"},
		{Name: "agents", Description: "palette.agents", Detail: "cmd.agents"},
		{Name: "export", Description: "palette.export", Detail: "cmd.export"},
		{Name: "skills", Description: "palette.skills", Detail: "cmd.skills"},
		{Name: "knowledge", Description: "palette.knowledge", Detail: "cmd.knowledge"},
		{Name: "insights", Description: "palette.insights", Detail: "cmd.insights"},
		{Name: "system", Description: "palette.system", Detail: "cmd.system"},
		{Name: "mailbox", Description: "palette.mailbox", Detail: "cmd.mailbox"},
		{Name: "presets", Description: "palette.presets", Detail: "cmd.presets"},
		{Name: "molt", Description: "palette.molt", Detail: "cmd.molt"},
		{Name: "nirvana", Description: "palette.nirvana", Detail: "cmd.nirvana"},
		{Name: "login", Description: "palette.login", Detail: "cmd.login"},
		{Name: "help", Description: "palette.help", Detail: "cmd.help"},
		{Name: "quit", Description: "palette.quit", Detail: "cmd.quit"},
	}
}

// ExportCommandsJSON writes ~/.lingtai-tui/commands.json with all slash
// commands and their descriptions resolved in every locale.
func ExportCommandsJSON(globalDir string) {
	type cmdEntry struct {
		Name   string            `json:"name"`
		Brief  map[string]string `json:"brief"`
		Detail map[string]string `json:"detail"`
	}

	langs := []string{"en", "zh", "wen"}
	cmds := DefaultCommands()
	entries := make([]cmdEntry, 0, len(cmds))

	for _, cmd := range cmds {
		briefs := make(map[string]string, len(langs))
		details := make(map[string]string, len(langs))
		for _, lang := range langs {
			briefs[lang] = i18n.TIn(lang, cmd.Description)
			details[lang] = i18n.TIn(lang, cmd.Detail)
		}
		entries = append(entries, cmdEntry{Name: cmd.Name, Brief: briefs, Detail: details})
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(globalDir, "commands.json"), data, 0o644)
}

// ExcludeCommands removes the named commands from the palette.
func (m *PaletteModel) ExcludeCommands(names ...string) {
	exclude := make(map[string]bool, len(names))
	for _, n := range names {
		exclude[n] = true
	}
	filtered := make([]Command, 0, len(m.commands))
	for _, c := range m.commands {
		if !exclude[c.Name] {
			filtered = append(filtered, c)
		}
	}
	m.commands = filtered
	m.filtered = filtered
	m.cursor = 0
	m.windowStart = 0
	m.clampWindow()
}

func (m PaletteModel) Init() tea.Cmd { return nil }

func (m PaletteModel) Update(msg tea.Msg) (PaletteModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			m.clampWindow()
			return m, nil
		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			m.clampWindow()
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
		m.windowStart = 0
		m.clampWindow()
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
	m.windowStart = 0
	m.clampWindow()
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
	if !m.canRender() {
		return 0
	}
	start, end := m.visibleWindow()
	return end - start + 2 // border top + visible commands + border bottom
}

func (m PaletteModel) View() string {
	if !m.canRender() {
		return ""
	}

	var b strings.Builder
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSubtle).
		Padding(0, 1)

	start, end := m.visibleWindow()
	for idx := start; idx < end; idx++ {
		cmd := m.filtered[idx]
		cursor := "  "
		if idx == m.cursor {
			cursor = "> "
		}
		name := "/" + cmd.Name
		desc := i18n.T(cmd.Description)
		line := cursor + lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render(padRightCells(name, 12)) + StyleSubtle.Render(desc)
		if m.bounded {
			line = fitPaletteLine(line, m.width-4)
		}
		b.WriteString(line)
		if idx < end-1 {
			b.WriteString("\n")
		}
	}

	return border.Render(b.String())
}

func padRightCells(s string, width int) string {
	stringWidth := lipgloss.Width(s)
	if stringWidth >= width {
		return s + " "
	}
	return s + strings.Repeat(" ", width-stringWidth)
}

func fitPaletteLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = ansi.Truncate(line, width, "")
	if padding := width - lipgloss.Width(line); padding > 0 {
		line += strings.Repeat(" ", padding)
	}
	return line
}
