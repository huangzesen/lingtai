"""Agent — BaseAgent + composable capabilities.

Layer 2 of the three-layer hierarchy:
    BaseAgent (kernel) → Agent (capabilities) → CustomAgent (domain)

Capabilities are declared at construction and sealed before start().
"""
from __future__ import annotations

from typing import Any

from .base_agent import BaseAgent


class Agent(BaseAgent):
    """BaseAgent with composable capabilities.

    Args:
        capabilities: Capability names to enable. Either a list of strings
            (no kwargs) or a dict mapping names to kwargs dicts.
            Each capability dict may include ``"provider"`` to route that
            capability to a specific LLM provider (e.g. ``"gemini"``, ``"minimax"``).
            Group names (e.g. ``"file"``) expand to individual capabilities.
        *args, **kwargs: Passed through to BaseAgent.
    """

    # Maps capability name → LLMService provider_config key
    _CAPABILITY_PROVIDER_KEYS: dict[str, str] = {
        "web_search": "web_search_provider",
        "vision": "vision_provider",
        "draw": "image_provider",
        "compose": "music_provider",
        "talk": "tts_provider",
        "listen": "audio_provider",
    }

    def __init__(
        self,
        *args: Any,
        capabilities: list[str] | dict[str, dict] | None = None,
        **kwargs: Any,
    ):
        super().__init__(*args, **kwargs)

        # Auto-load MCP servers from working directory
        self._load_mcp_from_workdir()

        # Expand groups and normalize to dict
        if isinstance(capabilities, list):
            from .capabilities import expand_groups
            expanded = expand_groups(capabilities)
            capabilities = {name: {} for name in expanded}
        elif isinstance(capabilities, dict):
            from .capabilities import _GROUPS
            expanded_dict: dict[str, dict] = {}
            for name, cap_kwargs in capabilities.items():
                if name in _GROUPS:
                    for sub in _GROUPS[name]:
                        expanded_dict[sub] = {}
                else:
                    expanded_dict[name] = cap_kwargs
            capabilities = expanded_dict

        # Extract per-capability provider overrides, apply to LLMService config
        # Store providers separately so they survive pop() and replay in delegates
        self._capability_providers: dict[str, str] = {}
        if capabilities:
            for name, cap_kwargs in capabilities.items():
                cap_provider = cap_kwargs.pop("provider", None)
                if cap_provider:
                    self._capability_providers[name] = cap_provider
                    config_key = self._CAPABILITY_PROVIDER_KEYS.get(name)
                    if config_key:
                        self.service._config[config_key] = cap_provider

        # Track for delegate replay
        self._capabilities: list[tuple[str, dict]] = []
        self._capability_managers: dict[str, Any] = {}

        # Register capabilities
        if capabilities:
            for name, cap_kwargs in capabilities.items():
                self._setup_capability(name, **cap_kwargs)

    def _setup_capability(self, name: str, **kwargs: Any) -> Any:
        """Load a named capability.

        Not directly sealed — but setup() calls add_tool() which checks the seal.
        Must only be called from __init__ (before start()).
        """
        from .capabilities import setup_capability

        self._capabilities.append((name, dict(kwargs)))
        mgr = setup_capability(self, name, **kwargs)
        self._capability_managers[name] = mgr
        return mgr

    def _load_mcp_from_workdir(self) -> None:
        """Auto-load MCP servers declared in working_dir/mcp/servers.json.

        Format:
            {
              "server-name": {
                "command": "xhelio-spice-mcp",
                "args": [],
                "env": {}
              }
            }

        The file is created by setup agents or manually. Each server's
        tools are auto-registered via connect_mcp().
        """
        import json

        mcp_config = self._working_dir / "mcp" / "servers.json"
        if not mcp_config.is_file():
            return

        try:
            servers = json.loads(mcp_config.read_text())
        except (json.JSONDecodeError, OSError):
            return

        if not isinstance(servers, dict):
            return

        from .logging import get_logger
        logger = get_logger()

        for name, cfg in servers.items():
            if not isinstance(cfg, dict) or "command" not in cfg:
                continue
            try:
                tools = self.connect_mcp(
                    command=cfg["command"],
                    args=cfg.get("args"),
                    env=cfg.get("env"),
                )
                logger.info("[%s] MCP %s: loaded %d tools (%s)",
                            self.agent_name, name, len(tools), ", ".join(tools))
            except Exception as e:
                logger.warning("[%s] MCP %s: failed to load: %s",
                               self.agent_name, name, e)

    def get_capability(self, name: str) -> Any:
        """Return the manager instance for a registered capability, or None."""
        return self._capability_managers.get(name)
