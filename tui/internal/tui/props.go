package tui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/fs"
)

// PropsModel is the single, vertically stacked /kanban screen. Bubble Tea
// commands own all bounded filesystem reads; rendering consumes only the
// accepted manual snapshot stored here.
type PropsModel struct {
	baseDir   string // .lingtai/ directory (for agent discovery)
	orchDir   string // admin agent's working dir (default selected)
	globalDir string // retained for path presentation/constructor compatibility
	width     int
	height    int

	// Manual snapshot: selected agent status plus bounded network/picker data.
	selectedDir    string         // working dir of the shown agent (defaults to orchDir)
	selectedStatus fs.AgentStatus // cached .status.json for selected agent
	agentNodes     []fs.AgentNode // discovered agents (for picker display)
	network        fs.Network
	// requestGeneration is issued only on the Bubble Tea update loop. It
	// prevents an older completion for the same selected directory from
	// overwriting a newer Init/Ctrl+R/selection request.
	requestGeneration uint64

	// Snapshot facts are refreshed only on Init, Ctrl+R, and agent selection.
	// These resolved facts make every render helper independent of the source
	// files, key/auth configuration, preset directories, and environment files.
	selectedRaw        map[string]any
	selectedLLM        kanbanLLMConfig
	selectedPresetInfo kanbanPresetInfo
	selectedTokens     fs.TokenTotals
	tokens             fs.TokenTotals
	adminStart         string // admin agent's created_at/started_at timestamp
	mailStatsAvailable bool

	// Scrollable viewport for content
	viewport viewport.Model
	ready    bool // viewport initialized

	// Agent picker overlay
	pickerOpen bool
	pickerIdx  int

	// Lower diagnostic snapshot, stacked below Network now on the same screen.
	detailByProvider          map[string]fs.TokenTotals
	detailDaemonByProvider    map[string]fs.TokenTotals // daemon token usage by provider/backend
	detailRecent              []fs.LedgerEntry          // selected main agent recent calls (newest first)
	detailDaemonRecent        []fs.DaemonLedgerEntry    // all daemon calls, newest first, tagged by run
	detailCurrentSessionStats fs.SessionTokenStats
	detailLastSessionStats    fs.SessionTokenStats
	// detailCurrentSessionToolCalls / detailLastSessionToolCalls hold lifecycle
	// tool_call counts for the same molt windows as the session API stats.
	// They are sourced from the same bounded 1000-line event tail as session
	// boundaries and rebuild markers.
	detailCurrentSessionToolCalls int64
	detailLastSessionToolCalls    int64
	detailContextStats            fs.ContextStats
	detailDaemonCounts            fs.DaemonCounts
	detailDaemonRunsScanned       int // exact recent dispatch run IDs attempted
	detailDaemonRunsTotal         int // valid dispatches in the recent dispatch window
	detailDaemonRunsTerminal      int // terminal cards observed in that window
	detailDaemonWindowState       string
	detailMCPNames                []string
	detailRebuilds                []time.Time // psyche_molt times, newest first; rendered as molt separators
	detailRefreshes               []time.Time // refresh_complete times, newest first; rendered as context-rebuilt separators
	lastReadStats                 []fs.BoundedReadStats
	detailDaemonRunIDs            []string
	selectedTokenRead             fs.BoundedReadStats
	networkTokenReads             []fs.BoundedReadStats
	detailEventRead               fs.BoundedReadStats
	detailContextRead             fs.BoundedReadStats
	detailDaemonTokenReads        []fs.BoundedReadStats
	detailSessionCoverage         bool
}

const (
	// detailRecentCalls is the number of recent token-ledger calls shown in each
	// lower recent-call lane (main agent first, then daemons).
	detailRecentCalls = 100
)

func NewPropsModel(baseDir, orchDir, globalDir string) PropsModel {
	return PropsModel{
		baseDir:     baseDir,
		orchDir:     orchDir,
		globalDir:   globalDir,
		selectedDir: orchDir,
	}
}

type propsLoadMsg struct {
	selectedDir    string
	generation     uint64
	network        fs.Network
	selectedStatus fs.AgentStatus
	agentNodes     []fs.AgentNode

	selectedRaw        map[string]any
	selectedLLM        kanbanLLMConfig
	selectedPresetInfo kanbanPresetInfo
	selectedTokens     fs.TokenTotals
	tokens             fs.TokenTotals
	adminStart         string

	detailByProvider              map[string]fs.TokenTotals
	detailDaemonByProvider        map[string]fs.TokenTotals
	detailRecent                  []fs.LedgerEntry
	detailDaemonRecent            []fs.DaemonLedgerEntry
	detailCurrentSessionStats     fs.SessionTokenStats
	detailLastSessionStats        fs.SessionTokenStats
	detailCurrentSessionToolCalls int64
	detailLastSessionToolCalls    int64
	detailContextStats            fs.ContextStats
	detailDaemonCounts            fs.DaemonCounts
	detailDaemonRunsScanned       int
	detailDaemonRunsTotal         int
	detailDaemonRunsTerminal      int
	detailDaemonWindowState       string
	detailDaemonRunIDs            []string
	detailMCPNames                []string
	detailRebuilds                []time.Time
	detailRefreshes               []time.Time
	readStats                     []fs.BoundedReadStats
	selectedTokenRead             fs.BoundedReadStats
	networkTokenReads             []fs.BoundedReadStats
	detailEventRead               fs.BoundedReadStats
	detailContextRead             fs.BoundedReadStats
	detailDaemonTokenReads        []fs.BoundedReadStats
	detailSessionCoverage         bool
}

func (m PropsModel) loadSnapshot() tea.Msg {
	msg := propsLoadMsg{selectedDir: m.selectedDir, generation: m.requestGeneration}
	net, _ := fs.BuildNetworkWithOptions(m.baseDir, fs.KanbanNetworkOptions(&msg.readStats))
	msg.network = net
	msg.agentNodes = net.Nodes
	agentRaw := map[string]any{}
	initRaw := map[string]any{}
	if m.selectedDir != "" {
		if loaded, read, err := fs.ReadKanbanAgentRaw(m.selectedDir); err == nil {
			agentRaw = loaded
			msg.readStats = append(msg.readStats, read)
		} else {
			msg.readStats = append(msg.readStats, read)
		}
		if loaded, reads, err := fs.ReadKanbanInitManifest(m.selectedDir); err == nil {
			initRaw = loaded
			msg.readStats = append(msg.readStats, reads...)
		} else {
			msg.readStats = append(msg.readStats, reads...)
		}
		status, statusRead := fs.ReadKanbanStatus(m.selectedDir)
		msg.selectedStatus = status
		msg.readStats = append(msg.readStats, statusRead)
	}
	msg.selectedRaw = mergeKanbanAgentRaw(agentRaw, initRaw)
	msg.selectedLLM = resolveKanbanLLMConfig(agentRaw, initRaw)
	msg.selectedPresetInfo = resolveKanbanPresetInfo(initRaw)

	adminRaw := agentRaw
	if m.orchDir != "" && m.orchDir != m.selectedDir {
		if loaded, read, err := fs.ReadKanbanAgentRaw(m.orchDir); err == nil {
			adminRaw = loaded
			msg.readStats = append(msg.readStats, read)
		} else {
			msg.readStats = append(msg.readStats, read)
		}
	}
	if value, ok := adminRaw["created_at"].(string); ok && value != "" {
		msg.adminStart = value
	} else if value, ok := adminRaw["started_at"].(string); ok && value != "" {
		msg.adminStart = value
	}

	if m.selectedDir == "" {
		return msg
	}

	selectedTokenLoaded := false
	var selectedWindow fs.KanbanTokenWindow
	for _, node := range net.Nodes {
		if node.IsHuman || node.WorkingDir == "" {
			continue
		}
		window := fs.ReadKanbanTokenWindow(filepath.Join(node.WorkingDir, "logs", "token_ledger.jsonl"), detailRecentCalls)
		msg.readStats = append(msg.readStats, window.Read)
		msg.networkTokenReads = append(msg.networkTokenReads, window.Read)
		msg.tokens.Input += window.Totals.Input
		msg.tokens.Output += window.Totals.Output
		msg.tokens.Thinking += window.Totals.Thinking
		msg.tokens.Cached += window.Totals.Cached
		msg.tokens.APICalls += window.Totals.APICalls
		if node.WorkingDir == m.selectedDir {
			selectedWindow = window
			selectedTokenLoaded = true
		}
	}
	if !selectedTokenLoaded {
		selectedWindow = fs.ReadKanbanTokenWindow(filepath.Join(m.selectedDir, "logs", "token_ledger.jsonl"), detailRecentCalls)
		msg.readStats = append(msg.readStats, selectedWindow.Read)
	}
	msg.selectedTokens = selectedWindow.Totals
	msg.selectedTokenRead = selectedWindow.Read
	msg.detailByProvider = selectedWindow.ByProvider
	msg.detailRecent = selectedWindow.Recent

	events := fs.ReadKanbanEventWindow(m.selectedDir, detailRecentCalls)
	msg.readStats = append(msg.readStats, events.Read)
	msg.detailEventRead = events.Read
	sessionStats, sessionCoverage := fs.KanbanSessionTokenStats(selectedWindow, events)
	msg.detailSessionCoverage = sessionCoverage
	msg.detailCurrentSessionStats = sessionStats.Current
	msg.detailLastSessionStats = sessionStats.Last
	if sessionCoverage {
		msg.detailCurrentSessionToolCalls = events.ToolCalls.Current
		msg.detailLastSessionToolCalls = events.ToolCalls.Last
	}
	msg.detailRebuilds = events.Rebuilds
	msg.detailRefreshes = events.Refreshes

	daemonDetail := fs.ReadKanbanDaemonDetailSnapshot(m.selectedDir, detailRecentCalls, &msg.readStats)
	msg.detailDaemonByProvider = daemonDetail.ByProvider
	msg.detailDaemonRecent = daemonDetail.Recent
	msg.detailDaemonCounts = daemonDetail.Counts
	msg.detailDaemonRunsScanned = daemonDetail.ScannedRuns
	msg.detailDaemonRunsTotal = daemonDetail.TotalRuns
	msg.detailDaemonRunsTerminal = daemonDetail.TerminalRuns
	msg.detailDaemonWindowState = daemonDetail.WindowState
	msg.detailDaemonRunIDs = daemonDetail.RunIDs
	msg.detailDaemonTokenReads = daemonDetail.TokenReads
	contextStats, contextRead := fs.ReadKanbanContextStats(m.selectedDir)
	msg.detailContextStats = contextStats
	msg.detailContextRead = contextRead
	msg.readStats = append(msg.readStats, contextRead)

	if mcp, ok := initRaw["mcp"].(map[string]interface{}); ok {
		for name := range mcp {
			if len(msg.detailMCPNames) >= fs.KanbanReadLimit {
				break
			}
			msg.detailMCPNames = append(msg.detailMCPNames, name)
		}
		sort.Strings(msg.detailMCPNames)
	}
	return msg
}

