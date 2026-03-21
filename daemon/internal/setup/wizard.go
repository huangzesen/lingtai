package setup

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"lingtai-daemon/internal/combo"
	"lingtai-daemon/internal/config"
	"lingtai-daemon/internal/i18n"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

//go:embed defaults/covenant_en.md
var defaultCovenantEN string

//go:embed defaults/covenant_zh.md
var defaultCovenantZH string

//go:embed defaults/covenant_wen.md
var defaultCovenantWEN string

//go:embed defaults/tool_translation.md
var toolTranslationWEN string

//go:embed defaults/tool_translation_zh.md
var toolTranslationZH string

//go:embed defaults/bash_policy.json
var defaultBashPolicy string

// Steps in the wizard.
type step int

const (
	StepCombo step = iota
	StepLang
	StepModel
	StepMultimodal
	StepMessaging
	StepGeneral
	StepReview

	// Internal field storage keys (not real steps in the wizard flow)
	StepIMAP     step = 100
	StepTelegram step = 101
)

func (s step) String() string {
	switch s {
	case StepCombo:
		return "Combo"
	case StepLang:
		return i18n.S("setup_lang")
	case StepModel:
		return i18n.S("setup_model")
	case StepMultimodal:
		return i18n.S("setup_multimodal")
	case StepMessaging:
		return i18n.S("setup_messaging")
	case StepGeneral:
		return i18n.S("setup_general")
	case StepReview:
		return i18n.S("setup_review")
	default:
		return "Unknown"
	}
}

// Styles
var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")) // cyan
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
	dimStyle     = lipgloss.NewStyle().Faint(true)
	promptStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15")) // white
)

// providers is the list of supported LLM providers.
var providers = []string{"minimax", "openai", "anthropic", "gemini", "custom"}

// Default endpoints for known providers (empty = provider SDK default).
var providerEndpoints = map[string]string{
	"minimax":   "https://api.minimaxi.com/v1",
	"openai":    "https://api.openai.com/v1",
	"anthropic": "https://api.anthropic.com",
	"gemini":    "https://generativelanguage.googleapis.com",
	"custom":    "",
}

// Default model names for known providers.
var providerModels = map[string]string{
	"minimax":   "MiniMax-M2.7-highspeed",
	"openai":    "gpt-5.4",
	"anthropic": "claude-opus-4-6",
	"gemini":    "gemini-3.1-pro",
	"custom":    "",
}

// MiniMax regional endpoints for quick setup.
var mmQuickEndpoints = []string{
	"https://api.minimaxi.com", // china (default)
	"https://api.minimax.io",   // international
}
var mmQuickEndpointLabels = []string{"China", "International"}

// mmCapability describes a multimodal capability row in the wizard grid.
type mmCapability struct {
	name      string            // display name
	configKey string            // JSON key in model.json
	envSuffix string            // for env var naming
	providers []string
	models    map[string]string
	endpoints map[string]string
}

var mmCaps = []mmCapability{
	{
		name: "Vision", configKey: "vision", envSuffix: "VISION",
		providers: []string{"minimax", "gemini"},
		models:    map[string]string{"minimax": "MiniMax-M2.7-highspeed", "gemini": "gemini-3.1-pro"},
		endpoints: map[string]string{"minimax": "https://api.minimaxi.com", "gemini": "https://generativelanguage.googleapis.com"},
	},
	{
		name: "Web Search", configKey: "web_search", envSuffix: "WEB_SEARCH",
		providers: []string{"minimax", "gemini"},
		models:    map[string]string{"minimax": "MiniMax-M2.7-highspeed", "gemini": "gemini-3.1-pro"},
		endpoints: map[string]string{"minimax": "https://api.minimaxi.com", "gemini": "https://generativelanguage.googleapis.com"},
	},
	{
		name: "Talk (TTS)", configKey: "talk", envSuffix: "TALK",
		providers: []string{"minimax"},
		models:    map[string]string{"minimax": ""},
		endpoints: map[string]string{"minimax": "https://api.minimaxi.com"},
	},
	{
		name: "Compose", configKey: "compose", envSuffix: "COMPOSE",
		providers: []string{"minimax"},
		models:    map[string]string{"minimax": ""},
		endpoints: map[string]string{"minimax": "https://api.minimaxi.com"},
	},
	{
		name: "Draw", configKey: "draw", envSuffix: "DRAW",
		providers: []string{"minimax"},
		models:    map[string]string{"minimax": ""},
		endpoints: map[string]string{"minimax": "https://api.minimaxi.com"},
	},
	{
		name: "Listen", configKey: "listen", envSuffix: "LISTEN",
		providers: []string{"local"},
		models:    map[string]string{"local": ""},
		endpoints: map[string]string{},
	},
}

// mmCapState holds the mutable state for one multimodal capability row.
type mmCapState struct {
	providerIdx   int
	keyInput      textinput.Model
	endpointInput textinput.Model
}

func newMMCapState(row int) mmCapState {
	cap := mmCaps[row]
	p := cap.providers[0]
	ep := cap.endpoints[p]

	keyInput := newTextInput("API key", "")
	keyInput.EchoMode = textinput.EchoPassword
	keyInput.EchoCharacter = '•'
	keyInput.Width = 30

	endpointInput := newTextInput("https://...", ep)
	endpointInput.Width = 38

	return mmCapState{providerIdx: 0, keyInput: keyInput, endpointInput: endpointInput}
}

// field is a labeled text input.
type field struct {
	label string
	input textinput.Model
}

// testResultMsg carries the outcome of an async connection test.
type testResultMsg struct {
	step   step
	result TestResult
}

// wizardModel is the Bubble Tea model for the setup wizard.
type wizardModel struct {
	step      step
	fields    map[step][]field
	focus     int // index of focused field within current step
	outputDir string

	// combo selection state
	combos    []combo.Combo
	comboIdx  int             // -1 = "Create new", 0..n = combo index
	comboName textinput.Model // for naming the combo on save

	// language selector state
	langIdx int

	// provider selector state
	providerIdx int

	// multimodal state
	mmMode          int // 0=chooser, 1=quick setup, 2=manual grid
	mmChooserIdx    int // chooser selection: 0=quick, 1=manual, 2=skip
	mmQuickKey1     textinput.Model // MINIMAX_API_KEY (vision, web_search)
	mmQuickKey2     textinput.Model // MINIMAX_MCP_API_KEY (talk, compose, draw)
	mmQuickFocus    int             // 0=endpoint, 1=key1, 2=key2
	mmQuickEndpoint int             // 0=international, 1=china
	mmRows          []mmCapState
	mmRow         int // active row (0-5)
	mmCol         int // active column: 0=provider, 1=key, 2=endpoint

	// messaging state
	msgMode       int // 0=chooser, 1=imap, 2=telegram
	msgChooserIdx int // 0=imap, 1=telegram, 2=skip
	msgImapDone   bool
	msgTgDone     bool

	// test results per step
	testResults map[step]*TestResult

	// final status
	done    bool
	err     error
	written []string // files written
}

func newTextInput(placeholder string, defaultVal string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(defaultVal)
	ti.CharLimit = 256
	ti.Width = 50
	return ti
}

