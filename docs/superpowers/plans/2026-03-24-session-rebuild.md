# Session Rebuild: Always Use Current Config

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every session creation/restoration path must use the current system prompt and tool schemas — never stale values from persisted history.

**Architecture:** Extract the common "create session with current config" pattern (already used by `_on_reset`) into a `SessionManager._rebuild_session(interface, tracked=True)` method. All three session paths (`restore_chat`, `_perform_refresh`, `_on_reset`) use it. Remove `resume_session()` from the LLM protocol entirely.

**Tech Stack:** Python, lingtai-kernel, lingtai

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `lingtai-kernel: src/lingtai_kernel/session.py` | Modify | Add `_rebuild_session()`, rewrite `restore_chat()`, refactor `_on_reset()` |
| `lingtai-kernel: src/lingtai_kernel/llm/service.py` | Modify | Remove `resume_session()` from ABC |
| `lingtai-kernel: src/lingtai_kernel/base_agent.py` | Modify | Fix `_perform_refresh()` to preserve history |
| `lingtai: src/lingtai/llm/service.py` | Modify | Remove `resume_session()` implementation |
| `lingtai-kernel: tests/test_session.py` | Modify | Update `restore_chat` and `_on_reset` tests, add `_rebuild_session` tests |
| `lingtai: tests/test_session.py` | Modify | Mirror kernel test updates (has same `resume_session` mocks) |

---

### Task 1: Add `_rebuild_session()` to SessionManager

**Files:**
- Modify: `lingtai-kernel: src/lingtai_kernel/session.py`
- Test: `lingtai-kernel: tests/test_session.py`

- [ ] **Step 1: Write failing test for `_rebuild_session`**

```python
def test_rebuild_session_uses_current_prompt_and_tools():
    """_rebuild_session should call create_session with current config + old interface."""
    sm, svc, _ = make_session_manager()
    mock_interface = MagicMock()
    mock_rebuilt = MagicMock()
    svc.create_session.return_value = mock_rebuilt

    sm._rebuild_session(mock_interface)

    call_kw = svc.create_session.call_args.kwargs
    assert call_kw["system_prompt"] == "test prompt"
    assert call_kw["tools"] == []
    assert call_kw["model"] == "test-model"
    assert call_kw["thinking"] == "high"
    assert call_kw["tracked"] is True
    assert call_kw["agent_type"] == "test"
    assert call_kw["provider"] is None
    assert call_kw["interface"] is mock_interface
    assert sm.chat is mock_rebuilt


def test_rebuild_session_tracked_false():
    """_rebuild_session(tracked=False) should pass tracked=False to create_session."""
    sm, svc, _ = make_session_manager()
    mock_interface = MagicMock()
    svc.create_session.return_value = MagicMock()

    sm._rebuild_session(mock_interface, tracked=False)

    call_kw = svc.create_session.call_args.kwargs
    assert call_kw["tracked"] is False
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_session.py::test_rebuild_session_uses_current_prompt_and_tools tests/test_session.py::test_rebuild_session_tracked_false -v`
Expected: FAIL — `_rebuild_session` does not exist

- [ ] **Step 3: Implement `_rebuild_session`**

In `lingtai-kernel: src/lingtai_kernel/session.py`, add this method to `SessionManager` (after `ensure_session`):

```python
def _rebuild_session(
    self, interface: "ChatInterface", tracked: bool = True,
) -> None:
    """Create a new chat session with current config, preserving history.

    Uses the current system prompt and tool schemas (not the stale values
    stored in the interface) so the session always reflects the agent's
    live configuration.

    Args:
        interface: Conversation history to preserve.
        tracked: Whether to register the session in LLMService._sessions.
            Use False for transient replacements (e.g. error recovery)
            where the original tracked session should remain the only
            registered one.
    """
    self._chat = self._llm_service.create_session(
        system_prompt=self._build_system_prompt_fn(),
        tools=self._build_tool_schemas_fn() or None,
        model=self._config.model or self._llm_service.model,
        thinking="high",
        agent_type=self._display_name,
        tracked=tracked,
        provider=self._config.provider,
        interface=interface,
    )
```

