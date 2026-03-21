# Web Dashboard Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a React + FastAPI web dashboard for managing and observing N lingtai agents, recreating the three_agents.py visual layout with dynamic agent support.

**Architecture:** FastAPI backend at `app/web/server/` wraps lingtai as a library — exposes agents, inbox, diary, and send over HTTP. React frontend at `app/web/frontend/` built with Vite + TypeScript + Tailwind polls the API. Launcher script `app/web/run.py` is the entry point (same shape as `examples/two_agents.py`).

**Tech Stack:** Python 3.11+ (FastAPI, uvicorn), React 18, TypeScript, Vite, Tailwind CSS v4

**Spec:** `docs/superpowers/specs/2026-03-17-web-dashboard-design.md`

---

## File Structure

| Action | Path | Responsibility |
|--------|------|---------------|
| Create | `app/web/__init__.py` | Empty package marker |
| Create | `app/web/server/__init__.py` | Empty package marker |
| Create | `app/web/server/state.py` | `AppState`, `AgentEntry` — agent registry, user mailbox, lifecycle |
| Create | `app/web/server/diary.py` | JSONL diary parser — reads events.jsonl files, maps event types |
| Create | `app/web/server/routes.py` | FastAPI router with `/api/agents`, `/api/inbox`, `/api/diary/{key}`, `/api/send` |
| Create | `app/web/server/main.py` | `create_app()` factory — CORS, router, static mount |
| Create | `app/web/run.py` | Launcher script — creates agents, starts server |
| Create | `app/web/requirements.txt` | `fastapi`, `uvicorn` |
| Create | `app/web/frontend/package.json` | React + Vite + Tailwind deps |
| Create | `app/web/frontend/index.html` | Vite entry HTML |
| Create | `app/web/frontend/vite.config.ts` | Vite config with `/api` proxy |
| Create | `app/web/frontend/tsconfig.json` | TypeScript config |
| Create | `app/web/frontend/tsconfig.app.json` | TypeScript app config |
| Create | `app/web/frontend/src/main.tsx` | React entry point |
| Create | `app/web/frontend/src/App.tsx` | Root component — layout, hooks, state |
| Create | `app/web/frontend/src/types.ts` | `AgentInfo`, `Email`, `DiaryEvent` interfaces |
| Create | `app/web/frontend/src/hooks/useAgents.ts` | Fetch agents once on mount |
| Create | `app/web/frontend/src/hooks/useInbox.ts` | Poll inbox every 1.5s |
| Create | `app/web/frontend/src/hooks/useDiary.ts` | Poll diary per agent with `?since=` |
| Create | `app/web/frontend/src/components/Header.tsx` | Logo, agent count |
| Create | `app/web/frontend/src/components/InboxPanel.tsx` | Inbox container + email list |
| Create | `app/web/frontend/src/components/EmailBubble.tsx` | Single email bubble |
| Create | `app/web/frontend/src/components/InputBar.tsx` | Target dropdown, CC/BCC, text input, send |
| Create | `app/web/frontend/src/components/DiaryPanel.tsx` | Diary container + tabs + entries |
| Create | `app/web/frontend/src/components/DiaryTabs.tsx` | All + per-agent tabs |
| Create | `app/web/frontend/src/components/DiaryEntry.tsx` | Single diary event line |
| Create | `app/web/frontend/src/index.css` | Tailwind directives + dark theme custom colors |

---

## Chunk 1: Backend — State & Diary Parser

### Task 1: Create package structure and state module

**Files:**
- Create: `app/__init__.py`
- Create: `app/web/__init__.py`
- Create: `app/web/server/__init__.py`
- Create: `app/web/server/state.py`
- Create: `app/web/requirements.txt`

- [ ] **Step 1: Create package markers**

Create `app/__init__.py`, `app/web/__init__.py`, and `app/web/server/__init__.py` — all empty files.

- [ ] **Step 2: Write `requirements.txt`**

```
fastapi>=0.100.0
uvicorn>=0.20.0
```

- [ ] **Step 3: Write `state.py`**