func (m *PropsModel) issueSnapshot() tea.Cmd {
	m.requestGeneration++
	return m.loadSnapshot
}

func (m *PropsModel) Init() tea.Cmd { return m.issueSnapshot() }

// propsHeaderLines is the number of lines used by the header (title + separator).
const propsHeaderLines = 2

// propsFooterLines is the number of lines used by the footer (separator + hints).
const propsFooterLines = 2

func (m PropsModel) Update(msg tea.Msg) (PropsModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpHeight := m.height - propsHeaderLines - propsFooterLines
		if vpHeight < 1 {
			vpHeight = 1
		}
		if !m.ready {
			m.viewport = viewport.New()
			m.viewport.SetWidth(m.width)
			m.viewport.SetHeight(vpHeight)
			m.ready = true
		} else {
			m.viewport.SetWidth(m.width)
			m.viewport.SetHeight(vpHeight)
		}
		m.syncViewportContent()

	case propsLoadMsg:
		if msg.selectedDir != m.selectedDir || msg.generation != m.requestGeneration {
			return m, nil
		}
		m.applySnapshot(msg)
		m.syncViewportContent()

	case tea.MouseWheelMsg:
		if !m.pickerOpen {
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case tea.KeyPressMsg:
		if m.pickerOpen {
			return m.updatePicker(msg)
		}
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
		case "ctrl+r":
			return m, m.issueSnapshot()
		case "ctrl+t":
			m.pickerOpen = true
			for i, n := range m.agentNodes {
				if n.WorkingDir == m.selectedDir {
					m.pickerIdx = i
					break
				}
			}
			m.syncViewportContent()
			return m, nil
		default:
			// Forward navigation keys (up/down/pgup/pgdn/home/end) to viewport
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *PropsModel) applySnapshot(msg propsLoadMsg) {
	m.network = msg.network
	m.mailStatsAvailable = false
	m.selectedStatus = msg.selectedStatus
	m.agentNodes = msg.agentNodes
	m.selectedRaw = msg.selectedRaw
	m.selectedLLM = msg.selectedLLM
	m.selectedPresetInfo = msg.selectedPresetInfo
	m.selectedTokens = msg.selectedTokens
	m.tokens = msg.tokens
	m.adminStart = msg.adminStart
	m.detailByProvider = msg.detailByProvider
	m.detailDaemonByProvider = msg.detailDaemonByProvider
	m.detailRecent = msg.detailRecent
	m.detailDaemonRecent = msg.detailDaemonRecent
	m.detailCurrentSessionStats = msg.detailCurrentSessionStats
	m.detailLastSessionStats = msg.detailLastSessionStats
	m.detailCurrentSessionToolCalls = msg.detailCurrentSessionToolCalls
	m.detailLastSessionToolCalls = msg.detailLastSessionToolCalls
	m.detailContextStats = msg.detailContextStats
	m.detailDaemonCounts = msg.detailDaemonCounts
	m.detailDaemonRunsScanned = msg.detailDaemonRunsScanned
	m.detailDaemonRunsTotal = msg.detailDaemonRunsTotal
	m.detailDaemonRunsTerminal = msg.detailDaemonRunsTerminal
	m.detailDaemonWindowState = msg.detailDaemonWindowState
	m.detailDaemonRunIDs = msg.detailDaemonRunIDs
	m.detailMCPNames = msg.detailMCPNames
	m.detailRebuilds = msg.detailRebuilds
	m.detailRefreshes = msg.detailRefreshes
	m.lastReadStats = msg.readStats
	m.selectedTokenRead = msg.selectedTokenRead
	m.networkTokenReads = msg.networkTokenReads
	m.detailEventRead = msg.detailEventRead
	m.detailContextRead = msg.detailContextRead
	m.detailDaemonTokenReads = msg.detailDaemonTokenReads
	m.detailSessionCoverage = msg.detailSessionCoverage
}

// syncViewportContent renders either the picker or the one stacked kanban.
func (m *PropsModel) syncViewportContent() {
	if !m.ready {
		return
	}
	if m.pickerOpen {
		m.viewport.SetContent(m.renderPicker())
		return
	}
	m.viewport.SetContent(m.renderBody())
}

func (m PropsModel) updatePicker(msg tea.KeyPressMsg) (PropsModel, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+t":
		m.pickerOpen = false
		m.syncViewportContent()
	case "up", "k":
		if m.pickerIdx > 0 {
			m.pickerIdx--
			m.syncViewportContent()
		}
	case "down", "j":
		if m.pickerIdx < len(m.agentNodes)-1 {
			m.pickerIdx++
			m.syncViewportContent()
		}
	case "enter":
		if m.pickerIdx < len(m.agentNodes) {
			m.selectedDir = m.agentNodes[m.pickerIdx].WorkingDir
			m.selectedStatus = fs.AgentStatus{}
			// Never label the previous agent's cached facts as the new selection
			// while its asynchronous snapshots are still in flight.
			m.applySnapshot(propsLoadMsg{})
		}
		m.pickerOpen = false
		m.syncViewportContent()
		return m, m.issueSnapshot()
	}
	return m, nil
}

type propsField struct {
	key   string
	label string
}

const (
	kanbanLLMIdentityLimit     = 256
	kanbanLLMSettingLimit      = 64
	kanbanBaseURLLimit         = 2048
	kanbanCompactEndpointLimit = 64
)

// kanbanLLMConfig is the presentation-local, secret-free LLM identity used by
// /kanban. Runtime-safe fields prefer the kernel-materialized llm block in
// .agent.json (the same source Telegram's automatic Task Card reads) and fall
// back field-by-field to the resolved/init manifest. Thinking uses the same
// resolver for compatibility but normally falls back because today's runtime
// safelist does not publish it.
type kanbanLLMConfig struct {
	Model        string
	Provider     string
	BaseURL      string
	Endpoint     string
	ServiceTier  string
	Thinking     string
	APICompat    string
	APIKeyEnv    string
	Streaming    string
	ContextLimit string
}

type kanbanPresetInfo struct {
	DefaultRef string
	ActiveRef  string
	Allowed    []string
}

func boundedKanbanText(v any, limit int) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" || len([]rune(s)) > limit || strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", false
	}
	return s, true
}

func boundedKanbanField(raw map[string]any, key string, limit int) (value string, present bool) {
	v, present := raw[key]
	if !present {
		return "", false
	}
	value, _ = boundedKanbanText(v, limit)
	return value, true
}

// llmTextField distinguishes an omitted field (present=false) from a present
// malformed value (present=true, value=""). A malformed materialized value is
// omitted rather than replaced with potentially stale manifest data.
func llmTextField(raw map[string]any, key string, limit int) (value string, present bool) {
	block, blockPresent := raw["llm"]
	if !blockPresent {
		return "", false
	}
	llm, ok := block.(map[string]any)
	if !ok {
		return "", true
	}
	return boundedKanbanField(llm, key, limit)
}

func resolvedKanbanLLMText(agentRaw, initRaw map[string]any, key string, limit int) string {
	if value, present := llmTextField(agentRaw, key, limit); present {
		return value
	}
	if value, present := llmTextField(initRaw, key, limit); present {
		return value
	}
	value, _ := boundedKanbanField(initRaw, key, limit)
	return value
}

