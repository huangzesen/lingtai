"""lingtai — generic AI agent framework with intrinsic tools, composable capabilities, and pluggable services."""

from importlib.metadata import version as _pkg_version

__version__ = _pkg_version("lingtai")

from lingtai_kernel.types import UnknownToolError
from lingtai_kernel.config import AgentConfig
from lingtai_kernel.base_agent import BaseAgent
from .agent import Agent
from lingtai_kernel.state import AgentState
from lingtai_kernel.message import Message, MSG_REQUEST, MSG_USER_INPUT

# Capabilities
from .capabilities import setup_capability
from .capabilities.bash import BashManager
from .capabilities.avatar import AvatarManager
from .capabilities.email import EmailManager

# Services
from .services.file_io import FileIOService, LocalFileIOService, GrepMatch
from lingtai_kernel.services.mail import MailService, FilesystemMailService
from lingtai_kernel.services.logging import LoggingService, JSONLLoggingService
from .services.vision import VisionService, create_vision_service
from .services.websearch import SearchService, SearchResult, create_search_service
from .services.tts import TTSService, create_tts_service
from .services.image_gen import ImageGenService, create_image_gen_service
from .services.transcription import TranscriptionService, TranscriptionResult, create_transcription_service
from .services.music_gen import MusicGenService, create_music_gen_service

__all__ = [
    "__version__",
    # Core
    "BaseAgent",
    "Agent",
    "Message",
    "AgentState",
    "MSG_REQUEST",
    "MSG_USER_INPUT",
    "AgentConfig",
    "UnknownToolError",
    # Capabilities
    "setup_capability",
    "BashManager",
    "AvatarManager",
    "EmailManager",
    # Services
    "FileIOService",
    "LocalFileIOService",
    "GrepMatch",
    "MailService",
    "FilesystemMailService",
    "LoggingService",
    "JSONLLoggingService",
    "VisionService",
    "create_vision_service",
    "SearchService",
    "SearchResult",
    "create_search_service",
    "TTSService",
    "create_tts_service",
    "ImageGenService",
    "create_image_gen_service",
    "TranscriptionService",
    "TranscriptionResult",
    "create_transcription_service",
    "MusicGenService",
    "create_music_gen_service",
]
