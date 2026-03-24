# Five-State Agent Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand the agent lifecycle from 4 states to 5 (ACTIVE/IDLE/STUCK/DORMANT/SUSPENDED), where DORMANT is sleep — heartbeat and listeners alive, soul flow off, LLM paused — and SUSPENDED is true process death triggered only by external signals.

**Architecture:** DORMANT is sleep. The soul flow (inner voice / consciousness) is tied to waking states — it cannot be selectively turned off. The only way to silence it is to change state to DORMANT, which implies soul off. Waking (DORMANT → ACTIVE) implies soul back on. This mirrors human consciousness: you can't stop your inner voice while awake; only sleep silences it.

| State | Body (heartbeat) | Mind (LLM) | Consciousness (soul flow) |
|---|---|---|---|
| ACTIVE | beating | working | flowing |
| IDLE | beating | waiting | flowing |
| STUCK | beating | broken | flowing |
| DORMANT (眠) | beating | waiting | **off** |
| SUSPENDED (假死) | off | off | off |

DORMANT keeps `_run_loop` alive but paused, heartbeat writing, IMAP/telegram listeners running. When any listener puts a message in the inbox, the agent wakes to ACTIVE and soul flow resumes. SUSPENDED is full process death, triggered only by `.suspend` file or OS signals (SIGINT/SIGTERM). The `.quell` file now means DORMANT (sleep), not death.

**Tech Stack:** Python 3.11+, threading, lingtai-kernel + lingtai

---

## File Structure

| File | Change | Responsibility |
|------|--------|----------------|
| `lingtai-kernel/src/lingtai_kernel/state.py` | Modify | Add SUSPENDED to enum |
| `lingtai-kernel/src/lingtai_kernel/config.py` | Modify | Rename `vigil` → `stamina` |
| `lingtai-kernel/src/lingtai_kernel/base_agent.py` | Modify | Heartbeat in DORMANT, dormant-wake loop, `.suspend` detection, vigil→stamina rename, stamina reset on wake |
| `lingtai-kernel/src/lingtai_kernel/intrinsics/system.py` | Modify | Self-quell → DORMANT (remove karma requirement), quell-other → DORMANT, vigil→stamina in `show` |
| `lingtai/src/lingtai/cli.py` | Modify | Only exit process on SUSPENDED, not DORMANT, vigil→stamina in init.json handling |
| `lingtai/src/lingtai/init_schema.py` | Modify | Rename `vigil` → `stamina` in validation |
| `lingtai/src/lingtai/agent.py` | Modify | `stop()` only kills addons/MCP on SUSPENDED, not DORMANT |
| `lingtai-kernel/tests/` | Modify | Update tests for new state + stamina rename |
| `lingtai/tests/` | Modify | Update CLI tests for stamina rename |

---

### Task 1: Add SUSPENDED to AgentState enum

**Files:**
- Modify: `lingtai-kernel/src/lingtai_kernel/state.py:8-23`

- [ ] **Step 1: Update the enum**

Add SUSPENDED and update the docstring:

```python
class AgentState(enum.Enum):
    """Lifecycle state of an agent.

    ACTIVE --(completed)--------> IDLE
    ACTIVE --(timeout/exception)-> STUCK
    IDLE   --(inbox message)----> ACTIVE
    STUCK  --(AED)--------------> ACTIVE  (session reset, fresh run loop)
    STUCK  --(AED timeout)------> DORMANT (sleep, listeners alive)
    ACTIVE/IDLE --(quell)--------> DORMANT
    DORMANT --(inbox message)---> ACTIVE  (wake from sleep)
    DORMANT --(.suspend/SIGINT)-> SUSPENDED (process exits)
    SUSPENDED --(lingtai run)---> IDLE    (reconstructed from working dir)
    """

    ACTIVE = "active"
    IDLE = "idle"
    STUCK = "stuck"
    DORMANT = "dormant"
    SUSPENDED = "suspended"
```

- [ ] **Step 2: Smoke-test import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.state import AgentState; print(AgentState.SUSPENDED)"`
Expected: `AgentState.SUSPENDED`

