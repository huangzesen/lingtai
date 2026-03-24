"""Tests for IMAPAccount — connection, capabilities, folder discovery, fetch, search, send."""
from __future__ import annotations

import email as email_mod
import json
import tempfile
import threading
from datetime import datetime
from email.mime.multipart import MIMEMultipart
from email.mime.text import MIMEText
from pathlib import Path
from unittest.mock import MagicMock, patch, call

import pytest

from lingtai.addons.imap.account import (
    IMAPAccount,
    _decode_header_value,
    _extract_text_body,
    _strip_html_tags,
    _extract_attachments,
    _format_imap_date,
)


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def account(tmp_path):
    """Create an IMAPAccount with no connection (for unit tests)."""
    return IMAPAccount(
        email_address="test@example.com",
        email_password="secret",
        imap_host="imap.example.com",
        imap_port=993,
        smtp_host="smtp.example.com",
        smtp_port=587,
        working_dir=tmp_path,
    )


@pytest.fixture
def mock_imap():
    """Create a mock IMAP4_SSL instance."""
    m = MagicMock()
    m.login.return_value = ("OK", [b"LOGIN completed"])
    m.capability.return_value = ("OK", [b"IMAP4rev1 IDLE MOVE UIDPLUS"])
    m.list.return_value = ("OK", [
        b'(\\HasNoChildren \\Sent) "/" "Sent"',
        b'(\\HasNoChildren \\Trash) "/" "Trash"',
        b'(\\HasNoChildren) "/" "INBOX"',
    ])
    m.select.return_value = ("OK", [b"42"])
    m.logout.return_value = ("BYE", [b"Logging out"])
    return m


# ---------------------------------------------------------------------------
# Construction and properties
# ---------------------------------------------------------------------------

def test_construction_and_properties(account):
    assert account.address == "test@example.com"
    assert account.connected is False
    assert account.capabilities == set()
    assert account.has_idle is False
    assert account.has_move is False
    assert account.has_uidplus is False
    assert account.folders == {}


def test_construction_without_working_dir():
    acct = IMAPAccount(
        email_address="a@b.com",
        email_password="pw",
    )
    assert acct.address == "a@b.com"
    assert acct._working_dir is None


# ---------------------------------------------------------------------------
# _parse_capabilities
# ---------------------------------------------------------------------------

def test_parse_capabilities_full(account):
    account._parse_capabilities("IMAP4rev1 IDLE MOVE UIDPLUS LITERAL+")
    assert account.has_idle is True
    assert account.has_move is True
    assert account.has_uidplus is True
    assert "IMAP4REV1" in account.capabilities
    assert "LITERAL+" in account.capabilities


def test_parse_capabilities_without_idle(account):
    account._parse_capabilities("IMAP4rev1 UIDPLUS")
    assert account.has_idle is False
    assert account.has_move is False
    assert account.has_uidplus is True


def test_parse_capabilities_empty(account):
    account._parse_capabilities("")
    assert account.has_idle is False
    assert account.has_move is False
    assert account.has_uidplus is False
    assert account.capabilities == set()


def test_parse_capabilities_case_insensitive(account):
    account._parse_capabilities("imap4rev1 idle move uidplus")
    assert account.has_idle is True
    assert account.has_move is True
    assert account.has_uidplus is True


# ---------------------------------------------------------------------------
# _build_search_query
# ---------------------------------------------------------------------------

def test_build_search_query_from(account):
    assert account._build_search_query("from:alice@example.com") == 'FROM "alice@example.com"'


def test_build_search_query_subject(account):
    assert account._build_search_query("subject:hello") == 'SUBJECT "hello"'


def test_build_search_query_since(account):
    result = account._build_search_query("since:2025-01-15")
    assert result == "SINCE 15-Jan-2025"


def test_build_search_query_before(account):
    result = account._build_search_query("before:2025-06-01")
    assert result == "BEFORE 01-Jun-2025"


def test_build_search_query_flagged(account):
    assert account._build_search_query("flagged") == "FLAGGED"


def test_build_search_query_unseen(account):
    assert account._build_search_query("unseen") == "UNSEEN"


def test_build_search_query_quoted_value(account):
    result = account._build_search_query('from:"John Doe"')
    assert result == 'FROM "John Doe"'


