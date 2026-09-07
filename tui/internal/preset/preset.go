package preset

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/anthropics/lingtai-tui/internal/config"
	lingfs "github.com/anthropics/lingtai-tui/internal/fs"
)

//go:embed all:covenant
var covenantFS embed.FS

//go:embed all:principle
var principleFS embed.FS

// Procedures are not localized — a single procedures.md lives at the root.
// ProceduresPath() checks for a lang-specific override first (<lang>/procedures.md),
// then falls back to the root file. To add a localized version in the future,
// create procedures/<lang>/procedures.md here and it will take precedence.
//
//go:embed all:procedures
var proceduresFS embed.FS

//go:embed all:templates
var templatesFS embed.FS

//go:embed all:soul
var soulFS embed.FS

//go:embed all:recipe_assets
var recipeAssetsFS embed.FS

// Note: no `all:` prefix — Go's default embed semantics exclude leading-dot
// and leading-underscore files/dirs, which keeps `.pytest_cache/` and
// `__pycache__/` out of the binary even when they exist on disk at build
// time. Skills MUST NOT ship legitimately-dot-prefixed content; if they
// need to, switch back to `all:skills` and add a runtime filter (or use a
// distinct subdir name).
//
//go:embed skills
var skillsFS embed.FS

// Preset is a reusable agent bundle. Templates ship with the TUI and
// live under ~/.lingtai-tui/presets/templates/ (regenerated on every
// launch, never user-edited). User-saved variants live under
// ~/.lingtai-tui/presets/saved/. The two directories are the only
// thing distinguishing a template from a user preset — there is no
// in-band marker.
//
// Description is a structured object with a required `summary` and an
// optional `tier` (cost/quality ladder, "1".."5"). Authors may add
// arbitrary extra keys (gains/loses/recommended_for/...); they round-trip
// through `Description.Extra`.
type Preset struct {
	Name        string                 `json:"name"`
	Description PresetDescription      `json:"description"`
	Manifest    map[string]interface{} `json:"manifest"`

	// Source is set by List/Load to record where the preset was read
	// from on disk. Runtime-only — never marshaled. Callers use this
	// instead of name-matching to ask "is this a template?".
	Source PresetSource `json:"-"`
}

// PresetSource records which directory a preset was read from. The
// directory IS the answer to "is this a template?" — no in-band marker,
// no name list to maintain.
type PresetSource int

const (
	// SourceUnknown is the zero value; in-memory presets that were never
	// loaded from disk have this. Treat as "saved" for safety so a
	// hand-built preset can't accidentally claim template status.
	SourceUnknown PresetSource = iota
	// SourceTemplate means the preset lives under presets/templates/.
	// Read-only from the TUI's perspective: the user edits a template
	// to materialize a saved variant; the template itself is rewritten
	// from embedded data on every launch.
	SourceTemplate
	// SourceSaved means the preset lives under presets/saved/. User-
	// owned; never touched by Bootstrap/SeedMissingBuiltins.
	SourceSaved
)

// PresetDescription is the structured commentary block on a preset. The
// kernel requires a non-empty summary; tier is optional but when present
// must be one of "1".."5".
//
// Extra holds any author-authored keys beyond summary/tier that the
// kernel surfaces verbatim to the agent. They round-trip through marshal
// so editing a preset in the TUI doesn't drop extra prose.
type PresetDescription struct {
	Summary string
	Tier    string
	Extra   map[string]interface{}
}

// MarshalJSON flattens Summary, Tier, and Extra into a single JSON object.
// Summary is always emitted (even when empty) because the kernel requires
// the key. Tier is omitted when empty. Extra keys are emitted last; they
// don't override Summary or Tier.
func (d PresetDescription) MarshalJSON() ([]byte, error) {
	out := make(map[string]interface{}, 2+len(d.Extra))
	for k, v := range d.Extra {
		if k == "summary" || k == "tier" {
			continue
		}
		out[k] = v
	}
	out["summary"] = d.Summary
	if d.Tier != "" {
		out["tier"] = d.Tier
	}
	return json.Marshal(out)
}

// UnmarshalJSON accepts the structured object form. A bare-string
// description (legacy on-disk shape) is wrapped as {summary: "<str>"}
// so older files load without forcing a migration pass on every read.
func (d *PresetDescription) UnmarshalJSON(data []byte) error {
	// String form: {"description": "..."} — wrap it.
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		d.Summary = asString
		d.Tier = ""
		d.Extra = nil
		return nil
	}
	var asMap map[string]interface{}
	if err := DecodeJSONUseNumber(data, &asMap); err != nil {
		return err
	}
	if v, ok := asMap["summary"].(string); ok {
		d.Summary = v
	}
	if v, ok := asMap["tier"].(string); ok {
		d.Tier = v
	}
	delete(asMap, "summary")
	delete(asMap, "tier")
	if len(asMap) > 0 {
		d.Extra = asMap
	} else {
		d.Extra = nil
	}
	return nil
}

// PresetsDir returns the parent directory ~/.lingtai-tui/presets/.
// The TUI only writes to its templates/ and saved/ subdirectories;
// PresetsDir itself stays around for the kernel-side migration meta
// file (kept at the parent to survive template re-extraction).
func PresetsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, config.GlobalDirName, "presets")
}

// TemplatesDir returns ~/.lingtai-tui/presets/templates/. The TUI
// regenerates these wholesale on every launch from embedded data —
// users should never edit files here directly.
func TemplatesDir() string {
	return filepath.Join(PresetsDir(), "templates")
}

// SavedDir returns ~/.lingtai-tui/presets/saved/. User territory:
// every Save() lands here, Bootstrap/Seed never touch it.
func SavedDir() string {
	return filepath.Join(PresetsDir(), "saved")
}

// listFromDir reads every *.json file from a single preset directory
// and stamps each result with the given source. Internal helper for
// List(); centralizes the parse-and-skip-malformed logic so the two
// directory walks can't drift.
func listFromDir(dir string, src PresetSource) ([]Preset, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Preset
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		if e.Name() == "_kernel_meta.json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		p, err := loadFromPath(path)
		if err != nil {
			if errors.Is(err, ErrCapabilityConflict) {
				return nil, err
			}
			continue
		}
		p.Source = src
		out = append(out, p)
	}
	return out, nil
}

// List returns saved presets first (alphabetical), then templates in
// the canonical product order. Each preset carries a Source field
// recording which directory it came from — callers should prefer
// p.Source over name-matching when asking "is this a template?".
func List() ([]Preset, error) {
	saved, err := listFromDir(SavedDir(), SourceSaved)
	if err != nil {
		return nil, fmt.Errorf("list saved presets: %w", err)
	}
	templates, err := listFromDir(TemplatesDir(), SourceTemplate)
	if err != nil {
		return nil, fmt.Errorf("list template presets: %w", err)
	}

	sort.Slice(saved, func(i, j int) bool {
		return saved[i].Name < saved[j].Name
	})
	templateOrder := map[string]int{
		"minimax": 0, "zhipu": 1, "mimo": 2, "deepseek": 3,
		"kimi": 4, "grok": 5, "nvidia": 6, "openrouter": 7,
		"codex": 8, "codex-pool": 9, "claude": 10, "custom": 11,
	}
	sort.Slice(templates, func(i, j int) bool {
		return templateOrder[templates[i].Name] < templateOrder[templates[j].Name]
	})

	return append(saved, templates...), nil
}

// HasAny returns true if at least one preset exists.
func HasAny() bool {
	presets, _ := List()
	return len(presets) > 0
}

// First returns the first available preset, or an empty Preset if none exist.
func First() Preset {
	presets, _ := List()
	if len(presets) > 0 {
		return presets[0]
	}
	return Preset{Manifest: map[string]interface{}{}}
}

// Load reads a single preset by name. Looks in saved/ first, then
// templates/ — a saved preset with the same name as a template wins
// (the user's variant overrides). Returns the loaded preset with
// Source populated.
//
// A directory is only skipped when its file is genuinely absent
// (errors.Is(..., fs.ErrNotExist) through loadFromPath's %w wrapping).
// Any other error — invalid JSON, permission/read failure, a path that is a
// directory — is
// returned to the caller with the underlying cause preserved instead of
// being collapsed into a generic "preset not found", so a broken file is
// distinguishable from a missing one (issue #483). Only when BOTH the
// saved and template files are absent do we return the not-found error.
func Load(name string) (Preset, error) {
	for _, attempt := range []struct {
		dir string
		src PresetSource
	}{
		{SavedDir(), SourceSaved},
		{TemplatesDir(), SourceTemplate},
	} {
		path := filepath.Join(attempt.dir, name+".json")
		p, err := loadFromPath(path)
		if err == nil {
			p.Source = attempt.src
			return p, nil
		}
		// Genuinely missing here — fall through to the next directory.
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		// A real failure (bad JSON, unreadable file, path is a directory):
		// surface it rather than masking it as not-found.
		return Preset{}, fmt.Errorf("load preset %q: %w", name, err)
	}
	return Preset{}, fmt.Errorf("preset not found: %s", name)
}

// loadFromPath reads + parses a preset file. Internal helper.
func loadFromPath(path string) (Preset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Preset{}, fmt.Errorf("read preset %s: %w", path, err)
	}
	var p Preset
	if err := DecodeJSONUseNumber(data, &p); err != nil {
		return Preset{}, fmt.Errorf("parse preset %s: %w", path, err)
	}
	p.NormalizeLegacyContextLimit()
	if err := p.NormalizeLegacyCapabilities(); err != nil {
		return Preset{}, fmt.Errorf("canonicalize preset capabilities: %w", err)
	}
	return p, nil
}

// NormalizeLegacyContextLimit accepts the old saved-preset shape where
// context_limit lived at manifest.context_limit. The canonical location is
// manifest.llm.context_limit; if both locations are present, keep the
// canonical llm value and drop the legacy root key.
func (p *Preset) NormalizeLegacyContextLimit() {
	if p == nil || p.Manifest == nil {
		return
	}
	rootCtx, hasRoot := p.Manifest["context_limit"]
	if hasRoot {
		delete(p.Manifest, "context_limit")
	}
	llm, _ := p.Manifest["llm"].(map[string]interface{})
	if llm == nil {
		return
	}

	// context_limit predates the exact-token capability path and existing
	// callers expect this known integer field as float64 after loading. Keep the
	// compatibility representation for both canonical and legacy-root input;
	// capability values remain json.Number through canonicalization and writes.
	if canonical, hasCanonical := llm["context_limit"]; hasCanonical {
		if number, ok := canonical.(json.Number); ok {
			if value, err := strconv.ParseFloat(string(number), 64); err == nil {
				llm["context_limit"] = value
			}
		}
		return
	}
	if !hasRoot {
		return
	}
	if number, ok := rootCtx.(json.Number); ok {
		if value, err := strconv.ParseFloat(string(number), 64); err == nil {
			rootCtx = value
		}
	}
	llm["context_limit"] = rootCtx
}

// validTiers mirrors the kernel-side TIER_VALUES in lingtai/presets.py.
var validTiers = map[string]bool{"1": true, "2": true, "3": true, "4": true, "5": true}

