"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description
- handle: handler function(agent, args) -> dict
"""
from . import mail, system, eigen

ALL_INTRINSICS = {
    "mail": {
        "schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handle": mail.handle,
        "system_prompt": "Send and receive messages. Check inbox, read, search, delete. Send to yourself to take persistent notes.",
    },
    "system": {
        "schema": system.SCHEMA, "description": system.DESCRIPTION, "handle": system.handle,
        "system_prompt": "Runtime, lifecycle, and synchronization. Inspect your state, sleep, shut down, or restart.",
    },
    "eigen": {
        "schema": eigen.SCHEMA, "description": eigen.DESCRIPTION, "handle": eigen.handle,
        "system_prompt": "Core self-management — working notes and context control.",
    },
}