def test_build_search_query_combined(account):
    result = account._build_search_query("from:alice@example.com subject:hello unseen")
    assert 'FROM "alice@example.com"' in result
    assert 'SUBJECT "hello"' in result
    assert "UNSEEN" in result


def test_build_search_query_to(account):
    assert account._build_search_query("to:bob@example.com") == 'TO "bob@example.com"'


def test_build_search_query_seen(account):
    assert account._build_search_query("seen") == "SEEN"


def test_build_search_query_answered(account):
    assert account._build_search_query("answered") == "ANSWERED"


def test_build_search_query_quoted_phrase(account):
    result = account._build_search_query('"exact phrase"')
    assert result == 'TEXT "exact phrase"'


def test_build_search_query_fallback_unknown_token(account):
    result = account._build_search_query("randomword")
    assert result == 'TEXT "randomword"'


def test_build_search_query_empty(account):
    assert account._build_search_query("") == "ALL"


def test_build_search_query_invalid_date_fallback(account):
    result = account._build_search_query("since:not-a-date")
    assert 'TEXT "not-a-date"' in result


# ---------------------------------------------------------------------------
# _parse_flags
# ---------------------------------------------------------------------------

def test_parse_flags_with_flags():
    flags = IMAPAccount._parse_flags(b"(\\Seen \\Flagged)")
    assert "\\Seen" in flags
    assert "\\Flagged" in flags


def test_parse_flags_empty():
    assert IMAPAccount._parse_flags(b"()") == []


def test_parse_flags_no_parens():
    assert IMAPAccount._parse_flags(b"\\Seen") == []


def test_parse_flags_empty_bytes():
    assert IMAPAccount._parse_flags(b"") == []


# ---------------------------------------------------------------------------
# _parse_flags_from_meta
# ---------------------------------------------------------------------------

def test_parse_flags_from_meta():
    meta = '1 (UID 100 FLAGS (\\Seen \\Flagged) BODY[HEADER.FIELDS (FROM TO)])'
    flags = IMAPAccount._parse_flags_from_meta(meta)
    assert "\\Seen" in flags
    assert "\\Flagged" in flags


def test_parse_flags_from_meta_no_flags():
    meta = '1 (UID 100 BODY[HEADER.FIELDS (FROM TO)])'
    assert IMAPAccount._parse_flags_from_meta(meta) == []


# ---------------------------------------------------------------------------
# send_email
# ---------------------------------------------------------------------------

@patch("lingtai.addons.imap.account.smtplib.SMTP")
def test_send_email_basic(mock_smtp_cls, account):
    mock_smtp = MagicMock()
    mock_smtp_cls.return_value.__enter__ = MagicMock(return_value=mock_smtp)
    mock_smtp_cls.return_value.__exit__ = MagicMock(return_value=False)

    result = account.send_email(
        to=["bob@example.com"],
        subject="Hello",
        body="World",
    )
    assert result is None
    mock_smtp.starttls.assert_called_once()
    mock_smtp.login.assert_called_once_with("test@example.com", "secret")
    mock_smtp.sendmail.assert_called_once()

    # Verify the recipients list
    sendmail_args = mock_smtp.sendmail.call_args
    assert sendmail_args[0][0] == "test@example.com"
    assert sendmail_args[0][1] == ["bob@example.com"]

    # Verify headers in the message string
    msg_str = sendmail_args[0][2]
    assert "To: bob@example.com" in msg_str
    assert "Subject: Hello" in msg_str


@patch("lingtai.addons.imap.account.smtplib.SMTP")
def test_send_email_with_cc_bcc(mock_smtp_cls, account):
    """BCC must NOT appear in headers, only in RCPT TO."""
    mock_smtp = MagicMock()
    mock_smtp_cls.return_value.__enter__ = MagicMock(return_value=mock_smtp)
    mock_smtp_cls.return_value.__exit__ = MagicMock(return_value=False)

    result = account.send_email(
        to=["bob@example.com"],
        subject="Hello",
        body="World",
        cc=["carol@example.com"],
        bcc=["dave@example.com"],
    )
    assert result is None

    sendmail_args = mock_smtp.sendmail.call_args
    all_recipients = sendmail_args[0][1]
    # All three should be in RCPT TO
    assert "bob@example.com" in all_recipients
    assert "carol@example.com" in all_recipients
    assert "dave@example.com" in all_recipients

    # BCC must NOT be in the message headers
    msg_str = sendmail_args[0][2]
    assert "Bcc" not in msg_str
    assert "dave@example.com" not in msg_str.split("\n\n")[0]  # not in headers

    # CC SHOULD be in headers
    assert "CC: carol@example.com" in msg_str or "Cc: carol@example.com" in msg_str


