"""Library capability — standalone knowledge store.

A structured knowledge archive persisted in library/library.json.
Agents submit, browse, read, organize, and delete entries.
Completely decoupled from psyche and memory — the agent decides
what to do with the knowledge it retrieves.

Usage:
    agent = Agent(capabilities=["library"])
"""
from __future__ import annotations

import hashlib
import json
import os
import re
import tempfile
from datetime import datetime, timezone
from typing import TYPE_CHECKING

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

PROVIDERS = {"providers": [], "default": "builtin"}


def get_description(lang: str = "en") -> str:
    return t(lang, "library.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["submit", "filter", "view", "consolidate", "delete", "export"],
                "description": t(lang, "library.action"),
            },
            "title": {
                "type": "string",
                "description": t(lang, "library.title"),
            },
            "summary": {
                "type": "string",
                "description": t(lang, "library.summary"),
            },
            "content": {
                "type": "string",
                "description": t(lang, "library.content"),
            },
            "supplementary": {
                "type": "string",
                "description": t(lang, "library.supplementary"),
            },
            "ids": {
                "type": "array",
                "items": {"type": "string"},
                "description": t(lang, "library.ids"),
            },
            "pattern": {
                "type": "string",
                "description": t(lang, "library.pattern"),
            },
            "limit": {
                "type": "integer",
                "description": t(lang, "library.limit"),
            },
            "depth": {
                "type": "string",
                "enum": ["content", "supplementary"],
                "description": t(lang, "library.depth"),
            },
        },
        "required": ["action"],
    }



