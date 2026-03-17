"""Glob capability — find files by pattern.

Usage: Agent(capabilities=["glob"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "pattern": {"type": "string", "description": "Glob pattern (e.g., '**/*.py')"},
        "path": {"type": "string", "description": "Directory to search in"},
    },
    "required": ["pattern"],
}

DESCRIPTION = (
    "Find files matching a glob pattern. "
    "Use '**/' for recursive search (e.g. '**/*.py' finds all Python files). "
    "Returns sorted list of matching file paths."
)


def setup(agent: "BaseAgent") -> None:
    """Set up the glob capability on an agent."""

    def handle_glob(args: dict) -> dict:
        pattern = args.get("pattern", "")
        if not pattern:
            return {"error": "pattern is required"}
        search_dir = args.get("path", str(agent._working_dir))
        if not Path(search_dir).is_absolute():
            search_dir = str(agent._working_dir / search_dir)
        try:
            matches = agent._file_io.glob(pattern, root=search_dir)
            return {"matches": matches, "count": len(matches)}
        except Exception as e:
            return {"error": f"Glob failed: {e}"}

    agent.add_tool("glob", schema=SCHEMA, handler=handle_glob, description=DESCRIPTION,
                    system_prompt="Find files by name pattern.")
