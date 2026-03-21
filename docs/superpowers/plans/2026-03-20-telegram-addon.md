# Telegram Addon Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Telegram Bot API addon that lets 灵台 agents interact with Telegram users for customer service — multi-account, text + images + documents, inline keyboards, long polling.

**Architecture:** Three-class pattern mirroring the IMAP addon: `TelegramAccount` (per-bot connection + polling), `TelegramService` (multi-account orchestrator), `TelegramManager` (tool dispatch + filesystem). Registered as an addon via `setup()` in `src/lingtai/addons/telegram/__init__.py`.

**Tech Stack:** Python 3.11+, `httpx` for Bot API HTTP calls (optional dep), `threading` for poll loops, `json`/`pathlib` for filesystem persistence.

**Spec:** `docs/superpowers/specs/2026-03-20-telegram-addon-design.md`

**Reference implementation:** `src/lingtai/addons/imap/` (service.py, manager.py, `__init__.py`)

---

## File Structure

| File | Responsibility |
|------|---------------|
| `src/lingtai/addons/telegram/__init__.py` | `setup()` function, config normalization, tool registration |
| `src/lingtai/addons/telegram/account.py` | `TelegramAccount` — one bot token, HTTP calls, polling thread |
| `src/lingtai/addons/telegram/service.py` | `TelegramService` — multi-account registry, routing |
| `src/lingtai/addons/telegram/manager.py` | `TelegramManager` — tool handler, filesystem ops, agent notification |
| `src/lingtai/addons/__init__.py` | Add `"telegram"` to `_BUILTIN` registry |
| `pyproject.toml` | Add `telegram = ["httpx>=0.27"]` optional dep |
| `tests/test_addon_telegram_account.py` | TelegramAccount unit tests |
| `tests/test_addon_telegram_service.py` | TelegramService unit tests |
| `tests/test_addon_telegram_manager.py` | TelegramManager unit tests |

---

### Task 1: TelegramAccount — Bot API Client

**Files:**
- Create: `src/lingtai/addons/telegram/account.py`
- Test: `tests/test_addon_telegram_account.py`

This is the lowest-level class. It owns one bot token and handles all HTTP calls to the Telegram Bot API plus the long-polling thread.

- [ ] **Step 1: Write failing test for construction**

```python
# tests/test_addon_telegram_account.py
from __future__ import annotations

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_addon_telegram_account.py::test_construction -v`
Expected: FAIL — module not found

- [ ] **Step 3: Write TelegramAccount class with constructor**

