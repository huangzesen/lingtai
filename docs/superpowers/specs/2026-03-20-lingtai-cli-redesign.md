# Lingtai CLI Redesign — Spec

## Problem

The current CLI has scattered concerns: global config at `~/.lingtai/`, subcommands for start/stop/list, separate Python entry point. It doesn't match how users actually work — in project directories.

## Design

### Core Principle

`lingtai` works like `git` — run it in any directory. The directory IS the project.

### Directory Layout

```
~/.lingtai/
  combos/
    gemini-pro.json          ← named combo (model + config + env)
    claude-work.json

~/Documents/EB1A-prep/       ← user's actual work folder
  .lingtai/
    configs/
      config.json            ← project config
      model.json             ← LLM provider config
      .env                   ← secrets
      bash_policy.json       ← bash capability policy
    <agent_id>/              ← 本我 working dir
      covenant.md            ← per-agent covenant
      system/
      mailbox/
      delegates/
      logs/
    <agent_id>/              ← 他我 working dir
      covenant.md
      ...
  user-files/                ← user's actual project files
```

### Combos

A combo is a named snapshot of the user's provider/model/key configuration. Stored at `~/.lingtai/combos/<name>.json`.

```json
{
  "name": "gemini-pro",
  "model": {
    "provider": "gemini",
    "model": "gemini-2.5-pro",
    "api_key_env": "GEMINI_API_KEY",
    "vision": { "provider": "gemini", "api_key_env": "GEMINI_API_KEY" },
    "web_search": { "provider": "gemini", "api_key_env": "GEMINI_API_KEY" }
  },
  "config": {
    "agent_name": "orchestrator",
    "agent_port": 8501,
    "language": "lzh",
    "max_turns": 50
  },
  "env": {
    "GEMINI_API_KEY": "sk-..."
  }
}
```

The `env` section stores actual secrets so the user never re-enters keys. This file should be `chmod 600`.

### CLI Flow

```
lingtai              → .lingtai/ exists?
                        YES → start agent, open chat TUI
                        NO  → setup wizard → creates .lingtai/ → start agent, open chat TUI

lingtai setup        → (re)run setup wizard in current directory
```

That's it. Two commands.

### Setup Wizard Flow

```
Step 1: Combo Selection
  ┌──────────────────────────────────────┐
  │  Use existing combo:                 │
  │  > gemini-pro (gemini/gemini-2.5-pro)│
  │    claude-work (anthropic/claude-4.6)│
  │    [Create new]                      │
  └──────────────────────────────────────┘

  If combo selected → skip to Step 4 (pre-filled from combo)
  If "Create new"   → continue to Step 2

Step 2: Language
  en / 中文 / 文言

Step 3: Model Configuration
  Provider, model, API key, endpoint
  + Multimodal capabilities (vision, web_search, talk, compose, draw, listen)

Step 4: General
  Agent name (default: orchestrator)
  Agent port (default: 8501)

Step 5: Messaging (optional)
  IMAP config
  Telegram config

Step 6: Review & Confirm
  Shows all settings
  "Name this combo:" (for saving to ~/.lingtai/combos/)
  Confirm → writes files

Step 7: (if combo selected in Step 1)
  "Update this combo with any changes? [Y/n]"
```

### What Gets Written

On setup:
1. `.lingtai/configs/config.json` — project config (agent_name, port, language, etc.)
2. `.lingtai/configs/model.json` — LLM provider config
3. `.lingtai/configs/.env` — secrets
4. `.lingtai/configs/bash_policy.json` — default bash policy
5. `.lingtai/<agent_name>/covenant.md` — default covenant (per language)
6. `~/.lingtai/combos/<name>.json` — combo snapshot (created or updated)

### Chat TUI

When `.lingtai/` exists and `lingtai` is run:

1. Load config from `.lingtai/configs/config.json`
2. Start the Python agent process (`python -m app .lingtai/configs/config.json`)
3. Show chat TUI — user types messages, sees agent responses
4. Future: toggle to topology view, agent management, etc.

### Config Resolution

The Python `app/__init__.py` derives `base_dir` from config path:
```
.lingtai/configs/config.json → parent → .lingtai/configs/ → parent → .lingtai/
```

So `.lingtai/` is the base_dir. Agent working dirs are `.lingtai/<agent_name>/`.

`config.json` does NOT contain `base_dir` — it's derived from the file's location.

### Language Propagation

`config.json` has a `"language"` field. This flows to:
- `AgentConfig.language` in Python
- All i18n strings via `t(lang, key)`
- Tool schema descriptions
- Default covenant selection
- Tool translation table (appended to covenant for zh/lzh)

### Covenant per Agent

- Covenant is per-agent, lives in the agent's working dir
- Default covenant written at setup time (language-appropriate)
- Avatar spawning copies parent's covenant unless overridden
- The agent reads its own covenant from its working dir at startup

### What `~/.lingtai/` Does NOT Do

- No project scanning
- No global agent registry
- No global config.json
- Just stores combos (named provider/key bundles)
