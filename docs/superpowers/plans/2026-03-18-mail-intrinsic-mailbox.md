# Mail Intrinsic Mailbox Upgrade — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade the mail intrinsic from a lossy FIFO pipe to a disk-backed mailbox with inbox, check, read, search, delete, and self-send — giving every BaseAgent persistent, compact-immune note storage.

**Architecture:** The mail intrinsic gains mailbox actions (check, read, search, delete) that read from `mailbox/inbox/` on disk — the same directory TCPMailService already persists to. The in-memory FIFO queue (`_mail_queue`) is removed. Self-send short-circuits TCP and persists directly to inbox. The email capability becomes a thin upgrade layer adding only reply/reply_all, CC/BCC, contacts, sent folder, and private mode.

**Tech Stack:** Python 3.11+, lingtai framework, pytest, unittest.mock

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `src/lingtai/intrinsics/mail.py` | **Rewrite** | New actions: send, check, read, search, delete. Mailbox filesystem helpers. Self-send logic. |
| `src/lingtai/base_agent.py` | **Modify** | Remove `_mail_queue`, `_mail_queue_lock`, `deque` import. Update `_on_normal_mail` to notify-only (MailService already persists). Keep `_mail_arrived` event. Rename `_email_notification` → `_mail_notification` everywhere. Update `_collapse_email_notifications` to reference `mail` tool. |
| `src/lingtai/message.py` | **Modify** | Rename `_email_notification` → `_mail_notification` (this is now a kernel-level concept). |
| `src/lingtai/intrinsics/__init__.py` | **Modify** | Update system_prompt for mail intrinsic. |
| `src/lingtai/capabilities/email.py` | **Modify** | Rename `_email_notification` → `_mail_notification`. Remove mailbox helpers that are now in mail intrinsic. EmailManager delegates check/read/search to mail intrinsic handler, adds reply/reply_all, CC/BCC, contacts, sent folder, private mode on top. |
| `src/lingtai/addons/gmail/manager.py` | **Modify** | Rename `_email_notification` → `_mail_notification` (line 215). |
| `tests/test_mail_intrinsic.py` | **Create** | Unit tests for the new mail intrinsic (check, read, search, delete, self-send). |
| `tests/test_layers_email.py` | **Modify** | Update tests to reflect email capability's thinner role; fix any references to old mail intrinsic behavior. |
| `tests/test_agent.py` | **Modify** | Remove `_mail_queue` assertions (lines 176-177). Replace with inbox notification checks. |
| `tests/test_silence_kill.py` | **Modify** | Replace all `_mail_queue` assertions with inbox-based checks. Silence/kill tests verify inbox stays empty; normal mail tests verify inbox gets a notification. |
| `tests/test_addon_gmail_manager.py` | **Modify** | Rename `_email_notification` → `_mail_notification` in assertions. |

---

### Task 1: Rewrite the mail intrinsic with mailbox support

**Files:**
- Rewrite: `src/lingtai/intrinsics/mail.py`
- Test: `tests/test_mail_intrinsic.py`

The core change — mail intrinsic goes from 2 actions (send, read-pop) to 6 actions (send, check, read, search, delete, and send-to-self that short-circuits). All reads are from `mailbox/inbox/` on disk.

- [ ] **Step 1: Write failing tests for the new mail intrinsic**

Create `tests/test_mail_intrinsic.py` with tests covering all 6 actions. Tests construct a mock agent with `_working_dir`, `_mail_service`, `_mail_arrived`, `_admin`, `_log` — no real LLM needed.

