---
name: lingtai-feishu-setup
description: Configure the Feishu (Lark) bot addon for this agent — read this when the human asks to set up a Feishu bot.
version: 2.0.0
---

# Feishu (Lark) Bot Setup

You are helping the human set up a Feishu bot for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## Fixed-by-Convention Path

**The Feishu config file lives at a single fixed location, shared by all agents in this project:**

```
.lingtai/.addons/feishu/config.json   (relative to project root)
```

- **Do not try to change this path.** The TUI and the kernel both expect it exactly here.
- The file is shared across all agents in the same project.
- Feishu supports one bot per config file. If you need multiple bots per project, that's currently not supported by a single shared config — ask the human whether they really need this before proceeding.
- From your agent's working directory, the relative path written in `init.json` is `../.addons/feishu/config.json`. You should not need to edit `init.json` — it was pre-populated when the agent was created.

## Rules

- **Secrets go in .env, never in config JSON.** Read your `init.json` to find the `env_file` field, then append the credentials there. Reference them from `config.json` via `app_id_env` / `app_secret_env`.
- **Activation:** after creating or editing the config, tell the human to run `/refresh` in the TUI. You cannot activate addons yourself.
- **Troubleshooting:** if the addon fails to load, check that `.lingtai/.addons/feishu/config.json` exists, is valid JSON, and the referenced env vars are set in `.env`. Report back to the human with the specific problem.

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

2. **Create the config file** at `.lingtai/.addons/feishu/config.json` relative to the project root:
   ```json
   {
     "app_id_env": "FEISHU_APP_ID",
     "app_secret_env": "FEISHU_APP_SECRET",
     "allowed_users": ["ou_xxxxxxxxxxxxxxxx", "ou_yyyyyyyyyyyyyyyy"]
   }
   ```
   - If no allowed_users requested, omit the field entirely (open access).

3. **Tell the human** the config is ready at `.lingtai/.addons/feishu/config.json` and ask them to run `/refresh` in the TUI to activate.

## Feishu Bot Setup (Platform Side)

The Feishu addon uses a **long WebSocket connection** (via lark-oapi SDK) to receive events in real time — no polling, no webhooks needed.

Setup on Feishu Open Platform:
1. Go to https://open.feishu.cn/app
2. Create an enterprise app (or use existing)
3. **Enable Bot capability** — this is required for the bot to receive messages
4. In **Event Subscriptions**, choose **"Use long connection to receive events"** (no URL needed)
5. Subscribe to the event: `im.message.receive_v1` (receive messages)
6. In **Permissions**, add: `im:message` (read and send messages)

## Config Reference

See the example config at `.lingtai/.skills/lingtai-feishu-setup/assets/config.json` for a full reference of all available fields.
