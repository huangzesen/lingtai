"""Mail intrinsic — point-to-point FIFO messaging.

Actions:
    send  — fire-and-forget message to an address
    check — count messages in the queue
    read  — pop and return the next message from the queue

The actual handlers live in BaseAgent (needs access to MailService and queue).
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
                "send: send a message (requires address, message; optional subject). "
                "check: count queued messages. "
                "read: pop and return the next message."
            ),
        },
        "address": {
            "type": "string",
            "description": "Target address for send (e.g. 127.0.0.1:8301)",
        },
        "subject": {"type": "string", "description": "Message subject (for send)"},
        "message": {"type": "string", "description": "Message body (for send)"},
        "attachments": {
            "type": "array",
            "items": {"type": "string"},
            "description": "List of file paths to attach to the message (for send)",
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Point-to-point messaging. Use 'send' to message another agent, "
    "'check' to see how many messages are queued, 'read' to pop the next message."
)
