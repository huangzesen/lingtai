# IMAP Addon Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the `imap` addon to faithfully expose IMAP/SMTP protocol capabilities — multi-account, server-side flags/search, IDLE push, folder management, CC/BCC, reply threading.

**Architecture:** Bottom-up build: `account.py` (single IMAP connection + SMTP) → `service.py` (multi-account coordinator) → `manager.py` (tool handler + filesystem) → `__init__.py` (setup/config). Each file is a clean rewrite — delete all existing code first. Email parsing helpers (`_decode_header_value`, `_extract_text_body`, `_extract_attachments`, `_strip_html_tags`) move from `service.py` to `account.py`.

**Tech Stack:** Python stdlib only — `imaplib`, `smtplib`, `email`, `threading`, `json`. No third-party dependencies.

**Spec:** `docs/superpowers/specs/2026-03-20-imap-addon-redesign.md`

**ERRATA (post-review fixes — implementer MUST apply these):**

1. **`fetch_envelopes` must NOT use IMAP ENVELOPE format.** The ENVELOPE response format is complex nested parenthesized lists that are fragile to parse with stdlib. Instead, use `FETCH (FLAGS BODY.PEEK[HEADER.FIELDS (FROM TO SUBJECT DATE)])` which returns standard RFC 822 headers parseable with Python's `email` module. Delete `_parse_envelope_data` entirely. Replace the ENVELOPE fetch+parse with header fetch+parse using `email.message_from_bytes()`.

2. **Add `fetch_headers_by_uids(folder, uids)` method** to `IMAPAccount`. The `search` action returns UIDs, and we need to fetch headers for those specific UIDs (not the N most recent). This method takes an explicit UID list and fetches `FLAGS BODY.PEEK[HEADER.FIELDS (...)]` for exactly those UIDs. `fetch_envelopes` should call this internally after selecting the last N UIDs.

3. **IDLE implementation must use raw socket, not private imaplib internals.** Replace `self._imap._new_tag()` and `self._imap.readline()` with direct socket access: `self._imap.send(b"tag IDLE\r\n")` using a manually managed tag counter, and read responses via `self._imap.readline()` → `self._imap._get_line()` is also private. The correct stdlib approach: use `imaplib.IMAP4.send()` (public) for sending and `imaplib.IMAP4.readline()` — but note `readline()` IS actually a public method on `IMAP4`. The real issue is `_new_tag()`. Fix: maintain a manual `_tag_counter` on `IMAPAccount` and format tags as `A{counter:04d}`.

4. **IDLE/agent-action interlock must use a condition variable.** The current plan releases `_lock` during the 25-min IDLE wait, which allows agent actions to acquire the lock and issue commands while the server is in IDLE mode (protocol violation). Fix: add `_in_idle: bool` flag and `_idle_done: threading.Event`. All public IMAP methods (`fetch_envelopes`, `fetch_full`, `search`, `store_flags`, `move_message`, `delete_message`) must, before acquiring `_lock`, check `_in_idle` and if True, set `_idle_event` to interrupt the IDLE wait, then wait on `_idle_done` before proceeding. The IDLE thread sets `_in_idle = True` before waiting, sends `DONE` when interrupted, sets `_idle_done`, clears `_in_idle`, and releases the lock.

5. **Task 3 Step 3 (manager rewrite) must include full implementation code**, not just prose requirements. The manager is the most complex piece. The implementer should write the complete `manager.py` following the 16 requirements listed, ensuring: `parse_email_id` uses `partition(":")` for account then `rpartition(":")` for folder/uid; `email_id` in action params is normalized from list or string; contacts paths are `imap/{address}/contacts.json`; `_reply` fetches original via `fetch_full`, extracts `message_id`/`references`, calls `send_email` with threading headers, then calls `store_flags({"answered": True})`.

6. **Add reply threading test to Task 3.** Test that `_reply` calls `account.fetch_full()` to get original's `message_id`, passes `in_reply_to` and `references` to `account.send_email()`, and calls `account.store_flags()` with `{"answered": True}`.

7. **`_check_new_mail` must call `on_message` OUTSIDE the lock** to prevent potential deadlock if the callback triggers an agent action. Collect payloads while holding the lock, then deliver them after releasing.

**Key conventions:**
- `from __future__ import annotations` in every file
- Dataclasses preferred over dicts for structured data
- All tests use `unittest.mock.MagicMock` for mocking
- Test functions follow `test_<what_is_tested>` naming
- No backward compatibility — clean rewrite

---

### Task 1: IMAPAccount — Connection, Capabilities, Folder Discovery

The foundation. `IMAPAccount` wraps a single IMAP connection with mutex-protected access, CAPABILITY parsing, and folder discovery with RFC 6154 role mapping.

**Files:**
- Create: `src/lingtai/addons/imap/account.py`
- Create: `tests/test_addon_imap_account.py`

- [ ] **Step 1: Write failing tests for IMAPAccount construction and capabilities**

```python
# tests/test_addon_imap_account.py
from __future__ import annotations

from unittest.mock import MagicMock, patch, PropertyMock
from lingtai.addons.imap.account import IMAPAccount


def test_construction():
    acct = IMAPAccount(
        email_address="agent@gmail.com",
        email_password="xxxx",
        imap_host="imap.gmail.com",
        smtp_host="smtp.gmail.com",
    )
    assert acct.address == "agent@gmail.com"
    assert not acct.connected


def test_parse_capabilities():
    """CAPABILITY response should be parsed into a dict."""
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="imap.gmail.com", smtp_host="smtp.gmail.com",
    )
    raw = b"IMAP4rev1 IDLE MOVE UIDPLUS"
    caps = acct._parse_capabilities(raw)
    assert caps["idle"] is True
    assert caps["move"] is True
    assert caps["uidplus"] is True


def test_parse_capabilities_missing():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="imap.gmail.com", smtp_host="smtp.gmail.com",
    )
    raw = b"IMAP4rev1"
    caps = acct._parse_capabilities(raw)
    assert caps["idle"] is False
    assert caps["move"] is False
    assert caps["uidplus"] is False
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_account.py -v --tb=short`
Expected: FAIL — `account.py` does not exist yet.

- [ ] **Step 3: Write IMAPAccount skeleton with construction and capability parsing**

