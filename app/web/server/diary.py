"""JSONL diary parser — reads agent event logs from disk."""
from __future__ import annotations

import json
from pathlib import Path


def parse_diary(log_file: Path, since: float = 0.0) -> list[dict]:
    """Read events.jsonl and return parsed diary entries.

    Args:
        log_file: Path to the agent's events.jsonl file.
        since: Only return entries with ts > since (0.0 = all).

    Returns:
        List of diary entry dicts with normalized type and fields.
    """
    entries: list[dict] = []
    if not log_file.exists():
        return entries

    with open(log_file, "r") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                e = json.loads(line)
            except (json.JSONDecodeError, ValueError):
                continue

            ts = e.get("ts", 0)
            if ts <= since:
                continue

            etype = e.get("type", "")
            entry = _map_event(etype, e, ts)
            if entry:
                entries.append(entry)

    return entries


def _map_event(etype: str, e: dict, ts: float) -> dict | None:
    """Map a JSONL event to the frontend diary entry format."""
    if etype == "diary":
        return {"type": "diary", "time": ts, "text": e.get("text", "")}

    if etype == "thinking":
        return {"type": "thinking", "time": ts, "text": e.get("text", "")}

    if etype == "tool_call":
        return {
            "type": "tool_call", "time": ts,
            "tool": e.get("tool_name", ""),
            "args": e.get("tool_args", {}),
        }

    if etype == "tool_reasoning":
        return {
            "type": "reasoning", "time": ts,
            "tool": e.get("tool", ""),
            "text": e.get("reasoning", ""),
        }

    if etype == "tool_result":
        return {
            "type": "tool_result", "time": ts,
            "tool": e.get("tool_name", ""),
            "status": e.get("status", ""),
        }

    if etype == "mail_sent":
        return {
            "type": "email_out", "time": ts,
            "to": e.get("address", ""),
            "subject": e.get("subject", ""),
            "message": e.get("message", ""),
        }

    if etype == "email_sent":
        to = e.get("to", [])
        if isinstance(to, list):
            to = ", ".join(to)
        return {
            "type": "email_out", "time": ts,
            "to": to,
            "subject": e.get("subject", ""),
            "message": e.get("message", ""),
        }

    if etype in ("mail_received", "email_received"):
        return {
            "type": "email_in", "time": ts,
            "from": e.get("sender", ""),
            "subject": e.get("subject", ""),
            "message": e.get("message", ""),
        }

    if etype == "cancel_received":
        return {
            "type": "cancel_received", "time": ts,
            "from": e.get("sender", ""),
            "subject": e.get("subject", ""),
        }

    if etype == "cancel_diary":
        return {"type": "cancel_diary", "time": ts, "text": e.get("text", "")}

    # Unknown event — include as raw JSON for debugging
    if etype in ("agent_state", "error", "shutdown_requested"):
        return {
            "type": "unknown", "time": ts,
            "text": json.dumps(e, default=str),
        }

    # Skip noisy internal events (llm_call, llm_response, compaction, etc.)
    return None
