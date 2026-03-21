# Silence & Kill Mail Types Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `cancel` mail type with two new types — `silence` (interrupt + idle, revivable by next email) and `kill` (hard stop, revivable by re-delegating).

**Architecture:** The `_cancel_event` mechanism in `BaseAgent` is reused for `silence` (same interrupt semantics). `kill` signals `_shutdown` and spawns a cleanup thread. The `cancel` type is removed entirely — no backward compat. Both mail intrinsic and email capability schemas are updated. Clock intrinsic's cancel-wake behavior is renamed to silence-wake.

**Tech Stack:** Python threading (Event, Thread), existing BaseAgent lifecycle

---

## Context

### Current cancel flow
1. Admin agent sends `type="cancel"` mail via mail/email tool
2. Target's `_on_mail_received` stores payload in `_cancel_mail`, sets `_cancel_event`
3. `_process_response` checks `_cancel_event` before each tool batch → calls `_handle_cancel_diary()` which returns empty result
4. `ToolExecutor.execute()` also checks `cancel_event` — skips remaining tools
5. Clock intrinsic `wait` wakes early on `_cancel_event`
6. Agent returns to idle, waiting for next message in `_run_loop`

### New design

| Type | Behavior | Agent state after | Revive |
|------|----------|-------------------|--------|
| `silence` | Set `_cancel_event` (interrupt current work), deactivate conscience timer, no LLM ack. Agent stays alive — `_run_loop` continues. | SLEEPING (idle in loop) | Next normal email |
| `kill` | Set `_shutdown` + `_cancel_event`. Agent thread exits `_run_loop`. Cleanup runs (mail stop, memory persist, lock release). | Dead | Parent re-delegates with same name |

### Files touched

| Action | File | What changes |
|--------|------|-------------|
| Modify | `src/lingtai/base_agent.py:96-99` | Remove `_cancel_mail`, keep `_cancel_event` |
| Modify | `src/lingtai/base_agent.py:436-458` | `_on_mail_received`: replace `cancel` branch with `silence` and `kill` branches |
| Modify | `src/lingtai/base_agent.py:651` | `_process_response`: add unconditional `_cancel_event.clear()` at top, simplify in-loop check |
| Delete | `src/lingtai/base_agent.py:727-739` | Remove `_handle_cancel_diary` method entirely |
| Modify | `src/lingtai/intrinsics/mail.py:33` | Schema: `"cancel"` → `"silence", "kill"` in type enum |
| Modify | `src/lingtai/intrinsics/mail.py:70` | Privilege gate: applies to both silence and kill |
| Modify | `src/lingtai/capabilities/email.py:92` | Schema: `"cancel"` → `"silence", "kill"` in type enum |
| Modify | `src/lingtai/capabilities/email.py:280` | Privilege gate: applies to both silence and kill |
| Modify | `src/lingtai/capabilities/email.py:644-649` | `on_normal_mail` docstring: cancel → silence/kill |
| Modify | `src/lingtai/intrinsics/clock.py:75,90` | Rename log `"cancelled"` → `"silenced"` |
| Rewrite | `tests/test_cancel_email.py` | Rename to `tests/test_silence_kill.py`, rewrite all tests |
| Modify | `tests/test_clock.py` | Update cancel wake test to use silence |

---

## Task 1: Remove `_cancel_mail` and `_handle_cancel_diary` from BaseAgent

The `_cancel_mail` payload was only used by `_handle_cancel_diary` for the LLM diary call (which was already removed in commit 9173d05). Neither `silence` nor `kill` needs the payload stored. The `_cancel_event` is sufficient.

**Files:**
- Modify: `src/lingtai/base_agent.py:98-99` (constructor)
- Modify: `src/lingtai/base_agent.py:674-675` (`_process_response` cancel check)
- Delete: `src/lingtai/base_agent.py:727-739` (`_handle_cancel_diary`)
- Test: `tests/test_silence_kill.py` (new file, replaces `tests/test_cancel_email.py`)

- [ ] **Step 1: Delete old test file, create new test file with silence interrupt test**

Delete `tests/test_cancel_email.py`. Create `tests/test_silence_kill.py`:

