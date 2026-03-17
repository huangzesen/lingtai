"""Write capability — create or overwrite a file.

Usage: Agent(capabilities=["write"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "file_path": {"type": "string", "description": "Absolute path to the file to write"},
        "content": {"type": "string", "description": "Content to write"},
    },
    "required": ["file_path", "content"],
}

DESCRIPTION = (
    "Create or overwrite a file with the given content. "
    "Parent directories are created automatically. "
    "Use this for creating new files or complete rewrites. "
    "For small changes to existing files, prefer edit."
)


def setup(agent: "BaseAgent") -> None:
    """Set up the write capability on an agent."""

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

    agent.add_tool("write", schema=SCHEMA, handler=handle_write, description=DESCRIPTION,
                    system_prompt="Create or overwrite files.")
