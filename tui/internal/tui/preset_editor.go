package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/preset"
)

// PresetEditorCommitMsg fires when the editor's working copy passes validation
// and the user completes a save action (normal save, clone-name Enter, or the
// Ctrl+E expert overwrite). Hosts (firstrun, /setup, library) decide what to do
// next — typically: persist via preset.Save, then advance their own state. The
// editor itself does NOT save to disk.
// Preset.Source is always SourceSaved so RefFor names the saved/ file those
// hosts create, including an expert overwrite that keeps a built-in name.
//
// APIKey carries the new key value the user typed in the editor, when
// they actually changed it. Empty means "unchanged — keep whatever's
// already in ~/.lingtai-tui/.env". The host writes this into Config.Keys
// using the preset's api_key_env name as the key.
type PresetEditorCommitMsg struct {
	Preset    preset.Preset
	APIKey    string
	APIKeySet bool   // true when the user typed/changed a value in this session
	Warning   string // localized, sanitized availability warning; never a credential
}

// PresetEditorCancelMsg fires on Esc (and after the dirty-prompt
// confirms discard). Hosts return to whichever screen they came from.
type PresetEditorCancelMsg struct{}

// editorField identifies a row in the form.
type editorField int

const (
	feName editorField = iota
	feSummary
	feTier
	feGains
	feLoses
	feProvider
	feModel
	feServiceTier
	feThinking
	feAPICompat
	feWireAPI
	feResponsesTransport
	feBaseURL
	feAPIKey
	feStreaming
	feKarma
	feNirvana
	feSave
)

// editorFieldOrder is the rendering order of fields. The cursor walks
// this slice; section headers render between transitions. Capabilities
// are not cursor-navigable or editable fields — every capability the
// kernel can grant an agent is always included, and the Capabilities
// section renders as a fixed informational list below (see formRows'
// capabilityRows), not as form rows.
var editorFieldOrder = []editorField{
	feName, feSummary, feTier, feGains, feLoses,
	feProvider, feModel, feServiceTier, feThinking, feAPICompat, feWireAPI, feResponsesTransport, feBaseURL, feAPIKey,
	feSave,
}

// saveFieldIndex is the cursor position of the [Save] button row. Tab
// jumps here from anywhere in the form so paste-and-save is two
// keystrokes away regardless of which field the user is editing.
var saveFieldIndex = len(editorFieldOrder) - 1

type editorMode int

const (
	emBrowse      editorMode = iota // navigating field list
	emInline                        // textinput active for the focused field
	emClonePrompt                   // built-in: prompt for new name on semantic edit
	emDirtyPrompt                   // legacy "discard? y/N" — kept for compat
	emExitPrompt                    // three-way exit on Esc: save / discard / cancel
)

// providerModels maps a provider name to the canonical native/default-route
// model lineup the editor cycles through with ←/→ on the model row. Exact
// route catalogs that differ from this default live in routeModelOverrides
// below; providers and routes without a matching catalog remain free text.
//
// Keep this in sync with each provider's official model list. When a
// new flagship ships, add it (and remove deprecated entries — agents
// will hit 4xx if they pick a retired model).
//
// CURATION RULE (tui/CONTRACT.md, "Model list curation"): every family
// listed here ships only its LATEST TWO GENERATIONS. A third-newest
// generation is removed in the same change that adds a new one. Variants
// within one generation (-highspeed, -pro, -mini, -sol/-terra/-luna) are not
// separate generations and all stay. See tui/internal/tui/SKILL.md for the
// per-provider source list and the rest of the inclusion checklist.
var providerModels = map[string][]string{
	// MiniMax native CN: the latest two documented native text generations,
	// newest first. OpenCode Go keeps its pre-PR catalog in the exact route
	// override below; INTL remains free text until parity is proven.
	"minimax": {
		"MiniMax-M2.7", "MiniMax-M2.7-highspeed",
		"MiniMax-M2.5", "MiniMax-M2.5-highspeed",
	},
	// Zhipu — two mutually exclusive id sets in one cycle, because the model
	// row is not coupled to the selected base_url row (known debt):
	//   * UPPERCASE: the native CN/INTL catalog names.
	//   * lowercase: the same generations as OpenCode Go serves them; an
	//     uppercase id there is rejected with "Model GLM-5.2 is not supported".
	// Latest two generations only, both spellings: GLM-5.2 and GLM-5.1.
	// GLM-5-Turbo/GLM-4.7/GLM-4.5-Air are older generations and dropped;
	// there is no `glm-5`, so no lowercase alias exists for one.
	"zhipu": {
		// native CN/INTL
		"GLM-5.2", "GLM-5.1",
		// OpenCode Go
		"glm-5.2", "glm-5.1",
	},
	// Kimi has no provider-global entry: its exact native Coding Plan route
	// gets a picker from routeModelOverrides, while OpenCode Go and Custom stay
	// free text so their distinct spellings and user values remain editable.
	//
	// MiMo native: V2.5 and its text-only Pro variant. Deprecated V2 entries
	// are retained only in the protected pre-PR OpenCode Go override below.
	"mimo":     {"mimo-v2.5", "mimo-v2.5-pro"},
	"deepseek": {"deepseek-v4-pro", "deepseek-v4-flash"},
	// Grok (xAI) via OpenCode Go — the Go /models list serves grok-4.5 and
	// nothing older that we have verified.
	"grok": {"grok-4.5"},
	// NVIDIA uses a bounded snapshot of stable model IDs verified as served by
	// the configured route. It is not a universal NVIDIA generation ladder;
	// keep the default flagship first and update the snapshot only from fresh
	// route evidence.
	"nvidia": {
		"nvidia/nemotron-3-ultra-550b-a55b",
		"nvidia/nemotron-3-super-120b-a12b",
		"nvidia/nemotron-3-nano-omni-30b-a3b-reasoning",
		"deepseek-ai/deepseek-v4-pro-0813",
		"deepseek-ai/deepseek-v4-flash-0731",
		"moonshotai/kimi-k3",
		"minimaxai/minimax-m3",
		"mistralai/mistral-nemotron",
		"openai/gpt-oss-20b",
		"nvidia/llama-3.1-nemotron-ultra-253b-v1",
	},
	// Codex: ChatGPT-OAuth-only models served by chatgpt.com/backend-api/codex.
	// Keep gpt-5.6-sol first to match the TUI default; the other named GPT-5.6
	// routes remain selectable when the endpoint/account
	// enables them. See SKILL.md next to this file for the canonical source list
	// and why each model is included or excluded (e.g. pro-only variants can 4xx).
	//
	// GPT-6 Astra is documented but not proven available on every authenticated
	// OAuth route, so keep the proven gpt-5.6-sol default first. The named
	// GPT-5.6 routes are variants of one generation; gpt-5.5 is retired from
	// this latest-two curation. Saved presets are never rewritten.
	"codex": {"gpt-5.6-sol", "gpt-6-astra", "gpt-5.6-terra", "gpt-5.6-luna"},
	// codex-pool serves the same ChatGPT-OAuth models as codex — it only
	// changes which token file each request routes through (the pool), not the
	// model catalog. Keep the two lists identical.
	"codex-pool": {"gpt-5.6-sol", "gpt-6-astra", "gpt-5.6-terra", "gpt-5.6-luna"},
	// Claude Code uses CLI aliases, not dated API IDs — `opus`/`fable`/
	// `sonnet`/`haiku` name concurrent tiers of one generation, so the
	// two-generation rule has nothing to trim here. Current Claude Code
	// resolves fable to claude-fable-5-1. Keep the old provider spellings only
	// so user-saved presets remain editable after the built-in moves to
	// canonical provider "claude-code".
	"claude-code":      {"opus", "fable", "sonnet", "haiku"},
	"claude_code":      {"opus", "fable", "sonnet", "haiku"},
	"claude-agent-sdk": {"opus", "fable", "sonnet", "haiku"},
	"claude_agent_sdk": {"opus", "fable", "sonnet", "haiku"},
}

// routeModelOverrides contains the few exact route catalogs needed to keep
// native curation separate from the protected OpenCode Go behavior. A
// provider with an override returns no catalog for an unlisted route, which
// deliberately leaves that route free text.
var routeModelOverrides = map[string]map[string][]string{
	"minimax": {
		preset.ProviderRegionURLs["minimax"][0].URL: {"MiniMax-M2.7", "MiniMax-M2.7-highspeed", "MiniMax-M2.5", "MiniMax-M2.5-highspeed"},
		preset.ProviderRegionURLs["minimax"][2].URL: {"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed"},
	},
	"mimo": {
		preset.ProviderRegionURLs["mimo"][0].URL: {"mimo-v2.5", "mimo-v2.5-pro"},
		preset.ProviderRegionURLs["mimo"][1].URL: {"mimo-v2.5", "mimo-v2.5-pro", "mimo-v2-pro", "mimo-v2-omni"},
	},
	"kimi": {
		preset.ProviderRegionURLs["kimi"][0].URL: {"k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"},
	},
}

// modelOptions is the single catalog lookup used by every model picker
// surface. Providers without route overrides use their canonical map; a
// provider with overrides returns only the exact route entry.
func modelOptions(provider, baseURL string) []string {
	if routes, ok := routeModelOverrides[provider]; ok {
		return routes[baseURL]
	}
	return providerModels[provider]
}

// reconcileModelForRoute keeps a known curated model valid when the user moves
// between exact routes without overwriting arbitrary free text. A protected Go
// picker receives its existing first entry when a model curated only for a
// different route cannot run there. Kimi's Go route deliberately remains free
// text, so a known native Kimi id is cleared and the existing non-empty-model
// save validation requires the user to enter an explicit Go id.
func reconcileModelForRoute(provider, baseURL, model string) string {
	contains := func(models []string, candidate string) bool {
		for _, option := range models {
			if option == candidate {
				return true
			}
		}
		return false
	}

	destination := modelOptions(provider, baseURL)
	if contains(destination, model) {
		return model
	}
	routes, ok := routeModelOverrides[provider]
	if !ok {
		return model
	}
	known := false
	for _, models := range routes {
		if contains(models, model) {
			known = true
			break
		}
	}
	if !known {
		return model
	}
	if len(destination) > 0 {
		return destination[0]
	}
	if provider == "kimi" {
		for _, region := range preset.ProviderRegionURLs[provider] {
			if region.Label == "OpenCode Go" && region.URL == baseURL {
				return ""
			}
		}
	}
	return model
}

var codexServiceTierOptions = []string{"normal", "fast"}

var codexThinkingOptions = []string{"low", "medium", "high", "xhigh"}

// customResponsesThinkingOptions is the reasoning-effort ladder every
// non-Codex thinking-capable provider offers: the kernel's canonical
// THINKING_LEVELS tuple (lingtai/kernel/config.py) plus a leading
// "default" pseudo-option. "default" is NOT a payload value — the kernel
// treats an omitted manifest.llm.thinking as its own default, so selecting
// it deletes the key. Anthropic maps these levels to a thinking budget;
// OpenAI-compatible providers pass them through as Responses
// reasoning.effort.
var customResponsesThinkingOptions = []string{"default", "none", "minimal", "low", "medium", "high", "xhigh"}

var wireAPIOptions = []string{"auto", "chat_completions", "responses"}