func boundedKanbanScalar(v any, limit int) (string, bool) {
	switch value := v.(type) {
	case string:
		return boundedKanbanText(value, limit)
	case bool, float64, float32, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return boundedKanbanText(fmt.Sprint(value), limit)
	default:
		return "", false
	}
}

func llmScalarField(raw map[string]any, key string, limit int) (value string, present bool) {
	block, blockPresent := raw["llm"]
	if !blockPresent {
		return "", false
	}
	llm, ok := block.(map[string]any)
	if !ok {
		return "", true
	}
	v, present := llm[key]
	if !present {
		return "", false
	}
	switch number := v.(type) {
	case float64, float32, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		value, _ = boundedKanbanText(fmt.Sprint(number), limit)
	}
	return value, true
}

func resolvedKanbanLLMScalar(agentRaw, initRaw map[string]any, key string, limit int) string {
	if value, present := llmScalarField(agentRaw, key, limit); present {
		return value
	}
	if value, present := llmScalarField(initRaw, key, limit); present {
		return value
	}
	value, _ := boundedKanbanScalar(initRaw[key], limit)
	return value
}

func manifestLLMScalar(raw map[string]any, key string, limit int) string {
	if block, ok := raw["llm"].(map[string]any); ok {
		value, _ := boundedKanbanScalar(block[key], limit)
		return value
	}
	value, _ := boundedKanbanScalar(raw[key], limit)
	return value
}

// parseKanbanBaseURL validates once and derives a bounded, host-only summary
// endpoint. Malformed URLs and userinfo are omitted rather than exposed.
func parseKanbanBaseURL(raw string) (baseURL, endpoint string) {
	baseURL, ok := boundedKanbanText(raw, kanbanBaseURLLimit)
	if !ok {
		return "", ""
	}
	u, err := url.ParseRequestURI(baseURL)
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.User != nil {
		return "", ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	return baseURL, "@" + truncate(host, kanbanCompactEndpointLimit)
}

func resolveKanbanLLMConfig(agentRaw, initRaw map[string]any) kanbanLLMConfig {
	baseURL, endpoint := parseKanbanBaseURL(resolvedKanbanLLMText(agentRaw, initRaw, "base_url", kanbanBaseURLLimit))
	return kanbanLLMConfig{
		Model:        resolvedKanbanLLMText(agentRaw, initRaw, "model", kanbanLLMIdentityLimit),
		Provider:     resolvedKanbanLLMText(agentRaw, initRaw, "provider", kanbanLLMIdentityLimit),
		BaseURL:      baseURL,
		Endpoint:     endpoint,
		ServiceTier:  resolvedKanbanLLMText(agentRaw, initRaw, "service_tier", kanbanLLMSettingLimit),
		Thinking:     resolvedKanbanLLMText(agentRaw, initRaw, "thinking", kanbanLLMSettingLimit),
		APICompat:    resolvedKanbanLLMText(agentRaw, initRaw, "api_compat", kanbanLLMSettingLimit),
		APIKeyEnv:    manifestLLMScalar(initRaw, "api_key_env", kanbanLLMIdentityLimit),
		Streaming:    manifestLLMScalar(initRaw, "streaming", kanbanLLMSettingLimit),
		ContextLimit: resolvedKanbanLLMScalar(agentRaw, initRaw, "context_limit", kanbanLLMSettingLimit),
	}
}

func mergeKanbanAgentRaw(agentRaw, initRaw map[string]any) map[string]any {
	merged := make(map[string]any, len(agentRaw)+len(initRaw))
	for key, value := range initRaw {
		merged[key] = value
	}
	for key, value := range agentRaw {
		merged[key] = value
	}
	return merged
}

// resolveKanbanPresetInfo presents only configured manifest references. It
// deliberately does not resolve keys, auth, OAuth pools, provider eligibility,
// or preset files, and therefore makes no health/availability claim.
func resolveKanbanPresetInfo(raw map[string]any) kanbanPresetInfo {
	block, _ := raw["preset"].(map[string]any)
	info := kanbanPresetInfo{}
	if block == nil {
		return info
	}
	info.DefaultRef, _ = block["default"].(string)
	info.ActiveRef, _ = block["active"].(string)
	switch allowed := block["allowed"].(type) {
	case []any:
		for _, entry := range allowed {
			if ref, ok := entry.(string); ok && strings.TrimSpace(ref) != "" && len(info.Allowed) < fs.KanbanReadLimit {
				info.Allowed = append(info.Allowed, ref)
			}
		}
	case []string:
		for _, ref := range allowed {
			if strings.TrimSpace(ref) != "" && len(info.Allowed) < fs.KanbanReadLimit {
				info.Allowed = append(info.Allowed, ref)
			}
		}
	}
	return info
}

func (m PropsModel) renderBody() string {
	// Exact important-first order: Agent now, Current session (both emitted by
	// renderLeft), Network now, then every non-duplicated diagnostic section.
	return strings.Join([]string{
		m.renderLeft(m.width),
		m.renderRight(m.width),
		m.renderDetail(),
	}, "\n")
}

func (m PropsModel) View() string {
	header := StyleTitle.Render("  "+i18n.T("props.title")) + "\n" + strings.Repeat("\u2500", m.width)

	scrollHint := ""
	if m.ready && !m.viewport.AtBottom() {
		scrollHint = " " + RuneBullet + " ↑↓ scroll"
	}

	refreshHint := i18n.T("props.ctrl_r_reload")

	footerLine := "  " + refreshHint + " " + RuneBullet +
		" " + i18n.T("hints.props_off") + " " + RuneBullet +
		" esc " + i18n.T("manage.back") + " " + RuneBullet +
		" " + i18n.T("hints.props_select") + scrollHint
	footer := strings.Repeat("\u2500", m.width) + "\n" + StyleFaint.Render(footerLine)

	return header + "\n" + PaintViewportBG(m.viewport.View(), m.width) + "\n" + footer
}

func padToWidth(s string, w int) string {
	visible := lipgloss.Width(s)
	if visible >= w {
		return s
	}
	return s + strings.Repeat(" ", w-visible)
}

func (m PropsModel) renderLeft(maxW int) string {
	labelStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	var lines []string

	raw := m.selectedRaw
	llm := m.selectedLLM
	presetInfo := m.selectedPresetInfo
	selectedNode, hasSelectedNode := m.selectedAgentNode()

	renderFields := func(fields []propsField) {
		for _, f := range fields {
			v, ok := raw[f.key]
			if hasSelectedNode {
				switch f.key {
				case "agent_name":
					if selectedNode.AgentName != "" {
						v, ok = selectedNode.AgentName, true
					}
				case "nickname":
					if selectedNode.Nickname != "" {
						v, ok = selectedNode.Nickname, true
					}
				case "state":
					if selectedNode.State != "" {
						v, ok = selectedNode.State, true
					}
				}
			}
			if !ok || v == nil {
				continue
			}
			val := fmt.Sprintf("%v", v)
			if val == "" {
				continue
			}
			if f.key == "state" {
				stateColor := StateColor(strings.ToUpper(val))
				val = lipgloss.NewStyle().Foreground(stateColor).Render(val)
			} else {
				if isTimestampPropField(f.key) {
					val = formatKanbanTimestamp(val)
				}
				val = valueStyle.Render(val)
			}
			lines = append(lines, "  "+labelStyle.Render(f.label+": ")+val)
		}
	}
	appendRow := func(label, value string) {
		if value != "" {
			lines = append(lines, "  "+labelStyle.Render(label+": ")+valueStyle.Render(value))
		}
	}

	lines = append(lines, "", "  "+sectionStyle.Render(i18n.T("props.section_agent_now")), "")
	if len(raw) == 0 && !hasSelectedNode {
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.no_data")))
	}
	renderFields([]propsField{
		{"agent_name", i18n.T("props.name")},
		{"nickname", i18n.T("props.nickname")},
		{"state", i18n.T("props.state")},
	})
	llmParts := make([]string, 0, 2)
	if llm.Model != "" {
		llmParts = append(llmParts, llm.Model)
	}
	if llm.Provider != "" {
		llmParts = append(llmParts, llm.Provider)
	}
	appendRow(i18n.T("props.llm_identity"), strings.Join(llmParts, " · "))
	appendRow(i18n.T("props.service_tier"), llm.ServiceTier)
	appendRow(i18n.T("props.thinking_effort"), llm.Thinking)
	appendRow(i18n.T("props.endpoint"), llm.Endpoint)
	appendRow(i18n.T("props.preset_active"), refDisplayName(presetInfo.ActiveRef))
	if len(presetInfo.Allowed) > 0 {
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.preset_allowed")+": ")+
			valueStyle.Render(i18n.TF("props.preset_configured_count", len(presetInfo.Allowed))))
	}

	// Context window (from cached .status.json)
	ctx := m.selectedStatus.Tokens.Context
	if ctx.WindowSize > 0 {
		pctColor := ColorAgent
		if ctx.UsagePct > 80 {
			pctColor = lipgloss.Color("#e06c75")
		} else if ctx.UsagePct > 60 {
			pctColor = lipgloss.Color("#e5c07b")
		}
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.context_usage")+": ")+lipgloss.NewStyle().Foreground(pctColor).Render(
			fmt.Sprintf("%s / %s (%.1f%%)", formatComma(int64(ctx.TotalTokens)), formatComma(int64(ctx.WindowSize)), ctx.UsagePct)))
	}

	// Current session: the complete since-molt economy tuple from .status.json.
	// The primary deliberately shows an absolute cache miss only; no default
	// cache-miss budget exists in the published status/manifest contract.
	session := m.selectedStatus.Tokens
	lines = append(lines, "", "  "+sectionStyle.Render(i18n.T("props.section_current_session")))
	if session.InputTokens > 0 || session.OutputTokens > 0 || session.ThinkingTokens > 0 || session.CachedTokens > 0 || session.APICalls > 0 {
		tokens := session.InputTokens + session.OutputTokens + session.ThinkingTokens
		if session.Estimated {
			warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b"))
			lines = append(lines, "  "+warnStyle.Render(i18n.T("props.estimated_usage_warning")))
		}
		lines = append(lines, "")
		if session.InputTokens > 0 {
			appendRow(i18n.T("props.session_input_tokens"), formatComma(session.InputTokens))
			appendRow(i18n.T("props.session_cache_hit_rate"), formatCacheRate(session.CachedTokens, session.InputTokens))
			appendRow(i18n.T("props.session_cache_miss"), formatComma(cacheMiss(session.CachedTokens, session.InputTokens)))
		}
		appendRow(i18n.T("props.session_api_calls"), formatComma(session.APICalls))
		if session.APICalls > 0 {
			appendRow(i18n.T("props.session_tokens_per_api_call"), formatComma(avgPerCall(tokens, session.APICalls)))
		}
	} else {
		lines = append(lines, "", "  "+labelStyle.Render(i18n.T("props.no_data")))
	}

	return strings.Join(lines, "\n")
}

