"""Mail intrinsic — point-to-point FIFO messaging.

Actions:
    send — fire-and-forget message to an address
    read — pop and return the next message from the queue
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

from pathlib import Path


def handle(agent, args: dict) -> dict:
    """Handle mail tool — FIFO send and read."""
    action = args.get("action", "send")
    if action == "send":
        return _send(agent, args)
    elif action == "read":
        return _read(agent, args)
    else:
        return {"status": "error", "message": f"Unknown mail action: {action}"}


def _send(agent, args: dict) -> dict:
    address = args.get("address", "")
    subject = args.get("subject", "")
    message_text = args.get("message", "")
    mail_type = args.get("type", "normal")

    if mail_type != "normal" and not agent._admin:
        return {"status": "error", "message": f"Not authorized to send type={mail_type!r} mail (requires admin=True)"}

    if not address:
        return {"status": "error", "message": "address is required"}
    if agent._mail_service is None:
        return {"status": "error", "message": "mail service not configured"}

    payload = {
        "from": agent._mail_service.address or agent.agent_id,
        "to": address,
        "subject": subject,
        "message": message_text,
        "type": mail_type,
    }
    attachments = args.get("attachments", [])
    if attachments:
        resolved = []
        for p in attachments:
            path = Path(p)
            if not path.is_absolute():
                path = agent._working_dir / path
            if not path.is_file():
                return {"status": "error", "message": f"Attachment not found: {path}"}
            resolved.append(str(path))
        payload["attachments"] = resolved
    err = agent._mail_service.send(address, payload)
    status = "delivered" if err is None else "refused"
    agent._log("mail_sent", address=address, subject=subject, status=status, message=message_text)
    if err is None:
        return {"status": "delivered", "to": address}
    else:
        return {"status": "refused", "error": err}


def _read(agent, args: dict) -> dict:
    with agent._mail_queue_lock:
        if not agent._mail_queue:
            return {"status": "ok", "message": None, "remaining": 0}
        entry = agent._mail_queue.popleft()
        remaining = len(agent._mail_queue)
    result = {
        "status": "ok",
        "from": entry["from"],
        "to": entry.get("to", ""),
        "subject": entry.get("subject", ""),
        "message": entry["message"],
        "time": entry["time"],
        "remaining": remaining,
    }
    if entry.get("attachments"):
        result["attachments"] = entry["attachments"]
    return result
