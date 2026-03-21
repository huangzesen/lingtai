# Mail/Email Refactor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the monolithic email system into a minimal `mail` intrinsic (FIFO queue + TCP transport) and a rich `email` capability (mailbox, reply, reply_all, cc, bcc).

**Architecture:** The `mail` intrinsic provides point-to-point messaging via a FIFO queue backed by `MailService`/`TCPMailService` (renamed from `EmailService`/`TCPEmailService`). The `email` capability intercepts incoming messages, stores them in a persistent mailbox, sends a notification to the mail FIFO, and exposes rich features (check-by-id, read-by-id, reply, reply_all, cc, bcc, multi-to).

**Tech Stack:** Python 3.11+, dataclasses, threading, TCP sockets, pytest

---

## File Structure

### Renamed files
| Old | New | Responsibility |
|-----|-----|---------------|
| `src/lingtai/services/email.py` | `src/lingtai/services/mail.py` | `MailService` ABC + `TCPMailService` (was `EmailService`/`TCPEmailService`) |
| `src/lingtai/intrinsics/email.py` | `src/lingtai/intrinsics/mail.py` | `mail` intrinsic schema: FIFO send/check/read |
| `tests/test_services_email.py` | `tests/test_services_mail.py` | Tests for `TCPMailService` |

### Modified files
| File | What changes |
|------|-------------|
| `src/lingtai/agent.py` | `email_service` → `mail_service`, `_email_service` → `_mail_service`, replace mailbox with FIFO `_mail_queue`, simplify `_on_mail_received` to just enqueue, strip all email methods down to FIFO send/check/read, remove reply/reply_all, rename intrinsic from `"email"` to `"mail"` |
| `src/lingtai/intrinsics/__init__.py` | `email` → `mail` in ALL_INTRINSICS |
| `src/lingtai/__init__.py` | `EmailService`/`TCPEmailService` → `MailService`/`TCPMailService` |
| `src/lingtai/capabilities/__init__.py` | Add `"email"` capability, remove `"group_chat"` |
| `tests/test_agent.py` | Update all email tests to mail FIFO tests |
| `examples/two_agents.py` | `TCPEmailService` → `TCPMailService`, `email_service=` → `mail_service=`, add `add_capability("email")` |
| `examples/chat_agent.py` | Same renames |
| `examples/chat_web.py` | Same renames |

### New files
| File | Responsibility |
|------|---------------|
| `src/lingtai/capabilities/email.py` | `email` capability: mailbox, check, read, reply, reply_all, cc, bcc, multi-to send. Intercepts mail receive to box messages. |
| `tests/test_layers_email.py` | Tests for the `email` capability |

### Deleted files
| File | Why |
|------|-----|
| `src/lingtai/services/email.py` | Replaced by `services/mail.py` |
| `src/lingtai/intrinsics/email.py` | Replaced by `intrinsics/mail.py` |
| `tests/test_services_email.py` | Replaced by `test_services_mail.py` |
| `src/lingtai/capabilities/group_chat.py` | Merged into `capabilities/email.py` |
| `tests/test_layers_group_chat.py` | Replaced by `test_layers_email.py` |

---

## Chunk 1: Rename service layer (EmailService → MailService)

### Task 1: Rename services/email.py → services/mail.py

**Files:**
- Delete: `src/lingtai/services/email.py`
- Create: `src/lingtai/services/mail.py`
- Delete: `tests/test_services_email.py`
- Create: `tests/test_services_mail.py`
- Modify: `src/lingtai/__init__.py`

- [ ] **Step 1: Create services/mail.py with renamed classes**

Copy `services/email.py` and rename:
- `EmailService` → `MailService`
- `TCPEmailService` → `TCPMailService`
- Update docstrings to say "mail" instead of "email"
- Keep the entire implementation identical otherwise

```python
# src/lingtai/services/mail.py
"""Mail service — point-to-point message transport.

MailService is the ABC. TCPMailService provides TCP transport with
JSON payload and length-prefix framing.
"""
# ... (copy full file, rename EmailService→MailService, TCPEmailService→TCPMailService)
```

