"""Delegate capability — spawn a new agent on a free TCP port.

Creates a new Agent with capabilities from the parent agent.
Optionally overrides the covenant and/or memory (system prompt sections).
Returns the new agent's mail address so the parent can communicate with it.
The reasoning field is sent as the first message (mission briefing) to the
spawned agent.

Usage:
    Agent(capabilities=["delegate"])
    # Then the LLM can call: delegate(covenant="researcher", memory="focus on X")
"""
from __future__ import annotations

import socket
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..agent import Agent

SCHEMA = {
    "type": "object",
    "properties": {
        "name": {
            "type": "string",
            "description": "Name for the new agent (required, e.g. 'researcher', 'analyst')",
        },
        "covenant": {
            "type": "string",
            "description": "Covenant override for the new agent (optional, default = copy parent)",
        },
        "memory": {
            "type": "string",
            "description": "Memory / context to inject (optional)",
        },
        "capabilities": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Capability names for the new agent (optional, default = same as parent minus delegate)",
        },
        "admin": {
            "type": "object",
            "description": (
                "Admin privileges for the new agent (optional, default = none). "
                "Dict of privilege name to boolean, e.g. {\"silence\": true}. "
                "Only grant privileges the child needs to manage its own children."
            ),
        },
        "provider": {
            "type": "string",
            "description": (
                "LLM provider for the new agent (optional, default = same as parent). "
                "Use a provider name defined in config, e.g. 'minimax', 'gemini', 'openrouter'."
            ),
        },
        "model": {
            "type": "string",
            "description": (
                "LLM model for the new agent (optional, default = same as parent). "
                "e.g. 'gemini-3-flash-preview', 'anthropic/claude-sonnet-4.6'."
            ),
        },
    },
    "required": ["name"],
}

DESCRIPTION = (
    "Spawn a new agent. "
    "Returns the new agent's mail address. "
    "Each spawned agent runs on its own TCP port with its own conversation. "
    "Use mail or email to communicate with spawned agents. "
    "Optionally override covenant, inject memory, or select capabilities. "
    "IMPORTANT: The reasoning field for this tool is sent as the first message "
    "to the spawned agent — write a thorough mission briefing: what to do, why, "
    "what context is needed, and what to report back."
)


class DelegateManager:
    """Spawns peer agents on free TCP ports."""

    def __init__(self, agent: "Agent"):
        self._agent = agent

    def handle(self, args: dict) -> dict:
        return self._spawn(args)

    def _spawn(self, args: dict) -> dict:
        from ..agent import Agent
        from ..services.mail import TCPMailService

        parent = self._agent
        reasoning = args.get("_reasoning")
        child_name = args.get("name", "delegate")

        # Get a free TCP port
        port = self._get_free_port()

        # Resolve covenant — override or copy parent
        covenant = args.get("covenant") or parent._prompt_manager.read_section("covenant") or ""
        memory = args.get("memory", "")

        # Build capabilities dict from parent (excluding delegate to prevent recursion)
        # provider is already in cap_kwargs — no re-injection needed
        requested = args.get("capabilities")
        caps: dict[str, dict] = {}
        for cap_name, cap_kwargs in parent._capabilities:
            if cap_name == "delegate":
                continue  # no recursive spawning
            if requested is not None and cap_name not in requested:
                continue
            caps[cap_name] = dict(cap_kwargs)

        # Delegate is a peer in the same base_dir
        delegate_working_dir = parent._base_dir / child_name
        mail_svc = TCPMailService(listen_port=port, working_dir=delegate_working_dir)

        # Build config — optionally override provider/model
        from ..config import AgentConfig
        child_provider = args.get("provider") or parent._config.provider
        child_model = args.get("model") or parent._config.model
        child_config = AgentConfig(
            max_turns=parent._config.max_turns,
            provider=child_provider,
            model=child_model,
            retry_timeout=parent._config.retry_timeout,
            thinking_budget=parent._config.thinking_budget,
        )

        # Create delegate agent
        admin = args.get("admin") or {}
        delegate = Agent(
            agent_name=child_name,
            service=parent.service,
            mail_service=mail_svc,
            config=child_config,
            base_dir=parent._base_dir,
            streaming=parent._streaming,
            covenant=covenant,
            memory=memory,
            capabilities=caps,
            admin=admin,
        )
        delegate.start()

        # Send reasoning as first prompt (mission briefing)
        if reasoning:
            delegate.send(reasoning, sender=parent.agent_id)

        address = mail_svc.address
        return {"status": "ok", "address": address, "agent_id": delegate.agent_id, "agent_name": delegate.agent_name}

    @staticmethod
    def _get_free_port() -> int:
        """Get an available TCP port from the OS."""
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()
        return port


def _build_schema(agent: "Agent") -> dict:
    """Build delegate schema with available providers from LLMService."""
    import copy
    schema = copy.deepcopy(SCHEMA)

    # Available providers = whatever is configured in provider_defaults
    try:
        defaults = agent.service._provider_defaults
        available = sorted(str(k) for k in defaults.keys() if isinstance(k, str))
    except (AttributeError, TypeError):
        available = []
    if not available:
        try:
            available = [str(agent.service.provider)]
        except (AttributeError, TypeError):
            available = []

    # Collect known models per provider
    provider_models: list[str] = []
    try:
        for pname, pdefaults in agent.service._provider_defaults.items():
            if isinstance(pdefaults, dict):
                m = pdefaults.get("model", "")
                if m:
                    provider_models.append(f"{pname}: {m}")
    except (AttributeError, TypeError):
        pass

    schema["properties"]["provider"]["description"] = (
        f"LLM provider for the new agent (optional, default = same as parent). "
        f"Available: {', '.join(available)}."
    )
    schema["properties"]["provider"]["enum"] = available

    if provider_models:
        schema["properties"]["model"]["description"] = (
            f"LLM model for the new agent (optional, default = same as parent). "
            f"Known: {'; '.join(provider_models)}."
        )

    return schema


def setup(agent: "Agent") -> DelegateManager:
    """Set up the delegate capability on an agent."""
    mgr = DelegateManager(agent)
    schema = _build_schema(agent)
    agent.add_tool("delegate", schema=schema, handler=mgr.handle, description=DESCRIPTION,
                    system_prompt="Spawn a new agent and communicate via email.")
    return mgr