func newWizardModel(outputDir string) wizardModel {
	// Detect initial language index
	initialLangIdx := 0
	for idx, code := range i18n.Languages {
		if code == i18n.Lang {
			initialLangIdx = idx
			break
		}
	}

	m := wizardModel{
		step:        StepCombo,
		outputDir:   outputDir,
		langIdx:     initialLangIdx,
		providerIdx: 0,
		testResults: make(map[step]*TestResult),
		fields:      make(map[step][]field),
	}

	// Step: Combo selection
	combos, _ := combo.List()
	m.combos = combos
	m.comboIdx = -1 // default to "Create new"
	m.comboName = textinput.New()
	m.comboName.Placeholder = "my-combo"
	m.comboName.CharLimit = 40
	m.comboName.Width = 30

	// Step: Lang has no text fields (uses left/right selector)

	// Step: Model
	defaultProvider := providers[0]
	apiKeyInput := newTextInput("sk-...", "")
	apiKeyInput.EchoMode = textinput.EchoPassword
	apiKeyInput.EchoCharacter = '•'
	m.fields[StepModel] = []field{
		{label: "Provider", input: newTextInput(defaultProvider, defaultProvider)},
		{label: "Model", input: newTextInput("model name", providerModels[defaultProvider])},
		{label: "API key", input: apiKeyInput},
		{label: "Endpoint", input: newTextInput("https://...", providerEndpoints[defaultProvider])},
	}

	// Step: Multimodal
	m.mmMode = 0 // start with chooser
	m.mmQuickEndpoint = 0 // china default
	qk1 := newTextInput("sk-...", "")
	qk1.EchoMode = textinput.EchoPassword
	qk1.EchoCharacter = '•'
	qk2 := newTextInput("sk-...", "")
	qk2.EchoMode = textinput.EchoPassword
	qk2.EchoCharacter = '•'
	m.mmQuickKey1 = qk1
	m.mmQuickKey2 = qk2
	m.mmRows = make([]mmCapState, len(mmCaps))
	for i := range mmCaps {
		m.mmRows[i] = newMMCapState(i)
	}

	// Step: IMAP
	imapPassInput := newTextInput("password", "")
	imapPassInput.EchoMode = textinput.EchoPassword
	imapPassInput.EchoCharacter = '•'
	m.fields[StepIMAP] = []field{
		{label: "Email address", input: newTextInput("you@example.com", "")},
		{label: "Password", input: imapPassInput},
		{label: "IMAP host", input: newTextInput("imap.example.com", "")},
		{label: "IMAP port", input: newTextInput("993", "993")},
		{label: "SMTP host", input: newTextInput("smtp.example.com", "")},
		{label: "SMTP port", input: newTextInput("587", "587")},
	}

	// Step: Telegram
	telegramInput := newTextInput("bot token", "")
	telegramInput.EchoMode = textinput.EchoPassword
	telegramInput.EchoCharacter = '•'
	m.fields[StepTelegram] = []field{
		{label: "Bot token", input: telegramInput},
	}

	// Step: General
	m.fields[StepGeneral] = []field{
		{label: "Agent name", input: newTextInput("orchestrator", "orchestrator")},
		{label: "Agent port", input: newTextInput("8501", "8501")},
		{label: "Bash policy (Enter = use default)", input: newTextInput("Enter = use default", "")},
	}

	// Step: Review has no fields

	// Pre-fill from existing config if available
	m.loadExisting()

	// Focus the first field
	if len(m.fields[StepModel]) > 0 {
		m.fields[StepModel][0].input.Focus()
	}

	return m
}

// loadExisting reads config.json, model.json, and .env from outputDir
// and pre-fills the wizard fields so returning users see their saved values.
func (m *wizardModel) loadExisting() {
	configPath := filepath.Join(m.outputDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return // no existing config
	}

	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return
	}

	// Load .env secrets
	config.LoadDotenv(m.outputDir)

	// Language
	var lang string
	if v, ok := raw["language"]; ok {
		json.Unmarshal(v, &lang)
		for idx, code := range i18n.Languages {
			if code == lang {
				m.langIdx = idx
				i18n.Lang = lang
				break
			}
		}
	}

	// Model — resolve from model.json or inline (use raw map to capture sub-objects)
	var modelRaw map[string]json.RawMessage
	if v, ok := raw["model"]; ok {
		var modelPath string
		if json.Unmarshal(v, &modelPath) == nil {
			// It's a file path
			modelData, err := os.ReadFile(filepath.Join(m.outputDir, modelPath))
			if err == nil {
				json.Unmarshal(modelData, &modelRaw)
			}
		} else {
			json.Unmarshal(v, &modelRaw)
		}
	}
	if modelRaw == nil {
		modelRaw = make(map[string]json.RawMessage)
	}

	// Parse main model fields
	var modelCfg struct {
		Provider  string `json:"provider"`
		Model     string `json:"model"`
		APIKeyEnv string `json:"api_key_env"`
		BaseURL   string `json:"base_url"`
	}
	// Re-marshal modelRaw back to parse the struct fields
	if b, err := json.Marshal(modelRaw); err == nil {
		json.Unmarshal(b, &modelCfg)
	}

	if modelCfg.Provider != "" {
		// Set provider index
		for idx, p := range providers {
			if p == modelCfg.Provider {
				m.providerIdx = idx
				break
			}
		}
		m.fields[StepModel][0].input.SetValue(modelCfg.Provider)
	}
	if modelCfg.Model != "" {
		m.fields[StepModel][1].input.SetValue(modelCfg.Model)
	}
	if modelCfg.APIKeyEnv != "" {
		if key := os.Getenv(modelCfg.APIKeyEnv); key != "" {
			m.fields[StepModel][2].input.SetValue(key)
		}
	}
	if modelCfg.BaseURL != "" {
		m.fields[StepModel][3].input.SetValue(modelCfg.BaseURL)
	}

	// Multimodal sub-configs
	for i, cap := range mmCaps {
		capRaw, ok := modelRaw[cap.configKey]
		if !ok {
			continue
		}
		var capCfg struct {
			Provider  string `json:"provider"`
			APIKeyEnv string `json:"api_key_env"`
			BaseURL   string `json:"base_url"`
		}
		if json.Unmarshal(capRaw, &capCfg) == nil {
			if capCfg.Provider != "" {
				for idx, p := range cap.providers {
					if p == capCfg.Provider {
						m.mmRows[i].providerIdx = idx
						break
					}
				}
			}
			if capCfg.APIKeyEnv != "" {
				if key := os.Getenv(capCfg.APIKeyEnv); key != "" {
					m.mmRows[i].keyInput.SetValue(key)
				}
			}
			if capCfg.BaseURL != "" {
				m.mmRows[i].endpointInput.SetValue(capCfg.BaseURL)
			}
		}
	}

	// IMAP
	if v, ok := raw["imap"]; ok {
		var imap struct {
			Email    string `json:"email_address"`
			PassEnv  string `json:"password_env"`
			IMAPHost string `json:"imap_host"`
			IMAPPort int    `json:"imap_port"`
			SMTPHost string `json:"smtp_host"`
			SMTPPort int    `json:"smtp_port"`
		}
		if json.Unmarshal(v, &imap) == nil {
			m.fields[StepIMAP][0].input.SetValue(imap.Email)
			if imap.PassEnv != "" {
				if pass := os.Getenv(imap.PassEnv); pass != "" {
					m.fields[StepIMAP][1].input.SetValue(pass)
				}
			}
			m.fields[StepIMAP][2].input.SetValue(imap.IMAPHost)
			if imap.IMAPPort > 0 {
				m.fields[StepIMAP][3].input.SetValue(strconv.Itoa(imap.IMAPPort))
			}
			m.fields[StepIMAP][4].input.SetValue(imap.SMTPHost)
			if imap.SMTPPort > 0 {
				m.fields[StepIMAP][5].input.SetValue(strconv.Itoa(imap.SMTPPort))
			}
		}
	}

	// Telegram
	if v, ok := raw["telegram"]; ok {
		var tg struct {
			TokenEnv string `json:"bot_token_env"`
		}
		if json.Unmarshal(v, &tg) == nil && tg.TokenEnv != "" {
			if token := os.Getenv(tg.TokenEnv); token != "" {
				m.fields[StepTelegram][0].input.SetValue(token)
			}
		}
	}

	// General
	var agentName, bashPolicy string
	var agentPort int
	if v, ok := raw["agent_name"]; ok {
		json.Unmarshal(v, &agentName)
	}
	if v, ok := raw["agent_port"]; ok {
		json.Unmarshal(v, &agentPort)
	}
	if v, ok := raw["bash_policy"]; ok {
		json.Unmarshal(v, &bashPolicy)
	}
	if agentName != "" {
		m.fields[StepGeneral][0].input.SetValue(agentName)
	}
	if agentPort > 0 {
		m.fields[StepGeneral][1].input.SetValue(strconv.Itoa(agentPort))
	}
	if bashPolicy != "" {
		m.fields[StepGeneral][2].input.SetValue(bashPolicy)
	}
}

