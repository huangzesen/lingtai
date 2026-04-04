"""FeishuManager — tool dispatch + filesystem persistence.

Storage layout:
    working_dir/feishu/{alias}/inbox/{uuid}/message.json
    working_dir/feishu/{alias}/sent/{uuid}/message.json
    working_dir/feishu/{alias}/contacts.json   open_id -> {alias, name, chat_id}
    working_dir/feishu/{alias}/read.json       list of read compound IDs
    working_dir/feishu/{alias}/state.json      bot_info

Compound message ID format: {alias}:{chat_id}:{feishu_message_id}
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
    from lingtai_kernel.base_agent import BaseAgent
    from .service import FeishuService

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": [
                "send", "check", "read", "reply", "search",
                "contacts", "add_contact", "remove_contact",
                "accounts",
            ],
            "description": (
                "send: send a text message to a user or chat "
                "(receive_id, receive_id_type, text; optional account). "
                "check: list recent conversations with unread counts "
                "(optional account). "
                "read: read messages from a specific chat "
                "(chat_id; optional limit, account). "
                "reply: reply to a specific message "
                "(message_id from read results, text). "
                "search: search inbox messages by regex "
                "(query; optional account, chat_id). "
                "contacts: list saved contacts (optional account). "
                "add_contact: save a contact "
                "(open_id, alias; optional name, chat_id). "
                "remove_contact: remove a contact (alias or open_id). "
                "accounts: list configured app accounts."
            ),
        },
        "account": {
            "type": "string",
            "description": (
                "App account alias (optional — defaults to first configured account)"
            ),
        },
        "receive_id": {
            "type": "string",
            "description": (
                "Recipient ID — open_id, user_id, email, or chat_id "
                "depending on receive_id_type"
            ),
        },
        "receive_id_type": {
            "type": "string",
            "enum": ["open_id", "user_id", "email", "chat_id", "union_id"],
            "description": (
                "Type of receive_id. Use 'open_id' for individual users "
                "(format: ou_xxx), 'chat_id' for group chats (format: oc_xxx). "
                "Defaults to 'open_id'."
            ),
        },
        "chat_id": {
            "type": "string",
            "description": "Feishu chat ID (oc_xxx for groups, or open_id for p2p)",
        },
        "text": {
            "type": "string",
            "description": "Message text content",
        },
        "message_id": {
            "type": "string",
            "description": (
                "Compound message ID returned by read/check: "
                "{alias}:{chat_id}:{feishu_message_id}"
            ),
        },
        "limit": {
            "type": "integer",
            "description": "Max messages to return (for read, default 10)",
            "default": 10,
        },
        "query": {
            "type": "string",
            "description": "Search query (regex pattern)",
        },
        "open_id": {
            "type": "string",
            "description": "Feishu open_id for a user (ou_xxx)",
        },
        "alias": {
            "type": "string",
            "description": "Human-friendly contact alias",
        },
        "name": {
            "type": "string",
            "description": "Display name for a contact",
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Feishu (Lark) bot client — interact with Feishu users and group chats. "
    "Use 'send' for outgoing text messages (specify receive_id + receive_id_type). "
    "'check' to see recent conversations with unread counts. "
    "'read' to read messages from a specific chat (returns compound message IDs). "
    "'reply' to respond to a message (use compound ID from read results). "
    "'search' to find messages by keyword or regex. "
    "'contacts' to manage saved contacts (open_id aliases). "
    "'accounts' to list configured app accounts."
)


class FeishuManager:
    """Tool handler + filesystem manager for the Feishu addon."""

    def __init__(
        self,
        agent: "BaseAgent",
        service: "FeishuService",
        working_dir: Path,
    ) -> None:
        self._agent = agent
        self._service = service
        self._working_dir = working_dir
        # Duplicate send protection: (alias, receive_id, text) -> count
        self._last_sent: dict[tuple[str, str, str], int] = {}
        self._dup_free_passes = 2

    def _account_dir(self, alias: str) -> Path:
        return self._working_dir / "feishu" / alias

    def _resolve_account(self, args: dict) -> str:
        return args.get("account") or self._service.default_account.alias

    @staticmethod
    def _parse_compound_id(compound_id: str) -> tuple[str, str, str]:
        """Parse '{alias}:{chat_id}:{feishu_message_id}' -> (alias, chat_id, msg_id)."""
        parts = compound_id.split(":", 2)
        if len(parts) != 3:
            raise ValueError(f"Invalid Feishu message ID format: {compound_id!r}")
        return parts[0], parts[1], parts[2]

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        self._service.start()

    def stop(self) -> None:
        self._service.stop()

    # ------------------------------------------------------------------
    # Action dispatch
    # ------------------------------------------------------------------

    def handle(self, args: dict) -> dict:
        action = args.get("action")
        try:
            if action == "send":
                return self._send(args)
            elif action == "check":
                return self._check(args)
            elif action == "read":
                return self._read(args)
            elif action == "reply":
                return self._reply(args)
            elif action == "search":
                return self._search(args)
            elif action == "contacts":
                return self._contacts(args)
            elif action == "add_contact":
                return self._add_contact(args)
            elif action == "remove_contact":
                return self._remove_contact(args)
            elif action == "accounts":
                return self._accounts()
            else:
                return {"error": f"Unknown feishu action: {action!r}"}
        except Exception as e:
            return {"error": str(e)}

    # ------------------------------------------------------------------
    # Incoming messages — called by FeishuService via on_message callback
    # ------------------------------------------------------------------

    def on_incoming(self, account_alias: str, data: object) -> None:
        """Persist an incoming Feishu message event to disk and notify agent."""
        try:
            event = getattr(data, "event", None)
            if event is None:
                return
            message = getattr(event, "message", None)
            sender = getattr(event, "sender", None)
            if message is None or sender is None:
                return

            feishu_msg_id: str = getattr(message, "message_id", "") or ""
            chat_id: str = getattr(message, "chat_id", "") or ""
            chat_type: str = getattr(message, "chat_type", "p2p") or "p2p"
            msg_type: str = getattr(message, "message_type", "text") or "text"
            content_str: str = getattr(message, "content", "{}") or "{}"
            create_time: str = getattr(message, "create_time", "") or ""
            parent_id: str = getattr(message, "parent_id", "") or ""

            sender_id = getattr(sender, "sender_id", None)
            open_id: str = (
                (getattr(sender_id, "open_id", "") or "") if sender_id else ""
            )

            text = ""
            try:
                content_data = json.loads(content_str)
                text = content_data.get("text", "")
            except (json.JSONDecodeError, AttributeError):
                text = content_str

            if create_time:
                try:
                    ts = int(create_time) / 1000
                    date_str = datetime.fromtimestamp(ts, tz=timezone.utc).strftime(
                        "%Y-%m-%dT%H:%M:%SZ"
                    )
                except (ValueError, OSError):
                    date_str = datetime.now(timezone.utc).strftime(
                        "%Y-%m-%dT%H:%M:%SZ"
                    )
            else:
                date_str = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

            compound_id = f"{account_alias}:{chat_id}:{feishu_msg_id}"

            payload = {
                "id": compound_id,
                "feishu_message_id": feishu_msg_id,
                "chat_id": chat_id,
                "chat_type": chat_type,
                "message_type": msg_type,
                "from_open_id": open_id,
                "text": text,
                "date": date_str,
                "parent_id": parent_id,
            }

            msg_uuid = str(uuid4())
            acct_dir = self._account_dir(account_alias)
            msg_dir = acct_dir / "inbox" / msg_uuid
            msg_dir.mkdir(parents=True, exist_ok=True)
            (msg_dir / "message.json").write_text(
                json.dumps(payload, indent=2, default=str),
                encoding="utf-8",
            )

            if open_id:
                self._upsert_contact(account_alias, open_id, chat_id)

        except Exception as exc:
            import logging as _logging
            _logging.getLogger(__name__).warning(
                "on_incoming processing error (%s): %s", account_alias, exc
            )
            return

        # Notify agent
        self._agent.wake("message_received")
        display_name = self._get_contact_name(account_alias, open_id) or open_id
        notification = (
            f"[system] New Feishu message from {display_name} via {account_alias}.\n"
            f'Use feishu(action="check") to see your messages.'
        )
        self._agent.notify("system", notification)
        self._agent.log(
            "feishu_received",
            sender=display_name,
            account=account_alias,
            text=text[:100],
        )

    # ------------------------------------------------------------------
    # Filesystem helpers
    # ------------------------------------------------------------------

    def _list_messages(self, account: str, folder: str = "inbox") -> list[dict]:
        """Load all messages from a folder, sorted by date (newest first)."""
        folder_dir = self._account_dir(account) / folder
        if not folder_dir.is_dir():
            return []
        messages = []
        for msg_dir in folder_dir.iterdir():
            msg_file = msg_dir / "message.json"
            if msg_dir.is_dir() and msg_file.is_file():
                try:
                    data = json.loads(msg_file.read_text(encoding="utf-8"))
                    data["_dir"] = str(msg_dir)
                    messages.append(data)
                except (json.JSONDecodeError, OSError):
                    continue
        messages.sort(key=lambda m: m.get("date", ""), reverse=True)
        return messages

    def _read_ids(self, account: str) -> set[str]:
        path = self._account_dir(account) / "read.json"
        if path.is_file():
            try:
                return set(json.loads(path.read_text(encoding="utf-8")))
            except (json.JSONDecodeError, OSError):
                return set()
        return set()

    def _mark_read(self, account: str, compound_ids: list[str]) -> None:
        ids = self._read_ids(account)
        ids.update(compound_ids)
        acct_dir = self._account_dir(account)
        acct_dir.mkdir(parents=True, exist_ok=True)
        target = acct_dir / "read.json"
        fd, tmp = tempfile.mkstemp(dir=str(acct_dir), suffix=".tmp")
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

    def _load_contacts(self, account: str) -> dict:
        path = self._account_dir(account) / "contacts.json"
        if path.is_file():
            try:
                return json.loads(path.read_text(encoding="utf-8"))
            except (json.JSONDecodeError, OSError):
                return {}
        return {}

    def _save_contacts(self, account: str, contacts: dict) -> None:
        acct_dir = self._account_dir(account)
        acct_dir.mkdir(parents=True, exist_ok=True)
        target = acct_dir / "contacts.json"
        fd, tmp = tempfile.mkstemp(dir=str(acct_dir), suffix=".tmp")
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

    def _upsert_contact(
        self, account: str, open_id: str, chat_id: str = ""
    ) -> None:
        contacts = self._load_contacts(account)
        existing = contacts.get(open_id, {})
        if not existing.get("chat_id") and chat_id:
            existing["chat_id"] = chat_id
        contacts[open_id] = existing
        self._save_contacts(account, contacts)

    def _get_contact_name(self, account: str, open_id: str) -> str:
        contacts = self._load_contacts(account)
        info = contacts.get(open_id, {})
        return info.get("name") or info.get("alias") or ""

    # ------------------------------------------------------------------
    # Actions
    # ------------------------------------------------------------------

    def _send(self, args: dict) -> dict:
        account = self._resolve_account(args)
        receive_id = args.get("receive_id", "")
        receive_id_type = args.get("receive_id_type", "open_id")
        text = args.get("text", "")

        if not receive_id:
            return {"error": "receive_id is required"}
        if not text:
            return {"error": "text is required"}

        dup_key = (account, receive_id, text)
        count = self._last_sent.get(dup_key, 0)
        if count >= self._dup_free_passes:
            return {
                "status": "blocked",
                "warning": "Identical message already sent. Think twice before repeating.",
            }

        acct = self._service.get_account(account)
        result = acct.send_text(receive_id, receive_id_type, text)

        self._last_sent[dup_key] = count + 1

        feishu_msg_id = result.get("message_id", "")
        chat_id = result.get("chat_id", receive_id)
        compound_id = f"{account}:{chat_id}:{feishu_msg_id}"
        sent_uuid = str(uuid4())
        sent_dir = self._account_dir(account) / "sent" / sent_uuid
        sent_dir.mkdir(parents=True, exist_ok=True)
        sent_record = {
            "id": compound_id,
            "feishu_message_id": feishu_msg_id,
            "to": {"receive_id": receive_id, "receive_id_type": receive_id_type},
            "chat_id": chat_id,
            "text": text,
            "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "status": "sent",
        }
        (sent_dir / "message.json").write_text(
            json.dumps(sent_record, indent=2, default=str),
            encoding="utf-8",
        )

        return {"status": "sent", "message_id": compound_id}

    def _check(self, args: dict) -> dict:
        account = self._resolve_account(args)
        messages = self._list_messages(account, "inbox")
        read_ids = self._read_ids(account)

        conversations: dict[str, dict] = {}
        for msg in messages:
            cid = msg.get("chat_id", "")
            if cid not in conversations:
                name = self._get_contact_name(account, msg.get("from_open_id", ""))
                conversations[cid] = {
                    "chat_id": cid,
                    "chat_type": msg.get("chat_type", "p2p"),
                    "last_from_open_id": msg.get("from_open_id", ""),
                    "last_from_name": name,
                    "last_text": (msg.get("text") or "")[:100],
                    "last_date": msg.get("date", ""),
                    "total": 0,
                    "unread": 0,
                }
            conversations[cid]["total"] += 1
            if msg.get("id") and msg["id"] not in read_ids:
                conversations[cid]["unread"] += 1

        return {
            "status": "ok",
            "total": len(messages),
            "conversations": list(conversations.values()),
        }

    def _read(self, args: dict) -> dict:
        account = self._resolve_account(args)
        chat_id = args.get("chat_id", "")
        limit = args.get("limit", 10)

        if not chat_id:
            return {"error": "chat_id is required"}

        messages = self._list_messages(account, "inbox")
        filtered = [m for m in messages if m.get("chat_id") == chat_id]
        recent = filtered[:limit]

        compound_ids = [m["id"] for m in recent if m.get("id")]
        if compound_ids:
            self._mark_read(account, compound_ids)

        cleaned = []
        for m in recent:
            name = self._get_contact_name(account, m.get("from_open_id", ""))
            cleaned.append({
                "id": m.get("id"),
                "feishu_message_id": m.get("feishu_message_id"),
                "chat_id": m.get("chat_id"),
                "chat_type": m.get("chat_type"),
                "from_open_id": m.get("from_open_id"),
                "from_name": name,
                "message_type": m.get("message_type"),
                "text": m.get("text"),
                "date": m.get("date"),
                "parent_id": m.get("parent_id"),
            })

        return {"status": "ok", "messages": cleaned}

    def _reply(self, args: dict) -> dict:
        compound_id = args.get("message_id", "")
        text = args.get("text", "")
        if not compound_id:
            return {"error": "message_id is required"}
        if not text:
            return {"error": "text is required"}

        alias, _chat_id, feishu_msg_id = self._parse_compound_id(compound_id)
        acct = self._service.get_account(alias)
        result = acct.reply_text(feishu_msg_id, text)

        new_msg_id = result.get("message_id", "")
        new_chat_id = result.get("chat_id", _chat_id)
        new_compound = f"{alias}:{new_chat_id}:{new_msg_id}"
        sent_uuid = str(uuid4())
        sent_dir = self._account_dir(alias) / "sent" / sent_uuid
        sent_dir.mkdir(parents=True, exist_ok=True)
        sent_record = {
            "id": new_compound,
            "feishu_message_id": new_msg_id,
            "reply_to": compound_id,
            "chat_id": new_chat_id,
            "text": text,
            "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "status": "sent",
        }
        (sent_dir / "message.json").write_text(
            json.dumps(sent_record, indent=2, default=str),
            encoding="utf-8",
        )

        return {"status": "sent", "message_id": new_compound}

    def _search(self, args: dict) -> dict:
        query = args.get("query", "")
        if not query:
            return {"error": "query is required"}
        account = self._resolve_account(args)
        target_chat = args.get("chat_id", "")

        try:
            pattern = re.compile(query, re.IGNORECASE)
        except re.error as e:
            return {"error": f"Invalid regex: {e}"}

        messages = self._list_messages(account, "inbox")
        matches = []
        for msg in messages:
            if target_chat and msg.get("chat_id") != target_chat:
                continue
            name = self._get_contact_name(account, msg.get("from_open_id", ""))
            searchable = " ".join([
                msg.get("from_open_id", ""),
                name,
                msg.get("text", ""),
            ])
            if pattern.search(searchable):
                matches.append({
                    "id": msg.get("id"),
                    "from_open_id": msg.get("from_open_id"),
                    "from_name": name,
                    "chat_id": msg.get("chat_id"),
                    "date": msg.get("date"),
                    "text": msg.get("text"),
                })

        return {"status": "ok", "total": len(matches), "messages": matches}

    def _contacts(self, args: dict) -> dict:
        account = self._resolve_account(args)
        return {"status": "ok", "contacts": self._load_contacts(account)}

    def _add_contact(self, args: dict) -> dict:
        account = self._resolve_account(args)
        open_id = args.get("open_id", "")
        alias = args.get("alias", "")
        if not open_id:
            return {"error": "open_id is required"}
        if not alias:
            return {"error": "alias is required"}
        contacts = self._load_contacts(account)
        contacts[open_id] = {
            "alias": alias,
            "name": args.get("name", alias),
            "chat_id": args.get("chat_id", ""),
        }
        self._save_contacts(account, contacts)
        return {"status": "added", "open_id": open_id, "alias": alias}

    def _remove_contact(self, args: dict) -> dict:
        account = self._resolve_account(args)
        open_id = args.get("open_id", "")
        alias = args.get("alias", "")
        contacts = self._load_contacts(account)

        if open_id and open_id in contacts:
            del contacts[open_id]
            self._save_contacts(account, contacts)
            return {"status": "removed", "open_id": open_id}
        elif alias:
            to_remove = [
                oid for oid, v in contacts.items() if v.get("alias") == alias
            ]
            for oid in to_remove:
                del contacts[oid]
            if to_remove:
                self._save_contacts(account, contacts)
                return {"status": "removed", "open_ids": to_remove}
        return {"error": "Contact not found"}

    def _accounts(self) -> dict:
        return {"status": "ok", "accounts": self._service.list_accounts()}
