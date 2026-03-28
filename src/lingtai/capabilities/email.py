"""Email capability — filesystem-based mailbox with search and contacts.

Storage layout:
    working_dir/mailbox/inbox/{uuid}/message.json   — received
    working_dir/mailbox/sent/{uuid}/message.json     — sent
    working_dir/mailbox/read.json                    — read tracking
    working_dir/mailbox/contacts.json                — contact book

Usage:
    agent.add_capability("email")
"""
from __future__ import annotations

import json
import os
import re
import shutil
import tempfile
import threading
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import TYPE_CHECKING
from uuid import uuid4

from lingtai_kernel.intrinsics.mail import (
    _list_inbox, _load_message, _read_ids, _mark_read, _save_read_ids,
    _message_summary, _mailbox_dir,
    _persist_to_outbox, _mailman,
)

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

PROVIDERS = {"providers": [], "default": "builtin"}

def get_description(lang: str = "en") -> str:
    return t(lang, "email.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": [
                    "send", "check", "read", "reply", "reply_all", "search",
                    "archive", "delete",
                    "contacts", "add_contact", "remove_contact", "edit_contact",
                ],
                "description": t(lang, "email.action"),
            },
            "address": {
                "oneOf": [
                    {"type": "string"},
                    {"type": "array", "items": {"type": "string"}},
                ],
                "description": t(lang, "email.address"),
            },
            "cc": {
                "type": "array",
                "items": {"type": "string"},
                "description": t(lang, "email.cc"),
            },
            "bcc": {
                "type": "array",
                "items": {"type": "string"},
                "description": t(lang, "email.bcc"),
            },
            "attachments": {
                "type": "array",
                "items": {"type": "string"},
                "description": t(lang, "email.attachments"),
            },
            "subject": {"type": "string", "description": t(lang, "email.subject")},
            "message": {"type": "string", "description": t(lang, "email.message")},
            "email_id": {
                "type": "array",
                "items": {"type": "string"},
                "description": t(lang, "email.email_id"),
            },
            "n": {
                "type": "integer",
                "description": t(lang, "email.n"),
                "default": 10,
            },
            "query": {
                "type": "string",
                "description": t(lang, "email.query"),
            },
            "folder": {
                "type": "string",
                "enum": ["inbox", "sent", "archive"],
                "description": t(lang, "email.folder"),
            },
            "delay": {
                "type": "integer",
                "description": t(lang, "email.delay"),
            },
            "type": {
                "type": "string",
                "enum": ["normal"],
                "description": t(lang, "email.type"),
            },
            "name": {
                "type": "string",
                "description": t(lang, "email.name"),
            },
            "note": {
                "type": "string",
                "description": t(lang, "email.note"),
            },
            "schedule": {
                "type": "object",
                "properties": {
                    "action": {
                        "type": "string",
                        "enum": ["create", "cancel", "list"],
                        "description": t(lang, "email.schedule_action"),
                    },
                    "interval": {
                        "type": "integer",
                        "description": t(lang, "email.schedule_interval"),
                    },
                    "count": {
                        "type": "integer",
                        "description": t(lang, "email.schedule_count"),
                    },
                    "schedule_id": {
                        "type": "string",
                        "description": t(lang, "email.schedule_id"),
                    },
                },
            },
        },
        "required": [],
    }