```python
# src/lingtai/addons/imap/account.py
"""IMAPAccount — single IMAP connection + SMTP for one email account.

Wraps imaplib/smtplib with:
- Mutex-protected IMAP access (for IDLE interrupt)
- CAPABILITY parsing (IDLE, MOVE, UIDPLUS)
- Folder discovery with RFC 6154 role mapping
- ENVELOPE fetch (headers only)
- Full body fetch with attachment extraction
- Server-side SEARCH
- Flag STORE
- SMTP send with CC/BCC and reply threading
"""
from __future__ import annotations

import imaplib
import json
import logging
import mimetypes
import re
import smtplib
import threading
import time
from datetime import datetime, timezone
from email import encoders, policy as email_policy
from email.header import decode_header
from email.mime.base import MIMEBase
from email.mime.multipart import MIMEMultipart
from email.mime.text import MIMEText
from email.utils import formataddr, parseaddr, make_msgid
from pathlib import Path
from typing import Callable
from uuid import uuid4

logger = logging.getLogger(__name__)

# RFC 6154 special-use attribute → role mapping
_SPECIAL_USE_ROLES: dict[str, str] = {
    "\\Trash": "trash",
    "\\Sent": "sent",
    "\\Archive": "archive",
    "\\Drafts": "drafts",
    "\\Junk": "junk",
}

# Fallback name heuristics (case-insensitive)
_NAME_HEURISTICS: dict[str, str] = {
    "trash": "trash",
    "deleted items": "trash",
    "deleted": "trash",
    "[gmail]/trash": "trash",
    "sent": "sent",
    "sent items": "sent",
    "sent mail": "sent",
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


def _extract_text_body(msg) -> str:
    """Extract plain-text body from an email.Message."""
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


def _extract_attachments(msg) -> list[dict]:
    """Extract file attachments from an email.Message."""
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


# ---------------------------------------------------------------------------
# IMAPAccount
# ---------------------------------------------------------------------------

class IMAPAccount:
    """Single IMAP/SMTP account with mutex-protected connection."""

    def __init__(
        self,
        email_address: str,
        email_password: str,
        *,
        imap_host: str = "imap.gmail.com",
        imap_port: int = 993,
        smtp_host: str = "smtp.gmail.com",
        smtp_port: int = 587,
        allowed_senders: list[str] | None = None,
        poll_interval: int = 30,
        working_dir: Path | str | None = None,
    ) -> None:
        self._email_address = email_address
        self._email_password = email_password
        self._imap_host = imap_host
        self._imap_port = imap_port
        self._smtp_host = smtp_host
        self._smtp_port = smtp_port
        self._allowed_senders = allowed_senders
        self._poll_interval = poll_interval
        self._working_dir = Path(working_dir) if working_dir else None

        # Connection state
        self._imap: imaplib.IMAP4_SSL | None = None
        self._lock = threading.Lock()
        self._idle_event = threading.Event()  # signal IDLE thread to wake
        self._running = False
        self._bg_thread: threading.Thread | None = None

        # Server state
        self._capabilities: dict[str, bool] = {
            "idle": False, "move": False, "uidplus": False,
        }
        self._folders: dict[str, dict] = {}  # name → {"role": str|None}
        self._selected_folder: str | None = None

        # Dedup state
        self._processed_uids: dict[str, set[str]] = {}  # folder → {uid, ...}
        self._load_state()

    @property
    def address(self) -> str:
        return self._email_address

    @property
    def connected(self) -> bool:
        return self._imap is not None

    # ------------------------------------------------------------------
    # Capability parsing
    # ------------------------------------------------------------------

    def _parse_capabilities(self, raw: bytes) -> dict[str, bool]:
        """Parse IMAP CAPABILITY response into our tracked capabilities."""
        text = raw.decode("ascii", errors="replace").upper()
        return {
            "idle": "IDLE" in text,
            "move": "MOVE" in text,
            "uidplus": "UIDPLUS" in text,
        }

    # ------------------------------------------------------------------
    # Folder discovery
    # ------------------------------------------------------------------

    def _discover_folders(self) -> dict[str, dict]:
        """Query LIST and map folders to roles via RFC 6154 + heuristics."""
        if self._imap is None:
            return {}
        status, data = self._imap.list()
        if status != "OK" or not data:
            return {}
        folders: dict[str, dict] = {}
        for item in data:
            if item is None:
                continue
            line = item.decode("utf-8", errors="replace") if isinstance(item, bytes) else str(item)
            # Parse: (attributes) "delimiter" "name"
            match = re.match(r'\(([^)]*)\)\s+"([^"]+)"\s+"?([^"]*)"?', line)
            if not match:
                # Try unquoted name
                match = re.match(r'\(([^)]*)\)\s+"([^"]+)"\s+(\S+)', line)
            if not match:
                continue
            attrs_str, _delimiter, name = match.groups()
            name = name.strip('"')
            # Check RFC 6154 special-use attributes
            role = None
            for attr, r in _SPECIAL_USE_ROLES.items():
                if attr.lower() in attrs_str.lower():
                    role = r
                    break
            # Fallback to name heuristics
            if role is None:
                role = _NAME_HEURISTICS.get(name.lower())
            folders[name] = {"role": role}
        return folders

    # ------------------------------------------------------------------
    # Connection lifecycle
    # ------------------------------------------------------------------

    def connect(self) -> None:
        """Connect to IMAP server, discover capabilities and folders."""
        self._imap = imaplib.IMAP4_SSL(self._imap_host, self._imap_port)
        self._imap.login(self._email_address, self._email_password)
        # Parse capabilities
        if self._imap.capabilities:
            cap_bytes = b" ".join(
                c if isinstance(c, bytes) else c.encode()
                for c in self._imap.capabilities
            )
            self._capabilities = self._parse_capabilities(cap_bytes)
        # Discover folders
        self._folders = self._discover_folders()
        self._save_state()
        logger.info("IMAP connected: %s (idle=%s, move=%s, uidplus=%s)",
                     self._email_address,
                     self._capabilities["idle"],
                     self._capabilities["move"],
                     self._capabilities["uidplus"])

    def disconnect(self) -> None:
        """Disconnect from IMAP server."""
        if self._imap is not None:
            try:
                self._imap.logout()
            except Exception:
                pass
            self._imap = None
            self._selected_folder = None

    def _select_folder(self, folder: str) -> bool:
        """Select a folder if not already selected. Returns True on success."""
        if self._imap is None:
            return False
        if self._selected_folder == folder:
            return True
        status, _ = self._imap.select(folder)
        if status == "OK":
            self._selected_folder = folder
            return True
        return False

    def get_folder_by_role(self, role: str) -> str | None:
        """Find folder name by role (trash, sent, archive, etc.)."""
        for name, info in self._folders.items():
            if info.get("role") == role:
                return name
        return None

    # ------------------------------------------------------------------
    # IMAP operations (all require self._lock)
    # ------------------------------------------------------------------

    def fetch_envelopes(self, folder: str = "INBOX", n: int = 10) -> list[dict]:
        """Fetch N most recent email headers from a folder via ENVELOPE.

        Returns list of dicts with: uid, from, to, subject, date, flags.
        Does NOT download body. Does NOT persist to disk.
        """
        with self._lock:
            if not self._select_folder(folder):
                return []
            # Search for all UIDs, take last N
            status, data = self._imap.uid("SEARCH", None, "ALL")  # type: ignore[arg-type]
            if status != "OK" or not data or not data[0]:
                return []
            all_uids = data[0].split()
            recent_uids = all_uids[-n:] if n > 0 else all_uids
            if not recent_uids:
                return []
            uid_set = b",".join(recent_uids)
            status, fetch_data = self._imap.uid(
                "FETCH", uid_set, "(ENVELOPE FLAGS)"
            )
            if status != "OK" or not fetch_data:
                return []
            return self._parse_envelopes(fetch_data, folder)

    def _parse_envelopes(self, fetch_data: list, folder: str) -> list[dict]:
        """Parse FETCH ENVELOPE responses into dicts."""
        results: list[dict] = []
        for item in fetch_data:
            if not isinstance(item, tuple) or len(item) < 2:
                continue
            header = item[0]
            if isinstance(header, bytes):
                header = header.decode("utf-8", errors="replace")
            # Extract UID from response
            uid_match = re.search(r"UID (\d+)", header)
            if not uid_match:
                continue
            uid = uid_match.group(1)
            # Extract FLAGS
            flags_match = re.search(r"FLAGS \(([^)]*)\)", header)
            flags_raw = flags_match.group(1) if flags_match else ""
            flags = self._parse_flags(flags_raw)
            # Extract ENVELOPE — this is complex, use a simpler approach
            # Fetch basic headers instead
            envelope = self._parse_envelope_data(item)
            results.append({
                "email_id": f"{self._email_address}:{folder}:{uid}",
                "uid": uid,
                "from": envelope.get("from", ""),
                "to": envelope.get("to", []),
                "subject": envelope.get("subject", ""),
                "date": envelope.get("date", ""),
                "flags": flags,
            })
        # Sort by UID descending (newest first)
        results.sort(key=lambda x: int(x["uid"]), reverse=True)
        return results

    def _parse_envelope_data(self, item: tuple) -> dict:
        """Extract from/to/subject/date from an ENVELOPE fetch response.

        ENVELOPE format is complex (nested parenthesized lists).
        We use a pragmatic approach: fetch headers separately if needed.
        For now, extract what we can from the raw response.
        """
        # item[1] contains the ENVELOPE data as bytes
        raw = item[1] if len(item) > 1 and isinstance(item[1], bytes) else b""
        raw_str = raw.decode("utf-8", errors="replace")
        # Simple extraction — ENVELOPE is: (date subject from sender reply-to to cc bcc in-reply-to message-id)
        # This is a best-effort parse
        result: dict = {"from": "", "to": [], "subject": "", "date": ""}
        # Try to extract subject from between first two NIL/quoted strings
        # This is fragile — a more robust approach would use a proper IMAP parser
        # For production, we should fetch specific headers instead
        return result

    def _parse_flags(self, flags_raw: str) -> dict:
        """Parse IMAP flag string into our flag dict."""
        flags_upper = flags_raw.upper()
        return {
            "seen": "\\SEEN" in flags_upper,
            "flagged": "\\FLAGGED" in flags_upper,
            "answered": "\\ANSWERED" in flags_upper,
            "draft": "\\DRAFT" in flags_upper,
            "deleted": "\\DELETED" in flags_upper,
        }

    def fetch_full(self, folder: str, uid: str) -> dict | None:
        """Fetch full email by UID — body, attachments, headers.

        Sets \\Seen on server (IMAP default behavior).
        Returns dict with full email data, or None if not found.
        """
        import email as email_mod
        with self._lock:
            if not self._select_folder(folder):
                return None
            status, data = self._imap.uid("FETCH", uid, "(BODY[] FLAGS)")
            if status != "OK" or not data or data[0] is None:
                return None
            # Parse flags from response header
            header = data[0][0]
            if isinstance(header, bytes):
                header = header.decode("utf-8", errors="replace")
            flags_match = re.search(r"FLAGS \(([^)]*)\)", header)
            flags_raw = flags_match.group(1) if flags_match else ""
            flags = self._parse_flags(flags_raw)
            # Parse email body
            raw_email = data[0][1]
            msg = email_mod.message_from_bytes(raw_email, policy=email_policy.default)
            from_raw = msg.get("From", "")
            _, from_addr = parseaddr(from_raw)
            to_raw = msg.get("To", "")
            to_addrs = [parseaddr(a)[1] for a in to_raw.split(",") if a.strip()]
            cc_raw = msg.get("Cc", "")
            cc_addrs = [parseaddr(a)[1] for a in cc_raw.split(",") if a.strip()] if cc_raw else []
            subject = _decode_header_value(msg.get("Subject", ""))
            date = msg.get("Date", "")
            message_id = msg.get("Message-ID", "")
            references = msg.get("References", "")
            body = _extract_text_body(msg)
            attachments = _extract_attachments(msg)
            return {
                "email_id": f"{self._email_address}:{folder}:{uid}",
                "uid": uid,
                "from": from_addr,
                "to": to_addrs,
                "cc": cc_addrs,
                "subject": subject,
                "date": date,
                "message": body,
                "message_id": message_id,
                "references": references,
                "flags": flags,
                "attachments": attachments,
            }

    def search(self, query: str, folder: str = "INBOX") -> list[str]:
        """Execute server-side IMAP SEARCH. Returns list of UIDs.

        Query syntax: "from:addr subject:text since:YYYY-MM-DD flagged unseen"
        Maps to IMAP SEARCH commands.
        """
        with self._lock:
            if not self._select_folder(folder):
                return []
            imap_query = self._build_search_query(query)
            status, data = self._imap.uid("SEARCH", None, imap_query)  # type: ignore[arg-type]
            if status != "OK" or not data or not data[0]:
                return []
            return [
                uid.decode("ascii") if isinstance(uid, bytes) else str(uid)
                for uid in data[0].split()
            ]

    def _build_search_query(self, query: str) -> str:
        """Parse user query syntax into IMAP SEARCH command string."""
        parts: list[str] = []
        # Tokenize: respect quoted strings
        tokens = re.findall(r'"[^"]*"|\S+', query)
        for token in tokens:
            if token.startswith('"') and token.endswith('"'):
                parts.append(f'TEXT {token}')
            elif ":" in token:
                key, _, val = token.partition(":")
                key = key.lower()
                if key == "from":
                    parts.append(f'FROM "{val}"')
                elif key == "to":
                    parts.append(f'TO "{val}"')
                elif key == "subject":
                    parts.append(f'SUBJECT "{val}"')
                elif key == "since":
                    parts.append(f'SINCE {self._format_imap_date(val)}')
                elif key == "before":
                    parts.append(f'BEFORE {self._format_imap_date(val)}')
                else:
                    parts.append(f'TEXT "{token}"')
            elif token.lower() in ("flagged", "unflagged", "seen", "unseen",
                                     "answered", "unanswered", "draft", "deleted"):
                parts.append(token.upper())
            else:
                parts.append(f'TEXT "{token}"')
        return " ".join(parts) if parts else "ALL"

    def _format_imap_date(self, date_str: str) -> str:
        """Convert YYYY-MM-DD to IMAP date format (DD-Mon-YYYY)."""
        try:
            dt = datetime.strptime(date_str, "%Y-%m-%d")
            return dt.strftime("%d-%b-%Y")
        except ValueError:
            return date_str  # pass through if already in right format

    # ------------------------------------------------------------------
    # Flag operations
    # ------------------------------------------------------------------

    _FLAG_MAP = {
        "seen": "\\Seen",
        "flagged": "\\Flagged",
        "answered": "\\Answered",
        "draft": "\\Draft",
        "deleted": "\\Deleted",
    }

    def store_flags(self, folder: str, uid: str, flags: dict[str, bool]) -> bool:
        """Set/clear flags on a message. Returns True on success."""
        with self._lock:
            if not self._select_folder(folder):
                return False
            for name, value in flags.items():
                imap_flag = self._FLAG_MAP.get(name)
                if not imap_flag:
                    continue
                cmd = "+FLAGS" if value else "-FLAGS"
                self._imap.uid("STORE", uid, cmd, f"({imap_flag})")
            return True

    # ------------------------------------------------------------------
    # Folder operations
    # ------------------------------------------------------------------

    def list_folders(self) -> list[dict]:
        """Return list of all folders with roles."""
        return [
            {"name": name, "role": info.get("role")}
            for name, info in self._folders.items()
        ]

    def move_message(self, folder: str, uid: str, dest_folder: str) -> bool:
        """Move a message to another folder. Returns True on success."""
        with self._lock:
            if not self._select_folder(folder):
                return False
            if self._capabilities.get("move"):
                status, _ = self._imap.uid("MOVE", uid, dest_folder)
                self._selected_folder = None  # folder state may have changed
                return status == "OK"
            else:
                # COPY + DELETE + EXPUNGE
                status, _ = self._imap.uid("COPY", uid, dest_folder)
                if status != "OK":
                    return False
                self._imap.uid("STORE", uid, "+FLAGS", "(\\Deleted)")
                if self._capabilities.get("uidplus"):
                    self._imap.uid("EXPUNGE", uid)
                else:
                    self._imap.expunge()
                self._selected_folder = None
                return True

    def delete_message(self, folder: str, uid: str) -> bool:
        """Delete a message — move to Trash, or flag+expunge if no Trash."""
        trash = self.get_folder_by_role("trash")
        if trash and folder != trash:
            return self.move_message(folder, uid, trash)
        else:
            with self._lock:
                if not self._select_folder(folder):
                    return False
                self._imap.uid("STORE", uid, "+FLAGS", "(\\Deleted)")
                if self._capabilities.get("uidplus"):
                    self._imap.uid("EXPUNGE", uid)
                else:
                    self._imap.expunge()
                return True

    # ------------------------------------------------------------------
    # SMTP send
    # ------------------------------------------------------------------

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
        """Send email via SMTP. Returns None on success, error string on failure.

        CC: added as header + in envelope RCPT TO.
        BCC: in envelope RCPT TO only (not in headers).
        in_reply_to/references: for threading replies.
        """
        if not subject and not body and not attachments:
            return "Cannot send empty email (no subject, no body, and no attachments)"

        # Validate attachment paths
        if attachments:
            for filepath in attachments:
                if not Path(filepath).is_file():
                    return f"Attachment not found: {filepath}"

        try:
            # Build MIME message
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

            # Headers
            mime_msg["From"] = formataddr(("", self._email_address))
            mime_msg["To"] = ", ".join(to)
            mime_msg["Subject"] = subject
            mime_msg["Message-ID"] = make_msgid()
            if cc:
                mime_msg["Cc"] = ", ".join(cc)
            # BCC is intentionally NOT added as a header
            if in_reply_to:
                mime_msg["In-Reply-To"] = in_reply_to
            if references:
                mime_msg["References"] = references

            # Build envelope recipient list: to + cc + bcc
            all_recipients = list(to)
            if cc:
                all_recipients.extend(cc)
            if bcc:
                all_recipients.extend(bcc)

            with smtplib.SMTP(self._smtp_host, self._smtp_port) as server:
                server.starttls()
                server.login(self._email_address, self._email_password)
                server.sendmail(
                    self._email_address,
                    all_recipients,
                    mime_msg.as_string(),
                )
            return None
        except Exception as e:
            error = f"SMTP send failed: {e}"
            logger.error(error)
            return error

    # ------------------------------------------------------------------
    # IDLE / Poll background thread
    # ------------------------------------------------------------------

    def start_listening(self, on_message: Callable[[dict], None]) -> None:
        """Start background thread for IDLE or polling."""
        if self._bg_thread is not None:
            return
        self._running = True
        self._bg_thread = threading.Thread(
            target=self._listen_loop,
            args=(on_message,),
            daemon=True,
        )
        self._bg_thread.start()

    def stop_listening(self) -> None:
        """Stop background thread."""
        self._running = False
        self._idle_event.set()  # wake IDLE if sleeping
        if self._bg_thread is not None:
            self._bg_thread.join(timeout=5.0)
            self._bg_thread = None

    def _listen_loop(self, on_message: Callable[[dict], None]) -> None:
        """Main loop — connect, IDLE or poll, reconnect on error."""
        backoff = [1, 2, 5, 10, 60]
        backoff_idx = 0
        while self._running:
            try:
                self.connect()
                backoff_idx = 0  # reset on successful connect
                if self._capabilities["idle"]:
                    self._idle_loop(on_message)
                else:
                    self._poll_loop(on_message)
            except Exception as e:
                logger.warning("IMAP error on %s, reconnecting in %ds: %s",
                               self._email_address, backoff[backoff_idx], e)
                wait = backoff[backoff_idx]
                backoff_idx = min(backoff_idx + 1, len(backoff) - 1)
                for _ in range(wait):
                    if not self._running:
                        return
                    time.sleep(1)
            finally:
                self.disconnect()

    def _idle_loop(self, on_message: Callable[[dict], None]) -> None:
        """IDLE-based listening loop."""
        with self._lock:
            self._select_folder("INBOX")
        while self._running:
            with self._lock:
                # Check for new mail before entering IDLE
                self._check_new_mail(on_message)
                # Enter IDLE
                self._imap.send(b"%s IDLE\r\n" % self._imap._new_tag().encode())
                # Read continuation response
                resp = self._imap.readline()
            # Wait for IDLE notification or timeout (25 min)
            self._idle_event.clear()
            self._idle_event.wait(timeout=25 * 60)
            # Exit IDLE
            with self._lock:
                self._imap.send(b"DONE\r\n")
                # Read IDLE completion response
                while True:
                    resp = self._imap.readline()
                    if isinstance(resp, bytes) and b"OK" in resp.upper():
                        break
            if not self._running:
                return

    def _poll_loop(self, on_message: Callable[[dict], None]) -> None:
        """Polling-based listening loop."""
        with self._lock:
            self._select_folder("INBOX")
        while self._running:
            with self._lock:
                try:
                    self._check_new_mail(on_message)
                except (imaplib.IMAP4.error, OSError) as e:
                    logger.info("IMAP stale on %s, reconnecting: %s",
                                self._email_address, e)
                    raise  # will reconnect in _listen_loop
            for _ in range(self._poll_interval):
                if not self._running:
                    return
                time.sleep(1)

    def _check_new_mail(self, on_message: Callable[[dict], None]) -> None:
        """Check for UNSEEN messages in INBOX and deliver new ones.

        Must be called while holding self._lock.
        """
        self._imap.noop()
        status, data = self._imap.uid("SEARCH", None, "UNSEEN")  # type: ignore[arg-type]
        if status != "OK" or not data or not data[0]:
            return
        folder_uids = self._processed_uids.setdefault("INBOX", set())
        for uid_bytes in data[0].split():
            uid = uid_bytes.decode("ascii") if isinstance(uid_bytes, bytes) else str(uid_bytes)
            if uid in folder_uids:
                continue
            try:
                self._fetch_and_deliver(uid, on_message)
            except Exception as e:
                logger.warning("Failed to fetch UID %s on %s: %s",
                               uid, self._email_address, e)
            folder_uids.add(uid)
        self._save_state()

    def _fetch_and_deliver(
        self, uid: str, on_message: Callable[[dict], None],
    ) -> None:
        """Fetch a single email and deliver it. Must hold self._lock."""
        import email as email_mod
        status, data = self._imap.uid("FETCH", uid, "(RFC822)")
        if status != "OK" or not data or data[0] is None:
            return
        raw_email = data[0][1]
        msg = email_mod.message_from_bytes(raw_email, policy=email_policy.default)
        from_raw = msg.get("From", "")
        _, from_addr = parseaddr(from_raw)
        subject = _decode_header_value(msg.get("Subject", ""))
        body = _extract_text_body(msg)
        attachments = _extract_attachments(msg)
        # Filter
        if self._allowed_senders is not None:
            if from_addr.lower() not in [s.lower() for s in self._allowed_senders]:
                logger.debug("Skipping from non-allowed sender: %s", from_addr)
                return
        payload = {
            "account": self._email_address,
            "email_id": f"{self._email_address}:INBOX:{uid}",
            "from": from_addr,
            "subject": subject,
            "message": body,
            "attachments": attachments,
        }
        on_message(payload)

    # ------------------------------------------------------------------
    # State persistence
    # ------------------------------------------------------------------

    def _state_path(self) -> Path | None:
        if self._working_dir is None:
            return None
        return self._working_dir / "imap" / self._email_address / "state.json"

    def _load_state(self) -> None:
        path = self._state_path()
        if path is None or not path.is_file():
            return
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
            raw_uids = data.get("processed_uids", {})
            self._processed_uids = {
                folder: set(uids) for folder, uids in raw_uids.items()
            }
            self._folders = data.get("folders", {})
            self._capabilities = data.get("capabilities", self._capabilities)
        except (json.JSONDecodeError, OSError) as e:
            logger.warning("Failed to load state for %s: %s", self._email_address, e)

    def _save_state(self) -> None:
        path = self._state_path()
        if path is None:
            return
        path.parent.mkdir(parents=True, exist_ok=True)
        serializable_uids = {}
        for folder, uids in self._processed_uids.items():
            uid_list = sorted(uids, key=lambda x: int(x) if x.isdigit() else 0)
            if len(uid_list) > 1000:
                uid_list = uid_list[-1000:]
                self._processed_uids[folder] = set(uid_list)
            serializable_uids[folder] = uid_list
        state = {
            "processed_uids": serializable_uids,
            "folders": self._folders,
            "capabilities": self._capabilities,
        }
        path.write_text(json.dumps(state, indent=2), encoding="utf-8")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_account.py -v --tb=short`
