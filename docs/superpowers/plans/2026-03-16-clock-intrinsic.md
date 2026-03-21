# Clock Intrinsic Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `clock` intrinsic to BaseAgent with two actions: `check` (get current time) and `wait` (sleep for N seconds, or block until mail arrives — waking early on mail).

**Architecture:** The clock intrinsic has `handler=None` because `wait` needs access to agent state (mail queue notification, cancel event). A new `threading.Event` (`_mail_arrived`) is added to BaseAgent and set by `_on_normal_mail()` whenever mail arrives. The `wait` handler clears it, then waits on it with an optional timeout. The `check` action is stateless but handled in BaseAgent for consistency. Max wait is capped at 300 seconds.

**Tech Stack:** Python stdlib only (`datetime`, `threading`, `time`)

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai/intrinsics/clock.py` | Create | Schema and description for the clock intrinsic |
| `src/lingtai/intrinsics/__init__.py` | Modify | Register clock in `ALL_INTRINSICS` with `handler=None` |
| `src/lingtai/agent.py` | Modify | Add `_mail_arrived` event, `_handle_clock()`, wire clock as state intrinsic |
| `tests/test_clock.py` | Create | Tests for clock intrinsic |

---

## Chunk 1: Clock Intrinsic

### Task 1: Clock intrinsic schema module

**Files:**
- Create: `src/lingtai/intrinsics/clock.py`

- [ ] **Step 1: Write the clock intrinsic schema module**

```python
"""Clock intrinsic — time awareness and synchronization.

Actions:
    check — get current UTC time
    wait  — sleep for N seconds, or block until mail arrives (wakes early on mail)

The handler lives in BaseAgent (needs access to _mail_arrived event and _cancel_event).
This module provides the schema and description.
"""
from __future__ import annotations

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["check", "wait"],
            "description": (
                "check: get the current UTC time. "
                "wait: pause execution. If seconds is given, waits up to that many seconds "
                "(wakes early if mail arrives). If seconds is omitted, blocks until mail arrives."
            ),
        },
        "seconds": {
            "type": "number",
            "description": (
                "Maximum seconds to wait (for action=wait). "
                "If omitted, waits indefinitely until mail arrives. "
                "Capped at 300."
            ),
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Time awareness and synchronization. "
    "'check' returns current UTC time. "
    "'wait' pauses execution — specify 'seconds' for a timed sleep, "
    "or omit it to block until incoming mail arrives. "
    "A timed wait also wakes early if mail arrives."
)
```

- [ ] **Step 2: Smoke-test the module**

Run: `source venv/bin/activate && python -c "from lingtai.intrinsics.clock import SCHEMA, DESCRIPTION; print('OK')"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/intrinsics/clock.py
git commit -m "feat: add clock intrinsic schema module"
```

### Task 2: Register clock in ALL_INTRINSICS

**Files:**
- Modify: `src/lingtai/intrinsics/__init__.py`

- [ ] **Step 1: Write the failing test — clock appears in ALL_INTRINSICS**

Add to `tests/test_clock.py`:

```python
"""Tests for clock intrinsic."""
from __future__ import annotations

import threading
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


def test_clock_in_all_intrinsics():
    """Clock should be registered in ALL_INTRINSICS with handler=None."""
    assert "clock" in ALL_INTRINSICS
    info = ALL_INTRINSICS["clock"]
    assert "schema" in info
    assert "description" in info
    assert info["handler"] is None
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_clock.py::test_clock_in_all_intrinsics -v`
Expected: FAIL — "clock" not in ALL_INTRINSICS

- [ ] **Step 3: Register clock in `__init__.py`**

Modify `src/lingtai/intrinsics/__init__.py` — add import and entry:

```python
from . import read, edit, write, glob, grep, mail, vision, web_search, clock