class EmailManager:
    """Filesystem-based email manager — reads/writes mailbox/ directory."""

    def __init__(self, agent: "BaseAgent", *, private_mode: bool = False):
        self._agent = agent
        self._private_mode = private_mode
        # Track consecutive identical sends per recipient to block loops.
        # Maps address → (message_text, count).
        self._last_sent: dict[str, tuple[str, int]] = {}
        self._dup_free_passes = 2  # allow this many identical sends
        self._stop_event = threading.Event()
        self._scheduler_thread: threading.Thread | None = None

    def start_scheduler(self) -> None:
        """Start the background scheduler thread."""
        if self._scheduler_thread is not None:
            return
        self._stop_event.clear()
        self._scheduler_thread = threading.Thread(
            target=self._scheduler_loop,
            name=f"scheduler-{self._agent._working_dir.name}",
            daemon=True,
        )
        self._scheduler_thread.start()

    def stop_scheduler(self) -> None:
        """Stop the scheduler thread cleanly."""
        self._stop_event.set()
        if self._scheduler_thread is not None:
            self._scheduler_thread.join(timeout=5.0)
            self._scheduler_thread = None

    @property
    def _mailbox_path(self) -> Path:
        return _mailbox_dir(self._agent)

    @property
    def _schedules_dir(self) -> Path:
        return self._mailbox_path / "schedules"

    # ------------------------------------------------------------------
    # Filesystem helpers
    # ------------------------------------------------------------------

    def _load_email(self, email_id: str) -> dict | None:
        """Load a single email by ID. Checks inbox (via mail intrinsic) then sent/."""
        # Inbox — delegate to mail intrinsic helper
        msg = _load_message(self._agent, email_id)
        if msg is not None:
            msg["_folder"] = "inbox"
            msg.setdefault("_mailbox_id", email_id)
            return msg
        # Sent — own logic (mail intrinsic only knows inbox)
        path = self._mailbox_path / "sent" / email_id / "message.json"
        if path.is_file():
            try:
                data = json.loads(path.read_text())
            except (json.JSONDecodeError, OSError):
                return None
            data["_folder"] = "sent"
            data.setdefault("_mailbox_id", email_id)
            return data
        # Archive
        path = self._mailbox_path / "archive" / email_id / "message.json"
        if path.is_file():
            try:
                data = json.loads(path.read_text())
            except (json.JSONDecodeError, OSError):
                return None
            data["_folder"] = "archive"
            data.setdefault("_mailbox_id", email_id)
            return data
        return None

    def _list_emails(self, folder: str) -> list[dict]:
        """Load all emails from a folder, sorted by time (newest first)."""
        if folder == "inbox":
            # Delegate to mail intrinsic helper
            messages = _list_inbox(self._agent)
            for m in messages:
                m["_folder"] = "inbox"
                m.setdefault("_mailbox_id", m.get("_mailbox_id", ""))
            return messages
        # Sent — own logic
        folder_dir = self._mailbox_path / folder
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

    def _email_summary(self, e: dict, read_set: set[str] | None = None) -> dict:
        """Build a summary dict from a raw email dict."""
        if read_set is None:
            read_set = _read_ids(self._agent)
        if e.get("_folder") == "inbox":
            # Delegate to mail intrinsic helper for base fields
            summary = _message_summary(e, read_set)
            # Add email-capability extras
            summary["folder"] = "inbox"
            if e.get("cc"):
                summary["cc"] = e["cc"]
            return summary
        if e.get("_folder") == "archive":
            summary = _message_summary(e, read_set)
            summary["folder"] = "archive"
            if e.get("cc"):
                summary["cc"] = e["cc"]
            return summary
        # Sent — own summary logic
        eid = e.get("_mailbox_id", "")
        entry = {
            "id": eid,
            "from": e.get("from", ""),
            "to": e.get("to", []),
            "subject": e.get("subject", "(no subject)"),
            "preview": e.get("message", "")[:200],
            "time": e.get("received_at") or e.get("sent_at") or e.get("time") or "",
            "folder": e.get("_folder", ""),
        }
        if e.get("cc"):
            entry["cc"] = e["cc"]
        return entry

    # ------------------------------------------------------------------
    # Action dispatch
    # ------------------------------------------------------------------

    def handle(self, args: dict) -> dict:
        # Schedule sub-object takes priority over action
        schedule = args.get("schedule")
        if schedule is not None:
            return self._handle_schedule(args, schedule)
        action = args.get("action")
        if not action:
            return {"error": "action is required (or pass a schedule object)"}
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
        elif action == "archive":
            return self._archive(args)
        elif action == "delete":
            return self._delete(args)
        elif action == "contacts":
            return self._contacts()
        elif action == "add_contact":
            return self._add_contact(args)
        elif action == "remove_contact":
            return self._remove_contact(args)
        elif action == "edit_contact":
            return self._edit_contact(args)
        else:
            return {"error": f"Unknown email action: {action}"}

    # ------------------------------------------------------------------
    # Schedule dispatch
    # ------------------------------------------------------------------

    def _handle_schedule(self, args: dict, schedule: dict) -> dict:
        action = schedule.get("action")
        if action == "create":
            return self._schedule_create(args, schedule)
        elif action == "cancel":
            return self._schedule_cancel(schedule)
        elif action == "list":
            return self._schedule_list()
        else:
            return {"error": f"Unknown schedule action: {action}"}

    def _schedule_create(self, args: dict, schedule: dict) -> dict:
        interval = schedule.get("interval")
        count = schedule.get("count")
        if interval is None or count is None:
            return {"error": "schedule.interval and schedule.count are required"}
        if interval <= 0 or count <= 0:
            return {"error": "schedule.interval and schedule.count must be positive"}

        raw_address = args.get("address", "")
        if isinstance(raw_address, str):
            to_list = [raw_address] if raw_address else []
        else:
            to_list = list(raw_address)
        if not to_list:
            return {"error": "address is required"}

        send_payload = {
            "address": args.get("address"),
            "subject": args.get("subject", ""),
            "message": args.get("message", ""),
            "cc": args.get("cc") or [],
            "bcc": args.get("bcc") or [],
            "type": args.get("type", "normal"),
        }
        if args.get("attachments"):
            send_payload["attachments"] = args["attachments"]

        schedule_id = uuid4().hex[:12]
        now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        record = {
            "schedule_id": schedule_id,
            "send_payload": send_payload,
            "interval": interval,
            "count": count,
            "sent": 0,
            "created_at": now,
            "last_sent_at": None,
        }

        sched_dir = self._schedules_dir / schedule_id
        sched_dir.mkdir(parents=True, exist_ok=True)
        self._write_schedule(sched_dir / "schedule.json", record)

        return {"status": "scheduled", "schedule_id": schedule_id, "interval": interval, "count": count}

    def _schedule_cancel(self, schedule: dict) -> dict:
        schedule_id = schedule.get("schedule_id")

        if not schedule_id:
            # Agent-level cancel — cancel ALL schedules
            self._schedules_dir.mkdir(parents=True, exist_ok=True)
            (self._schedules_dir / ".cancel").touch()
            return {"status": "cancelled", "message": "All schedules cancelled"}

        # Per-schedule cancel
        sched_dir = self._schedules_dir / schedule_id
        if not sched_dir.is_dir():
            return {"error": f"Schedule not found: {schedule_id}"}

        record = self._read_schedule(schedule_id)
        if record and (record.get("sent", 0) >= record.get("count", 0)):
            return {"status": "already_stopped", "schedule_id": schedule_id}
        if (sched_dir / ".cancel").is_file():
            return {"status": "already_stopped", "schedule_id": schedule_id}

        (sched_dir / ".cancel").touch()
        return {"status": "cancelled", "schedule_id": schedule_id}

    def _schedule_list(self) -> dict:
        schedules_dir = self._schedules_dir
        if not schedules_dir.is_dir():
            return {"status": "ok", "schedules": []}

        entries = []
        for sched_dir in schedules_dir.iterdir():
            if not sched_dir.is_dir():
                continue
            sched_file = sched_dir / "schedule.json"
            if not sched_file.is_file():
                continue
            try:
                record = json.loads(sched_file.read_text())
            except (json.JSONDecodeError, OSError):
                continue

            payload = record.get("send_payload", {})
            address = payload.get("address", "")
            if isinstance(address, list):
                address = ", ".join(address)

            sent = record.get("sent", 0)
            count = record.get("count", 0)
            cancelled = (sched_dir / ".cancel").is_file() or (schedules_dir / ".cancel").is_file()
            active = sent < count and not cancelled

            entries.append({
                "schedule_id": record.get("schedule_id", sched_dir.name),
                "to": address,
                "subject": payload.get("subject", ""),
                "interval": record.get("interval", 0),
                "count": count,
                "sent": sent,
                "cancelled": cancelled,
                "created_at": record.get("created_at", ""),
                "last_sent_at": record.get("last_sent_at"),
                "active": active,
            })

        entries.sort(key=lambda e: e.get("created_at", ""), reverse=True)
        return {"status": "ok", "schedules": entries}

    # ------------------------------------------------------------------
    # Schedule helpers
    # ------------------------------------------------------------------

    def _write_schedule(self, path: Path, record: dict) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp = tempfile.mkstemp(dir=str(path.parent), suffix=".tmp")
        try:
            os.write(fd, json.dumps(record, indent=2, default=str).encode())
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

    def _read_schedule(self, schedule_id: str) -> dict | None:
        path = self._schedules_dir / schedule_id / "schedule.json"
        if not path.is_file():
            return None
        try:
            return json.loads(path.read_text())
        except (json.JSONDecodeError, OSError):
            return None

    def _scheduler_loop(self) -> None:
        """Single polling loop that drives all schedules from disk state."""
        while not self._stop_event.is_set():
            try:
                self._scheduler_tick()
            except Exception:
                pass  # individual tick failures should not kill the service
            self._stop_event.wait(timeout=1.0)

    def _scheduler_tick(self) -> None:
        """One scan of all schedule folders."""
        schedules_dir = self._schedules_dir
        if not schedules_dir.is_dir():
            return

        # Agent-level cancel
        if (schedules_dir / ".cancel").is_file():
            return

        now = datetime.now(timezone.utc)

        for sched_dir in schedules_dir.iterdir():
            if not sched_dir.is_dir():
                continue

            # Per-schedule cancel
            if (sched_dir / ".cancel").is_file():
                continue

            sched_file = sched_dir / "schedule.json"
            if not sched_file.is_file():
                continue

            try:
                record = json.loads(sched_file.read_text())
            except (json.JSONDecodeError, OSError):
                continue

            sent = record.get("sent", 0)
            count = record.get("count", 0)
            if sent >= count:
                continue

            # Check if due
            last_sent_at = record.get("last_sent_at")
            if last_sent_at is not None:
                try:
                    last_dt = datetime.strptime(last_sent_at, "%Y-%m-%dT%H:%M:%SZ").replace(
                        tzinfo=timezone.utc
                    )
                except ValueError:
                    continue
                interval = record.get("interval", 0)
                due_at = last_dt + timedelta(seconds=interval)
                if now < due_at:
                    continue

            # Due — at-most-once: increment before send
            seq = sent + 1
            record["sent"] = seq
            self._write_schedule(sched_file, record)

            # Build send args with _schedule metadata
            send_payload = record.get("send_payload", {})
            remaining = count - seq
            interval = record.get("interval", 0)
            estimated_finish = (now + timedelta(seconds=remaining * interval)).strftime(
                "%Y-%m-%dT%H:%M:%SZ"
            )
            schedule_meta = {
                "schedule_id": record.get("schedule_id", sched_dir.name),
                "seq": seq,
                "total": count,
                "interval": interval,
                "scheduled_at": now.strftime("%Y-%m-%dT%H:%M:%SZ"),
                "estimated_finish": estimated_finish,
            }
            send_args = {**send_payload, "_schedule": schedule_meta}
            self._send(send_args)

            # Update last_sent_at
            record["last_sent_at"] = now.strftime("%Y-%m-%dT%H:%M:%SZ")
            self._write_schedule(sched_file, record)

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
        delay = args.get("delay", 0)

        if isinstance(raw_address, str):
            to_list = [raw_address] if raw_address else []
        else:
            to_list = list(raw_address)

        if not to_list:
            return {"error": "address is required"}

        # Block identical consecutive messages (skip for scheduled sends)
        all_targets = to_list + cc + bcc
        if args.get("_schedule"):
            duplicates = []
        else:
            duplicates = [
                addr for addr in all_targets
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

        # Private mode
        if self._private_mode:
            contact_addresses = {c["address"] for c in self._load_contacts()}
            not_in_contacts = [a for a in all_targets if a not in contact_addresses]
            if not_in_contacts:
                return {
                    "error": (
                        "Private mode: recipient not in contacts: "
                        f"{', '.join(not_in_contacts)}. "
                        "Register them first with add_contact."
                    ),
                }

        sender = (self._agent._mail_service.address
                  if self._agent._mail_service is not None and self._agent._mail_service.address
                  else str(self._agent._working_dir))

        # Build visible payload (no bcc)
        base_payload = {
            "from": sender,
            "to": to_list,
            "subject": subject,
            "message": message_text,
            "type": mail_type,
            "identity": self._agent._build_manifest(),
        }
        if cc:
            base_payload["cc"] = cc
        attachments = args.get("attachments", [])
        if attachments:
            base_payload["attachments"] = attachments

        # Dispatch each recipient through outbox → mailman (skip_sent=True)
        deliver_at = datetime.now(timezone.utc) + timedelta(seconds=delay)
        all_recipients = to_list + cc + bcc

        for addr in all_recipients:
            dispatch_payload = dict(base_payload)
            dispatch_payload["_dispatch_to"] = addr
            msg_id = _persist_to_outbox(self._agent, dispatch_payload, deliver_at)
            t = threading.Thread(
                target=_mailman,
                args=(self._agent, msg_id, dispatch_payload, deliver_at),
                kwargs={"skip_sent": True},
                name=f"mailman-{msg_id[:8]}",
                daemon=True,
            )
            t.start()

        # Write ONE sent record (email-level, preserving the "one email" view)
        sent_id = str(uuid4())
        sent_dir = self._mailbox_path / "sent" / sent_id
        sent_dir.mkdir(parents=True, exist_ok=True)
        sent_record = {
            **base_payload,
            "_mailbox_id": sent_id,
            "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "delay": delay,
        }
        if bcc:
            sent_record["bcc"] = bcc
        if args.get("_schedule"):
            sent_record["_schedule"] = args["_schedule"]
        (sent_dir / "message.json").write_text(
            json.dumps(sent_record, indent=2, default=str)
        )

        # Track duplicates
        for addr in all_recipients:
            prev = self._last_sent.get(addr)
            if prev is not None and prev[0] == message_text:
                self._last_sent[addr] = (message_text, prev[1] + 1)
            else:
                self._last_sent[addr] = (message_text, 1)

        self._agent._log(
            "email_sent", to=to_list, cc=cc, bcc=bcc,
            subject=subject, message=message_text, delay=delay,
        )

        return {"status": "sent", "to": to_list, "cc": cc, "bcc": bcc, "delay": delay}

    # ------------------------------------------------------------------
    # Check — list emails from a folder
    # ------------------------------------------------------------------

    def _check(self, args: dict) -> dict:
        folder = args.get("folder", "inbox")
        n = args.get("n", 10)
        emails = self._list_emails(folder)
        total = len(emails)
        recent = emails[:n] if n > 0 else emails
        read_set = _read_ids(self._agent)
        summaries = [self._email_summary(e, read_set) for e in recent]
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

        folder = args.get("folder")

        results = []
        errors = []
        for eid in ids:
            if folder:
                path = self._mailbox_path / folder / eid / "message.json"
                if path.is_file():
                    try:
                        data = json.loads(path.read_text())
                        data["_folder"] = folder
                        data.setdefault("_mailbox_id", eid)
                    except (json.JSONDecodeError, OSError):
                        errors.append(eid)
                        continue
                else:
                    errors.append(eid)
                    continue
            else:
                data = self._load_email(eid)
                if data is None:
                    errors.append(eid)
                    continue
            if data.get("_folder") == "inbox":
                _mark_read(self._agent, eid)
            entry = {
                "id": eid,
                "from": data.get("from", ""),
                "to": data.get("to", []),
                "subject": data.get("subject", "(no subject)"),
                "message": data.get("message", ""),
                "time": data.get("received_at") or data.get("sent_at") or data.get("time") or "",
                "folder": data.get("_folder", ""),
            }
            if data.get("cc"):
                entry["cc"] = data["cc"]
            if data.get("attachments"):
                entry["attachments"] = data["attachments"]
            results.append(entry)

        result = {"status": "ok", "emails": results}
        if errors:
            result["not_found"] = errors
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
        if isinstance(email_id, list):
            email_id = email_id[0] if email_id else ""
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
        if isinstance(email_id, list):
            email_id = email_id[0] if email_id else ""
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
            else str(self._agent._working_dir)
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
        read_set = _read_ids(self._agent)
        for f in folders:
            for email in self._list_emails(f):
                searchable = " ".join([
                    email.get("from", ""),
                    email.get("subject", ""),
                    email.get("message", ""),
                ])
                if pattern.search(searchable):
                    matches.append(self._email_summary(email, read_set))

        return {"status": "ok", "total": len(matches), "emails": matches}

    # ------------------------------------------------------------------
    # Archive
    # ------------------------------------------------------------------

    def _archive(self, args: dict) -> dict:
        """Move email(s) from inbox to archive."""
        ids = args.get("email_id", [])
        if isinstance(ids, str):
            ids = [ids]
        if not ids:
            return {"error": "email_id is required"}

        archived = []
        not_found = []
        archive_dir = self._mailbox_path / "archive"
        inbox_dir = self._mailbox_path / "inbox"

        for eid in ids:
            src = inbox_dir / eid
            if not src.is_dir():
                not_found.append(eid)
                continue
            dst = archive_dir / eid
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.move(str(src), str(dst))
            archived.append(eid)

        if archived:
            read_set = _read_ids(self._agent)
            read_set -= set(archived)
            _save_read_ids(self._agent, read_set)

        result: dict = {"status": "ok", "archived": archived}
        if not_found:
            result["not_found"] = not_found
        return result

    # ------------------------------------------------------------------
    # Delete
    # ------------------------------------------------------------------

    def _delete(self, args: dict) -> dict:
        """Remove email(s) from inbox or archive."""
        ids = args.get("email_id", [])
        if isinstance(ids, str):
            ids = [ids]
        if not ids:
            return {"error": "email_id is required"}

        folder = args.get("folder", "inbox")
        if folder not in ("inbox", "archive"):
            return {"error": f"Cannot delete from folder: {folder}"}

        folder_dir = self._mailbox_path / folder
        deleted = []
        not_found = []

        for eid in ids:
            target = folder_dir / eid
            if target.is_dir():
                shutil.rmtree(target)
                deleted.append(eid)
            else:
                not_found.append(eid)

        if deleted:
            read_set = _read_ids(self._agent)
            read_set -= set(deleted)
            _save_read_ids(self._agent, read_set)

        result: dict = {"status": "ok", "deleted": deleted}
        if not_found:
            result["not_found"] = not_found
        return result

    # ------------------------------------------------------------------
    # Contacts
    # ------------------------------------------------------------------

    @property
    def _contacts_path(self) -> Path:
        return self._mailbox_path / "contacts.json"

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
        self._mailbox_path.mkdir(parents=True, exist_ok=True)
        target = self._contacts_path
        fd, tmp = tempfile.mkstemp(dir=str(self._mailbox_path), suffix=".tmp")
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
        # Upsert by address
        for c in contacts:
            if c["address"] == address:
                c["name"] = name
                c["note"] = note
                self._save_contacts(contacts)
                return {"status": "updated", "contact": c}
        entry: dict = {"address": address, "name": name, "note": note}
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

def setup(agent: "BaseAgent", *, private_mode: bool = False) -> EmailManager:
    """Set up email capability — filesystem-based mailbox."""
    lang = agent._config.language
    mgr = EmailManager(agent, private_mode=private_mode)
    agent.override_intrinsic("mail")  # remove mail tool; email reimplements fully
    agent._mailbox_name = "email box"
    agent._mailbox_tool = "email"
    agent.add_tool(
        "email", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang),
    )
    mgr.start_scheduler()
    return mgr
