---
name: lingtai-wechat-setup
description: Configure the WeChat addon for this agent — read this when the human asks to set up WeChat.
version: 1.2.0
---

# WeChat Setup

You are helping the human connect this agent to WeChat via Tencent's iLink Bot API. Unlike other addons, WeChat uses **QR code login** — there are no static credentials to paste. Your job is to **walk the human through setup and execute the steps yourself** — do not just list instructions and ask the human to do everything.

## Fixed-by-Convention Path

**The WeChat config file lives at a single fixed location, shared by all agents in this project:**

```
.lingtai/.addons/wechat/config.json   (relative to project root)
```

- **Do not try to change this path.** The TUI and the kernel both expect it exactly here.
- The file is shared across all agents in the same project.
- One WeChat account per project.
- From your agent's working directory, the relative path in `init.json` is `../.addons/wechat/config.json`.

## Migration Check (IMPORTANT — do this first)

Before creating or editing the config file, **read your own `init.json`** and check `addons.wechat.config`. If the path points to the old global location (`~/.lingtai-tui/addons/...`), you MUST fix it:

1. Read your `init.json`
2. If `addons.wechat.config` contains `~/.lingtai-tui/` or an absolute path, replace it with:
   ```
   ../.addons/wechat/config.json
   ```
3. If the `addons` field is missing entirely and WeChat is not wired up, add it:
   ```json
   "addons": {
     "wechat": {
       "config": "../.addons/wechat/config.json"
     }
   }
   ```

This is a one-time fix for agents created before the local-config convention. New agents already have the correct relative path.

## Credentials

WeChat does NOT use static API keys. Instead, a `bot_token` is obtained by scanning a QR code with the WeChat mobile app. The token is stored separately from config:

```
.lingtai/.addons/wechat/credentials.json   (mode 0600, machine-managed)
```

You do NOT manually create this file. The login command creates it.

## What You Need From the Human

1. **A WeChat account** on their phone (the one that will be connected to the agent).
2. **Physical access** to scan a QR code displayed in the terminal.
3. **Allowed users** (optional) — WeChat user IDs to restrict who can message the bot. If omitted, anyone can message.

## What You Do

### First-Time Setup

1. **Create the config file** at `.lingtai/.addons/wechat/config.json` relative to the project root (create directories as needed):

   ```json
   {
     "base_url": "https://ilinkai.weixin.qq.com",
     "cdn_base_url": "https://novac2c.cdn.weixin.qq.com/c2c",
     "poll_interval": 1.0,
     "allowed_users": []
   }
   ```

   If the human provided specific allowed_users, include them as a list of WeChat user ID strings.

2. **Run the login command** to display the QR code:

   ```bash
   python -c "from lingtai.addons.wechat.login import cli_login; cli_login('.lingtai/.addons/wechat')"
   ```

   This will:
   - Display a QR code in the terminal
   - Wait for the human to scan it with WeChat on their phone
   - Save the `bot_token` to `credentials.json` on successful scan
   - Print "Connected as <user_id>" on success

3. **Activate:** run `system(action="refresh")` to reload the addon config. Then verify the connection is working. Tell the human WeChat is configured.

### Re-Login (Session Expired)

WeChat sessions can expire. When this happens, the addon pauses and sends the human a notification mail. To re-login:

1. Run the same login command from step 2 above.
2. Run `system(action="refresh")` to reload after successful login.

## Rules

- **Never edit `credentials.json` manually.** It is managed by the login command.
- **Config changes require refresh** — run `system(action="refresh")` yourself after any config change.
- **Status caveat:** after refresh, addon status may show `connected: false` even when working. Always verify by attempting actual operation — if it succeeds, the connection is fine.
- **If login fails** (QR expired, network error), retry the login command. Each attempt generates a fresh QR code.
- **The QR code expires in 5 minutes.** Tell the human to scan promptly.

## Config Reference

See the example config at `.lingtai/.skills/lingtai-wechat-setup/assets/config.json` for all available fields.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `base_url` | string | `https://ilinkai.weixin.qq.com` | iLink API endpoint |
| `cdn_base_url` | string | `https://novac2c.cdn.weixin.qq.com/c2c` | CDN for media uploads/downloads |
| `poll_interval` | float | `1.0` | Seconds between long-poll retries |
| `allowed_users` | string[] | `[]` | WeChat user IDs to accept. Empty = accept all. |
