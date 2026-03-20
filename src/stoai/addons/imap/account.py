"""IMAPAccount — single IMAP connection + SMTP credentials.

Protocol layer for one email account. Handles IMAP/SMTP directly via stdlib.
Used by IMAPMailService (multi-account coordinator) and IMAPMailManager (tool handler).

email_id format: {account}:{folder}:{uid}
"""
from __future__ import annotations

import email as email_mod
import imaplib
import json
import logging
import mimetypes
import re
import smtplib
import threading
import time
from datetime import datetime
from email import encoders, policy as email_policy
from email.header import decode_header
from email.mime.base import MIMEBase
from email.mime.multipart import MIMEMultipart
from email.mime.text import MIMEText
from email.utils import formataddr, formatdate, make_msgid, parseaddr
from pathlib import Path
from typing import Callable

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# RFC 6154 special-use attribute mapping
# ---------------------------------------------------------------------------

_SPECIAL_USE_ROLES: dict[str, str] = {
    "\\Trash": "trash",
    "\\Sent": "sent",
    "\\Archive": "archive",
    "\\Drafts": "drafts",
    "\\Junk": "junk",
}

_NAME_HEURISTICS: dict[str, str] = {
    "trash": "trash",
    "deleted items": "trash",
    "[gmail]/trash": "trash",
    "sent": "sent",
    "sent items": "sent",
    "[gmail]/sent mail": "sent",
    "archive": "archive",
    "all mail": "archive",
    "[gmail]/all mail": "archive",
    "drafts": "drafts",
    "[gmail]/drafts": "drafts",
    "junk": "junk",
    "spam": "junk",
    "[gmail]/spam": "junk",
}


# ---------------------------------------------------------------------------
# Email parsing helpers
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


def _extract_text_body(msg: email_mod.message.Message) -> str:
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


def _extract_attachments(msg: email_mod.message.Message) -> list[dict]:
    """Extract file attachments from an email.Message.

    Captures parts with Content-Disposition of 'attachment' or 'inline' (with filename).
    Skips plain text/html body parts that have no Content-Disposition.
    Returns list of {"filename": str, "data": bytes, "content_type": str}.
    """
    attachments: list[dict] = []
    if not msg.is_multipart():
        return attachments
    for part in msg.walk():
        content_disposition = str(part.get("Content-Disposition", ""))
        if not content_disposition or content_disposition == "None":
            continue
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


def _format_imap_date(dt: datetime) -> str:
    """Format a datetime to IMAP date string (DD-Mon-YYYY)."""
    return dt.strftime("%d-%b-%Y")


# ---------------------------------------------------------------------------
# IMAPAccount
# ---------------------------------------------------------------------------

