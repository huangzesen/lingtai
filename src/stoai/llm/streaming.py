"""Streaming accumulator for LLM response deltas.

Collects text, tool-call, and thought fragments during streaming and
finalizes them into an LLMResponse.  Provider-agnostic — each adapter
feeds deltas through the accumulator's methods, then calls finalize().
"""

from __future__ import annotations

import json
from typing import Any

from .base import LLMResponse, ToolCall, UsageMetadata


class StreamingAccumulator:
    """Collects streaming deltas and finalizes into an LLMResponse.

    Supports two tool-call styles:

    **Sequential** (Anthropic, OpenAI Responses) — one tool at a time::

        acc.start_tool(id="toolu_1", name="read_file")
        acc.add_tool_args('{"path":')
        acc.add_tool_args(' "foo.py"}')
        acc.finish_tool()

    **Index-keyed** (OpenAI Completions) — concurrent tools by index::

        acc.add_tool_delta(index=0, id="call_1", name="read", args_delta='{"p')
        acc.add_tool_delta(index=1, id="call_2", name="write", args_delta='{"')
        acc.add_tool_delta(index=0, args_delta='ath": "a"}')
        acc.add_tool_delta(index=1, args_delta='path": "b"}')
        acc.finish_all_tools()

    Text and thought deltas are simple appends.
    """

    def __init__(self) -> None:
        self._text_parts: list[str] = []
        self._thought_parts: list[str] = []
        self._thoughts: list[str] = []
        self._tool_calls: list[ToolCall] = []

        # Sequential tool state (Anthropic / OpenAI Responses)
        self._pending_tool: dict[str, str] | None = None

        # Index-keyed tool state (OpenAI Completions)
        self._pending_tools_by_index: dict[int, dict[str, str]] = {}

    # -- Text ---------------------------------------------------------------

    def add_text(self, delta: str) -> None:
        """Append a text delta."""
        self._text_parts.append(delta)

    # -- Thoughts -----------------------------------------------------------

    def add_thought(self, delta: str) -> None:
        """Append a thinking/reasoning delta to the current thought block."""
        self._thought_parts.append(delta)

    def finish_thought(self) -> None:
        """Close the current thought block (e.g. on content_block_stop)."""
        if self._thought_parts:
            self._thoughts.append("".join(self._thought_parts))
            self._thought_parts = []

    # -- Sequential tool calls (Anthropic, OpenAI Responses) ----------------

    def start_tool(self, *, id: str, name: str) -> None:
        """Begin accumulating a new tool call."""
        self._pending_tool = {"id": id, "name": name, "args_json": ""}

    def add_tool_args(self, delta: str) -> None:
        """Append JSON argument fragment to the current pending tool."""
        if self._pending_tool is not None:
            self._pending_tool["args_json"] += delta

    def finish_tool(self) -> None:
        """Finalize the current pending tool call."""
        if self._pending_tool is not None:
            self._tool_calls.append(_finalize_tool(self._pending_tool))
            self._pending_tool = None

    # -- Index-keyed tool calls (OpenAI Completions) ------------------------

    def add_tool_delta(
        self,
        index: int,
        *,
        id: str | None = None,
        name: str | None = None,
        args_delta: str | None = None,
    ) -> None:
        """Feed an index-keyed tool-call delta (OpenAI Completions style)."""
        if index not in self._pending_tools_by_index:
            self._pending_tools_by_index[index] = {
                "id": id or "",
                "name": name or "",
                "args_json": "",
            }
        entry = self._pending_tools_by_index[index]
        if id and not entry["id"]:
            entry["id"] = id
        if name and not entry["name"]:
            entry["name"] = name
        if args_delta:
            entry["args_json"] += args_delta

    def finish_all_tools(self) -> None:
        """Finalize all index-keyed pending tool calls."""
        for idx in sorted(self._pending_tools_by_index):
            self._tool_calls.append(
                _finalize_tool(self._pending_tools_by_index[idx])
            )
        self._pending_tools_by_index.clear()

    # -- Atomic tool call (Gemini Interactions) -----------------------------

    def add_tool(self, tool_call: ToolCall) -> None:
        """Add a fully-formed tool call (no accumulation needed)."""
        self._tool_calls.append(tool_call)

    # -- Finalization -------------------------------------------------------

    @property
    def text(self) -> str:
        """Joined text accumulated so far."""
        return "".join(self._text_parts)

    @property
    def tool_calls(self) -> list[ToolCall]:
        """Tool calls accumulated so far."""
        return self._tool_calls

    @property
    def thoughts(self) -> list[str]:
        """Completed thought blocks. Includes any unfinished block."""
        result = list(self._thoughts)
        if self._thought_parts:
            result.append("".join(self._thought_parts))
        return result

    def finalize(
        self,
        usage: UsageMetadata | None = None,
        raw: Any = None,
    ) -> LLMResponse:
        """Build the final LLMResponse from all accumulated deltas.

        Any pending thought block is automatically closed.
        Index-keyed tools are NOT auto-finalized — call finish_all_tools()
        explicitly before finalize() if using the index-keyed style.
        """
        # Close any open thought block
        self.finish_thought()

        # Consolidate thoughts into a single entry if multiple deltas
        thoughts = self._thoughts
        if len(thoughts) > 1:
            thoughts = ["".join(thoughts)]

        return LLMResponse(
            text=self.text,
            tool_calls=self._tool_calls,
            usage=usage or UsageMetadata(),
            thoughts=thoughts,
            raw=raw,
        )


def _finalize_tool(pending: dict[str, str]) -> ToolCall:
    """Parse a pending tool dict into a ToolCall."""
    args_json = pending["args_json"]
    try:
        args = json.loads(args_json) if args_json else {}
    except json.JSONDecodeError:
        args = {}
    return ToolCall(name=pending["name"], args=args, id=pending["id"] or None)
