# Status & Memory Intrinsics Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `status` (self-introspection) and `memory` (LTM reload) intrinsics, with git-controlled agent working directories.

**Architecture:** Git init happens in `start()`. LTM moves from manifest string to `ltm/ltm.md` file. `status` returns identity/runtime/tokens. `memory load` reads `ltm/ltm.md` into system prompt and git-commits changes. Both are state intrinsics with `handler=None`.

**Tech Stack:** Python stdlib (`subprocess`, `datetime`, `time`, `threading`), git CLI

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai/intrinsics/status.py` | Create | Schema and description for status intrinsic |
| `src/lingtai/intrinsics/memory.py` | Create | Schema and description for memory intrinsic |
| `src/lingtai/intrinsics/__init__.py` | Modify | Register both with `handler=None` |
| `src/lingtai/agent.py` | Modify | Git init in `start()`, `_started_at`/`_uptime_anchor`, `_handle_status`, `_handle_memory`, wire intrinsics, LTM migration, auto-load, manifest changes |
| `tests/test_status.py` | Create | Status intrinsic tests |
| `tests/test_memory.py` | Create | Memory intrinsic tests |
| `tests/test_agent.py` | Modify | Intrinsic count 9 → 11, LTM migration tests |

---

## Chunk 1: Git-Controlled Working Directory + Status Intrinsic

### Task 1: Git init in `start()`

**Files:**
- Modify: `src/lingtai/agent.py`

- [ ] **Step 1: Write the failing test**

Create `tests/test_git_init.py`:

```python
"""Tests for git-controlled agent working directory."""
from __future__ import annotations

import subprocess
from unittest.mock import MagicMock

import pytest

from lingtai.agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_start_creates_git_repo(tmp_path):
    """agent.start() should git init the working directory."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        git_dir = agent.working_dir / ".git"
        assert git_dir.is_dir(), "Working dir should have .git after start()"
    finally:
        agent.stop()


def test_start_creates_gitignore(tmp_path):
    """agent.start() should create opt-in .gitignore."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        gitignore = agent.working_dir / ".gitignore"
        assert gitignore.is_file()
        content = gitignore.read_text()
        assert "*" in content  # track nothing by default
        assert "!.gitignore" in content
        assert "!ltm/" in content
        assert "!ltm/**" in content
    finally:
        agent.stop()


def test_start_creates_ltm_dir(tmp_path):
    """agent.start() should create ltm/ directory and ltm.md."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_dir = agent.working_dir / "ltm"
        assert ltm_dir.is_dir()
        ltm_file = ltm_dir / "ltm.md"
        assert ltm_file.is_file()
    finally:
        agent.stop()


def test_start_makes_initial_commit(tmp_path):
    """agent.start() should make an initial git commit."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = subprocess.run(
            ["git", "log", "--oneline", "-1"],
            cwd=agent.working_dir,
            capture_output=True, text=True,
        )
        assert result.returncode == 0
        assert "init" in result.stdout.lower()
    finally:
        agent.stop()


