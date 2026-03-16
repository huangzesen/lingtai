"""Clock intrinsic — time awareness and synchronization.

Actions:
    check — get current UTC time
    wait  — sleep for N seconds, or block until mail arrives (wakes early on mail)

The handler lives in BaseAgent (needs access to _mail_arrived event and _cancel_event).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["check", "wait"],
            "description": (
                "check: get the current UTC time. "
                "wait: pause execution. If seconds is given, waits up to that many seconds "
                "(wakes early if mail arrives). If seconds is omitted, blocks until mail arrives."
            ),
        },
        "seconds": {
            "type": "number",
            "description": (
                "Maximum seconds to wait (for action=wait). "
                "If omitted, waits indefinitely until mail arrives. "
                "Capped at 300."
            ),
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Time awareness and synchronization. "
    "'check' returns current UTC time. "
    "'wait' pauses execution — specify 'seconds' for a timed sleep, "
    "or omit it to block until incoming mail arrives. "
    "A timed wait also wakes early if mail arrives."
)
