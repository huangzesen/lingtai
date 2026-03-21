"""Avatar capability — spawn peer agents on free TCP ports.

Maintains an append-only ledger (delegates/ledger.jsonl) that records every
spawn event.  Each line is a timestamped record of what was spawned,
to whom, with what mission, privileges, and capabilities.  The ledger is
never mutated — only appended to.  It forms a responsibility map that the
parent can consult before spawning again.

Lifecycle management (kill, revive) is handled by the email capability,
not here.  The avatar tool's only job is to spawn avatars (分身).

Usage:
    Agent(capabilities=["avatar"])
    # avatar(name="researcher", ...)   — spawn or re-activate
"""
from __future__ import annotations

import json
import socket
import time
from pathlib import Path
from typing import TYPE_CHECKING

from ..i18n import t

if TYPE_CHECKING:
    from ..agent import Agent

def get_description(lang: str = "en") -> str:
    return t(lang, "avatar.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "name": {
                "type": "string",
                "description": t(lang, "avatar.name"),
            },
            "covenant": {
                "type": "string",
                "description": t(lang, "avatar.covenant"),
            },
            "memory": {
                "type": "string",
                "description": t(lang, "avatar.memory"),
            },
            "capabilities": {
                "type": "array",
                "items": {"type": "string"},
                "description": t(lang, "avatar.capabilities"),
            },
            "admin": {
                "type": "object",
                "description": t(lang, "avatar.admin"),
            },
            "provider": {
                "type": "string",
                "description": t(lang, "avatar.provider"),
            },
            "model": {
                "type": "string",
                "description": t(lang, "avatar.model"),
            },
        },
        "required": ["name"],
    }


# Backward compat
SCHEMA = get_schema("en")
DESCRIPTION = get_description("en")


