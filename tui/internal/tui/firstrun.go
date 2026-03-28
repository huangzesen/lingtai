package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/preset"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FirstRunDoneMsg is emitted when first-run flow completes.
type FirstRunDoneMsg struct {
	OrchDir  string // full path to orchestrator directory
	OrchName string // agent name
}

// bootstrapDoneMsg signals that background setup (venv + assets) finished.
type bootstrapDoneMsg struct{}

// bootstrapErrMsg signals that background setup failed.
type bootstrapErrMsg struct{ err string }

// bootstrapProgressMsg reports a setup progress step (i18n key).
type bootstrapProgressMsg struct{ key string }

type firstRunStep int

const (
	stepWelcome firstRunStep = iota
	stepAPIKey
	stepPickPreset
	stepPresetKey
	stepAgentNameDir
	stepLaunching
)

// stepCount is the total number of wizard steps (for progress display)
const totalSteps = 4

// stepProgress returns the 1-based index and total for progress display
func stepProgress(step firstRunStep, hasPresets bool) (current int, total int) {
	total = totalSteps
	switch {
	case !hasPresets && step == stepAPIKey:
		return 1, total
	case !hasPresets && step == stepPickPreset:
		return 2, total
	case step == stepPickPreset || step == stepPresetKey:
		return 1, total
	case step == stepAgentNameDir:
		return 3, total
	case step == stepLaunching:
		return 4, total
	}
	return 1, total
}

// FirstRunModel orchestrates the first-run experience.
type FirstRunModel struct {
	step       firstRunStep
	setup      SetupModel
	presets    []preset.Preset
	cursor     int
	nameInput  textinput.Model
	dirInput   textinput.Model
	agentName  string
	agentDir   string
	message    string
	baseDir    string // .lingtai/ directory
	globalDir  string
	width      int
	height     int
	hasPresets bool
	// Focus state for combined name+dir step
	focusOnDir bool // legacy — replaced by fieldIdx
	fieldIdx   int  // 0=name, 1=dir, 2=lang, 3=stamina, 4=context_limit, 5=soul_delay, 6=molt_pressure
	// Agent config text inputs
	agentLangIdx     int // cycle: 0=en, 1=zh, 2=wen
	staminaInput     textinput.Model
	ctxLimitInput    textinput.Model
	soulDelayInput   textinput.Model
	moltPressInput   textinput.Model
	// Welcome page language selector
	langCursor   int
	welcomeOnly  bool // true when opened from /settings (return to mail after language pick)
	// Bootstrap state (venv + assets install)
	setupDone    bool           // true when bootstrap goroutine finishes
	setupErr     string         // non-empty if bootstrap failed
	setupStatus  string         // current progress i18n key
	progressCh   chan string     // channel for progress updates
	// Embedded key input for preset's provider
	presetKeyInput    textinput.Model
	presetEndpointIn  textinput.Model // base_url for custom provider
	presetModelIn     textinput.Model // model name for custom provider
	presetNameIn      textinput.Model // preset name for custom provider (separate from nameInput)
	presetKeyFieldIdx int             // 0=compat, 1=endpoint, 2=model, 3=key, 4=name (custom); 0=region,1=key (minimax)
	minimaxRegion     int             // 0=international, 1=china
	customCompat      int             // 0=openai, 1=anthropic
	selectedProvider  string          // provider of currently selected preset
	existingKeys      map[string]string // loaded from Config.Keys
}