// Validate returns the list of rule violations for this preset. Mirrors the
// kernel's load_preset validation gauntlet so the editor refuses to save
// anything the kernel will refuse to load. Empty slice = passes.
func (p Preset) Validate() []error {
	p.NormalizeLegacyContextLimit()
	var errs []error
	if err := p.NormalizeLegacyCapabilities(); err != nil {
		errs = append(errs, err)
	}
	if p.Description.Summary == "" {
		errs = append(errs, fmt.Errorf("description.summary must be non-empty"))
	}
	if p.Description.Tier != "" && !validTiers[p.Description.Tier] {
		errs = append(errs, fmt.Errorf("description.tier must be one of 1..5 (got %q)", p.Description.Tier))
	}
	llm, _ := p.Manifest["llm"].(map[string]interface{})
	if llm == nil {
		errs = append(errs, fmt.Errorf("manifest.llm must be an object"))
	} else {
		if s, _ := llm["provider"].(string); s == "" {
			errs = append(errs, fmt.Errorf("manifest.llm.provider must be non-empty"))
		}
		if s, _ := llm["model"].(string); s == "" {
			errs = append(errs, fmt.Errorf("manifest.llm.model must be non-empty"))
		}
		// A provider with a region table reaches its API through an explicit
		// endpoint — there is no implicit default in the kernel adapter. The
		// editor's Custom sentinel deliberately clears base_url so Enter opens
		// a blank inline edit, and nothing forces the user to type one (Esc
		// leaves it empty), so without this check two arrow presses and a save
		// persist a preset with no endpoint at all.
		//
		// Only an explicitly EMPTY value is a violation. A preset that simply
		// omits the key (hand-edited or recipe-imported saved presets, or the
		// pre-region-table wizard shape) is tolerated when regionSuffix can
		// assign a default region (zhipu → CN, minimax → INTL): AutoEnvVarName
		// stamps a slot for it, so such a preset remains well-defined. A
		// region-table provider without a region fallback (deepseek, kimi,
		// mimo, grok) still needs an explicit endpoint, so an absent key stays
		// a violation there. kimi and mimo joined that set when they gained a
		// region table — a base_url-less kimi/mimo preset that validated on an
		// older TUI must gain an explicit endpoint to save again.
		// The editor's Custom sentinel is the only path that writes an
		// explicit "", and that is always rejected.
		if provider, _ := llm["provider"].(string); provider != "" {
			if _, hasRegions := ProviderRegionURLs[provider]; hasRegions {
				if v, present := llm["base_url"]; present {
					if s, _ := v.(string); s == "" {
						errs = append(errs, fmt.Errorf(
							"manifest.llm.base_url must be non-empty for provider %q", provider))
					}
				} else if regionSuffix(provider, "") == "" {
					errs = append(errs, fmt.Errorf(
						"manifest.llm.base_url must be non-empty for provider %q", provider))
				}
			}
		}
		if v, ok := llm["context_limit"]; ok && v != nil {
			// JSON unmarshals numbers as float64; accept int-valued floats.
			switch n := v.(type) {
			case float64:
				if n != float64(int(n)) || n <= 0 {
					errs = append(errs, fmt.Errorf("manifest.llm.context_limit must be a positive integer"))
				}
			case int:
				if n <= 0 {
					errs = append(errs, fmt.Errorf("manifest.llm.context_limit must be a positive integer"))
				}
			case json.Number:
				parsed, err := strconv.ParseInt(string(n), 10, 64)
				if err != nil || parsed <= 0 {
					errs = append(errs, fmt.Errorf("manifest.llm.context_limit must be a positive integer"))
				}
			default:
				errs = append(errs, fmt.Errorf("manifest.llm.context_limit must be a positive integer"))
			}
		}
	}
	if _, hasRootCtx := p.Manifest["context_limit"]; hasRootCtx {
		errs = append(errs, fmt.Errorf("context_limit must live inside manifest.llm, not at manifest root"))
	}
	if caps, ok := p.Manifest["capabilities"]; ok {
		if _, isMap := caps.(map[string]interface{}); !isMap {
			errs = append(errs, fmt.Errorf("manifest.capabilities must be an object"))
		}
	}
	return errs
}

// ValidateSafeName rejects any name that isn't a single, non-empty path
// segment safe to use as a directory name or filename stem beneath an
// owning directory. It rejects blank values, "." and "..", and any path
// separator — both "/" and "\" — so escapes are blocked regardless of the
// platform the binary runs on (POSIX treats "\" as a literal filename
// character; Windows treats it as a separator). A name that passes is
// guaranteed to stay a direct child of whatever base directory it is
// joined to (issue #849).
func ValidateSafeName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("must not be blank")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("must not be %q", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("must not contain a path separator")
	}
	return nil
}

// Save writes a preset to the saved/ directory. Save NEVER writes to
// templates/ — that's owned by Bootstrap. Callers that want to seed
// a template must use writeTemplate (preset package internal).
func Save(p Preset) error {
	// The preset name becomes the filename stem under SavedDir(); enforce
	// the owning-layer containment invariant here so every caller — current
	// and future — is protected (issue #849).
	if err := ValidateSafeName(p.Name); err != nil {
		return fmt.Errorf("invalid preset name %q: %w", p.Name, err)
	}
	if err := p.NormalizeLegacyCapabilities(); err != nil {
		return fmt.Errorf("canonicalize preset capabilities: %w", err)
	}
	dir := SavedDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create saved presets dir: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal preset: %w", err)
	}
	path := filepath.Join(dir, p.Name+".json")
	return os.WriteFile(path, data, 0o644)
}

// Clone creates a deep copy of a preset with a new name.
// The original preset is not modified.
func Clone(src Preset, newName string) Preset {
	// Deep copy via JSON round-trip to avoid shared map references
	manifest := make(map[string]interface{})
	if data, err := json.Marshal(src.Manifest); err == nil {
		_ = DecodeJSONUseNumber(data, &manifest)
	}
	desc := src.Description
	if src.Description.Extra != nil {
		desc.Extra = make(map[string]interface{}, len(src.Description.Extra))
		for k, v := range src.Description.Extra {
			desc.Extra[k] = v
		}
	}
	return Preset{
		Name:        newName,
		Description: desc,
		Manifest:    manifest,
	}
}

// Delete removes a saved preset. Templates are immutable from the
// user's perspective; deleting them via the TUI is a no-op (the next
// Bootstrap re-extracts the file anyway). Returns an error only when
// a saved file existed and the unlink failed.
func Delete(name string) error {
	// The name becomes the filename stem under SavedDir(); enforce the
	// owning-layer containment invariant here so Delete can never target a
	// path outside saved/ regardless of caller (issue #849).
	if err := ValidateSafeName(name); err != nil {
		return fmt.Errorf("invalid preset name %q: %w", name, err)
	}
	path := filepath.Join(SavedDir(), name+".json")
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// EnsureDefault is now a no-op kept for callers that haven't been
// updated. Templates are unconditionally rewritten by RefreshTemplates
// on every Bootstrap, and saved presets are user territory — there is
// nothing to "ensure default" anymore.
func EnsureDefault() error { return nil }

// SeedMissingBuiltins is replaced by RefreshTemplates. Kept as a thin
// alias so old callers (lingtai-claude-code, codex-plugin) that import
// the preset package don't break on upgrade.
func SeedMissingBuiltins() error { return RefreshTemplates() }

// RefreshTemplates rewrites templates/ from BuiltinPresets() wholesale.
// Called from Bootstrap on every TUI launch. Deletes any *.json file
// in templates/ that's no longer in BuiltinPresets() so a TUI upgrade
// that retires a template (e.g. an obsolete provider) propagates
// cleanly. Saved presets in saved/ are never touched.
func RefreshTemplates() error {
	dir := TemplatesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create templates dir: %w", err)
	}
	want := map[string]bool{}
	for _, p := range BuiltinPresets() {
		want[p.Name+".json"] = true
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal template %s: %w", p.Name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, p.Name+".json"), data, 0o644); err != nil {
			return fmt.Errorf("write template %s: %w", p.Name, err)
		}
	}
	// Prune retired templates.
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			if !want[e.Name()] {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	return nil
}

// RegionURL pairs a human-readable label with an API base URL.
// Env, when non-empty, is the api_key_env slot the option implies (e.g.
// "DEEPSEEK_API_KEY") and is applied to the preset when the option is
// selected. Empty Env means "don't touch api_key_env"; the editor
// restores the slot it memoized when cycling back off an Env row.
//
// Env is deliberately present ONLY where a region genuinely implies a
// distinct credential (DeepSeek API vs OpenCode Go on deepseek, and the
// OpenCode Go row on zhipu/minimax/kimi/mimo/grok, are separate accounts).
// The plain native rows (zhipu/minimax CN and INTL, Kimi Code, MiMo) declare
// none: their slots are region-suffixed and host-stamped
// (ZHIPU_INTL_1_API_KEY, MINIMAX_CN_1_API_KEY, KIMI_1_API_KEY via
// AutoEnvVarName), and a flat Env on those rows would overwrite that slot
// on every base_url cycle. Their provider default lives in
// ProviderDefaultEnv instead.
//
// The Custom sentinel (empty URL) declares no Env either, and the editor
// additionally leaves api_key_env untouched when landing on it: the user is
// about to type their own endpoint and keeps whatever slot that endpoint
// actually uses. Note the consequence for a preset arriving from an Env row —
// the shared OpenCode Go slot carries over until the user changes it.
type RegionURL struct {
	Label string // user-facing option name, e.g. "CN", "INTL", "DeepSeek API", "Custom"
	URL   string
	Env   string // optional credential env-var name; empty = leave api_key_env alone
}

// ProviderRegionURLs maps provider names to their regional endpoint
// options. Providers not in this map have a single endpoint (or none)
// and their base_url is free-text in the editor. The first entry is
// the default for new presets. An entry with an empty URL is the free-text
// "Custom" sentinel: selecting it clears base_url so the editor opens an
// inline edit for any user-typed endpoint. At most one entry per provider
// may carry an empty URL.
var ProviderRegionURLs = map[string][]RegionURL{
	"deepseek": {
		{Label: "DeepSeek API", URL: "https://api.deepseek.com", Env: "DEEPSEEK_API_KEY"},
		// OpenCode Go is scoped to DeepSeek models served through the OpenCode Go
		// subscription (provider stays "deepseek"); other Go models are reached via
		// a Custom preset.
		{Label: "OpenCode Go", URL: "https://opencode.ai/zen/go/v1", Env: "OPENCODE_GO_API_KEY"},
		{Label: "Custom", URL: ""}, // empty URL = free text sentinel
	},
	"zhipu": {
		{Label: "CN", URL: "https://open.bigmodel.cn/api/coding/paas/v4"},
		{Label: "INTL", URL: "https://api.z.ai/api/coding/paas/v4"},
		{Label: "OpenCode Go", URL: "https://opencode.ai/zen/go/v1", Env: "OPENCODE_GO_API_KEY"},
	},
	"minimax": {
		{Label: "CN", URL: "https://api.minimaxi.com/anthropic"},
		{Label: "INTL", URL: "https://api.minimax.io/anthropic"},
		{Label: "OpenCode Go", URL: "https://opencode.ai/zen/go/v1", Env: "OPENCODE_GO_API_KEY"},
	},
	// OpenCode Go serves the Kimi K-series (kimi-k3, kimi-k2.7-code, ...)
	// under lowercase model ids; the native Kimi Code endpoint serves the
	// subscription model `kimi-for-coding`. Kimi had free-text base_url
	// before it gained this table, so it carries the Custom sentinel too —
	// a region table without one removes the only in-editor path to a
	// corporate proxy or relay.
	"kimi": {
		{Label: "Kimi Code", URL: "https://api.kimi.com/coding/v1"},
		{Label: "OpenCode Go", URL: "https://opencode.ai/zen/go/v1", Env: "OPENCODE_GO_API_KEY"},
		{Label: "Custom", URL: ""}, // empty URL = free text sentinel
	},
	// OpenCode Go serves the MiMo V2 family alongside Xiaomi's own endpoint.
	// Custom sentinel for the same reason as kimi above.
	"mimo": {
		{Label: "MiMo", URL: "https://api.xiaomimimo.com/v1"},
		{Label: "OpenCode Go", URL: "https://opencode.ai/zen/go/v1", Env: "OPENCODE_GO_API_KEY"},
		{Label: "Custom", URL: ""}, // empty URL = free text sentinel
	},
	// grok is the one provider whose DEFAULT row is OpenCode Go: the TUI has
	// no native xAI route (no verified api.x.ai endpoint/model pairing), so
	// `grok-4.5` is reached only through the Go subscription. Entry [0]
	// therefore declares OPENCODE_GO_API_KEY and grokPreset() ships that same
	// slot; ProviderDefaultEnv["grok"] holds the provider-generic fallback
	// name (see ProviderDefaultEnv below).
	"grok": {
		{Label: "OpenCode Go", URL: "https://opencode.ai/zen/go/v1", Env: "OPENCODE_GO_API_KEY"},
		{Label: "Custom", URL: ""}, // empty URL = free text sentinel
	},
}

