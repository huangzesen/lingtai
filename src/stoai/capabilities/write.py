"""Write capability — create or overwrite a file.

Usage: Agent(capabilities=["write"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

from ..i18n import t

if TYPE_CHECKING:
    from stoai_kernel.base_agent import BaseAgent


def get_description(lang: str = "en") -> str:
    return t(lang, "write.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "file_path": {"type": "string", "description": t(lang, "write.file_path")},
            "content": {"type": "string", "description": t(lang, "write.content")},
        },
        "required": ["file_path", "content"],
    }


# Backward compat
SCHEMA = get_schema("en")
DESCRIPTION = get_description("en")


def setup(agent: "BaseAgent") -> None:
    """Set up the write capability on an agent."""
    lang = agent._config.language

    def handle_write(args: dict) -> dict:
        path = args.get("file_path", "")
        content = args.get("content", "")
        if not path:
            return {"error": "file_path is required"}
        if not Path(path).is_absolute():
            path = str(agent._working_dir / path)
        try:
            agent._file_io.write(path, content)
            return {"status": "ok", "path": path, "bytes": len(content)}
        except Exception as e:
            return {"error": f"Cannot write {path}: {e}"}

    agent.add_tool("write", schema=get_schema(lang), handler=handle_write, description=get_description(lang))
