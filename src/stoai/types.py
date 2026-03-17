"""Core types for stoai."""
from __future__ import annotations


class UnknownToolError(Exception):
    """Raised when a tool name cannot be resolved."""
    def __init__(self, tool_name: str):
        self.tool_name = tool_name
        super().__init__(f"Unknown tool: {tool_name}")

