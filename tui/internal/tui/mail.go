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
	"github.com/anthropics/lingtai-tui/internal/preset"
)

// unlimitedPageSize is the effective page size when the user selects "unlimited".
const unlimitedPageSize = 999999

// ChatMessage represents a single message in the chat stream.
type ChatMessage struct {
	From        string
	To          string
	Subject     string
	Body        string
	Timestamp   string
	IsFromMe    bool     // human sent this
	IsFromOrch  bool     // orchestrator (主我) sent this
	Type        string   // "mail", "thinking", "diary", "insight"
	Attachments []string // file paths attached to the message
	Question    string   // question text (for /btw insight events)
}

// ViewChangeMsg requests the app to switch views.
type ViewChangeMsg struct {
	View string
}

type pulseTickMsg time.Time

func pulseTick() tea.Cmd {
	return tea.Every(250*time.Millisecond, func(t time.Time) tea.Msg { return pulseTickMsg(t) })
}

type mailRefreshMsg struct {
	cache        fs.MailCache // incrementally updated cache
	alive        bool
	state        string // active, idle, stuck, asleep, suspended, or ""
	orchName     string // agent name from .agent.json (may change at runtime)
	orchNickname string // nickname from .agent.json
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
	verboseThinking                     // ctrl+o cycle: mail + soul (thinking, diary, text_input, text_output)
	verboseExtended                     // ctrl+o cycle: everything (+ tool_call, tool_result)
)

// spinnerFrames is a star-burst spinner shown flanking the thinking quote.
var spinnerFrames = []string{"✶", "✸", "✹", "✺", "✹", "✸"}

// thinkingQuotes are short phrases shown rotating in the header while thinking.
// Chinese: segments from the three Bodhi verses (菩提偈).
// English: Buddhist concepts and sutric phrases.
// Classical Chinese: same as Chinese (shared literary tradition).
var thinkingQuotesMap = map[string][]string{
	"zh": {
		"菩提本无树", "明镜亦非台", "佛性常清净", "何处有尘埃",
		"身是菩提树", "心为明镜台", "明镜本清净", "何处染尘埃",
		"菩提本无树", "明镜亦非台", "本来无一物", "何处惹尘埃",
	},
	"wen": {
		"菩提本无树", "明镜亦非台", "佛性常清净", "何处有尘埃",
		"身是菩提树", "心为明镜台", "明镜本清净", "何处染尘埃",
		"菩提本无树", "明镜亦非台", "本来无一物", "何处惹尘埃",
	},
	"en": {
		"Cogitating", "Meditating", "Contemplating", "Deliberating", "Ruminating",
		"Perceiving", "Discerning", "Reasoning", "Examining", "Reflecting",
	},
}

type MailModel struct {
	humanDir         string
	humanAddr        string
	orchestrator     string // 本我 directory path (full path under .lingtai/)
	orchAddr         string // 本我 address (from .agent.json)
	orchName         string // 本我 agent name (true name)
	orchNickname     string // 本我 nickname (display name override)
	baseDir          string // .lingtai/ directory
	verbose          verboseLevel
	messages         []ChatMessage // derived from cache on each refresh
	cache            fs.MailCache  // incremental mail cache
	pageSize         int           // max messages shown (from settings)
	loadedExtra      int           // additional older messages loaded via ctrl+u
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
	lastBannerLines  int
	pendingMessage   string // full text from editor, sent on Enter
	globalDir      string // ~/.lingtai-tui/
	greetEnabled   bool   // from settings
	greetChecked   bool   // true after first refresh check
	greetLang      string // current language
	wasActive      bool   // true if previous refresh was ACTIVE
	quoteIdx       int    // which quote to show (advances on each ACTIVE transition)
	pulseTick      int    // pulse animation counter while ACTIVE
	showEditorWarn  bool   // one-time vim warning overlay
	editorWarnText  string // text to pass to editor after warning
	insightsEnabled bool   // from settings — show insight events
}

func NewMailModel(humanDir, humanAddr, baseDir, orchDir, orchName string, pageSize int, greeting bool, globalDir, lang string, insights bool) MailModel {
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
	if pageSize <= 0 {
		pageSize = unlimitedPageSize
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
		pollRate:     1 * time.Second,
		cache:        fs.NewMailCache(humanDir),
		pageSize:     pageSize,
		globalDir:      globalDir,
		greetEnabled:    greeting,
		greetLang:       lang,
		quoteIdx:        -1,
		insightsEnabled: insights,
	}
}