def test_start_skips_git_init_on_resume(tmp_path):
    """If .git exists (resume), start() should not reinitialize."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    agent.stop()
    # Get initial commit count
    result = subprocess.run(
        ["git", "rev-list", "--count", "HEAD"],
        cwd=agent.working_dir,
        capture_output=True, text=True,
    )
    initial_commits = int(result.stdout.strip())

    # Start again (resume)
    agent2 = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent2.start()
    try:
        result2 = subprocess.run(
            ["git", "rev-list", "--count", "HEAD"],
            cwd=agent2.working_dir,
            capture_output=True, text=True,
        )
        resume_commits = int(result2.stdout.strip())
        assert resume_commits == initial_commits, "Resume should not create new commits"
    finally:
        agent2.stop()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_git_init.py -v`
Expected: FAIL — no git init logic exists yet

- [ ] **Step 3: Implement git init in `start()`**

In `src/lingtai/agent.py`, add a helper method and call it from `start()`.

Add this method after `_release_lock()` (around line 827):

```python
    def _git_init_working_dir(self) -> None:
        """Initialize working directory as a git repo with opt-in tracking.

        Creates .gitignore (track nothing by default, whitelist ltm/),
        ltm/ directory, and makes an initial commit. Skips if .git exists.
        """
        git_dir = self._working_dir / ".git"
        if git_dir.is_dir():
            return  # Already initialized (resume)

        try:
            # git init
            subprocess.run(
                ["git", "init"],
                cwd=self._working_dir,
                capture_output=True, check=True,
            )

            # Configure git identity for this repo
            subprocess.run(
                ["git", "config", "user.email", "agent@lingtai"],
                cwd=self._working_dir,
                capture_output=True, check=True,
            )
            subprocess.run(
                ["git", "config", "user.name", "灵台 Agent"],
                cwd=self._working_dir,
                capture_output=True, check=True,
            )

            # .gitignore — opt-in tracking
            gitignore = self._working_dir / ".gitignore"
            gitignore.write_text(
                "# Track nothing by default\n"
                "*\n"
                "# Except these\n"
                "!.gitignore\n"
                "!ltm/\n"
                "!ltm/**\n"
            )

            # Create ltm/ directory and ltm.md
            ltm_dir = self._working_dir / "ltm"
            ltm_dir.mkdir(exist_ok=True)
            ltm_file = ltm_dir / "ltm.md"
            if not ltm_file.is_file():
                ltm_file.write_text("")

            # Initial commit
            subprocess.run(
                ["git", "add", ".gitignore", "ltm/"],
                cwd=self._working_dir,
                capture_output=True, check=True,
            )
            subprocess.run(
                ["git", "commit", "-m", "init: agent working directory"],
                cwd=self._working_dir,
                capture_output=True, check=True,
            )
        except (FileNotFoundError, subprocess.CalledProcessError):
            # Git not available — degrade gracefully. Agent still works,
            # just without git tracking for ltm.
            # Still create ltm/ directory and file
            ltm_dir = self._working_dir / "ltm"
            ltm_dir.mkdir(exist_ok=True)
            ltm_file = ltm_dir / "ltm.md"
            if not ltm_file.is_file():
                ltm_file.write_text("")
```

Add `import subprocess` near the top of agent.py, among the stdlib imports (after `import re` on line 20).

In `start()` (line 752), call `_git_init_working_dir()` at the beginning, after the early return and `_shutdown.clear()`:

```python
        # Initialize git repo in working directory (first start only)
        self._git_init_working_dir()

        # Capture startup time for uptime tracking
        from datetime import datetime, timezone
        self._started_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        self._uptime_anchor = time.monotonic()
```

Also add initial values in `__init__` after line 231 (near `self._cancel_event`):

```python
        self._started_at: str = ""
        self._uptime_anchor: float | None = None  # set in start(), None means not started
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_git_init.py -v`
Expected: ALL PASS

- [ ] **Step 5: Smoke-test**

Run: `python -c "import lingtai"`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/agent.py tests/test_git_init.py
git commit -m "feat: git init agent working directory on start"
```

### Task 2: Status schema module + registration

**Files:**
- Create: `src/lingtai/intrinsics/status.py`
- Modify: `src/lingtai/intrinsics/__init__.py`

- [ ] **Step 1: Create status schema module**

Create `src/lingtai/intrinsics/status.py`:

```python
"""Status intrinsic — agent self-inspection.

Actions:
    show — display agent identity, runtime, and resource usage

The handler lives in BaseAgent (needs access to agent state).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["show"],
            "description": (
                "show: display full agent self-inspection. Returns:\n"
                "- identity: agent_id, working_dir, mail_address (or null if no mail service)\n"
                "- runtime: started_at (UTC ISO), uptime_seconds\n"
                "- tokens.input_tokens, output_tokens, thinking_tokens, cached_tokens, "
                "total_tokens, api_calls: cumulative LLM usage since start\n"
                "- tokens.context.system_tokens, tools_tokens, history_tokens: "
                "current context window breakdown\n"
                "- tokens.context.window_size: total context window capacity\n"
                "- tokens.context.usage_pct: percentage of context window currently occupied\n"
                "Use this to monitor resource consumption, decide when to save "
                "important information to long-term memory, and identify yourself."
            ),
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Agent self-inspection. 'show' returns identity (agent_id, working_dir, "
    "mail address), runtime (uptime), and resource usage (cumulative tokens, "
    "context window breakdown with usage percentage). "
    "Check this to monitor your own resource consumption and decide when to "
    "save important information to long-term memory before context compaction."
)
```

- [ ] **Step 2: Write failing test for registration**

Create `tests/test_status.py`:

```python
"""Tests for status intrinsic — agent self-inspection."""
from __future__ import annotations

