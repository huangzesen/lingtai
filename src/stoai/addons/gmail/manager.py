"""GmailManager — filesystem-based Gmail mailbox with tool handler.

Storage layout:
    working_dir/gmail/inbox/{uuid}/message.json   — received
    working_dir/gmail/sent/{uuid}/message.json     — sent
    working_dir/gmail/read.json                    — read tracking
    working_dir/gmail/contacts.json                — contact book

Mirrors EmailManager patterns but uses gmail/ instead of mailbox/,
sends via GoogleMailService, and injects tcp_alias + account in every response.
"""
from __future__ import annotations

import json
import os
import re
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING
from uuid import uuid4

if TYPE_CHECKING:
    from ...base_agent import BaseAgent
    from .service import GoogleMailService

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": [
                "send", "check", "read", "reply", "search",
                "contacts", "add_contact", "remove_contact", "edit_contact",
            ],
            "description": (
                "send: send email via Gmail (requires address, message). "
                "check: list mailbox (optional folder, n). "
                "read: read emails by ID list (email_id=[id1, id2, ...]). "
                "You are encouraged to read multiple relevant or even all unread emails and think before acting. "
                "reply: reply to email (requires email_id, message). "
                "search: regex search mailbox (requires query, optional folder). "
                "contacts: list all contacts. "
                "add_contact: add/update contact (requires address, name; optional note). "
                "remove_contact: remove contact (requires address). "
                "edit_contact: update contact fields (requires address; optional name, note)."
            ),
        },
        "address": {
            "oneOf": [
                {"type": "string"},
                {"type": "array", "items": {"type": "string"}},
            ],
            "description": "Target email address(es) for send",
        },
        "subject": {"type": "string", "description": "Email subject line"},
        "message": {"type": "string", "description": "Email body"},
        "email_id": {
            "type": "array",
            "items": {"type": "string"},
            "description": "List of email IDs for read. For reply, pass a single-element list.",
        },
        "n": {
            "type": "integer",
            "description": "Max recent emails to show (for check, default 10)",
            "default": 10,
        },
        "query": {
            "type": "string",
            "description": "Regex pattern for search (matches from, subject, message)",
        },
        "folder": {
            "type": "string",
            "enum": ["inbox", "sent"],
            "description": "Folder for check/search. Default: inbox for check, both for search.",
        },
        "name": {
            "type": "string",
            "description": "Contact's human-readable name (for add_contact, edit_contact)",
        },
        "note": {
            "type": "string",
            "description": "Free-text note about the contact (for add_contact, edit_contact)",
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Gmail client — real email via Gmail IMAP/SMTP with its own mailbox "
    "(working_dir/gmail/). Every response includes account and tcp_alias fields. "
    "Use 'send' for outgoing email (saved to gmail/sent/). "
    "'check' to list inbox or sent (optional folder param). "
    "'read' to read by ID. "
    "'reply' to respond to a received email. "
    "'search' to find emails by regex (searches from, subject, message). "
    "'contacts' to list saved contacts. "
    "'add_contact' to register a contact (address, name, optional note). "
    "'remove_contact' to delete a contact by address. "
    "'edit_contact' to update fields on an existing contact."
)