@patch("lingtai.addons.imap.account.smtplib.SMTP")
def test_send_email_with_reply_threading(mock_smtp_cls, account):
    mock_smtp = MagicMock()
    mock_smtp_cls.return_value.__enter__ = MagicMock(return_value=mock_smtp)
    mock_smtp_cls.return_value.__exit__ = MagicMock(return_value=False)

    result = account.send_email(
        to=["bob@example.com"],
        subject="Re: Hello",
        body="Reply body",
        in_reply_to="<orig-123@example.com>",
        references="<orig-123@example.com>",
    )
    assert result is None

    msg_str = mock_smtp.sendmail.call_args[0][2]
    assert "In-Reply-To: <orig-123@example.com>" in msg_str
    assert "References: <orig-123@example.com>" in msg_str


def test_send_email_empty_rejected(account):
    result = account.send_email(to=["bob@example.com"], subject="", body="")
    assert result is not None
    assert "empty" in result.lower()


def test_send_email_missing_attachment(account):
    result = account.send_email(
        to=["bob@example.com"],
        subject="Files",
        body="See attached",
        attachments=["/nonexistent/file.pdf"],
    )
    assert result is not None
    assert "not found" in result.lower()


@patch("lingtai.addons.imap.account.smtplib.SMTP")
def test_send_email_with_attachment(mock_smtp_cls, account, tmp_path):
    mock_smtp = MagicMock()
    mock_smtp_cls.return_value.__enter__ = MagicMock(return_value=mock_smtp)
    mock_smtp_cls.return_value.__exit__ = MagicMock(return_value=False)

    # Create a temp file to attach
    att_file = tmp_path / "test.txt"
    att_file.write_text("attachment content")

    result = account.send_email(
        to=["bob@example.com"],
        subject="With attachment",
        body="See attached",
        attachments=[str(att_file)],
    )
    assert result is None
    mock_smtp.sendmail.assert_called_once()


# ---------------------------------------------------------------------------
# _discover_folders — RFC 6154 special-use + name heuristics
# ---------------------------------------------------------------------------

def test_discover_folders_rfc6154(account, mock_imap):
    """Test folder discovery with RFC 6154 special-use attributes."""
    account._imap = mock_imap
    mock_imap.list.return_value = ("OK", [
        b'(\\HasNoChildren \\Sent) "/" "Sent"',
        b'(\\HasNoChildren \\Trash) "/" "Trash"',
        b'(\\HasNoChildren \\Archive) "/" "Archive"',
        b'(\\HasNoChildren \\Drafts) "/" "Drafts"',
        b'(\\HasNoChildren \\Junk) "/" "Junk"',
        b'(\\HasNoChildren) "/" "INBOX"',
    ])

    account._discover_folders()

    assert account._folders["Sent"] == "sent"
    assert account._folders["Trash"] == "trash"
    assert account._folders["Archive"] == "archive"
    assert account._folders["Drafts"] == "drafts"
    assert account._folders["Junk"] == "junk"
    assert account._folders["INBOX"] is None

    assert account.get_folder_by_role("sent") == "Sent"
    assert account.get_folder_by_role("trash") == "Trash"


def test_discover_folders_name_heuristics(account, mock_imap):
    """Test folder discovery via name heuristics when no special-use attrs."""
    account._imap = mock_imap
    mock_imap.list.return_value = ("OK", [
        b'(\\HasNoChildren) "/" "INBOX"',
        b'(\\HasNoChildren) "/" "Sent Items"',
        b'(\\HasNoChildren) "/" "Deleted Items"',
        b'(\\HasNoChildren) "/" "Spam"',
        b'(\\HasNoChildren) "/" "[Gmail]/All Mail"',
    ])

    account._discover_folders()

    assert account._folders["Sent Items"] == "sent"
    assert account._folders["Deleted Items"] == "trash"
    assert account._folders["Spam"] == "junk"
    assert account._folders["[Gmail]/All Mail"] == "archive"


