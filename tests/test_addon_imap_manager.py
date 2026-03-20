from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock, PropertyMock

import pytest

from stoai.addons.imap.manager import IMAPMailManager, parse_email_id


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

def _make_account(address: str = "alice@gmail.com", connected: bool = True) -> MagicMock:
    """Create a mock IMAPAccount."""
    acct = MagicMock()
    acct.address = address
    acct.connected = connected
    return acct


def _make_service(default_address: str = "alice@gmail.com") -> MagicMock:
    """Create a mock IMAPMailService with one default account."""
    svc = MagicMock()
    default_acct = _make_account(default_address)
    svc.default_account = default_acct
    svc.accounts = [default_acct]
    svc.get_account.return_value = default_acct
    return svc


def _make_manager(tmp_path: Path) -> tuple[IMAPMailManager, MagicMock, MagicMock]:
    """Return (manager, agent_mock, service_mock)."""
    agent = MagicMock()
    agent._working_dir = str(tmp_path)
    svc = _make_service()
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")
    return mgr, agent, svc


# ---------------------------------------------------------------------------
# parse_email_id
# ---------------------------------------------------------------------------

class TestParseEmailId:
    def test_basic(self):
        assert parse_email_id("alice@gmail.com:INBOX:1042") == (
            "alice@gmail.com", "INBOX", "1042",
        )

    def test_folder_with_slash(self):
        assert parse_email_id("a@b.com:[Gmail]/Sent Mail:999") == (
            "a@b.com", "[Gmail]/Sent Mail", "999",
        )

    def test_folder_with_colon_in_name(self):
        # Edge case: folder name containing a colon
        assert parse_email_id("x@y.com:Folder:Sub:42") == (
            "x@y.com", "Folder:Sub", "42",
        )


# ---------------------------------------------------------------------------
# check
# ---------------------------------------------------------------------------

