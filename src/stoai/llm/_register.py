"""Register all built-in LLM adapter factories with LLMService.

Each factory uses lazy imports so provider SDKs are only loaded when first used.
Each factory receives (model, defaults, **kw) from _create_adapter() and maps
to the adapter's actual constructor signature.
"""
from __future__ import annotations


def register_all_adapters() -> None:
    from .service import LLMService

    def _gemini(*, model=None, defaults=None, api_key=None, max_rpm=0, **_kw):
        from .gemini.adapter import GeminiAdapter
        kw: dict = {}
        if api_key is not None: kw["api_key"] = api_key
        if max_rpm > 0: kw["max_rpm"] = max_rpm
        if model: kw["default_model"] = model
        return GeminiAdapter(**kw)

    def _anthropic(*, model=None, defaults=None, **kw):
        from .anthropic.adapter import AnthropicAdapter
        kw.pop("model", None)
        return AnthropicAdapter(**{k: v for k, v in kw.items() if v is not None})

    def _openai(*, model=None, defaults=None, **kw):
        from .openai.adapter import OpenAIAdapter
        kw.pop("model", None)
        return OpenAIAdapter(**{k: v for k, v in kw.items() if v is not None})

    def _minimax(*, model=None, defaults=None, **kw):
        from .minimax.adapter import MiniMaxAdapter
        kw.pop("model", None)
        return MiniMaxAdapter(**{k: v for k, v in kw.items() if v is not None})

    def _custom(*, model=None, defaults=None, **kw):
        from .custom.adapter import create_custom_adapter
        kw.pop("model", None)
        compat = defaults.get("api_compat", "openai") if defaults else "openai"
        return create_custom_adapter(api_compat=compat, **{k: v for k, v in kw.items() if v is not None})

    LLMService.register_adapter("gemini", _gemini)
    LLMService.register_adapter("anthropic", _anthropic)
    LLMService.register_adapter("openai", _openai)
    LLMService.register_adapter("minimax", _minimax)
    LLMService.register_adapter("custom", _custom)

    # Providers routed through the custom adapter
    for name in ("deepseek", "grok", "qwen", "glm", "kimi"):
        LLMService.register_adapter(name, _custom)
