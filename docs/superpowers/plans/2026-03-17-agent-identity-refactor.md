# Agent Identity Refactor Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `agent_id` an auto-generated UUID (globally unique, never collides), add `agent_name` as the human-readable label. Working directory uses `agent_name`. All communication and logging uses `agent_id`.

**Architecture:** `BaseAgent.__init__` auto-generates `agent_id` as a 12-char hex UUID. New required parameter `agent_name` replaces the old `agent_id` parameter. `WorkingDir` uses `agent_name` for the directory name. Billboard, manifest, discovery, and JSONL logs all carry both fields. Remove `instance_id` (agent_id IS the instance id now).

**Tech Stack:** Python 3.11+, lingtai framework

---

## Key Design Decisions

- `agent_id` = `uuid4().hex[:12]` — 12 hex chars, auto-generated, immutable
- `agent_name` = human label (e.g. "alice") — provided by caller, used for working dir
- Working dir = `{base_dir}/{agent_name}/` — NOT `{agent_name}_{agent_id}/` (decided against that)
- `WorkingDir` validation regex applies to `agent_name`, not `agent_id`
- JSONL logs carry both `agent_id` and `agent_name`
- TCP banner: `STOAI {agent_id}\n` (hash, not name — globally unique)
- Billboard file: `{agent_id}.json` (hash, not name — globally unique)
- `agent.agent_id` stays as the public attribute (UUID now)
- `agent.agent_name` is new public attribute

## File Structure

| Action | Path | What Changes |
|--------|------|-------------|
| Modify | `src/lingtai/base_agent.py` | Constructor: `agent_name` replaces `agent_id` param, auto-generate `agent_id`. Add `agent_name` attribute. Update manifest, billboard, discovery, logging. |
| Modify | `src/lingtai/workdir.py` | Constructor takes `agent_name` instead of `agent_id`. Path = `{base_dir}/{agent_name}/`. Rename `_agent_id` to `_agent_name`. |
| Modify | `src/lingtai/session.py` | Constructor takes both `agent_id` and `agent_name`. |
| Modify | `src/lingtai/agent.py` | Pass-through: forward `agent_name` to `BaseAgent.__init__`. |
| Modify | `src/lingtai/intrinsics/status.py` | Return both `agent_id` and `agent_name` in status show. |
| Modify | `src/lingtai/intrinsics/mail.py` | Fallback sender uses `agent_id` (not name). |
| Modify | `src/lingtai/capabilities/email.py` | Fallback sender/receiver uses `agent_id`. |
| Modify | `src/lingtai/capabilities/delegate.py` | Child naming: `agent_name=f"{parent.agent_name}_delegate_{port}"`. Return both id and name. |
| Modify | `src/lingtai/services/mail.py` | Banner uses `agent_id` (already does via `_banner_id`). |
| Modify | `app/web/server/state.py` | `register_agent` takes `agent_name`. `AgentEntry` has both fields. |
| Modify | `app/web/server/routes.py` | `/api/agents` returns both `id` and `name`. |
| Modify | `app/web/run.py` | Pass `agent_name` instead of `agent_id`. |
| Modify | `examples/two_agents.py` | `agent_name=` instead of `agent_id=`. |
| Modify | `examples/three_agents.py` | `agent_name=` instead of `agent_id=`. |
| Modify | `examples/chat_agent.py` | `agent_name=` instead of `agent_id=`. |
| Modify | `examples/chat_web.py` | `agent_name=` instead of `agent_id=`. |
| Modify | `examples/orchestration/__main__.py` | `agent_name=` instead of `agent_id=`. |
| Modify | All test files (15+) | `agent_name=` instead of `agent_id=` in constructors. |
| Modify | `CLAUDE.md` | Update documentation references. |

---

## Chunk 1: Core — WorkingDir + BaseAgent + SessionManager

### Task 1: Refactor WorkingDir to use agent_name

**Files:**
- Modify: `src/lingtai/workdir.py`

- [ ] **Step 1: Rename constructor param and internal field**

