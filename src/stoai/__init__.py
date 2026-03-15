"""stoai — generic research agent with intrinsic tools."""
from .types import (
    MCPTool,
    UnknownToolError,
    AgentNotConnectedError,
    EVENT_TOOL_CALL,
    EVENT_TOOL_RESULT,
    EVENT_TEXT_DELTA,
    EVENT_AGENT_STATE,
    EVENT_COMPACTION,
    EVENT_LLM_CALL,
    EVENT_LLM_RESPONSE,
    EVENT_TOKEN_USAGE,
    EVENT_THINKING,
    EVENT_DEBUG,
)
from .config import AgentConfig
from .agent import BaseAgent, Message, AgentState
from .layers.diary import DiaryManager, add_diary_layer
from .layers.plan import PlanManager, add_plan_layer

__all__ = [
    "BaseAgent",
    "Message",
    "AgentState",
    "MCPTool",
    "AgentConfig",
    "UnknownToolError",
    "AgentNotConnectedError",
    "EVENT_TOOL_CALL",
    "EVENT_TOOL_RESULT",
    "EVENT_TEXT_DELTA",
    "EVENT_AGENT_STATE",
    "EVENT_COMPACTION",
    "EVENT_LLM_CALL",
    "EVENT_LLM_RESPONSE",
    "EVENT_TOKEN_USAGE",
    "EVENT_THINKING",
    "EVENT_DEBUG",
    "DiaryManager",
    "add_diary_layer",
    "PlanManager",
    "add_plan_layer",
]
