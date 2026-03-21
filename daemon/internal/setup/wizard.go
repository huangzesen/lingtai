package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Steps in the wizard.
type step int

const (
	StepModel step = iota
	StepIMAP
	StepTelegram
	StepGeneral
	StepReview
)

func (s step) String() string {
	switch s {
	case StepModel:
		return "Model Configuration"
	case StepIMAP:
		return "IMAP / SMTP (optional — Esc to skip)"
	case StepTelegram:
		return "Telegram (optional — Esc to skip)"
	case StepGeneral:
		return "General Settings"
	case StepReview:
		return "Review & Save"
	default:
		return "Unknown"
	}
}

// Styles
var (
	headerStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")) // cyan
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
	dimStyle     = lipgloss.NewStyle().Faint(true)
	promptStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15")) // white
)

// providers is the list of supported LLM providers.
var providers = []string{"minimax", "openai", "anthropic", "gemini", "custom"}

// field is a labeled text input.
type field struct {
	label string
	input textinput.Model
}

// testResultMsg carries the outcome of an async connection test.
type testResultMsg struct {
	step   step
	result TestResult
}

// wizardModel is the Bubble Tea model for the setup wizard.
type wizardModel struct {
	step      step
	fields    map[step][]field
	focus     int // index of focused field within current step
	outputDir string

	// provider selector state (step 0, field 0)
	providerIdx int

	// test results per step
	testResults map[step]*TestResult

	// final status
	done    bool
	err     error
	written []string // files written
}

func newTextInput(placeholder string, defaultVal string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(defaultVal)
	ti.CharLimit = 256
	ti.Width = 50
	return ti
}

func newWizardModel(outputDir string) wizardModel {
	m := wizardModel{
		step:        StepModel,
		outputDir:   outputDir,
		providerIdx: 0,
		testResults: make(map[step]*TestResult),
		fields:      make(map[step][]field),
	}

	// Step: Model
	m.fields[StepModel] = []field{
		{label: "Provider", input: newTextInput("minimax", "minimax")},
		{label: "Model name", input: newTextInput("model name", "")},
		{label: "API key env var", input: newTextInput("e.g. MINIMAX_API_KEY", "")},
		{label: "Base URL (custom only)", input: newTextInput("https://...", "")},
	}

	// Step: IMAP
	m.fields[StepIMAP] = []field{
		{label: "Email address", input: newTextInput("you@example.com", "")},
		{label: "Password env var", input: newTextInput("e.g. EMAIL_PASS", "")},
		{label: "IMAP host", input: newTextInput("imap.example.com", "")},
		{label: "IMAP port", input: newTextInput("993", "993")},
		{label: "SMTP host", input: newTextInput("smtp.example.com", "")},
		{label: "SMTP port", input: newTextInput("587", "587")},
	}

	// Step: Telegram
	m.fields[StepTelegram] = []field{
		{label: "Bot token env var", input: newTextInput("e.g. TELEGRAM_BOT_TOKEN", "")},
	}

	// Step: General
	home, _ := os.UserHomeDir()
	defaultBase := filepath.Join(home, ".lingtai")
	m.fields[StepGeneral] = []field{
		{label: "Agent name", input: newTextInput("orchestrator", "orchestrator")},
		{label: "Base directory", input: newTextInput(defaultBase, defaultBase)},
		{label: "Agent port", input: newTextInput("8501", "8501")},
		{label: "Bash policy file", input: newTextInput("(optional)", "")},
		{label: "Covenant", input: newTextInput("(optional)", "")},
	}

	// Step: Review has no fields

	// Focus the first field
	if len(m.fields[StepModel]) > 0 {
		m.fields[StepModel][0].input.Focus()
	}

	return m
}

