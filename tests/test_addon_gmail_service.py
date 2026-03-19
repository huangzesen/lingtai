from __future__ import annotations

from pathlib import Path
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


def test_deliver_email_with_attachments(tmp_path):
    """Inbound emails with attachments should save files to disk."""
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
        working_dir=tmp_path,
    )
    received = []

    # Simulate an attachment list in the payload
    svc._deliver_email(
        lambda p: received.append(p),
        "user@gmail.com", "hi", "hello",
        attachments=[{"filename": "photo.png", "data": b"\x89PNG\r\n\x1a\n", "content_type": "image/png"}],
    )

    assert len(received) == 1
    payload = received[0]
    assert len(payload["attachments"]) == 1
    att = payload["attachments"][0]
    assert att["filename"] == "photo.png"
    assert att["size"] == 8
    # File should exist on disk
    assert Path(att["path"]).is_file()
    assert Path(att["path"]).read_bytes() == b"\x89PNG\r\n\x1a\n"


def test_deliver_email_without_attachments_has_empty_list(tmp_path):
    """Emails without attachments should have an empty attachments list."""
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
        working_dir=tmp_path,
    )
    received = []
    svc._deliver_email(lambda p: received.append(p), "user@gmail.com", "hi", "hello")

    assert len(received) == 1
    assert received[0]["attachments"] == []


# ---------------------------------------------------------------------------
# _extract_attachments tests
# ---------------------------------------------------------------------------

from email.mime.multipart import MIMEMultipart as StdMIMEMultipart
from email.mime.base import MIMEBase as StdMIMEBase
from email.mime.text import MIMEText as StdMIMEText
from email import encoders as std_encoders

from stoai.addons.gmail.service import _extract_attachments


def test_extract_attachments_from_multipart():
    """_extract_attachments should extract attachment parts from a MIME message."""
    msg = StdMIMEMultipart()
    msg.attach(StdMIMEText("hello", "plain"))
    att = StdMIMEBase("image", "png")
    att.set_payload(b"\x89PNG\r\n\x1a\n")
    std_encoders.encode_base64(att)
    att.add_header("Content-Disposition", "attachment", filename="photo.png")
    msg.attach(att)

    result = _extract_attachments(msg)
    assert len(result) == 1
    assert result[0]["filename"] == "photo.png"
    assert result[0]["content_type"] == "image/png"
    assert result[0]["data"] == b"\x89PNG\r\n\x1a\n"


def test_extract_attachments_inline_with_filename():
    """Inline parts with a filename (common for images) should also be extracted."""
    msg = StdMIMEMultipart()
    msg.attach(StdMIMEText("see image", "plain"))
    att = StdMIMEBase("image", "jpeg")
    att.set_payload(b"\xff\xd8\xff")
    std_encoders.encode_base64(att)
    att.add_header("Content-Disposition", "inline", filename="photo.jpg")
    msg.attach(att)

    result = _extract_attachments(msg)
    assert len(result) == 1
    assert result[0]["filename"] == "photo.jpg"


def test_extract_attachments_skips_plain_text():
    """Plain text/html parts without Content-Disposition should be skipped."""
    msg = StdMIMEMultipart()
    msg.attach(StdMIMEText("hello", "plain"))
    msg.attach(StdMIMEText("<b>hello</b>", "html"))

    result = _extract_attachments(msg)
    assert result == []


def test_extract_attachments_fallback_filename():
    """Attachments without a filename should get a generated one."""
    msg = StdMIMEMultipart()
    att = StdMIMEBase("application", "pdf")
    att.set_payload(b"%PDF-1.4")
    std_encoders.encode_base64(att)
    att.add_header("Content-Disposition", "attachment")  # no filename
    msg.attach(att)

    result = _extract_attachments(msg)
    assert len(result) == 1
    assert result[0]["filename"].startswith("attachment")


def test_deliver_email_sanitizes_attachment_filename(tmp_path):
    """Filenames with path traversal should be sanitized to basename."""
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
        working_dir=tmp_path,
    )
    received = []
    svc._deliver_email(
        lambda p: received.append(p),
        "user@gmail.com", "hi", "hello",
        attachments=[{"filename": "../../evil.sh", "data": b"malicious", "content_type": "application/x-sh"}],
    )
    att = received[0]["attachments"][0]
    assert att["filename"] == "evil.sh"
    # File must be inside the attachments dir, not escaped
    assert "attachments" in att["path"]
    assert Path(att["path"]).parent.name == "attachments"


def test_deliver_email_deduplicates_attachment_filenames(tmp_path):
    """Two attachments with the same name should not overwrite each other."""
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
        working_dir=tmp_path,
    )
    received = []
    svc._deliver_email(
        lambda p: received.append(p),
        "user@gmail.com", "hi", "hello",
        attachments=[
            {"filename": "report.pdf", "data": b"first", "content_type": "application/pdf"},
            {"filename": "report.pdf", "data": b"second", "content_type": "application/pdf"},
        ],
    )
    atts = received[0]["attachments"]
    assert len(atts) == 2
    # Names must differ
    names = [a["filename"] for a in atts]
    assert names[0] != names[1]
    # Both files must exist on disk with distinct content
    assert Path(atts[0]["path"]).read_bytes() == b"first"
    assert Path(atts[1]["path"]).read_bytes() == b"second"


def test_extract_attachments_non_multipart():
    """Non-multipart messages should return empty list."""
    msg = StdMIMEText("just text", "plain")
    result = _extract_attachments(msg)
    assert result == []