```python
"""Tests for the mail intrinsic — disk-backed mailbox."""
from __future__ import annotations

import json
import threading
from datetime import datetime, timezone
from pathlib import Path
from unittest.mock import MagicMock
from uuid import uuid4

import pytest

from lingtai.intrinsics import mail


def _make_mock_agent(tmp_path: Path, *, address: str = "127.0.0.1:9999") -> MagicMock:
    """Create a mock agent with the fields mail intrinsic needs."""
    agent = MagicMock()
    agent._working_dir = tmp_path
    agent._mail_service = MagicMock()
    agent._mail_service.address = address
    agent._mail_service.send.return_value = None  # success
    agent._mail_arrived = threading.Event()
    agent._admin = {}
    agent._log = MagicMock()
    agent.agent_id = "test123"
    return agent


def _make_inbox_message(
    working_dir: Path, *, sender: str = "other", to: str = "me",
    subject: str = "test", message: str = "body",
) -> str:
    """Create a message on disk in mailbox/inbox/{uuid}/message.json. Returns ID."""
    msg_id = str(uuid4())
    msg_dir = working_dir / "mailbox" / "inbox" / msg_id
    msg_dir.mkdir(parents=True, exist_ok=True)
    data = {
        "_mailbox_id": msg_id,
        "from": sender,
        "to": to,
        "subject": subject,
        "message": message,
        "received_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    (msg_dir / "message.json").write_text(json.dumps(data, indent=2))
    return msg_id


# ---------------------------------------------------------------------------
# send
# ---------------------------------------------------------------------------

class TestMailSend:
    def test_send_delivers(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = mail.handle(agent, {"action": "send", "address": "127.0.0.1:8888", "message": "hello"})
        assert result["status"] == "delivered"
        agent._mail_service.send.assert_called_once()

    def test_send_no_address(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = mail.handle(agent, {"action": "send", "message": "hello"})
        assert "error" in result or result.get("status") == "error"

    def test_send_no_mail_service(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        agent._mail_service = None
        result = mail.handle(agent, {"action": "send", "address": "x", "message": "hi"})
        assert "error" in result or result.get("status") == "error"

    def test_send_refused(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        agent._mail_service.send.return_value = "connection refused"
        result = mail.handle(agent, {"action": "send", "address": "x", "message": "hi"})
        assert result["status"] == "refused"

    def test_send_privilege_gate(self, tmp_path):
        """Non-normal types require admin privileges."""
        agent = _make_mock_agent(tmp_path)
        result = mail.handle(agent, {"action": "send", "address": "x", "message": "hi", "type": "silence"})
        assert "error" in result or result.get("status") == "error"

    def test_send_with_attachments(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        att = tmp_path / "file.txt"
        att.write_text("data")
        result = mail.handle(agent, {
            "action": "send", "address": "127.0.0.1:8888",
            "message": "see attached", "attachments": [str(att)],
        })
        assert result["status"] == "delivered"
        payload = agent._mail_service.send.call_args[0][1]
        assert str(att) in payload["attachments"]


# ---------------------------------------------------------------------------
# self-send (note to self)
# ---------------------------------------------------------------------------

class TestMailSelfSend:
    def test_self_send_persists_to_inbox(self, tmp_path):
        """Sending to own address should persist directly to inbox, no TCP."""
        agent = _make_mock_agent(tmp_path, address="127.0.0.1:9999")
        result = mail.handle(agent, {
            "action": "send", "address": "127.0.0.1:9999",
            "subject": "note", "message": "remember this",
        })
        assert result["status"] == "delivered"
        # Should NOT have called mail_service.send (short-circuit)
        agent._mail_service.send.assert_not_called()
        # Should be on disk
        inbox = tmp_path / "mailbox" / "inbox"
        assert inbox.is_dir()
        msg_dirs = list(inbox.iterdir())
        assert len(msg_dirs) == 1
        data = json.loads((msg_dirs[0] / "message.json").read_text())
        assert data["message"] == "remember this"
        assert data["subject"] == "note"
        assert data["from"] == "127.0.0.1:9999"

    def test_self_send_sets_mail_arrived(self, tmp_path):
        """Self-send should set _mail_arrived so clock wakes up."""
        agent = _make_mock_agent(tmp_path, address="127.0.0.1:9999")
        agent._mail_arrived = threading.Event()
        mail.handle(agent, {
            "action": "send", "address": "127.0.0.1:9999",
            "subject": "note", "message": "ping",
        })
        assert agent._mail_arrived.is_set()

    def test_self_send_no_mail_service_still_works(self, tmp_path):
        """Self-send should work even without mail service (address match by agent_id)."""
        agent = _make_mock_agent(tmp_path)
        agent._mail_service = None
        # When no mail service, self-send matches on agent_id
        result = mail.handle(agent, {
            "action": "send", "address": agent.agent_id,
            "subject": "note", "message": "no network needed",
        })
        assert result["status"] == "delivered"
        inbox = tmp_path / "mailbox" / "inbox"
        assert inbox.is_dir()


# ---------------------------------------------------------------------------
# check (list inbox)
# ---------------------------------------------------------------------------

class TestMailCheck:
    def test_check_empty(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = mail.handle(agent, {"action": "check"})
        assert result["status"] == "ok"
        assert result["total"] == 0

    def test_check_shows_messages(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        _make_inbox_message(agent._working_dir, sender="alice", subject="s1")
        _make_inbox_message(agent._working_dir, sender="bob", subject="s2")
        result = mail.handle(agent, {"action": "check"})
        assert result["total"] == 2
        assert all("id" in e for e in result["emails"])

    def test_check_n_limit(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        for i in range(5):
            _make_inbox_message(agent._working_dir, subject=f"msg-{i}")
        result = mail.handle(agent, {"action": "check", "n": 2})
        assert result["total"] == 5
        assert result["showing"] == 2

    def test_check_unread_flag(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        _make_inbox_message(agent._working_dir)
        result = mail.handle(agent, {"action": "check"})
        assert result["emails"][0]["unread"] is True


# ---------------------------------------------------------------------------
# read (by ID, non-destructive)
# ---------------------------------------------------------------------------

class TestMailRead:
    def test_read_by_id(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        mid = _make_inbox_message(agent._working_dir, subject="topic", message="full body")
        result = mail.handle(agent, {"action": "read", "id": [mid]})
        assert result["status"] == "ok"
        assert len(result["emails"]) == 1
        assert result["emails"][0]["message"] == "full body"

    def test_read_marks_as_read(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        mid = _make_inbox_message(agent._working_dir)
        # Initially unread
        check = mail.handle(agent, {"action": "check"})
        assert check["emails"][0]["unread"] is True
        # Read it
        mail.handle(agent, {"action": "read", "id": [mid]})
        # Now should be read
        check = mail.handle(agent, {"action": "check"})
        assert check["emails"][0]["unread"] is False

    def test_read_not_found(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = mail.handle(agent, {"action": "read", "id": ["nonexistent"]})
        assert result.get("not_found") == ["nonexistent"]

    def test_read_multiple(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        m1 = _make_inbox_message(agent._working_dir, message="one")
        m2 = _make_inbox_message(agent._working_dir, message="two")
        result = mail.handle(agent, {"action": "read", "id": [m1, m2]})
        assert len(result["emails"]) == 2

    def test_read_shows_attachments(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        mid = str(uuid4())
        msg_dir = tmp_path / "mailbox" / "inbox" / mid
        msg_dir.mkdir(parents=True)
        data = {
            "_mailbox_id": mid, "from": "x", "to": "y",
            "subject": "photo", "message": "look",
            "attachments": ["/path/to/photo.png"],
            "received_at": "2026-01-01T00:00:00Z",
        }
        (msg_dir / "message.json").write_text(json.dumps(data))
        result = mail.handle(agent, {"action": "read", "id": [mid]})
        assert "attachments" in result["emails"][0]


# ---------------------------------------------------------------------------
# search
# ---------------------------------------------------------------------------

class TestMailSearch:
    def test_search_by_subject(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        _make_inbox_message(agent._working_dir, subject="important meeting")
        _make_inbox_message(agent._working_dir, subject="casual chat")
        result = mail.handle(agent, {"action": "search", "query": "important"})
        assert result["total"] == 1

    def test_search_by_sender(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        _make_inbox_message(agent._working_dir, sender="alice")
        _make_inbox_message(agent._working_dir, sender="bob")
        result = mail.handle(agent, {"action": "search", "query": "alice"})
        assert result["total"] == 1

    def test_search_by_body(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        _make_inbox_message(agent._working_dir, message="the secret code is 42")
        _make_inbox_message(agent._working_dir, message="nothing here")
        result = mail.handle(agent, {"action": "search", "query": "secret.*42"})
        assert result["total"] == 1

    def test_search_empty_query(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = mail.handle(agent, {"action": "search"})
        assert "error" in result

    def test_search_invalid_regex(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = mail.handle(agent, {"action": "search", "query": "[invalid"})
        assert "error" in result


# ---------------------------------------------------------------------------
# delete
# ---------------------------------------------------------------------------

class TestMailDelete:
    def test_delete_removes_from_disk(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        mid = _make_inbox_message(agent._working_dir)
        result = mail.handle(agent, {"action": "delete", "id": [mid]})
        assert result["status"] == "ok"
        assert result["deleted"] == [mid]
        # Should no longer appear in check
        check = mail.handle(agent, {"action": "check"})
        assert check["total"] == 0

    def test_delete_not_found(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = mail.handle(agent, {"action": "delete", "id": ["nonexistent"]})
        assert result.get("not_found") == ["nonexistent"]

    def test_delete_multiple(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        m1 = _make_inbox_message(agent._working_dir)
        m2 = _make_inbox_message(agent._working_dir)
        result = mail.handle(agent, {"action": "delete", "id": [m1, m2]})
        assert len(result["deleted"]) == 2
        check = mail.handle(agent, {"action": "check"})
        assert check["total"] == 0

    def test_delete_cleans_read_tracking(self, tmp_path):
        """Deleted IDs should be removed from read.json."""
        agent = _make_mock_agent(tmp_path)
        mid = _make_inbox_message(agent._working_dir)
        mail.handle(agent, {"action": "read", "id": [mid]})
        mail.handle(agent, {"action": "delete", "id": [mid]})
        # read.json should not contain the deleted ID
        read_path = tmp_path / "mailbox" / "read.json"
        if read_path.is_file():
            read_ids = json.loads(read_path.read_text())
            assert mid not in read_ids
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_mail_intrinsic.py -v`
Expected: FAIL — current mail.py has no check/read-by-id/search/delete/self-send

