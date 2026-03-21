from __future__ import annotations

from unittest.mock import patch, MagicMock

from lingtai.addons.telegram.account import TelegramAccount


def test_construction():
    received = []
    acct = TelegramAccount(
        alias="support",
        bot_token="123456:ABC-DEF",
        allowed_users=None,
        poll_interval=1.0,
        on_message=lambda alias, update: received.append((alias, update)),
    )
    assert acct.alias == "support"
    assert acct._bot_token == "123456:ABC-DEF"
    assert acct._poll_thread is None  # not started yet


def test_send_message():
    """send_message should POST to sendMessage endpoint."""
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
    )
    with patch.object(acct, "_request", return_value={"message_id": 100}) as mock_req:
        result = acct.send_message(chat_id=12345, text="Hello!")
        mock_req.assert_called_once_with("sendMessage", json={
            "chat_id": 12345, "text": "Hello!",
        })
        assert result["message_id"] == 100


def test_send_message_with_reply_markup():
    """reply_markup should be included in the API payload."""
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
    )
    markup = {"inline_keyboard": [[{"text": "OK", "callback_data": "ok"}]]}
    with patch.object(acct, "_request", return_value={"message_id": 101}) as mock_req:
        acct.send_message(chat_id=12345, text="Choose:", reply_markup=markup)
        call_payload = mock_req.call_args[1]["json"]
        assert call_payload["reply_markup"] == markup


def test_send_photo(tmp_path):
    """send_photo should upload file via multipart."""
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
    )
    photo = tmp_path / "photo.png"
    photo.write_bytes(b"\x89PNG\r\n\x1a\n")
    with patch.object(acct, "_request", return_value={"message_id": 102}) as mock_req:
        acct.send_photo(chat_id=12345, photo_path=str(photo), caption="A photo")
        mock_req.assert_called_once()
        assert mock_req.call_args[1]["data"]["caption"] == "A photo"


def test_send_document(tmp_path):
    """send_document should upload file via multipart."""
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
    )
    doc = tmp_path / "report.pdf"
    doc.write_bytes(b"%PDF-1.4")
    with patch.object(acct, "_request", return_value={"message_id": 103}) as mock_req:
        acct.send_document(chat_id=12345, doc_path=str(doc), caption="Report")
        mock_req.assert_called_once()


def test_process_update_filters_by_allowed_users():
    """Updates from non-allowed users should be silently dropped."""
    received = []
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=[111],
        on_message=lambda alias, update: received.append(update),
    )
    acct._last_update_id = 0

    # Allowed user
    acct._process_update({
        "update_id": 1,
        "message": {"from": {"id": 111}, "chat": {"id": 111}, "text": "hi"},
    })
    assert len(received) == 1

    # Non-allowed user — should be dropped
    acct._process_update({
        "update_id": 2,
        "message": {"from": {"id": 999}, "chat": {"id": 999}, "text": "hi"},
    })
    assert len(received) == 1  # still 1


def test_process_update_accepts_all_when_no_filter():
    """With allowed_users=None, all messages should be accepted."""
    received = []
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda alias, update: received.append(update),
    )
    acct._last_update_id = 0
    acct._process_update({
        "update_id": 1,
        "message": {"from": {"id": 999}, "chat": {"id": 999}, "text": "hi"},
    })
    assert len(received) == 1


def test_process_update_tracks_offset():
    """_last_update_id should advance with each processed update."""
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
    )
    acct._last_update_id = 0
    acct._process_update({"update_id": 5, "message": {"from": {"id": 1}, "chat": {"id": 1}, "text": "hi"}})
    assert acct._last_update_id == 5
    acct._process_update({"update_id": 3, "message": {"from": {"id": 1}, "chat": {"id": 1}, "text": "hi"}})
    assert acct._last_update_id == 5  # should not go backwards


def test_callback_query_auto_answers():
    """Callback queries should trigger answerCallbackQuery."""
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
    )
    acct._last_update_id = 0
    with patch.object(acct, "_request") as mock_req:
        acct._process_update({
            "update_id": 1,
            "callback_query": {
                "id": "cq-123",
                "from": {"id": 111},
                "data": "yes",
            },
        })
        mock_req.assert_called_once_with(
            "answerCallbackQuery", json={"callback_query_id": "cq-123"},
        )


def test_state_persistence(tmp_path):
    """State should persist and reload last_update_id."""
    state_dir = tmp_path / "bot1"
    state_dir.mkdir()
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
        state_dir=state_dir,
    )
    acct._last_update_id = 42
    acct._bot_info = {"id": 123, "username": "test_bot"}
    acct._save_state()

    # New instance should load persisted state
    acct2 = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
        state_dir=state_dir,
    )
    assert acct2._last_update_id == 42
    assert acct2._bot_info["username"] == "test_bot"


def test_edit_message_text():
    """edit_message with is_caption=False should call editMessageText."""
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
    )
    with patch.object(acct, "_request", return_value={}) as mock_req:
        acct.edit_message(chat_id=123, message_id=456, text="updated")
        mock_req.assert_called_once_with("editMessageText", json={
            "chat_id": 123, "message_id": 456, "text": "updated",
        })


def test_edit_message_caption():
    """edit_message with is_caption=True should call editMessageCaption."""
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
    )
    with patch.object(acct, "_request", return_value={}) as mock_req:
        acct.edit_message(chat_id=123, message_id=456, text="new caption", is_caption=True)
        mock_req.assert_called_once_with("editMessageCaption", json={
            "chat_id": 123, "message_id": 456, "caption": "new caption",
        })


def test_delete_message():
    acct = TelegramAccount(
        alias="bot1", bot_token="TOKEN", allowed_users=None,
        on_message=lambda a, u: None,
    )
    with patch.object(acct, "_request", return_value=True) as mock_req:
        acct.delete_message(chat_id=123, message_id=456)
        mock_req.assert_called_once_with("deleteMessage", json={
            "chat_id": 123, "message_id": 456,
        })
