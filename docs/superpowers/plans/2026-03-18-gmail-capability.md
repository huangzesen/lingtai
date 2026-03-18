# Gmail Capability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `gmail` capability that lets a StoAI agent send and receive real email via Gmail, reusing the existing `email` capability's mailbox, notification, and tool surface.

**Architecture:** The gmail capability is a thin transport layer — a `GmailMailService` that implements the `MailService` ABC using IMAP (receive) and SMTP (send) instead of TCP. It plugs into the existing email capability unchanged. The agent gets the same `email` tool (send/check/read/reply/search/contacts) — the only difference is that "send" goes through Gmail SMTP and inbound emails arrive via IMAP polling instead of TCP. Address format changes from `127.0.0.1:8301` to real email addresses like `user@gmail.com`.

**Tech Stack:** Python stdlib (`imaplib`, `smtplib`, `email`), existing `MailService` ABC, existing `email` capability.

---

## Design

### Key Insight

The existing architecture already separates transport (`MailService`) from mailbox logic (`email` capability). The email capability calls `agent._mail_service.send(address, payload)` and receives mail via `agent._mail_service.listen(on_message=callback)`. We just need a new `MailService` implementation that speaks Gmail instead of TCP.

### What changes

1. **New file:** `src/stoai/services/gmail.py` — `GmailMailService(MailService)` using IMAP polling + SMTP send
2. **New file:** `src/stoai/capabilities/gmail.py` — capability that wires `GmailMailService` + `email` capability together
3. **New file:** `tests/test_services_gmail.py` — unit tests for `GmailMailService`
4. **New file:** `tests/test_capability_gmail.py` — integration test for gmail capability setup
5. **Modify:** `src/stoai/capabilities/__init__.py` — register `gmail` capability
6. **Delete:** `app/email/bridge.py` — the bridge is replaced by the service
7. **Modify:** `app/email/__main__.py` — simplify to use gmail capability directly

### What does NOT change

- `email.py` capability — untouched, reused as-is
- `BaseAgent` — no changes
- `MailService` ABC — no changes
- The `email` tool schema — agent sees the exact same tool

### Address format

Gmail addresses are real email addresses: `user@gmail.com`. The `MailService.address` property returns the Gmail address. The email capability's contacts, send, reply all work with these addresses natively — no translation needed.

### Flow

```
Inbound:  Gmail IMAP poll → new email → on_message(payload) → email capability → agent notification
Outbound: Agent calls email(send) → EmailManager._send() → GmailMailService.send() → SMTP
```

---

## File Structure

| File | Responsibility |
|------|---------------|
| `src/stoai/services/gmail.py` | `GmailMailService` — IMAP polling + SMTP send, implements `MailService` ABC |
| `src/stoai/capabilities/gmail.py` | `setup()` — creates `GmailMailService`, sets it as `agent._mail_service`, then delegates to `email.setup()` |
| `tests/test_services_gmail.py` | Unit tests for IMAP/SMTP logic (mocked) |
| `tests/test_capability_gmail.py` | Integration test: gmail capability sets up correctly, email tool is registered |
| `src/stoai/capabilities/__init__.py` | Register `"gmail"` in `_BUILTIN` |
| `app/email/__main__.py` | Simplified launcher using `capabilities={"gmail": {...}}` |

---

## Tasks

### Task 1: GmailMailService

**Files:**
- Create: `src/stoai/services/gmail.py`
- Test: `tests/test_services_gmail.py`

- [ ] **Step 1: Write failing test for GmailMailService construction**

```python
# tests/test_services_gmail.py
from __future__ import annotations
from unittest.mock import patch, MagicMock
from stoai.services.gmail import GmailMailService


def test_gmail_service_construction():
    svc = GmailMailService(
        gmail_address="agent@gmail.com",
        gmail_password="xxxx xxxx xxxx xxxx",
        poll_interval=30,
    )
    assert svc.address == "agent@gmail.com"
    assert svc._poll_interval == 30


def test_gmail_service_address_none_before_listen():
    """Address is available immediately (it's the gmail address)."""
    svc = GmailMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )
    assert svc.address == "agent@gmail.com"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_services_gmail.py -v`
