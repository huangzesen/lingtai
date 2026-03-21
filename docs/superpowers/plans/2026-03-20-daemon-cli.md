# Daemon 器灵 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `daemon` Go CLI binary — the lingtai product frontend with Bubble Tea TUI, setup wizard, and spirit management.

**Architecture:** Bottom-up: config loader → TCP mail client → agent process manager → manage command → setup wizard → interactive TUI. Each package is independent and testable. The TUI consumes all lower layers.

**Tech Stack:** Go 1.22+, Bubble Tea (TUI), Lipgloss (styling), Bubbles (components). No other external deps.

**Spec:** `docs/superpowers/specs/2026-03-20-daemon-cli-design.md`

**ERRATA (post-review fixes — implementer MUST apply these):**

1. **`LogTailer` must use a large scanner buffer.** Default `bufio.Scanner` has 64KB limit — agent log lines can be larger. On construction: `scanner.Buffer(make([]byte, 1024*1024), 1024*1024)`. On scanner error, re-open the file (not just re-create scanner on same fd), seek to end, and continue.

2. **`process_test.go` imports must be complete.** The test file uses `net`, `time`, and `testing` — include all imports in the initial code block, not as a separate note.

3. **`isAlive()` is Unix-only.** Add `//go:build !windows` to `list.go`. Document that Windows is not a supported target for v1. (The Python agent itself uses Unix signals extensively.)

4. **Rename `var strings` in `i18n/strings.go` to `var translations`** to avoid shadowing the built-in `strings` package.

5. **Setup wizard must include a round-trip test.** Add a test in `setup/tests_test.go` that writes config using the wizard's output format, then reads it with `config.Load()`, verifying the config loads correctly. This ensures the wizard and loader agree on the schema.

6. **`MailClient.Send` opens a new connection per message — this is intentional.** Document it in a comment: each send is a fresh TCP connection (connect → read banner → send → close). No persistent connection needed since user messages are infrequent.

**Key conventions:**
- Go module: `lingtai-daemon` (in `daemon/` subdirectory)
- Package naming: `internal/` for non-exported packages
- Tests: `_test.go` in same package
- Error handling: return errors, don't panic
- i18n: string map from day 0 (`internal/i18n/`)

**Wire protocol reference:** lingtai's `TCPMailService` uses:
1. Server sends `STOAI {banner}\n` on connect
2. Client reads and discards banner line
3. Client sends `[4-byte big-endian length][JSON bytes]`
4. Server reads length, then reads that many bytes

---

### Task 1: Project Scaffold + Config Loader

Initialize Go module, create directory structure, implement config loading.

**Files:**
- Create: `daemon/go.mod`
- Create: `daemon/main.go`
- Create: `daemon/internal/config/loader.go`
- Create: `daemon/internal/config/loader_test.go`
- Create: `daemon/internal/i18n/strings.go`

- [ ] **Step 1: Initialize Go module**

```bash
cd daemon
go mod init lingtai-daemon
```

- [ ] **Step 2: Create directory structure**

```bash
mkdir -p internal/{config,i18n,agent,tui,setup,manage}
```

- [ ] **Step 3: Write config loader tests**

```go
// daemon/internal/config/loader_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Basic(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{
		"model": {"provider": "minimax", "model": "test", "api_key_env": "K"},
		"agent_name": "myagent"
	}`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentName != "myagent" {
		t.Errorf("got %q, want %q", cfg.AgentName, "myagent")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{
		"model": {"provider": "minimax", "model": "test", "api_key_env": "K"}
	}`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentName != "orchestrator" {
		t.Errorf("agent_name default: got %q, want %q", cfg.AgentName, "orchestrator")
	}
	if cfg.AgentPort != 8501 {
		t.Errorf("agent_port default: got %d, want %d", cfg.AgentPort, 8501)
	}
	if cfg.MaxTurns != 50 {
		t.Errorf("max_turns default: got %d, want %d", cfg.MaxTurns, 50)
	}
	if cfg.CLI != false {
		t.Error("cli default should be false")
	}
}

func TestLoadConfig_ModelFromFile(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "model.json")
	os.WriteFile(modelPath, []byte(`{
		"provider": "openai", "model": "gpt-4o", "api_key_env": "OAI_KEY"
	}`), 0644)
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{"model": "model.json"}`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model.Provider != "openai" {
		t.Errorf("got provider %q, want %q", cfg.Model.Provider, "openai")
	}
}