// syncViewportHeight recalculates viewport height from current input/palette/banner size.
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
	bannerLines := m.bannerLineCount()
	if inputLines == m.lastInputLines && paletteLines == m.lastPaletteLines && bannerLines == m.lastBannerLines {
		return false
	}
	m.lastInputLines = inputLines
	m.lastPaletteLines = paletteLines
	m.lastBannerLines = bannerLines
	// Layout: header(2) + topBanner(0-1) + viewport + bottomBanner(0-1) + sep(1) + palette(N) + input(N) + border(1) + status(1)
	footerHeight := 1 + paletteLines + inputLines + 1 + 1
	vpHeight := m.height - 2 - bannerLines - footerHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.SetHeight(vpHeight)
	return true
}

// bannerLineCount returns the total lines reserved for top and bottom banners.
func (m *MailModel) bannerLineCount() int {
	n := 0
	if m.hasMoreOlder() {
		n++ // top banner
	}
	if m.loadedExtra > 0 {
		n++ // bottom banner (reserved when expanded)
	}
	return n
}

// hasMoreOlder returns true when there are messages beyond the visible window.
func (m *MailModel) hasMoreOlder() bool {
	return len(m.messages) > m.pageSize+m.loadedExtra
}

// olderCount returns how many messages are hidden above the visible window.
func (m *MailModel) olderCount() int {
	hidden := len(m.messages) - m.pageSize - m.loadedExtra
	if hidden < 0 {
		return 0
	}
	return hidden
}

// visibleMessages returns the tail of m.messages limited by pageSize + loadedExtra.
func (m *MailModel) visibleMessages() []ChatMessage {
	limit := m.pageSize + m.loadedExtra
	if limit >= len(m.messages) {
		return m.messages
	}
	return m.messages[len(m.messages)-limit:]
}

func (m MailModel) refreshMail() tea.Msg {
	// Refresh human location (no-op if cache is <1h old)
	go fs.UpdateHumanLocation(m.humanDir)

	// Incremental cache refresh — only reads new messages from disk
	cache := m.cache.Refresh()

	alive := m.orchestrator != "" && fs.IsAlive(m.orchestrator, 3.0)
	state := ""
	orchName := m.orchName
	orchNickname := ""
	if m.orchestrator != "" {
		if node, err := fs.ReadAgent(m.orchestrator); err == nil {
			state = node.State
			if node.AgentName != "" {
				orchName = node.AgentName
			}
			orchNickname = node.Nickname
		}
	}
	if !alive {
		state = "suspended"
	}
	return mailRefreshMsg{cache: cache, alive: alive, state: state, orchName: orchName, orchNickname: orchNickname}
}

// orchDisplayName returns the nickname if set, otherwise the agent name.
func (m MailModel) orchDisplayName() string {
	if m.orchNickname != "" {
		return m.orchNickname
	}
	return m.orchName
}

// buildMessages converts cached MailMessages to ChatMessages, merges with
// events if verbose, and sorts chronologically.
func (m *MailModel) buildMessages() {
	chatMsgs := make([]ChatMessage, 0, len(m.cache.Messages))
	humanName := m.humanName()
	for _, msg := range m.cache.Messages {
		parts := strings.Split(msg.From, "/")
		fromName := parts[len(parts)-1]
		isFromMe := msg.From == m.humanAddr || fromName == "human"
		// Use sender's identity from the message envelope when available
		displayFrom := fromName
		if !isFromMe {
			if nick, ok := msg.Identity["nickname"].(string); ok && nick != "" {
				displayFrom = nick
			} else if name, ok := msg.Identity["agent_name"].(string); ok && name != "" {
				displayFrom = name
			}
		}
		isFromOrch := !isFromMe && (msg.From == m.orchAddr || msg.From == m.orchestrator)
		cm := ChatMessage{
			From:        displayFrom,
			To:          m.orchDisplayName(),
			Subject:     msg.Subject,
			Body:        msg.Message,
			Timestamp:   msg.ReceivedAt,
			IsFromMe:    isFromMe,
			IsFromOrch:  isFromOrch,
			Type:        "mail",
			Attachments: msg.Attachments,
		}
		if isFromMe {
			cm.From = humanName
		} else {
			cm.To = humanName
		}
		chatMsgs = append(chatMsgs, cm)
	}

	// If verbose, read events
	if m.verbose != verboseOff && m.orchestrator != "" {
		eventsPath := filepath.Join(m.orchestrator, "logs", "events.jsonl")
		extended := m.verbose == verboseExtended
		events := ReadEvents(eventsPath, extended)
		chatMsgs = append(chatMsgs, events...)
	}

	// Read insight events independently of verbose mode
	if m.insightsEnabled && m.orchestrator != "" {
		eventsPath := filepath.Join(m.orchestrator, "logs", "events.jsonl")
		insights := ReadInsightEvents(eventsPath)
		chatMsgs = append(chatMsgs, insights...)
	}

	// Sort by timestamp
	sort.Slice(chatMsgs, func(i, j int) bool {
		return chatMsgs[i].Timestamp < chatMsgs[j].Timestamp
	})
	m.messages = chatMsgs
}

