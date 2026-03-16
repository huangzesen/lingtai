"""Status intrinsic — agent self-inspection and lifecycle.

Actions:
    show     — display agent identity, runtime, and resource usage
    shutdown — initiate graceful self-termination (use when you want to add more capabilities or tools, mail to admin request first and then shutdown)

The handler lives in BaseAgent (needs access to agent state).
This module provides the schema and description.
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
                "to add more capabilities or tools. Protocol: (1) mail your admin "
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
    "more capabilities or tools. Mail your admin first, then shutdown."
)
