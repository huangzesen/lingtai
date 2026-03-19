from __future__ import annotations

from unittest.mock import patch, MagicMock

from stoai.addons.gmail.service import GoogleMailService


def test_construction():
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="xxxx xxxx xxxx xxxx",
    )
    assert svc.address == "agent@gmail.com"


def test_send_via_smtp():
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )
    with patch("stoai.addons.gmail.service.smtplib.SMTP") as MockSMTP:
        mock_smtp = MagicMock()
        MockSMTP.return_value.__enter__ = MagicMock(return_value=mock_smtp)
        MockSMTP.return_value.__exit__ = MagicMock(return_value=False)

        result = svc.send("user@gmail.com", {
            "subject": "test",
            "message": "hello",
        })

        assert result is None
        mock_smtp.starttls.assert_called_once()
        mock_smtp.login.assert_called_once_with("agent@gmail.com", "test")
        mock_smtp.send_message.assert_called_once()


def test_deliver_email_persists(tmp_path):
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
        working_dir=tmp_path,
    )
    received = []
    svc._deliver_email(lambda p: received.append(p), "user@gmail.com", "hi", "hello")

    assert len(received) == 1
    assert received[0]["from"] == "user@gmail.com"
    mid = received[0]["_mailbox_id"]
    msg_file = tmp_path / "gmail" / "inbox" / mid / "message.json"
    assert msg_file.is_file()


def test_send_empty_rejected():
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )
    result = svc.send("user@gmail.com", {"subject": "", "message": ""})
    assert result is not None  # error string


def test_send_with_attachments(tmp_path):
    """Attachments should be included in the MIME message."""
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )

    # Create a test image file
    img_path = tmp_path / "photo.png"
    img_path.write_bytes(b"\x89PNG\r\n\x1a\n" + b"\x00" * 100)

    with patch("stoai.addons.gmail.service.smtplib.SMTP") as MockSMTP:
        mock_smtp = MagicMock()
        MockSMTP.return_value.__enter__ = MagicMock(return_value=mock_smtp)
        MockSMTP.return_value.__exit__ = MagicMock(return_value=False)

        result = svc.send("user@gmail.com", {
            "subject": "test",
            "message": "see attached",
            "attachments": [str(img_path)],
        })

        assert result is None
        mock_smtp.send_message.assert_called_once()
        sent_msg = mock_smtp.send_message.call_args[0][0]
        assert sent_msg.is_multipart()
        # Should have text part + 1 attachment
        parts = list(sent_msg.walk())
        content_types = [p.get_content_type() for p in parts]
        assert "image/png" in content_types


def test_send_without_attachments_still_works():
    """Plain text send (no attachments) should remain a simple MIMEText."""
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )
    with patch("stoai.addons.gmail.service.smtplib.SMTP") as MockSMTP:
        mock_smtp = MagicMock()
        MockSMTP.return_value.__enter__ = MagicMock(return_value=mock_smtp)
        MockSMTP.return_value.__exit__ = MagicMock(return_value=False)

        result = svc.send("user@gmail.com", {
            "subject": "test",
            "message": "hello",
        })

        assert result is None
        sent_msg = mock_smtp.send_message.call_args[0][0]
        # No attachments → plain MIMEText (not multipart)
        assert not sent_msg.is_multipart()


def test_send_with_missing_attachment():
    """Non-existent attachment path should return an error."""
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )
    result = svc.send("user@gmail.com", {
        "subject": "test",
        "message": "hello",
        "attachments": ["/nonexistent/file.png"],
    })
    assert result is not None  # error string
    assert "not found" in result.lower() or "not exist" in result.lower()
