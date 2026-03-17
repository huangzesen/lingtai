# Orchestration Example — Design Spec

## Overview

An example demonstrating multi-agent orchestration via stoai. One admin orchestrator agent runs as a background service, capable of spawning up to 10 subagents. A separate CLI email client lets the user communicate with the orchestrator. All communication (user ↔ admin, admin ↔ subagents, subagent ↔ subagent) flows through email.

## File Structure

### Git-tracked (in repo)

```
examples/orchestration/
    __main__.py      — background service: starts orchestrator agent
    cli.py           — email client: /send, /inbox, /read, /sent, /quit
```

### Runtime (not git-tracked)

```
~/.stoai/orchestration/playground/
    service.json             — pid, admin port/address, status
    admin/                   — orchestrator working dir (mailbox/, logs/, etc.)
    admin_delegate_*/        — subagent working dirs (created by delegate)
    user/                    — user mailbox dir (TCPMailService writes here)
```

## Service (`__main__.py`)

Entry point that starts the orchestrator agent as a long-running background service.

### LLM

- Provider: MiniMax (multimodal)
- Model: `MiniMax-M2.5-highspeed`
- Provider config: `web_search_provider: minimax`, `vision_provider: minimax`
- API key from `MINIMAX_API_KEY` env var (loaded from `.env` if present)

### Orchestrator Agent

- **agent_id:** `"admin"`
- **admin:** `True`
- **base_dir:** `~/.stoai/orchestration/playground/` (must be created before `Agent()` constructor — `mkdir -p`)
- **streaming:** `True`
- **config:** `AgentConfig(max_turns=20)`
- **role:** covenant text (see below)
- **capabilities:** `email`, `bash` (with policy file), `file`, `web_search`, `vision`, `anima`, `conscience` (interval=300), `delegate`

### Covenant

Passed as `role=` constructor parameter. The orchestrator's covenant establishes it as an admin that:

- Can delegate tasks to up to 10 subagents
- Generates covenants for each subagent it spawns (tailored to the subagent's mission)
- When delegating, must explicitly pass `capabilities=["email", "bash", "file", "web_search", "vision", "anima"]` to exclude conscience and delegate from subagents (only `delegate` is auto-excluded by code; conscience exclusion depends on the covenant instructing the LLM to pass an explicit capabilities list)
- Broadcasts all peer addresses to all subagents whenever a new one is spawned
- Communicates exclusively via email
- Has conscience (inner voice) active — stays proactive

Subagents receive capabilities: email, bash, file, web_search, vision, anima. No conscience, no delegate (no recursive spawning). The orchestrator generates their covenants.

### service.json

Written on startup, read by CLI:

```json
{
    "pid": 12345,
    "admin_address": "127.0.0.1:8301",
    "user_port": 8300,
    "status": "running",
    "started_at": "2026-03-17T10:00:00Z"
}
```

### Lifecycle

- Loads `.env` for API key
- Creates playground dir: `Path("~/.stoai/orchestration/playground/").expanduser().mkdir(parents=True, exist_ok=True)` — must happen before `Agent()` constructor (which raises `FileNotFoundError` if `base_dir` doesn't exist)
- Starts `TCPMailService` on a known port (e.g., 8301)
- Constructs and starts the orchestrator `Agent`
- Writes `service.json`
- Blocks until SIGINT/SIGTERM
- On shutdown: stops agent, updates `service.json` status to `"stopped"`

## CLI Email Client (`cli.py`)

A reusable terminal email client. Connects to a running orchestrator service.

### Connection

- Reads `service.json` from `~/.stoai/orchestration/playground/` to find admin address and user port
- Starts a `TCPMailService` on the user port (from `service.json`) with `working_dir=playground/user/` — the `working_dir` parameter is required so received emails are persisted to disk and `/inbox`/`/read` commands work
- User mailbox persists at `~/.stoai/orchestration/playground/user/mailbox/`
- Only one CLI instance can run at a time (the user port is fixed in `service.json`; a second instance fails to bind)

### Commands

| Command | Description |
|---------|-------------|
| `/send <message>` | Send email to the orchestrator |
| `/inbox` | List recent inbox emails (from, subject, time, unread status) |
| `/read <id>` | Read full email by ID. Print message text. Print path to email dir (for attachments) |
| `/sent` | List sent emails |
| `/quit` | Exit CLI. Agent keeps running. |

### Behavior

- On startup: print connection info, show unread count if any
- `/send`: constructs email payload with `from=user_address`, sends via `TCPMailService.send()` to admin address
- `/inbox` and `/sent`: read from on-disk mailbox at `playground/user/mailbox/{inbox,sent}/`
- `/read <id>`: load `message.json` from mailbox, print message text, if attachments exist print the path to `mailbox/inbox/<id>/attachments/`
- Unknown input (no `/` prefix): treat as `/send` (convenience — just type a message to send it)
- `/quit`: stop user's `TCPMailService`, exit process

### Reusability

`cli.py` is a generic email client. Future examples can reuse it by pointing at a different service.json path or target address. The core logic (command parsing, mailbox reading, send/receive) is not specific to orchestration.

## Attachments

Already supported by `TCPMailService` — files are base64-encoded over TCP, decoded and saved to `mailbox/inbox/{id}/attachments/`. The `/read` command shows the path to this directory when attachments are present.

## Bash Policy

Uses `examples/bash_policy.json` (already exists in repo) for the bash capability.

## Usage

```bash
# Terminal 1 — start the orchestrator service
python -m examples.orchestration

# Terminal 2 — connect the email client
python -m examples.orchestration.cli

# In the CLI:
/send Research quantum computing and summarize the top 3 breakthroughs of 2025
/inbox
/read 3a8f2b
/sent
/quit
```
