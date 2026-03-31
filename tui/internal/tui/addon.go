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

// AddonModel is the /addon view — two text fields for addon config paths.
type AddonModel struct {
	cursor  int
	inputs  [2]textinput.Model // 0=imap, 1=telegram
	orchDir string
	width   int
	height  int
}

func NewAddonModel(orchDir string) AddonModel {
	// Read existing addon paths from init.json
	imapPath, telegramPath := readAddonPaths(orchDir)

	imapInput := textinput.New()
	imapInput.Placeholder = ""
	imapInput.CharLimit = 256
	imapInput.SetWidth(60)
	imapInput.SetValue(imapPath)

	telegramInput := textinput.New()
	telegramInput.Placeholder = ""
	telegramInput.CharLimit = 256
	telegramInput.SetWidth(60)
	telegramInput.SetValue(telegramPath)

	// Focus the first input
	imapInput.Focus()

	return AddonModel{
		inputs:  [2]textinput.Model{imapInput, telegramInput},
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

	// Fields
	labels := [2]string{
		i18n.T("addon.imap_path"),
		i18n.T("addon.telegram_path"),
	}

	for i, input := range m.inputs {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		label := labels[i] + ":"
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, label))
		b.WriteString(fmt.Sprintf("    %s\n\n", input.View()))
	}

	// Footer
	b.WriteString(strings.Repeat("─", m.width) + "\n")
	hints := fmt.Sprintf("  ↑↓/tab %s  [esc] %s",
		i18n.T("addon.navigate"),
		i18n.T("addon.save_exit"))
	b.WriteString(StyleFaint.Render(hints) + "\n")

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

	imapVal := strings.TrimSpace(m.inputs[0].Value())
	telegramVal := strings.TrimSpace(m.inputs[1].Value())

	if imapVal == "" && telegramVal == "" {
		// Remove addons block entirely
		delete(init, "addons")
	} else {
		addons, ok := init["addons"].(map[string]interface{})
		if !ok {
			addons = make(map[string]interface{})
		}

		if imapVal != "" {
			addons["imap"] = map[string]interface{}{"config": imapVal}
		} else {
			delete(addons, "imap")
		}

		if telegramVal != "" {
			addons["telegram"] = map[string]interface{}{"config": telegramVal}
		} else {
			delete(addons, "telegram")
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
func readAddonPaths(orchDir string) (imapPath, telegramPath string) {
	initPath := filepath.Join(orchDir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return "", ""
	}
	var init map[string]interface{}
	if err := json.Unmarshal(data, &init); err != nil {
		return "", ""
	}

	addons, ok := init["addons"].(map[string]interface{})
	if !ok {
		return "", ""
	}

	if imap, ok := addons["imap"].(map[string]interface{}); ok {
		if cfg, ok := imap["config"].(string); ok {
			imapPath = cfg
		}
	}
	if telegram, ok := addons["telegram"].(map[string]interface{}); ok {
		if cfg, ok := telegram["config"].(string); ok {
			telegramPath = cfg
		}
	}
	return
}
