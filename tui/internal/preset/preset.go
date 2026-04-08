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

//go:embed all:templates
var templatesFS embed.FS

//go:embed all:soul
var soulFS embed.FS

//go:embed all:greet
var greetFS embed.FS

//go:embed tutorial.md
var tutorialMD []byte

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
		if bi { // both builtin: minimax first
			if presets[i].Name == "minimax" {
				return true
			}
			if presets[j].Name == "minimax" {
				return false
			}
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
		customPreset(),
	}
}

// builtinNames is the set of built-in preset names.
var builtinNames = map[string]bool{
	"minimax": true,
	"custom":  true,
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
	populate(globalDir, soulFS, "soul")
	populate(globalDir, greetFS, "greet")
	populate(globalDir, templatesFS, "templates")
	// Tutorial comment file — now in tutorial/ subfolder
	tutorialDir := filepath.Join(globalDir, "tutorial")
	os.MkdirAll(tutorialDir, 0o755)
	tutorialPath := filepath.Join(tutorialDir, "tutorial.md")
	os.WriteFile(tutorialPath, tutorialMD, 0o644)
	return EnsureDefault()
}

// PopulateBundledSkills writes the bundled skills into the project's
// .lingtai/.skills/ directory. Skips files that already exist so user
// modifications are preserved.
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
}

// CovenantPath returns the absolute path to the covenant file for a language.
func CovenantPath(globalDir, lang string) string {
	return filepath.Join(globalDir, "covenant", lang, "covenant.md")
}

// TutorialCommentPath returns the absolute path to the tutorial comment file.
func TutorialCommentPath(globalDir string) string {
	return filepath.Join(globalDir, "tutorial", "tutorial.md")
}

// SoulFlowPath returns the absolute path to the soul flow file for a language.
func SoulFlowPath(globalDir, lang string) string {
	return filepath.Join(globalDir, "soul", lang, "soul-flow.md")
}

// GreetPath returns the absolute path to the greet prompt file for a language.
func GreetPath(globalDir, lang string) string {
	return filepath.Join(globalDir, "greet", lang, "greet.md")
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
	CovenantFile  string   // path to covenant file
	PrincipleFile string   // path to principle file
	SoulFile      string   // path to soul flow file
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
		"manifest":       manifest,
		"covenant_file":  covenantFile,
		"principle_file": principleFile,
		"soul_file":      soulFile,
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
		addonsField := make(map[string]interface{}, len(opts.Addons))
		for _, name := range opts.Addons {
			addonsField[name] = map[string]interface{}{
				"config": AddonConfigPathFromAgent(name),
			}
		}
		initJSON["addons"] = addonsField
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

// GenerateTutorialInit creates a tutorial agent's init.json at {lingtaiDir}/tutorial/.
// The tutorial agent has a pre-defined name ("guide"), hardcoded lifecycle params,
// and a detailed comment (system prompt) that instructs it to teach the human step by step.
func GenerateTutorialInit(p Preset, lingtaiDir, globalDir, lang string) error {
	agentDir := filepath.Join(lingtaiDir, "tutorial")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("create tutorial dir: %w", err)
	}

	// Build manifest — inherit capabilities from the preset
	manifest := make(map[string]interface{})
	manifest["agent_name"] = "guide"
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
	manifest["admin"] = map[string]interface{}{"karma": true}
	manifest["soul"] = map[string]interface{}{"delay": 999999}
	manifest["stamina"] = 36000
	manifest["context_limit"] = 200000
	manifest["molt_pressure"] = 0.8
	manifest["molt_prompt"] = ""
	manifest["max_turns"] = 100
	manifest["streaming"] = false

	initJSON := map[string]interface{}{
		"manifest":       manifest,
		"covenant_file":  CovenantPath(globalDir, lang),
		"principle_file": PrinciplePath(globalDir, lang),
		"soul_file":      SoulFlowPath(globalDir, lang),
		"env_file":       config.EnvFilePath(globalDir),
		"comment_file":   TutorialCommentPath(globalDir),
		"venv_path":      filepath.Join(globalDir, "runtime", "venv"),
		"memory":         "",
		"prompt":         "",
	}

	data, err := json.MarshalIndent(initJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tutorial init.json: %w", err)
	}

	if err := os.WriteFile(filepath.Join(agentDir, "init.json"), data, 0o644); err != nil {
		return fmt.Errorf("write tutorial init.json: %w", err)
	}

	// Create .agent.json and mailbox dirs (same as GenerateInitJSONWithOpts)
	agentManifest := map[string]interface{}{
		"agent_name": "guide",
		"address":    "tutorial",
		"state":      "",
		"admin":      map[string]interface{}{"karma": true},
	}
	for _, sub := range []string{"mailbox/inbox", "mailbox/sent", "mailbox/archive"} {
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
