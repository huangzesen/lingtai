"""FeishuAccount — single app credential, WebSocket listener + REST sender.

One daemon thread per account runs the lark-oapi WebSocket client.
Constructor stores config only — no connections, no threads.
start() spawns the WebSocket thread and initialises the REST client.
stop() signals the thread to stop and joins it.
"""
from __future__ import annotations

import json
import logging
import threading
from pathlib import Path
from typing import Any, Callable

logger = logging.getLogger(__name__)

# lark_oapi is lazy-imported so the module stays importable without
# the optional dependency installed.
lark: Any = None


def _import_lark() -> Any:
    global lark
    if lark is None:
        import lark_oapi as _lark
        lark = _lark
    return lark


class FeishuAccount:
    """Manages a single Feishu (Lark) app credential — WS polling + REST sending."""

    def __init__(
        self,
        alias: str,
        app_id: str,
        app_secret: str,
        allowed_users: list[str] | None,
        on_message: Callable[[str, Any], None] | None = None,
        state_dir: Path | None = None,
    ) -> None:
        self.alias = alias
        self._app_id = app_id
        self._app_secret = app_secret
        self._allowed_users: set[str] | None = (
            set(allowed_users) if allowed_users else None
        )
        self._on_message = on_message
        self._state_dir = state_dir

        self._ws_thread: threading.Thread | None = None
        self._ws_client: Any = None
        self._rest_client: Any = None
        self._stop_event = threading.Event()
        self._bot_info: dict | None = None

        self._load_state()

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        """Build REST client, register WS event handler, start polling thread."""
        if self._ws_thread is not None:
            return

        _lark = _import_lark()

        # REST client (for sending)
        self._rest_client = (
            _lark.Client.builder()
            .app_id(self._app_id)
            .app_secret(self._app_secret)
            .build()
        )

        # Store minimal bot info — full bot info API path varies by SDK version
        self._bot_info = {"app_id": self._app_id}
        self._save_state()

        # Event handler
        def _handle_message(data: Any) -> None:
            try:
                self._process_event(data)
            except Exception as exc:
                logger.warning(
                    "Feishu event processing error (%s): %s", self.alias, exc
                )

        event_handler = (
            _lark.EventDispatcherHandler.builder("", "")
            .register_p2_im_message_receive_v1(_handle_message)
            .build()
        )

        # WebSocket client — start() blocks, run in daemon thread
        self._stop_event.clear()
        self._ws_client = _lark.ws.Client(
            self._app_id,
            self._app_secret,
            event_handler=event_handler,
            log_level=_lark.LogLevel.INFO,
        )

        self._ws_thread = threading.Thread(
            target=self._ws_loop,
            daemon=True,
            name=f"feishu-ws-{self.alias}",
        )
        self._ws_thread.start()
        logger.info(
            "Feishu account '%s' started (app_id=%s)",
            self.alias,
            self._app_id,
        )

    def _ws_loop(self) -> None:
        """Run the blocking WebSocket client in a background thread."""
        try:
            self._ws_client.start()
        except Exception as e:
            if not self._stop_event.is_set():
                logger.warning(
                    "Feishu WS client exited unexpectedly (%s): %s",
                    self.alias, e,
                )

    def stop(self) -> None:
        """Signal the WebSocket thread to stop."""
        self._stop_event.set()
        if self._ws_client is not None:
            try:
                self._ws_client.stop()
            except Exception:
                pass
        if self._ws_thread is not None:
            self._ws_thread.join(timeout=5.0)
            self._ws_thread = None

    # ------------------------------------------------------------------
    # Event processing
    # ------------------------------------------------------------------

    def _process_event(self, data: Any) -> None:
        """Filter by allowed_users and dispatch to on_message callback."""
        event = getattr(data, "event", None)
        if event is None:
            return

        sender = getattr(event, "sender", None)
        sender_id = getattr(sender, "sender_id", None) if sender else None
        open_id: str = getattr(sender_id, "open_id", "") if sender_id else ""

        if self._allowed_users is not None and open_id not in self._allowed_users:
            return

        if self._on_message:
            self._on_message(self.alias, data)

    # ------------------------------------------------------------------
    # Sending
    # ------------------------------------------------------------------

    def send_text(
        self,
        receive_id: str,
        receive_id_type: str,
        text: str,
    ) -> dict:
        """Send a plain-text message. Returns created Message fields as dict."""
        from lark_oapi.api.im.v1 import (
            CreateMessageRequest,
            CreateMessageRequestBody,
        )

        request = (
            CreateMessageRequest.builder()
            .receive_id_type(receive_id_type)
            .request_body(
                CreateMessageRequestBody.builder()
                .receive_id(receive_id)
                .msg_type("text")
                .content(json.dumps({"text": text}))
                .build()
            )
            .build()
        )
        response = self._rest_client.im.v1.message.create(request)
        if not response.success():
            raise RuntimeError(
                f"Feishu send_text failed: code={response.code} msg={response.msg}"
            )
        data = response.data
        return {
            "message_id": getattr(data, "message_id", ""),
            "chat_id": getattr(data, "chat_id", ""),
            "create_time": getattr(data, "create_time", ""),
        }

    def reply_text(self, message_id: str, text: str) -> dict:
        """Reply to a specific message by Feishu message_id."""
        from lark_oapi.api.im.v1 import (
            ReplyMessageRequest,
            ReplyMessageRequestBody,
        )

        request = (
            ReplyMessageRequest.builder()
            .message_id(message_id)
            .request_body(
                ReplyMessageRequestBody.builder()
                .msg_type("text")
                .content(json.dumps({"text": text}))
                .build()
            )
            .build()
        )
        response = self._rest_client.im.v1.message.reply(request)
        if not response.success():
            raise RuntimeError(
                f"Feishu reply_text failed: code={response.code} msg={response.msg}"
            )
        data = response.data
        return {
            "message_id": getattr(data, "message_id", ""),
            "chat_id": getattr(data, "chat_id", ""),
        }

    # ------------------------------------------------------------------
    # State persistence
    # ------------------------------------------------------------------

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
            self._bot_info = data.get("bot_info")
        except (json.JSONDecodeError, OSError) as e:
            logger.warning("Failed to load Feishu state: %s", e)

    def _save_state(self) -> None:
        path = self._state_path()
        if path is None:
            return
        path.parent.mkdir(parents=True, exist_ok=True)
        data = {"bot_info": self._bot_info}
        path.write_text(json.dumps(data, indent=2), encoding="utf-8")
