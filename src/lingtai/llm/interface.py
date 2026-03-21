"""Re-export kernel LLM interface types for backward compatibility."""
from lingtai_kernel.llm.interface import (
    TextBlock,
    ToolCallBlock,
    ToolResultBlock,
    ThinkingBlock,
    ContentBlock,
    InterfaceEntry,
    ChatInterface,
)

__all__ = [
    "TextBlock",
    "ToolCallBlock",
    "ToolResultBlock",
    "ThinkingBlock",
    "ContentBlock",
    "InterfaceEntry",
    "ChatInterface",
]
