# Karma Lifecycle Control — Design Spec

**Date:** 2026-03-21
**Status:** Draft
**Scope:** lingtai-kernel (primary), lingtai (secondary)

## Problem

Lifecycle control is scattered and asymmetric:
- **Kill** lives in mail intrinsic as a special message type — but it's not communication
- **Silence** also lives in mail as a special message type
- **Revive** doesn't exist — avatar has ad-hoc reactivation logic mixed with spawn logic
- **Purge** (delete agent entirely) doesn't exist
- Admin gates (`admin={"silence": True, "kill": True}`) are per-action instead of conceptual

Mail should be pure messaging. Lifecycle control should live in the system intrinsic.

## Design

### Admin Keys

Two admin keys replace the old per-action gates:

```python
admin={"karma": True}                  # silence, quell, revive
admin={"karma": True, "nirvana": True} # + annihilate
```

- **karma** (业) — the force of one's actions upon others. The 本我's actions on its 分身 shape the destiny of the entire agent network — and since the avatars are extensions of self, this is karma in the truest sense: your actions on others come back to affect you. Gates: silence, quell, revive.
- **nirvana** (涅槃) — authority to permanently release an agent from the cycle. Gates: annihilate.

Kernel defaults both to `False`. Lingtai sets `karma=True` for the 本我 (primary agent), `False` for 他我 (avatars). `nirvana` is always `False` unless explicitly granted.

The admin check is performed by the **caller** (self-check). The kernel trusts that callers with the admin key act in good faith. Humans can trigger the same operations directly (e.g., writing signal files or calling `start()`/`stop()` on the Python object).

### AgentState

```python
class AgentState(enum.Enum):
    ACTIVE  = "active"   # processing a message
    IDLE    = "idle"     # waiting for messages
    STUCK   = "stuck"    # error/timeout, AED attempting recovery
    DORMANT = "dormant"  # stopped, can be revived (was DEAD)
```

State transitions:
```
ACTIVE --(completed)--------> IDLE
ACTIVE --(timeout/exception)-> STUCK
IDLE   --(inbox message)----> ACTIVE
STUCK  --(AED)--------------> ACTIVE  (session reset, fresh run loop)
STUCK  --(AED timeout)------> DORMANT (shutdown)
ACTIVE/IDLE --(quell/shutdown)-> DORMANT
DORMANT --(revive)-----------> IDLE    (reconstructed from working dir)
DORMANT --(annihilate)-------> (ceases to exist)
```

`DEAD` is renamed to `DORMANT` — an agent that has been stopped (by self-shutdown, quell, or error) is dormant, not dead. It can be revived. Only annihilate truly destroys an agent.

### Mail Intrinsic — Pure Messaging

Mail loses all lifecycle control. The `type` field remains but `silence` and `kill` values are removed. Mail becomes purely communication:

- **send** — fire-and-forget message delivery
- **check** — list inbox with unread flags
- **read** — read message by ID
- **search** — regex search
- **delete** — remove message

The email capability (lingtai layer) continues to build on top of mail with reply, CC/BCC, contacts, archive, scheduled sends, etc. None of these are lifecycle operations.

### System Intrinsic — Karma/Nirvana Actions

The system intrinsic gains four new actions for lifecycle control over other agents:

| Action | Target | Admin Gate | Mechanism |
|--------|--------|------------|-----------|
| `show` | self | — | unchanged |
| `sleep` | self | — | unchanged |
| `shutdown` | self | — | unchanged |
| `restart` | self | — | unchanged |
| `silence` | other (by address) | `karma` | Write `.silence` signal file to target's working dir |
| `quell` | other (by address) | `karma` | Write `.quell` signal file to target's working dir |
| `revive` | other (by address) | `karma` | Reconstruct agent from working dir, call `start()` |
| `annihilate` | other (by address) | `nirvana` | Quell first if alive, then `shutil.rmtree(working_dir)` |

**Address format:** In the filesystem mail model, an address IS the working directory path. The system intrinsic uses the same address format — the path is passed directly to handshake functions.

**Self-targeting:** Karma/nirvana actions target **other** agents only. Self-lifecycle is handled by existing actions: `shutdown` (= self-quell) and `restart` (= self-rebirth). Self-annihilate is forbidden — an agent cannot delete its own working dir while running.

#### Silence (打断)

Interrupts a living agent's current work without stopping it.

1. Caller checks `admin.karma` — reject if not authorized.
2. Validate target: `handshake.is_agent(address)` and `handshake.is_alive(address)`.
3. Write `.silence` file to target's working dir.
4. Target's heartbeat loop detects `.silence`, sets `_cancel_event`, deletes the file.
5. Return `{"status": "silenced", "address": address}`.

#### Quell (沉寂)

Stops a living agent — it becomes dormant. Quell is the external equivalent of `shutdown`; the agent ends up in the same `DORMANT` state.

1. Caller checks `admin.karma` — reject if not authorized.
2. Validate target: `handshake.is_agent(address)` and `handshake.is_alive(address)`.
3. Write `.quell` file to target's working dir.
4. Target's heartbeat loop detects `.quell`, sets `_shutdown` (does NOT call `self.stop()` directly — that would deadlock the heartbeat thread). The run loop exits naturally when it sees `_shutdown`, and cleanup proceeds normally.
5. Return `{"status": "quelled", "address": address}`.

If the target is already dormant, return an error.

#### Revive (复活)

Restarts a dormant agent.