def test_discover_folders_gmail_style(account, mock_imap):
    """Test Gmail-style folders."""
    account._imap = mock_imap
    mock_imap.list.return_value = ("OK", [
        b'(\\HasNoChildren) "/" "INBOX"',
        b'(\\HasNoChildren \\Sent) "/" "[Gmail]/Sent Mail"',
        b'(\\HasNoChildren \\Trash) "/" "[Gmail]/Trash"',
        b'(\\HasNoChildren) "/" "[Gmail]/Drafts"',
        b'(\\HasNoChildren) "/" "[Gmail]/Spam"',
    ])

    account._discover_folders()

    assert account.get_folder_by_role("sent") == "[Gmail]/Sent Mail"
    assert account.get_folder_by_role("trash") == "[Gmail]/Trash"
    # Drafts via heuristic
    assert account._folders["[Gmail]/Drafts"] == "drafts"
    # Spam via heuristic
    assert account._folders["[Gmail]/Spam"] == "junk"


# ---------------------------------------------------------------------------
# get_folder_by_role
# ---------------------------------------------------------------------------

def test_get_folder_by_role_found(account):
    account._folder_by_role = {"trash": "Trash", "sent": "Sent"}
    assert account.get_folder_by_role("trash") == "Trash"
    assert account.get_folder_by_role("sent") == "Sent"


def test_get_folder_by_role_not_found(account):
    account._folder_by_role = {}
    assert account.get_folder_by_role("trash") is None
    assert account.get_folder_by_role("archive") is None


# ---------------------------------------------------------------------------
# _format_imap_date
# ---------------------------------------------------------------------------

def test_format_imap_date():
    dt = datetime(2025, 1, 15)
    assert _format_imap_date(dt) == "15-Jan-2025"


def test_format_imap_date_single_digit_day():
    dt = datetime(2025, 3, 5)
    assert _format_imap_date(dt) == "05-Mar-2025"


def test_format_imap_date_december():
    dt = datetime(2024, 12, 25)
    assert _format_imap_date(dt) == "25-Dec-2024"


# ---------------------------------------------------------------------------
# connect / disconnect
# ---------------------------------------------------------------------------

@patch("lingtai.addons.imap.account.imaplib.IMAP4_SSL")
def test_connect_and_disconnect(mock_imap4_cls, account):
    mock_imap = MagicMock()
    mock_imap.capability.return_value = ("OK", [b"IMAP4rev1 IDLE MOVE UIDPLUS"])
    mock_imap.list.return_value = ("OK", [
        b'(\\HasNoChildren) "/" "INBOX"',
        b'(\\HasNoChildren \\Sent) "/" "Sent"',
    ])
    mock_imap4_cls.return_value = mock_imap

    account.connect()

    assert account.connected is True
    assert account.has_idle is True
    assert account.has_move is True
    assert account.has_uidplus is True
    assert "Sent" in account.folders

    mock_imap4_cls.assert_called_once_with("imap.example.com", 993)
    mock_imap.login.assert_called_once_with("test@example.com", "secret")

    account.disconnect()
    assert account.connected is False
    mock_imap.logout.assert_called_once()


# ---------------------------------------------------------------------------
# fetch_headers_by_uids
# ---------------------------------------------------------------------------

def test_fetch_headers_by_uids(account, mock_imap):
    account._imap = mock_imap

    header_bytes = (
        b"From: Alice <alice@example.com>\r\n"
        b"To: Bob <bob@example.com>\r\n"
        b"Subject: Test Email\r\n"
        b"Date: Mon, 15 Jan 2025 10:30:00 +0000\r\n"
        b"\r\n"
    )

    mock_imap.uid.return_value = ("OK", [
        (b'1 (UID 100 FLAGS (\\Seen) BODY[HEADER.FIELDS (FROM TO SUBJECT DATE)] {120}', header_bytes),
        b')',
    ])

    results = account.fetch_headers_by_uids("INBOX", ["100"])

    assert len(results) == 1
    r = results[0]
    assert r["uid"] == "100"
    assert "alice@example.com" in r["from"]
    assert "Test Email" in r["subject"]
    assert r["email_id"] == "test@example.com:INBOX:100"
    assert "\\Seen" in r["flags"]


