"""FastAPI API routes."""
from __future__ import annotations

import json
import socket
from pathlib import Path

from fastapi import APIRouter, Request
from pydantic import BaseModel

from stoai.services.mail import TCPMailService

from .diary import parse_diary


def _check_agent_alive(host: str, port: int, timeout: float = 0.3) -> bool:
    """Check if a stoai agent is alive by reading its TCP banner."""
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(timeout)
        sock.connect((host, port))
        data = sock.recv(64)
        sock.close()
        return data.startswith(b"STOAI ")
    except (OSError, socket.timeout):
        return False

router = APIRouter()


# --- Request/Response models ---

class SendRequest(BaseModel):
    agent: str
    message: str
    cc: list[str] = []
    bcc: list[str] = []


# --- Helpers ---

def _get_state(request: Request):
    return request.app.state.app_state


# --- Endpoints ---

@router.get("/agents")
def list_agents(request: Request):
    state = _get_state(request)
    agents = []
    known_ids = set()

    # Registered agents (from AppState)
    for entry in state.agents.values():
        agent_id = entry.agent.agent_id
        known_ids.add(agent_id)
        agents.append({
            "id": agent_id,
            "name": entry.name,
            "key": entry.key,
            "address": entry.address,
            "port": entry.port,
            "status": entry.agent.state.value,
            "type": "admin" if entry.agent._admin else "agent",
        })

    # Discover unregistered agents from billboard + verify alive via TCP banner
    billboard_dir = Path.home() / ".stoai" / "billboard"
    if billboard_dir.is_dir():
        for f in billboard_dir.glob("*.json"):
            try:
                data = json.loads(f.read_text())
                agent_id = data.get("agent_id", "")
                if agent_id in known_ids:
                    continue
                address = data.get("address", "")
                if ":" not in address:
                    continue
                host, port_str = address.rsplit(":", 1)
                port = int(port_str)

                # Verify agent is alive via TCP banner
                if not _check_agent_alive(host, port):
                    # Stale billboard entry — agent is dead, clean up
                    try:
                        f.unlink()
                    except OSError:
                        pass
                    continue

                agents.append({
                    "id": agent_id,
                    "name": data.get("agent_name", agent_id),
                    "key": agent_id[:8],
                    "address": address,
                    "port": port,
                    "status": "active",
                    "type": "admin" if data.get("admin") else "agent",
                })
            except (json.JSONDecodeError, OSError, ValueError):
                continue

    return agents


@router.get("/inbox")
def get_inbox(request: Request):
    state = _get_state(request)
    return {"emails": state.get_inbox()}


def _resolve_working_dir(state, agent_key: str) -> Path | None:
    """Resolve agent key to working dir — checks registered agents then billboard."""
    entry = state.agents.get(agent_key)
    if entry:
        return entry.working_dir
    # Check billboard for unregistered agents (key is first 8 chars of agent_id)
    billboard_dir = Path.home() / ".stoai" / "billboard"
    if billboard_dir.is_dir():
        for f in billboard_dir.glob("*.json"):
            try:
                data = json.loads(f.read_text())
                if data.get("agent_id", "")[:8] == agent_key:
                    return Path(data["working_dir"])
            except (json.JSONDecodeError, OSError, KeyError):
                continue
    return None


@router.get("/diary/{agent_key}")
def get_diary(request: Request, agent_key: str, since: float = 0.0):
    state = _get_state(request)
    working_dir = _resolve_working_dir(state, agent_key)
    if not working_dir:
        return {"entries": [], "agent_key": agent_key}
    log_file = working_dir / "logs" / "events.jsonl"
    entries = parse_diary(log_file, since=since)
    return {"entries": entries, "agent_key": agent_key}


@router.get("/diary")
def get_all_diaries(request: Request, since: float = 0.0):
    """Batch diary endpoint — returns all agents' entries in one request."""
    state = _get_state(request)
    result = {}
    # Registered agents
    for key, entry in state.agents.items():
        log_file = entry.working_dir / "logs" / "events.jsonl"
        result[key] = parse_diary(log_file, since=since)
    # Billboard agents (unregistered)
    billboard_dir = Path.home() / ".stoai" / "billboard"
    if billboard_dir.is_dir():
        registered_ids = {e.agent.agent_id for e in state.agents.values()}
        for f in billboard_dir.glob("*.json"):
            try:
                data = json.loads(f.read_text())
                agent_id = data.get("agent_id", "")
                if agent_id in registered_ids:
                    continue
                key = agent_id[:8]
                working_dir = Path(data["working_dir"])
                log_file = working_dir / "logs" / "events.jsonl"
                result[key] = parse_diary(log_file, since=since)
            except (json.JSONDecodeError, OSError, KeyError):
                continue
    return result


@router.post("/send")
def send_email(request: Request, body: SendRequest):
    state = _get_state(request)
    entry = state.agents.get(body.agent)
    if not entry:
        return {"status": "failed", "error": f"Unknown agent: {body.agent}"}

    to_addr = entry.address
    cc_addrs = [
        state.agents[k].address
        for k in body.cc if k in state.agents
    ]
    bcc_addrs = [
        state.agents[k].address
        for k in body.bcc if k in state.agents
    ]

    # Build base payload — no bcc field on the wire
    payload = {
        "from": f"127.0.0.1:{state.user_port}",
        "to": [to_addr],
        "subject": "",
        "message": body.message,
    }
    if cc_addrs:
        payload["cc"] = cc_addrs

    # Fan out to all recipients
    sender = TCPMailService()
    all_addrs = [to_addr] + cc_addrs + bcc_addrs
    results = [sender.send(addr, payload) for addr in all_addrs]
    ok = all(r is None for r in results)
    return {"status": "delivered" if ok else "failed"}
