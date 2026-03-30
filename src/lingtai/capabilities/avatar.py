"""Avatar capability — spawn independent peer agents (分身).

Shallow (投胎): Copy init.json to a new working dir, strip name, launch.
    The avatar gets the same LLM config + capabilities but no identity,
    no memory, no history.  A fresh life.

Deep (二重身): Copy the entire working dir (system/, library/, exports/)
    plus init.json to a new dir, strip name + history, launch.
    The avatar is a doppelgänger — same character, memory, knowledge —
    but starts a fresh conversation.

Both modes launch `lingtai run <dir>` as a fully detached process.
The avatar is an independent life — it survives the parent's death.

Maintains an append-only ledger (delegates/ledger.jsonl) that records
every spawn event.

Usage:
    Agent(capabilities=["avatar"])
    # avatar(name="researcher")                    — shallow (投胎)
    # avatar(name="clone", type="deep")            — deep (二重身)
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path
from typing import TYPE_CHECKING

from ..i18n import t

if TYPE_CHECKING:
    from ..agent import Agent

PROVIDERS = {"providers": [], "default": "builtin"}

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
            "type": {
                "type": "string",
                "enum": ["shallow", "deep"],
                "description": t(lang, "avatar.type"),
            },
            "dir": {
                "type": "string",
                "description": t(lang, "avatar.dir"),
            },
            "comment": {
                "type": "string",
                "description": t(lang, "avatar.comment"),
            },
        },
        "required": ["name", "dir"],
    }



class AvatarManager:
    """Spawns avatar (分身) peer agents as detached processes.

    Each avatar gets its own working directory with init.json and is
    launched via `lingtai run`.  No in-process references — liveness
    is checked via the filesystem (handshake.is_alive).
    """

    def __init__(self, agent: "Agent"):
        self._agent = agent

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
        parent = self._agent
        reasoning = args.get("_reasoning")
        peer_name = args.get("name", "avatar")
        avatar_type = args.get("type", "shallow")

        if avatar_type not in ("shallow", "deep"):
            return {"error": "type must be 'shallow' or 'deep'"}

        # Check if this peer already exists and is live
        from lingtai_kernel.handshake import is_alive
        for record in self._read_ledger():
            if record.get("name") == peer_name:
                wd = record.get("working_dir", "")
                if wd and is_alive(wd):
                    return {
                        "status": "already_active",
                        "working_dir": wd,
                        "message": (
                            f"'{peer_name}' is already running. "
                            f"Use mail to communicate, or system intrinsic to manage lifecycle."
                        ),
                    }

        # Parent must have init.json
        parent_init_path = parent._working_dir / "init.json"
        if not parent_init_path.is_file():
            return {"error": "parent has no init.json — cannot spawn avatar"}

        try:
            parent_init = json.loads(parent_init_path.read_text())
        except (json.JSONDecodeError, OSError) as e:
            return {"error": f"failed to read parent init.json: {e}"}

        # Working dir: sibling of parent, named by caller
        dir_name = args.get("dir", peer_name)
        avatar_working_dir = parent._working_dir.parent / dir_name
        if avatar_working_dir.exists():
            return {"error": f"Directory '{dir_name}' already exists. Choose another name."}

        # Prepare the avatar's working directory
        if avatar_type == "deep":
            self._prepare_deep(parent._working_dir, avatar_working_dir)
        else:
            avatar_working_dir.mkdir(parents=True, exist_ok=True)

        # Resolve relative file paths to absolute so avatar can find them
        for key in ("env_file", "covenant_file", "comment_file", "soul_file"):
            val = parent_init.get(key)
            if val and not os.path.isabs(val):
                resolved = parent._working_dir / val
                if resolved.is_file():
                    parent_init[key] = str(resolved)

        # Inherit parent's venv_path so avatar can find the runtime
        if hasattr(parent, "_venv_path") and parent._venv_path:
            parent_init["venv_path"] = parent._venv_path

        # Write avatar's init.json (modified copy of parent's)
        avatar_comment = args.get("comment", "")
        avatar_init = self._make_avatar_init(parent_init, peer_name, reasoning or "", comment=avatar_comment)
        (avatar_working_dir / "init.json").write_text(
            json.dumps(avatar_init, indent=2, ensure_ascii=False)
        )

        # Clean stale signal files before launch
        for sig in (".suspend", ".sleep", ".interrupt"):
            sig_file = avatar_working_dir / sig
            if sig_file.is_file():
                sig_file.unlink(missing_ok=True)

        # Launch as detached process
        pid = self._launch(avatar_working_dir)

        # Record in ledger
        self._append_ledger(
            "avatar", peer_name,
            working_dir=str(avatar_working_dir),
            mission=reasoning or "",
            type=avatar_type,
            pid=pid,
        )

        return {
            "status": "ok",
            "working_dir": str(avatar_working_dir),
            "agent_name": peer_name,
            "type": avatar_type,
            "pid": pid,
        }

    # ------------------------------------------------------------------
    # Init.json construction
    # ------------------------------------------------------------------

    @staticmethod
    def _make_avatar_init(parent_init: dict, name: str, reasoning: str, comment: str = "") -> dict:
        """Build avatar's init.json from parent's, setting name and prompt."""
        init = json.loads(json.dumps(parent_init))  # deep copy
        init["manifest"]["agent_name"] = name
        init["prompt"] = reasoning
        # Avatar has no admin privileges
        init["manifest"]["admin"] = {}
        # Comment is not inherited — parent can set one explicitly for the avatar
        init["comment"] = comment
        init.pop("comment_file", None)
        # Addons (IMAP, Telegram) are not inherited — each agent must be
        # explicitly configured to avoid multiple agents polling the same account
        init.pop("addons", None)
        return init

    # ------------------------------------------------------------------
    # Deep copy — 二重身
    # ------------------------------------------------------------------

    @staticmethod
    def _prepare_deep(src: Path, dst: Path) -> None:
        """Copy identity + knowledge from parent, excluding runtime state."""
        dst.mkdir(parents=True, exist_ok=True)

        # system/ (character, memory, covenant, etc.)
        src_system = src / "system"
        if src_system.is_dir():
            dst_system = dst / "system"
            if dst_system.exists():
                shutil.rmtree(dst_system)
            shutil.copytree(src_system, dst_system)

        # library/
        src_lib = src / "library"
        if src_lib.is_dir():
            dst_lib = dst / "library"
            if dst_lib.exists():
                shutil.rmtree(dst_lib)
            shutil.copytree(src_lib, dst_lib)

        # exports/
        src_exports = src / "exports"
        if src_exports.is_dir():
            dst_exports = dst / "exports"
            if dst_exports.exists():
                shutil.rmtree(dst_exports)
            shutil.copytree(src_exports, dst_exports)

        # combo.json
        src_combo = src / "combo.json"
        if src_combo.is_file():
            shutil.copy2(src_combo, dst / "combo.json")

        # Explicitly do NOT copy: history/, mailbox/, delegates/,
        # .agent.json, .agent.heartbeat, logs/

    # ------------------------------------------------------------------
    # Process launch
    # ------------------------------------------------------------------

    @staticmethod
    def _launch(working_dir: Path) -> int:
        """Launch `lingtai run <dir>` as a fully detached process."""
        from lingtai.venv_resolve import resolve_venv, venv_python

        # Resolve Python from avatar's init.json → global runtime
        init_path = working_dir / "init.json"
        init_data = None
        if init_path.is_file():
            try:
                init_data = json.loads(init_path.read_text())
            except (ValueError, OSError):
                pass
        venv_dir = resolve_venv(init_data)
        python = venv_python(venv_dir)
        cmd = [python, "-m", "lingtai", "run", str(working_dir)]

        proc = subprocess.Popen(
            cmd,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        return proc.pid

    # ------------------------------------------------------------------
    # Ledger reading
    # ------------------------------------------------------------------

    def _read_ledger(self) -> list[dict]:
        """Read all ledger records."""
        if not self._ledger_path.is_file():
            return []
        records = []
        for line in self._ledger_path.read_text().splitlines():
            if line.strip():
                try:
                    records.append(json.loads(line))
                except json.JSONDecodeError:
                    continue
        return records


def setup(agent: "Agent", **kwargs) -> AvatarManager:
    """Set up the avatar capability on an agent."""
    lang = agent._config.language
    mgr = AvatarManager(agent)
    schema = get_schema(lang)
    agent.add_tool("avatar", schema=schema, handler=mgr.handle, description=get_description(lang))
    return mgr