// ProviderDefaultEnv maps each provider to its provider-generic api_key_env
// fallback slot. Consulted on provider switch only; it
// must not be read from a region cycle, so zhipu/minimax region rows declare
// no Env (see RegionURL) and their CN<->INTL cycling preserves whatever
// region-suffixed slot the host stamped (ZHIPU_INTL_1_API_KEY etc.).
//
// Values match each builtin preset's declared api_key_env ("" = OAuth/CLI
// providers with no key slot, or a generic placeholder the user replaces).
//
// grok is the single deliberate exception: grokPreset() declares
// OPENCODE_GO_API_KEY because its default (and only non-Custom) endpoint IS
// OpenCode Go, so an existing Go user needs no second copy of the key. The
// entry below is the provider-generic fallback name; the editor adopts the
// landing row's slot on a switch. Keeping the two different is what makes
// usesRegionDeclaredEnv treat the Go slot as a cross-provider account worth
// preserving across a save, instead of as this template's own shared slot.
var ProviderDefaultEnv = map[string]string{
	"minimax":    "MINIMAX_API_KEY",
	"zhipu":      "ZHIPU_API_KEY",
	"mimo":       "XIAOMI_API_KEY",
	"deepseek":   "DEEPSEEK_API_KEY",
	"gemini":     "GEMINI_API_KEY",
	"kimi":       "KIMI_CODE_API_KEY",
	"grok":       "GROK_API_KEY",
	"nvidia":     "NVIDIA_API_KEY",
	"openrouter": "OPENROUTER_API_KEY",
	"claude":     "",            // OAuth / CLI login
	"codex":      "",            // OAuth
	"codex-pool": "",            // OAuth
	"custom":     "LLM_API_KEY", // generic placeholder, user replaces
}

// BuiltinPresets returns the built-in presets.
func BuiltinPresets() []Preset {
	return []Preset{
		minimaxPreset(),
		zhipuPreset(),
		mimoPreset(),
		deepseekPreset(),
		geminiPreset(),
		kimiPreset(),
		grokPreset(),
		nvidiaPreset(),
		openrouterPreset(),
		codexPreset(),
		codexPoolPreset(),
		claudePreset(),
		customPreset(),
	}
}

// builtinNames is the set of built-in template names. Used by m030 to
// classify legacy files in presets/ during the directory split, and by
// IsBuiltin (which exists for callers that only have a Name, not a
// loaded Preset).
// Legacy provider spellings remain recognized for flat-layout migration.
var builtinNames = map[string]bool{
	"minimax":          true,
	"zhipu":            true,
	"mimo":             true,
	"deepseek":         true,
	"gemini":           true,
	"kimi":             true,
	"grok":             true,
	"nvidia":           true,
	"openrouter":       true,
	"codex":            true,
	"codex_oauth":      true,
	"codex-pool":       true,
	"codex_pool":       true,
	"claude":           true,
	"claude-agent-sdk": true,
	"claude_agent_sdk": true,
	"custom":           true,
}

// IsBuiltin reports whether `name` matches a TUI-shipped template.
// Prefer IsTemplate(p) when you have a loaded Preset — that uses the
// directory-of-origin and is robust against a user saving a preset
// under a name that happens to match a template.
func IsBuiltin(name string) bool {
	return builtinNames[name]
}

// IsTemplate reports whether the given preset was loaded from the
// templates/ directory. Use this in preference to IsBuiltin(p.Name)
// for any loaded preset — it's the canonical "is this read-only?"
// answer.
func IsTemplate(p Preset) bool {
	return p.Source == SourceTemplate
}

// RefFor returns the home-shortened on-disk path string this preset
// gets recorded as in init.json's manifest.preset.{default,active,
// allowed}. Templates resolve under presets/templates/, saved under
// presets/saved/. Presets without a Source (in-memory only, e.g.
// tests) fall back to the IsBuiltin name list.
func RefFor(p Preset) string {
	if p.Name == "" {
		return ""
	}
	subdir := "saved"
	switch p.Source {
	case SourceTemplate:
		subdir = "templates"
	case SourceSaved:
		subdir = "saved"
	default:
		if IsBuiltin(p.Name) {
			subdir = "templates"
		}
	}
	return "~/.lingtai-tui/presets/" + subdir + "/" + p.Name + ".json"
}

// isSyntheticPreset reports whether p is the in-memory-only "Keep current
// preset" sentinel that NewSetupModeModel builds from an existing init.json.
// It has no backing file on disk; RefFor would otherwise derive a non-existent
// path like ~/.lingtai-tui/presets/saved/keep_current.json.
//
// Be deliberately narrow here: SourceUnknown also appears on hand-built presets
// in tests or callers that intentionally want RefFor's saved/ fallback.
func isSyntheticPreset(p Preset) bool {
	return p.Source == SourceUnknown && p.Name == "keep_current"
}

// ResolvedRef is a single entry in ResolveRefs's output. It captures
// everything a UI surface (the kanban Presets section in particular)
// needs to render an at-a-glance health check for a preset path
// recorded in manifest.preset.{default,active,allowed}.
// CredentialFamily is the narrow set of provider-owned authentication
// families understood by the TUI. Keep this classifier here so every UI
// surface consumes the manifest provider rather than reimplementing alias
// lists or inferring auth from filenames.
type CredentialFamily string

const (
	CredentialFamilyOther       CredentialFamily = "other"
	CredentialFamilyCodexSingle CredentialFamily = "codex_single"
	CredentialFamilyCodexPool   CredentialFamily = "codex_pool"
	CredentialFamilyClaudeCLI   CredentialFamily = "claude_cli"
)

// ClassifyCredentialFamily classifies only exact manifest.llm.provider values.
// Unknown values, including path/filename-like strings, remain Other.
func ClassifyCredentialFamily(provider string) CredentialFamily {
	switch provider {
	case "codex", "codex_oauth":
		return CredentialFamilyCodexSingle
	case "codex-pool", "codex_pool":
		return CredentialFamilyCodexPool
	case "claude-code", "claude_code", "claude-agent-sdk", "claude_agent_sdk":
		return CredentialFamilyClaudeCLI
	default:
		return CredentialFamilyOther
	}
}

// ResolvedRef is a credential-aware view of one preset reference.
type ResolvedRef struct {
	// Ref is the original input string (e.g. "~/.lingtai-tui/presets/templates/mimo.json").
	Ref string
	// Name is the preset's filename stem (e.g. "mimo"). Empty when Ref
	// is malformed.
	Name string
	// Source is SourceTemplate when the resolved path lives under a
	// /templates/ segment, SourceSaved when it lives under /saved/,
	// SourceUnknown otherwise (legacy flat layout, custom user path).
	Source PresetSource
	// Exists reports whether the path exists.
	Exists bool
	// ManifestValid reports that the existing path was successfully loaded.
	// A false value is deliberately not interpreted as a legacy Codex preset.
	ManifestValid bool
	// Provider and Family come only from the loaded manifest provider.
	Provider string
	Family   CredentialFamily
	// HasKey reports whether the preset's credential is actually
	// configured. For a preset with a non-empty api_key_env, this is true
	// only when that env var has a value in the passed existingKeys map.
	// For a codex preset (provider "codex", which uses ChatGPT OAuth and
	// declares no api_key_env), this is true only when OAuth is configured
	// (see AuthState.CodexOAuthConfigured). For a codex-pool preset, this is
	// true only when the caller proves a usable member in the applicable pool
	// category, or a validated empty pool can use the legacy fallback. For a Claude preset
	// (provider "claude-code"/"claude_code", which authenticates through the
	// local Claude Code CLI login and declares no api_key_env),
	// this is true only when the CLI reports a logged-in session (see
	// AuthState.ClaudeCodeAuthConfigured). A preset with an empty
	// api_key_env that is not one of those OAuth/CLI providers has no
	// configured credential, so this is false. Only meaningful when Exists
	// is true.
	HasKey bool
	// CodexAuthRef is the codex preset's manifest.llm.codex_auth_path value
	// (verbatim, possibly ""). Empty with the field omitted or literal "" means
	// the preset accepts any usable stored Codex account (legacy file or any
	// per-account file); empty from a present wrong-type/whitespace value means
	// the preset fails closed (see ResolvePresetWithAuth). Only set for codex
	// presets; "" for all others. Lets a UI surface which account a codex
	// preset is bound to without re-reading the preset file.
	CodexAuthRef string
}

// AuthState carries machine-level credential facts the credential guard
// cannot derive from a preset file alone. Ordinary Codex OAuth, pool
// membership, and fallback readiness remain separate facts.
type AuthState struct {
	// CodexOAuthConfigured is the caller-provided fallback signal for a codex
	// preset that declares no manifest.llm.codex_auth_path when CodexAuthDir is
	// empty. Callers that know the token store directory should set CodexAuthDir
	// so the preset package can inspect legacy and per-account token files.
	CodexOAuthConfigured bool

	// CodexAuthDir is the directory (typically ~/.lingtai-tui) that
	// per-account codex_auth_path values resolve against when they are
	// relative or "~/"-prefixed. When set, explicit manifest.llm.codex_auth_path
	// values validate exactly that bound token file, so multiple Codex accounts
	// are judged independently. An empty codex_auth_path means "default Codex
	// credentials" and is valid when either the legacy token or any per-account
	// token under codex-auth/ is usable. Empty falls back to CodexOAuthConfigured.
	CodexAuthDir string

	// CodexPoolEligible says that a flat pool has a usable positively weighted
	// member, or that the validated empty pool may use the legacy fallback.
	CodexPoolEligible bool

	// CodexPoolEligibleModels is non-nil for a model-classified pool. It is
	// keyed by exact model; absent keys are false and do not fall back to the
	// flat fact. CodexPoolFallbackEligible covers an absent/empty applicable
	// category when the legacy token is valid.
	CodexPoolEligibleModels   map[string]bool
	CodexPoolFallbackEligible bool

	// ClaudeCodeAuthConfigured is true when the local Claude Code CLI
	// (`claude`) is installed and reports a logged-in session. The
	// claude-code provider authenticates through that existing CLI login
	// (no per-request API key, no separate token stored by the TUI), so a
	// Claude preset is credential-valid only when this is
	// true. Computed by the caller (see tui.claudeCodeAuthConfigured) and
	// passed in to avoid the preset→tui import cycle.
	ClaudeCodeAuthConfigured bool
}

// codexTokenFileValid reports whether the Codex OAuth token file at the
// resolved path parses and carries a non-empty refresh_token. Mirrors the
// tui package's readCodexTokenFile check but lives here to avoid the
// preset→tui import cycle. Token material is never returned or logged.
func codexTokenFileValid(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var tok struct {
		RefreshToken string `json:"refresh_token"`
	}
	return json.Unmarshal(data, &tok) == nil && strings.TrimSpace(tok.RefreshToken) != ""
}