func (m wizardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case testResultMsg:
		r := msg.result
		m.testResults[msg.step] = &r
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			// Skip optional steps
			if m.step == StepIMAP || m.step == StepTelegram {
				m.advanceStep()
				return m, nil
			}

		case "tab", "down":
			if m.step == StepReview {
				break
			}
			fields := m.fields[m.step]
			if m.focus < len(fields)-1 {
				fields[m.focus].input.Blur()
				m.focus++
				fields[m.focus].input.Focus()
				m.fields[m.step] = fields
			}
			return m, nil

		case "shift+tab", "up":
			if m.step == StepReview {
				break
			}
			fields := m.fields[m.step]
			if m.focus > 0 {
				fields[m.focus].input.Blur()
				m.focus--
				fields[m.focus].input.Focus()
				m.fields[m.step] = fields
			}
			return m, nil

		case "left":
			// Provider cycling (only on model step, field 0)
			if m.step == StepModel && m.focus == 0 {
				m.providerIdx = (m.providerIdx - 1 + len(providers)) % len(providers)
				m.fields[StepModel][0].input.SetValue(providers[m.providerIdx])
				return m, nil
			}

		case "right":
			if m.step == StepModel && m.focus == 0 {
				m.providerIdx = (m.providerIdx + 1) % len(providers)
				m.fields[StepModel][0].input.SetValue(providers[m.providerIdx])
				return m, nil
			}

		case "ctrl+t":
			// Run connection test
			return m, m.runTest()

		case "enter":
			if m.step == StepReview {
				m.written, m.err = m.writeConfig()
				m.done = true
				return m, tea.Quit
			}
			// On last field of current step, advance
			fields := m.fields[m.step]
			if fields == nil || m.focus >= len(fields)-1 {
				m.advanceStep()
				return m, nil
			}
			// Otherwise move to next field
			fields[m.focus].input.Blur()
			m.focus++
			fields[m.focus].input.Focus()
			m.fields[m.step] = fields
			return m, nil
		}
	}

	// Update the focused text input
	if m.step != StepReview {
		fields := m.fields[m.step]
		if m.focus < len(fields) {
			var cmd tea.Cmd
			fields[m.focus].input, cmd = fields[m.focus].input.Update(msg)
			m.fields[m.step] = fields
			return m, cmd
		}
	}

	return m, nil
}

func (m *wizardModel) advanceStep() {
	// Blur current fields
	if fields, ok := m.fields[m.step]; ok {
		for i := range fields {
			fields[i].input.Blur()
		}
		m.fields[m.step] = fields
	}

	m.step++
	m.focus = 0

	// Focus first field of new step
	if fields, ok := m.fields[m.step]; ok && len(fields) > 0 {
		fields[0].input.Focus()
		m.fields[m.step] = fields
	}
}

func (m wizardModel) View() string {
	if m.done {
		if m.err != nil {
			return errorStyle.Render(fmt.Sprintf("Error: %v\n", m.err))
		}
		var b strings.Builder
		b.WriteString(successStyle.Render("Configuration saved successfully!") + "\n\n")
		b.WriteString("Files written:\n")
		for _, f := range m.written {
			b.WriteString(fmt.Sprintf("  %s %s\n", successStyle.Render("\u2713"), f))
		}
		return b.String()
	}

	var b strings.Builder

	// Progress bar
	steps := []string{"Model", "IMAP", "Telegram", "General", "Review"}
	for i, name := range steps {
		if step(i) == m.step {
			b.WriteString(promptStyle.Render(fmt.Sprintf("[%s]", name)))
		} else if step(i) < m.step {
			b.WriteString(successStyle.Render(fmt.Sprintf(" %s ", name)))
		} else {
			b.WriteString(dimStyle.Render(fmt.Sprintf(" %s ", name)))
		}
		if i < len(steps)-1 {
			b.WriteString(dimStyle.Render(" > "))
		}
	}
	b.WriteString("\n\n")

	// Section header
	b.WriteString(headerStyle.Render(m.step.String()) + "\n\n")

	if m.step == StepReview {
		b.WriteString(m.renderReview())
		b.WriteString("\n" + dimStyle.Render("Press Enter to save, Ctrl+C to abort") + "\n")
		return b.String()
	}

	// Render fields
	fields := m.fields[m.step]
	for i, f := range fields {
		// Skip base_url field unless provider is custom
		if m.step == StepModel && i == 3 {
			provider := m.fields[StepModel][0].input.Value()
			if provider != "custom" {
				continue
			}
		}

		cursor := "  "
		if i == m.focus {
			cursor = promptStyle.Render("> ")
		}

		label := f.label
		if m.step == StepModel && i == 0 {
			label = fmt.Sprintf("%s (left/right to cycle)", label)
		}

		b.WriteString(fmt.Sprintf("%s%s\n", cursor, promptStyle.Render(label)))
		b.WriteString(fmt.Sprintf("  %s\n", f.input.View()))
	}

	// Show test result if any
	if tr, ok := m.testResults[m.step]; ok {
		b.WriteString("\n")
		if tr.OK {
			b.WriteString(fmt.Sprintf("  %s %s\n", successStyle.Render("\u2713"), tr.Message))
		} else {
			b.WriteString(fmt.Sprintf("  %s %s\n", errorStyle.Render("\u2717"), tr.Message))
		}
	}

	// Hints
	b.WriteString("\n")
	hints := []string{"Tab/Down: next field", "Shift+Tab/Up: prev field", "Enter: next step"}
	if m.step == StepIMAP || m.step == StepTelegram {
		hints = append(hints, "Esc: skip")
		hints = append(hints, "Ctrl+T: test connection")
	}
	b.WriteString(dimStyle.Render(strings.Join(hints, " | ")) + "\n")

	return b.String()
}

