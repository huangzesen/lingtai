# 灵台 Web Dashboard

React frontend + FastAPI backend for managing and observing N lingtai agents.

## Location

`app/web/` — application layer, separate from `src/lingtai/` (library) and `examples/` (demos). Designed to be migrated out as an independent package later.

## Architecture

Two processes: a FastAPI backend that wraps lingtai, and a React frontend that talks to it over HTTP.

```
app/web/
├── server/              # FastAPI backend (Python)
│   ├── main.py          # FastAPI app factory, CORS, static mount
│   ├── routes.py        # API endpoint handlers
│   └── state.py         # Agent registry, user mailbox state
├── frontend/            # React app (Vite + TypeScript + Tailwind)
│   ├── src/
│   │   ├── App.tsx
│   │   ├── components/
│   │   │   ├── Header.tsx
│   │   │   ├── InboxPanel.tsx
│   │   │   ├── DiaryPanel.tsx
│   │   │   ├── EmailBubble.tsx
│   │   │   ├── DiaryEntry.tsx
│   │   │   ├── InputBar.tsx
│   │   │   └── DiaryTabs.tsx
│   │   ├── hooks/
│   │   │   ├── useAgents.ts
│   │   │   ├── useInbox.ts
│   │   │   └── useDiary.ts
│   │   └── types.ts
│   ├── index.html
│   ├── tailwind.config.js
│   ├── vite.config.ts
│   └── package.json
├── run.py               # Example launcher script
└── requirements.txt     # fastapi, uvicorn
```

## Backend: FastAPI Server

### Design Principles

- Wraps lingtai — imports `Agent`, `LLMService`, `TCPMailService` as a library
- Does NOT modify lingtai internals
- Stateless diary reads — reads JSONL files from agent working directories on each request
- User inbox is in-memory (same pattern as existing examples, populated via TCPMailService callback)

### State (`server/state.py`)

```python
@dataclass
class AgentEntry:
    agent_id: str           # e.g. "alice"
    name: str               # e.g. "Alice"
    key: str                # e.g. "a" (short key for frontend)
    address: str            # e.g. "127.0.0.1:8301"
    port: int               # e.g. 8301
    agent: Agent            # lingtai Agent instance
    mail_service: TCPMailService
    working_dir: Path

class AppState:
    agents: dict[str, AgentEntry]   # key → AgentEntry
    user_mailbox: list[dict]        # received emails, thread-safe
    user_mailbox_lock: threading.Lock
    user_port: int
    base_dir: Path
    user_mail: TCPMailService       # user's TCP listener

    def register_agent(self, key, agent_id, name, port, llm, capabilities, covenant):
        """Create Agent + TCPMailService, wire together, store in registry.

        1. Creates TCPMailService(listen_port=port, working_dir=base_dir/agent_id)
        2. Creates Agent(agent_id=agent_id, service=llm, mail_service=...,
                         base_dir=base_dir, covenant=covenant, capabilities=capabilities)
        3. Stores AgentEntry in self.agents[key]
        """

    def start_all(self):
        """Start user mailbox listener + all agents."""

    def stop_all(self):
        """Stop all agents + user mailbox. Called on shutdown."""
```

**Agent status:** `AgentInfo.status` is derived from `entry.agent.state` (the `AgentState` enum: `ACTIVE` or `SLEEPING`). The `state` property is public on `BaseAgent`.

### API Endpoints (`server/routes.py`)

| Method | Path | Request | Response |
|--------|------|---------|----------|
| GET | `/api/agents` | — | `[{id, name, key, address, port, status}]` |
| GET | `/api/inbox` | — | `{emails: [{id, from, to, cc, subject, message, time}]}` |
| GET | `/api/diary/{agent_key}` | `?since=<float timestamp>` | `{entries: [{type, time, ...}], agent_key}` |
| POST | `/api/send` | `{agent: "a", message: "...", cc: ["b"], bcc: ["c"]}` | `{status: "delivered"\|"failed"}` |

### Diary Endpoint Detail

Reads `{working_dir}/logs/events.jsonl` directly from disk. Parses each JSONL line and maps event types:

| JSONL `type` | Frontend `type` | Fields extracted |
|-------------|----------------|-----------------|
| `diary` | `diary` | `ts`, `text` |
| `thinking` | `thinking` | `ts`, `text` |
| `tool_call` | `tool_call` | `ts`, `tool_name`, `tool_args` |
| `tool_reasoning` | `reasoning` | `ts`, `tool`, `reasoning` → mapped to `text` |
| `tool_result` | `tool_result` | `ts`, `tool_name`, `status` |
| `mail_sent` | `email_out` | `ts`, `address` → mapped to `to`, `subject`, `message` |
| `email_sent` | `email_out` | `ts`, `to` (list → join with ", "), `subject`, `message`, `delivered`, `refused` |
| `mail_received` | `email_in` | `ts`, `sender`, `subject`, `message` |
| `email_received` | `email_in` | `ts`, `sender`, `subject`, `message` |
| `cancel_received` | `cancel_received` | `ts`, `sender`, `subject` |
| `cancel_diary` | `cancel_diary` | `ts`, `text` |
| *(any other)* | `unknown` | `ts`, raw JSON stringified as `text` |

**Note:** lingtai has two layers of mail logging. The base mail intrinsic logs `mail_sent`/`mail_received` (simple fields). The email capability logs `email_sent`/`email_received` (richer fields with CC/BCC/delivered/refused). The diary parser must handle both.

The `?since=<timestamp>` parameter enables incremental polling — only entries with `ts > since` are returned.

### Send Endpoint Detail

Same fan-out logic as `three_agents.py`:
1. Resolve `agent` key to port
2. Resolve `cc` and `bcc` keys to addresses
3. Build base payload (no `bcc` field on wire)
4. Fan out via `TCPMailService.send()` to all recipients (to + cc + bcc)

### App Factory (`server/main.py`)

```python
def create_app(state: AppState) -> FastAPI:
    app = FastAPI(title="灵台 Web Dashboard")
    app.add_middleware(CORSMiddleware, ...)
    app.state.app_state = state
    app.include_router(api_router, prefix="/api")
    # Mount frontend build at "/" (production)
    return app
```

### Launcher Script (`run.py`)

Same shape as `two_agents.py` — a Python script that:
1. Creates `LLMService`
2. Creates N agents with `TCPMailService` + capabilities
3. Builds `AppState` with agent registry
4. Calls `create_app(state)` and runs with uvicorn

Users edit this script to add/remove agents, change models, configure capabilities. The frontend discovers agents dynamically from `/api/agents`.

Example structure:
```python
from app.web.server.main import create_app
from app.web.server.state import AppState, AgentEntry

state = AppState(base_dir=Path.home() / ".lingtai" / "web", user_port=8300)
state.register_agent("a", "alice", "Alice", 8301, llm, capabilities=[...], covenant="...")
state.register_agent("b", "bob", "Bob", 8302, llm, capabilities=[...], covenant="...")

app = create_app(state)
state.start_all()
try:
    uvicorn.run(app, host="0.0.0.0", port=8080)
finally:
    state.stop_all()  # graceful shutdown: stops all agents + user mailbox
```

## Frontend: React + Vite + Tailwind

### Visual Design

Faithful recreation of the `three_agents.py` UI:

- **Dark theme**: `#1a1a2e` (background), `#16213e` (panels), `#0f3460` (borders), `#e94560` (accent), `#e0e0e0` (text)
- **2-panel layout**: Inbox (flex-2, left) + Diary (flex-1, right)
- **Inbox**: Chat-bubble style — sent messages right-aligned (blue), received left-aligned (dark border)
- **Diary**: Scrolling log with colored tags per event type
- **Input bar**: Agent dropdown + CC/BCC toggles + text input + send button
- **Agent tabs**: "All" + one tab per agent, dynamically generated from `/api/agents`

### N-Agent Scaling

- Agent dropdown populated from `/api/agents` response
- Diary tabs generated dynamically — scroll horizontally if many agents
- CC/BCC checkboxes generated dynamically (excluding the "To" target)
- Agent colors assigned from a fixed palette array by index:
  ```typescript
  const AGENT_COLORS = [
    "#e94560", "#4ecdc4", "#f0a500", "#6bcb77",
    "#b06bcb", "#6b9bcb", "#cb6bb5", "#cbc76b",
  ];
  ```
- Agent name ↔ address mapping derived from `/api/agents`

### Components

**`Header.tsx`** — 灵台 logo, subtitle with agent count, user mailbox port.

**`InboxPanel.tsx`** — Container for inbox + input bar. Merges sent + received emails, sorts by time, renders as `EmailBubble` list. Auto-scrolls to bottom on new messages.