Expected: FAIL with `ModuleNotFoundError: No module named 'stoai.services.gmail'`

- [ ] **Step 3: Implement GmailMailService**

```python
# src/stoai/services/gmail.py
"""GmailMailService — MailService implementation using Gmail IMAP/SMTP.

Inbound:  Polls Gmail IMAP for UNSEEN emails at a configurable interval.
Outbound: Sends via Gmail SMTP (TLS on port 587).

Uses only Python stdlib (imaplib, smtplib, email). No extra dependencies.

Requires a Gmail App Password (2FA must be enabled on the Gmail account).
"""
from __future__ import annotations

import email as email_lib
import email.utils
import imaplib
import json
import logging
import re
import smtplib
import threading
import time
from datetime import datetime, timezone
from email.header import decode_header
from email.mime.text import MIMEText
from pathlib import Path
from typing import Any, Callable
from uuid import uuid4

from .mail import MailService

log = logging.getLogger(__name__)

_DEFAULT_POLL_INTERVAL = 30  # seconds


def _decode_header_value(value: str) -> str:
    """Decode RFC 2047 encoded header value."""
    parts = decode_header(value)
    decoded = []
    for data, charset in parts:
        if isinstance(data, bytes):
            decoded.append(data.decode(charset or "utf-8", errors="replace"))
        else:
            decoded.append(data)
    return "".join(decoded)


def _extract_text_body(msg: email_lib.message.Message) -> str:
    """Extract plain text body from email message."""
    if msg.is_multipart():
        for part in msg.walk():
            ct = part.get_content_type()
            if ct == "text/plain":
                payload = part.get_payload(decode=True)
                if payload:
                    charset = part.get_content_charset() or "utf-8"
                    return payload.decode(charset, errors="replace")
        # Fallback: try text/html and strip tags
        for part in msg.walk():
            ct = part.get_content_type()
            if ct == "text/html":
                payload = part.get_payload(decode=True)
                if payload:
                    charset = part.get_content_charset() or "utf-8"
                    html = payload.decode(charset, errors="replace")
                    return re.sub(r"<[^>]+>", "", html).strip()
        return ""
    else:
        payload = msg.get_payload(decode=True)
        if payload:
            charset = msg.get_content_charset() or "utf-8"
            return payload.decode(charset, errors="replace")
        return ""


class GmailMailService(MailService):
    """Gmail IMAP/SMTP implementation of MailService.

    Args:
        gmail_address: Gmail address (e.g., "agent@gmail.com").
        gmail_password: Gmail App Password (not regular password).
        allowed_senders: If set, only accept emails from these addresses.
        poll_interval: Seconds between IMAP polls (default 30).
        imap_host: IMAP server (default "imap.gmail.com").
        imap_port: IMAP port (default 993).
        smtp_host: SMTP server (default "smtp.gmail.com").
        smtp_port: SMTP port (default 587).
    """

    def __init__(
        self,
        gmail_address: str,
        gmail_password: str,
        allowed_senders: list[str] | None = None,
        poll_interval: int = _DEFAULT_POLL_INTERVAL,
        imap_host: str = "imap.gmail.com",
        imap_port: int = 993,
        smtp_host: str = "smtp.gmail.com",
        smtp_port: int = 587,
    ):
        self._gmail_address = gmail_address
        self._gmail_password = gmail_password
        self._allowed_senders = (
            {s.lower() for s in allowed_senders} if allowed_senders else None
        )
        self._poll_interval = poll_interval
        self._imap_host = imap_host
        self._imap_port = imap_port
        self._smtp_host = smtp_host
        self._smtp_port = smtp_port

        self._stop_event = threading.Event()
        self._poll_thread: threading.Thread | None = None
        self._processed_uids: set[str] = set()
        self._state_file: Path | None = None  # set by caller if persistence needed

    # ------------------------------------------------------------------
    # MailService interface
    # ------------------------------------------------------------------

    def send(self, address: str, message: dict) -> str | None:
        """Send an email via Gmail SMTP. Address is a real email address."""
        subject = message.get("subject", "")
        body = message.get("message", "")

        if not body and not subject:
            return "Cannot send empty email (no subject or message)"

        try:
            msg = MIMEText(body, "plain", "utf-8")
            msg["From"] = self._gmail_address
            msg["To"] = address
            msg["Subject"] = subject or "(no subject)"
            msg["Date"] = email_lib.utils.formatdate(localtime=True)
            msg["Message-ID"] = email_lib.utils.make_msgid(
                domain=self._gmail_address.split("@")[1]
            )

            with smtplib.SMTP(self._smtp_host, self._smtp_port) as smtp:
                smtp.starttls()
                smtp.login(self._gmail_address, self._gmail_password)
                smtp.send_message(msg)

            log.info("Sent email to %s: %s", address, subject)
            return None  # success

        except Exception as e:
            log.error("Failed to send email to %s: %s", address, e)
            return f"SMTP error: {e}"

    def listen(self, on_message: Callable[[dict], None]) -> None:
        """Start polling Gmail IMAP for new emails."""
        if self._poll_thread is not None:
            return  # Already listening

        self._stop_event.clear()
        self._poll_thread = threading.Thread(
            target=self._poll_loop,
            args=(on_message,),
            daemon=True,
            name="gmail-imap-poll",
        )
        self._poll_thread.start()

    def stop(self) -> None:
        """Stop the IMAP polling thread."""
        self._stop_event.set()
        if self._poll_thread:
            self._poll_thread.join(timeout=5.0)
            self._poll_thread = None

    @property
    def address(self) -> str | None:
        return self._gmail_address

    # ------------------------------------------------------------------
    # State persistence (optional — set _state_file to enable)
    # ------------------------------------------------------------------

    def _load_state(self) -> None:
        if self._state_file and self._state_file.is_file():
            try:
                data = json.loads(self._state_file.read_text())
                self._processed_uids = set(data.get("processed_uids", []))
                log.info("Loaded state: %d processed UIDs", len(self._processed_uids))
            except (json.JSONDecodeError, OSError):
                pass

    def _save_state(self) -> None:
        if not self._state_file:
            return
        self._state_file.parent.mkdir(parents=True, exist_ok=True)
        uids = sorted(self._processed_uids)
        if len(uids) > 1000:
            uids = uids[-1000:]
            self._processed_uids = set(uids)
        self._state_file.write_text(json.dumps({
            "processed_uids": uids,
            "saved_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        }, indent=2))

    # ------------------------------------------------------------------
    # IMAP polling
    # ------------------------------------------------------------------

    def _poll_loop(self, on_message: Callable[[dict], None]) -> None:
        """Main polling loop — connects to IMAP, polls for unseen emails."""
        self._load_state()

        while not self._stop_event.is_set():
            try:
                imap = imaplib.IMAP4_SSL(self._imap_host, self._imap_port)
                imap.login(self._gmail_address, self._gmail_password)
                imap.select("INBOX")
                log.info("IMAP connected to %s", self._gmail_address)

                while not self._stop_event.is_set():
                    self._check_new_emails(imap, on_message)
                    self._stop_event.wait(self._poll_interval)

            except (imaplib.IMAP4.error, OSError, ConnectionError) as e:
                log.warning("IMAP error: %s. Reconnecting in 30s...", e)
                if not self._stop_event.wait(30):
                    continue
                else:
                    break
            finally:
                try:
                    imap.logout()
                except Exception:
                    pass

    def _check_new_emails(
        self, imap: imaplib.IMAP4_SSL, on_message: Callable[[dict], None]
    ) -> None:
        """Fetch unseen emails and deliver to on_message callback."""
        try:
            imap.noop()
            status, data = imap.uid("SEARCH", None, "UNSEEN")
            if status != "OK":
                return

            uids = data[0].split()
            for uid in uids:
                uid_str = uid.decode()
                if uid_str in self._processed_uids:
                    continue

                payload = self._fetch_email(imap, uid_str)
                if payload is not None:
                    on_message(payload)

                self._processed_uids.add(uid_str)

            if uids:
                self._save_state()

        except (imaplib.IMAP4.error, OSError) as e:
            log.warning("Error checking emails: %s", e)

    def _fetch_email(self, imap: imaplib.IMAP4_SSL, uid: str) -> dict | None:
        """Fetch a single email by UID and convert to StoAI payload."""
        try:
            status, data = imap.uid("FETCH", uid, "(RFC822)")
            if status != "OK" or not data[0]:
                return None

            raw = data[0][1]
            msg = email_lib.message_from_bytes(raw)

            from_addr = email_lib.utils.parseaddr(msg.get("From", ""))[1].lower()
            subject = _decode_header_value(msg.get("Subject", ""))
            body = _extract_text_body(msg)

            # Filter by allowed senders
            if self._allowed_senders and from_addr not in self._allowed_senders:
                log.info("Ignoring email from %s (not in allowed senders)", from_addr)
                return None

            log.info("New email from %s: %s", from_addr, subject)

            # Build StoAI mail payload — sender is the real email address
            return {
                "from": from_addr,
                "to": [self._gmail_address],
                "subject": subject,
                "message": body,
            }

        except Exception as e:
            log.error("Error fetching email UID %s: %s", uid, e)
            return None
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_services_gmail.py -v`
Expected: PASS