var responsesTransportOptions = []string{"http", "websocket"}

const presetEditorFieldLabelWidth = 18

// modelHasVision documents which cataloged models actually accept image
// input. The editor no longer uses this to gate or auto-toggle the vision
// capability — vision is always included in the manifest like every other
// capability — so this map is reference data for tests only, kept because
// per-model vision support is still a real fact worth asserting against
// regressions in providerModels/the model catalog above.
//
// One entry per id in providerModels — a shipped native/default-route model
// with no entry here reads as text-only via Go's zero value, which is an
// omission, not a declaration. Route-only overrides intentionally do not add
// global capability claims: OpenCode Go modality is not inferred from native
// route metadata. Ids retired by the two-generation curation rule are removed
// from both maps together (tui/internal/tui/SKILL.md, "When you remove a
// retired model").
var modelHasVision = map[string]bool{
	// MiniMax native CN's reviewed Anthropic text route rejects image/document
	// input, so the curated native IDs are explicitly text-only. The protected
	// OpenCode Go IDs have no inherited native modality claim.
	"MiniMax-M2.7":           false,
	"MiniMax-M2.7-highspeed": false,
	"MiniMax-M2.5":           false,
	"MiniMax-M2.5-highspeed": false,
	// Zhipu coding-plan LLMs are text-only, in both the uppercase native
	// spelling and the lowercase OpenCode Go spelling of the same model.
	// Vision uses the separate GLM-4.6V model through the optional,
	// manually registered official MCP server.
	"GLM-5.2": false,
	"GLM-5.1": false,
	"glm-5.2": false,
	"glm-5.1": false,
	// MiMo: only native mimo-v2.5 has verified LingTai-side vision. The
	// native Pro sibling is text-only; the protected Go list is not granted
	// any additional modality claim.
	"mimo-v2.5":     true,
	"mimo-v2.5-pro": false,
	// DeepSeek: text-only across the board.
	"deepseek-v4-pro":   false,
	"deepseek-v4-flash": false,
	// Grok via OpenCode Go: the endpoint's image-input mapping for grok-4.5
	// is unverified, so this is a declared false, not an unknown. A model
	// name is never evidence of a wired vision route.
	"grok-4.5": false,
	// Codex (ChatGPT OAuth): official model documentation records image input
	// for the named routes, but this metadata does not assert account-specific
	// OAuth availability. Astra therefore remains picker-only until that gate
	// is satisfied for a given account.
	"gpt-6-astra":   true,
	"gpt-5.6-sol":   true,
	"gpt-5.6-terra": true,
	"gpt-5.6-luna":  true,
}

// PresetEditorModel is a single-page preset editor. Hosted by the
// firstrun/setup wizard and the library screen via embedding.
type PresetEditorModel struct {
	original preset.Preset // pristine copy for dirty diff + cancel
	working  preset.Preset // mutates as user edits

	// isBuiltin is set by the host. When true, semantic edits (llm.*
	// or capabilities.*) trigger a clone-first prompt on save so the
	// upstream built-in stays pristine and TUI upgrades can refresh it.
	isBuiltin bool

	cursor int // index into editorFieldOrder
	mode   editorMode

	// Inline textarea, reused for whichever field is being edited.
	// Textarea (not textinput) so paste from the system clipboard works
	// reliably — Bubble Tea's textinput drops characters on multi-byte
	// pastes. The editor intercepts Enter at the page level (see
	// updateInline) so multi-line behavior never surfaces.
	input textarea.Model

	// cloneNameInput captures the new preset name during the clone-first
	// prompt overlay.
	cloneNameInput textinput.Model

	// Display
	width, height int
	lang          string // "en"/"zh"/"wen" — drives tier label rendering
	scrollOffset  int    // first rendered form row; keeps focused field visible in short terminals

	// showJSON controls whether the right-hand JSON preview pane renders.
	// Hidden by default — the form is the source of truth and the JSON
	// dump usually just adds noise. Toggle with Ctrl+D for raw inspection.
	showJSON bool

	// savedCursor remembers where Tab jumped from so Shift+Tab can
	// return there. -1 when Tab hasn't been used (Shift+Tab is then a
	// no-op).
	savedCursor int

	// globalDir is ~/.lingtai-tui — the directory codex-auth.json lives
	// in. Passed by hosts so the editor can write the OAuth token bundle
	// when the user authenticates a codex preset's API-key row. May be
	// empty when no global dir is available (tests); in that case the
	// codex-OAuth branch falls back to inline edit.
	globalDir string

	// API key state. existingKeys is the host's Config.Keys snapshot
	// (env-var-name → value), used to prefill the api_key field when a
	// matching env var is already populated. apiKey is the live edit
	// buffer; apiKeySet flips true only when the user explicitly edits
	// the row (so an untouched masked key remains unchanged on commit,
	// while a pasted replacement is written by the host).
	existingKeys map[string]string
	apiKey       string
	apiKeySet    bool

	// regionEnvBeforeAdopt remembers the api_key_env the preset carried
	// just before a base_url cycle landed on a region row that declares
	// its own credential (OpenCode Go -> OPENCODE_GO_API_KEY). Cycling
	// back off that row restores it, so adopting a region credential is
	// a reversible move rather than a one-way door: the zhipu/minimax
	// CN/INTL rows declare no Env of their own, so without this memo one
	// extra → press would wrap to CN while still resolving through
	// OPENCODE_GO_API_KEY and destroy the user's ZHIPU_INTL_1_API_KEY.
	// Empty means "nothing to restore" (fall back to ProviderDefaultEnv).
	regionEnvBeforeAdopt string

	// Status
	saveErr string

	// statusMsg is a transient, non-error footer message (e.g. "Imported
	// Codex CLI credential"). It replaces the browse hint until the next
	// keypress; renderFooter prefers it over saveErr-free hints.
	statusMsg string
}

// NewPresetEditorModel builds an editor against a working copy of `p`.
// The model never mutates `p`; the host receives the modified version
// via PresetEditorCommitMsg. isBuiltin gates the clone-first prompt on
// semantic edits — derived from IsTemplate(p), which uses the preset's
// on-disk Source rather than its name (so a user-saved preset whose
// name happens to match a template is correctly treated as editable).
//
// existingKeys is Config.Keys (env-var-name → value). For user-owned
// presets, the editor uses it to display an already-saved key as masked.
// Templates intentionally start with a blank key buffer so creating a new
// preset never inherits the provider's old shared env slot by accident.
func NewPresetEditorModel(p preset.Preset, lang string, existingKeys map[string]string, globalDir string) PresetEditorModel {
	return NewPresetEditorModelWithBuiltinFlag(p, lang, existingKeys, globalDir, preset.IsTemplate(p))
}

// NewPresetEditorModelWithBuiltinFlag is the explicit-flag variant for
// callers that want to override built-in protection (e.g. tests, or
// a future "fork built-in" flow that has already cloned upstream).
func NewPresetEditorModelWithBuiltinFlag(p preset.Preset, lang string, existingKeys map[string]string, globalDir string, isBuiltin bool) PresetEditorModel {
	// Normalize legacy capability aliases before cloning so the form and its
	// eventual write path expose only the canonical shell key. Conflicts are
	// retained for Validate/commit to reject, but the error is visible now.
	normalizationErr := ""
	if err := p.NormalizeLegacyCapabilities(); err != nil {
		normalizationErr = err.Error()
	}
	// Inline editor uses textarea — paste from the system clipboard
	// works reliably (textinput drops chars on multi-byte pastes).
	// We render only one row; updateInline intercepts Enter and the
	// keymap's InsertNewline binding is cleared, so multi-line
	// semantics never surface. Styles match the rest of the TUI
	// (themedTextareaStyles); the default textarea ships with dark
	// focus colors that clash with the lipgloss palette.
	ta := textarea.New()
	ta.CharLimit = 512
	ta.SetWidth(50)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.KeyMap.InsertNewline.SetKeys() // no newlines — single line
	ta.SetStyles(themedTextareaStyles())
	cn := textinput.New()
	cn.CharLimit = 64
	cn.SetWidth(30)
	// For saved/user-owned presets, prefill the api_key buffer if the
	// declared env slot already holds a value; this lets the row render as
	// masked and preserves the key when untouched. For templates, keep the
	// buffer empty: editing a template creates a new preset, and that new
	// preset must not silently inherit an old provider-wide key.
	apiKey := ""
	if !isBuiltin {
		if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
			if envName, _ := llm["api_key_env"].(string); envName != "" {
				apiKey = existingKeys[envName]
			}
		}
	}
	return PresetEditorModel{
		original:       clonePresetForEditor(p),
		working:        clonePresetForEditor(p),
		isBuiltin:      isBuiltin,
		cursor:         0,
		savedCursor:    -1,
		mode:           emBrowse,
		input:          ta,
		cloneNameInput: cn,
		lang:           lang,
		existingKeys:   existingKeys,
		globalDir:      globalDir,
		apiKey:         apiKey,
		saveErr:        normalizationErr,
	}
}

func (m PresetEditorModel) Init() tea.Cmd { return nil }

