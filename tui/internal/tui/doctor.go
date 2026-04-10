package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
)

// doctorResultMsg is sent when the async diagnostic completes.
type doctorResultMsg struct {
	Lines []doctorLine
}

type doctorLine struct {
	Text string
	OK   bool // true = green check, false = red cross
	Hint bool // true = suggestion line (indented, dimmed)
}

// DoctorModel is the /doctor dedicated view.
type DoctorModel struct {
	orchDir string
	lines   []doctorLine
	loading bool
	width   int
	height  int
}

func NewDoctorModel(orchDir string) DoctorModel {
	return DoctorModel{orchDir: orchDir, loading: true}
}

func (m DoctorModel) Init() tea.Cmd {
	orchDir := m.orchDir
	return func() tea.Msg {
		return runDoctor(orchDir)
	}
}

func (m DoctorModel) Update(msg tea.Msg) (DoctorModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case doctorResultMsg:
		m.lines = msg.Lines
		m.loading = false
	case tea.KeyPressMsg:
		if msg.String() == "esc" {
			return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
		}
	}
	return m, nil
}

func (m DoctorModel) View() string {
	var b strings.Builder

	// Title bar
	title := StyleTitle.Render(i18n.T("app.title")) + " " +
		StyleAccent.Render(RuneBullet) + " " +
		StyleTitle.Render(i18n.T("doctor.title"))
	escHint := StyleAccent.Render("[esc] ") + StyleSubtle.Render(i18n.T("manage.back"))
	padding := m.width - lipgloss.Width(title) - lipgloss.Width(escHint) - 1
	if padding > 0 {
		b.WriteString(title + strings.Repeat(" ", padding) + escHint + "\n")
	} else {
		b.WriteString(title + "  " + escHint + "\n")
	}
	b.WriteString(strings.Repeat("─", m.width) + "\n\n")

	if m.loading {
		b.WriteString("  " + i18n.T("doctor.checking") + "\n")
		return b.String()
	}

	for _, line := range m.lines {
		if line.Hint {
			b.WriteString("  " + StyleAccent.Render(line.Text) + "\n")
		} else if line.OK {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(ColorAgent).Render(line.Text) + "\n")
		} else {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(ColorSuspended).Render(line.Text) + "\n")
		}
	}

	b.WriteString("\n" + strings.Repeat("─", m.width) + "\n")
	b.WriteString(StyleFaint.Render("  [esc] " + i18n.T("manage.back")) + "\n")

	return b.String()
}

// --- Diagnostic logic ---

// runDoctor performs the /doctor diagnostic and returns a doctorResultMsg.
func runDoctor(orchDir string) doctorResultMsg {
	var lines []doctorLine

	// Phase 0: check lingtai-portal on PATH
	if _, err := exec.LookPath("lingtai-portal"); err == nil {
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.portal_ok"), OK: true,
		})
	} else {
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.portal_missing"),
		})
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.suggest_portal"), Hint: true,
		})
	}

	// Phase 1: read events.jsonl for recent errors
	lastErr := findLastAPIError(orchDir)
	if lastErr != "" {
		lines = append(lines, doctorLine{
			Text: i18n.TF("doctor.last_error", lastErr),
		})
	}

	// Phase 2: read init.json to get LLM config
	provider, model, apiKey, baseURL, err := readLLMConfig(orchDir)
	if err != nil {
		lines = append(lines, doctorLine{
			Text: i18n.TF("doctor.config_error", err),
		})
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.suggest_refresh"), Hint: true,
		})
		return doctorResultMsg{Lines: lines}
	}

	// Phase 3: live API check
	status, detail := probeLLM(provider, model, apiKey, baseURL)

	switch status {
	case probeOK:
		lines = append(lines, doctorLine{
			Text: i18n.TF("doctor.llm_ok", provider, model), OK: true,
		})
		if lastErr != "" {
			lines = append(lines, doctorLine{
				Text: i18n.T("doctor.suggest_revive"), Hint: true,
			})
		} else {
			lines = append(lines, doctorLine{
				Text: i18n.T("doctor.healthy"), OK: true,
			})
		}
	case probeAuthError:
		lines = append(lines, doctorLine{
			Text: i18n.TF("doctor.llm_auth", detail),
		})
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.suggest_setup"), Hint: true,
		})
	case probeRateLimit:
		lines = append(lines, doctorLine{
			Text: i18n.TF("doctor.llm_rate", detail),
		})
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.suggest_wait"), Hint: true,
		})
	case probeOverloaded:
		lines = append(lines, doctorLine{
			Text: i18n.TF("doctor.llm_overloaded", detail),
		})
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.suggest_wait"), Hint: true,
		})
	case probeNetworkError:
		lines = append(lines, doctorLine{
			Text: i18n.TF("doctor.llm_network", detail),
		})
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.suggest_network"), Hint: true,
		})
	case probeNoKey:
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.llm_no_key"),
		})
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.suggest_setup"), Hint: true,
		})
	default:
		lines = append(lines, doctorLine{
			Text: i18n.TF("doctor.llm_unknown", detail),
		})
		lines = append(lines, doctorLine{
			Text: i18n.T("doctor.suggest_refresh"), Hint: true,
		})
	}

	return doctorResultMsg{Lines: lines}
}

// --- Event log scanning ---

type logEvent struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

