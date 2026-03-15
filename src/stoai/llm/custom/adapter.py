"""Custom adapter — generic OpenAI or Anthropic-compatible provider.

For OpenRouter, SiliconFlow, Ollama, vLLM, or any other compatible service.
All configuration comes from the user's config.json — no hardcoded defaults
except api_compat and disabled capabilities.
"""
from ..base import LLMAdapter

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


def create_custom_adapter(
    api_key: str | None = None,
    api_compat: str = "openai",
    base_url: str | None = None,
    supports_web_search: bool = False,
    supports_vision: bool = False,
    **kwargs,
) -> LLMAdapter:
    """Factory: creates an OpenAI or Anthropic-based adapter from user config."""
    if not base_url:
        raise ValueError("Custom provider requires a base_url in config")

    if api_compat == "anthropic":
        from ..anthropic.adapter import AnthropicAdapter
        adapter = AnthropicAdapter(api_key=api_key, base_url=base_url, **kwargs)
    else:
        from ..openai.adapter import OpenAIAdapter
        adapter = OpenAIAdapter(api_key=api_key, base_url=base_url, **kwargs)

    adapter.supports_web_search = supports_web_search
    adapter.supports_vision = supports_vision
    return adapter
