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

from lingtai_kernel.llm.base import FunctionSchema
from lingtai_kernel.message import MSG_REQUEST, _make_message

PROVIDERS = {"providers": [], "default": "builtin"}

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

    # Minimum text length to trigger a parent notification.
    # Short results (e.g. "[cancelled]") are suppressed to avoid notification storms.
    _NOTIFY_MIN_LEN = 20

    def __init__(self, agent: "Agent", max_emanations: int = 2,
                 max_turns: int = 30, timeout: float = 300.0,
                 notify_threshold: int = 20, max_result_chars: int = 2000):
        self._agent = agent
        self._max_emanations = max_emanations
        self._max_turns = max_turns
        self._timeout = timeout
        self._default_model = agent.service.model
        self._notify_threshold = notify_threshold
        self._max_result_chars = max_result_chars

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

    def _build_tool_surface(self, requested: list[str]) -> tuple[list[FunctionSchema], dict]:
        """Build filtered tool schemas and dispatch map for an emanation."""
        from ..capabilities import _GROUPS

        # Expand groups and filter blacklist
        tool_names: set[str] = set()
        for name in requested:
            if name in EMANATION_BLACKLIST:
                continue
            if name in _GROUPS:
                tool_names.update(_GROUPS[name])
            else:
                tool_names.add(name)

        # Identify MCP tools (all non-capability, non-blacklisted)
        capability_names = {cap_name for cap_name, _ in self._agent._capabilities}
        all_registered = {s.name for s in self._agent._tool_schemas}
        mcp_names = all_registered - capability_names - EMANATION_BLACKLIST
        tool_names |= mcp_names

        # Validate requested tools exist
        available = {s.name for s in self._agent._tool_schemas}
        missing = tool_names - available
        if missing:
            raise ValueError(f"Unknown tools for emanation: {missing}")

        # Build schemas and dispatch
        schema_map = {s.name: s for s in self._agent._tool_schemas}
        schemas = [schema_map[n] for n in sorted(tool_names) if n in schema_map]
        dispatch = {n: self._agent._tool_handlers[n]
                    for n in tool_names if n in self._agent._tool_handlers}
        return schemas, dispatch

    def _build_emanation_prompt(self, task: str, schemas: list[FunctionSchema]) -> str:
        """Build the system prompt for an emanation."""
        lines = [
            "You are a daemon emanation (分神) — a focused subagent dispatched by an agent.",
            "You have one task. Complete it, then provide your final report as text.",
            "Your intermediate text output will be seen by the main agent — treat it as a progress report.",
            'When you are done, explicitly state "task done" and summarize what you accomplished.',
            "",
            "You work in the agent's working directory. Other subagents may be working",
            "concurrently on different tasks in the same directory. Do not modify files",
            "outside your assigned scope.",
        ]

        # Tool descriptions
        tool_lines = []
        for s in schemas:
            if s.description:
                tool_lines.append(f"### {s.name}\n{s.description}")
        if tool_lines:
            lines.append("")
            lines.append("## tools")
            lines.extend(tool_lines)

        lines.append("")
        lines.append("Your task:")
        lines.append(task)

        return "\n".join(lines)

    def _run_emanation(self, em_id: str, task: str, tool_names: list[str],
                       model: str | None, cancel_event: threading.Event) -> str:
        """Run a single emanation's tool loop. Called in a worker thread."""
        schemas, dispatch = self._build_tool_surface(tool_names)
        system_prompt = self._build_emanation_prompt(task, schemas)

        if cancel_event.is_set():
            return "[cancelled]"

        session = self._agent.service.create_session(
            system_prompt=system_prompt,
            tools=schemas or None,
            model=model or self._default_model,
            thinking="default",
            tracked=False,
        )

        response = session.send(task)
        turns = 0
        while response.tool_calls and turns < self._max_turns:
            if cancel_event.is_set():
                return "[cancelled]"

            # Intermediate text → notify parent
            if response.text:
                self._notify_parent(em_id, response.text)

            tool_results = []
            for tc in response.tool_calls:
                handler = dispatch.get(tc.name)
                if handler is None:
                    result = {"status": "error", "message": f"Unknown tool: {tc.name}"}
                else:
                    try:
                        result = handler(tc.args or {})
                    except Exception as e:
                        result = {"status": "error", "message": str(e)}
                tool_results.append(
                    self._agent.service.make_tool_result(
                        tc.name, result, tool_call_id=tc.id,
                    )
                )

            response = session.send(tool_results)
            turns += 1

            # Inject follow-up as a separate user message
            followup = self._drain_followup(em_id)
            if followup:
                response = session.send(followup)
                turns += 1

        return response.text or "[no output]"

    def _notify_parent(self, em_id: str, text: str) -> None:
        """Send a [daemon] notification to parent's inbox."""
        notification = f"[daemon:{em_id}]\n\n{text}"
        msg = _make_message(MSG_REQUEST, "daemon", notification)
        self._agent.inbox.put(msg)

    def _drain_followup(self, em_id: str) -> str | None:
        """Drain the follow-up buffer for a specific emanation."""
        entry = self._emanations.get(em_id)
        if not entry:
            return None
        with entry["followup_lock"]:
            text = entry["followup_buffer"]
            entry["followup_buffer"] = ""
        return text or None

    def _handle_emanate(self, tasks: list[dict]) -> dict:
        if not tasks:
            return {"status": "error", "message": "No tasks provided"}

        # Clear completed emanations and stale pools
        self._emanations = {k: v for k, v in self._emanations.items()
                            if not v["future"].done()}
        self._pools = [(p, c) for p, c in self._pools if not c.is_set()]

        # Capacity check against pruned registry
        running = len(self._emanations)
        if running + len(tasks) > self._max_emanations:
            lang = self._agent._config.language
            return {"status": "error",
                    "message": t(lang, "daemon.limit_reached",
                                 running=running, requested=len(tasks),
                                 max=self._max_emanations)}

        cancel_event = threading.Event()
        pool = ThreadPoolExecutor(max_workers=len(tasks))
        self._pools.append((pool, cancel_event))

        ids = []
        for spec in tasks:
            em_id = f"em-{self._next_id}"
            self._next_id += 1
            ids.append(em_id)

            future = pool.submit(
                self._run_emanation,
                em_id, spec["task"], spec["tools"],
                spec.get("model"), cancel_event,
            )
            future.add_done_callback(
                lambda f, eid=em_id, task=spec["task"]:
                    self._on_emanation_done(eid, task, f)
            )
            self._emanations[em_id] = {
                "future": future,
                "task": spec["task"],
                "start_time": time.time(),
                "cancel_event": cancel_event,
                "followup_buffer": "",
                "followup_lock": threading.Lock(),
            }

        # Start watchdog
        watchdog = threading.Thread(
            target=self._watchdog, args=(cancel_event, self._timeout),
            daemon=True,
        )
        watchdog.start()

        self._log("daemon_emanate", ids=ids, count=len(tasks),
                  tasks=[{"task": s["task"][:80], "tools": s["tools"]} for s in tasks])

        return {"status": "dispatched", "count": len(tasks), "ids": ids}

    def _handle_list(self) -> dict:
        emanations = []
        for em_id, entry in self._emanations.items():
            elapsed = time.time() - entry["start_time"]
            future = entry["future"]
            if future.done():
                exc = future.exception()
                if exc:
                    status = "failed"
                else:
                    status = "done"
            else:
                status = "running"
            info = {"id": em_id, "task": entry["task"][:80],
                    "status": status, "elapsed_s": round(elapsed)}
            if status == "failed" and exc:
                info["error"] = str(exc)
            emanations.append(info)
        return {"emanations": emanations}

    def _handle_ask(self, em_id: str, message: str) -> dict:
        entry = self._emanations.get(em_id)
        if not entry:
            return {"status": "error", "message": f"Unknown emanation: {em_id}"}
        if entry["future"].done():
            return {"status": "error", "message": "not running"}
        with entry["followup_lock"]:
            if entry["followup_buffer"]:
                entry["followup_buffer"] += "\n\n" + message
            else:
                entry["followup_buffer"] = message
        self._log("daemon_ask", em_id=em_id, message_length=len(message))
        return {"status": "sent", "id": em_id}

    def _handle_reclaim(self) -> dict:
        cancelled = sum(1 for e in self._emanations.values()
                        if not e["future"].done())
        for pool, cancel in self._pools:
            cancel.set()
            pool.shutdown(wait=False, cancel_futures=True)
        self._pools.clear()
        self._emanations.clear()
        self._log("daemon_reclaim", cancelled_count=cancelled)
        return {"status": "reclaimed", "cancelled": cancelled}

    def _on_emanation_done(self, em_id: str, task_summary: str, future) -> None:
        elapsed = 0.0
        entry = self._emanations.get(em_id)
        if entry:
            elapsed = time.time() - entry["start_time"]
        try:
            text = future.result()
            self._log("daemon_result", em_id=em_id, status="done",
                      text_length=len(text), elapsed_ms=round(elapsed * 1000))
        except Exception as e:
            text = f"Failed: {e}"
            self._log("daemon_error", em_id=em_id,
                      exception=type(e).__name__, exception_message=str(e))

        # Truncate long results
        if len(text) > self._max_result_chars:
            text = text[:self._max_result_chars] + f"\n[truncated — {len(text)} chars total]"

        # Suppress notifications for short results to prevent notification storms
        if len(text) < self._notify_threshold:
            self._log("daemon_result", em_id=em_id, status="suppressed_short",
                      text_length=len(text))
        else:
            self._notify_parent(em_id, text)

    def _watchdog(self, cancel_event: threading.Event, timeout: float) -> None:
        """Kill emanations that exceed the timeout."""
        deadline = time.time() + timeout
        while time.time() < deadline:
            if cancel_event.is_set():
                return
            time.sleep(1.0)
        cancel_event.set()

    def _log(self, event_type: str, **fields) -> None:
        """Log through the parent agent's logging system."""
        if hasattr(self._agent, '_log'):
            self._agent._log(event_type, **fields)


def setup(agent: "Agent", max_emanations: int = 2,
          max_turns: int = 30, timeout: float = 300.0,
          notify_threshold: int = 20, max_result_chars: int = 2000) -> DaemonManager:
    """Set up the daemon capability on an agent."""
    lang = agent._config.language
    mgr = DaemonManager(agent, max_emanations=max_emanations,
                        max_turns=max_turns, timeout=timeout,
                        notify_threshold=notify_threshold,
                        max_result_chars=max_result_chars)
    schema = get_schema(lang)
    agent.add_tool("daemon", schema=schema, handler=mgr.handle,
                   description=get_description(lang))
    return mgr