class GmailManager:
    """Filesystem-based Gmail manager — reads/writes gmail/ directory."""

    def __init__(
        self,
        agent: "BaseAgent",
        gmail_service: "GoogleMailService",
        tcp_alias: str,
    ) -> None:
        self._agent = agent
        self._gmail_service = gmail_service
        self._tcp_alias = tcp_alias
        self._bridge = None  # set by setup() before start()
        # Duplicate send protection — maps address → (message_text, count)
        self._last_sent: dict[str, tuple[str, int]] = {}
        self._dup_free_passes = 2

    @property
    def _mailbox_dir(self) -> Path:
        return Path(self._agent._working_dir) / "gmail"

    # ------------------------------------------------------------------
    # Meta injection
    # ------------------------------------------------------------------

    def _inject_meta(self, result: dict) -> dict:
        """Add tcp_alias and account to every response."""
        result["tcp_alias"] = self._tcp_alias
        result["account"] = self._gmail_service.address
        return result

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        """Start IMAP poll and TCP bridge listener."""
        self._gmail_service.listen(on_message=self.on_gmail_received)

        if self._bridge is not None:
            def on_bridge_mail(payload: dict) -> None:
                to = payload.get("to", [])
                if isinstance(to, str):
                    to = [to]
                if not to:
                    return
                for addr in to:
                    self._gmail_service.send(addr, payload)

            self._bridge.listen(on_message=on_bridge_mail)

    def stop(self) -> None:
        """Stop IMAP poll and TCP bridge."""
        self._gmail_service.stop()
        if self._bridge is not None:
            self._bridge.stop()

    # ------------------------------------------------------------------
    # Action dispatch
    # ------------------------------------------------------------------

    def handle(self, args: dict) -> dict:
        action = args.get("action")
        if action == "send":
            return self._inject_meta(self._send(args))
        elif action == "check":
            return self._inject_meta(self._check(args))
        elif action == "read":
            return self._inject_meta(self._read(args))
        elif action == "reply":
            return self._inject_meta(self._reply(args))
        elif action == "search":
            return self._inject_meta(self._search(args))
        elif action == "contacts":
            return self._inject_meta(self._contacts())
        elif action == "add_contact":
            return self._inject_meta(self._add_contact(args))
        elif action == "remove_contact":
            return self._inject_meta(self._remove_contact(args))
        elif action == "edit_contact":
            return self._inject_meta(self._edit_contact(args))
        else:
            return self._inject_meta({"error": f"Unknown gmail action: {action}"})

    # ------------------------------------------------------------------
    # Receive handler — called by GoogleMailService IMAP poll
    # ------------------------------------------------------------------

    def on_gmail_received(self, payload: dict) -> None:
        """Handle incoming Gmail — save to mailbox and notify agent."""
        email_id = payload.get("_mailbox_id") or str(uuid4())

        sender = payload.get("from", "unknown")
        subject = payload.get("subject", "(no subject)")
        message = payload.get("message", "")

        self._agent._mail_arrived.set()

        preview = message[:100].replace("\n", " ")
        notification = (
            f'[system] 1 new message in gmail box.\n'
            f'  From: {sender} — {subject}\n'
            f'  {preview}...\n'
            f'Use gmail(action="check") to see your inbox.'
        )

        from ...message import _make_message, MSG_REQUEST
        self._agent._log(
            "gmail_received", sender=sender, subject=subject, message=message,
        )
        msg = _make_message(MSG_REQUEST, "system", notification)
        self._agent.inbox.put(msg)

    # ------------------------------------------------------------------
    # Filesystem helpers
    # ------------------------------------------------------------------

    def _load_email(self, email_id: str) -> dict | None:
        """Load a single email by ID. Checks inbox/ then sent/."""
        for folder in ("inbox", "sent"):
            path = self._mailbox_dir / folder / email_id / "message.json"
            if path.is_file():
                try:
                    data = json.loads(path.read_text())
                except (json.JSONDecodeError, OSError):
                    continue
                data["_folder"] = folder
                data.setdefault("_mailbox_id", email_id)
                return data
        return None

    def _list_emails(self, folder: str) -> list[dict]:
        """Load all emails from a folder, sorted by time (newest first)."""
        folder_dir = self._mailbox_dir / folder
        if not folder_dir.is_dir():
            return []
        emails = []
        for msg_dir in folder_dir.iterdir():
            msg_file = msg_dir / "message.json"
            if msg_dir.is_dir() and msg_file.is_file():
                try:
                    data = json.loads(msg_file.read_text())
                    data["_folder"] = folder
                    data.setdefault("_mailbox_id", msg_dir.name)
                    emails.append(data)
                except (json.JSONDecodeError, OSError):
                    continue

        def sort_key(e):
            return e.get("received_at") or e.get("sent_at") or e.get("time") or ""

        emails.sort(key=sort_key, reverse=True)
        return emails

    def _read_ids(self) -> set[str]:
        """Load the set of read email IDs from read.json."""
        path = self._mailbox_dir / "read.json"
        if path.is_file():
            try:
                return set(json.loads(path.read_text()))
            except (json.JSONDecodeError, OSError):
                return set()
        return set()

    def _mark_read(self, email_id: str) -> None:
        """Add email_id to read.json with atomic write."""
        ids = self._read_ids()
        ids.add(email_id)
        self._mailbox_dir.mkdir(parents=True, exist_ok=True)
        target = self._mailbox_dir / "read.json"
        fd, tmp = tempfile.mkstemp(dir=str(self._mailbox_dir), suffix=".tmp")
        try:
            os.write(fd, json.dumps(sorted(ids)).encode())
            os.close(fd)
            os.replace(tmp, str(target))
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise

    def _email_summary(self, e: dict, read_ids: set[str] | None = None) -> dict:
        """Build a summary dict from a raw email dict."""
        eid = e.get("_mailbox_id", "")
        if read_ids is None:
            read_ids = self._read_ids()
        entry = {
            "id": eid,
            "from": e.get("from", ""),
            "to": e.get("to", []),
            "subject": e.get("subject", "(no subject)"),
            "preview": e.get("message", "")[:200],
            "time": e.get("received_at") or e.get("sent_at") or e.get("time") or "",
            "folder": e.get("_folder", ""),
        }
        if e.get("_folder") == "inbox":
            entry["unread"] = eid not in read_ids
        return entry

    # ------------------------------------------------------------------
    # Send — deliver via Gmail SMTP + save to sent/
    # ------------------------------------------------------------------

    def _send(self, args: dict) -> dict:
        raw_address = args.get("address", "")
        subject = args.get("subject", "")
        message_text = args.get("message", "")

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

    # ------------------------------------------------------------------
    # Check — list emails from a folder
    # ------------------------------------------------------------------

    def _check(self, args: dict) -> dict:
        folder = args.get("folder", "inbox")
        n = args.get("n", 10)
        emails = self._list_emails(folder)
        total = len(emails)
        recent = emails[:n] if n > 0 else emails
        read_ids = self._read_ids()
        summaries = [self._email_summary(e, read_ids) for e in recent]
        return {"status": "ok", "total": total, "showing": len(summaries), "emails": summaries}

    # ------------------------------------------------------------------
    # Read — load full email by ID
    # ------------------------------------------------------------------

    def _read(self, args: dict) -> dict:
        ids = args.get("email_id", [])
        if isinstance(ids, str):
            ids = [ids]
        if not ids:
            return {"error": "email_id is required"}

        results = []
        errors = []
        for eid in ids:
            data = self._load_email(eid)
            if data is None:
                errors.append(eid)
                continue
            if data.get("_folder") == "inbox":
                self._mark_read(eid)
            results.append({
                "id": eid,
                "from": data.get("from", ""),
                "to": data.get("to", []),
                "subject": data.get("subject", "(no subject)"),
                "message": data.get("message", ""),
                "time": data.get("received_at") or data.get("sent_at") or data.get("time") or "",
                "folder": data.get("_folder", ""),
            })

        result = {"status": "ok", "emails": results}
        if errors:
            result["not_found"] = errors
        return result

    # ------------------------------------------------------------------
    # Reply
    # ------------------------------------------------------------------

    def _reply(self, args: dict) -> dict:
        email_id = args.get("email_id", "")
        if isinstance(email_id, list):
            email_id = email_id[0] if email_id else ""
        if not email_id:
            return {"error": "email_id is required for reply"}
        message_text = args.get("message", "")
        if not message_text:
            return {"error": "message is required for reply"}

        original = self._load_email(email_id)
        if original is None:
            return {"error": f"Email not found: {email_id}"}

        orig_subject = original.get("subject", "")
        subject = args.get("subject") or (
            orig_subject if orig_subject.startswith("Re: ") else f"Re: {orig_subject}"
        )

        return self._send({
            "address": original["from"],
            "subject": subject,
            "message": message_text,
        })

    # ------------------------------------------------------------------
    # Search
    # ------------------------------------------------------------------

    def _search(self, args: dict) -> dict:
        query = args.get("query", "")
        if not query:
            return {"error": "query is required for search"}

        folder = args.get("folder")
        folders = [folder] if folder else ["inbox", "sent"]

        try:
            pattern = re.compile(query, re.IGNORECASE)
        except re.error as e:
            return {"error": f"Invalid regex: {e}"}

        matches = []
        read_ids = self._read_ids()
        for f in folders:
            for email in self._list_emails(f):
                searchable = " ".join([
                    email.get("from", ""),
                    email.get("subject", ""),
                    email.get("message", ""),
                ])
                if pattern.search(searchable):
                    matches.append(self._email_summary(email, read_ids))

        return {"status": "ok", "total": len(matches), "emails": matches}

    # ------------------------------------------------------------------
    # Contacts
    # ------------------------------------------------------------------

    @property
    def _contacts_path(self) -> Path:
        return self._mailbox_dir / "contacts.json"

    def _load_contacts(self) -> list[dict]:
        """Load contacts list from disk."""
        if self._contacts_path.is_file():
            try:
                return json.loads(self._contacts_path.read_text())
            except (json.JSONDecodeError, OSError):
                return []
        return []

    def _save_contacts(self, contacts: list[dict]) -> None:
        """Atomically write contacts list to disk."""
        self._mailbox_dir.mkdir(parents=True, exist_ok=True)
        target = self._contacts_path
        fd, tmp = tempfile.mkstemp(dir=str(self._mailbox_dir), suffix=".tmp")
        try:
            os.write(fd, json.dumps(contacts, indent=2).encode())
            os.close(fd)
            os.replace(tmp, str(target))
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise

    def _contacts(self) -> dict:
        return {"status": "ok", "contacts": self._load_contacts()}

    def _add_contact(self, args: dict) -> dict:
        address = args.get("address", "")
        name = args.get("name", "")
        if not address:
            return {"error": "address is required"}
        if not name:
            return {"error": "name is required"}
        note = args.get("note", "")

        contacts = self._load_contacts()
        for c in contacts:
            if c["address"] == address:
                c["name"] = name
                c["note"] = note
                self._save_contacts(contacts)
                return {"status": "updated", "contact": c}
        entry = {"address": address, "name": name, "note": note}
        contacts.append(entry)
        self._save_contacts(contacts)
        return {"status": "added", "contact": entry}

    def _remove_contact(self, args: dict) -> dict:
        address = args.get("address", "")
        if not address:
            return {"error": "address is required"}
        contacts = self._load_contacts()
        new_contacts = [c for c in contacts if c["address"] != address]
        if len(new_contacts) == len(contacts):
            return {"error": f"Contact not found: {address}"}
        self._save_contacts(new_contacts)
        return {"status": "removed", "address": address}

    def _edit_contact(self, args: dict) -> dict:
        address = args.get("address", "")
        if not address:
            return {"error": "address is required"}
        contacts = self._load_contacts()
        for c in contacts:
            if c["address"] == address:
                if "name" in args:
                    c["name"] = args["name"]
                if "note" in args:
                    c["note"] = args["note"]
                self._save_contacts(contacts)
                return {"status": "updated", "contact": c}
        return {"error": f"Contact not found: {address}"}