```python
"""Agent registry and user mailbox state."""
from __future__ import annotations

import threading
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4

from lingtai import Agent, AgentConfig
from lingtai.llm import LLMService
from lingtai.services.mail import TCPMailService


@dataclass
class AgentEntry:
    """One registered agent."""
    agent_id: str
    name: str
    key: str
    address: str
    port: int
    agent: Agent
    mail_service: TCPMailService
    working_dir: Path


class AppState:
    """Shared application state for the FastAPI server."""

    def __init__(self, base_dir: Path, user_port: int = 8300):
        self.base_dir = base_dir
        self.base_dir.mkdir(parents=True, exist_ok=True)
        self.user_port = user_port
        self.agents: dict[str, AgentEntry] = {}
        self.user_mailbox: list[dict] = []
        self._mailbox_lock = threading.Lock()
        self.user_mail: TCPMailService | None = None

    def _on_user_mail(self, payload: dict) -> None:
        """Callback when user's TCPMailService receives an email."""
        ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        entry = {
            "id": f"mail_{uuid4().hex[:8]}",
            "from": payload.get("from", "unknown"),
            "to": payload.get("to", []),
            "cc": payload.get("cc", []),
            "subject": payload.get("subject", "(no subject)"),
            "message": payload.get("message", ""),
            "time": ts,
        }
        with self._mailbox_lock:
            self.user_mailbox.append(entry)

    def get_inbox(self) -> list[dict]:
        """Return a snapshot of the user mailbox."""
        with self._mailbox_lock:
            return list(self.user_mailbox)

    def register_agent(
        self,
        key: str,
        agent_id: str,
        name: str,
        port: int,
        llm: LLMService,
        capabilities: list | dict | None = None,
        covenant: str = "",
        config: AgentConfig | None = None,
    ) -> AgentEntry:
        """Create Agent + TCPMailService, wire together, store in registry."""
        working_dir = self.base_dir / agent_id
        mail_svc = TCPMailService(listen_port=port, working_dir=working_dir)
        agent = Agent(
            agent_id=agent_id,
            service=llm,
            mail_service=mail_svc,
            config=config or AgentConfig(max_turns=10),
            base_dir=self.base_dir,
            covenant=covenant,
            capabilities=capabilities or ["email", "web_search"],
        )
        entry = AgentEntry(
            agent_id=agent_id,
            name=name,
            key=key,
            address=f"127.0.0.1:{port}",
            port=port,
            agent=agent,
            mail_service=mail_svc,
            working_dir=working_dir,
        )
        self.agents[key] = entry
        return entry

    def start_all(self) -> None:
        """Start user mailbox listener + all agents."""
        self.user_mail = TCPMailService(listen_port=self.user_port)
        self.user_mail.listen(on_message=self._on_user_mail)
        for entry in self.agents.values():
            entry.agent.start()

    def stop_all(self) -> None:
        """Stop all agents + user mailbox."""
        for entry in self.agents.values():
            entry.agent.stop(timeout=5.0)
        if self.user_mail:
            self.user_mail.stop()
```

- [ ] **Step 4: Smoke-test the module**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "from app.web.server.state import AppState, AgentEntry; print('OK')"`
Expected: `OK`

- [ ] **Step 5: Commit**

```bash
git add app/
git commit -m "feat(web): add app state and agent registry"
```

---

### Task 2: Create diary parser

**Files:**
- Create: `app/web/server/diary.py`

- [ ] **Step 1: Write `diary.py`**

This module reads JSONL event files and maps event types to the frontend format. Same logic as the `/diary` handler in `examples/two_agents.py` but extracted into a reusable function with `?since=` support.

```python
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
```

- [ ] **Step 2: Smoke-test the module**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "from app.web.server.diary import parse_diary; print('OK')"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add app/web/server/diary.py
git commit -m "feat(web): add JSONL diary parser"
```

---

## Chunk 2: Backend — Routes & App Factory

### Task 3: Create API routes

**Files:**
- Create: `app/web/server/routes.py`

- [ ] **Step 1: Write `routes.py`**

```python
"""FastAPI API routes."""
from __future__ import annotations

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
    for entry in state.agents.values():
        agents.append({
            "id": entry.agent_id,
            "name": entry.name,
            "key": entry.key,
            "address": entry.address,
            "port": entry.port,
            "status": entry.agent.state.value,
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
```

- [ ] **Step 2: Smoke-test the module**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "from app.web.server.routes import router; print('OK')"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add app/web/server/routes.py
git commit -m "feat(web): add FastAPI API routes"
```

---

### Task 4: Create app factory

**Files:**
- Create: `app/web/server/main.py`

- [ ] **Step 1: Write `main.py`**

```python
"""FastAPI app factory."""
from __future__ import annotations

from pathlib import Path

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles

from .routes import router
from .state import AppState


def create_app(state: AppState) -> FastAPI:
    """Create and configure the FastAPI application."""
    app = FastAPI(title="灵台 Web Dashboard")

    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_methods=["*"],
        allow_headers=["*"],
    )

    app.state.app_state = state
    app.include_router(router, prefix="/api")

    # Mount frontend build if it exists (production mode)
    dist_dir = Path(__file__).parent.parent / "frontend" / "dist"
    if dist_dir.is_dir():
        app.mount("/", StaticFiles(directory=str(dist_dir), html=True), name="frontend")

    return app
```

- [ ] **Step 2: Smoke-test the module**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "from app.web.server.main import create_app; print('OK')"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add app/web/server/main.py
git commit -m "feat(web): add FastAPI app factory"
```

---

### Task 5: Create launcher script

**Files:**
- Create: `app/web/run.py`

- [ ] **Step 1: Write `run.py`**

Reference `examples/two_agents.py` for the agent setup pattern — same covenant builder, same capabilities, same LLM config. The only difference is that this uses `create_app()` + uvicorn instead of `http.server`.

