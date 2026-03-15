"""Delegate capability — agent spawning, role injection, and MCP tool wiring.

The delegate capability combines:
1. Agent spawning — create new agent instances
2. Role injection — give spawned agents specific roles via system prompt
3. MCP injection — give spawned agents specific tools
4. Mail wiring — connect spawned agents so they can communicate

This is a capability (not intrinsic) because it's a coordination capability
built on top of the base agent. The base mail intrinsic provides raw
messaging; delegate adds the orchestration patterns.

Usage:
    agent.add_capability("delegate", agent_factory=my_factory_fn)

Design notes:
- Delegate builds on the mail intrinsic for communication
- Spawned agents are tracked by the DelegateManager
- The delegate tool handles spawning + role injection + sending the initial task
- Sync-over-mail patterns (send and wait for reply) live here, not in base
"""
from __future__ import annotations

from typing import TYPE_CHECKING, Any, Callable

if TYPE_CHECKING:
    from ..agent import BaseAgent
    from ..types import MCPTool

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["spawn", "send", "list", "stop"],
            "description": "Action to perform",
        },
        "role": {
            "type": "string",
            "description": "Role/system prompt for the new agent (for spawn)",
        },
        "task": {
            "type": "string",
            "description": "Initial task to send to the agent (for spawn/send)",
        },
        "agent_id": {
            "type": "string",
            "description": "Target agent ID (for send/stop)",
        },
        "tools": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Tool names to give the spawned agent (for spawn)",
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Delegate work to sub-agents. Spawn new agents with specific roles and tools, "
    "send tasks to existing agents, list active agents, or stop them."
)


class DelegateManager:
    """Manages agent delegation — spawning, tracking, and communication.

    This is a stub implementation capturing the design intent.
    The full implementation requires:
    1. An agent_factory callable that creates BaseAgent instances
    2. MailService wiring for communication between agents
    3. Lifecycle management (tracking, cleanup)
    """

    def __init__(
        self,
        agent_factory: Callable[..., "BaseAgent"] | None = None,
        default_tools: list["MCPTool"] | None = None,
    ):
        self._factory = agent_factory
        self._default_tools = default_tools or []
        self._agents: dict[str, "BaseAgent"] = {}

    def handle(self, args: dict) -> dict:
        action = args.get("action")

        if action == "spawn":
            return self._spawn(args)
        elif action == "send":
            return self._send(args)
        elif action == "list":
            return self._list()
        elif action == "stop":
            return self._stop(args)
        else:
            return {"error": f"Unknown action: {action}"}

    def _spawn(self, args: dict) -> dict:
        if self._factory is None:
            return {"error": "No agent_factory configured — cannot spawn agents"}

        role = args.get("role", "")
        task = args.get("task", "")
        if not role and not task:
            return {"error": "At least one of 'role' or 'task' is required for spawn"}

        # TODO: full implementation
        # 1. Call self._factory() to create a new BaseAgent
        # 2. Inject role via agent.update_system_prompt("role", role, protected=True)
        # 3. Inject requested tools from available pool
        # 4. Wire mail service for communication
        # 5. Start the agent
        # 6. Send initial task via mail
        # 7. Track in self._agents

        return {"error": "spawn not yet implemented — agent_factory integration pending"}

    def _send(self, args: dict) -> dict:
        agent_id = args.get("agent_id", "")
        task = args.get("task", "")
        if not agent_id:
            return {"error": "agent_id is required for send"}
        if not task:
            return {"error": "task is required for send"}
        if agent_id not in self._agents:
            return {"error": f"No active agent with id: {agent_id}"}

        # TODO: send via mail service
        return {"error": "send not yet implemented"}

    def _list(self) -> dict:
        agents = [
            {"agent_id": aid, "status": "active"}
            for aid in self._agents
        ]
        return {"status": "ok", "agents": agents, "count": len(agents)}

    def _stop(self, args: dict) -> dict:
        agent_id = args.get("agent_id", "")
        if not agent_id:
            return {"error": "agent_id is required for stop"}
        if agent_id not in self._agents:
            return {"error": f"No active agent with id: {agent_id}"}

        # TODO: stop agent, clean up
        return {"error": "stop not yet implemented"}


def setup(
    agent: "BaseAgent",
    agent_factory: Callable[..., "BaseAgent"] | None = None,
    default_tools: list["MCPTool"] | None = None,
) -> DelegateManager:
    """Set up the delegate capability on an agent.

    Args:
        agent: The agent to extend.
        agent_factory: Callable that creates new BaseAgent instances.
        default_tools: Default MCP tools to give spawned agents.

    Returns:
        The DelegateManager instance for programmatic access.
    """
    mgr = DelegateManager(agent_factory=agent_factory, default_tools=default_tools)
    agent.add_tool("delegate", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    agent.update_system_prompt(
        "delegate_instructions",
        "You can delegate work to sub-agents via the delegate tool. "
        "Spawn new agents with specific roles and tools for specialized tasks. "
        "Use delegation for work that benefits from a separate context or expertise.",
    )
    return mgr
