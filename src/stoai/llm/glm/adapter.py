"""GLM (Zhipu AI) adapter — thin subclass of OpenAIAdapter.

GLM's API is OpenAI-compatible at https://api.z.ai.
Web search via intrinsic web_search tool type.
Vision via GLM-4.6V+ models.
"""
from ..openai.adapter import OpenAIAdapter

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


class GLMAdapter(OpenAIAdapter):
    """Zhipu GLM adapter — OpenAI-compatible."""

    supports_web_search = True
    supports_vision = True

    def __init__(self, api_key: str | None = None, **kwargs):
        import os
        key = api_key or os.getenv(DEFAULTS["api_key_env"])
        super().__init__(api_key=key, base_url=DEFAULTS["base_url"], **kwargs)
