package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lingtai-daemon/internal/agent"
	"lingtai-daemon/internal/config"
	"lingtai-daemon/internal/i18n"
	"lingtai-daemon/internal/manage"
)

// mailReceivedMsg wraps a received TCP mail message.
type mailReceivedMsg map[string]interface{}

// verboseTickMsg triggers periodic JSONL re-read while verbose is on.
type verboseTickMsg time.Time

// tickMsg triggers periodic mail polling.
type tickMsg time.Time

// logEvent is a parsed JSONL event from the agent's log.
type logEvent struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	Sender   string      `json:"sender,omitempty"`
	Subject  string      `json:"subject,omitempty"`
	To       interface{} `json:"to,omitempty"`
	ToolName string      `json:"tool_name,omitempty"`
	Name     string      `json:"name,omitempty"`
}

// ChatExitMsg signals that the chat view wants to return to the parent.
type ChatExitMsg struct{}

// ChatModel is the chat TUI model.
type ChatModel struct {
	config   *config.Config
	proc     *agent.Process
	mail     *agent.MailClient
	listener *agent.MailListener
	mailCh   chan map[string]interface{}

	viewport viewport.Model
	input    textinput.Model
	messages []string

	// Verbose mode (Ctrl+O): on-demand JSONL rendering
	verbose            bool
	verboseOffset      int64 // byte offset into JSONL file — resume from here
	verboseStartIdx    int   // index in messages[] where verbose output begins

	// Daemon switching: which daemon the TUI talks to
	activeName string // agent name of current target
	activePort int    // TCP port of current target

	width  int
	height int
	ready  bool
}

// New creates a new TUI model.
func NewChat(cfg *config.Config, proc *agent.Process) ChatModel {
	ti := textinput.New()
	ti.Placeholder = i18n.S("type_message")
	ti.Focus()
	ti.CharLimit = 4096
	ti.Width = 80

	mailClient := agent.NewMailClient(fmt.Sprintf("127.0.0.1:%d", cfg.AgentPort))

	return ChatModel{
		config:     cfg,
		proc:       proc,
		mail:       mailClient,
		mailCh:     make(chan map[string]interface{}, 32),
		input:      ti,
		messages:   []string{},
		activeName: cfg.AgentName,
		activePort: cfg.AgentPort,
	}
}

func (m ChatModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.pollMailEvents(),
	)
}

func (m ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, func() tea.Msg { return ChatExitMsg{} }
		case tea.KeyEsc:
			return m, func() tea.Msg { return ChatExitMsg{} }
		case tea.KeyCtrlO:
			m.verbose = !m.verbose
			if m.verbose {
				m.verboseStartIdx = len(m.messages)
				m.readVerboseLines()
				cmds = append(cmds, m.verboseTick())
			} else {
				// Remove verbose lines
				if m.verboseStartIdx < len(m.messages) {
					m.messages = m.messages[:m.verboseStartIdx]
				}
				m.verboseOffset = 0
			}
			m.updateViewport()
		case tea.KeyTab:
			m.cycleNextSpirit()
			m.updateViewport()
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			if text != "" {
				m.input.SetValue("")
				if handled := m.handleCommand(text); handled {
					m.updateViewport()
				} else {
					m.messages = append(m.messages, InputPrompt.Render("> ")+text)
					m.updateViewport()

					go m.mail.Send(map[string]interface{}{
						"from":    fmt.Sprintf("cli@localhost:%d", m.config.CLIPort),
						"message": text,
					})
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 1
		inputHeight := 3
		vpHeight := m.height - headerHeight - inputHeight

		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true

			if m.config.CLIPort > 0 {
				mailCh := m.mailCh
				listener, err := agent.NewMailListener(m.config.CLIPort, func(msg map[string]interface{}) {
					mailCh <- msg
				})
				if err == nil {
					m.listener = listener
				}
			}

			m.messages = append(m.messages, AgentMsg.Render(fmt.Sprintf(
				"%s — %s (port %d, PID %d)",
				i18n.S("title"), m.config.AgentName, m.config.AgentPort, m.proc.PID(),
			)))
			m.updateViewport()
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
		}
		m.input.Width = m.width - 4

	case mailReceivedMsg:
		text, _ := msg["message"].(string)
		sender, _ := msg["from"].(string)
		if text != "" {
			line := EmailMsg.Render(fmt.Sprintf("[%s] %s", sender, text))
			m.messages = append(m.messages, line)
			m.updateViewport()
		}
		cmds = append(cmds, m.pollMailEvents())

	case verboseTickMsg:
		if m.verbose {
			m.readVerboseLines()
			m.updateViewport()
			cmds = append(cmds, m.verboseTick())
		}

	case tickMsg:
		cmds = append(cmds, m.pollMailEvents())
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m ChatModel) View() string {
	if !m.ready {
		return "\n  " + i18n.S("starting")
	}

	statusLeft := TitleStyle.Render(i18n.S("title")) + " " + ActiveChannel.Render(m.activeName)
	indicators := []string{}
	if m.config.IMAP != nil {
		indicators = append(indicators, ActiveChannel.Render("IMAP"))
	} else {
		indicators = append(indicators, DisabledChannel.Render("IMAP"))
	}
	if m.config.Telegram != nil {
		indicators = append(indicators, ActiveChannel.Render("TG"))
	} else {
		indicators = append(indicators, DisabledChannel.Render("TG"))
	}
	if m.config.CLI {
		indicators = append(indicators, ActiveChannel.Render("CLI"))
	}
	if m.verbose {
		indicators = append(indicators, ActiveChannel.Render("verbose ●"))
	}
	statusRight := strings.Join(indicators, " ")
	statusBar := StatusBarStyle.Width(m.width).Render(
		statusLeft + strings.Repeat(" ", max(0, m.width-lipgloss.Width(statusLeft)-lipgloss.Width(statusRight)-4)) + statusRight,
	)

	inputBox := InputPrompt.Render("❯ ") + m.input.View()

	return statusBar + "\n" + m.viewport.View() + "\n" + inputBox
}

func (m *ChatModel) updateViewport() {
	content := strings.Join(m.messages, "\n")
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m ChatModel) pollMailEvents() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-m.mailCh:
			return mailReceivedMsg(msg)
		case <-time.After(1 * time.Second):
			return tickMsg(time.Now())
		}
	}
}

