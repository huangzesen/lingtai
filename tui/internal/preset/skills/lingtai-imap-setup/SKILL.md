---
name: lingtai-imap-setup
description: Configure the IMAP email addon for this agent — read this when the human asks to set up email.
version: 1.0.0
---

# IMAP Email Setup

You are helping the human set up IMAP email for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## Rules

- **Secrets go in .env, never in config JSON.** Read your `init.json` to find the `env_file` field, then append secrets there.
- **Config files go under** `{agentDir}/addons/imap/<account>/config.json` — one directory per email account. Never put configs in the agent's root directory.
- **Always use the `accounts` array format** — even for a single account. This makes adding more accounts later a simple append.
- **Activation:** after creating the config, tell the human to run `/refresh` in the TUI. You cannot activate addons yourself.

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

2. **Create the config file** at `{agentDir}/addons/imap/<email_address>/config.json`.
   In init.json, reference it as a relative path: `"config": "addons/imap/<email_address>/config.json"`.
   The path is resolved relative to the agent's working directory (where init.json lives).

   Example config (always use the `accounts` array format, even for a single account):
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

   **To add another account later**, just append to the `accounts` array.

3. **Tell the human** the config is ready and ask them to run `/refresh` in the TUI to activate.

## Config Reference

See the example config at `skills/lingtai-imap-setup/assets/config.json` for a full reference of all available fields.