func NewFirstRunModel(baseDir, globalDir string, hasPresets bool) FirstRunModel {
	ti := textinput.New()
	ti.CharLimit = 64
	ti.Width = 40

	di := textinput.New()
	di.CharLimit = 64
	di.Width = 40


	pki := textinput.New()
	pki.CharLimit = 128
	pki.Width = 50

	pei := textinput.New() // endpoint input for custom provider
	pei.CharLimit = 256
	pei.Width = 50
	pei.Placeholder = "https://openrouter.ai/api/v1"

	pmi := textinput.New() // model input for custom provider
	pmi.CharLimit = 64
	pmi.Width = 50
	pmi.Placeholder = "model-name"

	pni := textinput.New() // preset name input for custom provider
	pni.CharLimit = 64
	pni.Width = 50
	pni.Placeholder = "openrouter"

	si := textinput.New()
	si.CharLimit = 10
	si.Width = 15
	si.Prompt = ""

	ci := textinput.New()
	ci.CharLimit = 10
	ci.Width = 15
	ci.Prompt = ""

	sdi := textinput.New()
	sdi.CharLimit = 10
	sdi.Width = 15
	sdi.Prompt = ""

	mpi := textinput.New()
	mpi.CharLimit = 6
	mpi.Width = 15
	mpi.Prompt = ""

	// Load existing keys from Config.Keys
	cfg, _ := config.LoadConfig(globalDir)
	existingKeys := cfg.Keys
	if existingKeys == nil {
		existingKeys = make(map[string]string)
	}

	// Pre-set language cursor from global config
	langCursor := 0
	langOptions := []string{"en", "zh", "wen"}
	if cfg.Language != "" {
		for i, l := range langOptions {
			if l == cfg.Language {
				langCursor = i
				break
			}
		}
	}

	m := FirstRunModel{
		step:            stepWelcome,
		baseDir:         baseDir,
		globalDir:       globalDir,
		nameInput:       ti,
		dirInput:        di,
		hasPresets:      hasPresets,
		langCursor:      langCursor,
		presetKeyInput:   pki,
		presetEndpointIn: pei,
		presetModelIn:    pmi,
		presetNameIn:     pni,
		existingKeys:     existingKeys,
		staminaInput:    si,
		ctxLimitInput:   ci,
		soulDelayInput:  sdi,
		moltPressInput:  mpi,
		progressCh:      make(chan string, 4),
	}

	return m
}

func (m FirstRunModel) Init() tea.Cmd {
	if m.welcomeOnly {
		// Already bootstrapped — immediately signal done
		return func() tea.Msg { return bootstrapDoneMsg{} }
	}
	return tea.Batch(
		m.runBootstrap(m.progressCh),
		waitForProgress(m.progressCh),
	)
}

// waitForProgress listens on the progress channel and emits tea messages.
func waitForProgress(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		key, ok := <-ch
		if !ok {
			return nil // channel closed, bootstrap goroutine handles done/err
		}
		return bootstrapProgressMsg{key: key}
	}
}

// runBootstrap runs venv creation + asset population in a goroutine.
func (m FirstRunModel) runBootstrap(ch chan<- string) tea.Cmd {
	return func() tea.Msg {
		progress := func(key string) {
			ch <- key
		}
		// Venv (slow — creates venv + pip install). Quiet mode: no stdout/stderr leak.
		if err := config.EnsureVenvQuiet(m.globalDir, progress); err != nil {
			close(ch)
			return bootstrapErrMsg{err: err.Error()}
		}
		// Assets + default presets (fast)
		progress("welcome.step_presets")
		if err := preset.Bootstrap(m.globalDir); err != nil {
			close(ch)
			return bootstrapErrMsg{err: err.Error()}
		}
		close(ch)
		return bootstrapDoneMsg{}
	}
}

