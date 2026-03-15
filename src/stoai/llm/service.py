"""LLMService — single entry point between backend and LLM providers.

See docs/plans/2026-03-06-llm-service-design.md for design rationale.

This version is decoupled from any app-specific config system:
- API key resolution via injected ``key_resolver`` callable (defaults to env vars)
- Provider defaults via injected ``provider_defaults`` dict
"""

from __future__ import annotations

import json
import os
import threading
import time
import uuid
from collections.abc import Callable
from pathlib import Path
from typing import Any

from .base import (
    ChatSession,
    FunctionSchema,
    LLMAdapter,
    LLMResponse,
)
from .interface import ChatInterface, ToolResultBlock

# ---------------------------------------------------------------------------
# Model context-window registry
# ---------------------------------------------------------------------------

# Default context window when model is unknown and litellm registry is unavailable
DEFAULT_CONTEXT_WINDOW = 256_000

LITELLM_REGISTRY_URL = (
    "https://raw.githubusercontent.com/BerriAI/litellm/main/"
    "model_prices_and_context_window.json"
)
_CACHE_MAX_AGE = 86400  # 24 hours

_litellm_cache: dict[str, int] | None = None
_litellm_lock = threading.Lock()


def _get_cache_path(data_dir: str | None = None) -> Path:
    if data_dir:
        return Path(data_dir) / "model_context_windows.json"
    return Path.home() / ".stoai" / "model_context_windows.json"


def _fetch_litellm_registry(data_dir: str | None = None) -> dict[str, int]:
    """Fetch max_input_tokens from litellm registry, cache locally.

    Returns a flat dict of {model_name: max_input_tokens}.
    Entries are stored in two forms:
    - Bare names (e.g., "gemini-3-flash-preview", "claude-sonnet-4-6")
    - Provider-stripped names from prefixed entries (e.g., "minimax/MiniMax-M2.5" -> "MiniMax-M2.5")
    """
    cache_path = _get_cache_path(data_dir)

    # Try reading from cache
    if cache_path.exists():
        try:
            age = time.time() - cache_path.stat().st_mtime
            if age < _CACHE_MAX_AGE:
                cached = json.loads(cache_path.read_text(encoding="utf-8"))
                if isinstance(cached, dict) and cached:
                    return cached
        except Exception:
            pass

    # Fetch from GitHub
    try:
        import urllib.request
        req = urllib.request.Request(LITELLM_REGISTRY_URL, headers={
            "User-Agent": "stoai/1.0",
        })
        with urllib.request.urlopen(req, timeout=10) as resp:
            raw = json.loads(resp.read().decode("utf-8"))
    except Exception:
        # Try stale cache
        if cache_path.exists():
            try:
                return json.loads(cache_path.read_text(encoding="utf-8"))
            except Exception:
                pass
        return {}

    # Extract max_input_tokens
    result: dict[str, int] = {}
    for model_key, info in raw.items():
        if not isinstance(info, dict):
            continue
        max_input = info.get("max_input_tokens")
        if not max_input or not isinstance(max_input, (int, float)):
            continue
        max_input = int(max_input)

        result[model_key] = max_input

        if "/" in model_key:
            bare = model_key.split("/", 1)[1]
            if bare not in result:
                result[bare] = max_input

    # Cache to disk
    try:
        cache_path.parent.mkdir(parents=True, exist_ok=True)
        cache_path.write_text(json.dumps(result), encoding="utf-8")
    except Exception:
        pass

    return result


def _get_litellm_registry() -> dict[str, int]:
    """Get litellm registry (lazy-loaded, thread-safe)."""
    global _litellm_cache
    if _litellm_cache is not None:
        return _litellm_cache
    with _litellm_lock:
        if _litellm_cache is not None:
            return _litellm_cache
        _litellm_cache = _fetch_litellm_registry()
        return _litellm_cache


