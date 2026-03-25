"""Anthropic adapter — wraps the ``anthropic`` SDK for Claude models.

This is the **only** module that imports the ``anthropic`` package.

Key Anthropic API differences from OpenAI/Gemini:
- System prompt is a separate ``system`` parameter, not a message.
- Strict user/assistant alternation required — consecutive same-role messages
  must be merged.
- Tool results are sent inside a ``user`` message with ``tool_result`` blocks.
- Thinking/extended thinking is controlled via a ``thinking`` parameter with
  a token budget.
"""

from __future__ import annotations

import json
import uuid
from collections.abc import Callable
from typing import Any

import anthropic

from lingtai_kernel.logging import get_logger

logger = get_logger()

from lingtai_kernel.llm.base import (
    ChatSession,
    FunctionSchema,
    LLMResponse,
    ToolCall,
    UsageMetadata,
)
from lingtai_kernel.llm.interface import ToolResultBlock
from lingtai.llm.base import LLMAdapter
from lingtai_kernel.llm.interface import ChatInterface
from ..interface_converters import to_anthropic
from lingtai_kernel.llm.streaming import StreamingAccumulator


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _build_tools(
    schemas: list[FunctionSchema] | None, *, cache_tools: bool = False
) -> list[dict] | None:
    """Convert FunctionSchema list to Anthropic tool format."""
    if not schemas:
        return None
    tools = [
        {
            "name": s.name,
            "description": s.description,
            "input_schema": s.parameters,
        }
        for s in schemas
    ]
    if cache_tools and tools:
        tools[-1]["cache_control"] = {"type": "ephemeral"}
    return tools


def _build_system_with_cache(system_prompt: str) -> list[dict]:
    """Build system prompt as cached content blocks for Anthropic."""
    if not system_prompt:
        return []
    return [
        {
            "type": "text",
            "text": system_prompt,
            "cache_control": {"type": "ephemeral"},
        }
    ]


def _parse_response(raw) -> LLMResponse:
    """Parse an Anthropic Messages response into a provider-agnostic LLMResponse."""
    text_parts: list[str] = []
    tool_calls: list[ToolCall] = []
    thoughts: list[str] = []

    for block in raw.content:
        if block.type == "text":
            text_parts.append(block.text)
        elif block.type == "tool_use":
            tool_calls.append(
                ToolCall(
                    name=block.name,
                    args=block.input if isinstance(block.input, dict) else {},
                    id=block.id,
                )
            )
        elif block.type == "thinking":
            thinking_text = getattr(block, "thinking", None)
            if thinking_text:
                thoughts.append(thinking_text)

    # Token usage — includes cache metrics
    # Anthropic's input_tokens only counts tokens AFTER the last cache
    # breakpoint.  The true total is: input_tokens + cache_read + cache_write.
    # We normalise here so the rest of the system sees the same semantics as
    # OpenAI (prompt_tokens = total) and Gemini (prompt_token_count = total).
    usage = UsageMetadata()
    if raw.usage:
        cache_read = getattr(raw.usage, "cache_read_input_tokens", 0) or 0
        cache_write = getattr(raw.usage, "cache_creation_input_tokens", 0) or 0
        raw_input = getattr(raw.usage, "input_tokens", 0) or 0
        usage = UsageMetadata(
            input_tokens=raw_input + cache_read + cache_write,
            output_tokens=getattr(raw.usage, "output_tokens", 0) or 0,
            thinking_tokens=getattr(raw.usage, "thinking_tokens", 0) or 0,
            cached_tokens=cache_read,
        )
        if cache_read or cache_write:
            logger.debug(
                "Anthropic cache: read=%d write=%d uncached=%d total_input=%d",
                cache_read,
                cache_write,
                raw_input,
                usage.input_tokens,
            )

    return LLMResponse(
        text="\n".join(text_parts) if text_parts else "",
        tool_calls=tool_calls,
        usage=usage,
        thoughts=thoughts,
        raw=raw,
    )


def _tool_result_to_dict(block: ToolResultBlock) -> dict:
    """Convert a canonical ToolResultBlock to Anthropic tool_result dict."""
    return {
        "type": "tool_result",
        "tool_use_id": block.id,
        "content": block.content if isinstance(block.content, str) else json.dumps(block.content, default=str),
    }