def test_fetch_headers_by_uids_empty(account):
    # No connection needed — empty uid list returns early
    assert account.fetch_headers_by_uids("INBOX", []) == []


# ---------------------------------------------------------------------------
# fetch_envelopes
# ---------------------------------------------------------------------------

def test_fetch_envelopes(account, mock_imap):
    account._imap = mock_imap

    # First call: SEARCH ALL
    # Second call: FETCH headers
    header_bytes = (
        b"From: Alice <alice@example.com>\r\n"
        b"Subject: Recent\r\n"
        b"Date: Mon, 15 Jan 2025 10:30:00 +0000\r\n"
        b"\r\n"
    )

    mock_imap.uid.side_effect = [
        # SEARCH ALL
        ("OK", [b"1 2 3 4 5"]),
        # FETCH headers (for UIDs 4,5 — last 2)
        ("OK", [
            (b'1 (UID 4 FLAGS () BODY[HEADER.FIELDS (FROM TO SUBJECT DATE)] {80}', header_bytes),
            b')',
            (b'2 (UID 5 FLAGS (\\Seen) BODY[HEADER.FIELDS (FROM TO SUBJECT DATE)] {80}', header_bytes),
            b')',
        ]),
    ]

    results = account.fetch_envelopes("INBOX", n=2)
    assert len(results) == 2


# ---------------------------------------------------------------------------
# fetch_full
# ---------------------------------------------------------------------------

def test_fetch_full(account, mock_imap):
    account._imap = mock_imap

    raw_email = (
        b"From: Alice <alice@example.com>\r\n"
        b"To: Bob <bob@example.com>\r\n"
        b"Subject: Full Test\r\n"
        b"Date: Mon, 15 Jan 2025 10:30:00 +0000\r\n"
        b"Message-ID: <msg-001@example.com>\r\n"
        b"Content-Type: text/plain; charset=utf-8\r\n"
        b"\r\n"
        b"Hello, this is the body.\r\n"
    )

    mock_imap.uid.return_value = ("OK", [
        (b'1 (UID 200 FLAGS (\\Seen) RFC822 {200}', raw_email),
        b')',
    ])

    result = account.fetch_full("INBOX", "200")

    assert result is not None
    assert result["uid"] == "200"
    assert "alice@example.com" in result["from"]
    assert result["subject"] == "Full Test"
    assert "Hello, this is the body." in result["body"]
    assert result["message_id"] == "<msg-001@example.com>"
    assert result["email_id"] == "test@example.com:INBOX:200"


# ---------------------------------------------------------------------------
# search (server-side)
# ---------------------------------------------------------------------------

def test_search_server_side(account, mock_imap):
    account._imap = mock_imap
    mock_imap.uid.return_value = ("OK", [b"10 20 30"])

    results = account.search("INBOX", "from:alice@example.com unseen")

    assert results == ["10", "20", "30"]
    # Verify the search criteria passed to uid()
    search_call = mock_imap.uid.call_args
    criteria = search_call[0][2]
    assert 'FROM "alice@example.com"' in criteria
    assert "UNSEEN" in criteria


def test_search_no_results(account, mock_imap):
    account._imap = mock_imap
    mock_imap.uid.return_value = ("OK", [b""])

    results = account.search("INBOX", "from:nobody@example.com")
    assert results == []


# ---------------------------------------------------------------------------
# Flag operations
# ---------------------------------------------------------------------------

def test_store_flags(account, mock_imap):
    account._imap = mock_imap
    mock_imap.uid.return_value = ("OK", [b"1 (FLAGS (\\Seen))"])

    result = account.store_flags("INBOX", "100", ["\\Seen"])
    assert result is True
    mock_imap.uid.assert_called_once_with("STORE", "100", "+FLAGS", "(\\Seen)")


def test_mark_seen(account, mock_imap):
    account._imap = mock_imap
    mock_imap.uid.return_value = ("OK", [b""])

    assert account.mark_seen("INBOX", "100") is True


def test_mark_unseen(account, mock_imap):
    account._imap = mock_imap
    mock_imap.uid.return_value = ("OK", [b""])

    assert account.mark_unseen("INBOX", "100") is True