class AvatarManager:
    """Spawns avatar (分身) peer agents on free TCP ports.

    Keeps an in-memory reference table for live status checks and an
    append-only JSONL ledger on disk that records every spawn.
    """

    def __init__(self, agent: "Agent"):
        self._agent = agent
        self._peers: dict[str, "Agent"] = {}  # name -> live Agent reference

    # ------------------------------------------------------------------
    # Handler
    # ------------------------------------------------------------------

    def handle(self, args: dict) -> dict:
        return self._spawn(args)

    # ------------------------------------------------------------------
    # Ledger (append-only JSONL log of avatar spawn events)
    # ------------------------------------------------------------------

    @property
    def _ledger_path(self) -> Path:
        return self._agent._working_dir / "delegates" / "ledger.jsonl"

    def _append_ledger(self, event: str, name: str, **fields) -> None:
        """Append a single event record to the ledger."""
        self._ledger_path.parent.mkdir(parents=True, exist_ok=True)
        record = {"ts": time.time(), "event": event, "name": name, **fields}
        with open(self._ledger_path, "a") as f:
            f.write(json.dumps(record, ensure_ascii=False) + "\n")

    # ------------------------------------------------------------------
    # Live status (reads from in-memory agent refs)
    # ------------------------------------------------------------------

    def _live_status(self, name: str) -> str:
        """Get live status of an avatar from the kernel's AgentState."""
        from lingtai_kernel.state import AgentState
        peer = self._peers.get(name)
        if peer is None:
            return "stopped"
        if peer._thread is None or not peer._thread.is_alive():
            return "stopped"
        state = peer._state
        if state == AgentState.ACTIVE:
            return "active"
        if state == AgentState.ERROR:
            return "error"
        if state == AgentState.DEAD:
            return "stopped"
        return "idle"

    # ------------------------------------------------------------------
    # Core spawn
    # ------------------------------------------------------------------

    def _spawn(self, args: dict) -> dict:
        from ..agent import Agent
        from lingtai_kernel.services.mail import TCPMailService

        parent = self._agent
        reasoning = args.get("_reasoning")
        peer_name = args.get("name", "avatar")

        # Check if this peer already exists and is live
        existing = self._peers.get(peer_name)
        if existing is not None:
            status = self._live_status(peer_name)
            if status == "active":
                return {
                    "status": "already_active",
                    "address": existing._mail_service.address if existing._mail_service else None,
                    "agent_id": existing.agent_id,
                    "agent_name": existing.agent_name,
                    "message": f"'{peer_name}' is currently active. Use email to communicate.",
                }
            elif status == "idle":
                if reasoning:
                    existing.send(reasoning, sender=parent.agent_id)
                    self._append_ledger(
                        "reactivate", peer_name, mission=reasoning,
                    )
                return {
                    "status": "reactivated",
                    "address": existing._mail_service.address if existing._mail_service else None,
                    "agent_id": existing.agent_id,
                    "agent_name": existing.agent_name,
                    "message": f"'{peer_name}' was idle — sent new mission briefing.",
                }
            elif status == "error":
                return {
                    "status": status,
                    "agent_name": peer_name,
                    "message": (
                        f"'{peer_name}' is {status}. "
                        f"To revive: spawn a new avatar with the SAME agent name "
                        f"(use email to kill it first, then spawn again)."
                    ),
                }
            # stopped — clean up stale reference before spawning fresh
            self._peers.pop(peer_name, None)

        # Resolve spawn parameters
        covenant = args.get("covenant") or parent._prompt_manager.read_section("covenant") or ""
        memory = args.get("memory", "")
        admin = args.get("admin") or {}

        requested = args.get("capabilities")
        caps: dict[str, dict] = {}
        cap_names: list[str] = []
        for cap_name, cap_kwargs in parent._capabilities:
            if cap_name == "avatar":
                continue
            if requested is not None and cap_name not in requested:
                continue
            caps[cap_name] = dict(cap_kwargs)
            cap_names.append(cap_name)

        # Spawn peer agent
        port = self._get_free_port()
        avatar_working_dir = parent._base_dir / peer_name
        mail_svc = TCPMailService(listen_port=port, working_dir=avatar_working_dir)

        from lingtai_kernel.config import AgentConfig
        peer_provider = args.get("provider") or parent._config.provider
        peer_model = args.get("model") or parent._config.model
        peer_config = AgentConfig(
            max_turns=parent._config.max_turns,
            provider=peer_provider,
            model=peer_model,
            retry_timeout=parent._config.retry_timeout,
            thinking_budget=parent._config.thinking_budget,
            language=parent._config.language,
        )

        avatar = Agent(
            agent_name=peer_name,
            service=parent.service,
            mail_service=mail_svc,
            config=peer_config,
            base_dir=parent._base_dir,
            streaming=parent._streaming,
            covenant=covenant,
            memory=memory,
            capabilities=caps,
            admin=admin,
        )
        avatar.start()

        if reasoning:
            avatar.send(reasoning, sender=parent.agent_id)

        # Record
        self._peers[peer_name] = avatar
        address = mail_svc.address
        self._append_ledger(
            "avatar", peer_name,
            address=address,
            agent_id=avatar.agent_id,
            port=port,
            mission=reasoning or "",
            privileges=admin,
            capabilities=cap_names,
            provider=peer_provider,
            model=peer_model,
        )

        return {
            "status": "ok",
            "address": address,
            "agent_id": avatar.agent_id,
            "agent_name": avatar.agent_name,
        }

    @staticmethod
    def _get_free_port() -> int:
        """Get an available TCP port from the OS."""
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()
        return port


def _build_schema(agent: "Agent") -> dict:
    """Build avatar schema with available providers from LLMService."""
    import copy
    lang = agent._config.language
    schema = copy.deepcopy(get_schema(lang))

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

    provider_models: list[str] = []
    try:
        for pname, pdefaults in agent.service._provider_defaults.items():
            if isinstance(pdefaults, dict):
                m = pdefaults.get("model", "")
                if m:
                    provider_models.append(f"{pname}: {m}")
    except (AttributeError, TypeError):
        pass

    schema["properties"]["provider"]["description"] = t(
        lang, "avatar.provider_dynamic", available=", ".join(available)
    )
    schema["properties"]["provider"]["enum"] = available

    if provider_models:
        schema["properties"]["model"]["description"] = t(
            lang, "avatar.model_dynamic", known="; ".join(provider_models)
        )

    return schema


def setup(agent: "Agent") -> AvatarManager:
    """Set up the avatar capability on an agent."""
    lang = agent._config.language
    mgr = AvatarManager(agent)
    schema = _build_schema(agent)
    agent.add_tool("avatar", schema=schema, handler=mgr.handle, description=get_description(lang))
    return mgr
