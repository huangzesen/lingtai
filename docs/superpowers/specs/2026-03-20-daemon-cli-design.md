# Daemon 器灵 — Go TUI for 灵台

**Date:** 2026-03-20
**Status:** Approved

## Goal

A Go CLI binary (`daemon`) that IS the lingtai product. Beautiful terminal experience via Bubble Tea. Manages Python agent processes, provides interactive TUI, setup wizard, and spirit management.

## Commands

```bash
daemon                              # interactive TUI (default config.json)
daemon --config researcher.json     # interactive TUI with specific config
daemon --headless                   # background mode, no TUI (PID file, block on signal)
daemon setup                        # guided setup wizard (writes to CWD)
daemon manage                       # list running spirits (default ~/.lingtai)
daemon manage --base-dir /path      # list spirits in specific base dir
```

## Architecture

```
daemon/                     ← Go module (subdirectory of lingtai repo)
  go.mod
  go.sum
  main.go                  ← CLI entry point
  internal/
    tui/                   ← Bubble Tea interactive mode
      app.go               ← main TUI model
      styles.go            ← lipgloss styles
    setup/                 ← Bubble Tea setup wizard
      wizard.go            ← stepped form
      tests.go             ← connection testers (IMAP, SMTP, Telegram)
    manage/                ← spirit manager
      list.go              ← scan PID files, render table
    agent/                 ← Python process lifecycle
      process.go           ← start/stop/monitor subprocess
      mail.go              ← TCP mail client (JSON over TCP)
    config/                ← config file I/O
      loader.go            ← load/write config.json, model.json, .env
```

### Dependencies

- `github.com/charmbracelet/bubbletea` — TUI framework
- `github.com/charmbracelet/lipgloss` — styling
- `github.com/charmbracelet/bubbles` — common components (textinput, viewport, spinner)

No other external dependencies. IMAP/SMTP/HTTP testing uses Go stdlib.

## How It Works

### Interactive Mode (`daemon`)

1. Load config.json from current directory (or `--config` path).
2. Spawn `python -m app` as subprocess with `cli: false` forced in config (Go handles the human interface, not Python).
3. Open TCP mail connection to the agent on `agent_port`.
4. Write PID file to `{base_dir}/{agent_name}/agent.pid`.
5. Render Bubble Tea TUI:
   - Status bar: channels enabled, agent name, port
   - Message panel: scrollable log of all agent activity (received mail, sent mail, tool calls, diary)
   - Input box: user types messages → sent as TCP mail to agent
6. Agent replies via TCP mail → rendered in message panel.
7. Ctrl+C → SIGTERM to Python process → clean up PID file → exit.

### Headless Mode (`daemon --headless`)

1. Same subprocess spawn as interactive.
2. Print meta info to stdout (agent name, working dir, channels, PID).
3. Redirect Python subprocess stdout/stderr to `{base_dir}/{agent_name}/daemon.log` (shown in meta output).
4. Write PID file.
5. Block until SIGINT/SIGTERM.
6. Clean up PID file.

In interactive mode, Python stdout/stderr is suppressed (TUI owns the terminal). All agent activity is visible via the JSONL log tail.

Designed for `daemon --headless &` — run spirits in background.

### Setup Wizard (`daemon setup`)

Pure Go — no Python needed. Bubble Tea multi-step form:

1. **Model config** — provider, model name, API key env var. Test: check env var is set.
2. **IMAP** (optional) — email, password, hosts. Test: live IMAP login + SMTP auth.
3. **Telegram** (optional) — bot token. Test: `getMe` API call.
4. **General** — agent name, base dir, CLI toggle, port.
5. **Review** — show config preview.
6. **Write** — config.json, model.json, .env to **current working directory**.

Passwords/tokens go into `.env`, config files reference them via `*_env` keys.

User can skip any section. Ctrl+C exits without writing.

### Manage (`daemon manage`)

Scans `{base_dir}/*/agent.pid` for running spirits. Default `base_dir` is `~/.lingtai`. Override with `--base-dir`. For each:
1. Read PID file (JSON: pid, port, started timestamp).
2. Check if process is alive (`kill(pid, 0)` equivalent).
3. Render a colored table:

```
 NAME           PID     PORT   UPTIME      STATUS
 orchestrator   12345   8501   2h 15m      ● running
 researcher     12346   8502   45m         ● running
 writer         12347   8503   —           ✗ dead (stale PID)
```

Show hints: `kill <PID>` to stop, `daemon --config <path>` to start new.

## TUI Layout

```
┌─ 器灵 Daemon ──────────────────────────────────┐
│ ● IMAP: agent@gmail.com  ● Telegram: @mybot   │
│ ● Agent: orchestrator    ● Port: 8501          │
├────────────────────────────────────────────────┤
│                                                │
│ [imap ←] From alice@example.com: Meeting       │
│ [daemon] Checking your inbox...                │
│ [daemon] You have 3 unread emails.             │
│ [tg ←] User 123: Hello bot                    │
│ [daemon] Processing telegram message...        │
│ [tool] web_search                              │
│ [diary] I should prioritize the meeting email  │
│                                                │
├────────────────────────────────────────────────┤
│ > Type a message...                            │
└────────────────────────────────────────────────┘
```

### Message Colors

