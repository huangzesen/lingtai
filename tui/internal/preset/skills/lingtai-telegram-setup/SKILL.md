---
name: lingtai-telegram-setup
description: Configure the Telegram bot addon for this agent — read this when the human asks to set up a Telegram bot.
version: 1.0.0
---

# Telegram Bot Setup

You are helping the human set up a Telegram bot for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## Rules

- **Secrets go in .env, never in config JSON.** Read your `init.json` to find the `env_file` field, then append the bot token there.
- **Config files go under** `{agentDir}/addons/telegram/<bot_name>/config.json` — one directory per bot. Never put configs in the agent's root directory.
- **Activation:** after creating the config, tell the human to run `/refresh` in the TUI. You cannot activate addons yourself.

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

2. **Create the config file** at `{agentDir}/addons/telegram/<bot_name>/config.json`:
   ```json
   {
     "bot_token_env": "TELEGRAM_BOT_TOKEN",
     "allowed_users": [123456789],
     "poll_interval": 1.0
   }
   ```
   - If no allowed_users requested, omit the field entirely (open access).

3. **Tell the human** the config is ready and ask them to run `/refresh` in the TUI to activate.

## Config Reference

See the example config at `skills/lingtai-telegram-setup/assets/config.json` for a full reference of all available fields.
