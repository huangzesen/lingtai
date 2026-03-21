"""Custom adapter — named provider aliases with OpenAI, Anthropic, or Gemini compat.

Any provider name not matching a built-in (openai, anthropic, gemini, minimax)
routes here. The api_compat field selects the underlying adapter:

  "openai"    — OpenAI Chat Completions (default)
  "anthropic" — Anthropic Messages API
  "gemini"    — Google Gemini (requires api_key, ignores base_url)

Usage in config.json providers section:
  "openrouter":  {"api_compat": "openai",    "base_url": "https://openrouter.ai/api/v1", ...}
  "bedrock":     {"api_compat": "anthropic",  "base_url": "https://...", ...}
  "vertex":      {"api_compat": "gemini",     ...}
"""
from lingtai_kernel.llm.base import LLMAdapter

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


def create_custom_adapter(
    api_key: str | None = None,
    api_compat: str = "openai",
    base_url: str | None = None,
    **kwargs,
) -> LLMAdapter:
    """Factory: creates adapter based on api_compat."""
    if api_compat == "gemini":
        from ..gemini.adapter import GeminiAdapter
        return GeminiAdapter(api_key=api_key, **kwargs)
    elif api_compat == "anthropic":
        if not base_url:
            raise ValueError("Anthropic-compat provider requires a base_url")
        from ..anthropic.adapter import AnthropicAdapter
        return AnthropicAdapter(api_key=api_key, base_url=base_url, **kwargs)
    else:
        if not base_url:
            raise ValueError("OpenAI-compat provider requires a base_url")
        from ..openai.adapter import OpenAIAdapter
        return OpenAIAdapter(api_key=api_key, base_url=base_url, **kwargs)
