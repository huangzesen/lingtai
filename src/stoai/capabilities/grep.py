"""Grep capability — search file contents by regex.

Usage: Agent(capabilities=["grep"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "pattern": {"type": "string", "description": "Regex pattern to search for"},
        "path": {"type": "string", "description": "File or directory to search in"},
        "glob": {"type": "string", "description": "File glob filter (e.g., '*.py')", "default": "*"},
        "max_matches": {"type": "integer", "description": "Maximum matches to return", "default": 200},
    },
    "required": ["pattern"],
}

DESCRIPTION = (
    "Search file contents for lines matching a regex pattern. "
    "Returns matching lines with file path and line number. "
    "Searches recursively when given a directory. "
    "Use the glob filter to narrow to specific file types."
)


def setup(agent: "BaseAgent") -> None:
    """Set up the grep capability on an agent."""

    def handle_grep(args: dict) -> dict:
        pattern = args.get("pattern", "")
        if not pattern:
            return {"error": "pattern is required"}
        search_path = args.get("path", str(agent._working_dir))
        if not Path(search_path).is_absolute():
            search_path = str(agent._working_dir / search_path)
        max_matches = args.get("max_matches", 200)
        try:
            results = agent._file_io.grep(pattern, path=search_path, max_results=max_matches)
            matches = [{"file": r.path, "line": r.line_number, "text": r.line} for r in results]
            return {"matches": matches, "count": len(matches), "truncated": len(matches) >= max_matches}
        except Exception as e:
            return {"error": f"Grep failed: {e}"}

    agent.add_tool("grep", schema=SCHEMA, handler=handle_grep, description=DESCRIPTION,
                    system_prompt="Search file contents by regex.")
