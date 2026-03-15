"""Core types for stoai."""
from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable


@dataclass
class MCPTool:
    """A domain tool provided to an agent via MCP-compatible interface."""
    name: str
    schema: dict
    description: str
    handler: Callable[[dict], dict]


class UnknownToolError(Exception):
    """Raised when a tool name cannot be resolved."""
    def __init__(self, tool_name: str):
        self.tool_name = tool_name
        super().__init__(f"Unknown tool: {tool_name}")


class AgentNotConnectedError(Exception):
    """Raised when talk targets an agent that is not connected."""
    def __init__(self, target_id: str):
        self.target_id = target_id
        super().__init__(f"Agent not connected: {target_id}")


# Event type constants (used with on_event callback)
EVENT_TOOL_CALL = "tool_call"
EVENT_TOOL_RESULT = "tool_result"
EVENT_TEXT_DELTA = "text_delta"
EVENT_AGENT_STATE = "agent_state"
EVENT_COMPACTION = "compaction"
EVENT_LLM_CALL = "llm_call"
EVENT_LLM_RESPONSE = "llm_response"
EVENT_TOKEN_USAGE = "token_usage"
EVENT_THINKING = "thinking"
EVENT_DEBUG = "debug"
