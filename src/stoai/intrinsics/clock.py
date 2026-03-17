"""Clock intrinsic — time awareness and synchronization.

Actions:
    check — get current UTC time
    wait  — sleep for N seconds, or block until a message arrives (wakes early on incoming message)
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
                "(wakes early if a message arrives). If seconds is omitted, blocks until a message arrives."
            ),
        },
        "seconds": {
            "type": "number",
            "description": (
                "Maximum seconds to wait (for action=wait). "
                "If omitted, waits indefinitely until a message arrives. "
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
    "or omit it to block until an incoming message arrives. "
    "A timed wait also wakes early if a message arrives."
)

import time


def handle(agent, args: dict) -> dict:
    """Handle clock tool — time check and wait/sync."""
    action = args.get("action", "check")
    if action == "check":
        return _check(agent)
    elif action == "wait":
        return _wait(agent, args)
    else:
        return {"status": "error", "message": f"Unknown clock action: {action}"}


def _check(agent) -> dict:
    from datetime import datetime, timezone
    now = datetime.now(timezone.utc)
    return {
        "status": "ok",
        "utc": now.strftime("%Y-%m-%dT%H:%M:%SZ"),
        "unix": now.timestamp(),
    }


def _wait(agent, args: dict) -> dict:
    max_wait = 300
    seconds = args.get("seconds")
    if seconds is not None:
        seconds = float(seconds)
        if seconds < 0:
            return {"status": "error", "message": "seconds must be non-negative"}
        seconds = min(seconds, max_wait)

    agent._log("clock_wait_start", seconds=seconds)

    if agent._cancel_event.is_set():
        agent._log("clock_wait_end", reason="cancelled", waited=0.0)
        return {"status": "ok", "reason": "cancelled", "waited": 0.0}
    if agent._mail_arrived.is_set():
        agent._log("clock_wait_end", reason="mail_arrived", waited=0.0)
        return {"status": "ok", "reason": "mail_arrived", "waited": 0.0}

    agent._mail_arrived.clear()

    poll_interval = 0.5
    t0 = time.monotonic()

    while True:
        waited = time.monotonic() - t0

        if agent._cancel_event.is_set():
            agent._log("clock_wait_end", reason="cancelled", waited=waited)
            return {"status": "ok", "reason": "cancelled", "waited": waited}

        if agent._mail_arrived.is_set():
            agent._log("clock_wait_end", reason="mail_arrived", waited=waited)
            return {"status": "ok", "reason": "mail_arrived", "waited": waited}

        if seconds is not None and waited >= seconds:
            agent._log("clock_wait_end", reason="timeout", waited=waited)
            return {"status": "ok", "reason": "timeout", "waited": waited}

        if seconds is not None:
            remaining = seconds - waited
            sleep_time = min(poll_interval, remaining)
        else:
            sleep_time = poll_interval

        agent._mail_arrived.wait(timeout=sleep_time)