func (m ChatModel) verboseTick() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return verboseTickMsg(t)
	})
}

// readVerboseLines reads new JSONL lines from the agent's log file starting
// at verboseOffset. This is an on-demand read — no background goroutine.
func (m *ChatModel) readVerboseLines() {
	logPath := fmt.Sprintf("%s/%s/logs/events.jsonl", m.config.ProjectDir, m.activeName)
	f, err := os.Open(logPath)
	if err != nil {
		return
	}
	defer f.Close()

	f.Seek(m.verboseOffset, 0)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var event logEvent
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Type != "" {
			line := formatLogEvent(event)
			if line != "" {
				m.messages = append(m.messages, line)
			}
		}
	}

	// Update offset to current position
	offset, err := f.Seek(0, 1) // current position
	if err == nil {
		m.verboseOffset = offset
	}
}

// handleCommand processes /commands. Returns true if the input was a command.
func (m *ChatModel) handleCommand(text string) bool {
	if strings.HasPrefix(text, "/list") {
		spirits := manage.ScanSpirits(m.config.ProjectDir)
		if len(spirits) == 0 {
			m.messages = append(m.messages, DiaryMsg.Render(i18n.S("no_spirits")))
		} else {
			for _, s := range spirits {
				status := "●"
				if !s.Alive {
					status = "✗"
				}
				marker := ""
				if s.Port == m.activePort {
					marker = " ← active"
				}
				m.messages = append(m.messages, DiaryMsg.Render(
					fmt.Sprintf("  %s %-16s port:%d pid:%d%s", status, s.Name, s.Port, s.PID, marker),
				))
			}
		}
		return true
	}

	if strings.HasPrefix(text, "/connect ") {
		target := strings.TrimSpace(strings.TrimPrefix(text, "/connect "))
		if target == "" {
			return false
		}
		// Try as port number first
		if port, err := strconv.Atoi(target); err == nil {
			m.switchDaemon(target, port)
			return true
		}
		// Try as agent name — look up in running spirits
		spirits := manage.ScanSpirits(m.config.ProjectDir)
		for _, s := range spirits {
			if s.Name == target {
				m.switchDaemon(s.Name, s.Port)
				return true
			}
		}
		m.messages = append(m.messages, errorStyle.Render(fmt.Sprintf("%s: %s", i18n.S("unknown_daemon"), target)))
		return true
	}

	return false
}

// switchDaemon changes the target daemon the TUI talks to.
func (m *ChatModel) switchDaemon(name string, port int) {
	m.activeName = name
	m.activePort = port
	m.mail = agent.NewMailClient(fmt.Sprintf("127.0.0.1:%d", port))
	m.verboseOffset = 0 // reset verbose to read new daemon's log from start
	m.messages = append(m.messages, AgentMsg.Render(
		fmt.Sprintf("%s %s (port %d)", i18n.S("switched_to"), name, port),
	))
}

// cycleNextSpirit switches to the next running spirit via Tab.
func (m *ChatModel) cycleNextSpirit() {
	spirits := manage.ScanSpirits(m.config.ProjectDir)
	if len(spirits) == 0 {
		return
	}
	// Find current index
	currentIdx := -1
	for i, s := range spirits {
		if s.Port == m.activePort {
			currentIdx = i
			break
		}
	}
	// Advance to next alive spirit
	for offset := 1; offset <= len(spirits); offset++ {
		next := spirits[(currentIdx+offset)%len(spirits)]
		if next.Alive {
			m.switchDaemon(next.Name, next.Port)
			return
		}
	}
}

// stopListener shuts down the mail listener if running.
func (m *ChatModel) stopListener() {
	if m.listener != nil {
		m.listener.Stop()
		m.listener = nil
	}
}

var errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

func formatLogEvent(e logEvent) string {
	switch e.Type {
	case "response":
		if e.Text != "" {
			return AgentMsg.Render(e.Text)
		}
	case "tool_call":
		name := e.ToolName
		if name == "" {
			name = e.Name
		}
		if name != "" {
			return ToolCall.Render(fmt.Sprintf("⚡ %s", name))
		}
	case "mail_received":
		text := e.Text
		if e.Subject != "" {
			text = e.Subject
		}
		return IMAPReceived.Render(fmt.Sprintf("📨 %s: %s", e.Sender, text))
	case "mail_sent":
		return IMAPSent.Render(fmt.Sprintf("📤 → %v: %s", e.To, e.Text))
	case "diary":
		return DiaryMsg.Render(fmt.Sprintf("📝 %s", e.Text))
	}
	return ""
}