func anyCodexTokenFileValid(authDir string) bool {
	if codexTokenFileValid(filepath.Join(authDir, "codex-auth.json")) {
		return true
	}
	entries, err := os.ReadDir(filepath.Join(authDir, "codex-auth"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		if codexTokenFileValid(filepath.Join(authDir, "codex-auth", e.Name())) {
			return true
		}
	}
	return false
}

// resolveCodexAuthRef expands a preset's manifest.llm.codex_auth_path against
// authDir. Empty ref → the legacy ~/.lingtai-tui/codex-auth.json fallback.
// "~/"-prefixed and absolute refs are honored; a bare relative value resolves
// under authDir so it still lands in the TUI-owned tree.
func resolveCodexAuthRef(authDir, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return filepath.Join(authDir, "codex-auth.json")
	}
	if strings.HasPrefix(ref, "~/") || ref == "~" {
		return expandUserPath(ref)
	}
	if filepath.IsAbs(ref) {
		return ref
	}
	return filepath.Join(authDir, ref)
}

// ResolveRefs expands and inspects a list of preset path strings. For
// each ref, returns the directory-of-origin (templates/saved), whether
// the file exists, and whether its declared api_key_env has a value in
// existingKeys. Used by the kanban's Presets section to render an
// at-a-glance health check for an agent's preset surface.
//
// Ref strings are accepted in the same forms the kernel accepts:
// absolute, ~/-prefixed, or relative to the caller's working dir
// (relative paths are resolved against $PWD — pass absolute or
// home-relative for predictable behavior).
//
// existingKeys is the env-var-name → value map (typically
// Config.Keys). Pass nil when no key store is available; HasKey will
// be false for any preset that declares an api_key_env.
//
// ResolveRefs assumes NO OAuth is configured (the conservative default):
// a codex preset resolves to HasKey=false under this entry point. Callers
// that make credential-sensitive validity decisions should use
// ResolveRefsWithAuth and pass the real OAuth state.
func ResolveRefs(refs []string, existingKeys map[string]string) []ResolvedRef {
	return ResolveRefsWithAuth(refs, existingKeys, AuthState{})
}

// ResolveRefsWithAuth is ResolveRefs plus machine-level credential state
// (auth), so the codex-OAuth case can be judged correctly: a codex preset
// is valid only when auth.CodexOAuthConfigured is true. See ResolveRefs for
// the ref-string and existingKeys contracts.
func ResolveRefsWithAuth(refs []string, existingKeys map[string]string, auth AuthState) []ResolvedRef {
	out := make([]ResolvedRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, resolveOneRef(ref, existingKeys, auth))
	}
	return out
}

// ResolvePresetWithAuth applies the same credential-family rules as
// ResolveRefsWithAuth to an already-loaded in-memory preset. UI flows that
// already own a Preset should use this helper instead of duplicating provider
// and credential checks.
func ResolvePresetWithAuth(p Preset, existingKeys map[string]string, auth AuthState) ResolvedRef {
	r := ResolvedRef{
		Name:          p.Name,
		Source:        p.Source,
		Family:        CredentialFamilyOther,
		Exists:        true,
		ManifestValid: true,
	}
	llm, ok := p.Manifest["llm"].(map[string]interface{})
	if !ok || llm == nil {
		r.ManifestValid = false
		return r
	}
	r.Provider, _ = llm["provider"].(string)
	r.Family = ClassifyCredentialFamily(r.Provider)
	model, _ := llm["model"].(string)
	apiKeyEnv, _ := llm["api_key_env"].(string)
	codexAuthRaw, codexAuthPresent := llm["codex_auth_path"]
	r.CodexAuthRef, _ = codexAuthRaw.(string) // "" for absent or wrong-type values
	setAuth := func(ok bool) { r.HasKey = ok }
	switch r.Family {
	case CredentialFamilyCodexSingle:
		codexAuthStr, codexAuthIsString := codexAuthRaw.(string)
		switch {
		case codexAuthPresent && !codexAuthIsString:
			// Present but wrong-type explicit binding: fail closed.
			setAuth(false)
		case codexAuthPresent && codexAuthIsString && strings.TrimSpace(codexAuthStr) == "" && codexAuthStr != "":
			// Whitespace-only explicit value: fail closed (canonical empty is
			// an omitted field or literal "").
			setAuth(false)
		case codexAuthPresent && codexAuthIsString && strings.TrimSpace(codexAuthStr) != "":
			// Explicit nonempty binding: never the aggregate bool. With a known
			// dir, resolve exactly; without one, only absolute/"~" refs are
			// checkable, and any other relative ref fails closed.
			ref := strings.TrimSpace(codexAuthStr)
			if auth.CodexAuthDir != "" {
				setAuth(codexTokenFileValid(resolveCodexAuthRef(auth.CodexAuthDir, ref)))
			} else if filepath.IsAbs(ref) || strings.HasPrefix(ref, "~/") || ref == "~" {
				setAuth(codexTokenFileValid(resolveCodexAuthRef("", ref)))
			} else {
				setAuth(false)
			}
		default:
			// Unbound (field omitted or canonical empty): accept any usable
			// stored account when a dir is known, else the caller-provided
			// aggregate signal.
			if auth.CodexAuthDir != "" {
				setAuth(anyCodexTokenFileValid(auth.CodexAuthDir))
			} else {
				setAuth(auth.CodexOAuthConfigured)
			}
		}
	case CredentialFamilyCodexPool:
		if auth.CodexPoolEligibleModels != nil {
			eligible, present := auth.CodexPoolEligibleModels[model]
			setAuth(eligible || (!present && auth.CodexPoolFallbackEligible))
		} else {
			setAuth(auth.CodexPoolEligible)
		}
	case CredentialFamilyClaudeCLI:
		setAuth(auth.ClaudeCodeAuthConfigured)
	default:
		if apiKeyEnv != "" {
			setAuth(existingKeys[apiKeyEnv] != "")
		}
	}
	return r
}

func resolveOneRef(ref string, existingKeys map[string]string, auth AuthState) ResolvedRef {
	r := ResolvedRef{Ref: ref, Family: CredentialFamilyOther}
	if ref == "" {
		return r
	}
	abs := expandUserPath(ref)
	r.Name = strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	switch {
	case strings.Contains(abs, string(filepath.Separator)+"templates"+string(filepath.Separator)):
		r.Source = SourceTemplate
	case strings.Contains(abs, string(filepath.Separator)+"saved"+string(filepath.Separator)):
		r.Source = SourceSaved
	default:
		r.Source = SourceUnknown
	}
	if _, err := os.Stat(abs); err != nil {
		return r
	}
	r.Exists = true
	p, err := loadFromPath(abs)
	if err != nil {
		// Do not infer a provider from the basename/path. In particular, a
		// malformed or unreadable `codex.json` is not allowed to fail open
		// to the legacy Codex credential.
		return r
	}
	r = ResolvePresetWithAuth(p, existingKeys, auth)
	r.Ref = ref
	r.Name = strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	r.Source = func() PresetSource {
		switch {
		case strings.Contains(abs, string(filepath.Separator)+"templates"+string(filepath.Separator)):
			return SourceTemplate
		case strings.Contains(abs, string(filepath.Separator)+"saved"+string(filepath.Separator)):
			return SourceSaved
		default:
			return SourceUnknown
		}
	}()
	return r
}

// expandUserPath returns abs(`~/foo` → `$HOME/foo`), passing other forms
// through unchanged. Internal helper for ResolveRefs.
func expandUserPath(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// AutoSavedName picks a fresh saved-preset name derived from a template,
// using the same gap-fill counter as AutoEnvVarName. Pattern is
// "<template>-<N>" where N is the lowest positive integer that doesn't
// collide with anything in `existing`. Used when the user saves an
// edited template preset — we never overwrite the template, we always
// branch off a saved copy.
//
// existing is the set of preset names currently on disk (from List()).
// Returns "" when template is empty.
func AutoSavedName(template string, existing []string) string {
	if template == "" {
		return ""
	}
	used := map[int]bool{}
	wantPrefix := template + "-"
	for _, name := range existing {
		if !strings.HasPrefix(name, wantPrefix) {
			continue
		}
		mid := strings.TrimPrefix(name, wantPrefix)
		n := 0
		for _, c := range mid {
			if c < '0' || c > '9' {
				n = -1
				break
			}
			n = n*10 + int(c-'0')
		}
		if n > 0 {
			used[n] = true
		}
	}
	for n := 1; ; n++ {
		if !used[n] {
			return fmt.Sprintf("%s-%d", template, n)
		}
	}
}

// SavedCount returns the number of saved presets (Source == SourceSaved)
// in the list. Falls back to the legacy IsBuiltin check for any preset
// whose Source wasn't populated (e.g. hand-built test fixtures).
func SavedCount(presets []Preset) int {
	n := 0
	for _, p := range presets {
		switch p.Source {
		case SourceSaved:
			n++
		case SourceUnknown:
			if !IsBuiltin(p.Name) {
				n++
			}
		}
	}
	return n
}

// CountSavedByProvider returns the number of saved presets whose provider matches.
func CountSavedByProvider(presets []Preset, provider string) int {
	n := 0
	for _, p := range presets {
		if p.Source != SourceSaved {
			continue
		}
		llm, ok := p.Manifest["llm"].(map[string]interface{})
		if !ok {
			continue
		}
		if prov, _ := llm["provider"].(string); prov == provider {
			n++
		}
	}
	return n
}

func e() map[string]interface{} { return map[string]interface{}{} }

// skillsDefault returns the default skills capability config — two Tier 1
// paths: the network-shared skills shelf (resolved relative to the agent dir)
// and the TUI's per-user utilities directory. Users can edit init.json to
// add or remove paths; init.json is the ground truth and the capability
// reads it on every setup.
//
// `skills` itself is default-on in the kernel; this entry exists only to
// override the default kwargs (which carry no extra paths).
func skillsDefault() map[string]interface{} {
	return map[string]interface{}{
		"paths": []interface{}{
			"../.library_shared",
			"~/.lingtai-tui/utilities",
		},
	}
}

// openAICompatNoVisionPreset builds the manifest shape shared by
// OpenAI-compatible built-ins that do not wire a vision capability:
// an `api_compat: "openai"` LLM with an explicit base_url and a
// key sourced from api_key_env, plus the default DuckDuckGo web_search
// and skills capabilities. Some exact routes are text-only; Kimi Code is
// evidence-qualified as multimodal, but its coding endpoint/model mapping
// is not pinned tightly enough to expose built-in vision yet. Providers
// with a verified direct vision route build their Preset literally instead.
func openAICompatNoVisionPreset(name, summary, model, apiKeyEnv, baseURL string, tier string) Preset {
	desc := PresetDescription{Summary: summary}
	if tier != "" {
		desc.Tier = tier
	}
	return Preset{
		Name:        name,
		Description: desc,
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": name, "model": model,
				"api_key": nil, "api_key_env": apiKeyEnv,
				"base_url": baseURL, "api_compat": "openai",
			},
			"capabilities": map[string]interface{}{
				"web_search": map[string]interface{}{"provider": "duckduckgo"},
				"skills":     skillsDefault(),
			},
		},
	}
}

func minimaxPreset() Preset {
	mm := map[string]interface{}{
		"provider":    "minimax",
		"api_key_env": "MINIMAX_API_KEY",
	}
	return Preset{
		Name:        "minimax",
		Description: PresetDescription{Summary: "MiniMax M3 — full multimodal capabilities"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "minimax", "model": "MiniMax-M3",
				"api_key": nil, "api_key_env": "MINIMAX_API_KEY",
				"base_url": ProviderRegionURLs["minimax"][0].URL,
			},
			// Core caps (knowledge, skills, shell, avatar, daemon, mcp,
			// read/write/edit/glob/grep + psyche/email intrinsics) are
			// default-on in the kernel — only overrides and opt-in caps
			// belong here. See lingtai-kernel capabilities.CORE_DEFAULTS.
			"capabilities": map[string]interface{}{
				"web_search": mm,
				"vision":     mm,
				"skills":     skillsDefault(),
			},
		},
	}
}

