"""Anima capability — self-knowledge management.

Upgrades the system intrinsic (like email upgrades mail).
Adds evolving identity (covenant + character), structured memory,
and on-demand context compaction.

Usage:
    agent = Agent(capabilities=["anima"])
"""
from __future__ import annotations

import hashlib
import json
import os
import re
import tempfile
from datetime import datetime, timezone
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Callable

    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "object": {
            "type": "string",
            "enum": ["character", "library", "memory", "context"],
            "description": (
                "character: your evolving identity — what makes you special.\n"
                "library: your knowledge archive (system/library.json).\n"
                "memory: your active working memory "
                "(system/memory.md, loaded from library).\n"
                "context: your conversation context window."
            ),
        },
        "action": {
            "type": "string",
            "enum": [
                "update", "diff", "load",
                "submit", "filter", "view", "consolidate", "delete",
                "compact", "forget",
            ],
            "description": (
                "character: update | diff | load.\n"
                "library: submit | filter | view | consolidate | delete.\n"
                "memory: load | diff.\n"
                "context: compact | forget."
            ),
        },
        "title": {
            "type": "string",
            "description": (
                "Entry title — one line. "
                "Required for library submit and consolidate."
            ),
        },
        "summary": {
            "type": "string",
            "description": (
                "Entry summary — 1-3 sentences. Used for filtering. "
                "Required for library submit and consolidate."
            ),
        },
        "content": {
            "type": "string",
            "description": (
                "Text content — for character update (your identity profile), "
                "library submit/consolidate (main body, up to 500 words), "
                "or other actions that accept text."
            ),
        },
        "supplementary": {
            "type": "string",
            "description": (
                "Extended material for a library entry — unbounded. "
                "Optional for library submit and consolidate. "
                "Use when the content alone doesn't capture full detail."
            ),
        },
        "ids": {
            "type": "array",
            "items": {"type": "string"},
            "description": (
                "Entry IDs — for library view, consolidate, delete, "
                "and memory load."
            ),
        },
        "pattern": {
            "type": "string",
            "description": (
                "Regex pattern for library filter. "
                "Searches across titles, summaries, and content. "
                "Omit to list all entries."
            ),
        },
        "limit": {
            "type": "integer",
            "description": "Maximum entries to return for library filter.",
        },
        "depth": {
            "type": "string",
            "enum": ["content", "supplementary"],
            "description": (
                "Depth for library view. "
                "'content' (default): id + title + summary + content. "
                "'supplementary': id + title + summary + content + supplementary."
            ),
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
    "Self-knowledge management — identity, knowledge library, active memory, "
    "and context control.\n"
    "character: your evolving identity — what makes you *you*. "
    "Your personality, expertise, working style, and goals. Be active about "
    "updating this — your character is what distinguishes you from other agents. "
    "A well-developed character improves your autonomy and effectiveness. "
    "Consider structuring your character with sections like: "
    "Expertise (what you're good at), Tools & Packages (what you use), "
    "MCP Servers (what services you interface with), "
    "Pipelines (workflows you've mastered). "
    "update to write your character (write your full profile, it replaces previous), "
    "diff to review changes, load to apply.\n"
    "library: your knowledge archive — proactively insert knowledge for future "
    "reference. This is part of you: your knowledge defines your expertise. "
    "Academic style: title + summary + content (optional supplementary). "
    "submit to add entries. filter to browse (returns id + title + summary, "
    "optional regex pattern and limit). view to read at depth "
    "(content or supplementary). consolidate to merge entries. "
    "delete to remove. Be a thoughtful librarian — write clear titles, "
    "concise summaries (1-3 sentences), and structured content (up to 500 words).\n"
    "memory: load selected library entries into active memory by IDs "
    "(injects id + title + content into system prompt). "
    "diff to see uncommitted changes (inherited, rarely needed).\n"
    "context: compact to proactively free context space. "
    "forget to nuke conversation history and start fresh "
    "(email mailbox persists independently). "
    "Check usage via status show first.\n"
    "Workflow: filter to browse → view if you need detail → load to remember."
)


