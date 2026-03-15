"""Email intrinsic — structured inter-agent messaging with inbox.

Actions:
    send    — fire-and-forget message to an address
    check   — list emails in inbox (total count, showing N most recent)
    read    — read a specific email by ID

The actual handlers live in BaseAgent (needs access to EmailService and mailbox).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["send", "check", "read"],
            "description": (
                "send: send an email (requires address, message; optional subject). "
                "check: list inbox (returns total, showing, and list of emails with id/from/subject/preview/time/unread; optional n). "
                "read: read full email by ID (requires email_id)."
            ),
        },
        "address": {"type": "string", "description": "Target address for send (e.g. 127.0.0.1:8301)"},
        "subject": {"type": "string", "description": "Email subject line (for send)"},
        "message": {"type": "string", "description": "Email body (for send)"},
        "email_id": {"type": "string", "description": "Email ID to read (for read, get ID from check)"},
        "n": {"type": "integer", "description": "Max recent emails to show (for check)", "default": 10},
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Email tool for inter-agent communication. "
    "Use 'send' to email another agent, 'check' to list your inbox "
    "(shows total count and recent emails), 'read' to read a specific email by ID."
)
