"""IMAPMailManager — tool handler for multi-account IMAP email.

Registers a single ``imap`` tool with the agent and routes actions to the
correct :class:`IMAPAccount` via :class:`IMAPMailService`.

Storage layout (per-account):
    working_dir/imap/{address}/{folder}/{uid}/message.json  — fetched emails
    working_dir/imap/{address}/contacts.json                — contact book
"""
from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent
    from .account import IMAPAccount
    from .service import IMAPMailService


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def parse_email_id(email_id: str) -> tuple[str, str, str]:
    """Split ``account:folder:uid`` compound key.

    Uses first colon for account, last colon for uid so folder names
    containing colons (rare) or slashes (common, e.g. ``[Gmail]/Sent Mail``)
    are handled correctly.

    Examples::

        >>> parse_email_id("alice@gmail.com:INBOX:1042")
        ('alice@gmail.com', 'INBOX', '1042')
        >>> parse_email_id("a@b.com:[Gmail]/Sent Mail:999")
        ('a@b.com', '[Gmail]/Sent Mail', '999')
    """
    account, _, remainder = email_id.partition(":")
    folder, _, uid = remainder.rpartition(":")
    return account, folder, uid


# ---------------------------------------------------------------------------
# Tool schema
# ---------------------------------------------------------------------------

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": [
                "send", "check", "read", "reply", "search",
                "delete", "move", "flag", "folders",
                "contacts", "add_contact", "remove_contact", "edit_contact",
                "accounts",
            ],
            "description": (
                "send: send email via IMAP/SMTP (requires address, message; optional subject, cc, bcc, attachments). "
                "check: list recent envelopes from a folder (optional folder, n). "
                "read: fetch full email by ID list (email_id=[id1, ...]). "
                "You are encouraged to read multiple relevant or even all unread emails and think before acting. "
                "reply: reply to an email (requires email_id, message; optional cc). "
                "search: server-side IMAP search (requires query, optional folder). "
                "delete: delete email(s) by ID (email_id). "
                "move: move email(s) to another folder (email_id, folder=destination). "
                "flag: set/clear flags on email(s) (email_id, flags={flag: bool}). "
                "folders: list available IMAP folders. "
                "contacts: list all contacts. "
                "add_contact: add/update contact (requires address, name; optional note). "
                "remove_contact: remove contact (requires address). "
                "edit_contact: update contact fields (requires address; optional name, note). "
                "accounts: list configured IMAP accounts and connection status."
            ),
        },
        "account": {
            "type": "string",
            "description": "Which account to use (email address). Defaults to the primary account.",
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
        "cc": {
            "oneOf": [
                {"type": "string"},
                {"type": "array", "items": {"type": "string"}},
            ],
            "description": "CC address(es)",
        },
        "bcc": {
            "oneOf": [
                {"type": "string"},
                {"type": "array", "items": {"type": "string"}},
            ],
            "description": "BCC address(es)",
        },
        "email_id": {
            "oneOf": [
                {"type": "string"},
                {"type": "array", "items": {"type": "string"}},
            ],
            "description": "Email ID(s) — compound key: account:folder:uid",
        },
        "n": {
            "type": "integer",
            "description": "Max recent emails to show (for check, default 10)",
            "default": 10,
        },
        "query": {
            "type": "string",
            "description": "IMAP search query (e.g. from:addr subject:text unseen since:YYYY-MM-DD)",
        },
        "folder": {
            "type": "string",
            "description": "IMAP folder name (e.g. INBOX, [Gmail]/Sent Mail). For move: destination folder.",
        },
        "flags": {
            "type": "object",
            "description": "Dict of flag name to bool — e.g. {\"seen\": true, \"flagged\": false}",
        },
        "name": {
            "type": "string",
            "description": "Contact's human-readable name (for add_contact, edit_contact)",
        },
        "note": {
            "type": "string",
            "description": "Free-text note about the contact (for add_contact, edit_contact)",
        },
        "attachments": {
            "type": "array",
            "items": {"type": "string"},
            "description": "List of file paths to attach (absolute or relative to working dir).",
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "IMAP email client — real email via IMAP/SMTP with multi-account support. "
    "Every response includes account and tcp_alias fields. "
    "Actions: send, check, read, reply, search, delete, move, flag, folders, "
    "contacts, add_contact, remove_contact, edit_contact, accounts. "
    "Email IDs use compound key format: account:folder:uid.\n"
    "REPLY POLICY: "
    "When a human contacts you via internal email (email tool), reply via internal email. "
    "When you receive an IMAP email from an external address, do NOT reply unless: "
    "(1) you have explicit guidance on how to handle IMAP replies, or "
    "(2) you can confirm the sender is the same human who contacts you via internal email. "
    "Unknown external senders require confirmation from your human before replying."
)


# ---------------------------------------------------------------------------
# Flag name mapping (friendly name → IMAP system flag)
# ---------------------------------------------------------------------------

_FLAG_MAP: dict[str, str] = {
    "seen": "\\Seen",
    "flagged": "\\Flagged",
    "answered": "\\Answered",
    "deleted": "\\Deleted",
    "draft": "\\Draft",
}


# ---------------------------------------------------------------------------
# IMAPMailManager
# ---------------------------------------------------------------------------

class IMAPMailManager:
    """Tool handler for multi-account IMAP email.

    Registers a single ``imap`` tool and routes actions to the correct
    :class:`IMAPAccount` via :class:`IMAPMailService`.
    """

    def __init__(
        self,
        agent: "BaseAgent",
        service: "IMAPMailService",
        tcp_alias: str,
    ) -> None:
        self._agent = agent
        self._service = service
        self._tcp_alias = tcp_alias
        self._bridge = None  # set by setup() before start()
        # Duplicate send protection — maps address → (message_text, count)
        self._last_sent: dict[str, tuple[str, int]] = {}
        self._dup_free_passes = 2

    # ------------------------------------------------------------------
    # Meta injection
    # ------------------------------------------------------------------

    def _inject_meta(self, result: dict) -> dict:
        """Add tcp_alias and account to every response."""
        result["tcp_alias"] = self._tcp_alias
        result["account"] = self._service.default_account.address
        return result

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        """Start IMAP poll and TCP bridge listener."""
        self._service.listen(on_message=self.on_imap_received)

        if self._bridge is not None:
            def on_bridge_mail(payload: dict) -> None:
                to = payload.get("to", [])
                if isinstance(to, str):
                    to = [to]
                if not to:
                    return
                for addr in to:
                    self._service.send(addr, payload)

            self._bridge.listen(on_message=on_bridge_mail)

    def stop(self) -> None:
        """Stop IMAP poll and TCP bridge."""
        self._service.stop()
        if self._bridge is not None:
            self._bridge.stop()

    # ------------------------------------------------------------------
    # Action dispatch
    # ------------------------------------------------------------------

    def handle(self, args: dict) -> dict:
        action = args.get("action")
        account = self._service.get_account(args.get("account"))
        if account is None and action != "accounts":
            return self._inject_meta(
                {"error": f"Unknown account: {args.get('account')}"}
            )

        dispatch = {
            "send": self._send,
            "check": self._check,
            "read": self._read,
            "reply": self._reply,
            "search": self._search,
            "delete": self._delete,
            "move": self._move,
            "flag": self._flag,
            "folders": self._folders,
            "contacts": self._contacts,
            "add_contact": self._add_contact,
            "remove_contact": self._remove_contact,
            "edit_contact": self._edit_contact,
        }

        if action == "accounts":
            return self._inject_meta(self._accounts(args))
        elif action in dispatch:
            return self._inject_meta(dispatch[action](args, account))
        else:
            return self._inject_meta({"error": f"Unknown imap action: {action}"})

    # ------------------------------------------------------------------
    # Receive handler — called by IMAPMailService IMAP poll
    # ------------------------------------------------------------------

    def on_imap_received(self, payload: dict) -> None:
        """Handle incoming email notification from account. Notify agent."""
        account_addr = payload.get("account", "")
        email_id = payload.get("email_id", "")
        sender = payload.get("from", "unknown")
        subject = payload.get("subject", "(no subject)")
        message = payload.get("message", "")

        self._agent.wake("mail_arrived")

        if len(message) > 200:
            preview = message[:200].replace("\n", " ") + f"... ({len(message) - 200} more chars)"
        else:
            preview = message.replace("\n", " ")
        notification = (
            f'[system] New message in imap box.\n'
            f'  From: {sender}\n'
            f'  Subject: {subject}\n'
            f'  Email ID: {email_id}\n'
            f'  {preview}\n'
            f'Use imap(action="check") to see your inbox.'
        )

        self._agent.log(
            "imap_received", sender=sender, subject=subject,
            account=account_addr, email_id=email_id,
        )
        self._agent.notify("system", notification)

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _normalize_email_ids(args: dict) -> list[str]:
        """Normalize email_id param to list.

        Handles: single string, list of strings, or a JSON-encoded array
        string (LLMs sometimes wrap the value in ``[...]``).
        """
        ids = args.get("email_id", [])
        if isinstance(ids, str):
            stripped = ids.strip()
            if stripped.startswith("["):
                import json
                try:
                    ids = json.loads(stripped)
                except (json.JSONDecodeError, ValueError):
                    ids = [ids]
            else:
                ids = [ids]
        return ids

    @staticmethod
    def _normalize_addresses(raw: str | list | None) -> list[str]:
        """Normalize address param to list."""
        if raw is None:
            return []
        if isinstance(raw, str):
            return [raw] if raw else []
        return list(raw)

    def _contacts_path(self, account: "IMAPAccount") -> Path:
        """Per-account contacts path: imap/{address}/contacts.json."""
        return self._agent.working_dir / "imap" / account.address / "contacts.json"

    def _load_contacts(self, account: "IMAPAccount") -> list[dict]:
        path = self._contacts_path(account)
        if path.is_file():
            try:
                return json.loads(path.read_text())
            except (json.JSONDecodeError, OSError):
                return []
        return []

    def _save_contacts(self, account: "IMAPAccount", contacts: list[dict]) -> None:
        path = self._contacts_path(account)
        path.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp = tempfile.mkstemp(dir=str(path.parent), suffix=".tmp")
        try:
            os.write(fd, json.dumps(contacts, indent=2).encode())
            os.close(fd)
            os.replace(tmp, str(path))
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise

    # ------------------------------------------------------------------
    # Actions
    # ------------------------------------------------------------------

    def _send(self, args: dict, account: "IMAPAccount") -> dict:
        to_list = self._normalize_addresses(args.get("address"))
        subject = args.get("subject", "")
        message_text = args.get("message", "")
        cc = self._normalize_addresses(args.get("cc"))
        bcc = self._normalize_addresses(args.get("bcc"))
        raw_attachments = args.get("attachments", [])

        # Resolve attachment paths (relative -> absolute from working dir)
        attachments: list[str] = []
        for p in raw_attachments:
            path = Path(p)
            if not path.is_absolute():
                path = self._agent.working_dir / p
            attachments.append(str(path))

        if not to_list:
            return {"error": "address is required"}

        # Block identical consecutive messages to the same recipient
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

        err = account.send_email(
            to=to_list,
            subject=subject,
            body=message_text,
            cc=cc or None,
            bcc=bcc or None,
            attachments=attachments or None,
        )

        # Track last sent message per recipient for duplicate detection
        for addr in to_list:
            prev = self._last_sent.get(addr)
            if prev is not None and prev[0] == message_text:
                self._last_sent[addr] = (message_text, prev[1] + 1)
            else:
                self._last_sent[addr] = (message_text, 1)

        self._agent.log(
            "imap_sent", to=to_list, subject=subject, message=message_text,
        )

        if err is None:
            return {"status": "delivered", "to": to_list}
        else:
            return {"status": "error", "error": err}

    def _check(self, args: dict, account: "IMAPAccount") -> dict:
        folder = args.get("folder", "INBOX")
        n = args.get("n", 10)
        envelopes = account.fetch_envelopes(folder, n)
        return {"status": "ok", "total": len(envelopes), "emails": envelopes}

    def _read(self, args: dict, account: "IMAPAccount") -> dict:
        ids = self._normalize_email_ids(args)
        if not ids:
            return {"error": "email_id is required"}

        results: list[dict] = []
        errors: list[str] = []
        for eid in ids:
            acct_addr, folder, uid = parse_email_id(eid)
            # Use the account from the email_id if different
            target = self._service.get_account(acct_addr) or account
            data = target.fetch_full(folder, uid)
            if data is None:
                errors.append(eid)
                continue

            # Persist to disk: imap/{address}/{folder}/{uid}/message.json
            persist_dir = (
                self._agent.working_dir / "imap"
                / acct_addr / folder / uid
            )
            persist_dir.mkdir(parents=True, exist_ok=True)

            # Save attachments to disk
            attachments_raw = data.get("attachments_raw", [])
            saved_attachments: list[dict] = []
            for att in attachments_raw:
                att_path = persist_dir / att["filename"]
                att_path.write_bytes(att["data"])
                saved_attachments.append({
                    "filename": att["filename"],
                    "content_type": att["content_type"],
                    "size": len(att["data"]),
                    "path": str(att_path),
                })

            # Build the persisted record (exclude raw binary data)
            record = {
                "email_id": eid,
                "uid": uid,
                "from": data.get("from", ""),
                "from_address": data.get("from_address", ""),
                "to": data.get("to", ""),
                "cc": data.get("cc", ""),
                "subject": data.get("subject", ""),
                "date": data.get("date", ""),
                "message": data.get("body", ""),
                "message_id": data.get("message_id", ""),
                "references": data.get("references", ""),
                "flags": data.get("flags", []),
                "attachments": saved_attachments or data.get("attachments", []),
            }
            (persist_dir / "message.json").write_text(
                json.dumps(record, indent=2, default=str)
            )

            results.append(record)

        result = {"status": "ok", "emails": results}
        if errors:
            result["not_found"] = errors
        return result

    def _reply(self, args: dict, account: "IMAPAccount") -> dict:
        ids = self._normalize_email_ids(args)
        if not ids:
            return {"error": "email_id is required for reply"}
        email_id = ids[0]
        message_text = args.get("message", "")
        if not message_text:
            return {"error": "message is required for reply"}

        acct_addr, folder, uid = parse_email_id(email_id)
        target = self._service.get_account(acct_addr) or account

        original = target.fetch_full(folder, uid)
        if original is None:
            return {"error": f"Email not found: {email_id}"}

        # Build reply subject
        orig_subject = original.get("subject", "")
        subject = args.get("subject") or (
            orig_subject if orig_subject.startswith("Re: ") else f"Re: {orig_subject}"
        )

        # Threading headers
        orig_message_id = original.get("message_id", "")
        orig_references = original.get("references", "")
        in_reply_to = orig_message_id
        references = (orig_references + " " + orig_message_id).strip()

        # CC
        cc = self._normalize_addresses(args.get("cc"))

        # Reply to sender
        reply_to = original.get("from_address") or original.get("from", "")
        err = target.send_email(
            to=[reply_to],
            subject=subject,
            body=message_text,
            cc=cc or None,
            in_reply_to=in_reply_to or None,
            references=references or None,
        )

        # Mark as answered
        target.store_flags(folder, uid, ["\\Answered"])

        self._agent.log(
            "imap_sent", to=[reply_to], subject=subject, message=message_text,
            in_reply_to=email_id,
        )

        if err is None:
            return {"status": "delivered", "to": [reply_to], "in_reply_to": email_id}
        else:
            return {"status": "error", "error": err}

    def _search(self, args: dict, account: "IMAPAccount") -> dict:
        query = args.get("query", "")
        if not query:
            return {"error": "query is required for search"}

        folder = args.get("folder", "INBOX")
        uids = account.search(folder, query)
        if not uids:
            return {"status": "ok", "total": 0, "emails": []}

        headers = account.fetch_headers_by_uids(folder, uids)
        return {"status": "ok", "total": len(headers), "emails": headers}

    def _delete(self, args: dict, account: "IMAPAccount") -> dict:
        ids = self._normalize_email_ids(args)
        if not ids:
            return {"error": "email_id is required"}

        results: list[dict] = []
        for eid in ids:
            acct_addr, folder, uid = parse_email_id(eid)
            target = self._service.get_account(acct_addr) or account
            ok = target.delete_message(folder, uid)
            results.append({"email_id": eid, "deleted": ok})

        return {"status": "ok", "results": results}

    def _move(self, args: dict, account: "IMAPAccount") -> dict:
        ids = self._normalize_email_ids(args)
        if not ids:
            return {"error": "email_id is required"}
        dest_folder = args.get("folder", "")
        if not dest_folder:
            return {"error": "folder (destination) is required for move"}

        results: list[dict] = []
        for eid in ids:
            acct_addr, folder, uid = parse_email_id(eid)
            target = self._service.get_account(acct_addr) or account
            ok = target.move_message(folder, uid, dest_folder)
            results.append({"email_id": eid, "moved": ok})

        return {"status": "ok", "results": results}

    def _flag(self, args: dict, account: "IMAPAccount") -> dict:
        ids = self._normalize_email_ids(args)
        if not ids:
            return {"error": "email_id is required"}
        flags_dict = args.get("flags", {})
        if not flags_dict:
            return {"error": "flags is required"}

        # Convert dict of {flag_name: bool} to +FLAGS / -FLAGS calls
        add_flags: list[str] = []
        remove_flags: list[str] = []
        for name, value in flags_dict.items():
            imap_flag = _FLAG_MAP.get(name.lower(), f"\\{name.capitalize()}")
            if value:
                add_flags.append(imap_flag)
            else:
                remove_flags.append(imap_flag)

        results: list[dict] = []
        for eid in ids:
            acct_addr, folder, uid = parse_email_id(eid)
            target = self._service.get_account(acct_addr) or account
            ok = True
            if add_flags:
                ok = ok and target.store_flags(folder, uid, add_flags, action="+FLAGS")
            if remove_flags:
                ok = ok and target.store_flags(folder, uid, remove_flags, action="-FLAGS")
            results.append({"email_id": eid, "flagged": ok})

        return {"status": "ok", "results": results}

    def _folders(self, args: dict, account: "IMAPAccount") -> dict:
        raw = account.list_folders()
        folders = [{"name": name, "role": role} for name, role in raw.items()]
        return {"status": "ok", "folders": folders}

    def _accounts(self, args: dict) -> dict:
        acct_list = []
        for acct in self._service.accounts:
            acct_list.append({
                "address": acct.address,
                "connected": acct.connected,
            })
        return {"status": "ok", "accounts": acct_list}

    def _contacts(self, args: dict, account: "IMAPAccount") -> dict:
        return {"status": "ok", "contacts": self._load_contacts(account)}

    def _add_contact(self, args: dict, account: "IMAPAccount") -> dict:
        address = args.get("address", "")
        name = args.get("name", "")
        if not address:
            return {"error": "address is required"}
        if not name:
            return {"error": "name is required"}
        note = args.get("note", "")

        contacts = self._load_contacts(account)
        for c in contacts:
            if c["address"] == address:
                c["name"] = name
                c["note"] = note
                self._save_contacts(account, contacts)
                return {"status": "updated", "contact": c}
        entry = {"address": address, "name": name, "note": note}
        contacts.append(entry)
        self._save_contacts(account, contacts)
        return {"status": "added", "contact": entry}

    def _remove_contact(self, args: dict, account: "IMAPAccount") -> dict:
        address = args.get("address", "")
        if not address:
            return {"error": "address is required"}
        contacts = self._load_contacts(account)
        new_contacts = [c for c in contacts if c["address"] != address]
        if len(new_contacts) == len(contacts):
            return {"error": f"Contact not found: {address}"}
        self._save_contacts(account, new_contacts)
        return {"status": "removed", "address": address}

    def _edit_contact(self, args: dict, account: "IMAPAccount") -> dict:
        address = args.get("address", "")
        if not address:
            return {"error": "address is required"}
        contacts = self._load_contacts(account)
        for c in contacts:
            if c["address"] == address:
                if "name" in args:
                    c["name"] = args["name"]
                if "note" in args:
                    c["note"] = args["note"]
                self._save_contacts(account, contacts)
                return {"status": "updated", "contact": c}
        return {"error": f"Contact not found: {address}"}
