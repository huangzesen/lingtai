"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description
- handle_* or a Manager class: the implementation

Some intrinsics (mail) are implemented in BaseAgent because they need
access to agent state (services, etc.).
"""
from . import read, edit, write, glob, grep, mail, clock, status, memory

ALL_INTRINSICS = {
    "read": {"schema": read.SCHEMA, "description": read.DESCRIPTION, "handler": read.handle_read},
    "edit": {"schema": edit.SCHEMA, "description": edit.DESCRIPTION, "handler": edit.handle_edit},
    "write": {"schema": write.SCHEMA, "description": write.DESCRIPTION, "handler": write.handle_write},
    "glob": {"schema": glob.SCHEMA, "description": glob.DESCRIPTION, "handler": glob.handle_glob},
    "grep": {"schema": grep.SCHEMA, "description": grep.DESCRIPTION, "handler": grep.handle_grep},
    "mail": {"schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handler": None},
    "clock": {"schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handler": None},
    "status": {"schema": status.SCHEMA, "description": status.DESCRIPTION, "handler": None},
    "memory": {"schema": memory.SCHEMA, "description": memory.DESCRIPTION, "handler": None},
}