func (m wizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case testResultMsg:
		r := msg.result
		m.testResults[msg.step] = &r
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			// Multimodal: Esc goes back within sub-modes
			if m.step == StepMultimodal {
				switch m.mmMode {
				case 0: // chooser — do nothing
				case 1: // quick setup — back to chooser
					m.mmQuickKey1.Blur()
					m.mmQuickKey2.Blur()
					m.mmMode = 0
				case 2: // manual grid — back to chooser
					m.mmBlurCurrent()
					m.mmMode = 0
				}
				return m, nil
			}
			// Messaging: Esc goes back within sub-modes
			if m.step == StepMessaging {
				switch m.msgMode {
				case 0: // chooser — do nothing
				case 1, 2: // imap or telegram — back to chooser
					// blur current fields
					activeStep := StepIMAP
					if m.msgMode == 2 {
						activeStep = StepTelegram
					}
					if fields, ok := m.fields[activeStep]; ok {
						for i := range fields {
							fields[i].input.Blur()
						}
						m.fields[activeStep] = fields
					}
					m.msgMode = 0
					m.focus = 0
				}
				return m, nil
			}

		case "tab", "down":
			if m.step == StepCombo {
				total := len(m.combos) // -1 to len(combos)-1
				m.comboIdx++
				if m.comboIdx >= total {
					m.comboIdx = -1
				}
				return m, nil
			}
			if m.step == StepLang {
				m.langIdx = (m.langIdx + 1) % len(i18n.Languages)
				i18n.Lang = i18n.Languages[m.langIdx]
				return m, nil
			}
			if m.step == StepMultimodal {
				switch m.mmMode {
				case 0: // chooser
					m.mmChooserIdx = (m.mmChooserIdx + 1) % 3
				case 1: // quick setup
					switch m.mmQuickFocus {
					case 0:
						m.mmQuickFocus = 1
						m.mmQuickKey1.Focus()
					case 1:
						m.mmQuickKey1.Blur()
						m.mmQuickFocus = 2
						m.mmQuickKey2.Focus()
					case 2:
						m.mmQuickKey2.Blur()
						m.mmQuickFocus = 0
					}
				case 2: // manual grid
					if msg.String() == "tab" {
						m.mmTabNext()
					} else {
						m.mmMoveRow(+1)
					}
				}
				return m, nil
			}
			if m.step == StepMessaging {
				switch m.msgMode {
				case 0: // chooser
					m.msgChooserIdx = (m.msgChooserIdx + 1) % 3
				case 1, 2: // imap or telegram fields
					activeStep := StepIMAP
					if m.msgMode == 2 {
						activeStep = StepTelegram
					}
					fields := m.fields[activeStep]
					if m.focus < len(fields)-1 {
						fields[m.focus].input.Blur()
						m.focus++
						fields[m.focus].input.Focus()
						m.fields[activeStep] = fields
					}
				}
				return m, nil
			}
			if m.step == StepReview {
				break
			}
			fields := m.fields[m.step]
			if m.focus < len(fields)-1 {
				fields[m.focus].input.Blur()
				m.focus++
				fields[m.focus].input.Focus()
				m.fields[m.step] = fields
			}
			return m, nil

		case "shift+tab", "up":
			if m.step == StepCombo {
				m.comboIdx--
				if m.comboIdx < -1 {
					m.comboIdx = len(m.combos) - 1
				}
				return m, nil
			}
			if m.step == StepLang {
				m.langIdx = (m.langIdx - 1 + len(i18n.Languages)) % len(i18n.Languages)
				i18n.Lang = i18n.Languages[m.langIdx]
				return m, nil
			}
			if m.step == StepMultimodal {
				switch m.mmMode {
				case 0: // chooser
					m.mmChooserIdx = (m.mmChooserIdx - 1 + 3) % 3
				case 1: // quick setup
					switch m.mmQuickFocus {
					case 0:
						m.mmQuickFocus = 2
						m.mmQuickKey2.Focus()
					case 1:
						m.mmQuickKey1.Blur()
						m.mmQuickFocus = 0
					case 2:
						m.mmQuickKey2.Blur()
						m.mmQuickFocus = 1
						m.mmQuickKey1.Focus()
					}
				case 2: // manual grid
					if msg.String() == "shift+tab" {
						m.mmTabPrev()
					} else {
						m.mmMoveRow(-1)
					}
				}
				return m, nil
			}
			if m.step == StepMessaging {
				switch m.msgMode {
				case 0:
					m.msgChooserIdx = (m.msgChooserIdx - 1 + 3) % 3
				case 1, 2:
					activeStep := StepIMAP
					if m.msgMode == 2 {
						activeStep = StepTelegram
					}
					fields := m.fields[activeStep]
					if m.focus > 0 {
						fields[m.focus].input.Blur()
						m.focus--
						fields[m.focus].input.Focus()
						m.fields[activeStep] = fields
					}
				}
				return m, nil
			}
			if m.step == StepReview {
				break
			}
			fields := m.fields[m.step]
			if m.focus > 0 {
				fields[m.focus].input.Blur()
				m.focus--
				fields[m.focus].input.Focus()
				m.fields[m.step] = fields
			}
			return m, nil

		case "left":
			if m.step == StepLang {
				break
			}
			if m.step == StepModel && m.focus == 0 {
				m.providerIdx = (m.providerIdx - 1 + len(providers)) % len(providers)
				m.syncProviderDefaults()
				return m, nil
			}
			if m.step == StepMultimodal {
				switch m.mmMode {
				case 1: // quick setup — cycle endpoint
					if m.mmQuickFocus == 0 {
						m.mmQuickEndpoint = (m.mmQuickEndpoint - 1 + len(mmQuickEndpoints)) % len(mmQuickEndpoints)
					}
				case 2: // manual grid
					if m.mmCol == 0 {
						cap := mmCaps[m.mmRow]
						if len(cap.providers) > 1 {
							m.mmRows[m.mmRow].providerIdx = (m.mmRows[m.mmRow].providerIdx - 1 + len(cap.providers)) % len(cap.providers)
							m.syncMMDefaults(m.mmRow)
						}
					}
				}
				return m, nil
			}

		case "right":
			if m.step == StepLang {
				break
			}
			if m.step == StepModel && m.focus == 0 {
				m.providerIdx = (m.providerIdx + 1) % len(providers)
				m.syncProviderDefaults()
				return m, nil
			}
			if m.step == StepMultimodal {
				switch m.mmMode {
				case 1: // quick setup — cycle endpoint
					if m.mmQuickFocus == 0 {
						m.mmQuickEndpoint = (m.mmQuickEndpoint + 1) % len(mmQuickEndpoints)
					}
				case 2: // manual grid
					if m.mmCol == 0 {
						cap := mmCaps[m.mmRow]
						if len(cap.providers) > 1 {
							m.mmRows[m.mmRow].providerIdx = (m.mmRows[m.mmRow].providerIdx + 1) % len(cap.providers)
							m.syncMMDefaults(m.mmRow)
						}
					}
				}
				return m, nil
			}

		case "ctrl+t":
			// Run connection test
			return m, m.runTest()

		case "enter":
			if m.step == StepCombo {
				if m.comboIdx == -1 {
					// Create new — advance to StepLang
					m.advanceStep()
				} else {
					// Apply combo and skip to StepGeneral
					c := m.combos[m.comboIdx]
					m.applyCombo(c)
					m.step = StepGeneral
					m.focus = 0
					if fields, ok := m.fields[StepGeneral]; ok && len(fields) > 0 {
						fields[0].input.Focus()
						m.fields[StepGeneral] = fields
					}
				}
				return m, nil
			}
			if m.step == StepReview {
				m.written, m.err = m.writeConfig()
				m.done = true
				return m, tea.Quit
			}
			if m.step == StepMultimodal {
				switch m.mmMode {
				case 0: // chooser
					switch m.mmChooserIdx {
					case 0: // MiniMax Quick Setup
						m.mmMode = 1
						m.mmQuickFocus = 0
					case 1: // Manual Configuration
						m.mmMode = 2
					case 2: // Skip
						m.advanceStep()
					}
					return m, nil
				case 1: // quick setup — apply and advance
					m.mmQuickKey1.Blur()
					m.mmQuickKey2.Blur()
					m.mmApplyQuickSetup()
					m.advanceStep()
					return m, nil
				case 2: // manual grid — advance
					m.advanceStep()
					return m, nil
				}
			}
			if m.step == StepMessaging {
				switch m.msgMode {
				case 0: // chooser
					switch m.msgChooserIdx {
					case 0: // IMAP
						m.msgMode = 1
						m.focus = 0
						fields := m.fields[StepIMAP]
						if len(fields) > 0 {
							fields[0].input.Focus()
							m.fields[StepIMAP] = fields
						}
					case 1: // Telegram
						m.msgMode = 2
						m.focus = 0
						fields := m.fields[StepTelegram]
						if len(fields) > 0 {
							fields[0].input.Focus()
							m.fields[StepTelegram] = fields
						}
					case 2: // Skip
						m.advanceStep()
					}
					return m, nil
				case 1: // IMAP — enter on last field goes back to chooser with done flag
					fields := m.fields[StepIMAP]
					if m.focus >= len(fields)-1 {
						fields[m.focus].input.Blur()
						m.fields[StepIMAP] = fields
						m.msgImapDone = true
						m.msgMode = 0
						m.focus = 0
						return m, nil
					}
					fields[m.focus].input.Blur()
					m.focus++
					fields[m.focus].input.Focus()
					m.fields[StepIMAP] = fields
					return m, nil
				case 2: // Telegram — same pattern
					fields := m.fields[StepTelegram]
					if m.focus >= len(fields)-1 {
						fields[m.focus].input.Blur()
						m.fields[StepTelegram] = fields
						m.msgTgDone = true
						m.msgMode = 0
						m.focus = 0
						return m, nil
					}
					fields[m.focus].input.Blur()
					m.focus++
					fields[m.focus].input.Focus()
					m.fields[StepTelegram] = fields
					return m, nil
				}
			}
			// On last field of current step, advance
			fields := m.fields[m.step]
			if fields == nil || m.focus >= len(fields)-1 {
				m.advanceStep()
				return m, nil
			}
			// Otherwise move to next field
			fields[m.focus].input.Blur()
			m.focus++
			fields[m.focus].input.Focus()
			m.fields[m.step] = fields
			return m, nil
		}
	}

	// Update the focused text input
	if m.step == StepMultimodal {
		var cmd tea.Cmd
		switch m.mmMode {
		case 1: // quick setup
			if m.mmQuickFocus == 1 {
				m.mmQuickKey1, cmd = m.mmQuickKey1.Update(msg)
			} else if m.mmQuickFocus == 2 {
				m.mmQuickKey2, cmd = m.mmQuickKey2.Update(msg)
			}
		case 2: // manual grid
			if m.mmCol == 1 {
				m.mmRows[m.mmRow].keyInput, cmd = m.mmRows[m.mmRow].keyInput.Update(msg)
			} else if m.mmCol == 2 {
				m.mmRows[m.mmRow].endpointInput, cmd = m.mmRows[m.mmRow].endpointInput.Update(msg)
			}
		}
		return m, cmd
	}
	if m.step == StepMessaging && (m.msgMode == 1 || m.msgMode == 2) {
		activeStep := StepIMAP
		if m.msgMode == 2 {
			activeStep = StepTelegram
		}
		fields := m.fields[activeStep]
		if m.focus < len(fields) {
			var cmd tea.Cmd
			fields[m.focus].input, cmd = fields[m.focus].input.Update(msg)
			m.fields[activeStep] = fields
			return m, cmd
		}
	}
	if m.step == StepReview {
		var cmd tea.Cmd
		m.comboName, cmd = m.comboName.Update(msg)
		return m, cmd
	}
	if m.step != StepCombo && m.step != StepLang && m.step != StepMultimodal && m.step != StepMessaging {
		fields := m.fields[m.step]
		if m.focus < len(fields) {
			var cmd tea.Cmd
			fields[m.focus].input, cmd = fields[m.focus].input.Update(msg)
			m.fields[m.step] = fields
			return m, cmd
		}
	}

	return m, nil
}

