"""System intrinsic — agent memory management.

Actions:
    diff   — show uncommitted git diff for memory.md
    load   — read the file, inject into live system prompt, git add+commit

Objects:
    memory — system/memory.md (the agent's long-term memory)

The handler lives in BaseAgent (needs access to working_dir, prompt_manager, git).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["diff", "load"],
            "description": (
                "diff: show uncommitted git diff (what changed since last commit).\n"
                "load: read the file, inject into the live system prompt, "
                "and git commit. This updates the agent's live memory."
            ),
        },
        "object": {
            "type": "string",
            "enum": ["memory"],
            "description": "memory: the agent's long-term memory (system/memory.md).",
        },
    },
    "required": ["action", "object"],
}

DESCRIPTION = (
    "Agent memory management. Long-term memory lives in system/memory.md. "
    "Use 'diff' to see uncommitted changes, "
    "and 'load' to apply changes into the live system prompt (with git commit)."
)