func (m PropsModel) selectedAgentNode() (fs.AgentNode, bool) {
	for _, node := range m.agentNodes {
		if node.WorkingDir == m.selectedDir {
			return node, true
		}
	}
	return fs.AgentNode{}, false
}

func combineKanbanSourceStates(states ...string) string {
	counts := map[string]int{}
	total := 0
	for _, state := range states {
		if state != "" {
			counts[state]++
			total++
		}
	}
	if len(counts) == 0 {
		return ""
	}
	if counts[fs.KanbanSourcePartial] > 0 {
		return fs.KanbanSourcePartial
	}
	bad := counts[fs.KanbanSourceUnavailable] + counts[fs.KanbanSourceMalformed]
	if bad > 0 && bad < total {
		return fs.KanbanSourcePartial
	}
	if counts[fs.KanbanSourceUnavailable] > 0 {
		if len(counts) == 1 {
			return fs.KanbanSourceUnavailable
		}
		return fs.KanbanSourcePartial
	}
	if counts[fs.KanbanSourceMalformed] > 0 {
		if len(counts) == 1 {
			return fs.KanbanSourceMalformed
		}
		return fs.KanbanSourcePartial
	}
	if counts[fs.KanbanSourceRecent] > 0 {
		return fs.KanbanSourceRecent
	}
	if counts[fs.KanbanSourceAvailable] > 0 {
		return fs.KanbanSourceAvailable
	}
	return fs.KanbanSourceEmpty
}

func combineKanbanReads(reads []fs.BoundedReadStats) string {
	states := make([]string, 0, len(reads))
	for _, read := range reads {
		states = append(states, fs.KanbanReadState(read))
	}
	return combineKanbanSourceStates(states...)
}

func kanbanSourceTruth(state, source, empty string) string {
	switch state {
	case fs.KanbanSourceUnavailable:
		return i18n.TF("props.window_unavailable", source)
	case fs.KanbanSourceMalformed:
		return i18n.TF("props.window_malformed", source)
	case fs.KanbanSourcePartial:
		return i18n.TF("props.window_partial", source)
	case fs.KanbanSourceRecent:
		return i18n.TF("props.window_recent", source)
	case fs.KanbanSourceAvailable, fs.KanbanSourceEmpty, "":
		return empty
	default:
		return ""
	}
}

func tokenTotalsEmpty(t fs.TokenTotals) bool {
	return t.Input == 0 && t.Output == 0 && t.Thinking == 0 && t.Cached == 0 && t.APICalls == 0
}

func (m PropsModel) renderRight(maxW int) string {
	labelStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	var lines []string

	lines = append(lines, "", "  "+sectionStyle.Render(i18n.T("props.section_network_now")), "")

	stats := m.network.Stats
	totalAgents := len(m.network.Nodes)
	var humanCount, agentCount int
	for _, n := range m.network.Nodes {
		if n.IsHuman {
			humanCount++
		} else {
			agentCount++
		}
	}
	totalLabel := i18n.T("props.network_total")
	if m.network.AgentDirectoryTruncated {
		totalLabel = i18n.T("props.network_bounded_subset")
	}
	lines = append(lines, "  "+labelStyle.Render(totalLabel+": ")+
		valueStyle.Render(fmt.Sprintf("%d", totalAgents))+
		labelStyle.Render(fmt.Sprintf("  (%d %s, %d %s)",
			agentCount, i18n.T("props.network_agents"), humanCount, i18n.T("props.network_humans"))))
	if m.network.AgentDirectoryTruncated {
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.network_subset_notice")))
	}

	var stateParts []string
	if stats.Active > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("ACTIVE"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.active"), stats.Active)))
	}
	if stats.Idle > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("IDLE"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.idle"), stats.Idle)))
	}
	if stats.Stuck > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("STUCK"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.stuck"), stats.Stuck)))
	}
	if stats.Asleep > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("ASLEEP"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.asleep"), stats.Asleep)))
	}
	if stats.Suspended > 0 {
		c := lipgloss.NewStyle().Foreground(StateColor("SUSPENDED"))
		stateParts = append(stateParts, c.Render(fmt.Sprintf("%s: %d", i18n.T("state.suspended"), stats.Suspended)))
	}
	if len(stateParts) > 0 {
		stateLine := strings.Join(stateParts, "  ")
		if m.network.AgentDirectoryTruncated {
			stateLine = labelStyle.Render(i18n.T("props.network_subset_states")+": ") + stateLine
		}
		lines = append(lines, "  "+stateLine)
	}
	if m.network.Activity.Status != "" && !m.network.AgentDirectoryTruncated {
		c := lipgloss.NewStyle().Foreground(NetworkActivityColor(m.network.Activity.Status))
		lines = append(lines, "  "+labelStyle.Render(networkActivityLabel()+": ")+c.Render(networkActivityStatusLabel(m.network.Activity.Status)))
	}
	lines = append(lines, "  "+labelStyle.Render(i18n.T("props.network_daemons")+": ")+
		valueStyle.Render(fmt.Sprintf("%d %s", m.network.Activity.RunningDaemons, i18n.T("props.network_daemons_running"))))

	return strings.Join(lines, "\n")
}

