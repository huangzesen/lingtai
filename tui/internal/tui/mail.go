package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/fs"
)

// ChatMessage represents a single message in the chat stream.
type ChatMessage struct {
	From        string
	To          string
	Subject     string
	Body        string
	Timestamp   string
	IsFromMe    bool     // human sent this
	Type        string   // "mail", "thinking", "diary"
	Attachments []string // file paths attached to the message
}

// ViewChangeMsg requests the app to switch views.
type ViewChangeMsg struct {
	View string
}

type mailRefreshMsg struct {
	messages []ChatMessage
	alive    bool
	state    string // active, idle, stuck, asleep, suspended, or ""
	orchName string // agent name from .agent.json (may change at runtime)
}
type tickMsg time.Time

// EditorDoneMsg carries the final text from the external editor.
type EditorDoneMsg struct {
	Text string
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Every(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// MailModel is the main chat view — a single chronological stream.
// verboseLevel controls what events.jsonl entries are shown
type verboseLevel int

const (
	verboseOff      verboseLevel = iota // normal: mail only
	verboseThinking                     // ctrl+o cycle: mail + thinking + diary
	verboseExtended                     // ctrl+o cycle: everything (+ text_input, text_output, tool_call, tool_result)
)

type MailModel struct {
	humanDir         string
	humanAddr        string
	orchestrator     string // 本我 directory path (full path under .lingtai/)
	orchAddr         string // 本我 address (from .agent.json)
	orchName         string // 本我 agent name
	baseDir          string // .lingtai/ directory
	verbose          verboseLevel
	messages         []ChatMessage
	viewport         viewport.Model
	input            InputModel
	palette          PaletteModel
	width            int
	height           int
	ready            bool
	pollRate         time.Duration // refresh interval
	orchAlive        bool
	orchState        string    // agent state from .agent.json
	statusFlash      string    // transient status message shown in status bar
	statusExpiry     time.Time // when to clear the flash
	lastInputLines   int
	lastPaletteLines int
	pendingMessage   string // full text from editor, sent on Enter
}

func NewMailModel(humanDir, humanAddr, baseDir, orchDir, orchName string, pollRate int) MailModel {
	input := NewInputModel(humanDir)
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

// syncViewportHeight recalculates viewport height from current input/palette size.
// Returns true if the height actually changed.
func (m *MailModel) syncViewportHeight() bool {
	if !m.ready {
		return false
	}
	inputLines := m.input.LineCount()
	paletteLines := 0
	if m.input.IsPaletteActive() {
		paletteLines = m.palette.LineCount()
	}
	if inputLines == m.lastInputLines && paletteLines == m.lastPaletteLines {
		return false
	}
	m.lastInputLines = inputLines
	m.lastPaletteLines = paletteLines
	// Layout: header(2) + viewport + sep(1) + palette(N) + input(N) + border(1) + status(1)
	footerHeight := 1 + paletteLines + inputLines + 1 + 1
	vpHeight := m.height - 2 - footerHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.SetHeight(vpHeight)
	return true
}

func (m MailModel) refreshMail() tea.Msg {
	// Refresh human location (no-op if cache is <1h old)
	go fs.UpdateHumanLocation(m.humanDir)

	var chatMsgs []ChatMessage

	// Read inbox (messages FROM 本我 to human)
	inbox, _ := fs.ReadInbox(m.humanDir)
	for _, msg := range inbox {
		parts := strings.Split(msg.From, "/")
		fromName := parts[len(parts)-1]
		chatMsgs = append(chatMsgs, ChatMessage{
			From:        fromName,
			To:          m.humanName(),
			Subject:     msg.Subject,
			Body:        msg.Message,
			Timestamp:   msg.ReceivedAt,
			IsFromMe:    false,
			Type:        "mail",
			Attachments: msg.Attachments,
		})
	}

	// Read sent (messages FROM human to 本我)
	sent, _ := fs.ReadSent(m.humanDir)
	for _, msg := range sent {
		chatMsgs = append(chatMsgs, ChatMessage{
			From:        m.humanName(),
			To:          m.orchName,
			Subject:     msg.Subject,
			Body:        msg.Message,
			Timestamp:   msg.ReceivedAt,
			IsFromMe:    true,
			Type:        "mail",
			Attachments: msg.Attachments,
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
	orchName := m.orchName
	if m.orchestrator != "" {
		if node, err := fs.ReadAgent(m.orchestrator); err == nil {
			state = node.State
			if node.AgentName != "" {
				orchName = node.AgentName
			}
		}
	}
	if !alive {
		state = "suspended"
	}
	return mailRefreshMsg{messages: chatMsgs, alive: alive, state: state, orchName: orchName}
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
	case tea.MouseWheelMsg:
		// Forward scroll wheel events to viewport
		if m.ready {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width)
		if !m.ready {
			inputLines := m.input.LineCount()
			// sep(1) + input(N) + border(1) + status(1)
			footerHeight := 1 + inputLines + 1 + 1
			vpHeight := msg.Height - 2 - footerHeight
			if vpHeight < 1 {
				vpHeight = 1
			}
			m.viewport = viewport.New()
			m.viewport.SetWidth(msg.Width)
			m.viewport.SetHeight(vpHeight)
			m.viewport.SetContent(m.renderMessages())
			m.lastInputLines = inputLines
			m.ready = true
		} else {
			m.viewport.SetWidth(msg.Width)
			m.lastInputLines = -1 // force recalculate
			m.syncViewportHeight()
		}
		return m, nil

	case mailRefreshMsg:
		m.messages = msg.messages
		m.orchAlive = msg.alive
		m.orchState = msg.state
		if msg.orchName != "" {
			m.orchName = msg.orchName
		}
		if m.ready {
			atBottom := m.viewport.AtBottom()
			m.viewport.SetContent(m.renderMessages())
			if atBottom {
				m.viewport.GotoBottom()
			}
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refreshMail, tickEvery(m.pollRate))

	case SendMsg:
		var text string
		if m.pendingMessage != "" {
			text = m.pendingMessage
			m.pendingMessage = ""
		} else {
			text = m.input.Value()
		}
		if text == "" {
			return m, nil
		}
		// If text starts with /, treat as slash command
		if len(text) > 1 && text[0] == '/' {
			parts := strings.SplitN(text[1:], " ", 2)
			cmd := parts[0]
			args := ""
			if len(parts) > 1 {
				args = strings.TrimSpace(parts[1])
			}
			m.input.Reset()
			m.syncViewportHeight()
			return m, func() tea.Msg { return PaletteSelectMsg{Command: cmd, Args: args} }
		}
		if m.orchestrator != "" {
			fs.WriteMail(m.orchestrator, m.humanDir, m.humanAddr, m.orchAddr, "", text)
			m.input.Reset()
			m.syncViewportHeight()
			return m, m.refreshMail
		}
		return m, nil

	case OpenEditorMsg:
		// Open external editor with current text
		tmpFile, err := os.CreateTemp("", "lingtai-input-*.txt")
		if err != nil {
			return m, nil
		}
		tmpFile.WriteString(msg.Text)
		tmpFile.Close()
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}
		cmd := exec.Command(editor, tmpFile.Name())
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				os.Remove(tmpFile.Name())
				return nil
			}
			content, _ := os.ReadFile(tmpFile.Name())
			os.Remove(tmpFile.Name())
			return EditorDoneMsg{Text: string(content)}
		})

	case EditorDoneMsg:
		m.pendingMessage = msg.Text
		firstLine := strings.SplitAfterN(msg.Text, "\n", 2)[0]
		m.input.SetValue(firstLine)
		// Refresh viewport after external editor
		return m, m.refreshMail

	case PaletteSelectMsg:
		m.input.Reset()
		m.syncViewportHeight()
		// Forward to app
		return m, func() tea.Msg { return PaletteSelectMsg{Command: msg.Command} }

	case tea.KeyPressMsg:
		// If palette is active, route to palette
		if m.input.IsPaletteActive() {
			switch msg.String() {
			case "enter":
				// If input has args (space after /cmd), parse as command+args
				val := m.input.Value()
				if strings.Contains(val, " ") {
					parts := strings.SplitN(val[1:], " ", 2)
					cmd := parts[0]
					args := ""
					if len(parts) > 1 {
						args = strings.TrimSpace(parts[1])
					}
					m.input.Reset()
					return m, func() tea.Msg { return PaletteSelectMsg{Command: cmd, Args: args} }
				}
				// No args — select from palette
				m.input.Reset()
				m.syncViewportHeight()
				var cmd tea.Cmd
				m.palette, cmd = m.palette.Update(msg)
				return m, cmd
			case "up", "down":
				var cmd tea.Cmd
				m.palette, cmd = m.palette.Update(msg)
				return m, cmd
			case "esc":
				m.input.Reset()
				m.syncViewportHeight()
				return m, nil
			default:
				// Forward typing to input, then update palette filter
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				m.syncViewportHeight()
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
		case "ctrl+p":
			return m, func() tea.Msg { return ViewChangeMsg{View: "props"} }

		case "ctrl+o":
			// Cycle: normal → thinking → extended → normal
			switch m.verbose {
			case verboseOff:
				m.verbose = verboseThinking
			case verboseThinking:
				m.verbose = verboseExtended
			case verboseExtended:
				m.verbose = verboseOff
			}
			return m, m.refreshMail

		case "pgup", "pgdown":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		// If input is focused, forward keys to input
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.syncViewportHeight() && m.viewport.AtBottom() {
			m.viewport.GotoBottom()
		}
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
		return "\n" + StyleFaint.Render("  "+RuneBullet+" "+i18n.T("mail.no_messages"))
	}

	humanStyle := lipgloss.NewStyle().Foreground(ColorHuman).Bold(true)
	agentStyle := lipgloss.NewStyle().Foreground(ColorAgent).Bold(true)
	systemStyle := lipgloss.NewStyle().Foreground(ColorSystem).Bold(true)
	thinkingStyle := lipgloss.NewStyle().Foreground(ColorThinking)
	toolStyle := lipgloss.NewStyle().Foreground(ColorTool)
	sepStyle := lipgloss.NewStyle().Foreground(ColorTextDim)

	var b strings.Builder
	for _, msg := range m.messages {
		switch msg.Type {
		case "thinking", "diary", "text_input", "text_output", "tool_call", "tool_result":
			wrapWidth := m.width - 6
			if wrapWidth < 20 {
				wrapWidth = 20
			}
			var evStyle lipgloss.Style
			switch msg.Type {
			case "thinking", "diary":
				evStyle = thinkingStyle
			default:
				evStyle = toolStyle
			}
			wrapped := lipgloss.NewStyle().Width(wrapWidth).Render("[" + msg.Type + "] " + msg.Body)
			for _, line := range strings.Split(wrapped, "\n") {
				b.WriteString(evStyle.Render("  "+RuneBullet+" "+line) + "\n")
			}

		default: // "mail"
			if m.verbose != verboseOff {
				header := StyleFaint.Render("  "+RuneBullet+" ") +
					humanStyle.Render(msg.From) + sepStyle.Render(" → ") + sepStyle.Render(msg.To)
				if msg.Subject != "" {
					header += sepStyle.Render(" │ " + i18n.T("mail.subject_label") + " " + msg.Subject)
				}
				header += sepStyle.Render(" │ " + msg.Timestamp)
				b.WriteString(header + "\n")
			}

			var nameStyle lipgloss.Style
			if msg.IsFromMe {
				nameStyle = humanStyle
			} else if msg.From == i18n.T("mail.system_sender") {
				nameStyle = systemStyle
			} else {
				nameStyle = agentStyle
			}
			name := nameStyle.Render(msg.From)
			// Short timestamp (HH:MM)
			ts := ""
			if msg.Timestamp != "" {
				if t, err := time.Parse(time.RFC3339Nano, msg.Timestamp); err == nil {
					ts = StyleFaint.Render(" " + t.Local().Format("15:04"))
				}
			}
			// Wrap body to fit terminal width (indent 2 + name + ": ")
			prefix := fmt.Sprintf("  %s%s: ", name, ts)
			prefixWidth := lipgloss.Width(prefix)
			bodyWidth := m.width - prefixWidth
			if bodyWidth < 20 {
				bodyWidth = 20
			}
			// Render markdown for agent messages, plain wrap for user/system
			var wrappedBody string
			if !msg.IsFromMe && msg.From != i18n.T("mail.system_sender") {
				r, err := glamour.NewTermRenderer(
					glamour.WithStandardStyle("dark"),
					glamour.WithWordWrap(bodyWidth),
				)
				if err == nil {
					if rendered, rerr := r.Render(msg.Body); rerr == nil {
						wrappedBody = strings.TrimRight(rendered, "\n")
					}
				}
				if wrappedBody == "" {
					wrappedBody = lipgloss.NewStyle().Width(bodyWidth).Render(msg.Body)
				}
			} else {
				wrappedBody = lipgloss.NewStyle().Width(bodyWidth).Render(msg.Body)
			}
			// Indent continuation lines to align with first line
			lines := strings.Split(wrappedBody, "\n")
			b.WriteString("\n" + prefix + lines[0] + "\n")
			indent := strings.Repeat(" ", prefixWidth)
			for _, line := range lines[1:] {
				b.WriteString(indent + line + "\n")
			}
			// Show attachment paths if present
			if len(msg.Attachments) > 0 {
				b.WriteString(indent + StyleFaint.Render("Attachments:") + "\n")
				for i, att := range msg.Attachments {
					b.WriteString(indent + StyleFaint.Render(fmt.Sprintf("  [%d] %s", i+1, att)) + "\n")
				}
			}
		}
	}
	return b.String()
}

// humanName returns the human's display name. Prefers nickname from .agent.json,
// falls back to i18n "mail.you".
func (m MailModel) humanName() string {
	if node, err := fs.ReadAgent(m.humanDir); err == nil {
		if node.Nickname != "" {
			return node.Nickname
		}
	}
	return i18n.T("mail.you")
}

// AddSystemMessage shows a transient status message in the status bar.
// It auto-expires after 5 seconds.
func (m *MailModel) AddSystemMessage(body string) {
	m.statusFlash = body
	m.statusExpiry = time.Now().Add(5 * time.Second)
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
	stateStyle := lipgloss.NewStyle().Foreground(StateColor(strings.ToUpper(stateKey)))
	orchNameStyle := lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	titleRight := orchNameStyle.Render(m.orchName) + " " + stateStyle.Render("◉ "+stateLabel)

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

	// Status bar: left = flash or dir path, right = hints
	var leftLabel string
	if m.statusFlash != "" && time.Now().Before(m.statusExpiry) {
		leftLabel = lipgloss.NewStyle().Foreground(ColorAgent).Render("  ◉ " + m.statusFlash)
	} else {
		m.statusFlash = ""
		leftLabel = StyleSubtle.Render("  " + m.baseDir)
	}
	var hints string
	switch m.verbose {
	case verboseOff:
		hints = StyleFaint.Render(i18n.T("hints.verbose") + " " + RuneBullet + " " + i18n.T("hints.editor") + " " + RuneBullet + " " + i18n.T("hints.commands"))
	case verboseThinking:
		hints = lipgloss.NewStyle().Foreground(ColorAgent).Render(i18n.T("hints.verbose_on")) +
			StyleFaint.Render(" "+RuneBullet+" "+i18n.T("hints.editor")+" "+RuneBullet+" "+i18n.T("hints.commands"))
	case verboseExtended:
		hints = lipgloss.NewStyle().Foreground(ColorThinking).Render(i18n.T("hints.extended_on")) +
			StyleFaint.Render(" "+RuneBullet+" "+i18n.T("hints.editor")+" "+RuneBullet+" "+i18n.T("hints.commands"))
	}
	hints += StyleFaint.Render(" " + RuneBullet + " " + i18n.T("hints.props"))
	statusPad := m.width - lipgloss.Width(leftLabel) - lipgloss.Width(hints) - 1
	statusBar := leftLabel
	if statusPad > 0 {
		statusBar += strings.Repeat(" ", statusPad) + hints
	}

	footer := sep + "\n" + inputSection + "\n" + statusBar

	// Viewport fills the middle
	return header + "\n" + m.viewport.View() + "\n" + footer
}