func TestLoadConfig_ModelInline(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{
		"model": {"provider": "anthropic", "model": "claude", "api_key_env": "ANT"}
	}`), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model.Provider != "anthropic" {
		t.Errorf("got provider %q, want %q", cfg.Model.Provider, "anthropic")
	}
}

func TestLoadConfig_MissingModel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	os.WriteFile(cfgPath, []byte(`{"agent_name": "x"}`), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Error("expected error for missing model")
	}
}

func TestLoadDotenv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("TEST_DAEMON_VAR=hello123\n"), 0644)

	os.Unsetenv("TEST_DAEMON_VAR")
	LoadDotenv(dir)
	if v := os.Getenv("TEST_DAEMON_VAR"); v != "hello123" {
		t.Errorf("got %q, want %q", v, "hello123")
	}
	os.Unsetenv("TEST_DAEMON_VAR") // cleanup
}

func TestResolveEnvVar(t *testing.T) {
	os.Setenv("TEST_RESOLVE_KEY", "secret")
	defer os.Unsetenv("TEST_RESOLVE_KEY")

	val, err := ResolveEnvVar("TEST_RESOLVE_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "secret" {
		t.Errorf("got %q, want %q", val, "secret")
	}
}

func TestResolveEnvVar_Missing(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR")
	_, err := ResolveEnvVar("NONEXISTENT_VAR")
	if err == nil {
		t.Error("expected error for missing env var")
	}
}
```

- [ ] **Step 4: Implement config loader**

```go
// daemon/internal/config/loader.go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ModelConfig holds LLM provider settings.
type ModelConfig struct {
	Provider  string       `json:"provider"`
	Model     string       `json:"model"`
	APIKeyEnv string       `json:"api_key_env"`
	BaseURL   string       `json:"base_url,omitempty"`
	Vision    *ModelConfig `json:"vision,omitempty"`
	WebSearch *ModelConfig `json:"web_search,omitempty"`
}

// IMAPConfig holds IMAP addon settings (passed through to Python).
type IMAPConfig map[string]interface{}

// TelegramConfig holds Telegram addon settings (passed through to Python).
type TelegramConfig map[string]interface{}

// Config is the top-level daemon configuration.
type Config struct {
	Model     ModelConfig    `json:"-"` // resolved from "model" field
	IMAP      IMAPConfig     `json:"imap,omitempty"`
	Telegram  TelegramConfig `json:"telegram,omitempty"`
	CLI       bool           `json:"cli"`
	AgentName string         `json:"agent_name"`
	BaseDir   string         `json:"base_dir"`
	BashPolicy string        `json:"bash_policy,omitempty"`
	MaxTurns  int            `json:"max_turns"`
	AgentPort int            `json:"agent_port"`
	CLIPort   int            `json:"cli_port,omitempty"`
	Covenant  string         `json:"covenant,omitempty"`

	// Internal
	ConfigDir string `json:"-"` // directory containing config.json
}

// Load reads and validates a config.json file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config not found: %s", path)
	}

	absPath, _ := filepath.Abs(path)
	configDir := filepath.Dir(absPath)

	// Load .env from config directory
	LoadDotenv(configDir)

	// Parse into raw map first to handle the "model" field polymorphism
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}

	// Parse everything except "model" into Config struct
	cfg := &Config{
		AgentName: "orchestrator",
		BaseDir:   "~/.lingtai",
		MaxTurns:  50,
		AgentPort: 8501,
		CLI:       false,
		ConfigDir: configDir,
	}
	// Re-unmarshal to get defaults overridden
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Apply defaults for zero values
	if cfg.AgentName == "" {
		cfg.AgentName = "orchestrator"
	}
	if cfg.BaseDir == "" {
		cfg.BaseDir = "~/.lingtai"
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 50
	}
	if cfg.AgentPort == 0 {
		cfg.AgentPort = 8501
	}
	if cfg.CLIPort == 0 {
		cfg.CLIPort = cfg.AgentPort + 1
	}

	// Expand ~ in base_dir
	if strings.HasPrefix(cfg.BaseDir, "~") {
		home, _ := os.UserHomeDir()
		cfg.BaseDir = filepath.Join(home, cfg.BaseDir[1:])
	}

	// Resolve model config
	modelRaw, ok := raw["model"]
	if !ok {
		return nil, fmt.Errorf("'model' field is required in config.json")
	}

	// Try as string (file path) first
	var modelPath string
	if err := json.Unmarshal(modelRaw, &modelPath); err == nil {
		// It's a string — load from file
		fullPath := filepath.Join(configDir, modelPath)
		modelData, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("model config not found: %s", fullPath)
		}
		if err := json.Unmarshal(modelData, &cfg.Model); err != nil {
			return nil, fmt.Errorf("invalid model config: %w", err)
		}
	} else {
		// Try as inline object
		if err := json.Unmarshal(modelRaw, &cfg.Model); err != nil {
			return nil, fmt.Errorf("'model' must be a file path or inline object: %w", err)
		}
	}

	if cfg.Model.Provider == "" {
		return nil, fmt.Errorf("model.provider is required")
	}

	return cfg, nil
}

// LoadDotenv loads a .env file from the given directory into os.Environ.
// Existing env vars are not overwritten (setenv only if not already set).
func LoadDotenv(dir string) {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, "'\"")
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}

// ResolveEnvVar looks up an environment variable by name.
// Returns an error if the variable is not set.
func ResolveEnvVar(name string) (string, error) {
	val, ok := os.LookupEnv(name)
	if !ok || val == "" {
		return "", fmt.Errorf("environment variable %q is not set — add it to your environment or .env file", name)
	}
	return val, nil
}

// WorkingDir returns the agent's working directory: {base_dir}/{agent_name}
func (c *Config) WorkingDir() string {
	return filepath.Join(c.BaseDir, c.AgentName)
}
```

- [ ] **Step 5: Create i18n string map**

```go
// daemon/internal/i18n/strings.go
package i18n

import "os"

// Lang is the current language code.
var Lang = detectLang()

// S returns the localized string for the given key.
func S(key string) string {
	if m, ok := strings[Lang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	// Fall back to English
	if s, ok := strings["en"][key]; ok {
		return s
	}
	return key
}

var strings = map[string]map[string]string{
	"en": {
		"title":         "Daemon",
		"setup_title":   "Setup Wizard",
		"manage_title":  "Running Spirits",
		"starting":      "Starting agent...",
		"shutting_down":  "Shutting down...",
		"connected":     "Connected",
		"disconnected":  "Disconnected",
		"press_ctrl_c":  "Press Ctrl+C to shut down",
		"type_message":  "Type a message...",
		"no_spirits":    "No running spirits found.",
		"name":          "Name",
		"pid":           "PID",
		"port":          "Port",
		"uptime":        "Uptime",
		"status":        "Status",
		"running":       "running",
		"dead":          "dead (stale PID)",
		"setup_model":   "LLM Provider",
		"setup_imap":    "IMAP Email",
		"setup_telegram": "Telegram Bot",
		"setup_general": "General Settings",
		"setup_review":  "Review",
		"setup_done":    "Setup Complete",
	},
	"zh": {
		"title":         "器灵",
		"setup_title":   "设置向导",
		"manage_title":  "运行中的器灵",
		"starting":      "正在启动代理...",
		"shutting_down":  "正在关闭...",
		"connected":     "已连接",
		"disconnected":  "未连接",
		"press_ctrl_c":  "按 Ctrl+C 关闭",
		"type_message":  "输入消息...",
		"no_spirits":    "没有运行中的器灵。",
		"name":          "名称",
		"pid":           "进程号",
		"port":          "端口",
		"uptime":        "运行时间",
		"status":        "状态",
		"running":       "运行中",
		"dead":          "已停止（残留PID）",
		"setup_model":   "语言模型配置",
		"setup_imap":    "IMAP 邮箱",
		"setup_telegram": "Telegram 机器人",
		"setup_general": "基本设置",
		"setup_review":  "确认",
		"setup_done":    "设置完成",
	},
}

func detectLang() string {
	lang := os.Getenv("LANG")
	if len(lang) >= 2 && (lang[:2] == "zh") {
		return "zh"
	}
	return "en"
}
```

- [ ] **Step 6: Create minimal main.go**

```go
// daemon/main.go
package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]

	if len(args) > 0 {
		switch args[0] {
		case "setup":
			fmt.Println("daemon setup — not yet implemented")
			return
		case "manage":
			fmt.Println("daemon manage — not yet implemented")
			return
		}
	}

	fmt.Println("daemon — not yet implemented")
}
```

- [ ] **Step 7: Run tests**

```bash
cd daemon && go test ./internal/config/ -v
```

- [ ] **Step 8: Commit**

```bash
git add daemon/
git commit -m "feat(daemon): project scaffold + config loader with i18n"
```

---

### Task 2: TCP Mail Client

Implement the lingtai TCP mail protocol — connect, read banner, send/receive length-prefixed JSON.

**Files:**
- Create: `daemon/internal/agent/mail.go`
- Create: `daemon/internal/agent/mail_test.go`

- [ ] **Step 1: Write tests**

```go
// daemon/internal/agent/mail_test.go
package agent

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

// mockServer simulates a TCPMailService server.
func mockServer(t *testing.T, port int, banner string, handler func(net.Conn)) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Send banner
		fmt.Fprintf(conn, "STOAI %s\n", banner)
		handler(conn)
	}()
	return ln
}

func TestMailClient_Send(t *testing.T) {
	port := 19901
	received := make(chan map[string]interface{}, 1)

	ln := mockServer(t, port, "test-banner", func(conn net.Conn) {
		// Read length prefix
		var length uint32
		binary.Read(conn, binary.BigEndian, &length)
		buf := make([]byte, length)
		conn.Read(buf)
		var msg map[string]interface{}
		json.Unmarshal(buf, &msg)
		received <- msg
	})
	defer ln.Close()

	time.Sleep(50 * time.Millisecond) // let server start

	client := NewMailClient(fmt.Sprintf("127.0.0.1:%d", port))
	err := client.Send(map[string]interface{}{
		"from":    "cli@localhost:19902",
		"message": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-received:
		if msg["message"] != "hello" {
			t.Errorf("got message %q, want %q", msg["message"], "hello")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestMailClient_SendBannerRead(t *testing.T) {
	// Verify the client reads and discards the banner before sending
	port := 19902
	bannerRead := make(chan bool, 1)

	ln := mockServer(t, port, "my-banner-123", func(conn net.Conn) {
		// If client sent before reading banner, we'd get garbage
		var length uint32
		err := binary.Read(conn, binary.BigEndian, &length)
		if err == nil && length > 0 && length < 10000 {
			bannerRead <- true
		} else {
			bannerRead <- false
		}
	})
	defer ln.Close()

	time.Sleep(50 * time.Millisecond)

	client := NewMailClient(fmt.Sprintf("127.0.0.1:%d", port))
	client.Send(map[string]interface{}{"message": "test"})

	select {
	case ok := <-bannerRead:
		if !ok {
			t.Error("banner was not properly read before sending")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestMailListener_Receive(t *testing.T) {
	port := 19903
	received := make(chan map[string]interface{}, 1)

	listener, err := NewMailListener(port, func(msg map[string]interface{}) {
		received <- msg
	})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Stop()

	time.Sleep(50 * time.Millisecond)

	// Simulate agent sending a reply — connect, read banner, send message
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Read banner
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	banner := string(buf[:n])
	if len(banner) == 0 || banner[:5] != "STOAI" {
		t.Errorf("expected STOAI banner, got %q", banner)
	}

	// Send length-prefixed JSON
	payload, _ := json.Marshal(map[string]interface{}{
		"from":    "orchestrator",
		"message": "reply text",
	})
	binary.Write(conn, binary.BigEndian, uint32(len(payload)))
	conn.Write(payload)

	select {
	case msg := <-received:
		if msg["message"] != "reply text" {
			t.Errorf("got %q, want %q", msg["message"], "reply text")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}
```

- [ ] **Step 2: Implement mail client and listener**

```go
// daemon/internal/agent/mail.go
package agent

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// MailClient sends messages to a lingtai TCP mail server.
type MailClient struct {
	address string
}

// NewMailClient creates a client targeting the given address (host:port).
func NewMailClient(address string) *MailClient {
	return &MailClient{address: address}
}

// Send connects, reads banner, sends a length-prefixed JSON message.
func (c *MailClient) Send(msg map[string]interface{}) error {
	conn, err := net.Dial("tcp", c.address)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", c.address, err)
	}
	defer conn.Close()

	// Read and discard banner line (STOAI {id}\n)
	reader := bufio.NewReader(conn)
	_, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read banner: %w", err)
	}

	// Send length-prefixed JSON
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := binary.Write(conn, binary.BigEndian, uint32(len(payload))); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

// MailListener listens for incoming TCP mail messages.
type MailListener struct {
	listener net.Listener
	handler  func(map[string]interface{})
	wg       sync.WaitGroup
	done     chan struct{}
}

// NewMailListener starts a TCP mail server on the given port.
func NewMailListener(port int, handler func(map[string]interface{})) (*MailListener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, err
	}
	ml := &MailListener{
		listener: ln,
		handler:  handler,
		done:     make(chan struct{}),
	}
	ml.wg.Add(1)
	go ml.acceptLoop()
	return ml, nil
}

// Stop shuts down the listener.
func (ml *MailListener) Stop() {
	close(ml.done)
	ml.listener.Close()
	ml.wg.Wait()
}

func (ml *MailListener) acceptLoop() {
	defer ml.wg.Done()
	for {
		conn, err := ml.listener.Accept()
		if err != nil {
			select {
			case <-ml.done:
				return
			default:
				continue
			}
		}
		go ml.handleConn(conn)
	}
}

func (ml *MailListener) handleConn(conn net.Conn) {
	defer conn.Close()

	// Send banner
	fmt.Fprintf(conn, "STOAI daemon\n")

	// Read length-prefixed JSON
	var length uint32
	if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
		return
	}
	if length == 0 || length > 10*1024*1024 { // 10MB sanity limit
		return
	}
	buf := make([]byte, length)
	if _, err := readFull(conn, buf); err != nil {
		return
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(buf, &msg); err != nil {
		return
	}
	ml.handler(msg)
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
```

- [ ] **Step 3: Run tests**

```bash
cd daemon && go test ./internal/agent/ -v -timeout 10s
```

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/agent/
git commit -m "feat(daemon): TCP mail client + listener (lingtai wire protocol)"
```

---

### Task 3: Agent Process Manager

Start/stop/monitor the Python subprocess. Wait for TCP port readiness with backoff.

**Files:**
- Create: `daemon/internal/agent/process.go`
- Create: `daemon/internal/agent/process_test.go`
- Create: `daemon/internal/agent/pid.go`
- Create: `daemon/internal/agent/pid_test.go`

- [ ] **Step 1: Write PID file tests**

```go
// daemon/internal/agent/pid_test.go
package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.pid")

	err := WritePIDFile(path, 12345, 8501, "/path/to/config.json")
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	var info PIDInfo
	json.Unmarshal(data, &info)
	if info.PID != 12345 {
		t.Errorf("PID: got %d, want %d", info.PID, 12345)
	}
	if info.Port != 8501 {
		t.Errorf("Port: got %d, want %d", info.Port, 8501)
	}
}

func TestReadPIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.pid")

	WritePIDFile(path, 99999, 8501, "/cfg")

	info, err := ReadPIDFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.PID != 99999 {
		t.Errorf("got PID %d", info.PID)
	}
}

func TestRemovePIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.pid")

	WritePIDFile(path, 1, 1, "")
	RemovePIDFile(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("PID file should be deleted")
	}
}
```

- [ ] **Step 2: Implement PID file**

```go
// daemon/internal/agent/pid.go
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PIDInfo is the JSON content of an agent.pid file.
type PIDInfo struct {
	PID     int    `json:"pid"`
	Port    int    `json:"port"`
	Config  string `json:"config"`
	Started string `json:"started"`
}

// WritePIDFile writes agent.pid with process info.
func WritePIDFile(path string, pid, port int, configPath string) error {
	os.MkdirAll(filepath.Dir(path), 0755)
	info := PIDInfo{
		PID:     pid,
		Port:    port,
		Config:  configPath,
		Started: time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	return os.WriteFile(path, data, 0644)
}

// ReadPIDFile reads an agent.pid file.
func ReadPIDFile(path string) (*PIDInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var info PIDInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("invalid PID file: %w", err)
	}
	return &info, nil
}

// RemovePIDFile deletes the PID file.
func RemovePIDFile(path string) {
	os.Remove(path)
}
```

- [ ] **Step 3: Write process manager tests**

```go
// daemon/internal/agent/process_test.go
package agent

