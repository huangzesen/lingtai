# Filesystem-based Email Mailbox Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the in-memory email mailbox with filesystem-based storage, add sent email persistence, and add search.

**Architecture:** The mail service (`services/mail.py`) writes received emails to `mailbox/inbox/{uuid}/`. The email capability (`capabilities/email.py`) reads from the filesystem instead of an in-memory list, writes sent emails to `mailbox/sent/{uuid}/`, and tracks read state in `mailbox/read.json`. Search scans `message.json` files with Python regex.

**Tech Stack:** Python stdlib (json, re, pathlib, uuid, datetime). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-03-15-filesystem-email-mailbox-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai/services/mail.py` | Modify | Change persist path to `mailbox/inbox/`, inject `_mailbox_id` + `received_at` into payload |
| `src/lingtai/capabilities/email.py` | Rewrite | Remove in-memory mailbox, read/write filesystem, add search action |
| `tests/test_layers_email.py` | Rewrite | All tests use `tmp_path` filesystem fixtures instead of in-memory access |
| `tests/test_mail_service.py` | Modify | Update persist path assertions from `mailbox/` to `mailbox/inbox/` |

---

## Chunk 1: Mail Service Changes

### Task 1: Update TCPMailService persist path and inject metadata

**Files:**
- Modify: `src/lingtai/services/mail.py:170-191` (the `_handle_connection` persist block)
- Test: `tests/test_mail_service.py` (if exists, update path assertions)

- [ ] **Step 1: Update persist path from `mailbox/` to `mailbox/inbox/`**

In `_handle_connection`, change line 173:
```python
# Before:
msg_dir = self._working_dir / "mailbox" / msg_id
# After:
msg_dir = self._working_dir / "mailbox" / "inbox" / msg_id
```

- [ ] **Step 2: Inject `_mailbox_id` and `received_at` into payload**

After generating `msg_id` and before writing `message.json`, inject metadata:
```python
from datetime import datetime, timezone

msg_id = str(uuid.uuid4())
msg_dir = self._working_dir / "mailbox" / "inbox" / msg_id

# Inject metadata so email capability can use the directory ID
payload["_mailbox_id"] = msg_id
payload["received_at"] = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
```

The `message.json` write already serializes the full payload, so these fields will be persisted automatically.

- [ ] **Step 3: Run existing mail service tests**

Run: `pytest tests/test_mail_service.py -v` (if file exists)
Run: `pytest tests/ -x -q`
Fix any path-related failures.

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/services/mail.py tests/
git commit -m "refactor: mail service persists to mailbox/inbox/, injects _mailbox_id + received_at"
```

---

## Chunk 2: Email Capability — Core Filesystem Operations

### Task 2: Rewrite EmailManager to use filesystem

**Files:**
- Rewrite: `src/lingtai/capabilities/email.py`

This is the core rewrite. Replace the entire `EmailManager` class. The new class has no in-memory state — all operations go through the filesystem.

- [ ] **Step 1: Update imports and SCHEMA**

Remove `threading` import. Add `import json`, `import re`, `import os`, `import tempfile`. Update SCHEMA to add `query` and `folder` properties, update action descriptions, update DESCRIPTION.