```python
# src/lingtai/addons/telegram/account.py
"""TelegramAccount — single bot token, HTTP calls, polling thread.

One daemon thread per account runs the getUpdates long-poll loop.
Constructor stores config only — no threads, no API calls.
start() calls getMe and spawns the polling thread.
stop() signals the thread to stop and joins it.
"""
from __future__ import annotations

import json
import logging
import threading
import time
from pathlib import Path
from typing import Any, Callable

logger = logging.getLogger(__name__)

# httpx is lazy-imported to keep the module importable without the optional dep.
# Actual import happens in _ensure_client() on first API call.
httpx: Any = None

_API_BASE = "https://api.telegram.org/bot{token}/{method}"
_FILE_BASE = "https://api.telegram.org/file/bot{token}/{file_path}"


class TelegramAccount:
    """Manages a single Telegram bot token — polling + sending."""

    def __init__(
        self,
        alias: str,
        bot_token: str,
        allowed_users: list[int] | None,
        poll_interval: float = 1.0,
        on_message: Callable[[str, dict], None] | None = None,
        state_dir: Path | None = None,
    ) -> None:
        self.alias = alias
        self._bot_token = bot_token
        self._allowed_users = set(allowed_users) if allowed_users else None
        self._poll_interval = poll_interval
        self._on_message = on_message
        self._state_dir = state_dir

        self._poll_thread: threading.Thread | None = None
        self._stop_event = threading.Event()
        self._last_update_id: int = 0
        self._bot_info: dict | None = None
        self._client: httpx.Client | None = None

        self._load_state()

    # -- API helpers ---------------------------------------------------------

    def _api_url(self, method: str) -> str:
        return _API_BASE.format(token=self._bot_token, method=method)

    def _file_url(self, file_path: str) -> str:
        return _FILE_BASE.format(token=self._bot_token, file_path=file_path)

    def _ensure_client(self) -> None:
        """Lazy-import httpx and create client on first use."""
        global httpx
        if httpx is None or isinstance(httpx, type(None)):
            import httpx as _httpx
            httpx = _httpx
        if self._client is None:
            self._client = httpx.Client(timeout=httpx.Timeout(60.0, connect=10.0))

    def _request(self, method: str, **kwargs: Any) -> dict:
        """Make a Bot API request. Returns the 'result' field or raises."""
        self._ensure_client()
        resp = self._client.post(self._api_url(method), **kwargs)
        if resp.status_code == 429:
            retry_after = resp.json().get("parameters", {}).get("retry_after", 5)
            logger.warning("Rate limited, sleeping %ds", retry_after)
            time.sleep(retry_after)
            resp = self._client.post(self._api_url(method), **kwargs)
        data = resp.json()
        if not data.get("ok"):
            raise RuntimeError(f"Telegram API error: {data.get('description', data)}")
        return data.get("result", {})

    # -- Lifecycle -----------------------------------------------------------

    def start(self) -> None:
        """Call getMe, cache bot info, start polling thread."""
        if self._poll_thread is not None:
            return
        self._ensure_client()
        self._bot_info = self._request("getMe")
        self._save_state()
        self._stop_event.clear()
        self._poll_thread = threading.Thread(
            target=self._poll_loop, daemon=True,
            name=f"telegram-poll-{self.alias}",
        )
        self._poll_thread.start()
        logger.info("Telegram account '%s' started (@%s)",
                     self.alias, self._bot_info.get("username", "?"))

    def stop(self) -> None:
        """Signal polling thread to stop and join it."""
        self._stop_event.set()
        if self._poll_thread is not None:
            self._poll_thread.join(timeout=5.0)
            self._poll_thread = None
        if self._client is not None:
            self._client.close()
            self._client = None

    # -- Polling -------------------------------------------------------------

    def _poll_loop(self) -> None:
        """Main loop — getUpdates with long poll, dispatch to on_message."""
        while not self._stop_event.is_set():
            try:
                updates = self._request(
                    "getUpdates",
                    json={
                        "offset": self._last_update_id + 1,
                        "timeout": 30,
                    },
                )
                for update in updates:
                    self._process_update(update)
            except Exception as e:
                logger.warning("Telegram poll error (%s): %s", self.alias, e)
                # Backoff before retry
                if self._stop_event.wait(timeout=5.0):
                    return
                continue
            # Brief pause between poll cycles
            if self._stop_event.wait(timeout=self._poll_interval):
                return

    def _process_update(self, update: dict) -> None:
        """Process a single update — filter, dispatch, track offset."""
        update_id = update.get("update_id", 0)
        if update_id > self._last_update_id:
            self._last_update_id = update_id
            self._save_state()

        # Determine the user who triggered this update
        user_id = None
        if "message" in update:
            user_id = update["message"].get("from", {}).get("id")
        elif "callback_query" in update:
            user_id = update["callback_query"].get("from", {}).get("id")
            # Auto-answer callback query to dismiss spinner
            cq_id = update["callback_query"].get("id")
            if cq_id:
                try:
                    self._request("answerCallbackQuery", json={"callback_query_id": cq_id})
                except Exception:
                    pass
        elif "edited_message" in update:
            user_id = update["edited_message"].get("from", {}).get("id")

        # Filter by allowed users
        if self._allowed_users is not None and user_id not in self._allowed_users:
            return

        if self._on_message:
            self._on_message(self.alias, update)

    # -- Sending -------------------------------------------------------------

    def send_message(
        self,
        chat_id: int,
        text: str,
        reply_markup: dict | None = None,
        reply_to_message_id: int | None = None,
    ) -> dict:
        """Send a text message. Returns the sent Message object."""
        payload: dict[str, Any] = {"chat_id": chat_id, "text": text}
        if reply_markup:
            payload["reply_markup"] = reply_markup
        if reply_to_message_id:
            payload["reply_to_message_id"] = reply_to_message_id
        return self._request("sendMessage", json=payload)

    def send_photo(
        self, chat_id: int, photo_path: str, caption: str | None = None,
        reply_to_message_id: int | None = None,
    ) -> dict:
        """Send a photo via multipart upload."""
        with open(photo_path, "rb") as f:
            files = {"photo": (Path(photo_path).name, f, "image/jpeg")}
            data: dict[str, Any] = {"chat_id": str(chat_id)}
            if caption:
                data["caption"] = caption
            if reply_to_message_id:
                data["reply_to_message_id"] = str(reply_to_message_id)
            return self._request("sendPhoto", files=files, data=data)

    def send_document(
        self, chat_id: int, doc_path: str, caption: str | None = None,
        reply_to_message_id: int | None = None,
    ) -> dict:
        """Send a document via multipart upload."""
        with open(doc_path, "rb") as f:
            files = {"document": (Path(doc_path).name, f, "application/octet-stream")}
            data: dict[str, Any] = {"chat_id": str(chat_id)}
            if caption:
                data["caption"] = caption
            if reply_to_message_id:
                data["reply_to_message_id"] = str(reply_to_message_id)
            return self._request("sendDocument", files=files, data=data)

    def edit_message(
        self, chat_id: int, message_id: int, text: str,
        reply_markup: dict | None = None, is_caption: bool = False,
    ) -> dict:
        """Edit a sent message's text or caption."""
        if is_caption:
            payload: dict[str, Any] = {
                "chat_id": chat_id, "message_id": message_id, "caption": text,
            }
            if reply_markup:
                payload["reply_markup"] = reply_markup
            return self._request("editMessageCaption", json=payload)
        else:
            payload = {
                "chat_id": chat_id, "message_id": message_id, "text": text,
            }
            if reply_markup:
                payload["reply_markup"] = reply_markup
            return self._request("editMessageText", json=payload)

    def delete_message(self, chat_id: int, message_id: int) -> dict:
        """Delete a message."""
        return self._request("deleteMessage", json={
            "chat_id": chat_id, "message_id": message_id,
        })

    def get_file(self, file_id: str) -> tuple[str, bytes]:
        """Download a file by file_id. Returns (filename, data)."""
        file_info = self._request("getFile", json={"file_id": file_id})
        file_path = file_info["file_path"]
        filename = Path(file_path).name
        url = self._file_url(file_path)
        if self._client is None:
            self._client = httpx.Client(timeout=httpx.Timeout(60.0, connect=10.0))
        resp = self._client.get(url)
        resp.raise_for_status()
        return filename, resp.content

    # -- State persistence ---------------------------------------------------

    def _state_path(self) -> Path | None:
        if self._state_dir is None:
            return None
        return self._state_dir / "state.json"

    def _load_state(self) -> None:
        path = self._state_path()
        if path is None or not path.is_file():
            return
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
            self._last_update_id = data.get("last_update_id", 0)
            self._bot_info = data.get("bot_info")
        except (json.JSONDecodeError, OSError) as e:
            logger.warning("Failed to load Telegram state: %s", e)

    def _save_state(self) -> None:
        path = self._state_path()
        if path is None:
            return
        path.parent.mkdir(parents=True, exist_ok=True)
        data = {
            "last_update_id": self._last_update_id,
            "bot_info": self._bot_info,
        }
        path.write_text(json.dumps(data, indent=2), encoding="utf-8")
```