```python
"""Launch web dashboard with agents.

Usage:
    python -m app.web.run

Edit this file to add/remove agents, change models, or configure capabilities.
Frontend dev server: cd app/web/frontend && npm run dev
"""
from __future__ import annotations

import os
import sys
from pathlib import Path

# Load .env from project root
env_path = Path(__file__).parent.parent.parent / ".env"
if env_path.exists():
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith("#") and "=" in line:
            key, _, val = line.partition("=")
            os.environ.setdefault(key.strip(), val.strip().strip("'\""))

import uvicorn

from lingtai import AgentConfig
from lingtai.llm import LLMService

from .server.main import create_app
from .server.state import AppState


def make_covenant(name: str, address: str, contacts: dict[str, str]) -> str:
    """Build a structured covenant for an agent."""
    contact_lines = "\n".join(f"- {n}: {a}" for n, a in contacts.items())
    return (
        f"### Identity\n"
        f"Name: {name}\n"
        f"Address: {address}\n"
        f"\n"
        f"### Communication\n"
        f"- All communication — including with the user — is done via email.\n"
        f"- Addresses are ip:port format.\n"
        f"- Your text responses are your private diary — no one sees them.\n"
        f"- Email history is your long-term memory.\n"
        f"- Always report results back to whoever asked. Don't just do work silently.\n"
        f"- When emailing a peer, give enough context. Don't send one-word emails.\n"
        f"\n"
        f"### Contacts\n"
        f"{contact_lines}"
    )


USER_PORT = 8300
AGENTS = [
    {"key": "a", "id": "alice", "name": "Alice", "port": 8301},
    {"key": "b", "id": "bob", "name": "Bob", "port": 8302},
    {"key": "c", "id": "charlie", "name": "Charlie", "port": 8303},
]


def main():
    api_key = os.environ.get("MINIMAX_API_KEY")
    if not api_key:
        print("Error: MINIMAX_API_KEY not set.")
        sys.exit(1)

    llm = LLMService(
        provider="minimax",
        model="MiniMax-M2.5-highspeed",
        api_key=api_key,
        provider_config={"web_search_provider": "minimax"},
        provider_defaults={"minimax": {"model": "MiniMax-M2.5-highspeed"}},
    )

    base_dir = Path.home() / ".lingtai" / "web" / "playground"
    state = AppState(base_dir=base_dir, user_port=USER_PORT)

    # Build contact list for covenants
    all_contacts = {a["name"]: f"127.0.0.1:{a['port']}" for a in AGENTS}
    all_contacts["User"] = f"127.0.0.1:{USER_PORT}"

    for a in AGENTS:
        # Each agent's contacts = all others + user (excluding self)
        contacts = {k: v for k, v in all_contacts.items() if k != a["name"]}
        state.register_agent(
            key=a["key"],
            agent_id=a["id"],
            name=a["name"],
            port=a["port"],
            llm=llm,
            capabilities={"email": {}, "web_search": {}, "file": {}, "bash": {}},
            covenant=make_covenant(a["name"], f"127.0.0.1:{a['port']}", contacts),
        )

    app = create_app(state)
    state.start_all()

    print(f"User mailbox:  127.0.0.1:{USER_PORT}")
    for a in AGENTS:
        print(f"Agent {a['name']:8s}  127.0.0.1:{a['port']}")
    print("API server:    http://localhost:8080")
    print("Frontend dev:  cd app/web/frontend && npm run dev")
    print("Press Ctrl+C to shut down.")

    try:
        uvicorn.run(app, host="0.0.0.0", port=8080)
    except KeyboardInterrupt:
        print("\nShutting down...")
    finally:
        state.stop_all()
        print("Done.")


if __name__ == "__main__":
    main()
```

- [ ] **Step 2: Create `app/web/__main__.py`** so `python -m app.web` works

```python
from .run import main

main()
```

- [ ] **Step 3: Smoke-test the import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "from app.web.run import make_covenant; print('OK')"`
Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add app/web/run.py app/web/__main__.py app/web/requirements.txt
git commit -m "feat(web): add launcher script and requirements"
```

---

## Chunk 3: Frontend — Scaffold & Types

### Task 6: Scaffold React + Vite + Tailwind project

**Files:**
- Create: `app/web/frontend/` (Vite scaffold)

- [ ] **Step 1: Create Vite project**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai/app/web
npm create vite@latest frontend -- --template react-ts
```

- [ ] **Step 2: Install Tailwind CSS v4**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai/app/web/frontend
npm install
npm install tailwindcss @tailwindcss/vite
```

- [ ] **Step 3: Configure Vite** — add Tailwind plugin and API proxy

Replace `app/web/frontend/vite.config.ts`:

```typescript
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
```

- [ ] **Step 4: Configure Tailwind** — set up dark theme colors

Replace `app/web/frontend/src/index.css`:

```css
@import "tailwindcss";

@theme {
  --color-bg: #1a1a2e;
  --color-panel: #16213e;
  --color-panel-dark: #12122a;
  --color-border: #0f3460;
  --color-accent: #e94560;
  --color-accent-hover: #c73e54;
  --color-text: #e0e0e0;
  --color-text-muted: #888;
  --color-text-dim: #666;
  --color-text-faint: #555;
}
```