func (m FirstRunModel) Update(msg tea.Msg) (FirstRunModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case bootstrapProgressMsg:
		m.setupStatus = msg.key
		return m, waitForProgress(m.progressCh)

	case bootstrapDoneMsg:
		m.setupDone = true
		m.setupStatus = ""
		return m, nil

	case bootstrapErrMsg:
		m.setupDone = true
		m.setupErr = msg.err
		return m, nil

	case SetupDoneMsg:
		// API key saved -> move to preset picker (presets already created by Bootstrap)
		m.presets, _ = preset.List()
		// Reload keys after setup saves
		cfg, _ := config.LoadConfig(m.globalDir)
		m.existingKeys = cfg.Keys
		if m.existingKeys == nil {
			m.existingKeys = make(map[string]string)
		}
		m.step = stepPickPreset
		return m, nil

	case tea.KeyMsg:
		switch m.step {
		case stepWelcome:
			langs := []string{"en", "zh", "wen"}
			switch msg.String() {
			case "up":
				if m.langCursor > 0 {
					m.langCursor--
					i18n.SetLang(langs[m.langCursor])
				}
			case "down":
				if m.langCursor < len(langs)-1 {
					m.langCursor++
					i18n.SetLang(langs[m.langCursor])
				}
			case "enter":
				if !m.setupDone {
					return m, nil // blocked — still installing
				}
				lang := langs[m.langCursor]
				// Save language to global config
				cfg, _ := config.LoadConfig(m.globalDir)
				cfg.Language = lang
				config.SaveConfig(m.globalDir, cfg)
				// Opened from /settings — return to mail
				if m.welcomeOnly {
					return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
				}
				// Reload keys after potential config change
				m.existingKeys = cfg.Keys
				if m.existingKeys == nil {
					m.existingKeys = make(map[string]string)
				}
				// Bootstrap created presets — check if API key needed
				m.hasPresets = preset.HasAny()
				if !m.hasPresets {
					m.step = stepAPIKey
					m.setup = NewSetupModel(m.globalDir)
					return m, m.setup.Init()
				}
				m.step = stepPickPreset
				m.presets, _ = preset.List()
				return m, nil
			case "esc":
				if m.welcomeOnly {
					// Restore original language and return
					cfg, _ := config.LoadConfig(m.globalDir)
					if cfg.Language != "" {
						i18n.SetLang(cfg.Language)
					}
					return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
				}
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil

		case stepAPIKey:
			// Esc on provider selection goes back to welcome (not mail)
			if msg.String() == "esc" && m.setup.step == stepSelectProvider {
				m.step = stepWelcome
				return m, nil
			}
			var cmd tea.Cmd
			m.setup, cmd = m.setup.Update(msg)
			return m, cmd

		case stepPickPreset:
			switch msg.String() {
			case "up":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down":
				if m.cursor < len(m.presets)-1 {
					m.cursor++
				}
			case "enter":
				if m.cursor < len(m.presets) {
					p := m.presets[m.cursor]
					provider := m.getPresetProvider(p)
					m.selectedProvider = provider
					// Check if key is needed and missing
					if m.needsKey(provider) || provider == "custom" {
						m.step = stepPresetKey
						m.presetKeyInput.Reset()
						m.presetEndpointIn.Reset()
						m.presetModelIn.Reset()
						m.presetNameIn.Reset()
						m.presetKeyFieldIdx = 0
						if provider == "custom" {
							// field 0 = compat selector (no text focus)
							m.customCompat = 0
						} else if provider == "minimax" {
							// field 0 = region selector (no text focus)
							m.presetKeyInput.Blur()
						} else {
							m.presetKeyInput.Focus()
						}
						return m, textinput.Blink
					}
					// Key exists, proceed to name/dir
					m.enterAgentNameDir(p)
					return m, textinput.Blink
				}
			case "esc":
				m.step = stepWelcome
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil

		case stepPresetKey:
			isCustom := m.selectedProvider == "custom"
			isMinimax := m.selectedProvider == "minimax"
			fieldCount := 1 // default: key only
			if isCustom {
				fieldCount = 5 // compat + endpoint + model + key + name
			}
			if isMinimax {
				fieldCount = 2 // region + key
			}
			switch msg.String() {
			case "esc":
				m.step = stepPickPreset
				return m, nil
			case "up":
				if isCustom || isMinimax {
					m.presetKeyFieldIdx = (m.presetKeyFieldIdx - 1 + fieldCount) % fieldCount
					if isMinimax && m.presetKeyFieldIdx == 0 {
						m.presetKeyInput.Blur()
						return m, nil
					}
					return m, m.focusPresetKeyField()
				}
				return m, nil
			case "down", "tab":
				if isCustom || isMinimax {
					m.presetKeyFieldIdx = (m.presetKeyFieldIdx + 1) % fieldCount
					if isMinimax && m.presetKeyFieldIdx == 0 {
						m.presetKeyInput.Blur()
						return m, nil
					}
					return m, m.focusPresetKeyField()
				}
				return m, nil
			case "left", "right":
				// Toggle region for minimax
				if isMinimax && m.presetKeyFieldIdx == 0 {
					m.minimaxRegion = 1 - m.minimaxRegion
					return m, nil
				}
				// Toggle compat for custom
				if isCustom && m.presetKeyFieldIdx == 0 {
					m.customCompat = 1 - m.customCompat
					return m, nil
				}
			case "enter":
				key := m.presetKeyInput.Value()
				var newPresetName string
				if isCustom {
					endpoint := m.presetEndpointIn.Value()
					model := m.presetModelIn.Value()
					name := m.presetNameIn.Value()
					if endpoint == "" || model == "" || key == "" || name == "" {
						return m, nil // require all fields
					}
					compat := "openai"
					if m.customCompat == 1 {
						compat = "anthropic"
					}
					// Clone the template — don't mutate the original
					clone := preset.Clone(m.presets[m.cursor], name)
					if llm, ok := clone.Manifest["llm"].(map[string]interface{}); ok {
						llm["base_url"] = endpoint
						llm["model"] = model
						llm["api_compat"] = compat
					}
					if err := preset.Save(clone); err != nil {
						m.message = i18n.TF("firstrun.error", err)
						return m, nil
					}
					newPresetName = name
				}
				if isMinimax {
					// Clone the template with auto-name based on region
					p := m.presets[m.cursor]
					var name, baseURL string
					if m.minimaxRegion == 0 {
						name = "minimax_cn"
						baseURL = "https://api.minimaxi.com/anthropic"
					} else {
						name = "minimax_intl"
						baseURL = "https://api.minimax.io/anthropic"
					}
					clone := preset.Clone(p, name)
					if llm, ok := clone.Manifest["llm"].(map[string]interface{}); ok {
						llm["base_url"] = baseURL
					}
					if err := preset.Save(clone); err != nil {
						m.message = i18n.TF("firstrun.error", err)
						return m, nil
					}
					newPresetName = name
				}
				if key != "" {
					m.existingKeys[m.selectedProvider] = key
					cfg, _ := config.LoadConfig(m.globalDir)
					cfg.Keys = m.existingKeys
					config.SaveConfig(m.globalDir, cfg)
				} else if m.existingKeys[m.selectedProvider] == "" {
					return m, nil
				}
				// Reload presets and find the newly created one
				m.presets, _ = preset.List()
				if len(m.presets) == 0 {
					m.message = i18n.T("firstrun.no_presets")
					return m, nil
				}
				if newPresetName != "" {
					for i, p := range m.presets {
						if p.Name == newPresetName {
							m.cursor = i
							break
						}
					}
				}
				if m.cursor >= len(m.presets) {
					m.cursor = 0
				}
				p := m.presets[m.cursor]
				m.enterAgentNameDir(p)
				return m, textinput.Blink
			case "ctrl+c":
				return m, tea.Quit
			default:
				var cmd tea.Cmd
				if isCustom {
					switch m.presetKeyFieldIdx {
					case 0:
						// compat selector — no text input
					case 1:
						m.presetEndpointIn, cmd = m.presetEndpointIn.Update(msg)
					case 2:
						m.presetModelIn, cmd = m.presetModelIn.Update(msg)
					case 3:
						m.presetKeyInput, cmd = m.presetKeyInput.Update(msg)
					case 4:
						m.presetNameIn, cmd = m.presetNameIn.Update(msg)
					}
				} else if isMinimax && m.presetKeyFieldIdx == 1 {
					m.presetKeyInput, cmd = m.presetKeyInput.Update(msg)
				} else if !isMinimax {
					m.presetKeyInput, cmd = m.presetKeyInput.Update(msg)
				}
				return m, cmd
			}

		case stepAgentNameDir:
			langs := []string{"en", "zh", "wen"}
			switch msg.String() {
			case "tab", "down":
				m.fieldIdx = (m.fieldIdx + 1) % agentNameDirFieldCount
				return m, m.focusAgentField()
			case "up":
				m.fieldIdx = (m.fieldIdx - 1 + agentNameDirFieldCount) % agentNameDirFieldCount
				return m, m.focusAgentField()
			case "left":
				if m.fieldIdx == 2 { // language cycle
					m.agentLangIdx = (m.agentLangIdx - 1 + len(langs)) % len(langs)
				}
				return m, nil
			case "right":
				if m.fieldIdx == 2 { // language cycle
					m.agentLangIdx = (m.agentLangIdx + 1) % len(langs)
				}
				return m, nil
			case "enter":
				name := m.nameInput.Value()
				if name == "" {
					name = m.presets[m.cursor].Name
				}
				dirName := m.dirInput.Value()
				if dirName == "" {
					dirName = name
				}
				m.agentName = name
				m.agentDir = dirName
				// Check collision
				orchDir := filepath.Join(m.baseDir, dirName)
				if _, err := os.Stat(orchDir); err == nil {
					m.message = i18n.TF("firstrun.dir_exists", dirName)
					return m, nil
				}
				// Parse numeric fields
				stamina, err := strconv.ParseFloat(m.staminaInput.Value(), 64)
				if err != nil || stamina <= 0 {
					stamina = 36000
				}
				ctxLimit, err := strconv.Atoi(m.ctxLimitInput.Value())
				if err != nil || ctxLimit <= 0 {
					ctxLimit = 200000
				}
				soulDelay, err := strconv.ParseFloat(m.soulDelayInput.Value(), 64)
				if err != nil || soulDelay <= 0 {
					soulDelay = 120
				}
				moltPress, err := strconv.ParseFloat(m.moltPressInput.Value(), 64)
				if err != nil || moltPress <= 0 || moltPress > 1 {
					moltPress = 0.8
				}
				// Generate init.json and launch
				p := m.presets[m.cursor]
				opts := preset.AgentOpts{
					Language:     langs[m.agentLangIdx],
					Stamina:      stamina,
					ContextLimit: ctxLimit,
					SoulDelay:    soulDelay,
					MoltPressure: moltPress,
				}
				if err := preset.GenerateInitJSONWithOpts(p, m.agentName, dirName, m.baseDir, m.globalDir, opts); err != nil {
					m.message = i18n.TF("firstrun.error", err)
					return m, nil
				}
				m.step = stepLaunching
				m.message = i18n.TF("firstrun.created", m.agentName)
				return m, func() tea.Msg {
					return FirstRunDoneMsg{OrchDir: orchDir, OrchName: m.agentName}
				}
			case "esc":
				m.step = stepPickPreset
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			default:
				var cmd tea.Cmd
				switch m.fieldIdx {
				case 0:
					m.nameInput, cmd = m.nameInput.Update(msg)
				case 1:
					m.dirInput, cmd = m.dirInput.Update(msg)
				case 3:
					m.staminaInput, cmd = m.staminaInput.Update(msg)
				case 4:
					m.ctxLimitInput, cmd = m.ctxLimitInput.Update(msg)
				case 5:
					m.soulDelayInput, cmd = m.soulDelayInput.Update(msg)
				case 6:
					m.moltPressInput, cmd = m.moltPressInput.Update(msg)
				}
				return m, cmd
			}
		}
	}
	return m, nil
}

func (m FirstRunModel) View() string {
	var b strings.Builder

	switch m.step {
	case stepWelcome:
		return m.viewWelcome()
	default:
		// non-welcome steps: show standard title bar
	}

	// Title
	title := StyleTitle.Render("  " + i18n.T("firstrun.welcome"))
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	switch m.step {
	case stepAPIKey:
		stepNum, total := stepProgress(m.step, m.hasPresets)
		b.WriteString("\n  " + StyleSubtle.Render(fmt.Sprintf("Step %d/%d", stepNum, total)) + "\n\n")
		b.WriteString("  " + i18n.T("firstrun.no_presets") + "\n\n")
		b.WriteString(m.setup.View())

	case stepPickPreset:
		stepNum, total := stepProgress(m.step, m.hasPresets)
		b.WriteString("\n  " + StyleSubtle.Render(fmt.Sprintf("Step %d/%d: "+i18n.T("firstrun.pick_preset"), stepNum, total)) + "\n\n")
		for i, p := range m.presets {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}
			// i18n: try preset.name_<id> and preset.desc_<id>, fall back to raw fields
			displayName := i18n.T("preset.name_" + p.Name)
			if displayName == "preset.name_"+p.Name {
				displayName = p.Name
			}
			displayDesc := i18n.T("preset.desc_" + p.Name)
			if displayDesc == "preset.desc_"+p.Name {
				displayDesc = p.Description
			}
			name := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent).Render(displayName)
			desc := StyleSubtle.Render("  " + displayDesc)
			b.WriteString(cursor + name + desc + "\n")
		}
		b.WriteString("\n" + StyleFaint.Render("  "+i18n.T("firstrun.select_hint")) + "\n")
		b.WriteString(StyleFaint.Render("  [Ctrl+C] "+i18n.T("common.quit")) + "\n")

	case stepPresetKey:
		providerName := i18n.T("setup.provider_" + m.selectedProvider)
		if providerName == "setup.provider_"+m.selectedProvider {
			providerName = m.selectedProvider
		}
		b.WriteString("  " + i18n.TF("firstrun.enter_provider_key", providerName) + "\n\n")
		if m.selectedProvider == "custom" {
			warnStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
			b.WriteString("  " + warnStyle.Render(i18n.T("firstrun.custom_cost_warn")) + "\n\n")
			// Compat selector
			openaiLabel := "OpenAI"
			anthropicLabel := "Anthropic"
			if m.customCompat == 0 {
				openaiLabel = "● " + openaiLabel
				anthropicLabel = "○ " + anthropicLabel
			} else {
				openaiLabel = "○ " + openaiLabel
				anthropicLabel = "● " + anthropicLabel
			}
			compatStyle := lipgloss.NewStyle()
			if m.presetKeyFieldIdx == 0 {
				compatStyle = compatStyle.Bold(true).Foreground(ColorAccent)
			}
			b.WriteString("  " + i18n.T("firstrun.api_compat") + ":  " + compatStyle.Render(openaiLabel+"  "+anthropicLabel) + "\n")
			b.WriteString("  " + i18n.T("presets.endpoint") + ":    " + m.presetEndpointIn.View() + "\n")
			b.WriteString("  " + i18n.T("presets.model") + ":       " + m.presetModelIn.View() + "\n")
			b.WriteString("  " + i18n.T("setup.api_key_label") + "     " + m.presetKeyInput.View() + "\n")
			b.WriteString("  " + i18n.T("presets.enter_name") + " " + m.presetNameIn.View() + "\n\n")
			b.WriteString(StyleFaint.Render("  [↑↓] "+i18n.T("firstrun.toggle_field")+
				"  [←→] "+i18n.T("firstrun.toggle_region")+
				"  [Enter] "+i18n.T("setup.save")+
				"  [Esc] "+i18n.T("setup.back")) + "\n")
		} else if m.selectedProvider == "minimax" {
			// Region toggle
			intlLabel := i18n.T("firstrun.region_intl")
			chinaLabel := i18n.T("firstrun.region_china")
			if m.minimaxRegion == 0 {
				chinaLabel = "● " + chinaLabel
				intlLabel = "○ " + intlLabel
			} else {
				chinaLabel = "○ " + chinaLabel
				intlLabel = "● " + intlLabel
			}
			regionStyle := lipgloss.NewStyle()
			if m.presetKeyFieldIdx == 0 {
				regionStyle = regionStyle.Bold(true).Foreground(ColorAccent)
			}
			b.WriteString("  " + i18n.T("firstrun.region") + ":  " + regionStyle.Render(chinaLabel+"  "+intlLabel) + "\n")
			endpointURL := "api.minimaxi.com/anthropic"
			if m.minimaxRegion == 1 {
				endpointURL = "api.minimax.io/anthropic"
			}
			b.WriteString("            " + StyleFaint.Render(endpointURL) + "\n")
			b.WriteString("  " + i18n.T("setup.api_key_label") + "  " + m.presetKeyInput.View() + "\n\n")
			b.WriteString(StyleFaint.Render("  [↑↓] "+i18n.T("firstrun.toggle_field")+
				"  [←→] "+i18n.T("firstrun.toggle_region")+
				"  [Enter] "+i18n.T("setup.save")+
				"  [Esc] "+i18n.T("setup.back")) + "\n")
		} else {
			b.WriteString("  " + i18n.T("setup.api_key_label") + " " + m.presetKeyInput.View() + "\n\n")
			b.WriteString(StyleFaint.Render("  [Enter] "+i18n.T("setup.save")+
				"  [Esc] "+i18n.T("setup.back")) + "\n")
		}

	case stepAgentNameDir:
		stepNum, total := stepProgress(m.step, m.hasPresets)
		b.WriteString("\n  " + StyleSubtle.Render(fmt.Sprintf("Step %d/%d: "+i18n.T("firstrun.enter_name_dir"), stepNum, total)) + "\n\n")

		langs := []string{"en", "zh", "wen"}

		// Helper: cursor prefix for field index
		cur := func(idx int) string {
			if idx == m.fieldIdx {
				return "> "
			}
			return "  "
		}

		// 0: Name
		b.WriteString(cur(0) + i18n.T("firstrun.agent_name") + ": " + m.nameInput.View() + "\n")

		// 1: Dir
		b.WriteString(cur(1) + i18n.T("firstrun.agent_dir") + ": " + m.dirInput.View() + "\n")

		// 2: Language (cycle selector)
		langVal := langs[m.agentLangIdx]
		if m.fieldIdx == 2 {
			langVal = lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render("< " + langVal + " >")
		}
		b.WriteString(cur(2) + i18n.T("firstrun.language") + ": " + langVal + "\n")

		// 3-6: Numeric text inputs with hints
		type numField struct {
			idx   int
			label string
			hint  string
			view  string
		}
		numFields := []numField{
			{3, i18n.T("firstrun.stamina"), i18n.T("firstrun.stamina_hint"), m.staminaInput.View()},
			{4, i18n.T("firstrun.context_limit"), i18n.T("firstrun.context_limit_hint"), m.ctxLimitInput.View()},
			{5, i18n.T("firstrun.soul_delay"), i18n.T("firstrun.soul_delay_hint"), m.soulDelayInput.View()},
			{6, i18n.T("firstrun.molt_pressure"), i18n.T("firstrun.molt_pressure_hint"), m.moltPressInput.View()},
		}
		for _, nf := range numFields {
			hint := StyleFaint.Render(" (" + nf.hint + ")")
			b.WriteString(cur(nf.idx) + nf.label + ": " + nf.view + hint + "\n")
		}

		if m.message != "" {
			errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
			b.WriteString("\n  " + errStyle.Render(m.message) + "\n")
		}
		b.WriteString("\n" + StyleFaint.Render("  ↑↓ "+i18n.T("firstrun.toggle_field")+
			"  [Enter] "+i18n.T("firstrun.create_agent")+
			"  [Esc] "+i18n.T("firstrun.back")) + "\n")

	case stepLaunching:
		stepNum, total := stepProgress(m.step, m.hasPresets)
		b.WriteString("\n  " + StyleSubtle.Render(fmt.Sprintf("Step %d/%d: ", stepNum, total)) + i18n.T("firstrun.launching") + "\n\n")
		if m.message != "" {
			b.WriteString("  " + m.message + "\n")
		}
	}

	return b.String()
}