def _ensure_alternation(messages: list[dict]) -> list[dict]:
    """Merge consecutive same-role messages to satisfy Anthropic's alternation rule.

    Anthropic requires strict user/assistant alternation. If two consecutive
    messages have the same role, merge their content.
    """
    if not messages:
        return messages

    merged: list[dict] = []
    for msg in messages:
        if merged and merged[-1]["role"] == msg["role"]:
            prev = merged[-1]
            # Merge content — both could be str or list
            prev_content = prev.get("content", "")
            new_content = msg.get("content", "")

            # Normalize to list form for merging
            if isinstance(prev_content, str):
                prev_list = (
                    [{"type": "text", "text": prev_content}] if prev_content else []
                )
            else:
                prev_list = list(prev_content)

            if isinstance(new_content, str):
                new_list = (
                    [{"type": "text", "text": new_content}] if new_content else []
                )
            else:
                new_list = list(new_content)

            combined = prev_list + new_list
            prev["content"] = combined
        else:
            merged.append(dict(msg))

    return merged


def _response_to_messages(raw) -> list[dict]:
    """Convert an Anthropic response into message dicts for the history."""
    result: dict[str, Any] = {"role": "assistant", "content": []}

    for block in raw.content:
        if block.type == "text":
            result["content"].append({"type": "text", "text": block.text})
        elif block.type == "tool_use":
            result["content"].append(
                {
                    "type": "tool_use",
                    "id": block.id,
                    "name": block.name,
                    "input": block.input if isinstance(block.input, dict) else {},
                }
            )
        elif block.type == "thinking":
            # Include thinking blocks so history round-trips correctly
            result["content"].append(
                {
                    "type": "thinking",
                    "thinking": getattr(block, "thinking", ""),
                    # Anthropic requires a signature for thinking blocks in history
                    "signature": getattr(block, "signature", ""),
                }
            )

    if not result["content"]:
        result["content"] = [{"type": "text", "text": ""}]

    return [result]


# ---------------------------------------------------------------------------
# AnthropicChatSession
# ---------------------------------------------------------------------------


