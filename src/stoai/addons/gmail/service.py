"""GoogleMailService — Gmail IMAP/SMTP implementation of MailService.

Uses Gmail IMAP (polling UNSEEN) for receiving and SMTP (TLS on 587) for sending.
All stdlib: imaplib, smtplib, email.  No third-party dependencies.
"""
from __future__ import annotations

import imaplib
import json
import logging
import re
import smtplib
import threading
import time
from datetime import datetime, timezone
from email import policy as email_policy
from email.header import decode_header
from email.mime.text import MIMEText
from email.utils import formataddr, parseaddr
from pathlib import Path
from typing import Callable
from uuid import uuid4

from ...services.mail import MailService

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _decode_header_value(value: str) -> str:
    """Decode an RFC 2047 encoded header value to a plain string."""
    if not value:
        return ""
    parts: list[str] = []
    for fragment, charset in decode_header(value):
        if isinstance(fragment, bytes):
            parts.append(fragment.decode(charset or "utf-8", errors="replace"))
        else:
            parts.append(fragment)
    return "".join(parts)


def _extract_text_body(msg) -> str:
    """Extract plain-text body from an email.Message.

    Walks multipart messages looking for text/plain.
    Falls back to text/html with tag stripping if no plain part found.
    """
    if msg.is_multipart():
        plain_parts: list[str] = []
        html_parts: list[str] = []
        for part in msg.walk():
            ct = part.get_content_type()
            if ct == "text/plain":
                payload = part.get_payload(decode=True)
                if payload:
                    charset = part.get_content_charset() or "utf-8"
                    plain_parts.append(payload.decode(charset, errors="replace"))
            elif ct == "text/html":
                payload = part.get_payload(decode=True)
                if payload:
                    charset = part.get_content_charset() or "utf-8"
                    html_parts.append(payload.decode(charset, errors="replace"))
        if plain_parts:
            return "\n".join(plain_parts)
        if html_parts:
            return _strip_html_tags("\n".join(html_parts))
        return ""
    else:
        payload = msg.get_payload(decode=True)
        if payload is None:
            return ""
        charset = msg.get_content_charset() or "utf-8"
        text = payload.decode(charset, errors="replace")
        if msg.get_content_type() == "text/html":
            return _strip_html_tags(text)
        return text


def _strip_html_tags(html: str) -> str:
    """Naive HTML tag stripper (stdlib only)."""
    return re.sub(r"<[^>]+>", "", html)


# ---------------------------------------------------------------------------
# GoogleMailService
# ---------------------------------------------------------------------------

