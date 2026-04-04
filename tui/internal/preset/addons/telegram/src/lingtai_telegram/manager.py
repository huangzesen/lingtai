# src/lingtai_telegram/manager.py
"""TelegramManager — tool dispatch + filesystem persistence.

Storage layout:
    working_dir/telegram/{account}/inbox/{uuid}/message.json
    working_dir/telegram/{account}/inbox/{uuid}/attachments/
    working_dir/telegram/{account}/sent/{uuid}/message.json
    working_dir/telegram/{account}/contacts.json
    working_dir/telegram/{account}/read.json

Mirrors IMAPMailManager patterns with Telegram-specific adaptations.
"""
from __future__ import annotations

import json
import os
import re
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING, Any
from uuid import uuid4

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent
    from .service import TelegramService

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": [
                "send", "check", "read", "reply", "search",
                "delete", "edit",
                "contacts", "add_contact", "remove_contact",
                "accounts",
            ],
            "description": (
                "send: send message to a chat (chat_id, text; optional media, reply_markup). "
                "check: list recent conversations with unread counts (optional account). "
                "read: read messages from a chat (chat_id; optional limit). "
                "reply: reply to a specific message (message_id from read results, text). "
                "search: search messages (query; optional account, chat_id). "
                "delete: delete a bot message (message_id). "
                "edit: edit a bot message (message_id, text; optional reply_markup). "
                "contacts: list saved contacts. "
                "add_contact: save a chat (chat_id, alias). "
                "remove_contact: remove a contact (alias or chat_id). "
                "accounts: list configured bot accounts."
            ),
        },
        "account": {
            "type": "string",
            "description": "Bot account alias (optional — defaults to first configured account)",
        },
        "chat_id": {
            "type": "integer",
            "description": "Telegram chat ID",
        },
        "text": {
            "type": "string",
            "description": "Message text",
        },
        "message_id": {
            "type": "string",
            "description": "Compound message ID: {account}:{chat_id}:{message_id}",
        },
        "media": {
            "type": "object",
            "properties": {
                "type": {"type": "string", "enum": ["photo", "document"]},
                "path": {"type": "string"},
            },
            "description": "Media attachment: {type: 'photo'|'document', path: '/path/to/file'}",
        },
        "reply_markup": {
            "type": "object",
            "description": "Inline keyboard markup",
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
        "alias": {
            "type": "string",
            "description": "Contact alias for add_contact/remove_contact",
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Telegram bot client — interact with Telegram users via Bot API. "
    "Use 'send' for outgoing messages (text, photos, documents, inline keyboards). "
    "'check' to see recent conversations. "
    "'read' to read messages from a specific chat. "
    "'reply' to respond to a message (use compound ID from read results). "
    "'search' to find messages by text/sender. "
    "'delete'/'edit' to modify bot messages. "
    "'contacts' to manage saved contacts. "
    "'accounts' to list configured bot accounts."
)


class TelegramManager:
    """Tool handler + filesystem manager for the Telegram addon."""

    def __init__(
        self,
        agent: "BaseAgent",
        service: "TelegramService",
        working_dir: Path,
    ) -> None:
        self._agent = agent
        self._service = service
        self._working_dir = working_dir
        # Duplicate send protection: (account, chat_id, text) → count
        self._last_sent: dict[tuple[str, int, str], int] = {}
        self._dup_free_passes = 2

    def _account_dir(self, account: str) -> Path:
        return self._working_dir / "telegram" / account

    def _resolve_account(self, args: dict) -> str:
        """Get account alias from args, defaulting to first account."""
        return args.get("account") or self._service.default_account.alias

    @staticmethod
    def _parse_compound_id(compound_id: str) -> tuple[str, int, int]:
        """Parse '{account}:{chat_id}:{message_id}' → (account, chat_id, message_id)."""
        parts = compound_id.split(":")
        if len(parts) != 3:
            raise ValueError(f"Invalid message ID format: {compound_id}")
        return parts[0], int(parts[1]), int(parts[2])

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
            elif action == "delete":
                return self._delete(args)
            elif action == "edit":
                return self._edit(args)
            elif action == "contacts":
                return self._contacts(args)
            elif action == "add_contact":
                return self._add_contact(args)
            elif action == "remove_contact":
                return self._remove_contact(args)
            elif action == "accounts":
                return self._accounts()
            else:
                return {"error": f"Unknown telegram action: {action}"}
        except Exception as e:
            return {"error": str(e)}

    # ------------------------------------------------------------------
    # Incoming messages — called by TelegramService via on_message
    # ------------------------------------------------------------------

    def on_incoming(self, account_alias: str, update: dict) -> None:
        """Persist incoming update to disk and notify agent."""
        msg_id = str(uuid4())
        acct_dir = self._account_dir(account_alias)
        msg_dir = acct_dir / "inbox" / msg_id
        msg_dir.mkdir(parents=True, exist_ok=True)

        # Extract message data based on update type
        if "message" in update:
            tg_msg = update["message"]
            compound_id = f"{account_alias}:{tg_msg['chat']['id']}:{tg_msg['message_id']}"
            sender = tg_msg.get("from", {})
            payload = {
                "id": compound_id,
                "from": sender,
                "chat": tg_msg.get("chat", {}),
                "date": datetime.fromtimestamp(
                    tg_msg.get("date", 0), tz=timezone.utc,
                ).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "text": tg_msg.get("text") or tg_msg.get("caption") or "",
                "media": None,
                "reply_to_message_id": None,
                "callback_query": None,
            }
            # Handle reply_to
            if tg_msg.get("reply_to_message"):
                payload["reply_to_message_id"] = tg_msg["reply_to_message"]["message_id"]
            # Handle media
            self._download_media(account_alias, tg_msg, msg_dir, payload)
            username = sender.get("username") or sender.get("first_name", "unknown")

        elif "callback_query" in update:
            cq = update["callback_query"]
            tg_msg = cq.get("message", {})
            sender = cq.get("from", {})
            chat = tg_msg.get("chat", {})
            compound_id = f"{account_alias}:{chat.get('id', 0)}:{tg_msg.get('message_id', 0)}"
            payload = {
                "id": compound_id,
                "from": sender,
                "chat": chat,
                "date": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "text": "",
                "media": None,
                "reply_to_message_id": None,
                "callback_query": cq.get("data"),
            }
            username = sender.get("username") or sender.get("first_name", "unknown")

        elif "edited_message" in update:
            tg_msg = update["edited_message"]
            compound_id = f"{account_alias}:{tg_msg['chat']['id']}:{tg_msg['message_id']}"
            sender = tg_msg.get("from", {})
            payload = {
                "id": compound_id,
                "from": sender,
                "chat": tg_msg.get("chat", {}),
                "date": datetime.fromtimestamp(
                    tg_msg.get("edit_date", tg_msg.get("date", 0)), tz=timezone.utc,
                ).strftime("%Y-%m-%dT%H:%M:%SZ"),
                "text": tg_msg.get("text") or tg_msg.get("caption") or "",
                "media": None,
                "reply_to_message_id": None,
                "callback_query": None,
            }
            username = sender.get("username") or sender.get("first_name", "unknown")

            # Update existing inbox entry in-place if found
            existing_dir = self._find_inbox_by_compound_id(account_alias, compound_id)
            if existing_dir is not None:
                (existing_dir / "message.json").write_text(
                    json.dumps(payload, indent=2, default=str), encoding="utf-8",
                )
                # Clean up the unused new dir
                msg_dir.rmdir()
            else:
                (msg_dir / "message.json").write_text(
                    json.dumps(payload, indent=2, default=str), encoding="utf-8",
                )
        else:
            return  # unsupported update type

        # Persist (for message and callback_query types)
        if "edited_message" not in update:
            (msg_dir / "message.json").write_text(
                json.dumps(payload, indent=2, default=str), encoding="utf-8",
            )

        # Notify agent
        self._agent.wake("message_received")
        notification = (
            f"[system] New telegram message from {username} via {account_alias}.\n"
            f'Use telegram(action="check") to see your messages.'
        )
        self._agent.notify("system", notification)
        self._agent.log(
            "telegram_received", sender=username, account=account_alias,
            text=payload.get("text", "")[:100],
        )

    def _download_media(
        self, account_alias: str, tg_msg: dict, msg_dir: Path, payload: dict,
    ) -> None:
        """Download photo/document attachments from a Telegram message."""
        file_id = None
        media_type = None

        if tg_msg.get("photo"):
            # Photos come as array of sizes — take the largest
            file_id = tg_msg["photo"][-1]["file_id"]
            media_type = "photo"
        elif tg_msg.get("document"):
            file_id = tg_msg["document"]["file_id"]
            media_type = "document"

        if file_id is None:
            return

        try:
            acct = self._service.get_account(account_alias)
            filename, data = acct.get_file(file_id)
            att_dir = msg_dir / "attachments"
            att_dir.mkdir(parents=True, exist_ok=True)
            filepath = att_dir / filename
            filepath.write_bytes(data)
            payload["media"] = {
                "type": media_type,
                "filename": filename,
                "path": str(filepath),
                "size": len(data),
            }
        except Exception as e:
            import logging
            logging.getLogger(__name__).warning(
                "Failed to download media: %s", e,
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

    def _find_inbox_by_compound_id(self, account: str, compound_id: str) -> Path | None:
        """Find an existing inbox message dir by compound ID. Returns dir Path or None."""
        inbox_dir = self._account_dir(account) / "inbox"
        if not inbox_dir.is_dir():
            return None
        for msg_dir in inbox_dir.iterdir():
            msg_file = msg_dir / "message.json"
            if msg_dir.is_dir() and msg_file.is_file():
                try:
                    data = json.loads(msg_file.read_text(encoding="utf-8"))
                    if data.get("id") == compound_id:
                        return msg_dir
                except (json.JSONDecodeError, OSError):
                    continue
        return None

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

    # ------------------------------------------------------------------
    # Actions
    # ------------------------------------------------------------------

    def _send(self, args: dict) -> dict:
        account = self._resolve_account(args)
        chat_id = args.get("chat_id")
        text = args.get("text", "")
        media = args.get("media")
        reply_markup = args.get("reply_markup")

        if not chat_id:
            return {"error": "chat_id is required"}
        if not text and not media:
            return {"error": "text or media is required"}

        # Duplicate send protection
        dup_key = (account, chat_id, text)
        count = self._last_sent.get(dup_key, 0)
        if count >= self._dup_free_passes:
            return {
                "status": "blocked",
                "warning": "Identical message already sent. Think twice before repeating.",
            }

        acct = self._service.get_account(account)
        reply_to = args.get("_reply_to_message_id")

        # Send via Bot API
        if media:
            media_type = media.get("type")
            media_path = media.get("path", "")
            if media_type == "photo":
                result = acct.send_photo(
                    chat_id, media_path, caption=text or None,
                    reply_to_message_id=reply_to,
                )
            elif media_type == "document":
                result = acct.send_document(
                    chat_id, media_path, caption=text or None,
                    reply_to_message_id=reply_to,
                )
            else:
                return {"error": f"Unknown media type: {media_type}"}
        else:
            result = acct.send_message(
                chat_id, text, reply_markup=reply_markup,
                reply_to_message_id=reply_to,
            )

        # Track for duplicate detection
        self._last_sent[dup_key] = count + 1

        # Persist to sent/
        sent_id = str(uuid4())
        sent_dir = self._account_dir(account) / "sent" / sent_id
        sent_dir.mkdir(parents=True, exist_ok=True)
        tg_message_id = result.get("message_id", 0)
        compound_id = f"{account}:{chat_id}:{tg_message_id}"
        sent_record = {
            "id": compound_id,
            "to": {"chat_id": chat_id},
            "date": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "text": text,
            "media": media,
            "reply_markup": reply_markup,
            "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "status": "sent",
        }
        (sent_dir / "message.json").write_text(
            json.dumps(sent_record, indent=2, default=str), encoding="utf-8",
        )

        return {"status": "sent", "message_id": compound_id}

    def _check(self, args: dict) -> dict:
        account = self._resolve_account(args)
        messages = self._list_messages(account, "inbox")
        read_ids = self._read_ids(account)

        # Group by chat_id for conversation view
        conversations: dict[int, dict] = {}
        for msg in messages:
            cid = msg.get("chat", {}).get("id", 0)
            if cid not in conversations:
                conversations[cid] = {
                    "chat_id": cid,
                    "chat_type": msg.get("chat", {}).get("type", "private"),
                    "last_from": msg.get("from", {}),
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
            "messages": list(conversations.values()),
        }

    def _read(self, args: dict) -> dict:
        account = self._resolve_account(args)
        chat_id = args.get("chat_id")
        limit = args.get("limit", 10)

        if not chat_id:
            return {"error": "chat_id is required"}

        messages = self._list_messages(account, "inbox")
        filtered = [m for m in messages if m.get("chat", {}).get("id") == chat_id]
        recent = filtered[:limit]

        # Mark as read
        compound_ids = [m["id"] for m in recent if m.get("id")]
        if compound_ids:
            self._mark_read(account, compound_ids)

        # Strip internal fields
        cleaned = []
        for m in recent:
            cleaned.append({
                "id": m.get("id"),
                "from": m.get("from"),
                "chat": m.get("chat"),
                "date": m.get("date"),
                "text": m.get("text"),
                "media": m.get("media"),
                "callback_query": m.get("callback_query"),
                "reply_to_message_id": m.get("reply_to_message_id"),
            })

        return {"status": "ok", "messages": cleaned}

    def _reply(self, args: dict) -> dict:
        compound_id = args.get("message_id", "")
        text = args.get("text", "")
        if not compound_id:
            return {"error": "message_id is required"}
        if not text:
            return {"error": "text is required"}

        account, chat_id, tg_msg_id = self._parse_compound_id(compound_id)
        return self._send({
            "account": account,
            "chat_id": chat_id,
            "text": text,
            "media": args.get("media"),
            "reply_markup": args.get("reply_markup"),
            # We need to pass reply_to_message_id through
            "_reply_to_message_id": tg_msg_id,
        })

    def _search(self, args: dict) -> dict:
        query = args.get("query", "")
        if not query:
            return {"error": "query is required"}
        account = self._resolve_account(args)
        target_chat = args.get("chat_id")

        try:
            pattern = re.compile(query, re.IGNORECASE)
        except re.error as e:
            return {"error": f"Invalid regex: {e}"}

        messages = self._list_messages(account, "inbox")
        matches = []
        for msg in messages:
            if target_chat and msg.get("chat", {}).get("id") != target_chat:
                continue
            searchable = " ".join([
                str(msg.get("from", {}).get("username", "")),
                str(msg.get("from", {}).get("first_name", "")),
                msg.get("text", ""),
            ])
            if pattern.search(searchable):
                matches.append({
                    "id": msg.get("id"),
                    "from": msg.get("from"),
                    "date": msg.get("date"),
                    "text": msg.get("text"),
                })

        return {"status": "ok", "total": len(matches), "messages": matches}

    def _delete(self, args: dict) -> dict:
        compound_id = args.get("message_id", "")
        if not compound_id:
            return {"error": "message_id is required"}
        account, chat_id, tg_msg_id = self._parse_compound_id(compound_id)
        acct = self._service.get_account(account)
        acct.delete_message(chat_id=chat_id, message_id=tg_msg_id)
        return {"status": "deleted", "message_id": compound_id}

    def _edit(self, args: dict) -> dict:
        compound_id = args.get("message_id", "")
        text = args.get("text", "")
        if not compound_id:
            return {"error": "message_id is required"}
        if not text:
            return {"error": "text is required"}
        account, chat_id, tg_msg_id = self._parse_compound_id(compound_id)
        reply_markup = args.get("reply_markup")
        acct = self._service.get_account(account)

        # Detect if original message had media (caption edit vs text edit)
        is_caption = False
        sent_dir = self._account_dir(account) / "sent"
        if sent_dir.is_dir():
            for msg_dir in sent_dir.iterdir():
                msg_file = msg_dir / "message.json"
                if msg_dir.is_dir() and msg_file.is_file():
                    try:
                        data = json.loads(msg_file.read_text(encoding="utf-8"))
                        if data.get("id") == compound_id and data.get("media"):
                            is_caption = True
                            break
                    except (json.JSONDecodeError, OSError):
                        continue

        acct.edit_message(
            chat_id=chat_id, message_id=tg_msg_id, text=text,
            reply_markup=reply_markup, is_caption=is_caption,
        )
        return {"status": "edited", "message_id": compound_id}

    def _contacts(self, args: dict) -> dict:
        account = self._resolve_account(args)
        return {"status": "ok", "contacts": self._load_contacts(account)}

    def _add_contact(self, args: dict) -> dict:
        account = self._resolve_account(args)
        chat_id = args.get("chat_id")
        alias = args.get("alias", "")
        if not chat_id:
            return {"error": "chat_id is required"}
        if not alias:
            return {"error": "alias is required"}
        contacts = self._load_contacts(account)
        contacts[alias] = {
            "chat_id": chat_id,
            "username": args.get("username", ""),
            "first_name": args.get("first_name", ""),
        }
        self._save_contacts(account, contacts)
        return {"status": "added", "alias": alias}

    def _remove_contact(self, args: dict) -> dict:
        account = self._resolve_account(args)
        alias = args.get("alias", "")
        chat_id = args.get("chat_id")
        contacts = self._load_contacts(account)
        if alias and alias in contacts:
            del contacts[alias]
            self._save_contacts(account, contacts)
            return {"status": "removed", "alias": alias}
        elif chat_id:
            to_remove = [k for k, v in contacts.items() if v.get("chat_id") == chat_id]
            for k in to_remove:
                del contacts[k]
            if to_remove:
                self._save_contacts(account, contacts)
                return {"status": "removed", "aliases": to_remove}
        return {"error": "Contact not found"}

    def _accounts(self) -> dict:
        return {"status": "ok", "accounts": self._service.list_accounts()}
