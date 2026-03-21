# 灵台 Symposium — Design Spec

**Date:** 2026-03-16
**Repo:** `lingtai-symposium/` (separate git repo, sibling to `lingtai/`)
**Purpose:** A research orchestration app where an orchestrator agent delegates subagents and a memory agent to collaboratively investigate topics, with a React frontend showing a live kanban and agent connection graph.

## Overview

Symposium is the first proper application built on lingtai. It consists of:

1. **A Python backend** — single async process that hosts an HTTP/WebSocket server and spawns lingtai agents
2. **A React frontend** — Vite + TypeScript app with a kanban board, agent graph visualization, and LTM panel
3. **Two MCP tools** — shared context injected into all agents as `MCPTool` instances

The backend is thin — intelligence lives in lingtai agents. The backend provides shared state (agent registry + LTM) and bridges it to the frontend via WebSocket.

## Agent Architecture

### Roles

| Agent | Delegated by | Role |
|-------|-------------|------|
| **Orchestrator** | Backend (root) | Maintains research plan, delegates subagents, reviews LTM submissions, decides broadcasts |
| **Subagents** (N) | Orchestrator | Research specific topics, report findings to orchestrator, email reusable discoveries to memory agent |
| **Memory Agent** | Orchestrator | Audits knowledge submissions from subagents, forwards valuable ones to orchestrator for review |

### Agent Tree (Orchestration Mode)

```
         Orchestrator
        /     |      \
    SubA    SubB    MemoryAgent
```

