# Orchestration Example Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an orchestration example with a background agent service and a CLI email client.

**Architecture:** Two entry points under `examples/orchestration/` — `__main__.py` runs the admin orchestrator as a long-lived service, `cli.py` is a standalone email client that reads/writes the user's on-disk mailbox. All communication via `TCPMailService`. Runtime state at `~/.lingtai/orchestration/playground/`.

**Tech Stack:** Python 3.11+, lingtai Agent, TCPMailService, MiniMax LLM (multimodal)

**Spec:** `docs/superpowers/specs/2026-03-17-orchestration-example-design.md`

---

## File Structure

| Action | Path | Responsibility |
|--------|------|---------------|
| Create | `examples/orchestration/__init__.py` | Empty package marker |
| Create | `examples/orchestration/__main__.py` | Background service — starts orchestrator agent, writes service.json, blocks until signal |
| Create | `examples/orchestration/cli.py` | CLI email client — /send, /inbox, /read, /sent, /quit |

---

## Chunk 1: Service (`__main__.py`)

### Task 1: Create the orchestration package and service entry point

**Files:**
- Create: `examples/orchestration/__init__.py`
- Create: `examples/orchestration/__main__.py`

- [ ] **Step 1: Create empty `__init__.py`**

Create `examples/orchestration/__init__.py` — empty file.

- [ ] **Step 2: Write `__main__.py`**

```python
"""Orchestration service — starts the admin orchestrator agent.

Usage:
    python -m examples.orchestration

The admin agent listens on port 8301. User mailbox on port 8300.
Runtime data at ~/.lingtai/orchestration/playground/.
Press Ctrl+C to shut down.
"""
from __future__ import annotations

import json
import os
import signal
import sys
import threading
from datetime import datetime, timezone
from pathlib import Path

# Load .env
env_path = Path(__file__).parent.parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

from lingtai import Agent, AgentConfig
from lingtai.llm import LLMService
from lingtai.services.mail import TCPMailService

ADMIN_PORT = 8301
USER_PORT = 8300
PLAYGROUND = Path.home() / ".lingtai" / "orchestration" / "playground"
SERVICE_JSON = PLAYGROUND / "service.json"

COVENANT = """\
You are the Admin, an orchestrator agent.

## Delegation
- You can delegate tasks to subagents using the delegate tool.
- Maximum 10 subagents at any time.
- When delegating, ALWAYS pass capabilities explicitly:
  capabilities=["email", "bash", "file", "web_search", "vision", "anima"]
  This ensures subagents do NOT get conscience or delegate.
- Generate a tailored covenant for each subagent (pass as role= in delegate).
- After spawning a subagent, broadcast its address to ALL existing subagents
  by emailing each one the updated peer list.

## Communication
- All communication is via email. Your text responses are your private diary.
- When you receive an email, process the request and email your reply to the sender.
- Keep emails concise and actionable.
- Never go back and forth with courtesy emails.

## Initiative
- Your conscience (inner voice) is active. Use it to stay proactive.
- When idle, reflect on ongoing tasks and check on subagents.

## Contacts
- User: 127.0.0.1:""" + str(USER_PORT) + """
"""


def write_service_json(status: str = "running") -> None:
    """Write service.json with current state."""
    SERVICE_JSON.write_text(json.dumps({
        "pid": os.getpid(),
        "admin_address": f"127.0.0.1:{ADMIN_PORT}",
        "user_port": USER_PORT,
        "status": status,
        "started_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }, indent=2))


def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set. Check .env file.")
        sys.exit(1)

    # Create playground directory (must exist before Agent constructor)
    PLAYGROUND.mkdir(parents=True, exist_ok=True)

    print(f"Starting orchestration service...")
    print(f"  Playground: {PLAYGROUND}")
    print(f"  Admin port: {ADMIN_PORT}")
    print(f"  User port:  {USER_PORT}")

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_config={
            "web_search_provider": "minimax",
            "vision_provider": "minimax",
        },
        provider_defaults={
            "minimax": {"model": "MiniMax-M2.5-highspeed"},
        },
    )

    mail_svc = TCPMailService(
        listen_port=ADMIN_PORT,
        working_dir=PLAYGROUND / "admin",
    )

    policy = str(Path(__file__).parent.parent / "bash_policy.json")

    agent = Agent(
        agent_id="admin",
        service=llm,
        mail_service=mail_svc,
        config=AgentConfig(max_turns=20),
        base_dir=PLAYGROUND,
        streaming=True,
        admin=True,
        role=COVENANT,
        capabilities={
            "email": {},
            "bash": {"policy_file": policy},
            "file": {},
            "web_search": {},
            "vision": {},
            "anima": {},
            "conscience": {"interval": 300},
            "delegate": {},
        },
    )

    agent.start()
    write_service_json("running")

    print(f"Admin agent started. PID: {os.getpid()}")
    print(f"Service info: {SERVICE_JSON}")
    print("Press Ctrl+C to shut down.")

    # Block until signal
    stop_event = threading.Event()

    def on_signal(signum, frame):
        print("\nShutting down...")
        stop_event.set()

    signal.signal(signal.SIGINT, on_signal)
    signal.signal(signal.SIGTERM, on_signal)

    stop_event.wait()

    agent.stop(timeout=10.0)
    write_service_json("stopped")
    print("Done.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 3: Smoke test — verify the module imports**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && source venv/bin/activate && python -c "import examples.orchestration"`
Expected: No import errors. (The actual agent won't start without MINIMAX_API_KEY, but the module should import cleanly.)

- [ ] **Step 4: Commit**

```bash
git add examples/orchestration/__init__.py examples/orchestration/__main__.py
git commit -m "feat(examples): add orchestration service entry point"
```

---

## Chunk 2: CLI Email Client (`cli.py`)

### Task 2: Build the CLI email client

**Files:**
- Create: `examples/orchestration/cli.py`

- [ ] **Step 1: Write `cli.py`**

```python
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
import threading
import time
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4

from lingtai.services.mail import TCPMailService

PLAYGROUND = Path.home() / ".lingtai" / "orchestration" / "playground"
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


def send_email(admin_address: str, user_address: str, message: str) -> None:
    """Send an email to the admin orchestrator."""
    sender = TCPMailService()
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
    new_mail_event = threading.Event()

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
        new_mail_event.set()

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
                    send_email(admin_address, user_address, message)
                else:
                    print("  Usage: /send <message>")
            elif line.startswith("/"):
                print(f"  Unknown command: {line.split()[0]}")
                print("  Commands: /send /inbox /read /sent /quit")
            else:
                # Bare text = shorthand for /send
                send_email(admin_address, user_address, line)

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
```

- [ ] **Step 2: Smoke test — verify the module imports**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && source venv/bin/activate && python -c "from examples.orchestration.cli import run_cli"`
Expected: No import errors.

- [ ] **Step 3: Commit**

```bash
git add examples/orchestration/cli.py
git commit -m "feat(examples): add orchestration CLI email client"
```
