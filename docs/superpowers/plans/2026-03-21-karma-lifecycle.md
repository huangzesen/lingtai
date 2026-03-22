# Karma Lifecycle Control — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate lifecycle control (silence, quell, revive, annihilate) under the system intrinsic with `admin={karma, nirvana}` gates, making mail purely messaging and avatar purely spawn + ledger.

**Architecture:** Four lifecycle actions move from mail intrinsic to system intrinsic. Silence and quell use signal files detected by the heartbeat loop. Revive reconstructs the agent from its working dir via a `_revive_agent` hook. Annihilate quells then wipes the working dir. A new `handshake.py` module extracts shared validation logic.

**Tech Stack:** Python 3.11+, lingtai-kernel, lingtai, pytest

**Spec:** `docs/superpowers/specs/2026-03-21-samsara-lifecycle-design.md`

---

## File Map

### lingtai-kernel (all paths relative to `/Users/huangzesen/Documents/GitHub/lingtai-kernel/src/lingtai_kernel/`)

| File | Action | Responsibility |
|------|--------|----------------|
| `state.py` | Modify | Rename `DEAD` → `DORMANT`, update docstring |
| `handshake.py` | Create | `is_agent()`, `is_alive()`, `manifest()` utility functions |
| `services/mail.py` | Modify | Replace inline handshake with `handshake.py` imports |
| `intrinsics/mail.py` | Modify | Remove `silence`/`kill` from type enum and admin gate |
| `intrinsics/system.py` | Modify | Add silence, quell, revive, annihilate actions with admin gates |
| `base_agent.py` | Modify | Remove silence/kill from `_on_mail_received`, add signal file detection in heartbeat loop, add `_revive_agent` hook, rename DEAD refs |
| `i18n/en.json` | Modify | Add system_tool strings for new actions, update mail strings |
| `i18n/zh.json` | Modify | Same as en.json in Chinese |

### lingtai (all paths relative to `/Users/huangzesen/Documents/GitHub/lingtai/src/lingtai/`)

| File | Action | Responsibility |
|------|--------|----------------|
| `capabilities/avatar.py` | Modify | Strip reactivation/status logic, keep spawn + ledger only |
| `capabilities/email.py` | Modify | Remove silence/kill from type enum and admin gate |
| `agent.py` | Modify | Set `admin={"karma": True}` default for 本我 |
| `i18n/en.json` | Modify | Update email.type description, remove kill/silence references |
| `i18n/zh.json` | Modify | Same in Chinese |
| `i18n/wen.json` | Modify | Same in Literary Chinese |

### Tests (all paths relative to `/Users/huangzesen/Documents/GitHub/lingtai/tests/`)

| File | Action | Responsibility |
|------|--------|----------------|
| `test_silence_kill.py` | Rewrite | Rename to match new semantics, test signal file protocol + system intrinsic |
| `test_handshake.py` | Create | Test handshake utility |
| `test_karma.py` | Create | Test revive, annihilate, admin gate consolidation |

---

## Task 1: Rename `AgentState.DEAD` → `DORMANT` in kernel

**Files:**
- Modify: `lingtai-kernel:state.py`
- Modify: `lingtai-kernel:base_agent.py` (lines 5, 603, 620)

- [ ] **Step 1: Update state.py**

```python
# state.py — full file
"""AgentState — lifecycle state enum for 灵台 agents."""

from __future__ import annotations

import enum


class AgentState(enum.Enum):
    """Lifecycle state of an agent.

    ACTIVE --(completed)--------> IDLE
    ACTIVE --(timeout/exception)-> STUCK
    IDLE   --(inbox message)----> ACTIVE
    STUCK  --(AED)--------------> ACTIVE  (session reset, fresh run loop)
    STUCK  --(AED timeout)------> DORMANT (shutdown)
    ACTIVE/IDLE --(quell/shutdown)-> DORMANT
    DORMANT --(revive)-----------> IDLE    (reconstructed from working dir)
    """

    ACTIVE = "active"
    IDLE = "idle"
    STUCK = "stuck"
    DORMANT = "dormant"
```

- [ ] **Step 2: Update all DEAD references in base_agent.py**

Replace every `AgentState.DEAD` with `AgentState.DORMANT` in base_agent.py. There are references at:
- Line 5 docstring: change "ACTIVE, IDLE, ERROR, DEAD" to "ACTIVE, IDLE, STUCK, DORMANT"
- Line 603: `if self._state != AgentState.DEAD:` → `if self._state != AgentState.DORMANT:`
- Line 620: `self._set_state(AgentState.DEAD, ...)` → `self._set_state(AgentState.DORMANT, ...)`

