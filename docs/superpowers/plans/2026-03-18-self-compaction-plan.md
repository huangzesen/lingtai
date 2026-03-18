# Self-Compaction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace external-LLM compaction with agent self-compaction — the agent writes its own context summary, which gets injected into a fresh session. Automatic warnings at 80% context with forced forget after 5 ignored warnings. Agents are encouraged to save important findings to library before compacting.

**Architecture:** Two changes: (1) rewrite `anima._context_compact` so the agent's `prompt` IS the summary (no external LLM call), and (2) add a compaction pressure system in `BaseAgent._handle_request` that prepends `[system]` warnings to messages when context exceeds 80%, escalating from gentle reminders (1-2) to urgent warnings (3-4) to final warning (5) to auto-forget (6+).

**Tech Stack:** Python, existing stoai internals (SessionManager, AnimaManager, ChatInterface)

---

### Task 1: Rewrite `_context_compact` — agent's prompt IS the summary

**Files:**
- Modify: `src/stoai/capabilities/anima.py:504-561` (rewrite `_context_compact`)
- Modify: `src/stoai/capabilities/anima.py:110-116` (update `prompt` field description in SCHEMA)
- Modify: `src/stoai/capabilities/anima.py:144-147` (update DESCRIPTION for context section)
- Test: `tests/test_anima.py`

- [ ] **Step 1: Write the failing test**

```python
def test_context_compact_uses_agent_summary():
    """compact should wipe context and re-inject agent's prompt as summary."""
    agent = make_agent_with_anima()
    agent.start()
    # Simulate some conversation history
    agent._session.send("Hello, start working")
    before_tokens = agent._session._chat.interface.estimate_context_tokens()
    assert before_tokens > 0

    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "context",
        "action": "compact",
        "prompt": "Key findings: X=42, Y=17. Current task: analyze dataset Z.",
    })

    assert result["status"] == "ok"
    # Context should be much smaller (just the summary + system prompt)
    assert result["after_tokens"] < before_tokens
    # The summary should be in the new conversation
    iface = agent._session._chat.interface
    entries = [e for e in iface.entries if e.role == "user"]
    assert any("X=42" in str(e.content) for e in entries)
    agent.stop()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_anima.py::test_context_compact_uses_agent_summary -v`
Expected: FAIL (current implementation uses external LLM summarizer, not the prompt directly)

- [ ] **Step 3: Rewrite `_context_compact` in `anima.py`**

Replace lines 504-561 with:

```python
def _context_compact(self, args: dict) -> dict:
    """Agent self-compaction: prompt IS the summary, wipe + re-inject."""
    summary = args.get("prompt")
    if summary is None:
        return {"error": "prompt is required — write your context summary."}
    if not summary.strip():
        return {"error": "prompt cannot be empty — write what you need to remember."}

    if self._agent._chat is None:
        return {"error": "No active chat session to compact."}

    before_tokens = self._agent._chat.interface.estimate_context_tokens()

    # Wipe context and start fresh session
    self._agent._session._chat = None
    self._agent._session._interaction_id = None
    self._agent._session.ensure_session()

    # Inject the agent's summary as the opening context
    from ..llm.interface import TextBlock
    iface = self._agent._session._chat.interface
    iface.add_user_message(f"[Previous conversation summary]\n{summary}")
    iface.add_assistant_message(
        [TextBlock(text="Understood. I have my previous context restored.")],
    )

    after_tokens = iface.estimate_context_tokens()

    # Reset compaction warnings since agent just compacted
    if hasattr(self._agent._session, "_compaction_warnings"):
        self._agent._session._compaction_warnings = 0

    self._agent._log(
        "anima_compact",
        before_tokens=before_tokens,
        after_tokens=after_tokens,
    )

    return {
        "status": "ok",
        "before_tokens": before_tokens,
        "after_tokens": after_tokens,
    }
```

- [ ] **Step 4: Update SCHEMA prompt description**

In SCHEMA `"prompt"` field (line ~110-116), change to:

```python
"prompt": {
    "type": "string",
    "description": (
        "Your context summary — what you need to remember. "
        "Write everything important: current task, key findings, "
        "decisions made, data gathered, pending work. "
        "This replaces your entire conversation history. "
        "Required for context compact."
    ),
},
```

- [ ] **Step 5: Update DESCRIPTION for context section**

In DESCRIPTION (line ~144-147), change to:

```python
"context: compact to self-compact — write your own summary of what matters, "
"your conversation history is wiped and your summary becomes the new starting context. "
"forget to nuke conversation history completely (you lose everything). "
"Check usage via status show first.\n"
```

