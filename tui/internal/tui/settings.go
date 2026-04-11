package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/preset"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Settings holds per-project preferences at .lingtai/human/settings.json.
type Settings struct {
	Orchestrator string `json:"orchestrator,omitempty"`
	Brief        *bool  `json:"brief,omitempty"` // nil or true = enabled, false = disabled
}

// BriefEnabled returns whether the brief section is enabled for this project.
func (s Settings) BriefEnabled() bool {
	return s.Brief == nil || *s.Brief
}

// LoadSettings reads per-project settings from .lingtai/human/settings.json.
func LoadSettings(baseDir string) Settings {
	path := filepath.Join(baseDir, "human", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}
	}
	return s
}

// SaveSettings writes per-project settings to .lingtai/human/settings.json.
func SaveSettings(baseDir string, s Settings) error {
	dir := filepath.Join(baseDir, "human")
	os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644)
}

// SettingField represents a single configurable setting.
type SettingField struct {
	Key     string
	Label   string   // i18n key
	Options []string // values to cycle through
	Current int      // index into Options
}

// localFieldIdx identifies which local text field is active.
type localFieldIdx int

const (
	localNickname localFieldIdx = iota
	localAgentName
	localFieldCount
)

// SettingsModel is the /settings view.
type SettingsModel struct {
	cursor     int
	tuiConfig  config.TUIConfig
	fields     []SettingField
	globalDir  string
	projectDir string // .lingtai/ dir for per-project settings
	orchDir    string // orchestrator dir for agent name
	nickname   string // human's nickname from human/.agent.json
	agentName  string // agent's true name from orch/.agent.json
	editingLocal localFieldIdx // which local field is being edited (-1 = none)
	editing      bool
	width      int
	height     int
}

func NewSettingsModel(globalDir, projectDir, orchDir string, tuiCfg config.TUIConfig) SettingsModel {
	langOptions := []string{"en", "zh", "wen"}
	langCurrent := 0
	for i, l := range langOptions {
		if l == tuiCfg.Language {
			langCurrent = i
			break
		}
	}

	pageSizeOptions := []string{"100", "200", "unlimited"}
	pageSizeCurrent := 0 // default to 100
	if tuiCfg.MailPageSize <= 0 {
		pageSizeCurrent = 2 // unlimited
	} else {
		pageSizeStr := fmt.Sprintf("%d", tuiCfg.MailPageSize)
		for i, p := range pageSizeOptions {
			if p == pageSizeStr {
				pageSizeCurrent = i
				break
			}
		}
	}

	themeOptions := ThemeNames()
	themeCurrent := 0
	for i, t := range themeOptions {
		if t == tuiCfg.Theme {
			themeCurrent = i
			break
		}
	}
	// If no theme is set, find the default
	if tuiCfg.Theme == "" {
		for i, t := range themeOptions {
			if t == DefaultThemeName {
				themeCurrent = i
				break
			}
		}
	}

	insightsOptions := []string{"off", "on"}
	insightsCurrent := 0
	if tuiCfg.Insights {
		insightsCurrent = 1
	}

	// Read agent language from init.json
	agentLangOptions := []string{"en", "zh", "wen"}
	agentLangCurrent := 0
	if orchDir != "" {
		initPath := filepath.Join(orchDir, "init.json")
		if data, err := os.ReadFile(initPath); err == nil {
			var initData map[string]interface{}
			if err := json.Unmarshal(data, &initData); err == nil {
				if m, ok := initData["manifest"].(map[string]interface{}); ok {
					if l, ok := m["language"].(string); ok {
						for i, lang := range agentLangOptions {
							if lang == l {
								agentLangCurrent = i
								break
							}
						}
					}
				}
			}
		}
	}

	// Brief toggle — per-project
	briefOptions := []string{"on", "off"}
	briefCurrent := 0 // default on
	localSettings := LoadSettings(projectDir)
	if !localSettings.BriefEnabled() {
		briefCurrent = 1
	}

	fields := []SettingField{
		{Key: "language", Label: "settings.language", Options: langOptions, Current: langCurrent},
		{Key: "mail_page_size", Label: "settings.mail_page_size", Options: pageSizeOptions, Current: pageSizeCurrent},
		{Key: "theme", Label: "settings.theme", Options: themeOptions, Current: themeCurrent},
		{Key: "insights", Label: "settings.insights", Options: insightsOptions, Current: insightsCurrent},
		{Key: "brief", Label: "settings.brief", Options: briefOptions, Current: briefCurrent},
		{Key: "agent_lang", Label: "settings.agent_lang", Options: agentLangOptions, Current: agentLangCurrent},
	}

	// Read current nickname from human's .agent.json
	nickname := ""
	humanPath := filepath.Join(projectDir, "human", ".agent.json")
	if data, err := os.ReadFile(humanPath); err == nil {
		var manifest map[string]interface{}
		if err := json.Unmarshal(data, &manifest); err == nil {
			if n, ok := manifest["nickname"].(string); ok {
				nickname = n
			}
		}
	}

	// Read current agent name from orchestrator's .agent.json
	agentName := ""
	if orchDir != "" {
		if node, err := fs.ReadAgent(orchDir); err == nil {
			agentName = node.AgentName
		}
	}

	return SettingsModel{
		tuiConfig:    tuiCfg,
		fields:       fields,
		globalDir:    globalDir,
		projectDir:   projectDir,
		orchDir:      orchDir,
		nickname:     nickname,
		agentName:    agentName,
		editingLocal: -1,
	}
}

