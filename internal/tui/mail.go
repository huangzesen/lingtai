package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/anthropics/lingtai-tui/internal/fs"
)

type MailModel struct {
	inbox     []fs.MailMessage
	threads   map[string][]fs.MailMessage
	senders   []string
	cursor    int
	input     textinput.Model
	humanDir  string
	humanAddr string
	baseDir   string
	composing bool
}

func NewMailModel(humanDir, humanAddr, baseDir string) MailModel {
	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 1000
	ti.Width = 60
	return MailModel{input: ti, humanDir: humanDir, humanAddr: humanAddr, baseDir: baseDir, threads: make(map[string][]fs.MailMessage)}
}

type mailRefreshMsg struct{ messages []fs.MailMessage }

func (m MailModel) refreshMail() tea.Msg {
	messages, _ := fs.ReadInbox(m.humanDir)
	return mailRefreshMsg{messages: messages}
}

type tickMsg time.Time

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Every(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m MailModel) Init() tea.Cmd {
	return tea.Batch(m.refreshMail, tickEvery(time.Second))
}

func (m MailModel) Update(msg tea.Msg) (MailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case mailRefreshMsg:
		m.inbox = msg.messages
		m.rebuildThreads()
		return m, nil
	case tickMsg:
		return m, tea.Batch(m.refreshMail, tickEvery(time.Second))
	case tea.KeyMsg:
		if m.composing {
			switch msg.String() {
			case "enter":
				text := m.input.Value()
				if text != "" && m.cursor < len(m.senders) {
					sender := m.senders[m.cursor]
					agents, _ := fs.DiscoverAgents(m.baseDir)
					for _, a := range agents {
						if a.Address == sender {
							fs.WriteMail(a.WorkingDir, m.humanDir, m.humanAddr, sender, "", text)
							break
						}
					}
					m.input.SetValue("")
				}
				m.composing = false
				m.input.Blur()
				return m, m.refreshMail
			case "esc":
				m.composing = false
				m.input.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.senders)-1 {
				m.cursor++
			}
		case "enter", "c":
			m.composing = true
			m.input.Focus()
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m *MailModel) rebuildThreads() {
	m.threads = make(map[string][]fs.MailMessage)
	for _, msg := range m.inbox {
		m.threads[msg.From] = append(m.threads[msg.From], msg)
	}
	m.senders = make([]string, 0, len(m.threads))
	for sender := range m.threads {
		m.senders = append(m.senders, sender)
	}
	sort.Slice(m.senders, func(i, j int) bool {
		ti := m.threads[m.senders[i]]
		tj := m.threads[m.senders[j]]
		return ti[len(ti)-1].ReceivedAt > tj[len(tj)-1].ReceivedAt
	})
}

func (m MailModel) View() string {
	var b strings.Builder
	title := StyleTitle.Render("灵台 — Mail")
	b.WriteString("\n  " + title + "              [tab] Agents\n\n")
	if len(m.senders) == 0 {
		b.WriteString(StyleSubtle.Render("  No messages yet. Waiting for mail...\n"))
	}
	for i, sender := range m.senders {
		msgs := m.threads[sender]
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		parts := strings.Split(sender, "/")
		name := parts[len(parts)-1]
		count := fmt.Sprintf("(%d)", len(msgs))
		line := fmt.Sprintf("%s%s %s", cursor, name, StyleSubtle.Render(count))
		if i == m.cursor {
			b.WriteString(lipgloss.NewStyle().Bold(true).Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	if m.cursor < len(m.senders) {
		b.WriteString("\n" + strings.Repeat("─", 50) + "\n\n")
		msgs := m.threads[m.senders[m.cursor]]
		for _, msg := range msgs {
			parts := strings.Split(msg.From, "/")
			name := parts[len(parts)-1]
			b.WriteString(fmt.Sprintf("  %s: %s\n", lipgloss.NewStyle().Foreground(ColorActive).Render(name), msg.Message))
		}
	}
	b.WriteString("\n" + strings.Repeat("─", 50) + "\n")
	if m.composing {
		b.WriteString("  > " + m.input.View() + "\n")
	} else {
		b.WriteString(StyleSubtle.Render("  [Enter/c] Compose") + "\n")
	}
	return b.String()
}