func (m PresetEditorModel) Update(msg tea.Msg) (PresetEditorModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureFocusedVisible()
		return m, nil

	case tea.MouseWheelMsg:
		if m.mode == emBrowse {
			mouse := msg.Mouse()
			switch mouse.Button {
			case tea.MouseWheelUp:
				m.moveCursor(-3)
			case tea.MouseWheelDown:
				m.moveCursor(3)
			}
			return m, nil
		}

	case tea.KeyMsg:
		switch m.mode {
		case emInline:
			return m.updateInline(msg)
		case emClonePrompt:
			return m.updateClonePrompt(msg)
		case emDirtyPrompt:
			return m.updateDirtyPrompt(msg)
		case emExitPrompt:
			return m.updateExitPrompt(msg)
		default:
			return m.updateBrowse(msg)
		}
	}
	// Forward non-KeyMsg events (notably tea.PasteMsg from bracketed-paste
	// mode) to the active text widget. Without this, pasting into the
	// inline editor or the clone-name overlay silently drops the blob —
	// bubbletea v2 delivers paste as a separate msg type, not a KeyMsg.
	switch m.mode {
	case emInline:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case emClonePrompt:
		var cmd tea.Cmd
		m.cloneNameInput, cmd = m.cloneNameInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ───────────────────────────────────────────────────────────────────────────
// Update — browse mode (cursor over field rows)
// ───────────────────────────────────────────────────────────────────────────

func (m PresetEditorModel) updateBrowse(msg tea.KeyMsg) (PresetEditorModel, tea.Cmd) {
	// Transient footer message: visible for the frame that set it, cleared by
	// the next keypress (the import handler below re-sets it after this line).
	m.statusMsg = ""
	switch msg.String() {
	case "esc":
		// Clean editor (no edits made) → close immediately. Confirming
		// an exit when there's nothing to lose is the source of the
		// "I just glanced at this and Esc trapped me" complaint.
		// Dirty editor → show the three-way prompt so the user picks
		// save / discard / cancel intentionally.
		if !m.hasSemanticEdits() && !m.apiKeySet {
			return m, func() tea.Msg { return PresetEditorCancelMsg{} }
		}
		m.mode = emExitPrompt
		return m, nil
	case "up", "k":
		m.moveCursor(-1)
		return m, nil
	case "down", "j":
		m.moveCursor(1)
		return m, nil
	case "pgup":
		m.moveCursor(-m.visibleFormRows())
		return m, nil
	case "pgdown":
		m.moveCursor(m.visibleFormRows())
		return m, nil
	case "home":
		m.cursor = 0
		m.ensureFocusedVisible()
		return m, nil
	case "end":
		m.cursor = saveFieldIndex
		m.ensureFocusedVisible()
		return m, nil
	case "left", "h":
		// Cycle backwards on enum fields.
		m.cycleFocused(-1)
		return m, nil
	case "right", "l":
		m.cycleFocused(+1)
		return m, nil
	case "tab":
		// Jump straight to the Save button. Press Enter there to
		// commit (or Tab again to cycle back to the previous field).
		// Shift+Tab returns to the previously-focused field.
		m.savedCursor = m.cursor
		m.cursor = saveFieldIndex
		m.ensureFocusedVisible()
		return m, nil
	case "shift+tab":
		// Restore the cursor to wherever Tab jumped from. If we
		// haven't tabbed-to-save yet, no-op.
		if m.cursor == saveFieldIndex && m.savedCursor >= 0 && m.savedCursor < len(editorFieldOrder) {
			m.cursor = m.savedCursor
			m.ensureFocusedVisible()
		}
		return m, nil
	case "enter":
		return m.openInline()
	case "ctrl+s":
		return m.commit()
	case "ctrl+d":
		// Toggle the JSON preview pane. Raw inspection for power users
		// who want to see the on-disk shape; hidden by default to keep
		// the form uncluttered.
		m.showJSON = !m.showJSON
		return m, nil
	case "i":
		// One-click import of the Codex CLI's own credential (`codex
		// login` writes ~/.codex/auth.json) into the TUI store, bound to
		// this preset's API-key row. Only meaningful on the codex
		// API-key row while the bound account is invalid (the footer
		// hint codex.import_cli_hint advertises it only when a valid CLI
		// file exists). Errors — no CLI credential, already imported,
		// target exists — surface in the footer via saveErr.
		f := editorFieldOrder[m.cursor]
		if f == feAPIKey && preset.ClassifyCredentialFamily(asString(m.llmMap()["provider"])) == preset.CredentialFamilyCodexSingle && m.globalDir != "" {
			if _, valid := m.codexBoundAccountLabel(); !valid {
				ref, label, err := importCodexCLIAuth(m.globalDir)
				if err != nil {
					m.saveErr = err.Error()
				} else {
					m.setCodexAuthRef(ref)
					m.statusMsg = fmt.Sprintf(i18n.T("codex.import_cli_done"), label)
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// updateInline routes keys to the active textinput. Enter commits the
// edit into the working copy; Esc abandons the edit.
func (m PresetEditorModel) updateInline(msg tea.KeyMsg) (PresetEditorModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = emBrowse
		m.input.Blur()
		return m, nil
	case "enter":
		m.applyInline(m.input.Value())
		m.mode = emBrowse
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m PresetEditorModel) updateDirtyPrompt(msg tea.KeyMsg) (PresetEditorModel, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		return m, func() tea.Msg { return PresetEditorCancelMsg{} }
	default:
		// Anything else returns to browse without discarding.
		m.mode = emBrowse
		return m, nil
	}
}

// updateExitPrompt is the three-way "save / discard / cancel" overlay
// triggered by Esc when the editor has unsaved changes. Enter (the
// visible default) and `s` save and exit. `d` discards changes and
// exits. Esc and `c` cancel back to browse.
//
// The mapping deliberately makes Esc safe: a user who hit Esc by
// mistake and double-presses won't accidentally discard their edits.
// The destructive choice (discard) requires the explicit `d` key.
func (m PresetEditorModel) updateExitPrompt(msg tea.KeyMsg) (PresetEditorModel, tea.Cmd) {
	switch msg.String() {
	case "enter", "s", "S":
		m.mode = emBrowse
		updated, cmd := m.commit()
		return updated, cmd
	case "d", "D":
		return m, func() tea.Msg { return PresetEditorCancelMsg{} }
	default:
		// esc/c/n/anything else → return to browse, no exit.
		m.mode = emBrowse
		return m, nil
	}
}

// ───────────────────────────────────────────────────────────────────────────
// Field-level mutation
// ───────────────────────────────────────────────────────────────────────────

func (m *PresetEditorModel) openInline() (PresetEditorModel, tea.Cmd) {
	f := editorFieldOrder[m.cursor]
	switch f {
	case feName, feSummary, feGains, feLoses:
		m.input.SetValue(m.fieldString(f))
		m.input.CursorEnd()
		m.input.Focus()
		m.mode = emInline
	case feBaseURL:
		// Providers with regional endpoints use ←/→ cycling; Enter is a no-op
		// only while the current value is one of the known non-empty region
		// URLs. The Custom option (empty-URL sentinel) and any free-typed URL
		// open free-text inline edit instead. The selection rule is shared with
		// cycleFocused and baseURLRadioStrip via selectedRegionIndex.
		//
		// selectedRegionIndex returns -1 for an off-list value on a provider
		// with no Custom row; that opens inline edit too, which is the only
		// way to correct such a value from the editor.
		provider := asString(m.llmMap()["provider"])
		current := asString(m.llmMap()["base_url"])
		regions, hasRegions := preset.ProviderRegionURLs[provider]
		if hasRegions && len(regions) > 0 {
			if idx := selectedRegionIndex(regions, current); idx >= 0 && regions[idx].URL != "" {
				return *m, nil
			}
		}
		m.input.SetValue(m.fieldString(f))
		m.input.CursorEnd()
		m.input.Focus()
		m.mode = emInline
	case feAPIKey:
		// Non-Other credential families do not consume a typed API key at
		// save time. Keep this row visibly read-only so it cannot suggest
		// that typing here changes the bound OAuth/CLI credential.
		family := preset.ClassifyCredentialFamily(asString(m.llmMap()["provider"]))
		switch family {
		case preset.CredentialFamilyCodexSingle:
			m.saveErr = i18n.T("preset_editor.api_key_codex_readonly")
			return *m, nil
		case preset.CredentialFamilyCodexPool, preset.CredentialFamilyClaudeCLI:
			m.saveErr = i18n.T("preset_editor.api_key_managed_externally")
			return *m, nil
		}
		// Edit the live key buffer, not the env-var-name. We start
		// blank rather than prefilling the existing value so the user
		// can paste a new key without first deleting the masked
		// placeholder. apiKeySet flips on commit if they typed anything.
		m.input.SetValue("")
		m.input.CursorEnd()
		m.input.Focus()
		m.mode = emInline
	case feModel:
		provider := asString(m.llmMap()["provider"])
		baseURL := asString(m.llmMap()["base_url"])
		if models := modelOptions(provider, baseURL); len(models) > 0 {
			m.cycleFocused(+1)
		} else {
			m.input.SetValue(m.fieldString(f))
			m.input.CursorEnd()
			m.input.Focus()
			m.mode = emInline
		}
	case feServiceTier:
		if m.isCodexProvider() {
			m.cycleFocused(+1)
		}
	case feThinking:
		if m.hasThinking() {
			m.cycleFocused(+1)
		}
	case feTier:
		// Tier is an enum — Enter cycles like ←/→. No picker overlay.
		m.cycleFocused(+1)
	case feProvider, feAPICompat, feWireAPI, feResponsesTransport:
		// Enums — Enter cycles forward (same as Right). Lets the user
		// stay on the keyboard's "advance" key.
		m.cycleFocused(+1)
	case feSave:
		updated, cmd := m.commit()
		return updated, cmd
	}
	return *m, nil
}

// applyInline writes the textinput's current value into the working
// copy, with light coercion for numeric fields.
func (m *PresetEditorModel) applyInline(val string) {
	val = strings.TrimSpace(val)
	f := editorFieldOrder[m.cursor]
	llm := m.llmMap()
	switch f {
	case feName:
		// Empty name is silently ignored — name is required to save and
		// the validator will catch a bad write later. Spaces collapse to
		// underscores so the on-disk filename is shell-safe.
		if val != "" {
			m.working.Name = strings.ReplaceAll(val, " ", "_")
		}
	case feSummary:
		m.working.Description.Summary = val
	case feGains:
		m.setExtra("gains", val)
	case feLoses:
		m.setExtra("loses", val)
	case feModel:
		llm["model"] = val
	case feBaseURL:
		if val == "" {
			llm["base_url"] = nil
		} else {
			llm["base_url"] = val
		}
	case feAPIKey:
		// Store the raw key in the editor's buffer; the manifest
		// itself only holds api_key_env (the slot name), assigned at
		// commit time by the host's stampAutoEnvVar helper. Opening the
		// blank replacement editor and pressing Enter without typing is
		// a no-op, not a clear; key clearing needs an explicit future UI.
		if val == "" {
			return
		}
		m.apiKey = val
		m.apiKeySet = true
	}
}

func (m PresetEditorModel) isCodexProvider() bool {
	return preset.ClassifyCredentialFamily(asString(m.llmMap()["provider"])) == preset.CredentialFamilyCodexSingle
}

func isCodexThinkingProvider(provider string) bool {
	family := preset.ClassifyCredentialFamily(provider)
	return family == preset.CredentialFamilyCodexSingle || family == preset.CredentialFamilyCodexPool
}

func (m PresetEditorModel) hasCodexThinking() bool {
	return isCodexThinkingProvider(asString(m.llmMap()["provider"]))
}

// isThinkingLevel reports whether v is one of the kernel's canonical
// THINKING_LEVELS payload values. "default" is deliberately absent: it is a
// UI-only pseudo-option meaning "omit manifest.llm.thinking".
func isThinkingLevel(v string) bool {
	switch v {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	}
	return false
}

// llmHasLevelThinking reports whether an llm block takes the canonical
// THINKING_LEVELS ladder — every thinking-capable provider except the Codex
// family, which keeps its own ladder and its own xhigh default.
//
// Scope: Anthropic (the adapter turns the level into an extended-thinking
// budget) and every OpenAI-compatible provider (api_compat=openai), whose
// level rides through as Responses reasoning.effort. wire_api does NOT gate
// this: the field is accepted regardless of which OpenAI wire the preset
// selects. Providers with a native non-OpenAI adapter and no api_compat
// declaration — gemini, claude-code, minimax — stay out of scope.
func llmHasLevelThinking(llm map[string]interface{}) bool {
	if llm == nil {
		return false
	}
	provider := asString(llm["provider"])
	if isCodexThinkingProvider(provider) {
		return false // Codex owns its ladder; see codexThinkingOptions.
	}
	switch provider {
	case "anthropic", "openai":
		return true
	}
	// api_compat is the explicit declaration of which wire protocol (and so
	// which kernel adapter) the provider speaks; both adapters behind it
	// honor a configured thinking level.
	switch asString(llm["api_compat"]) {
	case "openai", "anthropic":
		return true
	}
	return false
}

func (m PresetEditorModel) hasLevelThinking() bool {
	return llmHasLevelThinking(m.llmMap())
}

func (m PresetEditorModel) hasThinking() bool {
	return m.hasCodexThinking() || m.hasLevelThinking()
}

// isCustomOpenAI reports whether the working preset is in the narrow scope
// where the OpenAI wire-format selector (wire_api) applies: the custom
// provider with api_compat=openai. Built-in OpenAI, Anthropic/Gemini custom
// compat, Codex, and all other providers never surface the wire_api field.
func (m PresetEditorModel) isCustomOpenAI() bool {
	llm, _ := m.working.Manifest["llm"].(map[string]interface{})
	return asString(llm["provider"]) == "custom" &&
		asString(llm["api_compat"]) == "openai"
}

// isCustomOpenAIResponses is the narrow custom-provider scope built on the
// only Kernel path that supports the Responses adapter: custom + OpenAI
// compatibility + explicit Responses. It gates the transport selector. The
// reasoning-effort selector is NOT gated on it — see llmHasLevelThinking,
// which accepts every OpenAI-compatible provider on any wire.
func (m PresetEditorModel) isCustomOpenAIResponses() bool {
	return m.isCustomOpenAI() && m.fieldString(feWireAPI) == "responses"
}

// codexAccountRefs returns the selectable codex_auth_path values for the
// account picker on the feAPIKey row: "" (legacy/default account) first, then
// each per-account file's home-shortened ref. Order is stable so ←/→ cycling
// is predictable.
func (m PresetEditorModel) codexAccountRefs() []string {
	refs := []string{""} // "" == legacy/default account
	if m.globalDir == "" {
		return refs
	}
	for _, a := range listCodexAccounts(m.globalDir) {
		if a.Legacy {
			continue // the legacy file is already represented by ""
		}
		refs = append(refs, a.Ref)
	}
	return refs
}

// codexAuthRef returns the preset's bound manifest.llm.codex_auth_path ("" when
// unset / legacy fallback).
func (m PresetEditorModel) codexAuthRef() string {
	return asString(m.llmMap()["codex_auth_path"])
}

// setCodexAuthRef writes (or clears) manifest.llm.codex_auth_path. An empty ref
// removes the field so the preset falls back to the legacy account with no
// stray key in the JSON.
func (m *PresetEditorModel) setCodexAuthRef(ref string) {
	llm := m.llmMap()
	if preset.ClassifyCredentialFamily(asString(llm["provider"])) != preset.CredentialFamilyCodexSingle || strings.TrimSpace(ref) == "" {
		delete(llm, "codex_auth_path")
		return
	}
	llm["codex_auth_path"] = ref
}

// codexBoundAccountLabel returns a non-secret label for the account the preset
// is currently bound to, plus whether that account's token file is valid.
func (m PresetEditorModel) codexBoundAccountLabel() (string, bool) {
	ref := m.codexAuthRef()
	path := resolveCodexAuthPath(m.globalDir, ref)
	valid := codexAuthPathValid(path)
	if ref == "" {
		// Legacy/default: prefer the stored email for the label.
		if tok, ok := readCodexTokenFile(path); ok && tok.Email != "" {
			return tok.Email, valid
		}
		return i18n.T("codex.account_default"), valid
	}
	if tok, ok := readCodexTokenFile(path); ok && tok.Email != "" {
		return tok.Email, valid
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), valid
}

// codexCLIImportAvailable reports whether the codex API-key row can offer the
// one-click CLI import: the preset uses a codex single credential, globalDir
// is set, the currently-bound account is invalid, and a valid Codex CLI
// credential file (~/.codex/auth.json or $CODEX_HOME/auth.json) exists to
// import. Drives the footer hint; the 'i' handler additionally requires the
// row to be focused.
func (m PresetEditorModel) codexCLIImportAvailable() bool {
	if m.globalDir == "" {
		return false
	}
	if preset.ClassifyCredentialFamily(asString(m.llmMap()["provider"])) != preset.CredentialFamilyCodexSingle {
		return false
	}
	if _, valid := m.codexBoundAccountLabel(); valid {
		return false
	}
	_, ok := readCodexCLIAuthFile(codexCLIAuthPath())
	return ok
}
func (m PresetEditorModel) codexServiceTier() string {
	llm, _ := m.working.Manifest["llm"].(map[string]interface{})
	if asString(llm["service_tier"]) == "fast" {
		return "fast"
	}
	return "normal"
}

func (m *PresetEditorModel) setCodexServiceTier(tier string) {
	llm := m.llmMap()
	if preset.ClassifyCredentialFamily(asString(llm["provider"])) != preset.CredentialFamilyCodexSingle || tier != "fast" {
		delete(llm, "service_tier")
		return
	}
	llm["service_tier"] = "fast"
}

// codexDefaultThinking is the reasoning effort LingTai applies to Codex
// when a preset omits (or carries an invalid) llm.thinking. LingTai is the
// primary brain, so it runs Codex at maximum effort by default.
const codexDefaultThinking = "xhigh"

func (m PresetEditorModel) codexThinking() string {
	llm, _ := m.working.Manifest["llm"].(map[string]interface{})
	switch asString(llm["thinking"]) {
	case "low", "medium", "high", "xhigh":
		return asString(llm["thinking"])
	default:
		return codexDefaultThinking
	}
}

func (m *PresetEditorModel) setCodexThinking(effort string) {
	llm := m.llmMap()
	if !isCodexThinkingProvider(asString(llm["provider"])) {
		delete(llm, "thinking")
		return
	}
	switch effort {
	case "low", "medium", "high", "xhigh":
		llm["thinking"] = effort
	default:
		// Absent/invalid resolves to the Codex default, persisted
		// explicitly so the running session actually receives xhigh.
		llm["thinking"] = codexDefaultThinking
	}
}

func (m PresetEditorModel) thinkingValue() string {
	if m.hasCodexThinking() {
		return m.codexThinking()
	}
	if m.hasLevelThinking() {
		if v := asString(m.llmMap()["thinking"]); isThinkingLevel(v) {
			return v
		}
		// Absent or invalid: the kernel's own default applies, which the
		// editor shows (and stores) as omission.
		return "default"
	}
	return ""
}

func (m PresetEditorModel) thinkingOptions() []string {
	if m.hasCodexThinking() {
		return codexThinkingOptions
	}
	if m.hasLevelThinking() {
		return customResponsesThinkingOptions
	}
	return nil
}

func (m *PresetEditorModel) setThinking(effort string) {
	if m.hasCodexThinking() {
		m.setCodexThinking(effort)
		return
	}
	llm := m.llmMap()
	if !m.hasLevelThinking() || !isThinkingLevel(effort) {
		// Out of scope, "default", or an unknown value — all of which mean
		// "carry no thinking field".
		delete(llm, "thinking")
		return
	}
	llm["thinking"] = effort
}

func normalizeServiceTier(manifest map[string]interface{}) {
	llm, _ := manifest["llm"].(map[string]interface{})
	if llm == nil || preset.ClassifyCredentialFamily(asString(llm["provider"])) != preset.CredentialFamilyCodexSingle {
		return
	}
	if asString(llm["service_tier"]) == "fast" {
		return
	}
	delete(llm, "service_tier")
}

func normalizeThinking(manifest map[string]interface{}) {
	llm, _ := manifest["llm"].(map[string]interface{})
	if llm == nil {
		return
	}
	if isCodexThinkingProvider(asString(llm["provider"])) {
		switch asString(llm["thinking"]) {
		case "low", "medium", "high", "xhigh":
			return
		default:
			// Codex with absent/invalid thinking is normalized to the default
			// so committed/cloned/generated presets explicitly carry it and the
			// running session receives xhigh rather than a UI-only fallback.
			llm["thinking"] = codexDefaultThinking
			return
		}
	}
	if llmHasLevelThinking(llm) {
		if isThinkingLevel(asString(llm["thinking"])) {
			return
		}
		// "default" is represented by omission for every non-Codex provider:
		// the Kernel's own main-session default then applies. No default is
		// forced here, so an absent field stays absent.
		delete(llm, "thinking")
		return
	}
	delete(llm, "thinking")
}

// normalizeWireAPI strips llm.wire_api whenever the preset leaves the narrow
// custom+openai scope where the OpenAI wire-format selector applies. Inside
// that scope, an explicit "auto" (the absence default) is omitted to keep the
// committed manifest minimal — absent and "auto" are semantically identical.
func normalizeWireAPI(manifest map[string]interface{}) {
	llm, _ := manifest["llm"].(map[string]interface{})
	if llm == nil {
		return
	}
	if asString(llm["provider"]) == "custom" && asString(llm["api_compat"]) == "openai" {
		if asString(llm["wire_api"]) == "auto" {
			delete(llm, "wire_api")
		}
		return
	}
	delete(llm, "wire_api")
}

// normalizeResponsesTransport keeps HTTP as the omission/default and removes
// stale transport values outside custom OpenAI-compatible Responses.
func normalizeResponsesTransport(manifest map[string]interface{}) {
	llm, _ := manifest["llm"].(map[string]interface{})
	if llm == nil {
		return
	}
	if asString(llm["provider"]) == "custom" &&
		asString(llm["api_compat"]) == "openai" &&
		asString(llm["wire_api"]) == "responses" {
		if asString(llm["responses_transport"]) != "websocket" {
			delete(llm, "responses_transport")
		}
		return
	}
	delete(llm, "responses_transport")
}

func normalizeLLMForCommit(manifest map[string]interface{}) {
	normalizeServiceTier(manifest)
	normalizeThinking(manifest)
	normalizeWireAPI(manifest)
	normalizeResponsesTransport(manifest)
}

// setExtra writes into Description.Extra, allocating the map on first
// use. Empty string deletes the key.
func (m *PresetEditorModel) setExtra(key, val string) {
	if val == "" {
		delete(m.working.Description.Extra, key)
		if len(m.working.Description.Extra) == 0 {
			m.working.Description.Extra = nil
		}
		return
	}
	if m.working.Description.Extra == nil {
		m.working.Description.Extra = map[string]interface{}{}
	}
	m.working.Description.Extra[key] = val
}

// cycleFocused rotates enum fields by `dir` (+1 or -1).
func (m *PresetEditorModel) cycleFocused(dir int) {
	f := editorFieldOrder[m.cursor]
	switch f {
	case feProvider:
		// The subset of builtins reachable from the provider cycle, in
		// BuiltinPresets order; kimi/gemini/codex-pool/claude are
		// intentionally reached only by opening their own template, so this
		// list is deliberately shorter than BuiltinPresets() rather than out
		// of sync with it. Anything added here must also be handled by the
		// model/base_url/api_key_env resets below.
		opts := []string{"minimax", "zhipu", "mimo", "deepseek", "grok", "nvidia", "openrouter", "codex", "custom"}
		oldProvider := m.fieldString(f)
		newProvider := cycleString(opts, oldProvider, dir)
		m.llmMap()["provider"] = newProvider
		normalizeWireAPI(m.working.Manifest)
		normalizeResponsesTransport(m.working.Manifest)
		if newProvider != oldProvider {
			delete(m.llmMap(), "thinking")
		}
		normalizeThinking(m.working.Manifest)
		// Reset model to the new provider's first canonical entry when the
		// current model isn't valid for the new provider. Without this, a
		// minimax→zhipu switch leaves "MiniMax-M2.7" in model
		// and validation passes silently while the kernel later 4xxs.
		baseURL := ""
		if regions, ok := preset.ProviderRegionURLs[newProvider]; ok && len(regions) > 0 {
			baseURL = regions[0].URL
		}
		if models := modelOptions(newProvider, baseURL); len(models) > 0 {
			currentModel := asString(m.llmMap()["model"])
			modelStillValid := false
			for _, mdl := range models {
				if mdl == currentModel {
					modelStillValid = true
					break
				}
			}
			if !modelStillValid {
				m.llmMap()["model"] = models[0]
			}
		}
		// Reset base_url and the credential env-var slot to the new
		// provider's defaults when switching to a provider with known
		// regional endpoints. base_url adopts the first region (the
		// default); api_key_env is reset from ProviderDefaultEnv so a slot
		// adopted from a previous base_url cycle (e.g. OpenCode Go ->
		// OPENCODE_GO_API_KEY) cannot follow the user into the next
		// provider: a zhipu preset resolving through OPENCODE_GO_API_KEY
		// reports "no key" for someone who has ZHIPU_API_KEY set, or sends
		// the wrong key to bigmodel.cn.
		//
		// The api_key_env reset is unconditional and normally comes from the
		// provider map, NOT from regions[0].Env: zhipu/minimax region rows
		// declare no Env precisely so a CN<->INTL base_url cycle preserves
		// the region-suffixed slot the host stamped (ZHIPU_INTL_1_API_KEY)
		// instead of overwriting it with a region-agnostic one. Providers
		// absent from ProviderDefaultEnv (none today; map is exhaustive)
		// keep their current api_key_env. The memoized pre-adoption slot goes
		// with it: it was taken from the previous provider's region cycle and
		// must not be restorable onto this one.
		//
		// The one exception: when the landing row (regions[0], which base_url
		// adopts just above) declares its own Env, that declaration wins.
		// Only grok differs from its default — its row IS OpenCode Go, so
		// taking ProviderDefaultEnv["grok"] = GROK_API_KEY here would leave
		// the preset pointed at https://opencode.ai/zen/go/v1 holding a
		// credential slot that endpoint does not use, and the user would have
		// to paste their OpenCode Go key into a second slot. The slot must
		// match the row you land on.
		if regions, ok := preset.ProviderRegionURLs[newProvider]; ok && len(regions) > 0 {
			m.llmMap()["base_url"] = regions[0].URL
		}
		if env, ok := preset.ProviderDefaultEnv[newProvider]; ok {
			want := env
			if regions := preset.ProviderRegionURLs[newProvider]; len(regions) > 0 && regions[0].Env != "" {
				want = regions[0].Env
			}
			m.llmMap()["api_key_env"] = want
		}
		m.regionEnvBeforeAdopt = ""
	case feModel:
		provider := asString(m.llmMap()["provider"])
		baseURL := asString(m.llmMap()["base_url"])
		if models := modelOptions(provider, baseURL); len(models) > 0 {
			next := cycleString(models, m.fieldString(f), dir)
			m.llmMap()["model"] = next
		}
	case feBaseURL:
		provider := asString(m.llmMap()["provider"])
		if regions, ok := preset.ProviderRegionURLs[provider]; ok && len(regions) > 0 {
			idx := selectedRegionIndex(regions, m.fieldString(f))
			var next, prev preset.RegionURL
			if idx >= 0 {
				prev = regions[idx]
			}
			if idx < 0 {
				// Off-list value on a provider with no Custom row: nothing is
				// selected, so enter the list at its edge the way cycleString
				// does — first option going right, last going left.
				if dir < 0 {
					next = regions[len(regions)-1]
				} else {
					next = regions[0]
				}
			} else {
				next = regions[(idx+dir+len(regions))%len(regions)]
			}
			if next.URL == "" {
				// Moving onto the Custom free-text sentinel: clear the value
				// so Enter opens a blank inline edit. This branch is only
				// reachable from a known region option; an off-list typed URL
				// resolves to the Custom row, so cycling away from it goes
				// straight to a real region and never clears the typed value
				// as a side effect.
				//
				// api_key_env is deliberately NOT touched here (pinned by
				// TestPresetEditorDeepseekRegionCycleRoundTrip): the user is
				// about to type their own endpoint and keeps whatever slot
				// that endpoint actually uses. Cycling on to a real region
				// row re-applies that row's rule.
				m.llmMap()["base_url"] = ""
				break
			}
			currentModel := asString(m.llmMap()["model"])
			m.llmMap()["model"] = reconcileModelForRoute(provider, next.URL, currentModel)
			m.llmMap()["base_url"] = next.URL
			// Region options can carry an implied credential env-var (e.g.
			// DeepSeek API -> DEEPSEEK_API_KEY, OpenCode Go ->
			// OPENCODE_GO_API_KEY); adopt it when the selected option declares
			// one. The Custom option declares none, so landing on it leaves
			// api_key_env untouched and the user keeps whatever credential
			// slot their endpoint actually uses.
			//
			// The adoption must be REVERSIBLE. deepseek self-heals on the way
			// out (its DeepSeek API row declares DEEPSEEK_API_KEY), but the
			// zhipu/minimax CN/INTL rows deliberately declare no Env, so
			// wrapping past OpenCode Go back to CN would otherwise leave the
			// preset pointing at bigmodel.cn while resolving through
			// OPENCODE_GO_API_KEY — silently destroying the user's
			// ZHIPU_INTL_1_API_KEY. Memoize the slot on the way in and put it
			// back on the way out.
			//
			// The restore is keyed on the STATE, not on the previously
			// selected row: an off-list base_url yields no selected row at all
			// (selectedRegionIndex == -1 on zhipu/minimax), and a preset that
			// pairs such a URL with OPENCODE_GO_API_KEY must still be healed on
			// the way onto CN rather than ride the adopted slot along.
			switch {
			case next.Env != "":
				cur := asString(m.llmMap()["api_key_env"])
				if prev.Env == "" && !regionDeclaredEnv(provider, cur) {
					m.regionEnvBeforeAdopt = cur
				}
				m.llmMap()["api_key_env"] = next.Env
			default: // next.Env == ""
				if cur := asString(m.llmMap()["api_key_env"]); regionDeclaredEnv(provider, cur) {
					restore := m.regionEnvBeforeAdopt
					if restore == "" {
						restore = preset.ProviderDefaultEnv[provider]
					}
					m.llmMap()["api_key_env"] = restore
					m.regionEnvBeforeAdopt = ""
				}
			}
		}
	case feAPICompat:
		opts := []string{"", "openai", "anthropic"}
		m.llmMap()["api_compat"] = cycleString(opts, m.fieldString(f), dir)
		normalizeWireAPI(m.working.Manifest)
		normalizeResponsesTransport(m.working.Manifest)
		normalizeThinking(m.working.Manifest)
	case feWireAPI:
		next := cycleString(wireAPIOptions, m.fieldString(f), dir)
		if next == "auto" {
			delete(m.llmMap(), "wire_api")
		} else {
			m.llmMap()["wire_api"] = next
		}
		normalizeResponsesTransport(m.working.Manifest)
		normalizeThinking(m.working.Manifest)
	case feResponsesTransport:
		next := cycleString(responsesTransportOptions, m.fieldString(f), dir)
		if next == "websocket" {
			m.llmMap()["responses_transport"] = next
		} else {
			delete(m.llmMap(), "responses_transport")
		}
	case feServiceTier:
		if m.isCodexProvider() {
			m.setCodexServiceTier(cycleString(codexServiceTierOptions, m.codexServiceTier(), dir))
		}
	case feThinking:
		if m.hasThinking() {
			m.setThinking(cycleString(m.thinkingOptions(), m.thinkingValue(), dir))
		}
	case feAPIKey:
		// Codex account selector: bind the preset to the next/previous
		// stored account by cycling manifest.llm.codex_auth_path. "" is the
		// legacy/default account.
		if m.isCodexProvider() {
			refs := m.codexAccountRefs()
			if len(refs) > 1 {
				m.setCodexAuthRef(cycleString(refs, m.codexAuthRef(), dir))
			}
		}
	case feTier:
		// Cycle ""→1→2→3→4→5→"" with → and reverse with ←. tierValues
		// is ordered best-first ([5..1]) for the library's picker, so
		// reverse it here for the natural ascending sweep.
		opts := []string{"", "1", "2", "3", "4", "5"}
		m.working.Description.Tier = cycleString(opts, m.working.Description.Tier, dir)
	}
}

func (m PresetEditorModel) commit() (PresetEditorModel, tea.Cmd) {
	if errs := m.working.Validate(); len(errs) > 0 {
		m.saveErr = localizedPresetValidationError(errs[0])
		return m, nil
	}
	m.saveErr = ""
	// Templates (built-ins) are starting points: the user picks one,
	// edits it, and saves. The save always materializes a *new* file
	// under an auto-generated name like `mimo-1` so the template stays
	// pristine and the user gets a saved preset they own.
	//
	// If the user explicitly renamed the preset in the editor (Name
	// differs from the template's name), respect that name. Otherwise
	// gap-fill the next "<template>-N" slot.
	committed := clonePresetForEditor(m.working)
	// Kernel core capabilities (knowledge, skills, shell, avatar, daemon,
	// mcp, file group) are floor-injected by apply_core_defaults at
	// runtime, so we deliberately do NOT stamp them into the saved
	// manifest. That keeps preset JSON minimal and avoids implying these
	// are ordinary opt-ins.
	if m.isBuiltin && (m.hasSemanticEdits() || m.apiKeySet) {
		if committed.Name == m.original.Name {
			existing, _ := preset.List()
			names := make([]string, 0, len(existing))
			for _, p := range existing {
				names = append(names, p.Name)
			}
			if auto := preset.AutoSavedName(m.original.Name, names); auto != "" {
				committed.Name = auto
			}
		}
		// Clear the inherited api_key_env so the host's stampAutoEnvVar
		// allocates a fresh slot (PROVIDER_N_API_KEY) under the new name.
		// Without this, the user's pasted key would overwrite the
		// template's shared slot (e.g. MIMO_API_KEY), polluting any
		// other preset that references it.
		//
		// Exception: a CROSS-PROVIDER account a region row declares
		// (OpenCode Go -> OPENCODE_GO_API_KEY) — the whole point of picking
		// that base_url option is to resolve through that one credential.
		// Dropping it here would make stampAutoEnvVar mint an unrelated
		// PROVIDER_N slot, so a user who already configured OpenCode Go on
		// deepseek would have to paste the same key again for zhipu. Keep it.
		//
		// The provider's OWN default slot is not such a case even when a
		// region row declares it (DeepSeek API -> DEEPSEEK_API_KEY): that is
		// the template's shared slot in the same sense as MIMO_API_KEY above,
		// so it is dropped like any other and stampAutoEnvVar mints
		// DEEPSEEK_1_API_KEY.
		if llm, ok := committed.Manifest["llm"].(map[string]interface{}); ok {
			if !usesRegionDeclaredEnv(llm) {
				delete(llm, "api_key_env")
			}
		}
	}
	normalizeLLMForCommit(committed.Manifest)
	return m, m.commitCmd(committed)
}

func localizedPresetValidationError(err error) string {
	if err == nil {
		return ""
	}
	switch err.Error() {
	case "description.summary must be non-empty":
		return i18n.T("preset_editor.validation.description_summary_required")
	case "manifest.llm must be an object":
		return i18n.T("preset_editor.validation.llm_object_required")
	case "manifest.llm.provider must be non-empty":
		return i18n.T("preset_editor.validation.llm_provider_required")
	case "manifest.llm.model must be non-empty":
		return i18n.T("preset_editor.validation.llm_model_required")
	default:
		return err.Error()
	}
}

// commitCmd is the single PresetEditorCommitMsg constructor. Every host writes
// editor results through preset.Save, whose destination is presets/saved/, so
// the runtime-only Source on the committed object must identify that exact
// destination even when its name matches a built-in template.
func (m PresetEditorModel) commitCmd(committed preset.Preset) tea.Cmd {
	committed.Source = preset.SourceSaved
	return func() tea.Msg {
		return PresetEditorCommitMsg{Preset: committed, APIKey: m.apiKey, APIKeySet: m.apiKeySet}
	}
}

// hasSemanticEdits reports whether the user changed any field whose
// in-place edit on a built-in would silently mask a TUI upgrade. The
// definition of "semantic" is: anything except description.summary,
// description.tier, and description.Extra (gains/loses/etc.).
func (m PresetEditorModel) hasSemanticEdits() bool {
	if m.working.Name != m.original.Name {
		return true
	}
	wm, _ := json.Marshal(m.working.Manifest)
	om, _ := json.Marshal(m.original.Manifest)
	return string(wm) != string(om)
}

// updateClonePrompt handles the new-name textinput overlay shown to
// gate semantic edits on built-in presets.
func (m PresetEditorModel) updateClonePrompt(msg tea.KeyMsg) (PresetEditorModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = emBrowse
		m.cloneNameInput.Blur()
		return m, nil
	case "ctrl+e":
		// Expert override: skip clone, save in place under the original
		// built-in name. The user explicitly accepts that future TUI
		// upgrades won't refresh this preset.
		m.mode = emBrowse
		m.cloneNameInput.Blur()
		committed := clonePresetForEditor(m.working)
		normalizeLLMForCommit(committed.Manifest)
		return m, m.commitCmd(committed)
	case "enter":
		newName := strings.TrimSpace(m.cloneNameInput.Value())
		if newName == "" {
			m.saveErr = "name cannot be empty"
			return m, nil
		}
		// The name becomes a filename stem under presets/saved/ via
		// preset.Save; reject path forms up front so the user sees a
		// clear error instead of an escape attempt (issue #849).
		if err := preset.ValidateSafeName(newName); err != nil {
			m.saveErr = "invalid name: " + err.Error()
			return m, nil
		}
		if newName == m.original.Name {
			m.saveErr = "pick a different name (or press Ctrl+E to overwrite the built-in)"
			return m, nil
		}
		m.working.Name = newName
		m.mode = emBrowse
		m.cloneNameInput.Blur()
		committed := clonePresetForEditor(m.working)
		normalizeLLMForCommit(committed.Manifest)
		return m, m.commitCmd(committed)
	}
	var cmd tea.Cmd
	m.cloneNameInput, cmd = m.cloneNameInput.Update(msg)
	return m, cmd
}

// ───────────────────────────────────────────────────────────────────────────
// Read-side helpers
// ───────────────────────────────────────────────────────────────────────────

func (m PresetEditorModel) llmMap() map[string]interface{} {
	llm, _ := m.working.Manifest["llm"].(map[string]interface{})
	if llm == nil {
		llm = map[string]interface{}{}
		m.working.Manifest["llm"] = llm
	}
	return llm
}

// fieldString returns the current display value for the given field.
func (m PresetEditorModel) fieldString(f editorField) string {
	llm, _ := m.working.Manifest["llm"].(map[string]interface{})
	switch f {
	case feName:
		return m.working.Name
	case feSummary:
		return m.working.Description.Summary
	case feTier:
		return m.working.Description.Tier
	case feGains:
		v, _ := m.working.Description.Extra["gains"].(string)
		return v
	case feLoses:
		v, _ := m.working.Description.Extra["loses"].(string)
		return v
	case feProvider:
		s, _ := llm["provider"].(string)
		return s
	case feModel:
		s, _ := llm["model"].(string)
		return s
	case feServiceTier:
		return m.codexServiceTier()
	case feThinking:
		return m.thinkingValue()
	case feAPICompat:
		s, _ := llm["api_compat"].(string)
		return s
	case feWireAPI:
		s, _ := llm["wire_api"].(string)
		if s == "" {
			return "auto"
		}
		return s
	case feResponsesTransport:
		if s, _ := llm["responses_transport"].(string); s == "websocket" {
			return s
		}
		return "http"
	case feBaseURL:
		s, _ := llm["base_url"].(string)
		return s
	case feAPIKey:
		// Codex uses an OAuth credential, not an API key. Show the bound
		// account (manifest.llm.codex_auth_path → resolved token file) and
		// its validity. When more than one account exists, ←/→ cycles the
		// binding (see isCyclable/cycleField). No secret is shown.
		family := preset.ClassifyCredentialFamily(asString(llm["provider"]))
		if family == preset.CredentialFamilyCodexSingle {
			if m.globalDir != "" {
				label, valid := m.codexBoundAccountLabel()
				if valid {
					return "✓ " + label
				}
				return "✗ " + label + " — " + i18n.T("codex.oauth_not_logged_in")
			}
			return i18n.T("codex.oauth_not_logged_in")
		}
		if family == preset.CredentialFamilyCodexPool || family == preset.CredentialFamilyClaudeCLI {
			return i18n.T("preset_editor.api_key_managed_externally")
		}
		// Other providers display the existing key masked. The env-var name
		// is an internal detail; the user only needs to see whether a key is
		// set.
		return maskAPIKey(m.apiKey)
	}
	return ""
}

func (m PresetEditorModel) isDirty() bool {
	a, _ := json.Marshal(m.working)
	b, _ := json.Marshal(m.original)
	return string(a) != string(b)
}

// ───────────────────────────────────────────────────────────────────────────
// View
// ───────────────────────────────────────────────────────────────────────────

func (m PresetEditorModel) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	bodyHeight := m.height - 4
	if bodyHeight < 3 {
		bodyHeight = 3
	}

	// Title bar.
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	title := titleStyle.Render(i18n.T("preset_editor.title") + ": " + m.working.Name)
	if label := tierLabel(m.working.Description.Tier, m.lang); label != "" {
		title += "  " + tierChipStyle(m.working.Description.Tier).Render(label)
	}

	// JSON preview is opt-in via Ctrl+D. When off (default), the form
	// claims the full width — clean & focused. When on AND wide enough,
	// split horizontally. Narrow terminals always show form-only.
	var body string
	if m.showJSON && m.width >= 100 {
		formW := m.width / 2
		previewW := m.width - formW - 1
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderForm(formW, bodyHeight),
			" ",
			m.renderPreview(previewW, bodyHeight),
		)
	} else {
		body = m.renderForm(m.width, bodyHeight)
	}

	footer := m.renderFooter()
	full := lipgloss.JoinVertical(lipgloss.Left, title, body, footer)

	switch m.mode {
	case emClonePrompt:
		full = m.renderCloneOverlay(full)
	case emDirtyPrompt:
		full = m.renderDirtyOverlay(full)
	case emExitPrompt:
		full = m.renderExitOverlay(full)
	}
	return full
}

type presetEditorRow struct {
	text     string
	field    editorField
	hasField bool
}

func (m PresetEditorModel) renderForm(width, height int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("245")).
		Width(width).
		Height(height).
		Padding(0, 1)

	rows := m.formRows(width)
	visibleRows := formContentHeight(height)
	start := m.scrollOffset
	if start < 0 {
		start = 0
	}
	if maxStart := maxScrollStart(len(rows), visibleRows); start > maxStart {
		start = maxStart
	}
	end := start + visibleRows
	if end > len(rows) {
		end = len(rows)
	}

	contentWidth := formInnerWidth(width)
	visible := make([]string, 0, visibleRows)
	for _, row := range rows[start:end] {
		// Every semantic row must occupy exactly one terminal row.
		// Several row builders contain ANSI styling, and plain rune-count
		// truncation is not enough: lipgloss will wrap over-wide styled
		// strings inside the bordered box, making the final Save row fall
		// below the alt-screen viewport even when our semantic row slice
		// includes it. Clamp the rendered row at the box's inner display
		// width as a final safety net.
		visible = append(visible, ansi.Truncate(row.text, contentWidth, "…"))
	}

	return box.Render(strings.Join(visible, "\n"))
}

func formInnerWidth(width int) int {
	// renderForm sets total Width(width), a border on both sides, and
	// horizontal padding of one cell on both sides. The text content must
	// fit within what remains or lipgloss wraps it into extra visual rows.
	inner := width - 4
	if inner < 1 {
		return 1
	}
	return inner
}

func (m PresetEditorModel) formRows(width int) []presetEditorRow {
	lbl := func(key string) string { return i18n.T("preset_editor.field_" + key) }
	row := func(f editorField, text string) presetEditorRow {
		return presetEditorRow{text: text, field: f, hasField: true}
	}
	plain := func(text string) presetEditorRow { return presetEditorRow{text: text} }

	var rows []presetEditorRow
	rows = append(rows, plain(m.sectionHeader(i18n.T("preset_editor.section_identity"))))
	// Name row renders the on-disk preset stem. Editable for non-builtins;
	// for builtins, the clone-first overlay still gates renames on save.
	rows = append(rows, row(feName, m.row(feName, lbl("name"), m.working.Name, width-4)))
	rows = append(rows, row(feSummary, m.row(feSummary, lbl("summary"), m.working.Description.Summary, width-4)))
	rows = append(rows, row(feTier, m.row(feTier, lbl("tier"), m.tierDisplay(), width-4)))
	rows = append(rows, row(feGains, m.row(feGains, lbl("gains"), asExtra(m.working.Description.Extra, "gains"), width-4)))
	rows = append(rows, row(feLoses, m.row(feLoses, lbl("loses"), asExtra(m.working.Description.Extra, "loses"), width-4)))
	rows = append(rows, plain(""))
	rows = append(rows, plain(m.sectionHeader(i18n.T("preset_editor.section_llm"))))
	llm, _ := m.working.Manifest["llm"].(map[string]interface{})
	rows = append(rows, row(feProvider, m.row(feProvider, lbl("provider"), asString(llm["provider"]), width-4)))
	rows = append(rows, row(feModel, m.row(feModel, lbl("model"), asString(llm["model"]), width-4)))
	if m.fieldVisible(feServiceTier) {
		rows = append(rows, row(feServiceTier, m.row(feServiceTier, lbl("service_tier"), m.codexServiceTier(), width-4)))
	}
	if m.fieldVisible(feThinking) {
		rows = append(rows, row(feThinking, m.row(feThinking, lbl("thinking"), m.thinkingValue(), width-4)))
	}
	rows = append(rows, row(feAPICompat, m.row(feAPICompat, lbl("api_compat"), asString(llm["api_compat"]), width-4)))
	if m.fieldVisible(feWireAPI) {
		rows = append(rows, row(feWireAPI, m.row(feWireAPI, lbl("wire_api"), m.fieldString(feWireAPI), width-4)))
	}
	if m.fieldVisible(feResponsesTransport) {
		rows = append(rows, row(feResponsesTransport, m.row(feResponsesTransport, lbl("responses_transport"), m.fieldString(feResponsesTransport), width-4)))
	}
	rows = append(rows, row(feBaseURL, m.row(feBaseURL, lbl("base_url"), asString(llm["base_url"]), width-4)))
	rows = append(rows, row(feAPIKey, m.row(feAPIKey, lbl("api_key"), m.fieldString(feAPIKey), width-4)))
	rows = append(rows, plain(""))
	// Capabilities — every tool/subsystem the runtime can grant an agent,
	// including web_search and vision. All of them are always included:
	// there is no separate editable-capability concept, no checkbox, and
	// no provider control on this page that can remove or change one.
	// Customizing what an agent can do is done outside the preset editor
	// by asking the agent to explain init.json and hand-editing it there
	// (capabilitiesGuidanceRow below).
	rows = append(rows, plain(m.sectionHeader(i18n.T("preset_editor.section_capabilities"))))
	capabilityRows := []string{
		"email", "psyche", "soul", "system",
		"knowledge", "skills", "shell",
		"avatar", "daemon", "mcp", "file",
		"web_search", "vision",
	}
	for _, capName := range capabilityRows {
		rows = append(rows, plain(m.mandatoryCapRow(capName, width-4)))
	}
	rows = append(rows, plain(""))
	rows = append(rows, plain(m.capabilitiesGuidanceRow(width-4)))
	rows = append(rows, plain(""))
	rows = append(rows, row(feSave, m.renderSaveButton()))
	return rows
}

// row renders a single field row with focus styling. When the row is
// in inline-edit mode (cursor here AND mode == emInline) the textinput
// renders in place of the value. The model row gets a special radio-
// strip render when the provider has a known model list, so all
// options are visible at once and ←/→ visibly moves the dot.
func (m PresetEditorModel) row(f editorField, key, value string, width int) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Width(presetEditorFieldLabelWidth)
	marker := "  "
	valStyle := lipgloss.NewStyle()
	focused := editorFieldOrder[m.cursor] == f
	if focused {
		marker = "▸ "
		valStyle = valStyle.Bold(true).Foreground(ColorAccent)
	}
	if m.mode == emInline && focused {
		return marker + keyStyle.Render(key) + m.input.View()
	}
	if f == feModel {
		if strip := m.modelRadioStrip(focused, valStyle); strip != "" {
			return marker + keyStyle.Render(key) + strip
		}
	}
	if f == feServiceTier {
		if strip := m.serviceTierRadioStrip(focused, valStyle); strip != "" {
			return marker + keyStyle.Render(key) + strip
		}
	}
	if f == feThinking {
		if strip := m.thinkingRadioStrip(focused, valStyle); strip != "" {
			return marker + keyStyle.Render(key) + strip
		}
	}
	if f == feWireAPI {
		return marker + keyStyle.Render(key) + m.wireAPIRadioStrip(focused, valStyle)
	}
	if f == feResponsesTransport {
		return marker + keyStyle.Render(key) + m.responsesTransportRadioStrip(focused, valStyle)
	}
	if f == feBaseURL {
		if strip := m.baseURLRadioStrip(focused, valStyle); strip != "" {
			return marker + keyStyle.Render(key) + strip
		}
	}
	if value == "" {
		value = "—"
	}
	if f != feTier {
		value = truncate(value, width-lipgloss.Width(marker)-presetEditorFieldLabelWidth)
	}
	return marker + keyStyle.Render(key) + valStyle.Render(value)
}