- [ ] **Step 5: Smoke-test the module**

Run: `python -c "from stoai.services.gmail import GmailMailService; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add src/stoai/services/gmail.py tests/test_services_gmail.py
git commit -m "feat: add GmailMailService — IMAP/SMTP implementation of MailService"
```

---

### Task 2: Gmail capability

**Files:**
- Create: `src/stoai/capabilities/gmail.py`
- Modify: `src/stoai/capabilities/__init__.py`
- Test: `tests/test_capability_gmail.py`

- [ ] **Step 1: Write failing test for gmail capability setup**

```python
# tests/test_capability_gmail.py
from __future__ import annotations
from unittest.mock import MagicMock, patch
from stoai.capabilities.gmail import setup


def test_gmail_capability_setup():
    """Gmail capability should set up GmailMailService and email tool."""
    agent = MagicMock()
    agent._mail_service = None
    agent._working_dir = "/tmp/test_agent"
    agent._admin = {}
    agent.agent_id = "test123"

    with patch("stoai.capabilities.gmail.GmailMailService") as MockGmail:
        mock_svc = MagicMock()
        mock_svc.address = "agent@gmail.com"
        MockGmail.return_value = mock_svc

        setup(
            agent,
            gmail_address="agent@gmail.com",
            gmail_password="xxxx",
            allowed_senders=["user@gmail.com"],
        )

    # Should have created GmailMailService
    MockGmail.assert_called_once()
    # Should have set it as the agent's mail service
    assert agent._mail_service == mock_svc
    # Should have called email setup (which calls override_intrinsic + add_tool)
    agent.override_intrinsic.assert_called_with("mail")
    agent.add_tool.assert_called_once()
    assert agent.add_tool.call_args[0][0] == "email"


def test_gmail_registered_in_builtin():
    """Gmail should be in the capabilities registry."""
    from stoai.capabilities import _BUILTIN
    assert "gmail" in _BUILTIN
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_capability_gmail.py -v`
Expected: FAIL

