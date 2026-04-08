---
name: lingtai-telegram-setup
description: Configure the Telegram bot addon for this agent — read this when the human asks to set up a Telegram bot.
version: 2.0.0
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
- From your agent's working directory, the relative path written in `init.json` is `../.addons/telegram/config.json`. You should not need to edit `init.json` — it was pre-populated when the agent was created.

## Rules

- **Secrets go in .env, never in config JSON.** Read your `init.json` to find the `env_file` field, then append the bot token there. Reference it from `config.json` via `bot_token_env`.
- **Activation:** after creating or editing the config, tell the human to run `/refresh` in the TUI. You cannot activate addons yourself.
- **Troubleshooting:** if the addon fails to load, check that `.lingtai/.addons/telegram/config.json` exists, is valid JSON, and the referenced env vars are set in `.env`. Report back to the human with the specific problem.

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

3. **Tell the human** the config is ready at `.lingtai/.addons/telegram/config.json` and ask them to run `/refresh` in the TUI to activate.

## Config Reference

See the example config at `skills/lingtai-telegram-setup/assets/config.json` for a full reference of all available fields.
