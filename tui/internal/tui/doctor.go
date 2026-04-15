package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/migrate"
)

// tuiVersion is set once at startup by main via SetTUIVersion.
// Used by /doctor for the version-skew check.
var tuiVersion = "dev"

// SetTUIVersion records the running TUI binary version for doctor diagnostics.
func SetTUIVersion(v string) {
	if v != "" {
		tuiVersion = v
	}
}

// doctorResultMsg is sent when the async diagnostic completes.
type doctorResultMsg struct {
	Lines []doctorLine
}

type doctorLine struct {
	Text string
	OK   bool // true = green check, false = red cross (ignored if Warn or Hint)
	Warn bool // true = amber indicator (neutral info, e.g. version drift)
	Hint bool // true = suggestion line (indented, dimmed)
}

// DoctorModel is the /doctor dedicated view.
type DoctorModel struct {
	orchDir   string
	globalDir string
	lines     []doctorLine
	loading   bool
	width     int
	height    int
}

func NewDoctorModel(orchDir, globalDir string) DoctorModel {
	return DoctorModel{orchDir: orchDir, globalDir: globalDir, loading: true}
}

func (m DoctorModel) Init() tea.Cmd {
	orchDir := m.orchDir
	globalDir := m.globalDir
	return func() tea.Msg {
		return runDoctor(orchDir, globalDir)
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
		switch {
		case line.Hint:
			b.WriteString("  " + StyleAccent.Render(line.Text) + "\n")
		case line.Warn:
			b.WriteString("  " + lipgloss.NewStyle().Foreground(ColorStuck).Render(line.Text) + "\n")
		case line.OK:
			b.WriteString("  " + lipgloss.NewStyle().Foreground(ColorAgent).Render(line.Text) + "\n")
		default:
			b.WriteString("  " + lipgloss.NewStyle().Foreground(ColorSuspended).Render(line.Text) + "\n")
		}
	}

	b.WriteString("\n" + strings.Repeat("─", m.width) + "\n")
	b.WriteString(StyleFaint.Render("  [esc] " + i18n.T("manage.back")) + "\n")

	return b.String()
}

// --- Diagnostic logic ---

// runDoctor performs the /doctor diagnostic and returns a doctorResultMsg.
func runDoctor(orchDir, globalDir string) doctorResultMsg {
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

	// Phase 0.5: kernel health — these are the checks that catch "TUI upgraded
	// but Python kernel is old/broken/missing", which is the most common cause
	// of a post-upgrade regression (especially for CN users hitting mirror
	// flakiness during install).
	kernelOK := checkKernelHealth(orchDir, globalDir, &lines)
	_ = kernelOK // intentionally not short-circuiting: LLM probe is still useful info

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

// --- Kernel health checks ---

// checkKernelHealth runs K1–K6 and appends findings to lines.
// Returns true if every hard check passed. Soft warnings (version drift) do
// not affect the return value but still surface as Warn lines.
func checkKernelHealth(orchDir, globalDir string, lines *[]doctorLine) bool {
	allOK := true

	// K1. TUI binary version (always shown, informational)
	*lines = append(*lines, doctorLine{
		Text: i18n.TF("doctor.tui_version", tuiVersion), OK: true,
	})

	// K2. Which Python the TUI will use for agents.
	python := config.LingtaiCmd(globalDir)
	venvPython := config.VenvPython(config.RuntimeVenvDir(globalDir))
	usingVenv := python == venvPython
	if _, err := os.Stat(python); err != nil {
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.python_missing", python),
		})
		*lines = append(*lines, doctorLine{
			Text: i18n.T("doctor.suggest_venv"), Hint: true,
		})
		return false // downstream checks need a working python
	}
	if usingVenv {
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.python_venv", python), OK: true,
		})
	} else {
		// Fallback PATH python — works but means the TUI's managed venv is missing.
		// Not a hard failure (dev installs hit this path), but worth surfacing.
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.python_fallback", python), Warn: true,
		})
	}

	// K3. lingtai package importable, and capture version string.
	kernelVersion, importErr := probeKernelImport(python)
	if importErr != "" {
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.kernel_import_fail", importErr),
		})
		*lines = append(*lines, doctorLine{
			Text: i18n.T("doctor.suggest_reinstall_kernel"), Hint: true,
		})
		return false // can't proceed with further kernel checks
	}
	*lines = append(*lines, doctorLine{
		Text: i18n.TF("doctor.kernel_version", kernelVersion), OK: true,
	})

	// Note: the TUI binary and the Python kernel (`lingtai` on PyPI) ship
	// from separate repos with independent version numbers — they are NOT
	// meant to track each other. An earlier version of /doctor warned on
	// mismatch; that check was wrong and has been removed. Users see both
	// versions via K1 and K3 above and can compare manually if relevant.

	// K5. `python -m lingtai --help` exits 0 (catches broken entry points,
	// missing CLI deps like click/typer, etc. that `import lingtai` alone misses).
	if err, stderr := probeKernelCLI(python); err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = err.Error()
		}
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.kernel_cli_fail", detail),
		})
		*lines = append(*lines, doctorLine{
			Text: i18n.T("doctor.suggest_force_reinstall"), Hint: true,
		})
		allOK = false
	}

	// K6. Migration version in .lingtai/meta.json vs this binary's CurrentVersion.
	// orchDir is <projectRoot>/.lingtai/<orchName>, so .lingtai/ is its parent.
	lingtaiDir := filepath.Dir(orchDir)
	projectVersion, metaErr := readMetaVersion(lingtaiDir)
	switch {
	case metaErr != nil:
		// meta.json missing or unreadable — surface but don't fail the suite,
		// since a project being opened for the first time legitimately has none.
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.meta_unreadable", metaErr.Error()), Warn: true,
		})
	case projectVersion == migrate.CurrentVersion:
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.migration_ok", projectVersion), OK: true,
		})
	case projectVersion > migrate.CurrentVersion:
		// User downgraded the TUI. Data format is ahead of the binary.
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.migration_ahead", projectVersion, migrate.CurrentVersion),
		})
		*lines = append(*lines, doctorLine{
			Text: i18n.T("doctor.suggest_upgrade_tui"), Hint: true,
		})
		allOK = false
	default:
		// projectVersion < CurrentVersion — migrations should have run at startup,
		// so hitting this means a silent migration failure or a stale state.
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.migration_behind", projectVersion, migrate.CurrentVersion),
		})
		*lines = append(*lines, doctorLine{
			Text: i18n.T("doctor.suggest_restart_tui"), Hint: true,
		})
		allOK = false
	}

	// K7. Orchestrator heartbeat. Uses the canonical 3.0s liveness threshold
	// from app.go / mail.go / nirvana.go. No remediation suggestion — a stale
	// heartbeat can mean ASLEEP, SUSPENDED, crashed, or just lagging; the
	// main view is the authoritative place for state and recovery actions.
	age, hbErr := readHeartbeatAge(orchDir)
	switch {
	case hbErr != nil && os.IsNotExist(hbErr):
		*lines = append(*lines, doctorLine{
			Text: i18n.T("doctor.heartbeat_missing"), Warn: true,
		})
	case hbErr != nil:
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.heartbeat_unreadable", hbErr.Error()), Warn: true,
		})
	case age < 3*time.Second:
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.heartbeat_fresh", formatHeartbeatAge(age)), OK: true,
		})
	default:
		*lines = append(*lines, doctorLine{
			Text: i18n.TF("doctor.heartbeat_stale", formatHeartbeatAge(age)), Warn: true,
		})
	}

	return allOK
}