// viewWelcome renders the welcome/language selection page.
func (m FirstRunModel) viewWelcome() string {
	langLabels := []string{"English", "现代汉语", "文言"}

	// Build content lines (without vertical centering first)
	var content strings.Builder

	// Braille logo (𢘐 — U+22610)
	logoLines := []string{
		"⠀⠀⠀⠀⠀⠀⣄⡀⠀⠀⠀⠀⠀⠀⠀⠀⢠⣤⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⠀⠀⠀⣿⡟⠁⠀⠀⠀⠀⠀⠀⢀⣾⡿⢯⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⠀⠀⠀⣿⡇⢠⡀⠀⠀⠀⠀⢀⣾⠟⠁⠈⢻⣦⡀⠀⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⢰⡇⠀⣿⡇⠀⢻⣦⡀⠀⣠⡿⠋⠀⠀⠀⠀⠙⢿⣦⣀⠀⠀⠀⠀⠀",
		"⠀⠀⣠⣿⠇⠀⣿⡇⠀⠈⠟⣣⡾⠋⠀⠀⠀⠀⠀⠀⠀⠀⠙⠿⣿⣶⣤⡄⠀",
		"⠀⠸⠿⠟⠀⠀⣿⡇⠀⠴⠛⠁⣀⣀⣀⣀⣀⣀⣀⣀⣀⣤⣶⣦⣌⠉⠀⠀⠀",
		"⠀⠀⠀⠀⠀⠀⣿⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⠀⠀⠀⣿⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⠀⠀⠀⣿⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⠀⠀⠀⣿⡇⠀⣀⣀⣀⣀⣀⣀⣀⣀⣿⣿⣀⣀⣀⣀⣀⣠⣦⣄⠀⠀",
		"⠀⠀⠀⠀⠀⠀⠟⠃⠀⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠁⠀",
	}
	logoStyle := lipgloss.NewStyle().Foreground(ColorAgent)
	for _, line := range logoLines {
		content.WriteString(centerText(logoStyle.Render(line), m.width) + "\n")
	}
	content.WriteString("\n")

	// Product name
	titleText := i18n.T("welcome.title")
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent)
	content.WriteString(centerText(titleStyle.Render(titleText), m.width) + "\n\n")

	// Poem (two lines)
	poemStyle := StyleSubtle
	content.WriteString(centerText(poemStyle.Render(i18n.T("welcome.poem_line1")), m.width) + "\n")
	content.WriteString(centerText(poemStyle.Render(i18n.T("welcome.poem_line2")), m.width) + "\n\n\n")

	// Language selector
	for i, label := range langLabels {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if i == m.langCursor {
			cursor = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		}
		line := cursor + style.Render(label)
		content.WriteString(centerText(line, m.width) + "\n")
	}

	// Bootstrap status
	if !m.welcomeOnly {
		content.WriteString("\n")
		if m.setupErr != "" {
			errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
			content.WriteString(centerText(errStyle.Render(i18n.TF("welcome.setup_failed", m.setupErr)), m.width) + "\n")
		} else if m.setupDone {
			doneStyle := lipgloss.NewStyle().Foreground(ColorAgent)
			content.WriteString(centerText(doneStyle.Render(i18n.T("welcome.ready")), m.width) + "\n")
		} else if m.setupStatus != "" {
			content.WriteString(centerText(StyleFaint.Render(i18n.T(m.setupStatus)), m.width) + "\n")
		} else {
			content.WriteString(centerText(StyleFaint.Render(i18n.T("welcome.installing")), m.width) + "\n")
		}
	}

	// Footer hints
	content.WriteString("\n")
	var hints string
	if m.setupDone || m.welcomeOnly {
		hints = StyleFaint.Render("↑↓ " + i18n.T("welcome.select_lang") + "  [Enter] " + i18n.T("welcome.confirm"))
	} else {
		hints = StyleFaint.Render("↑↓ " + i18n.T("welcome.select_lang") + "  (" + i18n.T("welcome.installing") + ")")
	}
	content.WriteString(centerText(hints, m.width) + "\n")

	// Vertical centering: pad top to center the content block
	contentStr := content.String()
	contentLines := strings.Count(contentStr, "\n")
	topPad := (m.height - contentLines) / 2
	if topPad < 1 {
		topPad = 1
	}

	return strings.Repeat("\n", topPad) + contentStr
}

