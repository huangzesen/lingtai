# Minimal Kernel Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Shrink BaseAgent kernel from 9 intrinsics to 4 (mail, clock, status, system), moving file I/O into 5 separate capabilities with a `"file"` group sugar, and removing `enabled_intrinsics`/`disabled_intrinsics` params.

**Architecture:** Two-phase change in one PR. Phase 1 renames `memory` → `system` intrinsic with `view`/`diff`/`load` actions on `role`/`ltm` objects, moving files to `system/`. Phase 2 extracts read/edit/write/glob/grep from intrinsics into capabilities, adds `"file"` sugar to 灵台Agent, and removes the enabled/disabled filtering params.

**Tech Stack:** Python 3.11+, pytest, lingtai framework

---

## File Structure

### Phase 1 — `memory` → `system`

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `src/lingtai/intrinsics/system.py` | Schema + description for `system` intrinsic (view/diff/load × role/ltm) |
| Modify | `src/lingtai/intrinsics/__init__.py` | Replace `memory` entry with `system` in `ALL_INTRINSICS` |
| Modify | `src/lingtai/base_agent.py` | Rename handler, update file paths (`ltm/ltm.md` → `system/ltm.md`, new `system/role.md`), add `view`/`diff`/`load` for both objects, update git init, stop, resume |
| Modify | `src/lingtai/prompt.py` | No change needed (sections are still named `"role"` and `"ltm"` in prompt manager) |
| Delete | `src/lingtai/intrinsics/memory.py` | Replaced by `system.py` |
| Create | `tests/test_system.py` | Tests for the `system` intrinsic (replaces `test_memory.py`) |
| Delete | `tests/test_memory.py` | Replaced by `test_system.py` |

### Phase 2 — File I/O → capabilities

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `src/lingtai/capabilities/read.py` | `setup(agent)` — registers `read` tool backed by agent's FileIOService |
| Create | `src/lingtai/capabilities/write.py` | `setup(agent)` — registers `write` tool backed by agent's FileIOService |
| Create | `src/lingtai/capabilities/edit.py` | `setup(agent)` — registers `edit` tool backed by agent's FileIOService |
| Create | `src/lingtai/capabilities/glob.py` | `setup(agent)` — registers `glob` tool backed by agent's FileIOService |
| Create | `src/lingtai/capabilities/grep.py` | `setup(agent)` — registers `grep` tool backed by agent's FileIOService |
| Modify | `src/lingtai/capabilities/__init__.py` | Add 5 new capabilities + `_GROUPS` sugar dict |
| Modify | `src/lingtai/lingtai_agent.py` | Expand `"file"` group before capability registration |
| Modify | `src/lingtai/base_agent.py` | Remove file intrinsic wiring, `_make_file_service_handler()`, `enabled_intrinsics`/`disabled_intrinsics` params, convenience methods |
| Modify | `src/lingtai/intrinsics/__init__.py` | Remove read/edit/write/glob/grep from `ALL_INTRINSICS` |
| Modify | `tests/test_agent.py` | Remove intrinsic filtering tests, update file I/O tests to use 灵台Agent + `capabilities=["file"]` |
| Create | `tests/test_layers_file.py` | Tests for file capabilities via 灵台Agent |
| Modify | `src/lingtai/__init__.py` | No structural changes needed (FileIOService stays exported) |

---

## Chunk 1: Phase 1 — `memory` → `system` intrinsic

### Task 1: Create `system` intrinsic schema

**Files:**
- Create: `src/lingtai/intrinsics/system.py`
- Test: `tests/test_system.py`

- [ ] **Step 1: Write the failing test for system intrinsic schema**

```python
# tests/test_system.py
"""Tests for system intrinsic — agent identity management (role + ltm)."""
from __future__ import annotations

from lingtai.intrinsics import ALL_INTRINSICS


def test_system_in_all_intrinsics():
    assert "system" in ALL_INTRINSICS
    info = ALL_INTRINSICS["system"]
    assert "schema" in info
    assert "description" in info
    assert info["handler"] is None  # handled by BaseAgent


def test_memory_not_in_all_intrinsics():
    """memory intrinsic should be completely removed."""
    assert "memory" not in ALL_INTRINSICS
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_system.py::test_system_in_all_intrinsics -v`
Expected: FAIL — `system` not in ALL_INTRINSICS

- [ ] **Step 3: Create `system.py` and update `__init__.py`**

Create `src/lingtai/intrinsics/system.py`:

```python
"""System intrinsic — agent identity management (role + ltm).

Actions:
    view   — read the current contents of role.md or ltm.md
    diff   — show uncommitted git diff for role.md or ltm.md
    load   — read the file, inject into live system prompt, git add+commit

Objects:
    role   — system/role.md (the agent's role / persona)
    ltm    — system/ltm.md (the agent's long-term memory)

The handler lives in BaseAgent (needs access to working_dir, prompt_manager, git).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["view", "diff", "load"],
            "description": (
                "view: read the current file contents.\n"
                "diff: show uncommitted git diff (what changed since last commit).\n"
                "load: read the file, inject into the live system prompt, "
                "and git commit. This transforms the agent — changes to role "
                "alter the agent's persona, changes to ltm update its memory."
            ),
        },
        "object": {
            "type": "string",
            "enum": ["role", "ltm"],
            "description": (
                "role: the agent's role/persona (system/role.md).\n"
                "ltm: the agent's long-term memory (system/ltm.md)."
            ),
        },
    },
    "required": ["action", "object"],
}

DESCRIPTION = (
    "Agent identity management. The agent's role lives in system/role.md "
    "and long-term memory in system/ltm.md. "
    "Use 'view' to read current contents, 'diff' to see uncommitted changes, "
    "and 'load' to apply changes into the live system prompt (with git commit). "
    "Loading transforms the agent: role changes alter persona, ltm changes update memory."
)
```

