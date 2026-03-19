"""System intrinsic — runtime, lifecycle, and synchronization.

Actions:
    show     — display agent identity, runtime, and resource usage
    sleep    — pause execution; wakes on incoming message or timeout
    shutdown — initiate graceful self-termination
    restart  — stop, reload MCP servers and config from working dir, restart
"""
from __future__ import annotations

import time
from datetime import datetime, timezone

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["show", "sleep", "shutdown", "restart"],
            "description": (
                "show: display full agent self-inspection. Returns:\n"
                "- identity: agent_id, working_dir, mail_address (or null if no mail service)\n"
                "- runtime: started_at (UTC ISO), uptime_seconds\n"
                "- tokens.input_tokens, output_tokens, thinking_tokens, cached_tokens, "
                "total_tokens, api_calls: cumulative LLM usage since start\n"
                "- tokens.context.system_tokens, tools_tokens, history_tokens: "
                "current context window breakdown\n"
                "- tokens.context.window_size: total context window capacity\n"
                "- tokens.context.usage_pct: percentage of context window currently occupied\n"
                "Use this to monitor resource consumption, decide when to save "
                "important information to long-term memory, and identify yourself.\n\n"
                "sleep: pause execution. If seconds is given, waits up to that many seconds "
                "(wakes early if a message arrives). If seconds is omitted, blocks until "
                "a message arrives. Capped at 300 seconds.\n\n"
                "shutdown: initiate graceful self-termination. Use when you want "
                "to add more capabilities or tools. Protocol: (1) contact your admin "
                "explaining what capabilities/tools you need and why, (2) then call "
                "shutdown. A successor agent may resume from your working directory "
                "and conversation history.\n\n"
                "restart: the agent stops, reloads MCP servers and config "
                "from its working directory (mcp/servers.json), and restarts with "
                "a fresh session but the same identity. Use after installing new "
                "MCP tools to pick them up without requiring external re-delegation."
            ),
        },
        "seconds": {
            "type": "number",
            "description": (
                "For sleep: maximum seconds to wait. "
                "If omitted, waits indefinitely until a message arrives. "
                "Capped at 300."
            ),
        },
        "reason": {
            "type": "string",
            "description": "Reason for shutdown or restart (logged to event log).",
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Runtime, lifecycle, and synchronization. "
    "'show' returns identity, runtime, and resource usage. "
    "'sleep' pauses execution — specify 'seconds' for a timed sleep, "
    "or omit it to block until an incoming message arrives. "
    "'shutdown' initiates graceful self-termination. "
    "'restart' triggers rebirth — reloads MCP servers from working dir and restarts."
)


def handle(agent, args: dict) -> dict:
    """Handle system tool — runtime, lifecycle, synchronization."""
    action = args.get("action", "show")
    handler = {
        "show": _show,
        "sleep": _sleep,
        "shutdown": _shutdown,
        "restart": _restart,
    }.get(action)
    if handler is None:
        return {"status": "error", "message": f"Unknown system action: {action}"}
    return handler(agent, args)


# ---------------------------------------------------------------------------
# show
# ---------------------------------------------------------------------------

def _show(agent, args: dict) -> dict:
    mail_addr = None
    if agent._mail_service is not None and agent._mail_service.address:
        mail_addr = agent._mail_service.address

    uptime = time.monotonic() - agent._uptime_anchor if agent._uptime_anchor is not None else 0.0

    usage = agent.get_token_usage()

    if agent._chat is not None:
        try:
            window_size = agent._chat.context_window()
            ctx_total = usage["ctx_total_tokens"]
            usage_pct = round(ctx_total / window_size * 100, 1) if window_size else 0.0
        except Exception:
            window_size = None
            usage_pct = None
    else:
        window_size = None
        usage_pct = None

    return {
        "status": "ok",
        "identity": {
            "agent_id": agent.agent_id,
            "agent_name": agent.agent_name,
            "working_dir": str(agent._working_dir),
            "mail_address": mail_addr,
        },
        "runtime": {
            "current_time": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "started_at": agent._started_at,
            "uptime_seconds": round(uptime, 1),
        },
        "tokens": {
            "input_tokens": usage["input_tokens"],
            "output_tokens": usage["output_tokens"],
            "thinking_tokens": usage["thinking_tokens"],
            "cached_tokens": usage["cached_tokens"],
            "total_tokens": usage["total_tokens"],
            "api_calls": usage["api_calls"],
            "context": {
                "system_tokens": usage["ctx_system_tokens"],
                "tools_tokens": usage["ctx_tools_tokens"],
                "history_tokens": usage["ctx_history_tokens"],
                "total_tokens": usage["ctx_total_tokens"],
                "window_size": window_size,
                "usage_pct": usage_pct,
            },
        },
    }


# ---------------------------------------------------------------------------
# sleep (ported from clock.wait)
# ---------------------------------------------------------------------------

def _sleep(agent, args: dict) -> dict:
    max_wait = 300
    seconds = args.get("seconds")
    if seconds is not None:
        seconds = float(seconds)
        if seconds < 0:
            return {"status": "error", "message": "seconds must be non-negative"}
        seconds = min(seconds, max_wait)

    agent._log("system_sleep_start", seconds=seconds)

    if agent._cancel_event.is_set():
        agent._log("system_sleep_end", reason="silenced", waited=0.0)
        return {"status": "ok", "reason": "silenced", "waited": 0.0}
    if agent._mail_arrived.is_set():
        agent._log("system_sleep_end", reason="mail_arrived", waited=0.0)
        return {"status": "ok", "reason": "mail_arrived", "waited": 0.0}

    agent._mail_arrived.clear()

    poll_interval = 0.5
    t0 = time.monotonic()

    while True:
        waited = time.monotonic() - t0

        if agent._cancel_event.is_set():
            agent._log("system_sleep_end", reason="silenced", waited=waited)
            return {"status": "ok", "reason": "silenced", "waited": waited}

        if agent._mail_arrived.is_set():
            agent._log("system_sleep_end", reason="mail_arrived", waited=waited)
            return {"status": "ok", "reason": "mail_arrived", "waited": waited}

        if seconds is not None and waited >= seconds:
            agent._log("system_sleep_end", reason="timeout", waited=waited)
            return {"status": "ok", "reason": "timeout", "waited": waited}

        if seconds is not None:
            remaining = seconds - waited
            sleep_time = min(poll_interval, remaining)
        else:
            sleep_time = poll_interval

        agent._mail_arrived.wait(timeout=sleep_time)


# ---------------------------------------------------------------------------
# shutdown
# ---------------------------------------------------------------------------

def _shutdown(agent, args: dict) -> dict:
    reason = args.get("reason", "")
    agent._log("shutdown_requested", reason=reason)
    agent._shutdown.set()
    return {
        "status": "ok",
        "message": "Shutdown initiated. A successor agent may resume from your working directory and conversation history.",
    }


# ---------------------------------------------------------------------------
# restart (formerly nirvana)
# ---------------------------------------------------------------------------

def _restart(agent, args: dict) -> dict:
    reason = args.get("reason", "")
    agent._log("restart_requested", reason=reason)
    agent._restart_requested = True
    agent._shutdown.set()
    return {
        "status": "ok",
        "message": "Restart initiated — rebirth in progress. "
                   "You will be reborn with the same identity but fresh tools. "
                   "Any new MCP servers in mcp/servers.json will be loaded.",
    }