func (m *wizardModel) syncProviderDefaults() {
	p := providers[m.providerIdx]
	m.fields[StepModel][0].input.SetValue(p)
	m.fields[StepModel][1].input.SetValue(providerModels[p])
	m.fields[StepModel][3].input.SetValue(providerEndpoints[p])
}

func (m *wizardModel) mmIsLocal(row int) bool {
	return mmCaps[row].providers[m.mmRows[row].providerIdx] == "local"
}

func (m *wizardModel) mmBlurCurrent() {
	if m.mmCol == 1 {
		m.mmRows[m.mmRow].keyInput.Blur()
	}
	if m.mmCol == 2 {
		m.mmRows[m.mmRow].endpointInput.Blur()
	}
}

func (m *wizardModel) mmFocusCurrent() {
	if m.mmCol == 1 {
		m.mmRows[m.mmRow].keyInput.Focus()
	}
	if m.mmCol == 2 {
		m.mmRows[m.mmRow].endpointInput.Focus()
	}
}

func (m *wizardModel) mmMoveRow(delta int) {
	m.mmBlurCurrent()
	m.mmRow = (m.mmRow + delta + len(mmCaps)) % len(mmCaps)
	m.mmFocusCurrent()
}

func (m *wizardModel) mmTabNext() {
	m.mmBlurCurrent()
	if m.mmIsLocal(m.mmRow) {
		m.mmRow = (m.mmRow + 1) % len(mmCaps)
		m.mmCol = 0
	} else {
		next := (m.mmCol + 1) % 3
		if next == 0 {
			m.mmRow = (m.mmRow + 1) % len(mmCaps)
		}
		m.mmCol = next
	}
	m.mmFocusCurrent()
}