// centerText centers a string within the given width.
func centerText(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	pad := (width - w) / 2
	return strings.Repeat(" ", pad) + s
}

// agentNameDirFieldCount is the number of fields in stepAgentNameDir.
const agentNameDirFieldCount = 7 // name, dir, lang, stamina, ctx_limit, soul_delay, molt_pressure

// enterAgentNameDir initialises all fields and transitions to stepAgentNameDir.
func (m *FirstRunModel) enterAgentNameDir(p preset.Preset) {
	defaultName := p.Name
	m.agentName = defaultName
	m.agentDir = defaultName
	m.nameInput.SetValue(defaultName)
	m.dirInput.SetValue(defaultName)
	m.fieldIdx = 0
	m.focusOnDir = false
	m.nameInput.Focus()
	m.dirInput.Blur()

	// Language — inherit from preset, fallback "en"
	m.agentLangIdx = 0
	if l, ok := p.Manifest["language"].(string); ok {
		for i, lang := range []string{"en", "zh", "wen"} {
			if lang == l {
				m.agentLangIdx = i
				break
			}
		}
	}

	// Numeric defaults
	m.staminaInput.SetValue("36000")
	m.ctxLimitInput.SetValue("200000")
	m.soulDelayInput.SetValue("120")
	m.moltPressInput.SetValue("0.8")
	m.staminaInput.Blur()
	m.ctxLimitInput.Blur()
	m.soulDelayInput.Blur()
	m.moltPressInput.Blur()

	m.step = stepAgentNameDir
}

