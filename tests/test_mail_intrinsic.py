"""Tests for the mail intrinsic — disk-backed mailbox with 5 actions."""
from __future__ import annotations

import json
import time
import threading
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai_kernel.intrinsics.mail import handle


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_mock_agent(tmp_path: Path, *, address: str = "127.0.0.1:9999") -> MagicMock:
    """Create a mock agent with all fields needed by the mail intrinsic."""
    agent = MagicMock()
    agent.agent_id = "abc123def456"
    agent._working_dir = tmp_path / "workdir"
    agent._working_dir.mkdir(parents=True, exist_ok=True)
    agent._admin = {}
    agent._mail_arrived = threading.Event()
    agent._log = MagicMock()

    mail_svc = MagicMock()
    mail_svc.address = address
    mail_svc.send.return_value = None  # success by default
    agent._mail_service = mail_svc

    return agent


def _make_inbox_message(
    working_dir: Path,
    *,
    sender: str = "127.0.0.1:8888",
    to: str = "127.0.0.1:9999",
    subject: str = "test subject",
    message: str = "test body",
    received_at: str = "2026-03-18T10:00:00Z",
    attachments: list[str] | None = None,
) -> str:
    """Create a message on disk in mailbox/inbox/{uuid}/message.json. Returns ID."""
    import uuid
    msg_id = str(uuid.uuid4())
    msg_dir = working_dir / "mailbox" / "inbox" / msg_id
    msg_dir.mkdir(parents=True, exist_ok=True)

    payload = {
        "_mailbox_id": msg_id,
        "from": sender,
        "to": to,
        "subject": subject,
        "message": message,
        "type": "normal",
        "received_at": received_at,
    }
    if attachments:
        payload["attachments"] = attachments

    (msg_dir / "message.json").write_text(json.dumps(payload, indent=2))
    return msg_id


# ---------------------------------------------------------------------------
# send tests
# ---------------------------------------------------------------------------

class TestSend:
    def test_send_delivers(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "subject": "hello",
            "message": "world",
        })
        assert result["status"] == "sent"
        assert result["to"] == "127.0.0.1:8888"
        assert result["delay"] == 0

    def test_send_no_address(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {"action": "send", "message": "hello"})
        assert "error" in result
        assert "address" in result["error"]

    def test_send_type_field_passes_through(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        # type field is no longer gated — mail is pure messaging
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "shh",
            "type": "silence",
        })
        assert result["status"] == "sent"

    def test_send_with_attachments(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        # Create a real file
        att = tmp_path / "file.txt"
        att.write_text("hello")

        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "see attached",
            "attachments": [str(att)],
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        sent = agent._working_dir / "mailbox" / "sent"
        sent_items = list(sent.iterdir())
        assert len(sent_items) == 1
        msg = json.loads((sent_items[0] / "message.json").read_text())
        assert msg["attachments"] == [str(att)]

    def test_send_attachment_not_found(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "see attached",
            "attachments": ["/nonexistent/file.txt"],
        })
        assert "error" in result
        assert "not found" in result["error"].lower()


# ---------------------------------------------------------------------------
# self-send tests
# ---------------------------------------------------------------------------

