"""Glob capability — find files by pattern.

Usage: Agent(capabilities=["glob"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

from ..i18n import t

if TYPE_CHECKING:
    from stoai_kernel.base_agent import BaseAgent


def get_description(lang: str = "en") -> str:
    return t(lang, "glob.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "pattern": {"type": "string", "description": t(lang, "glob.pattern")},
            "path": {"type": "string", "description": t(lang, "glob.path")},
        },
        "required": ["pattern"],
    }


# Backward compat
SCHEMA = get_schema("en")
DESCRIPTION = get_description("en")


def setup(agent: "BaseAgent") -> None:
    """Set up the glob capability on an agent."""
    lang = agent._config.language

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

    agent.add_tool("glob", schema=get_schema(lang), handler=handle_glob, description=get_description(lang))
