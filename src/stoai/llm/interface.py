"""Canonical LLM interaction interface.

Provides a provider-agnostic representation of the full program-LLM
interaction.  This is the single source of truth for conversation history.
Adapters rebuild provider-specific message formats from this on each API call.

Each ChatInterface instance is owned by one agent thread.  Not thread-safe.
Do not share across threads.
"""

from __future__ import annotations

import base64
import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any, Union


# ---------------------------------------------------------------------------
# Content blocks
# ---------------------------------------------------------------------------


@dataclass
class TextBlock:
    text: str

    def to_dict(self) -> dict:
        return {"type": "text", "text": self.text}


@dataclass
class ToolCallBlock:
    id: str
    name: str
    args: dict

    def to_dict(self) -> dict:
        return {"type": "tool_call", "id": self.id, "name": self.name, "args": self.args}


@dataclass
class ToolResultBlock:
    id: str
    name: str
    content: Any  # str or dict

    def to_dict(self) -> dict:
        return {"type": "tool_result", "id": self.id, "name": self.name, "content": self.content}


@dataclass
class ThinkingBlock:
    text: str
    provider_data: dict = field(default_factory=dict)

    def to_dict(self) -> dict:
        d: dict = {"type": "thinking", "text": self.text}
        if self.provider_data:
            d["provider_data"] = self.provider_data
        return d


@dataclass
class ImageBlock:
    data: bytes
    mime_type: str = "image/png"

    def to_dict(self) -> dict:
        return {
            "type": "image",
            "data": base64.b64encode(self.data).decode("ascii"),
            "mime_type": self.mime_type,
        }


ContentBlock = Union[TextBlock, ToolCallBlock, ToolResultBlock, ThinkingBlock, ImageBlock]


def content_block_from_dict(d: dict) -> ContentBlock:
    """Deserialize a content block from its dict representation."""
    btype = d["type"]
    if btype == "text":
        return TextBlock(text=d["text"])
    elif btype == "tool_call":
        return ToolCallBlock(id=d["id"], name=d["name"], args=d["args"])
    elif btype == "tool_result":
        return ToolResultBlock(id=d["id"], name=d["name"], content=d["content"])
    elif btype == "thinking":
        return ThinkingBlock(text=d["text"], provider_data=d.get("provider_data", {}))
    elif btype == "image":
        return ImageBlock(data=base64.b64decode(d["data"]), mime_type=d["mime_type"])
    else:
        raise ValueError(f"Unknown content block type: {btype}")


# ---------------------------------------------------------------------------
# InterfaceEntry
# ---------------------------------------------------------------------------


@dataclass
class InterfaceEntry:
    id: int
    role: str  # "system" | "user" | "assistant"
    content: list[ContentBlock]
    timestamp: float
    provider_data: dict = field(default_factory=dict)
    model: str | None = None       # which model produced this (assistant only)
    provider: str | None = None    # which provider (assistant only)
    usage: dict = field(default_factory=dict)  # per-message token usage
    _tools: list[dict] | None = field(default=None, repr=False)  # tools snapshot (system entries)

    def to_dict(self) -> dict:
        if self.role == "system":
            d: dict = {
                "id": self.id,
                "role": self.role,
                "system": self.content[0].text if self.content else "",
                "timestamp": self.timestamp,
            }
            if self._tools is not None:
                d["tools"] = self._tools
            return d
        d = {
            "id": self.id,
            "role": self.role,
            "content": [b.to_dict() for b in self.content],
            "timestamp": self.timestamp,
        }
        if self.provider_data:
            d["provider_data"] = self.provider_data
        if self.model is not None:
            d["model"] = self.model
        if self.provider is not None:
            d["provider"] = self.provider
        if self.usage:
            d["usage"] = self.usage
        return d

    @staticmethod
    def from_dict(d: dict) -> InterfaceEntry:
        if d["role"] == "system" and "system" in d:
            entry = InterfaceEntry(
                id=d["id"],
                role="system",
                content=[TextBlock(text=d["system"])],
                timestamp=d["timestamp"],
            )
            entry._tools = d.get("tools")
            return entry
        return InterfaceEntry(
            id=d["id"],
            role=d["role"],
            content=[content_block_from_dict(b) for b in d["content"]],
            timestamp=d["timestamp"],
            provider_data=d.get("provider_data", {}),
            model=d.get("model"),
            provider=d.get("provider"),
            usage=d.get("usage", {}),
        )


# ---------------------------------------------------------------------------
# ChatInterface
# ---------------------------------------------------------------------------


