package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
)

// SendMsg is emitted when the user presses Enter in the input box.
type SendMsg struct{}

// OpenEditorMsg is emitted when user presses Ctrl+E to open external editor.
type OpenEditorMsg struct {
	Text string
}

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
	// Enter is reserved for sending; shift+enter and ctrl+j insert newlines.
	ti.KeyMap.InsertNewline.SetKeys("shift+enter", "ctrl+j")
	ti.SetWidth(80)
	ti.ShowLineNumbers = false
	ti.SetStyles(themedTextareaStyles())

	// Let the textarea soft-wrap and auto-grow instead of inserting hard newlines.
	ti.DynamicHeight = true
	ti.MinHeight = 1
	ti.MaxHeight = 6

	m := InputModel{
		textarea:   ti,
		historyIdx: -1,
		humanDir:   humanDir,
	}
	m.loadHistory()
	return m
}

// themedTextareaStyles builds textarea.Styles from the active theme colors.
func themedTextareaStyles() textarea.Styles {
	var s textarea.Styles
	s.Focused = textarea.StyleState{
		Base:        lipgloss.NewStyle().Foreground(ColorText),
		CursorLine:  lipgloss.NewStyle(),
		Placeholder: lipgloss.NewStyle().Foreground(ColorTextDim),
		Prompt:      lipgloss.NewStyle().Foreground(ColorAccent),
		Text:        lipgloss.NewStyle().Foreground(ColorText),
	}
	s.Blurred = textarea.StyleState{
		Base:        lipgloss.NewStyle().Foreground(ColorTextDim),
		CursorLine:  lipgloss.NewStyle(),
		Placeholder: lipgloss.NewStyle().Foreground(ColorTextFaint),
		Prompt:      lipgloss.NewStyle().Foreground(ColorTextDim),
		Text:        lipgloss.NewStyle().Foreground(ColorTextDim),
	}
	s.Cursor = textarea.CursorStyle{
		Color: ColorCursor,
		Blink: true,
	}
	return s
}

// ApplyTheme updates the textarea styles to match the current theme.
func (m *InputModel) ApplyTheme() {
	m.textarea.SetStyles(themedTextareaStyles())
}

func (m InputModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
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
			// Cursor not on first visual line: let textarea handle cursor movement
			li := m.textarea.LineInfo()
			if m.textarea.Line() > 0 || li.RowOffset > 0 {
				break // fall through to textarea
			}
			// On first visual line: navigate history
			if len(m.history) > 0 && m.historyIdx < len(m.history)-1 {
				m.historyIdx++
				m.textarea.SetValue(m.history[len(m.history)-1-m.historyIdx])
				m.textarea.CursorEnd()
			}
			return m, nil
		case "down":
			// Cursor not on last visual line: let textarea handle cursor movement
			li := m.textarea.LineInfo()
			lastLogicalLine := m.textarea.Line() >= m.textarea.LineCount()-1
			lastVisualRow := li.RowOffset >= li.Height-1
			if !(lastLogicalLine && lastVisualRow) {
				break // fall through to textarea
			}
			// On last visual line: navigate history
			if m.historyIdx > 0 {
				m.historyIdx--
				m.textarea.SetValue(m.history[len(m.history)-1-m.historyIdx])
				m.textarea.CursorEnd()
			} else if m.historyIdx == 0 {
				m.historyIdx = -1
				m.textarea.SetValue("")
			}
			return m, nil
		case "ctrl+e":
			// Open external editor with current text
			return m, func() tea.Msg {
				return OpenEditorMsg{Text: m.textarea.Value()}
			}
		}
		// Forward to textarea for all other keys (including ctrl+j / shift+enter for newline)
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)

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
	// Bottom border — matches the top separator style in mail.go
	border := strings.Repeat("\u2500", m.width)
	return rendered + pad + hint + "\n" + border
}

// LineCount returns the number of display lines in the input.
// With DynamicHeight enabled, the textarea's Height() reflects the actual
// visual line count (clamped between MinHeight and MaxHeight).
func (m *InputModel) LineCount() int {
	return m.textarea.Height()
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
