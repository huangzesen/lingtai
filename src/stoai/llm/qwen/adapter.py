"""Qwen (Alibaba DashScope) adapter — thin subclass of OpenAIAdapter.

Qwen's API is OpenAI-compatible via DashScope.
Web search via built-in tool. Vision via Qwen-VL models.
"""
from ..openai.adapter import OpenAIAdapter

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


class QwenAdapter(OpenAIAdapter):
    """Alibaba Qwen adapter — OpenAI-compatible via DashScope."""

    supports_web_search = True
    supports_vision = True

    def __init__(self, api_key: str | None = None, **kwargs):
        import os
        key = api_key or os.getenv(DEFAULTS["api_key_env"])
        super().__init__(api_key=key, base_url=DEFAULTS["base_url"], **kwargs)