- [ ] **Step 3: Implement gmail capability**

```python
# src/stoai/capabilities/gmail.py
"""Gmail capability — real email via Gmail IMAP/SMTP.

Wraps the email capability with GmailMailService as the transport.
The agent gets the same `email` tool — send, check, read, reply, search, contacts.
The only difference: messages travel via Gmail instead of TCP.

Usage:
    agent = Agent(
        agent_name="myagent",
        service=llm,
        capabilities={
            "gmail": {
                "gmail_address": "agent@gmail.com",
                "gmail_password": "xxxx xxxx xxxx xxxx",
                "allowed_senders": ["you@gmail.com"],
            }
        },
    )
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

from ..services.gmail import GmailMailService
from . import email as email_capability

if TYPE_CHECKING:
    from ..base_agent import BaseAgent


def setup(
    agent: "BaseAgent",
    *,
    gmail_address: str,
    gmail_password: str,
    allowed_senders: list[str] | None = None,
    poll_interval: int = 30,
    private_mode: bool = False,
    imap_host: str = "imap.gmail.com",
    imap_port: int = 993,
    smtp_host: str = "smtp.gmail.com",
    smtp_port: int = 587,
) -> email_capability.EmailManager:
    """Set up gmail capability — real email via Gmail IMAP/SMTP.

    Creates a GmailMailService and injects it as the agent's mail service,
    then delegates to the email capability for mailbox + tool setup.
    """
    svc = GmailMailService(
        gmail_address=gmail_address,
        gmail_password=gmail_password,
        allowed_senders=allowed_senders,
        poll_interval=poll_interval,
        imap_host=imap_host,
        imap_port=imap_port,
        smtp_host=smtp_host,
        smtp_port=smtp_port,
    )

    # Persistence: store processed UIDs alongside agent's mailbox
    state_dir = Path(agent._working_dir) / "mailbox"
    state_dir.mkdir(parents=True, exist_ok=True)
    svc._state_file = state_dir / "gmail_state.json"

    # Inject as the agent's mail service
    agent._mail_service = svc

    # Delegate to email capability for mailbox, tool, and notification setup
    return email_capability.setup(agent, private_mode=private_mode)
```