Also grep for any string `"dead"` or `DEAD` in base_agent.py and update.

- [ ] **Step 3: Grep for DEAD references across kernel codebase**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && grep -rn "DEAD\|\.DEAD\|\"dead\"" src/`

Update any remaining references found.

- [ ] **Step 4: Grep for DEAD references across lingtai codebase**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && grep -rn "DEAD\|\.DEAD\|\"dead\"" src/ tests/`

Update any remaining references found (avatar.py line 139 at minimum).

- [ ] **Step 5: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.state import AgentState; print(AgentState.DORMANT)"`
Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -x -q`

- [ ] **Step 6: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && git add src/lingtai_kernel/state.py src/lingtai_kernel/base_agent.py && git commit -m "refactor: rename AgentState.DEAD to DORMANT"
cd /Users/huangzesen/Documents/GitHub/lingtai && git add src/ tests/ && git commit -m "refactor: follow kernel DEAD → DORMANT rename"
```

---

## Task 2: Create `handshake.py` utility in kernel

**Files:**
- Create: `lingtai-kernel:handshake.py`
- Modify: `lingtai-kernel:services/mail.py` (lines 138-159)

- [ ] **Step 1: Write test for handshake utility**

Create test file at `/Users/huangzesen/Documents/GitHub/lingtai/tests/test_handshake.py`:

```python
"""Tests for lingtai_kernel.handshake utility."""
from __future__ import annotations

import json
import time
from pathlib import Path

import pytest

from lingtai_kernel.handshake import is_agent, is_alive, manifest


@pytest.fixture
def agent_dir(tmp_path):
    """Create a minimal agent working directory."""
    meta = {"agent_id": "abc123", "agent_name": "test"}
    (tmp_path / ".agent.json").write_text(json.dumps(meta))
    return tmp_path


def test_is_agent_true(agent_dir):
    assert is_agent(agent_dir) is True


def test_is_agent_false(tmp_path):
    assert is_agent(tmp_path) is False


def test_is_agent_str_path(agent_dir):
    assert is_agent(str(agent_dir)) is True


def test_is_alive_fresh(agent_dir):
    (agent_dir / ".agent.heartbeat").write_text(str(time.time()))
    assert is_alive(agent_dir) is True


def test_is_alive_stale(agent_dir):
    (agent_dir / ".agent.heartbeat").write_text(str(time.time() - 5.0))
    assert is_alive(agent_dir) is False


def test_is_alive_no_heartbeat(agent_dir):
    assert is_alive(agent_dir) is False


def test_is_alive_custom_threshold(agent_dir):
    (agent_dir / ".agent.heartbeat").write_text(str(time.time() - 3.0))
    assert is_alive(agent_dir, threshold=5.0) is True
    assert is_alive(agent_dir, threshold=2.0) is False


def test_manifest_returns_dict(agent_dir):
    result = manifest(agent_dir)
    assert result == {"agent_id": "abc123", "agent_name": "test"}


def test_manifest_missing_file(tmp_path):
    with pytest.raises(FileNotFoundError):
        manifest(tmp_path)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_handshake.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'lingtai_kernel.handshake'`

- [ ] **Step 3: Write handshake.py**

Create at `lingtai-kernel:handshake.py` (next to `state.py`):

```python
"""Handshake utility — validate agent presence and liveness by working dir path.

Used by FilesystemMailService (mail delivery), system intrinsic (karma/nirvana
actions), and lingtai's revive logic.
"""
from __future__ import annotations

import json
import time
from pathlib import Path


def is_agent(path: str | Path) -> bool:
    """Check if an agent exists at *path* (has .agent.json)."""
    return (Path(path) / ".agent.json").is_file()


def is_alive(path: str | Path, threshold: float = 2.0) -> bool:
    """Check if the agent at *path* has a fresh heartbeat.

    Returns False if heartbeat file is missing, unreadable, or older
    than *threshold* seconds.
    """
    hb = Path(path) / ".agent.heartbeat"
    if not hb.is_file():
        return False
    try:
        ts = float(hb.read_text().strip())
    except (ValueError, OSError):
        return False
    return time.time() - ts < threshold


def manifest(path: str | Path) -> dict:
    """Read and return .agent.json contents.

    Raises FileNotFoundError if .agent.json does not exist.
    Raises json.JSONDecodeError if file is not valid JSON.
    """
    agent_json = Path(path) / ".agent.json"
    if not agent_json.is_file():
        raise FileNotFoundError(f"No .agent.json at {path}")
    return json.loads(agent_json.read_text())
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_handshake.py -v`
Expected: All PASS

