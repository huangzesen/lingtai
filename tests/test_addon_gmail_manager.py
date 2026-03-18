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
