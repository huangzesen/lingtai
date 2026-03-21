# 灵台 App — Autonomous Orchestrator CLI

**Date:** 2026-03-20
**Status:** Approved

## Goal

A single CLI command (`lingtai`) that launches an autonomous orchestrator agent. Fire and forget. This is THE product shipped with lingtai — the canonical way to run an agent.

## Launch

```bash
lingtai                         # launch with ./config.json
lingtai /path/to/config.json    # explicit config path
lingtai send "do something"     # send message to running instance via CLI channel
```

## Config: `config.json`

Agent configuration. All channel sections are optional — omit to disable.

```json
{
  "model": "model.json",

  "imap": {
    "email_address": "agent@gmail.com",
    "email_password_env": "GMAIL_APP_PASSWORD",
    "allowed_senders": ["you@gmail.com"],
    "imap_host": "imap.gmail.com",
    "smtp_host": "smtp.gmail.com"
  },

  "telegram": {
    "bot_token_env": "TELEGRAM_BOT_TOKEN",
    "allowed_users": [123456]
  },

  "cli": true,

  "agent_name": "orchestrator",
  "base_dir": "~/.lingtai",
  "bash_policy": "policy.json",
  "max_turns": 50,
  "agent_port": 8501
}
```

### Field Reference

| Field | Default | Description |
|-------|---------|-------------|
| `model` | required | Path to `model.json` (must end in `.json`) or inline object (same schema) |
| `imap` | `null` (disabled) | IMAP addon config. See IMAP addon spec. |
| `telegram` | `null` (disabled) | Telegram addon config. |
| `cli` | `false` | `true` = interactive chat mode. `false`/omitted = fire-and-forget. |
| `agent_name` | `"orchestrator"` | Agent name (determines working directory) |
| `base_dir` | `"~/.lingtai"` | Base directory for agent working dirs |
| `bash_policy` | `null` | Path to bash policy JSON. `null` = no bash capability. |
| `max_turns` | `50` | Max consecutive LLM turns before yielding |
| `agent_port` | `8501` | TCP port for inter-agent mail |
| `covenant` | `null` | Custom covenant text. `null` = uses default covenant (see below). |

### Env Var Resolution Pattern

All `*_env` fields across config are resolved by `config.py` at startup. The pattern is universal:

- `api_key_env: "MINIMAX_API_KEY"` → resolved to the env var's value
- `email_password_env: "GMAIL_APP_PASSWORD"` → resolved to the env var's value
- `bot_token_env: "TELEGRAM_BOT_TOKEN"` → resolved to the env var's value

Resolved values are passed to the framework using the non-`_env` key names (e.g., `email_password`, `bot_token`, `api_key`). If the env var is not set, startup fails with a clear error.

`config.py` also loads `.env` from the config file's directory if present.

### Model Field Disambiguation

The `model` field in `config.json` is either:
- A **string ending in `.json`** → file path (relative to config.json directory). Loaded as model config.
- An **inline object** `{"provider": "minimax", ...}` → same schema as model.json, embedded directly.

No ambiguity: strings always end in `.json`, objects are always inline config.

## Config: `model.json`

LLM provider configuration. Separated from agent config because LLM setup is complex (multimodal, multiple providers).

```json
{
  "provider": "minimax",
  "model": "MiniMax-M2.7-highspeed",
  "api_key_env": "MINIMAX_API_KEY",

  "vision": {
    "provider": "openai",
    "model": "gpt-4o",
    "api_key_env": "OPENAI_API_KEY"
  },

  "web_search": {
    "provider": "gemini",
    "model": "gemini-2.0-flash",
    "api_key_env": "GEMINI_API_KEY"
  }
}
```

### Field Reference

Top-level fields are the primary LLM (thinking/reasoning).

| Field | Default | Description |
|-------|---------|-------------|
| `provider` | `"minimax"` | LLM provider name |
| `model` | `"MiniMax-M2.7-highspeed"` | Model identifier |
| `api_key_env` | required | Environment variable name containing the API key |
| `base_url` | `null` | Custom API base URL (for proxies, custom endpoints) |
| `vision` | `null` | Optional vision provider config (same fields: provider, model, api_key_env) |
| `web_search` | `null` | Optional web search provider config (same fields) |

