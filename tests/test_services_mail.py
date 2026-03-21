"""Tests for MailService and TCPMailService."""
import base64
import json
import threading
import time
from pathlib import Path

import pytest

from lingtai_kernel.services.mail import TCPMailService


class TestTCPMailService:
    def test_send_to_listener(self):
        """Test basic send/receive via TCP."""
        received = []
        event = threading.Event()

        def on_message(msg):
            received.append(msg)
            event.set()

        import socket
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        listener = TCPMailService(listen_port=port)
        listener.listen(on_message)

        try:
            sender = TCPMailService()
            result = sender.send(
                f"127.0.0.1:{port}",
                {"from": "127.0.0.1:9999", "to": f"127.0.0.1:{port}", "message": "hello"},
            )
            assert result is None

            assert event.wait(timeout=5.0), "Message not received within timeout"
            assert len(received) == 1
            assert received[0]["message"] == "hello"
        finally:
            listener.stop()

    def test_send_to_nonexistent_returns_error(self):
        """Sending to a non-listening port should return an error string."""
        sender = TCPMailService()
        result = sender.send("127.0.0.1:1", {"message": "hello"})
        assert isinstance(result, str)
        assert "Cannot reach" in result

    def test_send_bad_address_returns_error(self):
        """Bad address format should return an error string."""
        sender = TCPMailService()
        result = sender.send("not-an-address", {"message": "hello"})
        assert isinstance(result, str)
        assert "Invalid address" in result
        result = sender.send("", {"message": "hello"})
        assert isinstance(result, str)

    def test_address_property(self):
        """Address should reflect listen config."""
        svc = TCPMailService()
        assert svc.address is None

        svc = TCPMailService(listen_port=8888)
        assert svc.address == "127.0.0.1:8888"

    def test_stop_is_idempotent(self):
        """Calling stop multiple times should not raise."""
        svc = TCPMailService(listen_port=0)
        svc.stop()
        svc.stop()

    def test_multiple_messages(self):
        """Multiple messages should all be received."""
        received = []
        all_done = threading.Event()

        def on_message(msg):
            received.append(msg)
            if len(received) >= 3:
                all_done.set()

        import socket
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        listener = TCPMailService(listen_port=port)
        listener.listen(on_message)

        try:
            sender = TCPMailService()
            for i in range(3):
                sender.send(f"127.0.0.1:{port}", {"message": f"msg-{i}"})
                time.sleep(0.05)

            assert all_done.wait(timeout=5.0), f"Only received {len(received)} of 3 messages"
            messages = [r["message"] for r in received]
            assert "msg-0" in messages
            assert "msg-1" in messages
            assert "msg-2" in messages
        finally:
            listener.stop()


class TestMailAttachments:
    def test_send_with_attachment(self, tmp_path):
        """Sender encodes attachment files into the wire message."""
        import socket
        import threading

        received = []
        event = threading.Event()

        def on_message(msg):
            received.append(msg)
            event.set()

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        # Create a file to attach
        sender_dir = tmp_path / "sender"
        sender_dir.mkdir()
        attachment = sender_dir / "image.png"
        attachment.write_bytes(b"\x89PNG_TEST_DATA")

        receiver_dir = tmp_path / "receiver"
        receiver_dir.mkdir()
        listener = TCPMailService(listen_port=port, working_dir=receiver_dir)
        listener.listen(on_message)

        try:
            sender = TCPMailService(working_dir=sender_dir)
            result = sender.send(
                f"127.0.0.1:{port}",
                {
                    "from": "sender",
                    "to": f"127.0.0.1:{port}",
                    "message": "here is an image",
                    "attachments": [str(attachment)],
                },
            )
            assert result is None
            assert event.wait(timeout=5.0)
            msg = received[0]
            # Receiver should get local file paths, not base64
            assert "attachments" in msg
            assert len(msg["attachments"]) == 1
            recv_path = Path(msg["attachments"][0])
            assert recv_path.exists()
            assert recv_path.read_bytes() == b"\x89PNG_TEST_DATA"
            assert "mailbox" in str(recv_path)
        finally:
            listener.stop()

    def test_send_without_attachment(self, tmp_path):
        """Messages without attachments still work normally."""
        import socket
        import threading

        received = []
        event = threading.Event()

        def on_message(msg):
            received.append(msg)
            event.set()

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        listener = TCPMailService(listen_port=port, working_dir=tmp_path)
        listener.listen(on_message)

        try:
            sender = TCPMailService()
            result = sender.send(
                f"127.0.0.1:{port}",
                {"from": "sender", "to": f"127.0.0.1:{port}", "message": "no attachments"},
            )
            assert result is None
            assert event.wait(timeout=5.0)
            assert received[0]["message"] == "no attachments"
        finally:
            listener.stop()

    def test_attachment_file_not_found(self, tmp_path):
        """send() returns False when attachment file does not exist."""
        sender = TCPMailService(working_dir=tmp_path)
        result = sender.send(
            "127.0.0.1:1",
            {"from": "s", "to": "r", "message": "hi", "attachments": ["/nonexistent/file.png"]},
        )
        assert isinstance(result, str)
        assert "Attachment not found" in result

    def test_mailbox_directory_structure(self, tmp_path):
        """Received messages are saved in mailbox/<uuid>/message.json + attachments/."""
        import socket
        import threading

        received = []
        event = threading.Event()

        def on_message(msg):
            received.append(msg)
            event.set()

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        sender_dir = tmp_path / "sender"
        sender_dir.mkdir()
        attachment = sender_dir / "song.mp3"
        attachment.write_bytes(b"MP3_DATA")

        receiver_dir = tmp_path / "receiver"
        receiver_dir.mkdir()
        listener = TCPMailService(listen_port=port, working_dir=receiver_dir)
        listener.listen(on_message)

        try:
            sender = TCPMailService(working_dir=sender_dir)
            sender.send(
                f"127.0.0.1:{port}",
                {"from": "s", "to": "r", "message": "music", "attachments": [str(attachment)]},
            )
            assert event.wait(timeout=5.0)

            # Check mailbox structure
            mailbox = receiver_dir / "mailbox" / "inbox"
            assert mailbox.is_dir()
            msg_dirs = list(mailbox.iterdir())
            assert len(msg_dirs) == 1
            msg_dir = msg_dirs[0]
            assert (msg_dir / "message.json").is_file()
            assert (msg_dir / "attachments" / "song.mp3").is_file()
            assert (msg_dir / "attachments" / "song.mp3").read_bytes() == b"MP3_DATA"
        finally:
            listener.stop()

    def test_message_without_attachments_also_persisted(self, tmp_path):
        """Even messages without attachments get persisted to mailbox."""
        import socket
        import threading

        received = []
        event = threading.Event()

        def on_message(msg):
            received.append(msg)
            event.set()

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        listener = TCPMailService(listen_port=port, working_dir=tmp_path)
        listener.listen(on_message)

        try:
            sender = TCPMailService()
            sender.send(f"127.0.0.1:{port}", {"from": "s", "to": "r", "message": "plain"})
            assert event.wait(timeout=5.0)

            mailbox = tmp_path / "mailbox" / "inbox"
            assert mailbox.is_dir()
            msg_dirs = list(mailbox.iterdir())
            assert len(msg_dirs) == 1
            assert (msg_dirs[0] / "message.json").is_file()
        finally:
            listener.stop()
