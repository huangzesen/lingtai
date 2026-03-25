# LLM Failure Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify LLM failure recovery into a simple AED process in the message loop: drop orphan tool call, rebuild session, retry up to 3 times, then ASLEEP. Remove all retry/reset logic from `llm_utils.py` and all AED logic from heartbeat.

**Architecture:** `_send_with_retry` becomes `_send` (single attempt). The message loop's exception handler runs the AED process: pop orphan tool call from ChatInterface, rebuild session, inject recovery message, retry. Heartbeat becomes purely mechanical — no STUCK detection, no AED triggering. Rename `cpr_timeout` → `aed_timeout`.

**Tech Stack:** Python, lingtai-kernel

**Spec:** `docs/superpowers/specs/2026-03-24-llm-failure-recovery-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `src/lingtai_kernel/llm_utils.py` | Modify | Simplify `_send_with_retry` → `_send` (single attempt, no retry) |
| `src/lingtai_kernel/session.py` | Modify | Remove `_on_reset`. `send()` drops `on_reset` callback. |
| `src/lingtai_kernel/base_agent.py` | Modify | AED loop in message loop exception handler. Remove AED from heartbeat. Rename `_cpr_start` → `_aed_start`, `_aed_pending` removed. `_perform_aed` removed. |
| `src/lingtai_kernel/config.py` | Modify | Rename `cpr_timeout` → `aed_timeout`, default 360. Add `max_aed_attempts`, default 3. |
| `src/lingtai_kernel/llm/interface.py` | Modify | Add `pop_orphan_tool_call()` — remove trailing assistant entry with unmatched tool calls |
| `src/lingtai_kernel/i18n/en.json` | Modify | Update `system.stuck_revive` message |
| `src/lingtai_kernel/i18n/zh.json` | Modify | Update `system.stuck_revive` message |
| `src/lingtai_kernel/i18n/wen.json` | Modify | Update `system.stuck_revive` message |
| `tests/test_session.py` | Modify | Remove `_on_reset` tests, update `send` tests |
| `tests/test_llm_utils.py` | Modify | Simplify retry tests |
| `tests/test_base_agent.py` | Modify | AED tests in message loop, heartbeat no longer does AED |

All files in `/Users/huangzesen/Documents/GitHub/lingtai-kernel/`.

---

### Task 1: Add `pop_orphan_tool_call()` to ChatInterface

**Files:**
- Modify: `src/lingtai_kernel/llm/interface.py`
- Test: `tests/test_interface.py` (or create if needed)

- [ ] **Step 1: Write failing tests**

```python
def test_pop_orphan_tool_call_removes_trailing_assistant_with_tool_calls():
    """Trailing assistant entry with ToolCallBlocks should be popped."""
    from lingtai_kernel.llm.interface import (
        ChatInterface, TextBlock, ToolCallBlock, ToolResultBlock,
    )
    iface = ChatInterface()
    iface.add_system("prompt")
    iface.add_user("hello")
    iface.add_assistant([TextBlock(text="Let me check."), ToolCallBlock(id="tc1", name="bash", args={"command": "ls"})])
    assert len(iface.entries) == 3  # system, user, assistant

    removed = iface.pop_orphan_tool_call()

    assert removed is True
    assert len(iface.entries) == 2  # system, user — assistant popped


def test_pop_orphan_tool_call_also_removes_trailing_tool_results():
    """If tool results follow the orphan assistant, pop both."""
    from lingtai_kernel.llm.interface import (
        ChatInterface, TextBlock, ToolCallBlock, ToolResultBlock,
    )
    iface = ChatInterface()
    iface.add_system("prompt")
    iface.add_user("hello")
    iface.add_assistant([ToolCallBlock(id="tc1", name="bash", args={"command": "ls"})])
    iface.add_tool_results([ToolResultBlock(tool_call_id="tc1", name="bash", content="file.txt")])
    assert len(iface.entries) == 4

    removed = iface.pop_orphan_tool_call()

    assert removed is True
    assert len(iface.entries) == 2  # system, user


