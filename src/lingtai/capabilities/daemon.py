"""Daemon capability (神識) — dispatch ephemeral subagents (分神).

Gives an agent the ability to split its consciousness into focused worker
fragments that operate in parallel on the same working directory.  Each
emanation is a disposable ChatSession with a curated tool surface — not an
agent.  Results return as [daemon:em-N] notifications in the parent's inbox.

Usage:
    Agent(capabilities=["daemon"])
    Agent(capabilities={"daemon": {"max_emanations": 4}})
"""
from __future__ import annotations

import threading
import time
from concurrent.futures import ThreadPoolExecutor
from typing import TYPE_CHECKING

from ..i18n import t

if TYPE_CHECKING:
    from ..agent import Agent

from lingtai_kernel.llm.base import FunctionSchema, ToolCall
from lingtai_kernel.message import MSG_REQUEST, _make_message


# Tools emanations can never use (no recursion, no spawning, no identity mutation)
EMANATION_BLACKLIST = {"daemon", "avatar", "psyche", "library"}


def get_description(lang: str = "en") -> str:
    return t(lang, "daemon.description")


def get_schema(lang: str = "en") -> dict:
    return {
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["emanate", "list", "ask", "reclaim"],
                "description": t(lang, "daemon.action"),
            },
            "tasks": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {
                        "task": {"type": "string"},
                        "tools": {"type": "array", "items": {"type": "string"}},
                        "model": {"type": "string"},
                    },
                    "required": ["task", "tools"],
                },
                "description": t(lang, "daemon.tasks"),
            },
            "id": {
                "type": "string",
                "description": t(lang, "daemon.id"),
            },
            "message": {
                "type": "string",
                "description": t(lang, "daemon.message"),
            },
        },
        "required": ["action"],
    }


class DaemonManager:
    """Manages subagent (emanation) lifecycle."""

    def __init__(self, agent: "Agent", max_emanations: int = 4,
                 max_turns: int = 30, timeout: float = 300.0):
        self._agent = agent
        self._max_emanations = max_emanations
        self._max_turns = max_turns
        self._timeout = timeout
        self._default_model = agent.service.model

        # Emanation registry: em_id → entry dict
        self._emanations: dict[str, dict] = {}
        self._next_id = 1
        # Pool tracking for reclaim
        self._pools: list[tuple[ThreadPoolExecutor, threading.Event]] = []

    def handle(self, args: dict) -> dict:
        action = args.get("action")
        if action == "emanate":
            return self._handle_emanate(args.get("tasks", []))
        elif action == "list":
            return self._handle_list()
        elif action == "ask":
            return self._handle_ask(args.get("id", ""), args.get("message", ""))
        elif action == "reclaim":
            return self._handle_reclaim()
        else:
            return {"status": "error", "message": f"Unknown action: {action}"}

    # Placeholder implementations — filled in subsequent tasks
    def _handle_emanate(self, tasks):
        return {"status": "error", "message": "not yet implemented"}

    def _handle_list(self):
        return {"emanations": []}

    def _handle_ask(self, em_id, message):
        return {"status": "error", "message": "not yet implemented"}

    def _handle_reclaim(self):
        return {"status": "reclaimed", "cancelled": 0}


def setup(agent: "Agent", max_emanations: int = 4,
          max_turns: int = 30, timeout: float = 300.0) -> DaemonManager:
    """Set up the daemon capability on an agent."""
    lang = agent._config.language
    mgr = DaemonManager(agent, max_emanations=max_emanations,
                        max_turns=max_turns, timeout=timeout)
    schema = get_schema(lang)
    agent.add_tool("daemon", schema=schema, handler=mgr.handle,
                   description=get_description(lang))
    return mgr
