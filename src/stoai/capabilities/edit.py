"""Edit capability — exact string replacement in a file.

Usage: Agent(capabilities=["edit"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

from ..i18n import t

if TYPE_CHECKING:
    from stoai_kernel.base_agent import BaseAgent


def get_description(lang: str = "en") -> str:
    return t(lang, "edit.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "file_path": {"type": "string", "description": t(lang, "edit.file_path")},
            "old_string": {"type": "string", "description": t(lang, "edit.old_string")},
            "new_string": {"type": "string", "description": t(lang, "edit.new_string")},
            "replace_all": {"type": "boolean", "description": t(lang, "edit.replace_all"), "default": False},
        },
        "required": ["file_path", "old_string", "new_string"],
    }


# Backward compat
SCHEMA = get_schema("en")
DESCRIPTION = get_description("en")


def setup(agent: "BaseAgent") -> None:
    """Set up the edit capability on an agent."""
    lang = agent._config.language

    def handle_edit(args: dict) -> dict:
        path = args.get("file_path", "")
        if not path:
            return {"error": "file_path is required"}
        if not Path(path).is_absolute():
            path = str(agent._working_dir / path)
        old = args.get("old_string", "")
        new = args.get("new_string", "")
        replace_all = args.get("replace_all", False)
        try:
            content = agent._file_io.read(path)
        except FileNotFoundError:
            return {"error": f"File not found: {path}"}
        except Exception as e:
            return {"error": f"Cannot read {path}: {e}"}
        count = content.count(old)
        if count == 0:
            return {"error": f"old_string not found in {path}"}
        if count > 1 and not replace_all:
            return {"error": f"old_string found {count} times — use replace_all=true or provide more context"}
        if replace_all:
            updated = content.replace(old, new)
        else:
            updated = content.replace(old, new, 1)
        try:
            agent._file_io.write(path, updated)
        except Exception as e:
            return {"error": f"Cannot write {path}: {e}"}
        return {"status": "ok", "replacements": count if replace_all else 1}

    agent.add_tool("edit", schema=get_schema(lang), handler=handle_edit, description=get_description(lang))
