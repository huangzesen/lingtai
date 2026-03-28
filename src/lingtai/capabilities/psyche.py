"""Psyche capability — identity, memory, and context management.

Upgrades the eigen intrinsic with:
- Evolving identity (covenant + character)
- Enhanced memory.edit with file import (files param)
- memory.load delegation to eigen
- Context molt delegation to eigen

Library is a separate standalone capability.

Usage:
    agent = Agent(capabilities=["psyche"])
"""
from __future__ import annotations

import os
from pathlib import Path
from typing import TYPE_CHECKING

from ..i18n import t

if TYPE_CHECKING:
    from lingtai_kernel.base_agent import BaseAgent

PROVIDERS = {"providers": [], "default": "builtin"}

def get_description(lang: str = "en") -> str:
    return t(lang, "psyche.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "object": {
                "type": "string",
                "enum": ["character", "memory", "context"],
                "description": t(lang, "psyche.object"),
            },
            "action": {
                "type": "string",
                "enum": ["update", "load", "edit", "molt"],
                "description": t(lang, "psyche.action"),
            },
            "content": {
                "type": "string",
                "description": t(lang, "psyche.content"),
            },
            "files": {
                "type": "array",
                "items": {"type": "string"},
                "description": t(lang, "psyche.files"),
            },
            "summary": {
                "type": "string",
                "description": t(lang, "psyche.summary"),
            },
        },
        "required": ["object", "action"],
    }



class PsycheManager:
    """Identity, memory, and context manager."""

    def __init__(self, agent: "BaseAgent", eigen_handler):
        self._agent = agent
        self._working_dir = agent._working_dir
        self._eigen_handler = eigen_handler

        # Paths
        system_dir = self._working_dir / "system"
        self._covenant_path = system_dir / "covenant.md"
        self._character_path = system_dir / "character.md"

    # ------------------------------------------------------------------
    # Dispatch
    # ------------------------------------------------------------------

    _VALID_ACTIONS: dict[str, set[str]] = {
        "character": {"update", "load"},
        "memory": {"edit", "load"},
        "context": {"molt"},
    }

    def handle(self, args: dict) -> dict:
        obj = args.get("object", "")
        action = args.get("action", "")

        valid = self._VALID_ACTIONS.get(obj)
        if valid is None:
            return {
                "error": f"Unknown object: {obj!r}. "
                f"Must be one of: {', '.join(sorted(self._VALID_ACTIONS))}.",
            }
        if action not in valid:
            return {
                "error": f"Invalid action {action!r} for {obj}. "
                f"Valid actions: {', '.join(sorted(valid))}.",
            }

        method = getattr(self, f"_{obj}_{action}")
        return method(args)

    # ------------------------------------------------------------------
    # Character actions
    # ------------------------------------------------------------------

    def _character_update(self, args: dict) -> dict:
        content = args.get("content", "")
        self._character_path.parent.mkdir(exist_ok=True)
        self._character_path.write_text(content)
        # Auto-load into system prompt
        self._character_load({})
        return {"status": "ok", "path": str(self._character_path)}

    def _character_load(self, _args: dict) -> dict:
        covenant = ""
        if self._covenant_path.is_file():
            covenant = self._covenant_path.read_text()
        character = self._character_path.read_text() if self._character_path.is_file() else ""

        parts = [p for p in [covenant, character] if p.strip()]
        combined = "\n\n".join(parts)

        if combined.strip():
            self._agent._prompt_manager.write_section(
                "covenant", combined, protected=True,
            )
        else:
            self._agent._prompt_manager.delete_section("covenant")
        self._agent._token_decomp_dirty = True
        self._agent._flush_system_prompt()

        rel_path = "system/character.md"
        git_diff = self._agent._workdir.diff(rel_path)

        self._agent._log(
            "psyche_character_load",
            changed=bool(git_diff),
        )

        return {
            "status": "ok",
            "size_bytes": len(combined.encode("utf-8")),
            "content_preview": combined[:200],
            "diff": {
                "changed": bool(git_diff),
                "git_diff": git_diff or "",
                "commit": None,
            },
        }

    # ------------------------------------------------------------------
    # Memory actions — upgrade eigen with files support
    # ------------------------------------------------------------------

    def _memory_edit(self, args: dict) -> dict:
        """Write content + optional file imports to memory.md."""
        content = args.get("content", "")
        files = args.get("files") or []

        parts = [content] if content else []

        # Append file contents with numbered dividers
        not_found = []
        for i, fpath in enumerate(files, start=1):
            if os.path.isabs(fpath):
                resolved = Path(fpath)
            else:
                resolved = self._working_dir / fpath
            if not resolved.is_file():
                not_found.append(fpath)
                continue
            file_content = resolved.read_text()
            parts.append(f"[file-{i}]\n{file_content}")

        if not_found:
            return {"error": f"Files not found: {', '.join(not_found)}"}

        if not parts:
            return {"error": "Provide content, files, or both."}

        combined = "\n\n".join(parts)

        # Delegate to eigen for the actual write (eigen auto-loads into prompt)
        return self._eigen_handler({
            "object": "memory", "action": "edit", "content": combined,
        })

    def _memory_load(self, args: dict) -> dict:
        """Delegate to eigen's memory load."""
        return self._eigen_handler({"object": "memory", "action": "load"})

    # ------------------------------------------------------------------
    # Context actions — delegate to eigen
    # ------------------------------------------------------------------

    def _context_molt(self, args: dict) -> dict:
        return self._eigen_handler({"object": "context", "action": "molt", "summary": args.get("summary")})


def setup(agent: "BaseAgent") -> PsycheManager:
    """Set up psyche capability — identity, memory, and context management."""
    lang = agent._config.language
    eigen_handler = agent.override_intrinsic("eigen")
    agent._eigen_owns_memory = True

    mgr = PsycheManager(agent, eigen_handler)

    # Auto-load character and memory into system prompt at boot
    mgr._character_load({})
    mgr._memory_load({})

    # Register post-molt hook to reload character + memory
    if not hasattr(agent, "_post_molt_hooks"):
        agent._post_molt_hooks = []
    agent._post_molt_hooks.append(lambda: (mgr._character_load({}), mgr._memory_load({})))

    agent.add_tool(
        "psyche", schema=get_schema(lang), handler=mgr.handle, description=get_description(lang),
    )
    return mgr