```python
"""Tests for silence and kill mail types."""
from __future__ import annotations

import threading
from unittest.mock import MagicMock

from lingtai.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Silence — interrupt + idle
# ---------------------------------------------------------------------------


def test_silence_sets_cancel_event(tmp_path):
    """Silence-type email should set _cancel_event."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert not agent._cancel_event.is_set()

    agent._on_mail_received({
        "from": "boss", "to": "test", "type": "silence",
    })

    assert agent._cancel_event.is_set()


def test_silence_bypasses_mail_queue(tmp_path):
    """Silence-type email should NOT enter the normal mail queue."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({
        "from": "boss", "to": "test", "type": "silence",
    })

    assert len(agent._mail_queue) == initial_count
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_silence_kill.py::test_silence_sets_cancel_event tests/test_silence_kill.py::test_silence_bypasses_mail_queue -v`

Expected: FAIL — `_on_mail_received` treats `type="silence"` as normal mail (unrecognized type falls through to `_on_normal_mail`).

- [ ] **Step 3: Implement silence branch in `_on_mail_received`**

In `src/lingtai/base_agent.py`, replace the cancel branch in `_on_mail_received`:

```python
def _on_mail_received(self, payload: dict) -> None:
    """Callback for MailService — routes by mail type.

    silence-type emails set the cancel event (interrupt current work).
    kill-type emails signal shutdown (hard stop).
    Normal emails are delegated to ``_on_normal_mail`` (which capabilities
    like email can replace).

    This method is never replaced — it is the stable entry point for all
    incoming mail.
    """
    mail_type = payload.get("type", "normal")

    if mail_type == "silence":
        self._cancel_event.set()
        self._log(
            "silence_received",
            sender=payload.get("from", "unknown"),
        )
        return

    if mail_type == "kill":
        # Handled in Task 2
        pass

    self._on_normal_mail(payload)
```

- [ ] **Step 4: Remove `_cancel_mail` from constructor**

In `src/lingtai/base_agent.py` constructor (~line 98-99), remove:
```python
self._cancel_mail: dict | None = None
```

Keep `self._cancel_event = threading.Event()`.

- [ ] **Step 5: Clear stale cancel event at top of `_process_response` and simplify cancel check**

**Critical:** The `_cancel_event` must be cleared unconditionally at the **start** of `_process_response`, not conditionally inside the tool-call branch. If silence arrives while the agent is idle (waiting in `inbox.get`), the event stays set. When the next request arrives, `_process_response` is called — if that response has no tool calls, the while loop breaks at `if not response.tool_calls: break` before ever reaching the cancel check. The stale event then corrupts all subsequent tool-call batches.

In `_process_response`, add a clear at the top of the method and simplify the tool-call-level check:

At the top of `_process_response` (before the while loop):
```python
def _process_response(self, response: LLMResponse) -> dict:
    """Handle tool calls and collect text output."""
    # Clear any stale cancel event from a previous silence.
    self._cancel_event.clear()
    guard = self._executor.guard
    ...
```

Then replace the existing cancel check inside the while loop (~line 674-675):
```python
if self._cancel_event.is_set():
    return self._handle_cancel_diary()
```
with:
```python
if self._cancel_event.is_set():
    self._cancel_event.clear()
    return {"text": "", "failed": False, "errors": []}
```

This double-clear is intentional: the top clear handles stale events from past silences, the in-loop clear handles silence arriving mid-processing.

- [ ] **Step 6: Delete `_handle_cancel_diary` method**

Remove the entire method at ~lines 727-739.

- [ ] **Step 7: Run tests to verify they pass**

Run: `python -m pytest tests/test_silence_kill.py -v`

Expected: PASS

- [ ] **Step 8: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 9: Commit**

```bash
git rm tests/test_cancel_email.py
git add tests/test_silence_kill.py src/lingtai/base_agent.py
git commit -m "refactor: replace cancel mail with silence — remove _cancel_mail and _handle_cancel_diary"
```

---

## Task 2: Implement silence conscience deactivation

When a silence mail arrives, deactivate the conscience timer if the capability is active. This requires accessing `_capability_managers` from `BaseAgent._on_mail_received`. Since `_capability_managers` is defined on `Agent` (layer 2), not `BaseAgent`, we need to use `getattr` to safely check.