import (
	"testing"
)

func TestWaitForPort_AlreadyOpen(t *testing.T) {
	// Start a listener, then check that WaitForPort succeeds immediately
	ln, _ := net.Listen("tcp", "127.0.0.1:19910")
	defer ln.Close()

	err := WaitForPort(19910, 2*time.Second)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestWaitForPort_Timeout(t *testing.T) {
	// Port 19911 is not listening
	err := WaitForPort(19911, 500*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}
```

Add missing imports at top of test file:
```go
import (
	"net"
	"testing"
	"time"
)
```

- [ ] **Step 4: Implement process manager**

```go
// daemon/internal/agent/process.go
package agent

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Process manages a Python agent subprocess.
type Process struct {
	cmd       *exec.Cmd
	configPath string
	agentPort int
	workingDir string
	pidPath   string
	logFile   *os.File
}

// StartOptions configures how to start the agent.
type StartOptions struct {
	ConfigPath string // path to config.json
	AgentPort  int
	WorkingDir string // {base_dir}/{agent_name}
	Headless   bool   // redirect stdout/stderr to log file
}

// Start spawns the Python agent subprocess.
func Start(opts StartOptions) (*Process, error) {
	// Ensure working dir exists
	os.MkdirAll(opts.WorkingDir, 0755)

	cmd := exec.Command("python", "-m", "app", opts.ConfigPath)
	cmd.Dir = filepath.Dir(opts.ConfigPath)

	p := &Process{
		cmd:        cmd,
		configPath: opts.ConfigPath,
		agentPort:  opts.AgentPort,
		workingDir: opts.WorkingDir,
		pidPath:    filepath.Join(opts.WorkingDir, "agent.pid"),
	}

	if opts.Headless {
		// Redirect to log file
		logPath := filepath.Join(opts.WorkingDir, "daemon.log")
		os.MkdirAll(filepath.Dir(logPath), 0755)
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		cmd.Stdout = f
		cmd.Stderr = f
		p.logFile = f
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start python: %w", err)
	}

	// Write PID file
	WritePIDFile(p.pidPath, cmd.Process.Pid, opts.AgentPort, opts.ConfigPath)

	// Wait for agent TCP port to be ready
	if err := WaitForPort(opts.AgentPort, 30*time.Second); err != nil {
		cmd.Process.Kill()
		RemovePIDFile(p.pidPath)
		return nil, fmt.Errorf("agent failed to start: %w", err)
	}

	return p, nil
}

// Stop sends SIGTERM and waits for the process to exit.
func (p *Process) Stop() error {
	if p.cmd.Process != nil {
		p.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- p.cmd.Wait() }()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			p.cmd.Process.Kill()
		}
	}
	RemovePIDFile(p.pidPath)
	if p.logFile != nil {
		p.logFile.Close()
	}
	return nil
}