// renderDetail renders the retained lower diagnostic sections in the same
// scrollable pane. Despite the historical helper name, it is not a separate
// mode and performs no filesystem/config/auth reads.
func (m PropsModel) renderDetail() string {
	labelStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	subtleStyle := lipgloss.NewStyle().Foreground(ColorTextFaint)

	var lines []string

	raw := m.selectedRaw
	llm := m.selectedLLM
	presetInfo := m.selectedPresetInfo

	appendSection := func(title string) {
		lines = append(lines, "", "  "+sectionStyle.Render(title), "")
	}
	appendDetailRow := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		lines = append(lines, "  "+labelStyle.Render(label+": ")+valueStyle.Render(value))
	}
	appendRawRow := func(key, label string) {
		v, ok := raw[key]
		if !ok || v == nil {
			return
		}
		value := fmt.Sprintf("%v", v)
		if isTimestampPropField(key) {
			value = formatKanbanTimestamp(value)
		}
		appendDetailRow(label, value)
	}

	appendSection(i18n.T("props.detail_identity_runtime"))
	appendRawRow("agent_id", i18n.T("props.id"))
	appendRawRow("address", i18n.T("props.address"))
	appendRawRow("language", i18n.T("props.language"))
	appendRawRow("started_at", i18n.T("props.started_at"))
	appendRawRow("combo", i18n.T("props.combo"))
	appendRawRow("soul_delay", i18n.T("props.soul_flow"))
	appendRawRow("molt_count", i18n.T("props.molt_count"))
	appendRawRow("max_turns", i18n.T("props.max_turns"))
	appendRawRow("max_rpm", i18n.T("props.max_rpm"))
	appendDetailRow(i18n.T("props.detail_agent_path"), m.selectedDir)
	appendDetailRow(i18n.T("props.detail_network_path"), m.baseDir)
	if m.orchDir != "" && m.orchDir != m.selectedDir {
		appendDetailRow(i18n.T("props.detail_orchestrator_path"), m.orchDir)
	}

	appendSection(i18n.T("props.detail_llm_configuration"))
	appendDetailRow(i18n.T("props.base_url"), llm.BaseURL)
	appendDetailRow(i18n.T("props.api_compat"), llm.APICompat)
	appendDetailRow(i18n.T("props.api_key_env"), llm.APIKeyEnv)
	appendDetailRow(i18n.T("props.streaming"), llm.Streaming)
	appendDetailRow(i18n.T("props.context_limit"), llm.ContextLimit)

	appendSection(i18n.T("props.detail_presets_capabilities_admin"))
	appendDetailRow(i18n.T("props.preset_default"), refDisplayName(presetInfo.DefaultRef))
	if len(presetInfo.Allowed) > 0 {
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.preset_allowed")+":"))
		for _, ref := range presetInfo.Allowed {
			lines = append(lines, "    "+valueStyle.Render(refDisplayName(ref)))
		}
		lines = append(lines, "    "+subtleStyle.Render(i18n.T("props.preset_refs_not_checked")))
	}
	if caps, ok := raw["capabilities"]; ok && caps != nil {
		capsJSON, _ := json.Marshal(caps)
		capNames := fs.CapabilitiesForDisplay(fs.ParseCapabilities(capsJSON))
		if len(capNames) > 0 {
			lines = append(lines, "  "+labelStyle.Render(i18n.T("props.section_capabilities")+":"))
			wrapWidth := 74
			if m.width > 0 {
				wrapWidth = max(1, m.width-6)
			}
			wrapped := lipgloss.NewStyle().Width(wrapWidth).Render(strings.Join(capNames, ", "))
			for _, line := range strings.Split(wrapped, "\n") {
				lines = append(lines, "    "+valueStyle.Render(line))
			}
		}
	}
	if adminMap, ok := raw["admin"].(map[string]any); ok && len(adminMap) > 0 {
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.section_admin")+":"))
		keys := make([]string, 0, len(adminMap))
		for key := range adminMap {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, "    "+valueStyle.Render(fmt.Sprintf("%s: %v", key, adminMap[key])))
		}
	}

	appendSection(i18n.T("props.detail_context_detail"))
	ctx := m.selectedStatus.Tokens.Context
	if ctx.WindowSize > 0 {
		appendDetailRow(i18n.T("props.context_system"), formatComma(int64(ctx.SystemTokens)))
		appendDetailRow(i18n.T("props.context_tools"), formatComma(int64(ctx.ToolsTokens)))
		appendDetailRow(i18n.T("props.context_history"), formatComma(int64(ctx.HistoryTokens)))
	}
	contextState := fs.KanbanReadState(m.detailContextRead)
	if notice := kanbanSourceTruth(contextState, i18n.T("props.source_context_history"), i18n.T("props.context_window_empty")); notice != "" && contextState != fs.KanbanSourceAvailable {
		lines = append(lines, "  "+subtleStyle.Render(notice))
	}
	if m.detailContextStats.Entries > 0 {
		stats := m.detailContextStats
		lines = append(lines, "    "+labelStyle.Render("entries:                  ")+
			valueStyle.Render(fmt.Sprintf("%d", stats.Entries)))
		lines = append(lines, "    "+labelStyle.Render("messages:                 ")+
			valueStyle.Render(fmt.Sprintf("system:%d  assistant:%d  user:%d", stats.SystemMessages, stats.AssistantMessages, stats.UserMessages)))
		lines = append(lines, "    "+labelStyle.Render("text input / output:      ")+
			valueStyle.Render(fmt.Sprintf("%d / %d", stats.TextInputs, stats.TextOutputs)))
		lines = append(lines, "    "+labelStyle.Render("tool calls / results:     ")+
			valueStyle.Render(fmt.Sprintf("%d / %d", stats.ToolCalls, stats.ToolResults)))
		if len(stats.ToolCounts) > 0 {
			lines = append(lines, "", "    "+labelStyle.Render("tools in recent context:"))
			for _, tc := range stats.ToolCounts {
				lines = append(lines, fmt.Sprintf("      %-14s calls:%s  results:%s",
					valueStyle.Render(tc.Name), formatComma(int64(tc.Calls)), formatComma(int64(tc.Results))))
			}
		}
	}

	// Selected-agent token/provider/session data all come from one bounded
	// 1000-line token tail and one bounded 1000-line event tail.
	appendSection(i18n.T("props.detail_recent_token_usage"))
	selectedTokenState := fs.KanbanReadState(m.selectedTokenRead)
	if notice := kanbanSourceTruth(selectedTokenState, i18n.T("props.source_token_ledger"), i18n.T("props.detail_no_tokens")); notice != "" && selectedTokenState != fs.KanbanSourceAvailable {
		lines = append(lines, "  "+subtleStyle.Render(notice))
	}
	if m.selectedTokens.APICalls > 0 {
		appendDetailRow("input", formatComma(m.selectedTokens.Input))
		appendDetailRow("output", formatComma(m.selectedTokens.Output))
		appendDetailRow("thinking", formatComma(m.selectedTokens.Thinking))
		cached := formatComma(m.selectedTokens.Cached)
		if m.selectedTokens.Input > 0 {
			cached += fmt.Sprintf(" (%.1f%%)", 100.0*float64(m.selectedTokens.Cached)/float64(m.selectedTokens.Input))
		}
		appendDetailRow("cached", cached)
		appendDetailRow("api_calls", formatComma(m.selectedTokens.APICalls))
	}
	lines = append(lines, "", "  "+sectionStyle.Render(i18n.T("props.detail_main_tokens_by_provider")), "")
	mainEmpty := kanbanSourceTruth(selectedTokenState, i18n.T("props.source_token_ledger"), i18n.T("props.detail_no_tokens"))
	lines = appendProviderRows(lines, m.detailByProvider, mainEmpty, labelStyle, valueStyle, subtleStyle)
	sessionCoverage := m.detailSessionCoverage
	if m.detailEventRead.Status == "" && (m.detailCurrentSessionStats.APICalls > 0 || m.detailLastSessionStats.APICalls > 0) {
		// Compatibility for explicitly constructed in-memory presentation tests;
		// real snapshots always carry detailEventRead.
		sessionCoverage = true
	}
	if sessionCoverage {
		lines = appendCurrentSessionDiagnostics(lines, m.detailCurrentSessionStats, m.detailCurrentSessionToolCalls, sectionStyle, labelStyle, valueStyle)
		lines = appendSessionAPIStats(lines, i18n.T("props.detail_last_session_recent"), m.detailLastSessionStats, m.detailLastSessionToolCalls, sectionStyle, labelStyle, valueStyle)
	} else if m.selectedTokens.APICalls > 0 {
		eventState := fs.KanbanReadState(m.detailEventRead)
		if notice := kanbanSourceTruth(eventState, i18n.T("props.source_event_log"), ""); notice != "" {
			lines = append(lines, "  "+subtleStyle.Render(notice))
		}
		lines = append(lines, "  "+subtleStyle.Render(i18n.T("props.session_partitions_omitted")), "")
	}

	// Network totals aggregate at most the recent 1000 token-ledger lines per
	// bounded agent, and topology/contact sources use the same 1000 profile.
	appendSection(i18n.T("props.detail_network_history_topology"))
	if m.adminStart != "" {
		appendDetailRow(i18n.T("props.network_created"), formatKanbanTimestamp(m.adminStart))
		if started, err := time.Parse(time.RFC3339, m.adminStart); err == nil {
			appendDetailRow(i18n.T("props.network_uptime"), formatDuration(time.Since(started)))
		}
	}
	lines = append(lines, "  "+labelStyle.Render(i18n.T("props.total_tokens")+":"))
	networkTokenState := combineKanbanReads(m.networkTokenReads)
	if notice := kanbanSourceTruth(networkTokenState, i18n.T("props.source_network_token_ledgers"), i18n.T("props.detail_no_tokens")); notice != "" && networkTokenState != fs.KanbanSourceAvailable {
		lines = append(lines, "    "+subtleStyle.Render(notice))
	}
	if networkTokenState != fs.KanbanSourceUnavailable && networkTokenState != fs.KanbanSourceMalformed || !tokenTotalsEmpty(m.tokens) {
		lines = append(lines, "    "+labelStyle.Render("Input:    ")+valueStyle.Render(formatComma(m.tokens.Input)))
		lines = append(lines, "    "+labelStyle.Render("Output:   ")+valueStyle.Render(formatComma(m.tokens.Output)))
		lines = append(lines, "    "+labelStyle.Render("Thinking: ")+valueStyle.Render(formatComma(m.tokens.Thinking)))
		networkCached := formatComma(m.tokens.Cached)
		if m.tokens.Input > 0 {
			networkCached += fmt.Sprintf(" (%.1f%%)", 100.0*float64(m.tokens.Cached)/float64(m.tokens.Input))
		}
		lines = append(lines, "    "+labelStyle.Render("Cached:   ")+valueStyle.Render(networkCached))
		appendDetailRow(i18n.T("props.total_api_calls"), formatComma(m.tokens.APICalls))
	}
	if m.mailStatsAvailable {
		appendDetailRow(i18n.T("props.total_mails"), fmt.Sprintf("%d", m.network.Stats.TotalMails))
	}
	tree := m.renderTree(m.width)
	if len(tree) > 0 {
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.tree")+":"))
		lines = append(lines, tree...)
	}

	// Configured MCP names and daemon facts share the lower recent daemon
	// window. No daemon directory listing is represented as a whole-directory total.
	appendSection(i18n.T("props.detail_daemons"))
	if len(m.detailMCPNames) > 0 {
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.detail_mcp")+":"))
		for _, name := range m.detailMCPNames {
			lines = append(lines, "    "+valueStyle.Render(name))
		}
	}
	switch m.detailDaemonWindowState {
	case fs.KanbanSourceAvailable, fs.KanbanSourceRecent, fs.KanbanSourcePartial:
		if notice := kanbanSourceTruth(m.detailDaemonWindowState, i18n.T("props.source_dispatch_ledger"), ""); notice != "" {
			lines = append(lines, "  "+subtleStyle.Render(notice))
		}
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.detail_daemons_running")+": ")+
			valueStyle.Render(fmt.Sprintf("%d", m.detailDaemonCounts.Running)))
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.detail_daemons_terminal")+": ")+
			valueStyle.Render(fmt.Sprintf("%d", m.detailDaemonRunsTerminal)))
		lines = append(lines, "  "+labelStyle.Render(i18n.T("props.detail_daemons_total")+": ")+
			valueStyle.Render(fmt.Sprintf("%d", m.detailDaemonCounts.Total)))
	case fs.KanbanSourceEmpty, "":
		lines = append(lines, "  "+subtleStyle.Render(i18n.T("props.daemon_window_empty")))
	case fs.KanbanSourceMalformed:
		lines = append(lines, "  "+subtleStyle.Render(i18n.TF("props.window_malformed", i18n.T("props.source_dispatch_ledger"))))
	default:
		lines = append(lines, "  "+subtleStyle.Render(i18n.TF("props.window_unavailable", i18n.T("props.source_dispatch_ledger"))))
	}
	lines = append(lines, "", "  "+sectionStyle.Render(i18n.T("props.detail_daemon_tokens_by_provider")), "")
	daemonStates := []string{m.detailDaemonWindowState}
	for _, read := range m.detailDaemonTokenReads {
		daemonStates = append(daemonStates, fs.KanbanReadState(read))
	}
	daemonTokenState := combineKanbanSourceStates(daemonStates...)
	daemonEmpty := kanbanSourceTruth(daemonTokenState, i18n.T("props.source_daemon_ledgers"), i18n.T("props.detail_no_tokens"))
	lines = appendProviderRows(lines, m.detailDaemonByProvider, daemonEmpty, labelStyle, valueStyle, subtleStyle)

	// Combined recent-window totals: arithmetic sum of the selected main-agent
	// tail and the per-run tails selected by the recent dispatch ledger.
	combined := make(map[string]fs.TokenTotals)
	for name, t := range m.detailByProvider {
		combined[name] = t
	}
	for name, t := range m.detailDaemonByProvider {
		c := combined[name]
		c.Input += t.Input
		c.Output += t.Output
		c.Thinking += t.Thinking
		c.Cached += t.Cached
		c.APICalls += t.APICalls
		combined[name] = c
	}
	if len(combined) > 0 {
		lines = append(lines, "")
		lines = append(lines, "  "+sectionStyle.Render(i18n.T("props.detail_combined_totals")))
		lines = append(lines, "")
		var tot fs.TokenTotals
		for _, t := range combined {
			tot.Input += t.Input
			tot.Output += t.Output
			tot.Thinking += t.Thinking
			tot.Cached += t.Cached
			tot.APICalls += t.APICalls
		}
		lines = append(lines, "    "+labelStyle.Render("input + output + thinking: ")+
			valueStyle.Render(formatComma(tot.Input+tot.Output+tot.Thinking)))
		lines = append(lines, "    "+labelStyle.Render("cached:                    ")+
			valueStyle.Render(formatComma(tot.Cached)))
		lines = append(lines, "    "+labelStyle.Render("miss:                      ")+
			valueStyle.Render(formatComma(cacheMiss(tot.Cached, tot.Input))))
		lines = append(lines, "    "+labelStyle.Render("api_calls:                 ")+
			valueStyle.Render(fmt.Sprintf("%d", tot.APICalls)))
		if tot.Input > 0 {
			lines = append(lines, "    "+labelStyle.Render("cache hit rate:            ")+
				valueStyle.Render(fmt.Sprintf("%.1f%%", 100.0*float64(tot.Cached)/float64(tot.Input))))
		}
		lines = append(lines, "")
	}

	// Raw recent token-ledger lanes are useful for diagnosis but visually noisy,
	// so they come last after the higher-signal provider totals, context, MCP,
	// and daemon-count summaries.
	lines = append(lines, m.renderRecentCallLanes()...)

	return strings.Join(lines, "\n")
}

