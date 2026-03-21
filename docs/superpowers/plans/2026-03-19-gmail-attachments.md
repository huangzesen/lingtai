# Gmail Attachment Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable the Gmail addon to send and receive file attachments (images and other files) via real Gmail SMTP/IMAP.

**Architecture:** Two changes to `GoogleMailService`: (1) outbound `send()` switches from `MIMEText` to `MIMEMultipart` when attachments are present, reading files from disk and auto-detecting MIME types; (2) inbound `_fetch_and_deliver` extracts non-text parts from multipart emails and saves them to disk alongside the message. `GmailManager` exposes attachments through the tool schema and passes them through on send, and includes attachment metadata in read results.

**Tech Stack:** Python stdlib only — `email.mime.multipart`, `email.mime.base`, `email.mime.text`, `mimetypes`, `email.encoders`.

---

### Task 1: Outbound — `GoogleMailService.send()` attachment support

**Files:**
- Modify: `src/lingtai/addons/gmail/service.py:1-19` (imports), `src/lingtai/addons/gmail/service.py:136-159` (`send` method)
- Test: `tests/test_addon_gmail_service.py`

- [ ] **Step 1: Write failing test for send with attachments**

Add to `tests/test_addon_gmail_service.py`:

```python
def test_send_with_attachments(tmp_path):
    """Attachments should be included in the MIME message."""
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )

    # Create a test image file
    img_path = tmp_path / "photo.png"
    img_path.write_bytes(b"\x89PNG\r\n\x1a\n" + b"\x00" * 100)

    with patch("lingtai.addons.gmail.service.smtplib.SMTP") as MockSMTP:
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
    with patch("lingtai.addons.gmail.service.smtplib.SMTP") as MockSMTP:
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_addon_gmail_service.py -v`
Expected: 3 new tests FAIL (no attachment handling in `send()`)

- [ ] **Step 3: Implement attachment support in `GoogleMailService.send()`**

In `src/lingtai/addons/gmail/service.py`, add imports at the top:

```python
import mimetypes
from email.mime.multipart import MIMEMultipart
from email.mime.base import MIMEBase
from email import encoders
```

Replace the `send` method:

```python
def send(self, address: str, message: dict) -> str | None:
    """Send an email via Gmail SMTP.  Returns None on success, error string on failure."""
    subject = message.get("subject", "")
    body = message.get("message", "")
    attachments = message.get("attachments", [])

    # Reject empty emails
    if not subject and not body and not attachments:
        return "Cannot send empty email (no subject, no message, and no attachments)"

    # Validate attachment paths before building the message
    for filepath in attachments:
        path = Path(filepath)
        if not path.is_file():
            return f"Attachment not found: {filepath}"

    try:
        if attachments:
            mime_msg = MIMEMultipart()
            mime_msg.attach(MIMEText(body, "plain", "utf-8"))
            for filepath in attachments:
                path = Path(filepath)
                content_type, _ = mimetypes.guess_type(str(path))
                if content_type is None:
                    content_type = "application/octet-stream"
                maintype, subtype = content_type.split("/", 1)
                part = MIMEBase(maintype, subtype)
                part.set_payload(path.read_bytes())
                encoders.encode_base64(part)
                part.add_header(
                    "Content-Disposition", "attachment",
                    filename=path.name,
                )
                mime_msg.attach(part)
        else:
            mime_msg = MIMEText(body, "plain", "utf-8")

        mime_msg["From"] = formataddr(("", self._gmail_address))
        mime_msg["To"] = address
        mime_msg["Subject"] = subject

        with smtplib.SMTP(self._smtp_host, self._smtp_port) as server:
            server.starttls()
            server.login(self._gmail_address, self._gmail_password)
            server.send_message(mime_msg)
        return None
    except Exception as e:
        error = f"SMTP send failed: {e}"
        logger.error(error)
        return error
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_addon_gmail_service.py -v`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/addons/gmail/service.py tests/test_addon_gmail_service.py
git commit -m "feat(gmail): add outbound attachment support to GoogleMailService.send()"
```

---

### Task 2: Inbound — extract and save attachments from received emails

**Files:**
- Modify: `src/lingtai/addons/gmail/service.py:46-80` (`_extract_text_body`), `src/lingtai/addons/gmail/service.py:186-214` (`_deliver_email`), `src/lingtai/addons/gmail/service.py:279-305` (`_fetch_and_deliver`)
- Test: `tests/test_addon_gmail_service.py`

- [ ] **Step 1: Write failing test for inbound attachment extraction**

Add to `tests/test_addon_gmail_service.py`:

```python
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_addon_gmail_service.py::test_deliver_email_with_attachments tests/test_addon_gmail_service.py::test_deliver_email_without_attachments_has_empty_list -v`
Expected: FAIL (`_deliver_email` doesn't accept `attachments` param)

- [ ] **Step 3: Implement inbound attachment handling**

Modify `_deliver_email` in `src/lingtai/addons/gmail/service.py`:

```python
def _deliver_email(
    self,
    on_message: Callable[[dict], None],
    from_addr: str,
    subject: str,
    body: str,
    attachments: list[dict] | None = None,
) -> None:
    """Build payload, persist to disk (if working_dir set), call on_message.

    attachments: list of {"filename": str, "data": bytes, "content_type": str}
    """
    msg_id = str(uuid4())
    received_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    attachment_meta: list[dict] = []

    if self._working_dir is not None and attachments:
        att_dir = self._working_dir / "gmail" / "inbox" / msg_id / "attachments"
        att_dir.mkdir(parents=True, exist_ok=True)
        for att in attachments:
            filename = att["filename"]
            data = att["data"]
            filepath = att_dir / filename
            filepath.write_bytes(data)
            attachment_meta.append({
                "filename": filename,
                "path": str(filepath),
                "size": len(data),
                "content_type": att.get("content_type", "application/octet-stream"),
            })

    payload: dict = {
        "from": from_addr,
        "to": [self._gmail_address],
        "subject": subject,
        "message": body,
        "_mailbox_id": msg_id,
        "received_at": received_at,
        "attachments": attachment_meta,
    }

    if self._working_dir is not None:
        msg_dir = self._working_dir / "gmail" / "inbox" / msg_id
        msg_dir.mkdir(parents=True, exist_ok=True)
        (msg_dir / "message.json").write_text(
            json.dumps(payload, indent=2, default=str),
            encoding="utf-8",
        )

    on_message(payload)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_addon_gmail_service.py -v`
Expected: All tests PASS

- [ ] **Step 5: Write failing test for `_extract_attachments` helper**

Add to `tests/test_addon_gmail_service.py`:

```python
from email.mime.multipart import MIMEMultipart
from email.mime.base import MIMEBase
from email.mime.text import MIMEText as StdMIMEText
from email import encoders as std_encoders