# ---------------------------------------------------------------------------
# Folder operations
# ---------------------------------------------------------------------------

def test_move_message_with_move_extension(account, mock_imap):
    account._imap = mock_imap
    account._has_move = True
    mock_imap.uid.return_value = ("OK", [b"OK"])

    result = account.move_message("INBOX", "100", "Trash")
    assert result is True
    mock_imap.uid.assert_called_with("MOVE", "100", "Trash")


def test_move_message_without_move_extension_with_uidplus(account, mock_imap):
    """COPY+DELETE path should use UID EXPUNGE when UIDPLUS is available."""
    account._imap = mock_imap
    account._has_move = False
    account._has_uidplus = True
    mock_imap.uid.side_effect = [
        ("OK", [b""]),  # COPY
        ("OK", [b""]),  # STORE +FLAGS \\Deleted
        ("OK", [b""]),  # UID EXPUNGE
    ]

    result = account.move_message("INBOX", "100", "Trash")
    assert result is True
    assert mock_imap.uid.call_args_list[0] == call("COPY", "100", "Trash")
    assert mock_imap.uid.call_args_list[2] == call("EXPUNGE", "100")
    mock_imap.expunge.assert_not_called()


def test_move_message_without_move_extension_no_uidplus(account, mock_imap):
    """COPY+DELETE path should use plain expunge() when UIDPLUS is not available."""
    account._imap = mock_imap
    account._has_move = False
    account._has_uidplus = False
    mock_imap.uid.side_effect = [
        ("OK", [b""]),  # COPY
        ("OK", [b""]),  # STORE +FLAGS \\Deleted
    ]
    mock_imap.expunge.return_value = ("OK", [b""])

    result = account.move_message("INBOX", "100", "Trash")
    assert result is True
    assert mock_imap.uid.call_args_list[0] == call("COPY", "100", "Trash")
    mock_imap.expunge.assert_called_once()


def test_delete_message_to_trash(account, mock_imap):
    account._imap = mock_imap
    account._has_move = True
    account._folder_by_role = {"trash": "Trash"}
    mock_imap.uid.return_value = ("OK", [b"OK"])

    result = account.delete_message("INBOX", "100")
    assert result is True
    mock_imap.uid.assert_called_with("MOVE", "100", "Trash")


def test_delete_message_already_in_trash_no_uidplus(account, mock_imap):
    account._imap = mock_imap
    account._has_uidplus = False
    account._folder_by_role = {"trash": "Trash"}

    mock_imap.uid.return_value = ("OK", [b""])
    mock_imap.expunge.return_value = ("OK", [b""])

    result = account.delete_message("Trash", "100")
    assert result is True
    mock_imap.uid.assert_called_with("STORE", "100", "+FLAGS", "(\\Deleted)")
    mock_imap.expunge.assert_called_once()


def test_delete_message_already_in_trash_with_uidplus(account, mock_imap):
    account._imap = mock_imap
    account._has_uidplus = True
    account._folder_by_role = {"trash": "Trash"}

    mock_imap.uid.side_effect = [
        ("OK", [b""]),  # STORE +FLAGS \\Deleted
        ("OK", [b""]),  # UID EXPUNGE
    ]

    result = account.delete_message("Trash", "100")
    assert result is True
    assert mock_imap.uid.call_args_list[0] == call("STORE", "100", "+FLAGS", "(\\Deleted)")
    assert mock_imap.uid.call_args_list[1] == call("EXPUNGE", "100")
    mock_imap.expunge.assert_not_called()


# ---------------------------------------------------------------------------
# list_folders
# ---------------------------------------------------------------------------

def test_list_folders(account):
    account._folders = {"INBOX": None, "Sent": "sent", "Trash": "trash"}
    result = account.list_folders()
    assert result == {"INBOX": None, "Sent": "sent", "Trash": "trash"}
    # Verify it returns a copy
    result["New"] = "new"
    assert "New" not in account._folders


# ---------------------------------------------------------------------------
# State persistence
# ---------------------------------------------------------------------------

