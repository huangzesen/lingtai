"""Tests for EmailService and TCPEmailService."""
import json
import threading
import time

import pytest

from stoai.services.email import TCPEmailService


class TestTCPEmailService:
    def test_send_to_listener(self):
        """Test basic send/receive via TCP."""
        received = []
        event = threading.Event()

        def on_message(msg):
            received.append(msg)
            event.set()

        # Start listener
        listener = TCPEmailService(listen_port=0)  # port 0 = OS assigns
        # We need a real port, so use a fixed one for testing
        import socket
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        listener = TCPEmailService(listen_port=port)
        listener.listen(on_message)

        try:
            # Send a message
            sender = TCPEmailService()
            result = sender.send(
                f"127.0.0.1:{port}",
                {"from": "127.0.0.1:9999", "to": f"127.0.0.1:{port}", "message": "hello"},
            )
            assert result is True

            # Wait for receipt
            assert event.wait(timeout=5.0), "Message not received within timeout"
            assert len(received) == 1
            assert received[0]["message"] == "hello"
        finally:
            listener.stop()

    def test_send_to_nonexistent_returns_false(self):
        """Sending to a non-listening port should return False."""
        sender = TCPEmailService()
        result = sender.send("127.0.0.1:1", {"message": "hello"})
        assert result is False

    def test_send_bad_address_returns_false(self):
        """Bad address format should return False."""
        sender = TCPEmailService()
        assert sender.send("not-an-address", {"message": "hello"}) is False
        assert sender.send("", {"message": "hello"}) is False

    def test_address_property(self):
        """Address should reflect listen config."""
        svc = TCPEmailService()
        assert svc.address is None

        svc = TCPEmailService(listen_port=8888)
        assert svc.address == "127.0.0.1:8888"

    def test_stop_is_idempotent(self):
        """Calling stop multiple times should not raise."""
        svc = TCPEmailService(listen_port=0)
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

        listener = TCPEmailService(listen_port=port)
        listener.listen(on_message)

        try:
            sender = TCPEmailService()
            for i in range(3):
                sender.send(f"127.0.0.1:{port}", {"message": f"msg-{i}"})
                time.sleep(0.05)  # small delay to avoid connection races

            assert all_done.wait(timeout=5.0), f"Only received {len(received)} of 3 messages"
            messages = [r["message"] for r in received]
            assert "msg-0" in messages
            assert "msg-1" in messages
            assert "msg-2" in messages
        finally:
            listener.stop()