def test_pop_orphan_tool_call_noop_when_clean():
    """No orphan — should not pop anything."""
    from lingtai_kernel.llm.interface import ChatInterface, TextBlock
    iface = ChatInterface()
    iface.add_system("prompt")
    iface.add_user("hello")
    iface.add_assistant([TextBlock(text="Hi there!")])

    removed = iface.pop_orphan_tool_call()

    assert removed is False
    assert len(iface.entries) == 3  # unchanged


def test_pop_orphan_tool_call_noop_on_empty():
    """Empty interface — should not crash."""
    from lingtai_kernel.llm.interface import ChatInterface
    iface = ChatInterface()

    removed = iface.pop_orphan_tool_call()

    assert removed is False
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_interface.py -k "pop_orphan" -v`
Expected: FAIL — `pop_orphan_tool_call` does not exist

- [ ] **Step 3: Implement `pop_orphan_tool_call`**

Add to `ChatInterface` class in `src/lingtai_kernel/llm/interface.py`:

```python
def pop_orphan_tool_call(self) -> bool:
    """Remove trailing orphan tool call (assistant with unmatched ToolCallBlocks).

    An orphan is a trailing assistant entry containing ToolCallBlocks that
    was never followed by a successful assistant response (i.e., it's the
    last entry, or only followed by tool results). This happens when an
    LLM call fails mid-tool-execution.

    Also removes any trailing tool result entries that belong to the orphan.

    Returns True if anything was removed, False if interface was clean.
    """
    if not self._entries:
        return False

    # First: pop trailing tool results (user entry with all ToolResultBlocks)
    dropped_results = False
    while self._entries and self._entries[-1].role == "user":
        from .interface import ToolResultBlock
        if all(isinstance(b, ToolResultBlock) for b in self._entries[-1].content):
            self._entries.pop()
            dropped_results = True
        else:
            break

    # Then: pop trailing assistant entry if it has ToolCallBlocks
    if self._entries and self._entries[-1].role == "assistant":
        from .interface import ToolCallBlock
        has_tool_calls = any(
            isinstance(b, ToolCallBlock) for b in self._entries[-1].content
        )
        if has_tool_calls:
            self._entries.pop()
            return True

    # If we only dropped results but no assistant, that's unexpected — put them back
    # Actually no, if results were orphaned without their assistant, still clean them.
    return dropped_results
```

Note: The imports are within the same module — use the block types directly. Check the actual import pattern in the file.

- [ ] **Step 4: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_interface.py -k "pop_orphan" -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/llm/interface.py tests/test_interface.py
git commit -m "feat: ChatInterface.pop_orphan_tool_call() — idempotent orphan cleanup"
```

---

### Task 2: Simplify `_send_with_retry` → `_send`

**Files:**
- Modify: `src/lingtai_kernel/llm_utils.py`
- Test: `tests/test_llm_utils.py`

- [ ] **Step 1: Read current tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && grep -n "def test_" tests/test_llm_utils.py | head -30`

Understand what existing tests cover so we know what to update.

- [ ] **Step 2: Replace `_send_with_retry` with `_send`**

The new `_send` function: submit the LLM call, wait up to `retry_timeout` (120s default), return result or raise on any error. No retries, no `on_reset` callback, no error classification for flow control.

```python
def _send(
    submit_fn,
    timeout_pool: ThreadPoolExecutor,
    retry_timeout: float,
    agent_name: str,
) -> LLMResponse:
    """Send a message to the LLM. Single attempt with timeout.

    Raises TimeoutError if the call doesn't complete within retry_timeout.
    Raises any LLM API error directly.
    """
    future: Future = submit_fn()
    t0 = time.monotonic()
    while True:
        elapsed = time.monotonic() - t0
        remaining = retry_timeout - elapsed
        if remaining <= 0:
            future.cancel()
            raise TimeoutError(f"LLM API call timed out after {elapsed:.0f}s")
        wait = min(_LLM_WARN_INTERVAL, remaining)
        try:
            result = future.result(timeout=wait)
            return result
        except TimeoutError:
            elapsed = time.monotonic() - t0
            if elapsed >= retry_timeout:
                future.cancel()
                raise TimeoutError(f"LLM API call timed out after {elapsed:.0f}s")
            _logger.warning(
                "[%s] LLM API not responding after %.0fs...",
                agent_name, elapsed,
            )