- [ ] **Step 3: Rewrite the mail intrinsic**

Rewrite `src/lingtai/intrinsics/mail.py`:

```python
"""Mail intrinsic — disk-backed mailbox with send, check, read, search, delete.

Storage layout (managed by MailService on receive, by self-send on local writes):
    working_dir/mailbox/inbox/{uuid}/message.json   — received messages
    working_dir/mailbox/read.json                    — read tracking (set of IDs)

Actions:
    send   — fire-and-forget message to an address (self-send short-circuits to inbox)
    check  — list inbox (recent N, with unread flags)
    read   — load full message(s) by ID, mark as read
    search — regex search inbox (from, subject, message)
    delete — remove message(s) from inbox
"""
from __future__ import annotations

import json
import os
import re
import shutil
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["send", "check", "read", "search", "delete"],
            "description": (
                "send: send a message (requires address, message; optional subject, attachments). "
                "Sending to your own address creates a persistent note. "
                "check: list inbox (optional n for max shown, default 10). "
                "read: read full messages by ID (id=[id1, id2, ...]). "
                "You are encouraged to read multiple relevant or even all unread messages and think before acting. "
                "search: regex search inbox (requires query). "
                "delete: remove messages from inbox (id=[id1, id2, ...])."
            ),
        },
        "address": {
            "type": "string",
            "description": "Target address for send (e.g. 127.0.0.1:8301). Use your own address to write a note to yourself.",
        },
        "subject": {"type": "string", "description": "Message subject (for send)"},
        "message": {"type": "string", "description": "Message body (for send)"},
        "attachments": {
            "type": "array",
            "items": {"type": "string"},
            "description": "File paths to attach to the message (for send)",
        },
        "type": {
            "type": "string",
            "enum": ["normal", "silence", "kill"],
            "description": (
                "Mail type (for send). 'normal' (default) is regular mail. "
                "'silence' interrupts the target agent and puts it to idle "
                "(revives on next email; requires admin.silence privilege). "
                "'kill' hard-stops the target agent (requires admin.kill privilege). "
                "To revive: re-delegate with the SAME name."
            ),
        },
        "id": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Message ID(s) — for read and delete actions.",
        },
        "n": {
            "type": "integer",
            "description": "Max recent messages to show (for check, default 10)",
            "default": 10,
        },
        "query": {
            "type": "string",
            "description": "Regex pattern for search (matches from, subject, message)",
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Disk-backed mailbox — send and receive messages, search, manage inbox. "
    "Messages persist on disk and survive context compaction. "
    "'send' delivers to another agent or to yourself (note-to-self). "
    "'check' lists inbox with unread flags. "
    "'read' loads full message(s) by ID. "
    "'search' finds messages by regex. "
    "'delete' removes messages from inbox. "
    "Etiquette: a short acknowledgement is fine, but do not reply to "
    "an acknowledgement — that creates pointless ping-pong."
)


# ------------------------------------------------------------------
# Mailbox filesystem helpers
# ------------------------------------------------------------------

def _mailbox_dir(agent) -> Path:
    return agent._working_dir / "mailbox"


def _inbox_dir(agent) -> Path:
    return _mailbox_dir(agent) / "inbox"


def _load_message(agent, msg_id: str) -> dict | None:
    """Load a single message from inbox by ID."""
    path = _inbox_dir(agent) / msg_id / "message.json"
    if path.is_file():
        try:
            data = json.loads(path.read_text())
            data.setdefault("_mailbox_id", msg_id)
            return data
        except (json.JSONDecodeError, OSError):
            return None
    return None


def _list_inbox(agent) -> list[dict]:
    """Load all inbox messages, sorted by time (newest first)."""
    inbox = _inbox_dir(agent)
    if not inbox.is_dir():
        return []
    messages = []
    for msg_dir in inbox.iterdir():
        msg_file = msg_dir / "message.json"
        if msg_dir.is_dir() and msg_file.is_file():
            try:
                data = json.loads(msg_file.read_text())
                data.setdefault("_mailbox_id", msg_dir.name)
                messages.append(data)
            except (json.JSONDecodeError, OSError):
                continue
    messages.sort(
        key=lambda e: e.get("received_at") or e.get("time") or "",
        reverse=True,
    )
    return messages


def _read_ids(agent) -> set[str]:
    """Load the set of read message IDs."""
    path = _mailbox_dir(agent) / "read.json"
    if path.is_file():
        try:
            return set(json.loads(path.read_text()))
        except (json.JSONDecodeError, OSError):
            return set()
    return set()


def _save_read_ids(agent, ids: set[str]) -> None:
    """Atomically write read IDs to disk."""
    mbox = _mailbox_dir(agent)
    mbox.mkdir(parents=True, exist_ok=True)
    target = mbox / "read.json"
    fd, tmp = tempfile.mkstemp(dir=str(mbox), suffix=".tmp")
    try:
        os.write(fd, json.dumps(sorted(ids)).encode())
        os.close(fd)
        os.replace(tmp, str(target))
    except Exception:
        try:
            os.close(fd)
        except OSError:
            pass
        if os.path.exists(tmp):
            os.unlink(tmp)
        raise


def _mark_read(agent, msg_id: str) -> None:
    ids = _read_ids(agent)
    ids.add(msg_id)
    _save_read_ids(agent, ids)


def _message_summary(msg: dict, read_ids: set[str]) -> dict:
    """Build a summary dict from a raw message."""
    mid = msg.get("_mailbox_id", "")
    entry = {
        "id": mid,
        "from": msg.get("from", ""),
        "to": msg.get("to", ""),
        "subject": msg.get("subject", "(no subject)"),
        "preview": msg.get("message", "")[:200],
        "time": msg.get("received_at") or msg.get("time") or "",
        "unread": mid not in read_ids,
    }
    return entry


# ------------------------------------------------------------------
# Self-send: persist directly to inbox without TCP
# ------------------------------------------------------------------

def _is_self_send(agent, address: str) -> bool:
    """Check if address matches this agent's own address or ID."""
    if agent._mail_service is not None and agent._mail_service.address:
        if address == agent._mail_service.address:
            return True
    return address == agent.agent_id


def _persist_to_inbox(agent, payload: dict) -> str:
    """Write a message directly to mailbox/inbox/{uuid}/message.json.
    Returns the message ID."""
    msg_id = str(uuid4())
    msg_dir = _inbox_dir(agent) / msg_id
    msg_dir.mkdir(parents=True, exist_ok=True)
    payload = {
        **payload,
        "_mailbox_id": msg_id,
        "received_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    (msg_dir / "message.json").write_text(
        json.dumps(payload, indent=2, default=str)
    )
    return msg_id


# ------------------------------------------------------------------
# Action handlers
# ------------------------------------------------------------------

def handle(agent, args: dict) -> dict:
    """Handle mail tool — disk-backed mailbox."""
    action = args.get("action", "send")
    if action == "send":
        return _send(agent, args)
    elif action == "check":
        return _check(agent, args)
    elif action == "read":
        return _read(agent, args)
    elif action == "search":
        return _search(agent, args)
    elif action == "delete":
        return _delete(agent, args)
    else:
        return {"error": f"Unknown mail action: {action}"}


def _send(agent, args: dict) -> dict:
    address = args.get("address", "")
    subject = args.get("subject", "")
    message_text = args.get("message", "")
    mail_type = args.get("type", "normal")

    if mail_type != "normal" and not agent._admin.get(mail_type):
        return {"error": f"Not authorized to send type={mail_type!r} mail (requires admin.{mail_type}=True)"}

    if not address:
        return {"error": "address is required"}

    sender = (
        agent._mail_service.address
        if agent._mail_service is not None and agent._mail_service.address
        else agent.agent_id
    )

    payload = {
        "from": sender,
        "to": address,
        "subject": subject,
        "message": message_text,
        "type": mail_type,
    }

    # Resolve attachments
    attachments = args.get("attachments", [])
    if attachments:
        resolved = []
        for p in attachments:
            path = Path(p)
            if not path.is_absolute():
                path = agent._working_dir / path
            if not path.is_file():
                return {"error": f"Attachment not found: {path}"}
            resolved.append(str(path))
        payload["attachments"] = resolved

    # Self-send: short-circuit TCP, persist directly to inbox
    if _is_self_send(agent, address):
        msg_id = _persist_to_inbox(agent, payload)
        agent._mail_arrived.set()
        agent._log("mail_sent", address=address, subject=subject, status="delivered", message=message_text, self_send=True)
        return {"status": "delivered", "to": address, "id": msg_id, "note": "saved to your inbox"}

    # Normal send via MailService
    if agent._mail_service is None:
        return {"error": "mail service not configured"}

    err = agent._mail_service.send(address, payload)
    status = "delivered" if err is None else "refused"
    agent._log("mail_sent", address=address, subject=subject, status=status, message=message_text)
    if err is None:
        return {"status": "delivered", "to": address}
    else:
        return {"status": "refused", "error": err}


def _check(agent, args: dict) -> dict:
    n = args.get("n", 10)
    messages = _list_inbox(agent)
    total = len(messages)
    recent = messages[:n] if n > 0 else messages
    read_ids_set = _read_ids(agent)
    summaries = [_message_summary(m, read_ids_set) for m in recent]
    return {"status": "ok", "total": total, "showing": len(summaries), "emails": summaries}


def _read(agent, args: dict) -> dict:
    ids = args.get("id", [])
    if isinstance(ids, str):
        ids = [ids]
    if not ids:
        return {"error": "id is required"}

    results = []
    not_found = []
    for mid in ids:
        data = _load_message(agent, mid)
        if data is None:
            not_found.append(mid)
            continue
        _mark_read(agent, mid)
        entry = {
            "id": mid,
            "from": data.get("from", ""),
            "to": data.get("to", ""),
            "subject": data.get("subject", "(no subject)"),
            "message": data.get("message", ""),
            "time": data.get("received_at") or data.get("time") or "",
        }
        if data.get("attachments"):
            entry["attachments"] = data["attachments"]
        results.append(entry)

    result = {"status": "ok", "emails": results}
    if not_found:
        result["not_found"] = not_found
    return result


def _search(agent, args: dict) -> dict:
    query = args.get("query", "")
    if not query:
        return {"error": "query is required for search"}

    try:
        pattern = re.compile(query, re.IGNORECASE)
    except re.error as e:
        return {"error": f"Invalid regex: {e}"}

    read_ids_set = _read_ids(agent)
    matches = []
    for msg in _list_inbox(agent):
        searchable = " ".join([
            msg.get("from", ""),
            msg.get("subject", ""),
            msg.get("message", ""),
        ])
        if pattern.search(searchable):
            matches.append(_message_summary(msg, read_ids_set))

    return {"status": "ok", "total": len(matches), "emails": matches}


def _delete(agent, args: dict) -> dict:
    ids = args.get("id", [])
    if isinstance(ids, str):
        ids = [ids]
    if not ids:
        return {"error": "id is required"}

    deleted = []
    not_found = []
    for mid in ids:
        msg_dir = _inbox_dir(agent) / mid
        if msg_dir.is_dir():
            shutil.rmtree(msg_dir)
            deleted.append(mid)
        else:
            not_found.append(mid)

    # Clean read tracking for deleted IDs
    if deleted:
        current_read = _read_ids(agent)
        cleaned = current_read - set(deleted)
        if len(cleaned) != len(current_read):
            _save_read_ids(agent, cleaned)

    agent._log("mail_deleted", deleted=deleted, not_found=not_found)

    result = {"status": "ok", "deleted": deleted}
    if not_found:
        result["not_found"] = not_found
    return result
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_mail_intrinsic.py -v`
Expected: all PASS