// PID returns the subprocess PID.
func (p *Process) PID() int {
	if p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// WaitForPort polls a TCP port with exponential backoff until it accepts connections.
func WaitForPort(port int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	backoff := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(backoff)
		backoff = backoff * 2
		if backoff > 5*time.Second {
			backoff = 5 * time.Second
		}
	}
	return fmt.Errorf("port %d not ready after %s", port, timeout)
}
```

- [ ] **Step 5: Run tests**

```bash
cd daemon && go test ./internal/agent/ -v -timeout 15s
```

- [ ] **Step 6: Commit**

```bash
git add daemon/internal/agent/
git commit -m "feat(daemon): agent process manager + PID files"
```

---

### Task 4: Manage Command

List running spirits by scanning PID files.

**Files:**
- Create: `daemon/internal/manage/list.go`
- Create: `daemon/internal/manage/list_test.go`

- [ ] **Step 1: Write tests**

```go
// daemon/internal/manage/list_test.go
package manage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestScanSpirits_Empty(t *testing.T) {
	dir := t.TempDir()
	spirits := ScanSpirits(dir)
	if len(spirits) != 0 {
		t.Errorf("expected 0 spirits, got %d", len(spirits))
	}
}

func TestScanSpirits_FindsPID(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "myagent")
	os.MkdirAll(agentDir, 0755)

	pidInfo := map[string]interface{}{
		"pid": os.Getpid(), // use our own PID so it's "alive"
		"port": 8501,
		"config": "/path/to/config.json",
		"started": "2026-03-20T12:00:00Z",
	}
	data, _ := json.Marshal(pidInfo)
	os.WriteFile(filepath.Join(agentDir, "agent.pid"), data, 0644)

	spirits := ScanSpirits(dir)
	if len(spirits) != 1 {
		t.Fatalf("expected 1 spirit, got %d", len(spirits))
	}
	if spirits[0].Name != "myagent" {
		t.Errorf("name: got %q, want %q", spirits[0].Name, "myagent")
	}
	if !spirits[0].Alive {
		t.Error("spirit should be alive (our own PID)")
	}
}

func TestScanSpirits_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "deadagent")
	os.MkdirAll(agentDir, 0755)

	pidInfo := map[string]interface{}{
		"pid": 999999999, // almost certainly not a real PID
		"port": 8501,
		"started": "2026-03-20T12:00:00Z",
	}
	data, _ := json.Marshal(pidInfo)
	os.WriteFile(filepath.Join(agentDir, "agent.pid"), data, 0644)

	spirits := ScanSpirits(dir)
	if len(spirits) != 1 {
		t.Fatalf("expected 1 spirit, got %d", len(spirits))
	}
	if spirits[0].Alive {
		t.Error("spirit with PID 999999999 should be dead")
	}
}
```

- [ ] **Step 2: Implement**

```go
// daemon/internal/manage/list.go
package manage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"lingtai-daemon/internal/i18n"
)

// Spirit represents a running (or stale) agent.
type Spirit struct {
	Name    string
	PID     int
	Port    int
	Config  string
	Started time.Time
	Alive   bool
}

// ScanSpirits scans base_dir for agent.pid files.
func ScanSpirits(baseDir string) []Spirit {
	var spirits []Spirit
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return spirits
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pidPath := filepath.Join(baseDir, entry.Name(), "agent.pid")
		data, err := os.ReadFile(pidPath)
		if err != nil {
			continue
		}
		var info struct {
			PID     int    `json:"pid"`
			Port    int    `json:"port"`
			Config  string `json:"config"`
			Started string `json:"started"`
		}
		if json.Unmarshal(data, &info) != nil {
			continue
		}
		started, _ := time.Parse(time.RFC3339, info.Started)
		spirits = append(spirits, Spirit{
			Name:    entry.Name(),
			PID:     info.PID,
			Port:    info.Port,
			Config:  info.Config,
			Started: started,
			Alive:   isAlive(info.PID),
		})
	}
	return spirits
}

