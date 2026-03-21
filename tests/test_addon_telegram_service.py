from __future__ import annotations

from unittest.mock import patch, MagicMock

from lingtai.addons.telegram.service import TelegramService


def test_construction_creates_accounts(tmp_path):
    config = [
        {"alias": "support", "bot_token": "TOKEN1"},
        {"alias": "sales", "bot_token": "TOKEN2"},
    ]
    svc = TelegramService(
        working_dir=tmp_path, accounts_config=config, on_message=lambda a, u: None,
    )
    assert svc.list_accounts() == ["support", "sales"]


def test_get_account(tmp_path):
    config = [{"alias": "support", "bot_token": "TOKEN1"}]
    svc = TelegramService(
        working_dir=tmp_path, accounts_config=config, on_message=lambda a, u: None,
    )
    acct = svc.get_account("support")
    assert acct.alias == "support"


def test_get_account_unknown_raises(tmp_path):
    config = [{"alias": "support", "bot_token": "TOKEN1"}]
    svc = TelegramService(
        working_dir=tmp_path, accounts_config=config, on_message=lambda a, u: None,
    )
    try:
        svc.get_account("unknown")
        assert False, "Should have raised"
    except KeyError:
        pass


def test_default_account(tmp_path):
    """First account should be the default."""
    config = [
        {"alias": "first", "bot_token": "TOKEN1"},
        {"alias": "second", "bot_token": "TOKEN2"},
    ]
    svc = TelegramService(
        working_dir=tmp_path, accounts_config=config, on_message=lambda a, u: None,
    )
    assert svc.default_account.alias == "first"


def test_start_stop_delegates(tmp_path):
    config = [{"alias": "bot1", "bot_token": "TOKEN1"}]
    svc = TelegramService(
        working_dir=tmp_path, accounts_config=config, on_message=lambda a, u: None,
    )
    with patch.object(svc._accounts["bot1"], "start") as mock_start, \
         patch.object(svc._accounts["bot1"], "stop") as mock_stop:
        svc.start()
        mock_start.assert_called_once()
        svc.stop()
        mock_stop.assert_called_once()


def test_on_message_callback(tmp_path):
    """Incoming updates should be forwarded via on_message callback."""
    received = []
    config = [{"alias": "bot1", "bot_token": "TOKEN1"}]
    svc = TelegramService(
        working_dir=tmp_path, accounts_config=config,
        on_message=lambda alias, update: received.append((alias, update)),
    )
    # Simulate an incoming update by calling account's on_message
    update = {"update_id": 1, "message": {"text": "hi"}}
    svc._accounts["bot1"]._on_message("bot1", update)
    assert len(received) == 1
    assert received[0] == ("bot1", update)