- [ ] **Step 2: Create tests/test_services_mail.py**

Copy `tests/test_services_email.py`, rename all `TCPEmailService` → `TCPMailService`, update import path to `lingtai.services.mail`.

- [ ] **Step 3: Run the new tests to verify they pass**

Run: `source venv/bin/activate && python -m pytest tests/test_services_mail.py -v`
Expected: All tests PASS

- [ ] **Step 4: Update src/lingtai/__init__.py**

Change:
```python
from .services.email import EmailService, TCPEmailService
```
to:
```python
from .services.mail import MailService, TCPMailService
```

Update `__all__`:
- `"EmailService"` → `"MailService"`
- `"TCPEmailService"` → `"TCPMailService"`

- [ ] **Step 5: Delete old files**

Delete `src/lingtai/services/email.py` and `tests/test_services_email.py`.

- [ ] **Step 6: Smoke-test import**

Run: `source venv/bin/activate && python -c "from lingtai import MailService, TCPMailService; print('ok')"`
Expected: `ok`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/services/mail.py tests/test_services_mail.py src/lingtai/__init__.py
git add -u  # picks up deletions
git commit -m "refactor: rename EmailService → MailService, TCPEmailService → TCPMailService"
```

---

## Chunk 2: Rewrite mail intrinsic as FIFO

### Task 2: Create intrinsics/mail.py (FIFO schema)

**Files:**
- Delete: `src/lingtai/intrinsics/email.py`
- Create: `src/lingtai/intrinsics/mail.py`
- Modify: `src/lingtai/intrinsics/__init__.py`

- [ ] **Step 1: Create intrinsics/mail.py**

Minimal FIFO intrinsic — send, check (count queued messages), read (pop next).

```python
# src/lingtai/intrinsics/mail.py
"""Mail intrinsic — point-to-point FIFO messaging.

Actions:
    send  — fire-and-forget message to an address
    check — count messages in the queue
    read  — pop and return the next message from the queue

The actual handlers live in BaseAgent (needs access to MailService and queue).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["send", "check", "read"],
            "description": (
                "send: send a message (requires address, message; optional subject). "
                "check: count queued messages. "
                "read: pop and return the next message."
            ),
        },
        "address": {
            "type": "string",
            "description": "Target address for send (e.g. 127.0.0.1:8301)",
        },
        "subject": {"type": "string", "description": "Message subject (for send)"},
        "message": {"type": "string", "description": "Message body (for send)"},
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Point-to-point messaging. Use 'send' to message another agent, "
    "'check' to see how many messages are queued, 'read' to pop the next message."
)
```

- [ ] **Step 2: Update intrinsics/__init__.py**

Change:
```python
from . import read, edit, write, glob, grep, email, vision, web_search
```
to:
```python
from . import read, edit, write, glob, grep, mail, vision, web_search
```

Change in `ALL_INTRINSICS`:
```python
"email": {"schema": email.SCHEMA, ...}
```
to:
```python
"mail": {"schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handler": None},
```

- [ ] **Step 3: Delete intrinsics/email.py**

- [ ] **Step 4: Smoke-test import**

Run: `source venv/bin/activate && python -c "from lingtai.intrinsics import ALL_INTRINSICS; assert 'mail' in ALL_INTRINSICS; assert 'email' not in ALL_INTRINSICS; print('ok')"`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/intrinsics/mail.py src/lingtai/intrinsics/__init__.py
git add -u
git commit -m "refactor: replace email intrinsic with minimal mail FIFO intrinsic"
```

---

## Chunk 3: Rewrite agent.py mail handling (FIFO)

### Task 3: Refactor BaseAgent from email to mail FIFO

**Files:**
- Modify: `src/lingtai/agent.py`
- Modify: `tests/test_agent.py`

This is the largest task. The agent changes:
- `email_service` param → `mail_service`
- `_email_service` → `_mail_service`
- Remove `_mailbox` / `_mailbox_lock` — replace with `_mail_queue: list[dict]` and `_mail_queue_lock`
- `_handle_email` → `_handle_mail` with FIFO semantics
- `_email_send` → `_mail_send` (point-to-point only, no cc/bcc/multi-to)
- `_email_check` → `_mail_check` (just returns count)
- `_email_read` → `_mail_read` (pops next message)
- Remove: `_email_lookup`, `_email_reply`, `_email_reply_all`
- `_on_email_received` → `_on_mail_received` (appends to FIFO, notifies inbox with full content)
- Intrinsic wiring: `"email"` → `"mail"`
- Public API: `email()` → `mail()`, remove `reply()`, `reply_all()`
- `start()` / `stop()`: `_email_service` → `_mail_service`
- Docstrings: email → mail

- [ ] **Step 1: Rename all email references to mail in agent.py**

Key changes in `__init__`:
```python
# Parameter
mail_service: Any | None = None,    # was email_service

# Attribute
self._mail_service = mail_service   # was _email_service

# FIFO queue replaces mailbox
self._mail_queue: list[dict] = []   # was _mailbox
self._mail_queue_lock = threading.Lock()  # was _mailbox_lock
```

Key changes in `_wire_intrinsics`:
```python
state_intrinsics["mail"] = self._handle_mail  # was "email" / _handle_email
```

Key changes in `start()` — use lambda trampoline so capabilities can intercept `_on_mail_received` even after `start()`:
```python
if self._mail_service is not None:
    try:
        self._mail_service.listen(on_message=lambda payload: self._on_mail_received(payload))
    except RuntimeError:
        pass
```

Key changes in `stop()`:
```python
if self._mail_service is not None:
    try:
        self._mail_service.stop()
    except Exception:
        pass
```

- [ ] **Step 2: Rewrite _handle_mail and FIFO methods**

```python
def _handle_mail(self, args: dict) -> dict:
    """Handle mail tool — FIFO send, check, read."""
    action = args.get("action", "send")
    if action == "send":
        return self._mail_send(args)
    elif action == "check":
        return self._mail_check(args)
    elif action == "read":
        return self._mail_read(args)
    else:
        return {"error": f"Unknown mail action: {action}"}

def _mail_send(self, args: dict) -> dict:
    """Send a message to another agent (point-to-point)."""
    address = args.get("address", "")
    subject = args.get("subject", "")
    message_text = args.get("message", "")

    if not address:
        return {"error": "address is required"}
    if self._mail_service is None:
        return {"error": "mail service not configured"}

    payload = {
        "from": self._mail_service.address or self.agent_id,
        "to": address,
        "subject": subject,
        "message": message_text,
    }
    success = self._mail_service.send(address, payload)
    status = "delivered" if success else "refused"
    self._log("mail_sent", address=address, subject=subject, status=status, message=message_text)
    if success:
        return {"status": "delivered", "to": address}
    else:
        return {"status": "refused", "error": f"Could not deliver to {address}"}

def _mail_check(self, args: dict) -> dict:
    """Count messages in the FIFO queue."""
    with self._mail_queue_lock:
        count = len(self._mail_queue)
    return {"status": "ok", "count": count}

def _mail_read(self, args: dict) -> dict:
    """Pop and return the next message from the FIFO queue."""
    with self._mail_queue_lock:
        if not self._mail_queue:
            return {"status": "ok", "message": None, "remaining": 0}
        entry = self._mail_queue.pop(0)
        remaining = len(self._mail_queue)
    return {
        "status": "ok",
        "from": entry["from"],
        "to": entry.get("to", ""),
        "subject": entry.get("subject", ""),
        "message": entry["message"],
        "time": entry["time"],
        "remaining": remaining,
    }
```

- [ ] **Step 3: Rewrite _on_mail_received (FIFO, no mailbox)**

```python
def _on_mail_received(self, payload: dict) -> None:
    """Callback for MailService — enqueues message in FIFO, notifies agent."""
    from datetime import datetime, timezone

    sender = payload.get("from", "unknown")
    subject = payload.get("subject", "")
    message = payload.get("message", "")
    timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    entry = {
        "from": sender,
        "to": payload.get("to", ""),
        "subject": subject,
        "message": message,
        "time": timestamp,
    }
    with self._mail_queue_lock:
        self._mail_queue.append(entry)

    # Notify agent with full content inline
    notification = (
        f'[Mail from {sender}]\n'
        f'Subject: {subject}\n'
        f'{message}'
    )
    self._log("mail_received", sender=sender, subject=subject, message=message)
    msg = _make_message(MSG_REQUEST, sender, notification)
    self.inbox.put(msg)
```

- [ ] **Step 4: Update public API**

Replace:
```python
def email(self, address, message, subject="") -> dict:
```
with:
```python
def mail(self, address: str, message: str, subject: str = "") -> dict:
    """Send a message to another agent (public API). Requires MailService."""
    return self._handle_mail({"action": "send", "address": address, "message": message, "subject": subject})
```

Remove `reply()` and `reply_all()` methods.

- [ ] **Step 5: Update BaseAgent docstring**

Change line 147:
```python
- ``email`` (EmailService): Message transport — backs email intrinsic.
```
to:
```python
- ``mail_service`` (MailService): Message transport — backs mail intrinsic.
```

- [ ] **Step 6: Update tests/test_agent.py**

Rename all email tests to mail tests:
- `test_email_without_service` → `test_mail_without_service`: calls `agent.mail(...)`, expects error
- `test_email_with_service` → `test_mail_with_service`: uses `TCPMailService`, `mail_service=`
- `test_email_to_bad_address` → `test_mail_to_bad_address`: same rename
- `test_email_inbox_wiring` → `test_mail_inbox_wiring`: calls `_on_mail_received`, checks FIFO
- `test_email_start_wires_listener` → `test_mail_start_wires_listener`: uses `TCPMailService`
- `test_intrinsics_enabled_by_default`: change `"email"` assertion to `"mail"`, update comment to say `mail` instead of `email`
- `test_disabled_intrinsics`: change `{"email", "vision"}` → `{"mail", "vision"}`, assert `"mail" not in agent._intrinsics`
- `test_enabled_intrinsics`: change assertion `"email" not in` → `"mail" not in`
- `test_enabled_and_disabled_raises`: change `disabled_intrinsics={"email"}` → `disabled_intrinsics={"mail"}`

Remove tests that no longer apply:
- `test_email_multi_to` — multi-to is now email capability
- `test_email_cc_delivered_and_visible` — cc is now email capability
- `test_email_bcc_delivered_but_hidden` — bcc is now email capability
- `test_email_on_received_stores_to_and_cc` — mailbox storage is now email capability
- `test_email_check_shows_to_and_cc` — mailbox check is now email capability
- `test_email_read_shows_to_and_cc` — mailbox read is now email capability
- `test_email_reply` — reply is now email capability
- `test_email_reply_no_double_re` — reply is now email capability
- `test_email_reply_all` — reply_all is now email capability
- `test_email_reply_all_excludes_reply_to_from_duplicates` — reply_all is now email capability

Add new FIFO-specific tests:
- `test_mail_check_returns_count`: enqueue 3 messages, check returns count=3
- `test_mail_read_pops_fifo`: enqueue 2 messages, read returns first, then second
- `test_mail_read_empty_queue`: read on empty returns message=None
- `test_mail_received_full_content_in_notification`: `_on_mail_received` puts full message in inbox notification

- [ ] **Step 7: Run tests**

Run: `source venv/bin/activate && python -c "import lingtai" && python -m pytest tests/test_agent.py tests/test_services_mail.py -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/agent.py tests/test_agent.py
git commit -m "refactor: replace email with minimal mail FIFO in BaseAgent"
```

---

## Chunk 4: Create email capability

### Task 4: Create capabilities/email.py

**Files:**
- Create: `src/lingtai/capabilities/email.py`
- Delete: `src/lingtai/capabilities/group_chat.py`
- Modify: `src/lingtai/capabilities/__init__.py`
- Create: `tests/test_layers_email.py`
- Delete: `tests/test_layers_group_chat.py`

The `email` capability:
1. Takes over the receive path — swaps `agent._on_mail_received` with its own callback that stores in mailbox + sends notification to mail FIFO
2. Registers `email` tool with actions: send (multi-to, cc, bcc), check, read, reply, reply_all
3. Provides `EmailManager` with public methods

- [ ] **Step 1: Write test_layers_email.py — setup tests**

```python
# tests/test_layers_email.py
"""Tests for the email capability (mailbox, reply, cc/bcc on top of mail)."""
import socket
import threading
from unittest.mock import MagicMock

from lingtai.agent import BaseAgent
from lingtai.config import AgentConfig


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def _get_free_port():
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()
    return port


def test_email_capability_registers_tool():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = agent.add_capability("email")
    assert "email" in agent._mcp_handlers
    assert mgr is not None


def test_email_capability_adds_system_prompt():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.add_capability("email")
    section = agent._prompt_manager.read_section("email_instructions")
    assert section is not None
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `source venv/bin/activate && python -m pytest tests/test_layers_email.py -v`
Expected: FAIL (email capability doesn't exist yet)

- [ ] **Step 3: Write capabilities/email.py — skeleton with setup()**

```python
# src/lingtai/capabilities/email.py
"""Email capability — mailbox, reply, reply_all, CC/BCC on top of mail.

