"""DeepSeek adapter — thin subclass of OpenAIAdapter.

DeepSeek's API is OpenAI-compatible at https://api.deepseek.com.
No native web search or vision support.
"""
from ..openai.adapter import OpenAIAdapter

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


class DeepSeekAdapter(OpenAIAdapter):
    """DeepSeek adapter — OpenAI-compatible, no web search or vision."""

    supports_web_search = False
    supports_vision = False

    def __init__(self, api_key: str | None = None, **kwargs):
        import os
        key = api_key or os.getenv(DEFAULTS["api_key_env"])
        super().__init__(api_key=key, base_url=DEFAULTS["base_url"], **kwargs)