Update `src/lingtai/intrinsics/__init__.py`:

```python
"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description
- handle_* or a Manager class: the implementation

Some intrinsics (mail, clock, status, system) are implemented in BaseAgent
because they need access to agent state (services, etc.).
"""
from . import read, edit, write, glob, grep, mail, clock, status, system

ALL_INTRINSICS = {
    "read": {"schema": read.SCHEMA, "description": read.DESCRIPTION, "handler": read.handle_read},
    "edit": {"schema": edit.SCHEMA, "description": edit.DESCRIPTION, "handler": edit.handle_edit},
    "write": {"schema": write.SCHEMA, "description": write.DESCRIPTION, "handler": write.handle_write},
    "glob": {"schema": glob.SCHEMA, "description": glob.DESCRIPTION, "handler": glob.handle_glob},
    "grep": {"schema": grep.SCHEMA, "description": grep.DESCRIPTION, "handler": grep.handle_grep},
    "mail": {"schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handler": None},
    "clock": {"schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handler": None},
    "status": {"schema": status.SCHEMA, "description": status.DESCRIPTION, "handler": None},
    "system": {"schema": system.SCHEMA, "description": system.DESCRIPTION, "handler": None},
}
```

Delete `src/lingtai/intrinsics/memory.py`.

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_system.py -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/intrinsics/system.py src/lingtai/intrinsics/__init__.py tests/test_system.py
git rm src/lingtai/intrinsics/memory.py
git commit -m "feat: create system intrinsic schema, replace memory"
```

---

### Task 2: Implement `_handle_system` in BaseAgent

**Files:**
- Modify: `src/lingtai/base_agent.py`
- Test: `tests/test_system.py`

- [ ] **Step 1: Write failing tests for system handler**

Append to `tests/test_system.py`:

```python
import subprocess
from unittest.mock import MagicMock

import pytest

