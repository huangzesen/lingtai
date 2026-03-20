"""Psyche capability — self-knowledge management.

Upgrades the eigen intrinsic (like email upgrades mail).
Adds evolving identity (covenant + character), structured library,
and memory construct (build memory from library entries + notes).

Usage:
    agent = Agent(capabilities=["psyche"])
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
    from stoai_kernel.base_agent import BaseAgent

def get_description(lang: str = "en") -> str:
    return t(lang, "psyche.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "object": {
                "type": "string",
                "enum": ["character", "library", "memory", "context"],
                "description": t(lang, "psyche.object"),
            },
            "action": {
                "type": "string",
                "enum": [
                    "update", "diff", "load",
                    "submit", "filter", "view", "consolidate", "delete",
                    "construct", "molt",
                ],
                "description": t(lang, "psyche.action"),
            },
            "title": {
                "type": "string",
                "description": t(lang, "psyche.title"),
            },
            "summary": {
                "type": "string",
                "description": t(lang, "psyche.summary"),
            },
            "content": {
                "type": "string",
                "description": t(lang, "psyche.content"),
            },
            "supplementary": {
                "type": "string",
                "description": t(lang, "psyche.supplementary"),
            },
            "ids": {
                "type": "array",
                "items": {"type": "string"},
                "description": t(lang, "psyche.ids"),
            },
            "notes": {
                "type": "string",
                "description": t(lang, "psyche.notes"),
            },
            "pattern": {
                "type": "string",
                "description": t(lang, "psyche.pattern"),
            },
            "limit": {
                "type": "integer",
                "description": t(lang, "psyche.limit"),
            },
            "depth": {
                "type": "string",
                "enum": ["content", "supplementary"],
                "description": t(lang, "psyche.depth"),
            },
        },
        "required": ["object", "action"],
    }


# Backward compat
SCHEMA = get_schema("en")
DESCRIPTION = get_description("en")


class PsycheManager:
    """Self-knowledge manager — character, library, memory, context."""

    def __init__(self, agent: "BaseAgent", eigen_handler, *, library_limit: int | None = None):
        self._agent = agent
        self._working_dir = agent._working_dir
        self._eigen_handler = eigen_handler
        self._max_entries = library_limit if library_limit is not None else self.DEFAULT_MAX_ENTRIES

        # Paths
        system_dir = self._working_dir / "system"
        self._covenant_path = system_dir / "covenant.md"
        self._character_path = system_dir / "character.md"
        self._memory_md = system_dir / "memory.md"
        self._library_json = system_dir / "library.json"

        # In-memory cache of entries
        self._entries: list[dict] = self._load_entries()

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    def _load_entries(self) -> list[dict]:
        """Load entries from library.json, or return empty list if missing."""
        if not self._library_json.is_file():
            return []
        try:
            data = json.loads(self._library_json.read_text())
            entries = data.get("entries", [])
            # Migrate legacy flat entries (pre-library format)
            for e in entries:
                if "title" not in e:
                    e["title"] = e.get("content", "")[:50] or "Untitled"
                    e["summary"] = e.get("content", "")[:200]
                    e["supplementary"] = ""
            return entries
        except (json.JSONDecodeError, OSError):
            return []

    def _save_entries(self) -> None:
        """Write entries to library.json with atomic write."""
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
        """Generate 8-char hex ID from content + timestamp."""
        return hashlib.sha256(
            (content + created_at).encode()
        ).hexdigest()[:8]

    def _load_library_entry(self, entry_id: str) -> dict | None:
        """Look up a library entry by ID."""
        for e in self._entries:
            if e["id"] == entry_id:
                return e
        return None

    # ------------------------------------------------------------------
    # Dispatch
    # ------------------------------------------------------------------

    _VALID_ACTIONS: dict[str, set[str]] = {
        "character": {"update", "diff", "load"},
        "library": {"submit", "filter", "view", "consolidate", "delete"},
        "memory": {"construct", "load"},
        "context": {"molt"},
    }

    def handle(self, args: dict) -> dict:
        """Main dispatch — routes by object + action."""
        obj = args.get("object", "")
        action = args.get("action", "")

        valid = self._VALID_ACTIONS.get(obj)
        if valid is None:
            return {
                "error": f"Unknown object: {obj!r}. "
                f"Must be one of: {', '.join(sorted(self._VALID_ACTIONS))}.",
            }
        if action not in valid:
            return {
                "error": f"Invalid action {action!r} for {obj}. "
                f"Valid actions: {', '.join(sorted(valid))}.",
            }

        method = getattr(self, f"_{obj}_{action}")
        return method(args)

    # ------------------------------------------------------------------
    # Character actions
    # ------------------------------------------------------------------

    def _character_update(self, args: dict) -> dict:
        content = args.get("content", "")
        self._character_path.parent.mkdir(exist_ok=True)
        self._character_path.write_text(content)
        return {"status": "ok", "path": str(self._character_path)}

    def _character_diff(self, _args: dict) -> dict:
        diff_text = self._agent._workdir.diff("system/character.md")
        return {"status": "ok", "path": str(self._character_path), "git_diff": diff_text}

    def _character_load(self, _args: dict) -> dict:
        # Read both files and concatenate
        covenant = ""
        if self._covenant_path.is_file():
            covenant = self._covenant_path.read_text()
        character = self._character_path.read_text() if self._character_path.is_file() else ""

        parts = [p for p in [covenant, character] if p.strip()]
        combined = "\n\n".join(parts)

        # Inject as protected section
        if combined.strip():
            self._agent._prompt_manager.write_section(
                "covenant", combined, protected=True,
            )
        else:
            self._agent._prompt_manager.delete_section("covenant")
        self._agent._token_decomp_dirty = True

        # Update live session
        if self._agent._chat is not None:
            self._agent._chat.update_system_prompt(
                self._agent._build_system_prompt()
            )

        # Git commit character.md
        rel_path = "system/character.md"
        git_diff, commit_hash = self._agent._workdir.diff_and_commit(
            rel_path, "character",
        )

        self._agent._log(
            "psyche_character_load",
            changed=commit_hash is not None,
        )

        return {
            "status": "ok",
            "size_bytes": len(combined.encode("utf-8")),
            "content_preview": combined[:200],
            "diff": {
                "changed": commit_hash is not None,
                "git_diff": git_diff or "",
                "commit": commit_hash,
            },
        }

    # ------------------------------------------------------------------
    # Library actions
    # ------------------------------------------------------------------

    DEFAULT_MAX_ENTRIES = 20

    def _library_submit(self, args: dict) -> dict:
        title = args.get("title", "").strip()
        summary = args.get("summary", "").strip()
        content = args.get("content", "").strip()
        supplementary = args.get("supplementary", "").strip()
        if not title:
            return {"error": "title is required for library submit."}
        if not summary:
            return {"error": "summary is required for library submit."}
        if not content:
            return {"error": "content is required for library submit."}
        if len(self._entries) >= self._max_entries:
            return {
                "error": f"Library is full ({self._max_entries} entries). "
                "Consolidate related entries first (library consolidate), "
                "delete obsolete ones (library delete), or use supplementary "
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

    def _library_filter(self, args: dict) -> dict:
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

    def _library_view(self, args: dict) -> dict:
        ids = args.get("ids")
        if not ids:
            return {"error": "ids is required for library view."}
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

    def _library_consolidate(self, args: dict) -> dict:
        ids = args.get("ids")
        title = args.get("title", "").strip()
        summary = args.get("summary", "").strip()
        content = args.get("content", "").strip()
        supplementary = args.get("supplementary", "").strip()
        if not ids:
            return {"error": "ids is required for library consolidate."}
        if not title:
            return {"error": "title is required for library consolidate."}
        if not summary:
            return {"error": "summary is required for library consolidate."}
        if not content:
            return {"error": "content is required for library consolidate."}

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

    def _library_delete(self, args: dict) -> dict:
        ids = args.get("ids")
        if not ids:
            return {"error": "ids is required for library delete."}

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

    # ------------------------------------------------------------------
    # Memory actions
    # ------------------------------------------------------------------

    def _memory_construct(self, args: dict) -> dict:
        """Build memory from library entries + free text notes."""
        ids = args.get("ids", [])
        notes = args.get("notes", "")

        parts = []
        if notes:
            parts.append(notes)

        # Load library entries by ID
        if ids:
            entries_by_id = {e["id"]: e for e in self._entries}
            invalid = [i for i in ids if i not in entries_by_id]
            if invalid:
                return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

            for entry_id in ids:
                e = entries_by_id[entry_id]
                parts.append(f"### [{e['id']}] {e['title']}\n{e['content']}")

        if not parts:
            return {"error": "Provide ids, notes, or both for memory construct."}

        content = "\n\n".join(parts)
        self._memory_md.parent.mkdir(exist_ok=True)
        self._memory_md.write_text(content + "\n")

        # Git commit
        self._agent._workdir.diff_and_commit("system/memory.md", "memory construct")

        self._agent._log(
            "psyche_memory_construct",
            entry_count=len(ids),
            length=len(content),
        )

        return {"status": "ok", "entries": len(ids), "length": len(content)}

    def _memory_load(self, args: dict) -> dict:
        """Load system/memory.md into the system prompt — delegates to eigen."""
        return self._eigen_handler({"object": "memory", "action": "load"})

    # ------------------------------------------------------------------
    # Context actions — delegate to eigen
    # ------------------------------------------------------------------

    def _context_molt(self, args: dict) -> dict:
        """Delegate molt to eigen's handler."""
        return self._eigen_handler({"object": "context", "action": "molt", "summary": args.get("summary")})


def setup(agent: "BaseAgent", *, library_limit: int | None = None) -> PsycheManager:
    """Set up psyche capability — self-knowledge management."""
    lang = agent._config.language
    eigen_handler = agent.override_intrinsic("eigen")
    agent._eigen_owns_memory = True

    mgr = PsycheManager(agent, eigen_handler, library_limit=library_limit)

    # Migrate existing memory.md content to library as a seed entry
    memory_file = agent._working_dir / "system" / "memory.md"
    if memory_file.is_file():
        existing = memory_file.read_text().strip()
        if existing and not mgr._entries:
            mgr.handle({
                "object": "library", "action": "submit",
                "title": "Initial memory (migrated)",
                "summary": existing[:200],
                "content": existing,
            })

    agent.add_tool(
        "psyche", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang),
    )
    return mgr