// mandatoryCapRow renders one capability row in the Capabilities section:
// a fixed, always-checked "[✓] name  description" line. Every capability
// listed in formRows' capabilityRows renders this way — there is no
// toggleable or provider-cyclable variant anymore; the row is purely
// informational.
func (m PresetEditorModel) mandatoryCapRow(name string, width int) string {
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	check := subtle.Render("[✓]")
	keyCol := subtle.Render(lipgloss.NewStyle().Width(15).Render(name))
	desc := i18n.T("firstrun.cap_desc." + name)
	desc = strings.ReplaceAll(desc, "\n", "  ")
	desc = truncate(desc, width-21)
	val := subtle.Render(desc)
	return "  " + check + " " + keyCol + val
}

// capabilitiesGuidanceRow renders the one-line explanation of how to
// customize an agent's capabilities now that this page offers no
// checkbox or provider control that can remove or change one: ask the
// agent to explain init.json, then hand-edit init.json directly.
func (m PresetEditorModel) capabilitiesGuidanceRow(width int) string {
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true)
	text := truncate(i18n.T("preset_editor.capabilities_guidance"), width)
	return "  " + subtle.Render(text)
}

// modelRadioStrip renders the model field as a horizontal radio strip
// (● selected ○ unselected) when the current provider+route has a known
// model lineup. Returns "" when there's no picker — caller falls back to the
// standard single-value render.
func (m PresetEditorModel) modelRadioStrip(focused bool, valStyle lipgloss.Style) string {
	provider := asString(m.llmMap()["provider"])
	baseURL := asString(m.llmMap()["base_url"])
	models := modelOptions(provider, baseURL)
	if len(models) == 0 {
		return ""
	}
	current := asString(m.llmMap()["model"])
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	parts := make([]string, 0, len(models))
	for _, mdl := range models {
		if mdl == current {
			if focused {
				parts = append(parts, valStyle.Render("● "+mdl))
			} else {
				parts = append(parts, "● "+mdl)
			}
		} else {
			parts = append(parts, subtle.Render("○ "+mdl))
		}
	}
	return strings.Join(parts, "  ")
}