- [ ] **Step 5: Verify it runs**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai/app/web/frontend
npm run dev
```

Open http://localhost:5173 — should show default Vite React page with Tailwind working.

- [ ] **Step 6: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
git add app/web/frontend/
git commit -m "feat(web): scaffold React + Vite + Tailwind frontend"
```

---

### Task 7: Create TypeScript types and constants

**Files:**
- Create: `app/web/frontend/src/types.ts`

- [ ] **Step 1: Write `types.ts`**

```typescript
export interface AgentInfo {
  id: string;
  name: string;
  key: string;
  address: string;
  port: number;
  status: "active" | "sleeping";
}

export interface Email {
  id: string;
  from: string;
  to?: string[];
  cc?: string[];
  subject: string;
  message: string;
  time: string;
}

export type DiaryEventType =
  | "diary"
  | "thinking"
  | "tool_call"
  | "reasoning"
  | "tool_result"
  | "email_out"
  | "email_in"
  | "cancel_received"
  | "cancel_diary"
  | "unknown";

export interface DiaryEvent {
  type: DiaryEventType;
  time: number;
  agent_key: string;
  agent_name: string;
  text?: string;
  tool?: string;
  args?: Record<string, unknown>;
  status?: string;
  to?: string;
  from?: string;
  subject?: string;
  message?: string;
}

export interface SentMessage {
  to: string;
  cc: string[];
  text: string;
  time: string;
}

/** Agent accent colors — indexed by agent order. */
export const AGENT_COLORS = [
  "#e94560",
  "#4ecdc4",
  "#f0a500",
  "#6bcb77",
  "#b06bcb",
  "#6b9bcb",
  "#cb6bb5",
  "#cbc76b",
];

/** Diary event tag colors: [background, text]. */
export const TAG_COLORS: Record<DiaryEventType, [string, string]> = {
  diary: ["#1a3a1a", "#6bcb77"],
  thinking: ["#3a3a1a", "#cbc76b"],
  tool_call: ["#1a1a3a", "#6b9bcb"],
  reasoning: ["#2a1a3a", "#b06bcb"],
  tool_result: ["#1a2a2a", "#6bcbbb"],
  email_out: ["#1a2a3a", "#6bb5cb"],
  email_in: ["#2a1a2a", "#cb6bb5"],
  cancel_received: ["#3a1a1a", "#e94560"],
  cancel_diary: ["#3a2a1a", "#f0a500"],
  unknown: ["#2a2a2a", "#888888"],
};

/** Tag display labels. */
export const TAG_LABELS: Record<DiaryEventType, string> = {
  diary: "diary",
  thinking: "thinking",
  tool_call: "tool",
  reasoning: "why",
  tool_result: "result",
  email_out: "sent",
  email_in: "received",
  cancel_received: "CANCELLED",
  cancel_diary: "cancel diary",
  unknown: "event",
};
```

- [ ] **Step 2: Commit**

```bash
git add app/web/frontend/src/types.ts
git commit -m "feat(web): add TypeScript types and constants"
```

---

## Chunk 4: Frontend — Hooks

### Task 8: Create data-fetching hooks

**Files:**
- Create: `app/web/frontend/src/hooks/useAgents.ts`
- Create: `app/web/frontend/src/hooks/useInbox.ts`
- Create: `app/web/frontend/src/hooks/useDiary.ts`

- [ ] **Step 1: Write `useAgents.ts`**

```typescript
import { useEffect, useState } from "react";
import type { AgentInfo } from "../types";

export function useAgents() {
  const [agents, setAgents] = useState<AgentInfo[]>([]);

  useEffect(() => {
    fetch("/api/agents")
      .then((r) => r.json())
      .then((data) => setAgents(data))
      .catch(() => {});
  }, []);

  const keyToName: Record<string, string> = {};
  const addressToName: Record<string, string> = {};
  for (const a of agents) {
    keyToName[a.key] = a.name;
    addressToName[a.address] = a.name;
  }

  return { agents, keyToName, addressToName };
}
```

- [ ] **Step 2: Write `useInbox.ts`**

```typescript
import { useCallback, useEffect, useRef, useState } from "react";
import type { Email, SentMessage } from "../types";

const POLL_MS = 1500;

export function useInbox() {
  const [receivedEmails, setReceivedEmails] = useState<Email[]>([]);
  const [sentMessages, setSentMessages] = useState<SentMessage[]>([]);
  const lastCountRef = useRef(0);

  useEffect(() => {
    const poll = async () => {
      try {
        const resp = await fetch("/api/inbox");
        const data = await resp.json();
        if (data.emails.length > lastCountRef.current) {
          lastCountRef.current = data.emails.length;
          setReceivedEmails(data.emails);
        }
      } catch {
        /* ignore */
      }
    };
    const id = setInterval(poll, POLL_MS);
    poll();
    return () => clearInterval(id);
  }, []);

  const addSent = useCallback((msg: SentMessage) => {
    setSentMessages((prev) => [...prev, msg]);
  }, []);

  return { receivedEmails, sentMessages, addSent };
}
```

- [ ] **Step 3: Write `useDiary.ts`**

