# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is 灵台

灵台 (Língtái) is a generic agent framework — an "agent operating system" providing the minimal kernel for AI agents: thinking (LLM), perceiving (vision, search), acting (file I/O), and communicating (inter-agent email). Domain tools, coordination, and orchestration are plugged in from outside via MCP-compatible interfaces.

Named after 灵台方寸山 — where 孙悟空 learned his 72 transformations. Each agent (器灵) can spawn avatars (分身) that venture into 三千世界 and return with experiences. The self-growing network of avatars IS the agent itself — memory becomes infinite through multiplication.

### Repository Scope

All Python code (kernel runtime and batteries-included wrapper) now lives in the `lingtai-kernel` repo and is published as the `lingtai` package on PyPI. This repo contains only the Go TUI and portal frontends.

- **`tui/`** — Terminal UI (Go + Bubble Tea). Launches and monitors agents from the command line. Builds to `tui/bin/lingtai-tui`.
- **`portal/`** — Web portal (Go + embedded web frontend). Provides a browser-based interface. Builds to `portal/bin/lingtai-portal`.

Neither binary has a direct Python dependency. Both communicate with Python agents exclusively through the filesystem (`.lingtai/` directory, heartbeat files, signal files). Agents are launched by the TUI via `python -m lingtai run <dir>` as a subprocess.

## Build

```bash
# Build the TUI
cd tui && make build
# Output: tui/bin/lingtai-tui

# Build the portal (builds embedded web frontend first)
cd portal && make build
# Output: portal/bin/lingtai-portal
```

Cross-compilation targets (darwin/linux/windows, amd64/arm64) are available via `make cross-compile` in each directory.

## Projects

### TUI (`tui/`)

Go + Bubble Tea terminal interface. Key facts:

- Binary name: `lingtai-tui` (never `lingtai` — that is the Python agent CLI)
- Launches agents via `python -m lingtai run <dir>` subprocess
- Communicates with running agents via filesystem only: reads `.lingtai/` metadata, heartbeat files, and signal files inside each agent working directory
- Agent discovery uses `lingtai_kernel.handshake` conventions (`is_agent`, `is_alive` checks on working directories)

### Migrations (`tui/internal/migrate/`)

Versioned, append-only, forward-only migration system. Each migration is a file `m<NNN>_<name>.go` exporting a function `func migrate<Name>(lingtaiDir string) error`. Register it in `migrate.go` by appending to the `migrations` slice and bumping `CurrentVersion`. Migrations run once per project at TUI launch (version tracked in `.lingtai/meta.json`). They can read global state (`globalTUIDir()` helper) but receive the project's `.lingtai/` dir as input. Print warnings directly with `fmt.Println` — no i18n needed since migrations run before the TUI renders.

**IMPORTANT: The TUI and portal share the same `meta.json` version space but have separate migration registries.** When adding migrations to the TUI, you MUST also bump `CurrentVersion` in `portal/internal/migrate/migrate.go` and register no-op stubs for TUI-specific migrations. Otherwise the portal refuses to open any project the TUI has already touched.

### Portal (`portal/`)

Go server with an embedded web frontend. Key facts:

- Binary name: `lingtai-portal`
- Web assets are built with `npm run build` inside `portal/web/` and embedded into the Go binary at compile time via `embed.go`
- Communicates with agents via filesystem only (same conventions as TUI)
- `make build` runs the full pipeline: web deps → web build → go build
