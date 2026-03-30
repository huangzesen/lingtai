package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/preset"
)

// UsePresetMsg is emitted when a preset is selected for use.
type UsePresetMsg struct {
	Name string
}

type presetsMode int

const (
	presetsListMode presetsMode = iota
	presetsEditorMode
	presetsNewMode // prompting for new preset name
)

// PresetsModel is the /presets view — list + editor.
type PresetsModel struct {
	mode    presetsMode
	presets []preset.Preset
	cursor  int
	message string
	width   int
	height  int

	// Editor state
	editPreset preset.Preset
	editCursor int
	editFields []presetField

	// New preset name input
	nameInput textinput.Model
}

type presetField struct {
	Key     string
	Label   string
	Options []string
	Current int
	IsBool  bool
	IsText  bool   // free-text input (e.g. model name)
	Text    string // current text value
}

// AllCapabilities is the list of all available capability names.
var AllCapabilities = []string{
	"file", "email", "bash", "web_search", "psyche", "library",
	"vision", "talk", "draw", "compose", "video", "listen", "web_read",
	"avatar", "daemon",
}

func NewPresetsModel() PresetsModel {
	ti := textinput.New()
	ti.Placeholder = i18n.T("presets.enter_name")
	ti.CharLimit = 64
	ti.SetWidth(40)

	presets, _ := preset.List()
	return PresetsModel{
		presets:   presets,
		nameInput: ti,
	}
}

func (m PresetsModel) Init() tea.Cmd { return nil }

func (m PresetsModel) Update(msg tea.Msg) (PresetsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch m.mode {
		case presetsListMode:
			return m.updateList(msg)
		case presetsEditorMode:
			return m.updateEditor(msg)
		case presetsNewMode:
			return m.updateNew(msg)
		}
	}
	return m, nil
}

func (m PresetsModel) updateList(msg tea.KeyPressMsg) (PresetsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
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
			return m, func() tea.Msg {
				return UsePresetMsg{Name: m.presets[m.cursor].Name}
			}
		}
	case "e":
		if m.cursor < len(m.presets) {
			m.editPreset = m.presets[m.cursor]
			m.editFields = buildEditFields(m.editPreset)
			m.editCursor = 0
			m.mode = presetsEditorMode
		}
	case "n":
		m.nameInput.SetValue("")
		m.nameInput.Focus()
		m.mode = presetsNewMode
		return m, textinput.Blink
	case "d":
		if m.cursor < len(m.presets) {
			preset.Delete(m.presets[m.cursor].Name)
			m.presets, _ = preset.List()
			if m.cursor >= len(m.presets) && m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

func (m PresetsModel) updateEditor(msg tea.KeyPressMsg) (PresetsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Save and return to list
		preset.Save(m.editPreset)
		m.presets, _ = preset.List()
		m.mode = presetsListMode
		return m, nil
	case "up":
		if m.editCursor > 0 {
			m.editCursor--
		}
	case "down":
		if m.editCursor < len(m.editFields)-1 {
			m.editCursor++
		}
	case "left":
		f := &m.editFields[m.editCursor]
		if f.Current > 0 {
			f.Current--
			applyEditField(&m.editPreset, f)
		}
	case "right", "space":
		f := &m.editFields[m.editCursor]
		if f.Current < len(f.Options)-1 {
			f.Current++
			applyEditField(&m.editPreset, f)
		}
	}
	return m, nil
}

func (m PresetsModel) updateNew(msg tea.KeyPressMsg) (PresetsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = presetsListMode
		return m, nil
	case "enter":
		name := m.nameInput.Value()
		if name == "" {
			return m, nil
		}
		// Create minimal preset
		p := preset.Preset{
			Name:        name,
			Description: "",
			Manifest: map[string]interface{}{
				"llm": map[string]interface{}{
					"provider":    "minimax",
					"model":       "MiniMax-M2.7-highspeed",
					"api_key":     nil,
					"api_key_env": "MINIMAX_API_KEY",
				},
				"capabilities": map[string]interface{}{
					"file": map[string]interface{}{},
				},
				"admin": map[string]interface{}{"karma": true},
			},
		}
		preset.Save(p)
		m.presets, _ = preset.List()
		// Enter editor for new preset
		m.editPreset = p
		m.editFields = buildEditFields(p)
		m.editCursor = 0
		m.mode = presetsEditorMode
		return m, nil
	default:
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd
	}
}

