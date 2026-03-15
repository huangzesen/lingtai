"""Talk intrinsic — inter-agent messaging.
The actual send logic lives in BaseAgent (it needs access to the connections dict
and the target agent's inbox). This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {"type": "string", "enum": ["send", "send_and_wait"]},
        "target_id": {"type": "string", "description": "ID of the target agent"},
        "message": {"type": "string", "description": "Message to send"},
        "timeout": {"type": "number", "description": "Timeout in seconds (for send_and_wait)", "default": 120},
    },
    "required": ["action", "target_id", "message"],
}
DESCRIPTION = "Send a message to another connected agent."