// isAlive checks if a process with the given PID is running.
func isAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Use Signal(0) to check.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// FormatTable renders spirits as a colored table string.
func FormatTable(spirits []Spirit) string {
	if len(spirits) == 0 {
		return fmt.Sprintf("  %s\n", i18n.S("no_spirits"))
	}

	header := fmt.Sprintf(
		" \033[1m%-20s %-8s %-6s %-12s %s\033[0m\n",
		i18n.S("name"), i18n.S("pid"), i18n.S("port"),
		i18n.S("uptime"), i18n.S("status"),
	)
	result := header
	for _, s := range spirits {
		uptime := ""
		status := ""
		if s.Alive {
			uptime = formatDuration(time.Since(s.Started))
			status = fmt.Sprintf("\033[32m● %s\033[0m", i18n.S("running"))
		} else {
			uptime = "—"
			status = fmt.Sprintf("\033[31m✗ %s\033[0m", i18n.S("dead"))
		}
		result += fmt.Sprintf(
			" %-20s %-8d %-6d %-12s %s\n",
			s.Name, s.PID, s.Port, uptime, status,
		)
	}
	result += fmt.Sprintf("\n  \033[2mStop with: kill <PID>\033[0m\n")
	return result
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
```

- [ ] **Step 3: Wire manage into main.go**

Update `main.go` case `"manage"`:
```go
case "manage":
    baseDir := "~/.lingtai"
    // check for --base-dir flag
    for i, arg := range args {
        if arg == "--base-dir" && i+1 < len(args) {
            baseDir = args[i+1]
        }
    }
    if strings.HasPrefix(baseDir, "~") {
        home, _ := os.UserHomeDir()
        baseDir = filepath.Join(home, baseDir[1:])
    }
    spirits := manage.ScanSpirits(baseDir)
    fmt.Print(manage.FormatTable(spirits))
    return
```

- [ ] **Step 4: Run tests**

```bash
cd daemon && go test ./internal/manage/ -v
```

- [ ] **Step 5: Commit**

```bash
git add daemon/
git commit -m "feat(daemon): manage command — list running spirits"
```

---

### Task 5: Setup Wizard

Bubble Tea multi-step form with connection testing.

**Files:**
- Create: `daemon/internal/setup/wizard.go`
- Create: `daemon/internal/setup/tests.go`
- Create: `daemon/internal/setup/tests_test.go`

- [ ] **Step 1: Add Bubble Tea dependencies**

```bash
cd daemon
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbles
```

- [ ] **Step 2: Write connection tester tests**

```go
// daemon/internal/setup/tests_test.go
package setup

import (
	"testing"
)

func TestTestEnvVar_Set(t *testing.T) {
	t.Setenv("TEST_SETUP_VAR", "value")
	result := TestEnvVar("TEST_SETUP_VAR")
	if !result.OK {
		t.Error("expected OK for set env var")
	}
}

func TestTestEnvVar_Missing(t *testing.T) {
	result := TestEnvVar("DEFINITELY_NOT_SET_XYZ")
	if result.OK {
		t.Error("expected failure for missing env var")
	}
}
```

- [ ] **Step 3: Implement connection testers**

```go
// daemon/internal/setup/tests.go
package setup

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"time"
)

// TestResult holds the result of a connection test.
type TestResult struct {
	OK      bool
	Message string
}

// TestEnvVar checks if an environment variable is set.
func TestEnvVar(name string) TestResult {
	val := os.Getenv(name)
	if val == "" {
		return TestResult{OK: false, Message: fmt.Sprintf("%s is not set", name)}
	}
	return TestResult{OK: true, Message: fmt.Sprintf("%s is set (%d chars)", name, len(val))}
}

// TestIMAP tests an IMAP SSL connection + login.
func TestIMAP(host string, port int, user, pass string) TestResult {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp", addr,
		&tls.Config{ServerName: host},
	)
	if err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("IMAP connect failed: %v", err)}
	}
	defer conn.Close()

	// Read greeting
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.Read(buf)

	// Send LOGIN
	fmt.Fprintf(conn, "A001 LOGIN %q %q\r\n", user, pass)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("IMAP login failed: %v", err)}
	}
	resp := string(buf[:n])
	if len(resp) > 4 && resp[:4] != "A001" {
		// May need to read more
	}
	// Check for OK
	if contains(resp, "OK") {
		fmt.Fprintf(conn, "A002 LOGOUT\r\n")
		return TestResult{OK: true, Message: "IMAP connection successful"}
	}
	return TestResult{OK: false, Message: fmt.Sprintf("IMAP login rejected: %s", resp)}
}

// TestSMTP tests an SMTP connection with STARTTLS + login.
func TestSMTP(host string, port int, user, pass string) TestResult {
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := smtp.Dial(addr)
	if err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("SMTP connect failed: %v", err)}
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("STARTTLS failed: %v", err)}
	}

	auth := smtp.PlainAuth("", user, pass, host)
	if err := client.Auth(auth); err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("SMTP auth failed: %v", err)}
	}

	client.Quit()
	return TestResult{OK: true, Message: "SMTP connection successful"}
}

