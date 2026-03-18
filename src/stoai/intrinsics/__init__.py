"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description
- handle: handler function(agent, args) -> dict
"""
from . import mail, clock, status, memory

ALL_INTRINSICS = {
    "mail": {
        "schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handle": mail.handle,
        "system_prompt": "Send and receive messages to other agents and users.",
    },
    "clock": {
        "schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handle": clock.handle,
        "system_prompt": "Pause execution and wait for messages or timeouts.",
    },
    "status": {
        "schema": status.SCHEMA, "description": status.DESCRIPTION, "handle": status.handle,
        "system_prompt": "Inspect your own state, token usage, and shut yourself down.",
    },
    "memory": {
        "schema": memory.SCHEMA, "description": memory.DESCRIPTION, "handle": memory.handle,
        "system_prompt": "Edit and load your long-term memory.",
    },
}