Also create the package `__init__` so imports work:

```python
# src/lingtai/addons/telegram/__init__.py
"""Telegram addon — placeholder, will be filled in Task 4."""
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_addon_telegram_account.py::test_construction -v`
Expected: PASS

- [ ] **Step 5: Write remaining account tests**

```python
# Append to tests/test_addon_telegram_account.py

from unittest.mock import patch, MagicMock


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
```

- [ ] **Step 6: Run all account tests**

Run: `python -m pytest tests/test_addon_telegram_account.py -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/addons/telegram/__init__.py src/lingtai/addons/telegram/account.py tests/test_addon_telegram_account.py
git commit -m "feat(telegram): add TelegramAccount — Bot API client with polling"
```

---

### Task 2: TelegramService — Multi-Account Orchestrator

**Files:**
- Create: `src/lingtai/addons/telegram/service.py`
- Test: `tests/test_addon_telegram_service.py`

Thin routing layer that creates and manages multiple TelegramAccount instances.

- [ ] **Step 1: Write failing tests**

```python
# tests/test_addon_telegram_service.py
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_addon_telegram_service.py -v`
Expected: FAIL — module not found

- [ ] **Step 3: Write TelegramService**

```python
# src/lingtai/addons/telegram/service.py
"""TelegramService — multi-account orchestrator.

Creates one TelegramAccount per config entry.
Routes outbound sends to the correct account by alias.
Delegates lifecycle (start/stop) to all accounts.
"""
from __future__ import annotations

import logging
from pathlib import Path
from typing import Callable

from .account import TelegramAccount

logger = logging.getLogger(__name__)


class TelegramService:
    """Multi-account Telegram bot service."""

    def __init__(
        self,
        working_dir: Path,
        accounts_config: list[dict],
        on_message: Callable[[str, dict], None],
    ) -> None:
        self._working_dir = working_dir
        self._on_message = on_message
        self._account_order: list[str] = []
        self._accounts: dict[str, TelegramAccount] = {}

        for cfg in accounts_config:
            alias = cfg["alias"]
            state_dir = working_dir / "telegram" / alias
            acct = TelegramAccount(
                alias=alias,
                bot_token=cfg["bot_token"],
                allowed_users=cfg.get("allowed_users"),
                poll_interval=cfg.get("poll_interval", 1.0),
                on_message=on_message,
                state_dir=state_dir,
            )
            self._accounts[alias] = acct
            self._account_order.append(alias)

    def get_account(self, alias: str) -> TelegramAccount:
        """Get account by alias. Raises KeyError if not found."""
        return self._accounts[alias]

    @property
    def default_account(self) -> TelegramAccount:
        """Return the first configured account."""
        return self._accounts[self._account_order[0]]

    def list_accounts(self) -> list[str]:
        """Return list of account aliases in config order."""
        return list(self._account_order)

    def start(self) -> None:
        """Start all accounts' polling threads."""
        for acct in self._accounts.values():
            acct.start()

    def stop(self) -> None:
        """Stop all accounts."""
        for acct in self._accounts.values():
            acct.stop()
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_addon_telegram_service.py -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/addons/telegram/service.py tests/test_addon_telegram_service.py
git commit -m "feat(telegram): add TelegramService — multi-account orchestrator"
```

---

### Task 3: TelegramManager — Tool Dispatch & Filesystem

**Files:**
- Create: `src/lingtai/addons/telegram/manager.py`
- Test: `tests/test_addon_telegram_manager.py`

