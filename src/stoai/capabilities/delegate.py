"""Delegate capability — spawn a new agent on a free TCP port.

Creates a new StoAIAgent with capabilities from the parent agent.
Optionally overrides the role and/or long-term memory (system prompt sections).
Returns the new agent's mail address so the parent can communicate with it.
The reasoning field is sent as the first message (mission briefing) to the
spawned agent.

Usage:
    StoAIAgent(capabilities=["delegate"])
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
    "Spawn a new agent. "
    "Returns the new agent's mail address. "
    "Each spawned agent runs on its own TCP port with its own conversation. "
    "Use mail or email to communicate with spawned agents. "
    "Optionally override role, inject long-term memory, or select capabilities. "
    "IMPORTANT: The reasoning field for this tool is sent as the first message "
    "to the spawned agent — write a thorough mission briefing: what to do, why, "
    "what context is needed, and what to report back."
)


class DelegateManager:
    """Spawns peer agents on free TCP ports."""

    def __init__(self, agent: "BaseAgent"):
        self._agent = agent

    def handle(self, args: dict) -> dict:
        return self._spawn(args)

    def _spawn(self, args: dict) -> dict:
        from ..stoai_agent import StoAIAgent
        from ..services.mail import TCPMailService

        parent = self._agent
        reasoning = args.get("_reasoning")

        # Get a free TCP port
        port = self._get_free_port()
        child_id = f"{parent.agent_id}_delegate_{port}"

        # Resolve role — override or copy parent
        role = args.get("role") or parent._prompt_manager.read_section("role") or ""
        ltm = args.get("ltm") or parent._prompt_manager.read_section("ltm") or ""

        # Build capabilities dict from parent (excluding delegate to prevent recursion)
        requested = args.get("capabilities")
        caps: dict[str, dict] = {}
        for cap_name, cap_kwargs in parent._capabilities:
            if cap_name == "delegate":
                continue  # no recursive spawning
            if requested is not None and cap_name not in requested:
                continue
            caps[cap_name] = cap_kwargs

        # Delegate is a peer in the same base_dir
        delegate_working_dir = parent._base_dir / child_id
        mail_svc = TCPMailService(listen_port=port, working_dir=delegate_working_dir)

        # Create delegate agent
        delegate = StoAIAgent(
            agent_id=child_id,
            service=parent.service,
            mail_service=mail_svc,
            config=parent._config,
            base_dir=parent._base_dir,
            streaming=parent._streaming,
            role=role,
            ltm=ltm,
            capabilities=caps,
        )
        delegate.start()

        # Send reasoning as first prompt (mission briefing)
        if reasoning:
            delegate.send(reasoning, sender=parent.agent_id, wait=False)

        address = mail_svc.address
        return {"status": "ok", "address": address, "agent_id": delegate.agent_id}

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