```

Update `send_with_timeout` and `send_with_timeout_stream` to call `_send` instead of `_send_with_retry`. Remove `on_reset`, `max_retries`, `reset_threshold` parameters from both. Keep `_SubmitFn` (still needed for the submit pattern).

Remove:
- `_send_with_retry` function
- `_LLM_MAX_RETRIES` constant
- `_SESSION_RESET_THRESHOLD` constant
- `_API_ERROR_RETRY_DELAYS` constant
- `_is_history_desync_error`, `_is_precondition_error`, `_is_bad_request_error` functions (or keep for logging only — check if used elsewhere)
- `_save_reset_snapshot` function (or keep if useful for debugging)

- [ ] **Step 3: Update callers**

In `src/lingtai_kernel/session.py`, update `send()` and `_send_streaming()` — remove `on_reset=self._on_reset` from `send_with_timeout` / `send_with_timeout_stream` calls. Remove `max_retries` and `reset_threshold` params if passed.

- [ ] **Step 4: Remove `_on_reset` from SessionManager**

Delete the `_on_reset` method entirely from `session.py`. The orphan cleanup is now done by `ChatInterface.pop_orphan_tool_call()` called from the message loop in `base_agent.py`.

- [ ] **Step 5: Update tests**

Update `tests/test_llm_utils.py` — remove tests for retry logic, `on_reset` callbacks, `_SESSION_RESET_THRESHOLD`, multi-attempt scenarios. Add/update tests for the simple `_send` behavior: success, timeout, API error.

Update `tests/test_session.py` — remove `_on_reset` tests (`test_on_reset_*`). Update `send` tests to not pass `on_reset`.

- [ ] **Step 6: Run all tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_llm_utils.py tests/test_session.py -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/llm_utils.py src/lingtai_kernel/session.py tests/
git commit -m "refactor: _send_with_retry → _send, single attempt, no internal retry"
```

---

### Task 3: AED loop in message loop exception handler

**Files:**
- Modify: `src/lingtai_kernel/base_agent.py:720-742`
- Modify: `src/lingtai_kernel/config.py`
- Modify: `src/lingtai_kernel/i18n/en.json`, `zh.json`, `wen.json`
- Test: `tests/test_base_agent.py`

- [ ] **Step 1: Update config — rename `cpr_timeout` → `aed_timeout`, add `max_aed_attempts`**

In `src/lingtai_kernel/config.py`:

```python
# Was:
# cpr_timeout: float = 1200.0  # 20 minutes — max CPR before pronouncing dead

# Now:
aed_timeout: float = 360.0   # 6 minutes — max AED retry window (3 attempts × 120s)
max_aed_attempts: int = 3     # max AED retry attempts before ASLEEP
```

Grep for all `cpr_timeout` references and rename to `aed_timeout`.

- [ ] **Step 2: Update i18n messages**

In `en.json`:
```json
"system.stuck_revive": "[system] LLM call failed at {ts}. You called: {tool_calls}. Retrying."
```

In `zh.json`:
```json
"system.stuck_revive": "[系统] LLM 调用于 {ts} 失败。你调用了：{tool_calls}。正在重试。"
```

In `wen.json`:
```json
"system.stuck_revive": "[系统] 灵识于 {ts} 失联。汝所调之器：{tool_calls}。正在重连。"
```

- [ ] **Step 3: Implement AED in message loop exception handler**

In `base_agent.py`, replace the exception handling in `_run_loop` (around lines 720-742):

