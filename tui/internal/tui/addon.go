package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
)

// AddonSavedMsg is sent when addon view is dismissed.
type AddonSavedMsg struct{}

// AddonModel is the /addon view — read-only display of configured addons.
// Scans {agentDir}/addons/{addon}/{account}/config.json for each addon.
type AddonModel struct {
	orchDir string
	width   int
	height  int
	// addonConfigs maps addon name → (account → JSON string)
	addonConfigs map[string]map[string]string
	// selectedAddon is the addon whose JSON is expanded (empty = none)
	selectedAddon string
}

func NewAddonModel(orchDir string) AddonModel {
	return AddonModel{
		orchDir:      orchDir,
		addonConfigs: readAddonConfigs(orchDir),
	}
}

func (m AddonModel) Init() tea.Cmd { return nil }

func (m AddonModel) Update(msg tea.Msg) (AddonModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return AddonSavedMsg{} }
		case "enter":
			// Toggle expand/collapse on the addon at current cursor position
			return m, nil
		}
	}
	return m, nil
}

func (m AddonModel) View() string {
	var b strings.Builder

	// Title bar
	titleText := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent).Render(i18n.T("welcome.title"))
	titleBar := titleText + " " + StyleAccent.Render(RuneBullet) + " " + StyleTitle.Render(i18n.T("addon.title"))
	escHint := StyleAccent.Render("[esc] ") + StyleSubtle.Render(i18n.T("addon.back"))
	padding := m.width - lipgloss.Width(titleBar) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(titleBar + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(titleBar + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	// Description
	b.WriteString(StyleSubtle.Render("  "+i18n.T("addon.readonly_desc")) + "\n\n")

	// Addon list
	hasAny := false
	for _, name := range AllAddons {
		accounts := m.addonConfigs[name]
		hasAny = accounts != nil && len(accounts) > 0
		label := strings.ToUpper(name[:1]) + name[1:]
		b.WriteString("  " + StyleTitle.Render(label) + "\n")

		if !hasAny {
			b.WriteString("    " + StyleFaint.Render(i18n.T("addon.not_configured")) + "\n\n")
		} else {
			for account, jsonContent := range accounts {
				b.WriteString("    " + StyleAccent.Render(account) + ":\n")
				// Pretty-print the JSON
				pretty := prettyJSON(jsonContent)
				for _, line := range strings.Split(pretty, "\n") {
					b.WriteString("      " + line + "\n")
				}
				b.WriteString("\n")
			}
		}
	}

	// Footer
	b.WriteString(strings.Repeat("─", m.width) + "\n")
	b.WriteString(StyleFaint.Render("  /refresh to apply changes after config edits") + "\n")

	return b.String()
}

// readAddonConfigs scans {agentDir}/addons/{addon}/{account}/config.json for all addons.
// Returns map[addon]map[account]jsonString.
func readAddonConfigs(orchDir string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	if orchDir == "" {
		return result
	}

	for _, addon := range AllAddons {
		addonBase := filepath.Join(orchDir, "addons", addon)
		entries, err := os.ReadDir(addonBase)
		if err != nil {
			continue
		}
		accountMap := make(map[string]string)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			account := entry.Name()
			configPath := filepath.Join(addonBase, account, "config.json")
			data, err := os.ReadFile(configPath)
			if err != nil {
				continue
			}
			accountMap[account] = string(data)
		}
		if len(accountMap) > 0 {
			result[addon] = accountMap
		}
	}
	return result
}

// prettyJSON returns a formatted (indented) JSON string, or the original on error.
func prettyJSON(data string) string {
	var v any
	if err := json.Unmarshal([]byte(data), &v); err != nil {
		return data
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return data
	}
	return string(out)
}