Change `WorkingDir.__init__`:
- Parameter: `agent_id: str` → `agent_name: str`
- Field: `self._agent_id` → `self._agent_name`
- Path: `self._path = self._base_dir / agent_name`
- Validation: regex applies to `agent_name`
- Rename `_AGENT_ID_RE` to `_AGENT_NAME_RE`

- [ ] **Step 2: Smoke-test**

```bash
python -c "from lingtai.workdir import WorkingDir; print('OK')"
```

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/workdir.py
git commit -m "refactor: WorkingDir uses agent_name instead of agent_id"
```

---

### Task 2: Refactor BaseAgent constructor

**Files:**
- Modify: `src/lingtai/base_agent.py`

This is the biggest single change. The constructor signature changes from `agent_id: str` to `agent_name: str`, and `agent_id` becomes auto-generated.

- [ ] **Step 1: Update constructor**

```python
def __init__(
    self,
    agent_name: str,    # was: agent_id: str
    service: LLMService,
    *,
    # ... rest unchanged ...
):
    import uuid as _uuid
    self.agent_name = agent_name
    self.agent_id = _uuid.uuid4().hex[:12]  # auto-generated
    # ... rest uses agent_name for WorkingDir, agent_id for identity ...
```

Key changes in `__init__`:
- `self.agent_id = _uuid.uuid4().hex[:12]` (auto-generated)
- `self.agent_name = agent_name` (new public attribute)
- `WorkingDir(base_dir=base_dir, agent_name=agent_name)` (was `agent_id=agent_id`)
- `SessionManager(..., agent_id=self.agent_id, agent_name=agent_name)` (pass both)
- Manifest: include both `agent_id` and `agent_name`, remove `instance_id`
- Billboard: filename = `{self.agent_id}.json` (was `instance_id`)
- Banner: `self._mail_service._banner_id = self.agent_id` (unchanged — already correct)
- Discovery info: include both fields, remove `instance_id`
- Thread name: `f"agent-{self.agent_name}"` (human-readable)
- Logger messages: `f"[{self.agent_name}]"` (human-readable for debugging)

- [ ] **Step 2: Update `_log()` method**

Add `agent_name` to every JSONL event:
```python
def _log(self, event_type: str, **fields) -> None:
    if self._log_service:
        self._log_service.log({
            "type": event_type,
            "agent_id": self.agent_id,
            "agent_name": self.agent_name,
            "ts": time.time(),
            **fields,
        })
```

- [ ] **Step 3: Update `stop()` method**

- Manifest write: include both `agent_id` and `agent_name`
- Billboard cleanup: use `self.agent_id` for filename

- [ ] **Step 4: Update `_get_discovery_info()`**

```python
def _get_discovery_info(self) -> dict:
    info = {
        "_lingtai": "agent",
        "agent_id": self.agent_id,
        "agent_name": self.agent_name,
        # ... rest same ...
    }
```

- [ ] **Step 5: Update `send()` public method**

The `send()` method returns a result dict. Update it to include `agent_name` if it uses `agent_id`.

- [ ] **Step 6: Smoke-test**

```bash
python -c "from lingtai.base_agent import BaseAgent; print('OK')"
```

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/base_agent.py
git commit -m "refactor: BaseAgent auto-generates agent_id, adds agent_name"
```

---

### Task 3: Update SessionManager

**Files:**
- Modify: `src/lingtai/session.py`

- [ ] **Step 1: Add agent_name parameter**

SessionManager constructor gets both `agent_id` (UUID) and `agent_name`:
- `agent_id` used for internal identity
- `agent_name` used for LLM adapter `agent_name=` / `agent_type=` params and log messages

- [ ] **Step 2: Commit**

```bash
git add src/lingtai/session.py
git commit -m "refactor: SessionManager accepts both agent_id and agent_name"
```

---

### Task 4: Update Agent subclass

**Files:**
- Modify: `src/lingtai/agent.py`

- [ ] **Step 1: Change constructor to forward agent_name**

`Agent.__init__` passes `agent_name` to `super().__init__()` instead of `agent_id`.

- [ ] **Step 2: Smoke-test**