def get_context_limit(model_name: str) -> int:
    """Return context window size for a model, or DEFAULT_CONTEXT_WINDOW if unknown.

    Resolution order:
    1. litellm community registry (cached, refreshed daily) — exact then prefix match
    2. DEFAULT_CONTEXT_WINDOW (256k)
    """
    if not model_name:
        return DEFAULT_CONTEXT_WINDOW

    # Try litellm registry — exact match first, then longest prefix
    registry = _get_litellm_registry()
    if registry:
        if model_name in registry:
            return registry[model_name]
        best, best_len = 0, 0
        for prefix, limit in registry.items():
            if model_name.startswith(prefix) and len(prefix) > best_len:
                best, best_len = limit, len(prefix)
        if best > 0:
            return best

    return DEFAULT_CONTEXT_WINDOW

COMPACTION_PROMPT = (
    "You are compacting conversation history for an AI agent so it can "
    "continue its session with less context. The most recent turns are "
    "preserved separately — do NOT repeat them.\n\n"
    "Preserve:\n"
    "- ALL errors, failures, and their details verbatim\n"
    "- Key decisions and user preferences\n"
    "- Data labels, dataset names, column names, and identifiers\n"
    "- Data that was fetched/computed and current state\n"
    "- Tool calls and their results\n\n"
    "Drop routine acknowledgments. "
    "Output ONLY the summary, no commentary.\n"
)


def _generate_session_id() -> str:
    """Generate a unique stoai session ID."""
    return f"st_{uuid.uuid4().hex[:12]}"


