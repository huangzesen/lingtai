---
name: lingtai-telegram-setup
description: Configure the Telegram bot addon for this agent — read this when the human asks to set up a Telegram bot.
version: 2.2.0
---

# Telegram Bot Setup

You are helping the human set up a Telegram bot for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## Fixed-by-Convention Path

**The Telegram config file lives at a single fixed location, shared by all agents in this project:**

```
.lingtai/.addons/telegram/config.json   (relative to project root)
```

- **Do not try to change this path.** The TUI and the kernel both expect it exactly here.
- The file is shared across all agents in the same project.
- Telegram supports one bot per config file. If you need multiple bots per project, that's currently not supported by a single shared config — ask the human whether they really need this before proceeding.
- From your agent's working directory, the relative path written in `init.json` is `../.addons/telegram/config.json`.

## Migration Check (IMPORTANT — do this first)

Before creating or editing the config file, **read your own `init.json`** and check `addons.telegram.config`. If the path points to the old global location (`~/.lingtai-tui/addons/...`), you MUST fix it:

1. Read your `init.json`
2. If `addons.telegram.config` contains `~/.lingtai-tui/` or an absolute path, replace it with:
   ```
   ../.addons/telegram/config.json
   ```
3. If the `addons` field is missing entirely and Telegram is not wired up, add it:
   ```json
   "addons": {
     "telegram": {
       "config": "../.addons/telegram/config.json"
     }
   }
   ```

This is a one-time fix for agents created before the local-config convention. New agents already have the correct relative path.

## Rules

- **Secrets go in .env, never in config JSON.** Read your `init.json` to find the `env_file` field, then append the bot token there. Reference it from `config.json` via `bot_token_env`.
- **Activation:** after creating or editing the config, run `system(action="refresh")` yourself to reload. Do not ask the human to refresh for you.
- **Troubleshooting:** if the addon fails to load, check that `.lingtai/.addons/telegram/config.json` exists, is valid JSON, and the referenced env vars are set in `.env`. Report back to the human with the specific problem.
- **Status caveat:** after refresh, addon status may show `connected: false` even when working. Always verify by attempting actual operation — if it succeeds, the connection is fine.

## What You Need From the Human

Ask the human for:
1. **Bot token** — from @BotFather on Telegram (`/newbot` → follow prompts → copy token)
2. **Allowed users** (optional) — Telegram user IDs allowed to message the bot. If omitted, anyone can message.
   - To find a user ID: have them message the bot first, the ID appears in the `from` field.

## What You Do

Once you have the bot token:

1. **Read your init.json** to find the `env_file` path. Then **append the token** to that .env file:
   ```
   TELEGRAM_BOT_TOKEN=<the token they gave you>
   ```

2. **Create the config file** at `.lingtai/.addons/telegram/config.json` relative to the project root:
   ```json
   {
     "bot_token_env": "TELEGRAM_BOT_TOKEN",
     "allowed_users": [123456789],
     "poll_interval": 1.0
   }
   ```
   - If no allowed_users requested, omit the field entirely (open access).

3. **Activate:** run `system(action="refresh")` to reload the addon config. Then verify the bot is responding. Tell the human Telegram is configured.

## Config Reference

See the example config at `.lingtai/.skills/lingtai-telegram-setup/assets/config.json` for a full reference of all available fields.
