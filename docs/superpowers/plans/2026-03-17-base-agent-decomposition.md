# BaseAgent Decomposition Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose the 1940-line `base_agent.py` monolith into four focused components via composition, leaving BaseAgent as a ~500-line kernel coordinator.

**Architecture:** Extract WorkingDir (filesystem/git), intrinsic handlers (into intrinsics/*.py), ToolExecutor (tool execution engine), and SessionManager (LLM session/tokens). Each component has no back-reference to BaseAgent — communication via constructor injection and callbacks.

**Tech Stack:** Python 3.11+, dataclasses, threading, subprocess, unittest.mock

**Spec:** `docs/superpowers/specs/2026-03-17-base-agent-decomposition-design.md`

---

## File Structure

| File | Responsibility | Action |
|------|---------------|--------|
| `src/stoai/workdir.py` | Working directory: lock, git init, manifest, diff/commit | Create |
| `src/stoai/tool_executor.py` | Tool execution: sequential/parallel, timing, errors | Create |
| `src/stoai/session.py` | LLM session: send/retry/reset, tokens, compaction | Create |
| `src/stoai/intrinsics/mail.py` | Add `handle(agent, args)` function | Modify |
| `src/stoai/intrinsics/clock.py` | Add `handle(agent, args)` function | Modify |
| `src/stoai/intrinsics/status.py` | Add `handle(agent, args)` function | Modify |
| `src/stoai/intrinsics/system.py` | Add `handle(agent, args)` function | Modify |
| `src/stoai/intrinsics/__init__.py` | Update ALL_INTRINSICS to include handle refs | Modify |
| `src/stoai/base_agent.py` | Remove extracted code, delegate to components | Modify |
| `tests/test_workdir.py` | Unit tests for WorkingDir | Create |
| `tests/test_tool_executor.py` | Unit tests for ToolExecutor | Create |
| `tests/test_session.py` | Unit tests for SessionManager | Create |
| `tests/test_clock.py` | Update handler calls from `agent._handle_clock` to `handle(agent, ...)` | Modify |
| `tests/test_status.py` | Update handler calls | Modify |
| `tests/test_system.py` | Update handler calls | Modify |
| `tests/test_agent.py` | Update `_handle_mail` calls | Modify |
| `tests/test_conscience.py` | Update intrinsic identity assertion | Modify |

---

## Chunk 1: WorkingDir Extraction

### Task 1: Create WorkingDir with tests

**Files:**
- Create: `src/stoai/workdir.py`
- Create: `tests/test_workdir.py`

- [ ] **Step 1: Write failing tests for WorkingDir**

```python
# tests/test_workdir.py
"""Tests for WorkingDir — filesystem, locking, git, manifest."""
from __future__ import annotations

import json
import subprocess
from pathlib import Path

import pytest

from stoai.workdir import WorkingDir


def test_init_creates_agent_dir(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    assert wd.path == tmp_path / "alice"
    assert wd.path.is_dir()


def test_lock_prevents_second_instance(tmp_path):
    wd1 = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd1.acquire_lock()
    try:
        wd2 = WorkingDir(base_dir=tmp_path, agent_id="alice")
        with pytest.raises(RuntimeError, match="already in use"):
            wd2.acquire_lock()
    finally:
        wd1.release_lock()


def test_lock_release_allows_reuse(tmp_path):
    wd1 = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd1.acquire_lock()
    wd1.release_lock()
    wd2 = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd2.acquire_lock()  # should not raise
    wd2.release_lock()


def test_git_init_creates_repo(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    assert (wd.path / ".git").is_dir()
    assert (wd.path / ".gitignore").is_file()
    assert (wd.path / "system" / "covenant.md").is_file()
    assert (wd.path / "system" / "memory.md").is_file()


def test_git_init_skips_if_already_initialized(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    result1 = subprocess.run(
        ["git", "rev-list", "--count", "HEAD"],
        cwd=wd.path, capture_output=True, text=True,
    )
    wd.init_git()  # second call — should be no-op
    result2 = subprocess.run(
        ["git", "rev-list", "--count", "HEAD"],
        cwd=wd.path, capture_output=True, text=True,
    )
    assert result1.stdout.strip() == result2.stdout.strip()


def test_read_manifest_returns_empty_when_missing(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    assert wd.read_manifest() == ("", "")


def test_write_and_read_manifest(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    manifest = {"agent_id": "alice", "role": "researcher", "started_at": "2026-01-01T00:00:00Z"}
    wd.write_manifest(manifest)
    role, ltm = wd.read_manifest()
    assert role == "researcher"


def test_diff_and_commit(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    # Write to tracked file
    memory_file = wd.path / "system" / "memory.md"
    memory_file.write_text("hello world")
    diff_text, commit_hash = wd.diff_and_commit("system/memory.md", "memory")
    assert commit_hash is not None
    assert diff_text  # should have some diff content


def test_diff_and_commit_no_changes(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    diff_text, commit_hash = wd.diff_and_commit("system/memory.md", "memory")
    assert diff_text is None
    assert commit_hash is None


def test_diff_read_only(tmp_path):
    wd = WorkingDir(base_dir=tmp_path, agent_id="alice")
    wd.init_git()
    memory_file = wd.path / "system" / "memory.md"
    memory_file.write_text("new content")
    result = wd.diff("system/memory.md")
    assert isinstance(result, str)
    # Should not commit — file should still show as changed
    status = subprocess.run(
        ["git", "status", "--porcelain", "system/memory.md"],
        cwd=wd.path, capture_output=True, text=True,
    )
    assert status.stdout.strip()  # still dirty


def test_invalid_agent_id_raises(tmp_path):
    with pytest.raises(ValueError, match="agent_id must match"):
        WorkingDir(base_dir=tmp_path, agent_id="bad agent!")
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_workdir.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'stoai.workdir'`

- [ ] **Step 3: Implement WorkingDir**

```python
# src/stoai/workdir.py
"""WorkingDir — agent working directory: lock, git, manifest."""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

if sys.platform == "win32":
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
_LOCK_FILE = ".agent.lock"
_MANIFEST_FILE = ".agent.json"


class WorkingDir:
    """Manages an agent's working directory — locking, git, manifest."""

    def __init__(self, base_dir: Path | str, agent_id: str) -> None:
        if not _AGENT_ID_RE.match(agent_id):
            raise ValueError(
                f"agent_id must match [a-zA-Z0-9_-]+, got: {agent_id!r}"
            )
        self._base_dir = Path(base_dir)
        self._agent_id = agent_id
        self._path = self._base_dir / agent_id
        self._path.mkdir(exist_ok=True)
        self._lock_file: Any = None

    @property
    def path(self) -> Path:
        return self._path

    # --- Lock lifecycle ---

    def acquire_lock(self) -> None:
        lock_path = self._path / _LOCK_FILE
        self._lock_file = open(lock_path, "w")
        try:
            _lock_fd(self._lock_file)
        except OSError:
            self._lock_file.close()
            self._lock_file = None
            raise RuntimeError(
                f"Working directory '{self._path}' is already in use "
                f"by another agent. Each agent needs its own directory."
            )

    def release_lock(self) -> None:
        if self._lock_file is not None:
            try:
                _unlock_fd(self._lock_file)
                self._lock_file.close()
            except OSError:
                pass
            self._lock_file = None

    # --- Git operations ---

    def init_git(self) -> None:
        git_dir = self._path / ".git"
        if git_dir.is_dir():
            return

        try:
            subprocess.run(
                ["git", "init"], cwd=self._path,
                capture_output=True, check=True,
            )
            subprocess.run(
                ["git", "config", "user.email", "agent@stoai"],
                cwd=self._path, capture_output=True, check=True,
            )
            subprocess.run(
                ["git", "config", "user.name", "StoAI Agent"],
                cwd=self._path, capture_output=True, check=True,
            )

            gitignore = self._path / ".gitignore"
            gitignore.write_text(
                "# Track nothing by default\n"
                "*\n"
                "# Except these\n"
                "!.gitignore\n"
                "!system/\n"
                "!system/**\n"
                "!logs/\n"
                "!logs/**\n"
            )

            system_dir = self._path / "system"
            system_dir.mkdir(exist_ok=True)
            covenant_file = system_dir / "covenant.md"
            if not covenant_file.is_file():
                covenant_file.write_text("")
            memory_file = system_dir / "memory.md"
            if not memory_file.is_file():
                memory_file.write_text("")

            subprocess.run(
                ["git", "add", ".gitignore", "system/"],
                cwd=self._path, capture_output=True, check=True,
            )
            subprocess.run(
                ["git", "commit", "-m", "init: agent working directory"],
                cwd=self._path, capture_output=True, check=True,
            )
        except (FileNotFoundError, subprocess.CalledProcessError):
            system_dir = self._path / "system"
            system_dir.mkdir(exist_ok=True)
            covenant_file = system_dir / "covenant.md"
            if not covenant_file.is_file():
                covenant_file.write_text("")
            memory_file = system_dir / "memory.md"
            if not memory_file.is_file():
                memory_file.write_text("")

    def diff(self, rel_path: str) -> str:
        try:
            result = subprocess.run(
                ["git", "diff", rel_path],
                cwd=self._path, capture_output=True, text=True,
            )
            diff_text = result.stdout.strip()
            if not diff_text:
                status_result = subprocess.run(
                    ["git", "status", "--porcelain", rel_path],
                    cwd=self._path, capture_output=True, text=True,
                )
                if status_result.stdout.strip():
                    file_path = self._path / rel_path
                    diff_text = f"(new/untracked file)\n{file_path.read_text()}"
        except (FileNotFoundError, subprocess.CalledProcessError):
            diff_text = ""
        return diff_text

    def diff_and_commit(self, rel_path: str, label: str) -> tuple[str | None, str | None]:
        try:
            diff_result = subprocess.run(
                ["git", "diff", rel_path],
                cwd=self._path, capture_output=True, text=True,
            )
            diff_cached = subprocess.run(
                ["git", "diff", "--cached", rel_path],
                cwd=self._path, capture_output=True, text=True,
            )
            status_result = subprocess.run(
                ["git", "status", "--porcelain", rel_path],
                cwd=self._path, capture_output=True, text=True,
            )

            has_changes = bool(
                diff_result.stdout.strip()
                or diff_cached.stdout.strip()
                or status_result.stdout.strip()
            )

            if not has_changes:
                return None, None

            diff_text = diff_result.stdout or status_result.stdout

            subprocess.run(
                ["git", "add", rel_path],
                cwd=self._path, capture_output=True, check=True,
            )

            if not diff_text.strip():
                staged = subprocess.run(
                    ["git", "diff", "--cached", rel_path],
                    cwd=self._path, capture_output=True, text=True,
                )
                diff_text = staged.stdout

            subprocess.run(
                ["git", "commit", "-m", f"system: update {label}"],
                cwd=self._path, capture_output=True, check=True,
            )

            hash_result = subprocess.run(
                ["git", "rev-parse", "--short", "HEAD"],
                cwd=self._path, capture_output=True, text=True,
            )
            commit_hash = hash_result.stdout.strip()

            return diff_text, commit_hash

        except (FileNotFoundError, subprocess.CalledProcessError):
            return None, None

    # --- Manifest ---

    def read_manifest(self) -> tuple[str, str]:
        path = self._path / _MANIFEST_FILE
        if not path.is_file():
            return "", ""
        try:
            data = json.loads(path.read_text())
            return data.get("role", ""), data.get("ltm", "")
        except (json.JSONDecodeError, OSError):
            corrupt = self._path / ".agent.json.corrupt"
            try:
                path.rename(corrupt)
            except OSError:
                pass
            return "", ""

    def write_manifest(self, manifest: dict) -> None:
        target = self._path / _MANIFEST_FILE
        tmp = self._path / ".agent.json.tmp"
        tmp.write_text(json.dumps(manifest, indent=2))
        os.replace(str(tmp), str(target))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_workdir.py -v`
Expected: All PASS

- [ ] **Step 5: Smoke-test import**

Run: `source venv/bin/activate && python -c "from stoai.workdir import WorkingDir; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add src/stoai/workdir.py tests/test_workdir.py
git commit -m "feat: extract WorkingDir from BaseAgent"
```

### Task 2: Wire WorkingDir into BaseAgent

**Files:**
- Modify: `src/stoai/base_agent.py`

- [ ] **Step 1: Replace file-locking helpers and WorkingDir methods in BaseAgent**

In `base_agent.py`:
1. Add import: `from .workdir import WorkingDir`
2. Remove the module-level `sys.platform` file-locking block (lines 59-77) and `_AGENT_ID_RE`
3. In `__init__`:
   - Replace agent_id validation with WorkingDir (it validates internally)
   - Replace `self._working_dir = self._base_dir / self.agent_id` + `mkdir` with `self._workdir = WorkingDir(base_dir=base_dir, agent_id=agent_id)`
   - Replace `self._acquire_lock()` with `self._workdir.acquire_lock()`
   - Replace `self._read_manifest()` with `self._workdir.read_manifest()`
   - Replace `self._write_manifest()` with the `self._workdir.write_manifest(manifest_dict)` call (build the dict in BaseAgent)
   - Keep `self._working_dir` as a property alias: already exists, just update to `return self._workdir.path`
4. In `start()`: replace `self._git_init_working_dir()` with `self._workdir.init_git()`
5. In `stop()`: replace `self._write_manifest()` with `self._workdir.write_manifest(...)`, `self._release_lock()` with `self._workdir.release_lock()`
6. Remove methods: `_acquire_lock`, `_release_lock`, `_git_init_working_dir`, `_read_manifest`, `_write_manifest`, `_git_diff_and_commit`
7. Remove class constants: `_LOCK_FILE`, `_MANIFEST_FILE`
8. In `_handle_system` (`_system_diff`, `_system_load`): replace `self._git_diff_and_commit(...)` with `self._workdir.diff_and_commit(...)` and raw subprocess git diff with `self._workdir.diff(...)`

- [ ] **Step 2: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All existing tests pass

- [ ] **Step 3: Smoke-test import**

Run: `python -c "import stoai"`
Expected: No errors

- [ ] **Step 4: Update anima capability to use WorkingDir**

The anima capability calls `self._agent._system_diff()` and `self._agent._git_diff_and_commit()` directly. These are removed in this task, so anima must be updated now.

In `src/stoai/capabilities/anima.py`:
- Replace `self._agent._system_diff(self._character_path, "character")` with a dict built from `self._agent._workdir.diff("system/character.md")`:
  ```python
  diff_text = self._agent._workdir.diff("system/character.md")
  return {"status": "ok", "path": str(self._character_path), "git_diff": diff_text}
  ```
- Replace `self._agent._git_diff_and_commit(rel_path, "character")` with `self._agent._workdir.diff_and_commit(rel_path, "character")`

- [ ] **Step 5: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All existing tests pass (including test_anima.py)

- [ ] **Step 6: Commit**

```bash
git add src/stoai/base_agent.py src/stoai/capabilities/anima.py
git commit -m "refactor: wire WorkingDir into BaseAgent, remove extracted methods"
```

---

## Chunk 2: Intrinsic Handler Extraction

### Task 3: Move intrinsic handlers to intrinsics/*.py

**Files:**
- Modify: `src/stoai/intrinsics/mail.py`
- Modify: `src/stoai/intrinsics/clock.py`
- Modify: `src/stoai/intrinsics/status.py`
- Modify: `src/stoai/intrinsics/system.py`
- Modify: `src/stoai/intrinsics/__init__.py`
- Modify: `src/stoai/base_agent.py`

- [ ] **Step 1: Add handle() functions to each intrinsic module**

For each module, add a `handle(agent, args)` function and private helpers. The handler accesses agent state via the `agent` parameter. Copy the method bodies from `base_agent.py`, replacing `self` with `agent`.

**`intrinsics/mail.py`** — add after existing SCHEMA/DESCRIPTION:

```python
def handle(agent, args: dict) -> dict:
    """Handle mail tool — FIFO send and read."""
    action = args.get("action", "send")
    if action == "send":
        return _send(agent, args)
    elif action == "read":
        return _read(agent, args)
    else:
        return {"error": f"Unknown mail action: {action}"}


def _send(agent, args: dict) -> dict:
    # Copy body of BaseAgent._mail_send, replacing self -> agent
    from pathlib import Path
    ...


def _read(agent, args: dict) -> dict:
    # Copy body of BaseAgent._mail_read, replacing self -> agent
    ...
```

**`intrinsics/clock.py`** — add `handle`, `_check`, `_wait`:
```python
def handle(agent, args: dict) -> dict:
    action = args.get("action", "check")
    if action == "check":
        return _check(agent)
    elif action == "wait":
        return _wait(agent, args)
    else:
        return {"error": f"Unknown clock action: {action}"}
```

**`intrinsics/status.py`** — add `handle`, `_show`, `_shutdown`:
```python
def handle(agent, args: dict) -> dict:
    action = args.get("action", "show")
    if action == "show":
        return _show(agent)
    elif action == "shutdown":
        return _shutdown(agent, args)
    else:
        return {"error": f"Unknown status action: {action}"}
```

**`intrinsics/system.py`** — add `handle`, `_diff`, `_load`.
Note: WorkingDir is already extracted at this point. Use `agent._workdir.diff(rel_path)` and `agent._workdir.diff_and_commit(rel_path, label)` instead of raw subprocess calls. Use `agent._working_dir` (property → `agent._workdir.path`) for filesystem paths.
```python
def handle(agent, args: dict) -> dict:
    action = args.get("action", "")
    obj = args.get("object", "")
    if obj != "memory":
        return {"error": f"Unknown object: {obj!r}. Must be 'memory'."}
    system_dir = agent._working_dir / "system"
    system_dir.mkdir(exist_ok=True)
    file_path = system_dir / "memory.md"
    if not file_path.is_file():
        file_path.write_text("")
    if action == "diff":
        return _diff(agent, file_path, "memory")
    elif action == "load":
        return _load(agent, file_path, "memory")
    else:
        return {"error": f"Unknown action: {action!r}. Must be 'diff' or 'load'."}

def _diff(agent, file_path, obj: str) -> dict:
    rel_path = f"system/{obj}.md"
    diff_text = agent._workdir.diff(rel_path)
    return {"status": "ok", "path": str(file_path), "git_diff": diff_text}

def _load(agent, file_path, obj: str) -> dict:
    content = file_path.read_text()
    size_bytes = len(content.encode("utf-8"))
    if content.strip():
        agent._prompt_manager.write_section(obj, content)
    else:
        agent._prompt_manager.delete_section(obj)
    agent._token_decomp_dirty = True
    if agent._chat is not None:
        agent._chat.update_system_prompt(agent._build_system_prompt())
    rel_path = f"system/{obj}.md"
    git_diff, commit_hash = agent._workdir.diff_and_commit(rel_path, obj)
    agent._log(f"system_load_{obj}", size_bytes=size_bytes, changed=commit_hash is not None)
    return {
        "status": "ok", "path": str(file_path), "size_bytes": size_bytes,
        "content_preview": content[:200],
        "diff": {"changed": commit_hash is not None, "git_diff": git_diff or "", "commit": commit_hash},
    }
```

- [ ] **Step 2: Update `intrinsics/__init__.py` to include handle references**

```python
"""Intrinsic tools available to all agents."""
from . import mail, clock, status, system

ALL_INTRINSICS = {
    "mail": {"schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handle": mail.handle},
    "clock": {"schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handle": clock.handle},
    "status": {"schema": status.SCHEMA, "description": status.DESCRIPTION, "handle": status.handle},
    "system": {"schema": system.SCHEMA, "description": system.DESCRIPTION, "handle": system.handle},
}
```

- [ ] **Step 3: Update BaseAgent._wire_intrinsics to use module handlers**

```python
def _wire_intrinsics(self) -> None:
    """Wire kernel intrinsic tool handlers."""
    for name, info in ALL_INTRINSICS.items():
        handle_fn = info["handle"]
        self._intrinsics[name] = lambda args, fn=handle_fn: fn(self, args)
```

- [ ] **Step 4: Remove the old handler methods from BaseAgent**

Remove: `_handle_mail`, `_mail_send`, `_mail_read`, `_handle_clock`, `_clock_check`, `_clock_wait`, `_handle_status`, `_status_shutdown`, `_status_show`, `_handle_system`, `_system_diff`, `_system_load`.

Also update `BaseAgent.mail()` which calls `self._handle_mail(...)` — change to call the intrinsic via `self._intrinsics["mail"](...)`.

- [ ] **Step 5: Update tests that call handler methods directly**

Replace `agent._handle_mail(args)` → `agent._intrinsics["mail"](args)`
Replace `agent._handle_clock(args)` → `agent._intrinsics["clock"](args)`
Replace `agent._handle_status(args)` → `agent._intrinsics["status"](args)`
Replace `agent._handle_system(args)` → `agent._intrinsics["system"](args)`

In `test_conscience.py`, update the identity assertion:
```python
# Old: assert agent._intrinsics["clock"] == agent._handle_clock
# New: just verify it's callable
assert callable(agent._intrinsics["clock"])
```

- [ ] **Step 6: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All pass

- [ ] **Step 7: Smoke-test import**

Run: `python -c "import stoai"`
Expected: No errors

- [ ] **Step 8: Commit**

```bash
git add src/stoai/intrinsics/ src/stoai/base_agent.py tests/
git commit -m "refactor: move intrinsic handlers into intrinsics/*.py modules"
```

---

## Chunk 3: ToolExecutor Extraction

### Task 4: Create ToolExecutor with tests

**Files:**
- Create: `src/stoai/tool_executor.py`
- Create: `tests/test_tool_executor.py`

- [ ] **Step 1: Write failing tests for ToolExecutor**

```python
# tests/test_tool_executor.py
"""Tests for ToolExecutor — sequential and parallel tool execution."""
from __future__ import annotations

import time
from unittest.mock import MagicMock

import pytest

from stoai.llm.base import ToolCall
from stoai.loop_guard import LoopGuard
from stoai.tool_executor import ToolExecutor


def make_executor(dispatch_fn=None, parallel_safe=None):
    if dispatch_fn is None:
        dispatch_fn = lambda tc: {"status": "ok", "result": f"ran {tc.name}"}
    make_result = MagicMock(side_effect=lambda name, result, **kw: {"name": name, "result": result})
    guard = LoopGuard(max_total_calls=50)
    return ToolExecutor(
        dispatch_fn=dispatch_fn,
        make_tool_result_fn=make_result,
        guard=guard,
        parallel_safe_tools=parallel_safe or set(),
    )


def test_execute_single_tool():
    executor = make_executor()
    calls = [ToolCall(name="read", args={"path": "/tmp"}, id="tc1")]
    results, intercepted, text = executor.execute(calls)
    assert len(results) == 1
    assert not intercepted


def test_execute_sequential_multiple():
    order = []
    def dispatch(tc):
        order.append(tc.name)
        return {"status": "ok"}
    executor = make_executor(dispatch_fn=dispatch)
    calls = [
        ToolCall(name="a", args={}, id="1"),
        ToolCall(name="b", args={}, id="2"),
    ]
    results, intercepted, text = executor.execute(calls)
    assert len(results) == 2
    assert order == ["a", "b"]  # sequential


def test_execute_parallel():
    def dispatch(tc):
        time.sleep(0.05)
        return {"status": "ok", "tool": tc.name}
    executor = make_executor(
        dispatch_fn=dispatch,
        parallel_safe={"a", "b"},
    )
    calls = [
        ToolCall(name="a", args={}, id="1"),
        ToolCall(name="b", args={}, id="2"),
    ]
    t0 = time.monotonic()
    results, intercepted, text = executor.execute(calls)
    elapsed = time.monotonic() - t0
    assert len(results) == 2
    assert elapsed < 0.15  # parallel, not 0.1+


def test_intercept_hook():
    executor = make_executor()
    hook = MagicMock(return_value="intercepted!")
    calls = [ToolCall(name="read", args={}, id="1")]
    results, intercepted, text = executor.execute(calls, on_result_hook=hook)
    assert intercepted
    assert text == "intercepted!"


def test_unknown_tool_error():
    def dispatch(tc):
        from stoai.types import UnknownToolError
        raise UnknownToolError(tc.name)
    executor = make_executor(dispatch_fn=dispatch)
    calls = [ToolCall(name="bogus", args={}, id="1")]
    errors = []
    results, intercepted, text = executor.execute(calls, collected_errors=errors)
    assert len(results) == 1
    assert "bogus" in errors[0]
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_tool_executor.py -v`
Expected: FAIL — `ModuleNotFoundError`

- [ ] **Step 3: Implement ToolExecutor**

Create `src/stoai/tool_executor.py`. Extract `_execute_single_tool`, `_execute_tools_sequential`, `_execute_tools_parallel` from `base_agent.py`. Key changes:
- `self._dispatch_tool(tc)` → `self._dispatch_fn(tc)`
- `self.service.make_tool_result(...)` → `self._make_tool_result_fn(...)`
- `self._on_tool_result_hook(...)` → `on_result_hook(...)` callback
- `self._cancel_event` → `cancel_event` parameter on `execute()`
- `self._log(...)` → `self._logger_fn(...)` if provided
- `self._PARALLEL_SAFE_TOOLS` → `self._parallel_safe_tools`

```python
# src/stoai/tool_executor.py
"""ToolExecutor — sequential and parallel tool call execution."""
from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any, Callable

from .llm.base import ToolCall
from .loop_guard import LoopGuard
from .tool_timing import ToolTimer, stamp_tool_result
from .types import UnknownToolError


class ToolExecutor:
    """Executes tool calls sequentially or in parallel."""

    def __init__(
        self,
        dispatch_fn: Callable[[ToolCall], Any],
        make_tool_result_fn: Callable,
        guard: LoopGuard,
        known_tools: set[str] | None = None,
        parallel_safe_tools: set[str] | None = None,
        logger_fn: Callable | None = None,
    ) -> None:
        self._dispatch_fn = dispatch_fn
        self._make_tool_result_fn = make_tool_result_fn
        self._guard = guard
        self._known_tools = known_tools or set()
        self._parallel_safe_tools = parallel_safe_tools or set()
        self._logger_fn = logger_fn

    @property
    def guard(self) -> LoopGuard:
        return self._guard

    @guard.setter
    def guard(self, value: LoopGuard) -> None:
        self._guard = value

    def _log(self, event_type: str, **fields) -> None:
        if self._logger_fn:
            self._logger_fn(event_type, **fields)

    def execute(
        self,
        tool_calls: list[ToolCall],
        *,
        on_result_hook: Callable | None = None,
        cancel_event: Any | None = None,
        collected_errors: list[str] | None = None,
    ) -> tuple[list, bool, str]:
        """Execute tool calls. Returns (results, intercepted, intercept_text)."""
        if collected_errors is None:
            collected_errors = []

        all_parallel_safe = (
            len(tool_calls) > 1
            and self._parallel_safe_tools
            and all(tc.name in self._parallel_safe_tools for tc in tool_calls)
        )

        if all_parallel_safe:
            return self._execute_parallel(
                tool_calls, collected_errors,
                on_result_hook=on_result_hook,
                cancel_event=cancel_event,
            )
        else:
            return self._execute_sequential(
                tool_calls, collected_errors,
                on_result_hook=on_result_hook,
                cancel_event=cancel_event,
            )

    def _execute_single(
        self,
        tc: ToolCall,
        collected_errors: list[str],
        *,
        on_result_hook: Callable | None = None,
    ) -> tuple[Any, bool, str]:
        tc_id = getattr(tc, "id", None)
        args = dict(tc.args) if tc.args else {}
        reasoning = args.pop("reasoning", None)
        args.pop("commentary", None)
        args.pop("_sync", None)

        if reasoning:
            self._log("tool_reasoning", tool=tc.name, reasoning=reasoning)
            args["_reasoning"] = reasoning

        verdict = self._guard.record_tool_call(tc.name, args)
        if verdict.blocked:
            result = {
                "status": "blocked",
                "_duplicate_warning": verdict.warning,
                "message": f"Execution skipped — duplicate call #{verdict.count}",
            }
            msg = self._make_tool_result_fn(tc.name, result, tool_call_id=tc_id)
            self._log("tool_result", tool_name=tc.name, status="blocked", elapsed_ms=0)
            return msg, False, ""

        self._log("tool_call", tool_name=tc.name, tool_args=args)
        timer = ToolTimer()
        try:
            # Pre-check for unknown tool (records in guard for limit tracking)
            if self._known_tools and tc.name not in self._known_tools:
                self._guard.record_invalid_tool(tc.name)
                raise UnknownToolError(tc.name)

            with timer:
                result = self._dispatch_fn(
                    ToolCall(name=tc.name, args=args, id=tc_id)
                )

            if isinstance(result, dict):
                stamp_tool_result(result, timer.elapsed_ms)

            status = result.get("status", "success") if isinstance(result, dict) else "success"
            self._log("tool_result", tool_name=tc.name, status=status, elapsed_ms=timer.elapsed_ms)

            if verdict.warning and isinstance(result, dict):
                result["_duplicate_warning"] = verdict.warning

            if isinstance(result, dict) and result.get("intercept"):
                intercept_text = result.get("text", "")
                result_msg = self._make_tool_result_fn(tc.name, result, tool_call_id=tc_id)
                return result_msg, True, intercept_text

            result_msg = self._make_tool_result_fn(tc.name, result, tool_call_id=tc_id)

            if isinstance(result, dict) and result.get("status") == "error":
                err_msg = result.get("message", "unknown error")
                collected_errors.append(f"{tc.name}: {err_msg}")

            if on_result_hook is not None:
                intercept = on_result_hook(tc.name, args, result)
                if intercept is not None:
                    return result_msg, True, intercept

            return result_msg, False, ""

        except Exception as e:
            err_result = {"status": "error", "message": str(e)}
            stamp_tool_result(err_result, timer.elapsed_ms)
            result_msg = self._make_tool_result_fn(tc.name, err_result, tool_call_id=tc_id)
            collected_errors.append(f"{tc.name}: {e}")
            self._log("error", source=tc.name, message=str(e))
            return result_msg, False, ""

    def _execute_sequential(
        self,
        tool_calls: list[ToolCall],
        collected_errors: list[str],
        *,
        on_result_hook: Callable | None = None,
        cancel_event: Any | None = None,
    ) -> tuple[list, bool, str]:
        tool_results = []
        for tc in tool_calls:
            if cancel_event is not None and cancel_event.is_set():
                return [], False, ""
            result_msg, intercepted, intercept_text = self._execute_single(
                tc, collected_errors, on_result_hook=on_result_hook,
            )
            if result_msg is not None:
                tool_results.append(result_msg)
            if intercepted:
                return tool_results, True, intercept_text
        return tool_results, False, ""

    def _execute_parallel(
        self,
        tool_calls: list[ToolCall],
        collected_errors: list[str],
        *,
        on_result_hook: Callable | None = None,
        cancel_event: Any | None = None,
    ) -> tuple[list, bool, str]:
        # Phase 1: Pre-check duplicates (sequential — guard not thread-safe)
        to_execute: list[tuple[int, ToolCall, dict]] = []
        tool_results: list[tuple[int, Any]] = []

        for i, tc in enumerate(tool_calls):
            tc_id = getattr(tc, "id", None)
            args = dict(tc.args) if tc.args else {}
            reasoning = args.pop("reasoning", None)
            args.pop("commentary", None)
            args.pop("_sync", None)

            if reasoning:
                self._log("tool_reasoning", tool=tc.name, reasoning=reasoning)
                args["_reasoning"] = reasoning

            verdict = self._guard.record_tool_call(tc.name, args)
            if verdict.blocked:
                result = {
                    "status": "blocked",
                    "_duplicate_warning": verdict.warning,
                    "message": f"Execution skipped — duplicate call #{verdict.count}",
                }
                tool_results.append((i, self._make_tool_result_fn(
                    tc.name, result, tool_call_id=tc_id,
                )))
            else:
                to_execute.append((i, tc, args))

        if not to_execute:
            tool_results.sort(key=lambda x: x[0])
            return [r for _, r in tool_results], False, ""

        # Phase 2: Execute in parallel
        results_map: dict[int, Any] = {}
        errors_map: dict[int, str] = {}

        def _run_one(index: int, tc: ToolCall, args: dict):
            timer = ToolTimer()
            with timer:
                result = self._dispatch_fn(
                    ToolCall(name=tc.name, args=args, id=tc.id)
                )
            if isinstance(result, dict):
                stamp_tool_result(result, timer.elapsed_ms)
            return index, result

        pool = ThreadPoolExecutor(max_workers=len(to_execute))
        try:
            futures = {
                pool.submit(_run_one, i, tc, args): i
                for i, tc, args in to_execute
            }
            for future in as_completed(futures, timeout=300.0):
                if cancel_event is not None and cancel_event.is_set():
                    pool.shutdown(wait=False, cancel_futures=True)
                    return [], False, ""
                try:
                    idx, result = future.result()
                    results_map[idx] = result
                except Exception as e:
                    idx = futures[future]
                    errors_map[idx] = str(e)
        except TimeoutError:
            for future, idx in futures.items():
                if idx not in results_map and idx not in errors_map:
                    errors_map[idx] = "Timed out"
        finally:
            pool.shutdown(wait=False, cancel_futures=True)

        # Phase 3: Build result messages (sequential)
        for i, tc, args in to_execute:
            tc_id = getattr(tc, "id", None)
            if i in results_map:
                result = results_map[i]
                tool_results.append((i, self._make_tool_result_fn(
                    tc.name, result, tool_call_id=tc_id,
                )))
                if isinstance(result, dict) and result.get("status") == "error":
                    err_msg = result.get("message", "unknown error")
                    collected_errors.append(f"{tc.name}: {err_msg}")
                if isinstance(result, dict) and result.get("intercept"):
                    tool_results.sort(key=lambda x: x[0])
                    return (
                        [r for _, r in tool_results],
                        True,
                        result.get("text", ""),
                    )
            elif i in errors_map:
                err_msg = errors_map[i]
                err_result = {"status": "error", "message": err_msg}
                tool_results.append((i, self._make_tool_result_fn(
                    tc.name, err_result, tool_call_id=tc_id,
                )))
                collected_errors.append(f"{tc.name}: {err_msg}")

        tool_results.sort(key=lambda x: x[0])
        return [r for _, r in tool_results], False, ""
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_tool_executor.py -v`
Expected: All PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from stoai.tool_executor import ToolExecutor; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add src/stoai/tool_executor.py tests/test_tool_executor.py
git commit -m "feat: extract ToolExecutor from BaseAgent"
```

### Task 5: Wire ToolExecutor into BaseAgent

**Files:**
- Modify: `src/stoai/base_agent.py`

- [ ] **Step 1: Update BaseAgent to use ToolExecutor**

1. Add import: `from .tool_executor import ToolExecutor`
2. In `_handle_request()`:
   - Create ToolExecutor with `dispatch_fn=self._dispatch_tool`, `make_tool_result_fn=lambda name, result, **kw: self.service.make_tool_result(name, result, provider=self._config.provider, **kw)`, `guard=guard`, `known_tools=set(self._intrinsics) | set(self._mcp_handlers)`, `parallel_safe_tools=self._PARALLEL_SAFE_TOOLS`, `logger_fn=self._log`
   - Store as `self._executor` (needed by `_process_response`)
3. In `_process_response()`:
   - Replace the sequential/parallel dispatch block with `self._executor.execute(response.tool_calls, on_result_hook=self._on_tool_result_hook, cancel_event=self._cancel_event, collected_errors=collected_errors)`
   - Remove `all_parallel_safe` logic, `_execute_tools_sequential` call, `_execute_tools_parallel` call
   - Update `guard.check_limit(...)` and `guard.check_invalid_tool_limit()` calls to use `self._executor.guard.check_limit(...)` and `self._executor.guard.check_invalid_tool_limit()`
   - Update `guard.record_calls(...)` to `self._executor.guard.record_calls(...)`
4. Remove methods: `_execute_single_tool`, `_execute_tools_sequential`, `_execute_tools_parallel`

- [ ] **Step 2: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All pass

- [ ] **Step 3: Smoke-test import**

Run: `python -c "import stoai"`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add src/stoai/base_agent.py
git commit -m "refactor: wire ToolExecutor into BaseAgent, remove extracted methods"
```

---

## Chunk 4: SessionManager Extraction

### Task 6: Create SessionManager with tests

**Files:**
- Create: `src/stoai/session.py`
- Create: `tests/test_session.py`

- [ ] **Step 1: Write failing tests for SessionManager**

```python
# tests/test_session.py
"""Tests for SessionManager — LLM session, token tracking, compaction."""
from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from stoai.session import SessionManager
from stoai.config import AgentConfig
from stoai.prompt import SystemPromptManager


def make_session_manager(**kw):
    svc = MagicMock()
    svc.model = "test-model"
    mock_session = MagicMock()
    mock_session.context_window.return_value = 100000
    mock_session.interface.estimate_context_tokens.return_value = 5000
    svc.create_session.return_value = mock_session
    config = kw.get("config", AgentConfig())
    pm = SystemPromptManager()
    return SessionManager(
        llm_service=svc,
        config=config,
        prompt_manager=pm,
        build_system_prompt_fn=lambda: "test prompt",
        build_tool_schemas_fn=lambda: [],
        agent_id="test",
    )


def test_ensure_session_creates_on_first_call():
    sm = make_session_manager()
    session = sm.ensure_session()
    assert session is not None
    assert sm.chat is not None


def test_ensure_session_reuses_existing():
    sm = make_session_manager()
    s1 = sm.ensure_session()
    s2 = sm.ensure_session()
    assert s1 is s2


def test_track_usage_accumulates():
    sm = make_session_manager()
    response = MagicMock()
    response.usage.input_tokens = 100
    response.usage.output_tokens = 50
    response.usage.thinking_tokens = 10
    response.usage.cached_tokens = 20
    sm.track_usage(response)
    usage = sm.get_token_usage()
    assert usage["input_tokens"] == 100
    assert usage["output_tokens"] == 50


def test_get_chat_state_empty_when_no_session():
    sm = make_session_manager()
    assert sm.get_chat_state() == {}


def test_restore_token_state():
    sm = make_session_manager()
    sm.restore_token_state({
        "input_tokens": 500,
        "output_tokens": 200,
        "thinking_tokens": 50,
        "cached_tokens": 100,
        "api_calls": 3,
    })
    usage = sm.get_token_usage()
    assert usage["input_tokens"] == 500
    assert usage["api_calls"] == 3
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_session.py -v`
Expected: FAIL — `ModuleNotFoundError`

- [ ] **Step 3: Implement SessionManager**

Create `src/stoai/session.py`. Extract `_ensure_session`, `_llm_send`, `_llm_send_streaming`, `_on_reset`, `_check_and_compact`, `_update_token_decomposition`, `_track_usage`, `get_token_usage`, `get_chat_state`, `restore_chat`, `restore_token_state` from `base_agent.py`.

Key changes from the original:
- All `self._chat` state lives on SessionManager
- `self._build_system_prompt()` → `self._build_system_prompt_fn()`
- `self._build_tool_schemas()` → `self._build_tool_schemas_fn()`
- `self.service` → `self._llm_service`
- Token tracking state (`_total_input_tokens`, etc.) moves here
- `_interaction_id` moves here
- `_timeout_pool` moves here
- `_streaming` flag received from BaseAgent
- `agent_id` passed for logging and session creation

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_session.py -v`
Expected: All PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "from stoai.session import SessionManager; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add src/stoai/session.py tests/test_session.py
git commit -m "feat: extract SessionManager from BaseAgent"
```

### Task 7: Wire SessionManager into BaseAgent

**Files:**
- Modify: `src/stoai/base_agent.py`
- Modify: `src/stoai/capabilities/anima.py` (update `_chat` references)

- [ ] **Step 1: Update BaseAgent to use SessionManager**

1. Add import: `from .session import SessionManager`
2. In `__init__`: create `self._session = SessionManager(...)` with the relevant args. Remove token tracking state variables (`_total_input_tokens`, `_total_output_tokens`, etc.), `_interaction_id`, `_timeout_pool`.
3. Keep `self._chat` as a property: `return self._session.chat`
4. In `_handle_request()`: replace `self._llm_send(content)` with `self._session.send(content)`
5. In `_process_response()`: replace `self._llm_send(tool_results)` with `self._session.send(tool_results)`
6. Replace all `self._track_usage(response)` with `self._session.track_usage(response)`
7. Replace `self._ensure_session()` with `self._session.ensure_session()`
8. Replace `self._check_and_compact()` with `self._session.check_and_compact()`
9. Thin wrappers for public API: `get_token_usage()`, `get_chat_state()`, `restore_chat()`, `restore_token_state()`
10. Remove extracted methods from BaseAgent
11. Update `_token_decomp_dirty` references — this now lives on SessionManager

- [ ] **Step 2: Update anima capability (remaining references)**

Note: `_system_diff` and `_git_diff_and_commit` references were already updated in Task 2 (WorkingDir wiring). This step handles the SessionManager-related references:

Replace `self._agent._chat` → `self._agent._session.chat`
Replace `self._agent._interaction_id` → `self._agent._session._interaction_id`
Replace `self._agent._token_decomp_dirty` → `self._agent._session._token_decomp_dirty`

- [ ] **Step 3: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All pass

- [ ] **Step 4: Smoke-test import**

Run: `python -c "import stoai"`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add src/stoai/base_agent.py src/stoai/session.py src/stoai/capabilities/anima.py
git commit -m "refactor: wire SessionManager into BaseAgent, remove extracted methods"
```

---

## Chunk 5: Final Verification

### Task 8: Full test suite and line count verification

- [ ] **Step 1: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: All pass

- [ ] **Step 2: Verify line counts**

Run: `wc -l src/stoai/base_agent.py src/stoai/workdir.py src/stoai/tool_executor.py src/stoai/session.py`
Expected: `base_agent.py` should be ~500 lines, total across all files should be approximately the original 1940.

- [ ] **Step 3: Smoke-test all imports**

Run: `python -c "from stoai.workdir import WorkingDir; from stoai.tool_executor import ToolExecutor; from stoai.session import SessionManager; import stoai; print('All OK')"`
Expected: `All OK`

- [ ] **Step 4: Verify no circular imports**

Run: `python -c "import stoai.base_agent; import stoai.workdir; import stoai.tool_executor; import stoai.session; print('No circular imports')"`
Expected: `No circular imports`