class LLMService:
    """Single entry point between backend and LLM providers.

    Responsibilities:
    - Adapter factory: constructs the right adapter from config
    - Session registry: assigns stoai session IDs, tracks active sessions
    - One-shot gateway: routes generate() through the same tracking path
    - Token accounting: centralizes per-session usage tracking via interface

    Does NOT:
    - Wrap ChatSession.send() — backend calls that directly
    - Handle fallback/retry — errors surface to the backend
    - Add business logic — pure delegation + bookkeeping

    Decoupling parameters:
    - ``key_resolver``: callable(provider) -> api_key | None.
      Defaults to reading ``{PROVIDER}_API_KEY`` from the environment.
    - ``provider_defaults``: dict mapping provider name to defaults dict
      (model, base_url, api_compat, etc.).  Defaults to empty dict.
    """

    def __init__(
        self,
        provider: str,
        model: str,
        api_key: str | None = None,
        base_url: str | None = None,
        provider_config: dict | None = None,
        key_resolver: Callable[[str], str | None] | None = None,
        provider_defaults: dict | None = None,
    ) -> None:
        self._provider = provider.lower()
        self._model = model
        self._base_url = base_url
        self._config = provider_config or {}
        self._key_resolver = key_resolver or (lambda p: os.environ.get(f"{p.upper()}_API_KEY"))
        self._provider_defaults = provider_defaults or {}
        self._adapters: dict[tuple[str, str | None], LLMAdapter] = {}
        self._adapter_lock = threading.Lock()
        self._adapters[(self._provider, base_url)] = self._create_adapter(self._provider, api_key, base_url)
        self._sessions: dict[str, ChatSession] = {}

    def _create_adapter(self, provider: str, api_key: str | None, base_url: str | None) -> LLMAdapter:
        # Build kwargs, omitting None values so adapters fall back to env vars
        key_kw: dict = {"api_key": api_key} if api_key is not None else {}
        url_kw: dict = {"base_url": base_url} if base_url is not None else {}
        defaults = self._get_provider_defaults(provider)
        max_rpm = defaults.get("max_rpm", 0) if defaults else 0
        rpm_kw: dict = {"max_rpm": max_rpm} if max_rpm > 0 else {}

        p = provider.lower()
        if p == "gemini":
            from .gemini.adapter import GeminiAdapter
            return GeminiAdapter(**key_kw, **rpm_kw)
        elif p == "anthropic":
            from .anthropic.adapter import AnthropicAdapter
            return AnthropicAdapter(**key_kw, **url_kw, **rpm_kw)
        elif p == "openai":
            from .openai.adapter import OpenAIAdapter
            return OpenAIAdapter(**key_kw, **url_kw, **rpm_kw)
        elif p == "minimax":
            from .minimax.adapter import MiniMaxAdapter
            return MiniMaxAdapter(**key_kw, **url_kw, **rpm_kw)
        elif p == "grok":
            from .grok.adapter import GrokAdapter
            return GrokAdapter(**key_kw, **rpm_kw)
        elif p == "deepseek":
            from .deepseek.adapter import DeepSeekAdapter
            return DeepSeekAdapter(**key_kw, **rpm_kw)
        elif p == "qwen":
            from .qwen.adapter import QwenAdapter
            return QwenAdapter(**key_kw, **rpm_kw)
        elif p == "kimi":
            from .kimi.adapter import create_kimi_adapter
            defaults = self._get_provider_defaults(p)
            compat = defaults.get("api_compat", "openai") if defaults else "openai"
            return create_kimi_adapter(**key_kw, api_compat=compat, **url_kw, **rpm_kw)
        elif p == "glm":
            from .glm.adapter import GLMAdapter
            return GLMAdapter(**key_kw, **rpm_kw)
        elif p == "custom":
            from .custom.adapter import create_custom_adapter
            defaults = self._get_provider_defaults(p)
            compat = defaults.get("api_compat", "openai") if defaults else "openai"
            ws = defaults.get("supports_web_search", False) if defaults else False
            vis = defaults.get("supports_vision", False) if defaults else False
            return create_custom_adapter(
                **key_kw, api_compat=compat, supports_web_search=ws,
                supports_vision=vis, **url_kw, **rpm_kw,
            )
        else:
            raise ValueError(f"Unknown provider: {provider!r}")

    # --- Adapter cache ---

    def get_adapter(self, provider: str, base_url: str | None = None) -> LLMAdapter:
        """Return cached adapter for *provider* + *base_url*, creating one on demand.

        The cache is keyed by ``(provider, base_url)`` so the same provider
        with different base URLs (e.g. OpenRouter vs local vLLM) gets separate
        adapter instances.

        Raises RuntimeError if the API key for *provider* is not configured.
        """
        provider = provider.lower()
        cache_key = (provider, base_url)

        # Fast path — no lock needed for reads of an already-cached adapter
        if cache_key in self._adapters:
            return self._adapters[cache_key]
        if base_url is None and (provider, None) in self._adapters:
            return self._adapters[(provider, None)]

        # Slow path — lock to prevent duplicate adapter creation
        with self._adapter_lock:
            # Double-check after acquiring lock
            if cache_key in self._adapters:
                return self._adapters[cache_key]
            if base_url is None and (provider, None) in self._adapters:
                return self._adapters[(provider, None)]

            # Need to create a new adapter — check API key first
            api_key = self._key_resolver(provider)
            if api_key is None:
                raise RuntimeError(
                    f"API key for provider {provider!r} is not configured. "
                    f"Set the appropriate environment variable or .env entry."
                )

            # For on-demand adapters without explicit base_url, check provider defaults
            effective_base_url = base_url
            if effective_base_url is None:
                defaults = self._get_provider_defaults(provider)
                effective_base_url = defaults.get("base_url") if defaults else None
            adapter = self._create_adapter(provider, api_key, effective_base_url)
            self._adapters[cache_key] = adapter
            return adapter

    # --- Capability routing ---

    def web_search(self, query: str) -> LLMResponse:
        """Web search — routed to configured web_search_provider."""
        provider_name = self._config.get("web_search_provider")
        if provider_name is None:
            return LLMResponse(text="")
        try:
            adapter = self.get_adapter(provider_name)
        except RuntimeError:
            return LLMResponse(text="")
        defaults = self._get_provider_defaults(provider_name)
        model = defaults.get("model", "") if defaults else ""
        return adapter.web_search(query, model=model)

    def make_multimodal_message(
        self, text: str, image_bytes: bytes, mime_type: str = "image/png"
    ) -> dict | None:
        """Vision — routed to configured vision_provider."""
        provider_name = self._config.get("vision_provider")
        if provider_name is None:
            return None
        try:
            adapter = self.get_adapter(provider_name)
        except RuntimeError:
            return None
        return adapter.make_multimodal_message(text, image_bytes, mime_type)

    def generate_vision(self, question: str, image_bytes: bytes, mime_type: str = "image/png") -> LLMResponse:
        """One-shot vision: send image + question, get text response.

        Routes to the configured vision_provider.
        """
        provider_name = self._config.get("vision_provider")
        if provider_name is None:
            return LLMResponse(text="")
        try:
            adapter = self.get_adapter(provider_name)
        except RuntimeError:
            return LLMResponse(text="")
        defaults = self._get_provider_defaults(provider_name)
        model = defaults.get("model", "") if defaults else ""
        return adapter.generate_vision(question, image_bytes, model=model, mime_type=mime_type)

    def _get_provider_defaults(self, provider_name: str) -> dict | None:
        """Get defaults for a provider from the injected provider_defaults dict."""
        return self._provider_defaults.get(provider_name)

    @property
    def provider(self) -> str:
        return self._provider

    @property
    def model(self) -> str:
        return self._model

    # --- Session management ---

    def create_session(
        self,
        system_prompt: str,
        tools: list[FunctionSchema] | None = None,
        *,
        model: str | None = None,
        thinking: str = "default",
        agent_type: str = "",
        tracked: bool = True,
        interaction_id: str | None = None,
        json_schema: dict | None = None,
        force_tool_call: bool = False,
        provider: str | None = None,
        interface: "ChatInterface | None" = None,
    ) -> ChatSession:
        """Start a new multi-turn conversation.

        Returns a ChatSession with a .session_id assigned.
        If *interface* is provided, restores an existing conversation history.
        """
        adapter = self.get_adapter(provider) if provider else self.get_adapter(self._provider, self._base_url)
        session_model = model or self._model
        ctx_window = get_context_limit(session_model)
        chat = adapter.create_chat(
            model=session_model,
            system_prompt=system_prompt,
            tools=tools,
            thinking=thinking,
            interaction_id=interaction_id,
            json_schema=json_schema,
            force_tool_call=force_tool_call,
            interface=interface,
            context_window=ctx_window,
        )
        if tracked:
            chat.session_id = _generate_session_id()
            chat._agent_type = agent_type
            chat._tracked = True
            self._sessions[chat.session_id] = chat
        else:
            chat.session_id = ""
            chat._tracked = False
        return chat

    def resume_session(self, saved_state: dict, *, thinking: str = "high") -> ChatSession:
        """Restore a session from a saved state dict."""
        session_id = saved_state.get("session_id", "")
        messages = saved_state.get("messages", [])
        metadata = saved_state.get("metadata", {})

        interface = ChatInterface.from_dict(messages)

        # Restore tools from interface so adapters can build provider-specific format
        tools = FunctionSchema.from_dicts(interface.current_tools)

        ctx_window = get_context_limit(self._model)
        chat = self.get_adapter(self._provider, self._base_url).create_chat(
            model=self._model,
            system_prompt=interface.current_system_prompt or "",
            tools=tools,
            interface=interface,
            thinking=thinking,
            context_window=ctx_window,
        )
        chat.session_id = session_id or _generate_session_id()
        chat._agent_type = metadata.get("agent_type", "")
        chat._tracked = metadata.get("tracked", True)
        if chat._tracked:
            self._sessions[chat.session_id] = chat
        return chat

    def get_session(self, session_id: str) -> ChatSession | None:
        """Look up an active session by ID."""
        return self._sessions.get(session_id)

    # --- One-shot generation ---

    def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        system_prompt: str | None = None,
        temperature: float | None = None,
        json_schema: dict | None = None,
        max_output_tokens: int | None = None,
        provider: str | None = None,
    ) -> LLMResponse:
        """Single-turn generation."""
        adapter = self.get_adapter(provider) if provider else self.get_adapter(self._provider, self._base_url)
        gen_model = model or self._model
        response = adapter.generate(
            model=gen_model,
            contents=prompt,
            system_prompt=system_prompt,
            temperature=temperature,
            json_schema=json_schema,
            max_output_tokens=max_output_tokens,
        )
        return response

    # --- Context compaction ---

    def check_and_compact(
        self,
        chat: ChatSession,
        *,
        summarizer: Callable[[str], str] | None = None,
        threshold: float = 0.8,
        provider: str | None = None,
    ) -> ChatSession | None:
        """Check context usage and compact if over threshold.

        Returns a NEW ChatSession if compaction happened, or None if no
        action needed.  The caller must replace their reference to the old
        session.

        Args:
            chat: The session to check.
            summarizer: Callable that takes conversation text and returns a
                summary string.  The caller is responsible for injecting any
                agent-specific context (system prompt, role) into the
                summarizer closure.  If None, no compaction is attempted.
            threshold: Fraction of context_window that triggers compaction.
            provider: Provider override for the new session.  If None, uses
                the service's default provider.
        """
        if summarizer is None:
            return None

        ctx_window = chat.context_window()
        if ctx_window <= 0:
            return None

        iface = chat.interface
        estimate = iface.estimate_context_tokens()
        if estimate <= 0 or estimate < ctx_window * threshold:
            return None

        # Find where to split
        boundary_id = iface.find_compaction_boundary(keep_turns=3)
        if boundary_id is None:
            return None

        # Format old entries for summary
        raw_text = iface.format_for_summary(boundary_id)
        if not raw_text.strip():
            return None

        summary = summarizer(raw_text)
        if not summary:
            return None

        # Build a new ChatInterface with: system preserved, summary +
        # assistant ack, then recent entries after boundary.
        from .interface import (
            ChatInterface as CI,
            TextBlock,
        )

        new_iface = CI()

        # Preserve system prompt + tools
        if iface.current_system_prompt is not None:
            new_iface.add_system(
                iface.current_system_prompt,
                tools=iface.current_tools,
            )

        # Add summary as user message + assistant acknowledgment
        new_iface.add_user_message(
            f"[Previous conversation summary]\n{summary}"
        )
        new_iface.add_assistant_message(
            [TextBlock(text="Understood. I have the context from the previous conversation.")],
        )

        # Copy recent entries (those with id >= boundary_id)
        for entry in iface.entries:
            if entry.id < boundary_id:
                continue
            if entry.role == "system":
                continue
            # Re-add entry content to new interface
            if entry.role == "user":
                # Could be text or tool results
                has_tool_results = bool(entry.content) and all(
                    isinstance(b, ToolResultBlock) for b in entry.content
                )
                if has_tool_results:
                    new_iface.add_tool_results(list(entry.content))
                else:
                    new_iface.add_user_blocks(list(entry.content))
            elif entry.role == "assistant":
                new_iface.add_assistant_message(
                    list(entry.content),
                    provider_data=entry.provider_data or None,
                    model=entry.model,
                    provider=entry.provider,
                    usage=entry.usage or None,
                )

        # Restore tools from interface as FunctionSchema
        tools = FunctionSchema.from_dicts(new_iface.current_tools)

        # Create a brand new session backed by the compacted interface.
        # Preserve the original session's provider so per-agent provider
        # overrides are not silently dropped.
        new_chat = self.create_session(
            system_prompt=new_iface.current_system_prompt or "",
            tools=tools,
            model=getattr(chat, "_model", None) or self._model,
            thinking="high",
            agent_type=chat._agent_type,
            tracked=chat._tracked,
            interface=new_iface,
            provider=provider,
        )

        return new_chat

    # --- Tool results ---

    def make_tool_result(
        self, tool_name: str, result: dict, *, tool_call_id: str | None = None,
        provider: str | None = None,
    ) -> ToolResultBlock:
        """Build a canonical ToolResultBlock."""
        adapter = self.get_adapter(provider) if provider else self.get_adapter(self._provider, self._base_url)
        return adapter.make_tool_result_message(
            tool_name, result, tool_call_id=tool_call_id,
        )