// renderRecentCallLanes renders the lower diagnostic ledger section in a
// single-column order: selected main-agent calls first, then stacked daemon
// calls. Raw ledgers are intentionally below the higher-signal summaries in
// the lower diagnostic renderer.
func (m PropsModel) renderRecentCallLanes() []string {
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+sectionStyle.Render(i18n.T("props.detail_recent_main")))
	lines = append(lines, "")
	lines = append(lines, m.renderMainCallRows()...)
	lines = append(lines, "")
	lines = append(lines, "  "+sectionStyle.Render(i18n.T("props.detail_recent_daemons")))
	lines = append(lines, "")
	lines = append(lines, m.renderDaemonCallRows()...)
	lines = append(lines, "")
	return lines
}

const (
	ledgerSeparatorReconstructLabel = "props.detail_recent_rebuild"
	ledgerSeparatorMoltLabel        = "props.detail_recent_molt"
)

// ledgerSeparatorLabelKeys maps each ledger row index (in a newest-first
// entries slice) to the dotted separator label(s) that should be drawn after
// that row.
//
// Three boundary sources, all rendered after the newer (earlier-in-slice) row:
//   - Reconstructed-context token-ledger rows — the first call after a context
//     reconstruction (codex_ws_delta_reason="epoch_reset"/"no_baseline" or
//     codex_transfer_mode="full"). That row is itself the low-cache
//     reconstruct call, so the boundary belongs immediately after it. Labeled
//     "context rebuilt".
//   - refreshTimes (refresh_complete event timestamps) — also labeled
//     "context rebuilt", for refreshes that left no distinguishing ledger row.
//   - moltTimes (psyche_molt event timestamps) — a separate "molt" label.
//
// addLedgerSeparatorLabel deduplicates, so a refresh timestamp and a
// full/no_baseline ledger row that land on the same boundary yield a single
// "context rebuilt" label.
func ledgerSeparatorLabelKeys(entries []fs.LedgerEntry, moltTimes, refreshTimes []time.Time) map[int][]string {
	out := map[int][]string{}
	if len(entries) < 2 {
		return out
	}
	for i := 0; i+1 < len(entries); i++ {
		if isReconstructLedgerEntry(entries[i]) {
			addLedgerSeparatorLabel(out, i, ledgerSeparatorReconstructLabel)
		}
	}

	parsed := make([]time.Time, len(entries))
	ok := make([]bool, len(entries))
	for i, e := range entries {
		if t, err := time.Parse(time.RFC3339, e.TS); err == nil {
			parsed[i] = t
			ok[i] = true
		}
	}
	markBoundaryTimes(out, parsed, ok, refreshTimes, ledgerSeparatorReconstructLabel)
	markBoundaryTimes(out, parsed, ok, moltTimes, ledgerSeparatorMoltLabel)
	return out
}

// markBoundaryTimes adds labelKey after the newest row that straddles each
// boundary time: the row at/after the boundary whose next-older row is before
// it (rows are newest-first). Out-of-window boundaries are ignored.
func markBoundaryTimes(out map[int][]string, parsed []time.Time, ok []bool, times []time.Time, labelKey string) {
	for _, r := range times {
		for i := 0; i+1 < len(parsed); i++ {
			if !ok[i] || !ok[i+1] {
				continue
			}
			if !parsed[i].Before(r) && parsed[i+1].Before(r) {
				addLedgerSeparatorLabel(out, i, labelKey)
				break
			}
		}
	}
}

func addLedgerSeparatorLabel(out map[int][]string, idx int, labelKey string) {
	for _, existing := range out[idx] {
		if existing == labelKey {
			return
		}
	}
	out[idx] = append(out[idx], labelKey)
}

// rebuildSeparatorIndexes is kept as a small test/helper compatibility wrapper
// for callers that only care whether any separator exists after a row.
func rebuildSeparatorIndexes(entries []fs.LedgerEntry, rebuilds []time.Time) map[int]bool {
	labels := ledgerSeparatorLabelKeys(entries, rebuilds, nil)
	out := make(map[int]bool, len(labels))
	for idx := range labels {
		out[idx] = true
	}
	return out
}

func isReconstructLedgerEntry(entry fs.LedgerEntry) bool {
	switch strings.ToLower(strings.TrimSpace(entry.CodexWSDeltaReason)) {
	case "epoch_reset", "no_baseline", "reconstruct", "reconstructed", "context_rebuild", "context_reconstructed":
		return true
	}
	// A full provider transfer rebuilds the whole working set, which is what a
	// /refresh forces ("codex_transfer_mode=full" + "no_baseline"). Treat any
	// full transfer as a reconstruction boundary even when the delta reason is
	// absent or unrecognized.
	if strings.EqualFold(strings.TrimSpace(entry.CodexTransferMode), "full") {
		return true
	}
	return false
}

