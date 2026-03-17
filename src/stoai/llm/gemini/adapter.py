"""Gemini adapter — wraps all google-genai SDK calls.

This is the **only** module in the project that imports ``google.genai``.
All other agent code talks to Gemini through the :class:`GeminiAdapter` and
:class:`GeminiChatSession` interfaces defined here.
"""

from __future__ import annotations

import json
from typing import Any

from google import genai
from google.genai import errors as genai_errors, types

from ...logging import get_logger

from ..base import (
    ChatSession,
    FunctionSchema,
    LLMAdapter,
    LLMResponse,
    ToolCall,
    ToolResultBlock,
    UsageMetadata,
)
from ..interface import ChatInterface
from ..streaming import StreamingAccumulator

logger = get_logger()

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _build_function_declarations(
    tools: list[FunctionSchema] | None,
) -> list[types.FunctionDeclaration] | None:
    """Convert our FunctionSchema list to Gemini FunctionDeclaration list."""
    if not tools:
        return None
    return [
        types.FunctionDeclaration(
            name=t.name,
            description=t.description,
            parameters=t.parameters,
        )
        for t in tools
    ]


def _parse_response(raw) -> LLMResponse:
    """Parse a raw Gemini response into a provider-agnostic LLMResponse."""
    text_parts: list[str] = []
    tool_calls: list[ToolCall] = []
    thoughts: list[str] = []

    candidates = getattr(raw, "candidates", None) or []
    if candidates:
        content = candidates[0].content
        if content and content.parts:
            for part in content.parts:
                if (
                    getattr(part, "thought", False)
                    and hasattr(part, "text")
                    and part.text
                ):
                    thoughts.append(part.text)
                elif (
                    hasattr(part, "function_call")
                    and part.function_call
                    and part.function_call.name
                ):
                    tool_calls.append(
                        ToolCall(
                            name=part.function_call.name.removeprefix("default_api:"),
                            args=dict(part.function_call.args)
                            if part.function_call.args
                            else {},
                        )
                    )
                elif hasattr(part, "text") and part.text:
                    text_parts.append(part.text)

    # Token usage
    meta = getattr(raw, "usage_metadata", None)
    usage = (
        UsageMetadata(
            input_tokens=getattr(meta, "prompt_token_count", 0) or 0,
            output_tokens=getattr(meta, "candidates_token_count", 0) or 0,
            thinking_tokens=getattr(meta, "thoughts_token_count", 0) or 0,
            cached_tokens=getattr(meta, "cached_content_token_count", 0) or 0,
        )
        if meta
        else UsageMetadata()
    )

    return LLMResponse(
        text="\n".join(text_parts) if text_parts else "",
        tool_calls=tool_calls,
        usage=usage,
        thoughts=thoughts,
        raw=raw,
    )


def _supports_thinking(model: str) -> bool:
    """Return True if the model supports thinking config (Gemini 3+)."""
    # Match model names like "gemini-3-flash-preview", "gemini-3-pro", etc.
    # Gemini 2.x (including 2.5-flash-preview) does NOT support thinking.
    parts = model.lower().replace("models/", "").split("-")
    if len(parts) >= 2 and parts[0] == "gemini":
        try:
            major = int(parts[1].split(".")[0])
            return major >= 3
        except (ValueError, IndexError):
            pass
    return False


def _thinking_config(level: str) -> types.ThinkingConfig | None:
    """Build a Gemini ThinkingConfig from a normalized level string.

    Returns None if thinking is disabled ("off").
    """
    if level == "off":
        return None
    level_upper = level.upper() if level != "default" else "LOW"
    return types.ThinkingConfig(include_thoughts=True, thinking_level=level_upper)


# ---------------------------------------------------------------------------
# GeminiChatSession
# ---------------------------------------------------------------------------


