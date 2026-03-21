# Working Directory by agent_id — Design Spec

**Date:** 2026-03-20
**Scope:** lingtai-kernel (primary), lingtai eigen intrinsic
**Status:** Draft

## Problem

`agent_name` is currently coupled to the filesystem path (`{base_dir}/{agent_name}/`), making it effectively immutable. The name is a human-facing alias — it should be changeable without breaking the working directory, mail, or avatar resume logic.

## Design Principles

- **agent_id** is identity — 12-char hex UUID, auto-generated, stable, used for filesystem paths
- **agent_name** is a true name (真名) — set once, never changed. The only way to "rename" is reincarnation (avatar into same folder = same agent_id, new name)
- **本我 (self)**: root agent names itself via eigen `name` action
- **他我 (other)**: parent names child at avatar spawn time (future — not in this spec)
- Names are i18n-friendly — no ASCII restriction, since names never touch the filesystem

## Changes

### 1. WorkingDir (lingtai-kernel)

**File:** `lingtai_kernel/workdir.py`

- Constructor signature: `__init__(self, base_dir: Path | str, agent_id: str)`
- Validation: `agent_id` must match `[0-9a-f]{12}`
- Path construction: `self._path = self._base_dir / agent_id`
- Remove `_agent_name` attribute, replace with `_agent_id`

### 2. BaseAgent (lingtai-kernel)

**File:** `lingtai_kernel/base_agent.py`

- `agent_name` parameter becomes optional, defaults to `None`
- `agent_id` generation moves **before** WorkingDir construction
- WorkingDir receives `agent_id` instead of `agent_name`
- New public method: `set_name(name: str) -> None`
  - Validates: non-empty string
  - Set-once: raises `RuntimeError` if `agent_name` is already set
  - Persists name to `.agent.json` manifest (the manifest is the source of truth for agent identity on disk)
  - Updates system prompt identity section
- Manifest (`.agent.json`): `agent_name` field can be `None` initially, populated by `set_name()`. On avatar reincarnation (future), the new agent reads this manifest to inherit identity.
- System prompt identity: shows name if set, otherwise shows only agent_id
- Thread name: `f"agent-{self.agent_id}"` (was `f"agent-{self.agent_name}"`)

### 3. Eigen Intrinsic — `name` action (lingtai-kernel)

**File:** `lingtai_kernel/intrinsics/eigen.py`

- New object+action: `name` / `set`
- Schema: `{"object": "name", "action": "set", "content": "<string>"}` (consistent with eigen's existing object/action/content pattern)
- Handler: calls `agent.set_name(content.strip())`
- Returns `{"status": "ok", "name": "<name>"}`, or `{"error": "..."}` if already named or empty
- Description in schema: "Your true name — set once, never changed."

### 4. SessionManager (lingtai-kernel)

**File:** `lingtai_kernel/session.py`

- `agent_name` parameter stays (for logging), but accepts `None`
- Logging falls back to `agent_id` when `agent_name` is `None`
- No structural changes

### 5. Tests

- All path assertions change from `base_dir / "agent_name"` to `base_dir / agent.agent_id`
- New tests:
  - `test_set_name_once` — set name succeeds
  - `test_set_name_twice_fails` — second set raises RuntimeError
  - `test_set_name_empty_fails` — empty string rejected
  - `test_eigen_name_action` — eigen handler calls set_name correctly
  - `test_workdir_uses_agent_id` — directory named by agent_id

## Not In Scope

- **Avatar reincarnation** — spawning into existing folders, parent-given names. Separate design.
- **Mail routing changes** — stays TCP address-based. Name changes are the agent's social problem, governed by covenant.
- **Wizard changes** — follow-up after kernel lands. "Agent Name" becomes optional cosmetic label.
- **Name-based lookup/registry** — future concern for avatar and network discovery.

## Migration

No backward compatibility needed. This is a breaking change to the kernel API:
- `WorkingDir(base_dir, agent_name=...)` → `WorkingDir(base_dir, agent_id=...)`
- `BaseAgent(agent_name, ...)` → `BaseAgent(..., agent_name=None)` (agent_name moves to keyword-only, optional)
- Existing agent directories (named by agent_name) are abandoned — agents start fresh with id-named dirs

## Constructor Signature After

```python
class BaseAgent:
    def __init__(
        self,
        service: LLMService,
        *,
        agent_name: str | None = None,
        file_io: Any | None = None,
        mail_service: Any | None = None,
        config: AgentConfig | None = None,
        base_dir: str | Path,
        context: Any = None,
        admin: dict | None = None,
        streaming: bool = False,
        covenant: str = "",
        memory: str = "",
    ):
```

Note: `agent_name` moves from positional to keyword-only, and becomes optional. This is a breaking API change — all callers must update.