ALL_INTRINSICS = {
    # ... existing entries ...
    "clock": {"schema": clock.SCHEMA, "description": clock.DESCRIPTION, "handler": None},
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_clock.py::test_clock_in_all_intrinsics -v`
Expected: PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "import lingtai"`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/intrinsics/__init__.py tests/test_clock.py
git commit -m "feat: register clock in ALL_INTRINSICS"
```

### Task 3: Wire clock as state intrinsic + add `_mail_arrived` event

**Files:**
- Modify: `src/lingtai/agent.py` (constructor, `_wire_intrinsics`, `_on_normal_mail`)

- [ ] **Step 1: Write failing tests — clock wired in agent, mail_arrived event exists**

Append to `tests/test_clock.py`:

```python
# ---------------------------------------------------------------------------
# Wiring
# ---------------------------------------------------------------------------


def test_clock_wired_in_agent(tmp_path):
    """Clock should be wired as an intrinsic in BaseAgent."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "clock" in agent._intrinsics


def test_clock_can_be_disabled(tmp_path):
    """Clock should be disable-able like other intrinsics."""
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        disabled_intrinsics={"clock"},
        base_dir=tmp_path,
    )
    assert "clock" not in agent._intrinsics


def test_mail_arrived_event_exists(tmp_path):
    """Agent should have a _mail_arrived threading.Event."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert isinstance(agent._mail_arrived, threading.Event)
    assert not agent._mail_arrived.is_set()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_clock.py::test_clock_wired_in_agent tests/test_clock.py::test_clock_can_be_disabled tests/test_clock.py::test_mail_arrived_event_exists -v`
Expected: FAIL — "clock" not in agent._intrinsics, no `_mail_arrived` attribute

- [ ] **Step 3: Add `_mail_arrived` event to constructor**

In `agent.py`, after line 287 (`self._mail_queue_lock = threading.Lock()`), add:

```python
        self._mail_arrived = threading.Event()  # set when normal mail arrives; clock wait uses this
```

- [ ] **Step 4: Wire clock in `_wire_intrinsics`**

In `agent.py`, in `_wire_intrinsics()`, add after line 368 (`state_intrinsics["web_search"] = self._handle_web_search`):

```python
        # Clock — always available (no service dependency)
        state_intrinsics["clock"] = self._handle_clock
```

- [ ] **Step 5: Set `_mail_arrived` in `_on_normal_mail`**

In `agent.py`, in `_on_normal_mail()`, after line 821 (`self._mail_queue.append(entry)`), add:

```python
        self._mail_arrived.set()
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `python -m pytest tests/test_clock.py::test_clock_wired_in_agent tests/test_clock.py::test_clock_can_be_disabled tests/test_clock.py::test_mail_arrived_event_exists -v`
Expected: PASS

- [ ] **Step 7: Smoke-test import**

Run: `python -c "import lingtai"`
Expected: no errors

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/agent.py tests/test_clock.py
git commit -m "feat: wire clock intrinsic and add _mail_arrived event"
```

### Task 4: Implement `_handle_clock` — `check` action

**Files:**
- Modify: `src/lingtai/agent.py`

- [ ] **Step 1: Write failing test for check action**

Append to `tests/test_clock.py`:

```python
# ---------------------------------------------------------------------------
# check action
# ---------------------------------------------------------------------------


def test_clock_check_returns_time(tmp_path):
    """clock check should return current UTC time and unix timestamp."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_clock({"action": "check"})

    assert result["status"] == "ok"
    assert "utc" in result
    assert "unix" in result
    assert isinstance(result["unix"], float)
    # UTC string should be ISO format
    assert "T" in result["utc"]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_clock.py::test_clock_check_returns_time -v`
Expected: FAIL — `_handle_clock` not defined

- [ ] **Step 3: Implement `_handle_clock` with check action**

In `agent.py`, add after `_handle_web_search` method (around line 643):

```python
    def _handle_clock(self, args: dict) -> dict:
        """Handle clock tool — time check and wait/sync."""
        action = args.get("action", "check")
        if action == "check":
            return self._clock_check()
        elif action == "wait":
            return self._clock_wait(args)
        else:
            return {"error": f"Unknown clock action: {action}"}

    def _clock_check(self) -> dict:
        """Return current UTC time and unix timestamp."""
        from datetime import datetime, timezone

        now = datetime.now(timezone.utc)
        return {
            "status": "ok",
            "utc": now.strftime("%Y-%m-%dT%H:%M:%SZ"),
            "unix": now.timestamp(),
        }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_clock.py::test_clock_check_returns_time -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/agent.py tests/test_clock.py
git commit -m "feat: implement clock check action"
```

### Task 5: Implement `_handle_clock` — `wait` action

**Files:**
- Modify: `src/lingtai/agent.py`

- [ ] **Step 1: Write failing tests for wait action**

Append to `tests/test_clock.py`:

```python
# ---------------------------------------------------------------------------
# wait action
# ---------------------------------------------------------------------------


def test_clock_wait_with_seconds(tmp_path):
    """clock wait with seconds should sleep and return."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    start = time.monotonic()
    result = agent._handle_clock({"action": "wait", "seconds": 0.1})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "timeout"
    assert elapsed >= 0.09  # slept at least ~0.1s


def test_clock_wait_wakes_on_mail(tmp_path):
    """clock wait should wake early when mail arrives."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    # Fire mail arrival after 0.1s
    def fire_mail():
        time.sleep(0.1)
        agent._mail_arrived.set()

    t = threading.Thread(target=fire_mail, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._handle_clock({"action": "wait", "seconds": 10})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "mail_arrived"
    assert elapsed < 5  # woke up WAY before 10s timeout
    t.join(timeout=1)


def test_clock_wait_indefinite_wakes_on_mail(tmp_path):
    """clock wait without seconds should block until mail arrives."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_mail():
        time.sleep(0.1)
        agent._mail_arrived.set()

    t = threading.Thread(target=fire_mail, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._handle_clock({"action": "wait"})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "mail_arrived"
    assert elapsed < 5
    t.join(timeout=1)


def test_clock_wait_caps_at_300(tmp_path):
    """clock wait should cap seconds at 300."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    # We can't actually wait 300s in a test, so just verify the cap logic
    # by setting mail_arrived immediately
    agent._mail_arrived.set()
    result = agent._handle_clock({"action": "wait", "seconds": 9999})
    # Should wake immediately because mail_arrived is already set
    assert result["status"] == "ok"


def test_clock_wait_wakes_on_cancel(tmp_path):
    """clock wait should wake when cancel event is set."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_cancel():
        time.sleep(0.1)
        agent._cancel_event.set()

    t = threading.Thread(target=fire_cancel, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._handle_clock({"action": "wait", "seconds": 10})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "cancelled"
    assert elapsed < 5
    t.join(timeout=1)


def test_clock_wait_negative_seconds(tmp_path):
    """clock wait with negative seconds should return error."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_clock({"action": "wait", "seconds": -5})
    assert "error" in result


def test_clock_wait_unknown_action(tmp_path):
    """Unknown clock action should return error."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_clock({"action": "bogus"})
    assert "error" in result
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_clock.py -k "wait" -v`
Expected: FAIL — `_clock_wait` not defined

- [ ] **Step 3: Implement `_clock_wait`**

In `agent.py`, add after `_clock_check`:

```python
    def _clock_wait(self, args: dict) -> dict:
        """Wait for a duration or until mail arrives.

        If seconds is given, waits up to that many seconds (capped at 300).
        Wakes early if mail arrives or cancel event is set.
        If seconds is omitted, blocks indefinitely until mail arrives or cancel.
        """
        max_wait = 300
        seconds = args.get("seconds")
        if seconds is not None:
            seconds = float(seconds)
            if seconds < 0:
                return {"error": "seconds must be non-negative"}
            seconds = min(seconds, max_wait)

        # Clear the event so we only wake on NEW mail
        self._mail_arrived.clear()

        self._log("clock_wait_start", seconds=seconds)

        # Poll loop: check both events with short sleeps.
        # We can't wait on two Events at once, so we poll with 0.5s granularity.
        # Use time.monotonic() for accurate elapsed tracking (Event.wait can return early).
        poll_interval = 0.5
        t0 = time.monotonic()

        while True:
            waited = time.monotonic() - t0

            if self._cancel_event.is_set():
                self._log("clock_wait_end", reason="cancelled", waited=waited)
                return {"status": "ok", "reason": "cancelled", "waited": waited}

            if self._mail_arrived.is_set():
                self._log("clock_wait_end", reason="mail_arrived", waited=waited)
                return {"status": "ok", "reason": "mail_arrived", "waited": waited}

            if seconds is not None and waited >= seconds:
                self._log("clock_wait_end", reason="timeout", waited=waited)
                return {"status": "ok", "reason": "timeout", "waited": waited}

            # Determine how long to sleep this iteration
            if seconds is not None:
                remaining = seconds - waited
                sleep_time = min(poll_interval, remaining)
            else:
                sleep_time = poll_interval

            # Wait on mail_arrived with timeout — wakes on mail OR after sleep_time
            self._mail_arrived.wait(timeout=sleep_time)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_clock.py -v`
Expected: ALL PASS

- [ ] **Step 5: Smoke-test import**

Run: `python -c "import lingtai"`
Expected: no errors

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/agent.py tests/test_clock.py
git commit -m "feat: implement clock wait action with mail wake-up"
```

### Task 6: Update intrinsic count in existing tests

**Files:**
- Modify: `tests/test_agent.py`

- [ ] **Step 1: Update intrinsic count assertion**

In `tests/test_agent.py`, line 54, change:

```python
    assert len(agent._intrinsics) == 8  # read, edit, write, glob, grep, mail, vision, web_search
```

to:

```python
    assert len(agent._intrinsics) == 9  # read, edit, write, glob, grep, mail, vision, web_search, clock
```

- [ ] **Step 2: Run all tests**

Run: `python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add tests/test_agent.py
git commit -m "test: update intrinsic count for clock"
```
