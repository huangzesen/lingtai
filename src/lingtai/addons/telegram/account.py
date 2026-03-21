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
