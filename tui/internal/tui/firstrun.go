package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"

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

// SetupSavedMsg is emitted when /setup rewrites the current agent's init.json.
type SetupSavedMsg struct{}

// PresetKeyEditorDoneMsg is emitted when an external editor returns with field text.
type PresetKeyEditorDoneMsg struct{ Text string }

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

// rehydrateDoneMsg is emitted when RehydrateNetwork finishes during the
// rehydration flow's stepPropagate. Carries the worker count on success
// or a non-empty error string on failure.
type rehydrateDoneMsg struct {
	workers int
	err     string
}

type firstRunStep int

const (
	stepWelcome firstRunStep = iota
	stepAPIKey
	stepPickPreset
	stepPresetKey
	stepCapabilities
	stepAgentNameDir
	stepRecipe            // picks one of 5 recipes (greeter, plain, adaptive, tutorial, custom)
	stepRecipeSwapConfirm // mid-life only — confirms recipe change (Task 9 wires this)
	stepPropagate         // rehydration only — runs after orchestrator save, before launch
	stepLaunching
)

// zhipuCodingModels lists models available on the Zhipu GLM Coding Plan.
var zhipuCodingModels = []string{"GLM-5.1", "GLM-5-Turbo", "GLM-4.7", "GLM-4.5-Air"}

// capInfo holds provider metadata for a single capability (from check-caps).
type capInfo struct {
	Providers []string `json:"providers"`
	Default   *string  `json:"default"`
}