func (m SettingsModel) Init() tea.Cmd { return nil }

func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		totalFields := len(m.fields) + int(localFieldCount)
		localStart := len(m.fields) // first local field index

		if m.editing {
			switch msg.String() {
			case "enter", "esc":
				m.editing = false
				m.saveLocal()
			case "backspace":
				ptr := m.localTextPtr()
				if len(*ptr) > 0 {
					runes := []rune(*ptr)
					*ptr = string(runes[:len(runes)-1])
				}
			case "ctrl+c":
				return m, tea.Quit
			default:
				ch := msg.String()
				if len(ch) == 1 || (len(ch) > 1 && !strings.HasPrefix(ch, "ctrl+")) {
					ptr := m.localTextPtr()
					*ptr += ch
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
		case "enter":
			if m.cursor >= localStart {
				m.editingLocal = localFieldIdx(m.cursor - localStart)
				m.editing = true
				return m, nil
			}
			if m.fields[m.cursor].Key == "language" {
				return m, func() tea.Msg { return ViewChangeMsg{View: "welcome"} }
			}
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < totalFields-1 {
				m.cursor++
			}
		case "left":
			if m.cursor < localStart {
				f := &m.fields[m.cursor]
				if f.Current > 0 {
					f.Current--
					return m, m.applyField(f)
				}
			}
		case "right":
			if m.cursor < localStart {
				f := &m.fields[m.cursor]
				if f.Current < len(f.Options)-1 {
					f.Current++
					return m, m.applyField(f)
				}
			}
		}
	}
	return m, nil
}

func (m *SettingsModel) applyField(f *SettingField) tea.Cmd {
	val := f.Options[f.Current]
	switch f.Key {
	case "language":
		m.tuiConfig.Language = val
		i18n.SetLang(val)
	case "mail_page_size":
		if val == "unlimited" {
			m.tuiConfig.MailPageSize = 0
		} else {
			size := 100
			fmt.Sscanf(val, "%d", &size)
			m.tuiConfig.MailPageSize = size
		}
	case "insights":
		m.tuiConfig.Insights = val == "on"
	case "theme":
		m.tuiConfig.Theme = val
		SetThemeByName(val)
	case "agent_lang":
		m.saveAgentLang(val)
		return nil // don't save TUI config — this writes to init.json
	case "brief":
		m.saveBrief(val == "on")
		return nil // don't save TUI config — this writes to per-project settings + init.json
	}
	config.SaveTUIConfig(m.globalDir, m.tuiConfig)
	return nil
}

