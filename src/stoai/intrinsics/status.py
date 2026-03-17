"""Status intrinsic — agent self-inspection and lifecycle.

Actions:
    show     — display agent identity, runtime, and resource usage
    shutdown — initiate graceful self-termination (use when you want to add more capabilities or tools, contact admin first and then shutdown)
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["show", "shutdown"],
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
                "shutdown: initiate graceful self-termination. Use when you want "
                "to add more capabilities or tools. Protocol: (1) contact your admin "
                "explaining what capabilities/tools you need and why, (2) then call "
                "shutdown. A successor agent may resume from your working directory "
                "and conversation history."
            ),
        },
        "reason": {
            "type": "string",
            "description": "Reason for shutdown (only used with action='shutdown'). Logged to event log and visible in conversation history for successor agents.",
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Agent self-inspection and lifecycle. "
    "'show' returns identity, runtime, and resource usage. "
    "'shutdown' initiates graceful self-termination — use when you want "
    "more capabilities or tools. Contact your admin first, then shutdown."
)

import time


def handle(agent, args: dict) -> dict:
    """Handle status tool — agent self-inspection and lifecycle."""
    action = args.get("action", "show")
    if action == "show":
        return _show(agent)
    elif action == "shutdown":
        return _shutdown(agent, args)
    else:
        return {"status": "error", "message": f"Unknown status action: {action}"}


def _shutdown(agent, args: dict) -> dict:
    reason = args.get("reason", "")
    agent._log("shutdown_requested", reason=reason)
    agent._shutdown.set()
    return {
        "status": "ok",
        "message": "Shutdown initiated. A successor agent may resume from your working directory and conversation history.",
    }


def _show(agent) -> dict:
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
            "working_dir": str(agent._working_dir),
            "mail_address": mail_addr,
        },
        "runtime": {
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