- [ ] **Step 5: Refactor FilesystemMailService.send() to use handshake**

In `lingtai-kernel:services/mail.py`, replace the inline handshake logic (lines 138-159) with calls to `handshake.py`. Add import at top:

```python
from ..handshake import is_agent, is_alive, manifest
```

Replace lines 138-159 in `send()`:

```python
        # --- handshake ------------------------------------------------
        if not is_agent(address):
            return f"No agent at {address}"

        if expected_agent_id is not None:
            try:
                agent_meta = manifest(address)
            except (json.JSONDecodeError, OSError):
                return f"Cannot read agent metadata at {address}"
            if agent_meta.get("agent_id") != expected_agent_id:
                return f"Agent at {address} has changed"

        if not is_alive(address):
            return f"Agent at {address} is not running"
```

- [ ] **Step 6: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.handshake import is_agent, is_alive, manifest; print('ok')"`
Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -x -q`

- [ ] **Step 7: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && git add src/lingtai_kernel/handshake.py src/lingtai_kernel/services/mail.py && git commit -m "feat: extract handshake utility from FilesystemMailService"
cd /Users/huangzesen/Documents/GitHub/lingtai && git add tests/test_handshake.py && git commit -m "test: add handshake utility tests"
```

---

## Task 3: Remove silence/kill from mail intrinsic

**Files:**
- Modify: `lingtai-kernel:intrinsics/mail.py` (lines 48, 358-359)
- Modify: `lingtai-kernel:i18n/en.json`
- Modify: `lingtai-kernel:i18n/zh.json`

- [ ] **Step 1: Remove silence/kill from mail type enum**

In `lingtai-kernel:intrinsics/mail.py` line 48, change:
```python
"enum": ["normal", "silence", "kill"],
```
to:
```python
"enum": ["normal"],
```

- [ ] **Step 2: Remove admin gate check from mail _send()**

In `lingtai-kernel:intrinsics/mail.py` around line 358-359, remove or simplify the admin gate:
```python
# Remove these lines:
if mail_type != "normal" and not agent._admin.get(mail_type):
    return {"error": f"Not authorized to send type={mail_type!r} mail (requires admin.{mail_type}=True)"}
```

