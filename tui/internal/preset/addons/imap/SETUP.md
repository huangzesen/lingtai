# IMAP Email Setup

You are helping the human set up IMAP email for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## What You Need From the Human

Ask the human for:
1. **Email address** — the agent's email (e.g., `myagent@gmail.com`)
2. **App Password** — a 16-char app password (NOT their regular password)
   - Gmail: Enable 2FA at myaccount.google.com/security → myaccount.google.com/apppasswords → create one
   - Outlook: Enable 2FA at account.microsoft.com/security → App passwords → create one
3. **Allowed senders** (optional) — email addresses allowed to message this agent. If omitted, anyone can send.

## What You Do

Once you have the email address and app password:

1. **Add the password to the .env file** (the path is in your init.json under `env_file`):
   Append this line:
   ```
   IMAP_PASSWORD=<the app password they gave you>
   ```

2. **Create the config file** at:
   ```
   ~/.lingtai-tui/addons/imap/<email_address>/config.json
   ```
   For example, if the email is `myagent@gmail.com`, write to:
   `~/.lingtai-tui/addons/imap/myagent@gmail.com/config.json`

   Contents (adjust host for Gmail/Outlook/other):
   ```json
   {
     "email_address": "<their email>",
     "email_password_env": "IMAP_PASSWORD",
     "imap_host": "imap.gmail.com",
     "smtp_host": "smtp.gmail.com",
     "allowed_senders": ["<human's email if provided>"],
     "poll_interval": 30
   }
   ```
   - Gmail: `imap.gmail.com` / `smtp.gmail.com`
   - Outlook: `imap.outlook.com` / `smtp.outlook.com`
   - If no allowed_senders, omit the field entirely.

3. **Tell the human** the config is ready. Give them the exact path and ask them to:
   - Type `/addon` in the TUI
   - Enter the config path (e.g., `~/.lingtai-tui/addons/imap/myagent@gmail.com/config.json`)
   - Then type `/refresh` to activate

## Reference
Template with comments: `~/.lingtai-tui/addons/imap/example/config.json`