func (m wizardModel) renderReview() string {
	var b strings.Builder

	// Model
	b.WriteString(promptStyle.Render("Model:") + "\n")
	b.WriteString(fmt.Sprintf("  Provider:    %s\n", m.fieldVal(StepModel, 0)))
	b.WriteString(fmt.Sprintf("  Model:       %s\n", m.fieldVal(StepModel, 1)))
	b.WriteString(fmt.Sprintf("  API Key Env: %s\n", m.fieldVal(StepModel, 2)))
	if m.fieldVal(StepModel, 0) == "custom" {
		b.WriteString(fmt.Sprintf("  Base URL:    %s\n", m.fieldVal(StepModel, 3)))
	}

	// IMAP
	if m.fieldVal(StepIMAP, 0) != "" {
		b.WriteString("\n" + promptStyle.Render("IMAP/SMTP:") + "\n")
		b.WriteString(fmt.Sprintf("  Email:     %s\n", m.fieldVal(StepIMAP, 0)))
		b.WriteString(fmt.Sprintf("  Pass Env:  %s\n", m.fieldVal(StepIMAP, 1)))
		b.WriteString(fmt.Sprintf("  IMAP:      %s:%s\n", m.fieldVal(StepIMAP, 2), m.fieldVal(StepIMAP, 3)))
		b.WriteString(fmt.Sprintf("  SMTP:      %s:%s\n", m.fieldVal(StepIMAP, 4), m.fieldVal(StepIMAP, 5)))
		m.renderTestResult(&b, StepIMAP)
	} else {
		b.WriteString("\n" + dimStyle.Render("IMAP/SMTP: skipped") + "\n")
	}

	// Telegram
	if m.fieldVal(StepTelegram, 0) != "" {
		b.WriteString("\n" + promptStyle.Render("Telegram:") + "\n")
		b.WriteString(fmt.Sprintf("  Token Env: %s\n", m.fieldVal(StepTelegram, 0)))
		m.renderTestResult(&b, StepTelegram)
	} else {
		b.WriteString("\n" + dimStyle.Render("Telegram: skipped") + "\n")
	}

	// General
	b.WriteString("\n" + promptStyle.Render("General:") + "\n")
	b.WriteString(fmt.Sprintf("  Agent Name: %s\n", m.fieldVal(StepGeneral, 0)))
	b.WriteString(fmt.Sprintf("  Base Dir:   %s\n", m.fieldVal(StepGeneral, 1)))
	b.WriteString(fmt.Sprintf("  Port:       %s\n", m.fieldVal(StepGeneral, 2)))
	if v := m.fieldVal(StepGeneral, 3); v != "" {
		b.WriteString(fmt.Sprintf("  Bash Policy: %s\n", v))
	}
	if v := m.fieldVal(StepGeneral, 4); v != "" {
		b.WriteString(fmt.Sprintf("  Covenant:    %s\n", v))
	}

	return b.String()
}

func (m wizardModel) renderTestResult(b *strings.Builder, s step) {
	if tr, ok := m.testResults[s]; ok {
		if tr.OK {
			b.WriteString(fmt.Sprintf("  %s %s\n", successStyle.Render("\u2713"), tr.Message))
		} else {
			b.WriteString(fmt.Sprintf("  %s %s\n", errorStyle.Render("\u2717"), tr.Message))
		}
	}
}

func (m wizardModel) fieldVal(s step, idx int) string {
	fields, ok := m.fields[s]
	if !ok || idx >= len(fields) {
		return ""
	}
	return fields[idx].input.Value()
}

