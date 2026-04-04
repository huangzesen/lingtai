"""IMAPAccount — single IMAP account with dual connections.

Protocol layer for one email account. Handles IMAP/SMTP directly via stdlib.
Used by IMAPMailService (multi-account coordinator) and IMAPMailManager (tool handler).

Two IMAP connections:
  _imap      — on-demand, for tool calls (fetch/search/flags/move/delete).
               Protected by _lock.  Lazy-connected on first use.
  _idle_imap — dedicated to the background IDLE listener thread.
               Owned exclusively by that thread, no lock needed.

email_id format: {account}:{folder}:{uid}
"""
from __future__ import annotations

import email as email_mod
import imaplib
import json
import logging
import mimetypes
import re
import select as _select_mod
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
    """Single IMAP account with dual connections.

    Two IMAP connections avoid the complexity of sharing one socket between
    a background IDLE listener and on-demand tool calls:

    - ``_imap``: lazy-connected on first tool call, protected by ``_lock``.
    - ``_idle_imap``: owned exclusively by the background listener thread.
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
        allowed_senders: list[str] | None = None,
        poll_interval: int = 30,
    ) -> None:
        self._email_address = email_address
        self._email_password = email_password
        self._imap_host = imap_host
        self._imap_port = imap_port
        self._smtp_host = smtp_host
        self._smtp_port = smtp_port
        self._working_dir = Path(working_dir) if working_dir else None
        self._allowed_senders = allowed_senders
        self._poll_interval = poll_interval

        # On-demand IMAP connection (tool calls)
        self._imap: imaplib.IMAP4_SSL | None = None
        self._lock = threading.Lock()

        # Dedicated IDLE connection (background listener) — no lock needed
        self._idle_imap: imaplib.IMAP4_SSL | None = None

        # Capabilities parsed from server
        self._capabilities: set[str] = set()
        self._has_idle = False
        self._has_move = False
        self._has_uidplus = False

        # Folder discovery
        self._folders: dict[str, str | None] = {}  # name -> role (None = no role)
        self._folder_by_role: dict[str, str] = {}  # role -> name

        # State persistence — per-folder set of int UIDs
        self._processed_uids: dict[str, set[int]] = {}

        # Reconnect backoff steps (seconds)
        self._backoff_steps = [1, 2, 5, 10, 60]
        self._backoff_index = 0

        # Background listening thread
        self._bg_thread: threading.Thread | None = None
        self._stop_event: threading.Event | None = None

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
    def folders(self) -> dict[str, str | None]:
        """Map of folder name -> role. Roles: trash, sent, archive, drafts, junk, or None."""
        return dict(self._folders)

    # -- Connection ----------------------------------------------------------

    def connect(self) -> None:
        """Connect the on-demand IMAP connection and parse capabilities."""
        self._imap = imaplib.IMAP4_SSL(self._imap_host, self._imap_port)
        self._imap.login(self._email_address, self._email_password)
        self._fetch_capabilities()
        self._discover_folders()
        self._save_state()
        logger.info("IMAP connected: %s (%s)", self._email_address, self._imap_host)

    def disconnect(self) -> None:
        """Disconnect the on-demand IMAP connection."""
        if self._imap is not None:
            try:
                self._imap.logout()
            except Exception:
                pass
            self._imap = None

    def _ensure_connected(self) -> imaplib.IMAP4_SSL:
        """Return the on-demand IMAP connection, connecting if needed."""
        if self._imap is None:
            self.connect()
        return self._imap  # type: ignore[return-value]

    def _connect_idle(self) -> imaplib.IMAP4_SSL:
        """Connect (or reconnect) the dedicated IDLE connection."""
        if self._idle_imap is not None:
            try:
                self._idle_imap.logout()
            except Exception:
                pass
        self._idle_imap = imaplib.IMAP4_SSL(self._imap_host, self._imap_port)
        self._idle_imap.login(self._email_address, self._email_password)
        logger.info("IMAP IDLE connected: %s", self._email_address)
        return self._idle_imap

    def _disconnect_idle(self) -> None:
        """Disconnect the dedicated IDLE connection."""
        if self._idle_imap is not None:
            try:
                self._idle_imap.logout()
            except Exception:
                pass
            self._idle_imap = None

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

        folders: dict[str, str | None] = {}
        folder_by_role: dict[str, str] = {}

        for item in data:
            if item is None:
                continue
            raw = item.decode("utf-8") if isinstance(item, bytes) else str(item)
            name, attrs = self._parse_list_entry(raw)
            if name is None:
                continue

            # Try RFC 6154 special-use attributes first
            role: str | None = None
            for attr in attrs:
                if attr in _SPECIAL_USE_ROLES:
                    role = _SPECIAL_USE_ROLES[attr]
                    break

            # Fall back to name heuristics
            if not role:
                name_lower = name.lower()
                role = _NAME_HEURISTICS.get(name_lower)

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

    # -- Header fetch --------------------------------------------------------

    def fetch_headers_by_uids(
        self, folder: str, uids: list[str],
    ) -> list[dict]:
        """Fetch headers for specific UIDs in a folder.

        Returns list of dicts with: uid, from, to, subject, date, flags, email_id.
        """
        if not uids:
            return []

        with self._lock:
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
            from:addr, to:addr, subject:text, since:YYYY-MM-DD, before:YYYY-MM-DD,
            flagged, unseen, seen, answered

        Quoted phrases: "exact phrase" → TEXT "exact phrase"
        Multiple terms are AND-ed. Unknown tokens become a TEXT search as fallback.
        """
        parts: list[str] = []

        # Tokenize — respecting quoted values
        # Matches: key:"quoted value", key:value, "quoted phrase", or standalone_keyword
        tokens = re.findall(r'(\w+:"[^"]*"|\w+:\S+|"[^"]*"|\w+)', query)

        for token in tokens:
            if token.startswith('"') and token.endswith('"'):
                # Quoted phrase → TEXT search
                phrase = token[1:-1]
                parts.append(f'TEXT "{phrase}"')
            elif ":" in token:
                key, _, value = token.partition(":")
                # Strip quotes from value
                value = value.strip('"')
                key_lower = key.lower()

                if key_lower == "from":
                    parts.append(f'FROM "{value}"')
                elif key_lower == "to":
                    parts.append(f'TO "{value}"')
                elif key_lower == "subject":
                    parts.append(f'SUBJECT "{value}"')
                elif key_lower == "since":
                    try:
                        dt = datetime.strptime(value, "%Y-%m-%d")
                        parts.append(f'SINCE {_format_imap_date(dt)}')
                    except ValueError:
                        parts.append(f'TEXT "{value}"')
                elif key_lower == "before":
                    try:
                        dt = datetime.strptime(value, "%Y-%m-%d")
                        parts.append(f'BEFORE {_format_imap_date(dt)}')
                    except ValueError:
                        parts.append(f'TEXT "{value}"')
                else:
                    # Unknown key — treat as TEXT search
                    parts.append(f'TEXT "{token}"')
            else:
                # Standalone keyword
                keyword = token.lower()
                if keyword == "flagged":
                    parts.append("FLAGGED")
                elif keyword == "unseen":
                    parts.append("UNSEEN")
                elif keyword == "seen":
                    parts.append("SEEN")
                elif keyword == "answered":
                    parts.append("ANSWERED")
                else:
                    # Fallback: treat as TEXT search
                    parts.append(f'TEXT "{token}"')

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
                if self._has_uidplus:
                    imap.uid("EXPUNGE", uid)
                else:
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
                imap = self._ensure_connected()
                imap.select(folder)
                imap.uid("STORE", uid, "+FLAGS", "(\\Deleted)")
                if self._has_uidplus:
                    imap.uid("EXPUNGE", uid)
                else:
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

            # CC in headers (but not BCC)
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

    # -- Background listening -------------------------------------------------

    def start_listening(
        self,
        on_message: Callable[[list[dict]], None],
        *,
        folder: str = "INBOX",
        poll_interval: int | None = None,
    ) -> None:
        """Start listening for new messages in a background daemon thread."""
        if self._bg_thread is not None:
            return  # already listening
        if poll_interval is None:
            poll_interval = self._poll_interval
        self._stop_event = threading.Event()
        self._bg_thread = threading.Thread(
            target=self._listen_wrapper,
            args=(folder, on_message, poll_interval),
            daemon=True,
        )
        self._bg_thread.start()

    def stop_listening(self) -> None:
        """Stop the background listening thread."""
        if hasattr(self, "_stop_event") and self._stop_event is not None:
            self._stop_event.set()
        if self._bg_thread is not None:
            self._bg_thread.join(timeout=10.0)
            self._bg_thread = None
        self._disconnect_idle()

    def _listen_wrapper(
        self,
        folder: str,
        on_message: Callable[[list[dict]], None],
        poll_interval: int,
    ) -> None:
        """Connect the IDLE connection and run the listen loop with reconnection."""
        while not self._stop_event.is_set():
            try:
                self._connect_idle()
                self._backoff_index = 0
                self._idle_loop(
                    folder, on_message,
                    poll_interval=poll_interval,
                    stop_event=self._stop_event,
                )
            except Exception as e:
                logger.warning("Listen error on %s: %s", self._email_address, e)
                self._disconnect_idle()
                delay = self._backoff_steps[
                    min(self._backoff_index, len(self._backoff_steps) - 1)
                ]
                self._backoff_index += 1
                if self._stop_event.wait(delay):
                    return

    # -- IDLE / poll (on dedicated _idle_imap connection) --------------------

    def _idle_loop(
        self,
        folder: str,
        on_message: Callable[[list[dict]], None],
        *,
        poll_interval: int = 30,
        stop_event: threading.Event | None = None,
    ) -> None:
        """Listen for new messages via IDLE (with poll fallback).

        Blocks until stop_event is set.  Runs entirely on ``_idle_imap``.
        on_message receives a list of header dicts for new messages.
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
                self._disconnect_idle()
                delay = self._backoff_steps[min(self._backoff_index, len(self._backoff_steps) - 1)]
                self._backoff_index += 1
                if stop_event.wait(delay):
                    return
                try:
                    self._connect_idle()
                    self._backoff_index = 0
                except Exception as ce:
                    logger.warning("Reconnect failed: %s", ce)

    def _idle_cycle(
        self,
        folder: str,
        on_message: Callable[[list[dict]], None],
        timeout: int,
        stop_event: threading.Event,
    ) -> None:
        """One IDLE cycle on _idle_imap: send IDLE, wait, send DONE."""
        imap = self._idle_imap
        if imap is None:
            raise RuntimeError("IDLE connection not established")

        imap.select(folder)

        # Send IDLE command via raw socket
        tag = imap._new_tag().decode("ascii")
        imap.send(f"{tag} IDLE\r\n".encode("ascii"))

        # Read continuation response (+)
        response = imap.readline()
        if not response.startswith(b"+"):
            logger.warning("IDLE not accepted: %s", response)
            return

        # Wait for server data or timeout
        try:
            # Cap at 25 minutes per RFC 2177
            effective_timeout = min(timeout, 1500)
            deadline = time.monotonic() + effective_timeout
            got_data = False
            while time.monotonic() < deadline:
                if stop_event.is_set():
                    break
                sock = imap.socket()
                ready, _, _ = _select_mod.select([sock], [], [], 1.0)
                if ready:
                    data = imap.readline()
                    if data:
                        got_data = True
                        break
        finally:
            # Send DONE to end IDLE
            try:
                imap.send(b"DONE\r\n")
                # Drain the tagged response
                while True:
                    line = imap.readline()
                    if not line:
                        break
                    decoded = line.decode("ascii", errors="replace").strip()
                    if decoded.startswith(tag):
                        break
            except Exception as e:
                logger.debug("IDLE DONE error: %s", e)

        # Check for new mail using the on-demand connection
        if not stop_event.is_set():
            self._check_new_mail(folder, on_message)

    def _poll_cycle(
        self,
        folder: str,
        on_message: Callable[[list[dict]], None],
        interval: int,
        stop_event: threading.Event,
    ) -> None:
        """One poll cycle on _idle_imap: NOOP, check new mail, sleep."""
        imap = self._idle_imap
        if imap is None:
            raise RuntimeError("IDLE connection not established")

        imap.select(folder)
        imap.noop()

        # Check for new mail using the on-demand connection
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

        Uses the on-demand connection (_imap) with locking.
        """
        new_headers: list[dict] = []

        with self._lock:
            imap = self._ensure_connected()
            imap.select(folder, readonly=True)

            status, data = imap.uid("SEARCH", None, "UNSEEN")
            if status != "OK" or not data or not data[0]:
                return

            uid_list = data[0].split()
            processed_for_folder = self._processed_uids.get(folder, set())
            new_uids = []
            for uid_bytes in uid_list:
                uid = uid_bytes.decode("ascii") if isinstance(uid_bytes, bytes) else str(uid_bytes)
                uid_int = int(uid)
                if uid_int not in processed_for_folder:
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
            if folder not in self._processed_uids:
                self._processed_uids[folder] = set()
            for uid in new_uids:
                self._processed_uids[folder].add(int(uid))
            self._save_state()

        # Filter by allowed_senders if configured (empty list = no filter)
        if new_headers and self._allowed_senders:
            allowed = {s.lower() for s in self._allowed_senders}
            filtered: list[dict] = []
            for hdr in new_headers:
                from_raw = hdr.get("from", "")
                _, from_addr = parseaddr(from_raw)
                if from_addr.lower() in allowed:
                    filtered.append(hdr)
            new_headers = filtered

        # Deliver outside the lock
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

            # processed_uids: {folder: set(uid_ints)}
            raw_uids = data.get("processed_uids", {})
            if isinstance(raw_uids, dict):
                self._processed_uids = {
                    folder: set(uid_list) for folder, uid_list in raw_uids.items()
                }
            else:
                self._processed_uids = {}

            # folders: {name: {"role": role}}
            if "folders" in data:
                self._folders = {}
                self._folder_by_role = {}
                for name, val in data["folders"].items():
                    role = val.get("role") or None if isinstance(val, dict) else None
                    self._folders[name] = role
                    if role and role not in self._folder_by_role:
                        self._folder_by_role[role] = name

            # capabilities: {name: bool}
            if "capabilities" in data:
                caps = data["capabilities"]
                if isinstance(caps, dict):
                    self._has_idle = bool(caps.get("idle", False))
                    self._has_move = bool(caps.get("move", False))
                    self._has_uidplus = bool(caps.get("uidplus", False))
                    # Rebuild _capabilities set from booleans
                    self._capabilities = set()
                    if self._has_idle:
                        self._capabilities.add("IDLE")
                    if self._has_move:
                        self._capabilities.add("MOVE")
                    if self._has_uidplus:
                        self._capabilities.add("UIDPLUS")
        except (json.JSONDecodeError, OSError) as e:
            logger.warning("Failed to load IMAP state for %s: %s", self._email_address, e)

    def _save_state(self) -> None:
        """Persist state to disk. Trims processed_uids to last 2000 per folder."""
        path = self._state_path()
        if path is None:
            return
        path.parent.mkdir(parents=True, exist_ok=True)

        # Trim each folder's UID set to last 2000, serialize to sorted lists
        trimmed_lists: dict[str, list[int]] = {}
        trimmed_sets: dict[str, set[int]] = {}
        for folder, uid_set in self._processed_uids.items():
            sorted_uids = sorted(uid_set)
            if len(sorted_uids) > 2000:
                sorted_uids = sorted_uids[-2000:]
            trimmed_lists[folder] = sorted_uids
            trimmed_sets[folder] = set(sorted_uids)
        self._processed_uids = trimmed_sets

        # Folders as {name: {"role": role}} objects (null for no role)
        folders_obj: dict[str, dict[str, str | None]] = {}
        for name, role in self._folders.items():
            folders_obj[name] = {"role": role}

        # Capabilities as {name: true/false} boolean dict
        caps_obj: dict[str, bool] = {
            "idle": self._has_idle,
            "move": self._has_move,
            "uidplus": self._has_uidplus,
        }

        state = {
            "processed_uids": trimmed_lists,
            "folders": folders_obj,
            "capabilities": caps_obj,
        }
        path.write_text(
            json.dumps(state, indent=2), encoding="utf-8",
        )