func buildEditFields(p preset.Preset) []presetField {
	var fields []presetField

	// Provider
	providers := []string{"minimax", "gemini", "openai", "anthropic", "custom"}
	provCurrent := 0
	if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
		if prov, ok := llm["provider"].(string); ok {
			for i, pr := range providers {
				if pr == prov {
					provCurrent = i
					break
				}
			}
		}
	}
	fields = append(fields, presetField{Key: "provider", Label: "presets.provider", Options: providers, Current: provCurrent})

	// Model — provider-dependent options
	currentProvider := providers[provCurrent]
	models := modelsForProvider(currentProvider)
	modelCurrent := 0
	if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
		if model, ok := llm["model"].(string); ok {
			for i, m := range models {
				if m == model {
					modelCurrent = i
					break
				}
			}
		}
	}
	fields = append(fields, presetField{Key: "model", Label: "presets.model", Options: models, Current: modelCurrent})

	// Endpoint — MiniMax only: international vs china
	if currentProvider == "minimax" {
		endpoints := []string{"international", "china"}
		epCurrent := 0
		if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
			if bu, ok := llm["base_url"].(string); ok && bu == "https://api.minimaxi.com/anthropic" {
				epCurrent = 1
			}
		}
		fields = append(fields, presetField{Key: "endpoint", Label: "presets.endpoint", Options: endpoints, Current: epCurrent})
	}

	// Language
	langs := []string{"en", "zh", "wen"}
	langCurrent := 0
	if l, ok := p.Manifest["language"].(string); ok {
		for i, lang := range langs {
			if lang == l {
				langCurrent = i
				break
			}
		}
	}
	fields = append(fields, presetField{Key: "language", Label: "presets.language", Options: langs, Current: langCurrent})

	// Karma
	boolOpts := []string{"false", "true"}
	karmaCurrent := 0
	if admin, ok := p.Manifest["admin"].(map[string]interface{}); ok {
		if karma, ok := admin["karma"].(bool); ok && karma {
			karmaCurrent = 1
		}
	}
	fields = append(fields, presetField{Key: "karma", Label: "presets.karma", Options: boolOpts, Current: karmaCurrent, IsBool: true})

	// Nirvana
	nirvanaCurrent := 0
	if admin, ok := p.Manifest["admin"].(map[string]interface{}); ok {
		if nir, ok := admin["nirvana"].(bool); ok && nir {
			nirvanaCurrent = 1
		}
	}
	fields = append(fields, presetField{Key: "nirvana", Label: "presets.nirvana", Options: boolOpts, Current: nirvanaCurrent, IsBool: true})

	// Capabilities (one field per capability)
	caps := make(map[string]bool)
	if capsMap, ok := p.Manifest["capabilities"].(map[string]interface{}); ok {
		for k := range capsMap {
			caps[k] = true
		}
	}
	for _, capName := range AllCapabilities {
		current := 0
		if caps[capName] {
			current = 1
		}
		fields = append(fields, presetField{
			Key:     "cap:" + capName,
			Label:   capName,
			Options: boolOpts,
			Current: current,
			IsBool:  true,
		})
	}

	return fields
}

// modelsForProvider returns the model options for a given provider.
func modelsForProvider(provider string) []string {
	switch provider {
	case "minimax":
		return []string{"MiniMax-M2.7-highspeed", "MiniMax-M2.7"}
	case "gemini":
		return []string{"gemini-3.0-flash", "gemini-2.5-flash", "gemini-2.5-pro"}
	case "openai":
		return []string{"gpt-4o", "gpt-4o-mini", "o3-mini"}
	case "anthropic":
		return []string{"claude-sonnet-4-6", "claude-haiku-4-5-20251001"}
	case "custom":
		return []string{"(edit init.json)"}
	default:
		return []string{"(edit init.json)"}
	}
}

func applyEditField(p *preset.Preset, f *presetField) {
	val := f.Options[f.Current]

	switch f.Key {
	case "provider":
		if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
			llm["provider"] = val
		}
	case "model":
		if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
			llm["model"] = val
		}
	case "endpoint":
		if llm, ok := p.Manifest["llm"].(map[string]interface{}); ok {
			switch val {
			case "international":
				llm["base_url"] = "https://api.minimax.io/anthropic"
			case "china":
				llm["base_url"] = "https://api.minimaxi.com/anthropic"
			}
		}
	case "language":
		p.Manifest["language"] = val
	case "karma":
		admin, ok := p.Manifest["admin"].(map[string]interface{})
		if !ok {
			admin = map[string]interface{}{}
			p.Manifest["admin"] = admin
		}
		admin["karma"] = val == "true"
	case "nirvana":
		admin, ok := p.Manifest["admin"].(map[string]interface{})
		if !ok {
			admin = map[string]interface{}{}
			p.Manifest["admin"] = admin
		}
		admin["nirvana"] = val == "true"
	default:
		if strings.HasPrefix(f.Key, "cap:") {
			capName := f.Key[4:]
			caps, ok := p.Manifest["capabilities"].(map[string]interface{})
			if !ok {
				caps = map[string]interface{}{}
				p.Manifest["capabilities"] = caps
			}
			if val == "true" {
				caps[capName] = map[string]interface{}{}
			} else {
				delete(caps, capName)
			}
		}
	}
}