**No raw API keys.** Only `api_key_env` — the env var name is resolved at startup.

If `vision`/`web_search` are omitted, the primary provider handles them (if it supports multimodal/grounding). If the primary provider doesn't support them, those capabilities are simply unavailable.

### How model.json Maps to LLMService

`config.py` translates the model config into `LLMService` constructor args:

```python
# Primary LLM
api_key = os.environ[model_cfg["api_key_env"]]
llm = LLMService(
    provider=model_cfg["provider"],
    model=model_cfg["model"],
    api_key=api_key,
    base_url=model_cfg.get("base_url"),
    provider_defaults=provider_defaults,  # see below
)

# Vision/web_search → provider_defaults
provider_defaults = {}
if "vision" in model_cfg:
    v = model_cfg["vision"]
    provider_defaults["vision_provider"] = v["provider"]
    # Vision provider's API key is resolved and injected via key_resolver
if "web_search" in model_cfg:
    ws = model_cfg["web_search"]
    provider_defaults["web_search_provider"] = ws["provider"]
```

Vision and web_search providers are registered with `LLMService` via `provider_defaults`. Their API keys are resolved from env vars and made available through the service's `key_resolver` callback. The exact wiring depends on how `LLMService` resolves secondary provider keys — `config.py` builds a `key_resolver` function that maps provider names to their resolved keys.

## Architecture

### Capabilities (always enabled)

| Capability | Notes |
|-----------|-------|
| `file` | read, write, edit, glob, grep |
| `psyche` | evolving identity, knowledge library, memory |
| `avatar` | spawn sub-agents — orchestrator decides their capabilities |
| `email` | inter-agent communication |

### Capabilities (conditional)

| Capability | Condition |
|-----------|-----------|
| `bash` | Enabled only if `bash_policy` is set in config |
| `web_search` | Enabled if primary or dedicated provider supports it |
| `vision` | Enabled if primary or dedicated provider supports it |

### Addons (conditional)

| Addon | Condition |
|-------|-----------|
| `imap` | Enabled if `imap` section present in config |
| `telegram` | Enabled if `telegram` section present in config |

### Communication Channels

Three optional channels to humans:

| Channel | Config | How it works |
|---------|--------|-------------|
| **IMAP** | `imap: {...}` | Real email via IMAP/SMTP. Gateway pattern — orchestrator redrafts before forwarding to internal agents. |
| **Telegram** | `telegram: {...}` | Telegram bot. Same gateway pattern. |
| **CLI** | `cli: true` | Interactive stdin/stdout via a local TCP mail listener. See CLI section. |

All three are peers — the orchestrator receives messages from any channel and responds via the same channel.

### Gateway Pattern

The orchestrator is the mail gateway between external world and internal agent network. External channels (IMAP, Telegram) never pipe raw content to internal agents. The orchestrator reads, understands, and redrafts messages before forwarding internally. This is behavioral (guided by covenant), not enforced by code.

## Launch Flow

```
lingtai config.json
  │
  ├── Load config.json
  ├── Load model.json (or inline model config)
  ├── Resolve all *_env fields → actual values
  ├── Validate config (fail fast with clear errors)
  │
  ├── Create LLMService (primary + vision/web_search via provider_defaults)
  ├── Create TCPMailService (inter-agent, on agent_port)
  ├── Build capabilities list (file, psyche, avatar, email + conditional)
  ├── Build addons dict (imap, telegram — if configured)
  │
  ├── Create Agent(orchestrator)
  ├── agent.start()
  │
  ├── Print meta (both modes):
  │     Agent:      orchestrator
  │     Working:    ~/.lingtai/orchestrator
  │     IMAP:       agent@gmail.com (or "disabled")
  │     Telegram:   @bot_name (or "disabled")
  │     CLI:        interactive (or "disabled")
  │     Ctrl+C to shut down.
  │
  ├── If cli=true:
  │     Start CLI listener (see below)
  │     Enter interactive loop (stdin → email, responses → stdout)
  ├── Else:
  │     Block until SIGINT/SIGTERM
  │
  └── agent.stop()
```

