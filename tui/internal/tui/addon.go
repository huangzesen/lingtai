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
// Reads from {lingtaiDir}/.addons/{addon}/config.json, a project-level
// shared location (one config file per addon, multi-account via accounts array).
type AddonModel struct {
	lingtaiDir string // <project>/.lingtai/ directory
	width      int
	height     int
	// addonConfigs maps addon name → JSON file content (or "" if missing/unreadable)
	addonConfigs map[string]string
	// addonErrors maps addon name → error message (e.g. "not found", "parse error")
	addonErrors map[string]string
}

// NewAddonModel constructs the /addon view. lingtaiDir is the project's .lingtai/
// directory (parent of all agent dirs). Addon configs live at
// lingtaiDir/.addons/<addon>/config.json.
func NewAddonModel(lingtaiDir string) AddonModel {
	configs, errs := readAddonConfigs(lingtaiDir)
	return AddonModel{
		lingtaiDir:   lingtaiDir,
		addonConfigs: configs,
		addonErrors:  errs,
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
	for _, name := range AllAddons {
		label := strings.ToUpper(name[:1]) + name[1:]
		configPath := addonConfigRelPath(name)
		b.WriteString("  " + StyleTitle.Render(label) + StyleFaint.Render("  "+configPath) + "\n")

		if errMsg, bad := m.addonErrors[name]; bad {
			b.WriteString("    " + StyleFaint.Render(errMsg) + "\n\n")
			continue
		}

		content, ok := m.addonConfigs[name]
		if !ok || content == "" {
			b.WriteString("    " + StyleFaint.Render(i18n.T("addon.not_configured")) + "\n\n")
			continue
		}

		// Pretty-print the JSON
		pretty := prettyJSON(content)
		for _, line := range strings.Split(strings.TrimRight(pretty, "\n"), "\n") {
			b.WriteString("    " + line + "\n")
		}
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(strings.Repeat("─", m.width) + "\n")
	b.WriteString(StyleFaint.Render("  "+i18n.T("addon.footer_hint")) + "\n")

	return b.String()
}

// addonConfigRelPath returns the canonical path (relative to project root) for
// an addon's config file. This is the only place the convention is defined —
// all other code uses this helper.
func addonConfigRelPath(addon string) string {
	return filepath.Join(".lingtai", ".addons", addon, "config.json")
}

// AddonConfigPath returns the absolute path to an addon's config file, given
// the project's .lingtai/ directory. Exported for use by other packages.
func AddonConfigPath(lingtaiDir, addon string) string {
	return filepath.Join(lingtaiDir, ".addons", addon, "config.json")
}

// readAddonConfigs reads {lingtaiDir}/.addons/{addon}/config.json for each
// known addon. Returns (configs, errors): configs holds addon→JSON-content
// for successful reads, errors holds addon→error-message for files that
// exist but couldn't be parsed. Addons with no file at all appear in neither map.
func readAddonConfigs(lingtaiDir string) (map[string]string, map[string]string) {
	configs := make(map[string]string)
	errs := make(map[string]string)
	if lingtaiDir == "" {
		return configs, errs
	}

	for _, addon := range AllAddons {
		configPath := AddonConfigPath(lingtaiDir, addon)
		data, err := os.ReadFile(configPath)
		if err != nil {
			// File missing or unreadable — not an error, just "not configured"
			continue
		}
		// Validate it parses as JSON; if not, report as an error
		var probe any
		if jerr := json.Unmarshal(data, &probe); jerr != nil {
			errs[addon] = i18n.TF("addon.parse_error", jerr.Error())
			continue
		}
		configs[addon] = string(data)
	}
	return configs, errs
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
