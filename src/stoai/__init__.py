"""stoai — generic AI agent framework with intrinsic tools, composable layers, and pluggable services."""
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

# Layers
from .layers.diary import DiaryManager, add_diary_layer
from .layers.plan import PlanManager, add_plan_layer
from .layers.bash import BashManager, add_bash_layer
from .layers.delegate import DelegateManager, add_delegate_layer

# Services
from .services.file_io import FileIOService, LocalFileIOService, GrepMatch
from .services.email import EmailService, TCPEmailService
from .services.vision import VisionService, LLMVisionService
from .services.search import SearchService, LLMSearchService, SearchResult

__all__ = [
    # Core
    "BaseAgent",
    "Message",
    "AgentState",
    "MCPTool",
    "AgentConfig",
    "UnknownToolError",
    "AgentNotConnectedError",
    # Events
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
    # Layers
    "DiaryManager",
    "add_diary_layer",
    "PlanManager",
    "add_plan_layer",
    "BashManager",
    "add_bash_layer",
    "DelegateManager",
    "add_delegate_layer",
    # Services
    "FileIOService",
    "LocalFileIOService",
    "GrepMatch",
    "EmailService",
    "TCPEmailService",
    "VisionService",
    "LLMVisionService",
    "SearchService",
    "LLMSearchService",
    "SearchResult",
]