// buildGreetPrompt reads the greet template, replaces placeholders, and returns
// the final prompt string. Returns "" if the template file is missing.
func (m *MailModel) buildGreetPrompt() string {
	path := preset.GreetPath(m.globalDir, m.greetLang)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prompt := string(data)
	prompt = strings.ReplaceAll(prompt, "{{time}}", time.Now().Format("2006-01-02 15:04"))
	prompt = strings.ReplaceAll(prompt, "{{addr}}", m.humanAddr)
	prompt = strings.ReplaceAll(prompt, "{{lang}}", m.greetLang)
	// Location from human's .agent.json
	loc := ""
	humanNode, err := fs.ReadAgent(m.humanDir)
	if err == nil && humanNode.Location != nil {
		parts := []string{}
		if humanNode.Location.City != "" {
			parts = append(parts, humanNode.Location.City)
		}
		if humanNode.Location.Region != "" {
			parts = append(parts, humanNode.Location.Region)
		}
		if humanNode.Location.Country != "" {
			parts = append(parts, humanNode.Location.Country)
		}
		loc = strings.Join(parts, ", ")
	}
	if loc == "" {
		loc = "unknown"
	}
	prompt = strings.ReplaceAll(prompt, "{{location}}", loc)
	// Soul delay from agent's init.json
	soulDelay := "unknown"
	if m.orchestrator != "" {
		if manifest, err := fs.ReadInitManifest(m.orchestrator); err == nil {
			if sd, ok := manifest["soul_delay"]; ok {
				soulDelay = fmt.Sprintf("%v", sd)
			}
		}
	}
	prompt = strings.ReplaceAll(prompt, "{{soul_delay}}", soulDelay)
	return prompt
}

