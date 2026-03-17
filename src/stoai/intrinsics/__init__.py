"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description

All kernel intrinsics are implemented in BaseAgent because they need
access to agent state (services, working_dir, etc.).
"""
from . import mail, clock, status, system

ALL_INTRINSICS = {
    "mail": {"schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handler": None},
    "clock": {"schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handler": None},
    "status": {"schema": status.SCHEMA, "description": status.DESCRIPTION, "handler": None},
    "system": {"schema": system.SCHEMA, "description": system.DESCRIPTION, "handler": None},
}
