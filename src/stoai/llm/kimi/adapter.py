"""Kimi (Moonshot) adapter — supports both OpenAI and Anthropic compat.

Default: OpenAI-compatible at https://api.moonshot.ai/v1.
User can set api_compat: "anthropic" to use Anthropic-compatible endpoint.
"""
from ..base import LLMAdapter

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


def create_kimi_adapter(
    api_key: str | None = None,
    api_compat: str | None = None,
    base_url: str | None = None,
    **kwargs,
) -> LLMAdapter:
    """Factory: creates an OpenAI or Anthropic-based adapter depending on api_compat."""
    import os

    compat = api_compat or DEFAULTS["api_compat"]
    key = api_key or os.getenv(DEFAULTS["api_key_env"])
    url = base_url or DEFAULTS["base_url"]

    if compat == "anthropic":
        from ..anthropic.adapter import AnthropicAdapter
        url = base_url or DEFAULTS.get("base_url_anthropic", url)
        adapter = AnthropicAdapter(api_key=key, base_url=url, **kwargs)
    else:
        from ..openai.adapter import OpenAIAdapter
        adapter = OpenAIAdapter(api_key=key, base_url=url, **kwargs)

    adapter.supports_web_search = True
    adapter.supports_vision = True
    return adapter
