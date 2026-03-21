"""Tests for IMAPMailService — multi-account coordinator."""
from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from lingtai.addons.imap.service import IMAPMailService

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

ACCOUNT_1 = {
    "email_address": "alice@example.com",
    "email_password": "pass1",
}

ACCOUNT_2 = {
    "email_address": "bob@example.com",
    "email_password": "pass2",
}


# ---------------------------------------------------------------------------
# Construction tests
# ---------------------------------------------------------------------------

def test_single_account_construction():
    svc = IMAPMailService(accounts=[ACCOUNT_1])
    assert svc.default_account.address == "alice@example.com"
    assert len(svc.accounts) == 1


def test_multi_account_construction():
    svc = IMAPMailService(accounts=[ACCOUNT_1, ACCOUNT_2])
    assert len(svc.accounts) == 2
    assert svc.default_account.address == "alice@example.com"


# ---------------------------------------------------------------------------
# Account lookup tests
# ---------------------------------------------------------------------------

def test_get_account_by_address():
    svc = IMAPMailService(accounts=[ACCOUNT_1, ACCOUNT_2])
    acct = svc.get_account("bob@example.com")
    assert acct is not None
    assert acct.address == "bob@example.com"


def test_get_account_default():
    svc = IMAPMailService(accounts=[ACCOUNT_1, ACCOUNT_2])
    acct = svc.get_account(None)
    assert acct is svc.default_account
    assert acct.address == "alice@example.com"


def test_get_account_unknown():
    svc = IMAPMailService(accounts=[ACCOUNT_1])
    acct = svc.get_account("nobody@example.com")
    assert acct is None


# ---------------------------------------------------------------------------
# MailService interface tests
# ---------------------------------------------------------------------------

def test_mail_service_send_delegates():
    svc = IMAPMailService(accounts=[ACCOUNT_1])
    with patch.object(svc.default_account, "send_email", return_value=None) as mock_send:
        result = svc.send(
            "recipient@example.com",
            {"subject": "Hello", "message": "World"},
        )
    mock_send.assert_called_once_with(
        to=["recipient@example.com"],
        subject="Hello",
        body="World",
    )
    assert result is None


def test_mail_service_address():
    svc = IMAPMailService(accounts=[ACCOUNT_1])
    assert svc.address == "alice@example.com"


def test_listen_starts_all_accounts():
    svc = IMAPMailService(accounts=[ACCOUNT_1, ACCOUNT_2])
    on_message = MagicMock()
    with (
        patch.object(svc.accounts[0], "start_listening") as mock_listen_0,
        patch.object(svc.accounts[1], "start_listening") as mock_listen_1,
    ):
        svc.listen(on_message)
    # listen() wraps the callback to dispatch each header individually
    mock_listen_0.assert_called_once()
    mock_listen_1.assert_called_once()
    # The wrapper should dispatch each dict from the list
    wrapper = mock_listen_0.call_args[0][0]
    wrapper([{"from": "a"}, {"from": "b"}])
    assert on_message.call_count == 2
    on_message.assert_any_call({"from": "a"})
    on_message.assert_any_call({"from": "b"})


def test_stop_stops_all_accounts():
    svc = IMAPMailService(accounts=[ACCOUNT_1, ACCOUNT_2])
    with (
        patch.object(svc.accounts[0], "stop_listening") as mock_stop_0,
        patch.object(svc.accounts[0], "disconnect") as mock_disc_0,
        patch.object(svc.accounts[1], "stop_listening") as mock_stop_1,
        patch.object(svc.accounts[1], "disconnect") as mock_disc_1,
    ):
        svc.stop()
    mock_stop_0.assert_called_once()
    mock_disc_0.assert_called_once()
    mock_stop_1.assert_called_once()
    mock_disc_1.assert_called_once()


# ---------------------------------------------------------------------------
# Config passthrough tests
# ---------------------------------------------------------------------------

def test_allowed_senders_and_poll_interval_passed_through():
    cfg = {
        "email_address": "alice@example.com",
        "email_password": "pass1",
        "allowed_senders": ["boss@example.com", "team@example.com"],
        "poll_interval": 60,
    }
    svc = IMAPMailService(accounts=[cfg])
    acct = svc.default_account
    assert acct._allowed_senders == ["boss@example.com", "team@example.com"]
    assert acct._poll_interval == 60


def test_allowed_senders_defaults_to_none():
    svc = IMAPMailService(accounts=[ACCOUNT_1])
    acct = svc.default_account
    assert acct._allowed_senders is None
    assert acct._poll_interval == 30