- [ ] **Step 3: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/state.py
git commit -m "feat: add SUSPENDED state to AgentState enum"
```

---

### Task 2: Heartbeat writes in DORMANT, detects `.suspend`

**Files:**
- Modify: `lingtai-kernel/src/lingtai_kernel/base_agent.py` — `_heartbeat_loop()` method (~line 568-633)

The heartbeat must:
1. Write heartbeat file in ALL states (remove the `if self._state != AgentState.DORMANT` guard)
2. `.quell` file → DORMANT (sleep, set `_dormant` event, do NOT set `_shutdown`)
3. `.suspend` file → SUSPENDED (set `_shutdown`, full process exit)
4. Vigil expired → DORMANT (not SUSPENDED)

- [ ] **Step 1: Add `_dormant` event to `__init__`**

Find the `_shutdown` initialization (~line 208) and add nearby:

```python
self._dormant = threading.Event()   # set when entering DORMANT; cleared on wake
```

- [ ] **Step 2: Rewrite heartbeat to write in all states and detect `.suspend`**

Replace the `_heartbeat_loop` method body. Key changes:
- Remove `if self._state != AgentState.DORMANT` guard on heartbeat write
- `.quell` → set `_dormant` + `_cancel_event`, transition to DORMANT, do NOT set `_shutdown`
- Add `.suspend` detection → set `_shutdown`, transition to SUSPENDED
- Vigil expired → set `_dormant` + `_cancel_event`, transition to DORMANT, do NOT set `_shutdown`
- AED failed → DORMANT (not SUSPENDED) — set `_dormant`, do NOT set `_shutdown`

```python
def _heartbeat_loop(self) -> None:
    """Beat every 1 second. AED if agent is STUCK."""
    while self._heartbeat_thread is not None and not self._shutdown.is_set():
        self._heartbeat = time.time()

        # Write heartbeat file in ALL living states (everything except SUSPENDED)
        try:
            hb_file = self._working_dir / ".agent.heartbeat"
            hb_file.write_text(str(self._heartbeat))
        except OSError:
            pass

        # --- signal file detection ---
        interrupt_file = self._working_dir / ".interrupt"
        if interrupt_file.is_file():
            try:
                interrupt_file.unlink()
            except OSError:
                pass
            self._cancel_event.set()
            self._log("interrupt_received", source="signal_file")

        # .suspend = SUSPENDED (full process death, external only)
        suspend_file = self._working_dir / ".suspend"
        if suspend_file.is_file():
            try:
                suspend_file.unlink()
            except OSError:
                pass
            self._cancel_event.set()
            self._set_state(AgentState.SUSPENDED, reason="suspend signal")
            self._shutdown.set()
            self._log("suspend_received", source="signal_file")

        # .quell = DORMANT (sleep, listeners stay alive)
        quell_file = self._working_dir / ".quell"
        if quell_file.is_file():
            try:
                quell_file.unlink()
            except OSError:
                pass
            self._cancel_event.set()
            self._set_state(AgentState.DORMANT, reason="quell signal")
            self._dormant.set()
            self._log("quell_received", source="signal_file")

        # Vigil enforcement — dormant when stamina expires
        if self._uptime_anchor is not None and self._state not in (AgentState.DORMANT, AgentState.SUSPENDED):
            elapsed = time.monotonic() - self._uptime_anchor
            if elapsed >= self._config.stamina:
                self._log("stamina_expired", elapsed=round(elapsed, 1), stamina=self._config.stamina)
                self._cancel_event.set()
                self._set_state(AgentState.DORMANT, reason="stamina expired")
                self._dormant.set()

        if self._state == AgentState.STUCK:
            now = time.monotonic()
            if self._cpr_start is None:
                self._cpr_start = now

            elapsed = now - self._cpr_start
            cpr_timeout = self._config.cpr_timeout
            if elapsed > cpr_timeout:
                # AED failed — go dormant (not suspended)
                self._log("heartbeat_dead", heartbeat=self._heartbeat, aed_seconds=elapsed)
                self._set_state(AgentState.DORMANT, reason="AED failed")
                self._persist_chat_history()
                self._dormant.set()
            elif not self._aed_pending:
                # Perform AED — hard restart
                self._aed_pending = True
                self._perform_aed()
        else:
            # Healthy or idle — reset AED window
            self._cpr_start = None
            self._aed_pending = False

        time.sleep(1.0)
```

- [ ] **Step 3: Smoke-test import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel"`

