package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	humanDir    string // .lingtai/human/ for persisting history

	// Simple input history (up/down arrows)
	history    []string
	historyIdx int
}

func NewInputModel(humanDir string) InputModel {
	ti := textarea.New()
	ti.Prompt = ""
	ti.Placeholder = i18n.T("mail.placeholder")
	ti.CharLimit = 5000
	// Enter is reserved for sending; Ctrl+J inserts newlines.
	ti.KeyMap.InsertNewline.SetKeys()
	ti.SetWidth(80)
	ti.SetHeight(1)
	ti.ShowLineNumbers = false

	m := InputModel{
		textarea:   ti,
		historyIdx: -1,
		humanDir:   humanDir,
	}
	m.loadHistory()
	return m
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
			// Multiline: let textarea handle cursor movement between lines
			if m.HasNewlines() {
				break // fall through to textarea
			}
			// Single-line or empty: navigate history
			if len(m.history) > 0 && m.historyIdx < len(m.history)-1 {
				m.historyIdx++
				m.textarea.SetValue(m.history[len(m.history)-1-m.historyIdx])
				m.textarea.CursorEnd()
				m.textarea.SetHeight(m.calcHeight())
			}
			return m, nil
		case "down":
			// Multiline: let textarea handle cursor movement between lines
			if m.HasNewlines() {
				break // fall through to textarea
			}
			// Single-line or empty: navigate history
			if m.historyIdx > 0 {
				m.historyIdx--
				m.textarea.SetValue(m.history[len(m.history)-1-m.historyIdx])
				m.textarea.CursorEnd()
				m.textarea.SetHeight(m.calcHeight())
			} else if m.historyIdx == 0 {
				m.historyIdx = -1
				m.textarea.SetValue("")
				m.textarea.SetHeight(1)
			}
			return m, nil
		}
		// Forward to textarea for all other keys (including ctrl+j for newline)
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		m.textarea.SetHeight(m.calcHeight())

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
	prefix := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("  > ")
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

// calcHeight returns the display height accounting for word-wrap.
func (m InputModel) calcHeight() int {
	val := m.textarea.Value()
	if val == "" {
		return 1
	}
	w := m.textarea.Width()
	if w <= 0 {
		w = 80
	}
	total := 0
	for _, line := range strings.Split(val, "\n") {
		// Each logical line takes at least 1 row, plus extra for wrapping
		lineLen := len([]rune(line))
		rows := 1
		if lineLen > w {
			rows = (lineLen + w - 1) / w
		}
		total += rows
	}
	if total > 6 {
		total = 6
	}
	if total < 1 {
		total = 1
	}
	return total
}

// LineCount returns the number of display lines in the input.
func (m InputModel) LineCount() int {
	return m.calcHeight()
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
		m.saveHistory()
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

func (m *InputModel) historyPath() string {
	return filepath.Join(m.humanDir, "history.json")
}

func (m *InputModel) loadHistory() {
	if m.humanDir == "" {
		return
	}
	data, err := os.ReadFile(m.historyPath())
	if err != nil {
		return
	}
	json.Unmarshal(data, &m.history)
}

func (m *InputModel) saveHistory() {
	if m.humanDir == "" {
		return
	}
	data, _ := json.Marshal(m.history)
	os.WriteFile(m.historyPath(), data, 0o644)
}