import time
from unittest.mock import MagicMock

import pytest

from lingtai.agent import BaseAgent
from lingtai.intrinsics import ALL_INTRINSICS


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Registration
# ---------------------------------------------------------------------------


def test_status_in_all_intrinsics():
    """Status should be registered in ALL_INTRINSICS with handler=None."""
    assert "status" in ALL_INTRINSICS
    info = ALL_INTRINSICS["status"]
    assert "schema" in info
    assert "description" in info
    assert info["handler"] is None
```

- [ ] **Step 3: Run test to verify it fails**

Run: `python -m pytest tests/test_status.py::test_status_in_all_intrinsics -v`
Expected: FAIL

- [ ] **Step 4: Register status in `__init__.py`**

In `src/lingtai/intrinsics/__init__.py`, add `status` to the import and registry:

```python
from . import read, edit, write, glob, grep, mail, vision, web_search, clock, status

ALL_INTRINSICS = {
    # ... existing entries ...
    "status": {"schema": status.SCHEMA, "description": status.DESCRIPTION, "handler": None},
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `python -m pytest tests/test_status.py::test_status_in_all_intrinsics -v`
Expected: PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/intrinsics/status.py src/lingtai/intrinsics/__init__.py tests/test_status.py
git commit -m "feat: add status intrinsic schema and register"
```

### Task 3: Wire status + implement `_handle_status`

**Files:**
- Modify: `src/lingtai/agent.py`

- [ ] **Step 1: Write failing tests**

Append to `tests/test_status.py`:

```python
# ---------------------------------------------------------------------------
# Wiring
# ---------------------------------------------------------------------------


def test_status_wired_in_agent(tmp_path):
    """Status should be wired as an intrinsic in BaseAgent."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "status" in agent._intrinsics


def test_status_can_be_disabled(tmp_path):
    """Status should be disable-able like other intrinsics."""
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        disabled_intrinsics={"status"},
        base_dir=tmp_path,
    )
    assert "status" not in agent._intrinsics


# ---------------------------------------------------------------------------
# show action
# ---------------------------------------------------------------------------


def test_status_show_returns_identity(tmp_path):
    """status show should return agent identity."""
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._handle_status({"action": "show"})
        assert result["status"] == "ok"
        identity = result["identity"]
        assert identity["agent_id"] == "alice"
        assert "alice" in identity["working_dir"]
        assert identity["mail_address"] is None  # no mail service
    finally:
        agent.stop()


def test_status_show_returns_runtime(tmp_path):
    """status show should return started_at and uptime."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        time.sleep(0.1)
        result = agent._handle_status({"action": "show"})
        runtime = result["runtime"]
        assert "T" in runtime["started_at"]  # ISO format
        assert runtime["uptime_seconds"] >= 0.05  # at least some uptime
    finally:
        agent.stop()


def test_status_show_returns_tokens(tmp_path):
    """status show should return token usage."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._handle_status({"action": "show"})
        tokens = result["tokens"]
        assert "input_tokens" in tokens
        assert "output_tokens" in tokens
        assert "total_tokens" in tokens
        assert "api_calls" in tokens
        assert "context" in tokens
        ctx = tokens["context"]
        assert "window_size" in ctx
        assert "usage_pct" in ctx
    finally:
        agent.stop()


def test_status_show_with_mail_service(tmp_path):
    """status show should include mail address when mail service is configured."""
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8301"
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        mail_service=mock_mail,
        base_dir=tmp_path,
    )
    agent.start()
    try:
        result = agent._handle_status({"action": "show"})
        assert result["identity"]["mail_address"] == "127.0.0.1:8301"
    finally:
        agent.stop()


def test_status_show_context_null_without_session(tmp_path):
    """Context fields should be null when no chat session exists."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    # Don't start — no chat session
    result = agent._handle_status({"action": "show"})
    ctx = result["tokens"]["context"]
    assert ctx["window_size"] is None
    assert ctx["usage_pct"] is None


def test_status_unknown_action(tmp_path):
    """Unknown status action should return error."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_status({"action": "bogus"})
    assert "error" in result
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_status.py -v`
Expected: FAIL — `_handle_status` not defined

- [ ] **Step 3: Wire status in `_wire_intrinsics`**

In `agent.py`, in `_wire_intrinsics()`, after `state_intrinsics["clock"] = self._handle_clock` (line 372), add:

```python
        # Status — always available (no service dependency)
        state_intrinsics["status"] = self._handle_status
```

- [ ] **Step 4: Implement `_handle_status`**

In `agent.py`, add after `_clock_wait` method (around line 729):

```python
    def _handle_status(self, args: dict) -> dict:
        """Handle status tool — agent self-inspection."""
        action = args.get("action", "show")
        if action == "show":
            return self._status_show()
        else:
            return {"error": f"Unknown status action: {action}"}

    def _status_show(self) -> dict:
        """Return full agent self-inspection payload."""
        # Identity
        mail_addr = None
        if self._mail_service is not None and self._mail_service.address:
            mail_addr = self._mail_service.address

        # Runtime
        uptime = time.monotonic() - self._uptime_anchor if self._uptime_anchor is not None else 0.0

        # Token usage
        usage = self.get_token_usage()

        # Context window — requires active chat session
        if self._chat is not None:
            try:
                window_size = self._chat.context_window()
                ctx_total = usage["ctx_total_tokens"]
                usage_pct = round(ctx_total / window_size * 100, 1) if window_size else 0.0
            except Exception:
                window_size = None
                usage_pct = None
        else:
            window_size = None
            usage_pct = None

        return {
            "status": "ok",
            "identity": {
                "agent_id": self.agent_id,
                "working_dir": str(self._working_dir),
                "mail_address": mail_addr,
            },
            "runtime": {
                "started_at": self._started_at,
                "uptime_seconds": round(uptime, 1),
            },
            "tokens": {
                "input_tokens": usage["input_tokens"],
                "output_tokens": usage["output_tokens"],
                "thinking_tokens": usage["thinking_tokens"],
                "cached_tokens": usage["cached_tokens"],
                "total_tokens": usage["total_tokens"],
                "api_calls": usage["api_calls"],
                "context": {
                    "system_tokens": usage["ctx_system_tokens"],
                    "tools_tokens": usage["ctx_tools_tokens"],
                    "history_tokens": usage["ctx_history_tokens"],
                    "total_tokens": usage["ctx_total_tokens"],
                    "window_size": window_size,
                    "usage_pct": usage_pct,
                },
            },
        }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `python -m pytest tests/test_status.py -v`
Expected: ALL PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/agent.py tests/test_status.py
git commit -m "feat: implement status intrinsic with show action"
```

### Task 4: Memory schema module + registration

**Files:**
- Create: `src/lingtai/intrinsics/memory.py`
- Modify: `src/lingtai/intrinsics/__init__.py`

- [ ] **Step 1: Create memory schema module**

Create `src/lingtai/intrinsics/memory.py`:

```python
"""Memory intrinsic — long-term memory management.

Actions:
    load — read ltm/ltm.md from disk, reload into live system prompt, git commit

The handler lives in BaseAgent (needs access to working_dir, prompt_manager, git).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["load"],
            "description": (
                "load: read the long-term memory file (ltm/ltm.md in working directory) "
                "and reload its contents into the live system prompt. "
                "Call this after editing ltm/ltm.md with the write/edit intrinsics "
                "to make your changes take effect in the current conversation.\n"
                "Returns: status, path (absolute path to the ltm file), "
                "size_bytes (file size), content_preview (first 200 chars), "
                "diff (git diff of changes with commit hash).\n"
                "If the file does not exist, it is created empty and loaded.\n"
                "Workflow: write/edit ltm/ltm.md → call memory load → "
                "changes are now part of your system prompt and committed to git."
            ),
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Long-term memory management. The agent's persistent memory lives in "
    "ltm/ltm.md (markdown file in working directory). "
    "Edit that file with read/write/edit intrinsics, then call 'load' to "
    "reload it into the live system prompt. "
    "Use this to persist important information across context compactions "
    "and agent restarts."
)
```

- [ ] **Step 2: Write failing test for registration**

Create `tests/test_memory.py`:

```python
"""Tests for memory intrinsic — long-term memory management."""
from __future__ import annotations