from lingtai.addons.gmail.service import _extract_attachments


def test_extract_attachments_from_multipart():
    """_extract_attachments should extract attachment parts from a MIME message."""
    msg = MIMEMultipart()
    msg.attach(StdMIMEText("hello", "plain"))
    att = MIMEBase("image", "png")
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
    msg = MIMEMultipart()
    msg.attach(StdMIMEText("see image", "plain"))
    att = MIMEBase("image", "jpeg")
    att.set_payload(b"\xff\xd8\xff")
    std_encoders.encode_base64(att)
    att.add_header("Content-Disposition", "inline", filename="photo.jpg")
    msg.attach(att)

    result = _extract_attachments(msg)
    assert len(result) == 1
    assert result[0]["filename"] == "photo.jpg"


def test_extract_attachments_skips_plain_text():
    """Plain text/html parts without Content-Disposition should be skipped."""
    msg = MIMEMultipart()
    msg.attach(StdMIMEText("hello", "plain"))
    msg.attach(StdMIMEText("<b>hello</b>", "html"))

    result = _extract_attachments(msg)
    assert result == []


def test_extract_attachments_fallback_filename():
    """Attachments without a filename should get a generated one."""
    msg = MIMEMultipart()
    att = MIMEBase("application", "pdf")
    att.set_payload(b"%PDF-1.4")
    std_encoders.encode_base64(att)
    att.add_header("Content-Disposition", "attachment")  # no filename
    msg.attach(att)

    result = _extract_attachments(msg)
    assert len(result) == 1
    assert result[0]["filename"].startswith("attachment")


def test_extract_attachments_non_multipart():
    """Non-multipart messages should return empty list."""
    msg = StdMIMEText("just text", "plain")
    result = _extract_attachments(msg)
    assert result == []
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `python -m pytest tests/test_addon_gmail_service.py::test_extract_attachments_from_multipart -v`
Expected: FAIL (`_extract_attachments` does not exist yet)

- [ ] **Step 7: Implement `_extract_attachments` helper**

Add to `src/lingtai/addons/gmail/service.py` (after `_strip_html_tags`):

```python
def _extract_attachments(msg) -> list[dict]:
    """Extract file attachments from an email.Message.

    Captures parts with Content-Disposition of 'attachment' or 'inline' (with filename).
    Skips plain text/html body parts that have no Content-Disposition.
    Returns list of {"filename": str, "data": bytes, "content_type": str}.
    """
    attachments = []
    if not msg.is_multipart():
        return attachments
    for part in msg.walk():
        content_disposition = str(part.get("Content-Disposition", ""))
        if not content_disposition or content_disposition == "None":
            continue
        # Accept "attachment" or "inline" with a filename (common for images)
        is_attachment = "attachment" in content_disposition
        is_inline_file = "inline" in content_disposition and part.get_filename()
        if not is_attachment and not is_inline_file:
            continue
        filename = part.get_filename()
        if filename:
            filename = _decode_header_value(filename)
        if not filename:
            ext = mimetypes.guess_extension(part.get_content_type() or "") or ".bin"
            filename = f"attachment{ext}"
        data = part.get_payload(decode=True)
        if data is None:
            continue
        attachments.append({
            "filename": filename,
            "data": data,
            "content_type": part.get_content_type() or "application/octet-stream",
        })
    return attachments
```