// stepProgress returns the 1-based index and total for progress display
func stepProgress(step firstRunStep, hasPresets, setupMode bool) (current int, total int) {
	if setupMode {
		total = 4 // preset → capabilities → details → recipe
	} else if hasPresets {
		total = 4 // preset → capabilities → details → recipe
	} else {
		total = 5 // api key → preset → capabilities → details → recipe
	}
	switch {
	case !hasPresets && step == stepAPIKey:
		return 1, total
	case !hasPresets && step == stepPickPreset:
		return 2, total
	case step == stepPickPreset || step == stepPresetKey:
		return 1, total
	case step == stepCapabilities:
		if setupMode || hasPresets {
			return 2, total
		}
		return 3, total
	case step == stepAgentNameDir:
		if setupMode || hasPresets {
			return 3, total
		}
		return 4, total
	case step == stepRecipe || step == stepRecipeSwapConfirm:
		if setupMode || hasPresets {
			return 4, total
		}
		return 5, total
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
	setupMode   bool // true when opened from /setup (skip welcome/bootstrap/tutorial, esc→mail)
	setupOrchDir  string // current agent dir (setup mode only — overwrites init.json here)
	setupOrchName string // current agent name (setup mode only — prefills name input)
	setupLoadedAddonNames []string // addon names loaded from existing init.json (setup mode)
	// Rehydration mode (agora cloned network): runs the normal first-run wizard
	// but prefills the orchestrator name/dir from .agent.json, locks the dir
	// (can't be edited), and adds stepPropagate at the end to propagate the
	// orchestrator's init.json to every other agent via preset.RehydrateNetwork.
	rehydrateMode     bool
	rehydrateOrchDir  string // existing orchestrator directory name (not a full path)
	rehydrateOrchName string // existing orchestrator agent_name from its .agent.json
	rehydrateWorkers  int    // count of workers propagated (set at stepPropagate completion)
	rehydrateErr      string // non-empty if RehydrateNetwork failed
	// Bootstrap state (venv + assets install)
	setupDone    bool        // true when bootstrap goroutine finishes
	setupErr     string      // non-empty if bootstrap failed
	setupStatus  string      // current progress i18n key (active step)
	setupSteps   []string    // completed step i18n keys (shown with checkmarks)
	progressCh   chan string // channel for progress updates
	// Embedded key input for preset's provider
	presetKeyInput    textarea.Model
	presetEndpointIn  textinput.Model   // base_url for custom provider
	presetModelIn     textinput.Model   // model name for custom provider
	presetNameIn      textinput.Model   // preset name for custom provider (separate from nameInput)
	presetKeyFieldIdx int               // 0=compat, 1=endpoint, 2=model, 3=key, 4=name (custom); 0=region, 1=model, 2=key (minimax/zhipu)
	minimaxRegion     int               // 0=china, 1=international
	minimaxModel      int               // 0=highspeed, 1=standard
	zhipuRegion       int               // 0=china, 1=international
	zhipuModel        int               // 0=GLM-5.1, 1=GLM-5-Turbo, 2=GLM-4.7, 3=GLM-4.5-Air
	customCompat      int               // 0=openai, 1=anthropic
	selectedProvider  string            // provider of currently selected preset
	existingKeys      map[string]string // loaded from Config.Keys
	// Capability selection state (stepCapabilities)
	capInfos     map[string]capInfo // from check-caps CLI output
	capSelected  map[string]bool    // user toggle state
	capProviders map[string]string  // user's chosen provider per capability (only for caps with ≥2 compatible options)
	capOrder     []string           // ordered list matching AllCapabilities
	capCursor    int                // current cursor position (0..len-1)
	capLoading   bool               // true while check-caps is running
	capErr       string             // error message if check-caps fails
	// Addon selection state (shown below capabilities)
	addonSelected map[string]bool // "imap", "telegram"
	addonOrder    []string        // ["imap", "telegram"]
	addonCursor   int             // cursor when in addon zone
	inAddonZone   bool            // true when cursor is in addon section

	// Recipe picker state (stepRecipe)
	recipeIdx         int             // cursor in recipe list (0..4 or 0..5 if imported)
	recipeCustomInput textinput.Model // folder path input for custom recipe
	recipeCustomErr   string          // validation error message
	currentRecipe     string          // loaded from .tui-asset/.recipe in setup mode
	currentCustomDir  string          // loaded from .tui-asset/.recipe in setup mode
	preselectedRecipe  string          // set by constructor for post-nirvana fresh start
	localRecipeDir     string          // non-empty if .lingtai-recipe/ found in project root
	importedRecipe    *preset.RecipeInfo // non-nil if .lingtai-recipe/ has valid recipe.json
	importedRecipeDir string             // path to .lingtai-recipe/ (only when importedRecipe != nil)

	// Recipe preview sub-view (reuses skills-style two-panel layout)
	recipePreview       bool            // true when in preview mode
	recipePreviewFile   int             // 0=greet.md, 1=comment.md
	recipePreviewVP     viewport.Model  // scrollable content pane
	recipePreviewReady  bool            // viewport initialized

	// Pending save state (captured at end of stepAgentNameDir, consumed by stepRecipe)
	pendingAgentOpts preset.AgentOpts
	pendingDirName   string

	// Swap-confirm state (stepRecipeSwapConfirm — wired in Task 9)
	pendingRecipeName string
	pendingCustomDir  string
	swapConfirmIdx    int // 0=swap, 1=fresh, 2=cancel
}

func NewFirstRunModel(baseDir, globalDir string, hasPresets bool, preselectedRecipe string) FirstRunModel {
	ti := textinput.New()
	ti.CharLimit = 64
	ti.SetWidth(40)

	di := textinput.New()
	di.CharLimit = 64
	di.SetWidth(40)

	pki := textarea.New()
	pki.CharLimit = 512
	pki.SetWidth(50)
	pki.SetHeight(1)
	pki.ShowLineNumbers = false
	pki.Placeholder = "paste API key here"
	pki.Prompt = ""
	pki.KeyMap.InsertNewline.SetKeys() // no newlines — single line
	pki.SetStyles(themedTextareaStyles())

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

	rci := textinput.New()
	rci.CharLimit = 512
	rci.SetWidth(50)
	rci.Placeholder = ".lingtai-recipe/ or absolute path"

	// Load existing keys from Config.Keys
	cfg, _ := config.LoadConfig(globalDir)
	existingKeys := cfg.Keys
	if existingKeys == nil {
		existingKeys = make(map[string]string)
	}

	// Pre-set language cursor from TUI config
	langCursor := 0
	langOptions := []string{"en", "zh", "wen"}
	tuiCfg := config.LoadTUIConfig(globalDir)
	for i, l := range langOptions {
		if l == tuiCfg.Language {
			langCursor = i
			break
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
		commentInput:      comi,
		nirvanaIdx:        1, // default false (1=false)
		progressCh:        make(chan string, 4),
		recipeCustomInput: rci,
		preselectedRecipe: preselectedRecipe,
	}

	// Detect project-local .lingtai-recipe/ directory.
	// The projectDir is one level up from baseDir (.lingtai/).
	projectDir := filepath.Dir(baseDir)
	if local := preset.ProjectLocalRecipeDir(projectDir); local != "" {
		lang := "en"
		if m.pendingAgentOpts.Language != "" {
			lang = m.pendingAgentOpts.Language
		}
		if info, err := preset.LoadRecipeInfo(local, lang); err == nil {
			m.importedRecipe = &info
			m.importedRecipeDir = local
		} else {
			// Has .lingtai-recipe/ but no valid recipe.json — fallback to custom pre-fill
			m.localRecipeDir = local
			m.recipeCustomInput.SetValue(local)
		}
	}

	// Default to imported recipe if detected and no explicit preselection
	if m.importedRecipe != nil && preselectedRecipe == "" {
		m.recipeIdx = 0
	} else {
		m.recipeIdx = m.recipeNameToIdx(preselectedRecipe)
	}

	return m
}

// NewSetupModeModel creates a FirstRunModel for /setup — skips welcome/bootstrap/tutorial,
// starts at preset selection with presets preloaded, and overwrites the current agent on completion.
func NewSetupModeModel(baseDir, globalDir, orchDir, orchName string) FirstRunModel {
	m := NewFirstRunModel(baseDir, globalDir, true, "")
	m.setupMode = true
	m.setupOrchDir = orchDir
	m.setupOrchName = orchName
	m.step = stepPickPreset
	m.presets, _ = preset.List()

	// Load existing addons from orchestrator's init.json so they are preserved
	// when the user reaches the capabilities step (enterCapabilities resets addonSelected).
	if orchDir != "" {
		initPath := filepath.Join(orchDir, "init.json")
		if data, err := os.ReadFile(initPath); err == nil {
			var existing map[string]interface{}
			if json.Unmarshal(data, &existing) == nil {
				if addons, ok := existing["addons"].(map[string]interface{}); ok && addons != nil {
					for name := range addons {
						m.setupLoadedAddonNames = append(m.setupLoadedAddonNames, name)
					}
				}
			}
		}
	}

	// Load current recipe state for pre-selection
	state, _ := preset.LoadRecipeState(baseDir)
	m.currentRecipe = state.Recipe
	m.currentCustomDir = state.CustomDir
	m.preselectedRecipe = state.Recipe
	m.recipeIdx = m.recipeNameToIdx(state.Recipe)
	if state.Recipe == preset.RecipeCustom && state.CustomDir != "" {
		m.recipeCustomInput.SetValue(state.CustomDir)
	}

	return m
}

// NewRehydrateModel creates a FirstRunModel for the agora rehydration flow.
// Unlike NewSetupModeModel, rehydration runs the FULL first-run wizard
// (welcome, bootstrap, tutorial, preset, capabilities, agent name, etc.) —
// the user is genuinely setting up a network for the first time on their
// machine. The only differences from a fresh first-run are:
//
//   - The orchestrator's agent name is prefilled from the existing
//     .agent.json's agent_name field (the user can still edit it).
//   - The orchestrator's directory name is locked to the existing directory
//     (cannot be renamed — the directory already exists on disk).
//   - The dir-exists check is skipped in the save handler (normal first-run
//     refuses to overwrite an existing directory; rehydration expects it).
//   - After the orchestrator's init.json is written, the wizard advances to
//     stepPropagate instead of stepLaunching, which calls
//     preset.RehydrateNetwork to propagate the new init.json to every
//     non-orchestrator agent. stepPropagate then advances to stepLaunching
//     as usual.
//
// orchDir is the existing orchestrator directory name (not a full path),
// and orchName is the agent_name read from that directory's .agent.json.
func NewRehydrateModel(baseDir, globalDir, orchDir, orchName string, hasPresets bool) FirstRunModel {
	m := NewFirstRunModel(baseDir, globalDir, hasPresets, "")
	m.rehydrateMode = true
	m.rehydrateOrchDir = orchDir
	m.rehydrateOrchName = orchName
	return m
}

func (m FirstRunModel) Init() tea.Cmd {
	if m.setupMode {
		// Already bootstrapped — no init needed, just blink for text inputs
		return nil
	}
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

// runRehydratePropagation runs preset.RehydrateNetwork in the background
// and emits a rehydrateDoneMsg with the worker count or an error. Called
// after the orchestrator's init.json has been written in rehydration mode.
func (m FirstRunModel) runRehydratePropagation() tea.Cmd {
	baseDir := m.baseDir
	orchDir := m.rehydrateOrchDir
	return func() tea.Msg {
		n, err := preset.RehydrateNetwork(baseDir, orchDir)
		if err != nil {
			return rehydrateDoneMsg{workers: n, err: err.Error()}
		}
		return rehydrateDoneMsg{workers: n}
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
		if m.recipePreviewReady {
			m.recipePreviewVP.SetWidth(m.width)
			m.recipePreviewVP.SetHeight(m.height - 4) // header + footer
			m.syncRecipePreviewContent()
		}
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

	case rehydrateDoneMsg:
		m.rehydrateWorkers = msg.workers
		m.rehydrateErr = msg.err
		// User presses Enter on the propagate page to advance to stepLaunching,
		// see the KeyPressMsg handler for stepPropagate below.
		return m, nil

	case capCheckDoneMsg:
		m.capLoading = false
		m.capInfos = msg.infos
		p := m.presets[m.cursor]
		provider := m.getPresetProvider(p)
		// Backfill capabilities not returned by check-caps so they're toggleable.
		// For custom/openrouter providers, skip MiniMax-only media capabilities
		// (vision, talk, draw, compose, video) — they cause errors.
		minimaxOnly := map[string]bool{"vision": true, "talk": true, "draw": true, "compose": true, "video": true}
		for _, name := range m.capOrder {
			if _, ok := m.capInfos[name]; !ok {
				if (provider == "custom" || provider == "zhipu") && minimaxOnly[name] {
					continue
				}
				m.capInfos[name] = capInfo{}
			}
		}
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
		m.initCapProviders()
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
		m.initCapProviders()
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

	case PresetKeyEditorDoneMsg:
		if msg.Text != "" {
			if focused := m.focusedPresetKeyInput(); focused != nil {
				focused.SetValue(msg.Text)
			} else if ta := m.focusedPresetKeyTextarea(); ta != nil {
				ta.SetValue(msg.Text)
			}
		}
		return m, textinput.Blink

	case tea.MouseWheelMsg:
		if m.step == stepRecipe && m.recipePreview && m.recipePreviewReady {
			var cmd tea.Cmd
			m.recipePreviewVP, cmd = m.recipePreviewVP.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyPressMsg:
		switch m.step {
		case stepWelcome:
			langs := []string{"en", "zh", "wen"}
			switch msg.String() {
			case "ctrl+t":
				// Cycle through registered themes
				names := ThemeNames()
				tuiCfg := config.LoadTUIConfig(m.globalDir)
				current := tuiCfg.Theme
				if current == "" {
					current = DefaultThemeName
				}
				next := names[0]
				for i, n := range names {
					if n == current {
						next = names[(i+1)%len(names)]
						break
					}
				}
				tuiCfg.Theme = next
				SetThemeByName(next)
				config.SaveTUIConfig(m.globalDir, tuiCfg)
				return m, nil
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
				if !m.setupDone || m.setupErr != "" {
					return m, nil // blocked — still installing or failed
				}
				lang := langs[m.langCursor]
				// Save language to TUI config
				tuiCfg := config.LoadTUIConfig(m.globalDir)
				tuiCfg.Language = lang
				config.SaveTUIConfig(m.globalDir, tuiCfg)
				// Opened from /settings — return to mail
				if m.welcomeOnly {
					return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
				}
				// Reload keys after potential config change
				keyCfg, _ := config.LoadConfig(m.globalDir)
				m.existingKeys = keyCfg.Keys
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
					tuiCfg := config.LoadTUIConfig(m.globalDir)
					i18n.SetLang(tuiCfg.Language)
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
					// Always go to key page — prefill if key exists
					m.step = stepPresetKey
					m.presetKeyInput.Reset()
					m.presetEndpointIn.Reset()
					m.presetModelIn.Reset()
					m.presetNameIn.Reset()
					m.presetKeyFieldIdx = 0
					if provider == "custom" {
						// field 0 = compat selector (no text focus)
						m.customCompat = 0
						// Prefill from saved preset manifest if available
						if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
							if baseURL, ok := llm["base_url"].(string); ok && baseURL != "" {
								m.presetEndpointIn.SetValue(baseURL)
							} else {
								m.presetEndpointIn.SetValue("https://openrouter.ai/api/v1")
							}
							if model, ok := llm["model"].(string); ok && model != "" {
								m.presetModelIn.SetValue(model)
							}
							if compat, ok := llm["api_compat"].(string); ok && compat == "anthropic" {
								m.customCompat = 1
							}
						} else {
							m.presetEndpointIn.SetValue("https://openrouter.ai/api/v1")
						}
						if p.Name != "" && !preset.IsBuiltin(p.Name) {
							m.presetNameIn.SetValue(p.Name)
						} else {
							m.presetNameIn.SetValue("openrouter")
						}
					} else if provider == "minimax" {
						// field 0 = region selector (no text focus)
						m.presetKeyInput.Blur()
						// Prefill region from saved preset's base_url
						if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
							if baseURL, ok := llm["base_url"].(string); ok && strings.Contains(baseURL, "minimaxi.com") {
								m.minimaxRegion = 0 // China
							} else {
								m.minimaxRegion = 1 // International
							}
						}
						// Prefill model from saved preset
						m.minimaxModel = 0 // default highspeed
						if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
							if model, ok := llm["model"].(string); ok && model == "MiniMax-M2.7" {
								m.minimaxModel = 1 // standard
							}
						}
					} else if provider == "zhipu" {
						// field 0 = region selector (no text focus)
						m.presetKeyInput.Blur()
						// Prefill region from saved preset's base_url
						m.zhipuRegion = 0 // default China
						if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
							if baseURL, ok := llm["base_url"].(string); ok && strings.Contains(baseURL, "api.z.ai") {
								m.zhipuRegion = 1 // International
							}
						}
						// Prefill model from saved preset
						m.zhipuModel = 0 // default GLM-5.1
						if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
							if model, ok := llm["model"].(string); ok {
								switch model {
								case "GLM-5-Turbo":
									m.zhipuModel = 1
								case "GLM-4.7":
									m.zhipuModel = 2
								case "GLM-4.5-Air":
									m.zhipuModel = 3
								}
							}
						}
					} else {
						m.presetKeyInput.Focus()
					}
					// Prefill with existing key
					if existing := m.existingKeys[provider]; existing != "" {
						m.presetKeyInput.SetValue(existing)
					}
					return m, textinput.Blink
				}
			case "backspace", "delete":
				// Delete a saved (non-builtin) preset
				if m.cursor < len(m.presets) && !preset.IsBuiltin(m.presets[m.cursor].Name) {
					name := m.presets[m.cursor].Name
					if err := preset.Delete(name); err == nil {
						// Refresh the list
						m.presets, _ = preset.List()
						if m.cursor >= len(m.presets) {
							m.cursor = len(m.presets) - 1
						}
						if m.cursor < 0 {
							m.cursor = 0
						}
					}
				}
				return m, nil
			case "esc":
				if m.setupMode {
					return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
				}
				m.step = stepWelcome
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil

		case stepPresetKey:
			isCustom := m.selectedProvider == "custom"
			isMinimax := m.selectedProvider == "minimax"
			isZhipu := m.selectedProvider == "zhipu"
			fieldCount := 1 // default: key only
			if isCustom {
				fieldCount = 5 // compat + endpoint + model + key + name
			}
			if isMinimax {
				fieldCount = 3 // region + model + key
			}
			if isZhipu {
				fieldCount = 3 // region + model + key
			}
			switch msg.String() {
			case "ctrl+e":
				// Open external editor for the currently focused text field
				var currentVal string
				if focused := m.focusedPresetKeyInput(); focused != nil {
					currentVal = focused.Value()
				} else if ta := m.focusedPresetKeyTextarea(); ta != nil {
					currentVal = ta.Value()
				} else {
					return m, nil
				}
				tmpFile, err := os.CreateTemp("", "lingtai-field-*.txt")
				if err != nil {
					return m, nil
				}
				tmpFile.WriteString(currentVal)
				tmpFile.Close()
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vim"
				}
				return m, tea.ExecProcess(exec.Command(editor, tmpFile.Name()), func(err error) tea.Msg {
					if err != nil {
						os.Remove(tmpFile.Name())
						return nil
					}
					content, _ := os.ReadFile(tmpFile.Name())
					os.Remove(tmpFile.Name())
					return PresetKeyEditorDoneMsg{Text: strings.TrimSpace(string(content))}
				})
			case "esc":
				m.step = stepPickPreset
				return m, nil
			case "up":
				if isCustom || isMinimax || isZhipu {
					m.presetKeyFieldIdx = (m.presetKeyFieldIdx - 1 + fieldCount) % fieldCount
					if (isMinimax || isZhipu) && m.presetKeyFieldIdx < 2 {
						m.presetKeyInput.Blur()
						return m, nil
					}
					return m, m.focusPresetKeyField()
				}
				return m, nil
			case "down", "tab":
				if isCustom || isMinimax || isZhipu {
					m.presetKeyFieldIdx = (m.presetKeyFieldIdx + 1) % fieldCount
					if (isMinimax || isZhipu) && m.presetKeyFieldIdx < 2 {
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
				// Toggle model for minimax
				if isMinimax && m.presetKeyFieldIdx == 1 {
					m.minimaxModel = 1 - m.minimaxModel
					return m, nil
				}
				// Toggle region for zhipu
				if isZhipu && m.presetKeyFieldIdx == 0 {
					m.zhipuRegion = 1 - m.zhipuRegion
					return m, nil
				}
				// Cycle model for zhipu (4 options): right=forward, left=backward
				if isZhipu && m.presetKeyFieldIdx == 1 {
					if msg.String() == "right" {
						m.zhipuModel = (m.zhipuModel + 1) % len(zhipuCodingModels)
					} else {
						m.zhipuModel = (m.zhipuModel + len(zhipuCodingModels) - 1) % len(zhipuCodingModels)
					}
					return m, nil
				}
				// Toggle compat for custom
				if isCustom && m.presetKeyFieldIdx == 0 {
					m.customCompat = 1 - m.customCompat
					return m, nil
				}
			case "enter":
				key := strings.TrimSpace(m.presetKeyInput.Value())
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
					var name, baseURL, model string
					if m.minimaxRegion == 0 {
						name = "minimax_cn"
						baseURL = "https://api.minimaxi.com/anthropic"
					} else {
						name = "minimax_intl"
						baseURL = "https://api.minimax.io/anthropic"
					}
					if m.minimaxModel == 0 {
						model = "MiniMax-M2.7-highspeed"
					} else {
						model = "MiniMax-M2.7"
					}
					clone := preset.Clone(p, name)
					if llm, ok := clone.Manifest["llm"].(map[string]interface{}); ok {
						llm["base_url"] = baseURL
						llm["model"] = model
					}
					if err := preset.Save(clone); err != nil {
						m.message = i18n.TF("firstrun.error", err)
						return m, nil
					}
					newPresetName = name
				}
				if isZhipu {
					// Clone the template with auto-name based on region
					p := m.presets[m.cursor]
					var name, baseURL string
					if m.zhipuRegion == 0 {
						name = "zhipu_cn"
						baseURL = "https://open.bigmodel.cn/api/coding/paas/v4"
					} else {
						name = "zhipu_intl"
						baseURL = "https://api.z.ai/api/coding/paas/v4"
					}
					model := zhipuCodingModels[m.zhipuModel]
					clone := preset.Clone(p, name)
					if llm, ok := clone.Manifest["llm"].(map[string]interface{}); ok {
						llm["base_url"] = baseURL
						llm["model"] = model
						llm["api_compat"] = "openai"
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
				} else if (isMinimax || isZhipu) && m.presetKeyFieldIdx == 2 {
					m.presetKeyInput, cmd = m.presetKeyInput.Update(msg)
				} else if !isMinimax && !isZhipu {
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
				if m.inAddonZone {
					if m.addonCursor > 0 {
						m.addonCursor--
					} else {
						// Exit addon zone, go to bottom of capability grid
						m.inAddonZone = false
						m.capCursor = colSize - 1 // bottom of left column
					}
				} else {
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
				}
			case "down":
				if m.inAddonZone {
					if m.addonCursor < len(m.addonOrder)-1 {
						m.addonCursor++
					}
				} else {
					if m.capCursor >= colSize {
						// Right column
						if m.capCursor < len(m.capOrder)-1 {
							m.capCursor++
						} else {
							// At bottom of right column — enter addon zone
							m.inAddonZone = true
							m.addonCursor = 0
						}
					} else {
						// Left column
						if m.capCursor < colSize-1 {
							m.capCursor++
						} else {
							// At bottom of left column — enter addon zone
							m.inAddonZone = true
							m.addonCursor = 0
						}
					}
				}
			case "left":
				if !m.inAddonZone && m.capCursor >= colSize {
					m.capCursor -= colSize
				}
			case "right":
				if !m.inAddonZone && m.capCursor < colSize && m.capCursor+colSize < len(m.capOrder) {
					m.capCursor += colSize
				}
			case "space":
				if m.inAddonZone {
					name := m.addonOrder[m.addonCursor]
					m.addonSelected[name] = !m.addonSelected[name]
				} else {
					name := m.capOrder[m.capCursor]
					info, ok := m.capInfos[name]
					if !ok {
						return m, nil
					}
					provider := m.getPresetProvider(m.presets[m.cursor])
					if m.isCapCompatible(info, provider) || m.isCapLocal(info) {
						m.capSelected[name] = !m.capSelected[name]
					}
				}
			case "tab":
				// Cycle the provider for the focused capability (if it has ≥2 compatible options).
				if !m.inAddonZone {
					name := m.capOrder[m.capCursor]
					info := m.capInfos[name]
					presetProvider := m.getPresetProvider(m.presets[m.cursor])
					compat := m.compatibleProviders(info, presetProvider)
					if len(compat) >= 2 {
						cur := m.capProviders[name]
						for i, p := range compat {
							if p == cur {
								m.capProviders[name] = compat[(i+1)%len(compat)]
								break
							}
						}
					}
				}
			case "ctrl+a":
				provider := m.getPresetProvider(m.presets[m.cursor])
				// If all selectable caps are selected, deselect all; otherwise select all
				allSelected := true
				for _, name := range m.capOrder {
					info := m.capInfos[name]
					if (m.isCapCompatible(info, provider) || m.isCapLocal(info)) && !m.capSelected[name] {
						allSelected = false
						break
					}
				}
				for _, name := range m.capOrder {
					info := m.capInfos[name]
					if m.isCapCompatible(info, provider) || m.isCapLocal(info) {
						m.capSelected[name] = !allSelected
					}
				}
			case "enter":
				m.applyCapSelections()
				p := m.presets[m.cursor]
				m.enterAgentNameDir(p)
				m.step = stepAgentNameDir
				return m, textinput.Blink
			case "esc":
				m.step = stepPickPreset
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
				if (m.setupMode || m.rehydrateMode) && m.fieldIdx == 1 { // skip dir field
					m.fieldIdx = 2
				}
				return m, m.focusAgentField()
			case "up":
				m.fieldIdx = (m.fieldIdx - 1 + agentNameDirFieldCount) % agentNameDirFieldCount
				if (m.setupMode || m.rehydrateMode) && m.fieldIdx == 1 { // skip dir field
					m.fieldIdx = 0
				}
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
					// CommentFile is set by stepRecipe from the chosen recipe
				}
				var selectedAddons []string
				for _, addonName := range m.addonOrder {
					if m.addonSelected[addonName] {
						selectedAddons = append(selectedAddons, addonName)
					}
				}
				opts.Addons = selectedAddons

				dirName := m.dirInput.Value()
				if dirName == "" {
					dirName = name
				}
				if m.rehydrateMode && m.rehydrateOrchDir != "" {
					dirName = m.rehydrateOrchDir
				}
				m.agentName = name
				m.agentDir = dirName

				// Validate dir doesn't already exist (first-run only, not setup/rehydrate)
				if !m.setupMode && !m.rehydrateMode {
					orchDir := filepath.Join(m.baseDir, dirName)
					if _, err := os.Stat(orchDir); err == nil {
						m.message = i18n.TF("firstrun.dir_exists", dirName)
						return m, nil
					}
				}

				// Stash for stepRecipe to consume
				m.pendingAgentOpts = opts
				m.pendingDirName = dirName
				if m.setupMode {
					m.pendingDirName = filepath.Base(m.setupOrchDir)
				}

				m.step = stepRecipe
				m.message = ""
				// Focus custom input if pre-selected to custom
				if m.recipeIdxToName(m.recipeIdx) == preset.RecipeCustom {
					m.recipeCustomInput.Focus()
				} else {
					m.recipeCustomInput.Blur()
				}
				return m, nil
			case "esc":
				m.step = stepCapabilities
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


		case stepRecipe:
			// Recipe preview sub-view: intercept all keys
			if m.recipePreview {
				switch msg.String() {
				case "ctrl+o", "esc":
					m.recipePreview = false
					return m, nil
				case "tab":
					m.recipePreviewFile = 1 - m.recipePreviewFile
					m.syncRecipePreviewContent()
					return m, nil
				case "ctrl+c":
					return m, tea.Quit
				default:
					var cmd tea.Cmd
					m.recipePreviewVP, cmd = m.recipePreviewVP.Update(msg)
					return m, cmd
				}
			}

			switch msg.String() {
			case "up":
				if m.recipeIdx > 0 {
					m.recipeIdx--
					m.recipeCustomErr = ""
				}
				// Focus/blur the custom input based on selection
				if m.recipeIdx == m.recipeMaxIdx() {
					m.recipeCustomInput.Focus()
				} else {
					m.recipeCustomInput.Blur()
				}
				return m, nil
			case "down":
				if m.recipeIdx < m.recipeMaxIdx() {
					m.recipeIdx++
					m.recipeCustomErr = ""
				}
				if m.recipeIdx == m.recipeMaxIdx() {
					m.recipeCustomInput.Focus()
				} else {
					m.recipeCustomInput.Blur()
				}
				return m, nil
			case "ctrl+o":
				m.enterRecipePreview()
				return m, nil
			case "esc":
				m.step = stepAgentNameDir
				m.message = ""
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			case "enter":
				recipeName := m.recipeIdxToName(m.recipeIdx)
				customDir := ""
				if recipeName == preset.RecipeImported {
					customDir = m.importedRecipeDir
				} else if recipeName == preset.RecipeCustom {
					customDir = m.recipeCustomInput.Value()
					if err := preset.ValidateCustomDir(customDir); err != nil {
						m.recipeCustomErr = err.Error()
						return m, nil
					}
				}

				// Mid-life recipe change detection -> route to swap confirm (Task 9)
				if m.setupMode && recipeChanged(m.currentRecipe, m.currentCustomDir, recipeName, customDir) {
					m.pendingRecipeName = recipeName
					m.pendingCustomDir = customDir
					m.step = stepRecipeSwapConfirm
					m.swapConfirmIdx = 0
					return m, nil
				}

				// First-run or unchanged recipe: save directly
				return m.performRecipeSave(recipeName, customDir)
			default:
				if m.recipeIdxToName(m.recipeIdx) == preset.RecipeCustom { // custom selected -- forward to input
					var cmd tea.Cmd
					m.recipeCustomInput, cmd = m.recipeCustomInput.Update(msg)
					return m, cmd
				}
				return m, nil
			}

		case stepRecipeSwapConfirm:
			switch msg.String() {
			case "up":
				if m.swapConfirmIdx > 0 {
					m.swapConfirmIdx--
				}
				return m, nil
			case "down":
				if m.swapConfirmIdx < 2 {
					m.swapConfirmIdx++
				}
				return m, nil
			case "esc":
				m.step = stepRecipe
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			case "enter":
				switch m.swapConfirmIdx {
				case 0: // Swap in place
					return m.performRecipeSave(m.pendingRecipeName, m.pendingCustomDir)
				case 1: // Fresh start
					return m, func() tea.Msg {
						return RecipeFreshStartMsg{
							Recipe:    m.pendingRecipeName,
							CustomDir: m.pendingCustomDir,
						}
					}
				case 2: // Cancel
					m.step = stepRecipe
					return m, nil
				}
			}
			return m, nil

		case stepPropagate:
			// Only Enter (to advance after result) or ctrl+c are valid.
			// Ignore Enter until rehydrateDoneMsg has arrived.
			switch msg.String() {
			case "enter":
				if m.rehydrateWorkers == 0 && m.rehydrateErr == "" {
					return m, nil // still running
				}
				if m.rehydrateErr != "" {
					return m, tea.Quit
				}
				orchDir := filepath.Join(m.baseDir, m.rehydrateOrchDir)
				m.step = stepLaunching
				m.message = i18n.TF("firstrun.created", m.agentName)
				return m, func() tea.Msg {
					return FirstRunDoneMsg{OrchDir: orchDir, OrchName: m.agentName}
				}
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}

	default:
		// Forward unhandled messages (e.g. tea.PasteMsg) to the focused textinput
		var cmd tea.Cmd
		switch m.step {
		case stepPresetKey:
			if focused := m.focusedPresetKeyInput(); focused != nil {
				*focused, cmd = focused.Update(msg)
			} else if ta := m.focusedPresetKeyTextarea(); ta != nil {
				*ta, cmd = ta.Update(msg)
			}
		case stepAgentNameDir:
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
			case 10:
				m.principleInput, cmd = m.principleInput.Update(msg)
			case 11:
				m.soulFlowInput, cmd = m.soulFlowInput.Update(msg)
			case 12:
				m.commentInput, cmd = m.commentInput.Update(msg)
			}
		case stepRecipe:
			if m.recipeIdxToName(m.recipeIdx) == preset.RecipeCustom {
				m.recipeCustomInput, cmd = m.recipeCustomInput.Update(msg)
			}
		case stepAPIKey:
			m.setup, cmd = m.setup.Update(msg)
		}
		return m, cmd
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
		stepNum, total := stepProgress(m.step, m.hasPresets, m.setupMode)
		b.WriteString("\n  " + StyleSubtle.Render(fmt.Sprintf("Step %d/%d", stepNum, total)) + "\n\n")
		b.WriteString("  " + i18n.T("firstrun.no_presets") + "\n\n")
		b.WriteString(m.setup.View())

	case stepPickPreset:
		stepNum, total := stepProgress(m.step, m.hasPresets, m.setupMode)
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
		// Show delete hint when cursor is on a saved (non-builtin) preset
		if m.cursor < len(m.presets) && !preset.IsBuiltin(m.presets[m.cursor].Name) {
			b.WriteString(StyleFaint.Render("  [Del] "+i18n.T("preset.delete")) + "\n")
		}
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
				"  [Ctrl+E] editor (allows pasting)"+
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
			// Model toggle
			hsLabel := "M2.7-highspeed"
			stdLabel := "M2.7"
			if m.minimaxModel == 0 {
				hsLabel = "● " + hsLabel
				stdLabel = "○ " + stdLabel
			} else {
				hsLabel = "○ " + hsLabel
				stdLabel = "● " + stdLabel
			}
			modelStyle := lipgloss.NewStyle()
			if m.presetKeyFieldIdx == 1 {
				modelStyle = modelStyle.Bold(true).Foreground(ColorAccent)
			}
			b.WriteString("  " + i18n.T("presets.model") + ":   " + modelStyle.Render(hsLabel+"  "+stdLabel) + "\n")
			b.WriteString("  " + i18n.T("setup.api_key_label") + "  " + m.presetKeyInput.View() + "\n\n")
			b.WriteString(StyleFaint.Render("  [↑↓] "+i18n.T("firstrun.toggle_field")+
				"  [←→] "+i18n.T("firstrun.toggle_region")+
				"  [Ctrl+E] editor (allows pasting)"+
				"  [Enter] "+i18n.T("setup.save")+
				"  [Esc] "+i18n.T("setup.back")) + "\n")
		} else if m.selectedProvider == "zhipu" {
			// Region toggle
			intlLabel := i18n.T("firstrun.region_intl")
			chinaLabel := i18n.T("firstrun.region_china")
			if m.zhipuRegion == 0 {
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
			endpointURL := "open.bigmodel.cn/api/coding/paas/v4"
			if m.zhipuRegion == 1 {
				endpointURL = "api.z.ai/api/coding/paas/v4"
			}
			b.WriteString("            " + StyleFaint.Render(endpointURL) + "\n")
			// Model cycle (4 options)
			var modelLabels []string
			for i, name := range zhipuCodingModels {
				if i == m.zhipuModel {
					modelLabels = append(modelLabels, "● "+name)
				} else {
					modelLabels = append(modelLabels, "○ "+name)
				}
			}
			modelStyle := lipgloss.NewStyle()
			if m.presetKeyFieldIdx == 1 {
				modelStyle = modelStyle.Bold(true).Foreground(ColorAccent)
			}
			b.WriteString("  " + i18n.T("presets.model") + ":   " + modelStyle.Render(strings.Join(modelLabels, "  ")) + "\n")
			b.WriteString("  " + i18n.T("setup.api_key_label") + "  " + m.presetKeyInput.View() + "\n\n")
			b.WriteString(StyleFaint.Render("  [↑↓] "+i18n.T("firstrun.toggle_field")+
				"  [←→] "+i18n.T("firstrun.toggle_region")+
				"  [Ctrl+E] editor (allows pasting)"+
				"  [Enter] "+i18n.T("setup.save")+
				"  [Esc] "+i18n.T("setup.back")) + "\n")
		} else {
			b.WriteString("  " + i18n.T("setup.api_key_label") + " " + m.presetKeyInput.View() + "\n\n")
			b.WriteString(StyleFaint.Render("  [Enter] "+i18n.T("setup.save")+
				"  [Ctrl+E] editor (allows pasting)"+
				"  [Esc] "+i18n.T("setup.back")) + "\n")
		}

	case stepCapabilities:
		stepNum, total := stepProgress(m.step, m.hasPresets, m.setupMode)
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

		// Build the left block (grid + addon list) into a separate builder so
		// that in wide mode we can join it horizontally with a side pane.
		var leftBlock strings.Builder
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
				isCurrent := idx == m.capCursor && !m.inAddonZone
				compatProvs := m.compatibleProviders(info, provider)

				if compat || local {
					if m.capSelected[name] {
						checkbox = "[✓]"
					} else {
						checkbox = "[ ]"
					}
					if !compat && local {
						hint = "(local)"
					} else if len(compatProvs) >= 2 {
						// Show active provider when there's a choice
						if prov := m.capProviders[name]; prov != "" {
							hint = "(" + prov + ")"
						}
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
			leftBlock.WriteString(line + "\n")
		}

		// Addon section
		leftBlock.WriteString("\n  " + StyleAccent.Render(i18n.T("firstrun.addons_section")) + "\n\n")
		for i, name := range m.addonOrder {
			var checkbox string
			if m.addonSelected[name] {
				checkbox = "[✓]"
			} else {
				checkbox = "[ ]"
			}
			prefix := "  "
			isCurrent := m.inAddonZone && i == m.addonCursor
			if isCurrent {
				cell := "> " + checkbox + " " + name
				leftBlock.WriteString(cursorStyle.Render(cell) + "\n")
			} else {
				leftBlock.WriteString(prefix + checkbox + " " + name + "\n")
			}
		}

		// Wide-mode layout: grid/addons on the left, description side pane on
		// the right. Threshold is capsWidePaneThreshold columns. Below it, we
		// fall back to the narrow layout (description collapses to one line).
		wide := m.width >= capsWidePaneThreshold
		if wide {
			// Left column fixed at 2 * cellWidth(38) + margin, right column fills
			// the rest up to a comfortable reading width.
			leftWidth := 80
			paneWidth := m.width - leftWidth - 4
			if paneWidth > 60 {
				paneWidth = 60
			}
			if paneWidth < 30 {
				// Not actually enough room for a useful pane — fall back to narrow.
				wide = false
			} else {
				pane := m.renderCapsSidePane(paneWidth)
				// Indent the pane by 2 spaces for visual separation from the grid.
				paneIndented := "  " + strings.ReplaceAll(pane, "\n", "\n  ")
				combined := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock.String(), paneIndented)
				b.WriteString(combined + "\n")
			}
		}
		if !wide {
			b.WriteString(leftBlock.String())
			// Narrow mode: show the one-line summary + active provider for the
			// focused item, in the spot where caps_recommend used to live.
			focusName, desc := m.focusedItemDesc()
			summary := descSummaryLine(desc)
			if summary != "" {
				provHint := ""
				if !m.inAddonZone {
					info := m.capInfos[focusName]
					compatProvs := m.compatibleProviders(info, provider)
					if prov := m.capProviders[focusName]; prov != "" && len(compatProvs) >= 2 {
						provHint = StyleFaint.Render(" ["+prov+"]") + StyleFaint.Render(" tab "+i18n.T("firstrun.cap_provider_cycle"))
					}
				}
				b.WriteString("\n  " + StyleAccent.Render("▸ ") + summary + provHint + "\n")
			}
		}

		// Footer. In narrow mode we keep the recommend/change-later guidance
		// right above the key hints. In wide mode the side pane carries the
		// per-item detail, so we fold recommend + change-later into a single
		// compact line above the key hints.
		if wide {
			b.WriteString("\n  " + StyleFaint.Render(i18n.T("firstrun.caps_recommend")+"  "+i18n.T("firstrun.caps_change_later")) + "\n")
		} else {
			b.WriteString("\n  " + StyleAccent.Render(i18n.T("firstrun.caps_recommend")) + "\n")
			b.WriteString("  " + StyleFaint.Render(i18n.T("firstrun.caps_change_later")) + "\n")
		}
		b.WriteString("\n" + StyleFaint.Render("  ↑↓←→ "+i18n.T("settings.select")+
			"  space "+i18n.T("settings.change")+
			"  tab "+i18n.T("firstrun.cap_provider_cycle")+
			"  Ctrl+A "+i18n.T("firstrun.caps_toggle_all")+
			"  [Enter] "+i18n.T("firstrun.confirm_caps")+
			"  [Esc] "+i18n.T("firstrun.back")) + "\n")

	case stepAgentNameDir:
		stepNum, total := stepProgress(m.step, m.hasPresets, m.setupMode)
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
		if m.setupMode {
			b.WriteString("  " + i18n.T("firstrun.agent_dir") + ": " + StyleFaint.Render(m.setupOrchDir) + "\n")
		} else {
			b.WriteString(cur(1) + i18n.T("firstrun.agent_dir") + ": " + m.dirInput.View() + "\n")
		}
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
		enterLabel := i18n.T("firstrun.create_agent")
		if m.setupMode {
			enterLabel = i18n.T("setup.save")
		}
		b.WriteString("\n" + StyleFaint.Render("  ↑↓ "+i18n.T("firstrun.toggle_field")+
			"  ←→ "+i18n.T("firstrun.toggle_region")+
			"  [Enter] "+enterLabel+
			"  [Esc] "+i18n.T("firstrun.back")) + "\n")

	case stepRecipe:
		if m.recipePreview {
			return m.viewRecipePreview()
		}
		return m.viewRecipe()

	case stepRecipeSwapConfirm:
		return m.viewRecipeSwapConfirm()

	case stepPropagate:
		// Rehydration: we've written the orchestrator's init.json and are
		// propagating it to the worker agents via preset.RehydrateNetwork.
		// The propagation is fast (few file reads/writes per agent) so the
		// user usually sees this for a beat, then the result line, then
		// presses Enter to advance.
		b.WriteString("\n  " + StyleTitle.Render("Propagating config to worker agents") + "\n\n")
		if m.rehydrateErr != "" {
			b.WriteString("  ✗ rehydration failed: " + m.rehydrateErr + "\n\n")
			b.WriteString(StyleFaint.Render("  Press Enter to exit. Run `lingtai-tui clean` and try again.") + "\n")
		} else if m.rehydrateWorkers > 0 {
			b.WriteString(fmt.Sprintf("  ✓ rehydrated %d worker agent(s)\n\n", m.rehydrateWorkers))
			b.WriteString(StyleFaint.Render("  Press Enter to launch the network.") + "\n")
		} else {
			b.WriteString("  Running…\n")
		}

	case stepLaunching:
		stepNum, total := stepProgress(m.step, m.hasPresets, m.setupMode)
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

	// Imported network banner (rehydration mode only)
	if m.rehydrateMode {
		bannerStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		// Count agent dirs in .lingtai/ (non-dot, non-human dirs with .agent.json)
		agentCount := 0
		if entries, err := os.ReadDir(m.baseDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "human" {
					continue
				}
				if _, err := os.Stat(filepath.Join(m.baseDir, e.Name(), ".agent.json")); err == nil {
					agentCount++
				}
			}
		}
		banner := i18n.TF("welcome.network_found", agentCount, m.rehydrateOrchName)
		content.WriteString(centerText(bannerStyle.Render(banner), m.width) + "\n\n")
	}

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
		hints = StyleFaint.Render("↑↓ " + i18n.T("welcome.select_lang") + "  [Enter] " + i18n.T("welcome.confirm") + "  [Ctrl+T] " + i18n.T("settings.theme"))
	} else {
		hints = StyleFaint.Render("↑↓ " + i18n.T("welcome.select_lang") + "  [Ctrl+T] " + i18n.T("settings.theme") + "  (" + i18n.T("welcome.installing") + ")")
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

// capsWidePaneThreshold is the terminal width at or above which the
// capabilities page splits into a left grid + right description pane.
// Below this, the description collapses to a single line under the grid.
const capsWidePaneThreshold = 110

// focusedItemDesc returns the raw i18n description for whichever item
// the cursor is currently on — a capability when inAddonZone is false,
// an addon otherwise. Returns "" if nothing is focused (shouldn't happen).
func (m FirstRunModel) focusedItemDesc() (name, desc string) {
	if m.inAddonZone {
		if m.addonCursor < 0 || m.addonCursor >= len(m.addonOrder) {
			return "", ""
		}
		name = m.addonOrder[m.addonCursor]
		return name, i18n.T("firstrun.addon_desc." + name)
	}
	if m.capCursor < 0 || m.capCursor >= len(m.capOrder) {
		return "", ""
	}
	name = m.capOrder[m.capCursor]
	return name, i18n.T("firstrun.cap_desc." + name)
}

// descSummaryLine returns the first line of a multi-line description
// (the one-sentence summary, by convention). Returns "" for an empty desc.
func descSummaryLine(desc string) string {
	if desc == "" {
		return ""
	}
	if i := strings.IndexByte(desc, '\n'); i >= 0 {
		return desc[:i]
	}
	return desc
}

// renderCapsSidePane renders the wide-mode right-hand description pane.
// It shows the currently-focused item's full description plus dynamic
// provider metadata for capabilities. Lines are hard-wrapped to paneWidth.
func (m FirstRunModel) renderCapsSidePane(paneWidth int) string {
	name, desc := m.focusedItemDesc()
	if name == "" {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	labelStyle := lipgloss.NewStyle().Foreground(ColorSubtle)

	var b strings.Builder
	b.WriteString(titleStyle.Render(name) + "\n\n")

	// Static description from i18n — may contain \n separators.
	for _, line := range strings.Split(desc, "\n") {
		for _, wrapped := range wrapLine(line, paneWidth) {
			b.WriteString(wrapped + "\n")
		}
	}

	// Dynamic capability metadata — only applies to capabilities, not addons.
	if !m.inAddonZone {
		if info, ok := m.capInfos[name]; ok && len(info.Providers) > 0 {
			b.WriteString("\n")
			presetProvider := m.getPresetProvider(m.presets[m.cursor])
			compatProvs := m.compatibleProviders(info, presetProvider)
			activeProv := m.capProviders[name]

			if len(compatProvs) >= 2 {
				// Render a provider picker: Providers: name1 · [name2] · name3
				activeStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorActive)
				b.WriteString(labelStyle.Render(i18n.T("firstrun.cap_meta_providers")) + " ")
				for i, p := range compatProvs {
					if i > 0 {
						b.WriteString(labelStyle.Render(" · "))
					}
					if p == activeProv {
						b.WriteString(activeStyle.Render("[" + p + "]"))
					} else {
						b.WriteString(p)
					}
				}
				b.WriteString("\n")
				b.WriteString(labelStyle.Render("  [tab] "+i18n.T("firstrun.cap_provider_cycle")) + "\n")
			} else if len(compatProvs) == 1 {
				b.WriteString(labelStyle.Render(i18n.T("firstrun.cap_meta_providers")) + " " + compatProvs[0] + "\n")
			}

			if info.Default != nil && *info.Default != "" {
				b.WriteString(labelStyle.Render(i18n.T("firstrun.cap_meta_default")) + " " + *info.Default + "\n")
			}
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

// wrapLine wraps a single line of text to the given width using simple
// space-based word wrapping. CJK text without spaces is returned unwrapped
// since char-boundary wrapping would corrupt multi-byte glyphs; the side
// pane is sized so this is rarely needed.
func wrapLine(s string, width int) []string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return []string{s}
	}
	// Only attempt wrapping if the line contains ASCII spaces. For CJK
	// strings without spaces we return as-is rather than risk splitting
	// a multi-byte character in half.
	if !strings.ContainsRune(s, ' ') {
		return []string{s}
	}
	var out []string
	words := strings.Split(s, " ")
	line := ""
	for _, w := range words {
		if line == "" {
			line = w
			continue
		}
		if lipgloss.Width(line)+1+lipgloss.Width(w) > width {
			out = append(out, line)
			line = w
		} else {
			line += " " + w
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// compatibleProviders returns the subset of a capability's providers that
// work with the current preset. A provider is considered usable if it
// matches the preset's LLM provider string OR if the capability has a
// non-nil default (meaning it has a free/builtin fallback like duckduckgo
// or whisper that works regardless of the LLM provider).
func (m FirstRunModel) compatibleProviders(info capInfo, presetProvider string) []string {
	if len(info.Providers) == 0 {
		return nil
	}
	var out []string
	for _, p := range info.Providers {
		if p == presetProvider {
			out = append(out, p)
		} else if info.Default != nil && p == *info.Default {
			out = append(out, p)
		}
	}
	return out
}

// initCapProviders sets the initial provider choice per capability based on
// the preset manifest. Called after check-caps completes and capInfos is populated.
func (m *FirstRunModel) initCapProviders() {
	m.capProviders = make(map[string]string)
	p := m.presets[m.cursor]
	presetProvider := m.getPresetProvider(p)
	caps, _ := p.Manifest["capabilities"].(map[string]interface{})
	for _, name := range m.capOrder {
		info := m.capInfos[name]
		compat := m.compatibleProviders(info, presetProvider)
		if len(compat) == 0 {
			continue
		}
		// If the preset already specifies a provider for this cap, use it
		// (as long as it's in the compatible set).
		if capCfg, ok := caps[name].(map[string]interface{}); ok {
			if prov, ok := capCfg["provider"].(string); ok {
				for _, c := range compat {
					if c == prov {
						m.capProviders[name] = prov
						break
					}
				}
			}
		}
		// If still unset, default to the first compatible provider.
		if m.capProviders[name] == "" {
			m.capProviders[name] = compat[0]
		}
	}
}

// enterCapabilities transitions to stepCapabilities.
func (m *FirstRunModel) enterCapabilities() tea.Cmd {
	m.step = stepCapabilities
	m.capLoading = true
	m.capErr = ""
	m.capCursor = 0
	m.capOrder = AllCapabilities
	m.capSelected = map[string]bool{"skills": true}
	m.capProviders = nil
	m.capInfos = nil
	m.addonOrder = AllAddons
	m.addonCursor = 0
	m.inAddonZone = false

	// In setup mode, preserve the addons already configured in the existing init.json.
	// Otherwise (fresh first-run), default to all available addons selected.
	if len(m.setupLoadedAddonNames) > 0 {
		m.addonSelected = map[string]bool{}
		for _, name := range m.setupLoadedAddonNames {
			m.addonSelected[name] = true
		}
	} else {
		m.addonSelected = map[string]bool{"imap": true, "telegram": true, "feishu": true}
	}

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
			capCfg := map[string]interface{}{}
			// Preserve existing config fields (e.g. api_key_env) if the preset
			// already specified them, then overlay the user's provider choice.
			if existing, ok := caps[name].(map[string]interface{}); ok {
				for k, v := range existing {
					capCfg[k] = v
				}
			}
			if prov, ok := m.capProviders[name]; ok && prov != "" {
				capCfg["provider"] = prov
			}
			caps[name] = capCfg
		} else {
			delete(caps, name)
		}
	}
}

// enterAgentNameDir initialises all fields and transitions to stepAgentNameDir.
func (m *FirstRunModel) enterAgentNameDir(p preset.Preset) {
	defaultName := p.Name
	defaultDir := p.Name
	if m.setupMode && m.setupOrchName != "" {
		defaultName = m.setupOrchName
	}
	if m.rehydrateMode {
		// Rehydration: prefill name from existing .agent.json, lock dir to
		// the existing directory. The dir input is displayed but not editable.
		if m.rehydrateOrchName != "" {
			defaultName = m.rehydrateOrchName
		}
		if m.rehydrateOrchDir != "" {
			defaultDir = m.rehydrateOrchDir
		}
	}
	m.agentName = defaultName
	m.agentDir = defaultDir
	m.nameInput.SetValue(defaultName)
	m.dirInput.SetValue(defaultDir)
	m.fieldIdx = 0
	m.nameInput.Focus()
	m.dirInput.Blur()

	// Language — inherit from preset, fallback to TUI config language
	m.agentLangIdx = 0
	presetLang, hasLang := p.Manifest["language"].(string)
	if !hasLang || presetLang == "" {
		presetLang = config.LoadTUIConfig(m.globalDir).Language
	}
	for i, lang := range []string{"en", "zh", "wen"} {
		if lang == presetLang {
			m.agentLangIdx = i
			break
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
// focusedPresetKeyInput returns a pointer to the currently focused text input
// in the preset key step, or nil if the current field is a selector (no text).
func (m *FirstRunModel) focusedPresetKeyInput() *textinput.Model {
	if m.selectedProvider == "minimax" || m.selectedProvider == "zhipu" {
		// minimax/zhipu key field (idx 2) is a textarea — handled separately
		return nil
	}
	switch m.presetKeyFieldIdx {
	case 1:
		return &m.presetEndpointIn
	case 2:
		return &m.presetModelIn
	case 3:
		// key field is a textarea — handled separately
		return nil
	case 4:
		return &m.presetNameIn
	}
	return nil
}

// focusedPresetKeyTextarea returns a pointer to presetKeyInput if it's focused.
func (m *FirstRunModel) focusedPresetKeyTextarea() *textarea.Model {
	if (m.selectedProvider == "minimax" || m.selectedProvider == "zhipu") && m.presetKeyFieldIdx == 2 {
		return &m.presetKeyInput
	}
	if m.selectedProvider != "minimax" && m.selectedProvider != "zhipu" && m.selectedProvider != "custom" {
		return &m.presetKeyInput
	}
	if m.selectedProvider == "custom" && m.presetKeyFieldIdx == 3 {
		return &m.presetKeyInput
	}
	return nil
}

func (m *FirstRunModel) focusPresetKeyField() tea.Cmd {
	m.presetEndpointIn.Blur()
	m.presetModelIn.Blur()
	m.presetKeyInput.Blur()
	m.presetNameIn.Blur()
	if m.selectedProvider == "minimax" || m.selectedProvider == "zhipu" {
		switch m.presetKeyFieldIdx {
		case 0, 1:
			return nil // region/model selector — no text focus
		case 2:
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

func (m FirstRunModel) hasImportedRecipe() bool {
	return m.importedRecipe != nil
}

func (m FirstRunModel) recipeMaxIdx() int {
	if m.hasImportedRecipe() {
		return 5
	}
	return 4
}

func (m FirstRunModel) recipeNameToIdx(name string) int {
	offset := 0
	if m.hasImportedRecipe() {
		if name == preset.RecipeImported {
			return 0
		}
		offset = 1
	}
	switch name {
	case preset.RecipeGreeter:
		return 1 + offset
	case preset.RecipePlain:
		return 2 + offset
	case preset.RecipeTutorial:
		return 3 + offset
	case preset.RecipeCustom:
		return 4 + offset
	default:
		return offset // adaptive
	}
}

func (m FirstRunModel) recipeIdxToName(idx int) string {
	if m.hasImportedRecipe() {
		if idx == 0 {
			return preset.RecipeImported
		}
		idx--
	}
	switch idx {
	case 1:
		return preset.RecipeGreeter
	case 2:
		return preset.RecipePlain
	case 3:
		return preset.RecipeTutorial
	case 4:
		return preset.RecipeCustom
	default:
		return preset.RecipeAdaptive
	}
}

func recipeChanged(oldRecipe, oldCustomDir, newRecipe, newCustomDir string) bool {
	if oldRecipe == "" {
		return false // legacy project, no current recipe
	}
	if oldRecipe != newRecipe {
		return true
	}
	if (oldRecipe == preset.RecipeCustom || oldRecipe == preset.RecipeImported) && oldCustomDir != newCustomDir {
		return true
	}
	return false
}

// viewRecipe renders the recipe picker page.
func (m FirstRunModel) viewRecipeSwapConfirm() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent)
	warnStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorSuspended)

	b.WriteString("\n  " + titleStyle.Render(i18n.T("recipe.swap_title")) + "\n\n")
	b.WriteString("  " + i18n.TF("recipe.swap_hint", m.currentRecipe, m.pendingRecipeName) + "\n\n")

	type option struct {
		label string
		desc  string
		warn  bool
	}
	opts := []option{
		{i18n.T("recipe.swap_inplace"), i18n.T("recipe.swap_inplace_desc"), false},
		{i18n.T("recipe.swap_fresh"), i18n.T("recipe.swap_fresh_desc"), true},
		{i18n.T("recipe.swap_cancel"), "", false},
	}

	for i, opt := range opts {
		cursor := "  "
		labelStyle := lipgloss.NewStyle().Foreground(ColorText)
		if i == m.swapConfirmIdx {
			cursor = "> "
			if opt.warn {
				labelStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorSuspended)
			} else {
				labelStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
			}
		} else if opt.warn {
			labelStyle = warnStyle
		}
		b.WriteString(cursor + labelStyle.Render(opt.label) + "\n")
		if opt.desc != "" {
			b.WriteString("    " + StyleFaint.Render(opt.desc) + "\n")
		}
	}

	b.WriteString("\n" + StyleFaint.Render(
		"  ↑↓ "+i18n.T("welcome.select_lang")+
			"  [Enter] "+i18n.T("welcome.confirm")+
			"  [Esc] "+i18n.T("firstrun.back")) + "\n")
	return b.String()
}

// recipeWidePaneThreshold is the terminal width at or above which the
// recipe picker shows a side pane with file previews.
const recipeWidePaneThreshold = 90

func (m FirstRunModel) viewRecipe() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent)
	b.WriteString("\n  " + titleStyle.Render(i18n.T("recipe.title")) + "\n\n")
	b.WriteString("  " + i18n.T("recipe.hint") + "\n")
	b.WriteString("\n")

	var leftBlock strings.Builder

	// Render imported recipe slot (if detected)
	if m.hasImportedRecipe() {
		importedIdx := 0
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if importedIdx == m.recipeIdx {
			cursor = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		}
		importedStyle := lipgloss.NewStyle().Foreground(ColorActive)
		leftBlock.WriteString("  " + importedStyle.Render(i18n.T("recipe.imported")) + "\n")
		leftBlock.WriteString(cursor + style.Render(m.importedRecipe.Name) + "\n")
		if m.importedRecipe.Description != "" {
			leftBlock.WriteString("    " + StyleFaint.Render(m.importedRecipe.Description) + "\n")
		}
		leftBlock.WriteString("\n  " + StyleFaint.Render("────") + "\n")
	}

	// Render bundled recipes
	recommendedStyle := lipgloss.NewStyle().Foreground(ColorAgent)
	leftBlock.WriteString("  " + recommendedStyle.Render(i18n.T("recipe.recommended")) + "\n")
	for bi, name := range preset.BundledRecipes() {
		idx := m.recipeNameToIdx(name)
		// Separator between adaptive (recommended) and the rest
		if bi == 1 {
			leftBlock.WriteString("\n  " + StyleFaint.Render(i18n.T("recipe.others")) + "\n")
		}
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if idx == m.recipeIdx {
			cursor = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		}
		label := i18n.T("recipe.name." + name)
		desc := i18n.T("recipe.desc." + name)
		leftBlock.WriteString(cursor + style.Render(label) + "\n")
		leftBlock.WriteString("    " + StyleFaint.Render(desc) + "\n")
	}

	// Render custom entry
	{
		customIdx := m.recipeMaxIdx()
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if customIdx == m.recipeIdx {
			cursor = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		}
		label := i18n.T("recipe.name." + preset.RecipeCustom)
		desc := i18n.T("recipe.desc." + preset.RecipeCustom)
		leftBlock.WriteString(cursor + style.Render(label) + "\n")
		leftBlock.WriteString("    " + StyleFaint.Render(desc) + "\n")
	}

	if m.recipeIdxToName(m.recipeIdx) == preset.RecipeCustom {
		leftBlock.WriteString("\n  " + i18n.T("recipe.custom_path") + "\n")
		leftBlock.WriteString("  " + m.recipeCustomInput.View() + "\n")
		if m.recipeCustomErr != "" {
			errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
			leftBlock.WriteString("  " + errStyle.Render(m.recipeCustomErr) + "\n")
		}
	}

	wide := m.width >= recipeWidePaneThreshold
	if wide {
		leftWidth := 50
		paneWidth := m.width - leftWidth - 6
		if paneWidth > 50 {
			paneWidth = 50
		}
		if paneWidth < 25 {
			wide = false
		} else {
			pane := m.renderRecipeSidePane(paneWidth)
			if pane != "" {
				paneIndented := "  │ " + strings.ReplaceAll(pane, "\n", "\n  │ ")
				combined := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock.String(), paneIndented)
				b.WriteString(combined + "\n")
			} else {
				wide = false
			}
		}
	}
	if !wide {
		b.WriteString(leftBlock.String())
	}

	b.WriteString("\n" + StyleFaint.Render(
		"  ↑↓ "+i18n.T("welcome.select_lang")+
			"  [Ctrl+O] "+i18n.T("recipe.preview")+
			"  [Enter] "+i18n.T("welcome.confirm")+
			"  [Esc] "+i18n.T("firstrun.back")) + "\n")
	return b.String()
}

// renderRecipeSidePane renders a preview of the greet.md and comment.md files
// for the currently selected recipe. Returns empty string if nothing to show.
func (m FirstRunModel) renderRecipeSidePane(paneWidth int) string {
	langs := []string{"en", "zh", "wen"}
	lang := langs[m.agentLangIdx]
	recipeName := m.recipeIdxToName(m.recipeIdx)

	// Resolve the recipe directory.
	var recipeDir string
	switch recipeName {
	case preset.RecipeImported:
		recipeDir = m.importedRecipeDir
	case preset.RecipeCustom:
		recipeDir = m.recipeCustomInput.Value()
		if recipeDir == "" {
			return ""
		}
	default:
		recipeDir = preset.RecipeDir(m.globalDir, recipeName)
	}

	greetPath := preset.ResolveGreetPath(recipeDir, lang)
	commentPath := preset.ResolveCommentPath(recipeDir, lang)

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorSubtle)
	const previewLines = 6

	var b strings.Builder

	// greet.md preview
	b.WriteString(labelStyle.Render("── "+i18n.T("recipe.preview_greet")+" ──") + "\n")
	b.WriteString(recipeFilePreview(greetPath, paneWidth, previewLines) + "\n\n")

	// comment.md preview
	b.WriteString(labelStyle.Render("── "+i18n.T("recipe.preview_comment")+" ──") + "\n")
	b.WriteString(recipeFilePreview(commentPath, paneWidth, previewLines) + "\n")
	return strings.TrimRight(b.String(), "\n")
}

// recipeFilePreview reads the first N lines of a file and returns them
// wrapped to paneWidth. Returns a placeholder if the file is missing or empty.
func recipeFilePreview(path string, paneWidth, maxLines int) string {
	if path == "" {
		return StyleFaint.Render(i18n.T("recipe.preview_not_found"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return StyleFaint.Render(i18n.T("recipe.preview_not_found"))
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return StyleFaint.Render(i18n.T("recipe.preview_empty"))
	}

	lines := strings.SplitN(content, "\n", maxLines+1)
	truncated := len(lines) > maxLines
	if truncated {
		lines = lines[:maxLines]
	}

	var out strings.Builder
	for _, line := range lines {
		for _, wrapped := range wrapLine(line, paneWidth) {
			out.WriteString(StyleFaint.Render(wrapped) + "\n")
		}
	}
	if truncated {
		out.WriteString(StyleFaint.Render("…") + "\n")
	}
	return strings.TrimRight(out.String(), "\n")
}

// ── Recipe preview (full-screen scrollable view) ───────────────────

// enterRecipePreview initializes the viewport and enters preview mode.
func (m *FirstRunModel) enterRecipePreview() {
	m.recipePreview = true
	m.recipePreviewFile = 0 // start with greet.md
	if !m.recipePreviewReady {
		m.recipePreviewVP = viewport.New()
		m.recipePreviewVP.SetWidth(m.width)
		m.recipePreviewVP.SetHeight(m.height - 4)
		m.recipePreviewReady = true
	}
	m.syncRecipePreviewContent()
}

// syncRecipePreviewContent updates the viewport with the current file's content.
func (m *FirstRunModel) syncRecipePreviewContent() {
	if !m.recipePreviewReady {
		return
	}
	langs := []string{"en", "zh", "wen"}
	lang := langs[m.agentLangIdx]
	recipeName := m.recipeIdxToName(m.recipeIdx)

	var recipeDir string
	switch recipeName {
	case preset.RecipeImported:
		recipeDir = m.importedRecipeDir
	case preset.RecipeCustom:
		recipeDir = m.recipeCustomInput.Value()
	default:
		recipeDir = preset.RecipeDir(m.globalDir, recipeName)
	}

	var filePath string
	if m.recipePreviewFile == 0 {
		filePath = preset.ResolveGreetPath(recipeDir, lang)
	} else {
		filePath = preset.ResolveCommentPath(recipeDir, lang)
	}

	content := m.renderRecipeFileContent(filePath)
	m.recipePreviewVP.SetContent(content)
	m.recipePreviewVP.GotoTop()
}

// renderRecipeFileContent renders a markdown file for the viewport using glamour.
func (m FirstRunModel) renderRecipeFileContent(path string) string {
	if path == "" {
		return "\n  " + StyleFaint.Render(i18n.T("recipe.preview_not_found"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "\n  " + StyleFaint.Render(i18n.T("recipe.preview_not_found"))
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "\n  " + StyleFaint.Render(i18n.T("recipe.preview_empty"))
	}

	contentWidth := m.width - 4
	if contentWidth < 40 {
		contentWidth = 40
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(ActiveTheme().GlamourStyle),
		glamour.WithWordWrap(contentWidth),
	)
	if err == nil {
		if rendered, rerr := r.Render(text); rerr == nil {
			return rendered
		}
	}

	// Fallback: plain text
	return "\n  " + lipgloss.NewStyle().Width(contentWidth).Render(text)
}

// viewRecipePreview renders the full-screen preview with header, viewport, footer.
func (m FirstRunModel) viewRecipePreview() string {
	fileLabels := []string{i18n.T("recipe.preview_greet"), i18n.T("recipe.preview_comment")}
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	unselectedStyle := StyleFaint

	// Header: tab bar for greet.md / comment.md
	var header strings.Builder
	header.WriteString("  ")
	for i, label := range fileLabels {
		if i == m.recipePreviewFile {
			header.WriteString(selectedStyle.Render("[ " + label + " ]"))
		} else {
			header.WriteString(unselectedStyle.Render("  " + label + "  "))
		}
		if i < len(fileLabels)-1 {
			header.WriteString("  ")
		}
	}

	titleBar := header.String() + "\n" + strings.Repeat("─", m.width)

	scrollHint := ""
	if m.recipePreviewReady && !m.recipePreviewVP.AtBottom() {
		scrollHint = " " + RuneBullet + " ↑↓/pgup/pgdn scroll"
	}
	footer := strings.Repeat("─", m.width) + "\n" +
		StyleFaint.Render("  [Tab] "+i18n.T("recipe.preview_switch")+
			"  [Ctrl+O] "+i18n.T("firstrun.back")+scrollHint)

	return titleBar + "\n" + m.recipePreviewVP.View() + "\n" + footer
}

// performRecipeSave executes the full save for the chosen recipe and the
// previously-stashed AgentOpts/dirName.
func (m FirstRunModel) performRecipeSave(recipeName, customDir string) (FirstRunModel, tea.Cmd) {
	lang := m.pendingAgentOpts.Language
	if lang == "" {
		lang = "en"
	}

	// Resolve comment, covenant, and procedures paths from recipe
	commentPath := resolveRecipeComment(m.globalDir, recipeName, customDir, lang)
	covenantPath := resolveRecipeCovenant(m.globalDir, recipeName, customDir, lang)
	proceduresPath := resolveRecipeProcedures(m.globalDir, recipeName, customDir, lang)
	opts := m.pendingAgentOpts
	opts.CommentFile = commentPath
	if covenantPath != "" {
		opts.CovenantFile = covenantPath
	}
	if proceduresPath != "" {
		opts.ProceduresFile = proceduresPath
	}

	p := m.presets[m.cursor]
	dirName := m.pendingDirName

	// Write init.json
	if err := preset.GenerateInitJSONWithOpts(p, m.agentName, dirName, m.baseDir, m.globalDir, opts); err != nil {
		m.message = i18n.TF("firstrun.error", err)
		m.step = stepAgentNameDir
		return m, nil
	}

	// Apply recipe: write .prompt + .tui-asset/.recipe
	orchDir := filepath.Join(m.baseDir, dirName)
	humanDir := filepath.Join(m.baseDir, "human")
	humanAddr := "human"
	if humanNode, err := fs.ReadAgent(humanDir); err == nil && humanNode.Address != "" {
		humanAddr = humanNode.Address
	}
	soulDelayStr := fmt.Sprintf("%v", opts.SoulDelay)
	if err := applyRecipe(
		m.baseDir, orchDir, m.globalDir, humanDir, humanAddr,
		recipeName, customDir, lang, soulDelayStr,
	); err != nil {
		m.message = i18n.TF("firstrun.error", err)
		m.step = stepAgentNameDir
		return m, nil
	}

	if m.setupMode {
		return m, func() tea.Msg { return SetupSavedMsg{} }
	}
	if m.rehydrateMode {
		m.step = stepPropagate
		m.message = ""
		return m, m.runRehydratePropagation()
	}
	m.step = stepLaunching
	m.message = i18n.TF("firstrun.created", m.agentName)
	return m, func() tea.Msg {
		return FirstRunDoneMsg{OrchDir: orchDir, OrchName: m.agentName}
	}
}