import subprocess
from unittest.mock import MagicMock

import pytest

from lingtai.agent import BaseAgent
from lingtai.intrinsics import ALL_INTRINSICS


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Registration
# ---------------------------------------------------------------------------


def test_memory_in_all_intrinsics():
    """Memory should be registered in ALL_INTRINSICS with handler=None."""
    assert "memory" in ALL_INTRINSICS
    info = ALL_INTRINSICS["memory"]
    assert "schema" in info
    assert "description" in info
    assert info["handler"] is None
```

- [ ] **Step 3: Run test to verify it fails**

Run: `python -m pytest tests/test_memory.py::test_memory_in_all_intrinsics -v`
Expected: FAIL

- [ ] **Step 4: Register memory in `__init__.py`**

In `src/lingtai/intrinsics/__init__.py`, add `memory` to the import and registry:

```python
from . import read, edit, write, glob, grep, mail, vision, web_search, clock, status, memory

ALL_INTRINSICS = {
    # ... existing entries ...
    "memory": {"schema": memory.SCHEMA, "description": memory.DESCRIPTION, "handler": None},
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `python -m pytest tests/test_memory.py::test_memory_in_all_intrinsics -v`
Expected: PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/intrinsics/memory.py src/lingtai/intrinsics/__init__.py tests/test_memory.py
git commit -m "feat: add memory intrinsic schema and register"
```

## Chunk 2: Memory Intrinsic + LTM Migration

### Task 5: Wire memory + implement `_handle_memory`

**Files:**
- Modify: `src/lingtai/agent.py`

- [ ] **Step 1: Write failing tests**

Append to `tests/test_memory.py`:

```python
# ---------------------------------------------------------------------------
# Wiring
# ---------------------------------------------------------------------------


def test_memory_wired_in_agent(tmp_path):
    """Memory should be wired as an intrinsic in BaseAgent."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "memory" in agent._intrinsics


def test_memory_can_be_disabled(tmp_path):
    """Memory should be disable-able like other intrinsics."""
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        disabled_intrinsics={"memory"},
        base_dir=tmp_path,
    )
    assert "memory" not in agent._intrinsics


# ---------------------------------------------------------------------------
# load action
# ---------------------------------------------------------------------------


def test_memory_load_empty_file(tmp_path):
    """memory load with empty ltm.md should return ok with no diff."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._handle_memory({"action": "load"})
        assert result["status"] == "ok"
        assert result["size_bytes"] == 0
        assert result["diff"]["changed"] is False
    finally:
        agent.stop()


def test_memory_load_after_edit(tmp_path):
    """memory load after writing ltm.md should show diff and commit."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        # Write LTM content
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        ltm_file.write_text("# Memory\n\n- important fact\n")

        result = agent._handle_memory({"action": "load"})
        assert result["status"] == "ok"
        assert result["size_bytes"] > 0
        assert "important fact" in result["content_preview"]
        assert result["diff"]["changed"] is True
        assert result["diff"]["commit"] is not None
        assert len(result["diff"]["commit"]) == 7  # short hash
    finally:
        agent.stop()


def test_memory_load_injects_into_system_prompt(tmp_path):
    """memory load should inject ltm content into system prompt manager."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        ltm_file.write_text("# Memory\n\nI learned something\n")

        agent._handle_memory({"action": "load"})

        section = agent._prompt_manager.read_section("ltm")
        assert section is not None
        assert "I learned something" in section
    finally:
        agent.stop()


def test_memory_load_empty_removes_section(tmp_path):
    """memory load with empty file should remove ltm section from prompt."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        # First load with content
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        ltm_file.write_text("some content")
        agent._handle_memory({"action": "load"})
        assert agent._prompt_manager.read_section("ltm") is not None

        # Then load with empty
        ltm_file.write_text("")
        agent._handle_memory({"action": "load"})
        section = agent._prompt_manager.read_section("ltm")
        assert section is None or section.strip() == ""
    finally:
        agent.stop()


def test_memory_load_no_change_no_commit(tmp_path):
    """memory load with no changes should not create a new commit."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        # Load twice — second time should have no changes
        result1 = agent._handle_memory({"action": "load"})
        result2 = agent._handle_memory({"action": "load"})
        assert result2["diff"]["changed"] is False
        assert result2["diff"]["commit"] is None
    finally:
        agent.stop()


def test_memory_load_git_diff_content(tmp_path):
    """memory load should return the actual git diff."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        ltm_file.write_text("first version\n")
        agent._handle_memory({"action": "load"})

        ltm_file.write_text("second version\n")
        result = agent._handle_memory({"action": "load"})
        assert result["diff"]["changed"] is True
        assert "first version" in result["diff"]["git_diff"]
        assert "second version" in result["diff"]["git_diff"]
    finally:
        agent.stop()


def test_memory_unknown_action(tmp_path):
    """Unknown memory action should return error."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_memory({"action": "bogus"})
    assert "error" in result


def test_memory_creates_ltm_if_missing(tmp_path):
    """memory load should create ltm dir and file if they don't exist."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        # Delete ltm dir (simulating edge case)
        import shutil
        ltm_dir = agent.working_dir / "ltm"
        if ltm_dir.exists():
            shutil.rmtree(ltm_dir)

        result = agent._handle_memory({"action": "load"})
        assert result["status"] == "ok"
        assert (agent.working_dir / "ltm" / "ltm.md").is_file()
    finally:
        agent.stop()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_memory.py -v`
Expected: FAIL — `_handle_memory` not defined

- [ ] **Step 3: Wire memory in `_wire_intrinsics`**

In `agent.py`, in `_wire_intrinsics()`, after the status wiring, add:

```python
        # Memory — always available (no service dependency)
        state_intrinsics["memory"] = self._handle_memory
```

- [ ] **Step 4: Implement `_handle_memory`**

In `agent.py`, add after `_status_show`:

```python
    def _handle_memory(self, args: dict) -> dict:
        """Handle memory tool — long-term memory management."""
        action = args.get("action", "load")
        if action == "load":
            return self._memory_load()
        else:
            return {"error": f"Unknown memory action: {action}"}

    def _memory_load(self) -> dict:
        """Read ltm/ltm.md, inject into system prompt, git commit."""
        ltm_dir = self._working_dir / "ltm"
        ltm_file = ltm_dir / "ltm.md"

        # Create if missing
        ltm_dir.mkdir(exist_ok=True)
        if not ltm_file.is_file():
            ltm_file.write_text("")

        # Read file
        content = ltm_file.read_text()
        size_bytes = ltm_file.stat().st_size

        # Inject into system prompt (or remove if empty)
        if content.strip():
            self._prompt_manager.write_section("ltm", content)
        else:
            self._prompt_manager.delete_section("ltm")
        self._token_decomp_dirty = True

        # Update live session's system prompt if one exists
        if self._chat is not None:
            self._chat.update_system_prompt(self._build_system_prompt())

        # Git diff + commit
        git_diff, commit_hash = self._git_diff_and_commit_ltm()

        self._log("memory_load", size_bytes=size_bytes, changed=commit_hash is not None)

        return {
            "status": "ok",
            "path": str(ltm_file),
            "size_bytes": size_bytes,
            "content_preview": content[:200],
            "diff": {
                "changed": commit_hash is not None,
                "git_diff": git_diff or "",
                "commit": commit_hash,
            },
        }

    def _git_diff_and_commit_ltm(self) -> tuple[str | None, str | None]:
        """Run git diff on ltm/ltm.md, stage, and commit if changed.

        Returns (diff_text, short_commit_hash) or (None, None) if no changes
        or git is not available.
        """
        try:
            # Check for changes
            diff_result = subprocess.run(
                ["git", "diff", "ltm/ltm.md"],
                cwd=self._working_dir,
                capture_output=True, text=True,
            )
            # Also check for untracked new content
            diff_cached = subprocess.run(
                ["git", "diff", "--cached", "ltm/ltm.md"],
                cwd=self._working_dir,
                capture_output=True, text=True,
            )
            # Check status for untracked/new files
            status_result = subprocess.run(
                ["git", "status", "--porcelain", "ltm/ltm.md"],
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

            # Capture the diff before staging
            diff_text = diff_result.stdout or status_result.stdout

            # Stage and commit
            subprocess.run(
                ["git", "add", "ltm/ltm.md"],
                cwd=self._working_dir,
                capture_output=True, check=True,
            )

            # Get diff of staged changes (for new files, diff_result is empty)
            if not diff_text.strip():
                staged = subprocess.run(
                    ["git", "diff", "--cached", "ltm/ltm.md"],
                    cwd=self._working_dir,
                    capture_output=True, text=True,
                )
                diff_text = staged.stdout

            subprocess.run(
                ["git", "commit", "-m", "ltm: update long-term memory"],
                cwd=self._working_dir,
                capture_output=True, check=True,
            )

            # Get short commit hash
            hash_result = subprocess.run(
                ["git", "rev-parse", "--short", "HEAD"],
                cwd=self._working_dir,
                capture_output=True, text=True,
            )
            commit_hash = hash_result.stdout.strip()

            return diff_text, commit_hash

        except (FileNotFoundError, subprocess.CalledProcessError):
            # Git not available or error — load still works, just no diff/commit
            return None, None
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `python -m pytest tests/test_memory.py -v`
Expected: ALL PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/agent.py tests/test_memory.py
git commit -m "feat: implement memory intrinsic with load action and git"
```

### Task 6: LTM migration from manifest to file

**Files:**
- Modify: `src/lingtai/agent.py`

- [ ] **Step 1: Write failing tests**

Append to `tests/test_memory.py`:

```python
# ---------------------------------------------------------------------------
# LTM migration
# ---------------------------------------------------------------------------


def test_ltm_migration_from_manifest(tmp_path):
    """Agent with ltm in manifest should migrate to ltm/ltm.md on init."""
    # Create an agent with LTM via constructor (old way)
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        base_dir=tmp_path,
        ltm="old manifest ltm content",
    )
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        assert ltm_file.is_file()
        assert ltm_file.read_text() == "old manifest ltm content"

        # LTM should be in system prompt
        section = agent._prompt_manager.read_section("ltm")
        assert "old manifest ltm content" in section
    finally:
        agent.stop()


def test_ltm_migration_does_not_overwrite_existing_file(tmp_path):
    """If ltm/ltm.md already exists, migration should not overwrite it."""
    # Pre-create ltm file
    working_dir = tmp_path / "test"
    working_dir.mkdir()
    ltm_dir = working_dir / "ltm"
    ltm_dir.mkdir()
    (ltm_dir / "ltm.md").write_text("existing file content")

    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        base_dir=tmp_path,
        ltm="manifest ltm",
    )
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        assert ltm_file.read_text() == "existing file content"
    finally:
        agent.stop()


def test_manifest_no_longer_stores_ltm(tmp_path):
    """After migration, manifest should not contain ltm field."""
    import json
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        base_dir=tmp_path,
        ltm="some ltm",
    )
    agent.start()
    agent.stop()

    manifest = json.loads((agent.working_dir / ".agent.json").read_text())
    assert "ltm" not in manifest


def test_auto_load_ltm_on_resume(tmp_path):
    """On resume, ltm/ltm.md should be auto-loaded into system prompt."""
    # First run — write ltm
    agent1 = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        base_dir=tmp_path,
        ltm="initial memory",
    )
    agent1.start()
    # Edit ltm file directly
    ltm_file = agent1.working_dir / "ltm" / "ltm.md"
    ltm_file.write_text("updated memory from file")
    agent1.stop()

    # Second run — should auto-load from file
    agent2 = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    section = agent2._prompt_manager.read_section("ltm")
    assert section is not None
    assert "updated memory from file" in section
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_memory.py -k "migration or manifest or auto_load" -v`
Expected: FAIL

- [ ] **Step 3: Implement LTM migration and auto-load**

In `agent.py`, modify the `__init__` method. Replace the LTM handling section (lines 268-280) with:

```python
        # Read manifest for resume (before prompt manager, so role can be restored)
        manifest_role, manifest_ltm = self._read_manifest()
        if not role and manifest_role:
            role = manifest_role

        # LTM migration: manifest → ltm/ltm.md
        ltm_dir = self._working_dir / "ltm"
        ltm_file = ltm_dir / "ltm.md"

        # If constructor ltm is provided and ltm file doesn't exist, write it
        if ltm and not ltm_file.is_file():
            ltm_dir.mkdir(exist_ok=True)
            ltm_file.write_text(ltm)
        # If manifest has ltm and file doesn't exist, migrate
        elif manifest_ltm and not ltm_file.is_file():
            ltm_dir.mkdir(exist_ok=True)
            ltm_file.write_text(manifest_ltm)

        # Auto-load LTM from file into prompt manager
        loaded_ltm = ""
        if ltm_file.is_file():
            loaded_ltm = ltm_file.read_text()

        # System prompt manager
        self._prompt_manager = SystemPromptManager()
        if role:
            self._prompt_manager.write_section("role", role, protected=True)
        if loaded_ltm.strip():
            self._prompt_manager.write_section("ltm", loaded_ltm)

        # Write manifest (without ltm — it now lives in ltm/ltm.md)
        self._write_manifest()
```

Also modify `_write_manifest()` to stop writing `ltm`:

```python
    def _write_manifest(self) -> None:
        """Write .agent.json atomically."""
        from datetime import datetime, timezone
        data = {
            "agent_id": self.agent_id,
            "started_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "role": self._prompt_manager.read_section("role") or "",
        }
        if self._mail_service is not None and self._mail_service.address:
            data["address"] = self._mail_service.address
        target = self._working_dir / self._MANIFEST_FILE
        tmp = self._working_dir / ".agent.json.tmp"
        tmp.write_text(json.dumps(data, indent=2))
        os.replace(str(tmp), str(target))
```

And modify `_read_manifest()` — it still needs to read `ltm` for migration purposes:

```python
    def _read_manifest(self) -> tuple[str, str]:
        """Read role and ltm from .agent.json. Returns ("", "") if not found.

        Note: ltm is read for migration purposes only. New agents store ltm
        in ltm/ltm.md, not in the manifest.
        """
        path = self._working_dir / self._MANIFEST_FILE
        if not path.is_file():
            return "", ""
        try:
            data = json.loads(path.read_text())
            return data.get("role", ""), data.get("ltm", "")
        except (json.JSONDecodeError, OSError):
            corrupt = self._working_dir / ".agent.json.corrupt"
            try:
                path.rename(corrupt)
            except OSError:
                pass
            logger.warning("Corrupt .agent.json renamed to .agent.json.corrupt")
            return "", ""
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_memory.py -v`
Expected: ALL PASS

- [ ] **Step 5: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/agent.py tests/test_memory.py
git commit -m "feat: migrate LTM from manifest to ltm/ltm.md file"
```

### Task 7: Update intrinsic count + run full suite

**Files:**
- Modify: `tests/test_agent.py`

- [ ] **Step 1: Update intrinsic count**

In `tests/test_agent.py`, change:

```python
    assert len(agent._intrinsics) == 9  # read, edit, write, glob, grep, mail, vision, web_search, clock
```

to:

```python
    assert len(agent._intrinsics) == 11  # read, edit, write, glob, grep, mail, vision, web_search, clock, status, memory
```

- [ ] **Step 2: Add LTM persist-on-stop logic to `agent.py`**

In `agent.py`, in `stop()`, before `self._write_manifest()`, add logic to persist LTM from prompt manager back to `ltm/ltm.md`:

```python
        # Persist LTM from prompt manager to file
        ltm_content = self._prompt_manager.read_section("ltm") or ""
        ltm_file = self._working_dir / "ltm" / "ltm.md"
        if ltm_file.is_file() or ltm_content:
            ltm_file.parent.mkdir(exist_ok=True)
            ltm_file.write_text(ltm_content)
```

This ensures that if the agent (or host code) modifies LTM via `_prompt_manager.write_section("ltm", ...)` during execution, the changes are persisted to the file on stop.

- [ ] **Step 3: Fix `test_agent_stop_persists_ltm`**

Replace `test_agent_stop_persists_ltm` in `tests/test_agent.py` (line 568):

Old:
```python
def test_agent_stop_persists_ltm(tmp_path):
    import json
    agent = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path, ltm="initial",
    )
    agent._prompt_manager.write_section("ltm", "updated knowledge")
    agent.stop()
    data = json.loads((tmp_path / "alice" / ".agent.json").read_text())
    assert data["ltm"] == "updated knowledge"
```

New:
```python
def test_agent_stop_persists_ltm(tmp_path):
    agent = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path, ltm="initial",
    )
    agent._prompt_manager.write_section("ltm", "updated knowledge")
    agent.stop()
    ltm_file = tmp_path / "alice" / "ltm" / "ltm.md"
    assert ltm_file.is_file()
    assert ltm_file.read_text() == "updated knowledge"
```

- [ ] **Step 4: Verify `test_agent_resume_reads_role_ltm` still passes**

This test creates with `ltm="knows python"`, stops, then resumes and checks `read_section("ltm") == "knows python"`. With the migration:
1. Constructor writes `ltm` to `ltm/ltm.md`
2. Auto-load reads it into prompt manager
3. `stop()` persists prompt manager LTM back to file
4. Resume auto-loads from file

This should pass as-is. Same for `test_agent_resume_explicit_overrides_manifest`.

- [ ] **Step 5: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/agent.py tests/test_agent.py
git commit -m "test: update intrinsic count and LTM tests for file-based flow"
```