- [ ] **Step 6: Run test to verify it passes**

Run: `python -m pytest tests/test_anima.py::test_context_compact_uses_agent_summary -v`
Expected: PASS

- [ ] **Step 7: Smoke test**

Run: `python -c "import stoai"`
Expected: No errors

- [ ] **Step 8: Commit**

```bash
git add src/stoai/capabilities/anima.py tests/test_anima.py
git commit -m "feat: self-compaction — agent writes its own context summary"
```

---

### Task 2: Remove external-LLM auto-compaction from SessionManager

**Files:**
- Modify: `src/stoai/session.py:155` (remove `_check_and_compact()` call)
- Modify: `src/stoai/session.py:283-328` (remove or gut `_check_and_compact` method)

- [ ] **Step 1: Remove the auto-compact call from `send()`**

In `session.py` line 155, remove:
```python
self._check_and_compact()
```

- [ ] **Step 2: Replace `_check_and_compact` with a context pressure check**

Replace the `_check_and_compact` method (lines 283-328) with a method that just returns the pressure level:

```python
def get_context_pressure(self) -> float:
    """Return context usage as fraction (0.0 to 1.0). Returns 0.0 if unknown."""
    if self._chat is None:
        return 0.0
    ctx_window = self._chat.context_window()
    if ctx_window <= 0:
        return 0.0
    estimate = self._chat.interface.estimate_context_tokens()
    return estimate / ctx_window if estimate > 0 else 0.0
```

- [ ] **Step 3: Add compaction warning counter to SessionManager.__init__**

Add to `__init__`:
```python
self._compaction_warnings: int = 0
```

- [ ] **Step 4: Run existing tests**

Run: `python -m pytest tests/ -v`
Expected: All pass (the old auto-compact was transparent)

- [ ] **Step 5: Commit**

```bash
git add src/stoai/session.py
git commit -m "refactor: remove external-LLM auto-compaction from SessionManager"
```

---

### Task 3: Add compaction pressure system to `_handle_request`

**Files:**
- Modify: `src/stoai/base_agent.py:681-707` (add pressure check before LLM call)
- Test: `tests/test_agent.py` (or new `tests/test_compaction.py`)

- [ ] **Step 1: Write the test**

```python
def test_compaction_warning_injected_at_80_percent():
    """At 80%+ context, a [system] warning should be prepended to content."""
    agent = make_test_agent()
    agent.start()
    # Mock session to report 85% context pressure
    agent._session.get_context_pressure = lambda: 0.85
    agent._session._compaction_warnings = 0

    # Capture what gets sent to LLM
    sent_content = []
    original_send = agent._session.send
    def capture_send(content):
        sent_content.append(content)
        return original_send(content)
    agent._session.send = capture_send

    agent.send("do something")
    # ... process
    assert any("[system]" in c for c in sent_content)
    assert any("compact" in c.lower() for c in sent_content)
    agent.stop()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_compaction.py::test_compaction_warning_injected_at_80_percent -v`

- [ ] **Step 3: Add compaction pressure logic to `_handle_request`**

In `base_agent.py`, **replace** lines 701-704 of `_handle_request` (from `content = self._pre_request(msg)` through `response = self._session.send(content)`) with the following block. Note: the `[Current time:]` prefix and `session.send()` call are included at the end — do NOT keep the originals:

```python
content = self._pre_request(msg)
current_time = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

# Compaction pressure — warn agent when context is getting full
# Only if anima capability is registered (agent needs it to self-compact)
cap_managers = getattr(self, "_capability_managers", {})
has_anima = "anima" in cap_managers
pressure = self._session.get_context_pressure()
if pressure >= 0.8 and has_anima:
    self._session._compaction_warnings += 1
    warnings = self._session._compaction_warnings
    if warnings > 5:
        # Auto-forget — agent ignored 5 warnings
        self._log("auto_forget", reason="ignored 5 compaction warnings", pressure=pressure)
        anima = cap_managers.get("anima")
        if anima is not None:
            anima._context_forget({})
        else:
            self._session._chat = None
            self._session._interaction_id = None
            self._session.ensure_session()
        self._session._compaction_warnings = 0
        content = (
            f"[system] Your conversation history was wiped because you ignored "
            f"5 compaction warnings. Check your email inbox and library for context. "
            f"Start fresh.\n\n{content}"
        )
    elif warnings == 5:
        content = (
            f"[system] ⏳ FINAL WARNING — countdown 0. Your context is {pressure:.0%} full. "
            f"You MUST compact NOW or your entire conversation history will be wiped. "
            f"Save critical findings to library (anima submit), then "
            f"call anima(object=context, action=compact, prompt=<your summary>).\n\n{content}"
        )
    elif warnings >= 3:
        remaining = 5 - warnings
        content = (
            f"[system] ⏳ Context pressure: {pressure:.0%} full — "
            f"countdown {remaining} {'turn' if remaining == 1 else 'turns'} until auto-wipe. "
            f"Save important findings to your library NOW (anima submit), then compact. "
            f"Call anima(object=context, action=compact, prompt=<your summary>).\n\n{content}"
        )
    else:
        remaining = 5 - warnings
        content = (
            f"[system] ⏳ Context pressure: {pressure:.0%} full — "
            f"countdown {remaining} turns until auto-wipe. "
            f"Consider saving important data to your library (anima submit) "
            f"and compacting soon. "
            f"Call anima(object=context, action=compact, prompt=<your summary>) — "
            f"write a summary of everything important.\n\n{content}"
        )

content = f"[Current time: {current_time}]\n\n{content}"
response = self._session.send(content)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_compaction.py -v`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `python -m pytest tests/ -v`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add src/stoai/base_agent.py tests/test_compaction.py
git commit -m "feat: compaction pressure warnings — 2 warnings then auto-forget"
```

---

### Task 4: Reset warning counter on successful compact

**Files:**
- Modify: `src/stoai/capabilities/anima.py` (already done in Task 1 — verify `_compaction_warnings = 0`)
- Test: verify in existing test

- [ ] **Step 1: Write integration test**

```python
def test_compaction_resets_warning_counter():
    """After successful compact, warning counter should reset to 0."""
    agent = make_test_agent_with_anima()
    agent.start()
    agent._session._compaction_warnings = 2  # simulate 2 warnings

    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "context",
        "action": "compact",
        "prompt": "My important context summary.",
    })

    assert result["status"] == "ok"
    assert agent._session._compaction_warnings == 0
    agent.stop()
```

- [ ] **Step 2: Run test**

Run: `python -m pytest tests/test_compaction.py::test_compaction_resets_warning_counter -v`
Expected: PASS (already implemented in Task 1)

- [ ] **Step 3: Commit if any changes needed**

---

### Task 5: Update covenant to encourage proactive compaction

**Files:**
- Modify: `app/web/examples/orchestrator.py` (add compaction guidance to COVENANT)

- [ ] **Step 1: Add compaction guidance to COVENANT**

In `orchestrator.py`, add to COVENANT:

```python
COVENANT = """\
### Communication
- Your text responses are your PRIVATE DIARY — nobody can see them. NEVER reply to anyone via text. ALL communication MUST go through email. If you want someone to read something, email them.
- Addresses are ip:port format.
- Email history is your long-term memory.
- Always report results back to whoever asked.
- When emailing a peer, give enough context.

### Context Management
- Check your context usage periodically (status show).
- When context exceeds 60%, proactively compact: save important findings to your library first (anima submit), then write a thorough summary and call anima(object=context, action=compact, prompt=<summary>).
- Your library persists across compactions — anything saved there is safe. Use it to deposit important data, findings, and decisions before compacting.
- If you receive a [system] compaction warning, you have a 5-turn countdown before your history is wiped. Don't panic — use the turns to save to library, then compact.
"""
```

- [ ] **Step 2: Commit**

```bash
git add app/web/examples/orchestrator.py
git commit -m "docs: add compaction guidance to orchestrator covenant"
```

---

### Task 6: Clean up — remove unused COMPACTION_PROMPT

**Files:**
- Modify: `src/stoai/llm/service.py` (remove `COMPACTION_PROMPT` constant if no longer used)
- Verify: grep for remaining references

- [ ] **Step 1: Check for remaining references**

Run: `grep -r "COMPACTION_PROMPT" src/stoai/`
If only `llm/service.py` defines it and `session.py` imported it (now removed), delete it.

- [ ] **Step 2: Remove if unused**

- [ ] **Step 3: Smoke test**

Run: `python -c "import stoai"`

- [ ] **Step 4: Run all tests**

Run: `python -m pytest tests/ -v`

- [ ] **Step 5: Commit**

```bash
git add src/stoai/llm/service.py src/stoai/session.py
git commit -m "refactor: remove unused COMPACTION_PROMPT (self-compaction replaces it)"
```