**Files:**
- Modify: `src/lingtai/base_agent.py:436-458` (`_on_mail_received` silence branch)
- Test: `tests/test_silence_kill.py`

- [ ] **Step 1: Write failing test for silence + conscience**

Add to `tests/test_silence_kill.py`:

```python
from lingtai.agent import Agent


def test_silence_deactivates_conscience(tmp_path):
    """Silence should deactivate conscience timer if active."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    # Activate conscience manually
    mgr._activate()
    assert mgr._horme_active

    agent._on_mail_received({"from": "boss", "type": "silence"})

    assert not mgr._horme_active
    assert mgr._timer is None
    agent.stop(timeout=1.0)


def test_silence_without_conscience_still_works(tmp_path):
    """Silence should work fine when conscience capability is not present."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    agent._on_mail_received({"from": "boss", "type": "silence"})

    assert agent._cancel_event.is_set()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_silence_kill.py::test_silence_deactivates_conscience -v`

Expected: FAIL — silence branch doesn't touch conscience yet.

- [ ] **Step 3: Add conscience deactivation to silence branch**

In `_on_mail_received`, update the silence branch:

```python
if mail_type == "silence":
    self._cancel_event.set()
    # Deactivate conscience if present (Agent layer has _capability_managers)
    cap_managers = getattr(self, "_capability_managers", {})
    conscience = cap_managers.get("conscience")
    if conscience is not None:
        conscience.stop()
    self._log(
        "silence_received",
        sender=payload.get("from", "unknown"),
    )
    return
```

Note: Use `conscience.stop()` (the existing cleanup method) not `_deactivate()` (which is the tool-facing method). `stop()` acquires the lock, sets `_horme_active = False`, and cancels the timer.

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_silence_kill.py -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/base_agent.py tests/test_silence_kill.py
git commit -m "feat: silence mail deactivates conscience timer"
```

---

## Task 3: Implement kill mail type

Kill sets `_shutdown` + `_cancel_event` to interrupt any in-progress work and exit the run loop. Cleanup (mail service stop, memory persist, lock release) runs via `stop()` on a separate thread to avoid deadlocking the mail listener callback thread (which is the thread calling `_on_mail_received`).

**Files:**
- Modify: `src/lingtai/base_agent.py:436-458` (`_on_mail_received` — add kill branch)
- Test: `tests/test_silence_kill.py`

- [ ] **Step 1: Write failing tests for kill**

Add to `tests/test_silence_kill.py`:

```python
def test_kill_sets_shutdown_and_cancel(tmp_path):
    """Kill-type email should set both _shutdown and _cancel_event immediately."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert not agent._shutdown.is_set()
    assert not agent._cancel_event.is_set()

    agent._on_mail_received({"from": "boss", "type": "kill"})

    # Both must be set synchronously (before the stop thread runs)
    assert agent._shutdown.is_set()
    assert agent._cancel_event.is_set()


def test_kill_bypasses_mail_queue(tmp_path):
    """Kill-type email should NOT enter the normal mail queue."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({"from": "boss", "type": "kill"})

    assert len(agent._mail_queue) == initial_count


