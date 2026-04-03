package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
)

// AddonSavedMsg is sent when addon config is saved to init.json.
type AddonSavedMsg struct{}

// AddonModel is the /addon view — one text field per registered addon.
// The field count is derived from AllAddons at construction time.
type AddonModel struct {
	cursor  int
	inputs  []textinput.Model
	orchDir string
	width   int
	height  int
}

func NewAddonModel(orchDir string) AddonModel {
	existing := readAddonPaths(orchDir)

	inputs := make([]textinput.Model, len(AllAddons))
	for i, name := range AllAddons {
		ti := textinput.New()
		ti.Placeholder = ""
		ti.CharLimit = 256
		ti.SetWidth(60)
		if path, ok := existing[name]; ok {
			ti.SetValue(path)
		}
		inputs[i] = ti
	}

	inputs[0].Focus()

	return AddonModel{
		inputs:  inputs,
		orchDir: orchDir,
	}
}

func (m AddonModel) Init() tea.Cmd { return textinput.Blink }

func (m AddonModel) Update(msg tea.Msg) (AddonModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			m.saveAddonPaths()
			return m, func() tea.Msg { return AddonSavedMsg{} }
		case "up":
			if m.cursor > 0 {
				m.cursor--
				m.updateFocus()
			}
			return m, nil
		case "down", "tab":
			if m.cursor < len(m.inputs)-1 {
				m.cursor++
				m.updateFocus()
			}
			return m, nil
		}
	}

	// Forward to focused input
	var cmd tea.Cmd
	m.inputs[m.cursor], cmd = m.inputs[m.cursor].Update(msg)
	return m, cmd
}

func (m *AddonModel) updateFocus() {
	for i := range m.inputs {
		if i == m.cursor {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

// addonFieldKey returns the i18n key for the addon path field (e.g. "addon.imap_path").
func addonFieldKey(name, suffix string) string {
	return "addon." + name + "_" + suffix
}

func (m AddonModel) View() string {
	var b strings.Builder

	// Title bar
	titleText := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent).Render(i18n.T("welcome.title"))
	titleBar := titleText + " " + StyleAccent.Render(RuneBullet) + " " + StyleTitle.Render(i18n.T("addon.title"))
	escHint := StyleAccent.Render("[esc] ") + StyleSubtle.Render(i18n.T("addon.save_exit"))
	padding := m.width - lipgloss.Width(titleBar) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(titleBar + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(titleBar + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	// Description
	b.WriteString(StyleSubtle.Render("  "+i18n.T("addon.desc")) + "\n")
	b.WriteString(StyleSubtle.Render("  "+i18n.T("addon.desc_template")) + "\n")
	b.WriteString(StyleSubtle.Render("  "+i18n.T("addon.desc_empty")) + "\n\n")

	// Fields — one per addon
	for i, name := range AllAddons {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		label := i18n.T(addonFieldKey(name, "path"))
		if label == addonFieldKey(name, "path") {
			// Fallback: capitalize the addon name
			label = strings.ToUpper(name[:1]) + name[1:] + " config"
		}
		b.WriteString(fmt.Sprintf("%s%s:\n", cursor, label))
		b.WriteString(fmt.Sprintf("    %s\n", m.inputs[i].View()))
		hint := i18n.T(addonFieldKey(name, "hint"))
		if hint == addonFieldKey(name, "hint") {
			hint = "~/.lingtai-tui/addons/" + name + "/example/config.json"
		}
		b.WriteString(StyleFaint.Render("    "+hint) + "\n\n")
	}

	// Footer
	b.WriteString(strings.Repeat("─", m.width) + "\n")
	footerHints := fmt.Sprintf("  ↑↓/tab %s  [esc] %s",
		i18n.T("addon.navigate"),
		i18n.T("addon.save_exit"))
	b.WriteString(StyleFaint.Render(footerHints) + "\n")

	return b.String()
}

// saveAddonPaths writes addon config paths to init.json.
// Empty values remove the addon from init.json.
func (m AddonModel) saveAddonPaths() {
	initPath := filepath.Join(m.orchDir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return
	}
	var init map[string]interface{}
	if err := json.Unmarshal(data, &init); err != nil {
		return
	}

	// Collect non-empty paths
	nonEmpty := false
	for i := range AllAddons {
		if strings.TrimSpace(m.inputs[i].Value()) != "" {
			nonEmpty = true
			break
		}
	}

	if !nonEmpty {
		delete(init, "addons")
	} else {
		addons, ok := init["addons"].(map[string]interface{})
		if !ok {
			addons = make(map[string]interface{})
		}

		for i, name := range AllAddons {
			path := strings.TrimSpace(m.inputs[i].Value())
			if path != "" {
				addons[name] = map[string]interface{}{"config": path}
			} else {
				delete(addons, name)
			}
		}

		if len(addons) > 0 {
			init["addons"] = addons
		} else {
			delete(init, "addons")
		}
	}

	out, err := json.MarshalIndent(init, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(initPath, append(out, '\n'), 0o644)
}

// readAddonPaths reads current addon config paths from init.json.
// Returns a map from addon name → config path.
func readAddonPaths(orchDir string) map[string]string {
	result := make(map[string]string)
	initPath := filepath.Join(orchDir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return result
	}
	var init map[string]interface{}
	if err := json.Unmarshal(data, &init); err != nil {
		return result
	}

	addons, ok := init["addons"].(map[string]interface{})
	if !ok {
		return result
	}

	for _, name := range AllAddons {
		if addon, ok := addons[name].(map[string]interface{}); ok {
			if cfg, ok := addon["config"].(string); ok {
				result[name] = cfg
			}
		}
	}
	return result
}