func (m *wizardModel) mmTabPrev() {
	m.mmBlurCurrent()
	if m.mmCol == 0 {
		m.mmRow = (m.mmRow - 1 + len(mmCaps)) % len(mmCaps)
		if m.mmIsLocal(m.mmRow) {
			m.mmCol = 0
		} else {
			m.mmCol = 2
		}
	} else {
		m.mmCol--
	}
	m.mmFocusCurrent()
}

func (m *wizardModel) syncMMDefaults(row int) {
	cap := mmCaps[row]
	p := cap.providers[m.mmRows[row].providerIdx]
	if ep, ok := cap.endpoints[p]; ok {
		m.mmRows[row].endpointInput.SetValue(ep)
	} else {
		m.mmRows[row].endpointInput.SetValue("")
	}
}

// mmApplyQuickSetup fills all non-local mmRows from the quick setup keys and endpoint.
func (m *wizardModel) mmApplyQuickSetup() {
	ep := mmQuickEndpoints[m.mmQuickEndpoint]
	apiKey := m.mmQuickKey1.Value()
	mcpKey := m.mmQuickKey2.Value()

	for i, cap := range mmCaps {
		if cap.providers[0] == "local" {
			continue
		}
		// Set provider to minimax
		m.mmRows[i].providerIdx = 0
		m.mmRows[i].endpointInput.SetValue(ep)
		// vision + web_search use MINIMAX_API_KEY, others use MINIMAX_MCP_API_KEY
		if cap.configKey == "vision" || cap.configKey == "web_search" {
			m.mmRows[i].keyInput.SetValue(apiKey)
		} else {
			m.mmRows[i].keyInput.SetValue(mcpKey)
		}
	}
}

func (m *wizardModel) advanceStep() {
	// Blur current fields
	if m.step == StepMultimodal {
		m.mmBlurCurrent()
	} else if fields, ok := m.fields[m.step]; ok {
		for i := range fields {
			fields[i].input.Blur()
		}
		m.fields[m.step] = fields
	}

	m.step++
	m.focus = 0
	m.mmRow = 0
	m.mmCol = 0

	// Focus first field of new step
	if m.step == StepReview {
		m.comboName.Focus()
	} else if m.step != StepMultimodal {
		if fields, ok := m.fields[m.step]; ok && len(fields) > 0 {
			fields[0].input.Focus()
			m.fields[m.step] = fields
		}
	}
}

// applyCombo pre-fills wizard fields from a saved combo.
func (m *wizardModel) applyCombo(c combo.Combo) {
	// Set env vars from combo
	for k, v := range c.Env {
		os.Setenv(k, v)
	}

	// Set language from combo config
	if lang, ok := c.Config["language"].(string); ok {
		for idx, code := range i18n.Languages {
			if code == lang {
				m.langIdx = idx
				i18n.Lang = code
				break
			}
		}
	}

	// Set model fields
	if provider, ok := c.Model["provider"].(string); ok {
		for idx, p := range providers {
			if p == provider {
				m.providerIdx = idx
				break
			}
		}
		m.fields[StepModel][0].input.SetValue(provider)
	}
	if model, ok := c.Model["model"].(string); ok {
		m.fields[StepModel][1].input.SetValue(model)
	}
	if apiKeyEnv, ok := c.Model["api_key_env"].(string); ok {
		if key := os.Getenv(apiKeyEnv); key != "" {
			m.fields[StepModel][2].input.SetValue(key)
		}
	}
	if baseURL, ok := c.Model["base_url"].(string); ok {
		m.fields[StepModel][3].input.SetValue(baseURL)
	}

	// Set general config fields
	if agentName, ok := c.Config["agent_name"].(string); ok {
		m.fields[StepGeneral][0].input.SetValue(agentName)
	}
	if port, ok := c.Config["agent_port"].(float64); ok {
		m.fields[StepGeneral][1].input.SetValue(fmt.Sprintf("%d", int(port)))
	}

	// Pre-fill combo name for save
	m.comboName.SetValue(c.Name)
}

func (m wizardModel) renderMMChooser() string {
	var b strings.Builder

	choices := []string{
		"MiniMax Quick Setup",
		"Manual Configuration",
		"Skip",
	}
	descs := []string{
		"Enter two API keys — fills vision, web search, talk, compose, draw automatically",
		"Configure each capability individually with any provider",
		"Skip multimodal setup for now",
	}

	for i, choice := range choices {
		if i == m.mmChooserIdx {
			b.WriteString(fmt.Sprintf("  %s  %s\n", promptStyle.Render(">"), promptStyle.Render(choice)))
			b.WriteString(fmt.Sprintf("       %s\n", dimStyle.Render(descs[i])))
		} else {
			b.WriteString(fmt.Sprintf("      %s\n", dimStyle.Render(choice)))
		}
	}

	b.WriteString("\n" + dimStyle.Render("↑/↓ to select, Enter to confirm") + "\n")
	return b.String()
}

func (m wizardModel) renderMMQuick() string {
	var b strings.Builder

	b.WriteString(promptStyle.Render("MiniMax Quick Setup") + "\n\n")

	// Endpoint selector
	ep := mmQuickEndpoints[m.mmQuickEndpoint]
	epLabel := mmQuickEndpointLabels[m.mmQuickEndpoint]
	if m.mmQuickFocus == 0 {
		b.WriteString(fmt.Sprintf("  %s Endpoint: ◀ %s (%s) ▶\n", promptStyle.Render(">"), ep, epLabel))
	} else {
		b.WriteString(fmt.Sprintf("    Endpoint: %s (%s)\n", dimStyle.Render(ep), dimStyle.Render(epLabel)))
	}
	b.WriteString("\n")

	// Key 1: MINIMAX_API_KEY
	label1 := "MINIMAX_API_KEY (vision, web search)"
	if m.mmQuickFocus == 1 {
		b.WriteString(fmt.Sprintf("  %s %s\n", promptStyle.Render(">"), promptStyle.Render(label1)))
		b.WriteString(fmt.Sprintf("    %s\n", m.mmQuickKey1.View()))
	} else {
		b.WriteString(fmt.Sprintf("    %s\n", label1))
		if m.mmQuickKey1.Value() != "" {
			b.WriteString(fmt.Sprintf("    %s\n", "••••••••"))
		} else {
			b.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render("(not set)")))
		}
	}
	b.WriteString("\n")

	// Key 2: MINIMAX_MCP_API_KEY
	label2 := "MINIMAX_MCP_API_KEY (talk, compose, draw)"
	if m.mmQuickFocus == 2 {
		b.WriteString(fmt.Sprintf("  %s %s\n", promptStyle.Render(">"), promptStyle.Render(label2)))
		b.WriteString(fmt.Sprintf("    %s\n", m.mmQuickKey2.View()))
	} else {
		b.WriteString(fmt.Sprintf("    %s\n", label2))
		if m.mmQuickKey2.Value() != "" {
			b.WriteString(fmt.Sprintf("    %s\n", "••••••••"))
		} else {
			b.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render("(not set)")))
		}
	}

	b.WriteString("\n" + dimStyle.Render("Tab: next field | ←/→: cycle endpoint | Enter: apply & continue | Esc: back") + "\n")
	return b.String()
}