```python
"""Email capability — filesystem-based mailbox with search.

Storage layout:
    working_dir/mailbox/inbox/{uuid}/message.json   — received
    working_dir/mailbox/sent/{uuid}/message.json     — sent
    working_dir/mailbox/read.json                    — read tracking
"""
from __future__ import annotations

import json
import os
import re
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING
from uuid import uuid4

if TYPE_CHECKING:
    from ..agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["send", "check", "read", "reply", "reply_all", "search"],
            "description": (
                "send: send with optional cc/bcc (requires address, message). "
                "check: list mailbox (optional folder, n). "
                "read: read email by ID (requires email_id). "
                "reply: reply to email (requires email_id, message). "
                "reply_all: reply to all recipients (requires email_id, message). "
                "search: regex search mailbox (requires query, optional folder)."
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
        "attachments": {
            "type": "array",
            "items": {"type": "string"},
            "description": "File paths to attach (for send)",
        },
        "subject": {"type": "string", "description": "Email subject line"},
        "message": {"type": "string", "description": "Email body"},
        "email_id": {
            "type": "string",
            "description": "Email ID for read/reply/reply_all (get from check)",
        },
        "n": {
            "type": "integer",
            "description": "Max recent emails to show (for check, default 10)",
            "default": 10,
        },
        "query": {
            "type": "string",
            "description": "Regex pattern for search (matches from, subject, message)",
        },
        "folder": {
            "type": "string",
            "enum": ["inbox", "sent"],
            "description": "Folder for check/search. Default: inbox for check, both for search.",
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Full email client — filesystem-based mailbox with inbox/sent folders, "
    "reply, reply-all, CC/BCC, attachments, and regex search. "
    "Use 'send' for outgoing email (saved to sent/). "
    "'check' to list inbox or sent (optional folder param). "
    "'read' to read by ID. "
    "'reply'/'reply_all' to respond. "
    "'search' to find emails by regex (searches from, subject, message). "
    "Attachments are stored alongside emails in the mailbox."
)
```

- [ ] **Step 2: Write the new EmailManager class**

