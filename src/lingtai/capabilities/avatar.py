"""Avatar capability — spawn peer agents with filesystem mail.

Maintains an append-only ledger (delegates/ledger.jsonl) that records every
spawn event.  Each line is a timestamped record of what was spawned,
to whom, with what mission, privileges, and capabilities.  The ledger is
never mutated — only appended to.  It forms a responsibility map that the
parent can consult before spawning again.

Lifecycle management (interrupt, sleep/lull, cpr, nirvana) is handled by
the system intrinsic's karma/nirvana actions, not here.  The avatar
tool's only job is to spawn avatars (分身).

Usage:
    Agent(capabilities=["avatar"])
    # avatar(name="researcher")           — spawn a blank avatar
    # avatar(name="clone", mirror=True)   — spawn a deep copy of self
"""
from __future__ import annotations

import json
import shutil
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
            "mirror": {
                "type": "boolean",
                "description": t(lang, "avatar.mirror"),
            },
        },
        "required": ["name"],
    }


# Backward compat
SCHEMA = get_schema("en")
DESCRIPTION = get_description("en")


class AvatarManager:
    """Spawns avatar (分身) peer agents with filesystem mail.

    Keeps an in-memory reference table for live status checks and an
    append-only JSONL ledger on disk that records every spawn.
    """

    def __init__(self, agent: "Agent", max_agents: int = 0):
        self._agent = agent
        self._max_agents = max_agents  # 0 = unlimited
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
    # Core spawn
    # ------------------------------------------------------------------

    def _spawn(self, args: dict) -> dict:
        from ..agent import Agent
        from lingtai_kernel.services.mail import FilesystemMailService

        parent = self._agent
        reasoning = args.get("_reasoning")
        peer_name = args.get("name", "avatar")
        mirror = args.get("mirror", False)

        # Check if this peer already exists and is live
        existing = self._peers.get(peer_name)
        if existing is not None:
            from lingtai_kernel.handshake import is_alive
            if is_alive(str(existing.working_dir)):
                return {
                    "status": "already_active",
                    "address": existing._mail_service.address if existing._mail_service else None,
                    "working_dir": str(existing._working_dir),
                    "agent_name": existing.agent_name,
                    "message": (
                        f"'{peer_name}' is already running. "
                        f"Use mail to communicate, or system intrinsic to manage lifecycle."
                    ),
                }
            # Not alive — clean up stale reference
            self._peers.pop(peer_name, None)

        # Agent count guard
        if self._max_agents > 0:
            base_dir = parent._working_dir.parent
            live = len(list(base_dir.glob("*/.agent.json")))
            if live >= self._max_agents:
                lang = parent._config.language
                return {"error": t(lang, "avatar.limit_reached", live=live, max=self._max_agents)}

        # Always inherit parent's covenant
        covenant = parent._prompt_manager.read_section("covenant") or ""

        # All capabilities inherited from parent
        caps: dict[str, dict] = {}
        cap_names: list[str] = []
        for cap_name, cap_kwargs in parent._capabilities:
            caps[cap_name] = dict(cap_kwargs)
            cap_names.append(cap_name)

        # Working dir: sibling of parent
        import secrets
        avatar_id = secrets.token_hex(3)
        avatar_working_dir = parent._working_dir.parent / avatar_id
        mail_svc = FilesystemMailService(working_dir=avatar_working_dir)

        # Inherit parent's LLM config
        from lingtai_kernel.config import AgentConfig
        peer_config = AgentConfig(
            max_turns=parent._config.max_turns,
            provider=parent._config.provider,
            model=parent._config.model,
            retry_timeout=parent._config.retry_timeout,
            thinking_budget=parent._config.thinking_budget,
            language=parent._config.language,
        )

        avatar = Agent(
            agent_name=peer_name,
            service=parent.service,
            mail_service=mail_svc,
            config=peer_config,
            working_dir=avatar_working_dir,
            streaming=parent._streaming,
            covenant=covenant,
            capabilities=caps,
            admin={},
        )

        # Mirror: copy identity files from parent before start
        if mirror:
            self._copy_identity(parent._working_dir, avatar._working_dir)

        avatar.start()

        # Copy combo.json if parent has one
        combo_path = parent._working_dir / "combo.json"
        if combo_path.is_file():
            shutil.copy2(combo_path, avatar._working_dir / "combo.json")

        if reasoning:
            avatar.send(reasoning, sender=str(parent._working_dir))

        # Record
        self._peers[peer_name] = avatar
        address = mail_svc.address
        self._append_ledger(
            "avatar", peer_name,
            address=address,
            working_dir=str(avatar._working_dir),
            mission=reasoning or "",
            capabilities=cap_names,
            mirror=mirror,
            provider=parent._config.provider,
            model=parent._config.model,
            language=parent._config.language,
        )

        return {
            "status": "ok",
            "address": address,
            "agent_name": avatar.agent_name,
        }

    # ------------------------------------------------------------------
    # Mirror — deep copy identity files
    # ------------------------------------------------------------------

    @staticmethod
    def _copy_identity(src: Path, dst: Path) -> None:
        """Copy identity files from parent to avatar working directory."""
        # system/character.md
        src_char = src / "system" / "character.md"
        if src_char.is_file():
            dst_char = dst / "system" / "character.md"
            dst_char.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src_char, dst_char)

        # system/memory.md
        src_mem = src / "system" / "memory.md"
        if src_mem.is_file():
            dst_mem = dst / "system" / "memory.md"
            dst_mem.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src_mem, dst_mem)

        # library/library.json
        src_lib = src / "library" / "library.json"
        if src_lib.is_file():
            dst_lib = dst / "library" / "library.json"
            dst_lib.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src_lib, dst_lib)

        # exports/ directory
        src_exports = src / "exports"
        if src_exports.is_dir():
            dst_exports = dst / "exports"
            if dst_exports.exists():
                shutil.rmtree(dst_exports)
            shutil.copytree(src_exports, dst_exports)


def setup(agent: "Agent", max_agents: int = 0) -> AvatarManager:
    """Set up the avatar capability on an agent."""
    lang = agent._config.language
    mgr = AvatarManager(agent, max_agents=max_agents)
    schema = get_schema(lang)
    agent.add_tool("avatar", schema=schema, handler=mgr.handle, description=get_description(lang))
    return mgr