func (m wizardModel) renderMsgChooser() string {
	var b strings.Builder

	choices := []struct{ name, desc string }{
		{"IMAP Email", "Connect to an IMAP/SMTP email account"},
		{"Telegram Bot", "Connect a Telegram bot"},
		{"Skip", "Skip external messaging setup"},
	}

	// Show checkmarks for completed configs
	for i, ch := range choices {
		done := ""
		if i == 0 && m.msgImapDone {
			done = successStyle.Render(" ✓")
		}
		if i == 1 && m.msgTgDone {
			done = successStyle.Render(" ✓")
		}

		if i == m.msgChooserIdx {
			b.WriteString(fmt.Sprintf("  %s  %s%s\n", promptStyle.Render(">"), promptStyle.Render(ch.name), done))
			b.WriteString(fmt.Sprintf("       %s\n", dimStyle.Render(ch.desc)))
		} else {
			b.WriteString(fmt.Sprintf("      %s%s\n", dimStyle.Render(ch.name), done))
		}
	}

	b.WriteString("\n" + dimStyle.Render("↑/↓ to select, Enter to configure, Esc to go back") + "\n")
	return b.String()
}

func (m wizardModel) renderMsgFields(fieldStep step, title string) string {
	var b strings.Builder

	b.WriteString(promptStyle.Render(title) + "\n\n")

	fields := m.fields[fieldStep]
	for i, f := range fields {
		cursor := "  "
		if i == m.focus {
			cursor = promptStyle.Render("> ")
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, promptStyle.Render(f.label)))
		b.WriteString(fmt.Sprintf("  %s\n", f.input.View()))
	}

	b.WriteString("\n" + dimStyle.Render("Tab/↓: next field | Enter: save & back | Esc: back | Ctrl+T: test") + "\n")
	return b.String()
}

func (m wizardModel) renderMultimodal() string {
	var b strings.Builder

	// Column headers
	b.WriteString(fmt.Sprintf("  %-16s %-20s %-24s %s\n",
		"Capability", "Provider", "API Key", "Endpoint"))
	b.WriteString("  " + strings.Repeat("\u2500", 72) + "\n")

	for i, cap := range mmCaps {
		state := m.mmRows[i]
		p := cap.providers[state.providerIdx]
		isActive := i == m.mmRow
		isLocal := p == "local"

		// Cursor
		cursor := "  "
		if isActive {
			cursor = promptStyle.Render("> ")
		}

		// Capability name
		capName := fmt.Sprintf("%-14s", cap.name)
		if isActive {
			capName = promptStyle.Render(capName)
		}

		// Provider
		var provStr string
		if isActive && m.mmCol == 0 && len(cap.providers) > 1 {
			provStr = fmt.Sprintf("\u25c0 %-8s \u25b6", p)
		} else {
			provStr = fmt.Sprintf("  %-8s  ", p)
		}
		provStr = fmt.Sprintf("%-18s", provStr)

		// Key
		var keyStr string
		if isLocal {
			keyStr = fmt.Sprintf("%-22s", dimStyle.Render("no config needed"))
		} else if isActive && m.mmCol == 1 {
			keyStr = state.keyInput.View()
		} else if state.keyInput.Value() != "" {
			keyStr = fmt.Sprintf("%-22s", "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022")
		} else {
			keyStr = fmt.Sprintf("%-22s", dimStyle.Render("(no key)"))
		}

		// Endpoint
		var epStr string
		if isLocal {
			epStr = dimStyle.Render("runs locally")
		} else if isActive && m.mmCol == 2 {
			epStr = state.endpointInput.View()
		} else if state.endpointInput.Value() != "" {
			epStr = state.endpointInput.Value()
		} else {
			epStr = dimStyle.Render("(no endpoint)")
		}

		b.WriteString(fmt.Sprintf("%s%s %s %s  %s\n", cursor, capName, provStr, keyStr, epStr))
	}

	// Hints
	b.WriteString("\n" + dimStyle.Render("\u2191/\u2193: move row | Tab: next field | \u2190/\u2192: cycle provider | Enter: next step | Esc: back") + "\n")

	return b.String()
}

func (m wizardModel) View() string {
	if m.done {
		if m.err != nil {
			return errorStyle.Render(fmt.Sprintf("Error: %v\n", m.err))
		}
		var b strings.Builder
		b.WriteString(successStyle.Render(i18n.S("setup_saved")) + "\n\n")
		b.WriteString(i18n.S("setup_files") + "\n")
		for _, f := range m.written {
			b.WriteString(fmt.Sprintf("  %s %s\n", successStyle.Render("\u2713"), f))
		}
		return b.String()
	}

	var b strings.Builder

	// Banner
	if m.langIdx == 0 {
		b.WriteString(headerStyle.Render("LingTai AI") + "\n")
		b.WriteString(dimStyle.Render("Awakened beneath the Bodhi;") + "\n")
		b.WriteString(dimStyle.Render("one mind, thousand avatars.") + "\n\n")
	} else {
		b.WriteString(headerStyle.Render("灵台AI") + "\n")
		b.WriteString(dimStyle.Render("灵台方寸山  斜月三星洞") + "\n")
		b.WriteString(dimStyle.Render("闻道菩提下  一心化万相") + "\n\n")
	}

	// Progress bar
	allSteps := []step{StepCombo, StepLang, StepModel, StepMultimodal, StepMessaging, StepGeneral, StepReview}
	for i, s := range allSteps {
		name := s.String()
		if s == m.step {
			b.WriteString(promptStyle.Render(fmt.Sprintf("[%s]", name)))
		} else if s < m.step {
			b.WriteString(successStyle.Render(fmt.Sprintf(" %s ", name)))
		} else {
			b.WriteString(dimStyle.Render(fmt.Sprintf(" %s ", name)))
		}
		if i < len(allSteps)-1 {
			b.WriteString(dimStyle.Render(" > "))
		}
	}
	b.WriteString("\n\n")

	// Section header
	b.WriteString(headerStyle.Render(m.step.String()) + "\n\n")

	// Combo selector
	if m.step == StepCombo {
		var cb strings.Builder
		cb.WriteString("\n  Select a combo or create new:\n\n")

		// "Create new" option
		cursor := " "
		if m.comboIdx == -1 {
			cursor = ">"
		}
		cb.WriteString(fmt.Sprintf("  %s [Create new]\n", cursor))

		// List existing combos
		for i, c := range m.combos {
			cursor = " "
			if m.comboIdx == i {
				cursor = ">"
			}
			provider, _ := c.Model["provider"].(string)
			model, _ := c.Model["model"].(string)
			cb.WriteString(fmt.Sprintf("  %s %s (%s/%s)\n", cursor, c.Name, provider, model))
		}

		cb.WriteString("\n" + dimStyle.Render("  ↑/↓ navigate  Enter select") + "\n")
		b.WriteString(cb.String())
		return b.String()
	}

	// Language selector (no text fields)
	if m.step == StepLang {
		for idx, code := range i18n.Languages {
			label := i18n.LanguageLabels[code]
			if idx == m.langIdx {
				b.WriteString(fmt.Sprintf("  %s  %s\n", promptStyle.Render(">"), promptStyle.Render(label)))
			} else {
				b.WriteString(fmt.Sprintf("     %s\n", dimStyle.Render(label)))
			}
		}
		b.WriteString("\n" + dimStyle.Render(i18n.S("setup_lang_hint")) + "\n")
		return b.String()
	}

	if m.step == StepMultimodal {
		switch m.mmMode {
		case 0:
			b.WriteString(m.renderMMChooser())
		case 1:
			b.WriteString(m.renderMMQuick())
		case 2:
			b.WriteString(m.renderMultimodal())
		}
		return b.String()
	}

	if m.step == StepMessaging {
		switch m.msgMode {
		case 0:
			b.WriteString(m.renderMsgChooser())
		case 1:
			b.WriteString(m.renderMsgFields(StepIMAP, "IMAP / SMTP"))
		case 2:
			b.WriteString(m.renderMsgFields(StepTelegram, "Telegram"))
		}
		return b.String()
	}

	if m.step == StepReview {
		b.WriteString(m.renderReview())
		b.WriteString("\n" + dimStyle.Render("Enter → save, Ctrl+C → abort") + "\n")
		return b.String()
	}

	// Render fields
	fields := m.fields[m.step]
	for i, f := range fields {
		// Skip base_url field unless provider is custom
		if m.step == StepModel && i == 3 {
			provider := m.fields[StepModel][0].input.Value()
			if provider != "custom" {
				continue
			}
		}

		cursor := "  "
		if i == m.focus {
			cursor = promptStyle.Render("> ")
		}

		label := f.label
		if m.step == StepModel && i == 0 {
			label = fmt.Sprintf("%s (left/right to cycle)", label)
		}

		b.WriteString(fmt.Sprintf("%s%s\n", cursor, promptStyle.Render(label)))
		b.WriteString(fmt.Sprintf("  %s\n", f.input.View()))
	}

	// Show test result if any
	if tr, ok := m.testResults[m.step]; ok {
		b.WriteString("\n")
		if tr.OK {
			b.WriteString(fmt.Sprintf("  %s %s\n", successStyle.Render("\u2713"), tr.Message))
		} else {
			b.WriteString(fmt.Sprintf("  %s %s\n", errorStyle.Render("\u2717"), tr.Message))
		}
	}

	// Hints
	b.WriteString("\n")
	hints := []string{"Tab/Down: next field", "Shift+Tab/Up: prev field", "Enter: next step"}
	b.WriteString(dimStyle.Render(strings.Join(hints, " | ")) + "\n")

	return b.String()
}