- [ ] **Step 8: Run `_extract_attachments` tests to verify they pass**

Run: `python -m pytest tests/test_addon_gmail_service.py -k "extract_attachments" -v`
Expected: All 5 tests PASS

- [ ] **Step 9: Update `_fetch_and_deliver` to use `_extract_attachments`**

Update `_fetch_and_deliver`:

```python
def _fetch_and_deliver(
    self,
    imap: imaplib.IMAP4_SSL,
    uid: str,
    on_message: Callable[[dict], None],
) -> None:
    """Fetch a single email by UID and deliver it."""
    status, data = imap.uid("FETCH", uid, "(RFC822)")
    if status != "OK" or not data or data[0] is None:
        return

    raw_email = data[0][1]  # type: ignore[index]
    import email
    msg = email.message_from_bytes(raw_email, policy=email_policy.default)

    from_raw = msg.get("From", "")
    _, from_addr = parseaddr(from_raw)
    subject = _decode_header_value(msg.get("Subject", ""))
    body = _extract_text_body(msg)
    attachments = _extract_attachments(msg)

    # Filter by allowed senders
    if self._allowed_senders is not None:
        if from_addr.lower() not in [s.lower() for s in self._allowed_senders]:
            logger.debug("Skipping email from non-allowed sender: %s", from_addr)
            return

    self._deliver_email(on_message, from_addr, subject, body, attachments=attachments)
```

- [ ] **Step 10: Run all service tests**

Run: `python -m pytest tests/test_addon_gmail_service.py -v`
Expected: All PASS

- [ ] **Step 11: Commit**

```bash
git add src/lingtai/addons/gmail/service.py tests/test_addon_gmail_service.py
git commit -m "feat(gmail): extract and save inbound email attachments"
```

---

### Task 3: Manager — expose attachments in tool schema and pass through on send/read

**Files:**
- Modify: `src/lingtai/addons/gmail/manager.py:27-101` (SCHEMA/DESCRIPTION), `src/lingtai/addons/gmail/manager.py:313-395` (`_send`), `src/lingtai/addons/gmail/manager.py:415-444` (`_read`)
- Test: `tests/test_addon_gmail_manager.py`

- [ ] **Step 1: Write failing test for send with attachments via manager**

Add to `tests/test_addon_gmail_manager.py`:

```python
def test_send_with_attachments(tmp_path):
    """Manager should pass attachments to the gmail service."""
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    svc.send.return_value = None
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    # Create a test file
    img = tmp_path / "photo.png"
    img.write_bytes(b"\x89PNG")

    result = mgr.handle({
        "action": "send",
        "address": "user@gmail.com",
        "subject": "photo",
        "message": "see attached",
        "attachments": [str(img)],
    })

    assert result["status"] == "delivered"
    call_payload = svc.send.call_args[0][1]
    assert call_payload["attachments"] == [str(img)]


def test_send_with_relative_attachment(tmp_path):
    """Attachments with relative paths should resolve from working dir."""
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    svc.send.return_value = None
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    # Create a test file inside working dir
    img = tmp_path / "photo.png"
    img.write_bytes(b"\x89PNG")

    result = mgr.handle({
        "action": "send",
        "address": "user@gmail.com",
        "subject": "photo",
        "message": "see attached",
        "attachments": ["photo.png"],
    })

    assert result["status"] == "delivered"
    call_payload = svc.send.call_args[0][1]
    # Should be resolved to absolute path
    assert call_payload["attachments"] == [str(img)]
```

- [ ] **Step 2: Write failing test for read with attachment metadata**

Add to `tests/test_addon_gmail_manager.py`:

```python
def test_read_includes_attachment_metadata(tmp_path):
    """Reading an email with attachments should include attachment info."""
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    eid = "email-with-attachment"
    msg_dir = tmp_path / "gmail" / "inbox" / eid
    msg_dir.mkdir(parents=True)
    att_dir = msg_dir / "attachments"
    att_dir.mkdir()
    (att_dir / "photo.png").write_bytes(b"\x89PNG")
    (msg_dir / "message.json").write_text(json.dumps({
        "from": "user@gmail.com", "to": ["agent@gmail.com"],
        "subject": "photo", "message": "see attached",
        "_mailbox_id": eid, "received_at": "2026-03-19T12:00:00Z",
        "attachments": [{"filename": "photo.png", "path": str(att_dir / "photo.png"), "size": 4, "content_type": "image/png"}],
    }))

    result = mgr.handle({"action": "read", "email_id": [eid]})
    assert result["status"] == "ok"
    email = result["emails"][0]
    assert len(email["attachments"]) == 1
    assert email["attachments"][0]["filename"] == "photo.png"
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `python -m pytest tests/test_addon_gmail_manager.py -v`
Expected: New tests FAIL

- [ ] **Step 4: Update SCHEMA to include attachments field**

In `src/lingtai/addons/gmail/manager.py`, add to `SCHEMA["properties"]`:

```python
"attachments": {
    "type": "array",
    "items": {"type": "string"},
    "description": "List of file paths to attach (absolute or relative to working dir). For images and documents.",
},
```

Update `"send"` description in the `action` enum description to mention attachments:

```
"send: send email via Gmail (requires address, message; optional attachments as file paths). "
```

Update `DESCRIPTION` to mention attachments.

- [ ] **Step 5: Update `_send` to pass attachments through**

In `GmailManager._send()`, resolve relative paths and include in payload:

```python
def _send(self, args: dict) -> dict:
    raw_address = args.get("address", "")
    subject = args.get("subject", "")
    message_text = args.get("message", "")
    raw_attachments = args.get("attachments", [])

    # Resolve attachment paths (relative → absolute from working dir)
    attachments = []
    for p in raw_attachments:
        path = Path(p)
        if not path.is_absolute():
            path = Path(self._agent._working_dir) / p
        attachments.append(str(path))

    if isinstance(raw_address, str):
        to_list = [raw_address] if raw_address else []
    else:
        to_list = list(raw_address)

    if not to_list:
        return {"error": "address is required"}

    # Block identical consecutive messages to the same recipient.
    duplicates = [
        addr for addr in to_list
        if (prev := self._last_sent.get(addr)) is not None
        and prev[0] == message_text
        and prev[1] >= self._dup_free_passes
    ]
    if duplicates:
        return {
            "status": "blocked",
            "warning": (
                "Identical message already sent to: "
                f"{', '.join(duplicates)}. "
                "This looks like a repetitive loop — "
                "think twice before sending."
            ),
        }

    sender = self._gmail_service.address or "unknown"

    base_payload = {
        "from": sender,
        "to": to_list,
        "subject": subject,
        "message": message_text,
        "attachments": attachments,
    }

    delivered = []
    refused = []
    errors = []
    for addr in to_list:
        err = self._gmail_service.send(addr, base_payload)
        if err is None:
            delivered.append(addr)
        else:
            refused.append(addr)
            errors.append(err)

    # Save to sent/
    sent_id = str(uuid4())
    sent_dir = self._mailbox_dir / "sent" / sent_id
    sent_dir.mkdir(parents=True, exist_ok=True)
    sent_record = {
        **base_payload,
        "_mailbox_id": sent_id,
        "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    (sent_dir / "message.json").write_text(
        json.dumps(sent_record, indent=2, default=str)
    )

    # Track last sent message per recipient for duplicate detection.
    for addr in to_list:
        prev = self._last_sent.get(addr)
        if prev is not None and prev[0] == message_text:
            self._last_sent[addr] = (message_text, prev[1] + 1)
        else:
            self._last_sent[addr] = (message_text, 1)

    self._agent._log(
        "gmail_sent", to=to_list, subject=subject, message=message_text,
        delivered=delivered, refused=refused,
    )

    if not refused:
        return {"status": "delivered", "to": to_list}
    elif not delivered:
        return {"status": "refused", "error": errors[0], "refused": refused}
    else:
        return {"status": "partial", "delivered": delivered, "refused": refused}
```

- [ ] **Step 6: Update `_read` to include attachment metadata**

In `GmailManager._read()`, replace the existing `results.append({...})` block (lines 431-439 of `manager.py`) with:

```python
results.append({
    "id": eid,
    "from": data.get("from", ""),
    "to": data.get("to", []),
    "subject": data.get("subject", "(no subject)"),
    "message": data.get("message", ""),
    "time": data.get("received_at") or data.get("sent_at") or data.get("time") or "",
    "folder": data.get("_folder", ""),
    "attachments": data.get("attachments", []),
})
```

The only change is the added `"attachments"` key at the end.

- [ ] **Step 7: Run all tests**

Run: `python -m pytest tests/test_addon_gmail_manager.py tests/test_addon_gmail_service.py -v`
Expected: All PASS

- [ ] **Step 8: Smoke test**

Run: `python -c "import lingtai"`
Expected: Clean import, no errors

- [ ] **Step 9: Commit**

```bash
git add src/lingtai/addons/gmail/manager.py tests/test_addon_gmail_manager.py
git commit -m "feat(gmail): expose attachments in tool schema and read results"
```