- [ ] **Step 4: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/base_agent.py
git commit -m "feat: heartbeat writes in DORMANT, .suspend for full death"
```

---

### Task 3: Rewrite `_run_loop` to support dormant-wake cycle

**Files:**
- Modify: `lingtai-kernel/src/lingtai_kernel/base_agent.py` — `_run_loop()` method (~line 665-704)

The run loop must:
1. When `_dormant` is set: pause the LLM loop, cancel soul timer, close LLM session
2. Wait for a message in the inbox (blocking)
3. When message arrives: clear `_dormant`, reopen LLM session, restart soul timer, resume

- [ ] **Step 1: Rewrite `_run_loop`**

```python
def _run_loop(self) -> None:
    """Wait for messages, process them. Agent persists between messages."""
    while True:
        while not self._shutdown.is_set():
            # --- Dormant sleep: pause LLM, wait for inbox message ---
            if self._dormant.is_set():
                self._cancel_soul_timer()
                self._session.close()
                self._log("dormant_sleep")

                # Block until a message arrives or shutdown
                while not self._shutdown.is_set():
                    try:
                        msg = self.inbox.get(timeout=1.0)
                        break
                    except queue.Empty:
                        continue
                else:
                    break  # shutdown was set — exit inner loop

                # Wake up
                self._dormant.clear()
                self._set_state(AgentState.ACTIVE, reason=f"woke from dormant: {msg.type}")
                self._log("dormant_wake", trigger=msg.type)
                self._session.reopen()
                self._reset_uptime()
                msg = self._concat_queued_messages(msg)
                # Fall through to handle the message below
            else:
                try:
                    msg = self.inbox.get(timeout=self._inbox_timeout)
                except queue.Empty:
                    continue
                msg = self._concat_queued_messages(msg)
                self._set_state(AgentState.ACTIVE, reason=f"received {msg.type}")

            sleep_state = AgentState.IDLE
            try:
                self._handle_message(msg)
            except TimeoutError as e:
                err_desc = str(e) or repr(e)
                logger.error(
                    f"[{self.agent_name}] LLM timeout in message handler: {err_desc}",
                    exc_info=True,
                )
                self._log("error", source="message_handler", message=err_desc)
                sleep_state = AgentState.STUCK
            except Exception as e:
                err_desc = str(e) or repr(e)
                logger.error(
                    f"[{self.agent_name}] Unhandled error in message handler: {err_desc}",
                    exc_info=True,
                )
                self._log("error", source="message_handler", message=err_desc)
                sleep_state = AgentState.STUCK
            finally:
                self._set_state(sleep_state)
                self._persist_chat_history()

        # Check for refresh (rebirth) before exiting
        if getattr(self, "_refresh_requested", False):
            self._refresh_requested = False
            self._perform_refresh()
            self._shutdown.clear()
            continue  # re-enter the message loop
        break  # SUSPENDED — exit for real
```

- [ ] **Step 2: Add `_reset_uptime` helper** (near `start()`)

```python
def _reset_uptime(self) -> None:
    """Reset the uptime anchor for stamina tracking (used on wake from dormant)."""
    self._uptime_anchor = time.monotonic()
```

- [ ] **Step 3: Verify `_session.reopen()` and `_session.close()` exist**

Check `lingtai-kernel/src/lingtai_kernel/session.py` for these methods. If `reopen()` doesn't exist, it may need to be added or we use `ensure_session()` on next LLM call instead. The session is lazy — it may just work on next `send()`.

If `reopen()` doesn't exist, remove the `self._session.reopen()` call — the session will reconnect lazily on the next LLM request.

- [ ] **Step 4: Smoke-test import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel"`

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/base_agent.py
git commit -m "feat: _run_loop supports dormant-wake cycle"
```

---

### Task 4: Update system intrinsic — quell = DORMANT for all agents

**Files:**
- Modify: `lingtai-kernel/src/lingtai_kernel/intrinsics/system.py` — quell handler (~lines 225-251)

Changes:
1. Self-quell: remove karma requirement — any agent can sleep. Set `_dormant`, NOT `_shutdown`.
2. Quell-other: still requires karma. Writes `.quell` file (which heartbeat reads as DORMANT).
3. No code path should set `_shutdown` from quell — only `.suspend` and SIGINT do that.

- [ ] **Step 1: Rewrite self-quell**

Replace the self-quell block:

```python
if not address:
    # Self-quell — any agent can put itself to sleep
    from ..state import AgentState
    reason = args.get("reason", "")
    agent._log("self_quell", reason=reason)
    agent._set_state(AgentState.DORMANT, reason="self-quell")
    agent._dormant.set()
    agent._cancel_event.set()
    return {
        "status": "ok",
        "message": t(agent._config.language, "system_tool.quell_message"),
    }
```

- [ ] **Step 2: Verify quell-other still writes `.quell`**

The existing quell-other code writes `.quell` to the target's working dir, which is correct — the target's heartbeat will detect it and go DORMANT. No change needed for quell-other.

- [ ] **Step 3: Smoke-test import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "from lingtai_kernel.intrinsics import system"`

