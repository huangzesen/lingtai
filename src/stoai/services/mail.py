"""MailService — abstract message transport backing the mail intrinsic.

First implementation: TCPMailService (JSON over TCP with length-prefix framing).
Future: WebSocketMailService, UnixSocketMailService, PipeMailService.

Design principles:
- Fire-and-forget: send() returns immediately, no request/response coupling
- Inbox model: listener queues incoming messages for the agent to process
- No registry: the caller must know the address (discovery is Forum's job)
- Address format is opaque to BaseAgent — each service defines its own
"""
from __future__ import annotations

import base64
import json
import socket
import struct
import threading
import uuid
from abc import ABC, abstractmethod
from pathlib import Path
from typing import Any, Callable


class MailService(ABC):
    """Abstract message transport service.

    Backs the mail intrinsic. Implementations provide the actual
    transport mechanism (TCP, WebSocket, Unix socket, etc.).
    """

    @abstractmethod
    def send(self, address: str, message: dict) -> str | None:
        """Send a message to an address. Returns None on success, error string on failure.

        Fire-and-forget — does not wait for a response.
        The address format is transport-specific (e.g. "localhost:8301" for TCP).
        """
        ...

    @abstractmethod
    def listen(self, on_message: Callable[[dict], None]) -> None:
        """Start listening for incoming messages.

        on_message is called for each received message.
        This should be non-blocking (start a background thread).
        """
        ...

    @abstractmethod
    def stop(self) -> None:
        """Stop listening and clean up resources."""
        ...

    @property
    @abstractmethod
    def address(self) -> str | None:
        """This service's listening address, or None if not listening."""
        ...


class TCPMailService(MailService):
    """TCP implementation — JSON over TCP with length-prefix framing.

    Message format on wire: [4-byte big-endian length][UTF-8 JSON payload]

    Usage:
        # Listening agent
        svc = TCPMailService(listen_port=8301)
        svc.listen(on_message=lambda msg: print(msg))

        # Sending agent
        svc = TCPMailService()  # no listen_port = send-only
        svc.send("localhost:8301", {"from": "localhost:8300", "message": "hello"})
    """

    def __init__(
        self,
        listen_port: int | None = None,
        listen_host: str = "127.0.0.1",
        working_dir: Path | str | None = None,
    ) -> None:
        self._listen_port = listen_port
        self._listen_host = listen_host
        self._working_dir = Path(working_dir) if working_dir else None
        self._server_socket: socket.socket | None = None
        self._listener_thread: threading.Thread | None = None
        self._running = False
        self._info_handler: Callable[[], dict] | None = None

    def send(self, address: str, message: dict) -> str | None:
        """Send a message to host:port. Returns None on success, error string on failure."""
        try:
            host, port_str = address.rsplit(":", 1)
            port = int(port_str)
        except (ValueError, AttributeError):
            return f"Invalid address {address!r} — expected host:port (e.g. 127.0.0.1:8302)"

        # Before JSON serialization, encode any file attachments
        if "attachments" in message and message["attachments"]:
            encoded = []
            for fpath in message["attachments"]:
                p = Path(fpath)
                if not p.is_file():
                    return f"Attachment not found: {fpath}"
                encoded.append({
                    "filename": p.name,
                    "data": base64.b64encode(p.read_bytes()).decode("ascii"),
                })
            # Create a NEW dict — do not mutate the original
            message = {k: v for k, v in message.items() if k != "attachments"}
            message["_encoded_attachments"] = encoded

        try:
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
                sock.settimeout(5.0)
                sock.connect((host, port))
                payload = json.dumps(message, ensure_ascii=False).encode("utf-8")
                # Length-prefix framing: 4-byte big-endian length + payload
                sock.sendall(struct.pack(">I", len(payload)) + payload)
                return None
        except (OSError, ConnectionError, TimeoutError) as e:
            return f"Cannot reach {address}: {e}"

    def listen(self, on_message: Callable[[dict], None]) -> None:
        """Start a TCP server listening for incoming messages."""
        if self._listen_port is None:
            raise RuntimeError("Cannot listen without a listen_port")

        self._server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._server_socket.bind((self._listen_host, self._listen_port))
        self._server_socket.listen(16)
        self._server_socket.settimeout(1.0)  # for clean shutdown
        self._running = True

        def _accept_loop():
            while self._running:
                try:
                    conn, addr = self._server_socket.accept()
                except socket.timeout:
                    continue
                except OSError:
                    break
                threading.Thread(
                    target=self._handle_connection,
                    args=(conn, on_message),
                    daemon=True,
                ).start()

        self._listener_thread = threading.Thread(target=_accept_loop, daemon=True)
        self._listener_thread.start()

    def _handle_connection(self, conn: socket.socket, on_message: Callable[[dict], None]) -> None:
        """Read a single length-prefixed JSON message from a connection."""
        try:
            conn.settimeout(10.0)
            # Read 4-byte length prefix
            length_data = self._recv_exact(conn, 4)
            if length_data is None:
                return
            length = struct.unpack(">I", length_data)[0]
            if length > 100_000_000:  # 100MB safety limit
                return
            # Read payload
            payload_data = self._recv_exact(conn, length)
            if payload_data is None:
                return
            payload = json.loads(payload_data.decode("utf-8"))

            # TCP discovery: respond with agent info and close
            if payload.get("_stoai") == "info" and self._info_handler:
                info = self._info_handler()
                resp = json.dumps(info, ensure_ascii=False).encode("utf-8")
                conn.sendall(struct.pack(">I", len(resp)) + resp)
                return

            # Persist to mailbox/inbox/ and decode attachments
            if self._working_dir is not None:
                from datetime import datetime, timezone
                msg_id = str(uuid.uuid4())
                msg_dir = self._working_dir / "mailbox" / "inbox" / msg_id

                if "_encoded_attachments" in payload:
                    att_dir = msg_dir / "attachments"
                    att_dir.mkdir(parents=True, exist_ok=True)
                    local_paths = []
                    for att in payload["_encoded_attachments"]:
                        out = att_dir / att["filename"]
                        out.write_bytes(base64.b64decode(att["data"]))
                        local_paths.append(str(out))
                    del payload["_encoded_attachments"]
                    payload["attachments"] = local_paths
                else:
                    msg_dir.mkdir(parents=True, exist_ok=True)

                # Inject metadata for email capability
                payload["_mailbox_id"] = msg_id
                payload["received_at"] = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

                # Save message.json (without binary data)
                (msg_dir / "message.json").write_text(
                    json.dumps({k: v for k, v in payload.items()}, indent=2, default=str)
                )

            on_message(payload)
        except (json.JSONDecodeError, OSError, UnicodeDecodeError):
            pass
        finally:
            conn.close()

    @staticmethod
    def _recv_exact(sock: socket.socket, n: int) -> bytes | None:
        """Read exactly n bytes from socket."""
        data = bytearray()
        while len(data) < n:
            chunk = sock.recv(n - len(data))
            if not chunk:
                return None
            data.extend(chunk)
        return bytes(data)

    def stop(self) -> None:
        """Stop the listener."""
        self._running = False
        if self._server_socket:
            try:
                self._server_socket.close()
            except OSError:
                pass
        if self._listener_thread:
            self._listener_thread.join(timeout=3.0)
        self._server_socket = None
        self._listener_thread = None

    @property
    def address(self) -> str | None:
        if self._listen_port is None:
            return None
        return f"{self._listen_host}:{self._listen_port}"