class GeminiChatSession(ChatSession):
    """Wraps a ``genai`` chat session."""

    def __init__(self, chat, context_window: int = 0, interface: ChatInterface | None = None):
        self._chat = chat
        self._context_window_size = context_window
        self._interface = interface or ChatInterface()

    @property
    def interface(self) -> ChatInterface:
        """The canonical ChatInterface for this session."""
        return self._interface

    def send(self, message) -> LLMResponse:
        """Send a message (text or list of tool-result Parts) and parse the response."""
        raw = self._chat.send_message(message)
        return _parse_response(raw)

    def get_history(self) -> list[dict]:
        """Return serializable history dicts (for session persistence)."""
        return [
            content.model_dump(exclude_none=True)
            for content in self._chat.get_history()
        ]

    def context_window(self) -> int:
        return self._context_window_size

    @property
    def raw_chat(self):
        """Escape hatch — the underlying ``genai`` chat object."""
        return self._chat


# ---------------------------------------------------------------------------
# InteractionsChatSession
# ---------------------------------------------------------------------------


def _sanitize_parameters_for_interactions(params: dict) -> dict:
    """Clean a JSON Schema parameters dict for the Interactions API.

    The Interactions API rejects ``"required": []`` (empty array) in tool
    parameter schemas — unlike the Chat API which tolerates it.  Strip the
    key when empty to avoid a 400 error.
    """
    if not params:
        return params
    cleaned = dict(params)
    if "required" in cleaned and not cleaned["required"]:
        del cleaned["required"]
    return cleaned


def _build_interactions_tools(
    tools: list[FunctionSchema] | None,
) -> list[dict] | None:
    """Convert FunctionSchema list to Interactions API tool dicts."""
    if not tools:
        return None
    return [
        {
            "type": "function",
            "name": t.name,
            "description": t.description,
            "parameters": _sanitize_parameters_for_interactions(t.parameters),
        }
        for t in tools
    ]


def _parse_interaction_response(interaction) -> LLMResponse:
    """Parse an Interactions API response into a provider-agnostic LLMResponse."""
    text_parts: list[str] = []
    tool_calls: list[ToolCall] = []
    thoughts: list[str] = []

    for output in interaction.outputs or []:
        otype = getattr(output, "type", None)
        if otype == "text":
            t = getattr(output, "text", None)
            if t:
                text_parts.append(t)
        elif otype == "function_call":
            tool_calls.append(
                ToolCall(
                    name=output.name.removeprefix("default_api:"),
                    args=dict(output.arguments) if output.arguments else {},
                    id=output.id,
                )
            )
        elif otype == "thought":
            # ThoughtContent.summary is a list of TextContent/ImageContent
            for summary_item in getattr(output, "summary", None) or []:
                if getattr(summary_item, "type", None) == "text":
                    t = getattr(summary_item, "text", None)
                    if t:
                        thoughts.append(t)

    # Token usage
    usage_obj = interaction.usage
    usage = (
        UsageMetadata(
            input_tokens=getattr(usage_obj, "total_input_tokens", 0) or 0,
            output_tokens=getattr(usage_obj, "total_output_tokens", 0) or 0,
            thinking_tokens=getattr(usage_obj, "total_thought_tokens", 0) or 0,
            cached_tokens=getattr(usage_obj, "total_cached_tokens", 0) or 0,
        )
        if usage_obj
        else UsageMetadata()
    )

    return LLMResponse(
        text="\n".join(text_parts) if text_parts else "",
        tool_calls=tool_calls,
        usage=usage,
        thoughts=thoughts,
        raw=interaction,
    )