```typescript
import { useEffect, useRef, useState } from "react";
import type { AgentInfo, DiaryEvent } from "../types";

const POLL_MS = 1500;

export function useDiary(agents: AgentInfo[]) {
  const [entries, setEntries] = useState<DiaryEvent[]>([]);
  const sinceRef = useRef<Record<string, number>>({});

  useEffect(() => {
    if (agents.length === 0) return;

    const poll = async () => {
      try {
        const fetches = agents.map(async (a) => {
          const since = sinceRef.current[a.key] ?? 0;
          const resp = await fetch(
            `/api/diary/${a.key}?since=${since}`
          );
          const data = await resp.json();
          const newEntries: DiaryEvent[] = (data.entries || []).map(
            (e: DiaryEvent) => ({
              ...e,
              agent_key: a.key,
              agent_name: a.name,
            })
          );
          // Update since to the latest timestamp
          if (newEntries.length > 0) {
            const maxTs = Math.max(...newEntries.map((e) => e.time));
            sinceRef.current[a.key] = maxTs;
          }
          return newEntries;
        });

        const results = await Promise.all(fetches);
        const allNew = results.flat();
        if (allNew.length > 0) {
          setEntries((prev) => {
            const combined = [...prev, ...allNew];
            combined.sort((a, b) => a.time - b.time);
            return combined;
          });
        }
      } catch {
        /* ignore */
      }
    };

    const id = setInterval(poll, POLL_MS);
    poll();
    return () => clearInterval(id);
  }, [agents]);

  return entries;
}
```

- [ ] **Step 4: Commit**

```bash
git add app/web/frontend/src/hooks/
git commit -m "feat(web): add useAgents, useInbox, useDiary hooks"
```

---

## Chunk 5: Frontend — Components

### Task 9: Create Header and EmailBubble components

**Files:**
- Create: `app/web/frontend/src/components/Header.tsx`
- Create: `app/web/frontend/src/components/EmailBubble.tsx`

- [ ] **Step 1: Write `Header.tsx`**

```tsx
import type { AgentInfo } from "../types";

interface HeaderProps {
  agents: AgentInfo[];
  userPort: number;
}

export function Header({ agents, userPort }: HeaderProps) {
  const activeCount = agents.filter((a) => a.status === "active").length;
  return (
    <div className="flex items-center gap-3 px-5 py-2.5 bg-panel border-b border-border">
      <h1 className="text-base font-bold text-accent">灵台</h1>
      <span className="text-xs text-text-dim">
        {agents.length} agent{agents.length !== 1 ? "s" : ""} · User
        mailbox :{userPort}
      </span>
      {activeCount > 0 && (
        <span className="text-xs text-emerald-400 ml-auto">
          ● {activeCount} active
        </span>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Write `EmailBubble.tsx`**

```tsx
interface EmailBubbleProps {
  direction: "sent" | "received";
  fromName?: string;
  toName?: string;
  cc?: string[];
  subject?: string;
  message: string;
  time: string;
}

export function EmailBubble({
  direction,
  fromName,
  toName,
  cc,
  subject,
  message,
  time,
}: EmailBubbleProps) {
  const isSent = direction === "sent";
  return (
    <div
      className={`px-3.5 py-2.5 rounded-lg text-sm leading-relaxed whitespace-pre-wrap break-words max-w-[80%] ${
        isSent
          ? "self-end bg-border"
          : "self-start bg-panel border border-border"
      }`}
    >
      <div className="text-[11px] text-text-dim mb-1">
        {isSent ? `To: ${toName}` : `From: ${fromName}`}
        {subject && subject !== "(no subject)" && ` — ${subject}`}
        {cc && cc.length > 0 && ` · CC: ${cc.join(", ")}`}
      </div>
      {escapeAndRender(message)}
    </div>
  );
}

function escapeAndRender(text: string) {
  return <span>{text}</span>;
}
```

- [ ] **Step 3: Commit**

```bash
git add app/web/frontend/src/components/Header.tsx app/web/frontend/src/components/EmailBubble.tsx
git commit -m "feat(web): add Header and EmailBubble components"
```

---

### Task 10: Create InputBar component

**Files:**
- Create: `app/web/frontend/src/components/InputBar.tsx`

- [ ] **Step 1: Write `InputBar.tsx`**

```tsx
import { useState, useRef } from "react";
import type { AgentInfo, SentMessage } from "../types";

interface InputBarProps {
  agents: AgentInfo[];
  keyToName: Record<string, string>;
  onSent: (msg: SentMessage) => void;
}