The heaviest class — handles all tool actions, filesystem persistence, and agent notifications. Reference: `src/lingtai/addons/imap/manager.py`.

- [ ] **Step 1: Write failing tests for core actions**

```python
# tests/test_addon_telegram_manager.py
from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock, patch

from lingtai.addons.telegram.manager import TelegramManager


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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_addon_telegram_manager.py -v`
Expected: FAIL — module not found

- [ ] **Step 3: Write TelegramManager**

```python
# src/lingtai/addons/telegram/manager.py
"""TelegramManager — tool dispatch + filesystem persistence.

Storage layout:
    working_dir/telegram/{account}/inbox/{uuid}/message.json
    working_dir/telegram/{account}/inbox/{uuid}/attachments/
    working_dir/telegram/{account}/sent/{uuid}/message.json
    working_dir/telegram/{account}/contacts.json
    working_dir/telegram/{account}/read.json

Mirrors IMAPMailManager patterns with Telegram-specific adaptations.
"""
from __future__ import annotations

import json
import os
import re
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING, Any
from uuid import uuid4

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent
    from .service import TelegramService

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": [
                "send", "check", "read", "reply", "search",
                "delete", "edit",
                "contacts", "add_contact", "remove_contact",
                "accounts",
            ],
            "description": (
                "send: send message to a chat (chat_id, text; optional media, reply_markup). "
                "check: list recent conversations with unread counts (optional account). "
                "read: read messages from a chat (chat_id; optional limit). "
                "reply: reply to a specific message (message_id from read results, text). "
                "search: search messages (query; optional account, chat_id). "
                "delete: delete a bot message (message_id). "
                "edit: edit a bot message (message_id, text; optional reply_markup). "
                "contacts: list saved contacts. "
                "add_contact: save a chat (chat_id, alias). "
                "remove_contact: remove a contact (alias or chat_id). "
                "accounts: list configured bot accounts."
            ),
        },
        "account": {
            "type": "string",
            "description": "Bot account alias (optional — defaults to first configured account)",
        },
        "chat_id": {
            "type": "integer",
            "description": "Telegram chat ID",
        },
        "text": {
            "type": "string",
            "description": "Message text",
        },
        "message_id": {
            "type": "string",
            "description": "Compound message ID: {account}:{chat_id}:{message_id}",
        },
        "media": {
            "type": "object",
            "properties": {
                "type": {"type": "string", "enum": ["photo", "document"]},
                "path": {"type": "string"},
            },
            "description": "Media attachment: {type: 'photo'|'document', path: '/path/to/file'}",
        },
        "reply_markup": {
            "type": "object",
            "description": "Inline keyboard markup",
        },
        "limit": {
            "type": "integer",
            "description": "Max messages to return (for read, default 10)",
            "default": 10,
        },
        "query": {
            "type": "string",
            "description": "Search query (regex pattern)",
        },
        "alias": {
            "type": "string",
            "description": "Contact alias for add_contact/remove_contact",
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Telegram bot client — interact with Telegram users via Bot API. "
    "Use 'send' for outgoing messages (text, photos, documents, inline keyboards). "
    "'check' to see recent conversations. "
    "'read' to read messages from a specific chat. "
    "'reply' to respond to a message (use compound ID from read results). "
    "'search' to find messages by text/sender. "
    "'delete'/'edit' to modify bot messages. "
    "'contacts' to manage saved contacts. "
    "'accounts' to list configured bot accounts."
)


class TelegramManager:
    """Tool handler + filesystem manager for the Telegram addon."""

    def __init__(
        self,
        agent: "BaseAgent",
        service: "TelegramService",
        working_dir: Path,
    ) -> None:
        self._agent = agent
        self._service = service
        self._working_dir = working_dir
        # Duplicate send protection: (account, chat_id, text) → count
        self._last_sent: dict[tuple[str, int, str], int] = {}
        self._dup_free_passes = 2

    def _account_dir(self, account: str) -> Path:
        return self._working_dir / "telegram" / account

    def _resolve_account(self, args: dict) -> str:
        """Get account alias from args, defaulting to first account."""
        return args.get("account") or self._service.default_account.alias

    @staticmethod
    def _parse_compound_id(compound_id: str) -> tuple[str, int, int]:
        """Parse '{account}:{chat_id}:{message_id}' → (account, chat_id, message_id)."""
        parts = compound_id.split(":")
        if len(parts) != 3:
            raise ValueError(f"Invalid message ID format: {compound_id}")
        return parts[0], int(parts[1]), int(parts[2])

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        self._service.start()

    def stop(self) -> None:
        self._service.stop()

    # ------------------------------------------------------------------
    # Action dispatch
    # ------------------------------------------------------------------

    def handle(self, args: dict) -> dict:
        action = args.get("action")
        try:
            if action == "send":
                return self._send(args)
            elif action == "check":
                return self._check(args)
            elif action == "read":
                return self._read(args)
            elif action == "reply":
                return self._reply(args)
            elif action == "search":
                return self._search(args)
            elif action == "delete":
                return self._delete(args)
            elif action == "edit":
                return self._edit(args)
            elif action == "contacts":
                return self._contacts(args)
            elif action == "add_contact":
                return self._add_contact(args)
            elif action == "remove_contact":
                return self._remove_contact(args)
            elif action == "accounts":
                return self._accounts()
            else:
                return {"error": f"Unknown telegram action: {action}"}
        except Exception as e:
            return {"error": str(e)}

    # ------------------------------------------------------------------
    # Incoming messages — called by TelegramService via on_message
    # ------------------------------------------------------------------

    def on_incoming(self, account_alias: str, update: dict) -> None:
        """Persist incoming update to disk and notify agent."""
        msg_id = str(uuid4())
        acct_dir = self._account_dir(account_alias)
        msg_dir = acct_dir / "inbox" / msg_id
        msg_dir.mkdir(parents=True, exist_ok=True)

        # Extract message data based on update type
        if "message" in update:
            tg_msg = update["message"]
            compound_id = f"{account_alias}:{tg_msg['chat']['id']}:{tg_msg['message_id']}"
            sender = tg_msg.get("from", {})
            payload = {
                "id": compound_id,
                "from": sender,
                "chat": tg_msg.get("chat", {}),
                "date": datetime.fromtimestamp(
                    tg_msg.get("date", 0), tz=timezone.utc,
                ).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "text": tg_msg.get("text") or tg_msg.get("caption") or "",
                "media": None,
                "reply_to_message_id": None,
                "callback_query": None,
            }
            # Handle reply_to
            if tg_msg.get("reply_to_message"):
                payload["reply_to_message_id"] = tg_msg["reply_to_message"]["message_id"]
            # Handle media
            self._download_media(account_alias, tg_msg, msg_dir, payload)
            username = sender.get("username") or sender.get("first_name", "unknown")

        elif "callback_query" in update:
            cq = update["callback_query"]
            tg_msg = cq.get("message", {})
            sender = cq.get("from", {})
            chat = tg_msg.get("chat", {})
            compound_id = f"{account_alias}:{chat.get('id', 0)}:{tg_msg.get('message_id', 0)}"
            payload = {
                "id": compound_id,
                "from": sender,
                "chat": chat,
                "date": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "text": "",
                "media": None,
                "reply_to_message_id": None,
                "callback_query": cq.get("data"),
            }
            username = sender.get("username") or sender.get("first_name", "unknown")

        elif "edited_message" in update:
            tg_msg = update["edited_message"]
            compound_id = f"{account_alias}:{tg_msg['chat']['id']}:{tg_msg['message_id']}"
            sender = tg_msg.get("from", {})
            payload = {
                "id": compound_id,
                "from": sender,
                "chat": tg_msg.get("chat", {}),
                "date": datetime.fromtimestamp(
                    tg_msg.get("edit_date", tg_msg.get("date", 0)), tz=timezone.utc,
                ).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "text": tg_msg.get("text") or tg_msg.get("caption") or "",
                "media": None,
                "reply_to_message_id": None,
                "callback_query": None,
            }
            username = sender.get("username") or sender.get("first_name", "unknown")
        else:
            return  # unsupported update type

        # Persist
        (msg_dir / "message.json").write_text(
            json.dumps(payload, indent=2, default=str), encoding="utf-8",
        )

        # Notify agent
        self._agent._mail_arrived.set()
        from lingtai_kernel.message import _make_message, MSG_REQUEST
        notification = (
            f"[system] New telegram message from {username} via {account_alias}.\n"
            f'Use telegram(action="check") to see your messages.'
        )
        msg = _make_message(MSG_REQUEST, "system", notification)
        self._agent.inbox.put(msg)
        self._agent._log(
            "telegram_received", sender=username, account=account_alias,
            text=payload.get("text", "")[:100],
        )

    def _download_media(
        self, account_alias: str, tg_msg: dict, msg_dir: Path, payload: dict,
    ) -> None:
        """Download photo/document attachments from a Telegram message."""
        file_id = None
        media_type = None

        if tg_msg.get("photo"):
            # Photos come as array of sizes — take the largest
            file_id = tg_msg["photo"][-1]["file_id"]
            media_type = "photo"
        elif tg_msg.get("document"):
            file_id = tg_msg["document"]["file_id"]
            media_type = "document"

        if file_id is None:
            return

        try:
            acct = self._service.get_account(account_alias)
            filename, data = acct.get_file(file_id)
            att_dir = msg_dir / "attachments"
            att_dir.mkdir(parents=True, exist_ok=True)
            filepath = att_dir / filename
            filepath.write_bytes(data)
            payload["media"] = {
                "type": media_type,
                "filename": filename,
                "path": str(filepath),
                "size": len(data),
            }
        except Exception as e:
            import logging
            logging.getLogger(__name__).warning(
                "Failed to download media: %s", e,
            )

    # ------------------------------------------------------------------
    # Filesystem helpers
    # ------------------------------------------------------------------

    def _list_messages(self, account: str, folder: str = "inbox") -> list[dict]:
        """Load all messages from a folder, sorted by date (newest first)."""
        folder_dir = self._account_dir(account) / folder
        if not folder_dir.is_dir():
            return []
        messages = []
        for msg_dir in folder_dir.iterdir():
            msg_file = msg_dir / "message.json"
            if msg_dir.is_dir() and msg_file.is_file():
                try:
                    data = json.loads(msg_file.read_text(encoding="utf-8"))
                    data["_dir"] = str(msg_dir)
                    messages.append(data)
                except (json.JSONDecodeError, OSError):
                    continue
        messages.sort(key=lambda m: m.get("date", ""), reverse=True)
        return messages

    def _read_ids(self, account: str) -> set[str]:
        path = self._account_dir(account) / "read.json"
        if path.is_file():
            try:
                return set(json.loads(path.read_text(encoding="utf-8")))
            except (json.JSONDecodeError, OSError):
                return set()
        return set()

    def _mark_read(self, account: str, compound_ids: list[str]) -> None:
        ids = self._read_ids(account)
        ids.update(compound_ids)
        acct_dir = self._account_dir(account)
        acct_dir.mkdir(parents=True, exist_ok=True)
        target = acct_dir / "read.json"
        fd, tmp = tempfile.mkstemp(dir=str(acct_dir), suffix=".tmp")
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

    def _load_contacts(self, account: str) -> dict:
        path = self._account_dir(account) / "contacts.json"
        if path.is_file():
            try:
                return json.loads(path.read_text(encoding="utf-8"))
            except (json.JSONDecodeError, OSError):
                return {}
        return {}

    def _save_contacts(self, account: str, contacts: dict) -> None:
        acct_dir = self._account_dir(account)
        acct_dir.mkdir(parents=True, exist_ok=True)
        target = acct_dir / "contacts.json"
        fd, tmp = tempfile.mkstemp(dir=str(acct_dir), suffix=".tmp")
        try:
            os.write(fd, json.dumps(contacts, indent=2).encode())
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

    # ------------------------------------------------------------------
    # Actions
    # ------------------------------------------------------------------

    def _send(self, args: dict) -> dict:
        account = self._resolve_account(args)
        chat_id = args.get("chat_id")
        text = args.get("text", "")
        media = args.get("media")
        reply_markup = args.get("reply_markup")

        if not chat_id:
            return {"error": "chat_id is required"}
        if not text and not media:
            return {"error": "text or media is required"}

        # Duplicate send protection
        dup_key = (account, chat_id, text)
        count = self._last_sent.get(dup_key, 0)
        if count >= self._dup_free_passes:
            return {
                "status": "blocked",
                "warning": "Identical message already sent. Think twice before repeating.",
            }

        acct = self._service.get_account(account)
        reply_to = args.get("_reply_to_message_id")

        # Send via Bot API
        if media:
            media_type = media.get("type")
            media_path = media.get("path", "")
            if media_type == "photo":
                result = acct.send_photo(
                    chat_id, media_path, caption=text or None,
                    reply_to_message_id=reply_to,
                )
            elif media_type == "document":
                result = acct.send_document(
                    chat_id, media_path, caption=text or None,
                    reply_to_message_id=reply_to,
                )
            else:
                return {"error": f"Unknown media type: {media_type}"}
        else:
            result = acct.send_message(
                chat_id, text, reply_markup=reply_markup,
                reply_to_message_id=reply_to,
            )

        # Track for duplicate detection
        self._last_sent[dup_key] = count + 1

        # Persist to sent/
        sent_id = str(uuid4())
        sent_dir = self._account_dir(account) / "sent" / sent_id
        sent_dir.mkdir(parents=True, exist_ok=True)
        tg_message_id = result.get("message_id", 0)
        compound_id = f"{account}:{chat_id}:{tg_message_id}"
        sent_record = {
            "id": compound_id,
            "to": {"chat_id": chat_id},
            "date": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "text": text,
            "media": media,
            "reply_markup": reply_markup,
            "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "status": "sent",
        }
        (sent_dir / "message.json").write_text(
            json.dumps(sent_record, indent=2, default=str), encoding="utf-8",
        )

        return {"status": "sent", "message_id": compound_id}

    def _check(self, args: dict) -> dict:
        account = self._resolve_account(args)
        messages = self._list_messages(account, "inbox")
        read_ids = self._read_ids(account)

        # Group by chat_id for conversation view
        conversations: dict[int, dict] = {}
        for msg in messages:
            cid = msg.get("chat", {}).get("id", 0)
            if cid not in conversations:
                conversations[cid] = {
                    "chat_id": cid,
                    "chat_type": msg.get("chat", {}).get("type", "private"),
                    "last_from": msg.get("from", {}),
                    "last_text": (msg.get("text") or "")[:100],
                    "last_date": msg.get("date", ""),
                    "total": 0,
                    "unread": 0,
                }
            conversations[cid]["total"] += 1
            if msg.get("id") and msg["id"] not in read_ids:
                conversations[cid]["unread"] += 1

        return {
            "status": "ok",
            "total": len(messages),
            "messages": list(conversations.values()),
        }

    def _read(self, args: dict) -> dict:
        account = self._resolve_account(args)
        chat_id = args.get("chat_id")
        limit = args.get("limit", 10)

        if not chat_id:
            return {"error": "chat_id is required"}

        messages = self._list_messages(account, "inbox")
        filtered = [m for m in messages if m.get("chat", {}).get("id") == chat_id]
        recent = filtered[:limit]

        # Mark as read
        compound_ids = [m["id"] for m in recent if m.get("id")]
        if compound_ids:
            self._mark_read(account, compound_ids)

        # Strip internal fields
        cleaned = []
        for m in recent:
            cleaned.append({
                "id": m.get("id"),
                "from": m.get("from"),
                "chat": m.get("chat"),
                "date": m.get("date"),
                "text": m.get("text"),
                "media": m.get("media"),
                "callback_query": m.get("callback_query"),
                "reply_to_message_id": m.get("reply_to_message_id"),
            })

        return {"status": "ok", "messages": cleaned}

    def _reply(self, args: dict) -> dict:
        compound_id = args.get("message_id", "")
        text = args.get("text", "")
        if not compound_id:
            return {"error": "message_id is required"}
        if not text:
            return {"error": "text is required"}

        account, chat_id, tg_msg_id = self._parse_compound_id(compound_id)
        return self._send({
            "account": account,
            "chat_id": chat_id,
            "text": text,
            "media": args.get("media"),
            "reply_markup": args.get("reply_markup"),
            # We need to pass reply_to_message_id through
            "_reply_to_message_id": tg_msg_id,
        })

    def _search(self, args: dict) -> dict:
        query = args.get("query", "")
        if not query:
            return {"error": "query is required"}
        account = self._resolve_account(args)
        target_chat = args.get("chat_id")

        try:
            pattern = re.compile(query, re.IGNORECASE)
        except re.error as e:
            return {"error": f"Invalid regex: {e}"}

        messages = self._list_messages(account, "inbox")
        matches = []
        for msg in messages:
            if target_chat and msg.get("chat", {}).get("id") != target_chat:
                continue
            searchable = " ".join([
                str(msg.get("from", {}).get("username", "")),
                str(msg.get("from", {}).get("first_name", "")),
                msg.get("text", ""),
            ])
            if pattern.search(searchable):
                matches.append({
                    "id": msg.get("id"),
                    "from": msg.get("from"),
                    "date": msg.get("date"),
                    "text": msg.get("text"),
                })

        return {"status": "ok", "total": len(matches), "messages": matches}

    def _delete(self, args: dict) -> dict:
        compound_id = args.get("message_id", "")
        if not compound_id:
            return {"error": "message_id is required"}
        account, chat_id, tg_msg_id = self._parse_compound_id(compound_id)
        acct = self._service.get_account(account)
        acct.delete_message(chat_id=chat_id, message_id=tg_msg_id)
        return {"status": "deleted", "message_id": compound_id}

    def _edit(self, args: dict) -> dict:
        compound_id = args.get("message_id", "")
        text = args.get("text", "")
        if not compound_id:
            return {"error": "message_id is required"}
        if not text:
            return {"error": "text is required"}
        account, chat_id, tg_msg_id = self._parse_compound_id(compound_id)
        reply_markup = args.get("reply_markup")
        acct = self._service.get_account(account)
        acct.edit_message(
            chat_id=chat_id, message_id=tg_msg_id, text=text,
            reply_markup=reply_markup,
        )
        return {"status": "edited", "message_id": compound_id}

    def _contacts(self, args: dict) -> dict:
        account = self._resolve_account(args)
        return {"status": "ok", "contacts": self._load_contacts(account)}

    def _add_contact(self, args: dict) -> dict:
        account = self._resolve_account(args)
        chat_id = args.get("chat_id")
        alias = args.get("alias", "")
        if not chat_id:
            return {"error": "chat_id is required"}
        if not alias:
            return {"error": "alias is required"}
        contacts = self._load_contacts(account)
        contacts[alias] = {
            "chat_id": chat_id,
            "username": args.get("username", ""),
            "first_name": args.get("first_name", ""),
        }
        self._save_contacts(account, contacts)
        return {"status": "added", "alias": alias}

    def _remove_contact(self, args: dict) -> dict:
        account = self._resolve_account(args)
        alias = args.get("alias", "")
        chat_id = args.get("chat_id")
        contacts = self._load_contacts(account)
        if alias and alias in contacts:
            del contacts[alias]
            self._save_contacts(account, contacts)
            return {"status": "removed", "alias": alias}
        elif chat_id:
            to_remove = [k for k, v in contacts.items() if v.get("chat_id") == chat_id]
            for k in to_remove:
                del contacts[k]
            if to_remove:
                self._save_contacts(account, contacts)
                return {"status": "removed", "aliases": to_remove}
        return {"error": "Contact not found"}

    def _accounts(self) -> dict:
        return {"status": "ok", "accounts": self._service.list_accounts()}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_addon_telegram_manager.py -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/addons/telegram/manager.py tests/test_addon_telegram_manager.py
git commit -m "feat(telegram): add TelegramManager — tool dispatch + filesystem"
```