func zhipuPreset() Preset {
	zp := map[string]interface{}{"provider": "zhipu"}
	return Preset{
		Name:        "zhipu",
		Description: PresetDescription{Summary: "Zhipu GLM Coding Plan — OpenAI-compatible"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "zhipu", "model": "GLM-5.2",
				"api_key": nil, "api_key_env": "ZHIPU_API_KEY",
				"base_url": ProviderRegionURLs["zhipu"][0].URL, "api_compat": "openai",
			},
			"capabilities": map[string]interface{}{
				"web_search": zp,
				"skills":     skillsDefault(),
			},
		},
	}
}

func mimoPreset() Preset {
	// mimo-v2.5 is the sweet spot: 1M context, vision-capable, supports tool
	// calls and thinking mode. The current text-only sibling is mimo-v2.5-pro;
	// retired V2 model IDs are not exposed by the TUI picker. Vision uses the
	// first-class MiMoVisionService (kernel: services/vision/mimo.py).
	mp := map[string]interface{}{
		"provider": "mimo",
		"model":    "mimo-v2.5",
	}
	return Preset{
		Name:        "mimo",
		Description: PresetDescription{Summary: "Xiaomi MiMo V2.5 — OpenAI-compatible, 1M context, vision + tools"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "mimo", "model": "mimo-v2.5",
				"api_key": nil, "api_key_env": "XIAOMI_API_KEY",
				"base_url": ProviderRegionURLs["mimo"][0].URL, "api_compat": "openai",
			},
			"capabilities": map[string]interface{}{
				"web_search": map[string]interface{}{"provider": "duckduckgo"},
				"vision":     mp,
				"skills":     skillsDefault(),
			},
		},
	}
}

func deepseekPreset() Preset {
	// DeepSeek's public API is text-only — no media generation. For audio
	// analysis (transcription, music critique), use the `listen` skill; for
	// media creation, register the MiniMax-Media MCP server via the
	// `mcp-manual` skill (kernel `mcp` capability).
	return openAICompatNoVisionPreset(
		"deepseek",
		"DeepSeek V4 Pro — OpenAI-compatible, 1M context window, tool calls",
		"deepseek-v4-pro", "DEEPSEEK_API_KEY", ProviderRegionURLs["deepseek"][0].URL, "")
}

func geminiPreset() Preset {
	// Gemini 3.8 Flash (Google) — multimodal model with native vision,
	// tool calling, and streaming. Uses Google's own Gemini adapter in
	// the kernel (not OpenAI-compat), so no base_url or api_compat.
	gm := map[string]interface{}{
		"provider":    "gemini",
		"api_key_env": "GEMINI_API_KEY",
	}
	return Preset{
		Name:        "gemini",
		Description: PresetDescription{Summary: "Gemini 3.8 Flash — Google's multimodal model, tool calls, vision", Tier: "3"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "gemini", "model": "gemini-3.8-flash",
				"api_key": nil, "api_key_env": "GEMINI_API_KEY",
			},
			// Gemini is multimodal/vision-capable — image inputs are
			// handled natively by the model. For audio analysis use the
			// `listen` skill; for media creation register a provider's
			// MCP server via `mcp-manual`.
			"capabilities": map[string]interface{}{
				"web_search": map[string]interface{}{"provider": "duckduckgo"},
				"vision":     gm,
				"skills":     skillsDefault(),
			},
		},
	}
}

func kimiPreset() Preset {
	// Kimi Code (Moonshot 月之暗面) — OpenAI-compatible coding API.
	// Subscription-based (no per-token billing); model `kimi-for-coding`.
	// Tool calling supported. The kernel auto-sets User-Agent
	// "LingTai-Agent/1.0" for the `kimi` provider per Kimi's ToS — UA
	// spoofing risks account suspension. The model family has multimodal
	// evidence, but the exact coding endpoint/model image-input mapping is not
	// pinned tightly enough to wire a built-in vision capability yet.
	return openAICompatNoVisionPreset(
		"kimi",
		"Kimi Code (Moonshot) — OpenAI-compatible, subscription-based, tool calling",
		"kimi-for-coding", "KIMI_CODE_API_KEY", ProviderRegionURLs["kimi"][0].URL, "3")
}

func grokPreset() Preset {
	// Grok (xAI) reached through the OpenCode Go subscription — the only
	// route this TUI has verified for it. `grok-4.5` is the id the Go
	// endpoint's model list serves; no native api.x.ai endpoint/model pairing
	// has been checked, so none is shipped (a user with an xAI key selects
	// the Custom base_url row, or clones the `custom` template).
	//
	// api_key_env is OPENCODE_GO_API_KEY rather than a grok-specific slot
	// because the shipped endpoint IS OpenCode Go: someone who already
	// configured that shared account for deepseek/zhipu/minimax/kimi/mimo
	// gets a working grok preset without pasting the key a second time. The
	// provider-generic GROK_API_KEY stays in ProviderDefaultEnv as the
	// fallback name, separate from the slot a switch adopts.
	//
	// Text-only: the Go endpoint's image-input mapping for grok-4.5 is not
	// pinned, so no vision capability is wired.
	return openAICompatNoVisionPreset(
		"grok",
		"Grok (xAI) — OpenAI-compatible via OpenCode Go",
		"grok-4.5", ProviderRegionURLs["grok"][0].Env, ProviderRegionURLs["grok"][0].URL, "3")
}

func nvidiaPreset() Preset {
	// NVIDIA NIM / NVIDIA API Catalog (build.nvidia.com) — an
	// OpenAI-compatible /chat/completions gateway hosting a large catalog
	// of open-weight models (Llama, Qwen, Kimi, GPT-OSS, Nemotron, ...) at
	// no per-token cost on the free developer tier. Default model is the
	// bounded stable agent snapshot's Nemotron 3 Ultra route; users clone this
	// preset to switch among the curated IDs in the TUI picker. Provider is the
	// generic "nvidia" string routed through the kernel's OpenAI-compatible
	// client via api_compat=openai + the explicit base_url. Text-only — no
	// media generation; use `listen` for audio, `mcp-manual` for media.
	//
	// NOTE: the kernel must register the "nvidia" provider with
	// prompt_cache_key disabled — NVIDIA NIM rejects that OpenAI-only field
	// with HTTP 400. See lingtai-kernel llm/_register.py.
	return openAICompatNoVisionPreset(
		"nvidia",
		"NVIDIA NIM — bounded route-served stable agent snapshot, tool calls",
		"nvidia/nemotron-3-ultra-550b-a55b", "NVIDIA_API_KEY", "https://integrate.api.nvidia.com/v1", "")
}

func openrouterPreset() Preset {
	return Preset{
		Name:        "openrouter",
		Description: PresetDescription{Summary: "OpenRouter — GLM 5.3 gateway route (interactive, non-batch)"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "openrouter", "model": "z-ai/glm-5.3",
				"api_key": nil, "api_key_env": "OPENROUTER_API_KEY",
				"base_url": nil,
			},
			// OpenRouter is a text-only /chat/completions gateway — no media
			// generation. For audio analysis use the `listen` skill; for
			// media creation register a provider's MCP server via `mcp-manual`.
			"capabilities": map[string]interface{}{
				"web_search": map[string]interface{}{"provider": "duckduckgo"},
				"skills":     skillsDefault(),
			},
		},
	}
}

func codexPreset() Preset {
	cx := map[string]interface{}{"provider": "codex", "api_key_env": ""}
	return Preset{
		Name:        "codex",
		Description: PresetDescription{Summary: "ChatGPT account — vision + web search + tools"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				// Use gpt-5.6-sol as the default after successful live testing.
				// The other named GPT-5.6 routes remain selectable for accounts
				// where they are enabled. The complete curated model list lives
				// in preset_editor.go's providerModels; the bare gpt-5.6 alias
				// is intentionally omitted.
				"provider": "codex", "model": "gpt-5.6-sol",
				"api_key": nil, "api_key_env": "",
				"base_url": "https://chatgpt.com/backend-api/codex",
				// LingTai is the primary brain, so Codex runs at maximum
				// reasoning effort by default. Carried explicitly
				// here (not a UI-only fallback) so the running session and
				// generated init.json actually receive xhigh.
				"thinking": "xhigh",
			},
			"capabilities": map[string]interface{}{
				"web_search": cx,
				"vision":     cx,
				"skills":     skillsDefault(),
			},
		},
	}
}

// codexPoolPreset mirrors the standard codex preset but binds the kernel's
// `codex-pool` provider, which load-balances across the ChatGPT accounts listed
// in the non-secret ~/.lingtai-tui/codex-auth-pool.json pool file (weights and
// account membership live THERE, not in this preset). It is purely how a user
// opts into pooling; selecting it never rewrites other presets. Model, endpoint,
// thinking level, and capabilities match codexPreset() so behavior is identical
// apart from the provider routing to the pool.
func codexPoolPreset() Preset {
	cx := map[string]interface{}{"provider": "codex-pool", "api_key_env": ""}
	return Preset{
		Name:        "codex-pool",
		Description: PresetDescription{Summary: "ChatGPT account pool — load-balances across your Codex accounts"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				// Same gpt-5.6-sol default and endpoint as the single-account
				// codex preset; only the provider differs so the kernel routes
				// through the pool. base_url stays the official Codex endpoint —
				// the pool selects among token files, not endpoints.
				"provider": "codex-pool", "model": "gpt-5.6-sol",
				"api_key": nil, "api_key_env": "",
				"base_url": "https://chatgpt.com/backend-api/codex",
				"thinking": "xhigh",
			},
			"capabilities": map[string]interface{}{
				"web_search": cx,
				"vision":     cx,
				"skills":     skillsDefault(),
			},
		},
	}
}

func claudePreset() Preset {
	return Preset{
		Name:        "claude",
		Description: PresetDescription{Summary: "Claude Code / Claude Max — uses your local Claude CLI login (no API key)"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				// The kernel's canonical Claude Code provider invokes the local
				// `claude` CLI, whose print-mode backend is shown as "claude-p"
				// in the TUI. It reuses the CLI's OAuth login — no per-request
				// API key, and the TUI stores no Anthropic token of its own. So
				// api_key is nil and api_key_env is empty; credential validity
				// is judged by detecting an existing `claude` CLI login (see
				// AuthState.ClaudeCodeAuthConfigured). Default model is the CLI
				// alias "opus"; the editor also offers "fable", whose full ID
				// is `claude-fable-5` in current Claude Code.
				"provider": "claude-code", "model": "opus",
				"api_key": nil, "api_key_env": "",
			},
			// Conservative capabilities: Claude Code is wired here as a completion
			// provider only. We do NOT route web_search or vision through it —
			// the CLI's own native tool surface is out of scope,
			// and there's no inherit path validated for this provider yet.
			// Keep the standard LingTai skills default so agents behave like
			// any other preset.
			"capabilities": map[string]interface{}{
				"skills": skillsDefault(),
			},
		},
	}
}

func customPreset() Preset {
	return Preset{
		Name:        "custom",
		Description: PresetDescription{Summary: "OpenAI-compatible API — full capabilities"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "custom", "model": "", "api_compat": "openai",
				"api_key": nil, "api_key_env": "LLM_API_KEY", "base_url": nil,
			},
			"capabilities": map[string]interface{}{
				"web_search": e(),
				// Inherit vision through the LLM's own endpoint. When the
				// relay is OpenAI-compatible and the underlying model
				// supports vision (gpt-4o/4.x, gpt-5.5, etc.), the kernel
				// routes through OpenAIVisionService with the LLM's
				// base_url. If the relay or model can't do vision the
				// call fails at runtime — no special handling.
				"vision": map[string]interface{}{"provider": "inherit"},
				"skills": skillsDefault(),
			},
		},
	}
}

