package preset

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anthropics/lingtai-tui/internal/config"
)

//go:embed all:covenant
var covenantFS embed.FS

//go:embed all:principle
var principleFS embed.FS

//go:embed all:procedures
var proceduresFS embed.FS

//go:embed all:templates
var templatesFS embed.FS

//go:embed all:soul
var soulFS embed.FS

//go:embed all:recipe_assets
var recipeAssetsFS embed.FS

//go:embed all:skills
var skillsFS embed.FS

// Preset is a reusable agent template stored at ~/.lingtai-tui/presets/.
type Preset struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Manifest    map[string]interface{} `json:"manifest"`
}

// PresetsDir returns ~/.lingtai-tui/presets/.
func PresetsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, config.GlobalDirName, "presets")
}

// List returns all presets from the presets directory.
func List() ([]Preset, error) {
	dir := PresetsDir()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read presets dir: %w", err)
	}
	var presets []Preset
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		p, err := Load(e.Name()[:len(e.Name())-5]) // strip .json
		if err != nil {
			continue
		}
		presets = append(presets, p)
	}
	// Saved presets first (alphabetically), then builtins (minimax first)
	sort.Slice(presets, func(i, j int) bool {
		bi, bj := IsBuiltin(presets[i].Name), IsBuiltin(presets[j].Name)
		if bi != bj {
			return !bi // saved (non-builtin) before builtin
		}
		if bi { // both builtin: minimax → zhipu → custom
			order := map[string]int{"minimax": 0, "zhipu": 1, "codex": 2, "custom": 3}
			return order[presets[i].Name] < order[presets[j].Name]
		}
		return presets[i].Name < presets[j].Name
	})
	return presets, nil
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

// Load reads a single preset by name.
func Load(name string) (Preset, error) {
	path := filepath.Join(PresetsDir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Preset{}, fmt.Errorf("read preset %s: %w", name, err)
	}
	var p Preset
	if err := json.Unmarshal(data, &p); err != nil {
		return Preset{}, fmt.Errorf("parse preset %s: %w", name, err)
	}
	return p, nil
}

// Save writes a preset to the presets directory.
func Save(p Preset) error {
	dir := PresetsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create presets dir: %w", err)
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
		json.Unmarshal(data, &manifest)
	}
	return Preset{
		Name:        newName,
		Description: src.Description,
		Manifest:    manifest,
	}
}

// Delete removes a preset file.
func Delete(name string) error {
	path := filepath.Join(PresetsDir(), name+".json")
	return os.Remove(path)
}

// EnsureDefaults creates all built-in presets if no presets exist.
func EnsureDefault() error {
	presets, _ := List()
	if len(presets) > 0 {
		return nil
	}
	for _, p := range BuiltinPresets() {
		if err := Save(p); err != nil {
			return err
		}
	}
	return nil
}

// BuiltinPresets returns the built-in presets.
func BuiltinPresets() []Preset {
	return []Preset{
		minimaxPreset(),
		zhipuPreset(),
		codexPreset(),
		customPreset(),
	}
}

// builtinNames is the set of built-in preset names.
var builtinNames = map[string]bool{
	"minimax":     true,
	"zhipu":       true,
	"codex":       true,
	"codex_oauth": true,
	"custom":      true,
}

// IsBuiltin returns true if name matches a built-in preset template.
func IsBuiltin(name string) bool {
	return builtinNames[name]
}

// SavedCount returns the number of non-builtin (saved) presets in the list.
func SavedCount(presets []Preset) int {
	n := 0
	for _, p := range presets {
		if !IsBuiltin(p.Name) {
			n++
		}
	}
	return n
}

func e() map[string]interface{} { return map[string]interface{}{} }