func (m PresetsModel) View() string {
	switch m.mode {
	case presetsListMode:
		return m.viewList()
	case presetsEditorMode:
		return m.viewEditor()
	case presetsNewMode:
		return m.viewNew()
	}
	return ""
}

func (m PresetsModel) viewList() string {
	var b strings.Builder

	// Title bar
	title := StyleTitle.Render(i18n.T("app.title")) + " " + StyleAccent.Render(RuneBullet) + " " + StyleTitle.Render(i18n.T("presets.title"))
	escHint := StyleAccent.Render("[esc] ") + StyleSubtle.Render(i18n.T("presets.back"))
	padding := m.width - lipgloss.Width(title) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(title + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(title + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	if len(m.presets) == 0 {
		b.WriteString(StyleSubtle.Render("  "+i18n.T("firstrun.no_presets")) + "\n")
	}

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
		name := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent).Render(p.Name)
		desc := StyleSubtle.Render("  " + p.Description)
		b.WriteString(cursor + name + desc + "\n")
	}

	// Footer
	b.WriteString("\n" + strings.Repeat("─", m.width) + "\n")
	hints := fmt.Sprintf("  [enter] %s  [e]%s  [n]%s  [d]%s       [esc] %s",
		i18n.T("presets.use"),
		i18n.T("presets.edit"),
		i18n.T("presets.new"),
		i18n.T("presets.delete"),
		i18n.T("presets.back"),
	)
	b.WriteString(StyleFaint.Render(hints) + "\n")

	return b.String()
}

func (m PresetsModel) viewEditor() string {
	var b strings.Builder

	// Title bar
	title := StyleTitle.Render(i18n.TF("presets.editor_title", m.editPreset.Name))
	escHint := StyleAccent.Render("[esc] ") + StyleSubtle.Render(i18n.T("presets.back"))
	padding := m.width - lipgloss.Width(title) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(title + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(title + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	capStarted := false
	for i, f := range m.editFields {
		// Capability section header
		if strings.HasPrefix(f.Key, "cap:") && !capStarted {
			capStarted = true
			b.WriteString("\n  " + i18n.T("presets.capabilities") + ":\n")
		}

		cursor := "  "
		if i == m.editCursor {
			cursor = "> "
		}

		var label string
		if strings.HasPrefix(f.Key, "cap:") {
			label = f.Label // capability name directly
		} else {
			label = i18n.T(f.Label)
		}

		displayVal := f.Options[f.Current]
		if f.IsBool {
			if displayVal == "true" {
				displayVal = "[x]"
			} else {
				displayVal = "[ ]"
			}
		}

		if i == m.editCursor {
			if f.IsBool {
				displayVal = lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render(displayVal)
			} else {
				displayVal = lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render("< " + displayVal + " >")
			}
		}

		if strings.HasPrefix(f.Key, "cap:") {
			b.WriteString(fmt.Sprintf("%s%s %s\n", cursor, displayVal, label))
		} else {
			b.WriteString(fmt.Sprintf("%s%-15s %s\n", cursor, label+":", displayVal))
		}
	}

	// Footer
	b.WriteString("\n" + strings.Repeat("─", m.width) + "\n")
	b.WriteString(StyleFaint.Render(fmt.Sprintf("  ↑↓ %s  ←→/space %s  [esc] %s",
		i18n.T("settings.select"), i18n.T("settings.change"), i18n.T("presets.back"))) + "\n")

	return b.String()
}

func (m PresetsModel) viewNew() string {
	var b strings.Builder

	title := StyleTitle.Render(i18n.T("app.title")) + " " + StyleAccent.Render(RuneBullet) + " " + StyleTitle.Render(i18n.T("presets.new"))
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	b.WriteString("  " + i18n.T("presets.enter_name") + "\n\n")
	b.WriteString("  " + m.nameInput.View() + "\n\n")
	b.WriteString(StyleFaint.Render("  [Enter] "+i18n.T("presets.create")+"  [Esc] "+i18n.T("presets.cancel")) + "\n")

	return b.String()
}