def test_check_delegates_to_account(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.fetch_envelopes.return_value = [
        {"email_id": "alice@gmail.com:INBOX:1", "uid": "1", "from": "bob@x.com",
         "to": "alice@gmail.com", "subject": "hi", "date": "2026-01-01", "flags": []},
    ]

    result = mgr.handle({"action": "check", "folder": "INBOX", "n": 5})
    acct.fetch_envelopes.assert_called_once_with("INBOX", 5)
    assert result["status"] == "ok"
    assert result["total"] == 1
    assert result["tcp_alias"] == "127.0.0.1:8399"
    assert result["account"] == "alice@gmail.com"


def test_check_defaults(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.fetch_envelopes.return_value = []

    mgr.handle({"action": "check"})
    acct.fetch_envelopes.assert_called_once_with("INBOX", 10)


# ---------------------------------------------------------------------------
# read
# ---------------------------------------------------------------------------

def test_read_delegates_to_account(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.fetch_full.return_value = {
        "uid": "42", "from": "bob@x.com", "from_address": "bob@x.com",
        "to": "alice@gmail.com", "subject": "hi", "date": "2026-01-01",
        "body": "hello", "attachments": [], "attachments_raw": [],
        "flags": [], "message_id": "<abc@x>", "references": "",
        "email_id": "alice@gmail.com:INBOX:42",
    }

    result = mgr.handle({"action": "read", "email_id": ["alice@gmail.com:INBOX:42"]})
    acct.fetch_full.assert_called_once_with("INBOX", "42")
    assert result["status"] == "ok"
    assert len(result["emails"]) == 1
    assert result["emails"][0]["message"] == "hello"


def test_read_persists_to_disk(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.fetch_full.return_value = {
        "uid": "42", "from": "bob@x.com", "from_address": "bob@x.com",
        "to": "alice@gmail.com", "subject": "hi", "date": "2026-01-01",
        "body": "hello", "attachments": [], "attachments_raw": [
            {"filename": "photo.png", "data": b"\x89PNG", "content_type": "image/png"},
        ],
        "flags": [], "message_id": "<abc@x>", "references": "",
        "email_id": "alice@gmail.com:INBOX:42",
    }

    mgr.handle({"action": "read", "email_id": ["alice@gmail.com:INBOX:42"]})

    msg_path = tmp_path / "imap" / "alice@gmail.com" / "INBOX" / "42" / "message.json"
    assert msg_path.is_file()
    data = json.loads(msg_path.read_text())
    assert data["subject"] == "hi"
    assert data["message"] == "hello"

    # Attachment saved to disk
    att_path = tmp_path / "imap" / "alice@gmail.com" / "INBOX" / "42" / "photo.png"
    assert att_path.is_file()
    assert att_path.read_bytes() == b"\x89PNG"


def test_read_normalizes_string_email_id(tmp_path):
    """email_id as string should be normalized to list."""
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.fetch_full.return_value = {
        "uid": "1", "from": "b@x.com", "from_address": "b@x.com",
        "to": "a@x.com", "subject": "s", "date": "", "body": "b",
        "attachments": [], "attachments_raw": [], "flags": [],
        "message_id": "", "references": "", "email_id": "alice@gmail.com:INBOX:1",
    }

    result = mgr.handle({"action": "read", "email_id": "alice@gmail.com:INBOX:1"})
    assert result["status"] == "ok"


# ---------------------------------------------------------------------------
# accounts
# ---------------------------------------------------------------------------

def test_accounts_action(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    result = mgr.handle({"action": "accounts"})
    assert result["status"] == "ok"
    assert len(result["accounts"]) == 1
    assert result["accounts"][0]["address"] == "alice@gmail.com"
    assert result["accounts"][0]["connected"] is True


# ---------------------------------------------------------------------------
# folders
# ---------------------------------------------------------------------------

def test_folders_action(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.list_folders.return_value = {"INBOX": None, "[Gmail]/Sent Mail": "sent", "[Gmail]/Trash": "trash"}

    result = mgr.handle({"action": "folders"})
    assert result["status"] == "ok"
    names = [f["name"] for f in result["folders"]]
    assert "INBOX" in names
    assert "[Gmail]/Sent Mail" in names


# ---------------------------------------------------------------------------
# delete
# ---------------------------------------------------------------------------

def test_delete_action(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.delete_message.return_value = True

    result = mgr.handle({"action": "delete", "email_id": ["alice@gmail.com:INBOX:42"]})
    acct.delete_message.assert_called_once_with("INBOX", "42")
    assert result["status"] == "ok"
    assert result["results"][0]["deleted"] is True


# ---------------------------------------------------------------------------
# move
# ---------------------------------------------------------------------------

def test_move_action(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.move_message.return_value = True

    result = mgr.handle({
        "action": "move",
        "email_id": ["alice@gmail.com:INBOX:42"],
        "folder": "[Gmail]/Trash",
    })
    acct.move_message.assert_called_once_with("INBOX", "42", "[Gmail]/Trash")
    assert result["status"] == "ok"
    assert result["results"][0]["moved"] is True


# ---------------------------------------------------------------------------
# flag
# ---------------------------------------------------------------------------

def test_flag_action(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.store_flags.return_value = True

    result = mgr.handle({
        "action": "flag",
        "email_id": ["alice@gmail.com:INBOX:42"],
        "flags": {"seen": True, "flagged": False},
    })
    # +FLAGS for seen, -FLAGS for flagged
    assert acct.store_flags.call_count == 2
    calls = acct.store_flags.call_args_list
    # First call: +FLAGS \\Seen
    assert calls[0] == (("INBOX", "42", ["\\Seen"]), {"action": "+FLAGS"})
    # Second call: -FLAGS \\Flagged
    assert calls[1] == (("INBOX", "42", ["\\Flagged"]), {"action": "-FLAGS"})
    assert result["status"] == "ok"


# ---------------------------------------------------------------------------
# search
# ---------------------------------------------------------------------------

def test_search_action(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.search.return_value = ["10", "20"]
    acct.fetch_headers_by_uids.return_value = [
        {"uid": "10", "from": "a@x.com", "subject": "a"},
        {"uid": "20", "from": "b@x.com", "subject": "b"},
    ]

    result = mgr.handle({"action": "search", "query": "from:test", "folder": "INBOX"})
    acct.search.assert_called_once_with("INBOX", "from:test")
    acct.fetch_headers_by_uids.assert_called_once_with("INBOX", ["10", "20"])
    assert result["total"] == 2


# ---------------------------------------------------------------------------
# reply
# ---------------------------------------------------------------------------

def test_reply_threading(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.fetch_full.return_value = {
        "uid": "42", "from": "Bob <bob@x.com>", "from_address": "bob@x.com",
        "to": "alice@gmail.com", "subject": "Question",
        "date": "2026-01-01", "body": "original text",
        "attachments": [], "attachments_raw": [], "flags": [],
        "message_id": "<msg123@x.com>",
        "references": "<prev@x.com>",
        "email_id": "alice@gmail.com:INBOX:42",
    }
    acct.send_email.return_value = None
    acct.store_flags.return_value = True

    result = mgr.handle({
        "action": "reply",
        "email_id": ["alice@gmail.com:INBOX:42"],
        "message": "Thanks!",
    })

    # Verify fetch_full called
    acct.fetch_full.assert_called_once_with("INBOX", "42")

    # Verify send_email called with threading headers
    acct.send_email.assert_called_once()
    call_args = acct.send_email.call_args
    # Could be positional or keyword — check both ways
    kwargs = call_args.kwargs
    args = call_args.args
    # to is first positional arg
    to = args[0] if args else kwargs.get("to")
    subject = args[1] if len(args) > 1 else kwargs.get("subject")
    body = args[2] if len(args) > 2 else kwargs.get("body")
    assert to == ["bob@x.com"]
    assert subject == "Re: Question"
    assert kwargs.get("in_reply_to") == "<msg123@x.com>"
    assert kwargs.get("references") == "<prev@x.com> <msg123@x.com>"

    # Verify store_flags called with answered
    acct.store_flags.assert_called_once_with("INBOX", "42", ["\\Answered"])

    assert result["status"] == "delivered"


# ---------------------------------------------------------------------------
# send
# ---------------------------------------------------------------------------

def test_send_with_cc_bcc(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.send_email.return_value = None

    result = mgr.handle({
        "action": "send",
        "address": "to@x.com",
        "subject": "test",
        "message": "hello",
        "cc": "cc@x.com",
        "bcc": ["bcc1@x.com", "bcc2@x.com"],
    })

    assert result["status"] == "delivered"
    call_kw = acct.send_email.call_args
    assert call_kw[1]["cc"] == ["cc@x.com"]
    assert call_kw[1]["bcc"] == ["bcc1@x.com", "bcc2@x.com"]


# ---------------------------------------------------------------------------
# contacts (per-account)
# ---------------------------------------------------------------------------

def test_contacts_per_account(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)

    # Add a contact
    result = mgr.handle({
        "action": "add_contact",
        "address": "bob@x.com",
        "name": "Bob",
        "note": "colleague",
    })
    assert result["status"] == "added"

    # Verify path is imap/{address}/contacts.json
    contacts_path = tmp_path / "imap" / "alice@gmail.com" / "contacts.json"
    assert contacts_path.is_file()
    data = json.loads(contacts_path.read_text())
    assert len(data) == 1
    assert data[0]["address"] == "bob@x.com"

    # List contacts
    result = mgr.handle({"action": "contacts"})
    assert result["status"] == "ok"
    assert len(result["contacts"]) == 1

    # Edit contact
    result = mgr.handle({
        "action": "edit_contact",
        "address": "bob@x.com",
        "note": "updated note",
    })
    assert result["status"] == "updated"

    # Remove contact
    result = mgr.handle({
        "action": "remove_contact",
        "address": "bob@x.com",
    })
    assert result["status"] == "removed"

    # Verify empty
    result = mgr.handle({"action": "contacts"})
    assert len(result["contacts"]) == 0


# ---------------------------------------------------------------------------
# duplicate send protection
# ---------------------------------------------------------------------------

def test_duplicate_send_blocked(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.send_email.return_value = None

    args = {"action": "send", "address": "user@x.com", "subject": "x", "message": "same"}

    # First two sends should succeed (free passes)
    r1 = mgr.handle(args)
    assert r1["status"] == "delivered"
    r2 = mgr.handle(args)
    assert r2["status"] == "delivered"

    # Third identical send should be blocked
    r3 = mgr.handle(args)
    assert r3["status"] == "blocked"


# ---------------------------------------------------------------------------
# on_imap_received
# ---------------------------------------------------------------------------

def test_on_imap_received_notifies_agent(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)

    payload = {
        "account": "alice@gmail.com",
        "email_id": "alice@gmail.com:INBOX:99",
        "from": "user@x.com",
        "subject": "hello",
        "message": "hi there",
    }
    mgr.on_imap_received(payload)

    # Should have enqueued a message
    agent.inbox.put.assert_called_once()
    msg = agent.inbox.put.call_args[0][0]
    assert msg.sender == "system"
    assert "imap box" in msg.content
    assert 'imap(action="check")' in msg.content

    # Should have logged
    agent._log.assert_called_once()
    log_args = agent._log.call_args
    assert log_args[0][0] == "imap_received"

    # Should have signaled mail arrival
    agent._mail_arrived.set.assert_called_once()


# ---------------------------------------------------------------------------
# meta injection
# ---------------------------------------------------------------------------

def test_every_response_has_meta(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    acct = svc.get_account.return_value
    acct.fetch_envelopes.return_value = []
    acct.list_folders.return_value = {}

    for action in ["check", "contacts", "folders", "accounts"]:
        result = mgr.handle({"action": action})
        assert "tcp_alias" in result, f"Missing tcp_alias for {action}"
        assert "account" in result, f"Missing account for {action}"


# ---------------------------------------------------------------------------
# lifecycle
# ---------------------------------------------------------------------------

def test_start_stop_lifecycle(tmp_path):
    mgr, agent, svc = _make_manager(tmp_path)
    mgr._bridge = MagicMock()

    mgr.start()
    svc.listen.assert_called_once()
    mgr._bridge.listen.assert_called_once()

    mgr.stop()
    svc.stop.assert_called_once()
    mgr._bridge.stop.assert_called_once()