class AnthropicChatSession(ChatSession):
    """Client-managed chat session for the Anthropic Messages API.

    Uses ChatInterface as the single source of truth. Rebuilds Anthropic
    message format from the interface on each API call.
    """

    def __init__(
        self,
        client: anthropic.Anthropic,
        model: str,
        system_prompt: str | list[dict],
        interface: ChatInterface,
        tools: list[dict] | None,
        tool_choice: dict | None,
        extra_kwargs: dict,
        client_kwargs: dict | None = None,
        context_window: int = 0,
    ):
        self._client = client
        self._model = model
        self._system = system_prompt
        self._interface = interface
        self._tools = tools
        self._tool_choice = tool_choice
        self._extra_kwargs = extra_kwargs
        self._client_kwargs = client_kwargs or {}
        self._context_window = context_window

    @property
    def interface(self) -> ChatInterface:
        """The canonical ChatInterface for this session."""
        return self._interface

    def _build_request_kwargs(self, messages: list[dict]) -> dict[str, Any]:
        kwargs: dict[str, Any] = {
            "model": self._model,
            "messages": messages,
            "max_tokens": self._extra_kwargs.get("max_tokens", 8192),
            **self._extra_kwargs,
        }
        if self._system:
            kwargs["system"] = self._system
        if self._tools:
            kwargs["tools"] = self._tools
            if self._tool_choice:
                kwargs["tool_choice"] = self._tool_choice
        return kwargs

    def send(self, message) -> LLMResponse:
        """Send a user message (str) or tool results (list of ToolResultBlock).

        For tool results, ``message`` is a list of ToolResultBlock (canonical).
        The message is committed to the interface before the API call so that
        add_user_message can strip unanswered tool_use entries.  On API error
        the last user entry is reverted via drop_trailing.
        """
        # Commit to interface first (add_user_message strips unanswered tool_use),
        # then build API messages from the clean interface.
        if isinstance(message, str):
            self._interface.add_user_message(message)
        elif isinstance(message, list):
            self._interface.add_tool_results(message)
        else:
            raise TypeError(f"Unsupported message type: {type(message)}")

        self._interface.enforce_tool_pairing()
        candidate_msgs = to_anthropic(self._interface)
        clean_messages = _ensure_alternation(candidate_msgs)
        kwargs = self._build_request_kwargs(clean_messages)

        try:
            raw = self._client.messages.create(**kwargs)
        except Exception as api_err:
            # Revert the interface on error - drop the last user entry
            self._interface.drop_trailing(lambda e: e.role == "user")
            raise

        # Parse response and add to interface
        response = _parse_response(raw)
        # Record assistant response from raw API object (preserves thinking signatures)
        from lingtai_kernel.llm.interface import TextBlock, ThinkingBlock, ToolCallBlock
        assistant_blocks = []
        for block in raw.content:
            if block.type == "thinking":
                sig = getattr(block, "signature", None)
                pd = {"anthropic": {"signature": sig}} if sig else {}
                assistant_blocks.append(ThinkingBlock(
                    text=getattr(block, "thinking", ""),
                    provider_data=pd,
                ))
            elif block.type == "text":
                assistant_blocks.append(TextBlock(text=block.text))
            elif block.type == "tool_use":
                assistant_blocks.append(ToolCallBlock(
                    id=block.id,
                    name=block.name,
                    args=block.input if isinstance(block.input, dict) else {},
                ))
        if assistant_blocks:
            self._interface.add_assistant_message(
                assistant_blocks,
                model=self._model,
                provider="anthropic",
                usage={
                    "input_tokens": response.usage.input_tokens,
                    "output_tokens": response.usage.output_tokens,
                    "thinking_tokens": response.usage.thinking_tokens,
                },
            )

        return response

    def send_stream(
        self,
        message,
        on_chunk: Callable[[str], None] | None = None,
    ) -> LLMResponse:
        """Streaming send. User message committed to history only after success."""
        from lingtai_kernel.llm.interface import TextBlock, ThinkingBlock, ToolCallBlock

        # Record user input into interface first
        if isinstance(message, str):
            self._interface.add_user_message(message)
        elif isinstance(message, list):
            # list of ToolResultBlock
            self._interface.add_tool_results(message)
        else:
            raise TypeError(f"Unsupported message type: {type(message)}")

        # Build ephemeral Anthropic messages from interface
        self._interface.enforce_tool_pairing()
        candidate_msgs = to_anthropic(self._interface)
        clean_messages = _ensure_alternation(candidate_msgs)
        kwargs = self._build_request_kwargs(clean_messages)

        acc = StreamingAccumulator()

        try:
            with self._client.messages.stream(**kwargs) as stream:
                for event in stream:
                    etype = getattr(event, "type", None)
                    if etype == "content_block_start":
                        block = getattr(event, "content_block", None)
                        if block and getattr(block, "type", None) == "tool_use":
                            acc.start_tool(id=block.id, name=block.name)
                    elif etype == "content_block_delta":
                        delta = getattr(event, "delta", None)
                        if delta is None:
                            continue
                        dtype = getattr(delta, "type", None)
                        if dtype == "text_delta":
                            t = getattr(delta, "text", "")
                            if t:
                                acc.add_text(t)
                                if on_chunk:
                                    on_chunk(t)
                        elif dtype == "thinking_delta":
                            t = getattr(delta, "thinking", "")
                            if t:
                                acc.add_thought(t)
                        elif dtype == "input_json_delta":
                            partial = getattr(delta, "partial_json", "")
                            if partial:
                                acc.add_tool_args(partial)
                    elif etype == "content_block_stop":
                        acc.finish_thought()
                        acc.finish_tool()

                final_message = stream.get_final_message()
        except Exception:
            # Revert the interface on error — drop the last user entry
            self._interface.drop_trailing(lambda e: e.role == "user")
            raise

        # Extract usage from final message (includes cache metrics)
        # Same normalisation as _parse_response — see comment there.
        usage = UsageMetadata()
        if final_message and final_message.usage:
            u = final_message.usage
            cache_read = getattr(u, "cache_read_input_tokens", 0) or 0
            cache_write = getattr(u, "cache_creation_input_tokens", 0) or 0
            raw_input = getattr(u, "input_tokens", 0) or 0
            usage = UsageMetadata(
                input_tokens=raw_input + cache_read + cache_write,
                output_tokens=getattr(u, "output_tokens", 0) or 0,
                thinking_tokens=getattr(u, "thinking_tokens", 0) or 0,
                cached_tokens=cache_read,
            )
            if cache_read or cache_write:
                logger.debug(
                    "Anthropic cache (stream): read=%d write=%d uncached=%d total_input=%d",
                    cache_read,
                    cache_write,
                    raw_input,
                    usage.input_tokens,
                )

        # Record assistant response into interface
        if final_message:
            assistant_blocks: list = []
            for block in final_message.content:
                if block.type == "thinking":
                    thinking_text = getattr(block, "thinking", "")
                    sig = getattr(block, "signature", None)
                    provider_data = {"anthropic": {"signature": sig}} if sig else {}
                    assistant_blocks.append(ThinkingBlock(text=thinking_text, provider_data=provider_data))
                elif block.type == "text":
                    assistant_blocks.append(TextBlock(text=block.text))
                elif block.type == "tool_use":
                    assistant_blocks.append(ToolCallBlock(
                        id=block.id,
                        name=block.name,
                        args=block.input if isinstance(block.input, dict) else {},
                    ))
            if assistant_blocks:
                self._interface.add_assistant_message(
                    assistant_blocks,
                    model=self._model,
                    provider="anthropic",
                    usage={
                        "input_tokens": usage.input_tokens,
                        "output_tokens": usage.output_tokens,
                        "thinking_tokens": usage.thinking_tokens,
                    },
                )

        return acc.finalize(usage=usage, raw=final_message)

    def commit_tool_results(self, tool_results: list) -> None:
        """Append tool results to interface without an API call."""
        if tool_results:
            # tool_results is a list of ToolResultBlock
            self._interface.add_tool_results(tool_results)

    def update_tools(self, tools: list[FunctionSchema] | None) -> None:
        """Replace the tool schemas for subsequent calls in this session."""
        self._tools = _build_tools(tools, cache_tools=True) if tools else None
        tool_dicts = FunctionSchema.list_to_dicts(tools)
        self._interface.add_system(
            self._interface.current_system_prompt or "", tools=tool_dicts,
        )

    def update_system_prompt(self, system_prompt: str) -> None:
        """Replace the system prompt for subsequent calls in this session."""
        self._system = _build_system_with_cache(system_prompt)
        self._interface.add_system(system_prompt, tools=self._interface.current_tools)

    def reset(self) -> None:
        """Create a truly fresh session instance while preserving state.

        Reconstructs a new AnthropicChatSession with a fresh HTTP client
        and copies all attributes onto self, giving a clean connection and
        fresh internal state.
        """
        if self._client_kwargs:
            new_client = anthropic.Anthropic(**self._client_kwargs)
            new_session = AnthropicChatSession(
                client=new_client,
                model=self._model,
                system_prompt=self._system,
                interface=self._interface,
                tools=self._tools,
                tool_choice=self._tool_choice,
                extra_kwargs=self._extra_kwargs,
                client_kwargs=self._client_kwargs,
            )
            self.__dict__.update(new_session.__dict__)

    # -- Context window -------------------------------------------------------

    def context_window(self) -> int:
        return self._context_window