Expected: PASS (3 tests)

- [ ] **Step 5: Write tests for SEARCH query builder**

Add to `tests/test_addon_imap_account.py`:

```python
def test_build_search_query_from():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    result = acct._build_search_query("from:alice@example.com")
    assert result == 'FROM "alice@example.com"'


def test_build_search_query_combined():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    result = acct._build_search_query("from:alice since:2026-03-01 unseen")
    assert 'FROM "alice"' in result
    assert "SINCE 01-Mar-2026" in result
    assert "UNSEEN" in result


def test_build_search_query_quoted():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    result = acct._build_search_query('"exact phrase"')
    assert result == 'TEXT "exact phrase"'


def test_build_search_query_fallback():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    result = acct._build_search_query("meeting notes")
    assert result == 'TEXT "meeting" TEXT "notes"'
```

- [ ] **Step 6: Run tests**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_account.py -v --tb=short`
Expected: PASS (7 tests)

- [ ] **Step 7: Write tests for flag parsing**

Add to `tests/test_addon_imap_account.py`:

```python
def test_parse_flags():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    flags = acct._parse_flags("\\Seen \\Flagged")
    assert flags["seen"] is True
    assert flags["flagged"] is True
    assert flags["answered"] is False
    assert flags["draft"] is False


def test_parse_flags_empty():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    flags = acct._parse_flags("")
    assert flags["seen"] is False
    assert flags["flagged"] is False