class ChatInterface:
    """Append-only log of canonical LLM interaction entries.

    Single source of truth for conversation history.  Adapters rebuild
    provider-specific formats from this on each API call.

    Not thread-safe.  Each instance is owned by one agent thread.
    """

    def __init__(self) -> None:
        self._entries: list[InterfaceEntry] = []
        self._next_id: int = 0
        self._current_system_text: str | None = None
        self._current_tools: list[dict] | None = None

    @property
    def entries(self) -> list[InterfaceEntry]:
        return self._entries

    @property
    def current_system_prompt(self) -> str | None:
        return self._current_system_text

    @property
    def current_tools(self) -> list[dict] | None:
        return self._current_tools

    def _append(self, role: str, content: list[ContentBlock], provider_data: dict | None = None) -> InterfaceEntry:
        entry = InterfaceEntry(
            id=self._next_id,
            role=role,
            content=content,
            timestamp=time.time(),
            provider_data=provider_data or {},
        )
        self._entries.append(entry)
        self._next_id += 1
        return entry

    # -- Sanitization ---------------------------------------------------------

    def enforce_tool_pairing(self) -> None:
        """Ensure every ToolCallBlock has a matching ToolResultBlock and vice versa.

        Walks all entries and:
        - Strips ToolCallBlocks from assistant entries that have no matching
          ToolResultBlock anywhere in subsequent user entries.
        - Strips ToolResultBlocks from user entries that have no matching
          ToolCallBlock in any preceding assistant entry.
        - If stripping leaves an entry with no content blocks, inserts a
          placeholder TextBlock.

        Mutates entries in place.  Idempotent.
        """
        # Collect all tool call IDs and all answered IDs
        all_call_ids: set[str] = set()
        answered_ids: set[str] = set()
        for entry in self._entries:
            for block in entry.content:
                if isinstance(block, ToolCallBlock):
                    all_call_ids.add(block.id)
                elif isinstance(block, ToolResultBlock):
                    answered_ids.add(block.id)

        # Nothing to fix if sets match
        if all_call_ids == answered_ids:
            return

        for entry in self._entries:
            if entry.role == "assistant":
                stripped_names: list[str] = []
                new_content: list[ContentBlock] = []
                for block in entry.content:
                    if isinstance(block, ToolCallBlock) and block.id not in answered_ids:
                        stripped_names.append(block.name)
                        continue
                    new_content.append(block)
                if not new_content:
                    new_content.append(TextBlock(
                        text=f"[Tool calls {', '.join(stripped_names)} were cancelled before completion.]"
                    ))
                entry.content = new_content

            elif entry.role == "user":
                new_content_u: list[ContentBlock] = []
                orphaned_names: list[str] = []
                for block in entry.content:
                    if isinstance(block, ToolResultBlock) and block.id not in all_call_ids:
                        orphaned_names.append(block.name)
                        continue
                    new_content_u.append(block)
                if orphaned_names:
                    new_content_u.append(TextBlock(
                        text=f"[Tool results ignored (no matching tool call): {', '.join(orphaned_names)}]"
                    ))
                if new_content_u:
                    entry.content = new_content_u

    # -- Add methods ----------------------------------------------------------

    def add_system(self, text: str, tools: list[dict] | None = None) -> None:
        """Record a system prompt + tools.  Only adds entry if either changed."""
        if text == self._current_system_text and tools == self._current_tools:
            return
        self._current_system_text = text
        self._current_tools = tools
        entry = self._append("system", [TextBlock(text=text)])
        entry._tools = tools

    def add_user_message(
        self,
        text: str,
        *,
        image_bytes: bytes | None = None,
        mime_type: str = "image/png",
    ) -> InterfaceEntry:
        blocks: list[ContentBlock] = [TextBlock(text=text)]
        if image_bytes is not None:
            blocks.append(ImageBlock(data=image_bytes, mime_type=mime_type))
        return self._append("user", blocks)

    def add_assistant_message(
        self,
        content: list[ContentBlock],
        provider_data: dict | None = None,
        *,
        model: str | None = None,
        provider: str | None = None,
        usage: dict | None = None,
    ) -> InterfaceEntry:
        entry = self._append("assistant", content, provider_data)
        entry.model = model
        entry.provider = provider
        entry.usage = usage or {}
        return entry

    def add_user_blocks(self, blocks: list[ContentBlock]) -> InterfaceEntry:
        """Record a user entry with pre-built content blocks (for converters)."""
        return self._append("user", blocks)

    def add_tool_results(self, results: list[ToolResultBlock]) -> InterfaceEntry:
        """Record tool results as a user-role entry."""
        return self._append("user", list(results))

    # -- Query methods --------------------------------------------------------

    def conversation_entries(self) -> list[InterfaceEntry]:
        """Return entries excluding system prompt entries."""
        return [e for e in self._entries if e.role != "system"]

    def last_assistant_entry(self) -> InterfaceEntry | None:
        """Return the most recent assistant entry, or None."""
        for e in reversed(self._entries):
            if e.role == "assistant":
                return e
        return None

    # -- Usage helpers ---------------------------------------------------------

    def total_usage(self) -> dict:
        """Sum tokens and count API calls across all assistant messages."""
        totals = {"input_tokens": 0, "output_tokens": 0, "thinking_tokens": 0, "calls": 0}
        for entry in self._entries:
            if entry.role == "assistant" and entry.usage:
                totals["input_tokens"] += entry.usage.get("input_tokens", 0)
                totals["output_tokens"] += entry.usage.get("output_tokens", 0)
                totals["thinking_tokens"] += entry.usage.get("thinking_tokens", 0)
                totals["calls"] += 1
        return totals

    def usage_by_model(self) -> dict[str, dict]:
        """Breakdown of usage per model name."""
        by_model: dict[str, dict] = {}
        for entry in self._entries:
            if entry.role == "assistant" and entry.model and entry.usage:
                if entry.model not in by_model:
                    by_model[entry.model] = {
                        "input_tokens": 0, "output_tokens": 0,
                        "thinking_tokens": 0, "calls": 0,
                    }
                by_model[entry.model]["input_tokens"] += entry.usage.get("input_tokens", 0)
                by_model[entry.model]["output_tokens"] += entry.usage.get("output_tokens", 0)
                by_model[entry.model]["thinking_tokens"] += entry.usage.get("thinking_tokens", 0)
                by_model[entry.model]["calls"] += 1
        return by_model

    # -- Truncation (for _on_reset rollback) ----------------------------------

    def drop_trailing(self, predicate: Callable[[InterfaceEntry], bool]) -> list[InterfaceEntry]:
        """Pop entries from the end while predicate is True.  Returns dropped entries."""
        dropped: list[InterfaceEntry] = []
        while self._entries and predicate(self._entries[-1]):
            dropped.append(self._entries.pop())
        dropped.reverse()
        return dropped

    def truncate_to(self, entry_id: int) -> list[InterfaceEntry]:
        """Remove entries with id > entry_id.  Returns removed entries."""
        idx = None
        for i, e in enumerate(self._entries):
            if e.id == entry_id:
                idx = i
                break
        if idx is None:
            return []
        removed = self._entries[idx + 1:]
        self._entries = self._entries[:idx + 1]
        return removed

    def truncate(self, max_entries: int = 20, keep_recent: int | None = None) -> None:
        """Truncate interface to max_entries, preserving system prompt.

        Args:
            max_entries: Maximum non-system entries to keep.
            keep_recent: If set, keep this many most recent non-system entries
                         at the end (for context window management). Without this,
                         keeps the first max_entries (oldest).
        """
        has_system = self._entries and self._entries[0].role == "system"
        non_system_entries = [e for e in self._entries if e.role != "system"]

        if len(non_system_entries) <= max_entries:
            return  # Nothing to truncate

        if keep_recent is not None:
            # Keep system (if any), then keep_recent entries at the end
            keep_from = len(non_system_entries) - keep_recent
            keep_from = max(0, keep_from)
            kept_non_system = non_system_entries[keep_from:]
        else:
            # Keep first max_entries non-system entries (no keep_recent)
            kept_non_system = non_system_entries[:max_entries]

        # Rebuild entries: system + kept non-system
        if has_system:
            self._entries = [self._entries[0]] + kept_non_system
        else:
            self._entries = kept_non_system

    def to_messages(self) -> list[dict]:
        """Convert to simple message list (role + content dicts).

        Used for adapters that need a basic message format.
        """
        messages = []
        for entry in self._entries:
            if entry.role == "system":
                continue  # Skip system in to_messages
            content = []
            for block in entry.content:
                if isinstance(block, TextBlock):
                    content.append({"type": "text", "text": block.text})
                elif isinstance(block, ToolCallBlock):
                    content.append(block.to_dict())
                elif isinstance(block, ToolResultBlock):
                    content.append(block.to_dict())
                elif isinstance(block, ThinkingBlock):
                    content.append({"type": "thinking", "text": block.text})
                elif isinstance(block, ImageBlock):
                    content.append(block.to_dict())
            messages.append({"role": entry.role, "content": content})
        return messages

    # -- Compaction helpers ----------------------------------------------------

    def estimate_context_tokens(
        self,
        system_prompt: str | None = None,
        tools: list[dict] | None = None,
    ) -> int:
        """Count tokens in system + tools + all messages using Google tokenizer.

        Uses the canonical entries (not provider-specific formats), so the
        estimate is provider-agnostic.  Falls back to current_system_prompt
        and current_tools if explicit args are not provided.
        """
        from ..token_counter import count_tokens
        import json

        total = 0

        # System prompt
        sp = system_prompt if system_prompt is not None else self._current_system_text
        if sp:
            total += count_tokens(sp)

        # Tool definitions
        t = tools if tools is not None else self._current_tools
        if t:
            total += count_tokens(json.dumps(t, default=str))

        # Conversation entries (skip system — already counted above)
        for entry in self._entries:
            if entry.role == "system":
                continue
            for block in entry.content:
                if isinstance(block, TextBlock):
                    total += count_tokens(block.text)
                elif isinstance(block, ToolCallBlock):
                    total += count_tokens(
                        f"{block.name}({json.dumps(block.args, default=str)})"
                    )
                elif isinstance(block, ToolResultBlock):
                    content_str = (
                        block.content
                        if isinstance(block.content, str)
                        else json.dumps(block.content, default=str)
                    )
                    total += count_tokens(content_str)
                elif isinstance(block, ThinkingBlock):
                    total += count_tokens(block.text)
                elif isinstance(block, ImageBlock):
                    # Rough estimate: images are ~1K tokens
                    total += 1000

        return total

    def find_compaction_boundary(self, keep_turns: int = 3) -> int | None:
        """Find entry index where compaction should split.

        Keeps the last *keep_turns* complete user-initiated turns intact.
        A "turn" starts with a user text message (not tool results).
        Tool-use/tool-result exchanges within a turn are never split.

        Returns the entry *id* at which to split (entries [0..id) get
        summarized, entries [id..] are kept), or None if there aren't
        enough turns to compact.
        """
        conv = [e for e in self._entries if e.role != "system"]
        if len(conv) < 6:  # need meaningful history to compact
            return None

        # Walk backward counting turn boundaries.
        # A turn boundary is a user entry whose content is NOT purely
        # ToolResultBlock — i.e., it contains a TextBlock (real user message).
        turns_found = 0
        boundary_idx = None
        for i in range(len(conv) - 1, -1, -1):
            entry = conv[i]
            if entry.role == "user":
                has_text = any(isinstance(b, TextBlock) for b in entry.content)
                has_only_tool_results = all(
                    isinstance(b, ToolResultBlock) for b in entry.content
                )
                if has_text and not has_only_tool_results:
                    turns_found += 1
                    if turns_found >= keep_turns:
                        boundary_idx = i
                        break

        if boundary_idx is None or boundary_idx <= 0:
            return None

        # The boundary entry id — everything before this gets summarized
        return conv[boundary_idx].id

    def format_for_summary(self, up_to_entry_id: int) -> str:
        """Format entries [0..up_to_entry_id) as text for summarization.

        Drops thinking blocks, truncates long tool results.
        """
        import json

        parts: list[str] = []
        for entry in self._entries:
            if entry.id >= up_to_entry_id:
                break
            if entry.role == "system":
                continue

            for block in entry.content:
                if isinstance(block, TextBlock):
                    parts.append(f"[{entry.role}] {block.text}")
                elif isinstance(block, ToolCallBlock):
                    args_str = json.dumps(block.args, default=str)[:200]
                    parts.append(
                        f"[{entry.role}] tool_use: {block.name}({args_str})"
                    )
                elif isinstance(block, ToolResultBlock):
                    content_str = (
                        block.content
                        if isinstance(block.content, str)
                        else json.dumps(block.content, default=str)
                    )
                    parts.append(
                        f"[{entry.role}] tool_result({block.name}): {content_str}"
                    )
                elif isinstance(block, ThinkingBlock):
                    pass  # Drop thinking — large and not actionable
                elif isinstance(block, ImageBlock):
                    parts.append(f"[{entry.role}] [image: {block.mime_type}]")

        return "\n".join(parts)

    # -- Serialization --------------------------------------------------------

    def to_dict(self) -> list[dict]:
        return [e.to_dict() for e in self._entries]

    @classmethod
    def from_dict(cls, data: list[dict]) -> ChatInterface:
        iface = cls()
        for d in data:
            entry = InterfaceEntry.from_dict(d)
            iface._entries.append(entry)
            if entry.role == "system" and entry.content:
                block = entry.content[0]
                if isinstance(block, TextBlock):
                    iface._current_system_text = block.text
                iface._current_tools = entry._tools
        if iface._entries:
            iface._next_id = max(e.id for e in iface._entries) + 1
        return iface