- [ ] **Step 5: Smoke-test the module**

Run: `python -c "from lingtai.intrinsics import mail; print(mail.SCHEMA['properties']['action']['enum'])"`
Expected: `['send', 'check', 'read', 'search', 'delete']`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/intrinsics/mail.py tests/test_mail_intrinsic.py
git commit -m "feat: upgrade mail intrinsic to disk-backed mailbox with check/read/search/delete/self-send"
```

---

### Task 2: Remove FIFO queue, rename `_email_notification` → `_mail_notification` everywhere, update tests

**Files:**
- Modify: `src/lingtai/base_agent.py:18,203-207,486-518,620-668` (remove `deque` import, `_mail_queue`, `_mail_queue_lock`; rewrite `_on_normal_mail`; update `_collapse_email_notifications`)
- Modify: `src/lingtai/message.py:38` (rename `_email_notification` → `_mail_notification`)
- Modify: `src/lingtai/intrinsics/__init__.py:13` (update system_prompt)
- Modify: `src/lingtai/capabilities/email.py:685` (rename `_email_notification` → `_mail_notification`)
- Modify: `src/lingtai/addons/gmail/manager.py:215` (rename `_email_notification` → `_mail_notification`)
- Modify: `tests/test_agent.py:163-177` (replace `_mail_queue` assertions with inbox notification checks)
- Modify: `tests/test_silence_kill.py:36-45,93-100,237-273` (replace `_mail_queue` assertions)
- Modify: `tests/test_addon_gmail_manager.py` (rename `_email_notification` in assertions)
- Test: `tests/test_mail_intrinsic.py`

**Important:** All `_email_notification` → `_mail_notification` renames happen in this task (including email.py and gmail addon) so that every commit leaves the test suite green.

- [ ] **Step 1: Write failing tests for updated notification behavior**

Add to `tests/test_mail_intrinsic.py`:

```python
# ---------------------------------------------------------------------------
# BaseAgent integration: notification on receive
# ---------------------------------------------------------------------------

