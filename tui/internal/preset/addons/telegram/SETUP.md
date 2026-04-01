# Telegram Bot Setup

You are helping the human set up a Telegram bot for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## What You Need From the Human

Ask the human for:
1. **Bot token** — from @BotFather on Telegram (`/newbot` → follow prompts → copy token)
2. **Allowed users** (optional) — Telegram user IDs allowed to message the bot. If omitted, anyone can message.
   - To find a user ID: have them message the bot first, the ID appears in the `from` field.

## What You Do

Once you have the bot token:

1. **Add the token to the .env file** (the path is in your init.json under `env_file`):
   Append this line:
   ```
   TELEGRAM_BOT_TOKEN=<the token they gave you>
   ```

2. **Create the config file** at:
   ```
   ~/.lingtai-tui/addons/telegram/<bot_name>/config.json
   ```
   For example, if the bot is called `myagent_bot`, write to:
   `~/.lingtai-tui/addons/telegram/myagent_bot/config.json`

   Contents:
   ```json
   {
     "bot_token_env": "TELEGRAM_BOT_TOKEN",
     "allowed_users": [123456789],
     "poll_interval": 1.0
   }
   ```
   - If no allowed_users, omit the field entirely (open access).

3. **Tell the human** the config is ready. Give them the exact path and ask them to:
   - Type `/addon` in the TUI
   - Enter the config path (e.g., `~/.lingtai-tui/addons/telegram/myagent_bot/config.json`)
   - Then type `/refresh` to activate

## Reference
Template with comments: `~/.lingtai-tui/addons/telegram/example/config.json`