```

- [ ] **Step 8: Run tests**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_account.py -v --tb=short`
Expected: PASS (9 tests)

- [ ] **Step 9: Write tests for SMTP send with CC/BCC**

Add to `tests/test_addon_imap_account.py`:

```python
from unittest.mock import patch, MagicMock


def test_send_basic():
    acct = IMAPAccount(
        email_address="agent@example.com", email_password="x",
        imap_host="h", smtp_host="smtp.example.com",
    )
    with patch("lingtai.addons.imap.account.smtplib.SMTP") as MockSMTP:
        mock_server = MagicMock()
        MockSMTP.return_value.__enter__ = MagicMock(return_value=mock_server)
        MockSMTP.return_value.__exit__ = MagicMock(return_value=False)
        result = acct.send_email(["user@example.com"], "test", "hello")
        assert result is None
        mock_server.starttls.assert_called_once()
        mock_server.sendmail.assert_called_once()
        # Check recipient list
        call_args = mock_server.sendmail.call_args
        assert call_args[0][1] == ["user@example.com"]


def test_send_with_cc_bcc():
    acct = IMAPAccount(
        email_address="agent@example.com", email_password="x",
        imap_host="h", smtp_host="smtp.example.com",
    )
    with patch("lingtai.addons.imap.account.smtplib.SMTP") as MockSMTP:
        mock_server = MagicMock()
        MockSMTP.return_value.__enter__ = MagicMock(return_value=mock_server)
        MockSMTP.return_value.__exit__ = MagicMock(return_value=False)
        result = acct.send_email(
            ["to@example.com"], "test", "hello",
            cc=["cc@example.com"], bcc=["bcc@example.com"],
        )
        assert result is None
        call_args = mock_server.sendmail.call_args
        recipients = call_args[0][1]
        assert "to@example.com" in recipients
        assert "cc@example.com" in recipients
        assert "bcc@example.com" in recipients
        # BCC must not appear in message headers
        msg_str = call_args[0][2]
        assert "Cc: cc@example.com" in msg_str
        assert "Bcc" not in msg_str


def test_send_with_reply_threading():
    acct = IMAPAccount(
        email_address="agent@example.com", email_password="x",
        imap_host="h", smtp_host="smtp.example.com",
    )
    with patch("lingtai.addons.imap.account.smtplib.SMTP") as MockSMTP:
        mock_server = MagicMock()
        MockSMTP.return_value.__enter__ = MagicMock(return_value=mock_server)
        MockSMTP.return_value.__exit__ = MagicMock(return_value=False)
        result = acct.send_email(
            ["user@example.com"], "Re: test", "reply body",
            in_reply_to="<original@msg.id>",
            references="<original@msg.id>",
        )
        assert result is None
        msg_str = mock_server.sendmail.call_args[0][2]
        assert "In-Reply-To: <original@msg.id>" in msg_str
        assert "References: <original@msg.id>" in msg_str


def test_send_empty_rejected():
    acct = IMAPAccount(
        email_address="agent@example.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    result = acct.send_email(["user@example.com"], "", "")
    assert result is not None


def test_send_missing_attachment():
    acct = IMAPAccount(
        email_address="agent@example.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    result = acct.send_email(
        ["user@example.com"], "test", "hello",
        attachments=["/nonexistent/file.png"],
    )
    assert result is not None
    assert "not found" in result.lower()
```

