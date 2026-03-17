"""Anima capability — self-knowledge management.

Upgrades the system intrinsic (like email upgrades mail).
Adds evolving role (covenant + character), structured memory,
and on-demand context compaction.

Usage:
    agent = Agent(capabilities=["anima"])
"""
from __future__ import annotations

import hashlib
import json
import os
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Callable

    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "object": {
            "type": "string",
            "enum": ["role", "memory", "context"],
            "description": (
                "role: the agent's identity (system/covenant.md + system/character.md).\n"
                "memory: the agent's long-term memory "
                "(system/memory.md, backed by system/memory.json).\n"
                "context: the agent's conversation context window."
            ),
        },
        "action": {
            "type": "string",
            "enum": [
                "update", "diff", "load",
                "submit", "consolidate",
                "compact",
            ],
            "description": (
                "role: update | diff | load.\n"
                "memory: submit | diff | consolidate | load.\n"
                "context: compact."
            ),
        },
        "content": {
            "type": "string",
            "description": (
                "Text content — for role update (character), "
                "memory submit (new entry), or memory consolidate (merged text)."
            ),
        },
        "ids": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Memory entry IDs — for memory consolidate.",
        },
        "prompt": {
            "type": "string",
            "description": (
                "Compaction guidance — what to preserve, what to compress. "
                "Required for context compact. Can be empty."
            ),
        },
    },
    "required": ["object", "action"],
}

DESCRIPTION = (
    "Self-knowledge management — evolving identity, structured memory, "
    "and context control. "
    "role: update your character (your identity, knowledge, experience), "
    "diff to review pending changes, load to apply into live system prompt. "
    "memory: submit new entries, diff to review, consolidate entries by ID "
    "into a single merged entry, load to apply. "
    "Memory IDs are visible in your system prompt. "
    "context: compact to proactively free context space — check usage via "
    "status show first, provide guidance on what to preserve. "
    "After any write (update/submit/consolidate), use diff then load to apply."
)