All agents are peers in the filesystem sense (siblings under `base_dir`), but logically form a tree rooted at the orchestrator. Note: subagents cannot sub-delegate (lingtai's delegate capability explicitly skips itself when replaying capabilities to children).

### Communication

All inter-agent communication uses the existing **email capability** (filesystem-based mailbox with CC/BCC support).

**Two information channels:**

| Channel | What | Mechanism | When to use |
|---------|------|-----------|-------------|
| **LTM** | Universal knowledge expressible in words | `update_ltm` MCP tool → calls `agent.update_system_prompt("ltm", content)` | Pitfalls, best practices, factual discoveries that benefit all agents |
| **Broadcast** | Actionable resources requiring agent-to-agent follow-up | Email CC-all from orchestrator | "SubB has a useful script — email SubB if you need it" |

The orchestrator decides which channel to use. Generic instructions in its system prompt guide this decision. The boundary between the two channels is intentionally flexible — the orchestrator uses judgment.

### Information Flow

**Findings (normal duty):**
```
Subagent ──[email]──▶ Orchestrator
```

**Reusable knowledge:**
```
Subagent ──[email]──▶ Memory Agent
    Memory Agent audits, if valuable:
Memory Agent ──[email]──▶ Orchestrator
    Orchestrator reviews, then either:
    (a) Orchestrator calls update_ltm() ──▶ all agents' LTM updated
    (b) Orchestrator sends CC-all email ──▶ agents contact each other
```

## MCP Tools

Two tools, injected as `MCPTool` instances to agents at construction. Role-based subsetting determines which agents get which tools.

### `manage_agents`

Registry of delegated agents. Three actions:

| Action | Parameters | Returns |
|--------|-----------|---------|
| `list` | (none) | Array of `{agent_id, role, address, status}` |
| `register` | `agent_id`, `role`, `address` | Confirmation |
| `deregister` | `agent_id` | Confirmation |

**Access:** Orchestrator gets all three actions. Subagents and memory agent get `list` action only (cannot register/deregister — only the orchestrator manages the registry after delegating).

### `update_ltm`

Update an agent's LTM by calling `agent.update_system_prompt("ltm", content)` on the target agent instance(s). This mutates the `"ltm"` section in the agent's `SystemPromptManager`, which is included in the system prompt on the next LLM turn.

| Parameter | Required | Description |
|-----------|----------|-------------|
| `content` | Yes | The LTM content (markdown text) |
| `agent_id` | No | Target agent. If omitted, updates all registered agents |

**Access:** Orchestrator only.

### Role-Based Tool Subsetting

```python
def tools_for_role(role: str, registry: AgentRegistry) -> list[MCPTool]:
    if role == "orchestrator":
        # Full manage_agents (list/register/deregister) + update_ltm
        return [manage_agents_full_tool, update_ltm_tool]
    # Subagents/memory: manage_agents with list action only
    return [manage_agents_list_only_tool]
```

### Registration Flow

When the orchestrator delegates a subagent, it must perform a two-step sequence:
1. Call `delegate(role=..., ...)` → receives `agent_id` and `address`
2. Call `manage_agents(action="register", agent_id=..., role=..., address=...)` → adds to registry

The orchestrator's system prompt explicitly instructs this pattern. The registry emits an `agent_registered` event, which the WebSocket server pushes to the frontend.

## Backend

### Tech Stack

- **Python 3.11+**
- **Starlette** (async HTTP/WebSocket server)
- **Uvicorn** (ASGI server)
- **lingtai** (agent framework, installed as dependency)

### File Structure

```
lingtai-symposium/
├── backend/
│   ├── pyproject.toml          # depends on lingtai, starlette, uvicorn
│   ├── src/symposium/
│   │   ├── __init__.py
│   │   ├── app.py              # Entry point: wire registry + server + orchestrator
│   │   ├── registry.py         # AgentRegistry: in-memory state + event bus
│   │   ├── mcp_tools.py        # MCPTool definitions + handlers + role subsetting
│   │   ├── server.py           # Starlette app: REST + WebSocket + static serving
│   │   └── orchestrator.py     # Orchestrator creation: role prompt, capabilities, delegation
│   └── tests/
│       └── ...
├── frontend/
│   ├── package.json
│   ├── tsconfig.json
│   ├── vite.config.ts
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── components/
│       │   ├── Kanban.tsx          # Agent cards grouped by status
│       │   ├── AgentGraph.tsx      # Tree layout + animated email edges
│       │   └── LTMPanel.tsx        # Current shared knowledge display
│       ├── hooks/
│       │   └── useWebSocket.ts     # Live state subscription
│       └── types.ts                # Shared TypeScript types
└── README.md
```

### Entry Point (`app.py`)

Startup sequence:

1. Create `AgentRegistry`
2. Create `LLMService` (reads provider/key from env)
3. Build MCP tools from registry
4. Create orchestrator `BaseAgent` with:
   - `mcp_tools=tools_for_role("orchestrator", registry)`
   - `add_capability("email")`
   - `add_capability("delegate")`
   - Role prompt with orchestration instructions
5. Start Starlette server (HTTP + WebSocket)
6. Wait for user task via HTTP POST, deliver to orchestrator

### AgentRegistry (`registry.py`)

In-memory shared state, thread-safe (all mutations protected by `threading.Lock` since MCP tool handlers are called from different agent threads):

```python
@dataclass
class AgentEntry:
    agent_id: str
    role: str
    address: str          # mail address, e.g. "127.0.0.1:54321"
    status: str           # "active", "idle", "stopped"
    agent_ref: BaseAgent  # live reference for update_system_prompt calls

class AgentRegistry:
    _lock: threading.Lock
    _agents: dict[str, AgentEntry]
    _event_callbacks: list[Callable]

    def register(self, agent_id, role, address, agent_ref) -> None
    def deregister(self, agent_id) -> None
    def list_agents(self) -> list[dict]  # returns dicts (no agent_ref exposed)
    def update_ltm(self, content, agent_id=None) -> None
    def subscribe(self, callback) -> None  # for WebSocket push
    def _emit(self, event_type, payload) -> None
```

`update_ltm` calls `agent_ref.update_system_prompt("ltm", content)` on target agent(s). The registry holds live `BaseAgent` references — the orchestrator passes these when calling `register` (the MCP handler resolves `agent_id` to the live instance via the delegate manager's child tracking).

### Event Bridge (`registry.py`)

Agent-internal events (email sent, status changes) must be bridged to the WebSocket server. The strategy: inject a custom `LoggingService` implementation (`SymposiumLoggingService`) into each agent that both writes JSONL (standard behavior) and forwards events to the registry's event bus.

```python
class SymposiumLoggingService(LoggingService):
    """Forwards agent events to the registry event bus."""

    def __init__(self, agent_id: str, registry: AgentRegistry):
        self._agent_id = agent_id
        self._registry = registry

    def log(self, event: dict) -> None:
        event_type = event.get("type")
        if event_type == "email_sent":
            self._registry._emit("email_sent", {
                "agent_id": self._agent_id, **event
            })
        elif event_type in ("agent_started", "agent_stopped"):
            self._registry._emit("agent_status_changed", {
                "agent_id": self._agent_id,
                "status": "active" if event_type == "agent_started" else "stopped",
            })
```

Each agent is constructed with this logging service. Registry events (`agent_registered`, `agent_deregistered`, `ltm_updated`) are emitted directly by the registry methods. Agent-internal events (`email_sent`, `agent_status_changed`) are emitted by the logging service bridge.

### HTTP/WebSocket Server (`server.py`)

**REST endpoints (for frontend):**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/agents` | List all agents from registry |
| `GET` | `/api/ltm` | Current LTM content |
| `GET` | `/api/emails/{agent_id}` | Email history for an agent (direct filesystem read of `base_dir/{agent_id}/mailbox/`) |
| `POST` | `/api/task` | Send research task to orchestrator |
| `GET` | `/` | Serve React build (`frontend/dist/`) |

**WebSocket (`/ws`):**

Pushes events to frontend:

| Event | Payload | Trigger |
|-------|---------|---------|
| `agent_registered` | `{agent_id, role, address}` | `manage_agents(action="register")` |
| `agent_deregistered` | `{agent_id}` | `manage_agents(action="deregister")` |
| `ltm_updated` | `{content, agent_id}` | `update_ltm()` |
| `email_sent` | `{agent_id, from, to, subject}` | Email capability sends mail (bridged via `SymposiumLoggingService`) |
| `agent_status_changed` | `{agent_id, status}` | Agent lifecycle events (bridged via `SymposiumLoggingService`) |

### Orchestrator Setup (`orchestrator.py`)

The orchestrator's system prompt includes:

- **Role:** Research orchestrator. Maintains a plan, delegates subagents for specific research tasks, delegates one memory agent.
- **LTM guidance:** Generic instructions for when to use `update_ltm` vs broadcast (CC-all email). LTM for universal knowledge in words. Broadcast for actionable resources requiring follow-up.
- **Memory agent instructions:** Delegate a memory agent early. Instruct subagents to email reusable discoveries to the memory agent.
- **Plan management:** Orchestrator maintains its plan as text in its own context. Can use file system for detailed planning if needed.

## Frontend

### Tech Stack

- **React 18+** with TypeScript
- **Vite** for bundling
- **reactflow** for agent graph visualization (tree layout + animated edges)
- **Tailwind CSS** for styling

### Layout

Three-panel layout:

```
┌──────────────────────────────────────────────┐
│  [Task Input Bar]                             │
├───────────────┬──────────────┬───────────────┤
│               │              │               │
│    Kanban     │  Agent Graph │   LTM Panel   │
│               │              │               │
│  ┌─────────┐ │     ◯ Orch   │  ## Shared    │
│  │ Orch    │ │    / | \     │  Knowledge    │
│  │ active  │ │   ◯  ◯  ◯   │               │
│  ├─────────┤ │  A   B  Mem  │  - pytest...  │
│  │ SubA    │ │              │  - avoid X... │
│  │ active  │ │  ~~~edge~~~  │               │
│  ├─────────┤ │  (animated)  │               │
│  │ SubB    │ │              │               │
│  │ active  │ │              │               │
│  ├─────────┤ │              │               │
│  │ Memory  │ │              │               │
│  │ active  │ │              │               │
│  └─────────┘ │              │               │
└───────────────┴──────────────┴───────────────┘
```

### Kanban Board (`Kanban.tsx`)

- Cards for each agent, grouped by status (active / idle / stopped)
- Each card shows: `agent_id`, `role`, brief status
- Click to expand: LTM content, recent emails, token usage

### Agent Graph (`AgentGraph.tsx`)

- Default: tree layout (orchestrator at root)
- Edges represent email contact (who has emailed whom)
- Animated edges for live email traffic
- Toggle for force-directed layout (future polish)

### LTM Panel (`LTMPanel.tsx`)

- Renders current shared LTM as markdown
- Live updates when orchestrator calls `update_ltm`

### WebSocket Hook (`useWebSocket.ts`)

- Connects to `/ws`
- Dispatches events to React state
- Reconnects on disconnect

## Startup & Usage

```bash
# Terminal 1: Backend
cd lingtai-symposium/backend
pip install -e .  # installs symposium + lingtai dependency
python -m symposium  # starts server on http://localhost:8080

# Terminal 2: Frontend (dev mode)
cd lingtai-symposium/frontend
npm install
npm run dev  # Vite dev server with proxy to backend

# Production: frontend built to dist/, served by backend directly
```

User opens `http://localhost:8080`, types a research task, and watches the agent tree grow and communicate in real time.

## Dependencies

### Backend (`pyproject.toml`)

```toml
dependencies = [
    "lingtai",           # agent framework (editable install during dev)
    "starlette",       # async HTTP/WebSocket
    "uvicorn",         # ASGI server
]
```

### Frontend (`package.json`)

```json
{
  "dependencies": {
    "react": "^18",
    "react-dom": "^18",
    "reactflow": "^11"
  },
  "devDependencies": {
    "vite": "^5",
    "typescript": "^5",
    "@types/react": "^18",
    "tailwindcss": "^4",
    "@tailwindcss/vite": "^4"
  }
}
```

## Out of Scope (for now)

- Persistent agent state across app restarts (agents are ephemeral per session)
- Authentication / multi-user
- Agent interruption by user (orchestrator prompt can handle this later)
- Force-directed graph layout (tree first, polish later)
- Fine-grained LTM vs broadcast boundary definitions (orchestrator uses judgment)
- Plan visibility in frontend (orchestrator manages plan internally)