// localTextPtr returns a pointer to the text being edited.
func (m *SettingsModel) localTextPtr() *string {
	switch m.editingLocal {
	case localNickname:
		return &m.nickname
	case localAgentName:
		return &m.agentName
	}
	return &m.nickname
}

func (m *SettingsModel) saveLocal() {
	switch m.editingLocal {
	case localNickname:
		m.saveNickname()
	case localAgentName:
		m.saveAgentName()
	}
}

func (m *SettingsModel) saveNickname() {
	humanPath := filepath.Join(m.projectDir, "human", ".agent.json")
	data, err := os.ReadFile(humanPath)
	if err != nil {
		return
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return
	}
	if m.nickname == "" {
		delete(manifest, "nickname")
	} else {
		manifest["nickname"] = m.nickname
	}
	if out, err := json.MarshalIndent(manifest, "", "  "); err == nil {
		os.WriteFile(humanPath, out, 0o644)
	}
}

func (m *SettingsModel) saveAgentName() {
	if m.orchDir == "" || m.agentName == "" {
		return
	}
	// Update init.json manifest.agent_name
	initPath := filepath.Join(m.orchDir, "init.json")
	if data, err := os.ReadFile(initPath); err == nil {
		var init map[string]interface{}
		if err := json.Unmarshal(data, &init); err == nil {
			if manifest, ok := init["manifest"].(map[string]interface{}); ok {
				manifest["agent_name"] = m.agentName
			}
			if out, err := json.MarshalIndent(init, "", "  "); err == nil {
				os.WriteFile(initPath, out, 0o644)
			}
		}
	}
}

func (m *SettingsModel) saveAgentLang(lang string) {
	if m.orchDir == "" {
		return
	}
	initPath := filepath.Join(m.orchDir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return
	}
	var initData map[string]interface{}
	if err := json.Unmarshal(data, &initData); err != nil {
		return
	}
	if manifest, ok := initData["manifest"].(map[string]interface{}); ok {
		manifest["language"] = lang
	}
	initData["covenant_file"] = preset.CovenantPath(m.globalDir, lang)
	initData["principle_file"] = preset.PrinciplePath(m.globalDir, lang)
	delete(initData, "covenant")
	delete(initData, "principle")
	if out, err := json.MarshalIndent(initData, "", "  "); err == nil {
		os.WriteFile(initPath, out, 0o644)
	}
}

func (m *SettingsModel) saveBrief(enabled bool) {
	// Persist to per-project settings
	s := LoadSettings(m.projectDir)
	s.Brief = &enabled
	SaveSettings(m.projectDir, s)

	if m.orchDir == "" {
		return
	}

	// Update init.json: add or remove brief_file
	initPath := filepath.Join(m.orchDir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return
	}
	var initData map[string]interface{}
	if err := json.Unmarshal(data, &initData); err != nil {
		return
	}

	projectPath := filepath.Dir(m.projectDir) // .lingtai/ → project root
	briefPath := fs.BriefFilePath(m.globalDir, projectPath)

	if enabled {
		initData["brief_file"] = briefPath
	} else {
		delete(initData, "brief_file")
		delete(initData, "brief")
		// Also remove system/brief.md so the kernel doesn't fall back to it
		systemBrief := filepath.Join(m.orchDir, "system", "brief.md")
		os.Remove(systemBrief)
	}

	if out, err := json.MarshalIndent(initData, "", "  "); err == nil {
		os.WriteFile(initPath, out, 0o644)
	}
}