func (m wizardModel) renderReview() string {
	var b strings.Builder

	// Language
	langCode := i18n.Languages[m.langIdx]
	langLabel := i18n.LanguageLabels[langCode]
	b.WriteString(promptStyle.Render(i18n.S("setup_lang")+":") + fmt.Sprintf(" %s (%s)\n", langLabel, langCode))

	// Model
	provider := m.fieldVal(StepModel, 0)
	b.WriteString("\n" + promptStyle.Render("Model:") + "\n")
	b.WriteString(fmt.Sprintf("  Provider:    %s\n", provider))
	b.WriteString(fmt.Sprintf("  Model:       %s\n", m.fieldVal(StepModel, 1)))
	if m.fieldVal(StepModel, 2) != "" {
		b.WriteString(fmt.Sprintf("  API key:     %s\n", "••••••••"))
	}
	if endpoint := m.fieldVal(StepModel, 3); endpoint != "" {
		b.WriteString(fmt.Sprintf("  Endpoint:    %s\n", endpoint))
	}

	// Multimodal capabilities
	for i, cap := range mmCaps {
		state := m.mmRows[i]
		p := cap.providers[state.providerIdx]
		if p == "local" {
			b.WriteString(fmt.Sprintf("\n"+dimStyle.Render("%s: runs locally")+"\n", cap.name))
			continue
		}
		key := state.keyInput.Value()
		ep := state.endpointInput.Value()
		if key == "" && ep == "" {
			b.WriteString(fmt.Sprintf("\n"+dimStyle.Render("%s: skipped")+"\n", cap.name))
			continue
		}
		b.WriteString(fmt.Sprintf("\n"+promptStyle.Render("%s:")+"\n", cap.name))
		b.WriteString(fmt.Sprintf("  Provider:    %s\n", p))
		if key != "" {
			b.WriteString(fmt.Sprintf("  API key:     %s\n", "••••••••"))
		} else {
			b.WriteString(fmt.Sprintf("  API key:     %s\n", dimStyle.Render("reusing main key")))
		}
		if ep != "" {
			b.WriteString(fmt.Sprintf("  Endpoint:    %s\n", ep))
		}
	}

	// IMAP
	if m.fieldVal(StepIMAP, 0) != "" {
		b.WriteString("\n" + promptStyle.Render("IMAP/SMTP:") + "\n")
		b.WriteString(fmt.Sprintf("  Email:     %s\n", m.fieldVal(StepIMAP, 0)))
		b.WriteString(fmt.Sprintf("  Password:  %s\n", "••••••••"))
		b.WriteString(fmt.Sprintf("  IMAP:      %s:%s\n", m.fieldVal(StepIMAP, 2), m.fieldVal(StepIMAP, 3)))
		b.WriteString(fmt.Sprintf("  SMTP:      %s:%s\n", m.fieldVal(StepIMAP, 4), m.fieldVal(StepIMAP, 5)))
		m.renderTestResult(&b, StepIMAP)
	} else {
		b.WriteString("\n" + dimStyle.Render("IMAP/SMTP: skipped") + "\n")
	}

	// Telegram
	if m.fieldVal(StepTelegram, 0) != "" {
		b.WriteString("\n" + promptStyle.Render("Telegram:") + "\n")
		b.WriteString(fmt.Sprintf("  Token:     %s\n", "••••••••"))
		m.renderTestResult(&b, StepTelegram)
	} else {
		b.WriteString("\n" + dimStyle.Render("Telegram: skipped") + "\n")
	}

	// General
	b.WriteString("\n" + promptStyle.Render("General:") + "\n")
	b.WriteString(fmt.Sprintf("  Agent Name: %s\n", m.fieldVal(StepGeneral, 0)))
	b.WriteString(fmt.Sprintf("  Port:       %s\n", m.fieldVal(StepGeneral, 1)))
	if v := m.fieldVal(StepGeneral, 2); v != "" {
		b.WriteString(fmt.Sprintf("  Bash Policy: %s\n", v))
	}

	// Combo name
	b.WriteString("\n" + promptStyle.Render("Save as combo:") + " ")
	b.WriteString(m.comboName.View() + "\n")

	// Save location
	b.WriteString("\n" + dimStyle.Render(fmt.Sprintf("Config → %s/configs/config.json", m.outputDir)) + "\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Secrets → %s/configs/.env", m.outputDir)) + "\n")

	return b.String()
}

func (m wizardModel) renderTestResult(b *strings.Builder, s step) {
	if tr, ok := m.testResults[s]; ok {
		if tr.OK {
			b.WriteString(fmt.Sprintf("  %s %s\n", successStyle.Render("\u2713"), tr.Message))
		} else {
			b.WriteString(fmt.Sprintf("  %s %s\n", errorStyle.Render("\u2717"), tr.Message))
		}
	}
}

func (m wizardModel) fieldVal(s step, idx int) string {
	fields, ok := m.fields[s]
	if !ok || idx >= len(fields) {
		return ""
	}
	return fields[idx].input.Value()
}

func (m wizardModel) runTest() tea.Cmd {
	if m.step != StepMessaging {
		return nil
	}
	switch m.msgMode {
	case 1: // IMAP
		return func() tea.Msg {
			email := m.fieldVal(StepIMAP, 0)
			pass := m.fieldVal(StepIMAP, 1)
			imapHost := m.fieldVal(StepIMAP, 2)
			imapPortStr := m.fieldVal(StepIMAP, 3)

			if pass == "" {
				return testResultMsg{step: StepIMAP, result: TestResult{OK: false, Message: "password is required"}}
			}

			imapPort, _ := strconv.Atoi(imapPortStr)
			if imapPort == 0 {
				imapPort = 993
			}

			r := TestIMAP(imapHost, imapPort, email, pass)
			return testResultMsg{step: StepIMAP, result: r}
		}

	case 2: // Telegram
		return func() tea.Msg {
			token := m.fieldVal(StepTelegram, 0)
			if token == "" {
				return testResultMsg{step: StepTelegram, result: TestResult{OK: false, Message: "bot token is required"}}
			}
			r := TestTelegram(token)
			return testResultMsg{step: StepTelegram, result: r}
		}

	default:
		return nil
	}
}