// renderMainCallRows renders the selected agent's recent per-call ledger
// entries (newest first). Rows deliberately avoid truncating model/endpoint
// fields so the lower ledger can preserve raw diagnostic evidence.
func (m PropsModel) renderMainCallRows() []string {
	subtleStyle := lipgloss.NewStyle().Foreground(ColorTextFaint)
	labelStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)

	if len(m.detailRecent) == 0 {
		state := fs.KanbanReadState(m.selectedTokenRead)
		message := kanbanSourceTruth(state, i18n.T("props.source_token_ledger"), i18n.T("props.detail_recent_empty"))
		return []string{"  " + subtleStyle.Render(message)}
	}

	headerLine := fmt.Sprintf("  %-24s  %-10s  %-24s  %10s  %10s  %10s  %10s  %10s  %7s  %s",
		"time", "provider", "model", "input", "output", "thinking", "cached", "miss", "cache%", "endpoint")
	lines := []string{"  " + labelStyle.Render(headerLine)}

	// Rows that start a reconstructed context, or that straddle a fallback
	// rebuild timestamp, get a faint dotted separator drawn after them.
	separators := ledgerSeparatorLabelKeys(m.detailRecent, m.detailRebuilds, m.detailRefreshes)
	sepWidth := lipgloss.Width(headerLine)

	for i, e := range m.detailRecent {
		provider := fs.DeriveLedgerProvider(e.Endpoint, e.Model)
		model := e.Model
		if model == "" {
			model = "—"
		}
		endpoint := e.Endpoint
		if endpoint == "" {
			endpoint = "—"
		}
		line := fmt.Sprintf("  %-24s  %-10s  %-24s  %10s  %10s  %10s  %10s  %10s  %7s  %s",
			shortTS(e.TS),
			provider,
			model,
			formatComma(e.Input),
			formatComma(e.Output),
			formatComma(e.Thinking),
			formatComma(e.Cached),
			formatComma(cacheMiss(e.Cached, e.Input)),
			formatCacheRate(e.Cached, e.Input),
			endpoint,
		)
		lines = append(lines, valueStyle.Render(line))
		for _, labelKey := range separators[i] {
			lines = append(lines, rebuildSeparatorLine(sepWidth, labelKey))
		}
	}
	return lines
}

// rebuildSeparatorLine renders the faint dotted boundary marking a context
// rebuild between two adjacent ledger calls.
func rebuildSeparatorLine(width int, labelKey string) string {
	if width < 8 {
		width = 8
	}
	faint := lipgloss.NewStyle().Foreground(ColorTextFaint)
	if labelKey == "" {
		labelKey = ledgerSeparatorReconstructLabel
	}
	label := i18n.T(labelKey)
	dots := width - lipgloss.Width(label) - 1
	if dots < 3 {
		return "  " + faint.Render(strings.Repeat("┈", width))
	}
	return "  " + faint.Render(strings.Repeat("┈", dots)+" "+label)
}

// renderDaemonCallRows renders all daemon per-call ledger entries (newest
// first), each row retaining daemon handle/state/backend/model/endpoint
// dimensions without truncation.
func (m PropsModel) renderDaemonCallRows() []string {
	subtleStyle := lipgloss.NewStyle().Foreground(ColorTextFaint)
	labelStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)

	if len(m.detailDaemonRecent) == 0 {
		states := []string{m.detailDaemonWindowState}
		for _, read := range m.detailDaemonTokenReads {
			states = append(states, fs.KanbanReadState(read))
		}
		state := combineKanbanSourceStates(states...)
		message := kanbanSourceTruth(state, i18n.T("props.source_daemon_ledgers"), i18n.T("props.detail_recent_daemons_empty"))
		return []string{"  " + subtleStyle.Render(message)}
	}

	lines := []string{
		"  " + labelStyle.Render(fmt.Sprintf("%-24s  %-10s  %-8s  %-10s  %-24s  %10s  %10s  %10s  %10s  %10s  %7s  %s",
			"time", "daemon", "state", "backend", "model", "input", "output", "thinking", "cached", "miss", "cache%", "endpoint")),
	}
	for _, e := range m.detailDaemonRecent {
		backend := e.Backend
		if backend == "" {
			backend = "—"
		}
		model := e.Model
		if model == "" {
			model = "—"
		}
		endpoint := e.Endpoint
		if endpoint == "" {
			endpoint = "—"
		}
		handle := e.Handle
		if handle == "" {
			handle = "—"
		}
		state := e.State
		if state == "" {
			state = "—"
		}
		line := fmt.Sprintf("  %-24s  %-10s  %-8s  %-10s  %-24s  %10s  %10s  %10s  %10s  %10s  %7s  %s",
			shortTS(e.TS),
			handle,
			state,
			backend,
			model,
			formatComma(e.Input),
			formatComma(e.Output),
			formatComma(e.Thinking),
			formatComma(e.Cached),
			formatComma(cacheMiss(e.Cached, e.Input)),
			formatCacheRate(e.Cached, e.Input),
			endpoint,
		)
		lines = append(lines, valueStyle.Render(line))
	}
	return lines
}

func avgPerCall(tokens, calls int64) int64 {
	if calls <= 0 {
		return 0
	}
	return (tokens + calls/2) / calls
}

func formatCacheRate(cached, input int64) string {
	if input <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", 100.0*float64(cached)/float64(input))
}

// cacheMiss is the absolute cache-miss token count for a ledger row: the input
// tokens NOT served from cache. input here is the true total input (raw +
// cache_read + cache_write, normalised per adapter), and cached is the
// cache_read portion, so the miss complement is input - cached. Clamped at 0 so
// rows with a missing/zero input (older ledger lines) never report a negative.
func cacheMiss(cached, input int64) int64 {
	miss := input - cached
	if miss < 0 {
		return 0
	}
	return miss
}

func isTimestampPropField(key string) bool {
	switch key {
	case "started_at", "created_at", "updated_at":
		return true
	default:
		return strings.HasSuffix(key, "_at") || strings.Contains(key, "timestamp")
	}
}

// formatKanbanTimestamp renders parseable timestamps in local time with an
// explicit UTC offset marker (for example, 2026-06-19 20:14 U-7:00).
// Non-parseable legacy strings keep the old compact trimming behavior.
func formatKanbanTimestamp(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		if len(ts) > 16 {
			return ts[:16]
		}
		return ts
	}
	local := t.Local()
	return local.Format("2006-01-02 15:04") + " " + utcOffsetLabel(local)
}

func utcOffsetLabel(t time.Time) string {
	_, offset := t.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hours := offset / 3600
	minutes := (offset % 3600) / 60
	return fmt.Sprintf("U%s%d:%02d", sign, hours, minutes)
}

// shortTS renders token-ledger timestamps for compact /kanban tables.
func shortTS(ts string) string {
	return formatKanbanTimestamp(ts)
}

// renderShareBar returns a small unicode bar (filled + empty cells)
// proportional to pct (0..100). width is the total cell count.
func renderShareBar(pct float64, width int) string {
	if width < 1 {
		width = 1
	}
	filled := int((pct / 100.0) * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	full := lipgloss.NewStyle().Foreground(ColorAccent).Render(strings.Repeat("█", filled))
	empty := lipgloss.NewStyle().Foreground(ColorTextFaint).Render(strings.Repeat("░", width-filled))
	return full + empty
}

// truncate trims s to n runes, appending "…" when shortened. Used to
// keep the recent-activity model column from overflowing.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func (m PropsModel) renderPicker() string {
	if len(m.agentNodes) == 0 {
		return ""
	}

	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(ColorText)
	selectedStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

	var lines []string
	lines = append(lines, "")
	lines = append(lines, "  "+sectionStyle.Render(i18n.T("props.select_agent")))
	lines = append(lines, "")

	for i, n := range m.agentNodes {
		name := n.AgentName
		if n.Nickname != "" {
			name = n.Nickname
		}
		if name == "" {
			name = "(unknown)"
		}

		state := n.State
		if state == "" {
			state = "──"
		}
		stateRendered := lipgloss.NewStyle().Foreground(StateColor(strings.ToUpper(state))).Render(state)

		marker := "  "
		style := nameStyle
		if n.WorkingDir == m.selectedDir {
			marker = "● "
		}
		if i == m.pickerIdx {
			style = selectedStyle
			marker = "> "
			if n.WorkingDir == m.selectedDir {
				marker = ">●"
			}
		}

		lines = append(lines, fmt.Sprintf("  %s%-18s %s", marker, style.Render(name), stateRendered))
	}

	lines = append(lines, "")
	lines = append(lines, "  "+StyleFaint.Render("↑↓ "+i18n.T("manage.select")+"  [enter]  [esc/ctrl+t] "+i18n.T("manage.back")))

	return strings.Join(lines, "\n")
}