func (m PresetEditorModel) serviceTierRadioStrip(focused bool, valStyle lipgloss.Style) string {
	if !m.isCodexProvider() {
		return ""
	}
	current := m.codexServiceTier()
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	parts := make([]string, 0, len(codexServiceTierOptions))
	for _, tier := range codexServiceTierOptions {
		if tier == current {
			if focused {
				parts = append(parts, valStyle.Render("● "+tier))
			} else {
				parts = append(parts, "● "+tier)
			}
		} else {
			parts = append(parts, subtle.Render("○ "+tier))
		}
	}
	return strings.Join(parts, "  ")
}

func (m PresetEditorModel) thinkingRadioStrip(focused bool, valStyle lipgloss.Style) string {
	if !m.hasThinking() {
		return ""
	}
	current := m.thinkingValue()
	options := m.thinkingOptions()
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	parts := make([]string, 0, len(options))
	for _, effort := range options {
		if effort == current {
			if focused {
				parts = append(parts, valStyle.Render("● "+effort))
			} else {
				parts = append(parts, "● "+effort)
			}
		} else {
			parts = append(parts, subtle.Render("○ "+effort))
		}
	}
	separator := "  "
	if m.hasLevelThinking() {
		// Seven level choices still need to fit in the standard 80-column form.
		separator = " "
	}
	return strings.Join(parts, separator)
}

