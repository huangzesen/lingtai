"""FastAPI API routes."""
from __future__ import annotations

import json
from pathlib import Path

from fastapi import APIRouter, Request
from pydantic import BaseModel

from lingtai.services.mail import TCPMailService

from .diary import parse_diary



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

    # Discover unregistered agents by scanning base_dir for .agent.json
    registered_names = {e.agent_name for e in state.agents.values()}
    if state.base_dir.is_dir():
        for manifest_file in state.base_dir.glob("*/.agent.json"):
            try:
                data = json.loads(manifest_file.read_text())
                agent_name = data.get("agent_name", "")
                if agent_name in registered_names:
                    continue
                agent_id = data.get("agent_id", "")
                address = data.get("address", "")
                port = int(address.split(":")[-1]) if ":" in address else 0
                agents.append({
                    "id": agent_id,
                    "name": agent_name,
                    "key": agent_id[:8],
                    "address": address,
                    "port": port,
                    "status": "unknown",
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
    """Resolve agent key to working dir — checks registered agents then base_dir scan."""
    entry = state.agents.get(agent_key)
    if entry:
        return entry.working_dir
    # Check base_dir for unregistered agents (key is first 8 chars of agent_id)
    if state.base_dir.is_dir():
        for manifest_file in state.base_dir.glob("*/.agent.json"):
            try:
                data = json.loads(manifest_file.read_text())
                if data.get("agent_id", "")[:8] == agent_key:
                    return manifest_file.parent
            except (json.JSONDecodeError, OSError):
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
    # Discovered agents (unregistered, from base_dir scan)
    registered_names = {e.agent_name for e in state.agents.values()}
    if state.base_dir.is_dir():
        for manifest_file in state.base_dir.glob("*/.agent.json"):
            try:
                data = json.loads(manifest_file.read_text())
                agent_name = data.get("agent_name", "")
                if agent_name in registered_names:
                    continue
                key = data.get("agent_id", "")[:8]
                log_file = manifest_file.parent / "logs" / "events.jsonl"
                result[key] = parse_diary(log_file, since=since)
            except (json.JSONDecodeError, OSError):
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
