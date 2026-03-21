# Filesystem-based Agent Identity Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `working_dir` with `base_dir + agent_id`, add `.agent.json` manifest for resume, add `.agent.lock` for liveness.

**Architecture:** `BaseAgent` constructor takes `base_dir` (required) instead of `working_dir`. `working_dir` becomes a computed property (`base_dir / agent_id`). `.agent.json` stores role/ltm for resume. `.agent.lock` is an OS file lock for cross-process conflict detection. Delegate spawns children as peer folders.

**Tech Stack:** Python stdlib (json, os, fcntl/msvcrt, re, pathlib, tempfile). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-03-15-filesystem-agent-identity-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai/agent.py` | Modify | Replace `working_dir` param with `base_dir`, add lock/manifest logic |
| `src/lingtai/capabilities/delegate.py` | Modify | Pass `base_dir` to child, remove `working_dir` |
| `tests/test_agent.py` | Modify | All `working_dir=` → `base_dir=`, add lock/manifest/resume tests |
| `tests/test_layers_email.py` | Modify | `working_dir=tmp_path` → `base_dir=tmp_path` |
| `tests/test_layers_bash.py` | Modify | `working_dir=` → `base_dir=` |
| `tests/test_layers_delegate.py` | Modify | `working_dir=` → `base_dir=` |
| `tests/test_layers_draw.py` | Modify | `working_dir=` → `base_dir=` (mock agent) |
| `tests/test_layers_talk.py` | Modify | `working_dir=` → `base_dir=` (mock agent) |
| `tests/test_layers_compose.py` | Modify | `working_dir=` → `base_dir=` (mock agent) |
| `tests/test_layers_listen.py` | Modify | `working_dir=` → `base_dir=` (mock agent) |
| `tests/test_three_agent_email.py` | Modify | `working_dir=` → `base_dir=` |
| `tests/test_services_mail.py` | Modify | `working_dir=` in TCPMailService → stays as-is (mail service keeps its own `working_dir`) |
| `tests/test_services_logging.py` | Modify | `working_dir=` → `base_dir=` |
| `tests/test_intrinsics_comm.py` | Modify | `working_dir=` → `base_dir=` |
| `examples/three_agents.py` | Modify | `working_dir=` → `base_dir=` |
| `examples/two_agents.py` | Modify | `working_dir=` → `base_dir=` |
| `examples/chat_agent.py` | Modify | `working_dir=` → `base_dir=` |
| `examples/chat_web.py` | Modify | `working_dir=` → `base_dir=` |

**Note:** Capabilities (`bash.py`, `email.py`, `draw.py`, `talk.py`, `compose.py`, `listen.py`) access `agent.working_dir` or `agent._working_dir` — these continue to work unchanged since `working_dir` becomes a computed property. The `TCPMailService` keeps its own `working_dir` parameter — it receives the computed value from the agent.

---

## Chunk 1: Core Agent Changes

### Task 1: Modify BaseAgent constructor — `base_dir` replaces `working_dir`

**Files:**
- Modify: `src/lingtai/agent.py:170-250` (constructor + property)

- [ ] **Step 1: Add `import os, re` and cross-platform lock helpers at module level**

After the existing imports (around line 17), add `import os`. After `logger = get_logger()` (around line 55), add:

```python
import sys as _sys

if _sys.platform == "win32":
    import msvcrt as _msvcrt

    def _lock_fd(fd):
        _msvcrt.locking(fd.fileno(), _msvcrt.LK_NBLCK, 1)

    def _unlock_fd(fd):
        _msvcrt.locking(fd.fileno(), _msvcrt.LK_UNLCK, 1)
else:
    import fcntl as _fcntl

    def _lock_fd(fd):
        _fcntl.flock(fd, _fcntl.LOCK_EX | _fcntl.LOCK_NB)

    def _unlock_fd(fd):
        _fcntl.flock(fd, _fcntl.LOCK_UN)


