---
name: lingtai-imap-setup
description: Configure the IMAP email addon for this agent — read this when the human asks to set up email.
version: 2.0.0
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
- From your agent's working directory (`<project>/.lingtai/<your-agent>/`), the relative path written in `init.json` is `../.addons/imap/config.json`. You should not need to edit `init.json` — it was pre-populated when the agent was created.

## Rules

- **Secrets go in .env, never in config JSON.** Read your `init.json` to find the `env_file` field, then append secrets there. Reference them from `config.json` via `*_env` fields (e.g. `email_password_env: "IMAP_PASSWORD"`).
- **Always use the `accounts` array format** — even for a single account. Adding more accounts later then becomes a simple append.
- **Activation:** after creating or editing the config, tell the human to run `/refresh` in the TUI. You cannot activate addons yourself.
- **Troubleshooting:** if the addon fails to load, check that `.lingtai/.addons/imap/config.json` exists, is valid JSON, and the referenced env vars are set in `.env`. Report back to the human with the specific problem.

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

3. **Tell the human** the config is ready at `.lingtai/.addons/imap/config.json` and ask them to run `/refresh` in the TUI to activate.

## Config Reference

See the example config at `skills/lingtai-imap-setup/assets/config.json` for a full reference of all available fields.
