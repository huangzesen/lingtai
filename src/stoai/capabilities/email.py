"""Email capability — mailbox, reply, reply_all, CC/BCC on top of mail.

Upgrades the base mail intrinsic (FIFO queue) with:
- Persistent mailbox (stored messages with IDs)
- check: list mailbox with filtering
- read: read specific message by ID
- reply: auto-fill address/subject from original
- reply_all: reply to all recipients minus self
- send: multi-to with CC/BCC
- Receive interception: boxes incoming messages, notifies agent inbox

Usage:
    agent.add_capability("email")
"""
from __future__ import annotations

import threading
from datetime import datetime, timezone
from typing import TYPE_CHECKING
from uuid import uuid4

if TYPE_CHECKING:
    from ..agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["send", "check", "read", "reply", "reply_all"],
            "description": (
                "send: send with optional cc/bcc (requires address, message). "
                "check: list mailbox (optional n for max recent). "
                "read: read email by ID (requires email_id). "
                "reply: reply to email (requires email_id, message). "
                "reply_all: reply to all recipients (requires email_id, message)."
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
            "description": "Max recent emails to show (for check)",
            "default": 10,
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Full email client — mailbox with persistent storage, reply, reply-all, "
    "CC/BCC. Use 'send' for multi-recipient email, 'check' to list mailbox, "
    "'read' to read by ID, 'reply'/'reply_all' for conversation."
)


class EmailManager:
    """Full email manager — mailbox, reply, CC/BCC on top of base mail."""

    def __init__(self, agent: "BaseAgent"):
        self._agent = agent
        self._mailbox: list[dict] = []
        self._mailbox_lock = threading.Lock()

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
        else:
            return {"error": f"Unknown email action: {action}"}

    # ------------------------------------------------------------------
    # Send with multi-to, CC, BCC
    # ------------------------------------------------------------------

    def _send(self, args: dict) -> dict:
        raw_address = args.get("address", "")
        subject = args.get("subject", "")
        message_text = args.get("message", "")
        cc = args.get("cc") or []
        bcc = args.get("bcc") or []

        if isinstance(raw_address, str):
            to_list = [raw_address] if raw_address else []
        else:
            to_list = list(raw_address)

        if not to_list:
            return {"error": "address is required"}
        if self._agent._mail_service is None:
            return {"error": "mail service not configured"}

        sender = self._agent._mail_service.address or self._agent.agent_id

        # Build visible payload (no bcc field)
        base_payload = {
            "from": sender,
            "to": to_list,
            "subject": subject,
            "message": message_text,
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
    # Mailbox: check, read
    # ------------------------------------------------------------------

    def _check(self, args: dict) -> dict:
        n = args.get("n", 10)
        with self._mailbox_lock:
            total = len(self._mailbox)
            recent = self._mailbox[-n:] if n > 0 else self._mailbox[:]
        emails = []
        for e in reversed(recent):  # newest first
            entry = {
                "id": e["id"],
                "from": e["from"],
                "to": e.get("to", []),
                "subject": e.get("subject", "(no subject)"),
                "preview": e["message"][:100],
                "time": e["time"],
                "unread": e.get("unread", False),
            }
            if e.get("cc"):
                entry["cc"] = e["cc"]
            emails.append(entry)
        return {"status": "ok", "total": total, "showing": len(emails), "emails": emails}

    def _read(self, args: dict) -> dict:
        email_id = args.get("email_id", "")
        if not email_id:
            return {"error": "email_id is required"}
        with self._mailbox_lock:
            for e in self._mailbox:
                if e["id"] == email_id:
                    e["unread"] = False
                    result = {
                        "status": "ok",
                        "id": e["id"],
                        "from": e["from"],
                        "to": e.get("to", []),
                        "subject": e.get("subject", "(no subject)"),
                        "message": e["message"],
                        "time": e["time"],
                    }
                    if e.get("cc"):
                        result["cc"] = e["cc"]
                    if e.get("attachments"):
                        result["attachments"] = e["attachments"]
                    return result
        return {"error": f"Email not found: {email_id}"}

    # ------------------------------------------------------------------
    # Reply, Reply All
    # ------------------------------------------------------------------

    def _lookup(self, email_id: str) -> dict | None:
        with self._mailbox_lock:
            for e in self._mailbox:
                if e["id"] == email_id:
                    return dict(e)  # shallow copy
        return None

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
    # Receive interception
    # ------------------------------------------------------------------

    def on_mail_received(self, payload: dict) -> None:
        """Intercept incoming mail — store in mailbox, notify agent inbox."""
        sender = payload.get("from", "unknown")
        to = payload.get("to") or []
        if isinstance(to, str):
            to = [to]
        cc = payload.get("cc") or []
        subject = payload.get("subject", "(no subject)")
        message = payload.get("message", "")
        email_id = f"mail_{uuid4().hex[:8]}"
        timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

        attachments = payload.get("attachments") or []
        email_entry = {
            "id": email_id,
            "from": sender,
            "to": to,
            "subject": subject,
            "message": message,
            "time": timestamp,
            "unread": True,
        }
        if cc:
            email_entry["cc"] = cc
        if attachments:
            email_entry["attachments"] = attachments
        with self._mailbox_lock:
            self._mailbox.append(email_entry)

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

        from ..agent import _make_message, MSG_REQUEST
        self._agent._log("email_received", sender=sender, to=to, cc=cc, subject=subject, message=message)
        msg = _make_message(MSG_REQUEST, sender, notification)
        self._agent.inbox.put(msg)


def setup(agent: "BaseAgent") -> EmailManager:
    """Set up email capability — mailbox, reply, CC/BCC on top of mail.

    Intercepts the agent's mail receive callback to box messages.
    """
    mgr = EmailManager(agent)

    # Intercept the mail receive path — works regardless of start() order
    # because start() uses a lambda trampoline
    agent._on_mail_received = mgr.on_mail_received

    agent.add_tool(
        "email", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )
    agent.update_system_prompt(
        "email_instructions",
        "You have full email capabilities via the email tool. "
        "Use 'send' with CC/BCC for group messaging, "
        "'check' to list your mailbox, 'read' to read by ID, "
        "'reply' to respond, 'reply_all' to respond to everyone. "
        "You can attach files to emails using the 'attachments' parameter. "
        "Received attachments are stored in the mailbox. "
        "To use an attachment elsewhere, create a symlink to it — "
        "do not move the original file. "
        "For simple point-to-point messages, the mail tool also works.",
    )
    return mgr
