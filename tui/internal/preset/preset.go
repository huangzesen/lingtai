package preset

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// BuiltinPresets returns the three built-in presets.
func BuiltinPresets() []Preset {
	return []Preset{
		minimaxPreset(),
		geminiPreset(),
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
				"vision": mm, "talk": mm, "draw": mm, "compose": mm,
				"listen": e(), "web_read": e(), "avatar": e(), "daemon": e(),
			},
			"admin": map[string]interface{}{"karma": true},
		},
	}
}

func geminiPreset() Preset {
	gm := map[string]interface{}{"provider": "gemini", "api_key_env": "GEMINI_API_KEY"}
	return Preset{
		Name:        "gemini",
		Description: "Gemini 3.0 Flash — vision + web search via Gemini",
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "gemini", "model": "gemini-3.0-flash",
				"api_key": nil, "api_key_env": "GEMINI_API_KEY", "base_url": nil,
			},
			"capabilities": map[string]interface{}{
				"file": e(), "email": e(), "bash": map[string]interface{}{"yolo": true},
				"web_search": gm, "psyche": e(), "library": e(),
				"vision": gm,
				"listen": e(), "web_read": e(), "avatar": e(), "daemon": e(),
			},
			"admin": map[string]interface{}{"karma": true},
		},
	}
}

func customPreset() Preset {
	return Preset{
		Name:        "custom",
		Description: "Custom provider — bring your own API key",
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider": "custom", "model": "",
				"api_key": nil, "api_key_env": "LLM_API_KEY", "base_url": nil,
			},
			"capabilities": map[string]interface{}{
				"file": e(), "email": e(), "bash": map[string]interface{}{"yolo": true},
				"web_search": e(), "psyche": e(), "library": e(),
				"vision": e(), "listen": e(), "web_read": e(), "avatar": e(), "daemon": e(),
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

// EnsureCovenants copies embedded covenants to ~/.lingtai/covenant/{lang}/covenant.md.
// Called once on TUI startup.
func EnsureCovenants(globalDir string) {
	for _, lang := range []string{"en", "zh", "wen"} {
		dir := filepath.Join(globalDir, "covenant", lang)
		target := filepath.Join(dir, "covenant.md")
		if _, err := os.Stat(target); err == nil {
			continue // already exists
		}
		os.MkdirAll(dir, 0o755)
		data := CovenantForLang(lang)
		if data != nil {
			os.WriteFile(target, data, 0o644)
		}
	}
}

// PrincipleForLang returns the embedded principle for the given language.
func PrincipleForLang(lang string) []byte {
	data, err := principleFS.ReadFile("principle/" + lang + "/principle.md")
	if err != nil {
		data, _ = principleFS.ReadFile("principle/en/principle.md")
	}
	return data
}

// EnsurePrinciples copies embedded principles to ~/.lingtai/principle/{lang}/principle.md.
func EnsurePrinciples(globalDir string) {
	for _, lang := range []string{"en", "zh", "wen"} {
		dir := filepath.Join(globalDir, "principle", lang)
		target := filepath.Join(dir, "principle.md")
		if _, err := os.Stat(target); err == nil {
			continue
		}
		os.MkdirAll(dir, 0o755)
		data := PrincipleForLang(lang)
		if data != nil {
			os.WriteFile(target, data, 0o644)
		}
	}
}

// PrinciplePath returns the absolute path to the principle file for a language.
func PrinciplePath(globalDir, lang string) string {
	return filepath.Join(globalDir, "principle", lang, "principle.md")
}

// EnsureTemplates copies embedded example files to ~/.lingtai/templates/.
func EnsureTemplates(globalDir string) {
	dir := filepath.Join(globalDir, "templates")
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		target := filepath.Join(dir, e.Name())
		if _, err := os.Stat(target); err == nil {
			continue // already exists
		}
		os.MkdirAll(dir, 0o755)
		data, err := templatesFS.ReadFile("templates/" + e.Name())
		if err == nil {
			os.WriteFile(target, data, 0o644)
		}
	}
}


// CovenantPath returns the absolute path to the covenant file for a language.
func CovenantPath(globalDir, lang string) string {
	return filepath.Join(globalDir, "covenant", lang, "covenant.md")
}

// DefaultPreset returns the first built-in preset (minimax).
func DefaultPreset() Preset {
	return minimaxPreset()
}

// GenerateInitJSON creates a full init.json from a preset at .lingtai/<agentName>/init.json.
func GenerateInitJSON(p Preset, agentName, dirName, lingtaiDir, globalDir string) error {
	agentDir := filepath.Join(lingtaiDir, dirName)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}

	// Build manifest with defaults
	manifest := make(map[string]interface{})
	manifest["agent_name"] = agentName
	// Use language from preset, default to "en"
	lang := "en"
	if l, ok := p.Manifest["language"].(string); ok && l != "" {
		lang = l
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
	manifest["soul"] = map[string]interface{}{"delay": 120}
	manifest["stamina"] = 36000
	manifest["context_limit"] = 200000
	manifest["molt_pressure"] = 0.8
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