// readHeartbeatAge reads the orchestrator's .agent.heartbeat file and returns
// how long ago it was written. Mirrors fs.IsAlive's parsing but returns the
// age directly so /doctor can display it. Errors on missing or malformed
// heartbeat files are returned verbatim; the caller distinguishes IsNotExist
// from parse failures.
func readHeartbeatAge(orchDir string) (time.Duration, error) {
	path := filepath.Join(orchDir, ".agent.heartbeat")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	ts, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return 0, err
	}
	return time.Since(time.Unix(int64(ts), 0)), nil
}

// formatHeartbeatAge renders a duration in the terse unit-appropriate form
// humans expect in diagnostic output: sub-second as "just now", single-digit
// seconds as "Ns ago", minutes as "Nm ago", hours as "Nh ago". Longer than
// a day just shows "Nd ago".
func formatHeartbeatAge(d time.Duration) string {
	switch {
	case d < time.Second:
		return i18n.T("doctor.heartbeat_just_now")
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// probeKernelImport runs `python -c "import lingtai; print(lingtai.__version__)"`
// capturing stderr on failure so we can surface the real ImportError
// (not just "exit status 1"). Returns (version, "") on success, or ("", errMsg).
func probeKernelImport(python string) (string, string) {
	cmd := exec.Command(python, "-c", "import lingtai; print(lingtai.__version__)")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		// Collapse multi-line tracebacks to the last meaningful line
		// (usually "ModuleNotFoundError: ..." or "ImportError: ...").
		if idx := strings.LastIndex(detail, "\n"); idx >= 0 {
			detail = strings.TrimSpace(detail[idx+1:])
		}
		return "", detail
	}
	return strings.TrimSpace(stdout.String()), ""
}

// probeKernelCLI runs `python -m lingtai --help` and reports failure with captured stderr.
func probeKernelCLI(python string) (error, string) {
	cmd := exec.Command(python, "-m", "lingtai", "--help")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return err, stderr.String()
	}
	return nil, ""
}

// readMetaVersion reads .lingtai/meta.json and returns the version field.
// Returns (0, nil) if the file doesn't exist; (0, err) on parse failure.
func readMetaVersion(lingtaiDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(lingtaiDir, "meta.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var meta struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return 0, err
	}
	return meta.Version, nil
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
