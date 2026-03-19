"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description
- handle: handler function(agent, args) -> dict
"""
from . import mail, clock, status, eigen

ALL_INTRINSICS = {
    "mail": {
        "schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handle": mail.handle,
        "system_prompt": "Send and receive messages. Check inbox, read, search, delete. Send to yourself to take persistent notes.",
    },
    "clock": {
        "schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handle": clock.handle,
        "system_prompt": "Pause execution and wait for messages or timeouts.",
    },
    "status": {
        "schema": status.SCHEMA, "description": status.DESCRIPTION, "handle": status.handle,
        "system_prompt": "Inspect your own state, token usage, and shut yourself down.",
    },
    "eigen": {
        "schema": eigen.SCHEMA, "description": eigen.DESCRIPTION, "handle": eigen.handle,
        "system_prompt": "Core self-management — working notes and context control.",
    },
}