func (m MailModel) Init() tea.Cmd {
	return tea.Batch(
		m.input.Init(),
		m.refreshMail,
		tickEvery(m.pollRate),
		pulseTick(),
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
			m.viewport.SetContent(m.renderMessages(m.visibleMessages()))
			m.lastInputLines = inputLines
			m.ready = true
		} else {
			m.viewport.SetWidth(msg.Width)
			m.lastInputLines = -1 // force recalculate
			m.syncViewportHeight()
		}
		return m, nil

	case mailRefreshMsg:
		m.cache = msg.cache
		m.orchAlive = msg.alive
		m.orchState = msg.state
		if msg.orchName != "" {
			m.orchName = msg.orchName
		}
		m.orchNickname = msg.orchNickname
		isActive := strings.EqualFold(m.orchState, "ACTIVE")
		if isActive && !m.wasActive {
			// Just became active — advance to next quote, reset pulse
			m.quoteIdx++
			m.pulseTick = 0
		}
		m.wasActive = isActive
		m.buildMessages()
		// Auto-greet: on first refresh, if history is empty, write .prompt
		if !m.greetChecked {
			m.greetChecked = true
			if m.greetEnabled && len(m.messages) == 0 && m.orchestrator != "" {
				if prompt := m.buildGreetPrompt(); prompt != "" {
					fs.WritePrompt(m.orchestrator, prompt)
				}
			}
		}
		if m.ready {
			atBottom := m.viewport.AtBottom()
			m.syncViewportHeight()
			m.viewport.SetContent(m.renderMessages(m.visibleMessages()))
			if atBottom {
				m.viewport.GotoBottom()
			}
		}
		return m, nil

	case pulseTickMsg:
		if strings.EqualFold(m.orchState, "ACTIVE") {
			m.pulseTick++
		}
		return m, pulseTick()

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
		// Show editor intro page before launching
		m.showEditorWarn = true
		m.editorWarnText = msg.Text
		return m, nil

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
		// Editor warning overlay — Enter proceeds, Esc cancels
		if m.showEditorWarn {
			switch msg.String() {
			case "enter":
				m.showEditorWarn = false
				return m, m.launchEditor(m.editorWarnText)
			case "esc", "ctrl+c":
				m.showEditorWarn = false
				return m, nil
			}
			return m, nil
		}

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

		case "ctrl+u":
			if m.ready && m.viewport.AtTop() && m.hasMoreOlder() {
				m.loadedExtra += m.pageSize
				m.syncViewportHeight()
				m.viewport.SetContent(m.renderMessages(m.visibleMessages()))
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd

		case "ctrl+d":
			if m.ready && m.viewport.AtBottom() && m.loadedExtra > 0 {
				m.loadedExtra = 0
				m.syncViewportHeight()
				m.viewport.SetContent(m.renderMessages(m.visibleMessages()))
				m.viewport.GotoBottom()
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd

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

func (m MailModel) renderMessages(msgs []ChatMessage) string {
	if len(msgs) == 0 {
		return "\n" + StyleFaint.Render("  "+RuneBullet+" "+i18n.T("mail.no_messages"))
	}

	humanStyle := lipgloss.NewStyle().Foreground(ColorHuman).Bold(true)
	agentStyle := lipgloss.NewStyle().Foreground(ColorAgent).Bold(true)
	avatarStyle := lipgloss.NewStyle().Foreground(ColorIdle).Bold(true)
	systemStyle := lipgloss.NewStyle().Foreground(ColorSystem).Bold(true)
	thinkingStyle := lipgloss.NewStyle().Foreground(ColorThinking)
	toolStyle := lipgloss.NewStyle().Foreground(ColorTool)
	sepStyle := lipgloss.NewStyle().Foreground(ColorTextDim)

	var b strings.Builder
	for _, msg := range msgs {
		switch msg.Type {
		case "thinking", "diary", "text_input", "text_output", "tool_call", "tool_result":
			wrapWidth := m.width - 6
			if wrapWidth < 20 {
				wrapWidth = 20
			}
			var evStyle lipgloss.Style
			switch msg.Type {
			case "thinking", "diary", "text_input", "text_output":
				evStyle = thinkingStyle
			default:
				evStyle = toolStyle
			}
			wrapped := lipgloss.NewStyle().Width(wrapWidth).Render("[" + msg.Type + "] " + msg.Body)
			for _, line := range strings.Split(wrapped, "\n") {
				b.WriteString(evStyle.Render("  "+RuneBullet+" "+line) + "\n")
			}

		case "insight":
			wrapWidth := m.width - 6
			if wrapWidth < 20 {
				wrapWidth = 20
			}
			barWidth := min(wrapWidth, 44)
			insightStyle := lipgloss.NewStyle().Foreground(ColorAccent)

			if msg.Question != "" {
				// /btw box: ┌─ btw ─── / question / ├─── / answer / └───
				b.WriteString(insightStyle.Render("  ┌─ btw "+strings.Repeat("─", max(barWidth-8, 1))) + "\n")
				wrapped := lipgloss.NewStyle().Width(max(wrapWidth-4, 10)).Render(msg.Question)
				for _, line := range strings.Split(wrapped, "\n") {
					b.WriteString(insightStyle.Render("  │ "+line) + "\n")
				}
				b.WriteString(insightStyle.Render("  ├"+strings.Repeat("─", max(barWidth-1, 1))) + "\n")
				wrapped = lipgloss.NewStyle().Width(max(wrapWidth-4, 10)).Render(msg.Body)
				for _, line := range strings.Split(wrapped, "\n") {
					b.WriteString(insightStyle.Render("  │ "+line) + "\n")
				}
				b.WriteString(insightStyle.Render("  └"+strings.Repeat("─", max(barWidth-1, 1))) + "\n")
			} else {
				// auto-insight: ★ insight ─── / bullets / ───
				b.WriteString(insightStyle.Render("  ★ insight "+strings.Repeat("─", max(barWidth-11, 1))) + "\n")
				wrapped := lipgloss.NewStyle().Width(max(wrapWidth-2, 10)).Render(msg.Body)
				for _, line := range strings.Split(wrapped, "\n") {
					b.WriteString(insightStyle.Render("  "+line) + "\n")
				}
				b.WriteString(insightStyle.Render("  "+strings.Repeat("─", barWidth)) + "\n")
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
			} else if msg.IsFromOrch {
				nameStyle = agentStyle
			} else {
				nameStyle = avatarStyle
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
					glamour.WithStandardStyle(ActiveTheme().GlamourStyle),
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

// launchEditor creates a temp file and opens $EDITOR (default: vim).
func (m MailModel) launchEditor(text string) tea.Cmd {
	tmpFile, err := os.CreateTemp("", "lingtai-input-*.txt")
	if err != nil {
		return nil
	}
	tmpFile.WriteString(text)
	tmpFile.Close()
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	cmd := exec.Command(editor, tmpFile.Name())
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			os.Remove(tmpFile.Name())
			return nil
		}
		content, _ := os.ReadFile(tmpFile.Name())
		os.Remove(tmpFile.Name())
		return EditorDoneMsg{Text: string(content)}
	})
}

// viewEditorWarn renders the editor confirmation overlay.
func (m MailModel) viewEditorWarn() string {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	var b strings.Builder

	title := StyleTitle.Render("  " + i18n.T("editor_warn.title"))
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	editorName := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render(editor)
	b.WriteString("  " + i18n.TF("editor_warn.editor_is", editorName) + "\n\n")
	b.WriteString("  " + StyleFaint.Render(i18n.T("editor_warn.change_hint")) + "\n")

	b.WriteString("\n" + strings.Repeat("─", m.width) + "\n")
	enterHint := StyleAccent.Render("[Enter] ") + StyleSubtle.Render(i18n.T("editor_warn.proceed"))
	escHint := StyleAccent.Render("[Esc] ") + StyleSubtle.Render(i18n.T("editor_warn.cancel"))
	b.WriteString("  " + enterHint + "    " + escHint + "\n")

	return b.String()
}

func (m MailModel) View() string {
	if m.showEditorWarn {
		return m.viewEditorWarn()
	}
	if !m.ready {
		return "\n  " + i18n.T("app.loading")
	}

	// Build header: left = app title, center = thinking quote, right = agent [state]
	titleLeft := StyleTitle.Render("  " + i18n.T("app.brand"))

	// State badge with color
	stateKey := m.orchState
	if stateKey == "" {
		stateKey = "unknown"
	}
	stateLabel := i18n.T("state." + stateKey)
	stateStyle := lipgloss.NewStyle().Foreground(StateColor(strings.ToUpper(stateKey)))
	orchNameStyle := lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	titleRight := orchNameStyle.Render(m.orchDisplayName()) + " " + stateStyle.Render("◉ "+stateLabel)

	// Thinking indicator: fixed quote per ACTIVE session, pulsing color + spinners
	titleCenter := ""
	if strings.EqualFold(m.orchState, "ACTIVE") {
		quotes := thinkingQuotesMap[i18n.Lang()]
		if quotes == nil {
			quotes = thinkingQuotesMap["en"]
		}
		quote := quotes[m.quoteIdx%len(quotes)]
		spinner := spinnerFrames[m.pulseTick%len(spinnerFrames)]
		shades := ActiveTheme().PulseShades
		shade := lipgloss.Color(shades[m.pulseTick%len(shades)])
		style := lipgloss.NewStyle().Foreground(shade)
		titleCenter = style.Render(spinner + " " + quote + " " + spinner)
	}

	leftW := lipgloss.Width(titleLeft)
	rightW := lipgloss.Width(titleRight)
	centerW := lipgloss.Width(titleCenter)
	var titleLine string
	if titleCenter != "" {
		// Three-part layout: left ... center ... right
		gapTotal := m.width - leftW - centerW - rightW - 1
		if gapTotal > 0 {
			leftGap := gapTotal / 2
			rightGap := gapTotal - leftGap
			titleLine = titleLeft + strings.Repeat(" ", leftGap) + titleCenter + strings.Repeat(" ", rightGap) + titleRight
		} else {
			titleLine = titleLeft + " " + titleCenter + " " + titleRight
		}
	} else {
		padding := m.width - leftW - rightW - 1
		if padding > 0 {
			titleLine = titleLeft + strings.Repeat(" ", padding) + titleRight
		} else {
			titleLine = titleLeft + "  " + titleRight
		}
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
		hints = StyleSubtle.Render(i18n.T("hints.verbose")) +
			StyleFaint.Render(" "+RuneBullet+" "+i18n.T("hints.editor")+" "+RuneBullet+" "+i18n.T("hints.commands"))
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

	// Top banner: "▲ N older — ctrl+u to load"
	topBanner := ""
	if m.hasMoreOlder() {
		bannerText := i18n.TF("mail.load_more", m.olderCount())
		topBanner = StyleFaint.Render(centerText(bannerText, m.width)) + "\n"
	}

	// Bottom banner: "▼ ctrl+d to collapse to recent"
	bottomBanner := ""
	if m.loadedExtra > 0 {
		bannerText := i18n.T("mail.collapse")
		bottomBanner = StyleFaint.Render(centerText(bannerText, m.width)) + "\n"
	}

	// Viewport fills the middle
	return header + "\n" + topBanner + m.viewport.View() + "\n" + bottomBanner + footer
}
