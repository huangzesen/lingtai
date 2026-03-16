"""Delegate capability — spawn a clone of this agent on a new TCP port.

Creates a new BaseAgent with the same LLM service, config, and capabilities.
Optionally overrides the role and/or long-term memory (system prompt sections).
Returns the new agent's mail address so the parent can communicate with it.

Usage:
    agent.add_capability("delegate")
    # Then the LLM can call: delegate(role="researcher", ltm="focus on X")
"""
from __future__ import annotations

import socket
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "role": {
            "type": "string",
            "description": "Role/system prompt override for the new agent (optional, default = copy parent)",
        },
        "ltm": {
            "type": "string",
            "description": "Long-term memory / context to inject (optional)",
        },
        "capabilities": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Capability names for the new agent (optional, default = same as parent minus delegate)",
        },
    },
}

DESCRIPTION = (
    "Spawn a new agent on a free TCP port, cloned from this agent. "
    "Returns the new agent's mail address. "
    "Each spawned agent runs on its own TCP port with its own conversation. "
    "Use mail or email to communicate with spawned agents. "
    "Optionally override role, inject long-term memory, or select capabilities."
)


class DelegateManager:
    """Spawns clone agents on free TCP ports."""

    def __init__(self, agent: "BaseAgent"):
        self._agent = agent

    def handle(self, args: dict) -> dict:
        return self._spawn(args)

    def _spawn(self, args: dict) -> dict:
        from ..base_agent import BaseAgent
        from ..services.mail import TCPMailService

        parent = self._agent

        # Get a free TCP port
        port = self._get_free_port()
        child_id = f"{parent.agent_id}_child_{port}"

        # Resolve role — override or copy parent
        role = args.get("role") or parent._prompt_manager.read_section("role") or ""
        ltm = args.get("ltm") or parent._prompt_manager.read_section("ltm") or ""

        # Child is a peer in the same base_dir
        child_working_dir = parent._base_dir / child_id
        mail_svc = TCPMailService(listen_port=port, working_dir=child_working_dir)

        # Create child agent as peer
        child = BaseAgent(
            agent_id=child_id,
            service=parent.service,
            mail_service=mail_svc,
            config=parent._config,
            base_dir=parent._base_dir,
            streaming=parent._streaming,
            role=role,
            ltm=ltm,
        )

        # Replay capabilities — filter if specified, skip delegate to prevent recursion
        requested = args.get("capabilities")
        for cap_name, cap_kwargs in parent._capabilities:
            if cap_name == "delegate":
                continue  # no recursive spawning
            if requested is not None and cap_name not in requested:
                continue
            child.add_capability(cap_name, **cap_kwargs)

        child.start()
        address = mail_svc.address
        return {"status": "ok", "address": address, "agent_id": child.agent_id}

    @staticmethod
    def _get_free_port() -> int:
        """Get an available TCP port from the OS."""
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()
        return port


def setup(agent: "BaseAgent") -> DelegateManager:
    """Set up the delegate capability on an agent."""
    mgr = DelegateManager(agent)
    agent.add_tool("delegate", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)
    return mgr
