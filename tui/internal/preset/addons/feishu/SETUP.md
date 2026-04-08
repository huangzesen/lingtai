# Feishu (Lark) Bot Setup

You are helping the human set up a Feishu bot for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## Rules

- **Find the .env file path** by reading your `init.json` — look for the `env_file` field. Secrets go there, never in config JSON.
- **Config files go under** `~/.lingtai/.addons/feishu/<bot_name>/config.json` where `<bot_name>` is the bot's name. Each bot gets its own directory. Do NOT put config files in the agent's working directory.
- **Never edit the example template** at `~/.lingtai/.addons/feishu/example/config.json` — it is a reference, not a working config.
- **Activation requires the human** to type `/addon` in the TUI, enter the config path, then `/refresh`. You cannot do this yourself.

## What You Need From the Human

Ask the human for:
1. **App ID** — from Feishu Open Platform Developer Console → Credentials (App ID, starts with `cli_`)
2. **App Secret** — from the same Credentials page
3. **Allowed open_ids** (optional) — Feishu open_id of users allowed to message the bot. If omitted, anyone can message.
   - To find a user's open_id: ask them to open the bot in Feishu — the open_id is visible in the developer console's contact directory.

## What You Do

Once you have the App ID and App Secret:

1. **Read your init.json** to find the `env_file` path. Then **append the credentials** to that .env file:
   ```
   FEISHU_APP_ID=cli_xxxxxxxxxxxxxxxxxxxxxxxx
   FEISHU_APP_SECRET=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```

2. **Create the config file** at `~/.lingtai/.addons/feishu/<bot_name>/config.json`.
   For example, if the bot is called `myagent_feishu`:
   `~/.lingtai/.addons/feishu/myagent_feishu/config.json`

   Contents:
   ```json
   {
     "app_id_env": "FEISHU_APP_ID",
     "app_secret_env": "FEISHU_APP_SECRET",
     "allowed_users": ["ou_xxxxxxxxxxxxxxxx", "ou_yyyyyyyyyyyyyyyy"]
   }
   ```
   - If no allowed_users requested, omit the field entirely (open access).

3. **Tell the human** the config is ready and give them the exact path. Ask them to:
   - Type `/addon` in the TUI
   - Enter the config path
   - Then type `/refresh` to activate

## How Feishu Bot Works

The Feishu addon uses a **long WebSocket connection** (via lark-oapi SDK) to receive events in real time — no polling, no webhooks needed.

Setup on Feishu Open Platform:
1. Go to https://open.feishu.cn/app
2. Create an enterprise app (or use existing)
3. **Enable Bot capability** — this is required for the bot to receive messages
4. In **Event Subscriptions**, choose **"Use long connection to receive events"** (no URL needed)
5. Subscribe to the event: `im.message.receive_v1` (receive messages)
6. In **Permissions**, add: `im:message` (read and send messages)

## Reference
Template with all fields and comments: `~/.lingtai/.addons/feishu/example/config.json`