class TestMailNotification:
    def test_on_normal_mail_sends_notification(self, tmp_path):
        """BaseAgent._on_normal_mail should put a notification in agent inbox."""
        from lingtai.base_agent import BaseAgent
        from unittest.mock import MagicMock

        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "test"
        agent = BaseAgent(agent_name="test", service=svc, base_dir=tmp_path)
        agent._on_normal_mail({
            "_mailbox_id": "abc123",
            "from": "alice",
            "to": "me",
            "subject": "hello",
            "message": "world",
        })
        assert not agent.inbox.empty()
        msg = agent.inbox.get_nowait()
        assert "hello" in msg.content
        assert "abc123" in msg.content
        assert msg._mail_notification is not None
        assert msg._mail_notification["email_id"] == "abc123"

    def test_on_normal_mail_generates_id_fallback(self, tmp_path):
        """If _mailbox_id is missing, _on_normal_mail should generate one."""
        from lingtai.base_agent import BaseAgent
        from unittest.mock import MagicMock

        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "test"
        agent = BaseAgent(agent_name="test", service=svc, base_dir=tmp_path)
        agent._on_normal_mail({
            "from": "alice",
            "message": "no mailbox id",
        })
        msg = agent.inbox.get_nowait()
        assert msg._mail_notification is not None
        assert msg._mail_notification["email_id"]  # not empty

    def test_on_normal_mail_sets_mail_arrived(self, tmp_path):
        """_on_normal_mail should set _mail_arrived for clock wake."""
        from lingtai.base_agent import BaseAgent
        from unittest.mock import MagicMock

        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "test"
        agent = BaseAgent(agent_name="test", service=svc, base_dir=tmp_path)
        agent._on_normal_mail({
            "_mailbox_id": "x",
            "from": "alice",
            "message": "hi",
        })
        assert agent._mail_arrived.is_set()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_mail_intrinsic.py::TestMailNotification -v`
Expected: FAIL — `_mail_notification` doesn't exist yet (still `_email_notification`)

- [ ] **Step 3: Update Message dataclass**

In `src/lingtai/message.py`, rename `_email_notification` → `_mail_notification`:

```python
# line 38: change field name
_mail_notification: dict | None = field(default=None, repr=False)
```

- [ ] **Step 4: Update BaseAgent**

In `src/lingtai/base_agent.py`:

1. **Remove FIFO queue** (lines ~203-206): delete `_mail_queue`, `_mail_queue_lock`. Keep `_mail_arrived`. Remove `from collections import deque` (line 18) — it is only used for `_mail_queue`.

2. **Rewrite `_on_normal_mail`** (lines ~486-518): No more queue append. Set `_mail_arrived`, build notification with `_mail_notification` metadata, put in inbox. Use `_mailbox_id` fallback for non-TCP mail services.

```python
def _on_normal_mail(self, payload: dict) -> None:
    """Handle a normal mail — notify agent via inbox.

    The message is already persisted to mailbox/inbox/ by MailService.
    This method only signals arrival and sends a notification.
    Capabilities (e.g. email) may replace this method.
    """
    from uuid import uuid4

    email_id = payload.get("_mailbox_id") or str(uuid4())
    sender = payload.get("from", "unknown")
    subject = payload.get("subject", "")
    message = payload.get("message", "")

    self._mail_arrived.set()

    preview = message[:200].replace("\n", " ")
    notification = (
        f'[Mail from {sender}]\n'
        f'  Subject: {subject}\n'
        f'  Preview: {preview}...\n'
        f'  ID: {email_id}\n'
        f'Use mail(action="read", id=["{email_id}"]) to read full message.'
    )

    self._log("mail_received", sender=sender, subject=subject, message=message)
    msg = _make_message(MSG_REQUEST, sender, notification)
    msg._mail_notification = {
        "email_id": email_id,
        "sender": sender,
        "subject": subject,
        "preview": preview,
    }
    self.inbox.put(msg)