def test_kill_stops_running_agent(tmp_path):
    """Kill should cause a running agent to exit its run loop."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    assert agent._thread.is_alive()

    agent._on_mail_received({"from": "boss", "type": "kill"})

    # Wait for the stop thread to complete (it calls agent.stop())
    agent._thread.join(timeout=5.0)
    assert not agent._thread.is_alive()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_silence_kill.py::test_kill_sets_shutdown tests/test_silence_kill.py::test_kill_bypasses_mail_queue -v`

Expected: FAIL — kill falls through to `_on_normal_mail`.

- [ ] **Step 3: Implement kill branch in `_on_mail_received`**

Add after the silence branch:

```python
if mail_type == "kill":
    self._cancel_event.set()
    self._shutdown.set()
    self._log(
        "kill_received",
        sender=payload.get("from", "unknown"),
    )
    # Run stop() in a separate thread to avoid deadlocking
    # the mail listener thread (stop() joins the agent thread).
    threading.Thread(
        target=self.stop,
        daemon=True,
        name=f"kill-{self.agent_name}",
    ).start()
    return
```

**Critical:** `_cancel_event` and `_shutdown` are set synchronously (before spawning the stop thread) so that tests can assert them immediately after `_on_mail_received` returns. The stop thread handles cleanup (join agent thread, stop mail service, persist memory, release lock).

Why a separate thread: `_on_mail_received` is called from the `TCPMailService` listener thread. `stop()` calls `self._thread.join()` which waits for the agent thread to exit. Calling `stop()` inline would block the mail listener thread until the agent thread exits — while `_shutdown` is already set so the agent will exit, the mail listener would be blocked waiting. A separate thread avoids this.

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_silence_kill.py -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/base_agent.py tests/test_silence_kill.py
git commit -m "feat: add kill mail type — hard stop via separate thread"
```

---

## Task 4: Update mail intrinsic schema

Replace `"cancel"` with `"silence"` and `"kill"` in the mail intrinsic's type enum and description. The privilege gate already blocks non-normal types — just needs the description updated.

**Files:**
- Modify: `src/lingtai/intrinsics/mail.py:31-37` (schema type enum + description)
- Test: `tests/test_silence_kill.py`

- [ ] **Step 1: Write failing test for admin privilege on both types**

Add to `tests/test_silence_kill.py`:

```python
# ---------------------------------------------------------------------------
# Admin privilege gate (mail intrinsic)
# ---------------------------------------------------------------------------


def test_non_admin_cannot_send_silence_via_mail(tmp_path):
    """Non-admin should be blocked from sending silence mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin=False,
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "message": "shh", "type": "silence",
    })
    assert "error" in result or result.get("status") == "error"


def test_non_admin_cannot_send_kill_via_mail(tmp_path):
    """Non-admin should be blocked from sending kill mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin=False,
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "message": "die", "type": "kill",
    })
    assert "error" in result or result.get("status") == "error"


def test_admin_can_send_silence_via_mail(tmp_path):
    """Admin should be able to send silence mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin=True,
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    mock_mail.send.return_value = None
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "message": "shh", "type": "silence",
    })
    assert result["status"] == "delivered"
    payload = mock_mail.send.call_args[0][1]
    assert payload["type"] == "silence"


def test_admin_can_send_kill_via_mail(tmp_path):
    """Admin should be able to send kill mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin=True,
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    mock_mail.send.return_value = None
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "message": "die", "type": "kill",
    })
    assert result["status"] == "delivered"
    payload = mock_mail.send.call_args[0][1]
    assert payload["type"] == "kill"
```

- [ ] **Step 2: Run tests — admin tests should already pass (privilege gate blocks all non-normal)**

Run: `python -m pytest tests/test_silence_kill.py::test_non_admin_cannot_send_silence_via_mail tests/test_silence_kill.py::test_admin_can_send_silence_via_mail -v`

Expected: The privilege gate `if mail_type != "normal" and not agent._admin` already handles this generically. These should PASS without schema changes. (The schema enum is for LLM tool-calling guidance, not enforcement.)

- [ ] **Step 3: Update mail intrinsic schema**

In `src/lingtai/intrinsics/mail.py`, change the type enum and description:

```python
"type": {
    "type": "string",
    "enum": ["normal", "silence", "kill"],
    "description": (
        "Mail type (for send). 'normal' (default) is regular mail. "
        "'silence' interrupts the target agent and puts it to idle "
        "(revives on next email; requires admin privilege). "
        "'kill' hard-stops the target agent permanently "
        "(requires admin privilege)."
    ),
},
```

- [ ] **Step 4: Run all tests**

Run: `python -m pytest tests/test_silence_kill.py -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/intrinsics/mail.py tests/test_silence_kill.py
git commit -m "feat: update mail intrinsic schema — silence and kill types"
```

---

## Task 5: Update email capability schema

Same change as Task 4 but for the email capability. The privilege gate in `EmailManager._send` already blocks non-normal types generically.

**Files:**
- Modify: `src/lingtai/capabilities/email.py:90-97` (schema type enum + description)
- Test: `tests/test_silence_kill.py`

- [ ] **Step 1: Write test for email capability privilege gate**

Add to `tests/test_silence_kill.py`:

```python
from lingtai.agent import Agent


# ---------------------------------------------------------------------------
# Admin privilege gate (email capability)
# ---------------------------------------------------------------------------


def test_non_admin_cannot_send_silence_via_email(tmp_path):
    """Non-admin should be blocked from sending silence via email."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["email"], admin=False,
    )
    mock_mail = MagicMock()
    mock_mail.address = "me"
    agent._mail_service = mock_mail
    mgr = agent.get_capability("email")

    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "shh", "type": "silence",
    })
    assert "error" in result