class AnimaManager:
    """Self-knowledge manager — role, memory, context."""

    def __init__(self, agent: "BaseAgent"):
        self._agent = agent
        self._working_dir = agent._working_dir
        self._original_system: Callable[[dict], dict] | None = None

        # Paths
        system_dir = self._working_dir / "system"
        self._covenant_path = system_dir / "covenant.md"
        self._character_path = system_dir / "character.md"
        self._memory_md = system_dir / "memory.md"
        self._memory_json = system_dir / "memory.json"

        # Ensure character.md exists
        system_dir.mkdir(exist_ok=True)
        if not self._character_path.is_file():
            self._character_path.write_text("")

        # In-memory cache of entries
        self._entries: list[dict] = self._load_entries()

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    def _load_entries(self) -> list[dict]:
        """Load entries from memory.json, or return empty list if missing."""
        if not self._memory_json.is_file():
            return []
        try:
            data = json.loads(self._memory_json.read_text())
            return data.get("entries", [])
        except (json.JSONDecodeError, OSError):
            return []

    def _save_entries(self) -> None:
        """Write entries to memory.json with atomic write."""
        data = {"version": 1, "entries": self._entries}
        self._memory_json.parent.mkdir(exist_ok=True)
        fd, tmp = tempfile.mkstemp(
            dir=str(self._memory_json.parent), suffix=".tmp",
        )
        try:
            os.write(fd, json.dumps(data, indent=2, ensure_ascii=False).encode())
            os.close(fd)
            os.replace(tmp, str(self._memory_json))
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise

    def _render_memory_md(self) -> None:
        """Render memory.json entries to memory.md."""
        lines = []
        for entry in self._entries:
            lines.append(f"- [{entry['id']}] {entry['content']}")
        self._memory_md.parent.mkdir(exist_ok=True)
        self._memory_md.write_text("\n".join(lines) + ("\n" if lines else ""))

    @staticmethod
    def _make_id(content: str, created_at: str) -> str:
        """Generate 8-char hex ID from content + timestamp."""
        return hashlib.sha256(
            (content + created_at).encode()
        ).hexdigest()[:8]

    # ------------------------------------------------------------------
    # Dispatch
    # ------------------------------------------------------------------

    _VALID_ACTIONS: dict[str, set[str]] = {
        "role": {"update", "diff", "load"},
        "memory": {"submit", "diff", "consolidate", "load"},
        "context": {"compact"},
    }

    def handle(self, args: dict) -> dict:
        """Main dispatch — routes by object + action."""
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
    # Role actions
    # ------------------------------------------------------------------

    def _role_update(self, args: dict) -> dict:
        content = args.get("content", "")
        self._character_path.parent.mkdir(exist_ok=True)
        self._character_path.write_text(content)
        return {"status": "ok", "path": str(self._character_path)}

    def _role_diff(self, _args: dict) -> dict:
        return self._agent._system_diff(self._character_path, "character")

    def _role_load(self, _args: dict) -> dict:
        # Read both files and concatenate
        covenant = ""
        if self._covenant_path.is_file():
            covenant = self._covenant_path.read_text()
        character = self._character_path.read_text() if self._character_path.is_file() else ""

        parts = [p for p in [covenant, character] if p.strip()]
        combined = "\n\n".join(parts)

        # Inject as protected section
        if combined.strip():
            self._agent._prompt_manager.write_section(
                "covenant", combined, protected=True,
            )
        else:
            self._agent._prompt_manager.delete_section("covenant")
        self._agent._token_decomp_dirty = True

        # Update live session
        if self._agent._chat is not None:
            self._agent._chat.update_system_prompt(
                self._agent._build_system_prompt()
            )

        # Git commit character.md
        rel_path = "system/character.md"
        git_diff, commit_hash = self._agent._git_diff_and_commit(
            rel_path, "character",
        )

        self._agent._log(
            "anima_role_load",
            changed=commit_hash is not None,
        )

        return {
            "status": "ok",
            "size_bytes": len(combined.encode("utf-8")),
            "content_preview": combined[:200],
            "diff": {
                "changed": commit_hash is not None,
                "git_diff": git_diff or "",
                "commit": commit_hash,
            },
        }

    # ------------------------------------------------------------------
    # Memory actions
    # ------------------------------------------------------------------

    def _memory_submit(self, args: dict) -> dict:
        content = args.get("content", "")
        if not content.strip():
            return {"error": "content is required for memory submit."}
        now = datetime.now(timezone.utc).isoformat()
        entry_id = self._make_id(content, now)
        self._entries.append({
            "id": entry_id,
            "content": content,
            "created_at": now,
        })
        self._save_entries()
        self._render_memory_md()
        return {"status": "ok", "id": entry_id}

    def _memory_diff(self, args: dict) -> dict:
        # Delegate to original system handler
        if self._original_system is None:
            return {"error": "anima not properly initialized (missing system handler)"}
        return self._original_system({"action": "diff", "object": "memory"})

    def _memory_consolidate(self, args: dict) -> dict:
        ids = args.get("ids")
        content = args.get("content", "")
        if not ids:
            return {"error": "ids is required for memory consolidate."}
        if not content.strip():
            return {"error": "content is required for memory consolidate."}

        # Validate IDs
        existing_ids = {e["id"] for e in self._entries}
        invalid = [i for i in ids if i not in existing_ids]
        if invalid:
            return {"error": f"Unknown memory IDs: {', '.join(invalid)}"}

        # Remove old entries
        ids_set = set(ids)
        self._entries = [e for e in self._entries if e["id"] not in ids_set]

        # Add consolidated entry
        now = datetime.now(timezone.utc).isoformat()
        new_id = self._make_id(content, now)
        self._entries.append({
            "id": new_id,
            "content": content,
            "created_at": now,
        })

        self._save_entries()
        self._render_memory_md()
        return {"status": "ok", "id": new_id, "removed": len(ids)}

    def _memory_load(self, args: dict) -> dict:
        # Delegate to original system handler
        if self._original_system is None:
            return {"error": "anima not properly initialized (missing system handler)"}
        return self._original_system({"action": "load", "object": "memory"})

    # ------------------------------------------------------------------
    # Context actions
    # ------------------------------------------------------------------

    def _context_compact(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if prompt is None:
            return {"error": "prompt is required for context compact (can be empty)."}

        if self._agent._chat is None:
            return {"error": "No active chat session to compact."}

        from ..llm.service import COMPACTION_PROMPT

        agent_prompt = self._agent._chat.interface.current_system_prompt or ""
        ctx_window = self._agent._chat.context_window()
        target_tokens = int(ctx_window * 0.2) if ctx_window > 0 else 2048

        def summarizer(text: str) -> str:
            prompt_parts = [COMPACTION_PROMPT]
            if prompt:
                prompt_parts.append(f"\nAgent guidance: {prompt}\n")
            prompt_parts.append(
                f"\nTarget summary length: ~{target_tokens} tokens "
                f"(20% of {ctx_window} token context window).\n"
            )
            if agent_prompt:
                prompt_parts.append(
                    f"\nThe agent's role:\n{agent_prompt}\n\n"
                    "Do your best to help this agent based on its role.\n"
                )
            prompt_parts.append(f"\nConversation history:\n{text}")
            response = self._agent.service.generate(
                "".join(prompt_parts),
                temperature=0.1,
                max_output_tokens=target_tokens,
            )
            return response.text.strip() if response and response.text else ""

        # Force compaction with threshold=0.0
        new_chat = self._agent.service.check_and_compact(
            self._agent._chat,
            summarizer=summarizer,
            threshold=0.0,
            provider=self._agent._config.provider,
        )
        if new_chat is not None:
            before_tokens = self._agent._chat.interface.estimate_context_tokens()
            after_tokens = new_chat.interface.estimate_context_tokens()
            self._agent._chat = new_chat
            self._agent._interaction_id = None
            self._agent._log(
                "anima_compact",
                before_tokens=before_tokens,
                after_tokens=after_tokens,
            )

        usage = self._agent.get_token_usage()
        return {
            "status": "ok",
            "context_tokens": usage.get("ctx_total_tokens", 0),
        }


def setup(agent: "BaseAgent") -> AnimaManager:
    """Set up anima capability — self-knowledge management."""
    mgr = AnimaManager(agent)
    mgr._original_system = agent.override_intrinsic("system")
    agent.add_tool(
        "anima", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )
    return mgr
