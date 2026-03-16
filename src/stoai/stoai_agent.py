"""StoAIAgent — BaseAgent + composable capabilities + domain tools.

Layer 2 of the three-layer hierarchy:
    BaseAgent (kernel) → StoAIAgent (capabilities) → CustomAgent (domain)

Capabilities and tools are declared at construction and sealed before start().
"""
from __future__ import annotations

from typing import Any

from .base_agent import BaseAgent
from .types import MCPTool


class StoAIAgent(BaseAgent):
    """BaseAgent with composable capabilities and domain tools.

    Args:
        capabilities: Capability names to enable. Either a list of strings
            (no kwargs) or a dict mapping names to kwargs dicts.
            Example: ``["vision", "bash"]`` or ``{"bash": {"policy_file": "p.json"}}``.
        tools: Domain tools (MCP tools) to register. Each tool gets an ``[MCP]``
            prefix in its LLM-visible description.
        *args, **kwargs: Passed through to BaseAgent.
    """

    def __init__(
        self,
        *args: Any,
        capabilities: list[str] | dict[str, dict] | None = None,
        tools: list[MCPTool] | None = None,
        **kwargs: Any,
    ):
        super().__init__(*args, **kwargs)

        # Normalize list to dict
        if isinstance(capabilities, list):
            capabilities = {name: {} for name in capabilities}

        # Track for delegate replay
        self._capabilities: list[tuple[str, dict]] = []
        self._capability_managers: dict[str, Any] = {}

        # Register capabilities
        if capabilities:
            for name, cap_kwargs in capabilities.items():
                self._setup_capability(name, **cap_kwargs)

        # Register domain tools
        if tools:
            for tool in tools:
                self.add_tool(
                    tool.name,
                    schema=tool.schema,
                    handler=tool.handler,
                    description=tool.description,
                )
                self._mcp_tool_names.add(tool.name)

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

    def get_capability(self, name: str) -> Any:
        """Return the manager instance for a registered capability, or None."""
        return self._capability_managers.get(name)
