# Telegram Bot Setup

You are helping the human set up a Telegram bot for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## Rules

- **Find the .env file path** by reading your `init.json` — look for the `env_file` field. Secrets go there, never in config JSON.
- **Config files go under** `~/.lingtai/.addons/telegram/<bot_name>/config.json` where `<bot_name>` is the bot's username. Each bot gets its own directory. Do NOT put config files in the agent's working directory.
- **Never edit the example template** at `~/.lingtai/.addons/telegram/example/config.json` — it is a reference, not a working config.
- **Activation requires the human** to type `/addon` in the TUI, enter the config path, then `/refresh`. You cannot do this yourself.

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

2. **Create the config file** at `~/.lingtai/.addons/telegram/<bot_name>/config.json`.
   For example, if the bot is called `myagent_bot`:
   `~/.lingtai/.addons/telegram/myagent_bot/config.json`

   Contents:
   ```json
   {
     "bot_token_env": "TELEGRAM_BOT_TOKEN",
     "allowed_users": [123456789],
     "poll_interval": 1.0
   }
   ```
   - If no allowed_users requested, omit the field entirely (open access).

3. **Tell the human** the config is ready and give them the exact path. Ask them to:
   - Type `/addon` in the TUI
   - Enter the config path
   - Then type `/refresh` to activate

## Reference
Template with all fields and comments: `~/.lingtai/.addons/telegram/example/config.json`
