"""Grok (xAI) adapter — thin subclass of OpenAIAdapter.

Grok's API is fully OpenAI-compatible at https://api.x.ai/v1.
Web search uses Grok's native Agent Tools (web_search, x_search).
Vision is supported natively on vision-capable models.
"""
from ..openai.adapter import OpenAIAdapter

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


class GrokAdapter(OpenAIAdapter):
    """xAI Grok adapter — OpenAI-compatible."""

    supports_web_search = True
    supports_vision = True

    def __init__(self, api_key: str | None = None, **kwargs):
        import os
        key = api_key or os.getenv(DEFAULTS["api_key_env"])
        super().__init__(api_key=key, base_url=DEFAULTS["base_url"], **kwargs)
