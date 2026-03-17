"""FastAPI API routes."""
from __future__ import annotations

from fastapi import APIRouter, Request
from pydantic import BaseModel

from stoai.services.mail import TCPMailService

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
    for entry in state.agents.values():
        agents.append({
            "id": entry.agent_id,
            "name": entry.name,
            "key": entry.key,
            "address": entry.address,
            "port": entry.port,
            "status": entry.agent.state.value,
            "type": "admin" if entry.agent._admin else "agent",
        })
    return agents


@router.get("/inbox")
def get_inbox(request: Request):
    state = _get_state(request)
    return {"emails": state.get_inbox()}


@router.get("/diary/{agent_key}")
def get_diary(request: Request, agent_key: str, since: float = 0.0):
    state = _get_state(request)
    entry = state.agents.get(agent_key)
    if not entry:
        return {"entries": [], "agent_key": agent_key}
    log_file = entry.working_dir / "logs" / "events.jsonl"
    entries = parse_diary(log_file, since=since)
    return {"entries": entries, "agent_key": agent_key}


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