func (m wizardModel) runTest() tea.Cmd {
	switch m.step {
	case StepIMAP:
		return func() tea.Msg {
			email := m.fieldVal(StepIMAP, 0)
			passEnv := m.fieldVal(StepIMAP, 1)
			imapHost := m.fieldVal(StepIMAP, 2)
			imapPortStr := m.fieldVal(StepIMAP, 3)

			pass := os.Getenv(passEnv)
			if pass == "" {
				return testResultMsg{step: StepIMAP, result: TestResult{OK: false, Message: fmt.Sprintf("env var %s is not set", passEnv)}}
			}

			imapPort, _ := strconv.Atoi(imapPortStr)
			if imapPort == 0 {
				imapPort = 993
			}

			r := TestIMAP(imapHost, imapPort, email, pass)
			return testResultMsg{step: StepIMAP, result: r}
		}

	case StepTelegram:
		return func() tea.Msg {
			tokenEnv := m.fieldVal(StepTelegram, 0)
			token := os.Getenv(tokenEnv)
			if token == "" {
				return testResultMsg{step: StepTelegram, result: TestResult{OK: false, Message: fmt.Sprintf("env var %s is not set", tokenEnv)}}
			}
			r := TestTelegram(token)
			return testResultMsg{step: StepTelegram, result: r}
		}

	default:
		return nil
	}
}

func (m wizardModel) writeConfig() ([]string, error) {
	if err := os.MkdirAll(m.outputDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create output directory: %w", err)
	}

	var written []string

	// 1. model.json
	modelCfg := map[string]interface{}{
		"provider":    m.fieldVal(StepModel, 0),
		"model":       m.fieldVal(StepModel, 1),
		"api_key_env": m.fieldVal(StepModel, 2),
	}
	if m.fieldVal(StepModel, 0) == "custom" && m.fieldVal(StepModel, 3) != "" {
		modelCfg["base_url"] = m.fieldVal(StepModel, 3)
	}
	modelPath := filepath.Join(m.outputDir, "model.json")
	if err := writeJSON(modelPath, modelCfg); err != nil {
		return written, fmt.Errorf("writing model.json: %w", err)
	}
	written = append(written, modelPath)

	// 2. config.json
	port, _ := strconv.Atoi(m.fieldVal(StepGeneral, 2))
	if port == 0 {
		port = 8501
	}

	cfg := map[string]interface{}{
		"model":      "model.json",
		"agent_name": m.fieldVal(StepGeneral, 0),
		"base_dir":   m.fieldVal(StepGeneral, 1),
		"agent_port": port,
	}

	if v := m.fieldVal(StepGeneral, 3); v != "" {
		cfg["bash_policy"] = v
	}
	if v := m.fieldVal(StepGeneral, 4); v != "" {
		cfg["covenant"] = v
	}

	// IMAP config
	if email := m.fieldVal(StepIMAP, 0); email != "" {
		imapPort, _ := strconv.Atoi(m.fieldVal(StepIMAP, 3))
		smtpPort, _ := strconv.Atoi(m.fieldVal(StepIMAP, 5))
		cfg["imap"] = map[string]interface{}{
			"email_address": email,
			"password_env":  m.fieldVal(StepIMAP, 1),
			"imap_host":     m.fieldVal(StepIMAP, 2),
			"imap_port":     imapPort,
			"smtp_host":     m.fieldVal(StepIMAP, 4),
			"smtp_port":     smtpPort,
		}
	}

	// Telegram config
	if tokenEnv := m.fieldVal(StepTelegram, 0); tokenEnv != "" {
		cfg["telegram"] = map[string]interface{}{
			"bot_token_env": tokenEnv,
		}
	}

	configPath := filepath.Join(m.outputDir, "config.json")
	if err := writeJSON(configPath, cfg); err != nil {
		return written, fmt.Errorf("writing config.json: %w", err)
	}
	written = append(written, configPath)

	// 3. .env (collect all env var references)
	var envLines []string
	if v := m.fieldVal(StepModel, 2); v != "" {
		envLines = append(envLines, fmt.Sprintf("# %s=your-api-key-here", v))
	}
	if v := m.fieldVal(StepIMAP, 1); v != "" {
		envLines = append(envLines, fmt.Sprintf("# %s=your-password-here", v))
	}
	if v := m.fieldVal(StepTelegram, 0); v != "" {
		envLines = append(envLines, fmt.Sprintf("# %s=your-token-here", v))
	}
	if len(envLines) > 0 {
		envPath := filepath.Join(m.outputDir, ".env")
		content := "# Environment variables for lingtai-daemon\n# Uncomment and fill in your values\n\n" + strings.Join(envLines, "\n") + "\n"
		if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
			return written, fmt.Errorf("writing .env: %w", err)
		}
		written = append(written, envPath)
	}

	return written, nil
}

func writeJSON(path string, data interface{}) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

// Run starts the interactive setup wizard, writing config to outputDir.
func Run(outputDir string) error {
	m := newWizardModel(outputDir)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("wizard error: %w", err)
	}
	if wm, ok := finalModel.(wizardModel); ok && wm.err != nil {
		return wm.err
	}
	return nil
}