- [ ] **Step 10: Run tests**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_account.py -v --tb=short`
Expected: PASS (14 tests)

- [ ] **Step 11: Write tests for folder discovery and role mapping**

Add to `tests/test_addon_imap_account.py`:

```python
def test_discover_folders_with_special_use():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    acct._imap = MagicMock()
    acct._imap.list.return_value = ("OK", [
        b'(\\HasNoChildren) "/" "INBOX"',
        b'(\\HasNoChildren \\Sent) "/" "[Gmail]/Sent Mail"',
        b'(\\HasNoChildren \\Trash) "/" "[Gmail]/Trash"',
        b'(\\HasNoChildren \\Junk) "/" "[Gmail]/Spam"',
        b'(\\HasNoChildren \\Drafts) "/" "[Gmail]/Drafts"',
    ])
    folders = acct._discover_folders()
    assert folders["INBOX"]["role"] is None
    assert folders["[Gmail]/Sent Mail"]["role"] == "sent"
    assert folders["[Gmail]/Trash"]["role"] == "trash"
    assert folders["[Gmail]/Spam"]["role"] == "junk"
    assert folders["[Gmail]/Drafts"]["role"] == "drafts"


def test_discover_folders_name_heuristics():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    acct._imap = MagicMock()
    acct._imap.list.return_value = ("OK", [
        b'(\\HasNoChildren) "/" "INBOX"',
        b'(\\HasNoChildren) "/" "Sent Items"',
        b'(\\HasNoChildren) "/" "Deleted Items"',
    ])
    folders = acct._discover_folders()
    assert folders["Sent Items"]["role"] == "sent"
    assert folders["Deleted Items"]["role"] == "trash"


def test_get_folder_by_role():
    acct = IMAPAccount(
        email_address="a@b.com", email_password="x",
        imap_host="h", smtp_host="h",
    )
    acct._folders = {
        "INBOX": {"role": None},
        "[Gmail]/Trash": {"role": "trash"},
        "[Gmail]/Sent Mail": {"role": "sent"},
    }
    assert acct.get_folder_by_role("trash") == "[Gmail]/Trash"
    assert acct.get_folder_by_role("sent") == "[Gmail]/Sent Mail"
    assert acct.get_folder_by_role("archive") is None
```

- [ ] **Step 12: Run tests**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_account.py -v --tb=short`
Expected: PASS (17 tests)

- [ ] **Step 13: Smoke test import**

Run: `source venv/bin/activate && python -c "from lingtai.addons.imap.account import IMAPAccount; print('OK')"`
Expected: `OK`

- [ ] **Step 14: Commit**

```bash
git add src/lingtai/addons/imap/account.py tests/test_addon_imap_account.py
git commit -m "feat(imap): add IMAPAccount — connection, capabilities, folders, search, flags, SMTP with CC/BCC"
```

---

### Task 2: IMAPMailService — Multi-Account Coordinator

Replaces the current single-account `IMAPMailService` with a multi-account coordinator that delegates to `IMAPAccount` instances.

**Files:**
- Rewrite: `src/lingtai/addons/imap/service.py`
- Create: `tests/test_addon_imap_service_v2.py` (new test file, delete old after)

- [ ] **Step 1: Write failing tests**

