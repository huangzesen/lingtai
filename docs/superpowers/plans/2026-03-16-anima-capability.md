# Anima Capability Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the `anima` capability that upgrades the `system` intrinsic with evolving role (covenant + character), structured memory, and on-demand context compaction.

**Architecture:** Four implementation chunks: (1) simplify the system intrinsic (rename files, remove view, remove role object), (2) add `override_intrinsic()` to BaseAgent and fix email to use it, (3) build the anima capability with AnimaManager, (4) update tests for the full stack.

**Tech Stack:** Python 3.11+, dataclasses, json, hashlib, subprocess (git), pytest

**Spec:** `docs/specs/2026-03-16-anima-capability-design.md`

---

## Chunk 1: Simplify System Intrinsic

Rename `role.md` → `covenant.md`, `ltm.md` → `memory.md`. Remove `view` action and `role` object. System intrinsic becomes: `memory` object with `diff`/`load` only. Covenant is injected at construction as a protected prompt section — no tool access needed.

### File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `src/lingtai/intrinsics/system.py` | New schema: memory object, diff/load only |
| Modify | `src/lingtai/base_agent.py:175-212` | Rename role→covenant, ltm→memory file paths in constructor |
| Modify | `src/lingtai/base_agent.py:517-611` | _handle_system: accept memory, remove role/view |
| Modify | `src/lingtai/base_agent.py:750-755` | stop(): persist "memory" section, not "ltm" |
| Modify | `src/lingtai/base_agent.py:792-868` | _git_init_working_dir: create covenant.md/memory.md |
| Modify | `src/lingtai/base_agent.py:870-889` | _read_manifest: read "role" key, map to covenant |
| Modify | `src/lingtai/base_agent.py:891-904` | _write_manifest: read "covenant" section, not "role" |
| Modify | `src/lingtai/prompt.py:52-58` | Update BASE_PROMPT text (role→covenant, LTM→memory) |
| Modify | `tests/test_system.py` | Rewrite ALL tests for new schema (remove old view/role tests) |
| Modify | `tests/test_agent.py:441-449` | Update resume test (role→covenant, ltm→memory) |

---

### Task 1.1: Update system intrinsic schema

**Files:**
- Modify: `src/lingtai/intrinsics/system.py`
- Test: `tests/test_system.py`

- [ ] **Step 1: Write failing tests for new schema**

Replace the existing schema tests in `tests/test_system.py`:

```python
def test_system_in_all_intrinsics():
    assert "system" in ALL_INTRINSICS
    info = ALL_INTRINSICS["system"]
    schema = info["schema"]
    # Only memory object
    assert schema["properties"]["object"]["enum"] == ["memory"]
    # Only diff and load actions
    assert schema["properties"]["action"]["enum"] == ["diff", "load"]


def test_system_wired_in_agent(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "system" in agent._intrinsics
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_system.py::test_system_in_all_intrinsics -v`
Expected: FAIL (schema still has role/ltm and view)

- [ ] **Step 3: Update intrinsics/system.py**

Replace the full file:

```python
"""System intrinsic — agent memory management.

Actions:
    diff   — show uncommitted git diff for memory.md
    load   — read the file, inject into live system prompt, git add+commit

Objects:
    memory — system/memory.md (the agent's long-term memory)

The handler lives in BaseAgent (needs access to working_dir, prompt_manager, git).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["diff", "load"],
            "description": (
                "diff: show uncommitted git diff (what changed since last commit).\n"
                "load: read the file, inject into the live system prompt, "
                "and git commit. This updates the agent's live memory."
            ),
        },
        "object": {
            "type": "string",
            "enum": ["memory"],
            "description": "memory: the agent's long-term memory (system/memory.md).",
        },
    },
    "required": ["action", "object"],
}

DESCRIPTION = (
    "Agent memory management. Long-term memory lives in system/memory.md. "
    "Use 'diff' to see uncommitted changes, "
    "and 'load' to apply changes into the live system prompt (with git commit)."
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_system.py::test_system_in_all_intrinsics tests/test_system.py::test_system_wired_in_agent -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/intrinsics/system.py tests/test_system.py
git commit -m "refactor: simplify system intrinsic schema to memory diff/load only"
```

---

### Task 1.2: Update BaseAgent file paths and constructor

**Files:**
- Modify: `src/lingtai/base_agent.py:175-212`
- Test: `tests/test_system.py`

- [ ] **Step 1: Write failing tests**

Add to `tests/test_system.py`:

```python
def test_covenant_constructor_arg_writes_to_system(tmp_path):
    """covenant= constructor arg should write to system/covenant.md."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        role="You are a helpful agent",
    )
    covenant_file = agent.working_dir / "system" / "covenant.md"
    assert covenant_file.is_file()
    assert covenant_file.read_text() == "You are a helpful agent"
    agent.stop(timeout=1.0)


def test_memory_constructor_arg_writes_to_system(tmp_path):
    """ltm= constructor arg should write to system/memory.md."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        ltm="initial memory",
    )
    memory_file = agent.working_dir / "system" / "memory.md"
    assert memory_file.is_file()
    assert memory_file.read_text() == "initial memory"
    agent.stop(timeout=1.0)


def test_covenant_is_protected_section(tmp_path):
    """Covenant should be a protected prompt section."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        role="researcher",
    )
    sections = agent._prompt_manager.list_sections()
    covenant_section = [s for s in sections if s["name"] == "covenant"]
    assert len(covenant_section) == 1
    assert covenant_section[0]["protected"] is True
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_system.py::test_covenant_constructor_arg_writes_to_system tests/test_system.py::test_memory_constructor_arg_writes_to_system tests/test_system.py::test_covenant_is_protected_section -v`
Expected: FAIL