The `type` field stays in the schema (for email capability's use), but the mail intrinsic no longer validates special types.

- [ ] **Step 3: Update kernel i18n strings**

In `lingtai-kernel:i18n/en.json` and `zh.json`, update the `mail.type_description` key to remove references to silence/kill. The description should just say mail type defaults to "normal".

- [ ] **Step 4: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.intrinsics.mail import get_schema; print(get_schema())"`
Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -x -q --ignore=tests/test_silence_kill.py`

(Ignore `test_silence_kill.py` — it tests the old architecture and will be rewritten in Task 8.)

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && git add src/lingtai_kernel/intrinsics/mail.py src/lingtai_kernel/i18n/ && git commit -m "refactor: remove silence/kill from mail intrinsic — mail is pure messaging"
```

---

## Task 4: Remove silence/kill from `_on_mail_received` in BaseAgent

**Files:**
- Modify: `lingtai-kernel:base_agent.py` (lines 435-472)

- [ ] **Step 1: Simplify _on_mail_received**

Replace the silence/kill routing in `_on_mail_received` (lines 435-472). The method should now only handle normal mail:

```python
    def _on_mail_received(self, payload: dict) -> None:
        """Callback for MailService — route incoming mail to inbox.

        This method is never replaced — it is the stable entry point for all
        incoming mail. Lifecycle control (silence, quell, revive, annihilate)
        is handled by the system intrinsic via signal files, not mail.
        """
        self._on_normal_mail(payload)
```

- [ ] **Step 2: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel"`
Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -x -q --ignore=tests/test_silence_kill.py`

(Ignore `test_silence_kill.py` — will be rewritten in Task 8.)

- [ ] **Step 3: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && git add src/lingtai_kernel/base_agent.py && git commit -m "refactor: remove silence/kill routing from _on_mail_received"
```

---

## Task 5: Add signal file detection to heartbeat loop

**Files:**
- Modify: `lingtai-kernel:base_agent.py` (heartbeat_loop at lines 597-632)

- [ ] **Step 1: Write failing test for signal file detection**

Add to `/Users/huangzesen/Documents/GitHub/lingtai/tests/test_karma.py`:

```python
"""Tests for karma/nirvana lifecycle control via system intrinsic."""
from __future__ import annotations

import threading
import time
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai_kernel.base_agent import BaseAgent
from lingtai_kernel.state import AgentState


def _make_agent(tmp_path, **kwargs):
    """Create a minimal BaseAgent for testing."""
    svc = MagicMock()
    svc.create_session.return_value = MagicMock()
    agent = BaseAgent(svc, base_dir=str(tmp_path), **kwargs)
    return agent


class TestSignalFiles:
    """Signal file detection in heartbeat loop."""

    def test_silence_signal_sets_cancel_event(self, tmp_path):
        agent = _make_agent(tmp_path)
        agent.start()
        try:
            # Write .silence signal file
            (agent.working_dir / ".silence").write_text("")
            # Wait for heartbeat to detect it
            time.sleep(2.0)
            assert agent._cancel_event.is_set()
            assert not (agent.working_dir / ".silence").exists(), "signal file should be deleted"
        finally:
            agent.stop()

    def test_quell_signal_sets_shutdown(self, tmp_path):
        agent = _make_agent(tmp_path)
        agent.start()
        # Write .quell signal file
        (agent.working_dir / ".quell").write_text("")
        # Wait for agent to shut down
        time.sleep(3.0)
        assert agent._shutdown.is_set()
        assert agent.state == AgentState.DORMANT
        assert not (agent.working_dir / ".quell").exists(), "signal file should be deleted"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_karma.py::TestSignalFiles -v`
Expected: FAIL — signal files not detected

- [ ] **Step 3: Add signal file detection to heartbeat loop**

In `lingtai-kernel:base_agent.py`, modify `_heartbeat_loop` (around line 597-632). Add signal file checks after the heartbeat write, before the AED check:

```python
    def _heartbeat_loop(self) -> None:
        """Beat every 1 second. Detect signal files. AED if agent is STUCK."""
        while self._heartbeat_thread is not None and not self._shutdown.is_set():
            self._heartbeat = time.time()

            # Write heartbeat file for all living states (not DORMANT)
            if self._state != AgentState.DORMANT:
                try:
                    hb_file = self._working_dir / ".agent.heartbeat"
                    hb_file.write_text(str(self._heartbeat))
                except OSError:
                    pass

            # --- signal file detection ---
            silence_file = self._working_dir / ".silence"
            if silence_file.is_file():
                try:
                    silence_file.unlink()
                except OSError:
                    pass
                self._cancel_event.set()
                self._log("silence_received", source="signal_file")

            quell_file = self._working_dir / ".quell"
            if quell_file.is_file():
                try:
                    quell_file.unlink()
                except OSError:
                    pass
                self._cancel_event.set()
                self._shutdown.set()
                self._log("quell_received", source="signal_file")

            # --- AED for STUCK agents ---
            if self._state == AgentState.STUCK:
                # ... existing AED logic unchanged ...
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_karma.py::TestSignalFiles -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && git add src/lingtai_kernel/base_agent.py && git commit -m "feat: detect .silence and .quell signal files in heartbeat loop"
cd /Users/huangzesen/Documents/GitHub/lingtai && git add tests/test_karma.py && git commit -m "test: add signal file detection tests"
```

---

## Task 6: Add karma/nirvana actions to system intrinsic

**Files:**
- Modify: `lingtai-kernel:intrinsics/system.py`
- Modify: `lingtai-kernel:i18n/en.json`
- Modify: `lingtai-kernel:i18n/zh.json`

- [ ] **Step 1: Write failing tests for system intrinsic karma actions**

Add to `/Users/huangzesen/Documents/GitHub/lingtai/tests/test_karma.py`:

```python
class TestSystemIntrinsicKarma:
    """Karma actions in system intrinsic."""

    def test_silence_requires_karma_admin(self, tmp_path):
        agent = _make_agent(tmp_path, admin={})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "silence", "address": "/some/path"})
        assert "error" in result

    def test_silence_with_karma_admin(self, tmp_path):
        # Create a target agent dir
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')
        (target_dir / ".agent.heartbeat").write_text(str(time.time()))

        agent = _make_agent(tmp_path / "sender", admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "silence", "address": str(target_dir)})
        assert result["status"] == "silenced"
        assert (target_dir / ".silence").is_file()

    def test_quell_writes_signal_file(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')
        (target_dir / ".agent.heartbeat").write_text(str(time.time()))

        agent = _make_agent(tmp_path / "sender", admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "quell", "address": str(target_dir)})
        assert result["status"] == "quelled"
        assert (target_dir / ".quell").is_file()

    def test_quell_rejects_dormant_target(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')
        # No heartbeat = dormant

        agent = _make_agent(tmp_path / "sender", admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "quell", "address": str(target_dir)})
        assert "error" in result

    def test_silence_self_rejected(self, tmp_path):
        agent = _make_agent(tmp_path, admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "silence", "address": str(agent.working_dir)})
        assert "error" in result

    def test_annihilate_requires_nirvana_admin(self, tmp_path):
        agent = _make_agent(tmp_path / "sender", admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "annihilate", "address": "/some/path"})
        assert "error" in result

    def test_annihilate_with_nirvana_admin(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')
        # Target is dormant (no heartbeat)

        agent = _make_agent(tmp_path / "sender", admin={"karma": True, "nirvana": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "annihilate", "address": str(target_dir)})
        assert result["status"] == "annihilated"
        assert not target_dir.exists()

    def test_annihilate_self_rejected(self, tmp_path):
        agent = _make_agent(tmp_path, admin={"karma": True, "nirvana": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "annihilate", "address": str(agent.working_dir)})
        assert "error" in result

    def test_revive_rejects_alive_target(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')
        (target_dir / ".agent.heartbeat").write_text(str(time.time()))

        agent = _make_agent(tmp_path / "sender", admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "revive", "address": str(target_dir)})
        assert "error" in result
        assert "already running" in result["message"]

    def test_revive_without_handler_returns_error(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')
        # No heartbeat = dormant

        agent = _make_agent(tmp_path / "sender", admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "revive", "address": str(target_dir)})
        assert "error" in result
        assert "not supported" in result["message"].lower()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_karma.py::TestSystemIntrinsicKarma -v`
Expected: FAIL — unknown action errors

- [ ] **Step 3: Add karma/nirvana actions to system.py**

In `lingtai-kernel:intrinsics/system.py`:

1. Update schema — add new actions to enum, add `address` parameter:

```python
def get_schema(lang: str = "en") -> dict:
    from ..i18n import t
    return {
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": ["show", "sleep", "shutdown", "restart",
                         "silence", "quell", "revive", "annihilate"],
                "description": t(lang, "system_tool.action_description"),
            },
            "address": {
                "type": "string",
                "description": t(lang, "system_tool.address_description"),
            },
            "seconds": {
                "type": "number",
                "description": t(lang, "system_tool.seconds_description"),
            },
            "reason": {
                "type": "string",
                "description": t(lang, "system_tool.reason_description"),
            },
        },
        "required": ["action"],
    }
```

2. Update handler dispatch:

```python
def handle(agent, args: dict) -> dict:
    """Handle system tool — runtime, lifecycle, synchronization, karma."""
    action = args.get("action", "show")
    handler = {
        "show": _show,
        "sleep": _sleep,
        "shutdown": _shutdown,
        "restart": _restart,
        "silence": _silence,
        "quell": _quell,
        "revive": _revive,
        "annihilate": _annihilate,
    }.get(action)
    if handler is None:
        return {"status": "error", "message": f"Unknown system action: {action}"}
    return handler(agent, args)
```

3. Add karma/nirvana handler functions:

```python
# ---------------------------------------------------------------------------
# Admin gate mapping
# ---------------------------------------------------------------------------

_KARMA_ACTIONS = {"silence", "quell", "revive"}
_NIRVANA_ACTIONS = {"annihilate"}


def _check_karma_gate(agent, action: str, args: dict) -> dict | None:
    """Validate admin gate and target for karma/nirvana actions.

    Returns an error dict if validation fails, None if OK.
    """
    from ..handshake import is_agent, is_alive

    # Admin gate
    if action in _KARMA_ACTIONS and not agent._admin.get("karma"):
        return {"status": "error", "message": f"Not authorized for {action} (requires admin.karma=True)"}
    if action in _NIRVANA_ACTIONS and not agent._admin.get("nirvana"):
        return {"status": "error", "message": f"Not authorized for {action} (requires admin.nirvana=True)"}

    address = args.get("address")
    if not address:
        return {"status": "error", "message": f"{action} requires an address"}

    # Self-targeting prevention
    if str(agent._working_dir) == str(address):
        return {"status": "error", "message": f"Cannot {action} self — use shutdown/restart instead"}

    # Target must be a valid agent
    if not is_agent(address):
        return {"status": "error", "message": f"No agent at {address}"}

    return None


# ---------------------------------------------------------------------------
# silence
# ---------------------------------------------------------------------------

def _silence(agent, args: dict) -> dict:
    from ..handshake import is_alive
    err = _check_karma_gate(agent, "silence", args)
    if err:
        return err
    address = args["address"]
    if not is_alive(address):
        return {"status": "error", "message": f"Agent at {address} is not running"}
    from pathlib import Path
    (Path(address) / ".silence").write_text("")
    agent._log("karma_silence", target=address)
    return {"status": "silenced", "address": address}


# ---------------------------------------------------------------------------
# quell
# ---------------------------------------------------------------------------

def _quell(agent, args: dict) -> dict:
    from ..handshake import is_alive
    err = _check_karma_gate(agent, "quell", args)
    if err:
        return err
    address = args["address"]
    if not is_alive(address):
        return {"status": "error", "message": f"Agent at {address} is not running — already dormant?"}
    from pathlib import Path
    (Path(address) / ".quell").write_text("")
    agent._log("karma_quell", target=address)
    return {"status": "quelled", "address": address}


# ---------------------------------------------------------------------------
# revive
# ---------------------------------------------------------------------------

def _revive(agent, args: dict) -> dict:
    from ..handshake import is_alive
    err = _check_karma_gate(agent, "revive", args)
    if err:
        return err
    address = args["address"]
    if is_alive(address):
        return {"status": "error", "message": f"Agent at {address} is already running"}

    revived = agent._revive_agent(address)
    if revived is None:
        return {"status": "error", "message": "Revive not supported — no _revive_agent handler"}
    agent._log("karma_revive", target=address)
    return {"status": "revived", "address": address}


# ---------------------------------------------------------------------------
# annihilate
# ---------------------------------------------------------------------------

def _annihilate(agent, args: dict) -> dict:
    import shutil
    from pathlib import Path
    from ..handshake import is_alive

    err = _check_karma_gate(agent, "annihilate", args)
    if err:
        return err
    address = args["address"]

    # Quell first if alive
    if is_alive(address):
        (Path(address) / ".quell").write_text("")
        # Poll for shutdown (10 second timeout)
        import time
        deadline = time.time() + 10.0
        while time.time() < deadline:
            if not is_alive(address):
                break
            time.sleep(0.5)
        else:
            if is_alive(address):
                return {"status": "error", "message": f"Agent at {address} did not quell within timeout"}

    shutil.rmtree(address)
    agent._log("karma_annihilate", target=address)
    return {"status": "annihilated", "address": address}
```

- [ ] **Step 4: Add `_revive_agent` hook to BaseAgent**

In `lingtai-kernel:base_agent.py`, add the hook method (near the other public API methods):

```python
    def _revive_agent(self, address: str) -> "BaseAgent | None":
        """Reconstruct and start a dormant agent at *address*.

        Returns the revived agent, or None if not supported.
        Override in subclasses (e.g. lingtai's Agent) to provide
        full reconstruction from persisted working dir state.
        """
        return None
```

- [ ] **Step 5: Update kernel i18n strings**

In `lingtai-kernel:i18n/en.json`, update `system_tool.action_description` to include the new actions and add `system_tool.address_description`:

```json
"system_tool.action_description": "'show' returns identity, runtime, resource usage. 'sleep' pauses — 'seconds' for timed, omit for indefinite. 'shutdown' self-terminates. 'restart' rebirths. 'silence' interrupts another agent (requires admin.karma). 'quell' stops another agent (requires admin.karma). 'revive' restarts a dormant agent (requires admin.karma). 'annihilate' permanently destroys another agent (requires admin.nirvana).",
"system_tool.address_description": "Target agent's address (working directory path). Required for silence, quell, revive, annihilate.",
```

Add equivalent strings in `zh.json`.

- [ ] **Step 6: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_karma.py::TestSystemIntrinsicKarma -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && git add src/lingtai_kernel/intrinsics/system.py src/lingtai_kernel/base_agent.py src/lingtai_kernel/i18n/ && git commit -m "feat: add silence, quell, revive, annihilate to system intrinsic"
cd /Users/huangzesen/Documents/GitHub/lingtai && git add tests/test_karma.py && git commit -m "test: add karma/nirvana system intrinsic tests"
```

---

## Task 7: Implement lingtai's `_revive_agent` override + LLM config persistence

The kernel's `_revive_agent` hook returns `None` by default. Lingtai's `Agent` must override it to provide full agent reconstruction from a working dir. This requires persisting LLM config to disk at construction time.

**Files:**
- Modify: `lingtai:agent.py` — override `_revive_agent`, persist LLM config
- Test: `lingtai:tests/test_karma.py` — add revive success-path test

- [ ] **Step 1: Write failing test for revive success path**

Add to `/Users/huangzesen/Documents/GitHub/lingtai/tests/test_karma.py`:

```python
class TestReviveLingtai:
    """Revive via lingtai Agent (full reconstruction)."""

    def test_revive_reconstructs_agent(self, tmp_path):
        from lingtai.agent import Agent

        svc = MagicMock()
        svc.create_session.return_value = MagicMock()
        svc._provider = "mock"
        svc._model = "test-model"
        svc._base_url = None

        # Create and start an agent
        agent = Agent(svc, base_dir=str(tmp_path), agent_name="alice",
                      admin={"karma": True})

        # Verify LLM config was persisted to working dir
        import json
        llm_config_path = agent.working_dir / "system" / "llm.json"
        assert llm_config_path.is_file()
        llm_config = json.loads(llm_config_path.read_text())
        assert llm_config["provider"] == "mock"
        assert llm_config["model"] == "test-model"

    def test_revive_agent_hook_returns_agent(self, tmp_path):
        from lingtai.agent import Agent

        svc = MagicMock()
        svc.create_session.return_value = MagicMock()
        svc._provider = "mock"
        svc._model = "test-model"
        svc._base_url = None

        # Create a "dormant" agent — construct, persist, don't start
        target = Agent(svc, base_dir=str(tmp_path / "agents"), agent_name="bob")
        target_dir = str(target.working_dir)

        # Create the reviving agent
        reviver = Agent(svc, base_dir=str(tmp_path / "reviver"), agent_name="admin",
                        admin={"karma": True})

        # Call the hook — should reconstruct the agent
        revived = reviver._revive_agent(target_dir)
        assert revived is not None
        assert revived.agent_name == "bob"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_karma.py::TestReviveLingtai -v`
Expected: FAIL — no LLM config persisted, `_revive_agent` returns None

- [ ] **Step 3: Persist LLM config at construction**

In `lingtai:agent.py`, after `super().__init__()`, persist the LLM service config:

```python
    # Persist LLM config for revive (self-sufficient agents contract)
    llm_config = {
        "provider": service._provider,
        "model": service._model,
    }
    if getattr(service, "_base_url", None):
        llm_config["base_url"] = service._base_url
    llm_dir = self._working_dir / "system"
    llm_dir.mkdir(exist_ok=True)
    (llm_dir / "llm.json").write_text(
        json.dumps(llm_config, ensure_ascii=False)
    )
```

- [ ] **Step 4: Override `_revive_agent` in Agent**

In `lingtai:agent.py`, add the override:

```python
    def _revive_agent(self, address: str) -> "Agent | None":
        """Reconstruct and start a dormant agent from its working dir."""
        import json
        from pathlib import Path
        from lingtai_kernel.handshake import is_agent, manifest

        target = Path(address)
        if not is_agent(target):
            return None

        # Read persisted config
        agent_meta = manifest(target)
        llm_path = target / "system" / "llm.json"
        if not llm_path.is_file():
            return None
        llm_config = json.loads(llm_path.read_text())

        # Reconstruct LLMService
        from lingtai_kernel.llm import LLMService
        svc = LLMService(
            provider=llm_config["provider"],
            model=llm_config["model"],
            base_url=llm_config.get("base_url"),
        )

        # Reconstruct Agent — use the existing working dir (same agent_id)
        revived = Agent(
            svc,
            agent_name=agent_meta.get("agent_name"),
            agent_id=agent_meta.get("agent_id"),
            base_dir=str(target.parent),
        )
        revived.start()
        return revived
```

- [ ] **Step 5: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_karma.py::TestReviveLingtai -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai && git add src/lingtai/agent.py tests/test_karma.py && git commit -m "feat: implement _revive_agent override + LLM config persistence"
```

---

## Task 8: Consolidate admin keys (`silence`/`kill` → `karma`/`nirvana`) in lingtai

**Files:**
- Modify: `lingtai:capabilities/email.py` (lines 96-100, 561-562)
- Modify: `lingtai:agent.py`
- Modify: `lingtai:i18n/en.json`, `zh.json`, `wen.json`

- [ ] **Step 1: Remove silence/kill type from email capability**

In `lingtai:capabilities/email.py` line 96-100, change type enum:
```python
"enum": ["normal", "silence", "kill"],
```
to:
```python
"enum": ["normal"],
```

Remove the admin gate check at lines 561-562:
```python
if mail_type != "normal" and not self._agent._admin.get(mail_type):
    return {"error": f"Not authorized to send type={mail_type!r} mail (requires admin.{mail_type}=True)"}
```

- [ ] **Step 2: Set default admin for 本我 in agent.py**

In `lingtai:agent.py`, when constructing the Agent (the 本我), set `admin={"karma": True}` as default if not provided. Find where kwargs are passed to `super().__init__()` and add:

```python
# Default karma authority for the primary agent (本我)
kwargs.setdefault("admin", {"karma": True})
```

- [ ] **Step 3: Update lingtai i18n strings**

In `lingtai:i18n/en.json`, `zh.json`, `wen.json`:
- Update `email.type` description to remove silence/kill references
- Update `avatar.admin` description to reference karma/nirvana instead of silence/kill

- [ ] **Step 4: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -x -q --ignore=tests/test_silence_kill.py`

(Ignore test_silence_kill.py for now — it will be rewritten in Task 9)

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai && git add src/lingtai/capabilities/email.py src/lingtai/agent.py src/lingtai/i18n/ && git commit -m "refactor: consolidate admin keys to karma/nirvana, remove silence/kill from email"
```

---

## Task 9: Rewrite test_silence_kill.py for new architecture

**Files:**
- Modify: `lingtai:tests/test_silence_kill.py` → rename or rewrite

- [ ] **Step 1: Rewrite the test file**

Replace the entire contents of `test_silence_kill.py` with tests that validate:

1. **Admin gate consolidation** — `admin={"karma": True}` gates silence/quell/revive; `admin={"nirvana": True}` gates annihilate
2. **Old admin keys no longer work** — `admin={"silence": True}` does nothing
3. **Mail type is now always normal** — no special types in mail intrinsic
4. **Signal files (covered in test_karma.py but verify integration)**
5. **_cancel_event still created** — internal state preserved
6. **Tool executor cancel check** — still works (cancel_event from signal files instead of mail)

Key tests to keep (adapted):
- `test_cancel_event_always_created`
- `test_admin_dict_stored` (update expected keys)
- `test_sequential_execution_stops_on_cancel`
- `test_normal_email_notifies_inbox`
- `test_non_admin_can_send_normal_mail`

Key tests to remove:
- All `test_*_silence_*` and `test_*_kill_*` via mail — these no longer apply

Key tests to add:
- `test_old_admin_keys_ignored` — `admin={"silence": True}` does not grant karma
- `test_karma_admin_stored`

- [ ] **Step 2: Run all tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/test_silence_kill.py -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai && git add tests/test_silence_kill.py && git commit -m "test: rewrite silence/kill tests for karma lifecycle architecture"
```

---

## Task 10: Strip avatar to spawn + ledger only

**Files:**
- Modify: `lingtai:capabilities/avatar.py`

- [ ] **Step 1: Write test for simplified avatar**

Add to or update existing avatar tests to verify:
- Spawn still works
- Ledger still records spawn events
- If name exists and agent is alive, return error (no more reactivation)
- No more status-checking logic

- [ ] **Step 2: Strip avatar capability**

In `lingtai:capabilities/avatar.py`:

1. Remove `_live_status()` method entirely (lines 125-140)
2. Simplify `_spawn()` — remove the idle/error/active status branching (lines 156-190). Replace with:

```python
        # Check if this peer already exists and is live
        existing = self._peers.get(peer_name)
        if existing is not None:
            from lingtai_kernel.handshake import is_alive
            if is_alive(str(existing.working_dir)):
                return {
                    "status": "already_active",
                    "address": existing._mail_service.address if existing._mail_service else None,
                    "agent_id": existing.agent_id,
                    "agent_name": existing.agent_name,
                    "message": (
                        f"'{peer_name}' is already running. "
                        f"Use mail to communicate, or system intrinsic to manage lifecycle."
                    ),
                }
            # Not alive — clean up stale reference
            self._peers.pop(peer_name, None)
```

3. Remove `AgentState` import if it's no longer used after removing `_live_status`.

- [ ] **Step 3: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -x -q`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai && git add -A && git commit -m "refactor: strip avatar to spawn + ledger only — lifecycle moves to system intrinsic"
```

---

## Task 11: Full integration test and cleanup

**Files:**
- All modified files across both repos

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: All PASS

- [ ] **Step 2: Smoke test imports**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel; print('kernel ok')"
cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai; print('lingtai ok')"
```

- [ ] **Step 3: Verify no stale references**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && grep -rn "\"kill\"\|\"silence\"\|DEAD\|admin\.silence\|admin\.kill" src/
cd /Users/huangzesen/Documents/GitHub/lingtai && grep -rn "\"kill\"\|AgentState\.DEAD\|admin\.silence\|admin\.kill" src/ tests/
```

Expected: No matches (or only in comments/docs explaining the migration)

- [ ] **Step 4: Final commit if any cleanup needed**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai && git add -A && git commit -m "chore: final cleanup for karma lifecycle migration"
```