```python
                sleep_state = AgentState.IDLE
                aed_attempts = 0
                while True:
                    try:
                        self._handle_message(msg)
                        break  # success — exit AED loop
                    except (TimeoutError, Exception) as e:
                        err_desc = str(e) or repr(e)
                        aed_attempts += 1

                        # Pop orphan tool call from interface (idempotent)
                        if self._session.chat is not None:
                            self._session.chat.interface.pop_orphan_tool_call()

                        if aed_attempts > self._config.max_aed_attempts:
                            # AED exhausted — go ASLEEP
                            logger.error(
                                f"[{self.agent_name}] AED exhausted after {aed_attempts - 1} attempts: {err_desc}",
                            )
                            self._log("aed_exhausted", attempts=aed_attempts - 1, error=err_desc)
                            sleep_state = AgentState.ASLEEP
                            self._asleep.set()
                            break

                        # Enter AED — STUCK state, rebuild session, retry
                        self._set_state(AgentState.STUCK, reason=f"AED attempt {aed_attempts}: {err_desc}")
                        self._log("aed_attempt", attempt=aed_attempts, error=err_desc)
                        logger.warning(
                            f"[{self.agent_name}] AED attempt {aed_attempts}/{self._config.max_aed_attempts}: {err_desc}",
                        )

                        # Rebuild session with current config, preserving history
                        if self._session.chat is not None:
                            self._session._rebuild_session(self._session.chat.interface)

                        # Summarize orphaned tool calls for the recovery message
                        # (already popped, so just note what was lost)
                        from datetime import datetime, timezone
                        ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
                        aed_msg = _t(self._config.language, "system.stuck_revive", ts=ts, tool_calls=err_desc)
                        msg = _make_message(MSG_REQUEST, "system", aed_msg)
                        # msg is now the new message for the next loop iteration

                if not self._asleep.is_set():
                    self._set_state(sleep_state)
                self._persist_chat_history()
```

- [ ] **Step 4: Remove AED from heartbeat**

In `_heartbeat_loop`, replace the STUCK block (lines 626-646):

```python
            # Was: STUCK detection + AED triggering + cpr_timeout
            # Now: heartbeat doesn't handle STUCK — AED is in message loop
            # Just reset aed tracking when not stuck
            if self._state != AgentState.STUCK:
                self._aed_start = None
```

Remove `_perform_aed` method entirely.
Remove `_aed_pending` field from `__init__`.
Rename `_cpr_start` → `_aed_start` (or just remove it — heartbeat no longer tracks STUCK duration).

- [ ] **Step 5: Write tests**

Test the AED loop in the message handler:

```python
def test_aed_retries_on_llm_failure():
    """AED should retry up to max_aed_attempts on LLM failure."""
    # Setup agent with mocked LLM that fails N times then succeeds
    ...

def test_aed_exhausted_goes_asleep():
    """After max_aed_attempts failures, agent should go ASLEEP."""
    ...

def test_aed_pops_orphan_tool_call():
    """AED should pop orphan tool call from interface on failure."""
    ...

def test_heartbeat_no_longer_triggers_aed():
    """Heartbeat should not call _perform_aed."""
    ...
```

The exact test implementation depends on how BaseAgent is constructed in tests. Check existing patterns in `tests/test_base_agent.py` and `tests/test_heartbeat.py`.

- [ ] **Step 6: Run full test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 7: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel"`
Expected: No errors

- [ ] **Step 8: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/base_agent.py src/lingtai_kernel/config.py src/lingtai_kernel/i18n/ tests/
git commit -m "refactor: AED loop in message handler, remove AED from heartbeat, rename cpr→aed"
```

---

### Task 4: Cleanup and verification

**Files:**
- Check: all files for stale references

- [ ] **Step 1: Grep for stale references**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
grep -rn "cpr_timeout\|_cpr_start\|_aed_pending\|_perform_aed\|_send_with_retry\|_on_reset\|on_reset\|_SESSION_RESET_THRESHOLD\|_LLM_MAX_RETRIES" --include="*.py" src/ tests/
```

Fix any remaining references.

- [ ] **Step 2: Run full kernel test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 3: Run lingtai test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS (lingtai depends on kernel — verify nothing broke)

- [ ] **Step 4: Smoke test both packages**

```bash
python -c "import lingtai_kernel; import lingtai; print('OK')"
```

- [ ] **Step 5: Commit any remaining fixes**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add -A && git commit -m "chore: cleanup stale AED/retry references"
```
