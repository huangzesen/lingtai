"""Mail intrinsic — point-to-point FIFO messaging.

Actions:
    send — fire-and-forget message to an address
    read — pop and return the next message from the queue

The actual handlers live in BaseAgent (needs access to MailService and queue).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["send", "read"],
            "description": (
                "send: send a message (requires address, message; optional subject). "
                "read: pop and return the next message (returns null if queue is empty)."
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
        "type": {
            "type": "string",
            "enum": ["normal", "cancel"],
            "description": (
                "Mail type (for send). 'normal' (default) is regular mail. "
                "'cancel' stops the target agent immediately (requires admin privilege)."
            ),
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Point-to-point FIFO messaging between agents. "
    "Messages are queued — 'read' pops the next one (first-in, first-out). "
    "For persistent mailbox with reply/CC/BCC, use the email capability instead. "
    "Etiquette: a short acknowledgement is fine, but do not reply to "
    "an acknowledgement — that creates pointless ping-pong."
)
