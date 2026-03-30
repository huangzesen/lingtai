package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/preset"
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

// capCheckDoneMsg delivers the parsed check-caps result.
type capCheckDoneMsg struct {
	infos map[string]capInfo
}

// capCheckErrMsg signals that check-caps failed.
type capCheckErrMsg struct{ err string }

// bootstrapProgressMsg reports a setup progress step (i18n key).
type bootstrapProgressMsg struct{ key string }

type firstRunStep int

const (
	stepWelcome firstRunStep = iota
	stepAPIKey
	stepPickPreset
	stepPresetKey
	stepCapabilities
	stepTutorial
	stepAgentNameDir
	stepLaunching
)

// capInfo holds provider metadata for a single capability (from check-caps).
type capInfo struct {
	Providers []string `json:"providers"`
	Default   *string  `json:"default"`
}

// stepProgress returns the 1-based index and total for progress display
func stepProgress(step firstRunStep, hasPresets bool) (current int, total int) {
	if hasPresets {
		total = 4
	} else {
		total = 5
	}
	switch {
	case !hasPresets && step == stepAPIKey:
		return 1, total
	case !hasPresets && step == stepPickPreset:
		return 2, total
	case step == stepPickPreset || step == stepPresetKey:
		return 1, total
	case step == stepCapabilities:
		if hasPresets {
			return 2, total
		}
		return 3, total
	case step == stepAgentNameDir:
		if hasPresets {
			return 3, total
		}
		return 4, total
	case step == stepLaunching:
		return total, total
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
	fieldIdx   int // see agentNameDirFieldCount for field indices
	// Agent config text inputs
	agentLangIdx   int // cycle: 0=en, 1=zh, 2=wen
	staminaInput   textinput.Model
	ctxLimitInput  textinput.Model
	soulDelayInput textinput.Model
	moltPressInput textinput.Model
	// Authority toggles
	karmaIdx   int // 0=true, 1=false
	nirvanaIdx int // 0=false, 1=true
	// Prompt path inputs
	covenantInput  textinput.Model
	principleInput textinput.Model
	soulFlowInput  textinput.Model
	commentInput   textinput.Model
	// Track whether user manually edited prompt paths (dirty = don't auto-update on lang change)
	covenantDirty  bool
	principleDirty bool
	soulFlowDirty  bool
	// Welcome page language selector
	langCursor  int
	welcomeOnly bool // true when opened from /settings (return to mail after language pick)
	// Bootstrap state (venv + assets install)
	setupDone    bool        // true when bootstrap goroutine finishes
	setupErr     string      // non-empty if bootstrap failed
	setupStatus  string      // current progress i18n key (active step)
	setupSteps   []string    // completed step i18n keys (shown with checkmarks)
	progressCh   chan string // channel for progress updates
	// Embedded key input for preset's provider
	presetKeyInput    textinput.Model
	presetEndpointIn  textinput.Model   // base_url for custom provider
	presetModelIn     textinput.Model   // model name for custom provider
	presetNameIn      textinput.Model   // preset name for custom provider (separate from nameInput)
	presetKeyFieldIdx int               // 0=compat, 1=endpoint, 2=model, 3=key, 4=name (custom); 0=region,1=key (minimax)
	minimaxRegion     int               // 0=international, 1=china
	customCompat      int               // 0=openai, 1=anthropic
	selectedProvider  string            // provider of currently selected preset
	existingKeys      map[string]string // loaded from Config.Keys
	// Capability selection state (stepCapabilities)
	capInfos    map[string]capInfo // from check-caps CLI output
	capSelected map[string]bool    // user toggle state
	capOrder    []string           // ordered list matching AllCapabilities
	capCursor   int                // current cursor position (0..len-1)
	capLoading  bool               // true while check-caps is running
	capErr      string             // error message if check-caps fails
	// Tutorial step state
	tutorialCursor int // 0=start/resume, 1=fresh (if exists), last=skip
}

func NewFirstRunModel(baseDir, globalDir string, hasPresets bool) FirstRunModel {
	ti := textinput.New()
	ti.CharLimit = 64
	ti.SetWidth(40)

	di := textinput.New()
	di.CharLimit = 64
	di.SetWidth(40)

	pki := textinput.New()
	pki.CharLimit = 128
	pki.SetWidth(50)

	pei := textinput.New() // endpoint input for custom provider
	pei.CharLimit = 256
	pei.SetWidth(50)
	pei.Placeholder = "https://openrouter.ai/api/v1"

	pmi := textinput.New() // model input for custom provider
	pmi.CharLimit = 64
	pmi.SetWidth(50)
	pmi.Placeholder = "model-name"

	pni := textinput.New() // preset name input for custom provider
	pni.CharLimit = 64
	pni.SetWidth(50)
	pni.Placeholder = "openrouter"

	si := textinput.New()
	si.CharLimit = 10
	si.SetWidth(15)
	si.Prompt = ""

	ci := textinput.New()
	ci.CharLimit = 10
	ci.SetWidth(15)
	ci.Prompt = ""

	sdi := textinput.New()
	sdi.CharLimit = 10
	sdi.SetWidth(15)
	sdi.Prompt = ""

	mpi := textinput.New()
	mpi.CharLimit = 6
	mpi.SetWidth(15)
	mpi.Prompt = ""

	covi := textinput.New()
	covi.CharLimit = 256
	covi.SetWidth(50)
	covi.Prompt = ""

	prini := textinput.New()
	prini.CharLimit = 256
	prini.SetWidth(50)
	prini.Prompt = ""

	sfli := textinput.New()
	sfli.CharLimit = 256
	sfli.SetWidth(50)
	sfli.Prompt = ""

	comi := textinput.New()
	comi.CharLimit = 256
	comi.SetWidth(50)
	comi.Prompt = ""

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
		step:             stepWelcome,
		baseDir:          baseDir,
		globalDir:        globalDir,
		nameInput:        ti,
		dirInput:         di,
		hasPresets:       hasPresets,
		langCursor:       langCursor,
		presetKeyInput:   pki,
		presetEndpointIn: pei,
		presetModelIn:    pmi,
		presetNameIn:     pni,
		existingKeys:     existingKeys,
		staminaInput:     si,
		ctxLimitInput:    ci,
		soulDelayInput:   sdi,
		moltPressInput:   mpi,
		covenantInput:    covi,
		principleInput:   prini,
		soulFlowInput:    sfli,
		commentInput:     comi,
		nirvanaIdx:       1, // default false (1=false)
		progressCh:       make(chan string, 4),
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
		// Resize text inputs to use available terminal width
		inputWidth := msg.Width - 20
		if inputWidth < 40 {
			inputWidth = 40
		}
		m.nameInput.SetWidth(inputWidth)
		m.dirInput.SetWidth(inputWidth)
		m.covenantInput.SetWidth(inputWidth)
		m.principleInput.SetWidth(inputWidth)
		m.soulFlowInput.SetWidth(inputWidth)
		m.commentInput.SetWidth(inputWidth)
		return m, nil

	case bootstrapProgressMsg:
		// Move current step to completed list, set new step as active
		if m.setupStatus != "" {
			m.setupSteps = append(m.setupSteps, m.setupStatus)
		}
		m.setupStatus = msg.key
		return m, waitForProgress(m.progressCh)

	case bootstrapDoneMsg:
		// Move final step to completed list
		if m.setupStatus != "" {
			m.setupSteps = append(m.setupSteps, m.setupStatus)
		}
		m.setupDone = true
		m.setupStatus = ""
		return m, nil

	case bootstrapErrMsg:
		m.setupDone = true
		m.setupErr = msg.err
		return m, nil

	case capCheckDoneMsg:
		m.capLoading = false
		m.capInfos = msg.infos
		p := m.presets[m.cursor]
		provider := m.getPresetProvider(p)
		presetCaps := make(map[string]bool)
		if capsMap, ok := p.Manifest["capabilities"].(map[string]interface{}); ok {
			for k := range capsMap {
				presetCaps[k] = true
			}
		}
		// Also treat "file" group as present if any of read/write/edit/glob/grep are
		if presetCaps["read"] || presetCaps["write"] || presetCaps["edit"] || presetCaps["glob"] || presetCaps["grep"] {
			presetCaps["file"] = true
		}
		for _, name := range m.capOrder {
			info, ok := m.capInfos[name]
			if !ok {
				continue
			}
			compat := m.isCapCompatible(info, provider)
			if (compat || m.isCapLocal(info)) && presetCaps[name] {
				m.capSelected[name] = true
			}
		}
		return m, nil

	case capCheckErrMsg:
		m.capLoading = false
		m.capErr = msg.err
		// Populate capInfos with empty entries so Space toggle works
		m.capInfos = make(map[string]capInfo)
		for _, name := range m.capOrder {
			m.capInfos[name] = capInfo{}
		}
		// Fallback: select all capabilities from the preset
		p := m.presets[m.cursor]
		if capsMap, ok := p.Manifest["capabilities"].(map[string]interface{}); ok {
			for k := range capsMap {
				m.capSelected[k] = true
			}
		}
		// Synthesize "file" group
		if m.capSelected["read"] || m.capSelected["write"] || m.capSelected["edit"] || m.capSelected["glob"] || m.capSelected["grep"] {
			m.capSelected["file"] = true
		}
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

	case tea.KeyPressMsg:
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
							m.presetEndpointIn.SetValue("https://openrouter.ai/api/v1")
							m.presetNameIn.SetValue("openrouter")
						} else if provider == "minimax" {
							// field 0 = region selector (no text focus)
							m.presetKeyInput.Blur()
						} else {
							m.presetKeyInput.Focus()
						}
						return m, textinput.Blink
					}
					// Key exists, proceed to capabilities
					return m, m.enterCapabilities()
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
				return m, m.enterCapabilities()
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

		case stepCapabilities:
			if m.capLoading {
				return m, nil
			}
			colSize := (len(m.capOrder) + 1) / 2
			switch msg.String() {
			case "up":
				if m.capCursor >= colSize {
					// Right column
					if m.capCursor > colSize {
						m.capCursor--
					}
				} else {
					// Left column
					if m.capCursor > 0 {
						m.capCursor--
					}
				}
			case "down":
				if m.capCursor >= colSize {
					// Right column
					if m.capCursor < len(m.capOrder)-1 {
						m.capCursor++
					}
				} else {
					// Left column
					if m.capCursor < colSize-1 {
						m.capCursor++
					}
				}
			case "left":
				if m.capCursor >= colSize {
					m.capCursor -= colSize
				}
			case "right":
				if m.capCursor < colSize && m.capCursor+colSize < len(m.capOrder) {
					m.capCursor += colSize
				}
			case "space":
				name := m.capOrder[m.capCursor]
				info, ok := m.capInfos[name]
				if !ok {
					return m, nil
				}
				provider := m.getPresetProvider(m.presets[m.cursor])
				if m.isCapCompatible(info, provider) || m.isCapLocal(info) {
					m.capSelected[name] = !m.capSelected[name]
				}
			case "enter":
				m.applyCapSelections()
				if config.TutorialDone(m.globalDir) {
					// Already did tutorial — skip straight to agent creation
					p := m.presets[m.cursor]
					m.enterAgentNameDir(p)
					m.step = stepAgentNameDir
					return m, textinput.Blink
				}
				m.step = stepTutorial
				m.tutorialCursor = 0
				return m, nil
			case "esc":
				m.step = stepPickPreset
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil

		case stepTutorial:
			tutorialDir := filepath.Join(m.baseDir, "tutorial")
			switch msg.String() {
			case "up":
				if m.tutorialCursor > 0 {
					m.tutorialCursor--
				}
			case "down":
				if m.tutorialCursor < 1 {
					m.tutorialCursor++
				}
			case "enter":
				config.MarkTutorialDone(m.globalDir)
				switch m.tutorialCursor {
				case 0: // Start Tutorial
					fs.SuspendAndWait(tutorialDir, 3*time.Second)
					os.RemoveAll(tutorialDir)
					p := m.presets[m.cursor]
					langs := []string{"en", "zh", "wen"}
					lang := langs[m.langCursor]
					if err := preset.GenerateTutorialInit(p, m.baseDir, m.globalDir, lang); err != nil {
						m.message = i18n.TF("firstrun.error", err)
						return m, nil
					}
					humanAddr, _ := filepath.Abs(filepath.Join(m.baseDir, "human"))
					return m, func() tea.Msg {
						fs.WritePrompt(tutorialDir, "You have just been created as the tutorial guide. A new user is waiting. Send them a welcome email to introduce yourself and begin Lesson 1. The human's email address is: "+humanAddr)
						return FirstRunDoneMsg{OrchDir: tutorialDir, OrchName: "guide"}
					}
				case 1: // Skip Tutorial
					p := m.presets[m.cursor]
					m.enterAgentNameDir(p)
					m.step = stepAgentNameDir
					return m, textinput.Blink
				}
			case "esc":
				m.step = stepCapabilities
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil

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
				switch m.fieldIdx {
				case 2: // language cycle
					m.agentLangIdx = (m.agentLangIdx - 1 + len(langs)) % len(langs)
					m.updatePromptPaths()
				case 7: // karma
					m.karmaIdx = (m.karmaIdx + 1) % 2
				case 8: // nirvana
					m.nirvanaIdx = (m.nirvanaIdx + 1) % 2
				}
				return m, nil
			case "right":
				switch m.fieldIdx {
				case 2: // language cycle
					m.agentLangIdx = (m.agentLangIdx + 1) % len(langs)
					m.updatePromptPaths()
				case 7: // karma
					m.karmaIdx = (m.karmaIdx + 1) % 2
				case 8: // nirvana
					m.nirvanaIdx = (m.nirvanaIdx + 1) % 2
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
				orchDir := filepath.Join(m.baseDir, dirName)
				if _, err := os.Stat(orchDir); err == nil {
					m.message = i18n.TF("firstrun.dir_exists", dirName)
					return m, nil
				}
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
				p := m.presets[m.cursor]
				opts := preset.AgentOpts{
					Language:      langs[m.agentLangIdx],
					Stamina:       stamina,
					ContextLimit:  ctxLimit,
					SoulDelay:     soulDelay,
					MoltPressure:  moltPress,
					Karma:         m.karmaIdx == 0,
					Nirvana:       m.nirvanaIdx == 0,
					CovenantFile:  m.covenantInput.Value(),
					PrincipleFile: m.principleInput.Value(),
					SoulFile:      m.soulFlowInput.Value(),
					CommentFile:   m.commentInput.Value(),
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
				m.step = stepTutorial
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
				case 9:
					m.covenantInput, cmd = m.covenantInput.Update(msg)
					m.covenantDirty = true
				case 10:
					m.principleInput, cmd = m.principleInput.Update(msg)
					m.principleDirty = true
				case 11:
					m.soulFlowInput, cmd = m.soulFlowInput.Update(msg)
					m.soulFlowDirty = true
				case 12:
					m.commentInput, cmd = m.commentInput.Update(msg)
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
		savedCount := preset.SavedCount(m.presets)
		for i, p := range m.presets {
			// Section headers between saved and template presets
			if savedCount > 0 && i == 0 {
				b.WriteString("  " + StyleFaint.Render(i18n.T("preset.saved")) + "\n")
			}
			if i == savedCount {
				if savedCount > 0 {
					b.WriteString("\n")
				}
				b.WriteString("  " + StyleFaint.Render(i18n.T("preset.templates")) + "\n")
			}
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

	case stepCapabilities:
		stepNum, total := stepProgress(m.step, m.hasPresets)
		b.WriteString("\n  " + StyleSubtle.Render(fmt.Sprintf("Step %d/%d: ", stepNum, total)+i18n.T("firstrun.select_caps")) + "\n\n")

		if m.capLoading {
			b.WriteString("  " + StyleSubtle.Render(i18n.T("firstrun.checking_caps")) + "\n")
			return b.String()
		}

		if m.capErr != "" {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(ColorSuspended).Render(m.capErr) + "\n\n")
		}

		provider := m.getPresetProvider(m.presets[m.cursor])
		colSize := (len(m.capOrder) + 1) / 2
		dimStyle := lipgloss.NewStyle().Foreground(ColorSubtle)
		cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)

		for row := 0; row < colSize; row++ {
			var line string
			for col := 0; col < 2; col++ {
				idx := row + col*colSize
				if idx >= len(m.capOrder) {
					break
				}
				name := m.capOrder[idx]
				info := m.capInfos[name]
				compat := m.isCapCompatible(info, provider)
				local := m.isCapLocal(info)

				var checkbox, hint string
				isCurrent := idx == m.capCursor

				if compat || local {
					if m.capSelected[name] {
						checkbox = "[x]"
					} else {
						checkbox = "[ ]"
					}
					if !compat && local {
						hint = "(local)"
					}
				} else {
					checkbox = "[-]"
					hint = strings.Join(info.Providers, ", ")
				}

				prefix := "  "
				if isCurrent {
					prefix = "> "
				}

				cell := prefix + checkbox + " " + name
				if hint != "" {
					cell += "  " + hint
				}

				if !compat && !local {
					cell = dimStyle.Render(cell)
				} else if isCurrent {
					cell = cursorStyle.Render(cell)
				}

				cellWidth := 38
				visWidth := lipgloss.Width(cell)
				if visWidth < cellWidth {
					cell += strings.Repeat(" ", cellWidth-visWidth)
				}
				line += cell
			}
			b.WriteString(line + "\n")
		}

		b.WriteString("\n" + StyleFaint.Render("  ↑↓←→ "+i18n.T("settings.select")+
			"  space "+i18n.T("settings.change")+
			"  [Enter] "+i18n.T("firstrun.confirm_caps")+
			"  [Esc] "+i18n.T("firstrun.back")) + "\n")

	case stepTutorial:
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent)
		b.WriteString("\n  " + titleStyle.Render(i18n.T("firstrun.tutorial_title")) + "\n\n")

		b.WriteString("  " + i18n.T("firstrun.tutorial_desc") + "\n")
		b.WriteString("  " + i18n.T("firstrun.tutorial_patience") + "\n")
		b.WriteString("  " + i18n.T("firstrun.tutorial_status_hint") + "\n")
		b.WriteString("\n")

		type tutorialOpt struct {
			label string
		}
		opts := []tutorialOpt{
			{i18n.T("firstrun.tutorial_start")},
			{i18n.T("firstrun.tutorial_skip")},
		}

		for i, opt := range opts {
			cursor := "  "
			style := lipgloss.NewStyle().Foreground(ColorText)
			if i == m.tutorialCursor {
				cursor = "> "
				style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
			}
			b.WriteString(cursor + style.Render(opt.label) + "\n")
		}

		if m.message != "" {
			errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
			b.WriteString("\n  " + errStyle.Render(m.message) + "\n")
		}
		b.WriteString("\n" + StyleFaint.Render("  ↑↓ "+i18n.T("welcome.select_lang")+
			"  [Enter] "+i18n.T("welcome.confirm")+
			"  [Esc] "+i18n.T("firstrun.back")) + "\n")

	case stepAgentNameDir:
		stepNum, total := stepProgress(m.step, m.hasPresets)
		b.WriteString("\n  " + StyleSubtle.Render(fmt.Sprintf("Step %d/%d: "+i18n.T("firstrun.enter_name_dir"), stepNum, total)) + "\n")

		langs := []string{"en", "zh", "wen"}
		sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)

		cur := func(idx int) string {
			if idx == m.fieldIdx {
				return "> "
			}
			return "  "
		}

		boolLabel := func(idx int) string {
			if idx == 0 {
				return "true"
			}
			return "false"
		}

		renderToggle := func(val string, active bool) string {
			if active {
				return lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render("< " + val + " >")
			}
			return val
		}

		// ── Identity ──
		b.WriteString("\n  " + sectionStyle.Render("── "+i18n.T("firstrun.section_identity")+" ──") + "\n")
		b.WriteString(cur(0) + i18n.T("firstrun.agent_name") + ": " + m.nameInput.View() + "\n")
		b.WriteString(cur(1) + i18n.T("firstrun.agent_dir") + ": " + m.dirInput.View() + "\n")
		langVal := langs[m.agentLangIdx]
		b.WriteString(cur(2) + i18n.T("firstrun.language") + ": " + renderToggle(langVal, m.fieldIdx == 2) + "\n")

		// ── Runtime ──
		b.WriteString("\n  " + sectionStyle.Render("── "+i18n.T("firstrun.section_runtime")+" ──") + "\n")
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

		// ── Authority ──
		b.WriteString("\n  " + sectionStyle.Render("── "+i18n.T("firstrun.section_authority")+" ──") + "\n")
		karmaVal := boolLabel(m.karmaIdx)
		karmaHint := StyleFaint.Render(" (" + i18n.T("firstrun.karma_hint") + ")")
		b.WriteString(cur(7) + i18n.T("firstrun.karma") + ": " + renderToggle(karmaVal, m.fieldIdx == 7) + karmaHint + "\n")
		nirvanaVal := boolLabel(m.nirvanaIdx)
		nirvanaHint := StyleFaint.Render(" (" + i18n.T("firstrun.nirvana_hint") + ")")
		b.WriteString(cur(8) + i18n.T("firstrun.nirvana") + ": " + renderToggle(nirvanaVal, m.fieldIdx == 8) + nirvanaHint + "\n")

		// ── Prompts ──
		b.WriteString("\n  " + sectionStyle.Render("── "+i18n.T("firstrun.section_prompts")+" ──") + "\n")
		b.WriteString(cur(9) + i18n.T("firstrun.covenant") + ": " + m.covenantInput.View() + "\n")
		b.WriteString(cur(10) + i18n.T("firstrun.principle") + ": " + m.principleInput.View() + "\n")
		b.WriteString(cur(11) + i18n.T("firstrun.soul_flow") + ": " + m.soulFlowInput.View() + "\n")
		commentHint := StyleFaint.Render(" (" + i18n.T("firstrun.comment_hint") + ")")
		b.WriteString(cur(12) + i18n.T("firstrun.comment") + ": " + m.commentInput.View() + commentHint + "\n")

		if m.message != "" {
			errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
			b.WriteString("\n  " + errStyle.Render(m.message) + "\n")
		}
		b.WriteString("\n" + StyleFaint.Render("  ↑↓ "+i18n.T("firstrun.toggle_field")+
			"  ←→ "+i18n.T("firstrun.toggle_region")+
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
		style := lipgloss.NewStyle().Foreground(ColorText)
		var line string
		if i == m.langCursor {
			style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
			line = style.Render("[" + label + "]")
		} else {
			line = " " + style.Render(label) + " "
		}
		content.WriteString(centerText(line, m.width) + "\n")
	}

	// Bootstrap status — single line, updates in place
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
const agentNameDirFieldCount = 13
// Field indices:
// 0=name, 1=dir, 2=lang,
// 3=stamina, 4=context_limit, 5=soul_delay, 6=molt_pressure,
// 7=karma, 8=nirvana,
// 9=covenant, 10=principle, 11=soul_flow, 12=comment

// runCheckCaps runs `python -m lingtai check-caps` in a goroutine.
func (m FirstRunModel) runCheckCaps() tea.Cmd {
	return func() tea.Msg {
		python := config.LingtaiCmd(m.globalDir)
		cmd := exec.Command(python, "-m", "lingtai", "check-caps")
		out, err := cmd.Output()
		if err != nil {
			return capCheckErrMsg{err: fmt.Sprintf("check-caps failed: %v", err)}
		}
		var infos map[string]capInfo
		if err := json.Unmarshal(out, &infos); err != nil {
			return capCheckErrMsg{err: fmt.Sprintf("check-caps parse error: %v", err)}
		}
		return capCheckDoneMsg{infos: infos}
	}
}

// enterCapabilities transitions to stepCapabilities.
func (m *FirstRunModel) enterCapabilities() tea.Cmd {
	m.step = stepCapabilities
	m.capLoading = true
	m.capErr = ""
	m.capCursor = 0
	m.capOrder = AllCapabilities
	m.capSelected = make(map[string]bool)
	m.capInfos = nil
	return m.runCheckCaps()
}

// isCapCompatible checks if a capability works with the given provider.
func (m FirstRunModel) isCapCompatible(info capInfo, provider string) bool {
	if len(info.Providers) == 0 {
		return true
	}
	if info.Default != nil {
		return true
	}
	for _, p := range info.Providers {
		if p == provider {
			return true
		}
	}
	return false
}

// isCapLocal checks if a capability has a "local" provider option.
func (m FirstRunModel) isCapLocal(info capInfo) bool {
	for _, p := range info.Providers {
		if p == "local" {
			return true
		}
	}
	return false
}

// applyCapSelections writes the user's capability selections back into the preset manifest.
func (m *FirstRunModel) applyCapSelections() {
	p := &m.presets[m.cursor]
	caps, ok := p.Manifest["capabilities"].(map[string]interface{})
	if !ok {
		caps = make(map[string]interface{})
		p.Manifest["capabilities"] = caps
	}

	for _, name := range m.capOrder {
		if name == "file" {
			delete(caps, "file") // remove group key; individual keys below
			fileKeys := []string{"read", "write", "edit", "glob", "grep"}
			if m.capSelected[name] {
				for _, fk := range fileKeys {
					if _, exists := caps[fk]; !exists {
						caps[fk] = map[string]interface{}{}
					}
				}
			} else {
				for _, fk := range fileKeys {
					delete(caps, fk)
				}
			}
			continue
		}

		if m.capSelected[name] {
			if _, exists := caps[name]; !exists {
				info := m.capInfos[name]
				provider := m.getPresetProvider(*p)
				if !m.isCapCompatible(info, provider) && m.isCapLocal(info) {
					caps[name] = map[string]interface{}{"provider": "local"}
				} else {
					caps[name] = map[string]interface{}{}
				}
			}
		} else {
			delete(caps, name)
		}
	}
}

// enterAgentNameDir initialises all fields and transitions to stepAgentNameDir.
func (m *FirstRunModel) enterAgentNameDir(p preset.Preset) {
	defaultName := p.Name
	m.agentName = defaultName
	m.agentDir = defaultName
	m.nameInput.SetValue(defaultName)
	m.dirInput.SetValue(defaultName)
	m.fieldIdx = 0
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

	// Pre-fill prompt paths based on language
	langs := []string{"en", "zh", "wen"}
	lang := langs[m.agentLangIdx]
	m.covenantInput.SetValue(preset.CovenantPath(m.globalDir, lang))
	m.principleInput.SetValue(preset.PrinciplePath(m.globalDir, lang))
	m.soulFlowInput.SetValue(preset.SoulFlowPath(m.globalDir, lang))
	m.commentInput.SetValue("")
	m.covenantDirty = false
	m.principleDirty = false
	m.soulFlowDirty = false
	m.karmaIdx = 0  // true
	m.nirvanaIdx = 1 // false

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
	m.covenantInput.Blur()
	m.principleInput.Blur()
	m.soulFlowInput.Blur()
	m.commentInput.Blur()

	switch m.fieldIdx {
	case 0:
		return m.nameInput.Focus()
	case 1:
		return m.dirInput.Focus()
	case 2:
		return nil // language — cycle selector
	case 3:
		return m.staminaInput.Focus()
	case 4:
		return m.ctxLimitInput.Focus()
	case 5:
		return m.soulDelayInput.Focus()
	case 6:
		return m.moltPressInput.Focus()
	case 7, 8:
		return nil // karma/nirvana — cycle selectors
	case 9:
		return m.covenantInput.Focus()
	case 10:
		return m.principleInput.Focus()
	case 11:
		return m.soulFlowInput.Focus()
	case 12:
		return m.commentInput.Focus()
	}
	return nil
}

// updatePromptPaths updates prompt path fields when language changes,
// but only if the user hasn't manually edited them.
func (m *FirstRunModel) updatePromptPaths() {
	langs := []string{"en", "zh", "wen"}
	lang := langs[m.agentLangIdx]
	if !m.covenantDirty {
		m.covenantInput.SetValue(preset.CovenantPath(m.globalDir, lang))
	}
	if !m.principleDirty {
		m.principleInput.SetValue(preset.PrinciplePath(m.globalDir, lang))
	}
	if !m.soulFlowDirty {
		m.soulFlowInput.SetValue(preset.SoulFlowPath(m.globalDir, lang))
	}
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