class GoogleMailService(MailService):
    """Gmail IMAP/SMTP implementation of MailService.

    Usage:
        svc = GoogleMailService(
            gmail_address="agent@gmail.com",
            gmail_password="xxxx xxxx xxxx xxxx",  # App Password
        )
        svc.listen(on_message=lambda msg: print(msg))
        svc.send("user@example.com", {"subject": "hi", "message": "hello"})
    """

    def __init__(
        self,
        gmail_address: str,
        gmail_password: str,
        *,
        allowed_senders: list[str] | None = None,
        poll_interval: int = 30,
        working_dir: Path | str | None = None,
        imap_host: str = "imap.gmail.com",
        imap_port: int = 993,
        smtp_host: str = "smtp.gmail.com",
        smtp_port: int = 587,
    ) -> None:
        self._gmail_address = gmail_address
        self._gmail_password = gmail_password
        self._allowed_senders = allowed_senders
        self._poll_interval = poll_interval
        self._working_dir = Path(working_dir) if working_dir else None
        self._imap_host = imap_host
        self._imap_port = imap_port
        self._smtp_host = smtp_host
        self._smtp_port = smtp_port

        self._poll_thread: threading.Thread | None = None
        self._running = False
        self._processed_uids: set[str] = set()

        # Load persisted state if available
        self._load_state()

    # -- MailService interface -----------------------------------------------

    def send(self, address: str, message: dict) -> str | None:
        """Send an email via Gmail SMTP.  Returns None on success, error string on failure."""
        subject = message.get("subject", "")
        body = message.get("message", "")

        # Reject empty emails
        if not subject and not body:
            return "Cannot send empty email (no subject and no message)"

        try:
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

    def listen(self, on_message: Callable[[dict], None]) -> None:
        """Start IMAP polling in a daemon thread."""
        if self._poll_thread is not None:
            return  # already listening
        self._running = True
        self._poll_thread = threading.Thread(
            target=self._poll_loop,
            args=(on_message,),
            daemon=True,
        )
        self._poll_thread.start()

    def stop(self) -> None:
        """Stop the IMAP poll thread."""
        self._running = False
        if self._poll_thread is not None:
            self._poll_thread.join(timeout=5.0)
            self._poll_thread = None

    @property
    def address(self) -> str | None:
        return self._gmail_address

    # -- Internal: delivery --------------------------------------------------

    def _deliver_email(
        self,
        on_message: Callable[[dict], None],
        from_addr: str,
        subject: str,
        body: str,
    ) -> None:
        """Build payload, persist to disk (if working_dir set), call on_message."""
        msg_id = str(uuid4())
        received_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

        payload: dict = {
            "from": from_addr,
            "to": [self._gmail_address],
            "subject": subject,
            "message": body,
            "_mailbox_id": msg_id,
            "received_at": received_at,
        }

        if self._working_dir is not None:
            msg_dir = self._working_dir / "gmail" / "inbox" / msg_id
            msg_dir.mkdir(parents=True, exist_ok=True)
            (msg_dir / "message.json").write_text(
                json.dumps(payload, indent=2, default=str),
                encoding="utf-8",
            )

        on_message(payload)

    # -- Internal: IMAP polling ----------------------------------------------

    def _poll_loop(self, on_message: Callable[[dict], None]) -> None:
        """Main polling loop — connect, poll UNSEEN, reconnect on error."""
        while self._running:
            imap: imaplib.IMAP4_SSL | None = None
            try:
                imap = imaplib.IMAP4_SSL(self._imap_host, self._imap_port)
                imap.login(self._gmail_address, self._gmail_password)
                imap.select("INBOX")
                logger.info("IMAP connected to %s", self._imap_host)

                while self._running:
                    self._check_new_emails(imap, on_message)
                    # Sleep in small increments for responsive shutdown
                    for _ in range(self._poll_interval):
                        if not self._running:
                            break
                        time.sleep(1)
            except Exception as e:
                logger.warning("IMAP error, reconnecting in 30s: %s", e)
                if imap is not None:
                    try:
                        imap.logout()
                    except Exception:
                        pass
                # Backoff before reconnect
                for _ in range(30):
                    if not self._running:
                        return
                    time.sleep(1)
            finally:
                if imap is not None:
                    try:
                        imap.logout()
                    except Exception:
                        pass

    def _check_new_emails(
        self,
        imap: imaplib.IMAP4_SSL,
        on_message: Callable[[dict], None],
    ) -> None:
        """Search for UNSEEN messages and deliver any new ones."""
        status, data = imap.uid("SEARCH", None, "UNSEEN")  # type: ignore[arg-type]
        if status != "OK" or not data or not data[0]:
            return

        uid_list = data[0].split()
        for uid_bytes in uid_list:
            uid = uid_bytes.decode("ascii") if isinstance(uid_bytes, bytes) else str(uid_bytes)
            if uid in self._processed_uids:
                continue
            try:
                self._fetch_and_deliver(imap, uid, on_message)
            except Exception as e:
                logger.warning("Failed to fetch UID %s: %s", uid, e)
            self._processed_uids.add(uid)
            self._save_state()

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

        # Filter by allowed senders
        if self._allowed_senders is not None:
            if from_addr.lower() not in [s.lower() for s in self._allowed_senders]:
                logger.debug("Skipping email from non-allowed sender: %s", from_addr)
                return

        self._deliver_email(on_message, from_addr, subject, body)

    # -- Internal: state persistence -----------------------------------------

    def _state_path(self) -> Path | None:
        if self._working_dir is None:
            return None
        return self._working_dir / "gmail" / "gmail_state.json"

    def _load_state(self) -> None:
        """Load processed UIDs from disk."""
        path = self._state_path()
        if path is None or not path.is_file():
            return
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
            self._processed_uids = set(data.get("processed_uids", []))
        except (json.JSONDecodeError, OSError) as e:
            logger.warning("Failed to load gmail state: %s", e)

    def _save_state(self) -> None:
        """Persist processed UIDs to disk.  Trims to last 1000."""
        path = self._state_path()
        if path is None:
            return
        path.parent.mkdir(parents=True, exist_ok=True)

        # Sort numerically, trim to last 1000
        uids = sorted(self._processed_uids, key=lambda x: int(x) if x.isdigit() else 0)
        if len(uids) > 1000:
            uids = uids[-1000:]
            self._processed_uids = set(uids)

        path.write_text(
            json.dumps({"processed_uids": uids}, indent=2),
            encoding="utf-8",
        )