_AGENT_ID_RE = re.compile(r"^[a-zA-Z0-9_-]+$")
```

- [ ] **Step 2: Replace `working_dir` param with `base_dir` in constructor**

Change the constructor signature: remove `working_dir: str | Path,`, add `base_dir: str | Path,`.

In the constructor body, replace:
```python
# Working directory for file intrinsics
self._working_dir = Path(working_dir)
```

With:
```python
# Validate agent_id
if not _AGENT_ID_RE.match(agent_id):
    raise ValueError(
        f"agent_id must match [a-zA-Z0-9_-]+, got: {agent_id!r}"
    )

# Base directory (shared root) and working directory (per-agent)
self._base_dir = Path(base_dir)
if not self._base_dir.is_dir():
    raise FileNotFoundError(f"base_dir does not exist: {self._base_dir}")
self._working_dir = self._base_dir / self.agent_id
self._working_dir.mkdir(exist_ok=True)
```

- [ ] **Step 3: Add lock acquisition after working_dir setup, before services**

After `self._working_dir.mkdir(exist_ok=True)`, add:

```python
# Acquire working directory lock
self._lock_file: Any = None
self._acquire_lock()
```

- [ ] **Step 4: Add manifest read — resume role/ltm before prompt manager setup**

Between lock acquisition and the `# System prompt manager` section, add:

```python
# Read manifest for resume (before prompt manager, so role/ltm can be restored)
manifest_role, manifest_ltm = self._read_manifest()
if not role and manifest_role:
    role = manifest_role
if not ltm and manifest_ltm:
    ltm = manifest_ltm
```

- [ ] **Step 5: Add manifest write after prompt manager setup**

After the `if ltm: ...` block, add:

```python
# Write manifest (new start or resume)
self._write_manifest()
```

- [ ] **Step 6: Update the `working_dir` property**

Change the property at line ~604 from:
```python
@property
def working_dir(self) -> Path:
    """The agent's working directory."""
    return self._working_dir
```
To:
```python
@property
def working_dir(self) -> Path:
    """The agent's working directory (base_dir / agent_id)."""
    return self._working_dir
```

(No functional change — the property already returns `self._working_dir`.)

- [ ] **Step 7: Add lock/manifest methods before `_on_mail_received`**

Insert before `def _on_mail_received`:

```python
# ------------------------------------------------------------------
# Working directory lock + manifest
# ------------------------------------------------------------------

_LOCK_FILE = ".agent.lock"
_MANIFEST_FILE = ".agent.json"

def _acquire_lock(self) -> None:
    """Acquire exclusive lock on working directory."""
    lock_path = self._working_dir / self._LOCK_FILE
    self._lock_file = open(lock_path, "w")
    try:
        _lock_fd(self._lock_file)
    except OSError:
        self._lock_file.close()
        self._lock_file = None
        raise RuntimeError(
            f"Working directory '{self._working_dir}' is already in use "
            f"by another agent. Each agent needs its own directory."
        )

def _release_lock(self) -> None:
    """Release working directory lock."""
    if self._lock_file is not None:
        try:
            _unlock_fd(self._lock_file)
            self._lock_file.close()
        except OSError:
            pass
        self._lock_file = None

def _read_manifest(self) -> tuple[str, str]:
    """Read role and ltm from .agent.json. Returns ("", "") if not found."""
    path = self._working_dir / self._MANIFEST_FILE
    if not path.is_file():
        return "", ""
    try:
        data = json.loads(path.read_text())
        return data.get("role", ""), data.get("ltm", "")
    except (json.JSONDecodeError, OSError):
        # Corrupt manifest — rename and treat as fresh
        corrupt = self._working_dir / ".agent.json.corrupt"
        try:
            path.rename(corrupt)
        except OSError:
            pass
        logger.warning("Corrupt .agent.json renamed to .agent.json.corrupt")
        return "", ""

def _write_manifest(self) -> None:
    """Write .agent.json atomically."""
    from datetime import datetime, timezone
    data = {
        "agent_id": self.agent_id,
        "started_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "role": self._prompt_manager.read_section("role") or "",
        "ltm": self._prompt_manager.read_section("ltm") or "",
    }
    if self._mail_service is not None and self._mail_service.address:
        data["address"] = self._mail_service.address
    target = self._working_dir / self._MANIFEST_FILE
    tmp = self._working_dir / ".agent.json.tmp"
    tmp.write_text(json.dumps(data, indent=2))
    os.replace(str(tmp), str(target))
```