class IMAPAccount:
    """Single IMAP connection + SMTP credentials for one email account.

    Provides folder discovery, header/full fetch, search, flag operations,
    folder operations, SMTP send, IDLE with poll fallback, and state persistence.
    """

    def __init__(
        self,
        email_address: str,
        email_password: str,
        *,
        imap_host: str = "imap.gmail.com",
        imap_port: int = 993,
        smtp_host: str = "smtp.gmail.com",
        smtp_port: int = 587,
        working_dir: Path | str | None = None,
    ) -> None:
        self._email_address = email_address
        self._email_password = email_password
        self._imap_host = imap_host
        self._imap_port = imap_port
        self._smtp_host = smtp_host
        self._smtp_port = smtp_port
        self._working_dir = Path(working_dir) if working_dir else None

        # IMAP connection
        self._imap: imaplib.IMAP4_SSL | None = None
        self._lock = threading.Lock()

        # Capabilities parsed from server
        self._capabilities: set[str] = set()
        self._has_idle = False
        self._has_move = False
        self._has_uidplus = False

        # Folder discovery
        self._folders: dict[str, str] = {}  # name -> role
        self._folder_by_role: dict[str, str] = {}  # role -> name

        # IDLE interlock (erratum #4)
        self._in_idle = False
        self._idle_event = threading.Event()  # signal IDLE thread to wake
        self._idle_done = threading.Event()   # IDLE thread signals it exited

        # IDLE tag counter (erratum #3)
        self._tag_counter = 0

        # State persistence
        self._processed_uids: set[str] = set()

        # Load persisted state
        self._load_state()

    # -- Properties ----------------------------------------------------------

    @property
    def address(self) -> str:
        return self._email_address

    @property
    def connected(self) -> bool:
        return self._imap is not None

    @property
    def capabilities(self) -> set[str]:
        return self._capabilities

    @property
    def has_idle(self) -> bool:
        return self._has_idle

    @property
    def has_move(self) -> bool:
        return self._has_move

    @property
    def has_uidplus(self) -> bool:
        return self._has_uidplus

    @property
    def folders(self) -> dict[str, str]:
        """Map of folder name -> role. Roles: trash, sent, archive, drafts, junk, or ''."""
        return dict(self._folders)

    # -- Connection ----------------------------------------------------------

    def connect(self) -> None:
        """Connect to the IMAP server and parse capabilities."""
        self._imap = imaplib.IMAP4_SSL(self._imap_host, self._imap_port)
        resp = self._imap.login(self._email_address, self._email_password)
        # Parse capabilities from login response and CAPABILITY command
        self._fetch_capabilities()
        self._discover_folders()
        self._save_state()
        logger.info("IMAP connected: %s (%s)", self._email_address, self._imap_host)

    def disconnect(self) -> None:
        """Disconnect from the IMAP server."""
        if self._imap is not None:
            try:
                self._imap.logout()
            except Exception:
                pass
            self._imap = None

    def _ensure_connected(self) -> imaplib.IMAP4_SSL:
        """Return the IMAP connection, raising if not connected."""
        if self._imap is None:
            raise RuntimeError("Not connected — call connect() first")
        return self._imap

    def _break_idle_if_needed(self) -> None:
        """If IDLE is active, break it and wait for the IDLE thread to finish.

        Erratum #4: All public IMAP methods must check _in_idle and if True,
        signal the IDLE thread to wake, then wait for _idle_done.
        """
        if self._in_idle:
            self._idle_event.set()
            self._idle_done.wait(timeout=10.0)

    # -- Capability parsing --------------------------------------------------

    def _fetch_capabilities(self) -> None:
        """Fetch and parse server CAPABILITY response."""
        imap = self._ensure_connected()
        status, data = imap.capability()
        if status == "OK" and data and data[0]:
            raw = data[0].decode("ascii") if isinstance(data[0], bytes) else str(data[0])
            self._parse_capabilities(raw)

    def _parse_capabilities(self, raw: str) -> None:
        """Parse a CAPABILITY response string into the capabilities set."""
        caps = set()
        for token in raw.upper().split():
            caps.add(token)
        self._capabilities = caps
        self._has_idle = "IDLE" in caps
        self._has_move = "MOVE" in caps
        self._has_uidplus = "UIDPLUS" in caps

    # -- Folder discovery ----------------------------------------------------

    def _discover_folders(self) -> None:
        """Discover folders via LIST and map them to roles using RFC 6154 + heuristics."""
        imap = self._ensure_connected()
        status, data = imap.list()
        if status != "OK" or not data:
            return

        folders: dict[str, str] = {}
        folder_by_role: dict[str, str] = {}

        for item in data:
            if item is None:
                continue
            raw = item.decode("utf-8") if isinstance(item, bytes) else str(item)
            name, attrs = self._parse_list_entry(raw)
            if name is None:
                continue

            # Try RFC 6154 special-use attributes first
            role = ""
            for attr in attrs:
                if attr in _SPECIAL_USE_ROLES:
                    role = _SPECIAL_USE_ROLES[attr]
                    break

            # Fall back to name heuristics
            if not role:
                name_lower = name.lower()
                role = _NAME_HEURISTICS.get(name_lower, "")

            folders[name] = role
            if role and role not in folder_by_role:
                folder_by_role[role] = name

        self._folders = folders
        self._folder_by_role = folder_by_role

    @staticmethod
    def _parse_list_entry(raw: str) -> tuple[str | None, list[str]]:
        """Parse an IMAP LIST response line into (folder_name, [attributes]).

        Example input: '(\\HasNoChildren \\Sent) "/" "Sent"'
        Returns: ("Sent", ["\\Sent"])
        """
        # Match: (attrs) "delimiter" "name"  or  (attrs) "delimiter" name
        m = re.match(r'\(([^)]*)\)\s+"([^"]*)"\s+(.*)', raw)
        if not m:
            return None, []

        attrs_raw = m.group(1)
        # group(2) is delimiter
        name_raw = m.group(3).strip()

        # Unquote folder name
        if name_raw.startswith('"') and name_raw.endswith('"'):
            name_raw = name_raw[1:-1]

        attrs = [a.strip() for a in attrs_raw.split() if a.strip()]

        return name_raw, attrs

    def get_folder_by_role(self, role: str) -> str | None:
        """Get folder name for a given role (trash, sent, archive, drafts, junk)."""
        return self._folder_by_role.get(role)

    # -- Header fetch (erratum #1: NO ENVELOPE, use BODY.PEEK[HEADER.FIELDS]) --

    def fetch_headers_by_uids(
        self, folder: str, uids: list[str],
    ) -> list[dict]:
        """Fetch headers for specific UIDs in a folder.

        Returns list of dicts with: uid, from, to, subject, date, flags, email_id.
        """
        if not uids:
            return []

        with self._lock:
            self._break_idle_if_needed()
            imap = self._ensure_connected()
            imap.select(folder, readonly=True)

            uid_set = ",".join(uids)
            status, data = imap.uid(
                "FETCH", uid_set,
                "(FLAGS BODY.PEEK[HEADER.FIELDS (FROM TO SUBJECT DATE)])",
            )
            if status != "OK" or not data:
                return []

            return self._parse_fetch_response(data, folder)

    def fetch_envelopes(self, folder: str, n: int = 20) -> list[dict]:
        """Fetch N most recent message headers from a folder.

        Internally selects the last N UIDs via SEARCH ALL, then calls
        fetch_headers_by_uids.
        """
        with self._lock:
            self._break_idle_if_needed()
            imap = self._ensure_connected()
            imap.select(folder, readonly=True)

            # Get all UIDs
            status, data = imap.uid("SEARCH", None, "ALL")
            if status != "OK" or not data or not data[0]:
                return []

            all_uids = data[0].split()
            # Take last N (most recent)
            recent_uids = all_uids[-n:] if n > 0 else all_uids
            uid_list = [u.decode("ascii") if isinstance(u, bytes) else str(u) for u in recent_uids]

        # Release lock, call fetch_headers_by_uids which acquires it
        return self.fetch_headers_by_uids(folder, uid_list)

    def _parse_fetch_response(
        self, data: list, folder: str,
    ) -> list[dict]:
        """Parse FETCH response data into header dicts."""
        results: list[dict] = []
        i = 0
        while i < len(data):
            item = data[i]
            if isinstance(item, tuple) and len(item) >= 2:
                meta_line = item[0]
                header_bytes = item[1]

                if isinstance(meta_line, bytes):
                    meta_line = meta_line.decode("ascii", errors="replace")

                # Extract UID from meta line
                uid_match = re.search(r"UID\s+(\d+)", meta_line, re.IGNORECASE)
                uid = uid_match.group(1) if uid_match else ""

                # Extract FLAGS from meta line
                flags = self._parse_flags_from_meta(meta_line)

                # Parse headers
                if isinstance(header_bytes, bytes):
                    msg = email_mod.message_from_bytes(header_bytes, policy=email_policy.default)
                else:
                    msg = email_mod.message_from_string(
                        header_bytes if isinstance(header_bytes, str) else "",
                        policy=email_policy.default,
                    )

                from_raw = msg.get("From", "")
                to_raw = msg.get("To", "")
                subject_raw = msg.get("Subject", "")
                date_raw = msg.get("Date", "")

                results.append({
                    "uid": uid,
                    "from": _decode_header_value(from_raw),
                    "to": _decode_header_value(to_raw),
                    "subject": _decode_header_value(subject_raw),
                    "date": date_raw,
                    "flags": flags,
                    "email_id": f"{self._email_address}:{folder}:{uid}",
                })
            i += 1
        return results

    @staticmethod
    def _parse_flags_from_meta(meta_line: str) -> list[str]:
        """Extract FLAGS from a FETCH response meta line."""
        m = re.search(r"FLAGS\s*\(([^)]*)\)", meta_line, re.IGNORECASE)
        if not m:
            return []
        return [f.strip() for f in m.group(1).split() if f.strip()]

    @staticmethod
    def _parse_flags(flags_bytes: bytes) -> list[str]:
        """Parse a FLAGS response (bytes) into a list of flag strings."""
        if not flags_bytes:
            return []
        raw = flags_bytes.decode("ascii", errors="replace") if isinstance(flags_bytes, bytes) else str(flags_bytes)
        m = re.search(r"\(([^)]*)\)", raw)
        if not m:
            return []
        inner = m.group(1).strip()
        if not inner:
            return []
        return [f.strip() for f in inner.split() if f.strip()]

    # -- Full message fetch --------------------------------------------------

    def fetch_full(self, folder: str, uid: str) -> dict | None:
        """Fetch the full message (body + attachments) for a single UID.

        Returns dict with: uid, from, to, subject, date, body, attachments, flags, email_id.
        """
        with self._lock:
            self._break_idle_if_needed()
            imap = self._ensure_connected()
            imap.select(folder, readonly=True)

            status, data = imap.uid("FETCH", uid, "(FLAGS RFC822)")
            if status != "OK" or not data or data[0] is None:
                return None

        # Parse outside lock
        raw_email = data[0][1]  # type: ignore[index]
        meta_line = data[0][0]
        if isinstance(meta_line, bytes):
            meta_line = meta_line.decode("ascii", errors="replace")
        flags = self._parse_flags_from_meta(meta_line)

        msg = email_mod.message_from_bytes(raw_email, policy=email_policy.default)

        from_raw = msg.get("From", "")
        _, from_addr = parseaddr(from_raw)
        to_raw = msg.get("To", "")
        subject = _decode_header_value(msg.get("Subject", ""))
        date_raw = msg.get("Date", "")
        message_id = msg.get("Message-ID", "")
        in_reply_to = msg.get("In-Reply-To", "")
        references = msg.get("References", "")

        body = _extract_text_body(msg)
        attachments = _extract_attachments(msg)

        # Strip binary data from attachment info for the return dict
        attachment_info = []
        for att in attachments:
            attachment_info.append({
                "filename": att["filename"],
                "content_type": att["content_type"],
                "size": len(att["data"]),
            })

        return {
            "uid": uid,
            "from": _decode_header_value(from_raw),
            "from_address": from_addr,
            "to": _decode_header_value(to_raw),
            "subject": subject,
            "date": date_raw,
            "body": body,
            "attachments": attachment_info,
            "attachments_raw": attachments,
            "flags": flags,
            "message_id": message_id,
            "in_reply_to": in_reply_to,
            "references": references,
            "email_id": f"{self._email_address}:{folder}:{uid}",
        }

    # -- Server-side IMAP SEARCH ---------------------------------------------

    def search(self, folder: str, query: str) -> list[str]:
        """Server-side IMAP SEARCH. Returns list of UIDs matching the query.

        Query syntax:
            from:addr          IMAP FROM "addr"
            subject:text       IMAP SUBJECT "text"
            since:YYYY-MM-DD   IMAP SINCE DD-Mon-YYYY
            before:YYYY-MM-DD  IMAP BEFORE DD-Mon-YYYY
            flagged            IMAP FLAGGED
            unseen             IMAP UNSEEN

        Multiple terms are AND-ed together.
        Quoted values: from:"John Doe" subject:"hello world"
        """
        with self._lock:
            self._break_idle_if_needed()
            imap = self._ensure_connected()
            imap.select(folder, readonly=True)

            search_criteria = self._build_search_query(query)
            status, data = imap.uid("SEARCH", None, search_criteria)
            if status != "OK" or not data or not data[0]:
                return []

            return [
                u.decode("ascii") if isinstance(u, bytes) else str(u)
                for u in data[0].split()
            ]

    @staticmethod
    def _build_search_query(query: str) -> str:
        """Build IMAP SEARCH criteria from a query string.

        Supported operators:
            from:addr, subject:text, since:YYYY-MM-DD, before:YYYY-MM-DD,
            flagged, unseen

        Multiple terms are AND-ed. Unknown tokens become a SUBJECT search as fallback.
        """
        parts: list[str] = []

        # Tokenize — respecting quoted values
        # Matches: key:"quoted value" or key:value or standalone_keyword
        tokens = re.findall(r'(\w+:"[^"]*"|\w+:\S+|\w+)', query)

        for token in tokens:
            if ":" in token and not token.startswith('"'):
                key, _, value = token.partition(":")
                # Strip quotes from value
                value = value.strip('"')
                key_lower = key.lower()

                if key_lower == "from":
                    parts.append(f'FROM "{value}"')
                elif key_lower == "subject":
                    parts.append(f'SUBJECT "{value}"')
                elif key_lower == "since":
                    try:
                        dt = datetime.strptime(value, "%Y-%m-%d")
                        parts.append(f'SINCE {_format_imap_date(dt)}')
                    except ValueError:
                        parts.append(f'SUBJECT "{value}"')
                elif key_lower == "before":
                    try:
                        dt = datetime.strptime(value, "%Y-%m-%d")
                        parts.append(f'BEFORE {_format_imap_date(dt)}')
                    except ValueError:
                        parts.append(f'SUBJECT "{value}"')
                else:
                    # Unknown key — treat as subject search
                    parts.append(f'SUBJECT "{token}"')
            else:
                # Standalone keyword
                keyword = token.lower()
                if keyword == "flagged":
                    parts.append("FLAGGED")
                elif keyword == "unseen":
                    parts.append("UNSEEN")
                else:
                    # Fallback: treat as subject search
                    parts.append(f'SUBJECT "{token}"')

        if not parts:
            return "ALL"

        # IMAP SEARCH: multiple criteria are implicitly AND-ed
        return " ".join(parts)

    # -- Flag STORE operations -----------------------------------------------

    def store_flags(
        self, folder: str, uid: str, flags: list[str], action: str = "+FLAGS",
    ) -> bool:
        """Set/add/remove flags on a message.

        action: "+FLAGS" to add, "-FLAGS" to remove, "FLAGS" to replace.
        """
        with self._lock:
            self._break_idle_if_needed()
            imap = self._ensure_connected()
            imap.select(folder)

            flag_str = " ".join(flags)
            status, _ = imap.uid("STORE", uid, action, f"({flag_str})")
            return status == "OK"

    def mark_seen(self, folder: str, uid: str) -> bool:
        """Mark a message as seen."""
        return self.store_flags(folder, uid, ["\\Seen"])

    def mark_unseen(self, folder: str, uid: str) -> bool:
        """Mark a message as unseen."""
        return self.store_flags(folder, uid, ["\\Seen"], action="-FLAGS")

    def mark_flagged(self, folder: str, uid: str) -> bool:
        """Flag a message (star)."""
        return self.store_flags(folder, uid, ["\\Flagged"])

    # -- Folder operations ---------------------------------------------------

    def list_folders(self) -> dict[str, str]:
        """Return discovered folders as {name: role}."""
        return dict(self._folders)

    def move_message(self, folder: str, uid: str, dest_folder: str) -> bool:
        """Move a message to another folder. Uses MOVE if supported, else COPY+DELETE."""
        with self._lock:
            self._break_idle_if_needed()
            imap = self._ensure_connected()
            imap.select(folder)

            if self._has_move:
                status, _ = imap.uid("MOVE", uid, dest_folder)
                return status == "OK"
            else:
                # COPY then mark deleted and expunge
                status, _ = imap.uid("COPY", uid, dest_folder)
                if status != "OK":
                    return False
                imap.uid("STORE", uid, "+FLAGS", "(\\Deleted)")
                imap.expunge()
                return True

    def delete_message(self, folder: str, uid: str) -> bool:
        """Delete a message — move to Trash if possible, else flag+expunge."""
        trash = self.get_folder_by_role("trash")
        if trash and folder != trash:
            return self.move_message(folder, uid, trash)
        else:
            # Already in trash or no trash folder — flag deleted + expunge
            with self._lock:
                self._break_idle_if_needed()
                imap = self._ensure_connected()
                imap.select(folder)
                imap.uid("STORE", uid, "+FLAGS", "(\\Deleted)")
                imap.expunge()
                return True

    # -- SMTP send -----------------------------------------------------------

    def send_email(
        self,
        to: list[str],
        subject: str,
        body: str,
        *,
        cc: list[str] | None = None,
        bcc: list[str] | None = None,
        attachments: list[str] | None = None,
        in_reply_to: str | None = None,
        references: str | None = None,
    ) -> str | None:
        """Send an email via SMTP.

        Returns None on success, error string on failure.

        BCC addresses are NOT included in headers — only in RCPT TO.
        Reply threading: In-Reply-To and References headers set when provided.
        """
        # Reject empty
        if not subject and not body and not attachments:
            return "Cannot send empty email (no subject, no body, and no attachments)"

        # Validate attachment paths
        if attachments:
            for filepath in attachments:
                if not Path(filepath).is_file():
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
                        "Content-Disposition", "attachment", filename=path.name,
                    )
                    mime_msg.attach(part)
            else:
                mime_msg = MIMEText(body, "plain", "utf-8")

            mime_msg["From"] = formataddr(("", self._email_address))
            mime_msg["To"] = ", ".join(to)
            mime_msg["Subject"] = subject
            mime_msg["Date"] = formatdate(localtime=True)
            mime_msg["Message-ID"] = make_msgid()

            # CC in headers (but not BCC — erratum)
            if cc:
                mime_msg["CC"] = ", ".join(cc)

            # Reply threading headers
            if in_reply_to:
                mime_msg["In-Reply-To"] = in_reply_to
            if references:
                mime_msg["References"] = references

            # All recipients for RCPT TO (To + CC + BCC)
            all_recipients = list(to)
            if cc:
                all_recipients.extend(cc)
            if bcc:
                all_recipients.extend(bcc)

            with smtplib.SMTP(self._smtp_host, self._smtp_port) as server:
                server.starttls()
                server.login(self._email_address, self._email_password)
                server.sendmail(self._email_address, all_recipients, mime_msg.as_string())
            return None

        except Exception as e:
            error = f"SMTP send failed: {e}"
            logger.error(error)
            return error

    # -- IDLE with poll fallback (errata #3, #4) -----------------------------

    def idle(
        self,
        folder: str,
        on_message: Callable[[list[dict]], None],
        *,
        poll_interval: int = 30,
        stop_event: threading.Event | None = None,
    ) -> None:
        """Listen for new messages via IDLE (with poll fallback).

        Blocks until stop_event is set.
        on_message receives a list of header dicts for new messages.

        Erratum #3: Uses manual _tag_counter, NOT imaplib._new_tag().
        Erratum #4: Sets _in_idle while waiting; other methods break IDLE via _idle_event.
        Erratum #5: on_message called OUTSIDE the lock.
        """
        if stop_event is None:
            stop_event = threading.Event()

        while not stop_event.is_set():
            try:
                if self._has_idle:
                    self._idle_cycle(folder, on_message, poll_interval, stop_event)
                else:
                    self._poll_cycle(folder, on_message, poll_interval, stop_event)
            except (imaplib.IMAP4.error, OSError) as e:
                logger.warning("IDLE/poll error, reconnecting: %s", e)
                try:
                    self.disconnect()
                except Exception:
                    pass
                # Back off before reconnect
                if stop_event.wait(10.0):
                    return
                try:
                    self.connect()
                except Exception as ce:
                    logger.warning("Reconnect failed: %s", ce)

    def _idle_cycle(
        self,
        folder: str,
        on_message: Callable[[list[dict]], None],
        timeout: int,
        stop_event: threading.Event,
    ) -> None:
        """One IDLE cycle: send IDLE, wait, send DONE, check for new mail."""
        with self._lock:
            imap = self._ensure_connected()
            imap.select(folder)

            # Send IDLE command using raw socket (erratum #3)
            self._tag_counter += 1
            tag = f"A{self._tag_counter:04d}"
            imap.send(f"{tag} IDLE\r\n".encode("ascii"))

            # Read continuation response (+)
            response = imap.readline()
            if not response.startswith(b"+"):
                logger.warning("IDLE not accepted: %s", response)
                return

            # Mark that we're in IDLE (erratum #4)
            self._in_idle = True
            self._idle_event.clear()
            self._idle_done.clear()

        # Wait outside the lock for either:
        # - timeout (re-IDLE)
        # - stop_event (shutdown)
        # - _idle_event (another method needs the connection)
        # - server notification (EXISTS etc.)
        try:
            # Use _idle_event to detect interrupts, check periodically for server data
            deadline = time.monotonic() + timeout
            got_data = False
            while time.monotonic() < deadline:
                if stop_event.is_set() or self._idle_event.is_set():
                    break
                # Check if server sent anything (non-blocking peek)
                with self._lock:
                    imap = self._ensure_connected()
                    # Use socket timeout to peek for data
                    old_timeout = imap.socket().gettimeout()
                    imap.socket().settimeout(1.0)
                    try:
                        data = imap.readline()
                        if data:
                            got_data = True
                            break
                    except (TimeoutError, OSError):
                        pass
                    finally:
                        imap.socket().settimeout(old_timeout)
        finally:
            # Send DONE and complete IDLE (always, even on error)
            with self._lock:
                try:
                    imap = self._ensure_connected()
                    imap.send(b"DONE\r\n")
                    # Read the tagged response
                    while True:
                        line = imap.readline()
                        if not line:
                            break
                        decoded = line.decode("ascii", errors="replace").strip()
                        if decoded.startswith(tag):
                            break
                except Exception as e:
                    logger.debug("IDLE DONE error: %s", e)
                finally:
                    self._in_idle = False
                    self._idle_done.set()

        # Check for new mail OUTSIDE the lock (erratum #5)
        if not stop_event.is_set():
            self._check_new_mail(folder, on_message)

    def _poll_cycle(
        self,
        folder: str,
        on_message: Callable[[list[dict]], None],
        interval: int,
        stop_event: threading.Event,
    ) -> None:
        """One poll cycle: NOOP, check new mail, sleep."""
        with self._lock:
            self._break_idle_if_needed()
            imap = self._ensure_connected()
            imap.select(folder)
            imap.noop()

        # Check outside lock (erratum #5)
        self._check_new_mail(folder, on_message)

        # Sleep in small increments for responsive shutdown
        for _ in range(interval):
            if stop_event.is_set():
                return
            time.sleep(1)

    def _check_new_mail(
        self,
        folder: str,
        on_message: Callable[[list[dict]], None],
    ) -> None:
        """Check for unseen messages and deliver new ones.

        Erratum #5: Collects payloads while holding the lock, delivers OUTSIDE.
        """
        new_headers: list[dict] = []

        with self._lock:
            self._break_idle_if_needed()
            imap = self._ensure_connected()
            imap.select(folder, readonly=True)

            status, data = imap.uid("SEARCH", None, "UNSEEN")
            if status != "OK" or not data or not data[0]:
                return

            uid_list = data[0].split()
            new_uids = []
            for uid_bytes in uid_list:
                uid = uid_bytes.decode("ascii") if isinstance(uid_bytes, bytes) else str(uid_bytes)
                compound_id = f"{self._email_address}:{folder}:{uid}"
                if compound_id not in self._processed_uids:
                    new_uids.append(uid)

            if not new_uids:
                return

            # Fetch headers for new UIDs
            uid_set = ",".join(new_uids)
            status, fetch_data = imap.uid(
                "FETCH", uid_set,
                "(FLAGS BODY.PEEK[HEADER.FIELDS (FROM TO SUBJECT DATE)])",
            )
            if status == "OK" and fetch_data:
                new_headers = self._parse_fetch_response(fetch_data, folder)

            # Mark as processed
            for uid in new_uids:
                compound_id = f"{self._email_address}:{folder}:{uid}"
                self._processed_uids.add(compound_id)
            self._save_state()

        # Deliver OUTSIDE the lock (erratum #5)
        if new_headers:
            on_message(new_headers)

    # -- State persistence ---------------------------------------------------

    def _state_path(self) -> Path | None:
        if self._working_dir is None:
            return None
        return self._working_dir / "imap" / self._email_address / "state.json"

    def _load_state(self) -> None:
        """Load persisted state from disk."""
        path = self._state_path()
        if path is None or not path.is_file():
            return
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
            self._processed_uids = set(data.get("processed_uids", []))
            # Restore cached folder mapping
            if "folders" in data:
                self._folders = data["folders"]
                self._folder_by_role = {}
                for name, role in self._folders.items():
                    if role and role not in self._folder_by_role:
                        self._folder_by_role[role] = name
            if "capabilities" in data:
                self._parse_capabilities(data["capabilities"])
        except (json.JSONDecodeError, OSError) as e:
            logger.warning("Failed to load IMAP state for %s: %s", self._email_address, e)

    def _save_state(self) -> None:
        """Persist state to disk. Trims processed_uids to last 2000."""
        path = self._state_path()
        if path is None:
            return
        path.parent.mkdir(parents=True, exist_ok=True)

        uids = sorted(self._processed_uids)
        if len(uids) > 2000:
            uids = uids[-2000:]
            self._processed_uids = set(uids)

        caps_str = " ".join(sorted(self._capabilities))

        state = {
            "processed_uids": uids,
            "folders": self._folders,
            "capabilities": caps_str,
        }
        path.write_text(
            json.dumps(state, indent=2), encoding="utf-8",
        )
