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
)

//go:embed all:covenant
var covenantFS embed.FS

//go:embed all:principle
var principleFS embed.FS

//go:embed all:templates
var templatesFS embed.FS

// Preset is a reusable agent template stored at ~/.lingtai/presets/.
type Preset struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Manifest    map[string]interface{} `json:"manifest"`
}

// PresetsDir returns ~/.lingtai/presets/.
func PresetsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lingtai", "presets")
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
	// Put minimax first (recommended)
	sort.Slice(presets, func(i, j int) bool {
		if presets[i].Name == "minimax" {
			return true
		}
		if presets[j].Name == "minimax" {
			return false
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
			},
			"admin": map[string]interface{}{"karma": true},
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
				"vision": e(), "web_read": e(), "avatar": e(), "daemon": e(),
				"listen": e(),
			},
			"admin": map[string]interface{}{"karma": true},
		},
	}
}

// CovenantForLang returns the embedded covenant for the given language.
func CovenantForLang(lang string) []byte {
	data, err := covenantFS.ReadFile("covenant/" + lang + "/covenant.md")
	if err != nil {
		data, _ = covenantFS.ReadFile("covenant/en/covenant.md")
	}
	return data
}

// PrincipleForLang returns the embedded principle for the given language.
func PrincipleForLang(lang string) []byte {
	data, err := principleFS.ReadFile("principle/" + lang + "/principle.md")
	if err != nil {
		data, _ = principleFS.ReadFile("principle/en/principle.md")
	}
	return data
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
		if _, err := os.Stat(target); err == nil {
			return nil // already exists
		}
		os.MkdirAll(filepath.Dir(target), 0o755)
		data, err := fsys.ReadFile(path)
		if err == nil {
			os.WriteFile(target, data, 0o644)
		}
		return nil
	})
}

// migrateAddonTemplates copies legacy ~/.lingtai/templates/{imap,telegram}.jsonc
// to ~/.lingtai/addons/{addon}/example/config.json as plain JSON.
// Skips if the target already exists.
func migrateAddonTemplates(globalDir string) {
	for _, addon := range []string{"imap", "telegram"} {
		target := filepath.Join(globalDir, "addons", addon, "example", "config.json")
		if _, err := os.Stat(target); err == nil {
			continue // already migrated
		}

		// Read from embedded FS (same source as populate uses for templates/)
		src := filepath.Join("templates", addon+".jsonc")
		data, err := templatesFS.ReadFile(src)
		if err != nil {
			continue
		}

		// Strip JSONC comments and trailing commas → plain JSON
		plain := stripJSONC(data)

		os.MkdirAll(filepath.Dir(target), 0o755)
		os.WriteFile(target, plain, 0o644)
	}
}

// stripJSONC removes // comments and trailing commas from JSONC bytes.
func stripJSONC(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	var out []string
	for _, line := range lines {
		// Remove // comments, but not inside quoted strings (e.g. URLs with //)
		inString := false
		stripped := line
		for i := 0; i < len(line)-1; i++ {
			if line[i] == '"' && (i == 0 || line[i-1] != '\\') {
				inString = !inString
			}
			if !inString && line[i] == '/' && line[i+1] == '/' {
				stripped = line[:i]
				break
			}
		}
		out = append(out, stripped)
	}
	text := strings.Join(out, "\n")
	// Remove trailing commas before } or ]
	for _, ch := range []string{"}", "]"} {
		text = strings.ReplaceAll(text, ",\n"+ch, "\n"+ch)
		text = strings.ReplaceAll(text, ", "+ch, " "+ch)
	}
	// Compact: parse and re-marshal to get clean JSON
	var parsed interface{}
	if err := json.Unmarshal([]byte(text), &parsed); err == nil {
		if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
			return append(pretty, '\n')
		}
	}
	// Fallback: return stripped text as-is
	return []byte(text)
}

// Bootstrap populates all embedded assets and default presets at ~/.lingtai/.
func Bootstrap(globalDir string) error {
	populate(globalDir, covenantFS, "covenant")
	populate(globalDir, principleFS, "principle")
	populate(globalDir, templatesFS, "templates")
	migrateAddonTemplates(globalDir)
	return EnsureDefault()
}


// CovenantPath returns the absolute path to the covenant file for a language.
func CovenantPath(globalDir, lang string) string {
	return filepath.Join(globalDir, "covenant", lang, "covenant.md")
}

// DefaultPreset returns the first built-in preset (minimax).
func DefaultPreset() Preset {
	return minimaxPreset()
}

// AgentOpts holds per-agent configuration values set at creation time.
type AgentOpts struct {
	Language     string  // "en", "zh", or "wen"
	Stamina      float64 // max uptime in seconds
	ContextLimit int     // token budget
	SoulDelay    float64 // seconds between soul cycles
	MoltPressure float64 // 0–1 ratio triggering molt
}

// DefaultAgentOpts returns sensible defaults for agent creation.
func DefaultAgentOpts() AgentOpts {
	return AgentOpts{
		Language:     "en",
		Stamina:      36000,
		ContextLimit: 200000,
		SoulDelay:    120,
		MoltPressure: 0.8,
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
	if admin, ok := p.Manifest["admin"]; ok {
		manifest["admin"] = admin
	}
	manifest["soul"] = map[string]interface{}{"delay": opts.SoulDelay}
	manifest["stamina"] = opts.Stamina
	manifest["context_limit"] = opts.ContextLimit
	manifest["molt_pressure"] = opts.MoltPressure
	manifest["molt_prompt"] = ""
	manifest["max_turns"] = 100
	manifest["streaming"] = true

	// Comment: persistent app-level system prompt for the orchestrator
	comment := "You are the 本我 (orchestrator) — the primary agent the human interacts with. " +
		"Templates and examples for setting up IMAP email and Telegram integrations " +
		"are available at " + filepath.Join(globalDir, "templates") + "/ — " +
		"guide the human there if they want to connect external services. " +
		"Covenants for all languages are at " + filepath.Join(globalDir, "covenant") + "/."

	// Resolve venv path: prefer runtime/venv/, fallback to legacy env/
	venvPath := filepath.Join(globalDir, "runtime", "venv")
	if _, err := os.Stat(filepath.Join(venvPath, "bin", "python")); err != nil {
		legacyVenv := filepath.Join(globalDir, "env")
		if _, err := os.Stat(filepath.Join(legacyVenv, "bin", "python")); err == nil {
			venvPath = legacyVenv
		}
	}

	initJSON := map[string]interface{}{
		"manifest":       manifest,
		"covenant_file":  CovenantPath(globalDir, lang),
		"principle_file": PrinciplePath(globalDir, lang),
		"venv_path":     venvPath,
		"memory":        "",
		"prompt":        "",
		"comment":       comment,
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
	absDir, _ := filepath.Abs(agentDir)
	agentManifest := map[string]interface{}{
		"agent_name": agentName,
		"address":    absDir,
		"state":      "",
		"admin":      p.Manifest["admin"],
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