// ProceduresPath returns the absolute path to the procedures file for a language.
// Checks the lang-specific path first, falls back to the root procedures.md.
func ProceduresPath(globalDir, lang string) string {
	p := filepath.Join(globalDir, "procedures", lang, "procedures.md")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(globalDir, "procedures", "procedures.md")
}

// populate mirrors an embedded FS subtree to globalDir, skipping existing files.
func populate(globalDir string, fsys embed.FS, root string) {
	fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		target := filepath.Join(globalDir, root, rel)
		os.MkdirAll(filepath.Dir(target), 0o755)
		data, err := fsys.ReadFile(path)
		if err == nil {
			os.WriteFile(target, data, 0o644)
		}
		return nil
	})
}

// Bootstrap populates all embedded assets and default presets at ~/.lingtai-tui/.
func Bootstrap(globalDir string) error {
	populate(globalDir, covenantFS, "covenant")
	populate(globalDir, principleFS, "principle")
	populate(globalDir, proceduresFS, "procedures")
	populate(globalDir, soulFS, "soul")
	populate(globalDir, templatesFS, "templates")
	populate(globalDir, recipeAssetsFS, "recipe_assets")
	// Rename recipe_assets -> recipes at the target path.
	// Unlike other populate() calls (which are merge-skip), recipes are
	// refreshed wholesale on every launch — the TUI manages this content,
	// users should not edit bundled recipe files.
	src := filepath.Join(globalDir, "recipe_assets")
	dst := filepath.Join(globalDir, "recipes")
	if _, err := os.Stat(src); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove old recipes dir: %v\n", err)
		}
		if err := os.Rename(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to rename recipe_assets to recipes: %v\n", err)
		}
	}
	// Templates are TUI-managed: rewritten wholesale on every launch
	// from embedded data, retired entries pruned. Saved presets in
	// presets/saved/ are user territory and never touched here.
	return RefreshTemplates()
}

// PopulateBundledLibrary extracts the TUI's embedded bundled skills into a
// stable per-user location: <globalDir>/utilities/ (typically
// ~/.lingtai-tui/utilities/). Agents reach these by default via the
// skills.paths entry in their init.json, which points at the same path.
//
// Called on every TUI startup so utility skills stay in sync with the
// shipped binary. Directory is rewritten from scratch so a TUI upgrade
// that renames or removes a utility propagates cleanly.
//
// Per-agent .library/ is owned by the kernel library capability, not by the TUI.
func PopulateBundledLibrary(globalDir string) {
	utilitiesDir := filepath.Join(globalDir, "utilities")
	os.RemoveAll(utilitiesDir)
	os.MkdirAll(utilitiesDir, 0o755)

	fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("skills", path)
		target := filepath.Join(utilitiesDir, rel)
		os.MkdirAll(filepath.Dir(target), 0o755)
		data, err := skillsFS.ReadFile(path)
		if err == nil {
			os.WriteFile(target, data, 0o644)
		}
		return nil
	})
}

// BundledSkillNames returns the set of skill directory names that are shipped
// with the TUI binary (embedded in skillsFS). Use this to distinguish
// intrinsic skills from user-created or recipe-imported ones.
func BundledSkillNames() map[string]bool {
	names := make(map[string]bool)
	entries, err := fs.ReadDir(skillsFS, "skills")
	if err != nil {
		return names
	}
	for _, e := range entries {
		if e.IsDir() {
			names[e.Name()] = true
		}
	}
	return names
}