// baseURLRadioStrip renders the base_url field as a horizontal radio
// strip showing region labels (e.g. "● CN  ○ INTL") when the current
// provider has regional endpoints. Returns "" when there's no region
// list — caller falls back to the standard single-value render.
// A region with an empty URL is the free-text "Custom" sentinel: it is
// selected whenever the current base_url is empty or doesn't match any
// known non-empty region URL, and the typed endpoint is appended after
// the strip so it stays visible (the strip path hides the raw value).
// A provider with no Custom row and an off-list base_url has nothing to
// select: every dot renders hollow and the raw value is appended, so the
// strip never claims an endpoint the preset does not point at.
func (m PresetEditorModel) baseURLRadioStrip(focused bool, valStyle lipgloss.Style) string {
	provider := asString(m.llmMap()["provider"])
	regions, ok := preset.ProviderRegionURLs[provider]
	if !ok || len(regions) == 0 {
		return ""
	}
	current := asString(m.llmMap()["base_url"])
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selected := selectedRegionIndex(regions, current)
	parts := make([]string, 0, len(regions))
	for i, r := range regions {
		if i == selected {
			if focused {
				parts = append(parts, valStyle.Render("● "+r.Label))
			} else {
				parts = append(parts, "● "+r.Label)
			}
		} else {
			parts = append(parts, subtle.Render("○ "+r.Label))
		}
	}
	strip := strings.Join(parts, "  ")
	// Custom, an unknown typed URL, or an off-list value on a provider with no
	// Custom row (selected < 0) shows the actual endpoint after the strip so
	// the user always sees exactly what will be saved.
	if (selected < 0 || regions[selected].URL == "") && current != "" {
		strip += "  " + subtle.Render(current)
	}
	return strip
}