class TestSelfSend:
    def test_self_send_persists_to_inbox(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:9999",  # same as agent's mail service address
            "subject": "note to self",
            "message": "remember this",
        })
        assert result["status"] == "sent"
        # Should NOT have called mail_service.send
        agent._mail_service.send.assert_not_called()
        time.sleep(0.2)
        # Should have persisted to disk
        inbox = agent._working_dir / "mailbox" / "inbox"
        msg_dirs = list(inbox.iterdir())
        assert len(msg_dirs) == 1
        msg = json.loads((msg_dirs[0] / "message.json").read_text())
        assert msg["subject"] == "note to self"
        assert msg["message"] == "remember this"

    def test_self_send_sets_mail_arrived(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        assert not agent._mail_arrived.is_set()
        handle(agent, {
            "action": "send",
            "address": "127.0.0.1:9999",
            "subject": "ping",
            "message": "self",
        })
        time.sleep(0.2)
        assert agent._mail_arrived.is_set()

    def test_self_send_no_mail_service_still_works(self, tmp_path):
        """When no mail service, self-send matches on agent_id."""
        agent = _make_mock_agent(tmp_path)
        agent._mail_service = None
        result = handle(agent, {
            "action": "send",
            "address": agent.agent_id,
            "subject": "self note",
            "message": "via agent_id",
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        inbox = agent._working_dir / "mailbox" / "inbox"
        assert len(list(inbox.iterdir())) == 1


# ---------------------------------------------------------------------------
# check tests
# ---------------------------------------------------------------------------

class TestCheck:
    def test_check_empty(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {"action": "check"})
        assert result["total"] == 0
        assert result["messages"] == []

    def test_check_shows_messages(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        msg_id = _make_inbox_message(
            agent._working_dir,
            sender="alice",
            subject="hi there",
            message="hello world",
        )
        result = handle(agent, {"action": "check"})
        assert result["total"] == 1
        assert len(result["messages"]) == 1
        summary = result["messages"][0]
        assert summary["id"] == msg_id
        assert summary["from"] == "alice"
        assert summary["subject"] == "hi there"
        assert "hello" in summary["preview"]
        assert summary["unread"] is True

    def test_check_n_limit(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        for i in range(5):
            _make_inbox_message(
                agent._working_dir,
                subject=f"msg {i}",
                received_at=f"2026-03-18T10:0{i}:00Z",
            )
        result = handle(agent, {"action": "check", "n": 2})
        assert result["total"] == 5
        assert result["shown"] == 2
        assert len(result["messages"]) == 2

    def test_check_unread_flag(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        msg_id = _make_inbox_message(agent._working_dir, subject="new msg")
        result = handle(agent, {"action": "check"})
        assert result["messages"][0]["unread"] is True
        assert result["unread"] == 1

        # After reading, unread should be False
        handle(agent, {"action": "read", "id": [msg_id]})
        result = handle(agent, {"action": "check"})
        assert result["messages"][0]["unread"] is False
        assert result["unread"] == 0


# ---------------------------------------------------------------------------
# read tests
# ---------------------------------------------------------------------------

class TestRead:
    def test_read_by_id(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        msg_id = _make_inbox_message(
            agent._working_dir,
            sender="bob",
            subject="important",
            message="details here",
        )
        result = handle(agent, {"action": "read", "id": [msg_id]})
        assert len(result["messages"]) == 1
        msg = result["messages"][0]
        assert msg["from"] == "bob"
        assert msg["subject"] == "important"
        assert msg["message"] == "details here"

    def test_read_marks_as_read(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        msg_id = _make_inbox_message(agent._working_dir)

        # Before read
        from lingtai_kernel.intrinsics.mail import _read_ids
        assert msg_id not in _read_ids(agent)

        handle(agent, {"action": "read", "id": [msg_id]})
        assert msg_id in _read_ids(agent)

    def test_read_not_found(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {"action": "read", "id": ["nonexistent-id"]})
        assert result["not_found"] == ["nonexistent-id"]
        assert result["messages"] == []

    def test_read_multiple(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        id1 = _make_inbox_message(agent._working_dir, subject="first")
        id2 = _make_inbox_message(agent._working_dir, subject="second")
        result = handle(agent, {"action": "read", "id": [id1, id2]})
        assert len(result["messages"]) == 2
        subjects = {m["subject"] for m in result["messages"]}
        assert subjects == {"first", "second"}

    def test_read_shows_attachments(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        msg_id = _make_inbox_message(
            agent._working_dir,
            subject="with files",
            attachments=["/path/to/file.txt"],
        )
        result = handle(agent, {"action": "read", "id": [msg_id]})
        assert result["messages"][0]["attachments"] == ["/path/to/file.txt"]

    def test_read_no_ids(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {"action": "read"})
        assert "error" in result


# ---------------------------------------------------------------------------
# search tests
# ---------------------------------------------------------------------------

class TestSearch:
    def test_search_by_subject(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        _make_inbox_message(agent._working_dir, subject="project update")
        _make_inbox_message(agent._working_dir, subject="lunch plans")
        result = handle(agent, {"action": "search", "query": "project"})
        assert result["total"] == 1
        assert result["messages"][0]["subject"] == "project update"

    def test_search_by_sender(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        _make_inbox_message(agent._working_dir, sender="alice@example.com")
        _make_inbox_message(agent._working_dir, sender="bob@example.com")
        result = handle(agent, {"action": "search", "query": "alice"})
        assert result["total"] == 1

    def test_search_by_body(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        _make_inbox_message(agent._working_dir, message="the quick brown fox")
        _make_inbox_message(agent._working_dir, message="lazy dog")
        result = handle(agent, {"action": "search", "query": "brown fox"})
        assert result["total"] == 1

    def test_search_empty_query(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {"action": "search", "query": ""})
        assert "error" in result

    def test_search_invalid_regex(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {"action": "search", "query": "[invalid"})
        assert "error" in result
        assert "regex" in result["error"].lower()


# ---------------------------------------------------------------------------
# delete tests
# ---------------------------------------------------------------------------

class TestDelete:
    def test_delete_removes_from_disk(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        msg_id = _make_inbox_message(agent._working_dir)
        msg_dir = agent._working_dir / "mailbox" / "inbox" / msg_id
        assert msg_dir.is_dir()

        result = handle(agent, {"action": "delete", "id": [msg_id]})
        assert result["deleted"] == [msg_id]
        assert not msg_dir.is_dir()

    def test_delete_not_found(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {"action": "delete", "id": ["nonexistent"]})
        assert result["not_found"] == ["nonexistent"]
        assert result["deleted"] == []

    def test_delete_multiple(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        id1 = _make_inbox_message(agent._working_dir, subject="a")
        id2 = _make_inbox_message(agent._working_dir, subject="b")
        result = handle(agent, {"action": "delete", "id": [id1, id2]})
        assert set(result["deleted"]) == {id1, id2}
        inbox = agent._working_dir / "mailbox" / "inbox"
        assert len(list(inbox.iterdir())) == 0

    def test_delete_cleans_read_tracking(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        msg_id = _make_inbox_message(agent._working_dir)
        # Mark as read
        handle(agent, {"action": "read", "id": [msg_id]})
        from lingtai_kernel.intrinsics.mail import _read_ids
        assert msg_id in _read_ids(agent)

        # Delete should clean from read.json
        handle(agent, {"action": "delete", "id": [msg_id]})
        assert msg_id not in _read_ids(agent)

    def test_delete_no_ids(self, tmp_path):
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {"action": "delete"})
        assert "error" in result


# ---------------------------------------------------------------------------
# outbox / mailman tests
# ---------------------------------------------------------------------------

class TestOutboxAndMailman:
    def test_send_writes_to_outbox_then_sent(self, tmp_path):
        """Every send writes to outbox, mailman moves to sent."""
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "subject": "hello",
            "message": "world",
        })
        assert result["status"] == "sent"
        assert result["delay"] == 0
        time.sleep(0.2)
        outbox = agent._working_dir / "mailbox" / "outbox"
        if outbox.exists():
            assert len(list(outbox.iterdir())) == 0
        sent = agent._working_dir / "mailbox" / "sent"
        assert sent.is_dir()
        sent_items = list(sent.iterdir())
        assert len(sent_items) == 1
        msg = json.loads((sent_items[0] / "message.json").read_text())
        assert msg["message"] == "world"
        assert msg["sent_at"]
        assert msg["status"] == "delivered"

    def test_send_with_delay(self, tmp_path):
        """Delayed send writes to outbox, mailman waits before dispatch."""
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "subject": "delayed",
            "message": "later",
            "delay": 1,
        })
        assert result["status"] == "sent"
        assert result["delay"] == 1
        agent._mail_service.send.assert_not_called()
        outbox = agent._working_dir / "mailbox" / "outbox"
        assert len(list(outbox.iterdir())) == 1
        time.sleep(1.5)
        agent._mail_service.send.assert_called_once()
        assert len(list(outbox.iterdir())) == 0
        sent = agent._working_dir / "mailbox" / "sent"
        assert len(list(sent.iterdir())) == 1

    def test_send_returns_delay_zero(self, tmp_path):
        """Default delay=0 always appears in return."""
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "hi",
        })
        assert result["delay"] == 0

    def test_self_send_through_mailman(self, tmp_path):
        """Self-send goes through outbox → mailman → inbox + sent."""
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:9999",
            "subject": "note",
            "message": "remember",
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        inbox = agent._working_dir / "mailbox" / "inbox"
        assert len(list(inbox.iterdir())) == 1
        assert agent._mail_arrived.is_set()
        agent._mail_service.send.assert_not_called()
        sent = agent._working_dir / "mailbox" / "sent"
        assert len(list(sent.iterdir())) == 1

    def test_send_no_mail_service_external(self, tmp_path):
        """External send with no mail service — mailman writes refused to sent."""
        agent = _make_mock_agent(tmp_path)
        agent._mail_service = None
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "hello",
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        sent = agent._working_dir / "mailbox" / "sent"
        assert sent.is_dir()
        sent_items = list(sent.iterdir())
        assert len(sent_items) == 1
        msg = json.loads((sent_items[0] / "message.json").read_text())
        assert msg["status"] == "refused"

    def test_send_refused_external(self, tmp_path):
        """External send refused by mail service — mailman writes refused to sent."""
        agent = _make_mock_agent(tmp_path)
        agent._mail_service.send.return_value = "connection refused"
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "hello",
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        sent = agent._working_dir / "mailbox" / "sent"
        sent_items = list(sent.iterdir())
        msg = json.loads((sent_items[0] / "message.json").read_text())
        assert msg["status"] == "refused"

    def test_mailman_exception_still_moves_to_sent(self, tmp_path):
        """If mail_service.send() raises, mailman writes refused to sent."""
        agent = _make_mock_agent(tmp_path)
        agent._mail_service.send.side_effect = ConnectionError("boom")
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "hello",
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        outbox = agent._working_dir / "mailbox" / "outbox"
        if outbox.exists():
            assert len(list(outbox.iterdir())) == 0
        sent = agent._working_dir / "mailbox" / "sent"
        sent_items = list(sent.iterdir())
        assert len(sent_items) == 1
        msg = json.loads((sent_items[0] / "message.json").read_text())
        assert msg["status"] == "refused"