// ReadBundledSkillFile returns the contents of a file inside a bundled skill,
// read straight from the embedded skillsFS. The skill argument is the skill
// directory name (e.g. "lingtai-tui-help"); relPath is the path inside that
// skill, using forward slashes (e.g. "assets/slash-commands.en.md"). This lets
// in-binary callers (like the TUI /help view) render bundled skill assets
// without relying on the on-disk extraction in PopulateBundledLibrary.
func ReadBundledSkillFile(skill, relPath string) (string, error) {
	data, err := skillsFS.ReadFile("skills/" + skill + "/" + relPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CovenantPath returns the absolute path to the covenant file for a language.
func CovenantPath(globalDir, lang string) string {
	return filepath.Join(globalDir, "covenant", lang, "covenant.md")
}

// SoulFlowPath returns the absolute path to the soul flow file for a language.
func SoulFlowPath(globalDir, lang string) string {
	return filepath.Join(globalDir, "soul", lang, "soul-flow.md")
}

// AddonConfigRelPath returns the path (relative to the project root) where an
// addon's config file should live. This is the one place the convention
// ".lingtai/.addons/<addon>/config.json" is encoded.
func AddonConfigRelPath(addon string) string {
	return filepath.Join(".lingtai", ".addons", addon, "config.json")
}

// AddonConfigPathFromAgent returns the path (relative to an agent's working
// directory, which is <project>/.lingtai/<agent>/) to an addon's config file.
// Used in init.json's "addons.<name>.config" field — the kernel resolves these
// paths against the agent's working_dir.
func AddonConfigPathFromAgent(addon string) string {
	return filepath.Join("..", ".addons", addon, "config.json")
}

// AddonSecretsPathFromAgent returns the path (relative to an agent's working
// directory) where an addon's config file lives under the admin-local
// .secrets/ convention introduced 2026-04-16. Used by first-creation seeding
// to prefer the new path when it exists on disk.
func AddonSecretsPathFromAgent(addon string) string {
	return filepath.Join(".secrets", addon+".json")
}

// defaultMCPSpec returns the canonical wiring for one of the curated
// addons (imap / telegram / feishu / wechat / whatsapp) — the Python module to invoke,
// the env-var name the MCP reads its config path from, and the config path
// (relative to the agent working dir) to point that env var at by default.
//
// Used by GenerateInitJSONWithOpts to seed init.json's mcp.<name> activation
// entries when the wizard selects an addon. supported=false for unknown
// names so the caller skips them silently rather than emitting a spec the
// kernel would reject.
//
// Note: this is the writer-side mirror of the migration's addonSpec table
// (m028). When you add a new curated addon, update both.
func defaultMCPSpec(name string) (module, envVar, configRel string, supported bool) {
	switch name {
	case "imap":
		return "lingtai.mcp_servers.imap", "LINGTAI_IMAP_CONFIG", filepath.Join(".secrets", "imap.json"), true
	case "telegram":
		return "lingtai.mcp_servers.telegram", "LINGTAI_TELEGRAM_CONFIG", filepath.Join(".secrets", "telegram.json"), true
	case "feishu":
		return "lingtai.mcp_servers.feishu", "LINGTAI_FEISHU_CONFIG", filepath.Join(".secrets", "feishu.json"), true
	case "wechat":
		return "lingtai.mcp_servers.wechat", "LINGTAI_WECHAT_CONFIG", filepath.Join(".secrets", "wechat", "config.json"), true
	case "whatsapp":
		return "lingtai.mcp_servers.whatsapp", "LINGTAI_WHATSAPP_CONFIG", filepath.Join(".secrets", "whatsapp.json"), true
	}
	return "", "", "", false
}

// DefaultMCPSpec is the exported form of defaultMCPSpec, so diagnostics
// (e.g. /doctor's addon-secrets check) can resolve the canonical config path
// without re-hardcoding the addon→path table (fable F4: the table must stay
// single-sourced).
func DefaultMCPSpec(name string) (module, envVar, configRel string, supported bool) {
	return defaultMCPSpec(name)
}

// DefaultPreset returns the first built-in preset (minimax).
func DefaultPreset() Preset {
	return minimaxPreset()
}

// AutoEnvVarName builds a deterministic api_key_env slot name for a
// preset, with a number suffix that gap-fills the lowest unused index.
//
// Shape: <PROVIDER>[_<REGION>]_<N>_API_KEY
//   - PROVIDER:   uppercased manifest.llm.provider
//   - REGION:     "CN" or "INTL" for minimax/zhipu (read from base_url);
//     omitted for other providers
//   - N:          the lowest positive integer not already present in
//     existingKeys (1-based). Reuses freed slots since the
//     user said API keys rapidly rotate anyway.
//
// existingKeys is the env-var-keyed map from Config.Keys — caller
// passes it in so this stays a pure function (no I/O).
//
// Returns "" when the preset has no provider — caller falls back to
// whatever api_key_env the preset already declared.
func AutoEnvVarName(p Preset, existingKeys map[string]string) string {
	llm, _ := p.Manifest["llm"].(map[string]interface{})
	provider, _ := llm["provider"].(string)
	if provider == "" {
		return ""
	}
	prefix := strings.ToUpper(provider)
	if region := regionSuffix(provider, llmString(llm, "base_url")); region != "" {
		prefix += "_" + region
	}
	// Find the lowest unused N. We scan existingKeys for entries that
	// match `<prefix>_<int>_API_KEY` and collect the integers.
	used := map[int]bool{}
	wantPrefix := prefix + "_"
	for name := range existingKeys {
		if !strings.HasPrefix(name, wantPrefix) || !strings.HasSuffix(name, "_API_KEY") {
			continue
		}
		mid := strings.TrimSuffix(strings.TrimPrefix(name, wantPrefix), "_API_KEY")
		// Only consider pure-integer suffixes — skip things like
		// MINIMAX_PERSONAL_API_KEY (no number) or MINIMAX_PROD_v2_API_KEY.
		n := 0
		for _, c := range mid {
			if c < '0' || c > '9' {
				n = -1
				break
			}
			n = n*10 + int(c-'0')
		}
		if n > 0 {
			used[n] = true
		}
	}
	for n := 1; ; n++ {
		if !used[n] {
			return fmt.Sprintf("%s_%d_API_KEY", prefix, n)
		}
	}
}

// regionSuffix returns "CN" / "INTL" for providers with regional
// splits, "" for everything else. Mirrors the wizard's existing
// region-detection logic so a preset that says "minimaxi.com" gets
// the same CN suffix the wizard would have applied.
//
// A region row that declares its own Env (OpenCode Go) is not a CN/INTL
// split — it is a separate account reached through a shared endpoint. It
// gets no suffix: the substring match below classifies by hostname, and
// opencode.ai contains neither "api.z.ai" nor "minimaxi.com", so zhipu
// would fall through to "CN" and minimax to "INTL" and the stamped slot
// (MINIMAX_INTL_1_API_KEY for https://opencode.ai/zen/go/v1) would lie
// about the endpoint it unlocks.
func regionSuffix(provider, baseURL string) string {
	for _, r := range ProviderRegionURLs[provider] {
		if r.URL != "" && r.URL == baseURL && r.Env != "" {
			return ""
		}
	}
	switch provider {
	case "minimax":
		if strings.Contains(baseURL, "minimaxi.com") {
			return "CN"
		}
		return "INTL"
	case "zhipu":
		if strings.Contains(baseURL, "api.z.ai") {
			return "INTL"
		}
		return "CN"
	}
	return ""
}

// llmString is a tiny accessor that returns a string field from an
// llm map without panicking on missing keys or wrong types.
func llmString(llm map[string]interface{}, key string) string {
	v, _ := llm[key].(string)
	return v
}

// AgentOpts holds per-agent configuration values set at creation time.
type AgentOpts struct {
	Language     string   // "en", "zh", or "wen"
	ContextLimit int      // token budget
	SoulDelay    *float64 // nil means omit soul.delay so the kernel default applies
	// SoulFlowEnabled is the wizard's soul-flow opt-in. When true,
	// GenerateInitJSONWithOpts writes LINGTAI_SOUL_FLOW_ENABLED=1 into the
	// global .env (the env_file the agent inherits at boot); when false it
	// removes the key. Default false — soul flow is opt-in, matching the
	// kernel default. This is distinct from SoulFile (the soul-flow prompt
	// path) and from SoulDelay (cadence after opt-in).
	SoulFlowEnabled bool
	MaxRpm          int      // API requests-per-minute cap (cooperative network gate); 0 disables
	MaxAedAttempts  int      // AED (auto-error-recovery) retry attempts per message turn before fallback/sleep
	Karma           bool     // lifecycle control over other agents
	Nirvana         bool     // permanent agent destruction
	CovenantFile    string   // path to covenant file
	SoulFile        string   // path to soul flow file
	CommentFile     string   // path to comment file (optional)
	Addons          []string // addon names to auto-populate in init.json (e.g. ["imap", "telegram"])
	// AllowedPresets lists the absolute (or ~-prefixed) paths of every
	// preset this agent is authorized to swap to at runtime. The default
	// preset is automatically included if missing. When empty, falls back
	// to a single-element list containing just the default preset.
	AllowedPresets []string
	// PreserveActivePreset, when true, leaves manifest.preset.active alone
	// and only updates manifest.preset.default to the chosen preset. Used
	// by /setup so a running agent doesn't get yanked mid-conversation —
	// the new choice takes effect on the next AED fallback or explicit
	// revert_preset call.
	PreserveActivePreset bool
}

// DefaultAgentOpts returns sensible defaults for agent creation.
func DefaultAgentOpts() AgentOpts {
	return AgentOpts{
		Language:       "en",
		ContextLimit:   500000,
		SoulDelay:      nil,
		MaxRpm:         60,
		MaxAedAttempts: DefaultMaxAedAttempts,
		Karma:          true,
		Nirvana:        false,
	}
}

// AED max-attempts validation bounds. DefaultMaxAedAttempts is the TUI
// first-run/setup default for newly generated init.json manifests. Keep this
// default explicit so setup, tests, and generated init.json agree on the same
// AED retry count.
const (
	DefaultMaxAedAttempts = 5
	MinMaxAedAttempts     = 1
	MaxMaxAedAttempts     = 100
)

// DefaultSoulFlowCadence is the soul.delay (seconds) the wizard stamps
// when the user opts into soul flow but leaves the cadence field blank.
// Two hours is a sane "proactive but not chatty" default. It is applied
// ONLY when soul flow is enabled — a default disabled agent omits the
// soul block entirely so the kernel's own default applies (and no fires
// happen while the env opt-in is off). This prevents an enabled agent
// from silently inheriting the kernel's huge no-op fallback delay.
const DefaultSoulFlowCadence = 7200.0

// ClampAedAttempts validates a user-supplied AED max-attempts value. A value of
// zero or below (the zero value, or empty/invalid input parsed to 0) falls back
// to DefaultMaxAedAttempts; anything above MaxMaxAedAttempts is clamped down to
// the ceiling. The result is always within [MinMaxAedAttempts, MaxMaxAedAttempts].
func ClampAedAttempts(n int) int {
	if n < MinMaxAedAttempts {
		return DefaultMaxAedAttempts
	}
	if n > MaxMaxAedAttempts {
		return MaxMaxAedAttempts
	}
	return n
}

// GenerateInitJSON creates a full init.json from a preset using default opts.
func GenerateInitJSON(p Preset, agentName, dirName, lingtaiDir, globalDir string) error {
	opts := DefaultAgentOpts()
	// Inherit language from preset if set
	if l, ok := p.Manifest["language"].(string); ok && l != "" {
		opts.Language = l
	}
	return GenerateInitJSONWithOpts(p, agentName, dirName, lingtaiDir, globalDir, opts)
}

// SyncCapabilityAPIKeyEnv propagates the LLM's api_key_env to any
// capability whose provider matches the LLM provider. This ensures
// capabilities like web_search and vision use the same resolved env
// var slot (e.g. "ZHIPU_CN_1_API_KEY") rather than a stale preset
// placeholder (e.g. "ZHIPU_API_KEY").
func SyncCapabilityAPIKeyEnv(manifest map[string]interface{}) {
	llm, _ := manifest["llm"].(map[string]interface{})
	if llm == nil {
		return
	}
	llmProvider, _ := llm["provider"].(string)
	llmKeyEnv, _ := llm["api_key_env"].(string)
	if llmProvider == "" || llmKeyEnv == "" {
		return
	}
	caps, _ := manifest["capabilities"].(map[string]interface{})
	if caps == nil {
		return
	}
	for _, cfg := range caps {
		capMap, ok := cfg.(map[string]interface{})
		if !ok {
			continue
		}
		capProvider, _ := capMap["provider"].(string)
		if capProvider != llmProvider {
			continue
		}
		capMap["api_key_env"] = llmKeyEnv
	}
}

// stripObsoleteInitFields removes top-level init.json fields that the kernel
// treats as ignored legacy input. New writers and explicit read-modify-write
// paths must not carry them forward, because their presence triggers a
// deterministic boot-time nudge before the agent can do useful work.
func stripObsoleteInitFields(initJSON map[string]interface{}) {
	delete(initJSON, "principle_file")
	delete(initJSON, "procedures_file")
}

// GenerateInitJSONWithOpts creates a full init.json from a preset with explicit agent options.
func GenerateInitJSONWithOpts(p Preset, agentName, dirName, lingtaiDir, globalDir string, opts AgentOpts) error {
	// The directory derived from dirName must remain a single contained
	// child of lingtaiDir: reject absolute paths, parent segments, and
	// either platform's separators before any join or mkdir (issue #849).
	if err := ValidateSafeName(dirName); err != nil {
		return fmt.Errorf("invalid agent directory name %q: %w", dirName, err)
	}
	if err := p.NormalizeLegacyCapabilities(); err != nil {
		return fmt.Errorf("canonicalize preset capabilities: %w", err)
	}
	agentDir := filepath.Join(lingtaiDir, dirName)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}

	// Keep the existing root and manifest fields when /setup regenerates an
	// agent. Known generated fields below overwrite these values; unrelated
	// user/kernel fields remain intact.
	var existingInit map[string]interface{}
	existingInitPath := filepath.Join(agentDir, "init.json")
	if existingData, err := os.ReadFile(existingInitPath); err == nil {
		if DecodeJSONUseNumber(existingData, &existingInit) != nil {
			existingInit = nil
		}
	}

	// Build manifest with opts
	manifest := make(map[string]interface{})
	if existingManifest, ok := existingInit["manifest"].(map[string]interface{}); ok {
		for key, value := range existingManifest {
			manifest[key] = value
		}
	}
	manifest["agent_name"] = agentName
	lang := opts.Language
	if lang == "" {
		lang = "en"
	}
	manifest["language"] = lang
	if llm, ok := p.Manifest["llm"]; ok {
		manifest["llm"] = llm
	}
	if caps, ok := p.Manifest["capabilities"]; ok {
		manifest["capabilities"] = caps
	}
	// Propagate the LLM's resolved api_key_env to capabilities that
	// share the same provider. The builtin preset templates use a
	// placeholder like "ZHIPU_API_KEY", but stampAutoEnvVar rewrites
	// the LLM's slot to "ZHIPU_CN_1_API_KEY" etc. Without this,
	// web_search/vision capabilities still reference the non-existent
	// placeholder and fail at boot.
	SyncCapabilityAPIKeyEnv(manifest)
	manifest["admin"] = map[string]interface{}{
		"karma":   opts.Karma,
		"nirvana": opts.Nirvana,
	}
	if opts.SoulDelay != nil {
		manifest["soul"] = map[string]interface{}{"delay": *opts.SoulDelay}
	}
	manifest["context_limit"] = opts.ContextLimit
	// molt_pressure and molt_prompt are intentionally NOT written: the kernel no
	// longer accepts configurable context.molt thresholds or messages (Jason
	// #4135/#4137/#4140). Existing init.json files that still carry stale keys
	// are left untouched (no migration); the kernel ignores them.
	// Per-wake loop budget: every iteration of the LLM/tool-call loop
	// counts as a turn, not just LLM requests, so tool-heavy work burns
	// through it quickly. The agent sleeps when the budget is exhausted.
	manifest["max_turns"] = 500
	manifest["max_rpm"] = opts.MaxRpm
	// AED max-attempts: normalize through ClampAedAttempts so a zero-value
	// AgentOpts (caller didn't set it) still writes a valid default rather
	// than 0, which the kernel would treat as "never retry".
	manifest["max_aed_attempts"] = ClampAedAttempts(opts.MaxAedAttempts)
	manifest["streaming"] = false
	// Track which preset this agent was created from. The kernel reads this
	// at boot to materialize manifest.llm + manifest.capabilities from the
	// referenced preset file. As of the path-as-name redesign, the value is
	// the preset's full path (in ~/... shorthand for portability across
	// machines), not its filename stem. The agent passes this same string
	// to system(action='refresh', preset='<path>') to swap.
	// The 'default' field is used by AED auto-fallback to revert to the
	// original preset when the active one keeps failing.
	if p.Name != "" {
		presetRef := RefFor(p)
		// Default behavior: both active and default point at the new
		// preset (the agent runs on the chosen preset immediately).
		// /setup mode (PreserveActivePreset=true) only updates default,
		// so the running agent keeps its current preset until an AED
		// fallback or explicit revert_preset takes effect.
		activeRef := presetRef
		// /setup mode also preserves the existing `allowed` list so
		// re-running the wizard never silently widens the authorized set.
		var existingAllowed []string
		if opts.PreserveActivePreset {
			existingInitPath := filepath.Join(agentDir, "init.json")
			if data, err := os.ReadFile(existingInitPath); err == nil {
				var existing map[string]interface{}
				if DecodeJSONUseNumber(data, &existing) == nil {
					if mn, ok := existing["manifest"].(map[string]interface{}); ok {
						if pre, ok := mn["preset"].(map[string]interface{}); ok {
							if cur, ok := pre["active"].(string); ok && cur != "" {
								activeRef = cur
							}
							if al, ok := pre["allowed"].([]interface{}); ok {
								for _, e := range al {
									if s, ok := e.(string); ok && s != "" {
										existingAllowed = append(existingAllowed, s)
									}
								}
							}
							// When the caller passed a synthetic preset (e.g. the
							// "Keep current preset" sentinel Name="keep_current"
							// built in NewSetupModeModel), RefFor() above produces
							// a path that doesn't correspond to any real file.
							// Preserve the existing on-disk default ref so we never
							// write keep_current.json into manifest.preset.{default,allowed}.
							// Older init.json files may lack manifest.preset.default; in that
							// case fall back to the real active ref instead.
							if isSyntheticPreset(p) {
								syntheticRef := RefFor(p)
								if existingDef, ok := pre["default"].(string); ok && existingDef != "" {
									presetRef = existingDef
								} else if activeRef != "" && activeRef != syntheticRef {
									presetRef = activeRef
								}
								// activeRef was already set to the existing active above; if it
								// was still pointing at the synthetic ref, snap it to the real
								// policy default.
								if activeRef == syntheticRef {
									activeRef = presetRef
								}
							}
						}
					}
				}
			}
		}

		// Build the allowed list. Caller-supplied AllowedPresets wins;
		// otherwise we keep the existing list (during /setup) or fall
		// back to the single-preset default. The default preset is always
		// present.
		//
		// `active` must also end up in `allowed` (the kernel's validate_init
		// enforces this). But when the caller passed an explicit
		// AllowedPresets list and the current `active` was deselected from
		// it, force-adding active back would silently re-authorize a preset
		// the user just chose to revoke. In that case we snap active to the
		// new default instead, which is always in allowed by construction.
		allowedSet := map[string]struct{}{}
		var allowed []string
		appendUnique := func(s string) {
			if s == "" {
				return
			}
			if _, exists := allowedSet[s]; exists {
				return
			}
			allowedSet[s] = struct{}{}
			allowed = append(allowed, s)
		}

		var seed []string
		userSuppliedAllowed := len(opts.AllowedPresets) > 0
		switch {
		case userSuppliedAllowed:
			seed = opts.AllowedPresets
		case len(existingAllowed) > 0:
			seed = existingAllowed
		}
		for _, s := range seed {
			appendUnique(s)
		}
		appendUnique(presetRef) // default must always be in allowed

		// Reconcile active against the authoritative allowed list. When
		// the caller has explicitly listed allowed presets and the current
		// active is no longer one of them, demote active to the default
		// (which is always allowed). When the caller didn't supply an
		// allowed list, we silently include active to preserve the prior
		// behavior of "I didn't touch the surface, don't change it".
		activeAllowed := false
		for _, s := range allowed {
			if s == activeRef {
				activeAllowed = true
				break
			}
		}
		if !activeAllowed {
			if userSuppliedAllowed {
				activeRef = presetRef
			} else {
				appendUnique(activeRef)
			}
		}

		manifest["preset"] = map[string]interface{}{
			"active":  activeRef,
			"default": presetRef,
			"allowed": allowed,
		}
	}

	// Resolve file paths — use opts if set, fallback to language defaults
	// Keep an existing supported covenant path during /setup; fresh agents
	// use the language default. Obsolete principle/procedures paths are not
	// resolved or emitted.
	covenantFile := opts.CovenantFile
	if covenantFile == "" {
		if existing, ok := existingInit["covenant_file"].(string); ok && existing != "" {
			covenantFile = existing
		} else {
			covenantFile = CovenantPath(globalDir, lang)
		}
	}
	// Load existing init.json addons + mcp fields so we preserve them across
	// regens. Critical for /setup: when the user changes non-addon settings,
	// existing addon registrations and MCP activations must not be dropped.
	// User edits always win over opts.Addons — opts only seeds the fields
	// on first creation.
	//
	// Reads both shapes for back-compat with init.json files written by older
	// TUIs (pre-v0.7.3 wrote a dict; new TUIs write a list). Both shapes get
	// converted to the new list-of-names form before re-writing, so the on-
	// disk file is normalized on the next refresh.
	var existingAddonsList []interface{}
	var existingMCP map[string]interface{}
	switch v := existingInit["addons"].(type) {
	case []interface{}:
		existingAddonsList = v
	case map[string]interface{}:
		// Legacy dict shape — extract just the names. Apply the same
		// identifier validation contract as the launch-time addon check so a
		// malformed or untrusted key can never be carried into a regenerated
		// init.json.
		for name := range v {
			if config.ValidateAddonKey(name) != nil {
				continue
			}
			existingAddonsList = append(existingAddonsList, name)
		}
	}
	if mcp, ok := existingInit["mcp"].(map[string]interface{}); ok && len(mcp) > 0 {
		existingMCP = mcp
	}

	envFile := config.EnvFilePath(globalDir)
	if existing, ok := existingInit["env_file"].(string); ok && existing != "" {
		envFile = existing
	}
	venvPath := filepath.Join(globalDir, "runtime", "venv")
	if existing, ok := existingInit["venv_path"].(string); ok && existing != "" {
		venvPath = existing
	}
	pad := interface{}("")
	if existing, ok := existingInit["pad"]; ok {
		pad = existing
	}

	initJSON := make(map[string]interface{}, len(existingInit)+8)
	for key, value := range existingInit {
		initJSON[key] = value
	}
	stripObsoleteInitFields(initJSON)
	initJSON["manifest"] = manifest
	initJSON["covenant_file"] = covenantFile
	initJSON["env_file"] = envFile
	initJSON["venv_path"] = venvPath
	initJSON["pad"] = pad
	// No seed-character field is written here. 灵台 (character) is durable
	// state owned by the agent and managed after creation via
	// system/lingtai.md / psyche — the kernel treats a missing seed as an
	// empty seed and lets the agent author its own character. The legacy
	// `prompt` field was an unknown key (boot warning, never honored); we
	// emit neither `prompt` nor `lingtai`.

	// Decide which addons to wire.
	//
	// Precedence:
	//   1. Pre-existing addons:[...] in init.json (preserved verbatim — user
	//      edits win).
	//   2. Otherwise, opts.Addons from the caller (the wizard's selection).
	//
	// The list is normalized to the new shape (list of curated MCP names).
	// The kernel's `mcp` capability decompresses each name from the catalog
	// into the per-agent mcp_registry.jsonl on boot.
	var addonsList []interface{}
	if existingAddonsList != nil {
		addonsList = existingAddonsList
	} else {
		for _, name := range opts.Addons {
			addonsList = append(addonsList, name)
		}
	}
	if addonsList != nil {
		initJSON["addons"] = addonsList
	}

	// Build the mcp activation map for any addon name in the list. Each entry
	// points at the local venv python (where `pip install lingtai` placed the
	// MCP packages) running `python -m lingtai_<name>` with the canonical
	// LINGTAI_<NAME>_CONFIG env var set to the .secrets/<name>.json convention.
	//
	// Pre-existing mcp.<name> entries take precedence — humans who customized
	// the spec (e.g., switched to a different Python or added env vars) keep
	// their settings.
	if len(addonsList) > 0 {
		venvPython := config.VenvPython(filepath.Join(globalDir, "runtime", "venv"))
		mcpField := make(map[string]interface{})
		for k, v := range existingMCP {
			mcpField[k] = v
		}
		for _, raw := range addonsList {
			name, ok := raw.(string)
			if !ok || name == "" {
				continue
			}
			if _, exists := mcpField[name]; exists {
				continue // user-set entry wins
			}
			module, envVar, configRel, supported := defaultMCPSpec(name)
			if !supported {
				continue // unknown name — let the kernel surface the warning
			}
			mcpField[name] = map[string]interface{}{
				"type":    "stdio",
				"command": venvPython,
				"args":    []interface{}{"-m", module},
				"env":     map[string]interface{}{envVar: configRel},
			}
		}
		if len(mcpField) > 0 {
			initJSON["mcp"] = mcpField
		}
	}

	// Comment file — only if user specified one
	if opts.CommentFile != "" {
		initJSON["comment_file"] = opts.CommentFile
	}

	data, err := json.MarshalIndent(initJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal init.json: %w", err)
	}

	initPath := filepath.Join(agentDir, "init.json")
	if err := lingfs.WriteFileAtomic(initPath, data, 0o644); err != nil {
		return fmt.Errorf("write init.json: %w", err)
	}

	// Persist the soul-flow opt-in into the global .env — the env_file the
	// agent inherits at boot (init.json "env_file" above points here). The
	// kernel reads LINGTAI_SOUL_FLOW_ENABLED from process env, so this is
	// the seam that actually turns soul flow on/off. SetEnvVar is merge-
	// preserving: it touches only this one key and leaves API keys,
	// comments, and unrelated vars intact. OFF removes the key rather than
	// writing =0, keeping the file minimal and matching the kernel default.
	optInValue := ""
	if opts.SoulFlowEnabled {
		optInValue = "1"
	}
	if err := config.SetEnvVar(globalDir, config.SoulFlowEnabledEnvVar, optInValue); err != nil {
		return fmt.Errorf("write soul-flow opt-in to .env: %w", err)
	}

	// Build the wizard-controlled subset of .agent.json. Other fields the
	// kernel populates at runtime (agent_id, created_at, molt_count,
	// language, soul_delay, soul_voice, started_at, capabilities,
	// nickname, etc) must NOT be touched here — re-running /setup against
	// an existing agent should preserve the agent's identity and history,
	// not reset it. Without this preservation, molt_count drops to 0 on
	// every /setup, which makes psyche overwrite earlier snapshots and
	// breaks soul-flow's "past self" continuity.
	agentManifest := map[string]interface{}{
		"agent_name": agentName,
		"address":    filepath.Base(agentDir),
		"admin": map[string]interface{}{
			"karma":   opts.Karma,
			"nirvana": opts.Nirvana,
		},
	}

	// Create mailbox structure
	for _, sub := range []string{
		"mailbox/inbox",
		"mailbox/sent",
		"mailbox/archive",
	} {
		os.MkdirAll(filepath.Join(agentDir, sub), 0o755)
	}

	// Merge with any existing .agent.json so kernel-owned identity fields
	// (molt_count etc.) survive a /setup-driven regen. The wizard owns the
	// keys it explicitly sets above; everything else is preserved verbatim.
	agentJSONPath := filepath.Join(agentDir, ".agent.json")
	merged := agentManifest
	if existing, err := os.ReadFile(agentJSONPath); err == nil {
		var prev map[string]interface{}
		if DecodeJSONUseNumber(existing, &prev) == nil {
			// Start from prev, then overwrite the wizard-controlled keys.
			merged = prev
			for k, v := range agentManifest {
				merged[k] = v
			}
		}
	} else {
		// Fresh agent — initialize state to "" so the kernel sees a blank.
		merged["state"] = ""
	}

	mdata, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal .agent.json: %w", err)
	}
	if err := lingfs.WriteFileAtomic(agentJSONPath, mdata, 0o644); err != nil {
		return fmt.Errorf("write .agent.json: %w", err)
	}

	return nil
}