- [ ] **Step 8: Update `stop()` to persist ltm and release lock**

Add to the end of `stop()`:

```python
# Persist final ltm and release lock
self._write_manifest()
self._release_lock()
```

- [ ] **Step 9: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 10: Commit**

```bash
git add src/lingtai/agent.py
git commit -m "refactor: replace working_dir with base_dir, add .agent.lock and .agent.json"
```

---

## Chunk 2: Delegate Changes

### Task 2: Update delegate to spawn children as peers

**Files:**
- Modify: `src/lingtai/capabilities/delegate.py:56-93`

- [ ] **Step 1: Update `_spawn` to use `base_dir`**

Replace the `_spawn` method body (lines 56-93):

```python
def _spawn(self, args: dict) -> dict:
    from ..agent import BaseAgent
    from ..services.mail import TCPMailService

    parent = self._agent

    # Get a free TCP port
    port = self._get_free_port()
    child_id = f"{parent.agent_id}_child_{port}"

    # Resolve role — override or copy parent
    role = args.get("role") or parent._prompt_manager.read_section("role") or ""
    ltm = args.get("ltm") or parent._prompt_manager.read_section("ltm") or ""

    # Child is a peer in the same base_dir
    child_working_dir = parent._base_dir / child_id
    mail_svc = TCPMailService(listen_port=port, working_dir=child_working_dir)

    child = BaseAgent(
        agent_id=child_id,
        service=parent.service,
        mail_service=mail_svc,
        config=parent._config,
        base_dir=parent._base_dir,
        streaming=parent._streaming,
        role=role,
        ltm=ltm,
    )

    # Replay capabilities — filter if specified, skip delegate to prevent recursion
    requested = args.get("capabilities")
    for cap_name, cap_kwargs in parent._capabilities:
        if cap_name == "delegate":
            continue  # no recursive spawning
        if requested is not None and cap_name not in requested:
            continue
        child.add_capability(cap_name, **cap_kwargs)

    child.start()
    address = mail_svc.address
    return {"status": "ok", "address": address, "agent_id": child.agent_id}
```

- [ ] **Step 2: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/capabilities/delegate.py
git commit -m "refactor: delegate spawns children as peers using base_dir"
```

---

## Chunk 3: Test Migration — Core Agent Tests

### Task 3: Migrate `tests/test_agent.py`

**Files:**
- Modify: `tests/test_agent.py`

This is the largest test file (~45 occurrences). The migration is mechanical:
- Every `working_dir="/tmp"` → `base_dir=tmp_path`
- Every `working_dir=str(tmp_path)` → `base_dir=tmp_path`
- Every `working_dir=tmp_path` → `base_dir=tmp_path`
- Tests that don't take `tmp_path` need it added to their signature

- [ ] **Step 1: Replace all `working_dir=` with `base_dir=` in test_agent.py**

Use search-and-replace:
- `working_dir="/tmp"` → `base_dir=tmp_path`
- `working_dir=str(tmp_path)` → `base_dir=tmp_path`
- `working_dir=tmp_path` → `base_dir=tmp_path`

For every test function that gains `tmp_path` but doesn't have it in its signature, add it.

- [ ] **Step 2: Update property tests**

The `test_working_dir_property` test should verify the computed property:
```python
def test_working_dir_property(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent.working_dir == tmp_path / "test"
```

The `test_working_dir_required` test becomes `test_base_dir_required`:
```python
def test_base_dir_required():
    with pytest.raises(TypeError):
        BaseAgent(agent_id="test", service=make_mock_service())
```

- [ ] **Step 3: Add new tests for lock, manifest, resume, validation**

```python
def test_agent_creates_manifest(tmp_path):
    import json
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    manifest = tmp_path / "alice" / ".agent.json"
    assert manifest.is_file()
    data = json.loads(manifest.read_text())
    assert data["agent_id"] == "alice"
    assert "started_at" in data


def test_agent_creates_lock_file(tmp_path):
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    assert (tmp_path / "alice" / ".agent.lock").is_file()


def test_agent_lock_conflict(tmp_path):
    """Two agents in the same directory should raise."""
    agent1 = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(RuntimeError, match="already in use"):
        agent2 = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)