func (m SettingsModel) View() string {
	var b strings.Builder

	// Title bar: product name · settings
	titleText := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent).Render(i18n.T("welcome.title"))
	titleBar := titleText + " " + StyleAccent.Render(RuneBullet) + " " + StyleTitle.Render(i18n.T("settings.title"))
	escHint := StyleAccent.Render("[esc] ") + StyleSubtle.Render(i18n.T("settings.back"))
	padding := m.width - lipgloss.Width(titleBar) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(titleBar + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(titleBar + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n")

	// Poem decoration
	b.WriteString(StyleFaint.Render("  "+i18n.T("welcome.poem_line1")) + "\n")
	b.WriteString(StyleFaint.Render("  "+i18n.T("welcome.poem_line2")) + "\n\n")

	// Fields
	labelStyle := lipgloss.NewStyle().Foreground(ColorText)
	dimValStyle := lipgloss.NewStyle().Foreground(ColorTextDim)
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	for i, f := range m.fields {
		// Insert "Local" section header before agent fields
		if f.Key == "brief" {
			b.WriteString("\n  " + sectionStyle.Render(i18n.T("settings.local")) + "\n")
		}
		cursor := "  "
		if i == m.cursor {
			cursor = StyleAccent.Render("> ")
		}
		label := labelStyle.Render(fmt.Sprintf("%-15s", i18n.T(f.Label)+":"))
		value := f.Options[f.Current]

		// Show display-friendly value
		displayVal := value
		if f.Key == "insights" || f.Key == "brief" || (f.Key == "mail_page_size" && value == "unlimited") {
			displayVal = i18n.T("settings." + value)
		} else if f.Key == "theme" {
			displayVal = i18n.T("theme." + value)
		}

		// Highlight selected
		if i == m.cursor {
			displayVal = lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render("< " + displayVal + " >")
		} else {
			displayVal = dimValStyle.Render(displayVal)
		}

		b.WriteString(cursor + label + " " + displayVal + "\n")
		// Show description for selected field
		if i == m.cursor {
			descKey := "settings." + f.Key + "_desc"
			desc := i18n.T(descKey)
			if desc != descKey { // key exists
				b.WriteString("  " + StyleFaint.Render(desc) + "\n")
			}
		}
	}

	// Local text fields
	localStart := len(m.fields)

	type localField struct {
		label string
		hint  string
		value string
		idx   localFieldIdx
	}
	locals := []localField{
		{i18n.T("settings.nickname"), i18n.T("settings.nickname_hint"), m.nickname, localNickname},
		{i18n.T("settings.agent_name"), i18n.T("settings.agent_name_hint"), m.agentName, localAgentName},
	}
	for i, lf := range locals {
		absIdx := localStart + i
		cursor := "  "
		if m.cursor == absIdx {
			cursor = StyleAccent.Render("> ")
		}
		label := labelStyle.Render(fmt.Sprintf("%-15s", lf.label+":"))
		displayVal := lf.value
		if displayVal == "" {
			displayVal = "—"
		}
		if m.editing && m.editingLocal == lf.idx {
			displayVal = lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render(lf.value + "▎")
		} else if m.cursor == absIdx {
			displayVal = lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render(displayVal)
		} else {
			displayVal = dimValStyle.Render(displayVal)
		}
		b.WriteString(cursor + label + " " + displayVal)
		if m.cursor == absIdx && !m.editing {
			b.WriteString("  " + StyleFaint.Render(lf.hint))
		}
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n" + strings.Repeat("─", m.width) + "\n")
	var hints string
	if m.editing {
		hints = fmt.Sprintf("  [Enter] %s  [Esc] %s", i18n.T("settings.confirm"), i18n.T("settings.back"))
	} else {
		hints = fmt.Sprintf("  ↑↓ %s  ←→ %s", i18n.T("settings.select"), i18n.T("settings.change"))
		if m.cursor >= localStart {
			hints += "  [Enter] " + i18n.T("settings.edit")
		} else if m.fields[m.cursor].Key == "language" {
			hints += "  [Enter] " + i18n.T("settings.welcome")
		}
		hints += "  [esc] " + i18n.T("settings.back")
	}
	b.WriteString(StyleFaint.Render(hints) + "\n")

	return b.String()
}