Note: `interaction_id` is intentionally omitted — a rebuilt session starts a new interaction with the provider. The old interaction ID from the saved session is stale and would cause errors.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_session.py::test_rebuild_session_uses_current_prompt_and_tools tests/test_session.py::test_rebuild_session_tracked_false -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/session.py tests/test_session.py
git commit -m "feat: add SessionManager._rebuild_session() — current config + old history"
```

---

### Task 2: Rewrite `restore_chat` to use `_rebuild_session`

**Files:**
- Modify: `lingtai-kernel: src/lingtai_kernel/session.py:375-387`
- Test: `lingtai-kernel: tests/test_session.py`

- [ ] **Step 1: Write failing test**

```python
def test_restore_chat_uses_current_config():
    """restore_chat should rebuild with current prompt+tools, not stale saved ones."""
    sm, svc, _ = make_session_manager()
    mock_rebuilt = MagicMock()
    svc.create_session.return_value = mock_rebuilt

    saved_state = {"messages": [
        {"id": 0, "role": "system", "system": "OLD stale prompt", "timestamp": 0.0,
         "tools": [{"name": "old_tool", "description": "gone", "parameters": {}}]},
        {"id": 1, "role": "user", "content": [{"type": "text", "text": "hello"}], "timestamp": 1.0},
    ]}
    sm.restore_chat(saved_state)

    # Must call create_session with CURRENT prompt, not "OLD stale prompt"
    call_kw = svc.create_session.call_args.kwargs
    assert call_kw["system_prompt"] == "test prompt"
    # Interface must carry the old history
    assert call_kw["interface"] is not None
    assert len(call_kw["interface"].entries) == 2  # system + user entries from saved state
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_session.py::test_restore_chat_uses_current_config -v`
Expected: FAIL — `restore_chat` still calls `resume_session`

- [ ] **Step 3: Rewrite `restore_chat`**

Replace `restore_chat` in `session.py`:

```python
def restore_chat(self, state: dict) -> None:
    """Restore chat history with current system prompt and tools.

    Deserializes the saved conversation history, then creates a new
    session using the current agent configuration (system prompt, tools)
    rather than the stale values from the persisted interface.
    """
    from .llm.interface import ChatInterface

    messages = state.get("messages")
    if messages:
        try:
            interface = ChatInterface.from_dict(messages)
            self._rebuild_session(interface)
            return
        except Exception as e:
            logger.warning(
                f"[{self._display_name}] Failed to restore chat: {e}. Starting fresh.",
                exc_info=True,
            )
    self.ensure_session()
```

- [ ] **Step 4: Fix existing tests that mock `resume_session`**

Update `test_restore_chat_with_state`:

```python
def test_restore_chat_with_state():
    sm, svc, _ = make_session_manager()
    mock_rebuilt = MagicMock()
    svc.create_session.return_value = mock_rebuilt
    sm.restore_chat({"messages": [
        {"id": 0, "role": "system", "system": "old prompt", "timestamp": 0.0},
        {"id": 1, "role": "user", "content": [{"type": "text", "text": "hi"}], "timestamp": 1.0},
    ]})
    assert sm.chat is mock_rebuilt
    # Should use create_session with interface, not resume_session
    call_kw = svc.create_session.call_args.kwargs
    assert call_kw["interface"] is not None
```

Update `test_restore_chat_fallback_on_error`:

```python
def test_restore_chat_fallback_on_error():
    sm, svc, _ = make_session_manager()
    # First create_session call (rebuild with interface) fails,
    # second (ensure_session fallback) succeeds
    call_count = [0]
    def side_effect(*args, **kwargs):
        call_count[0] += 1
        if call_count[0] == 1 and kwargs.get("interface") is not None:
            raise ValueError("bad state")
        return MagicMock()
    svc.create_session.side_effect = side_effect
    sm.restore_chat({"messages": [
        {"id": 0, "role": "system", "system": "prompt", "timestamp": 0.0},
        {"id": 1, "role": "user", "content": [{"type": "text", "text": "hi"}], "timestamp": 1.0},
    ]})
    assert sm.chat is not None
    assert svc.create_session.call_count == 2