- [ ] **Step 3: Update BaseAgent constructor**

In `base_agent.py`, update the file path section (around lines 175-212). The constructor params stay as `role=` and `ltm=` for backward compat at the API level, but files are renamed:

```python
# LTM and role file paths — renamed to covenant/memory
system_dir = self._working_dir / "system"
memory_file = system_dir / "memory.md"
covenant_file = system_dir / "covenant.md"

# If constructor ltm is provided and memory file doesn't exist, write it
if ltm and not memory_file.is_file():
    system_dir.mkdir(exist_ok=True)
    memory_file.write_text(ltm)
# If manifest has ltm and file doesn't exist, migrate
elif manifest_ltm and not memory_file.is_file():
    system_dir.mkdir(exist_ok=True)
    memory_file.write_text(manifest_ltm)

# If constructor role is provided and covenant file doesn't exist, write it
if role and not covenant_file.is_file():
    system_dir.mkdir(exist_ok=True)
    covenant_file.write_text(role)

# Auto-load memory from file into prompt manager
loaded_memory = ""
if memory_file.is_file():
    loaded_memory = memory_file.read_text()

# System prompt manager
self._prompt_manager = SystemPromptManager()
if role:
    self._prompt_manager.write_section("covenant", role, protected=True)
if loaded_memory.strip():
    self._prompt_manager.write_section("memory", loaded_memory)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_system.py::test_covenant_constructor_arg_writes_to_system tests/test_system.py::test_memory_constructor_arg_writes_to_system tests/test_system.py::test_covenant_is_protected_section -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/base_agent.py tests/test_system.py
git commit -m "refactor: rename role.md→covenant.md, ltm.md→memory.md in BaseAgent"
```

---

### Task 1.3: Update _handle_system handler

**Files:**
- Modify: `src/lingtai/base_agent.py:517-611`
- Test: `tests/test_system.py`

- [ ] **Step 1: Write failing tests**

Replace the handler tests in `tests/test_system.py`:

```python
def test_system_diff_memory(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        memory_file = agent.working_dir / "system" / "memory.md"
        memory_file.write_text("first version\n")
        agent._handle_system({"action": "load", "object": "memory"})
        memory_file.write_text("second version\n")
        result = agent._handle_system({"action": "diff", "object": "memory"})
        assert result["status"] == "ok"
        assert "first version" in result["git_diff"] or "second version" in result["git_diff"]
    finally:
        agent.stop()


def test_system_load_memory(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        memory_file = agent.working_dir / "system" / "memory.md"
        memory_file.write_text("# Memory\n\nimportant fact\n")
        result = agent._handle_system({"action": "load", "object": "memory"})
        assert result["status"] == "ok"
        assert result["diff"]["changed"] is True
        section = agent._prompt_manager.read_section("memory")
        assert "important fact" in section
    finally:
        agent.stop()


def test_system_load_empty_removes_section(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        memory_file = agent.working_dir / "system" / "memory.md"
        memory_file.write_text("some content")
        agent._handle_system({"action": "load", "object": "memory"})
        assert agent._prompt_manager.read_section("memory") is not None
        memory_file.write_text("")
        agent._handle_system({"action": "load", "object": "memory"})
        section = agent._prompt_manager.read_section("memory")
        assert section is None or section.strip() == ""
    finally:
        agent.stop()


def test_system_diff_no_changes(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._handle_system({"action": "diff", "object": "memory"})
        assert result["status"] == "ok"
        assert result["git_diff"] == ""
    finally:
        agent.stop()


def test_system_load_no_change_no_commit(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        agent._handle_system({"action": "load", "object": "memory"})
        result = agent._handle_system({"action": "load", "object": "memory"})
        assert result["diff"]["changed"] is False
    finally:
        agent.stop()


def test_system_unknown_action(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_system({"action": "view", "object": "memory"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_system_unknown_object(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_system({"action": "diff", "object": "role"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_system_creates_files_if_missing(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        import shutil
        system_dir = agent.working_dir / "system"
        if system_dir.exists():
            shutil.rmtree(system_dir)
        result = agent._handle_system({"action": "diff", "object": "memory"})
        assert result["status"] == "ok"
        assert (agent.working_dir / "system" / "memory.md").is_file()
    finally:
        agent.stop()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_system.py::test_system_diff_memory tests/test_system.py::test_system_unknown_object -v`
Expected: FAIL

- [ ] **Step 3: Update _handle_system**

Replace the handler in `base_agent.py`:

```python
def _handle_system(self, args: dict) -> dict:
    """Handle system tool — agent memory management."""
    action = args.get("action", "")
    obj = args.get("object", "")
    if obj != "memory":
        return {"error": f"Unknown object: {obj!r}. Must be 'memory'."}

    system_dir = self._working_dir / "system"
    system_dir.mkdir(exist_ok=True)
    file_path = system_dir / "memory.md"
    if not file_path.is_file():
        file_path.write_text("")

    if action == "diff":
        return self._system_diff(file_path, "memory")
    elif action == "load":
        return self._system_load(file_path, "memory")
    else:
        return {"error": f"Unknown action: {action!r}. Must be 'diff' or 'load'."}
```

Remove `_system_view` method entirely. Update `_system_load` to use section name `"memory"` (not `"role"`/`"ltm"`) — the `protected` flag logic changes: memory is never protected at the kernel level.

```python
def _system_load(self, file_path: Path, obj: str) -> dict:
    """Read a system file, inject into system prompt, git commit."""
    content = file_path.read_text()
    size_bytes = len(content.encode("utf-8"))

    # Inject into system prompt (or remove if empty)
    if content.strip():
        self._prompt_manager.write_section(obj, content)
    else:
        self._prompt_manager.delete_section(obj)
    self._token_decomp_dirty = True

    # Update live session's system prompt if one exists
    if self._chat is not None:
        self._chat.update_system_prompt(self._build_system_prompt())

    # Git diff + commit
    rel_path = f"system/{obj}.md"
    git_diff, commit_hash = self._git_diff_and_commit(rel_path, obj)

    self._log(f"system_load_{obj}", size_bytes=size_bytes, changed=commit_hash is not None)

    return {
        "status": "ok",
        "path": str(file_path),
        "size_bytes": size_bytes,
        "content_preview": content[:200],
        "diff": {
            "changed": commit_hash is not None,
            "git_diff": git_diff or "",
            "commit": commit_hash,
        },
    }
```

- [ ] **Step 4: Run all system tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_system.py -v`
Expected: ALL PASS

- [ ] **Step 5: Update stop() to persist "memory" section**

In `base_agent.py` around line 750, change:

```python
# Persist memory from prompt manager to file
memory_content = self._prompt_manager.read_section("memory") or ""
memory_file = self._working_dir / "system" / "memory.md"
if memory_file.is_file() or memory_content:
    memory_file.parent.mkdir(exist_ok=True)
    memory_file.write_text(memory_content)
```

- [ ] **Step 6: Update _write_manifest() to read "covenant" section**

In `base_agent.py` around line 891, change `read_section("role")` to `read_section("covenant")`:

```python
def _write_manifest(self) -> None:
    """Write .agent.json atomically."""
    from datetime import datetime, timezone
    data = {
        "agent_id": self.agent_id,
        "started_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "role": self._prompt_manager.read_section("covenant") or "",
    }
    # ... rest unchanged
```

Note: the manifest key stays `"role"` for backward compat with existing `.agent.json` files. Only the section name changes.

- [ ] **Step 7: Update _git_init_working_dir() to create covenant.md/memory.md**

In `base_agent.py` around lines 836-868, rename all occurrences of `role.md` → `covenant.md` and `ltm.md` → `memory.md`:

```python
# Create system/ directory with covenant.md and memory.md
system_dir = self._working_dir / "system"
system_dir.mkdir(exist_ok=True)
covenant_file = system_dir / "covenant.md"
if not covenant_file.is_file():
    covenant_file.write_text("")
memory_file = system_dir / "memory.md"
if not memory_file.is_file():
    memory_file.write_text("")
```

Apply the same change in the fallback block (lines 861-868).

- [ ] **Step 8: Update BASE_PROMPT in prompt.py**

Update the text to reflect the new naming:

```python
BASE_PROMPT = """\
# System Prompt

Your text responses are your private diary — not visible to anyone. All external communication and actions are done through tools.
Read your tool schemas carefully for capabilities, caveats and pipelines.
Your working directory is your identity — all your state, memory, and files live there.
Your covenant and memory sections below may be updated mid-session.
Automatic context compaction triggers at 80% of your context window — earlier conversation will be summarized to free space."""
```

- [ ] **Step 9: Remove ALL old tests from test_system.py**

Delete every test that references `view` action, `role` object, `ltm.md`, or `role.md` paths. The old tests to remove:
- `test_memory_not_in_all_intrinsics`
- `test_system_view_ltm_empty`
- `test_system_view_role_empty`
- `test_system_view_ltm_with_content`
- `test_system_load_ltm` (replaced by new `test_system_load_memory`)
- `test_system_load_role`
- `test_system_diff_ltm` (replaced by new `test_system_diff_memory`)
- `test_ltm_constructor_arg_writes_to_system` (replaced by new `test_memory_constructor_arg_writes_to_system`)
- `test_role_constructor_arg_writes_to_system` (replaced by new `test_covenant_constructor_arg_writes_to_system`)
- `test_existing_system_files_not_overwritten` (update to use `memory.md`)

The file should contain only the new tests written in Steps 1 and Task 1.2 Step 1.

- [ ] **Step 10: Update resume test in test_agent.py**

Find and update `test_agent_resume_reads_role_ltm` to use new section names:

```python
def test_agent_resume_reads_covenant_memory(tmp_path):
    agent = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path,
        role="researcher", ltm="knows python",
    )
    agent.stop()
    agent2 = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    assert agent2._prompt_manager.read_section("covenant") == "researcher"
    assert agent2._prompt_manager.read_section("memory") == "knows python"