def _convert_history_to_turns(history: list[dict]) -> list[dict]:
    """Convert Chat API history dicts to Interactions API TurnParam format.

    Chat API history is a list of dicts with "role" and "parts" keys.
    Interactions API wants TurnParam dicts with "role" and "content" keys,
    where content is a list of ContentParam dicts.

    Also handles client_history format (already in TurnParam format with
    "role" and "content" keys) — passes through directly.
    """
    turns: list[dict] = []
    for entry in history:
        # Check if already in TurnParam format (client_history from get_client_history)
        if "content" in entry:
            # Already in TurnParam format - pass through directly
            turns.append(
                {"role": entry.get("role", "user"), "content": entry["content"]}
            )
            continue

        role = entry.get("role", "user")
        parts = entry.get("parts", [])
        content_blocks: list[dict] = []

        for part in parts:
            if isinstance(part, str):
                content_blocks.append({"type": "text", "text": part})
            elif isinstance(part, dict):
                if "text" in part and not part.get("thought"):
                    content_blocks.append({"type": "text", "text": part["text"]})
                elif part.get("thought") and "text" in part:
                    # Thought blocks — include signature for proper chaining
                    thought_block: dict[str, Any] = {"type": "thought"}
                    if part.get("text"):
                        thought_block["summary"] = [
                            {"type": "text", "text": part["text"]}
                        ]
                    content_blocks.append(thought_block)
                elif "function_call" in part:
                    fc = part["function_call"]
                    content_blocks.append(
                        {
                            "type": "function_call",
                            "id": fc.get("id", fc.get("name", "")),
                            "name": fc["name"],
                            "arguments": fc.get("args", {}),
                        }
                    )
                elif "function_response" in part:
                    fr = part["function_response"]
                    resp = fr.get("response", {})
                    content_blocks.append(
                        {
                            "type": "function_result",
                            "call_id": fr.get("id", fr.get("name", "")),
                            "result": json.dumps(resp)
                            if not isinstance(resp, str)
                            else resp,
                            "name": fr.get("name", ""),
                        }
                    )

        if content_blocks:
            turns.append({"role": role, "content": content_blocks})

    return turns