// wireAPIRadioStrip renders all three wire choices so Custom OpenAI users can see the selector rather than only the current raw manifest value.
func (m PresetEditorModel) wireAPIRadioStrip(focused bool, valStyle lipgloss.Style) string {
	current := m.fieldString(feWireAPI)
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	parts := make([]string, 0, len(wireAPIOptions))
	for _, option := range wireAPIOptions {
		label := option
		if option == current {
			if focused {
				parts = append(parts, valStyle.Render("● "+label))
			} else {
				parts = append(parts, "● "+label)
			}
		} else {
			parts = append(parts, subtle.Render("○ "+label))
		}
	}
	return strings.Join(parts, "  ")
}

// responsesTransportRadioStrip shows the default HTTP path and explicit
// WebSocket v2 opt-in side by side.
func (m PresetEditorModel) responsesTransportRadioStrip(focused bool, valStyle lipgloss.Style) string {
	current := m.fieldString(feResponsesTransport)
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	parts := make([]string, 0, len(responsesTransportOptions))
	for _, option := range responsesTransportOptions {
		if option == current {
			if focused {
				parts = append(parts, valStyle.Render("● "+option))
			} else {
				parts = append(parts, "● "+option)
			}
		} else {
			parts = append(parts, subtle.Render("○ "+option))
		}
	}
	return strings.Join(parts, "  ")
}

// isCyclable reports whether a field accepts ←/→ to step through enum
// values. The model row is conditional on the current provider+route having
// a known model lineup; uncurated routes remain inline-edit-only.
func (m PresetEditorModel) isCyclable(f editorField) bool {
	switch f {
	case feProvider, feAPICompat, feTier:
		return true
	case feServiceTier:
		return m.isCodexProvider()
	case feThinking:
		return m.hasThinking()
	case feWireAPI:
		return m.isCustomOpenAI()
	case feResponsesTransport:
		return m.isCustomOpenAIResponses()
	case feAPIKey:
		// For codex, the "API key" row is an account selector: ←/→ binds the
		// preset to a different Codex OAuth account when more than one exists.
		return m.isCodexProvider() && len(m.codexAccountRefs()) > 1
	case feModel:
		provider := asString(m.llmMap()["provider"])
		baseURL := asString(m.llmMap()["base_url"])
		return len(modelOptions(provider, baseURL)) > 0
	case feBaseURL:
		provider := asString(m.llmMap()["provider"])
		_, hasRegions := preset.ProviderRegionURLs[provider]
		return hasRegions
	}
	return false
}

func (m PresetEditorModel) sectionHeader(label string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true).Render("── " + label + " ──")
}