```python
# tests/test_addon_imap_service_v2.py
from __future__ import annotations

from unittest.mock import MagicMock, patch
from lingtai.addons.imap.service import IMAPMailService


def test_single_account_construction():
    svc = IMAPMailService(accounts=[{
        "email_address": "a@gmail.com",
        "email_password": "x",
        "imap_host": "imap.gmail.com",
        "smtp_host": "smtp.gmail.com",
    }])
    assert svc.default_account.address == "a@gmail.com"
    assert len(svc.accounts) == 1


def test_multi_account_construction():
    svc = IMAPMailService(accounts=[
        {"email_address": "a@gmail.com", "email_password": "x",
         "imap_host": "imap.gmail.com", "smtp_host": "smtp.gmail.com"},
        {"email_address": "b@outlook.com", "email_password": "y",
         "imap_host": "outlook.office365.com", "smtp_host": "smtp.office365.com"},
    ])
    assert len(svc.accounts) == 2
    assert svc.default_account.address == "a@gmail.com"


def test_get_account_by_address():
    svc = IMAPMailService(accounts=[
        {"email_address": "a@gmail.com", "email_password": "x",
         "imap_host": "h", "smtp_host": "h"},
        {"email_address": "b@outlook.com", "email_password": "y",
         "imap_host": "h", "smtp_host": "h"},
    ])
    acct = svc.get_account("b@outlook.com")
    assert acct is not None
    assert acct.address == "b@outlook.com"


def test_get_account_default():
    svc = IMAPMailService(accounts=[
        {"email_address": "a@gmail.com", "email_password": "x",
         "imap_host": "h", "smtp_host": "h"},
    ])
    acct = svc.get_account(None)
    assert acct.address == "a@gmail.com"


def test_get_account_unknown():
    svc = IMAPMailService(accounts=[
        {"email_address": "a@gmail.com", "email_password": "x",
         "imap_host": "h", "smtp_host": "h"},
    ])
    acct = svc.get_account("unknown@example.com")
    assert acct is None


def test_mail_service_send_delegates():
    """MailService.send() should delegate to default account."""
    svc = IMAPMailService(accounts=[
        {"email_address": "a@gmail.com", "email_password": "x",
         "imap_host": "h", "smtp_host": "h"},
    ])
    with patch.object(svc.default_account, "send_email", return_value=None) as mock:
        result = svc.send("user@example.com", {"subject": "hi", "message": "hello"})
        assert result is None
        mock.assert_called_once()


def test_mail_service_address():
    svc = IMAPMailService(accounts=[
        {"email_address": "a@gmail.com", "email_password": "x",
         "imap_host": "h", "smtp_host": "h"},
    ])
    assert svc.address == "a@gmail.com"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_service_v2.py -v --tb=short`
Expected: FAIL — current `IMAPMailService` has different constructor.

- [ ] **Step 3: Rewrite service.py**

```python
# src/lingtai/addons/imap/service.py
"""IMAPMailService — multi-account IMAP/SMTP coordinator.

Manages N IMAPAccount instances. Implements MailService interface
for TCP bridge compatibility. Delegates to accounts by address.
"""
from __future__ import annotations

import logging
from pathlib import Path
from typing import Callable

from lingtai_kernel.services.mail import MailService
from .account import IMAPAccount

logger = logging.getLogger(__name__)


class IMAPMailService(MailService):
    """Multi-account IMAP/SMTP coordinator.

    Usage:
        svc = IMAPMailService(accounts=[
            {"email_address": "a@gmail.com", "email_password": "xxxx",
             "imap_host": "imap.gmail.com", "smtp_host": "smtp.gmail.com"},
        ])
        svc.listen(on_message=callback)
        svc.send("user@example.com", {"subject": "hi", "message": "hello"})
    """

    def __init__(
        self,
        accounts: list[dict],
        *,
        working_dir: Path | str | None = None,
    ) -> None:
        self._working_dir = Path(working_dir) if working_dir else None
        self._accounts: list[IMAPAccount] = []
        for cfg in accounts:
            acct = IMAPAccount(
                email_address=cfg["email_address"],
                email_password=cfg["email_password"],
                imap_host=cfg.get("imap_host", "imap.gmail.com"),
                imap_port=cfg.get("imap_port", 993),
                smtp_host=cfg.get("smtp_host", "smtp.gmail.com"),
                smtp_port=cfg.get("smtp_port", 587),
                allowed_senders=cfg.get("allowed_senders"),
                poll_interval=cfg.get("poll_interval", 30),
                working_dir=self._working_dir,
            )
            self._accounts.append(acct)
        self._account_map: dict[str, IMAPAccount] = {
            a.address: a for a in self._accounts
        }

    @property
    def accounts(self) -> list[IMAPAccount]:
        return list(self._accounts)

    @property
    def default_account(self) -> IMAPAccount:
        return self._accounts[0]

    def get_account(self, address: str | None) -> IMAPAccount | None:
        """Get account by address, or default if None."""
        if address is None:
            return self.default_account
        return self._account_map.get(address)

    # -- MailService interface -----------------------------------------------

    def send(self, address: str, message: dict) -> str | None:
        """Send via default account. Implements MailService for TCP bridge."""
        return self.default_account.send_email(
            to=[address],
            subject=message.get("subject", ""),
            body=message.get("message", ""),
        )

    def listen(self, on_message: Callable[[dict], None]) -> None:
        """Start listening on all accounts."""
        for acct in self._accounts:
            acct.start_listening(on_message)

    def stop(self) -> None:
        """Stop all accounts."""
        for acct in self._accounts:
            acct.stop_listening()
            acct.disconnect()

    @property
    def address(self) -> str | None:
        return self.default_account.address if self._accounts else None
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_service_v2.py -v --tb=short`
Expected: PASS (7 tests)

- [ ] **Step 5: Delete old test file, rename new one**

```bash
rm tests/test_addon_imap_service.py
mv tests/test_addon_imap_service_v2.py tests/test_addon_imap_service.py
```

- [ ] **Step 6: Run all imap tests**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_service.py tests/test_addon_imap_account.py -v --tb=short`
Expected: PASS (all)

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/addons/imap/service.py tests/test_addon_imap_service.py
git commit -m "feat(imap): rewrite IMAPMailService as multi-account coordinator"
```

---

### Task 3: IMAPMailManager — Tool Handler Rewrite

Rewrite the manager with the new tool schema, email_id parsing, server-side operations, and per-account filesystem.

**Files:**
- Rewrite: `src/lingtai/addons/imap/manager.py`
- Create: `tests/test_addon_imap_manager_v2.py` (delete old after)

- [ ] **Step 1: Write failing tests for email_id parsing and action dispatch**

