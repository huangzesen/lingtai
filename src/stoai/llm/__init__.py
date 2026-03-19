"""LLM adapter layer — multi-provider support.

Adapters import their SDK lazily — only the active provider needs to be installed.
"""
from .base import LLMAdapter, ChatSession, LLMResponse, ToolCall, FunctionSchema
from .service import LLMService

__all__ = [
    "LLMAdapter",
    "ChatSession",
    "LLMResponse",
    "ToolCall",
    "FunctionSchema",
    "LLMService",
]

# Register built-in adapters on import
from ._register import register_all_adapters as _register_all_adapters
_register_all_adapters()
