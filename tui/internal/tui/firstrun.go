package tui

import (
	"path/filepath"
	"strings"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/preset"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FirstRunDoneMsg is emitted when first-run flow completes.
type FirstRunDoneMsg struct {
	OrchDir  string // full path to orchestrator directory
	OrchName string // agent name
}

type firstRunStep int

const (
	stepCheckPresets firstRunStep = iota
	stepAPIKey
	stepPickPreset
	stepEditPreset
	stepNewPreset
	stepNameAgent
	stepLaunching
)

// FirstRunModel orchestrates the first-run experience.
type FirstRunModel struct {
	step      firstRunStep
	setup     SetupModel
	presets   []preset.Preset
	cursor    int
	nameInput textinput.Model
	message   string
	baseDir   string // .lingtai/ directory
	globalDir string
	width     int
	height    int
	// Embedded preset editor
	editPreset preset.Preset
	editFields []presetField
	editCursor int
}

func NewFirstRunModel(baseDir, globalDir string, hasPresets bool) FirstRunModel {
	ti := textinput.New()
	ti.CharLimit = 64
	ti.Width = 40

	m := FirstRunModel{
		baseDir:   baseDir,
		globalDir: globalDir,
		nameInput: ti,
	}

	if !hasPresets {
		m.step = stepAPIKey
		m.setup = NewSetupModel(globalDir)
	} else {
		m.step = stepPickPreset
		m.presets, _ = preset.List()
	}

	return m
}

func (m FirstRunModel) Init() tea.Cmd {
	switch m.step {
	case stepAPIKey:
		return m.setup.Init()
	case stepPickPreset:
		return nil
	}
	return nil
}

func (m FirstRunModel) Update(msg tea.Msg) (FirstRunModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case SetupDoneMsg:
		// API key saved -> create default preset -> move to preset picker
		preset.EnsureDefault()
		m.presets, _ = preset.List()
		m.step = stepPickPreset
		return m, nil

	case tea.KeyMsg:
		switch m.step {
		case stepAPIKey:
			var cmd tea.Cmd
			m.setup, cmd = m.setup.Update(msg)
			return m, cmd

		case stepPickPreset:
			switch msg.String() {
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
					m.step = stepNameAgent
					defaultName := m.presets[m.cursor].Name
					m.nameInput.SetValue(defaultName)
					m.nameInput.Focus()
					return m, textinput.Blink
				}
			case "e":
				if m.cursor < len(m.presets) {
					m.editPreset = m.presets[m.cursor]
					m.editFields = buildEditFields(m.editPreset)
					m.editCursor = 0
					m.step = stepEditPreset
				}
			case "n":
				m.nameInput.SetValue("")
				m.nameInput.Focus()
				m.step = stepNewPreset
				return m, textinput.Blink
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil

		case stepEditPreset:
			switch msg.String() {
			case "esc":
				preset.Save(m.editPreset)
				m.presets, _ = preset.List()
				m.step = stepPickPreset
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
			case "right", " ":
				f := &m.editFields[m.editCursor]
				if f.Current < len(f.Options)-1 {
					f.Current++
					applyEditField(&m.editPreset, f)
				}
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil

		case stepNewPreset:
			switch msg.String() {
			case "esc":
				m.step = stepPickPreset
			case "enter":
				name := m.nameInput.Value()
				if name == "" {
					return m, nil
				}
				p := preset.Preset{
					Name: name,
					Manifest: map[string]interface{}{
						"llm": map[string]interface{}{
							"provider":    "minimax",
							"model":       "MiniMax-M2.7-highspeed",
							"api_key":     nil,
							"api_key_env": "MINIMAX_API_KEY",
						},
						"capabilities": map[string]interface{}{"file": map[string]interface{}{}},
						"admin":        map[string]interface{}{"karma": true},
					},
				}
				preset.Save(p)
				m.presets, _ = preset.List()
				m.editPreset = p
				m.editFields = buildEditFields(p)
				m.editCursor = 0
				m.step = stepEditPreset
			case "ctrl+c":
				return m, tea.Quit
			default:
				var cmd tea.Cmd
				m.nameInput, cmd = m.nameInput.Update(msg)
				return m, cmd
			}
			return m, nil

		case stepNameAgent:
			switch msg.String() {
			case "enter":
				name := m.nameInput.Value()
				if name == "" {
					name = m.presets[m.cursor].Name
				}
				// Generate init.json and launch
				p := m.presets[m.cursor]
				if err := preset.GenerateInitJSON(p, name, m.baseDir); err != nil {
					m.message = i18n.TF("firstrun.error", err)
					return m, nil
				}
				m.step = stepLaunching
				m.message = i18n.TF("firstrun.created", name)

				orchDir := filepath.Join(m.baseDir, name)
				orchName := name
				return m, func() tea.Msg {
					return FirstRunDoneMsg{OrchDir: orchDir, OrchName: orchName}
				}
			case "esc":
				m.step = stepPickPreset
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			default:
				var cmd tea.Cmd
				m.nameInput, cmd = m.nameInput.Update(msg)
				return m, cmd
			}
		}
	}
	return m, nil
}

func (m FirstRunModel) View() string {
	var b strings.Builder

	// Title
	title := StyleTitle.Render("  " + i18n.T("firstrun.welcome"))
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	switch m.step {
	case stepAPIKey:
		b.WriteString("  " + i18n.T("firstrun.no_presets") + "\n\n")
		b.WriteString(m.setup.View())

	case stepPickPreset:
		b.WriteString("  " + i18n.T("firstrun.pick_preset") + "\n\n")
		for i, p := range m.presets {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}
			name := lipgloss.NewStyle().Bold(true).Render(p.Name)
			desc := StyleSubtle.Render("  " + p.Description)
			b.WriteString(cursor + name + desc + "\n")
		}
		b.WriteString("\n" + StyleSubtle.Render("  "+i18n.T("firstrun.select_hint")+
			"  [e] "+i18n.T("presets.edit")+
			"  [n] "+i18n.T("presets.new")) + "\n")

	case stepEditPreset:
		b.WriteString("  " + i18n.TF("presets.editor_title", m.editPreset.Name) + "\n\n")
		capStarted := false
		for idx, f := range m.editFields {
			if strings.HasPrefix(f.Key, "cap:") && !capStarted {
				capStarted = true
				b.WriteString("\n  " + i18n.T("presets.capabilities") + ":\n")
			}
			cursor := "  "
			if idx == m.editCursor {
				cursor = "> "
			}
			var label string
			if strings.HasPrefix(f.Key, "cap:") {
				label = f.Label
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
			if idx == m.editCursor {
				if f.IsBool {
					displayVal = lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render(displayVal)
				} else {
					displayVal = lipgloss.NewStyle().Bold(true).Foreground(ColorActive).Render("< " + displayVal + " >")
				}
			}
			if strings.HasPrefix(f.Key, "cap:") {
				b.WriteString(cursor + displayVal + " " + label + "\n")
			} else {
				b.WriteString(cursor + label + ": " + displayVal + "\n")
			}
		}
		b.WriteString("\n" + StyleSubtle.Render("  ↑↓ "+i18n.T("settings.select")+
			"  ←→/space "+i18n.T("settings.change")+
			"  [esc] "+i18n.T("presets.back")) + "\n")

	case stepNewPreset:
		b.WriteString("  " + i18n.T("presets.enter_name") + "\n\n")
		b.WriteString("  " + m.nameInput.View() + "\n\n")
		b.WriteString(StyleSubtle.Render("  [Enter] "+i18n.T("presets.create")+
			"    [Esc] "+i18n.T("presets.cancel")) + "\n")

	case stepNameAgent:
		selectedPreset := m.presets[m.cursor].Name
		b.WriteString("  " + i18n.TF("firstrun.enter_name", selectedPreset) + "\n\n")
		b.WriteString("  " + m.nameInput.View() + "\n\n")
		b.WriteString(StyleSubtle.Render("  "+i18n.T("firstrun.create_hint")) + "\n")

	case stepLaunching:
		b.WriteString("  " + i18n.T("firstrun.launching") + "\n\n")
		if m.message != "" {
			b.WriteString("  " + m.message + "\n")
		}
	}

	return b.String()
}