```

3. **Update `_collapse_email_notifications`** (lines ~620-668): Rename all `_email_notification` → `_mail_notification`. Update merged notification text to reference `mail` tool instead of `email`.

```python
def _collapse_email_notifications(self, msg: Message) -> Message:
    """Collapse consecutive mail notification messages into one."""
    if msg._mail_notification is None:
        return msg

    notifications = [msg._mail_notification]
    requeue: list[Message] = []

    while True:
        try:
            queued = self.inbox.get_nowait()
        except queue.Empty:
            break
        if queued._mail_notification is not None:
            notifications.append(queued._mail_notification)
        else:
            requeue.append(queued)

    for m in requeue:
        self.inbox.put(m)

    if len(notifications) == 1:
        return msg

    lines = [f"[{len(notifications)} new messages arrived]", ""]
    for i, n in enumerate(notifications, 1):
        lines.append(
            f'{i}. From {n["sender"]} — Subject: {n["subject"]}\n'
            f'   Preview: {n["preview"]}...\n'
            f'   ID: {n["email_id"]}'
        )
    lines.append("")
    lines.append(
        'Use mail(action="check") to see your inbox, or '
        'mail(action="read", id=["..."]) to read a specific message.'
    )
    merged_content = "\n".join(lines)
    merged = _make_message(MSG_REQUEST, "system", merged_content)
    self._log("mail_notifications_collapsed", count=len(notifications))
    return merged