```bash
python -c "from lingtai import Agent; print('OK')"
```

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/agent.py
git commit -m "refactor: Agent forwards agent_name to BaseAgent"
```

---

## Chunk 2: Intrinsics + Capabilities

### Task 5: Update intrinsics

**Files:**
- Modify: `src/lingtai/intrinsics/status.py`
- Modify: `src/lingtai/intrinsics/mail.py`

- [ ] **Step 1: Update status intrinsic**

`status show` returns both `agent_id` and `agent_name`:
```python
"agent_id": agent.agent_id,
"agent_name": agent.agent_name,
```

- [ ] **Step 2: Update mail intrinsic**

Fallback sender: `agent._mail_service.address or agent.agent_id` — this is already correct (agent_id is now the UUID, which is fine as a fallback identifier).

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/intrinsics/
git commit -m "refactor: intrinsics use agent_name + agent_id"
```

---

### Task 6: Update capabilities

**Files:**
- Modify: `src/lingtai/capabilities/email.py`
- Modify: `src/lingtai/capabilities/delegate.py`

- [ ] **Step 1: Update email capability**

Fallback sender/receiver: uses `agent.agent_id` (UUID) — already correct for identity. No functional change needed, just verify.

- [ ] **Step 2: Update delegate capability**

Child naming:
```python
child_name = f"{parent.agent_name}_delegate_{port}"
```
Constructor:
```python
delegate = Agent(agent_name=child_name, ...)
```
Return dict:
```python
{"agent_id": delegate.agent_id, "agent_name": delegate.agent_name, ...}
```

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/capabilities/
git commit -m "refactor: capabilities use agent_name"
```

---

## Chunk 3: Web Dashboard

### Task 7: Update web dashboard

**Files:**
- Modify: `app/web/server/state.py`
- Modify: `app/web/server/routes.py`
- Modify: `app/web/server/diary.py`
- Modify: `app/web/run.py`

- [ ] **Step 1: Update state.py**

`AgentEntry`: add `agent_name` field.
`register_agent()`: takes `agent_name` instead of `agent_id`, passes to `Agent(agent_name=...)`.

- [ ] **Step 2: Update routes.py**

`/api/agents` response includes both `id` (agent_id UUID) and `name` (agent_name):
```python
{
    "id": entry.agent.agent_id,
    "name": entry.agent_name,
    ...
}
```

`/api/diary/{agent_key}` — the diary endpoint reads JSONL files from `working_dir`, which is based on `agent_name`. No change needed since `AgentEntry.working_dir` is already correct.

- [ ] **Step 3: Update diary.py**

JSONL events now have both `agent_id` and `agent_name`. The diary parser doesn't use these fields (it's given the agent key externally), so no change needed.

- [ ] **Step 4: Update run.py**

Change `agent_id=a["id"]` to `agent_name=a["id"]` (the config dict value "alice" is a name, not an id now). Also rename the config key from `"id"` to `"name"` for clarity.

- [ ] **Step 5: Rebuild frontend**

```bash
cd app/web/frontend && npm run build
```

No frontend code changes needed — the API response field names (`id`, `name`) stay the same.

- [ ] **Step 6: Commit**

```bash
git add app/web/
git commit -m "refactor(web): use agent_name in dashboard"
```

---

## Chunk 4: Examples

### Task 8: Update all examples

**Files:**
- Modify: `examples/two_agents.py`
- Modify: `examples/three_agents.py`
- Modify: `examples/chat_agent.py`
- Modify: `examples/chat_web.py`
- Modify: `examples/orchestration/__main__.py`

- [ ] **Step 1: Update each example**

In all examples, change constructor calls:
```python
# Before:
Agent(agent_id="alice", ...)
# After:
Agent(agent_name="alice", ...)
```

For `two_agents.py` and `three_agents.py`, the diary endpoint uses `agent_ids = {"a": "alice", ...}` to map keys to working dir names. Rename to `agent_names`:
```python
agent_names = {"a": "alice", "b": "bob"}
for key, agent_name in agent_names.items():
    log_file = base_dir / agent_name / "logs" / "events.jsonl"