def test_agent_lock_released_on_stop(tmp_path):
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    agent.stop()
    # Should be able to create a new agent in the same dir
    agent2 = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)


def test_agent_resume_reads_role_ltm(tmp_path):
    import json
    agent = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path,
        role="researcher", ltm="knows python",
    )
    agent.stop()
    # Resume — no role/ltm passed, should read from manifest
    agent2 = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    assert agent2._prompt_manager.read_section("role") == "researcher"
    assert agent2._prompt_manager.read_section("ltm") == "knows python"


def test_agent_resume_explicit_overrides_manifest(tmp_path):
    agent = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path,
        role="old role", ltm="old ltm",
    )
    agent.stop()
    agent2 = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path,
        role="new role",
    )
    assert agent2._prompt_manager.read_section("role") == "new role"
    # ltm not overridden — should be restored from manifest
    assert agent2._prompt_manager.read_section("ltm") == "old ltm"


def test_agent_stop_persists_ltm(tmp_path):
    import json
    agent = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path, ltm="initial",
    )
    agent._prompt_manager.write_section("ltm", "updated knowledge")
    agent.stop()
    data = json.loads((tmp_path / "alice" / ".agent.json").read_text())
    assert data["ltm"] == "updated knowledge"


def test_agent_corrupt_manifest(tmp_path):
    """Corrupt .agent.json should be renamed and treated as fresh."""
    agent_dir = tmp_path / "alice"
    agent_dir.mkdir()
    (agent_dir / ".agent.json").write_text("{corrupt json")
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    assert (agent_dir / ".agent.json.corrupt").is_file()
    assert agent._prompt_manager.read_section("role") is None