class InteractionsChatSession(ChatSession):
    """Chat session backed by the Gemini Interactions API.

    Instead of accumulating conversation history client-side (quadratic
    cost), each call passes ``previous_interaction_id`` so the server
    retrieves history automatically.  Only the new input is sent per call.
    """

    def __init__(
        self,
        client: genai.Client,
        model: str,
        config_kwargs: dict[str, Any],
        prev_interaction_id: str | None = None,
        context_window: int = 0,
        interface: ChatInterface | None = None,
    ):
        self._client = client
        self._model = model
        self._config_kwargs = (
            config_kwargs  # system_instruction, tools, generation_config, etc.
        )
        self._interaction_id: str | None = prev_interaction_id
        self._context_window_size = context_window
        self._interface = interface or ChatInterface()
        # Pending seed turns from a session resume with full history.
        # If set, prepended to the first send() call as Iterable[TurnParam].
        self._pending_seed_turns: list[dict] | None = None
        # Client-side mirror of conversation history for session fork support
        self._client_history: list[dict] = []

    @property
    def interface(self) -> ChatInterface:
        """The canonical ChatInterface for this session."""
        return self._interface

    def send(self, message) -> LLMResponse:
        """Send a message and return the parsed response.

        ``message`` can be:
        - A string (user text message)
        - A list of ToolResultBlock (tool results from make_tool_result)
        - A list of ``FunctionResultContentParam`` dicts (tool results)
        """
        # Record tool results in canonical interface (matches Anthropic/OpenAI)
        if isinstance(message, list) and message and isinstance(message[0], ToolResultBlock):
            self._interface.add_tool_results(message)

        converted_input = self._convert_input(message)

        # If we have pending seed turns (session resume with history but no
        # interaction_id), prepend them to the first call as TurnParam list.
        if self._pending_seed_turns is not None:
            seed = self._pending_seed_turns
            self._pending_seed_turns = None
            # Record seed turns into client history
            self._client_history.extend(seed)
            # Merge: seed turns + new user message as a final user turn
            seed.append({"role": "user", "content": converted_input})
            converted_input = seed
        else:
            # Record user turn
            self._client_history.append({"role": "user", "content": converted_input})

        kwargs: dict[str, Any] = {
            "model": self._model,
            "input": converted_input,
            **self._config_kwargs,
        }
        if self._interaction_id:
            kwargs["previous_interaction_id"] = self._interaction_id

        interaction = self._client.interactions.create(**kwargs)
        self._interaction_id = interaction.id
        response = _parse_interaction_response(interaction)

        # Record model turn from response outputs
        self._record_model_turn(interaction)

        return response

    def send_stream(self, message, on_chunk=None) -> LLMResponse:
        """Send with streaming — calls on_chunk(text_delta) as text arrives.

        Function call deltas arrive atomically (full args in one event),
        so no incremental merging is needed.
        """
        # Record tool results in canonical interface (matches Anthropic/OpenAI)
        if isinstance(message, list) and message and isinstance(message[0], ToolResultBlock):
            self._interface.add_tool_results(message)

        converted_input = self._convert_input(message)

        if self._pending_seed_turns is not None:
            seed = self._pending_seed_turns
            self._pending_seed_turns = None
            self._client_history.extend(seed)
            seed.append({"role": "user", "content": converted_input})
            converted_input = seed
        else:
            self._client_history.append({"role": "user", "content": converted_input})

        kwargs: dict[str, Any] = {
            "model": self._model,
            "input": converted_input,
            "stream": True,
            **self._config_kwargs,
        }
        if self._interaction_id:
            kwargs["previous_interaction_id"] = self._interaction_id

        acc = StreamingAccumulator()
        usage = UsageMetadata()
        interaction_id: str | None = None

        for event in self._client.interactions.create(**kwargs):
            etype = getattr(event, "event_type", None)

            if etype == "interaction.start":
                interaction_id = getattr(
                    getattr(event, "interaction", event), "id", None
                )

            elif etype == "content.delta":
                delta = getattr(event, "delta", None)
                if delta is None:
                    continue
                dtype = getattr(delta, "type", None)
                if dtype == "text":
                    t = getattr(delta, "text", None)
                    if t:
                        acc.add_text(t)
                        if on_chunk:
                            on_chunk(t)
                elif dtype == "function_call":
                    acc.add_tool(ToolCall(
                        name=delta.name.removeprefix("default_api:"),
                        args=dict(delta.arguments) if delta.arguments else {},
                        id=getattr(delta, "id", None),
                    ))
                elif dtype == "thought":
                    t = getattr(delta, "thought", None)
                    if t:
                        acc.add_thought(t)

            elif etype == "interaction.complete":
                interaction_obj = getattr(event, "interaction", event)
                interaction_id = getattr(interaction_obj, "id", interaction_id)
                usage_obj = getattr(interaction_obj, "usage", None)
                if usage_obj:
                    usage = UsageMetadata(
                        input_tokens=getattr(usage_obj, "total_input_tokens", 0) or 0,
                        output_tokens=getattr(usage_obj, "total_output_tokens", 0) or 0,
                        thinking_tokens=getattr(usage_obj, "total_thought_tokens", 0)
                        or 0,
                        cached_tokens=getattr(usage_obj, "total_cached_tokens", 0) or 0,
                    )

        if interaction_id:
            self._interaction_id = interaction_id

        result = acc.finalize(usage=usage)

        # Record model turn from accumulated outputs
        model_content = []
        if result.thoughts:
            model_content.append(
                {
                    "type": "thought",
                    "summary": [{"type": "text", "text": result.thoughts[0]}],
                }
            )
        if result.text:
            model_content.append({"type": "text", "text": result.text})
        for tc in result.tool_calls:
            model_content.append(
                {
                    "type": "function_call",
                    "id": tc.id,
                    "name": tc.name,
                    "arguments": tc.args,
                }
            )
        if model_content:
            self._client_history.append({"role": "model", "content": model_content})

        return result

    def commit_tool_results(self, tool_results: list) -> None:
        """Append tool results to client history without an API call."""
        if tool_results:
            self._client_history.append({"role": "user", "content": tool_results})

    def update_tools(self, tools: list[FunctionSchema] | None) -> None:
        """Replace tool schemas for subsequent Interactions API calls."""
        if tools:
            self._config_kwargs["tools"] = _build_interactions_tools(tools)
        else:
            self._config_kwargs.pop("tools", None)
        tool_dicts = FunctionSchema.list_to_dicts(tools)
        self._interface.add_system(
            self._interface.current_system_prompt or "", tools=tool_dicts,
        )

    def update_system_prompt(self, system_prompt: str) -> None:
        """Replace the system prompt for subsequent Interactions API calls."""
        self._config_kwargs["system_instruction"] = system_prompt
        self._interface.add_system(system_prompt, tools=self._interface.current_tools)

    def context_window(self) -> int:
        return self._context_window_size

    def get_history(self) -> list[dict]:
        """Return serializable state including both interaction_id and client history."""
        return [
            {"_interaction_id": self._interaction_id},
            {"_client_history": self._client_history},
        ]

    def get_client_history(self) -> list[dict]:
        """Return the client-side conversation history (TurnParam format)."""
        return list(self._client_history)

    @property
    def interaction_id(self) -> str | None:
        """Current interaction ID for session chaining."""
        return self._interaction_id

    def _record_model_turn(self, interaction) -> None:
        """Record the model's response outputs as a client-side history turn."""
        content_blocks = []
        for output in interaction.outputs or []:
            otype = getattr(output, "type", None)
            if otype == "text":
                t = getattr(output, "text", None)
                if t:
                    content_blocks.append({"type": "text", "text": t})
            elif otype == "function_call":
                content_blocks.append(
                    {
                        "type": "function_call",
                        "id": output.id,
                        "name": output.name.removeprefix("default_api:"),
                        "arguments": dict(output.arguments) if output.arguments else {},
                    }
                )
            elif otype == "thought":
                for summary_item in getattr(output, "summary", None) or []:
                    if getattr(summary_item, "type", None) == "text":
                        t = getattr(summary_item, "text", None)
                        if t:
                            content_blocks.append(
                                {
                                    "type": "thought",
                                    "summary": [{"type": "text", "text": t}],
                                }
                            )
        if content_blocks:
            self._client_history.append({"role": "model", "content": content_blocks})

    # -- Internal helpers ------------------------------------------------------

    @staticmethod
    def _convert_input(message) -> Any:
        """Convert from our internal message format to Interactions API input.

        The Interactions API requires ``input`` to be a list of ContentParam
        dicts — bare strings are rejected with ``'value at top-level must be
        a list'``.

        Handles:
        - str → wrapped as ``[{"type": "text", "text": ...}]``
        - list of ToolResultBlock → converted to FunctionResultContentParam dicts
        - list of FunctionResultContentParam dicts → passed as-is
        - list of Part objects (legacy Chat API format) → converted to
          FunctionResultContentParam dicts
        """
        if isinstance(message, str):
            return [{"type": "text", "text": message}]

        if isinstance(message, list):
            # Check if these are already Interactions API dicts
            if message and isinstance(message[0], dict) and "type" in message[0]:
                return message

            converted = []
            for item in message:
                if isinstance(item, dict) and "type" in item:
                    converted.append(item)
                elif isinstance(item, ToolResultBlock):
                    # Canonical ToolResultBlock from make_tool_result()
                    content = item.content
                    converted.append(
                        {
                            "type": "function_result",
                            "call_id": item.id,
                            "result": json.dumps(content, default=str)
                            if not isinstance(content, str)
                            else content,
                            "name": item.name,
                        }
                    )
                elif hasattr(item, "function_response") and item.function_response:
                    # Legacy Chat API Part objects
                    fr = item.function_response
                    converted.append(
                        {
                            "type": "function_result",
                            "call_id": getattr(fr, "id", "") or fr.name,
                            "result": json.dumps(fr.response)
                            if not isinstance(fr.response, str)
                            else fr.response,
                            "name": fr.name,
                        }
                    )
                elif hasattr(item, "inline_data") and item.inline_data:
                    # Legacy Chat API Part with image data
                    import base64

                    converted.append(
                        {
                            "type": "inline_data",
                            "data": base64.b64encode(item.inline_data.data).decode(
                                "ascii"
                            ),
                            "mime_type": item.inline_data.mime_type,
                        }
                    )
                elif hasattr(item, "text") and item.text is not None:
                    # Legacy Chat API Part with text
                    converted.append({"type": "text", "text": item.text})
                else:
                    # Unknown item type — pass through and let the API handle it
                    converted.append(item)
            return converted

        # Single non-string item — wrap in list
        return [message]