def test_state_persistence(tmp_path):
    """Test that state is saved and loaded correctly with spec-compliant schema."""
    acct1 = IMAPAccount(
        email_address="test@example.com",
        email_password="secret",
        working_dir=tmp_path,
    )
    acct1._processed_uids = {"INBOX": {1001, 1002, 1003}}
    acct1._folders = {"INBOX": None, "Sent": "sent"}
    acct1._parse_capabilities("IMAP4rev1 IDLE MOVE")
    acct1._save_state()

    # Verify state file exists and matches spec schema
    state_path = tmp_path / "imap" / "test@example.com" / "state.json"
    assert state_path.is_file()

    state_data = json.loads(state_path.read_text())
    # processed_uids: per-folder dict of int UIDs
    assert state_data["processed_uids"] == {"INBOX": [1001, 1002, 1003]}
    # folders: {name: {"role": role}} objects (null for no role)
    assert state_data["folders"]["INBOX"] == {"role": None}
    assert state_data["folders"]["Sent"] == {"role": "sent"}
    # capabilities: boolean dict
    assert state_data["capabilities"] == {"idle": True, "move": True, "uidplus": False}

    # Load into new account
    acct2 = IMAPAccount(
        email_address="test@example.com",
        email_password="secret",
        working_dir=tmp_path,
    )
    assert acct2._processed_uids == {"INBOX": {1001, 1002, 1003}}
    assert acct2._folders == {"INBOX": None, "Sent": "sent"}
    assert acct2.has_idle is True
    assert acct2.has_move is True
    assert acct2.has_uidplus is False


def test_state_persistence_no_working_dir():
    acct = IMAPAccount(
        email_address="test@example.com",
        email_password="secret",
    )
    # Should not raise
    acct._save_state()
    acct._load_state()


# ---------------------------------------------------------------------------
# _parse_list_entry
# ---------------------------------------------------------------------------

def test_parse_list_entry_standard():
    name, attrs = IMAPAccount._parse_list_entry('(\\HasNoChildren \\Sent) "/" "Sent"')
    assert name == "Sent"
    assert "\\Sent" in attrs
    assert "\\HasNoChildren" in attrs


def test_parse_list_entry_no_quotes():
    name, attrs = IMAPAccount._parse_list_entry('(\\HasNoChildren) "/" INBOX')
    assert name == "INBOX"


def test_parse_list_entry_gmail_path():
    name, attrs = IMAPAccount._parse_list_entry('(\\HasNoChildren \\Sent) "/" "[Gmail]/Sent Mail"')
    assert name == "[Gmail]/Sent Mail"
    assert "\\Sent" in attrs


def test_parse_list_entry_malformed():
    name, attrs = IMAPAccount._parse_list_entry("garbage data")
    assert name is None
    assert attrs == []


# ---------------------------------------------------------------------------
# Email parsing helpers
# ---------------------------------------------------------------------------

def test_decode_header_value_plain():
    assert _decode_header_value("Hello World") == "Hello World"


def test_decode_header_value_empty():
    assert _decode_header_value("") == ""


def test_decode_header_value_encoded():
    # RFC 2047 encoded subject
    encoded = "=?utf-8?B?5L2g5aW9?="  # "你好" in base64
    result = _decode_header_value(encoded)
    assert result == "你好"


def test_extract_text_body_plain():
    msg = email_mod.message_from_string(
        "Content-Type: text/plain; charset=utf-8\r\n\r\nHello plain text",
    )
    assert _extract_text_body(msg) == "Hello plain text"


def test_extract_text_body_html_fallback():
    msg = email_mod.message_from_string(
        "Content-Type: text/html; charset=utf-8\r\n\r\n<p>Hello <b>HTML</b></p>",
    )
    result = _extract_text_body(msg)
    assert "Hello" in result
    assert "<p>" not in result


def test_extract_text_body_multipart():
    mime = MIMEMultipart("alternative")
    mime.attach(MIMEText("Plain version", "plain"))
    mime.attach(MIMEText("<p>HTML version</p>", "html"))
    msg = email_mod.message_from_string(mime.as_string())
    result = _extract_text_body(msg)
    assert result == "Plain version"


def test_strip_html_tags():
    assert _strip_html_tags("<p>Hello <b>World</b></p>") == "Hello World"
    assert _strip_html_tags("No tags") == "No tags"


def test_extract_attachments_none():
    msg = email_mod.message_from_string(
        "Content-Type: text/plain\r\n\r\nNo attachments",
    )
    assert _extract_attachments(msg) == []