| Event | Color | Prefix |
|-------|-------|--------|
| IMAP received | Green | `[imap ←]` |
| IMAP sent | Yellow | `[imap →]` |
| Telegram received | Green | `[tg ←]` |
| Telegram sent | Yellow | `[tg →]` |
| Email received (inter-agent) | Cyan | `[email ←]` |
| Email sent (inter-agent) | Cyan | `[email →]` |
| Agent response to CLI | White/Bold | `[daemon]` |
| Tool call | Blue | `[tool]` |
| Diary/thinking | Dim | `[diary]` |

### Input

- Enter → send message to agent via TCP mail
- Ctrl+C → graceful shutdown
- Scroll up/down in message panel (mouse or keys)

## Communication Protocol

### Wire Protocol (TCP Mail)

lingtai's `TCPMailService` uses length-prefixed JSON over TCP. **Important:** the server sends a banner line on connect that the client must read first.

**Connection sequence:**
1. Go connects to `localhost:{agent_port}`
2. Python sends `STOAI {banner_id}\n` (a `\n`-terminated banner line)
3. Go reads and discards the banner line
4. Go sends `[4-byte big-endian length][JSON payload bytes]`
5. Python reads the length prefix, then reads that many bytes of JSON

**Go → Python Agent:**

```json
{"from": "cli@localhost:8502", "to": ["localhost:8501"], "subject": "", "message": "user's text"}
```

The agent discovers the Go TUI's listen address from the `"from"` field of incoming messages — no separate registration needed. The agent replies by sending to the address in `"from"`.

### Go Listening Port

Go listens on `cli_port` (configurable in config.json, default: `agent_port + 1`, e.g., 8502). Same TCP mail protocol in reverse — Go is the server, Python connects to send replies.

### Startup Handshake

Python takes several seconds to start (imports, agent construction, TCP bind). The Go binary must wait for the agent's TCP port to become available before connecting:

1. Spawn Python subprocess
2. Poll `agent_port` with exponential backoff (100ms → 200ms → 500ms → 1s → 2s, cap at 5s, timeout after 30s total)
3. Once TCP connect succeeds, proceed with PID file and TUI
4. If timeout exceeded, print error and exit

### Agent Events (Log File)

For non-mail events (diary, tool calls, thinking), the Go TUI tails the agent's JSONL log file at `{base_dir}/{agent_name}/logs/events.jsonl`. Each line is a JSON event.

**Complete JSONL event schema:**

| Event type | Required fields | Notes |
|-----------|----------------|-------|
| `diary` | `text` | Agent's internal monologue |
| `thinking` | `text` | LLM reasoning output |
| `imap_received` | `sender`, `subject` | Incoming IMAP email |
| `imap_sent` | `to`, `subject` | Outgoing IMAP email |
| `telegram_received` | `sender`, `subject` | Incoming Telegram message |
| `telegram_sent` | `to`, `subject` | Outgoing Telegram message |
| `email_received` | `sender`, `subject` | Inter-agent email received |
| `email_sent` | `to`, `subject` | Inter-agent email sent |
| `tool_call` | `tool_name` OR `name` | Tool invocation (check both field names) |

All events also have `type` and `timestamp` fields.

This gives the TUI full visibility into agent activity without requiring a special protocol — just tail the log.

## PID File Convention

Path: `{base_dir}/{agent_name}/agent.pid`

```json
{"pid": 12345, "port": 8501, "config": "/path/to/config.json", "started": "2026-03-20T12:00:00Z"}
```

Written on startup, deleted on clean shutdown. Stale PID files (process dead) shown as "dead" in `daemon manage`.

## Python Side Changes

Minimal changes to existing `app/`:
- The Go binary spawns `python -m app <config_path>` where the config has `"cli": false`.
- In headless mode, Python's `app/` `main()` runs the agent and blocks on signals (existing behavior when `cli: false`).
- The Go binary handles PID files, not Python. Python just runs the agent.
- The `TerminalLoggingService` in `app/__init__.py` writes to the JSONL log (already does this via `JSONLLoggingService`). The Go TUI reads this log.

## Config Files

Same schema as the existing Python app spec — config.json, model.json, .env. The setup wizard writes these. The Go binary reads config.json to know the agent_port and base_dir before spawning Python.

## i18n (Day 0)

The TUI title, status labels, and setup wizard prompts support i18n. Default: English. Chinese available.

Implementation: simple string map keyed by language code. Language detected from `LANG` env var or `--lang` flag. No external i18n library — just a Go map.

```go
var strings = map[string]map[string]string{
    "en": {"title": "Daemon", "setup_title": "Setup Wizard", ...},
    "zh": {"title": "器灵", "setup_title": "设置向导", ...},
}
```

## What We're NOT Building

- Web UI
- Multi-spirit orchestration (spirits are independent OS processes)
- Auto-update
- Remote management (manage only reads local PID files)
- Plugin system for the TUI

## Build & Install

```bash
cd daemon
go build -o daemon ./main.go
# or
go install ./...
```

The binary requires Python 3.11+ and `lingtai` installed (`pip install -e .` in the parent repo).

## Testing Strategy

- `config/loader_test.go` — config loading, .env parsing, validation
- `agent/process_test.go` — subprocess lifecycle (mock exec)
- `agent/mail_test.go` — TCP mail send/receive (mock TCP)
- `manage/list_test.go` — PID file scanning, liveness check
- `setup/tests_test.go` — IMAP/SMTP/Telegram connection testers (mock net)
- Integration: manual test with real agent