func minimaxPreset() Preset {
	mm := map[string]interface{}{"provider": "minimax", "api_key_env": "MINIMAX_API_KEY"}
	return Preset{
		Name:        "minimax",
		Description: "MiniMax M2.7 — full multimodal capabilities",
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "minimax", "model": "MiniMax-M2.7-highspeed",
				"api_key": nil, "api_key_env": "MINIMAX_API_KEY", "base_url": nil,
			},
			"capabilities": map[string]interface{}{
				"file": e(), "email": e(), "bash": map[string]interface{}{"yolo": true},
				"web_search": mm, "psyche": e(), "library": e(),
				"vision": mm, "talk": mm, "draw": mm, "video": mm, "compose": mm,
				"listen": e(), "web_read": e(), "avatar": e(), "daemon": e(),
				"skills": e(),
			},
			"admin": map[string]interface{}{"karma": true},
			"streaming": false,
		},
	}
}

func zhipuPreset() Preset {
	zp := map[string]interface{}{"provider": "zhipu", "api_key_env": "ZHIPU_API_KEY"}
	return Preset{
		Name:        "zhipu",
		Description: "Zhipu GLM Coding Plan — OpenAI-compatible",
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "zhipu", "model": "GLM-5.1",
				"api_key": nil, "api_key_env": "ZHIPU_API_KEY",
				"base_url": nil, "api_compat": "openai",
			},
			"capabilities": map[string]interface{}{
				"file": e(), "email": e(), "bash": map[string]interface{}{"yolo": true},
				"web_search": zp, "psyche": e(), "library": e(),
				"vision": zp, "web_read": zp,
				"avatar": e(), "daemon": e(),
				"listen": e(), "skills": e(),
			},
			"admin":     map[string]interface{}{"karma": true},
			"streaming": false,
		},
	}
}

func codexPreset() Preset {
	cx := map[string]interface{}{"provider": "codex", "api_key_env": ""}
	return Preset{
		Name:        "codex",
		Description: "ChatGPT account — vision + web search + tools",
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "codex", "model": "gpt-5.4",
				"api_key": nil, "api_key_env": "",
				"base_url": "https://chatgpt.com/backend-api",
			},
			"capabilities": map[string]interface{}{
				"file": e(), "email": e(), "bash": map[string]interface{}{"yolo": true},
				"web_search": cx, "psyche": e(), "library": e(),
				"vision": cx, "web_read": cx,
				"avatar": e(), "daemon": e(),
				"listen": e(), "skills": e(),
			},
			"admin":     map[string]interface{}{"karma": true},
			"streaming": false,
		},
	}
}

func customPreset() Preset {
	return Preset{
		Name:        "custom",
		Description: "OpenAI-compatible API — full capabilities",
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "custom", "model": "",
				"api_key": nil, "api_key_env": "LLM_API_KEY", "base_url": nil,
			},
			"capabilities": map[string]interface{}{
				"file": e(), "email": e(), "bash": map[string]interface{}{"yolo": true},
				"web_search": e(), "psyche": e(), "library": e(),
				"web_read": e(), "avatar": e(), "daemon": e(),
				"listen": e(), "skills": e(),
			},
			"admin": map[string]interface{}{"karma": true},
			"streaming": false,
		},
	}
}

// PrinciplePath returns the absolute path to the principle file for a language.
func PrinciplePath(globalDir, lang string) string {
	return filepath.Join(globalDir, "principle", lang, "principle.md")
}

// ProceduresPath returns the absolute path to the procedures file for a language.
func ProceduresPath(globalDir, lang string) string {
	return filepath.Join(globalDir, "procedures", lang, "procedures.md")
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
	return EnsureDefault()
}

// PopulateBundledSkills writes the bundled (canonical) skills into the
// project's .lingtai/.skills/ directory, overwriting any existing files
// at the same path. Called on every TUI startup so canonical skills stay
// in sync with the shipped binary.
//
// Only paths present in the embedded skills/ tree are touched. Files the
// user has added under .lingtai/.skills/ that are NOT part of the bundled
// set are left alone — that is how users add their own custom skills.
// Edits to canonical skills are not preserved (by design: they are
// tui-managed and refreshed on each launch).
func PopulateBundledSkills(lingtaiDir string) {
	skillsDir := filepath.Join(lingtaiDir, ".skills")
	fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel("skills", path)
		target := filepath.Join(skillsDir, rel)
		os.MkdirAll(filepath.Dir(target), 0o755)
		data, err := skillsFS.ReadFile(path)
		if err == nil {
			os.WriteFile(target, data, 0o644)
		}
		return nil
	})

	// Clean up skills removed or renamed between TUI versions.
	// Only non-symlink directories are removed (symlinks are recipe-managed).
	removedSkills := []string{
		"lingtai-agora", // renamed to lingtai-export-network in v0.4.40
	}
	for _, name := range removedSkills {
		p := filepath.Join(skillsDir, name)
		info, err := os.Lstat(p)
		if err != nil {
			continue // doesn't exist — nothing to do
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue // symlink — managed by recipe_skills.go, leave it
		}
		os.RemoveAll(p)
	}
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