func (m wizardModel) writeConfig() ([]string, error) {
	// Config files go to configs/ subdirectory
	configsDir := filepath.Join(m.outputDir, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create configs directory: %w", err)
	}

	var written []string

	// Derive env var name from provider
	provider := m.fieldVal(StepModel, 0)
	apiKeyEnv := strings.ToUpper(provider) + "_API_KEY"

	// 1. model.json
	modelCfg := map[string]interface{}{
		"provider":    provider,
		"model":       m.fieldVal(StepModel, 1),
		"api_key_env": apiKeyEnv,
	}
	if endpoint := m.fieldVal(StepModel, 3); endpoint != "" {
		modelCfg["base_url"] = endpoint
	}

	// Multimodal capability configs
	for i, cap := range mmCaps {
		state := m.mmRows[i]
		p := cap.providers[state.providerIdx]
		if p == "local" {
			continue
		}
		key := state.keyInput.Value()
		ep := state.endpointInput.Value()
		if key == "" && ep == "" {
			continue
		}

		capKeyEnv := apiKeyEnv // reuse main key by default
		if key != "" && key != m.fieldVal(StepModel, 2) {
			capKeyEnv = strings.ToUpper(p) + "_" + cap.envSuffix + "_API_KEY"
		}

		capCfg := map[string]interface{}{
			"provider":    p,
			"api_key_env": capKeyEnv,
		}
		if ep != "" {
			capCfg["base_url"] = ep
		}
		modelCfg[cap.configKey] = capCfg
	}

	modelPath := filepath.Join(configsDir, "model.json")
	if err := writeJSON(modelPath, modelCfg); err != nil {
		return written, fmt.Errorf("writing model.json: %w", err)
	}
	written = append(written, modelPath)

	// 2. config.json
	port, _ := strconv.Atoi(m.fieldVal(StepGeneral, 1))
	if port == 0 {
		port = 8501
	}

	agentName := m.fieldVal(StepGeneral, 0)
	cfg := map[string]interface{}{
		"model":      "model.json",
		"language":   i18n.Languages[m.langIdx],
		"agent_name": agentName,
		"agent_port": port,
	}

	bashPolicy := m.fieldVal(StepGeneral, 2)
	if bashPolicy == "" {
		bashPolicy = filepath.Join(configsDir, "bash_policy.json")
	}
	cfg["bash_policy"] = bashPolicy

	// IMAP config
	if email := m.fieldVal(StepIMAP, 0); email != "" {
		imapPort, _ := strconv.Atoi(m.fieldVal(StepIMAP, 3))
		smtpPort, _ := strconv.Atoi(m.fieldVal(StepIMAP, 5))
		cfg["imap"] = map[string]interface{}{
			"email_address": email,
			"password_env":  "IMAP_PASSWORD",
			"imap_host":     m.fieldVal(StepIMAP, 2),
			"imap_port":     imapPort,
			"smtp_host":     m.fieldVal(StepIMAP, 4),
			"smtp_port":     smtpPort,
		}
	}

	// Telegram config
	if token := m.fieldVal(StepTelegram, 0); token != "" {
		cfg["telegram"] = map[string]interface{}{
			"bot_token_env": "TELEGRAM_BOT_TOKEN",
		}
	}

	configPath := filepath.Join(configsDir, "config.json")
	if err := writeJSON(configPath, cfg); err != nil {
		return written, fmt.Errorf("writing config.json: %w", err)
	}
	written = append(written, configPath)

	// 3. .env (save actual secrets)
	var envLines []string
	if apiKey := m.fieldVal(StepModel, 2); apiKey != "" {
		envLines = append(envLines, fmt.Sprintf("%s=%s", apiKeyEnv, apiKey))
	}
	// Multimodal capability keys
	for i, cap := range mmCaps {
		state := m.mmRows[i]
		p := cap.providers[state.providerIdx]
		if p == "local" {
			continue
		}
		key := state.keyInput.Value()
		if key != "" && key != m.fieldVal(StepModel, 2) {
			capKeyEnv := strings.ToUpper(p) + "_" + cap.envSuffix + "_API_KEY"
			envLines = append(envLines, fmt.Sprintf("%s=%s", capKeyEnv, key))
		}
	}
	if password := m.fieldVal(StepIMAP, 1); password != "" {
		envLines = append(envLines, fmt.Sprintf("IMAP_PASSWORD=%s", password))
	}
	if token := m.fieldVal(StepTelegram, 0); token != "" {
		envLines = append(envLines, fmt.Sprintf("TELEGRAM_BOT_TOKEN=%s", token))
	}
	if len(envLines) > 0 {
		envPath := filepath.Join(configsDir, ".env")
		content := "# LingTai secrets — do not commit this file\n\n" + strings.Join(envLines, "\n") + "\n"
		if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
			return written, fmt.Errorf("writing .env: %w", err)
		}
		written = append(written, envPath)
	}

	// 4. Default files
	bashPolicyPath := filepath.Join(configsDir, "bash_policy.json")
	if _, err := os.Stat(bashPolicyPath); os.IsNotExist(err) {
		os.WriteFile(bashPolicyPath, []byte(defaultBashPolicy), 0644)
		written = append(written, bashPolicyPath)
	}

	// 5. Default covenant — written to agent working dir (per-agent, not per-project)
	agentDir := filepath.Join(m.outputDir, agentName)
	os.MkdirAll(agentDir, 0755)
	covenantPath := filepath.Join(agentDir, "covenant.md")
	if _, err := os.Stat(covenantPath); os.IsNotExist(err) {
		defaultCovenant := defaultCovenantEN
		toolTranslation := ""
		langCode := i18n.Languages[m.langIdx]
		if langCode == "lzh" {
			defaultCovenant = defaultCovenantWEN
			toolTranslation = toolTranslationWEN
		} else if langCode == "zh" {
			defaultCovenant = defaultCovenantZH
			toolTranslation = toolTranslationZH
		}
		if toolTranslation != "" {
			defaultCovenant += "\n---\n\n" + toolTranslation
		}
		os.WriteFile(covenantPath, []byte(defaultCovenant), 0644)
		written = append(written, covenantPath)
	}

	// Save combo
	if name := m.comboName.Value(); name != "" {
		envMap := make(map[string]string)
		for _, line := range envLines {
			if k, v, ok := strings.Cut(line, "="); ok {
				envMap[k] = v
			}
		}
		combo.Save(combo.Combo{
			Name:  name,
			Model: modelCfg,
			Config: map[string]interface{}{
				"agent_name": agentName,
				"agent_port": port,
				"language":   i18n.Languages[m.langIdx],
			},
			Env: envMap,
		})
	}

	return written, nil
}

func writeJSON(path string, data interface{}) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

// Run starts the interactive setup wizard, writing config to outputDir.
func Run(outputDir string) error {
	m := newWizardModel(outputDir)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("wizard error: %w", err)
	}
	if wm, ok := finalModel.(wizardModel); ok && wm.err != nil {
		return wm.err
	}
	return nil
}
