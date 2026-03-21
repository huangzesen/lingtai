# Consolidate clock + status → system intrinsic

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge the `clock` intrinsic (1 action: `wait`) into the `status` intrinsic and rename it to `system`. Rename `wait` → `sleep`, `nirvana` → `restart`. Reduce from 4 intrinsics to 3.

**Architecture:** Delete `clock.py`, rename `status.py` → `system.py`, add `sleep` action (ported from clock's `wait`), rename `nirvana` → `restart`. Update `__init__.py`, `base_agent.py`, and all tests.

**Tech Stack:** Python 3.11+, pytest

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Delete | `src/lingtai/intrinsics/clock.py` | Removed — `sleep` moves to system |
| Rename+Modify | `src/lingtai/intrinsics/status.py` → `src/lingtai/intrinsics/system.py` | New `system` intrinsic: `show`, `sleep`, `shutdown`, `restart` |
| Modify | `src/lingtai/intrinsics/__init__.py` | Register `system` instead of `clock` + `status` |
| Modify | `src/lingtai/base_agent.py:202,570-611` | Line 202 comment "clock wait" → "system sleep". `_nirvana_requested` → `_restart_requested`, `_perform_nirvana` → `_perform_restart`, log events |
| Delete | `tests/test_clock.py` | Removed — sleep tests move to test_system.py |
| Rename+Modify | `tests/test_status.py` → `tests/test_system.py` | All system intrinsic tests (show + sleep + shutdown + restart) |
| Modify | `tests/test_layers_file.py:130-133` | Update intrinsic set assertion: `{"mail", "system", "eigen"}` |
| Modify | `tests/test_agent.py:49-55,296-298` | Update intrinsic name references: `"clock"` → `"system"`, count 4 → 3 |
| Modify | `tests/test_conscience.py:71-78` | Update test: `"clock"` → `"system"` |
| Modify | `tests/test_silence_kill.py:328` | Update ToolCall: `name="clock"` → `name="system"`, `args={"action": "check"}` → `args={"action": "show"}` |

---

### Task 1: Create `system.py` with sleep action, rename nirvana → restart

**Files:**
- Create: `src/lingtai/intrinsics/system.py` (from status.py + clock.py sleep logic)
- Delete: `src/lingtai/intrinsics/clock.py`
- Delete: `src/lingtai/intrinsics/status.py`

- [ ] **Step 1: Create `system.py`**

Merge `status.py` and `clock.py` into `system.py`. The new file has 4 actions: `show`, `sleep`, `shutdown`, `restart`.

```python
"""System intrinsic — runtime, lifecycle, and synchronization.

Actions:
    show     — display agent identity, runtime, and resource usage
    sleep    — pause execution; wakes on incoming message or timeout
    shutdown — initiate graceful self-termination
    restart  — stop, reload MCP servers and config from working dir, restart
"""
from __future__ import annotations

import time
from datetime import datetime, timezone

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["show", "sleep", "shutdown", "restart"],
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
                "important information to long-term memory, and identify yourself.\n\n"
                "sleep: pause execution. If seconds is given, waits up to that many seconds "
                "(wakes early if a message arrives). If seconds is omitted, blocks until "
                "a message arrives. Capped at 300 seconds.\n\n"
                "shutdown: initiate graceful self-termination. Use when you want "
                "to add more capabilities or tools. Protocol: (1) contact your admin "
                "explaining what capabilities/tools you need and why, (2) then call "
                "shutdown. A successor agent may resume from your working directory "
                "and conversation history.\n\n"
                "restart: the agent stops, reloads MCP servers and config "
                "from its working directory (mcp/servers.json), and restarts with "
                "a fresh session but the same identity. Use after installing new "
                "MCP tools to pick them up without requiring external re-delegation."
            ),
        },
        "seconds": {
            "type": "number",
            "description": (
                "For sleep: maximum seconds to wait. "
                "If omitted, waits indefinitely until a message arrives. "
                "Capped at 300."
            ),
        },
        "reason": {
            "type": "string",
            "description": "Reason for shutdown or restart (logged to event log).",
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Runtime, lifecycle, and synchronization. "
    "'show' returns identity, runtime, and resource usage. "
    "'sleep' pauses execution — specify 'seconds' for a timed sleep, "
    "or omit it to block until an incoming message arrives. "
    "'shutdown' initiates graceful self-termination. "
    "'restart' triggers rebirth — reloads MCP servers from working dir and restarts."
)


def handle(agent, args: dict) -> dict:
    """Handle system tool — runtime, lifecycle, synchronization."""
    action = args.get("action", "show")
    handler = {
        "show": _show,
        "sleep": _sleep,
        "shutdown": _shutdown,
        "restart": _restart,
    }.get(action)
    if handler is None:
        return {"status": "error", "message": f"Unknown system action: {action}"}
    return handler(agent, args)


# ---------------------------------------------------------------------------
# show
# ---------------------------------------------------------------------------

def _show(agent, args: dict) -> dict:
    mail_addr = None
    if agent._mail_service is not None and agent._mail_service.address:
        mail_addr = agent._mail_service.address

    uptime = time.monotonic() - agent._uptime_anchor if agent._uptime_anchor is not None else 0.0

    usage = agent.get_token_usage()

    if agent._chat is not None:
        try:
            window_size = agent._chat.context_window()
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
            "agent_id": agent.agent_id,
            "agent_name": agent.agent_name,
            "working_dir": str(agent._working_dir),
            "mail_address": mail_addr,
        },
        "runtime": {
            "current_time": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "started_at": agent._started_at,
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


# ---------------------------------------------------------------------------
# sleep (ported from clock.wait)
# ---------------------------------------------------------------------------

def _sleep(agent, args: dict) -> dict:
    max_wait = 300
    seconds = args.get("seconds")
    if seconds is not None:
        seconds = float(seconds)
        if seconds < 0:
            return {"status": "error", "message": "seconds must be non-negative"}
        seconds = min(seconds, max_wait)

    agent._log("system_sleep_start", seconds=seconds)

    if agent._cancel_event.is_set():
        agent._log("system_sleep_end", reason="silenced", waited=0.0)
        return {"status": "ok", "reason": "silenced", "waited": 0.0}
    if agent._mail_arrived.is_set():
        agent._log("system_sleep_end", reason="mail_arrived", waited=0.0)
        return {"status": "ok", "reason": "mail_arrived", "waited": 0.0}

    agent._mail_arrived.clear()

    poll_interval = 0.5
    t0 = time.monotonic()

    while True:
        waited = time.monotonic() - t0

        if agent._cancel_event.is_set():
            agent._log("system_sleep_end", reason="silenced", waited=waited)
            return {"status": "ok", "reason": "silenced", "waited": waited}

        if agent._mail_arrived.is_set():
            agent._log("system_sleep_end", reason="mail_arrived", waited=waited)
            return {"status": "ok", "reason": "mail_arrived", "waited": waited}

        if seconds is not None and waited >= seconds:
            agent._log("system_sleep_end", reason="timeout", waited=waited)
            return {"status": "ok", "reason": "timeout", "waited": waited}

        if seconds is not None:
            remaining = seconds - waited
            sleep_time = min(poll_interval, remaining)
        else:
            sleep_time = poll_interval

        agent._mail_arrived.wait(timeout=sleep_time)


# ---------------------------------------------------------------------------
# shutdown
# ---------------------------------------------------------------------------

def _shutdown(agent, args: dict) -> dict:
    reason = args.get("reason", "")
    agent._log("shutdown_requested", reason=reason)
    agent._shutdown.set()
    return {
        "status": "ok",
        "message": "Shutdown initiated. A successor agent may resume from your working directory and conversation history.",
    }


# ---------------------------------------------------------------------------
# restart (formerly nirvana)
# ---------------------------------------------------------------------------

def _restart(agent, args: dict) -> dict:
    reason = args.get("reason", "")
    agent._log("restart_requested", reason=reason)
    agent._restart_requested = True
    agent._shutdown.set()
    return {
        "status": "ok",
        "message": "Restart initiated — rebirth in progress. "
                   "You will be reborn with the same identity but fresh tools. "
                   "Any new MCP servers in mcp/servers.json will be loaded.",
    }
```

- [ ] **Step 2: Delete old files**

```bash
rm src/lingtai/intrinsics/clock.py
rm src/lingtai/intrinsics/status.py
```

- [ ] **Step 3: Update `__init__.py`**

Replace contents of `src/lingtai/intrinsics/__init__.py`:

```python
"""Intrinsic tools available to all agents.

Each intrinsic has:
- SCHEMA: JSON Schema dict for tool parameters
- DESCRIPTION: human-readable description
- handle: handler function(agent, args) -> dict
"""
from . import mail, system, eigen

ALL_INTRINSICS = {
    "mail": {
        "schema": mail.SCHEMA, "description": mail.DESCRIPTION, "handle": mail.handle,
        "system_prompt": "Send and receive messages. Check inbox, read, search, delete. Send to yourself to take persistent notes.",
    },
    "system": {
        "schema": system.SCHEMA, "description": system.DESCRIPTION, "handle": system.handle,
        "system_prompt": "Runtime, lifecycle, and synchronization. Inspect your state, sleep, shut down, or restart.",
    },
    "eigen": {
        "schema": eigen.SCHEMA, "description": eigen.DESCRIPTION, "handle": eigen.handle,
        "system_prompt": "Core self-management — working notes and context control.",
    },
}
```

- [ ] **Step 4: Smoke-test the module**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && source venv/bin/activate && python -c "from lingtai.intrinsics import ALL_INTRINSICS; print(sorted(ALL_INTRINSICS.keys()))"`
Expected: `['eigen', 'mail', 'system']`

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/intrinsics/system.py src/lingtai/intrinsics/__init__.py
git rm src/lingtai/intrinsics/clock.py src/lingtai/intrinsics/status.py
git commit -m "refactor: consolidate clock + status into system intrinsic

Merge clock (wait) and status (show, shutdown, nirvana) into a single
'system' intrinsic. Rename wait→sleep, nirvana→restart. Reduces
intrinsics from 4 to 3: mail, system, eigen."
```

---

### Task 2: Update base_agent.py — nirvana → restart

**Files:**
- Modify: `src/lingtai/base_agent.py:202,570-611`

- [ ] **Step 1: Update comment on line 202**

```python
        self._mail_arrived = threading.Event()  # set when normal mail arrives; system sleep uses this
```

- [ ] **Step 2: Rename nirvana → restart in base_agent.py**

All changes in `base_agent.py`:

Line 570-575 — rename the check:
```python
            # Check for restart (rebirth) before exiting
            if getattr(self, "_restart_requested", False):
                self._restart_requested = False
                self._perform_restart()
```

Line 578 — rename the method:
```python
    def _perform_restart(self) -> None:
```

Line 580 — rename log event:
```python
        self._log("restart_start")
```

Line 611 — rename log event:
```python
        self._log("restart_complete", tools=list(self._mcp_handlers.keys()))
```

- [ ] **Step 3: Smoke-test**

Run: `python -c "import lingtai"`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/base_agent.py
git commit -m "refactor: rename nirvana → restart in base_agent"
```

---

### Task 3: Update test_system.py (merge test_status + test_clock)

**Files:**
- Create: `tests/test_system.py` (merged from test_status.py + test_clock.py)
- Delete: `tests/test_status.py`
- Delete: `tests/test_clock.py`

- [ ] **Step 1: Create `tests/test_system.py`**

Merge both test files, updating all `"clock"` → `"system"`, `"status"` → `"system"`, `"wait"` → `"sleep"` in action args:

```python
"""Tests for system intrinsic — runtime, lifecycle, and synchronization."""
from __future__ import annotations

import threading
import time
from unittest.mock import MagicMock

import pytest

from lingtai.base_agent import BaseAgent
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


def test_system_in_all_intrinsics():
    assert "system" in ALL_INTRINSICS
    info = ALL_INTRINSICS["system"]
    assert "schema" in info
    assert "description" in info
    assert callable(info["handle"])


def test_system_wired_in_agent(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert "system" in agent._intrinsics


# ---------------------------------------------------------------------------
# show action
# ---------------------------------------------------------------------------


def test_system_show_returns_identity(tmp_path):
    agent = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._intrinsics["system"]({"action": "show"})
        assert result["status"] == "ok"
        identity = result["identity"]
        assert identity["agent_name"] == "alice"
        assert "alice" in identity["working_dir"]
        assert identity["mail_address"] is None
    finally:
        agent.stop()


def test_system_show_returns_runtime(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        time.sleep(0.1)
        result = agent._intrinsics["system"]({"action": "show"})
        runtime = result["runtime"]
        assert "T" in runtime["started_at"]
        assert runtime["uptime_seconds"] >= 0.05
    finally:
        agent.stop()


def test_system_show_returns_tokens(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._intrinsics["system"]({"action": "show"})
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


def test_system_show_with_mail_service(tmp_path):
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8301"
    agent = BaseAgent(
        agent_name="test",
        service=make_mock_service(),
        mail_service=mock_mail,
        base_dir=tmp_path,
    )
    agent.start()
    try:
        result = agent._intrinsics["system"]({"action": "show"})
        assert result["identity"]["mail_address"] == "127.0.0.1:8301"
    finally:
        agent.stop()


def test_system_show_context_null_without_session(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "show"})
    ctx = result["tokens"]["context"]
    assert ctx["window_size"] is None
    assert ctx["usage_pct"] is None


# ---------------------------------------------------------------------------
# sleep action (formerly clock.wait)
# ---------------------------------------------------------------------------


def test_mail_arrived_event_exists(tmp_path):
    """Agent should have a _mail_arrived threading.Event."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert isinstance(agent._mail_arrived, threading.Event)
    assert not agent._mail_arrived.is_set()


def test_system_sleep_with_seconds(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    start = time.monotonic()
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": 0.1})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "timeout"
    assert elapsed >= 0.09


def test_system_sleep_wakes_on_mail(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_mail():
        time.sleep(0.1)
        agent._mail_arrived.set()

    t = threading.Thread(target=fire_mail, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": 10})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "mail_arrived"
    assert elapsed < 5
    t.join(timeout=1)


def test_system_sleep_indefinite_wakes_on_mail(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_mail():
        time.sleep(0.1)
        agent._mail_arrived.set()

    t = threading.Thread(target=fire_mail, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._intrinsics["system"]({"action": "sleep"})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "mail_arrived"
    assert elapsed < 5
    t.join(timeout=1)


def test_system_sleep_caps_at_300(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._mail_arrived.set()
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": 9999})
    assert result["status"] == "ok"


def test_system_sleep_wakes_on_silence(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_silence():
        time.sleep(0.1)
        agent._cancel_event.set()

    t = threading.Thread(target=fire_silence, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": 10})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "silenced"
    assert elapsed < 5
    t.join(timeout=1)


def test_system_sleep_negative_seconds(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": -5})
    assert result["status"] == "error"


# ---------------------------------------------------------------------------
# shutdown
# ---------------------------------------------------------------------------


def test_system_shutdown(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "shutdown", "reason": "need bash"})
    assert result["status"] == "ok"
    assert "Shutdown initiated" in result["message"]
    assert agent._shutdown.is_set()
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# restart
# ---------------------------------------------------------------------------


def test_system_restart(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "restart", "reason": "new tools"})
    assert result["status"] == "ok"
    assert "Restart initiated" in result["message"]
    assert agent._restart_requested is True
    assert agent._shutdown.is_set()
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# unknown action
# ---------------------------------------------------------------------------


def test_system_unknown_action(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "bogus"})
    assert result["status"] == "error"
```

- [ ] **Step 2: Delete old test files**

```bash
rm tests/test_clock.py
rm tests/test_status.py
```

- [ ] **Step 3: Run tests**

Run: `python -m pytest tests/test_system.py -v`
Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add tests/test_system.py
git rm tests/test_clock.py tests/test_status.py
git commit -m "test: merge test_clock + test_status → test_system"
```

---

### Task 4: Fix remaining test references

**Files:**
- Modify: `tests/test_layers_file.py:130-133`
- Modify: `tests/test_agent.py:49-55,296-298`
- Modify: `tests/test_conscience.py:71-78`
- Modify: `tests/test_silence_kill.py:328` (if applicable)

- [ ] **Step 1: Update `tests/test_layers_file.py`**

Line 130-133 — update intrinsic set:
```python
    """BaseAgent should have exactly 3 intrinsics: mail, system, eigen."""
    ...
    assert set(agent._intrinsics.keys()) == {"mail", "system", "eigen"}
```

- [ ] **Step 2: Update `tests/test_agent.py`**

Lines 48-55 — replace the 4 individual asserts + count with 3 + count:
```python
    assert "mail" in agent._intrinsics
    assert "system" in agent._intrinsics
    assert "eigen" in agent._intrinsics
    # File I/O is now a capability, not intrinsic
    assert "read" not in agent._intrinsics
    assert "write" not in agent._intrinsics
    assert len(agent._intrinsics) == 3  # mail, system, eigen
```

Line 296-298 — update clock reference to system:
```python
    agent._intrinsics["system"] = lambda args: {"status": "ok", "time": "12:00"}
    ...
    tc = ToolCall(name="system", args={"action": "show"})
```

- [ ] **Step 3: Update `tests/test_conscience.py`**

Line 71-78 — update clock reference:
```python
def test_system_intrinsic_unchanged(tmp_path):
    """conscience does NOT replace the system intrinsic."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    assert callable(agent._intrinsics["system"])
    agent.stop(timeout=1.0)
```

- [ ] **Step 4: Update `tests/test_silence_kill.py:328`**

Line 328 — update ToolCall (currently `name="clock", args={"action": "check"}`):
```python
    tc = ToolCall(name="system", args={"action": "show"}, id="tc1")
```

- [ ] **Step 5: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add tests/test_layers_file.py tests/test_agent.py tests/test_conscience.py tests/test_silence_kill.py
git commit -m "test: update all references from clock/status to system"
```

---

### Task 5: Final verification

- [ ] **Step 1: Smoke-test import**

Run: `python -c "import lingtai"`

- [ ] **Step 2: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: all pass, no references to old `clock` or `status` intrinsic names

- [ ] **Step 3: Grep for stale references**

Run: `rg '"clock"' src/ tests/ --glob '*.py'` — should find nothing
Run: `rg 'intrinsics\["status"\]' tests/ --glob '*.py'` — should find nothing
Run: `rg 'nirvana' src/ tests/ --glob '*.py'` — should find nothing