// PropagatePresetPolicy rewrites manifest.preset.{default,allowed} on every
// agent under lingtaiDir (skipping the human pseudo-agent and the agent
// passed via skipDir, which the wizard's own save already handled).
//
// The intent: /setup is a network-wide preset-policy reset. Whatever the
// wizard's allowed list is becomes the authoritative allowed surface for
// all agents in the project; whatever the wizard's default is becomes
// every agent's default. Per-agent active is preserved when still in the
// new allowed list, otherwise demoted to the new default (which is always
// in allowed by construction).
//
// Best-effort per agent: malformed init.json or missing preset block is
// silently skipped so one bad agent doesn't block the propagation. Returns
// the count of agents successfully updated and the first error encountered
// (for surfacing to the user) — the walk doesn't abort on errors.
func PropagatePresetPolicy(lingtaiDir, skipDir, defaultRef string, allowed []string) (int, error) {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return 0, fmt.Errorf("read lingtai dir: %w", err)
	}

	allowedSet := map[string]struct{}{}
	for _, s := range allowed {
		allowedSet[s] = struct{}{}
	}

	var firstErr error
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "human" || name == skipDir {
			continue
		}
		agentDir := filepath.Join(lingtaiDir, name)
		// Skip non-agent dirs (no init.json).
		initPath := filepath.Join(agentDir, "init.json")
		if _, err := os.Stat(initPath); err != nil {
			continue
		}
		if err := rewritePresetBlock(initPath, defaultRef, allowed, allowedSet); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", name, err)
			}
			continue
		}
		count++
	}
	return count, firstErr
}

// rewritePresetBlock updates one agent's init.json preset block in place.
// Sets default and allowed to the network policy; preserves active if
// still in allowed, otherwise demotes to default. Silently no-ops when
// the agent has no preset block (older shape) — the kernel's regen on
// next boot will populate it.
func rewritePresetBlock(initPath, defaultRef string, allowed []string, allowedSet map[string]struct{}) error {
	data, err := os.ReadFile(initPath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	var raw map[string]interface{}
	if err := DecodeJSONUseNumber(data, &raw); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	stripObsoleteInitFields(raw)
	manifest, ok := raw["manifest"].(map[string]interface{})
	if !ok {
		return nil // no manifest, nothing to propagate
	}
	pre, ok := manifest["preset"].(map[string]interface{})
	if !ok {
		return nil // older shape — kernel handles regen
	}

	currentActive, _ := pre["active"].(string)
	newActive := currentActive
	if _, stillAllowed := allowedSet[currentActive]; !stillAllowed {
		newActive = defaultRef
	}

	// Materialize allowed as []interface{} so JSON round-trips cleanly.
	allowedJSON := make([]interface{}, len(allowed))
	for i, s := range allowed {
		allowedJSON[i] = s
	}

	pre["active"] = newActive
	pre["default"] = defaultRef
	pre["allowed"] = allowedJSON

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := lingfs.WriteFileAtomic(initPath, out, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