from lingtai.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_system_wired_in_agent(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "system" in agent._intrinsics
    assert "memory" not in agent._intrinsics


def test_system_view_ltm_empty(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._handle_system({"action": "view", "object": "ltm"})
        assert result["status"] == "ok"
        assert result["content"] == ""
        assert result["path"].endswith("system/ltm.md")
    finally:
        agent.stop()


def test_system_view_role_empty(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._handle_system({"action": "view", "object": "role"})
        assert result["status"] == "ok"
        assert result["content"] == ""
        assert result["path"].endswith("system/role.md")
    finally:
        agent.stop()


def test_system_view_ltm_with_content(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "system" / "ltm.md"
        ltm_file.write_text("# Memory\n\nimportant fact\n")
        result = agent._handle_system({"action": "view", "object": "ltm"})
        assert result["status"] == "ok"
        assert "important fact" in result["content"]
    finally:
        agent.stop()


def test_system_load_ltm(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "system" / "ltm.md"
        ltm_file.write_text("# Memory\n\nimportant fact\n")
        result = agent._handle_system({"action": "load", "object": "ltm"})
        assert result["status"] == "ok"
        assert result["diff"]["changed"] is True
        section = agent._prompt_manager.read_section("ltm")
        assert "important fact" in section
    finally:
        agent.stop()


def test_system_load_role(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        role_file = agent.working_dir / "system" / "role.md"
        role_file.write_text("You are a researcher")
        result = agent._handle_system({"action": "load", "object": "role"})
        assert result["status"] == "ok"
        section = agent._prompt_manager.read_section("role")
        assert "researcher" in section
    finally:
        agent.stop()


def test_system_load_empty_removes_section(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "system" / "ltm.md"
        ltm_file.write_text("some content")
        agent._handle_system({"action": "load", "object": "ltm"})
        assert agent._prompt_manager.read_section("ltm") is not None

        ltm_file.write_text("")
        agent._handle_system({"action": "load", "object": "ltm"})
        section = agent._prompt_manager.read_section("ltm")
        assert section is None or section.strip() == ""
    finally:
        agent.stop()


def test_system_diff_ltm(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "system" / "ltm.md"
        # First load to commit initial state
        ltm_file.write_text("first version\n")
        agent._handle_system({"action": "load", "object": "ltm"})
        # Edit without loading
        ltm_file.write_text("second version\n")
        result = agent._handle_system({"action": "diff", "object": "ltm"})
        assert result["status"] == "ok"
        assert "first version" in result["git_diff"] or "second version" in result["git_diff"]
    finally:
        agent.stop()


def test_system_diff_no_changes(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._handle_system({"action": "diff", "object": "ltm"})
        assert result["status"] == "ok"
        assert result["git_diff"] == ""
    finally:
        agent.stop()


def test_system_load_no_change_no_commit(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        agent._handle_system({"action": "load", "object": "ltm"})
        result = agent._handle_system({"action": "load", "object": "ltm"})
        assert result["diff"]["changed"] is False
    finally:
        agent.stop()


def test_system_unknown_action(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_system({"action": "bogus", "object": "ltm"})
    assert "error" in result


def test_system_unknown_object(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_system({"action": "view", "object": "bogus"})
    assert "error" in result


# ---------------------------------------------------------------------------
# Lifecycle integration (constructor arg, stop persistence, resume auto-load)
# ---------------------------------------------------------------------------


def test_ltm_constructor_arg_writes_to_system(tmp_path):
    """ltm= constructor arg should write to system/ltm.md."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        ltm="initial memory",
    )
    ltm_file = agent.working_dir / "system" / "ltm.md"
    assert ltm_file.is_file()
    assert ltm_file.read_text() == "initial memory"
    agent.stop(timeout=1.0)


def test_role_constructor_arg_writes_to_system(tmp_path):
    """role= constructor arg should write to system/role.md."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        role="researcher",
    )
    role_file = agent.working_dir / "system" / "role.md"
    assert role_file.is_file()
    assert role_file.read_text() == "researcher"
    agent.stop(timeout=1.0)


def test_existing_system_files_not_overwritten(tmp_path):
    """If system/ltm.md already exists, constructor arg should not overwrite it."""
    working_dir = tmp_path / "test"
    working_dir.mkdir()
    system_dir = working_dir / "system"
    system_dir.mkdir()
    (system_dir / "ltm.md").write_text("existing content")

    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        ltm="constructor ltm",
    )
    assert (agent.working_dir / "system" / "ltm.md").read_text() == "existing content"
    agent.stop(timeout=1.0)


def test_system_creates_files_if_missing(tmp_path):
    """system intrinsic should create missing files."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        import shutil
        system_dir = agent.working_dir / "system"
        if system_dir.exists():
            shutil.rmtree(system_dir)

        result = agent._handle_system({"action": "view", "object": "ltm"})
        assert result["status"] == "ok"
        assert (agent.working_dir / "system" / "ltm.md").is_file()
    finally:
        agent.stop()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_system.py::test_system_wired_in_agent -v`
Expected: FAIL — `_handle_system` not found

- [ ] **Step 3: Implement `_handle_system` in `base_agent.py`**

In `base_agent.py`, make these changes:

1. In `_wire_intrinsics()`, replace `state_intrinsics["memory"] = self._handle_memory` with `state_intrinsics["system"] = self._handle_system`.

2. Replace the entire `_handle_memory` / `_memory_load` / `_git_diff_and_commit_ltm` section with:

```python
# ------------------------------------------------------------------
# System intrinsic
# ------------------------------------------------------------------

def _handle_system(self, args: dict) -> dict:
    """Handle system tool — agent identity management (role + ltm)."""
    action = args.get("action", "view")
    obj = args.get("object", "")
    if obj not in ("role", "ltm"):
        return {"error": f"Unknown object: {obj!r}. Must be 'role' or 'ltm'."}

    system_dir = self._working_dir / "system"
    system_dir.mkdir(exist_ok=True)
    file_path = system_dir / f"{obj}.md"
    if not file_path.is_file():
        file_path.write_text("")

    if action == "view":
        return self._system_view(file_path)
    elif action == "diff":
        return self._system_diff(file_path, obj)
    elif action == "load":
        return self._system_load(file_path, obj)
    else:
        return {"error": f"Unknown action: {action!r}. Must be 'view', 'diff', or 'load'."}

def _system_view(self, file_path: Path) -> dict:
    """Read the current contents of a system file."""
    content = file_path.read_text()
    return {
        "status": "ok",
        "path": str(file_path),
        "content": content,
        "size_bytes": len(content.encode("utf-8")),
    }

def _system_diff(self, file_path: Path, obj: str) -> dict:
    """Show uncommitted git diff for a system file."""
    rel_path = f"system/{obj}.md"
    try:
        result = subprocess.run(
            ["git", "diff", rel_path],
            cwd=self._working_dir,
            capture_output=True, text=True,
        )
        diff_text = result.stdout.strip()
        # Also check for untracked/new file changes
        if not diff_text:
            status_result = subprocess.run(
                ["git", "status", "--porcelain", rel_path],
                cwd=self._working_dir,
                capture_output=True, text=True,
            )
            if status_result.stdout.strip():
                # Untracked or new — show the file content as the "diff"
                diff_text = f"(new/untracked file)\n{file_path.read_text()}"
    except (FileNotFoundError, subprocess.CalledProcessError):
        diff_text = ""

    return {
        "status": "ok",
        "path": str(file_path),
        "git_diff": diff_text,
    }

def _system_load(self, file_path: Path, obj: str) -> dict:
    """Read a system file, inject into system prompt, git commit."""
    content = file_path.read_text()
    size_bytes = len(content.encode("utf-8"))

    # Inject into system prompt (or remove if empty)
    protected = (obj == "role")
    if content.strip():
        self._prompt_manager.write_section(obj, content, protected=protected)
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

def _git_diff_and_commit(self, rel_path: str, label: str) -> tuple[str | None, str | None]:
    """Run git diff on a file, stage, and commit if changed.

    Returns (diff_text, short_commit_hash) or (None, None) if no changes.
    """
    try:
        diff_result = subprocess.run(
            ["git", "diff", rel_path],
            cwd=self._working_dir,
            capture_output=True, text=True,
        )
        diff_cached = subprocess.run(
            ["git", "diff", "--cached", rel_path],
            cwd=self._working_dir,
            capture_output=True, text=True,
        )
        status_result = subprocess.run(
            ["git", "status", "--porcelain", rel_path],
            cwd=self._working_dir,
            capture_output=True, text=True,
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
            cwd=self._working_dir,
            capture_output=True, check=True,
        )

        if not diff_text.strip():
            staged = subprocess.run(
                ["git", "diff", "--cached", rel_path],
                cwd=self._working_dir,
                capture_output=True, text=True,
            )
            diff_text = staged.stdout

        subprocess.run(
            ["git", "commit", "-m", f"system: update {label}"],
            cwd=self._working_dir,
            capture_output=True, check=True,
        )

        hash_result = subprocess.run(
            ["git", "rev-parse", "--short", "HEAD"],
            cwd=self._working_dir,
            capture_output=True, text=True,
        )
        commit_hash = hash_result.stdout.strip()

        return diff_text, commit_hash

    except (FileNotFoundError, subprocess.CalledProcessError):
        return None, None
```

3. Update `_git_init_working_dir()` — replace `ltm/` directory creation with `system/`:

```python
# .gitignore — opt-in tracking
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

# Create system/ directory with role.md and ltm.md
system_dir = self._working_dir / "system"
system_dir.mkdir(exist_ok=True)
role_file = system_dir / "role.md"
if not role_file.is_file():
    role_file.write_text("")
ltm_file = system_dir / "ltm.md"
if not ltm_file.is_file():
    ltm_file.write_text("")

# Initial commit
subprocess.run(
    ["git", "add", ".gitignore", "system/"],
    cwd=self._working_dir,
    capture_output=True, check=True,
)
```

And update the fallback (no-git) block similarly.

4. Update `__init__` — LTM file path from `ltm/ltm.md` → `system/ltm.md`, role path to `system/role.md`:

```python
# LTM and role file paths
system_dir = self._working_dir / "system"
ltm_file = system_dir / "ltm.md"
role_file = system_dir / "role.md"

# If constructor ltm is provided and ltm file doesn't exist, write it
if ltm and not ltm_file.is_file():
    system_dir.mkdir(exist_ok=True)
    ltm_file.write_text(ltm)

# If constructor role is provided, also write to system/role.md
if role and not role_file.is_file():
    system_dir.mkdir(exist_ok=True)
    role_file.write_text(role)

# Auto-load LTM from file into prompt manager
loaded_ltm = ""
if ltm_file.is_file():
    loaded_ltm = ltm_file.read_text()
```

5. Update `stop()` — persist LTM to `system/ltm.md`:

```python
# Persist LTM from prompt manager to file
ltm_content = self._prompt_manager.read_section("ltm") or ""
ltm_file = self._working_dir / "system" / "ltm.md"
if ltm_file.is_file() or ltm_content:
    ltm_file.parent.mkdir(exist_ok=True)
    ltm_file.write_text(ltm_content)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_system.py -v`
Expected: ALL PASS

- [ ] **Step 5: Smoke test import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/base_agent.py tests/test_system.py
git commit -m "feat: implement system intrinsic handler (view/diff/load × role/ltm)"
```

---

### Task 3: Update existing tests that reference `memory` or `ltm/`

**Files:**
- Delete: `tests/test_memory.py`
- Modify: `tests/test_agent.py` (update intrinsic count, path references)
- Modify: `tests/test_git_init.py` (update `ltm/` → `system/` paths)

- [ ] **Step 1: Delete old memory tests**

```bash
git rm tests/test_memory.py
```

- [ ] **Step 2: Update `test_agent.py`**

Changes needed:

1. `test_intrinsics_enabled_by_default`: change `len(agent._intrinsics) == 9` comment to list `system` instead of `memory`. The count stays 9 (for now, until Phase 2).

2. `test_agent_stop_persists_ltm`: update path from `ltm/ltm.md` to `system/ltm.md`:
```python
ltm_file = tmp_path / "alice" / "system" / "ltm.md"
```

3. `test_agent_resume_reads_role_ltm`: no change needed (tests prompt manager sections, not file paths — both `__init__` and `stop()` are updated atomically in Task 2).

4. `test_agent_resume_explicit_overrides_manifest`: no change needed.

- [ ] **Step 3: Update `test_git_init.py`**

Changes needed:

1. `.gitignore` assertions: replace `!ltm/` and `!ltm/**` with `!system/` and `!system/**`.

2. Rename `test_start_creates_ltm_dir` → `test_start_creates_system_dir`. Assert `system/` directory exists with both `role.md` and `ltm.md` (instead of `ltm/` with `ltm.md`).

- [ ] **Step 4: Run full test suite**

Run: `python -m pytest tests/test_agent.py tests/test_system.py tests/test_git_init.py -v`
Expected: ALL PASS

- [ ] **Step 5: Run full test suite for regressions**

Run: `python -m pytest tests/ -v`
Expected: ALL PASS (or only pre-existing failures)

- [ ] **Step 6: Commit**

```bash
git add tests/test_agent.py tests/test_git_init.py
git commit -m "refactor: update tests for memory→system rename, delete test_memory.py"
```

---

## Chunk 2: Phase 2 — File I/O → capabilities

### Task 4: Create 5 file capability modules

**Files:**
- Create: `src/lingtai/capabilities/read.py`
- Create: `src/lingtai/capabilities/write.py`
- Create: `src/lingtai/capabilities/edit.py`
- Create: `src/lingtai/capabilities/glob.py`
- Create: `src/lingtai/capabilities/grep.py`
- Test: `tests/test_layers_file.py`

- [ ] **Step 1: Write failing tests for file capabilities**

```python
# tests/test_layers_file.py
"""Tests for file I/O capabilities (read, write, edit, glob, grep)."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from lingtai.lingtai_agent import 灵台Agent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_file_sugar_expands_to_five(tmp_path):
    """capabilities=["file"] should register all 5 file tools."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    for name in ("read", "write", "edit", "glob", "grep"):
        assert name in agent._mcp_handlers, f"{name} not registered"
    agent.stop(timeout=1.0)


def test_individual_file_capability(tmp_path):
    """Each file capability can be loaded individually."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["read", "write"],
    )
    assert "read" in agent._mcp_handlers
    assert "write" in agent._mcp_handlers
    assert "edit" not in agent._mcp_handlers
    assert "glob" not in agent._mcp_handlers
    assert "grep" not in agent._mcp_handlers
    agent.stop(timeout=1.0)


def test_write_and_read_via_capability(tmp_path):
    """Write and read files through capability handlers."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    # Write
    write_result = agent._mcp_handlers["write"](
        {"file_path": str(agent.working_dir / "test.txt"), "content": "hello world"}
    )
    assert write_result["status"] == "ok"

    # Read
    read_result = agent._mcp_handlers["read"](
        {"file_path": str(agent.working_dir / "test.txt")}
    )
    assert "hello world" in read_result["content"]
    agent.stop(timeout=1.0)


def test_edit_via_capability(tmp_path):
    """Edit files through capability handler."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    (agent.working_dir / "test.txt").write_text("hello world")
    result = agent._mcp_handlers["edit"](
        {"file_path": str(agent.working_dir / "test.txt"), "old_string": "hello", "new_string": "goodbye"}
    )
    assert result["status"] == "ok"
    assert (agent.working_dir / "test.txt").read_text() == "goodbye world"
    agent.stop(timeout=1.0)


def test_glob_via_capability(tmp_path):
    """Glob files through capability handler."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    (agent.working_dir / "a.py").write_text("pass")
    (agent.working_dir / "b.py").write_text("pass")
    (agent.working_dir / "c.txt").write_text("text")
    result = agent._mcp_handlers["glob"](
        {"pattern": "*.py", "path": str(agent.working_dir)}
    )
    assert result["count"] == 2
    agent.stop(timeout=1.0)


def test_grep_via_capability(tmp_path):
    """Grep files through capability handler."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    (agent.working_dir / "test.py").write_text("def hello():\n    pass\n")
    result = agent._mcp_handlers["grep"](
        {"pattern": "def hello", "path": str(agent.working_dir)}
    )
    assert result["count"] >= 1
    agent.stop(timeout=1.0)


def test_base_agent_has_no_file_intrinsics(tmp_path):
    """BaseAgent should NOT have file intrinsics after phase 2."""
    from lingtai.base_agent import BaseAgent
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    for name in ("read", "write", "edit", "glob", "grep"):
        assert name not in agent._intrinsics, f"{name} should not be in BaseAgent intrinsics"
    agent.stop(timeout=1.0)


def test_base_agent_kernel_only(tmp_path):
    """BaseAgent should have exactly 4 intrinsics: mail, clock, status, system."""
    from lingtai.base_agent import BaseAgent
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert set(agent._intrinsics.keys()) == {"mail", "clock", "status", "system"}
    agent.stop(timeout=1.0)


def test_file_capability_uses_file_io_service(tmp_path):
    """File capabilities should use the agent's FileIOService."""
    from lingtai.services.file_io import LocalFileIOService
    svc = LocalFileIOService(root=tmp_path)
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        file_io=svc,
        capabilities=["file"],
    )
    result = agent._mcp_handlers["write"](
        {"file_path": str(tmp_path / "test.txt"), "content": "via service"}
    )
    assert result["status"] == "ok"
    assert (tmp_path / "test.txt").read_text() == "via service"
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_layers_file.py::test_file_sugar_expands_to_five -v`
Expected: FAIL — `read` not a known capability

- [ ] **Step 3: Create the 5 capability modules**

Each capability follows the same pattern — `setup(agent)` creates a handler that delegates to the agent's `_file_io` service, then calls `agent.add_tool()`.

Create `src/lingtai/capabilities/read.py`:

```python
"""Read capability — read text file contents.

Usage: 灵台Agent(capabilities=["read"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "file_path": {"type": "string", "description": "Absolute path to the file to read"},
        "offset": {"type": "integer", "description": "Line number to start from (1-based)", "default": 1},
        "limit": {"type": "integer", "description": "Max lines to read", "default": 2000},
    },
    "required": ["file_path"],
}

DESCRIPTION = (
    "Read the contents of a text file. Returns numbered lines. "
    "Text files only — cannot read binary, images, or audio. "
    "Use offset/limit to read specific sections of large files."
)


def setup(agent: "BaseAgent") -> None:
    """Set up the read capability on an agent."""

    def handle_read(args: dict) -> dict:
        path = args.get("file_path", "")
        if not path:
            return {"error": "file_path is required"}
        if not Path(path).is_absolute():
            path = str(agent._working_dir / path)
        offset = args.get("offset", 1)
        limit = args.get("limit", 2000)
        try:
            content = agent._file_io.read(path)
        except FileNotFoundError:
            return {"error": f"File not found: {path}"}
        except Exception as e:
            return {"error": f"Cannot read {path}: {e}"}
        lines = content.splitlines(keepends=True)
        start = max(0, offset - 1)
        selected = lines[start:start + limit]
        numbered = "".join(f"{start + i + 1}\t{line}" for i, line in enumerate(selected))
        return {"content": numbered, "total_lines": len(lines), "lines_shown": len(selected)}

    agent.add_tool("read", schema=SCHEMA, handler=handle_read, description=DESCRIPTION)
```

Create `src/lingtai/capabilities/write.py`:

```python
"""Write capability — create or overwrite a file.

Usage: 灵台Agent(capabilities=["write"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "file_path": {"type": "string", "description": "Absolute path to the file to write"},
        "content": {"type": "string", "description": "Content to write"},
    },
    "required": ["file_path", "content"],
}

DESCRIPTION = (
    "Create or overwrite a file with the given content. "
    "Parent directories are created automatically. "
    "Use this for creating new files or complete rewrites. "
    "For small changes to existing files, prefer edit."
)


def setup(agent: "BaseAgent") -> None:
    """Set up the write capability on an agent."""

    def handle_write(args: dict) -> dict:
        path = args.get("file_path", "")
        content = args.get("content", "")
        if not path:
            return {"error": "file_path is required"}
        if not Path(path).is_absolute():
            path = str(agent._working_dir / path)
        try:
            agent._file_io.write(path, content)
            return {"status": "ok", "path": path, "bytes": len(content)}
        except Exception as e:
            return {"error": f"Cannot write {path}: {e}"}

    agent.add_tool("write", schema=SCHEMA, handler=handle_write, description=DESCRIPTION)
```

Create `src/lingtai/capabilities/edit.py`:

```python
"""Edit capability — exact string replacement in a file.

Usage: 灵台Agent(capabilities=["edit"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "file_path": {"type": "string", "description": "Absolute path to the file to edit"},
        "old_string": {"type": "string", "description": "The exact text to find and replace"},
        "new_string": {"type": "string", "description": "The replacement text"},
        "replace_all": {"type": "boolean", "description": "Replace all occurrences", "default": False},
    },
    "required": ["file_path", "old_string", "new_string"],
}

DESCRIPTION = "Replace an exact string in a file. Fails if old_string is not found or is ambiguous."


def setup(agent: "BaseAgent") -> None:
    """Set up the edit capability on an agent."""

    def handle_edit(args: dict) -> dict:
        path = args.get("file_path", "")
        if not path:
            return {"error": "file_path is required"}
        if not Path(path).is_absolute():
            path = str(agent._working_dir / path)
        old = args.get("old_string", "")
        new = args.get("new_string", "")
        replace_all = args.get("replace_all", False)
        try:
            content = agent._file_io.read(path)
        except FileNotFoundError:
            return {"error": f"File not found: {path}"}
        except Exception as e:
            return {"error": f"Cannot read {path}: {e}"}
        count = content.count(old)
        if count == 0:
            return {"error": f"old_string not found in {path}"}
        if count > 1 and not replace_all:
            return {"error": f"old_string found {count} times — use replace_all=true or provide more context"}
        if replace_all:
            updated = content.replace(old, new)
        else:
            updated = content.replace(old, new, 1)
        try:
            agent._file_io.write(path, updated)
        except Exception as e:
            return {"error": f"Cannot write {path}: {e}"}
        return {"status": "ok", "replacements": count if replace_all else 1}

    agent.add_tool("edit", schema=SCHEMA, handler=handle_edit, description=DESCRIPTION)
```

Create `src/lingtai/capabilities/glob.py`:

```python
"""Glob capability — find files by pattern.

Usage: 灵台Agent(capabilities=["glob"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "pattern": {"type": "string", "description": "Glob pattern (e.g., '**/*.py')"},
        "path": {"type": "string", "description": "Directory to search in"},
    },
    "required": ["pattern"],
}

DESCRIPTION = (
    "Find files matching a glob pattern. "
    "Use '**/' for recursive search (e.g. '**/*.py' finds all Python files). "
    "Returns sorted list of matching file paths."
)


def setup(agent: "BaseAgent") -> None:
    """Set up the glob capability on an agent."""

    def handle_glob(args: dict) -> dict:
        pattern = args.get("pattern", "")
        if not pattern:
            return {"error": "pattern is required"}
        search_dir = args.get("path", str(agent._working_dir))
        if not Path(search_dir).is_absolute():
            search_dir = str(agent._working_dir / search_dir)
        try:
            matches = agent._file_io.glob(pattern, root=search_dir)
            return {"matches": matches, "count": len(matches)}
        except Exception as e:
            return {"error": f"Glob failed: {e}"}

    agent.add_tool("glob", schema=SCHEMA, handler=handle_glob, description=DESCRIPTION)
```

Create `src/lingtai/capabilities/grep.py`:

```python
"""Grep capability — search file contents by regex.

Usage: 灵台Agent(capabilities=["grep"]) or capabilities=["file"]
"""
from __future__ import annotations

from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "pattern": {"type": "string", "description": "Regex pattern to search for"},
        "path": {"type": "string", "description": "File or directory to search in"},
        "glob": {"type": "string", "description": "File glob filter (e.g., '*.py')", "default": "*"},
        "max_matches": {"type": "integer", "description": "Maximum matches to return", "default": 200},
    },
    "required": ["pattern"],
}

DESCRIPTION = (
    "Search file contents for lines matching a regex pattern. "
    "Returns matching lines with file path and line number. "
    "Searches recursively when given a directory. "
    "Use the glob filter to narrow to specific file types."
)


def setup(agent: "BaseAgent") -> None:
    """Set up the grep capability on an agent."""

    def handle_grep(args: dict) -> dict:
        pattern = args.get("pattern", "")
        if not pattern:
            return {"error": "pattern is required"}
        search_path = args.get("path", str(agent._working_dir))
        if not Path(search_path).is_absolute():
            search_path = str(agent._working_dir / search_path)
        max_matches = args.get("max_matches", 200)
        try:
            results = agent._file_io.grep(pattern, path=search_path, max_results=max_matches)
            matches = [{"file": r.path, "line": r.line_number, "text": r.line} for r in results]
            return {"matches": matches, "count": len(matches), "truncated": len(matches) >= max_matches}
        except Exception as e:
            return {"error": f"Grep failed: {e}"}

    agent.add_tool("grep", schema=SCHEMA, handler=handle_grep, description=DESCRIPTION)
```

- [ ] **Step 4: Run tests to verify they still fail (capabilities not registered yet)**

Run: `python -m pytest tests/test_layers_file.py::test_file_sugar_expands_to_five -v`
Expected: FAIL — `read` not a known capability

- [ ] **Step 5: Commit capability modules**

```bash
git add src/lingtai/capabilities/read.py src/lingtai/capabilities/write.py src/lingtai/capabilities/edit.py src/lingtai/capabilities/glob.py src/lingtai/capabilities/grep.py tests/test_layers_file.py
git commit -m "feat: create 5 file I/O capability modules (read, write, edit, glob, grep)"
```

---

### Task 5: Register file capabilities + `"file"` sugar

**Files:**
- Modify: `src/lingtai/capabilities/__init__.py`
- Modify: `src/lingtai/lingtai_agent.py`

- [ ] **Step 1: Update capability registry with 5 new entries + groups**

Update `src/lingtai/capabilities/__init__.py`:

```python
"""Composable agent capabilities — add via 灵台Agent(capabilities=[...])."""
from __future__ import annotations

import importlib
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

# Registry of built-in capability names → module paths (relative to this package).
_BUILTIN: dict[str, str] = {
    "bash": ".bash",
    "delegate": ".delegate",
    "email": ".email",
    "draw": ".draw",
    "compose": ".compose",
    "talk": ".talk",
    "listen": ".listen",
    "vision": ".vision",
    "web_search": ".web_search",
    "read": ".read",
    "write": ".write",
    "edit": ".edit",
    "glob": ".glob",
    "grep": ".grep",
}

# Group names that expand to multiple capabilities.
_GROUPS: dict[str, list[str]] = {
    "file": ["read", "write", "edit", "glob", "grep"],
}


def expand_groups(names: list[str]) -> list[str]:
    """Expand group names (e.g. 'file') into individual capability names."""
    result = []
    for name in names:
        if name in _GROUPS:
            result.extend(_GROUPS[name])
        else:
            result.append(name)
    return result


def setup_capability(agent: "BaseAgent", name: str, **kwargs: Any) -> Any:
    """Look up a capability by *name* and call its ``setup(agent, **kwargs)``.

    Returns whatever the capability's ``setup`` function returns (typically
    a manager instance).

    Raises ``ValueError`` if the name is unknown or the module lacks ``setup``.
    """
    module_path = _BUILTIN.get(name)
    if module_path is None:
        raise ValueError(
            f"Unknown capability: {name!r}. "
            f"Available: {', '.join(sorted(_BUILTIN))}. "
            f"Groups: {', '.join(sorted(_GROUPS))}"
        )
    mod = importlib.import_module(module_path, package=__package__)
    setup_fn = getattr(mod, "setup", None)
    if setup_fn is None:
        raise ValueError(
            f"Capability module {name!r} does not export a setup() function"
        )
    return setup_fn(agent, **kwargs)
```

- [ ] **Step 2: Update 灵台Agent to expand groups**

Update `src/lingtai/lingtai_agent.py`:

```python
"""灵台Agent — BaseAgent + composable capabilities + domain tools.

Layer 2 of the three-layer hierarchy:
    BaseAgent (kernel) → 灵台Agent (capabilities) → CustomAgent (domain)

Capabilities and tools are declared at construction and sealed before start().
"""
from __future__ import annotations

from typing import Any

from .base_agent import BaseAgent
from .types import MCPTool


class 灵台Agent(BaseAgent):
    """BaseAgent with composable capabilities and domain tools.

    Args:
        capabilities: Capability names to enable. Either a list of strings
            (no kwargs) or a dict mapping names to kwargs dicts.
            Example: ``["file", "bash"]`` or ``{"bash": {"policy_file": "p.json"}}``.
            Group names (e.g. ``"file"``) expand to individual capabilities.
        tools: Domain tools (MCP tools) to register. Each tool gets an ``[MCP]``
            prefix in its LLM-visible description.
        *args, **kwargs: Passed through to BaseAgent.
    """

    def __init__(
        self,
        *args: Any,
        capabilities: list[str] | dict[str, dict] | None = None,
        tools: list[MCPTool] | None = None,
        **kwargs: Any,
    ):
        super().__init__(*args, **kwargs)

        # Expand groups and normalize to dict
        if isinstance(capabilities, list):
            from .capabilities import expand_groups
            expanded = expand_groups(capabilities)
            capabilities = {name: {} for name in expanded}
        elif isinstance(capabilities, dict):
            from .capabilities import _GROUPS
            expanded_dict: dict[str, dict] = {}
            for name, cap_kwargs in capabilities.items():
                if name in _GROUPS:
                    # Groups expand without kwargs — sub-caps define their own signatures
                    for sub in _GROUPS[name]:
                        expanded_dict[sub] = {}
                else:
                    expanded_dict[name] = cap_kwargs
            capabilities = expanded_dict

        # Track for delegate replay
        self._capabilities: list[tuple[str, dict]] = []
        self._capability_managers: dict[str, Any] = {}

        # Register capabilities
        if capabilities:
            for name, cap_kwargs in capabilities.items():
                self._setup_capability(name, **cap_kwargs)

        # Register domain tools
        if tools:
            for tool in tools:
                self.add_tool(
                    tool.name,
                    schema=tool.schema,
                    handler=tool.handler,
                    description=tool.description,
                )
                self._mcp_tool_names.add(tool.name)

    def _setup_capability(self, name: str, **kwargs: Any) -> Any:
        """Load a named capability.

        Not directly sealed — but setup() calls add_tool() which checks the seal.
        Must only be called from __init__ (before start()).
        """
        from .capabilities import setup_capability

        self._capabilities.append((name, dict(kwargs)))
        mgr = setup_capability(self, name, **kwargs)
        self._capability_managers[name] = mgr
        return mgr

    def get_capability(self, name: str) -> Any:
        """Return the manager instance for a registered capability, or None."""
        return self._capability_managers.get(name)
```

- [ ] **Step 3: Run file capability tests**

Run: `python -m pytest tests/test_layers_file.py -v`
Expected: ALL PASS (except base agent tests which need Phase 2 removal)

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/capabilities/__init__.py src/lingtai/lingtai_agent.py
git commit -m "feat: register file capabilities, add 'file' group sugar"
```

---

### Task 6: Remove file I/O from BaseAgent kernel + remove enabled/disabled params

**Files:**
- Modify: `src/lingtai/base_agent.py`
- Modify: `src/lingtai/intrinsics/__init__.py`
- Modify: `tests/test_agent.py`
- Modify: `tests/test_status.py` (remove `disabled_intrinsics` test)
- Modify: `tests/test_clock.py` (remove `disabled_intrinsics` test)
- Delete: `tests/test_intrinsics_file.py`

- [ ] **Step 1: Strip file intrinsics from `ALL_INTRINSICS`**

Update `src/lingtai/intrinsics/__init__.py`:

```python
"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description

All kernel intrinsics are implemented in BaseAgent because they need
access to agent state (services, working_dir, etc.).
"""
from . import mail, clock, status, system

ALL_INTRINSICS = {
    "mail": {"schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handler": None},
    "clock": {"schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handler": None},
    "status": {"schema": status.SCHEMA, "description": status.DESCRIPTION, "handler": None},
    "system": {"schema": system.SCHEMA, "description": system.DESCRIPTION, "handler": None},
}
```

Note: The intrinsic schema files `read.py`, `edit.py`, `write.py`, `glob.py`, `grep.py` stay in `intrinsics/` for now — they're still importable. The capabilities reference their own copies of the schemas. We can delete these later in a cleanup pass, but they do no harm.

- [ ] **Step 2: Strip file I/O from `base_agent.py`**

Remove from `__init__` signature:
- `enabled_intrinsics: set[str] | None = None,`
- `disabled_intrinsics: set[str] | None = None,`
- The validation block: `if enabled_intrinsics is not None and disabled_intrinsics is not None: raise ValueError(...)`

Remove from `__init__` body:
- The call: `self._wire_intrinsics(enabled_intrinsics, disabled_intrinsics)` → replace with `self._wire_intrinsics()`

Simplify `_wire_intrinsics()`:
```python
def _wire_intrinsics(self) -> None:
    """Wire kernel intrinsic tool handlers."""
    self._intrinsics["mail"] = self._handle_mail
    self._intrinsics["clock"] = self._handle_clock
    self._intrinsics["status"] = self._handle_status
    self._intrinsics["system"] = self._handle_system
```

Remove entirely:
- `_make_file_service_handler()` method (the big method with read/edit/write/glob/grep branches)

Remove the convenience methods at the bottom:
- `read_file()`, `write_file()`, `edit_file()`, `glob()`, `grep()` — these were wrappers over intrinsic handlers that no longer exist on BaseAgent

Update docstring: remove mention of `file_io` backing read/edit/write/glob/grep.

Note: Keep `self._file_io` and the FileIOService auto-creation — capabilities still need access to it via `agent._file_io`.

- [ ] **Step 3: Update `tests/test_agent.py`**

Remove these tests entirely from `test_agent.py`:
- `test_disabled_intrinsics`
- `test_enabled_intrinsics`
- `test_enabled_and_disabled_raises`
- `test_file_intrinsics_use_service`
- `test_file_intrinsics_auto_create_service`
- `test_no_file_io_disables_file_intrinsics`

Update `test_intrinsics_enabled_by_default`:
```python
def test_intrinsics_enabled_by_default(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "mail" in agent._intrinsics
    assert "clock" in agent._intrinsics
    assert "status" in agent._intrinsics
    assert "system" in agent._intrinsics
    # File I/O is now a capability, not intrinsic
    assert "read" not in agent._intrinsics
    assert "write" not in agent._intrinsics
    assert len(agent._intrinsics) == 4  # mail, clock, status, system
```

Update `test_execute_single_tool_intrinsic` — it manually injects `"read"` into `_intrinsics`, but `read` is no longer a kernel intrinsic. Change it to test with a real kernel intrinsic (e.g. mock the `clock` handler):
```python
def test_execute_single_tool_intrinsic(tmp_path):
    """Intrinsic tools should be callable via _dispatch_tool."""
    from lingtai.llm.base import ToolCall
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    # Replace the clock intrinsic with a mock
    agent._intrinsics["clock"] = lambda args: {"status": "ok", "time": "12:00"}

    tc = ToolCall(name="clock", args={"action": "check"})
    result = agent._dispatch_tool(tc)
    assert result["status"] == "ok"
```

Remove `test_status_can_be_disabled` from `tests/test_status.py` and `test_clock_can_be_disabled` from `tests/test_clock.py` — the concept of disabling kernel intrinsics no longer applies.

Delete `tests/test_intrinsics_file.py` (these tests tested the intrinsic handlers — now covered by `test_layers_file.py`).

Delete the old intrinsic schema files that are now dead code:
```bash
git rm src/lingtai/intrinsics/read.py src/lingtai/intrinsics/edit.py src/lingtai/intrinsics/write.py src/lingtai/intrinsics/glob.py src/lingtai/intrinsics/grep.py
```

- [ ] **Step 4: Run all tests**

Run: `python -m pytest tests/test_agent.py tests/test_layers_file.py tests/test_system.py -v`
Expected: ALL PASS

- [ ] **Step 5: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Expected: No errors

- [ ] **Step 6: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: ALL PASS (or only pre-existing failures unrelated to this change)

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/base_agent.py src/lingtai/intrinsics/__init__.py tests/test_agent.py
git rm tests/test_intrinsics_file.py
git commit -m "refactor: remove file I/O from kernel, remove enabled/disabled intrinsics params"
```

---

### Task 7: Update CLAUDE.md and remaining references

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update CLAUDE.md**

Key changes:
- BaseAgent now has 4 intrinsics (mail, clock, status, system), not 9
- `memory` intrinsic replaced by `system` (view/diff/load × role/ltm)
- File I/O (read, edit, write, glob, grep) are now 5 individual capabilities with `"file"` group sugar
- `enabled_intrinsics`/`disabled_intrinsics` params removed
- File paths: `ltm/ltm.md` → `system/ltm.md`, new `system/role.md`
- Capabilities count: 14 (was 9) — 5 file + 9 original
- Three-tier tool table updated

- [ ] **Step 2: Run smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for minimal kernel (4 intrinsics, file capabilities)"
```
