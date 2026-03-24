"""LLMService — concrete implementation of the kernel ABC.

Adapter-based LLM access: adapter registry, session management,
one-shot generation, and model context-window lookup.

Decoupled from any app-specific config system:
- API key resolution via injected ``key_resolver`` callable (defaults to env vars)
- Provider defaults via injected ``provider_defaults`` dict
"""

from __future__ import annotations

import os
import uuid
from collections.abc import Callable
from typing import Any

from lingtai_kernel.llm.base import (
    ChatSession,
    FunctionSchema,
    LLMResponse,
)
from lingtai_kernel.llm.interface import ChatInterface, ToolResultBlock
from lingtai_kernel.llm.service import LLMService as LLMServiceABC

from .base import LLMAdapter

# ---------------------------------------------------------------------------
# Model context-window registry (built-in, no external fetches)
# ---------------------------------------------------------------------------

# Known model context windows — prefix-matched, so "claude-sonnet-4" covers
# all dated variants.  Maintained manually; update when new models ship.
CONTEXT_WINDOWS: dict[str, int] = {
    # Anthropic
    "claude-opus-4":        200_000,
    "claude-sonnet-4":      200_000,
    "claude-haiku-4":       200_000,
    "claude-3-5-sonnet":    200_000,
    "claude-3-5-haiku":     200_000,
    "claude-3-opus":        200_000,
    "claude-3-sonnet":      200_000,
    "claude-3-haiku":       200_000,
    # Google Gemini
    "gemini-3-flash":     1_000_000,
    "gemini-2.5-pro":     1_000_000,
    "gemini-2.5-flash":   1_000_000,
    "gemini-2.0-flash":   1_048_576,
    "gemini-1.5-pro":     2_097_152,
    "gemini-1.5-flash":   1_048_576,
    # OpenAI
    "gpt-4.1":              1_047_576,
    "gpt-4.1-mini":         1_047_576,
    "gpt-4.1-nano":         1_047_576,
    "gpt-4o":               128_000,
    "gpt-4o-mini":          128_000,
    "gpt-4-turbo":          128_000,
    "o3":                   200_000,
    "o3-mini":              200_000,
    "o4-mini":              200_000,
    # MiniMax
    "MiniMax-M2":           200_000,
    # DeepSeek
    "deepseek-chat":        128_000,
    "deepseek-reasoner":    128_000,
    # Qwen
    "qwen-max":             128_000,
    "qwen-plus":            128_000,
    "qwen-turbo":           128_000,
    "qwen3":                128_000,
    # GLM
    "glm-4":                128_000,
    # Kimi
    "moonshot-v1":          128_000,
    # Grok
    "grok-3":               131_072,
    "grok-3-mini":          131_072,
}


def get_context_limit(model_name: str) -> int:
    """Return context window size for a model.

    Raises ValueError if the model is not in the built-in registry.

    Resolution: exact match, then longest prefix match.
    """
    if not model_name:
        raise ValueError("model_name is required for context window lookup")

    # Exact match
    if model_name in CONTEXT_WINDOWS:
        return CONTEXT_WINDOWS[model_name]

    # Longest prefix match
    best, best_len = 0, 0
    for prefix, limit in CONTEXT_WINDOWS.items():
        if model_name.startswith(prefix) and len(prefix) > best_len:
            best, best_len = limit, len(prefix)
    if best > 0:
        return best

    raise ValueError(
        f"Unknown model {model_name!r} — not in CONTEXT_WINDOWS registry. "
        f"Add it to lingtai.llm.service.CONTEXT_WINDOWS or pass context_window= explicitly."
    )


def _generate_session_id() -> str:
    """Generate a unique lingtai session ID."""
    return f"st_{uuid.uuid4().hex[:12]}"


class LLMService(LLMServiceABC):
    """Concrete LLM service — adapter registry, session management, generation.

    Responsibilities:
    - Adapter factory: constructs adapters via class-level registry
    - Session registry: assigns lingtai session IDs, tracks active sessions
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

    _adapter_registry: dict[str, Callable[..., LLMAdapter]] = {}

    @classmethod
    def register_adapter(cls, name: str, factory: Callable[..., LLMAdapter]) -> None:
        """Register an adapter factory by provider name.

        The factory receives keyword arguments: model, defaults, api_key,
        base_url, max_rpm.
        """
        cls._adapter_registry[name.lower()] = factory

    def __init__(
        self,
        provider: str,
        model: str,
        api_key: str | None = None,
        base_url: str | None = None,
        key_resolver: Callable[[str], str | None] | None = None,
        provider_defaults: dict | None = None,
    ) -> None:
        self._provider = provider.lower()
        self._model = model
        self._base_url = base_url
        self._key_resolver = key_resolver or (lambda p: os.environ.get(f"{p.upper()}_API_KEY"))
        self._provider_defaults = provider_defaults or {}
        self._adapters: dict[tuple[str, str | None], LLMAdapter] = {}
        self._adapter_lock = threading.Lock()
        self._adapters[(self._provider, base_url)] = self._create_adapter(self._provider, api_key, base_url)
        self._sessions: dict[str, ChatSession] = {}

    def _create_adapter(self, provider: str, api_key: str | None, base_url: str | None) -> LLMAdapter:
        key_kw: dict = {"api_key": api_key} if api_key is not None else {}
        defaults = self._get_provider_defaults(provider)
        effective_url = base_url or (defaults.get("base_url") if defaults else None)
        url_kw: dict = {"base_url": effective_url} if effective_url is not None else {}
        max_rpm = defaults.get("max_rpm", 0) if defaults else 0
        rpm_kw: dict = {"max_rpm": max_rpm} if max_rpm > 0 else {}

        p = provider.lower()
        factory = self._adapter_registry.get(p)
        if factory is None:
            raise RuntimeError(
                f"No adapter registered for provider {provider!r}. "
                f"Registered: {', '.join(sorted(self._adapter_registry)) or '(none)'}. "
                f"If using lingtai, ensure 'import lingtai' runs before creating LLMService."
            )

        return factory(
            model=self._model,
            defaults=defaults,
            **key_kw, **url_kw, **rpm_kw,
        )

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
        # When no base_url specified, find any cached adapter for this provider
        if base_url is None:
            for (p, _url), adapter in self._adapters.items():
                if p == provider:
                    return adapter

        # Slow path — lock to prevent duplicate adapter creation
        with self._adapter_lock:
            # Double-check after acquiring lock
            if cache_key in self._adapters:
                return self._adapters[cache_key]
            if base_url is None:
                for (p, _url), adapter in self._adapters.items():
                    if p == provider:
                        return adapter

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
        interface: ChatInterface | None = None,
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