Upgrades the base mail intrinsic (FIFO queue) with:
- Persistent mailbox (stored messages with IDs)
- check: list mailbox with filtering
- read: read specific message by ID
- reply: auto-fill address/subject from original
- reply_all: reply to all recipients minus self
- send: multi-to with CC/BCC
- Receive interception: boxes incoming messages, notifies mail FIFO

Usage:
    agent.add_capability("email")
"""
from __future__ import annotations

import threading
from datetime import datetime, timezone
from typing import TYPE_CHECKING
from uuid import uuid4

if TYPE_CHECKING:
    from ..agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["send", "check", "read", "reply", "reply_all"],
            "description": (
                "send: send with optional cc/bcc (requires address, message). "
                "check: list mailbox (optional n for max recent). "
                "read: read email by ID (requires email_id). "
                "reply: reply to email (requires email_id, message). "
                "reply_all: reply to all recipients (requires email_id, message)."
            ),
        },
        "address": {
            "oneOf": [
                {"type": "string"},
                {"type": "array", "items": {"type": "string"}},
            ],
            "description": "Target address(es) for send",
        },
        "cc": {
            "type": "array",
            "items": {"type": "string"},
            "description": "CC addresses — visible to all recipients",
        },
        "bcc": {
            "type": "array",
            "items": {"type": "string"},
            "description": "BCC addresses — hidden from other recipients",
        },
        "subject": {"type": "string", "description": "Email subject line"},
        "message": {"type": "string", "description": "Email body"},
        "email_id": {
            "type": "string",
            "description": "Email ID for read/reply/reply_all (get from check)",
        },
        "n": {
            "type": "integer",
            "description": "Max recent emails to show (for check)",
            "default": 10,
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Full email client — mailbox with persistent storage, reply, reply-all, "
    "CC/BCC. Use 'send' for multi-recipient email, 'check' to list mailbox, "
    "'read' to read by ID, 'reply'/'reply_all' for conversation."
)


class EmailManager:
    """Full email manager — mailbox, reply, CC/BCC on top of base mail."""

    def __init__(self, agent: "BaseAgent"):
        self._agent = agent
        self._mailbox: list[dict] = []
        self._mailbox_lock = threading.Lock()

    def handle(self, args: dict) -> dict:
        action = args.get("action")
        if action == "send":
            return self._send(args)
        elif action == "check":
            return self._check(args)
        elif action == "read":
            return self._read(args)
        elif action == "reply":
            return self._reply(args)
        elif action == "reply_all":
            return self._reply_all(args)
        else:
            return {"error": f"Unknown email action: {action}"}

    # --- Send with multi-to, CC, BCC ---

    def _send(self, args: dict) -> dict:
        raw_address = args.get("address", "")
        subject = args.get("subject", "")
        message_text = args.get("message", "")
        cc = args.get("cc") or []
        bcc = args.get("bcc") or []

        if isinstance(raw_address, str):
            to_list = [raw_address] if raw_address else []
        else:
            to_list = list(raw_address)

        if not to_list:
            return {"error": "address is required"}
        if self._agent._mail_service is None:
            return {"error": "mail service not configured"}

        sender = self._agent._mail_service.address or self._agent.agent_id

        base_payload = {
            "from": sender,
            "to": to_list,
            "subject": subject,
            "message": message_text,
        }
        if cc:
            base_payload["cc"] = cc

        all_recipients = to_list + cc + bcc
        delivered = []
        refused = []
        for addr in all_recipients:
            ok = self._agent._mail_service.send(addr, base_payload)
            if ok:
                delivered.append(addr)
            else:
                refused.append(addr)

        self._agent._log(
            "email_sent", to=to_list, cc=cc, bcc=bcc,
            subject=subject, message=message_text,
            delivered=delivered, refused=refused,
        )

        if not refused:
            return {"status": "delivered", "to": to_list, "cc": cc, "bcc": bcc}
        elif not delivered:
            return {"status": "refused", "error": "Could not deliver to any recipient", "refused": refused}
        else:
            return {"status": "partial", "delivered": delivered, "refused": refused}

    # --- Mailbox: check, read ---

    def _check(self, args: dict) -> dict:
        n = args.get("n", 10)
        with self._mailbox_lock:
            total = len(self._mailbox)
            recent = self._mailbox[-n:] if n > 0 else self._mailbox[:]
        emails = []
        for e in reversed(recent):
            entry = {
                "id": e["id"],
                "from": e["from"],
                "to": e.get("to", []),
                "subject": e.get("subject", "(no subject)"),
                "preview": e["message"][:100],
                "time": e["time"],
                "unread": e.get("unread", False),
            }
            if e.get("cc"):
                entry["cc"] = e["cc"]
            emails.append(entry)
        return {"status": "ok", "total": total, "showing": len(emails), "emails": emails}

    def _read(self, args: dict) -> dict:
        email_id = args.get("email_id", "")
        if not email_id:
            return {"error": "email_id is required"}
        with self._mailbox_lock:
            for e in self._mailbox:
                if e["id"] == email_id:
                    e["unread"] = False
                    result = {
                        "status": "ok",
                        "id": e["id"],
                        "from": e["from"],
                        "to": e.get("to", []),
                        "subject": e.get("subject", "(no subject)"),
                        "message": e["message"],
                        "time": e["time"],
                    }
                    if e.get("cc"):
                        result["cc"] = e["cc"]
                    return result
        return {"error": f"Email not found: {email_id}"}

    # --- Reply, Reply All ---

    def _lookup(self, email_id: str) -> dict | None:
        with self._mailbox_lock:
            for e in self._mailbox:
                if e["id"] == email_id:
                    return dict(e)
        return None

    def _reply(self, args: dict) -> dict:
        email_id = args.get("email_id", "")
        if not email_id:
            return {"error": "email_id is required for reply"}
        message_text = args.get("message", "")
        if not message_text:
            return {"error": "message is required for reply"}

        original = self._lookup(email_id)
        if original is None:
            return {"error": f"Email not found: {email_id}"}

        orig_subject = original.get("subject", "")
        subject = args.get("subject") or (
            orig_subject if orig_subject.startswith("Re: ") else f"Re: {orig_subject}"
        )

        return self._send({
            "address": original["from"],
            "subject": subject,
            "message": message_text,
            "cc": args.get("cc") or [],
            "bcc": args.get("bcc") or [],
        })

    def _reply_all(self, args: dict) -> dict:
        email_id = args.get("email_id", "")
        if not email_id:
            return {"error": "email_id is required for reply_all"}
        message_text = args.get("message", "")
        if not message_text:
            return {"error": "message is required for reply_all"}

        original = self._lookup(email_id)
        if original is None:
            return {"error": f"Email not found: {email_id}"}

        my_address = (
            self._agent._mail_service.address
            if self._agent._mail_service
            else self._agent.agent_id
        )

        reply_to = original["from"]
        orig_to = original.get("to") or []
        if isinstance(orig_to, str):
            orig_to = [orig_to]
        orig_cc = original.get("cc") or []
        other_recipients = [
            addr for addr in orig_to + orig_cc
            if addr != my_address and addr != reply_to
        ]

        extra_cc = args.get("cc") or []
        extra_bcc = args.get("bcc") or []

        orig_subject = original.get("subject", "")
        subject = args.get("subject") or (
            orig_subject if orig_subject.startswith("Re: ") else f"Re: {orig_subject}"
        )

        return self._send({
            "address": reply_to,
            "subject": subject,
            "message": message_text,
            "cc": other_recipients + extra_cc,
            "bcc": extra_bcc,
        })

    # --- Receive interception ---

    def on_mail_received(self, payload: dict) -> None:
        """Intercept incoming mail — store in mailbox, notify via mail FIFO."""
        sender = payload.get("from", "unknown")
        to = payload.get("to") or []
        if isinstance(to, str):
            to = [to]
        cc = payload.get("cc") or []
        subject = payload.get("subject", "(no subject)")
        message = payload.get("message", "")
        email_id = f"mail_{uuid4().hex[:8]}"
        timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

        email_entry = {
            "id": email_id,
            "from": sender,
            "to": to,
            "subject": subject,
            "message": message,
            "time": timestamp,
            "unread": True,
        }
        if cc:
            email_entry["cc"] = cc
        with self._mailbox_lock:
            self._mailbox.append(email_entry)

        # Send notification to agent's mail FIFO (not the full content)
        preview = message[:80].replace("\n", " ")
        notification = (
            f'[New email from {sender}]\n'
            f'  Subject: {subject}\n'
            f'  Preview: {preview}...\n'
            f'  ID: {email_id}\n'
            f'Use email(action="read", email_id="{email_id}") to read full message. '
            f'Use email(action="reply", email_id="{email_id}", message="...") to reply.'
        )

        # Put notification into the agent's mail FIFO
        from ..agent import _make_message, MSG_REQUEST
        self._agent._log("email_received", sender=sender, to=to, cc=cc, subject=subject, message=message)
        msg = _make_message(MSG_REQUEST, sender, notification)
        self._agent.inbox.put(msg)
```

- [ ] **Step 4: Write setup() function**

```python
def setup(agent: "BaseAgent") -> EmailManager:
    """Set up email capability — mailbox, reply, CC/BCC on top of mail.

    Intercepts the agent's mail receive callback to box messages.
    """
    mgr = EmailManager(agent)

    # Intercept the mail receive path
    agent._on_mail_received = mgr.on_mail_received

    agent.add_tool(
        "email", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )
    agent.update_system_prompt(
        "email_instructions",
        "You have full email capabilities via the email tool. "
        "Use 'send' with CC/BCC for group messaging, "
        "'check' to list your mailbox, 'read' to read by ID, "
        "'reply' to respond, 'reply_all' to respond to everyone. "
        "For simple point-to-point messages, the mail tool also works.",
    )
    return mgr
```

- [ ] **Step 5: Update capabilities/__init__.py**

Change registry:
```python
_BUILTIN: dict[str, str] = {
    "bash": ".bash",
    "delegate": ".delegate",
    "email": ".email",
}
```

Remove `"group_chat"` entry.

- [ ] **Step 6: Delete old files**

Delete `src/lingtai/capabilities/group_chat.py` and `tests/test_layers_group_chat.py`.

- [ ] **Step 7: Write remaining tests in test_layers_email.py**

Add tests for:
- `test_email_receive_intercept`: call `mgr.on_mail_received(payload)` directly, verify message stored in `mgr._mailbox` and notification put in `agent.inbox`. Do NOT require TCP — test the interception logic directly:
  ```python
  def test_email_receive_intercept():
      agent = BaseAgent(agent_id="test", service=make_mock_service())
      mgr = agent.add_capability("email")
      mgr.on_mail_received({"from": "sender", "to": ["test"], "subject": "hi", "message": "body"})
      assert len(mgr._mailbox) == 1
      assert mgr._mailbox[0]["from"] == "sender"
      assert not agent.inbox.empty()
      notification = agent.inbox.get_nowait()
      assert "hi" in notification.content  # subject in notification
  ```
- `test_email_check_mailbox`: receive 2 emails, check returns both with IDs
- `test_email_read_by_id`: receive email, read by ID returns full content
- `test_email_send_multi_to`: send to multiple addresses
- `test_email_send_cc_visible`: CC addresses in payload
- `test_email_send_bcc_hidden`: BCC not in payload
- `test_email_reply`: auto-fills address, Re: prefix
- `test_email_reply_no_double_re`: doesn't stack Re: Re:
- `test_email_reply_all`: sends to sender + all original recipients minus self
- `test_email_reply_all_excludes_self`: self not in recipients
- `test_email_without_mail_service`: returns error

- [ ] **Step 8: Run all tests**

Run: `source venv/bin/activate && python -c "import lingtai" && python -m pytest tests/ -v`
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add src/lingtai/capabilities/email.py src/lingtai/capabilities/__init__.py tests/test_layers_email.py
git add -u
git commit -m "feat: add email capability (mailbox, reply, cc/bcc) on top of mail intrinsic"
```

---

## Chunk 5: Update examples and exports

### Task 5: Update examples and __init__.py

**Files:**
- Modify: `examples/two_agents.py`
- Modify: `examples/chat_agent.py`
- Modify: `examples/chat_web.py`
- Modify: `src/lingtai/__init__.py`
- Modify: `src/lingtai/capabilities/delegate.py` (comment only)

- [ ] **Step 1: Update examples/two_agents.py**

- `from lingtai.services.email import TCPEmailService` → `from lingtai.services.mail import TCPMailService`
- `TCPEmailService(...)` → `TCPMailService(...)`
- `email_service=email_a` → `mail_service=email_a` (keep var names or rename to `mail_a`/`mail_b` for clarity)
- After agent creation, add `agent_a.add_capability("email")` and `agent_b.add_capability("email")`
- In `ChatHandler.do_POST`, `TCPEmailService()` → `TCPMailService()`
- In diary handler, keep the `email_sent`/`email_received` log event handling (those come from the email capability now)

- [ ] **Step 2: Update examples/chat_agent.py**

Same pattern: rename imports, constructor params, add `agent.add_capability("email")`.

- [ ] **Step 3: Update examples/chat_web.py**

Same pattern.

- [ ] **Step 4: Update src/lingtai/__init__.py exports**

Add `EmailManager` to imports and `__all__`:
```python
from .capabilities.email import EmailManager
```

Remove `GroupChatManager` if it was exported (it wasn't).

- [ ] **Step 5: Update delegate.py comment**

Change comment referencing EmailService to MailService.

- [ ] **Step 6: Run full test suite**

Run: `source venv/bin/activate && python -c "import lingtai" && python -m pytest tests/ -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add examples/ src/lingtai/__init__.py src/lingtai/capabilities/delegate.py
git commit -m "chore: update examples and exports for mail/email refactor"
```

---

## Chunk 6: Final cleanup and verification

### Task 6: Final verification

- [ ] **Step 1: Verify no stale email references**

Run: `grep -r "EmailService\|TCPEmailService\|email_service" src/ tests/ examples/ --include="*.py" | grep -v __pycache__`
Expected: No matches (or only in comments/strings that are correct)

- [ ] **Step 2: Run full test suite**

Run: `source venv/bin/activate && python -m pytest tests/ -v`
Expected: All PASS

- [ ] **Step 3: Smoke-test imports**

```bash
python -c "
from lingtai import MailService, TCPMailService, EmailManager
from lingtai.services.mail import MailService, TCPMailService
from lingtai.capabilities.email import EmailManager
from lingtai.intrinsics import ALL_INTRINSICS
assert 'mail' in ALL_INTRINSICS
assert 'email' not in ALL_INTRINSICS
print('All imports OK')
"
```

- [ ] **Step 4: Verify architecture**

Confirm the layering:
- `agent.mail("addr", "msg")` — uses mail FIFO intrinsic only
- `agent.add_capability("email")` — adds mailbox, check, read, reply, reply_all, cc, bcc
- Incoming messages without email capability → full content in inbox notification
- Incoming messages with email capability → stored in mailbox, short notification in inbox

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "refactor: complete mail/email split — FIFO intrinsic + email capability"
```