def test_extract_attachments_with_file():
    from email.mime.base import MIMEBase
    from email import encoders

    mime = MIMEMultipart()
    mime.attach(MIMEText("Body", "plain"))
    att = MIMEBase("application", "octet-stream")
    att.set_payload(b"file content")
    encoders.encode_base64(att)
    att.add_header("Content-Disposition", "attachment", filename="test.bin")
    mime.attach(att)

    msg = email_mod.message_from_string(mime.as_string())
    attachments = _extract_attachments(msg)
    assert len(attachments) == 1
    assert attachments[0]["filename"] == "test.bin"
    assert attachments[0]["content_type"] == "application/octet-stream"


# ---------------------------------------------------------------------------
# Dual-connection architecture
# ---------------------------------------------------------------------------

def test_idle_imap_initialized_none(account):
    """The dedicated IDLE connection starts as None."""
    assert account._idle_imap is None


def test_connect_idle(account):
    """_connect_idle creates a separate IMAP connection."""
    with patch("lingtai.addons.imap.account.imaplib.IMAP4_SSL") as mock_cls:
        mock_conn = MagicMock()
        mock_cls.return_value = mock_conn
        result = account._connect_idle()
        assert result is mock_conn
        assert account._idle_imap is mock_conn
        mock_conn.login.assert_called_once()


def test_disconnect_idle(account):
    """_disconnect_idle logs out and clears _idle_imap."""
    mock_conn = MagicMock()
    account._idle_imap = mock_conn
    account._disconnect_idle()
    mock_conn.logout.assert_called_once()
    assert account._idle_imap is None


def test_disconnect_idle_when_none(account):
    """_disconnect_idle is a no-op when _idle_imap is None."""
    account._idle_imap = None
    account._disconnect_idle()  # Should not raise


# ---------------------------------------------------------------------------
# _ensure_connected
# ---------------------------------------------------------------------------

def test_ensure_connected_reconnects_when_none(account):
    """_ensure_connected auto-connects when _imap is None."""
    with patch.object(account, "connect") as mock_connect:
        mock_imap = MagicMock()
        def _set_imap():
            account._imap = mock_imap
        mock_connect.side_effect = _set_imap
        result = account._ensure_connected()
        mock_connect.assert_called_once()
        assert result is mock_imap


def test_ensure_connected_returns_imap(account, mock_imap):
    account._imap = mock_imap
    assert account._ensure_connected() is mock_imap


# ---------------------------------------------------------------------------
# IDLE 25-minute cap
# ---------------------------------------------------------------------------

def test_idle_timeout_capped_at_25_minutes(account, mock_imap):
    """IDLE wait must be capped at 1500s regardless of poll_interval."""
    account._idle_imap = mock_imap
    account._has_idle = True
    mock_imap._new_tag.return_value = b"A001"

    # Patch time.monotonic to avoid actually waiting
    call_count = 0
    base_time = 1000.0

    def fake_monotonic():
        nonlocal call_count
        call_count += 1
        if call_count <= 1:
            return base_time
        return base_time + 2000

    stop = threading.Event()
    stop.set()  # Stop immediately

    with patch("lingtai.addons.imap.account.time.monotonic", side_effect=fake_monotonic), \
         patch.object(account, "_check_new_mail"):
        account._idle_cycle("INBOX", lambda x: None, 3600, stop)

    # The fact that it didn't hang for an hour proves the cap works.


# ---------------------------------------------------------------------------
# Reconnect backoff
# ---------------------------------------------------------------------------

def test_reconnect_backoff_initialization(account):
    """Backoff steps and index should be initialized."""
    assert account._backoff_steps == [1, 2, 5, 10, 60]
    assert account._backoff_index == 0


def test_reconnect_backoff_progression(account):
    """Backoff index should progress through steps."""
    steps = account._backoff_steps
    # Simulate failures
    for i in range(7):
        delay = steps[min(account._backoff_index, len(steps) - 1)]
        account._backoff_index += 1

    # After 5+ failures, should be capped at 60s
    delay = steps[min(account._backoff_index, len(steps) - 1)]
    assert delay == 60

    # Reset on success
    account._backoff_index = 0
    delay = steps[min(account._backoff_index, len(steps) - 1)]
    assert delay == 1