export function InputBar({ agents, keyToName, onSent }: InputBarProps) {
  const [target, setTarget] = useState(agents[0]?.key ?? "");
  const [ccVisible, setCcVisible] = useState(false);
  const [bccVisible, setBccVisible] = useState(false);
  const [ccChecked, setCcChecked] = useState<Record<string, boolean>>({});
  const [bccChecked, setBccChecked] = useState<Record<string, boolean>>({});
  const inputRef = useRef<HTMLInputElement>(null);

  const sendEmail = async () => {
    const text = inputRef.current?.value.trim();
    if (!text || !target) return;
    inputRef.current!.value = "";

    const cc = Object.keys(ccChecked).filter((k) => ccChecked[k] && k !== target);
    const bcc = Object.keys(bccChecked).filter((k) => bccChecked[k] && k !== target);
    // BCC takes precedence — remove from CC if in both
    const finalCC = cc.filter((k) => !bcc.includes(k));

    onSent({ to: target, cc: finalCC, text, time: new Date().toISOString() });

    await fetch("/api/send", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent: target, message: text, cc: finalCC, bcc }),
    });

    setCcChecked({});
    setBccChecked({});
    inputRef.current?.focus();
  };

  const otherAgents = agents.filter((a) => a.key !== target);

  return (
    <div>
      {ccVisible && (
        <div className="px-4 py-1.5 bg-panel text-xs text-text-muted border-t border-border">
          CC:{" "}
          {otherAgents.map((a) => (
            <label key={a.key} className="mr-3 cursor-pointer">
              <input
                type="checkbox"
                className="mr-1"
                checked={!!ccChecked[a.key]}
                onChange={(e) =>
                  setCcChecked((p) => ({ ...p, [a.key]: e.target.checked }))
                }
              />
              {a.name}
            </label>
          ))}
        </div>
      )}
      {bccVisible && (
        <div className="px-4 py-1.5 bg-panel text-xs text-text-muted border-t border-border">
          BCC:{" "}
          {otherAgents.map((a) => (
            <label key={a.key} className="mr-3 cursor-pointer">
              <input
                type="checkbox"
                className="mr-1"
                checked={!!bccChecked[a.key]}
                onChange={(e) =>
                  setBccChecked((p) => ({ ...p, [a.key]: e.target.checked }))
                }
              />
              {a.name}
            </label>
          ))}
        </div>
      )}
      <div className="flex items-center gap-2 px-4 py-2.5 bg-panel border-t border-border">
        <select
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          className="px-2 py-1.5 border border-border rounded-md bg-bg text-text text-sm"
        >
          {agents.map((a) => (
            <option key={a.key} value={a.key}>
              To: {a.name} (:{a.port})
            </option>
          ))}
        </select>
        <button
          onClick={() => setCcVisible((v) => !v)}
          className={`px-2.5 py-1.5 text-xs border rounded-md cursor-pointer ${
            ccVisible
              ? "text-text border-accent"
              : "text-text-muted border-border bg-bg"
          }`}
        >
          CC
        </button>
        <button
          onClick={() => setBccVisible((v) => !v)}
          className={`px-2.5 py-1.5 text-xs border rounded-md cursor-pointer ${
            bccVisible
              ? "text-text border-accent"
              : "text-text-muted border-border bg-bg"
          }`}
        >
          BCC
        </button>
        <input
          ref={inputRef}
          placeholder="Type a message..."
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              sendEmail();
            }
          }}
          className="flex-1 px-3 py-1.5 border border-border rounded-md bg-bg text-text text-sm outline-none focus:border-accent"
          autoFocus
        />
        <button
          onClick={sendEmail}
          className="px-4 py-1.5 bg-accent text-white border-none rounded-md cursor-pointer text-sm hover:bg-accent-hover"
        >
          Send
        </button>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add app/web/frontend/src/components/InputBar.tsx
git commit -m "feat(web): add InputBar component with CC/BCC"
```

---

### Task 11: Create InboxPanel component

**Files:**
- Create: `app/web/frontend/src/components/InboxPanel.tsx`

- [ ] **Step 1: Write `InboxPanel.tsx`**

```tsx
import { useEffect, useRef, useMemo } from "react";
import type { AgentInfo, Email, SentMessage } from "../types";
import { EmailBubble } from "./EmailBubble";
import { InputBar } from "./InputBar";

interface InboxPanelProps {
  agents: AgentInfo[];
  keyToName: Record<string, string>;
  addressToName: Record<string, string>;
  receivedEmails: Email[];
  sentMessages: SentMessage[];
  onSent: (msg: SentMessage) => void;
}

