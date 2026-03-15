"""Diary layer — immutable append-only agent log."""
from __future__ import annotations

import json
import re
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING, Optional

if TYPE_CHECKING:
    from ..agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["save", "catalogue", "view"],
            "description": "Action to perform",
        },
        "title": {"type": "string", "description": "Entry title slug (for save/view)"},
        "summary": {"type": "string", "description": "One-line summary (for save)"},
        "content": {"type": "string", "description": "Full entry content (for save)"},
        "n": {"type": "integer", "description": "Number of entries to list (for catalogue)", "default": 20},
    },
    "required": ["action"],
}
DESCRIPTION = "Save notes, observations, and learnings to a persistent diary. Use catalogue to list entries and view to read them."


def _slugify(title: str) -> str:
    """Convert a title to a safe filename slug."""
    slug = title.lower().strip()
    slug = re.sub(r"[^\w\s-]", "", slug)
    slug = re.sub(r"[\s_]+", "-", slug)
    slug = re.sub(r"-+", "-", slug)
    return slug[:80]


class DiaryManager:
    """Manages a file-based diary for an agent.

    Each entry is stored as a JSON file: <diary_dir>/<timestamp>-<slug>.json
    with fields: title, summary, content, created_at.
    """

    def __init__(self, diary_dir: Optional[Path] = None) -> None:
        if diary_dir is not None:
            self._dir = Path(diary_dir)
            self._dir.mkdir(parents=True, exist_ok=True)
        else:
            self._dir = None

    def _entry_path(self, title: str) -> Optional[Path]:
        """Find the path for an existing entry by title slug, or None."""
        if self._dir is None:
            return None
        slug = _slugify(title)
        # Search for any file containing the slug
        for p in sorted(self._dir.glob("*.json")):
            if slug in p.stem:
                return p
        return None

    def handle(self, args: dict) -> dict:
        action = args.get("action")

        if action == "save":
            if self._dir is None:
                return {"status": "not configured — diary_dir is not set"}
            title = args.get("title", "")
            summary = args.get("summary", "")
            content = args.get("content", "")
            if not title:
                return {"error": "title is required for save"}
            slug = _slugify(title)
            now = datetime.now(timezone.utc)
            ts = now.strftime("%Y%m%d-%H%M%S")
            filename = f"{ts}-{slug}.json"
            entry = {
                "title": title,
                "summary": summary,
                "content": content,
                "created_at": now.isoformat(),
            }
            path = self._dir / filename
            path.write_text(json.dumps(entry, ensure_ascii=False, indent=2), encoding="utf-8")
            return {"status": "ok", "path": str(path)}

        elif action == "catalogue":
            if self._dir is None:
                return {"status": "not configured — diary_dir is not set", "entries": []}
            n = args.get("n", 20)
            files = sorted(self._dir.glob("*.json"), reverse=True)[:n]
            entries = []
            for f in files:
                try:
                    data = json.loads(f.read_text(encoding="utf-8"))
                    entries.append({
                        "title": data.get("title", f.stem),
                        "summary": data.get("summary", ""),
                        "created_at": data.get("created_at", ""),
                    })
                except Exception:
                    entries.append({"title": f.stem, "summary": "(unreadable)", "created_at": ""})
            return {"status": "ok", "entries": entries, "count": len(entries)}

        elif action == "view":
            if self._dir is None:
                return {"status": "not configured — diary_dir is not set"}
            title = args.get("title", "")
            if not title:
                return {"error": "title is required for view"}
            path = self._entry_path(title)
            if path is None:
                return {"error": f"Entry not found: {title}"}
            try:
                data = json.loads(path.read_text(encoding="utf-8"))
                return {
                    "status": "ok",
                    "title": data.get("title", ""),
                    "summary": data.get("summary", ""),
                    "content": data.get("content", ""),
                    "created_at": data.get("created_at", ""),
                }
            except Exception as e:
                return {"error": f"Cannot read entry: {e}"}

        else:
            return {"error": f"Unknown action: {action}"}


def add_diary_layer(agent: "BaseAgent", diary_dir: Path | str) -> DiaryManager:
    """Add diary capability to an agent.

    Returns the DiaryManager instance for programmatic access.
    """
    mgr = DiaryManager(diary_dir=diary_dir)
    agent.add_tool("manage_diary", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt("diary_instructions",
        "You have a diary tool. Use it to record observations, lessons learned, "
        "and notes about your work. Entries are immutable once saved. "
        "Review past entries when they might be relevant.")
    return mgr