---

### Task 4: Setup & Registration

**Files:**
- Modify: `src/lingtai/addons/telegram/__init__.py` (replace placeholder)
- Modify: `src/lingtai/addons/__init__.py` (add to `_BUILTIN`)
- Modify: `pyproject.toml` (add optional dep)

- [ ] **Step 1: Write `setup()` in `__init__.py`**

```python
# src/lingtai/addons/telegram/__init__.py
"""Telegram addon — Bot API client for customer service.

Adds a `telegram` tool with its own mailbox (working_dir/telegram/).
Supports multiple bot accounts, text + images + documents, inline keyboards.

Usage (single account):
    agent = Agent(
        capabilities=["email", "file"],
        addons={"telegram": {
            "bot_token": "123456:ABC-DEF...",
            "allowed_users": [111, 222],
        }},
    )

Usage (multi-account):
    agent = Agent(
        capabilities=["email", "file"],
        addons={"telegram": {
            "accounts": [
                {"alias": "support", "bot_token": "123:ABC", "allowed_users": [111]},
                {"alias": "sales", "bot_token": "789:DEF"},
            ],
        }},
    )
"""
from __future__ import annotations

import logging
from pathlib import Path
from typing import TYPE_CHECKING

from .manager import TelegramManager, SCHEMA, DESCRIPTION
from .service import TelegramService

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    accounts: list[dict] | None = None,
    bot_token: str | None = None,
    allowed_users: list[int] | None = None,
    poll_interval: float = 1.0,
    **kwargs,
) -> TelegramManager:
    """Set up Telegram addon — registers telegram tool, creates services.

    Listeners are NOT started here — they start in TelegramManager.start(),
    which is called by Agent.start() via the addon lifecycle.
    """
    # Normalize single-account shorthand to accounts list
    if accounts is None:
        if bot_token is None:
            raise ValueError("telegram addon requires 'bot_token' or 'accounts'")
        accounts = [{
            "alias": "default",
            "bot_token": bot_token,
            "allowed_users": allowed_users,
            "poll_interval": poll_interval,
        }]

    working_dir = Path(agent._working_dir)

    # Use a list to hold the manager reference so the lambda can capture it
    # before the manager is created (resolved on first call, after start()).
    mgr_ref: list[TelegramManager | None] = [None]

    svc = TelegramService(
        working_dir=working_dir,
        accounts_config=accounts,
        on_message=lambda alias, update: mgr_ref[0].on_incoming(alias, update),
    )

    mgr = TelegramManager(agent=agent, service=svc, working_dir=working_dir)
    mgr_ref[0] = mgr

    account_names = ", ".join(svc.list_accounts())
    agent.add_tool(
        "telegram", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt=(
            f"Telegram bot accounts: {account_names}\n"
            f"Use telegram(action=...) for Telegram conversations."
        ),
    )

    log.info("Telegram addon configured: %s", account_names)
    return mgr
```