def test_non_admin_cannot_send_kill_via_email(tmp_path):
    """Non-admin should be blocked from sending kill via email."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["email"], admin=False,
    )
    mock_mail = MagicMock()
    mock_mail.address = "me"
    agent._mail_service = mock_mail
    mgr = agent.get_capability("email")

    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "die", "type": "kill",
    })
    assert "error" in result
```

- [ ] **Step 2: Run tests — should pass (generic privilege gate)**

Run: `python -m pytest tests/test_silence_kill.py::test_non_admin_cannot_send_silence_via_email tests/test_silence_kill.py::test_non_admin_cannot_send_kill_via_email -v`

Expected: PASS (the `if mail_type != "normal" and not self._agent._admin` check already handles this).

- [ ] **Step 3: Update email capability schema**

In `src/lingtai/capabilities/email.py`, change the type enum and description:

```python
"type": {
    "type": "string",
    "enum": ["normal", "silence", "kill"],
    "description": (
        "Mail type (for send). 'normal' (default) is regular mail. "
        "'silence' interrupts the target agent and puts it to idle "
        "(revives on next email; requires admin privilege). "
        "'kill' hard-stops the target agent (requires admin privilege). "
        "To revive after kill: re-delegate with the SAME agent name "
        "(preserves working directory, character, and mailbox). "
        "The revived agent gets a NEW agent_id and address — "
        "update your contacts after re-delegating."
    ),
},
```

- [ ] **Step 4: Update `on_normal_mail` docstring**

In `src/lingtai/capabilities/email.py`, update the `on_normal_mail` docstring (~line 644-649). Change:
```python
    """Handle normal mail — save to mailbox and notify agent.

    Replaces BaseAgent._on_normal_mail when the email capability is active.
    Cancel-type emails never reach this method — they are handled by
    BaseAgent._on_mail_received before delegation.
    """
```
to:
```python
    """Handle normal mail — save to mailbox and notify agent.

    Replaces BaseAgent._on_normal_mail when the email capability is active.
    Silence-type and kill-type emails never reach this method — they are
    handled by BaseAgent._on_mail_received before delegation.
    """
```

- [ ] **Step 5: Run all tests including email tests**

Run: `python -m pytest tests/test_silence_kill.py tests/test_layers_email.py -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/email.py tests/test_silence_kill.py
git commit -m "feat: update email capability schema — silence and kill types"
```

---

## Task 6: Update clock intrinsic — rename "cancelled" to "silenced"

The clock `wait` action checks `_cancel_event` and returns `reason: "cancelled"`. Since this event is now triggered by silence mail, rename to `"silenced"`.

**Files:**
- Modify: `src/lingtai/intrinsics/clock.py:75-77,90-92` (reason strings and log events)
- Modify: `tests/test_clock.py` (update cancel wake test)

- [ ] **Step 1: Update clock intrinsic**

In `src/lingtai/intrinsics/clock.py`, change both occurrences:

Line 75-77:
```python
if agent._cancel_event.is_set():
    agent._log("clock_wait_end", reason="silenced", waited=0.0)
    return {"status": "ok", "reason": "silenced", "waited": 0.0}
```

Lines 90-92:
```python
if agent._cancel_event.is_set():
    agent._log("clock_wait_end", reason="silenced", waited=waited)
    return {"status": "ok", "reason": "silenced", "waited": waited}
```

- [ ] **Step 2: Update clock test**

In `tests/test_clock.py`, find the test `test_clock_wait_wakes_on_cancel` and update:
- Rename to `test_clock_wait_wakes_on_silence`
- Change assertion: `assert result["reason"] == "silenced"` (was `"cancelled"`)

- [ ] **Step 3: Run clock tests**

Run: `python -m pytest tests/test_clock.py -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/intrinsics/clock.py tests/test_clock.py
git commit -m "refactor: rename clock cancelled reason to silenced"
```

---

## Task 7: Add remaining edge-case tests and normal mail tests

Round out the test file with tests for normal mail flow (unchanged), missing type defaults, unrecognized types, and the tool executor cancel check.

**Files:**
- Test: `tests/test_silence_kill.py`

- [ ] **Step 1: Add normal mail and edge-case tests**

Add to `tests/test_silence_kill.py`:

```python
# ---------------------------------------------------------------------------
# Normal mail — unchanged behavior
# ---------------------------------------------------------------------------


def test_normal_email_queued(tmp_path):
    """Normal-type email should go through the regular queue path."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({
        "from": "colleague", "to": "test", "subject": "hello",
        "message": "hi there", "type": "normal",
    })

    assert len(agent._mail_queue) == initial_count + 1
    assert not agent._cancel_event.is_set()