// TestTelegram tests a Telegram bot token via getMe.
func TestTelegram(token string) TestResult {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return TestResult{OK: false, Message: fmt.Sprintf("Telegram API error: %v", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
	}
	json.Unmarshal(body, &result)

	if result.OK {
		return TestResult{OK: true, Message: fmt.Sprintf("Telegram bot: @%s", result.Result.Username)}
	}
	return TestResult{OK: false, Message: fmt.Sprintf("Telegram rejected: %s", string(body))}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Implement setup wizard**

This is the largest piece — a Bubble Tea multi-step form. The wizard has these pages:
1. Model config (provider, model, api_key_env)
2. IMAP (optional — email, password, hosts)
3. Telegram (optional — bot token)
4. General (agent name, base dir, port)
5. Review + write

Create `daemon/internal/setup/wizard.go` with:
- A `model` struct implementing `tea.Model`
- Steps as an enum
- Each step renders a form with input fields
- Tab/Enter to navigate, Esc to skip optional sections
- Connection test results shown inline with colored indicators
- Final step writes config.json, model.json, .env

The wizard should use lipgloss for:
- Section headers (bold + cyan)
- Success indicators (green ✓)
- Error indicators (red ✗)
- Input prompts (white)
- Dim hints

Implementation is ~300-400 lines. The full code should be written by the implementer following the Bubble Tea patterns (Init/Update/View) with the step-based form state machine.

- [ ] **Step 5: Wire setup into main.go**

```go
case "setup":
    setup.Run()
    return
```

- [ ] **Step 6: Build and test manually**

```bash
cd daemon && go build -o daemon . && ./daemon setup
```

- [ ] **Step 7: Commit**

```bash
git add daemon/
git commit -m "feat(daemon): setup wizard with Bubble Tea + connection testing"
```

---

### Task 6: Interactive TUI

The main event — Bubble Tea TUI with message panel, input, status bar, and JSONL log tailing.

**Files:**
- Create: `daemon/internal/tui/app.go`
- Create: `daemon/internal/tui/styles.go`
- Create: `daemon/internal/tui/logtail.go`

- [ ] **Step 1: Create styles**

```go
// daemon/internal/tui/styles.go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Status bar
	StatusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1)

	ActiveChannel = lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true)

	DisabledChannel = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	// Messages
	IMAPReceived = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))   // green
	IMAPSent     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))  // yellow
	TGReceived   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))   // green
	TGSent       = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))  // yellow
	EmailMsg     = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))   // cyan
	AgentMsg     = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	ToolCall     = lipgloss.NewStyle().Foreground(lipgloss.Color("69"))   // blue
	DiaryMsg     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))  // dim

	// Title
	TitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("75")).
		Padding(0, 1)

	// Input
	InputPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))

	// Borders
	BorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))
)
```

- [ ] **Step 2: Create JSONL log tailer**

```go
// daemon/internal/tui/logtail.go
package tui

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// LogEvent is a parsed JSONL event from the agent's log.
type LogEvent struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Subject   string `json:"subject,omitempty"`
	To        interface{} `json:"to,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	Name      string `json:"name,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// GetToolName returns the tool name, checking both field names.
func (e LogEvent) GetToolName() string {
	if e.ToolName != "" {
		return e.ToolName
	}
	return e.Name
}

// LogTailer tails a JSONL file and sends events to a channel.
type LogTailer struct {
	path   string
	events chan LogEvent
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewLogTailer starts tailing the given JSONL file.
func NewLogTailer(path string) *LogTailer {
	lt := &LogTailer{
		path:   path,
		events: make(chan LogEvent, 100),
		done:   make(chan struct{}),
	}
	lt.wg.Add(1)
	go lt.tailLoop()
	return lt
}

// Events returns the channel of parsed log events.
func (lt *LogTailer) Events() <-chan LogEvent {
	return lt.events
}

// Stop stops the tailer.
func (lt *LogTailer) Stop() {
	close(lt.done)
	lt.wg.Wait()
}

func (lt *LogTailer) tailLoop() {
	defer lt.wg.Done()

	// Wait for file to exist
	for {
		select {
		case <-lt.done:
			return
		default:
		}
		if _, err := os.Stat(lt.path); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	f, err := os.Open(lt.path)
	if err != nil {
		return
	}
	defer f.Close()

	// Seek to end
	f.Seek(0, 2)

	scanner := bufio.NewScanner(f)
	for {
		select {
		case <-lt.done:
			return
		default:
		}
		if scanner.Scan() {
			var event LogEvent
			if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Type != "" {
				lt.events <- event
			}
		} else {
			// No new data, wait briefly
			time.Sleep(200 * time.Millisecond)
			// Reset scanner to pick up new data
			scanner = bufio.NewScanner(f)
		}
	}
}
```

- [ ] **Step 3: Implement main TUI model**

Create `daemon/internal/tui/app.go` with:

- `Model` struct: messages ([]string), viewport, textinput, config, mail client/listener, log tailer, width/height
- `Init()`: start log tailer, return `tea.Batch` of tick commands
- `Update()`: handle key events (Enter → send, Ctrl+C → quit), window resize, log events, mail received
- `View()`: render status bar + message panel (viewport) + input box

The TUI model is the most complex piece (~200-300 lines). Key behaviors:
- Messages are colored strings appended to a viewport
- Log events from the JSONL tailer are formatted and added as messages
- TCP mail replies are formatted and added as `[daemon]` prefixed messages
- User input sends TCP mail to agent, adds as `> user text` message
- Status bar shows: title (器灵/Daemon), active channels, agent name, port

The implementer should follow standard Bubble Tea patterns with `viewport` and `textinput` from the `bubbles` package.

- [ ] **Step 4: Wire TUI into main.go**

The default command (no subcommand) should:
1. Load config
2. Start agent process
3. Start log tailer
4. Start mail listener
5. Run Bubble Tea program
6. On quit: stop everything

```go
// In main.go default case:
cfg, err := config.Load(configPath)
// ... error handling ...

proc, err := agent.Start(agent.StartOptions{...})
// ... error handling ...

tui.Run(cfg, proc)
proc.Stop()
```

- [ ] **Step 5: Build and test manually**

```bash
cd daemon && go build -o daemon .
# Test with a real config:
# ./daemon
# Test headless:
# ./daemon --headless
```

- [ ] **Step 6: Commit**

```bash
git add daemon/
git commit -m "feat(daemon): interactive TUI with message panel, input, log tailing"
```

---

### Task 7: Wire Everything + Headless Mode

Complete `main.go` with all commands and flags.

**Files:**
- Modify: `daemon/main.go`

- [ ] **Step 1: Implement full main.go**

```go
// daemon/main.go
package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"lingtai-daemon/internal/agent"
	"lingtai-daemon/internal/config"
	"lingtai-daemon/internal/i18n"
	"lingtai-daemon/internal/manage"
	"lingtai-daemon/internal/setup"
	"lingtai-daemon/internal/tui"
)

func main() {
	args := os.Args[1:]
	configPath := "config.json"
	headless := false

	// Parse flags
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 < len(args) {
				configPath = args[i+1]
				i++
			}
		case "--headless":
			headless = true
		case "--lang":
			if i+1 < len(args) {
				i18n.Lang = args[i+1]
				i++
			}
		default:
			positional = append(positional, args[i])
		}
	}

	// Subcommands
	if len(positional) > 0 {
		switch positional[0] {
		case "setup":
			setup.Run()
			return
		case "manage":
			baseDir := "~/.lingtai"
			for i, arg := range args {
				if arg == "--base-dir" && i+1 < len(args) {
					baseDir = args[i+1]
				}
			}
			if strings.HasPrefix(baseDir, "~") {
				home, _ := os.UserHomeDir()
				baseDir = filepath.Join(home, baseDir[1:])
			}
			spirits := manage.ScanSpirits(baseDir)
			fmt.Print(manage.FormatTable(spirits))
			return
		}
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
		os.Exit(1)
	}

	// Start agent process
	proc, err := agent.Start(agent.StartOptions{
		ConfigPath: configPath,
		AgentPort:  cfg.AgentPort,
		WorkingDir: cfg.WorkingDir(),
		Headless:   headless,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
		os.Exit(1)
	}

	if headless {
		// Print meta and block
		printMeta(cfg, proc)
		fmt.Printf("  \033[2mLog: %s/daemon.log\033[0m\n", cfg.WorkingDir())
		fmt.Printf("  \033[2m%s\033[0m\n\n", i18n.S("press_ctrl_c"))

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		fmt.Printf("\n  %s\n", i18n.S("shutting_down"))
		proc.Stop()
	} else {
		// Interactive TUI
		tui.Run(cfg, proc)
		proc.Stop()
	}
}

func printMeta(cfg *config.Config, proc *agent.Process) {
	title := i18n.S("title")
	fmt.Printf("\n  \033[1m\033[36m%s\033[0m\n\n", title)
	fmt.Printf("  \033[1mAgent:\033[0m      %s\n", cfg.AgentName)
	fmt.Printf("  \033[1mWorking:\033[0m    %s\n", cfg.WorkingDir())
	fmt.Printf("  \033[1mPort:\033[0m       %d\n", cfg.AgentPort)
	fmt.Printf("  \033[1mPID:\033[0m        %d\n", proc.PID())

	if cfg.IMAP != nil {
		addr, _ := cfg.IMAP["email_address"].(string)
		fmt.Printf("  \033[1mIMAP:\033[0m       \033[32m● %s\033[0m\n", addr)
	} else {
		fmt.Printf("  \033[1mIMAP:\033[0m       \033[2mdisabled\033[0m\n")
	}

	if cfg.Telegram != nil {
		fmt.Printf("  \033[1mTelegram:\033[0m   \033[32m● enabled\033[0m\n")
	} else {
		fmt.Printf("  \033[1mTelegram:\033[0m   \033[2mdisabled\033[0m\n")
	}

	if cfg.CLI {
		fmt.Printf("  \033[1mCLI:\033[0m        \033[32m● interactive\033[0m\n")
	} else {
		fmt.Printf("  \033[1mCLI:\033[0m        \033[2mdisabled\033[0m\n")
	}
	fmt.Println()
}
```

- [ ] **Step 2: Build final binary**

```bash
cd daemon && go build -o daemon .
```

- [ ] **Step 3: Test all commands**

```bash
./daemon --help  # (or just runs default)
./daemon setup
./daemon manage
./daemon --headless --config ../config.example.json  # will fail without real config, but tests the flow
```

- [ ] **Step 4: Commit**

```bash
git add daemon/
git commit -m "feat(daemon): complete CLI with all commands and headless mode"
```

---

### Task 8: Integration Test + Polish

Final integration, manual testing, build verification.

- [ ] **Step 1: Run all Go tests**

```bash
cd daemon && go test ./... -v
```

- [ ] **Step 2: Build clean**

```bash
cd daemon && go build -o daemon .
ls -la daemon  # verify binary exists
```

- [ ] **Step 3: Test setup wizard end-to-end** (manual)

```bash
cd /tmp && /path/to/daemon setup
# Walk through wizard, verify config.json + model.json + .env written
```

- [ ] **Step 4: Test manage** (manual)

```bash
daemon manage
# Should show "No running spirits found."
```

- [ ] **Step 5: Commit final**

```bash
cd /path/to/lingtai
git add daemon/
git commit -m "feat(daemon): 器灵 complete — Go TUI for lingtai"
```
