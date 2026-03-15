"""stoai — generic AI agent framework with intrinsic tools, composable capabilities, and pluggable services."""
from .types import (
    MCPTool,
    UnknownToolError,
)
from .config import AgentConfig
from .agent import BaseAgent, Message, AgentState

# Capabilities
from .capabilities import setup_capability
from .capabilities.bash import BashManager
from .capabilities.delegate import DelegateManager
from .capabilities.email import EmailManager

# Services
from .services.file_io import FileIOService, LocalFileIOService, GrepMatch
from .services.mail import MailService, TCPMailService
from .services.vision import VisionService, LLMVisionService
from .services.search import SearchService, LLMSearchService, SearchResult
from .services.logging import LoggingService, JSONLLoggingService

__all__ = [
    # Core
    "BaseAgent",
    "Message",
    "AgentState",
    "MCPTool",
    "AgentConfig",
    "UnknownToolError",
    # Capabilities
    "setup_capability",
    "BashManager",
    "DelegateManager",
    "EmailManager",
    # Services
    "FileIOService",
    "LocalFileIOService",
    "GrepMatch",
    "MailService",
    "TCPMailService",
    "VisionService",
    "LLMVisionService",
    "SearchService",
    "LLMSearchService",
    "SearchResult",
    "LoggingService",
    "JSONLLoggingService",
]
