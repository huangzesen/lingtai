# Filesystem-Based Mail: Path as Address

**Date**: 2026-03-21
**Status**: Draft
**Scope**: lingtai-kernel + lingtai + Go daemon

## Problem

Mail addresses are currently TCP ports (`127.0.0.1:8501`). Ports are ephemeral — they change across restarts, can conflict, and make contacts stale immediately. The entire mail system depends on active TCP connections, meaning offline agents can't receive mail.

## Core Design Principle

**The filesystem path IS the address.** Delivering mail = writing a file to a directory. TCP is removed entirely for local communication. Remote agents are a future concern (IMAP), not part of this design.

## Address Format

The address is the agent's **working directory** — the identity root:

```
/Users/huangzesen/agents/a1b2c3d4e5f6
```

Each mail service defines its own relative mailbox path within the working directory:

| Service | Relative path | Inbox path |
|---------|--------------|------------|
| Mail intrinsic (kernel) | `mailbox/` | `{address}/mailbox/inbox/{uuid}/` |
| Email capability (lingtai) | `email/` | `{address}/email/inbox/{uuid}/` |

Mail-to-mail, email-to-email. Bridging between services is not lingtai's concern.

## Handshake on Send (Identity + Health Check)

Before writing to a recipient's inbox, the sender verifies the destination AND that the recipient is alive:

1. Read `{address}/.agent.json` — verify it exists, is valid JSON
2. If the recipient is in the sender's contacts, verify `agent_id` matches the contact's stored `agent_id`. If the recipient is NOT in contacts (e.g., replying to an unknown sender from a `from` field), skip `agent_id` verification — the `.agent.json` existence check alone is sufficient.
3. Read `{address}/.agent.heartbeat` — verify the timestamp is within 2 seconds of current time. If missing or stale → agent is dead.
4. If all checks pass → write to `{address}/{mailbox_rel}/inbox/{uuid}/`

Failure cases:
- `.agent.json` missing → error: "no agent at this address"
- `agent_id` mismatch (contact exists) → error: "agent at this address has changed"
- `.agent.heartbeat` missing or stale (>2s old) → error: "agent is not running"

This makes local mail a health-check protocol — agents always know whether peers are alive at the point of communication.

## Heartbeat

The `.agent.heartbeat` file is a plain-text UTC timestamp, written by the agent's **main loop** (~1s interval). It serves as the sole liveness signal — no PID checks, no lock probing, fully cross-platform.

```
{working_dir}/.agent.heartbeat    ← "2026-03-21T10:00:00.500000Z"
```

- **Writer**: the agent's `run_loop` — written at natural loop points (after LLM response, after tool execution, during `system.sleep`). NOT written by the mail polling thread.
- **Reader**: any sender reads this file and compares against current UTC time. Alive if `now - heartbeat < 2s`.
- **Startup**: first heartbeat written when the agent's main loop begins.
- **Clean shutdown**: `stop()` deletes the file.
- **Crash / stuck**: heartbeat goes stale — the 2s threshold catches this naturally. A brain-dead agent (polling thread alive but main loop stuck) correctly shows as dead.
- **Human participant**: the Go daemon writes the human's heartbeat on its own tick.

**Key design choice**: heartbeat comes from the main loop, not the polling thread. This ensures that an agent whose `run_loop` has failed or gotten stuck is correctly reported as dead, even if the inbox polling thread is still running. The polling thread can still receive `kill` mail to force-terminate the process.

## Message Delivery

Sending mail = writing files to the recipient's inbox directory:

```
{recipient_address}/{mailbox_rel}/inbox/{uuid}/
├── message.json      ← message payload
└── attachments/      ← actual files (no base64)
    ├── report.pdf
    └── image.png
```

**message.json payload** (same fields as today, addresses become paths):

```json
{
  "from": "/Users/huangzesen/agents/b2c3d4e5f6a1",
  "to": "/Users/huangzesen/agents/a1b2c3d4e5f6",
  "subject": "Hello",
  "message": "...",
  "type": "normal",
  "_mailbox_id": "uuid",
  "received_at": "2026-03-21T10:00:00Z"
}
```

**Attachments** are real files — copied or moved into `attachments/`. No more base64 encoding over TCP. This makes large files, binary data, images trivially simple.

**Self-send** (notes to self): write to own inbox path. No special case.

## FilesystemMailService (Kernel)

Replaces `TCPMailService` entirely. Lives in `lingtai_kernel.services.mail`.

### Constructor

```python
FilesystemMailService(working_dir: str | Path, mailbox_rel: str = "mailbox")
```

- `working_dir`: the agent's working directory (the address)
- `mailbox_rel`: relative path for this service's mailbox within the working dir

### ABC

```python
class MailService(ABC):
    @property
    @abstractmethod
    def address(self) -> str: ...          # own working dir path (always set, never None)

    @abstractmethod
    def send(self, address: str, payload: dict) -> str | None: ...
             # returns None on success, error string on failure

    @abstractmethod
    def listen(self, on_message: Callable[[dict], None]) -> None: ...

    @abstractmethod
    def stop(self) -> None: ...
```

The `address` property is narrowed from `str | None` to `str` — a filesystem mail service always has an address (its working dir).

Attachments are listed in the `payload` dict under `"attachments"` (list of file paths). The `send()` implementation reads these paths and copies the files into the recipient's inbox. This preserves the existing calling convention in the mail intrinsic and email capability — no caller changes needed.

### Implementation

