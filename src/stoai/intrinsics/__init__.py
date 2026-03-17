"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description
- handle: handler function(agent, args) -> dict
"""
from . import mail, clock, status, system

ALL_INTRINSICS = {
    "mail": {"schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handle": mail.handle},
    "clock": {"schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handle": clock.handle},
    "status": {"schema": status.SCHEMA, "description": status.DESCRIPTION, "handle": status.handle},
    "system": {"schema": system.SCHEMA, "description": system.DESCRIPTION, "handle": system.handle},
}
