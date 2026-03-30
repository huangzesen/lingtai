package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Settings holds TUI preferences persisted to .lingtai/human/settings.json.
type Settings struct {
	Language     string `json:"language"`
	PollRate     int    `json:"poll_rate"`
	Verbose      bool   `json:"verbose"`
	Orchestrator string `json:"orchestrator,omitempty"`
}

// DefaultSettings returns sensible defaults.
func DefaultSettings() Settings {
	return Settings{
		Language: "en",
		PollRate: 1,
		Verbose:  false,
	}
}

// LoadSettings reads settings from .lingtai/human/settings.json.
func LoadSettings(baseDir string) Settings {
	path := filepath.Join(baseDir, "human", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultSettings()
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettings()
	}
	// Fill defaults for zero values
	if s.Language == "" {
		s.Language = "en"
	}
	if s.PollRate == 0 {
		s.PollRate = 1
	}
	return s
}

// SaveSettings writes settings to .lingtai/human/settings.json.
func SaveSettings(baseDir string, s Settings) error {
	dir := filepath.Join(baseDir, "human")
	os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644)
}

// SettingField represents a single configurable setting.
type SettingField struct {
	Key     string
	Label   string   // i18n key
	Options []string // values to cycle through
	Current int      // index into Options
}

// SettingsModel is the /settings view.
type SettingsModel struct {
	cursor    int
	settings  Settings
	fields    []SettingField
	baseDir   string
	globalDir string
	width     int
	height    int
}

func NewSettingsModel(baseDir, globalDir string, settings Settings) SettingsModel {
	// Read language from global config
	langOptions := []string{"en", "zh", "wen"}
	langCurrent := 0
	if globalCfg, err := config.LoadConfig(globalDir); err == nil && globalCfg.Language != "" {
		for i, l := range langOptions {
			if l == globalCfg.Language {
				langCurrent = i
				break
			}
		}
	} else {
		for i, l := range langOptions {
			if l == settings.Language {
				langCurrent = i
				break
			}
		}
	}

	pollOptions := []string{"1", "2", "3", "5"}
	pollCurrent := 0
	pollStr := fmt.Sprintf("%d", settings.PollRate)
	for i, p := range pollOptions {
		if p == pollStr {
			pollCurrent = i
			break
		}
	}

	verboseOptions := []string{"off", "on"}
	verboseCurrent := 0
	if settings.Verbose {
		verboseCurrent = 1
	}

	fields := []SettingField{
		{Key: "language", Label: "settings.language", Options: langOptions, Current: langCurrent},
		{Key: "poll_rate", Label: "settings.poll_rate", Options: pollOptions, Current: pollCurrent},
		{Key: "verbose", Label: "settings.verbose", Options: verboseOptions, Current: verboseCurrent},
	}

	return SettingsModel{
		settings:  settings,
		fields:    fields,
		baseDir:   baseDir,
		globalDir: globalDir,
	}
}

func (m SettingsModel) Init() tea.Cmd { return nil }

func (m SettingsModel) Update(msg tea.Msg) (SettingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
		case "enter":
			// Open welcome page when pressing Enter on the language field
			if m.fields[m.cursor].Key == "language" {
				return m, func() tea.Msg { return ViewChangeMsg{View: "welcome"} }
			}
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.fields)-1 {
				m.cursor++
			}
		case "left":
			f := &m.fields[m.cursor]
			if f.Current > 0 {
				f.Current--
				m.applyField(f)
			}
		case "right":
			f := &m.fields[m.cursor]
			if f.Current < len(f.Options)-1 {
				f.Current++
				m.applyField(f)
			}
		}
	}
	return m, nil
}

func (m *SettingsModel) applyField(f *SettingField) {
	val := f.Options[f.Current]
	switch f.Key {
	case "language":
		m.settings.Language = val
		i18n.SetLang(val)
		// Save language to global config
		if cfg, err := config.LoadConfig(m.globalDir); err == nil {
			cfg.Language = val
			config.SaveConfig(m.globalDir, cfg)
		}
	case "poll_rate":
		rate := 1
		fmt.Sscanf(val, "%d", &rate)
		m.settings.PollRate = rate
	case "verbose":
		m.settings.Verbose = val == "on"
	}
	SaveSettings(m.baseDir, m.settings)
}

func (m SettingsModel) View() string {
	var b strings.Builder

	// Title bar: product name · settings
	titleText := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent).Render(i18n.T("welcome.title"))
	titleBar := titleText + " " + StyleAccent.Render(RuneBullet) + " " + StyleTitle.Render(i18n.T("settings.title"))
	escHint := StyleAccent.Render("[esc] ") + StyleSubtle.Render(i18n.T("settings.back"))
	padding := m.width - lipgloss.Width(titleBar) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(titleBar + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(titleBar + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n")

	// Poem decoration
	b.WriteString(StyleFaint.Render("  "+i18n.T("welcome.poem_line1")) + "\n")
	b.WriteString(StyleFaint.Render("  "+i18n.T("welcome.poem_line2")) + "\n\n")

	// Fields
	for i, f := range m.fields {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		label := i18n.T(f.Label) + ":"
		value := f.Options[f.Current]

		// Show display-friendly value
		displayVal := value
		if f.Key == "poll_rate" {
			displayVal = value + "s"
		}
		if f.Key == "verbose" {
			displayVal = i18n.T("settings." + value)
		}

		// Highlight selected
		if i == m.cursor {
			displayVal = lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render("< " + displayVal + " >")
		}

		line := fmt.Sprintf("%s%-15s %s", cursor, label, displayVal)
		b.WriteString(line + "\n")
	}

	// Footer
	b.WriteString("\n" + strings.Repeat("─", m.width) + "\n")
	hints := fmt.Sprintf("  ↑↓ %s  ←→ %s", i18n.T("settings.select"), i18n.T("settings.change"))
	if m.fields[m.cursor].Key == "language" {
		hints += "  [Enter] " + i18n.T("settings.welcome")
	}
	hints += "  [esc] " + i18n.T("settings.back")
	b.WriteString(StyleFaint.Render(hints) + "\n")

	return b.String()
}

// GetSettings returns the current settings.
func (m SettingsModel) GetSettings() Settings {
	return m.settings
}