```

- [ ] **Step 5: Run all session tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_session.py -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/session.py tests/test_session.py
git commit -m "refactor: restore_chat uses _rebuild_session — always current config"
```

---

### Task 3: Refactor `_on_reset` to use `_rebuild_session`

**Files:**
- Modify: `lingtai-kernel: src/lingtai_kernel/session.py:234-277`
- Test: `lingtai-kernel: tests/test_session.py`

- [ ] **Step 1: Run existing tests as baseline**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_session.py -v`
Expected: ALL PASS

- [ ] **Step 2: Refactor `_on_reset`**

In `_on_reset`, replace the inline `create_session` call (lines 259-267):

```python
        # Was:
        # self._chat = self._llm_service.create_session(
        #     system_prompt=self._build_system_prompt_fn(),
        #     tools=self._build_tool_schemas_fn() or None,
        #     model=self._config.model or self._llm_service.model,
        #     thinking="high",
        #     tracked=False,
        #     provider=self._config.provider,
        #     interface=iface,
        # )

        # Now:
        self._rebuild_session(iface, tracked=False)
```

`tracked=False` preserves the original `_on_reset` behavior: error-recovery sessions are transient and should not be registered in `LLMService._sessions` (avoids leaking stale sessions on repeated errors).

- [ ] **Step 3: Update `test_on_reset_passes_interface_to_new_session`**

The existing test asserts `tracked is False` — this still holds because we pass `tracked=False`. But verify the test still passes, and update it to also verify the current system prompt is used:

```python
def test_on_reset_passes_interface_to_new_session():
    sm, svc, _ = make_session_manager()
    sm.ensure_session()

    mock_chat = MagicMock()
    mock_iface = MagicMock()
    mock_iface.last_assistant_entry.return_value = None
    mock_iface.entries = []
    mock_chat.interface = mock_iface

    svc.create_session.return_value = MagicMock()
    sm._on_reset(mock_chat, "failed")

    # The new session should receive the interface for history continuity
    call_kwargs = svc.create_session.call_args.kwargs
    assert call_kwargs["interface"] is mock_iface
    assert call_kwargs["tracked"] is False
    assert call_kwargs["system_prompt"] == "test prompt"  # current config, not stale
```

- [ ] **Step 4: Run all session tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/test_session.py -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/session.py tests/test_session.py
git commit -m "refactor: _on_reset uses _rebuild_session(tracked=False)"
```

---

### Task 4: Fix `_perform_refresh` to preserve history

**Files:**
- Modify: `lingtai-kernel: src/lingtai_kernel/base_agent.py:752-786`
- Test: `lingtai-kernel: tests/test_heartbeat.py` (check for existing refresh tests)

- [ ] **Step 1: Check for existing refresh tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && grep -n "perform_refresh\|refresh" tests/test_heartbeat.py tests/test_agent.py 2>/dev/null | head -20`

Understand what test infrastructure exists for constructing a BaseAgent with mocked services.

- [ ] **Step 2: Write failing test**

The exact test depends on existing helpers. The test should verify:

```python
def test_perform_refresh_preserves_chat_history():
    """_perform_refresh should keep conversation history while reloading tools."""
    agent = _make_agent()  # use existing test helper
    # Give the agent a live session with an interface
    mock_interface = MagicMock()
    agent._session.chat = MagicMock()
    agent._session.chat.interface = mock_interface

    agent._perform_refresh()

    # Session should be rebuilt (not None)
    assert agent._session.chat is not None
    # create_session should have been called with the old interface
    call_kw = agent._session._llm_service.create_session.call_args.kwargs
    assert call_kw["interface"] is mock_interface
