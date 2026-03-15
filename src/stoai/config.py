"""Agent configuration — injected at construction, not read from files."""
from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class AgentConfig:
    """Configuration for a BaseAgent instance.

    The host app reads its own config files and passes resolved values here.
    No file-based config reading inside stoai.
    """
    max_turns: int = 50
    provider: str | None = None  # None = use LLMService's provider
    model: str | None = None
    api_key: str | None = None
    base_url: str | None = None
    retry_timeout: float = 30.0
    thinking_budget: int | None = None
    data_dir: str | None = None  # for cache files (e.g., model context windows)
    bash_policy_file: str | None = None  # path to bash policy JSON