```

- [ ] **Step 5: Rename `_email_notification` → `_mail_notification` in email capability and gmail addon**

In `src/lingtai/capabilities/email.py` line 685:
```python
# Change: msg._email_notification = {
msg._mail_notification = {
```

In `src/lingtai/addons/gmail/manager.py` line 215:
```python
# Change: msg._email_notification = {
msg._mail_notification = {
```

In `tests/test_addon_gmail_manager.py`, update any assertions referencing `_email_notification` to `_mail_notification`.

- [ ] **Step 6: Update intrinsics/__init__.py**

Change the mail system_prompt:

```python
"mail": {
    "schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handle": mail.handle,
    "system_prompt": "Send and receive messages. Check inbox, read, search, delete. Send to yourself to take persistent notes.",
},
```

- [ ] **Step 7: Update test_agent.py — remove `_mail_queue` assertions**

In `tests/test_agent.py`, the test `test_mail_inbox_wiring` (lines 163-177) asserts on `_mail_queue`. Rewrite to check inbox notification instead:

```python
def test_mail_inbox_wiring(tmp_path):
    """_on_mail_received should notify agent inbox."""
    agent = BaseAgent(agent_name="receiver", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({
        "_mailbox_id": "test-id-123",
        "from": "127.0.0.1:9999",
        "to": "127.0.0.1:8301",
        "message": "inbox test",
    })
    assert not agent.inbox.empty()
    msg = agent.inbox.get_nowait()
    assert "inbox test" in msg.content
    assert msg.sender == "127.0.0.1:9999"
    assert msg._mail_notification is not None
    assert msg._mail_notification["email_id"] == "test-id-123"
```

- [ ] **Step 8: Update test_silence_kill.py — replace `_mail_queue` assertions**

Replace all `_mail_queue` assertions with inbox-based checks:

For `test_silence_bypasses_mail_queue` (line 36) and `test_kill_bypasses_mail_queue` (line 93):
```python
def test_silence_bypasses_inbox(tmp_path):
    """Silence-type mail should NOT send a notification to agent inbox."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({"from": "boss", "to": "test", "type": "silence"})
    assert agent.inbox.empty()
```

For `test_normal_email_queued` (line 237), `test_missing_type_defaults_to_normal` (line 251), `test_unrecognized_type_treated_as_normal` (line 264):
```python
def test_normal_email_notifies_inbox(tmp_path):
    """Normal-type mail should send a notification to agent inbox."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({
        "_mailbox_id": "test123",
        "from": "colleague", "to": "test", "subject": "hello",
        "message": "hi there", "type": "normal",
    })
    assert not agent.inbox.empty()
    assert not agent._cancel_event.is_set()

def test_missing_type_defaults_to_normal(tmp_path):
    """Mail without a type field should be treated as normal."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({"from": "colleague", "to": "test", "message": "hi"})
    assert not agent.inbox.empty()
    assert not agent._cancel_event.is_set()

def test_unrecognized_type_treated_as_normal(tmp_path):
    """Unrecognized mail type should be treated as normal."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({"from": "someone", "type": "bogus", "message": "test"})
    assert not agent.inbox.empty()
```

- [ ] **Step 9: Run all tests**

Run: `python -m pytest tests/ -v`
Expected: all PASS

- [ ] **Step 10: Smoke-test**

Run: `python -c "import lingtai"`
Expected: no import errors

- [ ] **Step 11: Commit**

```bash
git add src/lingtai/base_agent.py src/lingtai/message.py src/lingtai/intrinsics/__init__.py \
    src/lingtai/capabilities/email.py src/lingtai/addons/gmail/manager.py \
    tests/test_agent.py tests/test_silence_kill.py tests/test_addon_gmail_manager.py \
    tests/test_mail_intrinsic.py
git commit -m "refactor: remove FIFO mail queue, rename _email_notification to _mail_notification everywhere"
```

---

### Task 3: Slim down the email capability

**Files:**
- Modify: `src/lingtai/capabilities/email.py`
- Modify: `tests/test_layers_email.py`

The email capability no longer needs its own mailbox filesystem helpers — it delegates inbox operations to the mail intrinsic's functions. It keeps: reply/reply_all, CC/BCC, contacts, sent folder, private mode, duplicate loop detection.

- [ ] **Step 1: Write test to verify email still works after refactor**

Add a test to `tests/test_layers_email.py`:

```python
def test_email_check_uses_mail_intrinsic_data(tmp_path):
    """Email capability's check should see messages persisted by mail intrinsic."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    # Create message using mail intrinsic's format (same disk layout)
    _make_inbox_email(agent.working_dir, sender="alice", subject="from-mail", message="body")
    result = mgr.handle({"action": "check"})
    assert result["total"] == 1
```

- [ ] **Step 2: Run test — should pass (same disk layout)**

Run: `python -m pytest tests/test_layers_email.py::test_email_check_uses_mail_intrinsic_data -v`
Expected: PASS (email already reads from same `mailbox/inbox/` dir)

- [ ] **Step 3: Refactor EmailManager to use mail intrinsic helpers**

In `src/lingtai/capabilities/email.py`:

1. Import mail intrinsic helpers:
```python
from ..intrinsics.mail import (
    _list_inbox, _load_message, _read_ids, _mark_read,
    _message_summary, _mailbox_dir,
)
```

2. Remove duplicated methods from EmailManager:
   - Remove `_load_email` — use `_load_message` (note: email also searches sent/, so keep a thin wrapper)
   - Remove `_list_emails` for inbox — use `_list_inbox` (keep sent-folder listing)
   - Remove `_read_ids`, `_mark_read`, `_email_summary` for inbox — use the mail intrinsic versions
   - Keep `_list_emails` for sent folder (mail intrinsic doesn't have sent/)

3. The `_check` method should delegate to `_list_inbox` for inbox, keep its own `_list_sent` for sent.

4. The `_read` method should use `_load_message` for inbox, keep its own loader for sent.

5. The `_search` method should use `_list_inbox` for inbox, keep its own for sent.

- [ ] **Step 4: Update email `on_normal_mail` to reference mail tool**

In `EmailManager.on_normal_mail`, update the notification text to mention `email` tool (since that's what's registered when the capability is active):

```python
notification = (
    f'[New email from {sender}]\n'
    f'  Subject: {subject}\n'
    f'  Preview: {preview}...\n'
    f'  ID: {email_id}\n'
    f'Use email(action="read", email_id="{email_id}") to read full message. '
    f'Use email(action="reply", email_id="{email_id}", message="...") to reply.'
)
```

(This is already correct — no change needed.)

- [ ] **Step 5: Run full email test suite**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: all PASS

- [ ] **Step 6: Run all tests**

Run: `python -m pytest tests/ -v`
Expected: all PASS (no regressions)

- [ ] **Step 7: Smoke-test**

Run: `python -c "import lingtai"`
Expected: no import errors

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/capabilities/email.py tests/test_layers_email.py
git commit -m "refactor: email capability delegates inbox ops to mail intrinsic helpers"
```

---

### Task 4: Final sweep — verify no stale references remain

**Files:**
- Verify: all source and test files

Most renames were done in Task 2. This task is a safety check.

- [ ] **Step 1: Search for stale references**

Run: `grep -rn "_email_notification" src/ tests/`
Expected: zero hits (all renamed in Task 2).

Run: `grep -rn "_mail_queue" src/ tests/`
Expected: zero hits (all removed in Task 2).

Run: `grep -rn "from collections import deque" src/lingtai/base_agent.py`
Expected: zero hits (removed in Task 2).

- [ ] **Step 2: Fix any remaining references**

If any hits from Step 1, fix them now.

- [ ] **Step 3: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: all PASS

- [ ] **Step 4: Commit (only if Step 2 had fixes)**

```bash
git add -u
git commit -m "chore: clean up remaining stale references"
```

---

### Task 5: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update the mail intrinsic description in CLAUDE.md**

Update the intrinsics section to reflect the new mail actions (send, check, read, search, delete, self-send). Update the "Key Modules" intrinsics description. Update the email capability description to reflect it's now a thin upgrade layer.

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for mail intrinsic mailbox upgrade"
```