- **`send(address, payload)`**: reads `{address}/.agent.json` for handshake verification (see Handshake on Send), creates `{address}/{mailbox_rel}/inbox/{uuid}/`, writes `message.json`. If `payload["attachments"]` contains file paths, copies those files into the `attachments/` subfolder. Returns `None` on success, error string on failure.
- **`listen(on_message)`**: starts a daemon thread that polls own `{mailbox_rel}/inbox/` for new message directories. Tracks seen UUIDs in a `set`. On new message: reads `message.json`, calls `on_message(payload)`. Loads existing UUIDs on startup to avoid re-notifying old messages.
- **`stop()`**: signals the polling thread to exit.
- **`address`** property: returns `str(working_dir)`.

### Poll interval

~0.5 seconds (configurable). The polling thread is lightweight — just listing a directory for new entries. Platform-native filesystem watchers (`inotify`/`FSEvents`) are a future optimization, not a current requirement.

## Notification Mechanism

Same as today. When the polling thread detects a new message, it calls `on_message(payload)` which triggers `BaseAgent._on_mail_received()`. This injects a `[system]` notification into the agent's LLM conversation, exactly as the TCP listener callback does now.

No change to `BaseAgent._on_mail_received()`, `_on_normal_mail()`, or the mail type routing (`normal`/`silence`/`kill`).

## Human as First-Class Participant

Humans get a working directory with the same structure as agents. The only distinguishing marker: `admin: null` in `.agent.json`.

**Human's `.agent.json`**:
```json
{
  "agent_id": "human_zesen",
  "agent_name": "Zesen",
  "started_at": "...",
  "working_dir": "/Users/huangzesen/agents/human_zesen",
  "admin": null,
  "language": "zh",
  "address": "/Users/huangzesen/agents/human_zesen"
}
```

Liveness is determined by `.agent.heartbeat`, not by any field in `.agent.json`. The Go daemon writes the human's heartbeat via the same polling loop that monitors the human's inbox.

**Human's directory structure** (same as agent):
```
{base_dir}/human_zesen/
├── .agent.json
├── mailbox/
│   ├── inbox/
│   ├── sent/
│   ├── contacts.json
│   └── read.json
└── ...
```

The TUI reads/writes the human's mailbox directory. When the user types a message, the TUI writes to the agent's inbox. When the agent replies, it writes to the human's inbox. The TUI polls the human's inbox for new messages.

## Contact Structure

```json
{
  "address": "/Users/huangzesen/agents/a1b2c3d4e5f6",
  "name": "Alice",
  "agent_id": "a1b2c3d4e5f6",
  "note": ""
}
```

- `address`: the agent's working directory (full path)
- `agent_id`: used for handshake verification on send
- Agents learn addresses through **introduction only** — no auto-discovery, no scanning

## Agent Discovery

Explicit introduction only. Agents know who they know.

Introduction happens when:
- The daemon starts an agent — it introduces the human and agent to each other by writing into both `contacts.json` files
- The host app configures contacts at construction

Receiving mail from an unknown sender (address not in contacts) does NOT auto-add them to contacts. The `from` field is visible in the message, so the agent can see the sender's address — but adding to contacts is a deliberate action (via the email capability's contact management or programmatically by the host).

No scanning of `base_dir`, no registry, no discovery protocol.

## Daemon/TUI Changes

### Startup flow (revised)

1. Read config → get `base_dir` and agent settings
2. Create human working directory at `{base_dir}/human_{id}/` if it doesn't exist
3. Write human's `.agent.json`
4. Start Python agent process
5. Wait for agent's `.agent.json` to appear (instead of `WaitForPort`)
6. Exchange introductions — write human's address into agent's `contacts.json` and agent's address into human's `contacts.json`

### TUI communication

- **Sending**: write `message.json` to `{agent_address}/mailbox/inbox/{uuid}/`, record in human's `mailbox/sent/`
- **Receiving**: poll `{human_mailbox}/inbox/` for new message directories, display in chat view

### Deleted from Go daemon

- `MailClient`, `MailListener` (TCP classes in `mail.go`)
- `WaitForPort` logic
- Port-related config for mail (`agent_port` may still exist for process management)

### Replaced with

- Go filesystem operations: `os.MkdirAll`, `os.WriteFile`, directory polling
- A small Go package for reading/writing the lingtai mailbox format (JSON message files)

## Impact on Existing Code

### Kernel (`lingtai-kernel`)

| Component | Change |
|-----------|--------|
| `TCPMailService` | **Deleted** — replaced by `FilesystemMailService` |
| `MailService` ABC | Simplified — no port, no banner, path-based |
| Mail intrinsic | Minimal — `_mailman` calls `mail_service.send(path, ...)` instead of TCP send |
| `BaseAgent` | No port params in constructor. `_on_mail_received` unchanged |
| `_build_manifest()` | `address` field now holds working dir path (already dynamic) |

### Lingtai

| Component | Change |
|-----------|--------|
| Email capability | Minimal — address format changes, attachment handling simplified. `mailbox_rel="email"` |
| All other capabilities | No changes |

### Go Daemon

| Component | Change |
|-----------|--------|
| `mail.go` | **Rewritten** — filesystem read/write instead of TCP |
| `config/loader.go` | Remove port-based mail config |
| `tui/app.go` | Filesystem-based send/receive |
| Agent startup | Wait for `.agent.json` instead of `WaitForPort` |

## Migration

Clean break. No backward compatibility layer.

- Old contacts with `host:port` addresses: invalid, agents re-introduced on next startup
- Old inbox messages with TCP addresses: stale history, left as-is
- Existing agent sessions: nuked (user confirmed)

## What This Enables

- **Offline delivery**: mail queues up in inbox even when recipient isn't running
- **Trivial attachments**: real files, no base64, no size limits
- **Debuggable**: `ls` an agent's inbox, `cat` a message
- **Stable addresses**: paths don't change across restarts
- **Human-agent parity**: same mailbox structure, same protocol, same tools
- **No port conflicts**: filesystem paths don't collide