func (m PresetEditorModel) tierDisplay() string {
	if m.working.Description.Tier == "" {
		return ""
	}
	return tierChipStyle(m.working.Description.Tier).Render(tierLabel(m.working.Description.Tier, m.lang))
}

// renderPreview is the right-hand pane: live JSON + validation status.
func (m PresetEditorModel) renderPreview(width, height int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("245")).
		Width(width).
		Height(height).
		Padding(0, 1)

	js, _ := json.MarshalIndent(m.working, "", "  ")
	preview := string(js)
	// Truncate overly long previews — the form is the source of truth,
	// the preview is for orientation. Width-trim happens via lipgloss.
	maxLines := height - 8
	if maxLines < 4 {
		maxLines = 4
	}
	lines := strings.Split(preview, "\n")
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], "  …")
	}
	preview = strings.Join(lines, "\n")

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true).Render("── JSON ──"))
	b.WriteString("\n")
	b.WriteString(preview)
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true).Render("── " + i18n.T("preset_editor.validation") + " ──"))
	b.WriteString("\n")
	if errs := m.working.Validate(); len(errs) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("84")).Render("✓ " + i18n.T("preset_editor.valid")))
	} else {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		for _, e := range errs {
			b.WriteString(errStyle.Render("✗ "+e.Error()) + "\n")
		}
	}
	return box.Render(b.String())
}

func (m *PresetEditorModel) moveCursor(delta int) {
	if delta == 0 {
		m.normalizeCursor()
		m.ensureFocusedVisible()
		return
	}
	step := 1
	if delta < 0 {
		step = -1
		delta = -delta
	}
	for i := 0; i < delta; i++ {
		next := m.cursor + step
		for next >= 0 && next <= saveFieldIndex && !m.fieldVisible(editorFieldOrder[next]) {
			next += step
		}
		if next < 0 || next > saveFieldIndex {
			break
		}
		m.cursor = next
	}
	m.normalizeCursor()
	m.ensureFocusedVisible()
}

func (m *PresetEditorModel) ensureFocusedVisible() {
	m.normalizeCursor()
	if m.width == 0 || m.height == 0 {
		return
	}
	rows := m.formRows(m.width)
	focused := m.focusedRowIndex(rows)
	if focused < 0 {
		return
	}
	visibleRows := m.visibleFormRows()
	if visibleRows < 1 {
		visibleRows = 1
	}
	if focused < m.scrollOffset {
		m.scrollOffset = focused
	} else if focused >= m.scrollOffset+visibleRows {
		m.scrollOffset = focused - visibleRows + 1
	}
	if maxStart := maxScrollStart(len(rows), visibleRows); m.scrollOffset > maxStart {
		m.scrollOffset = maxStart
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m PresetEditorModel) fieldVisible(f editorField) bool {
	switch f {
	case feServiceTier:
		return m.isCodexProvider()
	case feThinking:
		return m.hasThinking()
	case feWireAPI:
		return m.isCustomOpenAI()
	case feResponsesTransport:
		return m.isCustomOpenAIResponses()
	default:
		return true
	}
}

func (m *PresetEditorModel) normalizeCursor() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > saveFieldIndex {
		m.cursor = saveFieldIndex
	}
	if m.fieldVisible(editorFieldOrder[m.cursor]) {
		return
	}
	for i := m.cursor + 1; i <= saveFieldIndex; i++ {
		if m.fieldVisible(editorFieldOrder[i]) {
			m.cursor = i
			return
		}
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if m.fieldVisible(editorFieldOrder[i]) {
			m.cursor = i
			return
		}
	}
}

func (m PresetEditorModel) focusedRowIndex(rows []presetEditorRow) int {
	if m.cursor < 0 || m.cursor >= len(editorFieldOrder) {
		return -1
	}
	focusedField := editorFieldOrder[m.cursor]
	for i, row := range rows {
		if row.hasField && row.field == focusedField {
			return i
		}
	}
	return -1
}

func (m PresetEditorModel) visibleFormRows() int {
	return formContentHeight(m.height - 4)
}

func formContentHeight(boxHeight int) int {
	// renderForm applies a rounded border and no vertical padding, so the
	// interior content is the requested box height minus top/bottom border.
	rows := boxHeight - 2
	if rows < 1 {
		return 1
	}
	return rows
}

func maxScrollStart(rowCount, visibleRows int) int {
	if visibleRows < 1 {
		visibleRows = 1
	}
	if rowCount <= visibleRows {
		return 0
	}
	return rowCount - visibleRows
}

func (m PresetEditorModel) renderFooter() string {
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	if m.saveErr != "" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("  " + m.saveErr)
	}
	switch m.mode {
	case emInline:
		return hintStyle.Render("  " + i18n.T("preset_editor.hint_inline"))
	case emDirtyPrompt:
		return hintStyle.Render("  " + i18n.T("preset_editor.hint_dirty"))
	case emExitPrompt:
		return hintStyle.Render("  " + i18n.T("preset_editor.hint_exit"))
	}
	hint := i18n.T("preset_editor.hint_browse")
	if m.statusMsg != "" {
		// Transient feedback (e.g. a completed CLI credential import)
		// replaces the generic browse hint until the next keypress.
		hint = m.statusMsg
	} else if m.codexCLIImportAvailable() {
		// A `codex login` credential exists while this preset's bound
		// account is invalid: advertise the one-click import on the
		// codex API-key row.
		hint += "  " + i18n.T("codex.import_cli_hint")
	}
	return hintStyle.Render("  " + hint)
}

func (m PresetEditorModel) renderCloneOverlay(_ string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	body := titleStyle.Render(i18n.T("preset_editor.clone_title")) + "\n\n" +
		i18n.T("preset_editor.clone_explain") + "\n\n" +
		subtle.Render("name: ") + m.cloneNameInput.View() + "\n\n" +
		subtle.Render(i18n.T("preset_editor.clone_hint"))
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m PresetEditorModel) renderDirtyOverlay(_ string) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(1, 2).
		Render(i18n.T("preset_editor.dirty_prompt") + "\n\n" +
			lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("[y] "+i18n.T("preset_editor.discard")+
				"   [n/Esc] "+i18n.T("preset_editor.cancel_discard")))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, style)
}

func (m PresetEditorModel) renderExitOverlay(_ string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	body := titleStyle.Render(i18n.T("preset_editor.exit_title")) + "\n\n" +
		subtle.Render(i18n.T("preset_editor.exit_hint"))
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// renderSaveButton emits the save row at the bottom of the form. When
// the cursor is on it, the row pops in accent color; Enter triggers
// commit. Acts like a button users can find by tabbing down.
func (m PresetEditorModel) renderSaveButton() string {
	focused := editorFieldOrder[m.cursor] == feSave
	label := "[ " + i18n.T("preset_editor.save_button") + " ]"
	if focused {
		return "▸ " + lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent).
			Render(label)
	}
	return "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(label)
}

// ───────────────────────────────────────────────────────────────────────────
// Private helpers
// ───────────────────────────────────────────────────────────────────────────

// clonePresetForEditor deep-copies a Preset via JSON round-trip so the
// editor's working copy doesn't share map references with the caller.
// preset.Clone changes the Name; we preserve every serialized field here.
// Source is runtime-only and is deliberately re-established by commitCmd.
func clonePresetForEditor(p preset.Preset) preset.Preset {
	data, err := json.Marshal(p)
	if err != nil {
		return p
	}
	var out preset.Preset
	if err := preset.DecodeJSONUseNumber(data, &out); err != nil {
		return p
	}
	return out
}

func asBool(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

func asExtra(extra map[string]interface{}, key string) string {
	if extra == nil {
		return ""
	}
	s, _ := extra[key].(string)
	return s
}

// maskAPIKey returns a display form for an API key — the last 4 chars
// preceded by ••• padding, or the i18n placeholder when empty. We never
// show the full key on screen; pasting a new value triggers a fresh
// edit which then masks again on commit.
func maskAPIKey(key string) string {
	if key == "" {
		return i18n.T("preset_editor.api_key_unset")
	}
	if len(key) <= 4 {
		return strings.Repeat("•", len(key))
	}
	return "••••••••" + key[len(key)-4:]
}

// usesRegionDeclaredEnv reports whether llm's current api_key_env is a
// CROSS-PROVIDER shared credential its current base_url's region row declares
// (today only OpenCode Go -> OPENCODE_GO_API_KEY). Picking such a row means
// "resolve through that one account", so commit() must not strip the slot from
// an edited built-in and mint an unrelated PROVIDER_N one.
//
// The provider's own ProviderDefaultEnv is deliberately excluded even when a
// region row declares it (deepseek's DeepSeek API row declares
// DEEPSEEK_API_KEY): that is the template's shared slot, exactly what
// per-preset numbering exists to replace, so keeping it would let a second
// deepseek preset overwrite the first one's key. A free-typed URL, an Env-less
// region row (zhipu/minimax CN/INTL), or a mismatched slot all return false.
func usesRegionDeclaredEnv(llm map[string]interface{}) bool {
	env := asString(llm["api_key_env"])
	baseURL := asString(llm["base_url"])
	if env == "" || baseURL == "" {
		return false
	}
	provider := asString(llm["provider"])
	for _, r := range preset.ProviderRegionURLs[provider] {
		if r.URL == baseURL && r.Env != "" {
			return r.Env == env && r.Env != preset.ProviderDefaultEnv[provider]
		}
	}
	return false
}

// regionDeclaredEnv reports whether env is a credential slot ANY region row of
// provider declares, regardless of which row is currently selected. This is the
// state-shaped question cycleFocused asks when landing on an Env-less row: a
// preset resolving through a row-declared slot while pointing somewhere that
// row does not cover is the thing the restore exists to undo, and keying the
// restore on the state rather than on the previously selected row also covers
// selectedRegionIndex == -1 (an off-list base_url on a provider with no Custom
// row).
func regionDeclaredEnv(provider string, env string) bool {
	if env == "" {
		return false
	}
	for _, r := range preset.ProviderRegionURLs[provider] {
		if r.Env == env {
			return true
		}
	}
	return false
}

// selectedRegionIndex resolves which ProviderRegionURLs option is selected
// for the given current base_url. A value matching a known non-empty URL
// selects that option; the empty-URL Custom sentinel absorbs an empty or
// free-typed value.
//
// Returns -1 — "no region selected" — when the value matches nothing and the
// provider has no Custom row (zhipu, minimax). Callers MUST handle that case
// rather than index with it: falling back to index 0 would render a solid
// dot on CN for a preset that points somewhere else entirely, stating
// positively something that is false. All consumers (openInline,
// baseURLRadioStrip, cycleFocused) share this rule so the strip's
// highlighted dot, the Enter no-op, and the cycle always agree.
func selectedRegionIndex(regions []preset.RegionURL, current string) int {
	for i, r := range regions {
		if r.URL != "" && r.URL == current {
			return i
		}
	}
	for i, r := range regions {
		if r.URL == "" {
			return i
		}
	}
	return -1
}

// cycleString rotates `cur` through `opts` by `dir` steps. Unknown
// values land at index 0 on +1, last index on -1.
func cycleString(opts []string, cur string, dir int) string {
	idx := 0
	for i, v := range opts {
		if v == cur {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(opts)) % len(opts)
	return opts[idx]
}
