package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"stoai-daemon/internal/agent"
	"stoai-daemon/internal/config"
	"stoai-daemon/internal/i18n"
)

// logEventMsg wraps a LogEvent for the Bubble Tea message loop.
type logEventMsg LogEvent

// mailReceivedMsg wraps a received TCP mail message.
type mailReceivedMsg map[string]interface{}

// tickMsg triggers periodic log event polling.
type tickMsg time.Time

// Model is the main TUI model.
type Model struct {
	config   *config.Config
	proc     *agent.Process
	mail     *agent.MailClient
	listener *agent.MailListener
	tailer   *LogTailer

	viewport viewport.Model
	input    textinput.Model
	messages []string

	width  int
	height int
	ready  bool
	err    error
}

// New creates a new TUI model.
func New(cfg *config.Config, proc *agent.Process) Model {
	ti := textinput.New()
	ti.Placeholder = i18n.S("type_message")
	ti.Focus()
	ti.CharLimit = 4096
	ti.Width = 80

	// Mail client targeting the agent
	mailClient := agent.NewMailClient(fmt.Sprintf("127.0.0.1:%d", cfg.AgentPort))

	m := Model{
		config:   cfg,
		proc:     proc,
		mail:     mailClient,
		input:    ti,
		messages: []string{},
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.pollLogEvents(),
		m.pollMailEvents(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input.Value())
			if text != "" {
				m.input.SetValue("")
				m.messages = append(m.messages, InputPrompt.Render("> ")+text)
				m.updateViewport()

				// Send via TCP mail to agent
				go m.mail.Send(map[string]interface{}{
					"from":    fmt.Sprintf("cli@localhost:%d", m.config.CLIPort),
					"message": text,
				})
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 1 // status bar
		inputHeight := 3  // input box
		vpHeight := m.height - headerHeight - inputHeight

		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true

			// Start log tailer
			logPath := fmt.Sprintf("%s/logs/events.jsonl", m.config.WorkingDir())
			m.tailer = NewLogTailer(logPath)

			// Start mail listener for agent replies
			if m.config.CLIPort > 0 {
				listener, err := agent.NewMailListener(m.config.CLIPort, func(msg map[string]interface{}) {
					// Will be picked up by pollMailEvents
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

	case logEventMsg:
		event := LogEvent(msg)
		line := formatLogEvent(event)
		if line != "" {
			m.messages = append(m.messages, line)
			m.updateViewport()
		}
		cmds = append(cmds, m.pollLogEvents())

	case mailReceivedMsg:
		text, _ := msg["message"].(string)
		sender, _ := msg["from"].(string)
		if text != "" {
			line := EmailMsg.Render(fmt.Sprintf("[%s] %s", sender, text))
			m.messages = append(m.messages, line)
			m.updateViewport()
		}
		cmds = append(cmds, m.pollMailEvents())

	case tickMsg:
		cmds = append(cmds, m.pollLogEvents())
	}

	var cmd tea.Cmd

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	// Update text input
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	// Status bar
	statusLeft := TitleStyle.Render(i18n.S("title"))
	channels := []string{}
	if m.config.IMAP != nil {
		channels = append(channels, ActiveChannel.Render("IMAP"))
	} else {
		channels = append(channels, DisabledChannel.Render("IMAP"))
	}
	if m.config.Telegram != nil {
		channels = append(channels, ActiveChannel.Render("TG"))
	} else {
		channels = append(channels, DisabledChannel.Render("TG"))
	}
	if m.config.CLI {
		channels = append(channels, ActiveChannel.Render("CLI"))
	}
	statusRight := strings.Join(channels, " ")
	statusBar := StatusBarStyle.Width(m.width).Render(
		statusLeft + strings.Repeat(" ", max(0, m.width-lipgloss.Width(statusLeft)-lipgloss.Width(statusRight)-4)) + statusRight,
	)

	// Input
	inputBox := InputPrompt.Render("❯ ") + m.input.View()

	return statusBar + "\n" + m.viewport.View() + "\n" + inputBox
}

func (m *Model) updateViewport() {
	content := strings.Join(m.messages, "\n")
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m Model) pollLogEvents() tea.Cmd {
	return func() tea.Msg {
		if m.tailer == nil {
			time.Sleep(500 * time.Millisecond)
			return tickMsg(time.Now())
		}
		select {
		case event := <-m.tailer.Events():
			return logEventMsg(event)
		case <-time.After(500 * time.Millisecond):
			return tickMsg(time.Now())
		}
	}
}

func (m Model) pollMailEvents() tea.Cmd {
	return func() tea.Msg {
		// Poll is handled via the listener callback + log events
		// For now, just tick
		time.Sleep(1 * time.Second)
		return tickMsg(time.Now())
	}
}

func formatLogEvent(e LogEvent) string {
	switch e.Type {
	case "response":
		if e.Text != "" {
			return AgentMsg.Render(e.Text)
		}
	case "tool_call":
		name := e.GetToolName()
		if name != "" {
			return ToolCall.Render(fmt.Sprintf("⚡ %s", name))
		}
	case "mail_received":
		sender := e.Sender
		text := e.Text
		if e.Subject != "" {
			text = e.Subject
		}
		return IMAPReceived.Render(fmt.Sprintf("📨 %s: %s", sender, text))
	case "mail_sent":
		return IMAPSent.Render(fmt.Sprintf("📤 → %v: %s", e.To, e.Text))
	case "diary":
		return DiaryMsg.Render(fmt.Sprintf("📝 %s", e.Text))
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Run starts the TUI.
func Run(cfg *config.Config, proc *agent.Process) {
	m := New(cfg, proc)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