- [ ] **Step 2: Add `"telegram"` to `_BUILTIN` in `src/lingtai/addons/__init__.py`**

Add to the `_BUILTIN` dict:
```python
_BUILTIN: dict[str, str] = {
    "imap": ".imap",
    "telegram": ".telegram",
}
```

- [ ] **Step 3: Add optional dependency to `pyproject.toml`**

Add under `[project.optional-dependencies]`:
```toml
telegram = ["httpx>=0.27"]
all = ["lingtai[gemini,openai,anthropic,minimax,telegram]"]
```

- [ ] **Step 4: Smoke test**

Run: `python -c "from lingtai.addons.telegram import setup; print('OK')"`
Expected: `OK` (if httpx is installed) or ImportError for httpx (expected if not installed)

Run: `pip install httpx && python -c "from lingtai.addons.telegram import setup; print('OK')"`
Expected: `OK`

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/addons/telegram/__init__.py src/lingtai/addons/__init__.py pyproject.toml
git commit -m "feat(telegram): register addon + httpx optional dep"
```

---

### Task 5: Run Full Test Suite & Fix Issues

**Files:**
- All test files from Tasks 1-3

- [ ] **Step 1: Run all telegram tests**

Run: `python -m pytest tests/test_addon_telegram_account.py tests/test_addon_telegram_service.py tests/test_addon_telegram_manager.py -v`
Expected: All PASS

- [ ] **Step 2: Run full project test suite to check for regressions**

Run: `python -m pytest tests/ -v`
Expected: All PASS (no regressions)

- [ ] **Step 3: Smoke test the module import**

Run: `python -c "import lingtai"`
Expected: No errors

- [ ] **Step 4: Fix any failing tests**

If any tests fail, fix the implementation and re-run.

- [ ] **Step 5: Final commit if any fixes were needed**

```bash
git add -u
git commit -m "fix(telegram): address test failures"
```