class AnimaManager:
    """Self-knowledge manager — character, memory, context."""

    def __init__(self, agent: "BaseAgent"):
        self._agent = agent
        self._working_dir = agent._working_dir
        self._original_system: Callable[[dict], dict] | None = None

        # Paths
        system_dir = self._working_dir / "system"
        self._covenant_path = system_dir / "covenant.md"
        self._character_path = system_dir / "character.md"
        self._memory_md = system_dir / "memory.md"
        self._library_json = system_dir / "library.json"

        # In-memory cache of entries
        self._entries: list[dict] = self._load_entries()

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    def _load_entries(self) -> list[dict]:
        """Load entries from library.json, or return empty list if missing."""
        if not self._library_json.is_file():
            return []
        try:
            data = json.loads(self._library_json.read_text())
            entries = data.get("entries", [])
            # Migrate legacy flat entries (pre-library format)
            for e in entries:
                if "title" not in e:
                    e["title"] = e.get("content", "")[:50] or "Untitled"
                    e["summary"] = e.get("content", "")[:200]
                    e["supplementary"] = ""
            return entries
        except (json.JSONDecodeError, OSError):
            return []

    def _save_entries(self) -> None:
        """Write entries to library.json with atomic write."""
        data = {"version": 1, "entries": self._entries}
        self._library_json.parent.mkdir(exist_ok=True)
        fd, tmp = tempfile.mkstemp(
            dir=str(self._library_json.parent), suffix=".tmp",
        )
        try:
            os.write(fd, json.dumps(data, indent=2, ensure_ascii=False).encode())
            os.close(fd)
            os.replace(tmp, str(self._library_json))
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise

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
        "character": {"update", "diff", "load"},
        "library": {"submit", "filter", "view", "consolidate", "delete"},
        "memory": {"load", "diff"},
        "context": {"compact", "forget"},
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
    # Character actions
    # ------------------------------------------------------------------

    def _character_update(self, args: dict) -> dict:
        content = args.get("content", "")
        self._character_path.parent.mkdir(exist_ok=True)
        self._character_path.write_text(content)
        return {"status": "ok", "path": str(self._character_path)}

    def _character_diff(self, _args: dict) -> dict:
        diff_text = self._agent._workdir.diff("system/character.md")
        return {"status": "ok", "path": str(self._character_path), "git_diff": diff_text}

    def _character_load(self, _args: dict) -> dict:
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
        git_diff, commit_hash = self._agent._workdir.diff_and_commit(
            rel_path, "character",
        )

        self._agent._log(
            "anima_character_load",
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

    def _library_submit(self, args: dict) -> dict:
        title = args.get("title", "").strip()
        summary = args.get("summary", "").strip()
        content = args.get("content", "").strip()
        supplementary = args.get("supplementary", "").strip()
        if not title:
            return {"error": "title is required for library submit."}
        if not summary:
            return {"error": "summary is required for library submit."}
        if not content:
            return {"error": "content is required for library submit."}
        now = datetime.now(timezone.utc).isoformat()
        entry_id = self._make_id(title + content, now)
        self._entries.append({
            "id": entry_id,
            "title": title,
            "summary": summary,
            "content": content,
            "supplementary": supplementary,
            "created_at": now,
        })
        self._save_entries()
        return {"status": "ok", "id": entry_id}

    def _library_filter(self, args: dict) -> dict:
        pattern = args.get("pattern")
        limit = args.get("limit")
        entries = self._entries
        if pattern:
            try:
                rx = re.compile(pattern, re.IGNORECASE)
            except re.error as exc:
                return {"error": f"Invalid regex pattern: {exc}"}
            entries = [
                e for e in entries
                if rx.search(e["title"])
                or rx.search(e["summary"])
                or rx.search(e["content"])
            ]
        if limit is not None and limit > 0:
            entries = entries[:limit]
        return {
            "status": "ok",
            "entries": [
                {"id": e["id"], "title": e["title"], "summary": e["summary"]}
                for e in entries
            ],
        }

    def _library_view(self, args: dict) -> dict:
        ids = args.get("ids")
        if not ids:
            return {"error": "ids is required for library view."}
        depth = args.get("depth", "content")

        entries_by_id = {e["id"]: e for e in self._entries}
        invalid = [i for i in ids if i not in entries_by_id]
        if invalid:
            return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

        result_entries = []
        for entry_id in ids:
            e = entries_by_id[entry_id]
            item = {
                "id": e["id"],
                "title": e["title"],
                "summary": e["summary"],
                "content": e["content"],
            }
            if depth == "supplementary":
                item["supplementary"] = e.get("supplementary", "")
            result_entries.append(item)

        return {"status": "ok", "entries": result_entries}

    def _library_consolidate(self, args: dict) -> dict:
        ids = args.get("ids")
        title = args.get("title", "").strip()
        summary = args.get("summary", "").strip()
        content = args.get("content", "").strip()
        supplementary = args.get("supplementary", "").strip()
        if not ids:
            return {"error": "ids is required for library consolidate."}
        if not title:
            return {"error": "title is required for library consolidate."}
        if not summary:
            return {"error": "summary is required for library consolidate."}
        if not content:
            return {"error": "content is required for library consolidate."}

        existing_ids = {e["id"] for e in self._entries}
        invalid = [i for i in ids if i not in existing_ids]
        if invalid:
            return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

        ids_set = set(ids)
        self._entries = [e for e in self._entries if e["id"] not in ids_set]

        now = datetime.now(timezone.utc).isoformat()
        new_id = self._make_id(title + content, now)
        self._entries.append({
            "id": new_id,
            "title": title,
            "summary": summary,
            "content": content,
            "supplementary": supplementary,
            "created_at": now,
        })

        self._save_entries()
        return {"status": "ok", "id": new_id, "removed": len(ids)}

    def _library_delete(self, args: dict) -> dict:
        ids = args.get("ids")
        if not ids:
            return {"error": "ids is required for library delete."}

        existing_ids = {e["id"] for e in self._entries}
        invalid = [i for i in ids if i not in existing_ids]
        if invalid:
            return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

        ids_set = set(ids)
        before = len(self._entries)
        self._entries = [e for e in self._entries if e["id"] not in ids_set]
        removed = before - len(self._entries)

        self._save_entries()
        return {"status": "ok", "removed": removed}

    def _memory_diff(self, args: dict) -> dict:
        # Delegate to original system handler
        if self._original_system is None:
            return {"error": "anima not properly initialized (missing system handler)"}
        return self._original_system({"action": "diff", "object": "memory"})

    def _memory_load(self, args: dict) -> dict:
        ids = args.get("ids")
        if not ids:
            return {"error": "ids is required for memory load."}

        entries_by_id = {e["id"]: e for e in self._entries}
        invalid = [i for i in ids if i not in entries_by_id]
        if invalid:
            return {"error": f"Unknown library IDs: {', '.join(invalid)}"}

        # Render memory.md with id + title + content
        lines = []
        for entry_id in ids:
            e = entries_by_id[entry_id]
            lines.append(f"### [{e['id']}] {e['title']}\n{e['content']}")
        content = "\n\n".join(lines) + ("\n" if lines else "")

        # Write to disk
        self._memory_md.parent.mkdir(exist_ok=True)
        self._memory_md.write_text(content)

        # Inject into system prompt
        if content.strip():
            self._agent._prompt_manager.write_section("memory", content)
        else:
            self._agent._prompt_manager.delete_section("memory")
        self._agent._token_decomp_dirty = True

        if self._agent._chat is not None:
            self._agent._chat.update_system_prompt(
                self._agent._build_system_prompt()
            )

        # Git commit
        rel_path = "system/memory.md"
        git_diff, commit_hash = self._agent._workdir.diff_and_commit(
            rel_path, "memory",
        )

        self._agent._log(
            "anima_memory_load",
            entry_count=len(ids),
            changed=commit_hash is not None,
        )

        return {
            "status": "ok",
            "loaded": len(ids),
            "size_bytes": len(content.encode("utf-8")),
            "content_preview": content[:200],
            "diff": {
                "changed": commit_hash is not None,
                "git_diff": git_diff or "",
                "commit": commit_hash,
            },
        }

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
        target_tokens = min(int(ctx_window * 0.2), 16384) if ctx_window > 0 else 2048

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

    def _context_forget(self, args: dict) -> dict:
        """Nuke the entire conversation context and start a fresh session."""
        if self._agent._chat is None:
            return {"error": "No active chat session."}

        before_tokens = self._agent._chat.interface.estimate_context_tokens()
        self._agent._session._chat = None
        self._agent._session._interaction_id = None
        self._agent._session.ensure_session()
        self._agent._log("anima_forget", before_tokens=before_tokens)

        return {"status": "ok", "freed_tokens": before_tokens}


def setup(agent: "BaseAgent") -> AnimaManager:
    """Set up anima capability — self-knowledge management."""
    mgr = AnimaManager(agent)
    mgr._original_system = agent.override_intrinsic("system")
    agent._anima_owns_memory = True

    # Migrate existing memory.md content to library as a seed entry
    memory_file = agent._working_dir / "system" / "memory.md"
    if memory_file.is_file():
        existing = memory_file.read_text().strip()
        if existing and not mgr._entries:
            mgr.handle({
                "object": "library", "action": "submit",
                "title": "Initial memory (migrated)",
                "summary": existing[:200],
                "content": existing,
            })

    agent.add_tool(
        "anima", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt="Manage your identity, knowledge library, and memory.",
    )
    return mgr