```

If `_make_agent` doesn't exist, construct a `BaseAgent` with mocked `LLMService`, set `_sealed = True` so refresh can unseal, and mock `_load_mcp_from_workdir`.

- [ ] **Step 3: Fix `_perform_refresh`**

In `base_agent.py`, replace line 782-783:

```python
        # Was:
        # Reset session so next message creates fresh one with new tools
        # self._session.chat = None

        # Now — rebuild session with current config, preserving chat history
        if self._session.chat is not None:
            self._session._rebuild_session(self._session.chat.interface)
        # If no session exists yet, ensure_session() will create one on next message
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/base_agent.py tests/
git commit -m "fix: _perform_refresh preserves chat history via _rebuild_session"
```

---

### Task 5: Remove `resume_session` from kernel ABC

**Files:**
- Modify: `lingtai-kernel: src/lingtai_kernel/llm/service.py:47-51`

- [ ] **Step 1: Remove from ABC**

In `lingtai-kernel: src/lingtai_kernel/llm/service.py`, delete:

```python
    @abstractmethod
    def resume_session(
        self, saved_state: dict, *, thinking: str = "high"
    ) -> "ChatSession":
        """Restore a session from saved state."""
```

- [ ] **Step 2: Grep for any remaining references in kernel**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && grep -rn "resume_session" --include="*.py" src/ tests/`
Expected: No hits

- [ ] **Step 3: Run full kernel test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 4: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -c "import lingtai_kernel"`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai-kernel
git add src/lingtai_kernel/llm/service.py
git commit -m "refactor: remove resume_session from LLMService ABC — replaced by create_session(interface=)"
```

---

### Task 6: Remove `resume_session` from lingtai LLMService + fix lingtai tests

**Files:**
- Modify: `lingtai: src/lingtai/llm/service.py:224-252`
- Modify: `lingtai: tests/test_session.py` (has same `resume_session` mocks as kernel)

- [ ] **Step 1: Delete `resume_session` method**

In `lingtai: src/lingtai/llm/service.py`, delete the entire `resume_session` method (lines 224-252).

- [ ] **Step 2: Update lingtai `tests/test_session.py`**

Apply the same test fixes as Task 2 Step 4 — update `test_restore_chat_with_state` and `test_restore_chat_fallback_on_error` to not mock `resume_session`.

- [ ] **Step 3: Grep for remaining references**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && grep -rn "resume_session" --include="*.py" src/ tests/`
Expected: No hits

- [ ] **Step 4: Run full lingtai test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 5: Smoke test**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -c "import lingtai"`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
git add src/lingtai/llm/service.py tests/test_session.py
git commit -m "refactor: remove resume_session — restore_chat now uses create_session(interface=)"
```

---

### Task 7: Update CLAUDE.md and spec doc

**Files:**
- Modify: `lingtai: docs/specs/2026-03-22-agent-isolation-design.md`
- Modify: `lingtai: CLAUDE.md` (if it references `resume_session`)

- [ ] **Step 1: Update spec doc**

In the agent-isolation design spec, update references to `resume_session` to reflect the new pattern. The LLMService protocol should list: `create_session()` (with `interface=` for restoration), `generate()`, and `make_tool_result()`.

- [ ] **Step 2: Check and update CLAUDE.md**

Run: `grep -n "resume_session" /Users/huangzesen/Documents/GitHub/lingtai/CLAUDE.md`
Update any references. The protocol description should reflect that `resume_session` no longer exists and `create_session(interface=)` is used for session restoration.

- [ ] **Step 3: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
git add docs/ CLAUDE.md
git commit -m "docs: update spec and CLAUDE.md — resume_session removed"
```

---

### Task 8: End-to-end verification

- [ ] **Step 1: Run full kernel test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai-kernel && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 2: Run full lingtai test suite**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai && python -m pytest tests/ -v`
Expected: ALL PASS

- [ ] **Step 3: Live test — tool changes across restarts**

Launch the telegram test agent, stop it, add capabilities, relaunch — verify the agent sees the new tools while preserving history:

```bash
cd /Users/huangzesen/Documents/GitHub/lingtai
source venv/bin/activate
# 1. Run telegram_chat.py WITHOUT file/bash/vision capabilities, chat briefly, stop
# 2. Re-run WITH capabilities=["file", "bash", ...], verify agent:
#    - Remembers previous messages
#    - Knows about new tools (ask it to run a shell command or read a file)
```

- [ ] **Step 4: Live test — refresh preserves history**

In the live agent, trigger a `system(action="refresh")` via Telegram and verify the agent retains conversation context after refresh.