class LibraryManager:
    """Knowledge archive — submit, browse, read, organize, delete."""

    DEFAULT_MAX_ENTRIES = 20

    def __init__(self, agent: "BaseAgent", *, library_limit: int | None = None):
        self._agent = agent
        self._working_dir = agent._working_dir
        self._max_entries = library_limit if library_limit is not None else self.DEFAULT_MAX_ENTRIES

        self._library_json = self._working_dir / "library" / "library.json"
        self._exports_dir = self._working_dir / "exports"
        self._entries: list[dict] = self._load_entries()

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    def _load_entries(self) -> list[dict]:
        if not self._library_json.is_file():
            return []
        try:
            data = json.loads(self._library_json.read_text())
            entries = data.get("entries", [])
            for e in entries:
                if "title" not in e:
                    e["title"] = e.get("content", "")[:50] or "Untitled"
                    e["summary"] = e.get("content", "")[:200]
                    e["supplementary"] = ""
            return entries
        except (json.JSONDecodeError, OSError):
            return []

    def _save_entries(self) -> None:
        data = {"version": 1, "entries": self._entries}
        self._library_json.parent.mkdir(exist_ok=True)
        fd, tmp = tempfile.mkstemp(
            dir=str(self._library_json.parent), suffix=".tmp",
        )
        try:
            os.write(fd, json.dumps(data, indent=2, ensure_ascii=False).encode())
            os.close(fd)
            os.replace(tmp, str(self._library_json))
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise

    @staticmethod
    def _make_id(content: str, created_at: str) -> str:
        return hashlib.sha256(
            (content + created_at).encode()
        ).hexdigest()[:8]

    # ------------------------------------------------------------------
    # Dispatch
    # ------------------------------------------------------------------

    _VALID_ACTIONS = {"submit", "filter", "view", "consolidate", "delete", "export"}

    def handle(self, args: dict) -> dict:
        action = args.get("action", "")
        if action not in self._VALID_ACTIONS:
            return {
                "error": f"Unknown action: {action!r}. "
                f"Valid: {', '.join(sorted(self._VALID_ACTIONS))}.",
            }
        method = getattr(self, f"_{action}")
        return method(args)

    # ------------------------------------------------------------------
    # Actions
    # ------------------------------------------------------------------

    def _submit(self, args: dict) -> dict:
        title = args.get("title", "").strip()
        summary = args.get("summary", "").strip()
        content = args.get("content", "").strip()
        supplementary = args.get("supplementary", "").strip()
        if not title:
            return {"error": "title is required for submit."}
        if not summary:
            return {"error": "summary is required for submit."}
        if not content:
            return {"error": "content is required for submit."}
        if len(self._entries) >= self._max_entries:
            return {
                "error": f"Library is full ({self._max_entries} entries). "
                "Consolidate related entries first, "
                "delete obsolete ones, or use supplementary "
                "to pack more detail into existing entries.",
                "entries": len(self._entries),
                "max": self._max_entries,
            }
        now = datetime.now(timezone.utc).isoformat()
        entry_id = self._make_id(title + content, now)
        self._entries.append({
            "id": entry_id,
            "title": title,
            "summary": summary,
            "content": content,
            "supplementary": supplementary,
            "created_at": now,
        })
        self._save_entries()
        return {
            "status": "ok",
            "id": entry_id,
            "entries": len(self._entries),
            "max": self._max_entries,
        }

    def _filter(self, args: dict) -> dict:
        pattern = args.get("pattern")
        limit = args.get("limit")
        entries = self._entries
        if pattern:
            try:
                rx = re.compile(pattern, re.IGNORECASE)
            except re.error as exc:
                return {"error": f"Invalid regex pattern: {exc}"}
            entries = [
                e for e in entries
                if rx.search(e["title"])
                or rx.search(e["summary"])
                or rx.search(e["content"])
            ]
        if limit is not None and limit > 0:
            entries = entries[:limit]
        return {
            "status": "ok",
            "entries": [
                {"id": e["id"], "title": e["title"], "summary": e["summary"]}
                for e in entries
            ],
        }

    def _view(self, args: dict) -> dict:
        ids = args.get("ids")
        if not ids:
            return {"error": "ids is required for view."}
        depth = args.get("depth", "content")

        entries_by_id = {e["id"]: e for e in self._entries}
        invalid = [i for i in ids if i not in entries_by_id]
        if invalid:
            return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

        result_entries = []
        for entry_id in ids:
            e = entries_by_id[entry_id]
            item = {
                "id": e["id"],
                "title": e["title"],
                "summary": e["summary"],
                "content": e["content"],
            }
            if depth == "supplementary":
                item["supplementary"] = e.get("supplementary", "")
            result_entries.append(item)

        return {"status": "ok", "entries": result_entries}

    def _consolidate(self, args: dict) -> dict:
        ids = args.get("ids")
        title = args.get("title", "").strip()
        summary = args.get("summary", "").strip()
        content = args.get("content", "").strip()
        supplementary = args.get("supplementary", "").strip()
        if not ids:
            return {"error": "ids is required for consolidate."}
        if not title:
            return {"error": "title is required for consolidate."}
        if not summary:
            return {"error": "summary is required for consolidate."}
        if not content:
            return {"error": "content is required for consolidate."}

        existing_ids = {e["id"] for e in self._entries}
        invalid = [i for i in ids if i not in existing_ids]
        if invalid:
            return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

        ids_set = set(ids)
        self._entries = [e for e in self._entries if e["id"] not in ids_set]

        now = datetime.now(timezone.utc).isoformat()
        new_id = self._make_id(title + content, now)
        self._entries.append({
            "id": new_id,
            "title": title,
            "summary": summary,
            "content": content,
            "supplementary": supplementary,
            "created_at": now,
        })

        self._save_entries()
        return {"status": "ok", "id": new_id, "removed": len(ids)}

    def _delete(self, args: dict) -> dict:
        ids = args.get("ids")
        if not ids:
            return {"error": "ids is required for delete."}

        existing_ids = {e["id"] for e in self._entries}
        invalid = [i for i in ids if i not in existing_ids]
        if invalid:
            return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

        ids_set = set(ids)
        before = len(self._entries)
        self._entries = [e for e in self._entries if e["id"] not in ids_set]
        removed = before - len(self._entries)

        self._save_entries()
        return {"status": "ok", "removed": removed}

    def _export(self, args: dict) -> dict:
        ids = args.get("ids")
        if not ids:
            return {"error": "ids is required for export."}

        entries_by_id = {e["id"]: e for e in self._entries}
        invalid = [i for i in ids if i not in entries_by_id]
        if invalid:
            return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

        self._exports_dir.mkdir(parents=True, exist_ok=True)

        exported = []
        for entry_id in ids:
            e = entries_by_id[entry_id]
            parts = [f"# {e['title']}", f"\n{e['content']}"]
            if e.get("supplementary", "").strip():
                parts.append(f"\n---\n{e['supplementary']}")
            text = "\n".join(parts)

            path = self._exports_dir / f"{entry_id}.txt"
            path.write_text(text)
            # Return path relative to working dir for use in memory.edit(files=[...])
            exported.append(str(path.relative_to(self._working_dir)))

        return {"status": "ok", "files": exported, "count": len(exported)}


def setup(agent: "BaseAgent", *, library_limit: int | None = None) -> LibraryManager:
    """Set up library capability — standalone knowledge store."""
    lang = agent._config.language

    mgr = LibraryManager(agent, library_limit=library_limit)

    agent.add_tool(
        "library", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang),
    )
    return mgr
