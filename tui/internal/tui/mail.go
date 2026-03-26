package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/fs"
)

// ChatMessage represents a single message in the chat stream.
type ChatMessage struct {
	From      string
	To        string
	Subject   string
	Body      string
	Timestamp string
	IsFromMe  bool   // human sent this
	Type      string // "mail", "thinking", "diary"
}

// ViewChangeMsg requests the app to switch views.
type ViewChangeMsg struct {
	View string
}

type mailRefreshMsg struct {
	messages []ChatMessage
	alive    bool
	state    string // active, idle, stuck, asleep, suspended, or ""
}
type tickMsg time.Time

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Every(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// MailModel is the main chat view — a single chronological stream.
// verboseLevel controls what events.jsonl entries are shown
type verboseLevel int

const (
	verboseOff      verboseLevel = iota // normal: mail only
	verboseThinking                     // ctrl+o: mail + thinking + diary
	verboseExtended                     // ctrl+e: everything (+ text_input, text_output, tool_call, tool_result)
)

type MailModel struct {
	humanDir     string
	humanAddr    string
	orchestrator string // 本我 directory path (full path under .lingtai/)
	orchAddr     string // 本我 address (from .agent.json)
	orchName     string // 本我 agent name
	baseDir      string // .lingtai/ directory
	verbose      verboseLevel
	messages     []ChatMessage
	viewport     viewport.Model
	input        InputModel
	palette      PaletteModel
	width        int
	height       int
	ready        bool
	pollRate     time.Duration // refresh interval
	orchAlive    bool
	orchState    string // agent state from .agent.json
}

func NewMailModel(humanDir, humanAddr, baseDir, orchDir, orchName string, pollRate int) MailModel {
	input := NewInputModel()
	input.textarea.Focus()
	palette := NewPaletteModel()
	// Resolve orchestrator address from .agent.json
	orchAddr := orchDir
	if orchDir != "" {
		if node, err := fs.ReadAgent(orchDir); err == nil && node.Address != "" {
			orchAddr = node.Address
		}
	}
	if pollRate <= 0 {
		pollRate = 1
	}
	return MailModel{
		humanDir:     humanDir,
		humanAddr:    humanAddr,
		baseDir:      baseDir,
		orchestrator: orchDir,
		orchAddr:     orchAddr,
		orchName:     orchName,
		input:        input,
		palette:      palette,
		pollRate:     time.Duration(pollRate) * time.Second,
	}
}

func (m MailModel) refreshMail() tea.Msg {
	var chatMsgs []ChatMessage

	// Read inbox (messages FROM 本我 to human)
	inbox, _ := fs.ReadInbox(m.humanDir)
	for _, msg := range inbox {
		parts := strings.Split(msg.From, "/")
		fromName := parts[len(parts)-1]
		chatMsgs = append(chatMsgs, ChatMessage{
			From:      fromName,
			To:        i18n.T("mail.you"),
			Subject:   msg.Subject,
			Body:      msg.Message,
			Timestamp: msg.ReceivedAt,
			IsFromMe:  false,
			Type:      "mail",
		})
	}

	// Read sent (messages FROM human to 本我)
	sent, _ := fs.ReadSent(m.humanDir)
	for _, msg := range sent {
		chatMsgs = append(chatMsgs, ChatMessage{
			From:      i18n.T("mail.you"),
			To:        m.orchName,
			Subject:   msg.Subject,
			Body:      msg.Message,
			Timestamp: msg.ReceivedAt,
			IsFromMe:  true,
			Type:      "mail",
		})
	}

	// If verbose, read events
	if m.verbose != verboseOff && m.orchestrator != "" {
		eventsPath := filepath.Join(m.orchestrator, "logs", "events.jsonl")
		extended := m.verbose == verboseExtended
		events := ReadEvents(eventsPath, extended)
		chatMsgs = append(chatMsgs, events...)
	}

	// Sort by timestamp
	sort.Slice(chatMsgs, func(i, j int) bool {
		return chatMsgs[i].Timestamp < chatMsgs[j].Timestamp
	})

	alive := m.orchestrator != "" && fs.IsAlive(m.orchestrator, 3.0)
	state := ""
	if m.orchestrator != "" {
		if node, err := fs.ReadAgent(m.orchestrator); err == nil {
			state = node.State
		}
	}
	if !alive {
		state = "suspended"
	}
	return mailRefreshMsg{messages: chatMsgs, alive: alive, state: state}
}

func (m MailModel) Init() tea.Cmd {
	return tea.Batch(
		m.input.Init(),
		m.refreshMail,
		tickEvery(m.pollRate),
	)
}

func (m MailModel) Update(msg tea.Msg) (MailModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.MouseMsg:
		// Only forward scroll wheel events to viewport
		if m.ready && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width)
		// Layout: header(2) + viewport + footer(sep + input(N lines) + status)
		inputHeight := 1
		if m.ready {
			inputHeight = m.input.LineCount()
		}
		footerHeight := 1 + inputHeight + 1 // sep + input + status
		vpHeight := msg.Height - 2 - footerHeight
		if vpHeight < 1 {
			vpHeight = 1
		}
		if !m.ready {
			m.viewport = viewport.New(msg.Width, vpHeight)
			m.viewport.SetContent(m.renderMessages())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = vpHeight
		}
		return m, nil

	case mailRefreshMsg:
		prevCount := len(m.messages)
		m.messages = msg.messages
		m.orchAlive = msg.alive
		m.orchState = msg.state
		if m.ready {
			atBottom := m.viewport.AtBottom()
			m.viewport.SetContent(m.renderMessages())
			// Scroll to bottom if: was already at bottom, or message count changed significantly
			// (verbose toggle causes big jumps in message count)
			if atBottom || len(m.messages) != prevCount {
				m.viewport.GotoBottom()
			}
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refreshMail, tickEvery(m.pollRate))

	case SendMsg:
		text := m.input.Value()
		if text != "" && m.orchestrator != "" {
			fs.WriteMail(m.orchestrator, m.humanDir, m.humanAddr, m.orchAddr, "", text)
			m.input.Reset()
			return m, m.refreshMail
		}
		return m, nil

	case PaletteSelectMsg:
		m.input.Reset()
		// Forward to app
		return m, func() tea.Msg { return PaletteSelectMsg{Command: msg.Command} }

	case tea.KeyMsg:
		// If palette is active, route to palette
		if m.input.IsPaletteActive() {
			switch msg.String() {
			case "enter", "up", "down":
				var cmd tea.Cmd
				m.palette, cmd = m.palette.Update(msg)
				return m, cmd
			case "esc":
				m.input.Reset()
				return m, nil
			default:
				// Forward typing to input, then update palette filter
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				// Extract filter from input (text after "/")
				val := m.input.Value()
				if len(val) > 1 {
					m.palette.SetFilter(val[1:])
				} else {
					m.palette.SetFilter("")
				}
				return m, cmd
			}
		}

		switch msg.String() {
		case "ctrl+o":
			// Toggle: off → thinking → off
			if m.verbose == verboseThinking {
				m.verbose = verboseOff
			} else {
				m.verbose = verboseThinking
			}
			return m, m.refreshMail

		case "ctrl+e":
			// Toggle: off → extended → off
			if m.verbose == verboseExtended {
				m.verbose = verboseOff
			} else {
				m.verbose = verboseExtended
			}
			return m, m.refreshMail

		case "pgup", "pgdown", "up", "down":
			// Scroll viewport — arrow keys scroll when input is empty
			if msg.String() == "up" || msg.String() == "down" {
				if m.input.Value() != "" {
					break // let it fall through to input handler
				}
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		// If input is focused, forward keys to input
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		// Check if slash was typed
		if m.input.IsPaletteActive() {
			val := m.input.Value()
			if len(val) > 1 {
				m.palette.SetFilter(val[1:])
			} else {
				m.palette.SetFilter("")
			}
		}
		return m, cmd
	}

	// Forward all other messages (including textinput blink) to input
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m MailModel) renderMessages() string {
	if len(m.messages) == 0 {
		return "\n" + StyleSubtle.Render("  "+i18n.T("mail.no_messages"))
	}

	var b strings.Builder
	for _, msg := range m.messages {
		switch msg.Type {
		case "thinking", "diary", "text_input", "text_output", "tool_call", "tool_result":
			// Event types — decreasing brightness by depth:
			//   thinking/diary = medium (#718096)
			//   tool/text = dimmest (#4a5568, same as ColorSubtle)
			wrapWidth := m.width - 6 // "  ┊ " prefix
			if wrapWidth < 20 {
				wrapWidth = 20
			}
			var eventStyle lipgloss.Style
			switch msg.Type {
			case "thinking", "diary":
				eventStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#718096"))
			default: // tool_call, tool_result, text_input, text_output
				eventStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4a5568"))
			}
			wrapped := lipgloss.NewStyle().Width(wrapWidth).Render("[" + msg.Type + "] " + msg.Body)
			for _, line := range strings.Split(wrapped, "\n") {
				b.WriteString(eventStyle.Render("  \u250a "+line) + "\n")
			}

		default: // "mail"
			if m.verbose != verboseOff {
				// Show headers
				header := StyleSubtle.Render(fmt.Sprintf("  \u250a %s \u2192 %s", msg.From, msg.To))
				if msg.Subject != "" {
					header += StyleSubtle.Render(fmt.Sprintf(" | %s %s", i18n.T("mail.subject_label"), msg.Subject))
				}
				header += StyleSubtle.Render(fmt.Sprintf(" | %s", msg.Timestamp))
				b.WriteString(header + "\n")
			}

			nameStyle := lipgloss.NewStyle().Foreground(ColorActive).Bold(true)
			if msg.IsFromMe {
				nameStyle = lipgloss.NewStyle().Foreground(ColorMail).Bold(true)
			}
			name := nameStyle.Render(msg.From)
			// Short timestamp (HH:MM)
			ts := ""
			if msg.Timestamp != "" {
				if t, err := time.Parse(time.RFC3339Nano, msg.Timestamp); err == nil {
					ts = StyleSubtle.Render(" " + t.Local().Format("2006-01-02-15:04:05"))
				}
			}
			// Wrap body to fit terminal width (indent 2 + name + ": ")
			prefix := fmt.Sprintf("  %s%s: ", name, ts)
			prefixWidth := lipgloss.Width(prefix)
			bodyWidth := m.width - prefixWidth
			if bodyWidth < 20 {
				bodyWidth = 20
			}
			wrappedBody := lipgloss.NewStyle().Width(bodyWidth).Render(msg.Body)
			// Indent continuation lines to align with first line
			lines := strings.Split(wrappedBody, "\n")
			b.WriteString("\n" + prefix + lines[0] + "\n")
			indent := strings.Repeat(" ", prefixWidth)
			for _, line := range lines[1:] {
				b.WriteString(indent + line + "\n")
			}
		}
	}
	return b.String()
}

func (m MailModel) View() string {
	if !m.ready {
		return "\n  " + i18n.T("app.loading")
	}

	// Build header: left = app title, right = agent [state]
	titleLeft := StyleTitle.Render("  " + i18n.T("app.brand"))

	// State badge with color
	stateKey := m.orchState
	if stateKey == "" {
		stateKey = "unknown"
	}
	stateLabel := i18n.T("state." + stateKey)
	var stateStyle lipgloss.Style
	switch stateKey {
	case "active":
		stateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#48bb78"))
	case "idle":
		stateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4299e1"))
	case "asleep":
		stateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ecc94b"))
	case "stuck":
		stateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ed8936"))
	case "suspended":
		stateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f56565"))
	default:
		stateStyle = StyleSubtle
	}
	titleRight := StyleTitle.Render(m.orchName) + " " + stateStyle.Render("["+stateLabel+"]")

	padding := m.width - lipgloss.Width(titleLeft) - lipgloss.Width(titleRight) - 1
	var titleLine string
	if padding > 0 {
		titleLine = titleLeft + strings.Repeat(" ", padding) + titleRight
	} else {
		titleLine = titleLeft + "  " + titleRight
	}
	header := titleLine + "\n" + strings.Repeat("\u2500", m.width)

	// Build footer
	sep := strings.Repeat("\u2500", m.width)
	var inputSection string
	if m.input.IsPaletteActive() {
		inputSection = m.palette.View() + "\n" + m.input.View()
	} else {
		inputSection = m.input.View()
	}

	// Status bar: .lingtai path on left, hints on right
	dirLabel := StyleSubtle.Render("  " + m.baseDir)
	var hints string
	switch m.verbose {
	case verboseOff:
		hints = StyleSubtle.Render(i18n.T("hints.verbose") + "  " + i18n.T("hints.extended") + "  " + i18n.T("hints.commands"))
	case verboseThinking:
		hints = lipgloss.NewStyle().Foreground(ColorActive).Render(i18n.T("hints.verbose_off")) +
			StyleSubtle.Render("  " + i18n.T("hints.extended") + "  " + i18n.T("hints.commands"))
	case verboseExtended:
		hints = StyleSubtle.Render(i18n.T("hints.verbose") + "  ") +
			lipgloss.NewStyle().Foreground(lipgloss.Color("#718096")).Render(i18n.T("hints.extended_off")) +
			StyleSubtle.Render("  " + i18n.T("hints.commands"))
	}
	if m.input.HasNewlines() {
		newlineHint := lipgloss.NewStyle().Foreground(lipgloss.Color("#718096")).Render("  " + i18n.T("hints.newline"))
		hints += newlineHint
	}
	statusPad := m.width - lipgloss.Width(dirLabel) - lipgloss.Width(hints) - 1
	statusBar := dirLabel
	if statusPad > 0 {
		statusBar += strings.Repeat(" ", statusPad) + hints
	}

	footer := sep + "\n" + inputSection + "\n" + statusBar

	// Viewport fills the middle
	return header + "\n" + m.viewport.View() + "\n" + footer
}
