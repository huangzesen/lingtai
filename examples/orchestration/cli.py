"""CLI email client — send/receive emails to the orchestration service.

Usage:
    python -m examples.orchestration.cli

Commands:
    /send <message>   Send email to the admin orchestrator
    /inbox            List recent inbox emails
    /read <id>        Read full email by ID (shows attachments path)
    /sent             List sent emails
    /quit             Exit CLI (agent keeps running)

    Typing without a / prefix sends the text as an email (shorthand for /send).

Requires the orchestration service to be running (python -m examples.orchestration).
"""
from __future__ import annotations

import json
import sys
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4

from stoai.services.mail import TCPMailService

PLAYGROUND = Path.home() / ".stoai" / "orchestration" / "playground"
SERVICE_JSON = PLAYGROUND / "service.json"
USER_DIR = PLAYGROUND / "user"


def load_service_info() -> dict:
    """Load service.json. Exits if service is not running."""
    if not SERVICE_JSON.is_file():
        print(f"Error: Service not running. No {SERVICE_JSON}")
        print("Start the service first: python -m examples.orchestration")
        sys.exit(1)
    info = json.loads(SERVICE_JSON.read_text())
    if info.get("status") != "running":
        print(f"Warning: Service status is '{info.get('status')}'. It may not be running.")
    return info


def list_emails(folder: str, n: int = 10) -> list[dict]:
    """Load emails from on-disk mailbox, sorted newest first."""
    folder_dir = USER_DIR / "mailbox" / folder
    if not folder_dir.is_dir():
        return []
    emails = []
    for msg_dir in folder_dir.iterdir():
        msg_file = msg_dir / "message.json"
        if msg_dir.is_dir() and msg_file.is_file():
            try:
                data = json.loads(msg_file.read_text())
                data["_mailbox_id"] = msg_dir.name
                data["_folder"] = folder
                emails.append(data)
            except (json.JSONDecodeError, OSError):
                continue
    # Sort by time (newest first)
    def sort_key(e):
        return e.get("received_at") or e.get("sent_at") or e.get("time") or ""
    emails.sort(key=sort_key, reverse=True)
    return emails[:n]


def print_email_list(emails: list[dict], folder: str) -> None:
    """Print a formatted email list."""
    if not emails:
        print(f"  (no emails in {folder})")
        return
    for e in emails:
        eid = e.get("_mailbox_id", "?")[:8]
        sender = e.get("from", "?")
        to = e.get("to", [])
        if isinstance(to, list):
            to = ", ".join(to)
        subject = e.get("subject", "(no subject)")
        ts = e.get("received_at") or e.get("sent_at") or e.get("time") or ""
        # Truncate timestamp to time only
        if "T" in ts:
            ts = ts.split("T")[1][:8]

        if folder == "inbox":
            print(f"  [{eid}] from:{sender}  {subject}  ({ts})")
        else:
            print(f"  [{eid}] to:{to}  {subject}  ({ts})")


def read_email(email_id: str) -> None:
    """Read and display a full email by ID (prefix match)."""
    for folder in ("inbox", "sent"):
        folder_dir = USER_DIR / "mailbox" / folder
        if not folder_dir.is_dir():
            continue
        for msg_dir in folder_dir.iterdir():
            if msg_dir.name.startswith(email_id) and msg_dir.is_dir():
                msg_file = msg_dir / "message.json"
                if not msg_file.is_file():
                    continue
                data = json.loads(msg_file.read_text())
                print(f"\n--- Email [{msg_dir.name[:8]}] ---")
                print(f"From: {data.get('from', '?')}")
                to = data.get("to", [])
                if isinstance(to, list):
                    to = ", ".join(to)
                print(f"To: {to}")
                if data.get("cc"):
                    print(f"CC: {', '.join(data['cc'])}")
                print(f"Subject: {data.get('subject', '(no subject)')}")
                ts = data.get("received_at") or data.get("sent_at") or ""
                print(f"Time: {ts}")
                print(f"\n{data.get('message', '')}\n")
                # Check for attachments
                att_dir = msg_dir / "attachments"
                if att_dir.is_dir() and any(att_dir.iterdir()):
                    print(f"Attachments: {att_dir}")
                    for f in att_dir.iterdir():
                        print(f"  - {f.name}")
                print(f"---\nFull path: {msg_dir}")
                return
    print(f"  Email not found: {email_id}")


def send_email(admin_address: str, user_address: str, message: str,
               mail_service: TCPMailService | None = None) -> None:
    """Send an email to the admin orchestrator."""
    sender = mail_service or TCPMailService()
    # Also save to user's sent/ folder
    sent_id = str(uuid4())
    sent_dir = USER_DIR / "mailbox" / "sent" / sent_id
    sent_dir.mkdir(parents=True, exist_ok=True)
    payload = {
        "from": user_address,
        "to": [admin_address],
        "subject": "",
        "message": message,
    }
    (sent_dir / "message.json").write_text(json.dumps({
        **payload,
        "_mailbox_id": sent_id,
        "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }, indent=2))
    ok = sender.send(admin_address, payload)
    if ok:
        print(f"  Sent to {admin_address}")
    else:
        print(f"  Failed to send to {admin_address}")


def run_cli(admin_address: str, user_port: int) -> None:
    """Main CLI loop."""
    user_address = f"127.0.0.1:{user_port}"

    # Ensure user mailbox dir exists
    USER_DIR.mkdir(parents=True, exist_ok=True)

    # Start TCPMailService to receive emails in real-time
    user_mail = TCPMailService(listen_port=user_port, working_dir=USER_DIR)
    def on_mail(payload: dict) -> None:
        sender = payload.get("from", "?")
        subject = payload.get("subject", "")
        preview = payload.get("message", "")[:80].replace("\n", " ")
        print(f"\n  [New mail from {sender}] {subject}")
        if preview:
            print(f"  {preview}...")
        eid = payload.get("_mailbox_id", "")[:8]
        if eid:
            print(f"  /read {eid}")
        print("> ", end="", flush=True)

    user_mail.listen(on_message=on_mail)

    # Show status
    inbox = list_emails("inbox")
    unread_count = len(inbox)
    print(f"Connected to {admin_address} as {user_address}")
    if unread_count:
        print(f"You have {unread_count} emails. Type /inbox to see them.")
    print("Type /send <message> or just type a message. /quit to exit.\n")

    try:
        while True:
            try:
                line = input("> ")
            except EOFError:
                break

            line = line.strip()
            if not line:
                continue

            if line == "/quit":
                break
            elif line == "/inbox":
                emails = list_emails("inbox")
                print_email_list(emails, "inbox")
            elif line == "/sent":
                emails = list_emails("sent")
                print_email_list(emails, "sent")
            elif line.startswith("/read "):
                email_id = line[6:].strip()
                if email_id:
                    read_email(email_id)
                else:
                    print("  Usage: /read <id>")
            elif line.startswith("/send "):
                message = line[6:].strip()
                if message:
                    send_email(admin_address, user_address, message, user_mail)
                else:
                    print("  Usage: /send <message>")
            elif line.startswith("/"):
                print(f"  Unknown command: {line.split()[0]}")
                print("  Commands: /send /inbox /read /sent /quit")
            else:
                # Bare text = shorthand for /send
                send_email(admin_address, user_address, line, user_mail)

    except KeyboardInterrupt:
        print()

    user_mail.stop()
    print("CLI exited. Agent is still running.")


def main():
    info = load_service_info()
    admin_address = info["admin_address"]
    user_port = info["user_port"]
    run_cli(admin_address, user_port)


if __name__ == "__main__":
    main()