```

- [ ] **Step 11: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_system.py tests/test_agent.py tests/test_prompt.py -v`
Expected: ALL PASS

- [ ] **Step 12: Commit**

```bash
git add src/lingtai/base_agent.py src/lingtai/prompt.py tests/test_system.py tests/test_agent.py
git commit -m "refactor: rename role→covenant, ltm→memory in kernel, remove system view"
```

---

## Chunk 2: Override Intrinsic Mechanism + Email Fix

Add `override_intrinsic()` to BaseAgent. Fix email to use it (remove mail intrinsic when email is active).

### File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `src/lingtai/base_agent.py` | Add override_intrinsic() method |
| Modify | `src/lingtai/capabilities/email.py:531-538` | Use override_intrinsic("mail") |
| Create | `tests/test_override_intrinsic.py` | Tests for the new mechanism |
| Modify | `tests/test_layers_email.py` | Verify mail intrinsic removed when email active |

---

### Task 2.1: Add override_intrinsic to BaseAgent

**Files:**
- Modify: `src/lingtai/base_agent.py`
- Create: `tests/test_override_intrinsic.py`

- [ ] **Step 1: Write failing tests**

Create `tests/test_override_intrinsic.py`:

```python
"""Tests for BaseAgent.override_intrinsic() — capability upgrade mechanism."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from lingtai.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_override_intrinsic_removes_from_dict(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "system" in agent._intrinsics
    agent.override_intrinsic("system")
    assert "system" not in agent._intrinsics
    agent.stop(timeout=1.0)


def test_override_intrinsic_returns_original_handler(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    original = agent._intrinsics["system"]
    returned = agent.override_intrinsic("system")
    assert returned is original
    agent.stop(timeout=1.0)


def test_override_intrinsic_raises_after_start(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.override_intrinsic("system")
    finally:
        agent.stop(timeout=2.0)


def test_override_intrinsic_raises_unknown(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(KeyError):
        agent.override_intrinsic("nonexistent")
    agent.stop(timeout=1.0)


def test_override_intrinsic_tool_no_longer_visible(tmp_path):
    """After override, the intrinsic should not appear in tool schemas."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.override_intrinsic("system")
    schemas = agent._build_tool_schemas()
    schema_names = [s.name for s in schemas]
    assert "system" not in schema_names
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_override_intrinsic.py -v`
Expected: FAIL (method doesn't exist)

- [ ] **Step 3: Implement override_intrinsic**

Add to `base_agent.py`, near `add_tool` and `remove_tool` (around line 1791):

```python
def override_intrinsic(self, name: str) -> Callable[[dict], dict]:
    """Remove an intrinsic and return its handler for delegation.

    Called by capabilities that upgrade an intrinsic (email → mail,
    anima → system). Must be called before start() (tool surface sealed).

    Returns the original handler so the capability can delegate to it.
    """
    if self._sealed:
        raise RuntimeError("Cannot modify tools after start()")
    handler = self._intrinsics.pop(name)  # raises KeyError if missing
    self._token_decomp_dirty = True
    return handler
```

Also add the import at the top of the file if not present: `from typing import Callable` (check if already imported via `TYPE_CHECKING`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_override_intrinsic.py -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/base_agent.py tests/test_override_intrinsic.py
git commit -m "feat: add override_intrinsic() to BaseAgent for capability upgrades"
```

---

### Task 2.2: Fix email to use override_intrinsic

**Files:**
- Modify: `src/lingtai/capabilities/email.py:531-538`
- Modify: `tests/test_layers_email.py`

- [ ] **Step 1: Write failing test**

Add to `tests/test_layers_email.py`:

```python
def test_email_removes_mail_intrinsic(tmp_path):
    """When email capability is active, mail intrinsic should be removed."""
    from lingtai.agent import Agent
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["email"],
    )
    assert "mail" not in agent._intrinsics
    # But email tool should exist
    assert "email" in agent._mcp_handlers
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_layers_email.py::test_email_removes_mail_intrinsic -v`
Expected: FAIL (mail still in _intrinsics)

- [ ] **Step 3: Update email setup()**

In `src/lingtai/capabilities/email.py`, update the `setup` function:

```python
def setup(agent: "BaseAgent") -> EmailManager:
    """Set up email capability — filesystem-based mailbox."""
    mgr = EmailManager(agent)
    agent.override_intrinsic("mail")  # remove mail tool; email reimplements fully
    agent._on_normal_mail = mgr.on_normal_mail
    agent.add_tool(
        "email", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )
    return mgr
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_layers_email.py -v`
Expected: ALL PASS

- [ ] **Step 5: Run full test suite to check for regressions**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS (or only pre-existing failures)

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/email.py tests/test_layers_email.py
git commit -m "fix: email capability now removes mail intrinsic via override_intrinsic"
```

---

## Chunk 3: Anima Capability

Build the anima capability: AnimaManager, tool schema, setup function. Register in capabilities registry.

### File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `src/lingtai/capabilities/anima.py` | AnimaManager, SCHEMA, DESCRIPTION, setup() |
| Modify | `src/lingtai/capabilities/__init__.py:11-27` | Register anima in _BUILTIN |
| Create | `tests/test_anima.py` | Full test coverage for anima capability |

---

### Task 3.1: Create anima capability — schema and setup

**Files:**
- Create: `src/lingtai/capabilities/anima.py`
- Modify: `src/lingtai/capabilities/__init__.py`
- Create: `tests/test_anima.py`

- [ ] **Step 1: Write failing test for setup**

Create `tests/test_anima.py`:

```python
"""Tests for anima capability — self-knowledge management."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from lingtai.agent import Agent
from lingtai.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_anima_setup_removes_system_intrinsic(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    assert "system" not in agent._intrinsics
    assert "anima" in agent._mcp_handlers
    agent.stop(timeout=1.0)


def test_anima_manager_accessible(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    assert mgr is not None
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_anima.py::test_anima_setup_removes_system_intrinsic -v`
Expected: FAIL (anima not in _BUILTIN)

- [ ] **Step 3: Create anima.py with skeleton**

Create `src/lingtai/capabilities/anima.py`:

```python
"""Anima capability — self-knowledge management.

Upgrades the system intrinsic (like email upgrades mail).
Adds evolving role (covenant + character), structured memory,
and on-demand context compaction.

Usage:
    agent = Agent(capabilities=["anima"])
"""
from __future__ import annotations

import hashlib
import json
import os
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Callable

    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "object": {
            "type": "string",
            "enum": ["role", "memory", "context"],
            "description": (
                "role: the agent's identity (system/covenant.md + system/character.md).\n"
                "memory: the agent's long-term memory "
                "(system/memory.md, backed by system/memory.json).\n"
                "context: the agent's conversation context window."
            ),
        },
        "action": {
            "type": "string",
            "enum": [
                "update", "diff", "load",
                "submit", "consolidate",
                "compact",
            ],
            "description": (
                "role: update | diff | load.\n"
                "memory: submit | diff | consolidate | load.\n"
                "context: compact."
            ),
        },
        "content": {
            "type": "string",
            "description": (
                "Text content — for role update (character), "
                "memory submit (new entry), or memory consolidate (merged text)."
            ),
        },
        "ids": {
            "type": "array",
            "items": {"type": "string"},
            "description": "Memory entry IDs — for memory consolidate.",
        },
        "prompt": {
            "type": "string",
            "description": (
                "Compaction guidance — what to preserve, what to compress. "
                "Required for context compact. Can be empty."
            ),
        },
    },
    "required": ["object", "action"],
}

DESCRIPTION = (
    "Self-knowledge management — evolving identity, structured memory, "
    "and context control. "
    "role: update your character (your identity, knowledge, experience), "
    "diff to review pending changes, load to apply into live system prompt. "
    "memory: submit new entries, diff to review, consolidate entries by ID "
    "into a single merged entry, load to apply. "
    "Memory IDs are visible in your system prompt. "
    "context: compact to proactively free context space — check usage via "
    "status show first, provide guidance on what to preserve. "
    "After any write (update/submit/consolidate), use diff then load to apply."
)


class AnimaManager:
    """Self-knowledge manager — role, memory, context."""

    def __init__(self, agent: "BaseAgent"):
        self._agent = agent
        self._working_dir = agent._working_dir
        self._original_system: Callable[[dict], dict] | None = None

        # Paths
        system_dir = self._working_dir / "system"
        self._covenant_path = system_dir / "covenant.md"
        self._character_path = system_dir / "character.md"
        self._memory_md = system_dir / "memory.md"
        self._memory_json = system_dir / "memory.json"

        # Ensure character.md exists
        system_dir.mkdir(exist_ok=True)
        if not self._character_path.is_file():
            self._character_path.write_text("")

        # In-memory cache of entries
        self._entries: list[dict] = self._load_entries()

    # ------------------------------------------------------------------
    # Persistence
    # ------------------------------------------------------------------

    def _load_entries(self) -> list[dict]:
        """Load entries from memory.json, or return empty list if missing."""
        if not self._memory_json.is_file():
            return []
        try:
            data = json.loads(self._memory_json.read_text())
            return data.get("entries", [])
        except (json.JSONDecodeError, OSError):
            return []

    def _save_entries(self) -> None:
        """Write entries to memory.json with atomic write."""
        data = {"version": 1, "entries": self._entries}
        self._memory_json.parent.mkdir(exist_ok=True)
        fd, tmp = tempfile.mkstemp(
            dir=str(self._memory_json.parent), suffix=".tmp",
        )
        try:
            os.write(fd, json.dumps(data, indent=2, ensure_ascii=False).encode())
            os.close(fd)
            os.replace(tmp, str(self._memory_json))
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise

    def _render_memory_md(self) -> None:
        """Render memory.json entries to memory.md."""
        lines = []
        for entry in self._entries:
            lines.append(f"- [{entry['id']}] {entry['content']}")
        self._memory_md.parent.mkdir(exist_ok=True)
        self._memory_md.write_text("\n".join(lines) + ("\n" if lines else ""))

    @staticmethod
    def _make_id(content: str, created_at: str) -> str:
        """Generate 8-char hex ID from content + timestamp."""
        return hashlib.sha256(
            (content + created_at).encode()
        ).hexdigest()[:8]

    # ------------------------------------------------------------------
    # Dispatch
    # ------------------------------------------------------------------

    _VALID_ACTIONS: dict[str, set[str]] = {
        "role": {"update", "diff", "load"},
        "memory": {"submit", "diff", "consolidate", "load"},
        "context": {"compact"},
    }

    def handle(self, args: dict) -> dict:
        """Main dispatch — routes by object + action."""
        obj = args.get("object", "")
        action = args.get("action", "")

        valid = self._VALID_ACTIONS.get(obj)
        if valid is None:
            return {
                "error": f"Unknown object: {obj!r}. "
                f"Must be one of: {', '.join(sorted(self._VALID_ACTIONS))}.",
            }
        if action not in valid:
            return {
                "error": f"Invalid action {action!r} for {obj}. "
                f"Valid actions: {', '.join(sorted(valid))}.",
            }

        method = getattr(self, f"_{obj}_{action}")
        return method(args)

    # ------------------------------------------------------------------
    # Role actions
    # ------------------------------------------------------------------

    def _role_update(self, args: dict) -> dict:
        content = args.get("content", "")
        self._character_path.parent.mkdir(exist_ok=True)
        self._character_path.write_text(content)
        return {"status": "ok", "path": str(self._character_path)}

    def _role_diff(self, _args: dict) -> dict:
        return self._agent._system_diff(self._character_path, "character")

    def _role_load(self, _args: dict) -> dict:
        # Read both files and concatenate
        covenant = ""
        if self._covenant_path.is_file():
            covenant = self._covenant_path.read_text()
        character = self._character_path.read_text() if self._character_path.is_file() else ""

        parts = [p for p in [covenant, character] if p.strip()]
        combined = "\n\n".join(parts)

        # Inject as protected section
        if combined.strip():
            self._agent._prompt_manager.write_section(
                "covenant", combined, protected=True,
            )
        else:
            self._agent._prompt_manager.delete_section("covenant")
        self._agent._token_decomp_dirty = True

        # Update live session
        if self._agent._chat is not None:
            self._agent._chat.update_system_prompt(
                self._agent._build_system_prompt()
            )

        # Git commit character.md
        rel_path = "system/character.md"
        git_diff, commit_hash = self._agent._git_diff_and_commit(
            rel_path, "character",
        )

        self._agent._log(
            "anima_role_load",
            changed=commit_hash is not None,
        )

        return {
            "status": "ok",
            "size_bytes": len(combined.encode("utf-8")),
            "content_preview": combined[:200],
            "diff": {
                "changed": commit_hash is not None,
                "git_diff": git_diff or "",
                "commit": commit_hash,
            },
        }

    # ------------------------------------------------------------------
    # Memory actions
    # ------------------------------------------------------------------

    def _memory_submit(self, args: dict) -> dict:
        content = args.get("content", "")
        if not content.strip():
            return {"error": "content is required for memory submit."}
        now = datetime.now(timezone.utc).isoformat()
        entry_id = self._make_id(content, now)
        self._entries.append({
            "id": entry_id,
            "content": content,
            "created_at": now,
        })
        self._save_entries()
        self._render_memory_md()
        return {"status": "ok", "id": entry_id}

    def _memory_diff(self, args: dict) -> dict:
        # Delegate to original system handler
        if self._original_system is None:
            return {"error": "anima not properly initialized (missing system handler)"}
        return self._original_system({"action": "diff", "object": "memory"})

    def _memory_consolidate(self, args: dict) -> dict:
        ids = args.get("ids")
        content = args.get("content", "")
        if not ids:
            return {"error": "ids is required for memory consolidate."}
        if not content.strip():
            return {"error": "content is required for memory consolidate."}

        # Validate IDs
        existing_ids = {e["id"] for e in self._entries}
        invalid = [i for i in ids if i not in existing_ids]
        if invalid:
            return {"error": f"Unknown memory IDs: {', '.join(invalid)}"}

        # Remove old entries
        ids_set = set(ids)
        self._entries = [e for e in self._entries if e["id"] not in ids_set]

        # Add consolidated entry
        now = datetime.now(timezone.utc).isoformat()
        new_id = self._make_id(content, now)
        self._entries.append({
            "id": new_id,
            "content": content,
            "created_at": now,
        })

        self._save_entries()
        self._render_memory_md()
        return {"status": "ok", "id": new_id, "removed": len(ids)}

    def _memory_load(self, args: dict) -> dict:
        # Delegate to original system handler
        if self._original_system is None:
            return {"error": "anima not properly initialized (missing system handler)"}
        return self._original_system({"action": "load", "object": "memory"})

    # ------------------------------------------------------------------
    # Context actions
    # ------------------------------------------------------------------

    def _context_compact(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if prompt is None:
            return {"error": "prompt is required for context compact (can be empty)."}

        if self._agent._chat is None:
            return {"error": "No active chat session to compact."}

        from ..llm.service import COMPACTION_PROMPT

        agent_prompt = self._agent._chat.interface.current_system_prompt or ""
        ctx_window = self._agent._chat.context_window()
        target_tokens = int(ctx_window * 0.2) if ctx_window > 0 else 2048

        def summarizer(text: str) -> str:
            prompt_parts = [COMPACTION_PROMPT]
            if prompt:
                prompt_parts.append(f"\nAgent guidance: {prompt}\n")
            prompt_parts.append(
                f"\nTarget summary length: ~{target_tokens} tokens "
                f"(20% of {ctx_window} token context window).\n"
            )
            if agent_prompt:
                prompt_parts.append(
                    f"\nThe agent's role:\n{agent_prompt}\n\n"
                    "Do your best to help this agent based on its role.\n"
                )
            prompt_parts.append(f"\nConversation history:\n{text}")
            response = self._agent.service.generate(
                "".join(prompt_parts),
                temperature=0.1,
                max_output_tokens=target_tokens,
            )
            return response.text.strip() if response and response.text else ""

        # Force compaction with threshold=0.0
        new_chat = self._agent.service.check_and_compact(
            self._agent._chat,
            summarizer=summarizer,
            threshold=0.0,
            provider=self._agent._config.provider,
        )
        if new_chat is not None:
            before_tokens = self._agent._chat.interface.estimate_context_tokens()
            after_tokens = new_chat.interface.estimate_context_tokens()
            self._agent._chat = new_chat
            self._agent._interaction_id = None
            self._agent._log(
                "anima_compact",
                before_tokens=before_tokens,
                after_tokens=after_tokens,
            )

        usage = self._agent.get_token_usage()
        return {
            "status": "ok",
            "context_tokens": usage.get("ctx_total_tokens", 0),
        }


def setup(agent: "BaseAgent") -> AnimaManager:
    """Set up anima capability — self-knowledge management."""
    mgr = AnimaManager(agent)
    mgr._original_system = agent.override_intrinsic("system")
    agent.add_tool(
        "anima", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )
    return mgr
```

- [ ] **Step 4: Register in capabilities/__init__.py**

Add `"anima": ".anima"` to `_BUILTIN` dict in `src/lingtai/capabilities/__init__.py`.

- [ ] **Step 5: Run setup tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_anima.py::test_anima_setup_removes_system_intrinsic tests/test_anima.py::test_anima_manager_accessible -v`
Expected: PASS

- [ ] **Step 6: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Expected: No errors

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/capabilities/anima.py src/lingtai/capabilities/__init__.py tests/test_anima.py
git commit -m "feat: add anima capability skeleton with setup and schema"
```

---

### Task 3.2: Test role actions

**Files:**
- Modify: `tests/test_anima.py`

- [ ] **Step 1: Write role tests**

Add to `tests/test_anima.py`:

```python
def test_role_update_writes_character(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        role="You are helpful",
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "role", "action": "update", "content": "I am a PDF specialist"})
    assert result["status"] == "ok"
    character = (agent.working_dir / "system" / "character.md").read_text()
    assert character == "I am a PDF specialist"
    agent.stop(timeout=1.0)


def test_role_update_empty_clears_character(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    mgr.handle({"object": "role", "action": "update", "content": "something"})
    mgr.handle({"object": "role", "action": "update", "content": ""})
    character = (agent.working_dir / "system" / "character.md").read_text()
    assert character == ""
    agent.stop(timeout=1.0)


def test_role_diff(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        mgr.handle({"object": "role", "action": "update", "content": "new character"})
        result = mgr.handle({"object": "role", "action": "diff"})
        assert result["status"] == "ok"
        assert "new character" in result["git_diff"]
    finally:
        agent.stop()


def test_role_load_combines_covenant_and_character(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        role="You are helpful",
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        mgr.handle({"object": "role", "action": "update", "content": "I specialize in PDFs"})
        mgr.handle({"object": "role", "action": "load"})
        section = agent._prompt_manager.read_section("covenant")
        assert "You are helpful" in section
        assert "I specialize in PDFs" in section
    finally:
        agent.stop()
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_anima.py -v -k role`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add tests/test_anima.py
git commit -m "test: add role action tests for anima capability"
```

---

### Task 3.3: Test memory actions

**Files:**
- Modify: `tests/test_anima.py`

- [ ] **Step 1: Write memory tests**

Add to `tests/test_anima.py`:

```python
def test_memory_submit(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "memory", "action": "submit",
        "content": "Agent bob knows CDF format",
    })
    assert result["status"] == "ok"
    assert "id" in result
    assert len(result["id"]) == 8
    # Check JSON
    data = json.loads((agent.working_dir / "system" / "memory.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["content"] == "Agent bob knows CDF format"
    # Check rendered markdown
    md = (agent.working_dir / "system" / "memory.md").read_text()
    assert result["id"] in md
    assert "Agent bob knows CDF format" in md
    agent.stop(timeout=1.0)


def test_memory_submit_empty_rejected(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "memory", "action": "submit", "content": ""})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_memory_consolidate(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r1 = mgr.handle({"object": "memory", "action": "submit", "content": "fact A"})
    r2 = mgr.handle({"object": "memory", "action": "submit", "content": "fact B"})

    result = mgr.handle({
        "object": "memory", "action": "consolidate",
        "ids": [r1["id"], r2["id"]],
        "content": "combined fact AB",
    })
    assert result["status"] == "ok"
    assert result["removed"] == 2
    assert "id" in result

    # Only one entry left
    data = json.loads((agent.working_dir / "system" / "memory.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["content"] == "combined fact AB"
    agent.stop(timeout=1.0)


def test_memory_consolidate_invalid_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "memory", "action": "consolidate",
        "ids": ["nonexist"],
        "content": "merged",
    })
    assert "error" in result
    assert "nonexist" in result["error"]
    agent.stop(timeout=1.0)


def test_memory_consolidate_no_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "memory", "action": "consolidate", "content": "x"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_memory_diff_delegates_to_system(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        mgr.handle({"object": "memory", "action": "submit", "content": "test entry"})
        result = mgr.handle({"object": "memory", "action": "diff"})
        assert result["status"] == "ok"
    finally:
        agent.stop()


def test_memory_load_delegates_to_system(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        mgr.handle({"object": "memory", "action": "submit", "content": "test entry"})
        result = mgr.handle({"object": "memory", "action": "load"})
        assert result["status"] == "ok"
        section = agent._prompt_manager.read_section("memory")
        assert "test entry" in section
    finally:
        agent.stop()
```

Add `import json` at top of test file.

- [ ] **Step 2: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_anima.py -v -k memory`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add tests/test_anima.py
git commit -m "test: add memory action tests for anima capability"
```

---

### Task 3.4: Test context compact and error handling

**Files:**
- Modify: `tests/test_anima.py`

- [ ] **Step 1: Write context and error tests**

Add to `tests/test_anima.py`:

```python
def test_context_compact_requires_prompt(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "context", "action": "compact"})
    assert "error" in result
    assert "prompt" in result["error"]
    agent.stop(timeout=1.0)


def test_context_compact_no_session(tmp_path):
    """Compact without active chat should return error."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "context", "action": "compact", "prompt": ""})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_invalid_object(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "bogus", "action": "diff"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_invalid_action_for_object(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "role", "action": "submit"})
    assert "error" in result
    assert "update" in result["error"]  # should list valid actions
    agent.stop(timeout=1.0)


def test_memory_id_deterministic(tmp_path):
    """Same content + timestamp should produce same ID."""
    from lingtai.capabilities.anima import AnimaManager
    id1 = AnimaManager._make_id("hello", "2026-03-16T00:00:00Z")
    id2 = AnimaManager._make_id("hello", "2026-03-16T00:00:00Z")
    assert id1 == id2
    assert len(id1) == 8


def test_memory_id_differs_by_content(tmp_path):
    from lingtai.capabilities.anima import AnimaManager
    id1 = AnimaManager._make_id("hello", "2026-03-16T00:00:00Z")
    id2 = AnimaManager._make_id("world", "2026-03-16T00:00:00Z")
    assert id1 != id2
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_anima.py -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add tests/test_anima.py
git commit -m "test: add context compact and error handling tests for anima"
```

---

### Task 3.5: Full regression test

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 2: Smoke test import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai; from lingtai.capabilities.anima import AnimaManager, SCHEMA; print('OK')" `
Expected: `OK`

- [ ] **Step 3: Final commit if any fixups needed**

```bash
git add -A
git commit -m "fix: test fixups for anima capability"
```

---

## Chunk 4: Update CLAUDE.md

Update project documentation to reflect the new architecture.

### Task 4.1: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update Architecture section**

In `CLAUDE.md`, make these specific changes:

**Intrinsics table** — change system row from "4 intrinsics (mail, clock, status, system)" with view/diff/load on role/ltm to: system intrinsic operates on `memory` object with `diff`/`load` actions only. Covenant is injected at construction (protected, no tool access).

**Capabilities table** — add row:

```
| `anima` | `capabilities=["anima"]` | Upgrades system intrinsic — evolving role (covenant + character), structured memory (submit/consolidate), on-demand context compaction |
```

**Working directory structure** — replace:
```
working_dir/system/role.md → working_dir/system/covenant.md
working_dir/system/ltm.md → working_dir/system/memory.md
```
Add: `working_dir/system/character.md` (anima only) and `working_dir/system/memory.json` (anima only).

**System Prompt Structure** — replace "role and LTM sections" with "covenant and memory sections".

**Key Modules / intrinsics/** — update system.py description: "system intrinsic supports `diff`/`load` actions on `memory` object."

**Built-in Capabilities** — add anima to the count (15 built-in) and the table.

- [ ] **Step 2: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for anima capability and covenant/memory rename"
```