```python
class EmailManager:
    """Filesystem-based email manager — reads/writes mailbox/ directory."""

    def __init__(self, agent: "BaseAgent"):
        self._agent = agent

    @property
    def _mailbox_dir(self) -> Path:
        return self._agent._working_dir / "mailbox"

    # ------------------------------------------------------------------
    # Filesystem helpers
    # ------------------------------------------------------------------

    def _load_email(self, email_id: str) -> dict | None:
        """Load a single email by ID. Checks inbox/ then sent/."""
        for folder in ("inbox", "sent"):
            path = self._mailbox_dir / folder / email_id / "message.json"
            if path.is_file():
                data = json.loads(path.read_text())
                data["_folder"] = folder
                return data
        return None

    def _list_emails(self, folder: str) -> list[dict]:
        """Load all emails from a folder, sorted by time (newest first)."""
        folder_dir = self._mailbox_dir / folder
        if not folder_dir.is_dir():
            return []
        emails = []
        for msg_dir in folder_dir.iterdir():
            msg_file = msg_dir / "message.json"
            if msg_dir.is_dir() and msg_file.is_file():
                try:
                    data = json.loads(msg_file.read_text())
                    data["_folder"] = folder
                    data.setdefault("_mailbox_id", msg_dir.name)
                    emails.append(data)
                except (json.JSONDecodeError, OSError):
                    continue
        # Sort by timestamp — received_at for inbox, sent_at for sent
        def sort_key(e):
            return e.get("received_at") or e.get("sent_at") or e.get("time") or ""
        emails.sort(key=sort_key, reverse=True)
        return emails

    def _read_ids(self) -> set[str]:
        """Load the set of read email IDs from read.json."""
        path = self._mailbox_dir / "read.json"
        if path.is_file():
            try:
                return set(json.loads(path.read_text()))
            except (json.JSONDecodeError, OSError):
                return set()
        return set()

    def _mark_read(self, email_id: str) -> None:
        """Add email_id to read.json with atomic write."""
        ids = self._read_ids()
        ids.add(email_id)
        self._mailbox_dir.mkdir(parents=True, exist_ok=True)
        target = self._mailbox_dir / "read.json"
        fd, tmp = tempfile.mkstemp(dir=str(self._mailbox_dir), suffix=".tmp")
        try:
            os.write(fd, json.dumps(sorted(ids)).encode())
            os.close(fd)
            os.replace(tmp, str(target))
        except Exception:
            os.close(fd) if not os.get_inheritable(fd) else None
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise

    def _email_summary(self, e: dict, read_ids: set[str] | None = None) -> dict:
        """Build a summary dict from a raw email dict."""
        eid = e.get("_mailbox_id", "")
        if read_ids is None:
            read_ids = self._read_ids()
        entry = {
            "id": eid,
            "from": e.get("from", ""),
            "to": e.get("to", []),
            "subject": e.get("subject", "(no subject)"),
            "preview": e.get("message", "")[:100],
            "time": e.get("received_at") or e.get("sent_at") or e.get("time") or "",
            "folder": e.get("_folder", ""),
        }
        # Only track unread for inbox
        if e.get("_folder") == "inbox":
            entry["unread"] = eid not in read_ids
        if e.get("cc"):
            entry["cc"] = e["cc"]
        return entry

    # ------------------------------------------------------------------
    # Action dispatch
    # ------------------------------------------------------------------

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
        elif action == "search":
            return self._search(args)
        else:
            return {"error": f"Unknown email action: {action}"}

    # ------------------------------------------------------------------
    # Send — deliver + save to sent/
    # ------------------------------------------------------------------

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

        # Build visible payload (no bcc field)
        base_payload = {
            "from": sender,
            "to": to_list,
            "subject": subject,
            "message": message_text,
        }
        if cc:
            base_payload["cc"] = cc
        attachments = args.get("attachments", [])
        if attachments:
            base_payload["attachments"] = attachments

        # Deliver to all recipients: to + cc + bcc
        all_recipients = to_list + cc + bcc
        delivered = []
        refused = []
        for addr in all_recipients:
            ok = self._agent._mail_service.send(addr, base_payload)
            if ok:
                delivered.append(addr)
            else:
                refused.append(addr)

        # Save to sent/ (includes bcc for sender's records)
        sent_id = str(uuid4())
        sent_dir = self._mailbox_dir / "sent" / sent_id
        sent_dir.mkdir(parents=True, exist_ok=True)
        sent_record = {
            **base_payload,
            "_mailbox_id": sent_id,
            "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        }
        if bcc:
            sent_record["bcc"] = bcc
        (sent_dir / "message.json").write_text(
            json.dumps(sent_record, indent=2, default=str)
        )

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

    # ------------------------------------------------------------------
    # Check — list emails from a folder
    # ------------------------------------------------------------------

    def _check(self, args: dict) -> dict:
        folder = args.get("folder", "inbox")
        n = args.get("n", 10)
        emails = self._list_emails(folder)
        total = len(emails)
        recent = emails[:n] if n > 0 else emails
        read_ids = self._read_ids()
        summaries = [self._email_summary(e, read_ids) for e in recent]
        return {"status": "ok", "total": total, "showing": len(summaries), "emails": summaries}

    # ------------------------------------------------------------------
    # Read — load full email by ID
    # ------------------------------------------------------------------

    def _read(self, args: dict) -> dict:
        email_id = args.get("email_id", "")
        if not email_id:
            return {"error": "email_id is required"}

        data = self._load_email(email_id)
        if data is None:
            return {"error": f"Email not found: {email_id}"}

        # Mark as read (inbox only)
        if data.get("_folder") == "inbox":
            self._mark_read(email_id)

        result = {
            "status": "ok",
            "id": email_id,
            "from": data.get("from", ""),
            "to": data.get("to", []),
            "subject": data.get("subject", "(no subject)"),
            "message": data.get("message", ""),
            "time": data.get("received_at") or data.get("sent_at") or data.get("time") or "",
            "folder": data.get("_folder", ""),
        }
        if data.get("cc"):
            result["cc"] = data["cc"]
        if data.get("attachments"):
            result["attachments"] = data["attachments"]
        return result

    # ------------------------------------------------------------------
    # Lookup — used by reply/reply_all
    # ------------------------------------------------------------------

    def _lookup(self, email_id: str) -> dict | None:
        return self._load_email(email_id)

    # ------------------------------------------------------------------
    # Reply, Reply All (unchanged logic, just uses _lookup)
    # ------------------------------------------------------------------

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

    # ------------------------------------------------------------------
    # Search
    # ------------------------------------------------------------------

    def _search(self, args: dict) -> dict:
        query = args.get("query", "")
        if not query:
            return {"error": "query is required for search"}

        folder = args.get("folder")
        folders = [folder] if folder else ["inbox", "sent"]

        try:
            pattern = re.compile(query, re.IGNORECASE)
        except re.error as e:
            return {"error": f"Invalid regex: {e}"}

        matches = []
        read_ids = self._read_ids()
        for f in folders:
            for email in self._list_emails(f):
                searchable = " ".join([
                    email.get("from", ""),
                    email.get("subject", ""),
                    email.get("message", ""),
                ])
                if pattern.search(searchable):
                    matches.append(self._email_summary(email, read_ids))

        return {"status": "ok", "total": len(matches), "emails": matches}

    # ------------------------------------------------------------------
    # Receive interception
    # ------------------------------------------------------------------

    def on_mail_received(self, payload: dict) -> None:
        """Intercept incoming mail — send notification to agent inbox.

        The mail service already wrote message.json to disk.
        We just need to extract the ID and notify.
        """
        # Use _mailbox_id from mail service, or fall back to generating one
        email_id = payload.get("_mailbox_id") or str(uuid4())

        sender = payload.get("from", "unknown")
        to = payload.get("to") or []
        if isinstance(to, str):
            to = [to]
        cc = payload.get("cc") or []
        subject = payload.get("subject", "(no subject)")
        message = payload.get("message", "")

        # Send notification to agent inbox (not the full content)
        preview = message[:80].replace("\n", " ")
        notification = (
            f'[New email from {sender}]\n'
            f'  Subject: {subject}\n'
            f'  Preview: {preview}...\n'
            f'  ID: {email_id}\n'
            f'Use email(action="read", email_id="{email_id}") to read full message. '
            f'Use email(action="reply", email_id="{email_id}", message="...") to reply.'
        )

        from ..agent import _make_message, MSG_REQUEST
        self._agent._log("email_received", sender=sender, to=to, cc=cc, subject=subject, message=message)
        msg = _make_message(MSG_REQUEST, sender, notification)
        self._agent.inbox.put(msg)


def setup(agent: "BaseAgent") -> EmailManager:
    """Set up email capability — filesystem-based mailbox."""
    mgr = EmailManager(agent)
    agent._on_mail_received = mgr.on_mail_received
    agent.add_tool(
        "email", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )
    return mgr
```