```python
# tests/test_addon_imap_manager_v2.py
from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock
from lingtai.addons.imap.manager import IMAPMailManager, parse_email_id


def test_parse_email_id():
    account, folder, uid = parse_email_id("alice@gmail.com:INBOX:1042")
    assert account == "alice@gmail.com"
    assert folder == "INBOX"
    assert uid == "1042"


def test_parse_email_id_folder_with_slash():
    account, folder, uid = parse_email_id("a@b.com:[Gmail]/Sent Mail:999")
    assert account == "a@b.com"
    assert folder == "[Gmail]/Sent Mail"
    assert uid == "999"


def test_check_delegates_to_account(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    acct = MagicMock()
    acct.address = "agent@gmail.com"
    acct.fetch_envelopes.return_value = [
        {"email_id": "agent@gmail.com:INBOX:1", "uid": "1",
         "from": "user@gmail.com", "to": ["agent@gmail.com"],
         "subject": "hello", "date": "2026-03-20", "flags": {"seen": False}},
    ]
    svc.get_account.return_value = acct
    svc.default_account = acct
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({"action": "check"})
    assert result["status"] == "ok"
    assert len(result["emails"]) == 1
    acct.fetch_envelopes.assert_called_once()


def test_read_delegates_to_account(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    acct = MagicMock()
    acct.address = "agent@gmail.com"
    acct.fetch_full.return_value = {
        "email_id": "agent@gmail.com:INBOX:1", "uid": "1",
        "from": "user@gmail.com", "to": ["agent@gmail.com"],
        "subject": "hello", "message": "body text",
        "date": "2026-03-20", "flags": {"seen": True},
        "message_id": "<msg@id>", "references": "",
        "cc": [], "attachments": [],
    }
    svc.get_account.return_value = acct
    svc.default_account = acct
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({"action": "read", "email_id": ["agent@gmail.com:INBOX:1"]})
    assert result["status"] == "ok"
    assert len(result["emails"]) == 1
    assert result["emails"][0]["message"] == "body text"


def test_accounts_action(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    acct1 = MagicMock()
    acct1.address = "a@gmail.com"
    acct1.connected = True
    acct2 = MagicMock()
    acct2.address = "b@outlook.com"
    acct2.connected = False
    svc.accounts = [acct1, acct2]
    svc.default_account = acct1
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({"action": "accounts"})
    assert len(result["accounts"]) == 2


def test_folders_action(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    acct = MagicMock()
    acct.address = "agent@gmail.com"
    acct.list_folders.return_value = [
        {"name": "INBOX", "role": None},
        {"name": "[Gmail]/Trash", "role": "trash"},
    ]
    svc.get_account.return_value = acct
    svc.default_account = acct
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({"action": "folders"})
    assert result["status"] == "ok"
    assert len(result["folders"]) == 2


def test_delete_action(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    acct = MagicMock()
    acct.address = "agent@gmail.com"
    acct.delete_message.return_value = True
    svc.get_account.return_value = acct
    svc.default_account = acct
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({"action": "delete", "email_id": ["agent@gmail.com:INBOX:1"]})
    assert result["status"] == "ok"
    acct.delete_message.assert_called_once_with("INBOX", "1")


def test_move_action(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    acct = MagicMock()
    acct.address = "agent@gmail.com"
    acct.move_message.return_value = True
    svc.get_account.return_value = acct
    svc.default_account = acct
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({"action": "move", "email_id": ["agent@gmail.com:INBOX:1"], "folder": "Archive"})
    assert result["status"] == "ok"
    acct.move_message.assert_called_once_with("INBOX", "1", "Archive")


def test_flag_action(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    acct = MagicMock()
    acct.address = "agent@gmail.com"
    acct.store_flags.return_value = True
    svc.get_account.return_value = acct
    svc.default_account = acct
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({
        "action": "flag",
        "email_id": ["agent@gmail.com:INBOX:1"],
        "flags": {"flagged": True, "seen": False},
    })
    assert result["status"] == "ok"
    acct.store_flags.assert_called_once_with("INBOX", "1", {"flagged": True, "seen": False})


def test_search_action(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    acct = MagicMock()
    acct.address = "agent@gmail.com"
    acct.search.return_value = ["100", "101"]
    acct.fetch_envelopes.return_value = [
        {"email_id": "agent@gmail.com:INBOX:100", "uid": "100",
         "from": "alice@example.com", "to": ["agent@gmail.com"],
         "subject": "meeting", "date": "2026-03-20", "flags": {"seen": False}},
    ]
    svc.get_account.return_value = acct
    svc.default_account = acct
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({"action": "search", "query": "from:alice"})
    assert result["status"] == "ok"
    acct.search.assert_called_once()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_manager_v2.py -v --tb=short`
Expected: FAIL

- [ ] **Step 3: Rewrite manager.py**

The full manager rewrite. Key changes from current:
- `parse_email_id()` function to split `account:folder:uid`
- All mailbox actions (`check`, `read`, `search`, `flag`, `move`, `delete`) delegate to `IMAPAccount` via `IMAPMailService`
- New actions: `folders`, `accounts`, `flag`, `move`, `delete`
- `reply` uses `In-Reply-To`/`References` from original email
- `send` supports `cc`/`bcc`
- Contacts are per-account (under `imap/{address}/contacts.json`)
- `on_imap_received` handles account field in payload
- No more `read.json` — flags live on server

Write the complete manager to `src/lingtai/addons/imap/manager.py`. This is a full rewrite — the implementer should replace the entire file. The manager should:

