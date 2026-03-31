package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
)

// SetupDoneMsg is emitted when API key setup completes.
type SetupDoneMsg struct{}

// Provider selection state
const (
	stepSelectProvider = iota
	stepEnterKey
)

// Supported providers — display names resolved via i18n at render time.
var providers = []struct {
	id      string
	nameKey string // i18n key for display name
}{
	{"minimax", "setup.provider_minimax"},
	{"custom", "setup.provider_custom"},
}

// SetupModel handles API key configuration (provider-agnostic).
type SetupModel struct {
	step         int // current step
	providerIdx  int // selected provider index
	input        textinput.Model
	globalDir    string
	done         bool
	err          error
	existingKeys map[string]string // existing keys for display
	width        int
	height       int
}

func NewSetupModel(globalDir string) SetupModel {
	ti := textinput.New()
	ti.Placeholder = i18n.T("setup.prompt_api_key")
	ti.Focus()
	ti.CharLimit = 128
	ti.SetWidth(50)

	// Load existing keys for display
	existingKeys := make(map[string]string)
	cfg, err := config.LoadConfig(globalDir)
	if err == nil && cfg.Keys != nil {
		existingKeys = cfg.Keys
	}

	return SetupModel{
		input:        ti,
		globalDir:    globalDir,
		step:         stepSelectProvider,
		providerIdx:  0,
		existingKeys: existingKeys,
	}
}

func (m SetupModel) Init() tea.Cmd { return textinput.Blink }

func maskKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) > 8 {
		return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
	}
	return strings.Repeat("*", len(key))
}

func (m SetupModel) Update(msg tea.Msg) (SetupModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			return m.handleEnter()
		case "up", "left":
			if m.step == stepSelectProvider {
				m.providerIdx = (m.providerIdx - 1 + len(providers)) % len(providers)
			}
			return m, nil
		case "down", "right":
			if m.step == stepSelectProvider {
				m.providerIdx = (m.providerIdx + 1) % len(providers)
			}
			return m, nil
		case "tab":
			if m.step == stepSelectProvider {
				m.providerIdx = (m.providerIdx + 1) % len(providers)
			}
			return m, nil
		case "esc":
			if m.step == stepEnterKey {
				m.step = stepSelectProvider
				m.input.Reset()
				return m, nil
			}
			return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m SetupModel) handleEnter() (SetupModel, tea.Cmd) {
	provider := providers[m.providerIdx].id

	if m.step == stepSelectProvider {
		// Always go to key input step — user can change or keep existing key
		m.step = stepEnterKey
		m.input.Reset()
		m.input.Placeholder = i18n.TF("firstrun.enter_provider_key", i18n.T(providers[m.providerIdx].nameKey))
		return m, textinput.Blink
	}

	// stepEnterKey
	key := m.input.Value()
	if key == "" && m.existingKeys[provider] == "" {
		return m, nil // require key
	}
	if key == "" {
		// Keep existing key, save and done
		cfg, _ := config.LoadConfig(m.globalDir)
		cfg.Keys = m.existingKeys
		if err := config.SaveConfig(m.globalDir, cfg); err != nil {
			m.err = err
			return m, nil
		}
		m.done = true
		return m, func() tea.Msg { return SetupDoneMsg{} }
	}

	// Save new key
	m.existingKeys[provider] = key
	cfg, _ := config.LoadConfig(m.globalDir)
	cfg.Keys = m.existingKeys
	if err := config.SaveConfig(m.globalDir, cfg); err != nil {
		m.err = err
		return m, nil
	}
	m.done = true
	return m, func() tea.Msg { return SetupDoneMsg{} }
}

func (m SetupModel) View() string {
	var b strings.Builder

	// Title bar
	title := StyleTitle.Render(i18n.T("app.title")) + " " + StyleAccent.Render(RuneBullet) + " " + StyleTitle.Render(i18n.T("setup.title"))
	escHint := StyleAccent.Render("[esc] ") + StyleSubtle.Render(i18n.T("setup.back"))
	padding := m.width - lipgloss.Width(title) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(title + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(title + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	if m.step == stepSelectProvider {
		m.viewProviderSelection(&b)
	} else {
		m.viewKeyInput(&b)
	}

	return b.String()
}

func (m SetupModel) viewProviderSelection(b *strings.Builder) {
	b.WriteString("  " + i18n.T("setup.select_provider") + "\n\n")

	// Show provider options
	for i, p := range providers {
		prefix := "  ○ "
		selected := i == m.providerIdx
		if selected {
			prefix = "  ● "
		}
		name := i18n.T(p.nameKey)
		masked := maskKey(m.existingKeys[p.id])
		if masked != "" {
			b.WriteString(fmt.Sprintf("%s%s: %s\n", prefix, name, masked))
		} else {
			b.WriteString(fmt.Sprintf("%s%s\n", prefix, name))
		}
	}

	b.WriteString("\n")
	b.WriteString(StyleFaint.Render("  [↑↓] "+i18n.T("setup.navigate")+"  [Enter] "+i18n.T("setup.confirm")) + "\n")

	// Error
	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
		b.WriteString("  " + errStyle.Render(i18n.TF("setup.error", m.err.Error())) + "\n")
	}
}

func (m SetupModel) viewKeyInput(b *strings.Builder) {
	providerName := i18n.T(providers[m.providerIdx].nameKey)
	b.WriteString(fmt.Sprintf("  %s: %s\n\n", i18n.T("setup.provider"), providerName))

	// Show existing key if any
	existingKey := maskKey(m.existingKeys[providers[m.providerIdx].id])
	if existingKey != "" {
		b.WriteString(fmt.Sprintf("  %s: %s\n", i18n.T("setup.current_key"), existingKey))
		b.WriteString("  " + i18n.T("setup.enter_new_or_empty") + "\n\n")
	}

	// Input
	b.WriteString("  " + i18n.T("setup.api_key_label") + " " + m.input.View() + "\n\n")

	// Error
	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
		b.WriteString("  " + errStyle.Render(i18n.TF("setup.error", m.err.Error())) + "\n")
	}

	// Hints
	b.WriteString(StyleSubtle.Render("  [Enter] "+i18n.T("setup.save")+"    [Esc] "+i18n.T("setup.back")) + "\n")
}

func (m SetupModel) Done() bool { return m.done }