- [ ] **Step 3: Smoke-test the import**

Run: `python -c "import lingtai"`

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/capabilities/email.py
git commit -m "refactor: rewrite email capability to use filesystem-based mailbox"
```

---

## Chunk 3: Test Rewrite

### Task 3: Rewrite all email tests for filesystem-based mailbox

**Files:**
- Rewrite: `tests/test_layers_email.py`

All tests need `tmp_path` fixtures. Helper functions create `message.json` files on disk to simulate received emails (what the mail service would do). No more `mgr._mailbox` access.

- [ ] **Step 1: Write test helpers**

```python
"""Tests for the email capability (filesystem-based mailbox)."""
import json
import socket
import threading
from pathlib import Path
from unittest.mock import MagicMock
from uuid import uuid4
from datetime import datetime, timezone

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


def _make_inbox_email(tmp_path, *, sender="sender", to=None, subject="test",
                       message="body", cc=None, attachments=None):
    """Create an email on disk in mailbox/inbox/{uuid}/message.json.
    Returns the email_id (directory name)."""
    email_id = str(uuid4())
    msg_dir = tmp_path / "mailbox" / "inbox" / email_id
    msg_dir.mkdir(parents=True, exist_ok=True)
    data = {
        "_mailbox_id": email_id,
        "from": sender,
        "to": to or ["test"],
        "subject": subject,
        "message": message,
        "received_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    if cc:
        data["cc"] = cc
    if attachments:
        data["attachments"] = attachments
    (msg_dir / "message.json").write_text(json.dumps(data, indent=2))
    return email_id
```

- [ ] **Step 2: Write setup and receive tests**

```python
def test_email_capability_registers_tool(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    assert "email" in agent._mcp_handlers
    assert "email" in [s.name for s in agent._mcp_schemas]
    assert mgr is not None


def test_email_receive_notification(tmp_path):
    """on_mail_received should send notification to agent inbox."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    mgr.on_mail_received({
        "_mailbox_id": "abc123",
        "from": "sender",
        "to": ["test"],
        "subject": "hi",
        "message": "body",
    })
    assert not agent.inbox.empty()
    notification = agent.inbox.get_nowait()
    assert "hi" in notification.content
    assert "abc123" in notification.content


def test_email_receive_fallback_id(tmp_path):
    """on_mail_received should generate ID if _mailbox_id is absent."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    mgr.on_mail_received({"from": "sender", "message": "body"})
    assert not agent.inbox.empty()
```

- [ ] **Step 3: Write check and read tests**

```python
def test_email_check_inbox(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    eid1 = _make_inbox_email(tmp_path, sender="a", subject="s1", message="m1")
    eid2 = _make_inbox_email(tmp_path, sender="b", subject="s2", message="m2")
    result = mgr.handle({"action": "check"})
    assert result["status"] == "ok"
    assert result["total"] == 2
    assert all("id" in e for e in result["emails"])


def test_email_check_sent(tmp_path):
    """check with folder=sent should show sent emails."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    mgr.handle({"action": "send", "address": "someone", "message": "hello", "subject": "test"})
    result = mgr.handle({"action": "check", "folder": "sent"})
    assert result["total"] == 1
    assert result["emails"][0]["from"] == "me"


def test_email_check_empty_mailbox(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    result = mgr.handle({"action": "check"})
    assert result["status"] == "ok"
    assert result["total"] == 0


def test_email_read_by_id(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(tmp_path, sender="sender", subject="topic", message="full body")
    result = mgr.handle({"action": "read", "email_id": eid})
    assert result["status"] == "ok"
    assert result["message"] == "full body"
    assert result["subject"] == "topic"


def test_email_read_marks_as_read(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(tmp_path, message="m")
    # First check — should be unread
    result = mgr.handle({"action": "check"})
    assert result["emails"][0]["unread"] is True
    # Read it
    mgr.handle({"action": "read", "email_id": eid})
    # Now should be read
    result = mgr.handle({"action": "check"})
    assert result["emails"][0]["unread"] is False


def test_email_read_shows_attachments(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(tmp_path, subject="photo", message="look",
                            attachments=["/path/to/photo.png"])
    result = mgr.handle({"action": "read", "email_id": eid})
    assert result["status"] == "ok"
    assert "attachments" in result
    assert any("photo.png" in p for p in result["attachments"])
```

- [ ] **Step 4: Write send tests (sent folder persistence)**

```python
def test_email_send_saves_to_sent(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "hello", "subject": "test",
    })
    assert result["status"] == "delivered"
    sent_dir = tmp_path / "mailbox" / "sent"
    assert sent_dir.is_dir()
    sent_emails = list(sent_dir.iterdir())
    assert len(sent_emails) == 1
    msg = json.loads((sent_emails[0] / "message.json").read_text())
    assert msg["message"] == "hello"
    assert msg["sent_at"]  # timestamp present


def test_email_send_saves_bcc_in_sent(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    mgr.handle({
        "action": "send", "address": "someone",
        "message": "secret", "bcc": ["hidden"],
    })
    sent_dir = tmp_path / "mailbox" / "sent"
    msg = json.loads(list((sent_dir).iterdir())[0].joinpath("message.json").read_text())
    assert msg["bcc"] == ["hidden"]
```

- [ ] **Step 5: Write reply tests**

```python
def test_email_reply(tmp_path):
    agent = BaseAgent(agent_id="replier", service=make_mock_service(), working_dir=tmp_path)
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(tmp_path, sender="alice", subject="Original topic", message="Please respond")
    result = mgr.handle({"action": "reply", "email_id": eid, "message": "Here is my reply"})
    assert result["status"] == "delivered"
    sent_payload = mock_svc.send.call_args[0][1]
    assert sent_payload["subject"] == "Re: Original topic"


def test_email_reply_no_double_re(tmp_path):
    agent = BaseAgent(agent_id="replier", service=make_mock_service(), working_dir=tmp_path)
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(tmp_path, sender="other", subject="Re: Already replied", message="text")
    result = mgr.handle({"action": "reply", "email_id": eid, "message": "follow up"})
    sent_payload = mock_svc.send.call_args[0][1]
    assert sent_payload["subject"] == "Re: Already replied"


def test_email_reply_all(tmp_path):
    agent = BaseAgent(agent_id="replier", service=make_mock_service(), working_dir=tmp_path)
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(tmp_path, sender="alice", to=["me", "bob"],
                            cc=["charlie"], subject="Group thread", message="discussion")
    result = mgr.handle({"action": "reply_all", "email_id": eid, "message": "my thoughts"})
    assert result["status"] == "delivered"
    sent_addresses = [call[0][0] for call in mock_svc.send.call_args_list]
    assert "alice" in sent_addresses
    assert "bob" in sent_addresses
    assert "charlie" in sent_addresses
    assert "me" not in sent_addresses
```

- [ ] **Step 6: Write search tests**

```python
def test_email_search_by_subject(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    _make_inbox_email(tmp_path, subject="important meeting", message="body1")
    _make_inbox_email(tmp_path, subject="casual chat", message="body2")
    result = mgr.handle({"action": "search", "query": "important"})
    assert result["total"] == 1
    assert "important" in result["emails"][0]["subject"]


def test_email_search_by_sender(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    _make_inbox_email(tmp_path, sender="alice@test", message="hello")
    _make_inbox_email(tmp_path, sender="bob@test", message="world")
    result = mgr.handle({"action": "search", "query": "alice"})
    assert result["total"] == 1


def test_email_search_by_message_body(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    _make_inbox_email(tmp_path, message="the secret code is 42")
    _make_inbox_email(tmp_path, message="nothing interesting")
    result = mgr.handle({"action": "search", "query": "secret.*42"})
    assert result["total"] == 1


def test_email_search_folder_filter(tmp_path):
    """Search with folder param should only search that folder."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    _make_inbox_email(tmp_path, message="keyword in inbox")
    mgr.handle({"action": "send", "address": "someone", "message": "keyword in sent"})
    # Search both — should find 2
    result = mgr.handle({"action": "search", "query": "keyword"})
    assert result["total"] == 2
    # Search inbox only — should find 1
    result = mgr.handle({"action": "search", "query": "keyword", "folder": "inbox"})
    assert result["total"] == 1


def test_email_search_invalid_regex(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    result = mgr.handle({"action": "search", "query": "[invalid"})
    assert "error" in result
```

- [ ] **Step 7: Write error case tests**

```python
def test_email_without_mail_service(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    result = mgr.handle({"action": "send", "address": "someone", "message": "hello"})
    assert "error" in result


def test_email_read_not_found(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mgr = agent.add_capability("email")
    result = mgr.handle({"action": "read", "email_id": "nonexistent"})
    assert "error" in result


def test_email_send_with_attachments(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    result = mgr.handle({
        "action": "send",
        "address": "127.0.0.1:8888",
        "subject": "file for you",
        "message": "see attached",
        "attachments": ["/path/to/file.png"],
    })
    assert result["status"] == "delivered"
    sent = mail_svc.send.call_args[0][1]
    assert sent.get("attachments") == ["/path/to/file.png"]
```

- [ ] **Step 8: Keep TCP integration tests**

Keep `test_email_send_multi_to`, `test_email_send_cc_visible`, `test_email_send_bcc_hidden` as-is — they test the transport layer via real TCP sockets and don't access `mgr._mailbox`. Just update `working_dir="/tmp"` to use `tmp_path`.

- [ ] **Step 9: Run all tests**

Run: `python -c "import lingtai"` (smoke test)
Run: `pytest tests/test_layers_email.py -v`
Run: `pytest tests/ -x -q`

- [ ] **Step 10: Commit**

```bash
git add tests/test_layers_email.py
git commit -m "test: rewrite email tests for filesystem-based mailbox"
```

---

## Chunk 4: Integration Verification

### Task 4: Verify end-to-end and clean up

- [ ] **Step 1: Run full test suite**

Run: `pytest tests/ -x -q`
All tests must pass.

- [ ] **Step 2: Check for any remaining references to `_mailbox` or `_mailbox_lock`**

Run: `grep -r "_mailbox_lock\|mgr\._mailbox" src/ tests/`
Should return nothing.

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "refactor: complete filesystem-based email mailbox migration"
```