def test_missing_type_defaults_to_normal(tmp_path):
    """Mail without a type field should be treated as normal."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({
        "from": "colleague", "to": "test", "message": "hi",
    })

    assert len(agent._mail_queue) == initial_count + 1
    assert not agent._cancel_event.is_set()


def test_unrecognized_type_treated_as_normal(tmp_path):
    """Unrecognized mail type should be treated as normal."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({
        "from": "someone", "type": "bogus", "message": "test",
    })

    assert len(agent._mail_queue) == initial_count + 1


def test_non_admin_can_send_normal_mail(tmp_path):
    """Non-admin should be able to send normal mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin=False,
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    mock_mail.send.return_value = None
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "subject": "hello", "message": "hi there",
    })
    assert result["status"] == "delivered"


# ---------------------------------------------------------------------------
# Internal state
# ---------------------------------------------------------------------------


def test_cancel_event_always_created(tmp_path):
    """Agent should always have _cancel_event (no external injection)."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert isinstance(agent._cancel_event, threading.Event)
    assert not agent._cancel_event.is_set()


def test_admin_flag_stored(tmp_path):
    """Admin flag should be stored on the agent."""
    agent_normal = BaseAgent(agent_name="a", service=make_mock_service(), base_dir=tmp_path)
    assert agent_normal._admin is False

    agent_admin = BaseAgent(agent_name="b", service=make_mock_service(), base_dir=tmp_path, admin=True)
    assert agent_admin._admin is True


# ---------------------------------------------------------------------------
# Tool executor cancel check
# ---------------------------------------------------------------------------


def test_sequential_execution_stops_on_cancel(tmp_path):
    """Sequential tool execution should return empty when cancel event is set."""
    from lingtai.loop_guard import LoopGuard
    from lingtai.tool_executor import ToolExecutor
    from lingtai.llm import ToolCall

    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._cancel_event.set()

    tc = ToolCall(name="clock", args={"action": "check"}, id="tc1")
    guard = LoopGuard(max_total_calls=10)

    executor = ToolExecutor(
        dispatch_fn=agent._dispatch_tool,
        make_tool_result_fn=lambda name, result, **kw: agent.service.make_tool_result(
            name, result, provider=agent._config.provider, **kw
        ),
        guard=guard,
        known_tools=set(agent._intrinsics) | set(agent._mcp_handlers),
        logger_fn=agent._log,
    )
    results, intercepted, text = executor.execute(
        [tc], cancel_event=agent._cancel_event, collected_errors=[],
    )

    assert results == []
    assert intercepted is False
```

- [ ] **Step 2: Run all tests**

Run: `python -m pytest tests/test_silence_kill.py -v`

Expected: PASS

- [ ] **Step 3: Run full test suite**

Run: `python -m pytest tests/ -v`

Expected: All tests pass (except any pre-existing failures unrelated to this change).

- [ ] **Step 4: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 5: Commit**

```bash
git add tests/test_silence_kill.py
git commit -m "test: complete silence/kill test coverage"
```

---

## Task 8: Update CLAUDE.md

Update references to cancel mail in the project documentation.

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update CLAUDE.md**

In CLAUDE.md, the `BaseAgent` description mentions `_cancel_mail`. Update to remove this reference. Specifically:
- In the BaseAgent module description, remove `_cancel_mail` from the list of attributes if present
- In the `BaseAgent` class description, update any mention of cancel-type mail to describe silence/kill

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for silence/kill mail types"
```
