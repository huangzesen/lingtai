---
name: lingtai-imap-setup
description: Configure the IMAP email addon for this agent — read this when the human asks to set up email.
version: 2.2.0
---

# IMAP Email Setup

You are helping the human set up IMAP email for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## Fixed-by-Convention Path

**The IMAP config file lives at a single fixed location, shared by all agents in this project:**

```
.lingtai/.addons/imap/config.json   (relative to project root)
```

- **Do not try to change this path.** The TUI and the kernel both expect it exactly here.
- The file is shared across all agents in the same project — one IMAP config serves every agent.
- For multi-account support, use the `accounts` array inside the single `config.json` (see example below). Never create per-account subdirectories.
- From your agent's working directory (`<project>/.lingtai/<your-agent>/`), the relative path written in `init.json` is `../.addons/imap/config.json`.

## Migration Check (IMPORTANT — do this first)

Before creating or editing the config file, **read your own `init.json`** and check `addons.imap.config`. If the path points to the old global location (`~/.lingtai-tui/addons/...`), you MUST fix it:

1. Read your `init.json`
2. If `addons.imap.config` contains `~/.lingtai-tui/` or an absolute path, replace it with:
   ```
   ../.addons/imap/config.json
   ```
3. If the `addons` field is missing entirely and IMAP is not wired up, add it:
   ```json
   "addons": {
     "imap": {
       "config": "../.addons/imap/config.json"
     }
   }
   ```

This is a one-time fix for agents created before the local-config convention. New agents already have the correct relative path.

## Rules

- **Secrets go in .env, never in config JSON.** Read your `init.json` to find the `env_file` field, then append secrets there. Reference them from `config.json` via `*_env` fields (e.g. `email_password_env: "IMAP_PASSWORD"`).
- **Always use the `accounts` array format** — even for a single account. Adding more accounts later then becomes a simple append.
- **Activation:** after creating or editing the config, run `system(action="refresh")` yourself to reload. Do not ask the human to refresh for you.
- **Troubleshooting:** if the addon fails to load, check that `.lingtai/.addons/imap/config.json` exists, is valid JSON, and the referenced env vars are set in `.env`. Report back to the human with the specific problem.
- **Status caveat:** after refresh, `imap(action="accounts")` may show `connected: false` even when IMAP is working. This is a known display bug. Always verify with `imap(action="check")` — if it returns emails, the connection is working regardless of what `connected` says.

## What You Need From the Human

Ask the human for:
1. **Email address** — the agent's email (e.g., `myagent@gmail.com`)
2. **App Password** — a 16-char app password (NOT their regular password)
   - Gmail: Enable 2FA at myaccount.google.com/security → myaccount.google.com/apppasswords → create one
   - Outlook: Enable 2FA at account.microsoft.com/security → App passwords → create one
3. **Allowed senders** (optional) — email addresses allowed to message this agent. If omitted, anyone can send.

## What You Do

Once you have the email address and app password:

1. **Read your init.json** to find the `env_file` path. Then **append the password** to that .env file:
   ```
   IMAP_PASSWORD=<the app password they gave you>
   ```
   For multiple accounts, use distinct env var names (e.g., `IMAP_PASSWORD_GMAIL`, `IMAP_PASSWORD_UCLA`).

2. **Create (or edit) the config file** at `.lingtai/.addons/imap/config.json` relative to the project root. If the file already exists with other accounts, append to its `accounts` array — do not overwrite it.

   Example config (always use the `accounts` array format):
   ```json
   {
     "accounts": [
       {
         "email_address": "<their email>",
         "email_password_env": "IMAP_PASSWORD",
         "imap_host": "imap.gmail.com",
         "smtp_host": "smtp.gmail.com",
         "allowed_senders": ["<human's email if provided>"],
         "poll_interval": 30
       }
     ]
   }
   ```
   - Gmail: `imap.gmail.com` / `smtp.gmail.com`
   - Outlook: `imap.outlook.com` / `smtp.outlook.com`
   - If no allowed_senders requested, omit the field entirely.

   **To add another account later**, just append another object to the `accounts` array in the same file.

3. **Activate:** run `system(action="refresh")` to reload the addon config. Then verify with `imap(action="check")` — if it returns emails or connects without error, you're done. Tell the human IMAP is configured.

## Config Reference

See the example config at `.lingtai/.skills/lingtai-imap-setup/assets/config.json` for a full reference of all available fields.
