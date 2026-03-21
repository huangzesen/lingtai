# Three-Agent Email Example — Design Spec

**Date:** 2026-03-15
**File:** `examples/three_agents.py`
**Purpose:** Browser-based playground for testing email CC/BCC flows between three agents.

## Overview

A new example app following the `two_agents.py` pattern, extended to three agents with CC/BCC support in the compose UI. Single self-contained Python file with embedded HTML/CSS/JS.

## Backend

### Ports

| Component | Port |
|-----------|------|
| User mailbox | TCP 8300 |
| Alice | TCP 8301 |
| Bob | TCP 8302 |
| Charlie | TCP 8303 |
| Web UI | HTTP 8080 |

### LLM

MiniMax (`MiniMax-M2.5-highspeed`), same as `two_agents.py`. Reads `MINIMAX_API_KEY` from env / `.env`.

### Agents

Three identical agents, each with:
- `TCPMailService` on their respective port
- `MemoryLoggingService` (keys: `a`, `b`, `c`)
- `add_capability("email")`
- `AgentConfig(max_turns=10)`
- Same generic prompt (proactive, email-only communication, no courtesy loops)
- Role section with name, address, and contacts list (other two agents + user)

### HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serve HTML page |
| GET | `/inbox` | Return user mailbox (received emails) |
| GET | `/diary` | Return agent activity logs, keyed by `a`, `b`, `c` |
| POST | `/send` | Send email to agent(s) |

#### POST /send payload

```json
{
  "agent": "a",
  "message": "hello",
  "cc": ["b"],
  "bcc": ["c"]
}
```

- `agent`: required, single target (`"a"`, `"b"`, or `"c"`)
- `message`: required, email body
- `cc`: optional, array of agent keys — mapped to addresses, included in email payload
- `bcc`: optional, array of agent keys — mapped to addresses, included in email payload

The handler uses a standalone `TCPMailService()` (send-only, no listen_port) and calls `sender.send()` directly — the user is not an agent and has no EmailManager. The handler must replicate the fan-out and BCC-stripping logic from `EmailManager._send()`:

1. Build a base payload: `{from, to, cc, subject, message}` — **no `bcc` field** in the wire payload
2. Send the base payload to the primary `to` address
3. Send the base payload to each CC address
4. Send the base payload to each BCC address (they receive it but are not listed anywhere)

```python
# Pseudocode
base = {"from": user_addr, "to": [to_addr], "subject": "", "message": msg}
if cc_addrs:
    base["cc"] = cc_addrs
for addr in [to_addr] + cc_addrs + bcc_addrs:
    sender.send(addr, base)
```

## Frontend

### Layout

Same split as `two_agents.py`:
- Left (flex:2): Inbox panel with compose bar
- Right (flex:1): Diary panel with agent tabs

### Diary Panel

Tabs at the top: **All | Alice | Bob | Charlie**

- "All" shows interleaved entries from all agents, sorted by time (default)
- Agent tabs filter to that agent's entries only
- Color tags: Alice = red (`#e94560`, class `alice`), Bob = teal (`#4ecdc4`, class `bob`), Charlie = amber (`#f0a500`, class `charlie`)

### Compose Bar

Bottom of inbox panel, left to right:
1. **To:** dropdown — Alice / Bob / Charlie (single select)
2. **CC** button — toggles a checkbox row showing the two agents not in To
3. **BCC** button — toggles a checkbox row showing the two agents not in To
4. **Message input** — text field
5. **Send button**

CC/BCC rows appear below the main compose line when toggled. Checkboxes update dynamically when the To selection changes (exclude the To target from CC/BCC options). If a checked agent becomes the To target, uncheck and hide them. An agent cannot appear in both CC and BCC — BCC takes precedence if both are somehow checked (unlikely with the UI, but handle it).

### Inbox Display

Emails show:
- Sent: "To: Alice" (+ "CC: Bob, Charlie" if CC was used). The `sentMessages` array stores `{to, cc, text, time}`.
- Received: "From: Alice" with subject, CC field if present. The `on_user_mail` callback captures `from`, `to`, `cc`, `subject`, `message` from the payload.
- BCC is never displayed (blind by design)

#### GET /inbox response

```json
{
  "emails": [
    {"id": "...", "from": "127.0.0.1:8301", "to": ["127.0.0.1:8300"], "cc": ["127.0.0.1:8302"], "subject": "...", "message": "...", "time": "..."}
  ]
}
```

## File Structure

Single file: `examples/three_agents.py`. No external dependencies beyond lingtai and its optional `minimax` extra.

## Notes

- Uses same ports as `two_agents.py` (8300-8302 + 8080). Only run one at a time.
- Shutdown sequence: `server.shutdown()`, `user_mail.stop()`, then `agent.stop(timeout=5.0)` for all three agents.
- Each agent's contacts list has 3 entries: two other agents + user.

## Non-Goals

- No distinct agent specializations (all generic)
- No WebSocket (polling at 1.5s, same as existing)
- No authentication or persistence
- No refactoring of `two_agents.py`
