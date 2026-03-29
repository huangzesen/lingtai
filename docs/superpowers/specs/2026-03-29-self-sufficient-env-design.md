# Self-Sufficient Agent Environment

**Date:** 2026-03-29
**Status:** Approved

## Problem

Agents depend on the TUI injecting API keys via environment variables at process launch time. The TUI's `LaunchAgent()` reads `~/.lingtai-tui/config.json`, maps provider names to env var names (`minimax` -> `MINIMAX_API_KEY`), and sets them on the subprocess. The agent's `init.json` references these via `api_key_env`.

This breaks when the agent is not launched by the TUI:
- **Self-refresh**: the deferred relaunch subprocess has no TUI to inject keys
- **Terminal launch**: `lingtai run <dir>` from a shell has no keys
- **Avatars**: spawned agents may lack keys if parent doesn't propagate env

The agent is not self-sufficient. It cannot restart itself.

## Design

Three changes across TUI and kernel.

### 1. TUI: Write `.env` file and reference it in init.json

**Location:** `~/.lingtai-tui/.env` (one global file, all agents share it)

**Format:**
```
MINIMAX_API_KEY=sk-xxx
LLM_API_KEY=sk-yyy
```

**When written:** Every time `SaveConfig()` is called (user sets up or updates keys). A new function `WriteEnvFile(globalDir string, cfg Config)` maps provider keys to env var names and writes the file.

**init.json:** Both `GenerateInitJSONWithOpts()` and `GenerateTutorialInit()` set `"env_file"` to the absolute path of `~/.lingtai-tui/.env`.

**LaunchAgent():** Remove the env var injection code (lines 83-95 in `launcher.go`). The agent loads its own keys via `env_file`. No more TUI babysitting.

### 2. Kernel: Enforce env_file when api_key_env is used

**Location:** `validate_init()` in `init_schema.py`

**Rule:** If `manifest.llm.api_key_env` is set and `manifest.llm.api_key` is null/empty, then top-level `env_file` must be present and non-empty.

**Error message:** `"llm.api_key_env is set but no env_file provided — the agent cannot resolve the API key without it"`

This catches misconfigured agents at boot, not at first LLM call.

### 3. Kernel: Fix refresh tool result message

**Location:** i18n files (`en.json`, `zh.json`, `wen.json`)

**`system_tool.refresh_message`** (returned to agent as tool result):
- en: `"Refresh initiated — you will be suspended and relaunched momentarily. Do not make any more tool calls."`
- zh: `"刷新已启动——你将被挂起并立即重新启动。请勿再进行任何工具调用。"`
- wen: `"更衣已启——汝将假死而重生。勿再调器。"`

**`system.refresh_successful`** (injected as [system] message after relaunch) — already exists, no changes needed.

## Files Changed

| File | Change |
|------|--------|
| `tui/internal/config/global.go` | Add `WriteEnvFile()` function |
| `tui/internal/config/global.go` | Call `WriteEnvFile()` from `SaveConfig()` |
| `tui/internal/preset/preset.go` | Set `env_file` in `GenerateInitJSONWithOpts()` and `GenerateTutorialInit()` |
| `tui/internal/process/launcher.go` | Remove env var injection from `LaunchAgent()` |
| `lingtai-kernel: init_schema.py` | Add `api_key_env` requires `env_file` validation |
| `lingtai-kernel: i18n/en.json` | Update `system_tool.refresh_message` |
| `lingtai-kernel: i18n/zh.json` | Update `system_tool.refresh_message` |
| `lingtai-kernel: i18n/wen.json` | Update `system_tool.refresh_message` |

## What This Achieves

- Agent is fully self-sufficient: `lingtai run <dir>` works from anywhere
- Refresh works: deferred relaunch needs no env inheritance
- Avatars work: they inherit `env_file` from parent init.json
- Validation catches misconfigured agents before they crash
- Single source of truth for API keys: `~/.lingtai-tui/.env`