1. Caller checks `admin.karma` — reject if not authorized.
2. Validate target: `handshake.is_agent(address)`.
3. Validate target is dormant (heartbeat stale or absent).
4. Delegate to `_revive_agent(address)` hook — the hook handles full reconstruction and calls `start()` internally.
5. Return `{"status": "revived", "address": address}`.

Revive is **full reconstruction** — the same operation as creating and starting an agent for the first time. The caller reads the working dir, constructs a new agent object (with `LLMService`, capabilities, etc.), and calls `start()`. The `start()` method restores chat history and memory from the working dir. If the target is already alive, return an error.

**Revive extension point:** The kernel's system intrinsic cannot perform revive alone — it doesn't know how to construct an `Agent` with capabilities and LLM config. The kernel defines a hook `_revive_agent(address: str) -> BaseAgent | None` on `BaseAgent` (returns `None` by default). Lingtai's `Agent` overrides this to:
1. Read the persisted LLM config from the target's working dir.
2. Construct a new `Agent` with the appropriate `LLMService` and capabilities.
3. Call `start()` and return the agent.

If no override is provided (pure kernel usage), the system intrinsic returns an error explaining that revive requires a revive handler.

**Self-sufficient agents contract:** Each agent's working dir must contain everything needed to reconstruct and `start()` it, including LLM service configuration. The kernel does not prescribe the format — it requires that the working dir is self-sufficient. Lingtai is responsible for persisting model/provider info (e.g., writing the model config to the working dir at construction) so that `LLMService` can be reconstructed during revive.

#### Annihilate (湮灭)

Permanently removes an agent — released from the cycle.

1. Caller checks `admin.nirvana` — reject if not authorized.
2. Validate target: `handshake.is_agent(address)`.
3. If target is alive (heartbeat fresh), quell it first — write `.quell`, poll heartbeat staleness with a timeout of 10 seconds. If the agent fails to quell within the timeout, return an error (do not force-wipe a running agent).
4. `shutil.rmtree(working_dir)` — delete the entire working directory.
5. Return `{"status": "annihilated", "address": address}`.

Annihilate is irreversible. The agent's identity, memory, conversation history, and mailbox are all destroyed.

### Signal File Protocol

Living agents detect lifecycle signals via their heartbeat loop (already polling every ~1 second):

| File | Detected by | Action |
|------|-------------|--------|
| `.silence` | Heartbeat loop | Set `_cancel_event`, delete file |
| `.quell` | Heartbeat loop | Set `_shutdown` + `_cancel_event`, delete file |

Signal files are atomic — write the file, the agent detects it on the next heartbeat tick. No new threads or listeners needed.

Revive and annihilate do **not** use signal files — they are external actions performed by the caller on a dormant (or soon-to-be-dormant) agent.

### Handshake Utility (`lingtai_kernel/handshake.py`)

Extract the handshake logic currently duplicated in `FilesystemMailService.send()` into a standalone utility:

```python
def is_agent(path: str | Path) -> bool:
    """Check if .agent.json exists at path."""

def is_alive(path: str | Path, threshold: float = 2.0) -> bool:
    """Check if .agent.heartbeat is fresh (< threshold seconds, default 2)."""

def manifest(path: str | Path) -> dict:
    """Read and return .agent.json contents."""
```

Used by:
- `FilesystemMailService.send()` — existing handshake before mail delivery
- System intrinsic karma/nirvana actions — validate target before acting
- Lingtai's revive logic — read manifest to reconstruct agent

### Avatar Capability — Spawn + Ledger Only

Avatar strips all reactivation and status-checking logic. It becomes:

1. **Spawn** — create a new `Agent` with a unique working dir, register in ledger.
2. **Ledger** — append-only JSONL at `delegates/ledger.jsonl` recording spawn events.

If the requested name already exists as a live agent, return an error directing the caller to use mail (for communication) or system intrinsic (for lifecycle control).

Avatar no longer handles:
- Reactivation of idle agents (use mail to send a message)
- Status checking of existing agents (use system `show` or handshake utility)
- Error recovery guidance (use system `revive`)
- Cleanup of stopped agents (use system `annihilate`)

## Migration

### Backward-Incompatible Changes

1. `admin={"silence": True, "kill": True}` → `admin={"karma": True}`. Old keys no longer recognized.
2. `AgentState.DEAD` → `AgentState.DORMANT`. All code referencing `DEAD` must update.
3. Mail `type="kill"` and `type="silence"` removed. Mail `type` field stays for normal/email-capability use.
4. `_on_mail_received` no longer handles silence/kill routing.
5. Avatar's reactivation/status-checking logic removed.

### Files Changed

**lingtai-kernel:**
- `state.py` — `DEAD` → `DORMANT`
- `base_agent.py` — remove silence/kill from `_on_mail_received`, add signal file detection in heartbeat loop, update all `DEAD` references
- `intrinsics/system.py` — add silence, quell, revive, annihilate actions with admin gates
- `intrinsics/mail.py` — remove `silence`/`kill` from type enum and send handler
- `services/mail.py` — extract handshake logic to `handshake.py`, import from there
- `handshake.py` — new file, handshake utility
- `i18n/strings.go` (daemon) — update strings for new actions/states

**lingtai:**
- `capabilities/avatar.py` — strip to spawn + ledger only
- `capabilities/email.py` — remove kill/silence type handling from send
- `agent.py` — set `admin={"karma": True}` for 本我
- LLM config persistence — ensure model/provider info is written to agent working dir

**Tests:**
- `test_silence_kill.py` — rewrite for new signal file protocol and system intrinsic actions
- New tests for revive, annihilate, handshake utility, admin gate consolidation
