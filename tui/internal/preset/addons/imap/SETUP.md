# IMAP Email Setup

You are helping the human set up IMAP email for this agent. Your job is to **create the config file yourself** — do not just list the steps and ask the human to do it.

## Rules

- **Find the .env file path** by reading your `init.json` — look for the `env_file` field. Secrets go there, never in config JSON.
- **Config files go under** `~/.lingtai/.addons/imap/<account>/config.json` where `<account>` is the email address. Each account gets its own directory. Do NOT put config files in the agent's working directory.
- **Never edit the example template** at `~/.lingtai/.addons/imap/example/config.json` — it is a reference, not a working config.
- **Always use the `accounts` array format** — even for a single account. This makes adding more accounts later a simple append.
- **Activation requires the human** to type `/addon` in the TUI, enter the config path, then `/refresh`. You cannot do this yourself.

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

2. **Create the config file** at `~/.lingtai/.addons/imap/<email_address>/config.json`.
   For example, if the email is `myagent@gmail.com`:
   `~/.lingtai/.addons/imap/myagent@gmail.com/config.json`

   Always use the `accounts` array format:
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

   **To add another account later**, just append to the `accounts` array:
   ```json
   {
     "accounts": [
       {
         "email_address": "agent@gmail.com",
         "email_password_env": "IMAP_PASSWORD_GMAIL",
         "imap_host": "imap.gmail.com",
         "smtp_host": "smtp.gmail.com"
       },
       {
         "email_address": "agent@ucla.edu",
         "email_password_env": "IMAP_PASSWORD_UCLA",
         "imap_host": "imap.gmail.com",
         "smtp_host": "smtp.gmail.com"
       }
     ]
   }
   ```

3. **Tell the human** the config is ready and give them the exact path. Ask them to:
   - Type `/addon` in the TUI
   - Enter the config path
   - Then type `/refresh` to activate

## Reference
Template with all fields and comments: `~/.lingtai/.addons/imap/example/config.json`