**`EmailBubble.tsx`** — Single email. Props: `direction` (sent/received), `from`/`to` name, `cc` names, `subject`, `message`, `time`. Sent = right-aligned blue, received = left-aligned dark.

**`InputBar.tsx`** — Agent target dropdown, CC/BCC toggle buttons + checkbox rows, text input (Enter to send), Send button. CC/BCC rows hide the current "To" target. Calls `POST /api/send`.

**`DiaryPanel.tsx`** — Container for tabs + diary stream.

**`DiaryTabs.tsx`** — "All" tab + one tab per agent. Active tab highlighted with accent color. Click filters diary to that agent.

**`DiaryEntry.tsx`** — Single diary line. Timestamp + agent tag (colored) + event type tag + content. Event type tags with background colors matching current CSS:
  - `diary` → green
  - `thinking` → yellow
  - `tool_call` → blue
  - `reasoning` → purple
  - `tool_result` → teal
  - `email_out` → light blue (with expandable body)
  - `email_in` → pink (with expandable body)
  - `cancel_received` → red
  - `cancel_diary` → orange

### Hooks

**`useAgents()`** — Fetches `/api/agents` once on mount. Returns `agents` array and lookup maps (`keyToName`, `addressToName`).

**`useInbox()`** — Polls `/api/inbox` every 1.5s. Maintains `receivedEmails` state. Also tracks locally `sentMessages` (added on send, before server confirms). Merges and sorts both lists.

**`useDiary(agentKeys: string[])`** — Polls `/api/diary/{key}?since=<last_ts>` for each agent every 1.5s using `Promise.all` (concurrent, not serial). Accumulates entries in state. Returns all entries sorted by timestamp. The `since` parameter avoids re-fetching the entire log each poll.

### Types (`types.ts`)

```typescript
interface AgentInfo {
  id: string;        // "alice"
  name: string;      // "Alice"
  key: string;       // "a"
  address: string;   // "127.0.0.1:8301"
  port: number;
  status: "active" | "sleeping";
}

interface Email {
  id: string;
  from: string;       // sender address
  to?: string[];      // extracted from payload "to" field
  cc?: string[];      // extracted from payload "cc" field
  subject: string;
  message: string;
  time: string;       // ISO timestamp — set by server on receipt
}

interface DiaryEvent {
  type: "diary" | "thinking" | "tool_call" | "reasoning" | "tool_result"
      | "email_out" | "email_in" | "cancel_received" | "cancel_diary" | "unknown";
  time: number;        // Unix timestamp
  agent_key: string;
  agent_name: string;
  // Type-specific fields
  text?: string;       // diary, thinking, reasoning (mapped from JSONL "reasoning"), cancel_diary, unknown
  tool?: string;       // tool_call, reasoning, tool_result
  args?: Record<string, unknown>;  // tool_call
  status?: string;     // tool_result
  to?: string;         // email_out (string — joined from list if needed)
  from?: string;       // email_in
  subject?: string;    // email_out, email_in, cancel_received
  message?: string;    // email_out, email_in
}
```

## Development Setup

### Backend
```bash
cd app/web
pip install -r requirements.txt   # fastapi, uvicorn
python run.py                     # Starts agents + API server on :8080
```

### Frontend
```bash
cd app/web/frontend
npm install
npm run dev                       # Vite dev server on :5173, proxies /api to :8080
```

### Production
```bash
cd app/web/frontend
npm run build                     # Outputs to dist/
# FastAPI serves dist/ as static files at "/"
```

### Vite Proxy Config
```typescript
// vite.config.ts
export default defineConfig({
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
});
```

## Data Flow

```
User types message
  → InputBar calls POST /api/send
  → FastAPI resolves agent key → port
  → TCPMailService.send() to agent TCP port (+ CC/BCC fan-out)
  → Agent processes request (LLM loop)
  → Agent writes to events.jsonl (via JSONLLoggingService)
  → useDiary polls GET /api/diary/{key}?since=X
  → DiaryPanel renders new entries

Agent emails user
  → TCPMailService callback → AppState.user_mailbox
  → useInbox polls GET /api/inbox
  → InboxPanel renders new email bubble
```

## What This Does NOT Do

- Does not modify `src/lingtai/` — purely a consumer of the lingtai API
- Does not add WebSocket (polling is sufficient for now, can upgrade later)
- Does not add runtime agent creation (agents defined in `run.py`, discovered via `/api/agents`)
- Does not add authentication or multi-user support
- Does not persist user mailbox to disk (in-memory only, same as current examples)
