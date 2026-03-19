from __future__ import annotations
import json
from pathlib import Path
from unittest.mock import MagicMock
from stoai.addons.gmail.manager import GmailManager


def test_check_returns_tcp_alias(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({"action": "check"})
    assert result["tcp_alias"] == "127.0.0.1:8399"
    assert result["account"] == "agent@gmail.com"


def test_check_lists_gmail_inbox(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    eid = "test-email-1"
    msg_dir = tmp_path / "gmail" / "inbox" / eid
    msg_dir.mkdir(parents=True)
    (msg_dir / "message.json").write_text(json.dumps({
        "from": "user@gmail.com", "to": ["agent@gmail.com"],
        "subject": "hello", "message": "hi there",
        "_mailbox_id": eid, "received_at": "2026-03-18T12:00:00Z",
    }))

    result = mgr.handle({"action": "check"})
    assert len(result["emails"]) == 1
    assert result["emails"][0]["from"] == "user@gmail.com"


def test_send_uses_gmail_service(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    svc.send.return_value = None
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({
        "action": "send", "address": "user@gmail.com",
        "subject": "test", "message": "hello",
    })
    assert result["status"] == "delivered"
    svc.send.assert_called_once()


def test_every_response_has_meta(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    for action in ["check", "contacts"]:
        result = mgr.handle({"action": action})
        assert "tcp_alias" in result
        assert "account" in result

    result = mgr.handle({"action": "read", "email_id": "nope"})
    assert "tcp_alias" in result


def test_on_gmail_received_notifies_agent(tmp_path):
    """on_gmail_received should enqueue a notification to the agent inbox."""
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    payload = {
        "_mailbox_id": "test-123",
        "from": "user@gmail.com",
        "subject": "hello",
        "message": "hi there",
    }
    mgr.on_gmail_received(payload)

    # Should have enqueued a message
    agent.inbox.put.assert_called_once()
    msg = agent.inbox.put.call_args[0][0]
    assert hasattr(msg, "_mail_notification")
    assert msg._mail_notification["email_id"] == "test-123"
    assert msg._mail_notification["sender"] == "user@gmail.com"
    # Should have logged
    agent._log.assert_called_once()


def test_duplicate_send_blocked(tmp_path):
    """Sending the same message 3+ times should be blocked."""
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    svc.send.return_value = None
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    args = {"action": "send", "address": "user@gmail.com", "subject": "x", "message": "same"}

    # First two sends should succeed (free passes)
    r1 = mgr.handle(args)
    assert r1["status"] == "delivered"
    r2 = mgr.handle(args)
    assert r2["status"] == "delivered"

    # Third identical send should be blocked
    r3 = mgr.handle(args)
    assert r3["status"] == "blocked"


def test_start_stop_lifecycle(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")
    mgr._bridge = MagicMock()

    mgr.start()
    svc.listen.assert_called_once()
    mgr._bridge.listen.assert_called_once()

    mgr.stop()
    svc.stop.assert_called_once()
    mgr._bridge.stop.assert_called_once()