// DefaultPreset returns the first built-in preset (minimax).
func DefaultPreset() Preset {
	return minimaxPreset()
}

// AgentOpts holds per-agent configuration values set at creation time.
type AgentOpts struct {
	Language      string   // "en", "zh", or "wen"
	Stamina       float64  // max uptime in seconds
	ContextLimit  int      // token budget
	SoulDelay     float64  // seconds between soul cycles
	MoltPressure  float64  // 0–1 ratio triggering molt
	Karma         bool     // lifecycle control over other agents
	Nirvana       bool     // permanent agent destruction
	CovenantFile   string   // path to covenant file
	PrincipleFile  string   // path to principle file
	ProceduresFile string   // path to procedures file
	BriefFile      string   // path to brief file (externally maintained by secretary)
	SoulFile       string   // path to soul flow file
	CommentFile   string   // path to comment file (optional)
	Addons        []string // addon names to auto-populate in init.json (e.g. ["imap", "telegram"])
}

// DefaultAgentOpts returns sensible defaults for agent creation.
func DefaultAgentOpts() AgentOpts {
	return AgentOpts{
		Language:     "en",
		Stamina:      36000,
		ContextLimit: 200000,
		SoulDelay:    120,
		MoltPressure: 0.8,
		Karma:        true,
		Nirvana:      false,
	}
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

// GenerateInitJSONWithOpts creates a full init.json from a preset with explicit agent options.
func GenerateInitJSONWithOpts(p Preset, agentName, dirName, lingtaiDir, globalDir string, opts AgentOpts) error {
	agentDir := filepath.Join(lingtaiDir, dirName)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}

	// Build manifest with opts
	manifest := make(map[string]interface{})
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
	manifest["admin"] = map[string]interface{}{
		"karma":   opts.Karma,
		"nirvana": opts.Nirvana,
	}
	manifest["soul"] = map[string]interface{}{"delay": opts.SoulDelay}
	manifest["stamina"] = opts.Stamina
	manifest["context_limit"] = opts.ContextLimit
	manifest["molt_pressure"] = opts.MoltPressure
	manifest["molt_prompt"] = ""
	manifest["max_turns"] = 100
	manifest["streaming"] = false

	// Resolve file paths — use opts if set, fallback to language defaults
	covenantFile := opts.CovenantFile
	if covenantFile == "" {
		covenantFile = CovenantPath(globalDir, lang)
	}
	principleFile := opts.PrincipleFile
	if principleFile == "" {
		principleFile = PrinciplePath(globalDir, lang)
	}
	proceduresFile := opts.ProceduresFile
	if proceduresFile == "" {
		proceduresFile = ProceduresPath(globalDir, lang)
	}
	soulFile := opts.SoulFile
	if soulFile == "" {
		soulFile = SoulFlowPath(globalDir, lang)
	}

	// Load existing init.json addons field so we preserve it across regens.
	// This is critical for /setup: when the user changes non-addon settings,
	// the existing addon configuration must not be dropped. User edits
	// always win over opts.Addons — opts only seeds the field on first creation.
	var existingAddons map[string]interface{}
	existingInitPath := filepath.Join(agentDir, "init.json")
	if existingData, err := os.ReadFile(existingInitPath); err == nil {
		var existing map[string]interface{}
		if json.Unmarshal(existingData, &existing) == nil {
			if addons, ok := existing["addons"].(map[string]interface{}); ok && len(addons) > 0 {
				existingAddons = addons
			}
		}
	}

	initJSON := map[string]interface{}{
		"manifest":         manifest,
		"covenant_file":    covenantFile,
		"principle_file":   principleFile,
		"procedures_file":  proceduresFile,
		"soul_file":        soulFile,
		"env_file":       config.EnvFilePath(globalDir),
		"venv_path":      filepath.Join(globalDir, "runtime", "venv"),
		"memory":         "",
		"prompt":         "",
	}
	if existingAddons != nil {
		// Preserve user-edited addon config (takes precedence over opts.Addons)
		initJSON["addons"] = existingAddons
	} else if len(opts.Addons) > 0 {
		// First-creation: seed init.json.addons from opts, pointing each declared
		// addon to its fixed-by-convention config path. The path is relative to
		// the agent's working_dir (<project>/.lingtai/<agent>/), so "../" escapes
		// the agent dir and "/.addons/<name>/config.json" reaches the project-level
		// shared addon config directory at <project>/.lingtai/.addons/.
		//
		// Filter: only wire an addon if its config.json already exists on disk.
		// If a user selects an addon in the wizard but hasn't created the config
		// file yet (via a setup skill, manual edit, or recipient-side setup
		// after cloning), we "let it be" — the wizard selection is a no-op
		// rather than producing a stale entry that makes the kernel emit
		// "failed to load" system messages on every launch. Once the user
		// creates the config later, they can re-run /setup to wire it up.
		addonsField := make(map[string]interface{})
		for _, name := range opts.Addons {
			// Absolute path on disk: <lingtaiDir>/.addons/<name>/config.json.
			// (lingtaiDir is already the .lingtai/ directory, so no leading
			// ".lingtai/" — that's only in AddonConfigRelPath which is
			// relative to the project root.)
			absPath := filepath.Join(lingtaiDir, ".addons", name, "config.json")
			if _, err := os.Stat(absPath); err != nil {
				continue // config missing — skip silently
			}
			addonsField[name] = map[string]interface{}{
				"config": AddonConfigPathFromAgent(name),
			}
		}
		if len(addonsField) > 0 {
			initJSON["addons"] = addonsField
		}
	}

	// Comment file — only if user specified one
	if opts.CommentFile != "" {
		initJSON["comment_file"] = opts.CommentFile
	}

	// Brief file — externally maintained by the secretary agent.
	// Only set for admin agents (karma=true); avatars don't need it.
	if opts.BriefFile != "" && opts.Karma {
		initJSON["brief_file"] = opts.BriefFile
	}

	data, err := json.MarshalIndent(initJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal init.json: %w", err)
	}

	initPath := filepath.Join(agentDir, "init.json")
	if err := os.WriteFile(initPath, data, 0o644); err != nil {
		return fmt.Errorf("write init.json: %w", err)
	}

	// Also create .agent.json manifest for the agent
	agentManifest := map[string]interface{}{
		"agent_name": agentName,
		"address":    filepath.Base(agentDir),
		"state":      "",
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

	mdata, _ := json.MarshalIndent(agentManifest, "", "  ")
	os.WriteFile(filepath.Join(agentDir, ".agent.json"), mdata, 0o644)

	return nil
}

// CapabilityIcons returns emoji icons for enabled capabilities in a preset.
func (p Preset) CapabilityIcons() string {
	var icons []string
	caps, ok := p.Manifest["capabilities"].(map[string]interface{})
	if !ok {
		return ""
	}

	iconMap := map[string]string{
		"file":       "📄",
		"email":      "📧",
		"bash":       "💻",
		"web_search": "🔍",
		"psyche":     "🧠",
		"library":    "📚",
		"vision":     "👁️",
		"talk":       "🔊",
		"draw":       "🎨",
		"compose":    "🎵",
		"video":      "🎬",
		"listen":     "👂",
		"web_read":   "📖",
		"avatar":     "👤",
		"daemon":     "⚡",
	}

	for key, val := range caps {
		if val == nil {
			continue
		}
		if m, ok := val.(map[string]interface{}); ok && len(m) == 0 {
			continue
		}
		if icon, ok := iconMap[key]; ok {
			icons = append(icons, icon)
		}
	}

	var b strings.Builder
	for i, icon := range icons {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(icon)
	}
	return b.String()
}