```

- [ ] **Step 2: Smoke-test import**

```bash
python -c "from examples.two_agents import main; print('OK')"
```

- [ ] **Step 3: Commit**

```bash
git add examples/
git commit -m "refactor: examples use agent_name"
```

---

## Chunk 5: Tests

### Task 9: Update all test files

**Files:**
- Modify: All test files that construct `BaseAgent` or `Agent`

Every test that does `BaseAgent(agent_id="test", ...)` must change to `BaseAgent(agent_name="test", ...)`.

Every assertion that checks `agent.agent_id` expecting a human name must either:
- Check `agent.agent_name` instead, OR
- Check that `agent.agent_id` is a 12-char hex string

- [ ] **Step 1: Bulk rename constructor parameter across all test files**

Use search-and-replace: `agent_id=` → `agent_name=` in all `BaseAgent(` and `Agent(` constructor calls.

Affected files:
- `tests/test_agent.py`
- `tests/test_workdir.py`
- `tests/test_status.py`
- `tests/test_clock.py`
- `tests/test_cancel_email.py`
- `tests/test_three_agent_email.py`
- `tests/test_intrinsics_comm.py`
- `tests/test_anima.py`
- `tests/test_layers_email.py`
- `tests/test_layers_bash.py`
- `tests/test_layers_draw.py`
- `tests/test_agent_capabilities.py`
- `tests/test_layers_delegate.py`
- `tests/test_system.py`
- `tests/test_override_intrinsic.py`
- `tests/test_git_init.py`
- `tests/test_vision_capability.py`
- `tests/test_web_search_capability.py`
- `tests/test_layers_file.py`
- `tests/test_conscience.py`

- [ ] **Step 2: Fix assertions**

Tests that assert `agent.agent_id == "alice"` should change to `agent.agent_name == "alice"`.
Tests that assert `data["agent_id"]` in manifest/status should also check `data["agent_name"]`.

Key assertions to update:
- `test_agent.py` line 376: `assert s["agent_id"] == "test"` → check `agent_name`
- `test_agent.py` line 420: `assert data["agent_id"] == "alice"` → check `agent_name`
- `test_status.py` line 41: `assert identity["agent_id"] == "alice"` → also check `agent_name`
- `test_layers_delegate.py` line 30: `assert "agent_id" in result` → keep (agent_id still exists, now UUID)

- [ ] **Step 3: Fix WorkingDir tests**

`test_workdir.py` — rename constructor param from `agent_id=` to `agent_name=`:
```python
WorkingDir(base_dir=tmp_path, agent_name="test")
```

Fix validation test:
```python
def test_invalid_agent_name_raises(tmp_path):
    with pytest.raises(ValueError):
        WorkingDir(base_dir=tmp_path, agent_name="bad name!")
```

- [ ] **Step 4: Run all tests**

```bash
python -m pytest tests/ -x -q
```

Fix any remaining failures iteratively.

- [ ] **Step 5: Commit**

```bash
git add tests/
git commit -m "refactor: tests use agent_name"
```

---

## Chunk 6: Documentation

### Task 10: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update all references**

Replace mentions of `agent_id` parameter with `agent_name` in:
- Architecture section
- Extension pattern code examples
- Key modules descriptions
- Any API signatures shown

Add note about `agent_id` being auto-generated UUID.

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for agent_name refactor"
```

---

## Chunk 7: Final verification

### Task 11: Full test suite + smoke test

- [ ] **Step 1: Run full test suite**

```bash
python -m pytest tests/ -q
```

Expected: all tests pass (except pre-existing failures unrelated to this refactor).

- [ ] **Step 2: Smoke-test web dashboard**

```bash
python -m app.web
```

Verify:
- `curl http://localhost:8080/api/agents` returns agents with both `id` (UUID) and `name`
- Billboard files appear in `~/.lingtai/billboard/` with UUID filenames
- Agent working dirs are named by `agent_name` (e.g. `alice/`, not UUID)

- [ ] **Step 3: Verify JSONL log format**

Check an agent's `logs/events.jsonl` — each line should have both `agent_id` (UUID) and `agent_name`.