- [ ] **Step 4: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/intrinsics/system.py
git commit -m "feat: self-quell = DORMANT, no karma required"
```

---

### Task 5: Update CLI — only exit on SUSPENDED

**Files:**
- Modify: `lingtai/src/lingtai/cli.py` — `run()` function (~lines 185-215)

The CLI currently calls `agent._shutdown.wait()` and then `agent.stop()`. Now:
1. SIGINT/SIGTERM → write `.suspend`, set `_shutdown` (process exits)
2. The main thread blocks on `_shutdown.wait()` — this only fires for SUSPENDED now
3. `agent.stop()` only called on SUSPENDED

- [ ] **Step 1: Rewrite signal handlers and run loop**

```python
def run(working_dir: Path) -> None:
    """Full boot sequence: load, build, start, block, stop."""
    data = load_init(working_dir)
    agent = build_agent(data, working_dir)

    # Signal handlers: SIGTERM/SIGINT → suspend (full death)
    suspend_file = working_dir / ".suspend"

    def _signal_handler(signum, frame):
        suspend_file.touch()
        agent._shutdown.set()

    signal.signal(signal.SIGTERM, _signal_handler)
    signal.signal(signal.SIGINT, _signal_handler)

    try:
        agent.start()

        # Inject starting prompt if provided
        prompt = data.get("prompt", "")
        if prompt:
            from lingtai_kernel.message import _make_message, MSG_REQUEST
            agent.inbox.put(_make_message(MSG_REQUEST, "system", prompt))

        # Block until SUSPENDED (not DORMANT — dormant is handled inside _run_loop)
        agent._shutdown.wait()
    finally:
        try:
            agent.stop(timeout=10.0)
        except Exception:
            pass
```

- [ ] **Step 2: Update Agent.stop() to handle addon cleanup**

In `lingtai/src/lingtai/agent.py`, the `stop()` method already kills MCP clients and addon managers before calling `super().stop()`. This is correct for SUSPENDED — everything must die. No change needed since `stop()` is only called on SUSPENDED now.

- [ ] **Step 3: Smoke-test import**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && source venv/bin/activate && python -c "import lingtai"`

- [ ] **Step 4: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
git add src/lingtai/cli.py
git commit -m "feat: CLI only exits on SUSPENDED, SIGINT/SIGTERM write .suspend"
```

---

### Task 6: Update tests

**Files:**
- Modify: `lingtai-kernel/tests/` — state-related tests
- Modify: `lingtai/tests/test_cli.py` — CLI tests

- [ ] **Step 1: Test SUSPENDED state exists**

```python
def test_suspended_state():
    from lingtai_kernel.state import AgentState
    assert AgentState.SUSPENDED.value == "suspended"
```

- [ ] **Step 2: Test .quell → DORMANT (not SUSPENDED)**

Verify that when `.quell` is detected by heartbeat, agent transitions to DORMANT and `_dormant` is set but `_shutdown` is NOT set.

- [ ] **Step 3: Test .suspend → SUSPENDED**

Verify that when `.suspend` is detected by heartbeat, agent transitions to SUSPENDED and `_shutdown` IS set.

- [ ] **Step 4: Test dormant wake on inbox message**

Create agent, put it in DORMANT, put a message in inbox, verify it wakes to ACTIVE.

- [ ] **Step 5: Test self-quell without karma**

Verify any agent (even without `admin.karma`) can self-quell to DORMANT.

- [ ] **Step 6: Run full test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -q`
Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && source venv/bin/activate && python -m pytest tests/ -q`

- [ ] **Step 7: Commit**

```bash
# kernel tests
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add tests/
git commit -m "test: five-state lifecycle tests"

# lingtai tests
cd /Users/huangzesen/Documents/GitHub/lingtai
git add tests/
git commit -m "test: CLI suspend signal tests"
```

---

## Design Philosophy

**Soul flow is consciousness.** It cannot be selectively toggled — it is bound to the agent's state of being. While awake (ACTIVE/IDLE/STUCK), the inner voice is always present, just as a human cannot turn off their stream of consciousness. The only way to silence it is to fall asleep (DORMANT). Waking up means consciousness resumes. This is not a feature toggle — it's a state transition.

**DORMANT = nap with no alarm clock.** A napping agent pauses for N seconds and wakes on timer or mail. A dormant agent has no timer — it sleeps indefinitely until an external event (email, IMAP, telegram, inter-agent mail) wakes it. Both share the same body: heartbeat on, listeners on, LLM paused. The difference is only in what wakes them.

**SUSPENDED = 假死.** The body appears dead but can be revived. No process, no heartbeat, just the folder on disk. Only `lingtai run` brings it back.

## Implementation Notes

- **`_session.close()` in dormant**: The LLM session should be closed to free resources. On wake, the session reconnects lazily on the next `send()` call. Check if `SessionManager.close()` exists and is idempotent.
- **Soul flow**: Cancelled on dormant entry via `_cancel_soul_timer()`. On wake, the soul timer restarts naturally when the agent finishes processing the wake message and returns to IDLE (the existing `_schedule_soul_whisper` flow handles this).
- **Vigil reset on wake**: When waking from dormant, reset `_uptime_anchor` so the stamina timer starts fresh. A new day begins.
- **Chat history preserved**: `_persist_chat_history()` is called in `_run_loop` finally block, so context is saved before dormant. On wake, the existing context is still in memory (no reload needed). The agent wakes as itself.
- **Addons (IMAP/telegram)**: These are started in `Agent.__init__` or `start()`. Since we don't call `stop()` on DORMANT, they keep running. Their `on_message` callbacks put messages in `agent.inbox`, which wakes the dormant loop.
