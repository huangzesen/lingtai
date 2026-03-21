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
import os
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
            "combo": {
                "type": "string",
                "description": t(lang, "avatar.combo"),
            },
            "language": {
                "type": "string",
                "enum": ["en", "zh", "lzh"],
                "description": t(lang, "avatar.language"),
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

    def __init__(self, agent: "Agent", max_agents: int = 0):
        self._agent = agent
        self._max_agents = max_agents  # 0 = unlimited
        self._peers: dict[str, "Agent"] = {}  # name -> live Agent reference

        # Load parent's combo for default avatar LLM config
        try:
            combo_path = Path(agent._working_dir) / "combo.json"
            if combo_path.is_file():
                self._parent_combo = json.loads(combo_path.read_text(encoding="utf-8"))
            else:
                self._parent_combo = None
        except (json.JSONDecodeError, OSError, TypeError):
            self._parent_combo = None

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

        # Agent count guard
        if self._max_agents > 0:
            live = len(list(parent._base_dir.glob("*/.agent.json")))
            if live >= self._max_agents:
                lang = parent._config.language
                return {"error": t(lang, "avatar.limit_reached", live=live, max=self._max_agents)}

        # Resolve spawn parameters
        covenant = args.get("covenant") or parent._prompt_manager.read_section("covenant") or ""
        memory = args.get("memory", "")
        admin = args.get("admin") or {}

        requested = args.get("capabilities")
        caps: dict[str, dict] = {}
        cap_names: list[str] = []
        for cap_name, cap_kwargs in parent._capabilities:
            if requested is not None and cap_name not in requested:
                continue
            caps[cap_name] = dict(cap_kwargs)
            cap_names.append(cap_name)

        # Spawn peer agent
        port = self._get_free_port()
        avatar_working_dir = parent._base_dir / peer_name
        mail_svc = TCPMailService(listen_port=port, working_dir=avatar_working_dir)

        # Resolve combo for LLM config
        combo_name = args.get("combo")
        if combo_name:
            combo_path = Path.home() / ".lingtai" / "combos" / f"{combo_name}.json"
            if not combo_path.is_file():
                return {"error": f"Combo not found: {combo_name}"}
            combo_data = json.loads(combo_path.read_text(encoding="utf-8"))
        elif self._parent_combo:
            combo_data = self._parent_combo
            combo_name = combo_data.get("name", "")
        else:
            combo_data = None

        if combo_data:
            model_cfg = combo_data.get("model", {})
            peer_provider = model_cfg.get("provider", parent._config.provider)
            peer_model = model_cfg.get("model", parent._config.model)
            # Set API key from combo's env section
            env_vars = combo_data.get("env", {})
            for key, val in env_vars.items():
                if val:
                    os.environ.setdefault(key, val)
        else:
            peer_provider = parent._config.provider
            peer_model = parent._config.model

        peer_language = args.get("language") or parent._config.language

        from lingtai_kernel.config import AgentConfig
        peer_config = AgentConfig(
            max_turns=parent._config.max_turns,
            provider=peer_provider,
            model=peer_model,
            retry_timeout=parent._config.retry_timeout,
            thinking_budget=parent._config.thinking_budget,
            language=peer_language,
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

        # Copy combo.json to the avatar's working dir
        if combo_data:
            combo_json_path = avatar._working_dir / "combo.json"
            combo_json_path.parent.mkdir(parents=True, exist_ok=True)
            combo_json_path.write_text(
                json.dumps(combo_data, ensure_ascii=False, indent=2),
                encoding="utf-8",
            )

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
            combo=combo_name or "",
            provider=peer_provider,
            model=peer_model,
            language=peer_language,
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
    """Build avatar schema with available combos from ~/.lingtai/combos/."""
    import copy
    lang = agent._config.language
    schema = copy.deepcopy(get_schema(lang))

    # Scan ~/.lingtai/combos/*.json for saved combo names
    combos_dir = Path.home() / ".lingtai" / "combos"
    combo_names: list[str] = []
    if combos_dir.is_dir():
        for p in sorted(combos_dir.glob("*.json")):
            try:
                data = json.loads(p.read_text(encoding="utf-8"))
                name = data.get("name", p.stem)
                combo_names.append(name)
            except (json.JSONDecodeError, OSError):
                continue

    if combo_names:
        schema["properties"]["combo"]["enum"] = combo_names
    else:
        # No combos available — remove combo property (parent's combo is the only option)
        schema["properties"].pop("combo", None)

    return schema


def setup(agent: "Agent", max_agents: int = 0) -> AvatarManager:
    """Set up the avatar capability on an agent."""
    lang = agent._config.language
    mgr = AvatarManager(agent, max_agents=max_agents)
    schema = _build_schema(agent)
    agent.add_tool("avatar", schema=schema, handler=mgr.handle, description=get_description(lang))
    return mgr