# ---------------------------------------------------------------------------
# AnthropicAdapter
# ---------------------------------------------------------------------------


class AnthropicAdapter(LLMAdapter):
    """Adapter that wraps the ``anthropic`` SDK for Claude models."""


    def __init__(
        self,
        api_key: str,
        *,
        base_url: str | None = None,
        timeout_ms: int = 300_000,
        max_rpm: int = 0,
    ):
        self._base_url = base_url
        kwargs: dict[str, Any] = {
            "api_key": api_key,
            "timeout": timeout_ms / 1000.0,
        }
        if base_url:
            kwargs["base_url"] = base_url
        self._client_kwargs = dict(kwargs)  # store for session reset
        self._client = anthropic.Anthropic(**kwargs)
        self._setup_gate(max_rpm)

    @staticmethod
    def _resolve_thinking_budget(thinking: str) -> int | None:
        """Resolve thinking tier to budget tokens using config."""
        if thinking == "default" or thinking is None:
            return None

        if thinking == "high":
            tier = "high"
        elif thinking == "low":
            tier = "low"
        else:
            return None

        if tier == "high":
            return 16384
        elif tier in ("low", "medium"):
            return 2048
        return None

    # -- LLMAdapter interface --------------------------------------------------

    def create_chat(
        self,
        model: str,
        system_prompt: str,
        tools: list[FunctionSchema] | None = None,
        *,
        json_schema: dict | None = None,
        force_tool_call: bool = False,
        interface: ChatInterface | None = None,
        thinking: str = "default",
        interaction_id: str | None = None,  # ignored — Gemini-specific
        context_window: int = 0,
    ) -> AnthropicChatSession:
        # Create interface from scratch or from history
        if interface is not None:
            iface = interface
        else:
            iface = ChatInterface()
            tool_dicts = (
                [{"name": t.name, "description": t.description, "parameters": t.parameters} for t in tools]
                if tools else None
            )
            iface.add_system(system_prompt, tools=tool_dicts)

        anthropic_tools = _build_tools(tools, cache_tools=True)
        tool_choice: dict | None = None
        if force_tool_call and anthropic_tools:
            tool_choice = {"type": "any"}

        # JSON schema enforcement via tool-based structured output
        if json_schema is not None and anthropic_tools is None:
            anthropic_tools = []
        if json_schema is not None:
            schema_tool_name = json_schema.get("title", "structured_output")
            anthropic_tools.append(
                {
                    "name": schema_tool_name,
                    "description": "Return the structured response matching the required schema.",
                    "input_schema": json_schema,
                }
            )
            tool_choice = {"type": "tool", "name": schema_tool_name}

        extra_kwargs: dict[str, Any] = {}

        # Thinking/extended thinking
        thinking_budget = self._resolve_thinking_budget(thinking)
        if thinking_budget is not None:
            extra_kwargs["thinking"] = {
                "type": "enabled",
                "budget_tokens": thinking_budget,
            }
            extra_kwargs["max_tokens"] = max(
                thinking_budget * 2, thinking_budget + 8192
            )

        return AnthropicChatSession(
            client=self._client,
            model=model,
            system_prompt=_build_system_with_cache(system_prompt),
            interface=iface,
            tools=anthropic_tools,
            tool_choice=tool_choice,
            extra_kwargs=extra_kwargs,
            client_kwargs=self._client_kwargs,
            context_window=context_window,
        )

    def generate(
        self,
        model: str,
        contents: str | list,
        *,
        system_prompt: str | None = None,
        temperature: float | None = None,
        json_schema: dict | None = None,
        max_output_tokens: int | None = None,
    ) -> LLMResponse:
        messages: list[dict] = []
        if isinstance(contents, str):
            messages.append({"role": "user", "content": contents})
        elif isinstance(contents, list):
            messages.append({"role": "user", "content": contents})
        else:
            messages.append({"role": "user", "content": str(contents)})

        kwargs: dict[str, Any] = {
            "model": model,
            "messages": messages,
            "max_tokens": max_output_tokens or 8192,
        }
        if system_prompt:
            kwargs["system"] = system_prompt
        if temperature is not None:
            kwargs["temperature"] = temperature

        if json_schema is not None:
            tools = [
                {
                    "name": json_schema.get("title", "structured_output"),
                    "description": "Return the structured response.",
                    "input_schema": json_schema,
                }
            ]
            kwargs["tools"] = tools
            kwargs["tool_choice"] = {
                "type": "tool",
                "name": json_schema.get("title", "structured_output"),
            }

        raw = self._client.messages.create(**kwargs)
        return _parse_response(raw)

    def make_tool_result_message(
        self, tool_name: str, result: dict, *, tool_call_id: str | None = None
    ) -> ToolResultBlock:
        """Build a canonical ToolResultBlock."""
        return ToolResultBlock(
            id=tool_call_id or f"toolu_{uuid.uuid4().hex[:24]}",
            name=tool_name,
            content=result,
        )

    def is_quota_error(self, exc: Exception) -> bool:
        """Check if the exception is an Anthropic rate-limit error."""
        return isinstance(exc, anthropic.RateLimitError)

    # -- Convenience properties ------------------------------------------------

    @property
    def client(self):
        """Escape hatch — the underlying ``anthropic.Anthropic`` client."""
        return self._client