- [ ] **Step 4: Register in capabilities registry**

In `src/stoai/capabilities/__init__.py`, add `"gmail": ".gmail"` to `_BUILTIN`:

```python
_BUILTIN: dict[str, str] = {
    ...
    "gmail": ".gmail",
    ...
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `python -m pytest tests/test_capability_gmail.py -v`
Expected: PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "from stoai.capabilities.gmail import setup; print('OK')"`
Expected: `OK`

- [ ] **Step 7: Commit**

```bash
git add src/stoai/capabilities/gmail.py src/stoai/capabilities/__init__.py tests/test_capability_gmail.py
git commit -m "feat: add gmail capability — real email via IMAP/SMTP"
```

---

### Task 3: Simplify app/email launcher

**Files:**
- Modify: `app/email/__main__.py`
- Delete: `app/email/bridge.py`

- [ ] **Step 1: Rewrite `__main__.py` to use gmail capability**

The launcher becomes much simpler — no bridge, no bridge port. Just an agent with `gmail` capability.

```python
# app/email/__main__.py
"""Launch a StoAI agent with Gmail — interact via real email.

Usage:
    python -m app.email

Configure via app/email/config.json (see config.example.json).
Once launched, send an email to the configured Gmail address.
The agent replies back to your email. No CLI, no web UI needed.
"""
from __future__ import annotations

import json
import logging
import os
import signal
import sys
import threading
from pathlib import Path

# Load .env from project root
env_path = Path(__file__).parent.parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

from stoai import Agent, AgentConfig
from stoai.llm import LLMService

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(name)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("app.email")

CONFIG_DIR = Path(__file__).parent
DEFAULT_PLAYGROUND = Path.home() / ".stoai" / "email"


def load_config() -> dict:
    config_file = CONFIG_DIR / "config.json"
    if not config_file.is_file():
        print(f"Error: No config.json found at {config_file}")
        print(f"Copy config.example.json and fill in your details:")
        print(f"  cp {CONFIG_DIR / 'config.example.json'} {config_file}")
        sys.exit(1)
    return json.loads(config_file.read_text())


def main():
    cfg = load_config()

    # Gmail settings
    gmail_address = cfg.get("gmail_address")
    gmail_password = cfg.get("gmail_password") or os.environ.get("GMAIL_APP_PASSWORD")
    if not gmail_address or not gmail_password:
        print("Error: gmail_address and gmail_password (or GMAIL_APP_PASSWORD env) required")
        sys.exit(1)

    allowed_senders = cfg.get("allowed_senders", [])

    # Agent settings
    agent_name = cfg.get("agent_name", "agent")
    playground = Path(cfg.get("playground", str(DEFAULT_PLAYGROUND)))
    playground.mkdir(parents=True, exist_ok=True)

    # LLM settings
    provider = cfg.get("provider", "minimax")
    model = cfg.get("model", "MiniMax-M2.5-highspeed")
    api_key_env = cfg.get("api_key_env", f"{provider.upper()}_API_KEY")
    api_key = os.environ.get(api_key_env)
    if not api_key:
        print(f"Error: {api_key_env} not set in environment or .env")
        sys.exit(1)

    base_url = cfg.get("base_url")
    max_turns = cfg.get("max_turns", 20)

    # Build capabilities — gmail replaces both email + mail service
    capabilities = cfg.get("capabilities", {})
    # Inject gmail config into capabilities
    capabilities["gmail"] = {
        "gmail_address": gmail_address,
        "gmail_password": gmail_password,
        "allowed_senders": allowed_senders or None,
        "poll_interval": cfg.get("poll_interval", 30),
    }
    # Remove plain "email" if present — gmail includes it
    capabilities.pop("email", None)

    # Covenant
    covenant = cfg.get("covenant", (
        "## Communication\n"
        "- All communication is via email. Your text responses are your private diary.\n"
        "- When you receive an email, process the request and email your reply to the sender.\n"
        "- Keep emails concise and helpful.\n"
        "- Never go back and forth with courtesy emails."
    ))

    # Character
    character = cfg.get("character", (
        "## Role\n"
        "You are a personal AI assistant reachable by email.\n"
        "You help with questions, research, writing, and analysis."
    ))

    # Build LLM service
    provider_config = {}
    if cfg.get("web_search_provider"):
        provider_config["web_search_provider"] = cfg["web_search_provider"]
    if cfg.get("vision_provider"):
        provider_config["vision_provider"] = cfg["vision_provider"]

    llm = LLMService(
        provider=provider,
        model=model,
        api_key=api_key,
        base_url=base_url,
        provider_config=provider_config,
    )

    # Write character.md if not present
    char_dir = playground / agent_name / "system"
    char_dir.mkdir(parents=True, exist_ok=True)
    char_file = char_dir / "character.md"
    if not char_file.is_file():
        char_file.write_text(character)

    # Create and start agent
    agent = Agent(
        agent_name=agent_name,
        service=llm,
        config=AgentConfig(max_turns=max_turns),
        base_dir=playground,
        streaming=True,
        covenant=covenant,
        capabilities=capabilities,
    )

    agent.start()

    print()
    print(f"  Agent:    {agent_name}")
    print(f"  Gmail:    {gmail_address}")
    if allowed_senders:
        print(f"  Accepts:  {', '.join(allowed_senders)}")
    else:
        print(f"  Accepts:  anyone")
    print(f"  Data:     {playground}")
    print()
    print("Send an email to interact. Press Ctrl+C to shut down.")
    print()

    # Block until signal
    stop_event = threading.Event()

    def on_signal(signum, frame):
        print("\nShutting down...")
        stop_event.set()

    signal.signal(signal.SIGINT, on_signal)
    signal.signal(signal.SIGTERM, on_signal)

    stop_event.wait()
    agent.stop(timeout=10.0)
    print("Done.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Delete bridge.py**

```bash
rm app/email/bridge.py
```

- [ ] **Step 3: Update config.example.json** — remove `bridge_port` and `agent_port`

- [ ] **Step 4: Smoke-test the launcher**

Run: `python -c "from app.email.__main__ import load_config; print('OK')"`
Expected: `OK`

- [ ] **Step 5: Commit**

```bash
git add app/email/__main__.py app/email/config.example.json
git rm app/email/bridge.py
git commit -m "refactor: simplify email launcher — use gmail capability, remove bridge"
```

---

### Task 4: Handle mailbox persistence in GmailMailService

**Files:**
- Modify: `src/stoai/services/gmail.py`

The `TCPMailService` persists incoming emails to `mailbox/inbox/{uuid}/message.json` inside `_handle_connection`. The `GmailMailService` needs to do the same — the email capability reads from this filesystem mailbox.

However, looking at the flow more carefully: `BaseAgent._on_mail_received` → `email.on_normal_mail` already receives the payload dict. The email capability's `on_normal_mail` generates its own `_mailbox_id` and the `TCPMailService` also writes to disk. The disk write happens in `TCPMailService._handle_connection` (before calling `on_message`), and the email capability reads from disk in `_load_email`.

**Key question:** Does the `on_message` callback need the email to already be on disk? Looking at `on_normal_mail` in `email.py:640-678` — it uses `payload.get("_mailbox_id")` and sends a notification. The actual email data lives on disk because `TCPMailService` wrote it there. When the agent later calls `email(action="read", email_id=...)`, `EmailManager._load_email` reads from `mailbox/inbox/{id}/message.json`.

So `GmailMailService` must also persist to `mailbox/inbox/{uuid}/message.json` before calling `on_message`.

- [ ] **Step 1: Write failing test**

```python
# Add to tests/test_services_gmail.py
def test_gmail_service_persists_email(tmp_path):
    """Emails should be saved to mailbox/inbox/ before on_message is called."""
    from unittest.mock import patch, MagicMock
    import imaplib

    svc = GmailMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )
    svc._working_dir = tmp_path

    received = []
    def on_msg(payload):
        # At this point, the email should be on disk
        mid = payload.get("_mailbox_id")
        assert mid is not None
        msg_file = tmp_path / "mailbox" / "inbox" / mid / "message.json"
        assert msg_file.is_file()
        received.append(payload)

    # Simulate _deliver_email
    svc._deliver_email(
        on_message=on_msg,
        from_addr="user@gmail.com",
        subject="test",
        body="hello",
    )

    assert len(received) == 1
    assert received[0]["from"] == "user@gmail.com"
