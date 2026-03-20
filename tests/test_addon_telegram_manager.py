# tests/test_addon_telegram_manager.py
from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock, patch

from stoai.addons.telegram.manager import TelegramManager


def _make_manager(tmp_path) -> tuple[TelegramManager, MagicMock, MagicMock]:
    """Helper to create a TelegramManager with mocked agent and service."""
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.default_account = MagicMock()
    svc.default_account.alias = "default"
    svc.list_accounts.return_value = ["default"]
    mgr = TelegramManager(agent=agent, service=svc, working_dir=tmp_path)
    return mgr, agent, svc


def test_check_empty_inbox(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    result = mgr.handle({"action": "check"})
    assert result["status"] == "ok"
    assert result["total"] == 0


def test_check_with_messages(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    # Create an inbox message
    msg_dir = tmp_path / "telegram" / "default" / "inbox" / "uuid-1"
    msg_dir.mkdir(parents=True)
    (msg_dir / "message.json").write_text(json.dumps({
        "id": "default:111:1",
        "from": {"id": 111, "username": "alice", "first_name": "Alice"},
        "chat": {"id": 111, "type": "private"},
        "date": "2026-03-20T10:00:00Z",
        "text": "Hello",
    }))
    result = mgr.handle({"action": "check", "account": "default"})
    assert result["total"] == 1
    assert result["messages"][0]["chat_id"] == 111
    assert result["messages"][0]["unread"] == 1
    assert result["messages"][0]["last_from"]["username"] == "alice"


def test_read_marks_as_read(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    msg_dir = tmp_path / "telegram" / "default" / "inbox" / "uuid-1"
    msg_dir.mkdir(parents=True)
    (msg_dir / "message.json").write_text(json.dumps({
        "id": "default:111:1",
        "from": {"id": 111, "username": "alice", "first_name": "Alice"},
        "chat": {"id": 111, "type": "private"},
        "date": "2026-03-20T10:00:00Z",
        "text": "Hello",
    }))
    result = mgr.handle({"action": "read", "account": "default", "chat_id": 111})
    assert result["status"] == "ok"
    assert len(result["messages"]) == 1

    # Check should now show it as read
    read_ids = mgr._read_ids("default")
    assert "default:111:1" in read_ids


def test_send_text(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct_mock = MagicMock()
    acct_mock.alias = "default"
    acct_mock.send_message.return_value = {"message_id": 100}
    svc.get_account.return_value = acct_mock
    svc.default_account = acct_mock

    result = mgr.handle({"action": "send", "chat_id": 111, "text": "Hi!"})
    assert result["status"] == "sent"
    acct_mock.send_message.assert_called_once()
    # Should persist to sent/
    sent_dir = tmp_path / "telegram" / "default" / "sent"
    assert sent_dir.exists()
    sent_files = list(sent_dir.iterdir())
    assert len(sent_files) == 1


def test_send_with_photo(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct_mock = MagicMock()
    acct_mock.alias = "default"
    acct_mock.send_photo.return_value = {"message_id": 101}
    svc.get_account.return_value = acct_mock
    svc.default_account = acct_mock

    photo = tmp_path / "photo.png"
    photo.write_bytes(b"\x89PNG")

    result = mgr.handle({
        "action": "send", "chat_id": 111, "text": "See photo",
        "media": {"type": "photo", "path": str(photo)},
    })
    assert result["status"] == "sent"
    acct_mock.send_photo.assert_called_once()


def test_reply(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    # Create an inbox message to reply to
    msg_dir = tmp_path / "telegram" / "default" / "inbox" / "uuid-1"
    msg_dir.mkdir(parents=True)
    (msg_dir / "message.json").write_text(json.dumps({
        "id": "default:111:50",
        "from": {"id": 111, "username": "alice", "first_name": "Alice"},
        "chat": {"id": 111, "type": "private"},
        "date": "2026-03-20T10:00:00Z",
        "text": "Help me",
    }))
    acct_mock = MagicMock()
    acct_mock.alias = "default"
    acct_mock.send_message.return_value = {"message_id": 51}
    svc.get_account.return_value = acct_mock
    svc.default_account = acct_mock

    result = mgr.handle({
        "action": "reply", "message_id": "default:111:50", "text": "Sure!",
    })
    assert result["status"] == "sent"
    acct_mock.send_message.assert_called_once()
    call_kwargs = acct_mock.send_message.call_args[1]
    assert call_kwargs.get("reply_to_message_id") == 50


def test_search(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    msg_dir = tmp_path / "telegram" / "default" / "inbox" / "uuid-1"
    msg_dir.mkdir(parents=True)
    (msg_dir / "message.json").write_text(json.dumps({
        "id": "default:111:1",
        "from": {"id": 111, "username": "alice", "first_name": "Alice"},
        "chat": {"id": 111, "type": "private"},
        "date": "2026-03-20T10:00:00Z",
        "text": "I need help with my ORDER-123",
    }))
    result = mgr.handle({"action": "search", "query": "ORDER-123"})
    assert result["total"] == 1


def test_contacts_crud(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    # Add
    result = mgr.handle({
        "action": "add_contact", "account": "default",
        "chat_id": 111, "alias": "alice",
    })
    assert result["status"] == "added"
    # List
    result = mgr.handle({"action": "contacts", "account": "default"})
    assert len(result["contacts"]) == 1
    assert result["contacts"]["alice"]["chat_id"] == 111
    # Remove
    result = mgr.handle({
        "action": "remove_contact", "account": "default", "alias": "alice",
    })
    assert result["status"] == "removed"
    result = mgr.handle({"action": "contacts", "account": "default"})
    assert len(result["contacts"]) == 0


def test_accounts_action(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    result = mgr.handle({"action": "accounts"})
    assert result["accounts"] == ["default"]


def test_on_incoming_persists_and_notifies(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct_mock = MagicMock()
    acct_mock.alias = "default"
    acct_mock.get_file.return_value = ("photo.jpg", b"\xff\xd8")
    svc.get_account.return_value = acct_mock

    update = {
        "update_id": 1,
        "message": {
            "message_id": 42,
            "from": {"id": 111, "username": "alice", "first_name": "Alice"},
            "chat": {"id": 111, "type": "private"},
            "date": 1710928200,
            "text": "Hello!",
        },
    }
    mgr.on_incoming("default", update)

    # Should persist to inbox
    inbox_dir = tmp_path / "telegram" / "default" / "inbox"
    assert inbox_dir.exists()
    msg_dirs = list(inbox_dir.iterdir())
    assert len(msg_dirs) == 1

    # Should notify agent
    agent._mail_arrived.set.assert_called_once()
    agent.inbox.put.assert_called_once()
    agent._log.assert_called_once()


def test_on_incoming_with_photo(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct_mock = MagicMock()
    acct_mock.alias = "default"
    acct_mock.get_file.return_value = ("photo.jpg", b"\xff\xd8\xff")
    svc.get_account.return_value = acct_mock

    update = {
        "update_id": 1,
        "message": {
            "message_id": 42,
            "from": {"id": 111, "username": "alice", "first_name": "Alice"},
            "chat": {"id": 111, "type": "private"},
            "date": 1710928200,
            "caption": "Check this out",
            "photo": [
                {"file_id": "small_id", "width": 100, "height": 100},
                {"file_id": "large_id", "width": 800, "height": 600},
            ],
        },
    }
    mgr.on_incoming("default", update)

    # Should download the largest photo
    acct_mock.get_file.assert_called_once_with("large_id")
    # Should persist with attachment
    inbox_dir = tmp_path / "telegram" / "default" / "inbox"
    msg_dirs = list(inbox_dir.iterdir())
    assert len(msg_dirs) == 1
    msg = json.loads((msg_dirs[0] / "message.json").read_text())
    assert msg["media"]["type"] == "photo"
    assert msg["media"]["filename"] == "photo.jpg"
    # Attachment file should exist on disk
    att_path = msg_dirs[0] / "attachments" / "photo.jpg"
    assert att_path.is_file()
    assert att_path.read_bytes() == b"\xff\xd8\xff"


def test_on_incoming_callback_query(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    update = {
        "update_id": 2,
        "callback_query": {
            "id": "cq-1",
            "from": {"id": 111, "username": "alice", "first_name": "Alice"},
            "message": {
                "message_id": 42,
                "chat": {"id": 111, "type": "private"},
            },
            "data": "yes",
        },
    }
    mgr.on_incoming("default", update)

    # Should persist as regular inbox message with callback_query field
    inbox_dir = tmp_path / "telegram" / "default" / "inbox"
    msg_dirs = list(inbox_dir.iterdir())
    assert len(msg_dirs) == 1
    msg = json.loads((msg_dirs[0] / "message.json").read_text())
    assert msg["callback_query"] == "yes"


def test_on_incoming_edited_message(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    update = {
        "update_id": 3,
        "edited_message": {
            "message_id": 42,
            "from": {"id": 111, "username": "alice", "first_name": "Alice"},
            "chat": {"id": 111, "type": "private"},
            "date": 1710928200,
            "edit_date": 1710928300,
            "text": "Updated message",
        },
    }
    mgr.on_incoming("default", update)

    inbox_dir = tmp_path / "telegram" / "default" / "inbox"
    msg_dirs = list(inbox_dir.iterdir())
    assert len(msg_dirs) == 1
    msg = json.loads((msg_dirs[0] / "message.json").read_text())
    assert msg["text"] == "Updated message"


def test_duplicate_send_blocked(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct_mock = MagicMock()
    acct_mock.alias = "default"
    acct_mock.send_message.return_value = {"message_id": 100}
    svc.get_account.return_value = acct_mock
    svc.default_account = acct_mock

    args = {"action": "send", "chat_id": 111, "text": "same message"}
    # First two sends succeed (free passes)
    r1 = mgr.handle(args)
    assert r1["status"] == "sent"
    r2 = mgr.handle(args)
    assert r2["status"] == "sent"
    # Third identical send blocked
    r3 = mgr.handle(args)
    assert r3["status"] == "blocked"


def test_delete_action(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct_mock = MagicMock()
    acct_mock.alias = "default"
    acct_mock.delete_message.return_value = True
    svc.get_account.return_value = acct_mock
    svc.default_account = acct_mock

    result = mgr.handle({
        "action": "delete", "message_id": "default:111:42",
    })
    assert result["status"] == "deleted"
    acct_mock.delete_message.assert_called_once_with(chat_id=111, message_id=42)


def test_edit_action(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct_mock = MagicMock()
    acct_mock.alias = "default"
    acct_mock.edit_message.return_value = {}
    svc.get_account.return_value = acct_mock
    svc.default_account = acct_mock

    result = mgr.handle({
        "action": "edit", "message_id": "default:111:42", "text": "updated",
    })
    assert result["status"] == "edited"
    acct_mock.edit_message.assert_called_once()


def test_start_stop_lifecycle(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    mgr.start()
    svc.start.assert_called_once()
    mgr.stop()
    svc.stop.assert_called_once()