// findLastAPIError scans events.jsonl for the most recent aed_attempt,
// aed_exhausted, or refresh_init_error event and returns the error string.
func findLastAPIError(orchDir string) string {
	logPath := filepath.Join(orchDir, "logs", "events.jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lastError string
	for scanner.Scan() {
		var ev logEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "aed_attempt", "aed_exhausted", "refresh_init_error":
			if ev.Error != "" {
				lastError = ev.Error
			}
		}
	}
	return lastError
}

// --- Init.json / env resolution ---

func readLLMConfig(orchDir string) (provider, model, apiKey, baseURL string, err error) {
	initPath := filepath.Join(orchDir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("cannot read init.json")
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", "", "", "", fmt.Errorf("invalid init.json")
	}

	manifest, _ := raw["manifest"].(map[string]interface{})
	if manifest == nil {
		return "", "", "", "", fmt.Errorf("no manifest in init.json")
	}

	llm, _ := manifest["llm"].(map[string]interface{})
	if llm == nil {
		return "", "", "", "", fmt.Errorf("no manifest.llm in init.json")
	}

	provider, _ = llm["provider"].(string)
	model, _ = llm["model"].(string)
	apiKey, _ = llm["api_key"].(string)
	baseURL, _ = llm["base_url"].(string)

	if apiKey == "" {
		apiKeyEnv, _ := llm["api_key_env"].(string)
		if apiKeyEnv != "" {
			envFile, _ := raw["env_file"].(string)
			apiKey = lookupEnvKey(envFile, orchDir, apiKeyEnv)
		}
	}

	return provider, model, apiKey, baseURL, nil
}

// lookupEnvKey resolves an environment variable name, checking os.Environ first,
// then parsing the .env file without mutating the process environment.
func lookupEnvKey(envFile, workingDir, envVarName string) string {
	if val, ok := os.LookupEnv(envVarName); ok {
		return val
	}
	if envFile == "" {
		return ""
	}

	p := envFile
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, p[2:])
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(workingDir, p)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		if strings.TrimSpace(key) == envVarName {
			return strings.Trim(strings.TrimSpace(val), "'\"")
		}
	}
	return ""
}

// --- Live LLM probe ---

type probeStatus int

const (
	probeOK probeStatus = iota
	probeAuthError
	probeRateLimit
	probeOverloaded
	probeNetworkError
	probeNoKey
	probeUnknown
)

func probeLLM(provider, model, apiKey, baseURL string) (probeStatus, string) {
	if apiKey == "" {
		return probeNoKey, ""
	}

	url, headers := providerProbeConfig(provider, apiKey, baseURL)
	if url == "" {
		return probeUnknown, "unknown provider: " + provider
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return probeNetworkError, err.Error()
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, apiKey) {
			errMsg = "connection failed"
		}
		return probeNetworkError, errMsg
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	body := string(bodyBytes)

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return probeOK, ""
	case resp.StatusCode == 404 || resp.StatusCode == 405:
		// /v1/models not supported but server responded — connectivity and auth OK
		return probeOK, ""
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return probeAuthError, fmt.Sprintf("%d %s", resp.StatusCode, extractErrorMessage(body))
	case resp.StatusCode == 429:
		return probeRateLimit, "429 rate limited"
	case resp.StatusCode == 529 || resp.StatusCode == 503:
		return probeOverloaded, fmt.Sprintf("%d overloaded", resp.StatusCode)
	default:
		return probeUnknown, fmt.Sprintf("%d %s", resp.StatusCode, extractErrorMessage(body))
	}
}

func providerProbeConfig(provider, apiKey, baseURL string) (string, map[string]string) {
	switch provider {
	case "anthropic":
		base := "https://api.anthropic.com"
		if baseURL != "" {
			base = strings.TrimRight(baseURL, "/")
		}
		return base + "/v1/models", map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": "2023-06-01",
		}
	case "openai":
		base := "https://api.openai.com"
		if baseURL != "" {
			base = strings.TrimRight(baseURL, "/")
		}
		return base + "/v1/models", map[string]string{
			"Authorization": "Bearer " + apiKey,
		}
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta/models", map[string]string{
			"x-goog-api-key": apiKey,
		}
	case "minimax":
		base := "https://api.minimax.io/anthropic"
		if baseURL != "" {
			base = strings.TrimRight(baseURL, "/")
		}
		return base + "/v1/models", map[string]string{
			"x-api-key":         apiKey,
			"anthropic-version": "2023-06-01",
		}
	case "zhipu":
		base := "https://open.bigmodel.cn/api/coding/paas/v4"
		if baseURL != "" {
			base = strings.TrimRight(baseURL, "/")
		}
		return base + "/models", map[string]string{
			"Authorization": "Bearer " + apiKey,
		}
	case "custom":
		if baseURL == "" {
			return "", nil
		}
		base := strings.TrimRight(baseURL, "/")
		return base + "/v1/models", map[string]string{
			"Authorization": "Bearer " + apiKey,
		}
	default:
		if baseURL != "" {
			base := strings.TrimRight(baseURL, "/")
			return base + "/v1/models", map[string]string{
				"Authorization": "Bearer " + apiKey,
			}
		}
		return "", nil
	}
}

func extractErrorMessage(body string) string {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		if len(body) > 100 {
			return body[:100] + "..."
		}
		return body
	}
	if errObj, ok := obj["error"].(map[string]interface{}); ok {
		if msg, ok := errObj["message"].(string); ok {
			return msg
		}
	}
	if errStr, ok := obj["error"].(string); ok {
		return errStr
	}
	if msg, ok := obj["message"].(string); ok {
		return msg
	}
	if len(body) > 100 {
		return body[:100] + "..."
	}
	return body
}