```

- [ ] **Step 2: Add `_working_dir` and `_deliver_email` to GmailMailService**

Add a `_working_dir` property (set by gmail capability) and a `_deliver_email` method that persists to disk then calls `on_message`. Refactor `_fetch_email` to use `_deliver_email`.

In `GmailMailService.__init__`, add:
```python
self._working_dir: Path | None = None  # set by capability for persistence
```

Add method:
```python
def _deliver_email(
    self,
    on_message: Callable[[dict], None],
    from_addr: str,
    subject: str,
    body: str,
) -> None:
    """Persist email to mailbox and deliver to callback."""
    msg_id = str(uuid4())
    payload = {
        "from": from_addr,
        "to": [self._gmail_address],
        "subject": subject,
        "message": body,
        "_mailbox_id": msg_id,
        "received_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }

    # Persist to disk if working_dir is set
    if self._working_dir is not None:
        msg_dir = self._working_dir / "mailbox" / "inbox" / msg_id
        msg_dir.mkdir(parents=True, exist_ok=True)
        (msg_dir / "message.json").write_text(
            json.dumps(payload, indent=2, default=str)
        )

    on_message(payload)
```

Update `_check_new_emails` to use `_deliver_email` instead of building payload in `_fetch_email`.

- [ ] **Step 3: Run tests**

Run: `python -m pytest tests/test_services_gmail.py -v`
Expected: PASS

- [ ] **Step 4: Update gmail capability to set `_working_dir`**

In `src/stoai/capabilities/gmail.py`, add after creating `svc`:
```python
svc._working_dir = Path(agent._working_dir)
```

- [ ] **Step 5: Commit**

```bash
git add src/stoai/services/gmail.py src/stoai/capabilities/gmail.py tests/test_services_gmail.py
git commit -m "feat: GmailMailService persists emails to mailbox before delivery"
```

---

### Task 5: End-to-end manual test

- [ ] **Step 1: Clear old state**

```bash
rm -rf ~/.stoai/email
```

- [ ] **Step 2: Update config.json** — remove `agent_port` and `bridge_port` if present

- [ ] **Step 3: Launch**

```bash
source venv/bin/activate && python -m app.email
```

- [ ] **Step 4: Send a test email from your personal Gmail to `stoaiagent@gmail.com`**

- [ ] **Step 5: Verify in terminal:**
- `IMAP connected to stoaiagent@gmail.com`
- `New email from huangzs1997@gmail.com: <subject>`
- Agent processes and sends reply
- `Sent email to huangzs1997@gmail.com: Re: <subject>`

- [ ] **Step 6: Send a second email while agent is running — verify it picks it up within 30s**

- [ ] **Step 7: Check your Gmail inbox for the agent's reply**