// focusAgentField focuses the input at m.fieldIdx and blurs all others.
// Returns the blink command for the newly focused input.
func (m *FirstRunModel) focusPresetKeyField() tea.Cmd {
	m.presetEndpointIn.Blur()
	m.presetModelIn.Blur()
	m.presetKeyInput.Blur()
	m.presetNameIn.Blur()
	if m.selectedProvider == "minimax" {
		switch m.presetKeyFieldIdx {
		case 0:
			return nil // region selector — no text focus
		case 1:
			return m.presetKeyInput.Focus()
		}
		return nil
	}
	switch m.presetKeyFieldIdx {
	case 0:
		return nil // compat selector — no text focus
	case 1:
		return m.presetEndpointIn.Focus()
	case 2:
		return m.presetModelIn.Focus()
	case 3:
		return m.presetKeyInput.Focus()
	case 4:
		return m.presetNameIn.Focus()
	}
	return nil
}

func (m *FirstRunModel) focusAgentField() tea.Cmd {
	m.nameInput.Blur()
	m.dirInput.Blur()
	m.staminaInput.Blur()
	m.ctxLimitInput.Blur()
	m.soulDelayInput.Blur()
	m.moltPressInput.Blur()
	m.focusOnDir = false

	switch m.fieldIdx {
	case 0:
		return m.nameInput.Focus()
	case 1:
		m.focusOnDir = true
		return m.dirInput.Focus()
	case 2:
		return nil // language — cycle selector, no text input
	case 3:
		return m.staminaInput.Focus()
	case 4:
		return m.ctxLimitInput.Focus()
	case 5:
		return m.soulDelayInput.Focus()
	case 6:
		return m.moltPressInput.Focus()
	}
	return nil
}

// getPresetProvider extracts provider name from a preset
func (m FirstRunModel) getPresetProvider(p preset.Preset) string {
	if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
		if provider, ok := llm["provider"].(string); ok {
			return provider
		}
	}
	return "minimax" // default
}

// needsKey returns true if the provider's key is not configured
func (m FirstRunModel) needsKey(provider string) bool {
	_, hasKey := m.existingKeys[provider]
	return !hasKey
}

// presetNeedsKey returns true if the preset's provider key is missing (for warning display)
func (m FirstRunModel) presetNeedsKey(p preset.Preset) bool {
	provider := m.getPresetProvider(p)
	return m.needsKey(provider)
}