## CLI Interactive Mode

### Mechanism

The CLI channel uses a local TCP mail listener to exchange messages with the orchestrator — the same inter-agent email mechanism used by all agents.

1. **CLI starts a `TCPMailService`** on a dedicated port (`agent_port + 1`, e.g., 8502). This gives the CLI a mail address: `cli@localhost:8502`.
2. **User types a message** → CLI sends it as inter-agent email `From: cli@localhost:8502 To: localhost:8501` (the orchestrator's port).
3. **Orchestrator receives it** like any other inter-agent email. The `From:` address tells it to reply to the CLI.
4. **Orchestrator replies** via `email(action="send", address="localhost:8502")` → delivered to CLI's TCP listener.
5. **CLI prints the response** to stdout.

This means the CLI is a real email peer — no special plumbing. The orchestrator uses the same `email` tool it uses for all inter-agent communication.

### User Experience

```
  Agent:      orchestrator
  Working:    /Users/you/.lingtai/orchestrator
  IMAP:       agent@gmail.com
  Telegram:   disabled
  CLI:        interactive (localhost:8502)
  Ctrl+C to shut down.

> What's in my inbox?
[orchestrator] You have 3 unread emails:
  1. From alice@example.com — "Meeting tomorrow"
  2. From bob@example.com — "Project update"
  3. From newsletter@example.com — "Weekly digest"
> Summarize the first one
[orchestrator] The meeting email from Alice says...
```

### `lingtai send`

For non-interactive mode, `lingtai send "message"` is a one-shot fire-and-forget:

1. Reads `config.json` to find `agent_port`.
2. Sends a TCP mail to `localhost:{agent_port}` with `From: cli@local` (no reply listener).
3. Exits immediately.

The orchestrator processes the request and acts via its configured channels (IMAP, Telegram). There is no response path back to `lingtai send` — it is not a channel, just a trigger.

```bash
lingtai send "Check my inbox and reply to anything urgent"
```

## File Structure

```
app/
├── __init__.py        — main() entry point, launch flow
├── __main__.py        — calls main() for `python -m app` invocation
├── config.py          — config loading, model loading, env var resolution, validation
└── cli.py             — interactive CLI loop (TCPMailService listener + stdin/stdout)
```

Plus `pyproject.toml` console_scripts entry: `lingtai = "app:main"`

Example configs shipped alongside:
```
config.example.json
model.example.json
```

## Covenant (default)

The orchestrator's default covenant guides its behavior:

```
## Communication
- You have multiple communication channels. Use the same channel to reply.
- When you receive an imap email, reply via imap.
- When you receive a telegram message, reply via telegram.
- When you receive a CLI message, reply via the CLI channel (email).
- Your text responses are your private diary.
- Keep messages concise and helpful.
- Never go back and forth with courtesy messages.

## Gateway
- You are the gateway between the external world and your internal agents.
- When forwarding external messages to internal agents, always redraft them.
- Never pipe raw external content into the internal agent network.

## Initiative
- Regularly check your communication channels for new messages.
- When idle, check if anything needs attention.
```

Overridable via `covenant` field in config.json.

## What We're NOT Building

- Web UI (exists separately in `app/web`)
- Multi-orchestrator coordination
- Config wizard or auto-setup
- Built-in scheduling (agents have their own initiative via psyche)
- Authentication/authorization (handled by channel-level allowed_senders/allowed_users)

## Testing Strategy

- Config loading: valid, missing fields, inline vs file model, env var resolution, `.env` loading
- Launch flow: mock Agent/LLMService, verify capabilities/addons wired correctly based on config
- CLI mode: mock TCPMailService, verify message send/receive flow
- `lingtai send`: mock TCP send, verify one-shot delivery
- Integration: manual test with real config
