package tui

import (
	"strings"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SendMsg is emitted when the user presses Enter in the input box.
type SendMsg struct{}

// InputModel wraps a textarea with slash-command palette detection.
// Enter sends the message (via SendMsg). Ctrl+J inserts a newline.
type InputModel struct {
	textarea    textarea.Model
	showPalette bool
	width       int

	// Simple input history (up/down arrows)
	history    []string
	historyIdx int
}

func NewInputModel() InputModel {
	ti := textarea.New()
	ti.Prompt = ""
	ti.Placeholder = i18n.T("mail.placeholder")
	ti.CharLimit = 5000
	// Enter is reserved for sending; Ctrl+J inserts newlines.
	ti.KeyMap.InsertNewline.SetKeys()
	ti.SetWidth(80)
	ti.SetHeight(1)
	ti.ShowLineNumbers = false

	return InputModel{
		textarea:   ti,
		historyIdx: -1,
	}
}

func (m InputModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.showPalette {
				m.showPalette = false
				m.textarea.SetValue("")
				return m, nil
			}
		case "enter":
			return m, func() tea.Msg { return SendMsg{} }
		case "up":
			if len(m.history) > 0 && m.historyIdx < len(m.history)-1 {
				m.historyIdx++
				m.textarea.SetValue(m.history[len(m.history)-1-m.historyIdx])
				m.textarea.CursorEnd()
			}
			return m, nil
		case "down":
			if m.historyIdx > 0 {
				m.historyIdx--
				m.textarea.SetValue(m.history[len(m.history)-1-m.historyIdx])
				m.textarea.CursorEnd()
			} else if m.historyIdx == 0 {
				m.historyIdx = -1
				m.textarea.SetValue("")
			}
			return m, nil
		}
		// Forward to textarea for all other keys (including ctrl+j for newline)
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)

		// Auto-grow height based on content
		lines := strings.Count(m.textarea.Value(), "\n") + 1
		if lines > 6 {
			lines = 6
		}
		if lines < 1 {
			lines = 1
		}
		m.textarea.SetHeight(lines)

		// After update, check if slash is first char → activate palette
		newVal := m.textarea.Value()
		if len(newVal) > 0 && newVal[0] == '/' {
			m.showPalette = true
		} else {
			m.showPalette = false
		}
		return m, cmd
	}

	// Forward all other messages to textarea (including cursor blink)
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m InputModel) View() string {
	hint := lipgloss.NewStyle().Foreground(ColorSubtle).Render("[/]")
	// Use textarea's own rendered view (handles cursor, wrapping, multiline)
	taView := m.textarea.View()
	// Prefix first line with "> ", indent continuations
	lines := strings.Split(taView, "\n")
	prefix := "  > "
	indent := "    "
	var b strings.Builder
	for i, line := range lines {
		if i == 0 {
			b.WriteString(prefix + line)
		} else {
			b.WriteString("\n" + indent + line)
		}
	}
	rendered := b.String()

	// Right-align the [/] hint on the first line
	firstLineWidth := lipgloss.Width(prefix) + lipgloss.Width(lines[0])
	pad := ""
	if m.width > firstLineWidth+lipgloss.Width(hint) {
		pad = strings.Repeat(" ", m.width-firstLineWidth-lipgloss.Width(hint))
	}
	return rendered + pad + hint
}

// LineCount returns the number of display lines in the input.
func (m InputModel) LineCount() int {
	lines := strings.Count(m.textarea.Value(), "\n") + 1
	if lines > 6 {
		lines = 6
	}
	if lines < 1 {
		lines = 1
	}
	return lines
}

func (m InputModel) Value() string {
	return m.textarea.Value()
}

// HasNewlines returns true if the current input contains newlines.
func (m InputModel) HasNewlines() bool {
	return strings.Contains(m.textarea.Value(), "\n")
}

func (m *InputModel) SetValue(s string) {
	m.textarea.SetValue(s)
	if len(s) > 0 && s[0] == '/' {
		m.showPalette = true
	} else {
		m.showPalette = false
	}
}

func (m *InputModel) Reset() {
	val := m.textarea.Value()
	if val != "" {
		m.history = append(m.history, val)
		if len(m.history) > 100 {
			m.history = m.history[len(m.history)-100:]
		}
	}
	m.historyIdx = -1
	m.textarea.Reset()
	m.textarea.SetHeight(1)
	m.showPalette = false
}

func (m *InputModel) Focus() tea.Cmd {
	return m.textarea.Focus()
}

func (m *InputModel) Blur() {
	m.textarea.Blur()
}

func (m InputModel) Focused() bool {
	return m.textarea.Focused()
}

func (m InputModel) IsPaletteActive() bool {
	return m.showPalette
}

func (m *InputModel) SetWidth(w int) {
	m.width = w
	// Leave room for "> " prefix + "[/]" hint
	if w > 10 {
		m.textarea.SetWidth(w - 10)
	}
}
