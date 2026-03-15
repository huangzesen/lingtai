"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description
- handle_* or a Manager class: the implementation

Some intrinsics (talk, vision, web_search) are implemented in BaseAgent
because they need access to agent state (connections, LLM service, etc.).
"""
from . import read, edit, write, glob, grep, talk, vision, web_search

ALL_INTRINSICS = {
    "read": {"schema": read.SCHEMA, "description": read.DESCRIPTION, "handler": read.handle_read},
    "edit": {"schema": edit.SCHEMA, "description": edit.DESCRIPTION, "handler": edit.handle_edit},
    "write": {"schema": write.SCHEMA, "description": write.DESCRIPTION, "handler": write.handle_write},
    "glob": {"schema": glob.SCHEMA, "description": glob.DESCRIPTION, "handler": glob.handle_glob},
    "grep": {"schema": grep.SCHEMA, "description": grep.DESCRIPTION, "handler": grep.handle_grep},
    "talk": {"schema": talk.SCHEMA, "description": talk.DESCRIPTION, "handler": None},
    "vision": {"schema": vision.SCHEMA, "description": vision.DESCRIPTION, "handler": None},
    "web_search": {"schema": web_search.SCHEMA, "description": web_search.DESCRIPTION, "handler": None},
}
