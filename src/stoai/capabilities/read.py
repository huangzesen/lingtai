"""Read capability — read text file contents.

Usage: Agent(capabilities=["read"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "file_path": {"type": "string", "description": "Absolute path to the file to read"},
        "offset": {"type": "integer", "description": "Line number to start from (1-based)", "default": 1},
        "limit": {"type": "integer", "description": "Max lines to read", "default": 2000},
    },
    "required": ["file_path"],
}

DESCRIPTION = (
    "Read the contents of a text file. Returns numbered lines. "
    "Text files only — cannot read binary, images, or audio. "
    "Use offset/limit to read specific sections of large files."
)


def setup(agent: "BaseAgent") -> None:
    """Set up the read capability on an agent."""

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

    agent.add_tool("read", schema=SCHEMA, handler=handle_read, description=DESCRIPTION,
                    system_prompt="Read file contents.")