1. Define `parse_email_id(email_id: str) -> tuple[str, str, str]` that splits on first `:` (account) and last `:` (uid), with everything in between as folder.
2. Update `SCHEMA` with all new actions: `send`, `check`, `read`, `reply`, `search`, `delete`, `move`, `flag`, `folders`, `contacts`, `add_contact`, `remove_contact`, `edit_contact`, `accounts`. Add `cc`, `bcc`, `flags`, `account` properties.
3. Update `DESCRIPTION` to reflect new capabilities.
4. Rewrite `IMAPMailManager.__init__` to accept `service: IMAPMailService` (not `imap_service: IMAPMailService`).
5. Route all actions through `service.get_account(args.get("account"))`.
6. `_check` calls `account.fetch_envelopes()`.
7. `_read` calls `account.fetch_full()` and persists result to disk.
8. `_reply` fetches original's `message_id`/`references`, calls `account.send_email()` with threading headers, sets `\Answered` flag.
9. `_send` calls `account.send_email()` with `cc`/`bcc`.
10. `_search` calls `account.search()` then fetches envelopes for matched UIDs.
11. `_delete` calls `account.delete_message()`.
12. `_move` calls `account.move_message()`.
13. `_flag` calls `account.store_flags()`.
14. `_folders` calls `account.list_folders()`.
15. `_accounts` lists all accounts with connection status.
16. Contacts use per-account paths: `imap/{address}/contacts.json`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_manager_v2.py -v --tb=short`
Expected: PASS (11 tests)

- [ ] **Step 5: Write contact tests**

Add to `tests/test_addon_imap_manager_v2.py`:

```python
def test_contacts_per_account(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    acct = MagicMock()
    acct.address = "agent@gmail.com"
    svc.get_account.return_value = acct
    svc.default_account = acct
    mgr = IMAPMailManager(agent, service=svc, tcp_alias="127.0.0.1:8399")

    # Add contact
    result = mgr.handle({"action": "add_contact", "address": "bob@example.com", "name": "Bob"})
    assert result["status"] == "added"

    # List contacts
    result = mgr.handle({"action": "contacts"})
    assert len(result["contacts"]) == 1

    # Contacts file should be under account directory
    contacts_file = tmp_path / "imap" / "agent@gmail.com" / "contacts.json"
    assert contacts_file.is_file()
```

- [ ] **Step 6: Run tests**

Run: `source venv/bin/activate && python -m pytest tests/test_addon_imap_manager_v2.py -v --tb=short`
Expected: PASS (12 tests)

- [ ] **Step 7: Delete old test file, rename new one**

```bash
rm tests/test_addon_imap_manager.py
mv tests/test_addon_imap_manager_v2.py tests/test_addon_imap_manager.py
```

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/addons/imap/manager.py tests/test_addon_imap_manager.py
git commit -m "feat(imap): rewrite IMAPMailManager — full tool schema with folders, flags, search, multi-account"
```

---

### Task 4: Setup & Config — `__init__.py` Rewrite

Rewrite setup to support single-account shorthand and multi-account config.

**Files:**
- Rewrite: `src/lingtai/addons/imap/__init__.py`
- Rewrite: `tests/test_addons.py`

- [ ] **Step 1: Write failing tests**

```python
# tests/test_addons.py
from __future__ import annotations
from unittest.mock import MagicMock, patch


def test_addon_registry():
    from lingtai.addons import _BUILTIN
    assert "imap" in _BUILTIN


def test_agent_addon_lifecycle():
    from lingtai.agent import Agent
    import inspect
    sig = inspect.signature(Agent.__init__)
    assert "addons" in sig.parameters


def test_setup_single_account():
    """Single-account shorthand should work."""
    from lingtai.addons.imap import setup
    agent = MagicMock()
    agent._working_dir = "/tmp/test"
    with patch("lingtai.addons.imap.TCPMailService"):
        mgr = setup(
            agent,
            email_address="a@gmail.com",
            email_password="x",
            imap_host="imap.gmail.com",
            smtp_host="smtp.gmail.com",
            bridge_port=8399,
        )
    assert mgr is not None
    agent.add_tool.assert_called_once()
    tool_name = agent.add_tool.call_args[0][0]
    assert tool_name == "imap"


def test_setup_multi_account():
    """Multi-account config should work."""
    from lingtai.addons.imap import setup
    agent = MagicMock()
    agent._working_dir = "/tmp/test"
    with patch("lingtai.addons.imap.TCPMailService"):
        mgr = setup(
            agent,
            accounts=[
                {"email_address": "a@gmail.com", "email_password": "x",
                 "imap_host": "imap.gmail.com", "smtp_host": "smtp.gmail.com"},
                {"email_address": "b@outlook.com", "email_password": "y",
                 "imap_host": "outlook.office365.com", "smtp_host": "smtp.office365.com"},
            ],
            bridge_port=8399,
        )
    assert mgr is not None
    # System prompt should mention both accounts
    call_kwargs = agent.add_tool.call_args[1]
    assert "a@gmail.com" in call_kwargs["system_prompt"]
    assert "b@outlook.com" in call_kwargs["system_prompt"]


def test_setup_no_account_raises():
    """Neither accounts nor email_address should raise ValueError."""
    from lingtai.addons.imap import setup
    agent = MagicMock()
    agent._working_dir = "/tmp/test"
    import pytest
    with pytest.raises(ValueError):
        setup(agent, bridge_port=8399)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `source venv/bin/activate && python -m pytest tests/test_addons.py -v --tb=short`
Expected: FAIL — current setup() has different signature.

- [ ] **Step 3: Rewrite `__init__.py`**

```python
# src/lingtai/addons/imap/__init__.py
"""IMAP addon — real email via IMAP/SMTP.

Adds an `imap` tool with multi-account support. Each account gets
its own IMAP connection, SMTP credentials, and on-disk mailbox.

Usage:
    # Single account
    agent = Agent(
        addons={"imap": {
            "email_address": "agent@gmail.com",
            "email_password": "xxxx",
            "imap_host": "imap.gmail.com",
            "smtp_host": "smtp.gmail.com",
        }},
    )

    # Multi-account
    agent = Agent(
        addons={"imap": {
            "accounts": [
                {"email_address": "a@gmail.com", "email_password": "x",
                 "imap_host": "imap.gmail.com", "smtp_host": "smtp.gmail.com"},
                {"email_address": "b@outlook.com", "email_password": "y",
                 "imap_host": "outlook.office365.com", "smtp_host": "smtp.office365.com"},
            ],
        }},
    )
"""
from __future__ import annotations

import logging
from pathlib import Path
from typing import TYPE_CHECKING

from lingtai_kernel.services.mail import TCPMailService
from .manager import IMAPMailManager, SCHEMA, DESCRIPTION
from .service import IMAPMailService

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    # Single-account shorthand
    email_address: str | None = None,
    email_password: str | None = None,
    imap_host: str = "imap.gmail.com",
    imap_port: int = 993,
    smtp_host: str = "smtp.gmail.com",
    smtp_port: int = 587,
    allowed_senders: list[str] | None = None,
    poll_interval: int = 30,
    # Multi-account
    accounts: list[dict] | None = None,
    # Addon-level
    bridge_port: int = 8399,
) -> IMAPMailManager:
    """Set up IMAP addon — registers imap tool, creates services."""
    working_dir = Path(agent._working_dir)
    tcp_alias = f"127.0.0.1:{bridge_port}"

    # Build accounts list
    if accounts is None:
        if email_address is None:
            raise ValueError(
                "IMAP addon requires either 'accounts' list or "
                "'email_address' + 'email_password'"
            )
        accounts = [{
            "email_address": email_address,
            "email_password": email_password,
            "imap_host": imap_host,
            "imap_port": imap_port,
            "smtp_host": smtp_host,
            "smtp_port": smtp_port,
            "allowed_senders": allowed_senders,
            "poll_interval": poll_interval,
        }]

    imap_svc = IMAPMailService(accounts=accounts, working_dir=working_dir)
    bridge = TCPMailService(listen_port=bridge_port)

    mgr = IMAPMailManager(agent, service=imap_svc, tcp_alias=tcp_alias)
    mgr._bridge = bridge

    # Build system prompt listing all accounts
    account_lines = []
    for acct in imap_svc.accounts:
        account_lines.append(f"  - {acct.address}")
    accounts_str = "\n".join(account_lines)

    agent.add_tool(
        "imap", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt=(
            f"IMAP email accounts:\n{accounts_str}\n"
            f"Internal TCP alias: {tcp_alias} "
            f"(other agents can send to this address to relay via IMAP/SMTP)\n"
            f"Use imap(action=...) for external email. "
            f"Use email(action=...) for inter-agent communication.\n"
            f"Use imap(action=\"accounts\") to see connection status."
        ),
    )

    for acct in imap_svc.accounts:
        log.info("IMAP addon configured: %s", acct.address)
    log.info("IMAP bridge: %s", tcp_alias)
    return mgr
```

- [ ] **Step 4: Run tests**

Run: `source venv/bin/activate && python -m pytest tests/test_addons.py -v --tb=short`
Expected: PASS (5 tests)

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/addons/imap/__init__.py tests/test_addons.py
git commit -m "feat(imap): rewrite setup with multi-account config support"
```

---

### Task 5: Update App Launcher & Config

Update `app/email/__main__.py` to use new config key names and multi-account support.

**Files:**
- Modify: `app/email/__main__.py`
- Modify: `app/email/config.json`
- Modify: `app/email/config.example.json`

- [ ] **Step 1: Update `config.json`**

The config already uses `email_address`/`email_password` (from the rename). Verify it works with the new setup signature. Add `imap_host` and `smtp_host` if not present.

- [ ] **Step 2: Update `__main__.py`**

Key changes:
- Remove the manual addon dict construction — just pass config fields through.
- Update `TerminalLoggingService._DISPLAY_EVENTS` (already done in rename).
- Update covenant to reference `imap(action="check")` (already done in rename).
- Pass `imap_host`/`smtp_host` from config.

- [ ] **Step 3: Smoke test the launcher**

Run: `source venv/bin/activate && python -c "from app.email.__main__ import main; print('import OK')"`
Expected: `import OK`

- [ ] **Step 4: Commit**

```bash
git add app/email/__main__.py app/email/config.json app/email/config.example.json
git commit -m "feat(imap): update email app launcher for new IMAP addon API"
```

---

### Task 6: Full Test Suite & Smoke Test

Run all tests, fix any failures, verify the whole addon works end-to-end.

**Files:**
- All test files

- [ ] **Step 1: Run full test suite**

Run: `source venv/bin/activate && python -m pytest tests/ -v --tb=short`
Expected: ALL PASS

- [ ] **Step 2: Smoke test imports**

Run: `source venv/bin/activate && python -c "import lingtai; from lingtai.addons.imap.account import IMAPAccount; from lingtai.addons.imap.service import IMAPMailService; from lingtai.addons.imap.manager import IMAPMailManager; print('all OK')"`
Expected: `all OK`

- [ ] **Step 3: Fix any failures**

Address any import errors, test failures, or integration issues.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat(imap): complete IMAP addon redesign — multi-account, server-side flags/search, IDLE, folders"
```

---

### Task 7: Update CLAUDE.md

Update the project CLAUDE.md to reflect the new IMAP addon architecture.

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update the Built-in Capabilities / Addons section**

If there is an addon section, update it. If not, the addon description in the existing docs should be accurate after the code change.

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for IMAP addon redesign"
```
