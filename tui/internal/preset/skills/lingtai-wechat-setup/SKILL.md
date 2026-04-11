---
name: lingtai-wechat-setup
description: Configure the WeChat addon for this agent — read this when the human asks to set up WeChat.
version: 1.0.0
---

# WeChat Setup

You are helping the human connect this agent to WeChat via Tencent's iLink Bot API. Unlike other addons, WeChat uses **QR code login** — there are no static credentials to paste.

## Fixed-by-Convention Path

**The WeChat config file lives at a single fixed location, shared by all agents in this project:**

```
.lingtai/.addons/wechat/config.json   (relative to project root)
```

- **Do not try to change this path.** The TUI and the kernel both expect it exactly here.
- The file is shared across all agents in the same project.
- One WeChat account per project.
- From your agent's working directory, the relative path in `init.json` is `../.addons/wechat/config.json`. You should not need to edit `init.json`.

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

3. **Tell the human** to run `/refresh` in the TUI to activate the WeChat addon.

### Re-Login (Session Expired)

WeChat sessions can expire. When this happens, the addon pauses and sends the human a notification mail. To re-login:

1. Run the same login command from step 2 above.
2. Tell the human to run `/refresh` after successful login.

## Rules

- **Never edit `credentials.json` manually.** It is managed by the login command.
- **Config changes require `/refresh`** to take effect.
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
