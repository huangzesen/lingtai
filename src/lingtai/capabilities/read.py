"""Read capability — read text file contents.

Usage: Agent(capabilities=["read"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

PROVIDERS = {"providers": [], "default": "builtin"}


def get_description(lang: str = "en") -> str:
    return t(lang, "read.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "file_path": {"type": "string", "description": t(lang, "read.file_path")},
            "offset": {"type": "integer", "description": t(lang, "read.offset"), "default": 1},
            "limit": {"type": "integer", "description": t(lang, "read.limit"), "default": 2000},
        },
        "required": ["file_path"],
    }



def setup(agent: "BaseAgent") -> None:
    """Set up the read capability on an agent."""
    lang = agent._config.language

    def handle_read(args: dict) -> dict:
        path = args.get("file_path", "")
        if not path:
            return {"status": "error", "message": "file_path is required"}
        if not Path(path).is_absolute():
            path = str(agent._working_dir / path)
        offset = args.get("offset", 1)
        limit = args.get("limit", 2000)
        try:
            content = agent._file_io.read(path)
        except FileNotFoundError:
            return {"status": "error", "message": f"File not found: {path}"}
        except Exception as e:
            return {"status": "error", "message": f"Cannot read {path}: {e}"}
        lines = content.splitlines(keepends=True)
        start = max(0, offset - 1)
        selected = lines[start:start + limit]
        numbered = "".join(f"{start + i + 1}\t{line}" for i, line in enumerate(selected))
        return {"content": numbered, "total_lines": len(lines), "lines_shown": len(selected)}

    agent.add_tool("read", schema=get_schema(lang), handler=handle_read, description=get_description(lang))