func (m PropsModel) renderTree(maxW int) []string {
	nodes := m.network.Nodes
	edges := m.network.AvatarEdges
	if len(nodes) == 0 {
		return nil
	}

	nodeMap := make(map[string]fs.AgentNode)
	for _, n := range nodes {
		nodeMap[n.Address] = n
	}

	childrenOf := make(map[string][]string)
	childSet := make(map[string]bool)
	for _, e := range edges {
		childrenOf[e.Parent] = append(childrenOf[e.Parent], e.Child)
		childSet[e.Child] = true
	}

	// Roots: human first, then admins (no parent)
	var roots []fs.AgentNode
	for _, n := range nodes {
		if n.IsHuman {
			roots = append([]fs.AgentNode{n}, roots...)
		} else if !childSet[n.Address] {
			roots = append(roots, n)
		}
	}

	nameOf := func(n fs.AgentNode) string {
		if n.Nickname != "" {
			return n.Nickname
		}
		if n.AgentName != "" {
			return n.AgentName
		}
		parts := strings.Split(n.Address, "/")
		return parts[len(parts)-1]
	}

	var lines []string
	var walk func(addr, prefix string, isLast, isRoot bool)
	walk = func(addr, prefix string, isLast, isRoot bool) {
		n, ok := nodeMap[addr]
		if !ok {
			return
		}
		connector := ""
		if !isRoot {
			if isLast {
				connector = "└ "
			} else {
				connector = "├ "
			}
		}
		stateColor := StateColor(strings.ToUpper(n.State))
		name := lipgloss.NewStyle().Foreground(stateColor).Render(nameOf(n))
		dimPrefix := lipgloss.NewStyle().Foreground(ColorTextFaint).Render(prefix + connector)
		lines = append(lines, "  "+dimPrefix+name)

		children := childrenOf[addr]
		childPrefix := prefix
		if !isRoot {
			if isLast {
				childPrefix += "  "
			} else {
				childPrefix += "│ "
			}
		}
		for i, c := range children {
			walk(c, childPrefix, i == len(children)-1, false)
		}
	}

	for i, r := range roots {
		walk(r.Address, "", i == len(roots)-1, true)
	}
	return lines
}

func formatComma(n int64) string {
	if n < 0 {
		return "-" + formatComma(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	offset := len(s) % 3
	if offset > 0 {
		result.WriteString(s[:offset])
	}
	for i := offset; i < len(s); i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// appendProviderRows renders a per-provider/backend token usage table for the
// given byProvider map. Returns the lines slice with rows appended. Used for
// both main-agent and daemon provider/backend breakdowns.
func appendProviderRows(lines []string, byProvider map[string]fs.TokenTotals, emptyMessage string, labelStyle, valueStyle, subtleStyle lipgloss.Style) []string {
	// Compute total spend across providers for the share bar.
	var grandSpend int64
	for _, t := range byProvider {
		grandSpend += t.Input + t.Output + t.Thinking
	}

	// Stable order: highest spend first.
	type provLine struct {
		name  string
		t     fs.TokenTotals
		spend int64
	}
	var rows []provLine
	for name, t := range byProvider {
		rows = append(rows, provLine{name, t, t.Input + t.Output + t.Thinking})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].spend != rows[j].spend {
			return rows[i].spend > rows[j].spend
		}
		return rows[i].name < rows[j].name
	})

	if len(rows) == 0 {
		if emptyMessage == "" {
			emptyMessage = i18n.T("props.detail_no_tokens")
		}
		return append(lines, "  "+subtleStyle.Render(emptyMessage))
	}
	for _, r := range rows {
		pct := 0.0
		if grandSpend > 0 {
			pct = 100.0 * float64(r.spend) / float64(grandSpend)
		}
		bar := renderShareBar(pct, 20)
		nameStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent)
		header := fmt.Sprintf("  %-14s %s %5.1f%%",
			nameStyle.Render(r.name), bar, pct)
		lines = append(lines, header)
		lines = append(lines, "    "+labelStyle.Render("input:    ")+valueStyle.Render(formatComma(r.t.Input))+
			labelStyle.Render("    output:    ")+valueStyle.Render(formatComma(r.t.Output)))
		lines = append(lines, "    "+labelStyle.Render("thinking: ")+valueStyle.Render(formatComma(r.t.Thinking))+
			labelStyle.Render("    cached:    ")+valueStyle.Render(formatComma(r.t.Cached)))
		hitStr := ""
		if r.t.Input > 0 {
			hitStr = fmt.Sprintf("    cache hit: %.1f%%", 100.0*float64(r.t.Cached)/float64(r.t.Input))
		}
		lines = append(lines, "    "+labelStyle.Render("api_calls: ")+valueStyle.Render(fmt.Sprintf("%d", r.t.APICalls))+
			labelStyle.Render("    miss:      ")+valueStyle.Render(formatComma(cacheMiss(r.t.Cached, r.t.Input)))+
			labelStyle.Render(hitStr))
		lines = append(lines, "")
	}
	return lines
}

func appendSessionAPIStats(lines []string, title string, stats fs.SessionTokenStats, toolCalls int64, sectionStyle, labelStyle, valueStyle lipgloss.Style) []string {
	if stats.APICalls <= 0 {
		return lines
	}
	tokens := stats.Input + stats.Output + stats.Thinking
	lines = append(lines, "  "+sectionStyle.Render(title))
	lines = append(lines, "")
	lines = append(lines, "    "+labelStyle.Render("api_calls:                 ")+
		valueStyle.Render(fmt.Sprintf("%d", stats.APICalls)))
	lines = append(lines, "    "+labelStyle.Render("tool_calls:                ")+
		valueStyle.Render(fmt.Sprintf("%d", toolCalls)))
	lines = append(lines, "    "+labelStyle.Render("tokens:                    ")+
		valueStyle.Render(formatComma(tokens)))
	lines = append(lines, "    "+labelStyle.Render("input / output / thinking: ")+
		valueStyle.Render(fmt.Sprintf("%s / %s / %s", formatComma(stats.Input), formatComma(stats.Output), formatComma(stats.Thinking))))
	lines = append(lines, "    "+labelStyle.Render("cached / missed:           ")+
		valueStyle.Render(fmt.Sprintf("%s / %s", formatComma(stats.Cached), formatComma(cacheMiss(stats.Cached, stats.Input)))))
	lines = append(lines, "    "+labelStyle.Render("cache hit rate:            ")+
		valueStyle.Render(formatCacheRate(stats.Cached, stats.Input)))
	lines = append(lines, "    "+labelStyle.Render("tokens/api_call:           ")+
		valueStyle.Render(formatComma(avgPerCall(tokens, stats.APICalls))))
	lines = append(lines, "    "+labelStyle.Render("tool_calls/api_call:       ")+
		valueStyle.Render(fmt.Sprintf("%.2f", float64(toolCalls)/float64(stats.APICalls))))
	if stats.HasCodexTransferMode {
		lines = append(lines, "    "+labelStyle.Render("transfer full / incremental: ")+
			valueStyle.Render(fmt.Sprintf("%d / %d", stats.CodexFull, stats.CodexIncremental)))
	}
	lines = append(lines, "")
	return lines
}

// appendCurrentSessionDiagnostics omits rows already shown in Current session
// above (api_calls, cache miss/rate, input, and tokens/api_call) while retaining
// the old lower layer's additional session evidence.
func appendCurrentSessionDiagnostics(lines []string, stats fs.SessionTokenStats, toolCalls int64, sectionStyle, labelStyle, valueStyle lipgloss.Style) []string {
	if stats.APICalls <= 0 {
		return lines
	}
	tokens := stats.Input + stats.Output + stats.Thinking
	lines = append(lines, "  "+sectionStyle.Render(i18n.T("props.detail_current_session_recent")))
	lines = append(lines, "")
	lines = append(lines, "    "+labelStyle.Render("tool_calls:                ")+
		valueStyle.Render(fmt.Sprintf("%d", toolCalls)))
	lines = append(lines, "    "+labelStyle.Render("tokens:                    ")+
		valueStyle.Render(formatComma(tokens)))
	lines = append(lines, "    "+labelStyle.Render("output / thinking:         ")+
		valueStyle.Render(fmt.Sprintf("%s / %s", formatComma(stats.Output), formatComma(stats.Thinking))))
	lines = append(lines, "    "+labelStyle.Render("cached:                    ")+
		valueStyle.Render(formatComma(stats.Cached)))
	lines = append(lines, "    "+labelStyle.Render("tool_calls/api_call:       ")+
		valueStyle.Render(fmt.Sprintf("%.2f", float64(toolCalls)/float64(stats.APICalls))))
	if stats.HasCodexTransferMode {
		lines = append(lines, "    "+labelStyle.Render("transfer full / incremental: ")+
			valueStyle.Render(fmt.Sprintf("%d / %d", stats.CodexFull, stats.CodexIncremental)))
	}
	lines = append(lines, "")
	return lines
}

// refDisplayName extracts the filename stem from a preset path string
// for compact display. "~/.lingtai-tui/presets/saved/mimo-1.json"
// → "mimo-1". Empty input → empty output.
func refDisplayName(ref string) string {
	if ref == "" {
		return ""
	}
	// Strip directory prefix.
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	// Strip extension.
	if i := strings.LastIndex(ref, "."); i >= 0 {
		ref = ref[:i]
	}
	return ref
}