export function InboxPanel({
  agents,
  keyToName,
  addressToName,
  receivedEmails,
  sentMessages,
  onSent,
}: InboxPanelProps) {
  const scrollRef = useRef<HTMLDivElement>(null);

  const allMessages = useMemo(() => {
    const items: Array<{
      type: "sent" | "received";
      time: string;
      msg: SentMessage | Email;
    }> = [];
    for (const s of sentMessages) {
      items.push({ type: "sent", time: s.time, msg: s });
    }
    for (const e of receivedEmails) {
      items.push({ type: "received", time: e.time, msg: e });
    }
    items.sort((a, b) => a.time.localeCompare(b.time));
    return items;
  }, [sentMessages, receivedEmails]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [allMessages]);

  return (
    <div className="flex-[2] flex flex-col border-r border-border">
      <div className="px-4 py-2 text-xs text-accent uppercase tracking-widest border-b border-border">
        Inbox
      </div>
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto p-4 flex flex-col gap-2"
      >
        {allMessages.map((item, i) => {
          if (item.type === "sent") {
            const s = item.msg as SentMessage;
            const ccNames = s.cc.map((k) => keyToName[k] || k);
            return (
              <EmailBubble
                key={`sent-${i}`}
                direction="sent"
                toName={keyToName[s.to] || s.to}
                cc={ccNames.length > 0 ? ccNames : undefined}
                message={s.text}
                time={s.time}
              />
            );
          } else {
            const e = item.msg as Email;
            const fromName = addressToName[e.from] || e.from;
            const ccNames = (e.cc || []).map(
              (addr) => addressToName[addr] || addr
            );
            return (
              <EmailBubble
                key={e.id}
                direction="received"
                fromName={fromName}
                subject={e.subject}
                cc={ccNames.length > 0 ? ccNames : undefined}
                message={e.message}
                time={e.time}
              />
            );
          }
        })}
      </div>
      <InputBar agents={agents} keyToName={keyToName} onSent={onSent} />
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add app/web/frontend/src/components/InboxPanel.tsx
git commit -m "feat(web): add InboxPanel component"
```

---

### Task 12: Create DiaryTabs, DiaryEntry, and DiaryPanel components

**Files:**
- Create: `app/web/frontend/src/components/DiaryTabs.tsx`
- Create: `app/web/frontend/src/components/DiaryEntry.tsx`
- Create: `app/web/frontend/src/components/DiaryPanel.tsx`

- [ ] **Step 1: Write `DiaryTabs.tsx`**

```tsx
import type { AgentInfo } from "../types";
import { AGENT_COLORS } from "../types";

interface DiaryTabsProps {
  agents: AgentInfo[];
  activeTab: string;
  onTabChange: (tab: string) => void;
}

export function DiaryTabs({ agents, activeTab, onTabChange }: DiaryTabsProps) {
  return (
    <div className="flex gap-0 border-b border-border overflow-x-auto">
      <button
        className={`px-3 py-2 text-xs uppercase tracking-widest border-b-2 cursor-pointer bg-transparent ${
          activeTab === "all"
            ? "text-accent border-accent"
            : "text-text-dim border-transparent hover:text-text"
        }`}
        onClick={() => onTabChange("all")}
      >
        All
      </button>
      {agents.map((a, i) => (
        <button
          key={a.key}
          className={`px-3 py-2 text-xs uppercase tracking-widest border-b-2 cursor-pointer bg-transparent ${
            activeTab === a.key
              ? "border-accent"
              : "border-transparent hover:text-text"
          }`}
          style={{
            color:
              activeTab === a.key
                ? AGENT_COLORS[i % AGENT_COLORS.length]
                : undefined,
          }}
          onClick={() => onTabChange(a.key)}
        >
          {a.name}
        </button>
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Write `DiaryEntry.tsx`**

```tsx
import type { AgentInfo, DiaryEvent } from "../types";
import { AGENT_COLORS, TAG_COLORS, TAG_LABELS } from "../types";

interface DiaryEntryProps {
  event: DiaryEvent;
  agents: AgentInfo[];
  addressToName: Record<string, string>;
}

export function DiaryEntry({ event, agents, addressToName }: DiaryEntryProps) {
  const agentIndex = agents.findIndex((a) => a.key === event.agent_key);
  const agentColor = AGENT_COLORS[agentIndex % AGENT_COLORS.length] || "#888";
  const [tagBg, tagFg] = TAG_COLORS[event.type] || TAG_COLORS.unknown;
  const tagLabel = TAG_LABELS[event.type] || event.type;

  const ts = new Date(event.time * 1000).toLocaleTimeString();

  let content: React.ReactNode = null;

  switch (event.type) {
    case "diary":
    case "thinking":
    case "cancel_diary":
    case "unknown":
      content = event.text || "";
      break;
    case "tool_call": {
      const args = JSON.stringify(event.args || {}).slice(0, 80);
      content = `${event.tool}(${args})`;
      break;
    }
    case "reasoning":
      content = `${event.tool}: ${event.text || ""}`;
      break;
    case "tool_result":
      content = `${event.tool} → ${event.status || ""}`;
      break;
    case "email_out": {
      const toName = addressToName[event.to || ""] || event.to || "";
      const subj = event.subject ? ` — ${event.subject}` : "";
      content = (
        <>
          to {toName}
          {subj}
          {event.message && (
            <div className="mt-1 p-1.5 bg-white/[0.03] rounded text-[11px] text-text-muted max-h-[200px] overflow-y-auto whitespace-pre-wrap">
              {event.message}
            </div>
          )}
        </>
      );
      break;
    }
    case "email_in": {
      const fromName = addressToName[event.from || ""] || event.from || "";
      const subj = event.subject ? ` — ${event.subject}` : "";
      content = (
        <>
          from {fromName}
          {subj}
          {event.message && (
            <div className="mt-1 p-1.5 bg-white/[0.03] rounded text-[11px] text-text-muted max-h-[200px] overflow-y-auto whitespace-pre-wrap">
              {event.message}
            </div>
          )}
        </>
      );
      break;
    }
    case "cancel_received": {
      const fromName = addressToName[event.from || ""] || event.from || "";
      content = `by ${fromName}${event.subject ? ` — ${event.subject}` : ""}`;
      break;
    }
  }

  return (
    <div className="py-1 border-b border-bg leading-relaxed text-xs">
      <span className="text-text-faint">{ts}</span>{" "}
      <span className="font-bold" style={{ color: agentColor }}>
        [{event.agent_name}]
      </span>{" "}
      <span
        className="text-[10px] px-1 rounded mr-1"
        style={{ backgroundColor: tagBg, color: tagFg }}
      >
        {tagLabel}
      </span>
      {content}
    </div>
  );
}
```

- [ ] **Step 3: Write `DiaryPanel.tsx`**

```tsx
import { useEffect, useMemo, useRef, useState } from "react";
import type { AgentInfo, DiaryEvent } from "../types";
import { DiaryEntry } from "./DiaryEntry";
import { DiaryTabs } from "./DiaryTabs";

interface DiaryPanelProps {
  agents: AgentInfo[];
  entries: DiaryEvent[];
  addressToName: Record<string, string>;
}

export function DiaryPanel({ agents, entries, addressToName }: DiaryPanelProps) {
  const [activeTab, setActiveTab] = useState("all");
  const scrollRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(
    () =>
      activeTab === "all"
        ? entries
        : entries.filter((e) => e.agent_key === activeTab),
    [entries, activeTab]
  );

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [filtered]);

  return (
    <div className="flex-1 flex flex-col bg-panel-dark">
      <DiaryTabs
        agents={agents}
        activeTab={activeTab}
        onTabChange={setActiveTab}
      />
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto p-3 text-xs text-text-muted"
      >
        {filtered.map((e, i) => (
          <DiaryEntry
            key={`${e.agent_key}-${e.time}-${i}`}
            event={e}
            agents={agents}
            addressToName={addressToName}
          />
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add app/web/frontend/src/components/DiaryTabs.tsx app/web/frontend/src/components/DiaryEntry.tsx app/web/frontend/src/components/DiaryPanel.tsx
git commit -m "feat(web): add DiaryPanel, DiaryTabs, DiaryEntry components"
```

---

## Chunk 6: Frontend — App Assembly

### Task 13: Wire up App.tsx and main.tsx

**Files:**
- Modify: `app/web/frontend/src/App.tsx`
- Modify: `app/web/frontend/src/main.tsx`

- [ ] **Step 1: Replace `App.tsx`**

```tsx
import { useAgents } from "./hooks/useAgents";
import { useInbox } from "./hooks/useInbox";
import { useDiary } from "./hooks/useDiary";
import { Header } from "./components/Header";
import { InboxPanel } from "./components/InboxPanel";
import { DiaryPanel } from "./components/DiaryPanel";

const USER_PORT = 8300;

export default function App() {
  const { agents, keyToName, addressToName } = useAgents();
  const { receivedEmails, sentMessages, addSent } = useInbox();
  const entries = useDiary(agents);

  return (
    <div className="h-screen flex flex-col bg-bg text-text font-sans">
      <Header agents={agents} userPort={USER_PORT} />
      <div className="flex-1 flex overflow-hidden">
        <InboxPanel
          agents={agents}
          keyToName={keyToName}
          addressToName={addressToName}
          receivedEmails={receivedEmails}
          sentMessages={sentMessages}
          onSent={addSent}
        />
        <DiaryPanel
          agents={agents}
          entries={entries}
          addressToName={addressToName}
        />
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Replace `main.tsx`**

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);
```

- [ ] **Step 3: Clean up scaffolded files**

Delete the Vite scaffolded files that are no longer needed:
- `app/web/frontend/src/App.css`
- `app/web/frontend/src/assets/` (entire directory)

- [ ] **Step 4: Verify frontend builds**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai/app/web/frontend
npm run build
```

Expected: build succeeds with no TypeScript errors.

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
git add app/web/frontend/
git commit -m "feat(web): wire up App with all components and hooks"
```

---

## Chunk 7: Integration & Gitignore

### Task 14: Update .gitignore and add app __init__.py

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: Add frontend build artifacts and .superpowers to .gitignore**

Append to `.gitignore`:

```
# Web dashboard frontend build
app/web/frontend/node_modules/
app/web/frontend/dist/

# Superpowers brainstorm sessions
.superpowers/
```

- [ ] **Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: update .gitignore for web dashboard"
```

---

### Task 15: End-to-end smoke test

- [ ] **Step 1: Install backend deps**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
pip install fastapi uvicorn
```

- [ ] **Step 2: Smoke-test backend imports**

```bash
python -c "from app.web.server.main import create_app; from app.web.server.state import AppState; print('Backend OK')"
```

- [ ] **Step 3: Build frontend**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai/app/web/frontend
npm install && npm run build
```

- [ ] **Step 4: Run full stack (manual test)**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
python -m app.web
```

Verify:
- Server starts on :8080 without errors
- `curl http://localhost:8080/api/agents` returns the agent list JSON
- `curl http://localhost:8080/api/inbox` returns `{"emails": []}`
- Open http://localhost:8080 in browser — should show the dashboard UI
- Send a message to an agent — should appear in diary stream

- [ ] **Step 5: Commit any fixes from smoke test**
