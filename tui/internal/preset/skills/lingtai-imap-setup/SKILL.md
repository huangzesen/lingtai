---
name: lingtai-imap-setup
description: Configure the IMAP email addon for this agent — read this when the human asks to set up email.
version: 3.0.0
---

# IMAP Email Setup

You are helping the human set up IMAP email for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## New Convention — Admin-Local `.secrets/`

**The IMAP config lives inside your own working directory:**

```
.secrets/imap.json   (relative to your working directory)
```

- This file is **yours alone** — other agents in the network do not read it.
- The `init.json` `addons.imap.config` field is simply `.secrets/imap.json` (no `../`, no absolute path).
- Secrets go **directly in the JSON** as plaintext. No `*_env` indirection. No `.env` file involved. The config is self-contained.

### Why

Addons are an admin-only responsibility. Avatars should never configure addons — the orchestrator owns them. Colocating the config with the orchestrator's working directory makes that ownership explicit.

## Legacy Path (back-compat only)

If your `init.json` already has `addons.imap.config` pointing at `../.addons/imap/config.json` and the old setup is working, **leave it alone**. The old path keeps functioning. Only migrate to the new convention when the human explicitly asks you to, or when you are creating IMAP setup for the first time.

If the human does ask you to migrate an old setup:
1. Read the old config at `../.addons/imap/config.json`.
2. Resolve any `*_env` fields by reading the referenced env vars from the `.env` file named in your `init.json` `env_file` field, substituting the plaintext value and dropping the `_env` suffix.
3. Write the resolved config to `.secrets/imap.json`.
4. Update `init.json` `addons.imap.config` to `.secrets/imap.json`.
5. Run `system(action="refresh")`.
6. Verify with `imap(action="check")`. If it works, tell the human the old file at `../.addons/imap/config.json` can be deleted at their discretion.

## Rules

- **Always use the `accounts` array format** — even for a single account. Adding more accounts later is then just an append.
- **Secrets are plaintext in the JSON.** Do not use `email_password_env`. The `.env` file is only for LLM API keys and other non-addon secrets.
- **Activation:** after creating or editing `.secrets/imap.json`, run `system(action="refresh")` yourself to reload.
- **Troubleshooting:** if the addon fails to load, check that `.secrets/imap.json` exists, is valid JSON, and that your `init.json` `addons.imap.config` points at `.secrets/imap.json`. Report back to the human with the specific problem.
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

1. **Create the config file** at `.secrets/imap.json` in your own working directory. If the file already exists with other accounts, append to its `accounts` array — do not overwrite it.

   Example config (always use the `accounts` array format):
   ```json
   {
     "accounts": [
       {
         "email_address": "<their email>",
         "email_password": "<app password plaintext>",
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

2. **Wire it into `init.json`.** Read your `init.json`. If `addons.imap` is missing, add it:
   ```json
   "addons": {
     "imap": {
       "config": ".secrets/imap.json"
     }
   }
   ```
   If `addons.imap.config` already exists and points at a legacy path (e.g. `../.addons/imap/config.json`), see the "Legacy Path" section above — do **not** rewrite it unless the human asked you to migrate.

3. **Activate:** run `system(action="refresh")` to reload the addon config. Then verify with `imap(action="check")` — if it returns emails or connects without error, you're done. Tell the human IMAP is configured.

## Config Reference

See the example config at `.lingtai/.library/lingtai-imap-setup/assets/config.json` for a full reference of all available fields.