def test_agent_id_validation(tmp_path):
    with pytest.raises(ValueError, match="agent_id"):
        BaseAgent(agent_id="bad/id", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(ValueError, match="agent_id"):
        BaseAgent(agent_id="../escape", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(ValueError, match="agent_id"):
        BaseAgent(agent_id="", service=make_mock_service(), base_dir=tmp_path)


def test_base_dir_must_exist(tmp_path):
    with pytest.raises(FileNotFoundError):
        BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path / "nonexistent")
```

- [ ] **Step 4: Run tests**

Run: `pytest tests/test_agent.py -v`

- [ ] **Step 5: Commit**

```bash
git add tests/test_agent.py
git commit -m "test: migrate test_agent.py to base_dir, add lock/manifest/resume tests"
```

---

## Chunk 4: Test Migration — Capability and Integration Tests

### Task 4: Migrate remaining test files

**Files:**
- Modify: `tests/test_layers_email.py` (~29 occurrences)
- Modify: `tests/test_layers_bash.py` (~14 occurrences)
- Modify: `tests/test_layers_delegate.py` (~10 occurrences)
- Modify: `tests/test_layers_draw.py` (~8 occurrences)
- Modify: `tests/test_layers_talk.py` (~7 occurrences)
- Modify: `tests/test_layers_compose.py` (~7 occurrences)
- Modify: `tests/test_layers_listen.py` (~10 occurrences)
- Modify: `tests/test_three_agent_email.py` (~7 occurrences)
- Modify: `tests/test_services_logging.py` (~3 occurrences)
- Modify: `tests/test_intrinsics_comm.py` (~1 occurrence)

The migration pattern is the same across all files:

**For tests using real `BaseAgent`:**
- `BaseAgent(..., working_dir="/tmp")` → `BaseAgent(..., base_dir=tmp_path)`
- `BaseAgent(..., working_dir=tmp_path)` → `BaseAgent(..., base_dir=tmp_path)`
- `BaseAgent(..., working_dir=str(tmp_path))` → `BaseAgent(..., base_dir=tmp_path)`
- Add `tmp_path` to function signature if not already present

**For tests using `MagicMock` agents (draw, talk, compose, listen):**
- These mock `agent.working_dir` as a property. Since the real property returns `base_dir / agent_id`, mocks should set `agent.working_dir = tmp_path` (they don't construct a real BaseAgent, so no `base_dir` needed).
- No changes needed for mock agents — they already set `agent.working_dir = tmp_path`.

**For `test_layers_delegate.py`:**
- Tests creating real `BaseAgent` with `working_dir="/tmp"` → `base_dir=tmp_path`
- The delegate test that spawns children should verify peer folder creation

**For `test_three_agent_email.py`:**
- `_make_agent` helper: `working_dir=d` → `base_dir=d.parent, agent_id=d.name` OR simplify: give each agent a unique name under a shared tmp_path base_dir.

- [ ] **Step 1: Migrate each test file**

Apply the pattern to each file. For `test_three_agent_email.py`, update `_make_agent`:

```python
def _make_agent(agent_id: str, port: int, base_dir: Path):
    mail_svc = TCPMailService(listen_port=port, working_dir=base_dir / agent_id)
    agent = BaseAgent(
        agent_id=agent_id,
        service=_make_mock_service(),
        mail_service=mail_svc,
        base_dir=base_dir,
    )
    mgr = agent.add_capability("email")
    return agent, mgr
```

And in `setup_method`, use a single shared base_dir:
```python
self.base_dir = Path(tempfile.mkdtemp())
for name, port in self.ports.items():
    agent, mgr = _make_agent(name, port, self.base_dir)
```

- [ ] **Step 2: Run all tests**

Run: `pytest tests/ -x -q`

- [ ] **Step 3: Commit**

```bash
git add tests/
git commit -m "test: migrate all test files to base_dir"
```

---

## Chunk 5: Example Migration

### Task 5: Update all example scripts

**Files:**
- Modify: `examples/three_agents.py`
- Modify: `examples/two_agents.py`
- Modify: `examples/chat_agent.py`
- Modify: `examples/chat_web.py`

- [ ] **Step 1: Update examples**

For multi-agent examples (`three_agents.py`, `two_agents.py`):
- Remove the manual per-agent directory creation (`(base_dir / name).mkdir(...)`)
- Replace `working_dir=base_dir / "alice"` with `base_dir=base_dir`
- Replace `TCPMailService(listen_port=8301, working_dir=base_dir / "alice")` with `TCPMailService(listen_port=8301, working_dir=base_dir / "alice")` — mail service keeps `working_dir`, it receives the computed path

For single-agent examples (`chat_agent.py`, `chat_web.py`):
- Replace `working_dir="."` with `base_dir="."`

- [ ] **Step 2: Smoke-test examples parse**

Run: `python -c "import examples.three_agents"` (or just check syntax)

- [ ] **Step 3: Run full test suite**

Run: `python -c "import lingtai"` && `pytest tests/ -x -q`

- [ ] **Step 4: Commit**

```bash
git add examples/
git commit -m "refactor: migrate examples to base_dir"
```

---

## Chunk 6: Final Verification

### Task 6: Verify no stale `working_dir` references remain

- [ ] **Step 1: Grep for stale references**

Run: `grep -rn "working_dir=" src/lingtai/agent.py` — should only find the `TCPMailService` passthrough and internal `self._working_dir` assignment.

Run: `grep -rn 'working_dir="/tmp"' tests/` — should return nothing.

Run: `grep -rn 'working_dir="' examples/` — should return nothing (except possibly mail service `working_dir`).

- [ ] **Step 2: Run full test suite**

Run: `pytest tests/ -x -q`
All tests must pass.

- [ ] **Step 3: Smoke-test import**

Run: `python -c "import lingtai"`

- [ ] **Step 4: Final commit if any cleanup needed**

```bash
git add -A
git commit -m "refactor: complete filesystem-based agent identity migration"
```