# ---------------------------------------------------------------------------
# GeminiAdapter
# ---------------------------------------------------------------------------


class GeminiAdapter(LLMAdapter):
    """Adapter that wraps all ``google-genai`` SDK calls."""

    supports_web_search = True
    supports_vision = True

    def __init__(self, api_key: str, timeout_ms: int = 300_000, max_rpm: int = 0):
        self._client = genai.Client(
            api_key=api_key,
            http_options=types.HttpOptions(
                timeout=timeout_ms,
                retry_options=types.HttpRetryOptions(),
            ),
        )
        # When True, make_tool_result_message() produces Interactions API dicts
        # instead of Chat API Part objects.
        self._use_interactions: bool = False
        self._setup_gate(max_rpm)

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
        interaction_id: str | None = None,
        context_window: int = 0,
    ) -> ChatSession:
        # Check if Interactions API is enabled
        use_interactions = True

        # Convert interface to seed turns for history seeding
        seed_turns: list[dict] | None = None
        if interface and interface.conversation_entries():
            from ..interface_converters import to_gemini
            seed_turns = to_gemini(interface)

        if use_interactions and not json_schema:
            # Interactions API path — server-side conversation state
            return self._create_interactions_session(
                model,
                system_prompt,
                tools,
                seed_turns=seed_turns,
                interface=interface,
                thinking=thinking,
                force_tool_call=force_tool_call,
                interaction_id=interaction_id,
                context_window=context_window,
            )

        # --- Chat API path (used for json_schema mode) ---
        # Build GenerateContentConfig
        config_kwargs: dict[str, Any] = {}
        config_kwargs["system_instruction"] = system_prompt

        # Only send thinking_config for Gemini 3+ models.
        # Per-call `thinking` param indicates the tier: "high" = smart model
        # (orchestrator), "low" = sub-agent. Config overrides per tier.
        if _supports_thinking(model):
            GEMINI_THINKING_MODEL = "high"
            GEMINI_THINKING_SUB_AGENT = "high"

            if thinking in ("high", "default"):
                effective = GEMINI_THINKING_MODEL
            else:
                effective = GEMINI_THINKING_SUB_AGENT
            tc = _thinking_config(effective)
            if tc is not None:
                config_kwargs["thinking_config"] = tc

        fds = _build_function_declarations(tools)
        if fds:
            config_kwargs["tools"] = [types.Tool(function_declarations=fds)]
        if force_tool_call and tools:
            config_kwargs["tool_config"] = types.ToolConfig(
                function_calling_config=types.FunctionCallingConfig(mode="ANY")
            )

        # JSON schema enforcement
        if json_schema is not None:
            config_kwargs["response_mime_type"] = "application/json"
            config_kwargs["response_schema"] = json_schema

        config = types.GenerateContentConfig(**config_kwargs)

        # Create the chat
        create_kwargs: dict[str, Any] = {"model": model, "config": config}
        # Chat API path is only used for json_schema mode — history seeding
        # is not meaningful here, so we skip seed_turns.

        self._use_interactions = False
        chat = self._client.chats.create(**create_kwargs)

        return GeminiChatSession(chat, context_window=context_window, interface=interface)

    def _create_interactions_session(
        self,
        model: str,
        system_prompt: str,
        tools: list[FunctionSchema] | None = None,
        *,
        seed_turns: list[dict] | None = None,
        interface: ChatInterface | None = None,
        thinking: str = "default",
        force_tool_call: bool = False,
        interaction_id: str | None = None,
        context_window: int = 0,
    ) -> InteractionsChatSession:
        """Create an InteractionsChatSession with server-side state.

        If ``interaction_id`` is provided, the session resumes from that
        interaction (server retrieves the history).  If ``seed_turns`` is
        provided without an ``interaction_id``, the first call seeds the
        conversation via ``Iterable[TurnParam]``.
        """
        config_kwargs: dict[str, Any] = {
            "system_instruction": system_prompt,
        }

        # Tools as Interactions API format
        interactions_tools = _build_interactions_tools(tools)
        if interactions_tools:
            config_kwargs["tools"] = interactions_tools

        # Generation config (thinking + tool_choice)
        gen_config: dict[str, Any] = {}
        if _supports_thinking(model):
            GEMINI_THINKING_MODEL = "high"
            GEMINI_THINKING_SUB_AGENT = "high"

            if thinking in ("high", "default"):
                effective = GEMINI_THINKING_MODEL
            else:
                effective = GEMINI_THINKING_SUB_AGENT
            if effective != "off":
                level_upper = effective.upper() if effective != "default" else "LOW"
                gen_config["thinking_level"] = level_upper.lower()

        if force_tool_call and tools:
            gen_config["tool_choice"] = "any"

        if gen_config:
            config_kwargs["generation_config"] = gen_config

        self._use_interactions = True

        # If resuming from a saved interaction_id, history is server-side
        if interaction_id:
            return InteractionsChatSession(
                self._client,
                model,
                config_kwargs,
                prev_interaction_id=interaction_id,
                context_window=context_window,
                interface=interface,
            )

        # If seeding with conversation history, use pre-converted TurnParam
        # turns for the first call. The InteractionsChatSession will send
        # this as its first input and then chain from the returned
        # interaction_id.
        session = InteractionsChatSession(
            self._client,
            model,
            config_kwargs,
            prev_interaction_id=None,
            context_window=context_window,
            interface=interface,
        )

        if seed_turns:
            session._pending_seed_turns = seed_turns

        return session

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
        config_kwargs: dict[str, Any] = {}
        if system_prompt is not None:
            config_kwargs["system_instruction"] = system_prompt
        if temperature is not None:
            config_kwargs["temperature"] = temperature
        if max_output_tokens is not None:
            config_kwargs["max_output_tokens"] = max_output_tokens
        if json_schema is not None:
            config_kwargs["response_mime_type"] = "application/json"
            config_kwargs["response_schema"] = json_schema

        config = types.GenerateContentConfig(**config_kwargs) if config_kwargs else None

        raw = self._client.models.generate_content(
            model=model,
            contents=contents,
            config=config,
        )
        return _parse_response(raw)

    def make_tool_result_message(
        self, tool_name: str, result: dict, *, tool_call_id: str | None = None
    ) -> ToolResultBlock:
        """Build a canonical ToolResultBlock."""
        return ToolResultBlock(
            id=tool_call_id or tool_name,
            name=tool_name,
            content=result,
        )

    def make_multimodal_message(
        self, text: str, image_bytes: bytes, mime_type: str = "image/png"
    ) -> list:
        """Build a Gemini multimodal message (image + text as Part list).

        Gemini's send_message() already accepts a list of Parts, so
        GeminiChatSession.send() needs no changes.
        """
        return [
            types.Part.from_bytes(data=image_bytes, mime_type=mime_type),
            types.Part.from_text(text=text),
        ]

    def is_quota_error(self, exc: Exception) -> bool:
        if isinstance(exc, genai_errors.ClientError):
            return getattr(exc, "code", None) == 429 or "RESOURCE_EXHAUSTED" in str(exc)
        return False

    # -- Gemini-specific methods (not in ABC) ----------------------------------

    def web_search(self, query: str, model: str) -> LLMResponse:
        """Execute a web search via Gemini's Google Search grounding."""
        gen_config = types.GenerateContentConfig(
            tools=[types.Tool(google_search=types.GoogleSearch())],
        )
        raw = self._client.models.generate_content(
            model=model,
            contents=query,
            config=gen_config,
        )
        return _parse_response(raw)

    def generate_multimodal(self, model: str, contents: list, **kwargs) -> LLMResponse:
        """Generate with multimodal content (e.g. image/PDF bytes + text).

        This is an escape hatch for document extraction which sends
        ``types.Part.from_bytes(...)`` directly.
        """
        config = types.GenerateContentConfig(**kwargs) if kwargs else None
        raw = self._client.models.generate_content(
            model=model,
            contents=contents,
            config=config,
        )
        return _parse_response(raw)

    def generate_vision(
        self, question: str, image_bytes: bytes, *, model: str = "",
        mime_type: str = "image/png",
    ) -> LLMResponse:
        """One-shot vision via Gemini's multimodal API."""
        contents = self.make_multimodal_message(question, image_bytes, mime_type)
        return self.generate_multimodal(model=model, contents=contents)

    @staticmethod
    def make_bytes_part(data: bytes, mime_type: str) -> Any:
        """Create a Gemini Part from raw bytes (for document/image input)."""
        return types.Part.from_bytes(data=data, mime_type=mime_type)

    @property
    def client(self):
        """Escape hatch — the underlying ``genai.Client``."""
        return self._client
