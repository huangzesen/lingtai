"""Email capability — filesystem-based mailbox with search.

Storage layout:
    working_dir/mailbox/inbox/{uuid}/message.json   — received
    working_dir/mailbox/sent/{uuid}/message.json     — sent
    working_dir/mailbox/read.json                    — read tracking

Usage:
    agent.add_capability("email")
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
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["send", "check", "read", "reply", "reply_all", "search"],
            "description": (
                "send: send with optional cc/bcc (requires address, message). "
                "check: list mailbox (optional folder, n). "
                "read: read email by ID (requires email_id). "
                "reply: reply to email (requires email_id, message). "
                "reply_all: reply to all recipients (requires email_id, message). "
                "search: regex search mailbox (requires query, optional folder)."
            ),
        },
        "address": {
            "oneOf": [
                {"type": "string"},
                {"type": "array", "items": {"type": "string"}},
            ],
            "description": "Target address(es) for send",
        },
        "cc": {
            "type": "array",
            "items": {"type": "string"},
            "description": "CC addresses — visible to all recipients",
        },
        "bcc": {
            "type": "array",
            "items": {"type": "string"},
            "description": "BCC addresses — hidden from other recipients",
        },
        "attachments": {
            "type": "array",
            "items": {"type": "string"},
            "description": "File paths to attach (for send)",
        },
        "subject": {"type": "string", "description": "Email subject line"},
        "message": {"type": "string", "description": "Email body"},
        "email_id": {
            "type": "string",
            "description": "Email ID for read/reply/reply_all (get from check)",
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
        "type": {
            "type": "string",
            "enum": ["normal", "cancel"],
            "description": (
                "Mail type (for send). 'normal' (default) is regular mail. "
                "'cancel' stops the target agent immediately (requires admin privilege)."
            ),
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Full email client — filesystem-based mailbox with inbox/sent folders, "
    "reply, reply-all, CC/BCC, attachments, and regex search. "
    "Use 'send' for outgoing email (saved to sent/). "
    "'check' to list inbox or sent (optional folder param). "
    "'read' to read by ID. "
    "'reply'/'reply_all' to respond. "
    "'search' to find emails by regex (searches from, subject, message). "
    "Attachments are stored alongside emails in the mailbox. "
    "Etiquette: a short acknowledgement is fine, but do not reply to "
    "an acknowledgement — that creates pointless ping-pong."
)


class EmailManager:
    """Filesystem-based email manager — reads/writes mailbox/ directory."""

    def __init__(self, agent: "BaseAgent"):
        self._agent = agent
        # Track last sent message per recipient to block identical consecutive sends.
        self._last_sent: dict[str, str] = {}  # address → message text

    @property
    def _mailbox_dir(self) -> Path:
        return self._agent._working_dir / "mailbox"

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
        # Sort by timestamp — received_at for inbox, sent_at for sent
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
            "preview": e.get("message", "")[:100],
            "time": e.get("received_at") or e.get("sent_at") or e.get("time") or "",
            "folder": e.get("_folder", ""),
        }
        # Only track unread for inbox
        if e.get("_folder") == "inbox":
            entry["unread"] = eid not in read_ids
        if e.get("cc"):
            entry["cc"] = e["cc"]
        return entry

    # ------------------------------------------------------------------
    # Action dispatch
    # ------------------------------------------------------------------

    def handle(self, args: dict) -> dict:
        action = args.get("action")
        if action == "send":
            return self._send(args)
        elif action == "check":
            return self._check(args)
        elif action == "read":
            return self._read(args)
        elif action == "reply":
            return self._reply(args)
        elif action == "reply_all":
            return self._reply_all(args)
        elif action == "search":
            return self._search(args)
        else:
            return {"error": f"Unknown email action: {action}"}

    # ------------------------------------------------------------------
    # Send — deliver + save to sent/
    # ------------------------------------------------------------------

    def _send(self, args: dict) -> dict:
        raw_address = args.get("address", "")
        subject = args.get("subject", "")
        message_text = args.get("message", "")
        mail_type = args.get("type", "normal")
        cc = args.get("cc") or []
        bcc = args.get("bcc") or []

        # Privilege gate: only admin agents can send non-normal mail
        if mail_type != "normal" and not self._agent._admin:
            return {"error": f"Not authorized to send type={mail_type!r} mail (requires admin=True)"}

        if isinstance(raw_address, str):
            to_list = [raw_address] if raw_address else []
        else:
            to_list = list(raw_address)

        if not to_list:
            return {"error": "address is required"}
        if self._agent._mail_service is None:
            return {"error": "mail service not configured"}

        # Block identical consecutive messages to the same recipient.
        all_targets = to_list + cc + bcc
        duplicates = [
            addr for addr in all_targets
            if self._last_sent.get(addr) == message_text
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

        sender = self._agent._mail_service.address or self._agent.agent_id

        # Build visible payload (no bcc field)
        base_payload = {
            "from": sender,
            "to": to_list,
            "subject": subject,
            "message": message_text,
            "type": mail_type,
        }
        if cc:
            base_payload["cc"] = cc
        attachments = args.get("attachments", [])
        if attachments:
            base_payload["attachments"] = attachments

        # Deliver to all recipients: to + cc + bcc
        all_recipients = to_list + cc + bcc
        delivered = []
        refused = []
        for addr in all_recipients:
            ok = self._agent._mail_service.send(addr, base_payload)
            if ok:
                delivered.append(addr)
            else:
                refused.append(addr)

        # Save to sent/ (includes bcc for sender's records)
        sent_id = str(uuid4())
        sent_dir = self._mailbox_dir / "sent" / sent_id
        sent_dir.mkdir(parents=True, exist_ok=True)
        sent_record = {
            **base_payload,
            "_mailbox_id": sent_id,
            "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        }
        if bcc:
            sent_record["bcc"] = bcc
        (sent_dir / "message.json").write_text(
            json.dumps(sent_record, indent=2, default=str)
        )

        # Track last sent message per recipient for duplicate detection.
        for addr in all_recipients:
            self._last_sent[addr] = message_text

        self._agent._log(
            "email_sent", to=to_list, cc=cc, bcc=bcc,
            subject=subject, message=message_text,
            delivered=delivered, refused=refused,
        )

        if not refused:
            return {"status": "delivered", "to": to_list, "cc": cc, "bcc": bcc}
        elif not delivered:
            return {"status": "refused", "error": "Could not deliver to any recipient", "refused": refused}
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
        email_id = args.get("email_id", "")
        if not email_id:
            return {"error": "email_id is required"}

        data = self._load_email(email_id)
        if data is None:
            return {"error": f"Email not found: {email_id}"}

        # Mark as read (inbox only)
        if data.get("_folder") == "inbox":
            self._mark_read(email_id)

        result = {
            "status": "ok",
            "id": email_id,
            "from": data.get("from", ""),
            "to": data.get("to", []),
            "subject": data.get("subject", "(no subject)"),
            "message": data.get("message", ""),
            "time": data.get("received_at") or data.get("sent_at") or data.get("time") or "",
            "folder": data.get("_folder", ""),
        }
        if data.get("cc"):
            result["cc"] = data["cc"]
        if data.get("attachments"):
            result["attachments"] = data["attachments"]
        return result

    # ------------------------------------------------------------------
    # Lookup — used by reply/reply_all
    # ------------------------------------------------------------------

    def _lookup(self, email_id: str) -> dict | None:
        return self._load_email(email_id)

    # ------------------------------------------------------------------
    # Reply, Reply All
    # ------------------------------------------------------------------

    def _reply(self, args: dict) -> dict:
        email_id = args.get("email_id", "")
        if not email_id:
            return {"error": "email_id is required for reply"}
        message_text = args.get("message", "")
        if not message_text:
            return {"error": "message is required for reply"}

        original = self._lookup(email_id)
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
            "cc": args.get("cc") or [],
            "bcc": args.get("bcc") or [],
        })

    def _reply_all(self, args: dict) -> dict:
        email_id = args.get("email_id", "")
        if not email_id:
            return {"error": "email_id is required for reply_all"}
        message_text = args.get("message", "")
        if not message_text:
            return {"error": "message is required for reply_all"}

        original = self._lookup(email_id)
        if original is None:
            return {"error": f"Email not found: {email_id}"}

        my_address = (
            self._agent._mail_service.address
            if self._agent._mail_service
            else self._agent.agent_id
        )

        reply_to = original["from"]
        orig_to = original.get("to") or []
        if isinstance(orig_to, str):
            orig_to = [orig_to]
        orig_cc = original.get("cc") or []
        other_recipients = [
            addr for addr in orig_to + orig_cc
            if addr != my_address and addr != reply_to
        ]

        extra_cc = args.get("cc") or []
        extra_bcc = args.get("bcc") or []

        orig_subject = original.get("subject", "")
        subject = args.get("subject") or (
            orig_subject if orig_subject.startswith("Re: ") else f"Re: {orig_subject}"
        )

        return self._send({
            "address": reply_to,
            "subject": subject,
            "message": message_text,
            "cc": other_recipients + extra_cc,
            "bcc": extra_bcc,
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
    # Receive interception
    # ------------------------------------------------------------------

    def on_normal_mail(self, payload: dict) -> None:
        """Handle normal mail — save to mailbox and notify agent.

        Replaces BaseAgent._on_normal_mail when the email capability is active.
        Cancel-type emails never reach this method — they are handled by
        BaseAgent._on_mail_received before delegation.
        """
        # Use _mailbox_id from mail service, or fall back to generating one
        email_id = payload.get("_mailbox_id") or str(uuid4())

        sender = payload.get("from", "unknown")
        to = payload.get("to") or []
        if isinstance(to, str):
            to = [to]
        cc = payload.get("cc") or []
        subject = payload.get("subject", "(no subject)")
        message = payload.get("message", "")

        # Send notification to agent inbox (not the full content)
        preview = message[:80].replace("\n", " ")
        notification = (
            f'[New email from {sender}]\n'
            f'  Subject: {subject}\n'
            f'  Preview: {preview}...\n'
            f'  ID: {email_id}\n'
            f'Use email(action="read", email_id="{email_id}") to read full message. '
            f'Use email(action="reply", email_id="{email_id}", message="...") to reply.'
        )

        from ..message import _make_message, MSG_REQUEST
        self._agent._log("email_received", sender=sender, to=to, cc=cc, subject=subject, message=message)
        msg = _make_message(MSG_REQUEST, sender, notification)
        self._agent.inbox.put(msg)


def setup(agent: "BaseAgent") -> EmailManager:
    """Set up email capability — filesystem-based mailbox."""
    mgr = EmailManager(agent)
    agent._on_normal_mail = mgr.on_normal_mail
    agent.add_tool(
        "email", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )
    return mgr
