# Daemon 器灵 v2 Improvements

Post-v1 improvements for the Go TUI. These assume v1 is shipped and working.

---

## 1. Verbose Mode (Ctrl+O)

Toggle rendering of the daemon's JSONL log in the TUI.

- **Off (default):** TUI is a clean chat client — only TCP mail send/receive
- **On (Ctrl+O):** TUI reads `{working_dir}/logs/events.jsonl` and renders events inline (diary, tool calls, thinking, channel activity)
- **No background tailer.** On toggle-on, read the JSONL file. On toggle-off, stop rendering. The file is the single source of truth — no in-memory buffer, no caching.
- Status bar shows `verbose ●` when on
- Remove `logtail.go` background goroutine from v1 — replace with on-demand file read

## 2. Daemon Switching

Switch which daemon the TUI talks to, without restarting.

- **Tab** or `/connect <name|port>` to switch target daemon
- `/list` shows running spirits (reuse `manage.ScanSpirits`)
- Implementation: update `MailClient.address` to the new port. New connection per message means no reconnect needed — just change the target.
- Status bar shows which daemon is active
- When switching, verbose mode (if on) switches to the new daemon's JSONL file

## 3. Verbose Mode — Lightweight JSONL Rendering

When Ctrl+O is toggled on:
1. Read `{working_dir}/logs/events.jsonl` from disk
2. Parse each line, render with colors (same scheme as v1 styles)
3. Show in the viewport above the chat messages (or interleaved by timestamp)
4. On subsequent toggle-on, re-read from where we last stopped (track byte offset)
5. While verbose is on, periodically check for new lines (1s interval) — simple `Seek` + `Read`, no goroutine

## 4. TUI Simplification

Remove from v1 plan:
- `internal/tui/logtail.go` — no background log tailer needed
- JSONL event schema table in spec — verbose mode reads raw JSONL, renders by `type` field
- Log event channel in TUI model — not needed without background tailer

The TUI in v1 is purely a TCP mail chat client. Verbose mode is a v2 feature.
