# LoggingService Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a 6th optional service (LoggingService) to BaseAgent that writes structured JSONL event logs for debugging and real-time monitoring.

**Architecture:** `LoggingService` ABC in `services/logging.py` with `JSONLLoggingService` as first implementation. BaseAgent gets a `_log()` helper and 13 call sites covering lifecycle, tool dispatch, LLM usage, email, and errors.

**Tech Stack:** Python stdlib only (json, threading, abc, time). No new dependencies.

**Spec:** `docs/specs/2026-03-15-logging-service-design.md`

---

## Chunk 1: Service + Tests

### Task 1: LoggingService ABC + JSONLLoggingService

**Files:**
- Create: `src/stoai/services/logging.py`
- Test: `tests/test_services_logging.py`

- [ ] **Step 1: Write the service module**

```python
# src/stoai/services/logging.py
"""LoggingService — structured event logging backing agent observability.

First implementation: JSONLLoggingService (append JSON lines to a file).

Design principles:
- Single method: log(event) — thread-safe, fire-and-forget
- Persistent: events written to disk, not transient callbacks
- Swappable: Forum can subclass to build communication graphs in real-time
"""
from __future__ import annotations

import json
import threading
from abc import ABC, abstractmethod
from pathlib import Path


class LoggingService(ABC):
    """Abstract structured event logging service.

    Backs agent observability. Implementations provide the actual
    storage mechanism (JSONL file, database, network sink, etc.).
    """

    @abstractmethod
    def log(self, event: dict) -> None:
        """Log a structured event. Must be thread-safe."""

    def close(self) -> None:
        """Flush and release resources. Default no-op."""


class JSONLLoggingService(LoggingService):
    """Append structured events as JSON lines to a file.

    Thread-safe via lock. Flushes after every write for real-time tailing.
    """

    def __init__(self, path: Path | str) -> None:
        self._path = Path(path)
        self._path.parent.mkdir(parents=True, exist_ok=True)
        self._file = open(self._path, "a")
        self._lock = threading.Lock()
        self._closed = False

    def log(self, event: dict) -> None:
        if self._closed:
            return
        line = json.dumps(event, default=str)
        with self._lock:
            self._file.write(line + "\n")
            self._file.flush()

    def close(self) -> None:
        if not self._closed:
            self._closed = True
            self._file.close()
```

- [ ] **Step 2: Write failing tests**

```python
# tests/test_services_logging.py
"""Tests for stoai.services.logging."""
import json
import threading
from pathlib import Path

from stoai.services.logging import LoggingService, JSONLLoggingService


class TestJSONLLoggingService:

    def test_log_writes_jsonl(self, tmp_path):
        """Events are written as JSON lines."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.log({"type": "test", "value": 42})
        svc.log({"type": "test", "value": 99})
        svc.close()

        lines = log_file.read_text().strip().split("\n")
        assert len(lines) == 2
        assert json.loads(lines[0]) == {"type": "test", "value": 42}
        assert json.loads(lines[1]) == {"type": "test", "value": 99}

    def test_log_default_str_for_non_serializable(self, tmp_path):
        """Non-JSON-serializable values are converted via str()."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.log({"path": Path("/tmp/foo")})
        svc.close()

        line = json.loads(log_file.read_text().strip())
        assert line["path"] == "/tmp/foo"

    def test_close_is_idempotent(self, tmp_path):
        """Calling close() twice does not raise."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.close()
        svc.close()  # should not raise

    def test_log_after_close_is_noop(self, tmp_path):
        """Logging after close does not raise or write."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.close()
        svc.log({"type": "test"})  # should not raise
        assert log_file.read_text().strip() == ""

    def test_creates_parent_dirs(self, tmp_path):
        """Parent directories are created if they don't exist."""
        log_file = tmp_path / "nested" / "dir" / "test.jsonl"
        svc = JSONLLoggingService(log_file)
        svc.log({"type": "test"})
        svc.close()
        assert log_file.exists()

    def test_append_mode(self, tmp_path):
        """Opening an existing file appends, does not truncate."""
        log_file = tmp_path / "test.jsonl"
        log_file.write_text('{"existing": true}\n')

        svc = JSONLLoggingService(log_file)
        svc.log({"type": "new"})
        svc.close()

        lines = log_file.read_text().strip().split("\n")
        assert len(lines) == 2
        assert json.loads(lines[0])["existing"] is True
        assert json.loads(lines[1])["type"] == "new"

    def test_thread_safety(self, tmp_path):
        """Concurrent writes don't corrupt the file."""
        log_file = tmp_path / "test.jsonl"
        svc = JSONLLoggingService(log_file)

        def writer(thread_id):
            for i in range(50):
                svc.log({"thread": thread_id, "i": i})

        threads = [threading.Thread(target=writer, args=(t,)) for t in range(4)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()
        svc.close()

        lines = log_file.read_text().strip().split("\n")
        assert len(lines) == 200  # 4 threads * 50 writes
        # Every line must be valid JSON
        for line in lines:
            json.loads(line)

    def test_abc_cannot_instantiate(self):
        """LoggingService ABC cannot be instantiated directly."""
        try:
            LoggingService()
            assert False, "Should have raised TypeError"
        except TypeError:
            pass
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `source venv/bin/activate && python -m pytest tests/test_services_logging.py -v`
Expected: All 8 tests PASS

- [ ] **Step 4: Export from `__init__.py`**

Modify: `src/stoai/__init__.py`

Add after the SearchService import line:
```python
from .services.logging import LoggingService, JSONLLoggingService
```

Add to `__all__`:
```python
    "LoggingService",
    "JSONLLoggingService",
```

- [ ] **Step 5: Smoke-test import**

Run: `source venv/bin/activate && python -c "from stoai import LoggingService, JSONLLoggingService; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add src/stoai/services/logging.py tests/test_services_logging.py src/stoai/__init__.py
git commit -m "feat: add LoggingService ABC + JSONLLoggingService"
```

---

## Chunk 2: BaseAgent Integration

### Task 2: Add logging_service to BaseAgent constructor + _log() helper

**Files:**
- Modify: `src/stoai/agent.py`

- [ ] **Step 1: Add `logging_service` parameter to `__init__`**

In `src/stoai/agent.py`, add `logging_service` parameter after `streaming`:

```python
        cancel_event: threading.Event | None = None,
        streaming: bool = False,
        logging_service: Any | None = None,
    ):
```

And store it after `self._streaming = streaming`:

```python
        self._log_service = logging_service
```

- [ ] **Step 2: Add `_log()` helper method**

Add after the `_set_state()` method (around line 611), before the main loop section:

```python
    def _log(self, event_type: str, **fields) -> None:
        """Write a structured event to the logging service, if configured."""
        if self._log_service:
            self._log_service.log({
                "type": event_type,
                "agent_id": self.agent_id,
                "ts": time.time(),
                **fields,
            })
```

- [ ] **Step 3: Smoke-test**

Run: `source venv/bin/activate && python -c "import stoai" && python -m pytest tests/test_agent.py -q`
Expected: All agent tests pass, no import errors

- [ ] **Step 4: Commit**

```bash
git add src/stoai/agent.py
git commit -m "feat: add logging_service param and _log() helper to BaseAgent"
```

### Task 3: Wire 13 log call sites into BaseAgent

**Files:**
- Modify: `src/stoai/agent.py`

- [ ] **Step 1: `_set_state()` — log `agent_state`**

In `_set_state()`, after the `self._idle` logic (after `self._idle.clear()`), add:

```python
        self._log("agent_state", old=old.value, new=new_state.value, reason=reason)
```

- [ ] **Step 2: `stop()` — log `agent_stop` and call `close()`**

In `stop()`, add `agent_stop` log before `self._shutdown.set()`:

```python
        self._log("agent_stop")
```

After the email service stop block, add:

```python
        # Close LoggingService if configured
        if self._log_service is not None:
            try:
                self._log_service.close()
            except Exception:
                pass
```

- [ ] **Step 3: `_run_loop()` — log `error` on unhandled exception**

In `_run_loop()`, after the `logger.error(...)` line inside the exception handler, add:

```python
                self._log("error", source="message_handler", message=err_desc)
```

- [ ] **Step 4: `_execute_single_tool()` — log blocked path**

In `_execute_single_tool()`, inside the `if verdict.blocked:` block, before the `return msg, False, ""`, add:

```python
            self._log("tool_result", tool_name=tc.name, status="blocked", elapsed_ms=0)
```

- [ ] **Step 5: `_execute_single_tool()` — log `tool_call` before dispatch**

After the `timer = ToolTimer()` line, before the `try:`, add:

```python
        self._log("tool_call", tool_name=tc.name, tool_args=args)
```

- [ ] **Step 6: `_execute_single_tool()` — log `tool_result` after dispatch**

After `stamp_tool_result(result, timer.elapsed_ms)`, add:

```python
            status = result.get("status", "success") if isinstance(result, dict) else "success"
            self._log("tool_result", tool_name=tc.name, status=status, elapsed_ms=timer.elapsed_ms)
```

- [ ] **Step 7: `_execute_single_tool()` — log `error` on exception**

In the `except Exception as e:` block, after `collected_errors.append(...)`, add:

```python
            self._log("error", source=tc.name, message=str(e))
```

- [ ] **Step 8: `_llm_send()` — log `llm_call` before API call**

At the start of `_llm_send()`, after `self._check_and_compact()`, add:

```python
        self._log("llm_call", model=self._config.model or self.service.model or "unknown")
```

- [ ] **Step 9: `_track_usage()` — log `llm_response`**

At the end of `_track_usage()`, after the token state is updated (after `self._api_calls = token_state["api_calls"]`), add:

```python
        if response.usage:
            self._log(
                "llm_response",
                input_tokens=response.usage.input_tokens,
                output_tokens=response.usage.output_tokens,
                thinking_tokens=response.usage.thinking_tokens,
                cached_tokens=response.usage.cached_tokens,
            )
```

- [ ] **Step 10: `_on_reset()` — log `llm_reset`**

In `_on_reset()`, after creating the new chat session (after `interface=iface,` closing paren), add:

```python
        self._log("llm_reset", entries_kept=len(iface.entries))
```

- [ ] **Step 11: `_check_and_compact()` — log `compaction`**

In `_check_and_compact()`, replace the existing compaction block:

```python
        if new_chat is not None:
            self._chat = new_chat
            self._interaction_id = None
```

with:

```python
        if new_chat is not None:
            before_tokens = self._chat.interface.estimate_context_tokens()
            after_tokens = new_chat.interface.estimate_context_tokens()
            self._chat = new_chat
            self._interaction_id = None
            self._log("compaction", before_tokens=before_tokens, after_tokens=after_tokens)
```

- [ ] **Step 12: `_handle_email()` — log `email_sent`**

In `_handle_email()`, after `success = self._email_service.send(address, payload)`, before the if/else, add:

```python
        preview = message_text[:200] if message_text else ""
        status = "delivered" if success else "refused"
        self._log("email_sent", address=address, status=status, message_preview=preview)
```

Then update the return to use the already-computed status variable:

```python
        if success:
            return {"status": "delivered"}
        else:
            return {"status": "refused", "error": f"Could not deliver to {address}"}
```

- [ ] **Step 13: `_on_email_received()` — log `email_received`**

In `_on_email_received()`, after `self.inbox.put(msg)`, add:

```python
        preview = str(content)[:200] if content else ""
        self._log("email_received", sender=sender, message_preview=preview)
```

- [ ] **Step 14: Run all tests**

Run: `source venv/bin/activate && python -c "import stoai" && python -m pytest tests/ -q`
Expected: All tests pass

- [ ] **Step 15: Commit**

```bash
git add src/stoai/agent.py
git commit -m "feat: wire 13 log call sites into BaseAgent"
```

---

## Chunk 3: Integration Tests + Docs

### Task 4: BaseAgent integration tests

**Files:**
- Modify: `tests/test_services_logging.py`
- Modify: `tests/test_agent.py`

- [ ] **Step 1: Add BaseAgent + LoggingService integration test**

Append to `tests/test_services_logging.py`:

```python
from unittest.mock import MagicMock
from stoai import BaseAgent
from stoai.llm import ToolCall


def make_mock_service():
    svc = MagicMock()
    svc.model = "test-model"
    svc.make_tool_result.return_value = {"role": "tool", "content": "ok"}
    return svc


class TestBaseAgentLoggingIntegration:

    def test_tool_call_logged(self, tmp_path):
        """Executing a tool logs tool_call and tool_result events."""
        log_file = tmp_path / "agent.jsonl"
        log_svc = JSONLLoggingService(log_file)

        agent = BaseAgent(
            agent_id="test",
            service=make_mock_service(),
            logging_service=log_svc,
        )
        # Register a simple tool
        agent.add_tool("greet", {"type": "object", "properties": {}}, lambda args: {"status": "ok"})

        from stoai.loop_guard import LoopGuard
        guard = LoopGuard()
        errors = []
        tc = ToolCall(name="greet", args={})
        agent._execute_single_tool(tc, guard, errors)
        log_svc.close()

        lines = log_file.read_text().strip().split("\n")
        events = [json.loads(line) for line in lines]
        types = [e["type"] for e in events]
        assert "tool_call" in types
        assert "tool_result" in types
        # Verify agent_id is injected
        assert all(e["agent_id"] == "test" for e in events)
        # Verify ts is present
        assert all("ts" in e for e in events)

    def test_no_logging_service_is_noop(self, tmp_path):
        """Agent without logging_service does not raise on tool execution."""
        agent = BaseAgent(
            agent_id="test",
            service=make_mock_service(),
        )
        agent.add_tool("greet", {"type": "object", "properties": {}}, lambda args: {"status": "ok"})

        from stoai.loop_guard import LoopGuard
        guard = LoopGuard()
        errors = []
        tc = ToolCall(name="greet", args={})
        agent._execute_single_tool(tc, guard, errors)  # should not raise

    def test_state_change_logged(self, tmp_path):
        """State transitions are logged."""
        log_file = tmp_path / "agent.jsonl"
        log_svc = JSONLLoggingService(log_file)

        from stoai import AgentState
        agent = BaseAgent(
            agent_id="test",
            service=make_mock_service(),
            logging_service=log_svc,
        )
        agent._set_state(AgentState.ACTIVE, reason="test")
        log_svc.close()

        lines = log_file.read_text().strip().split("\n")
        events = [json.loads(line) for line in lines]
        state_events = [e for e in events if e["type"] == "agent_state"]
        assert len(state_events) >= 1
```

- [ ] **Step 2: Run all tests**

Run: `source venv/bin/activate && python -m pytest tests/ -v`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add tests/test_services_logging.py tests/test_agent.py
git commit -m "test: add BaseAgent + LoggingService integration tests"
```

### Task 5: Update docs

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/status.md`

- [ ] **Step 1: Update CLAUDE.md services table**

Add `LoggingService` row to the Five Services table:

```
| `LoggingService` | structured JSONL event logging | `JSONLLoggingService` |
```

Update the table header from "Five Services" to "Six Services".

Update the agent.py description to mention "6 optional services".

- [ ] **Step 2: Update docs/status.md**

Add under Services section:

```
- [x] `services/logging.py` — LoggingService ABC + JSONLLoggingService (wired into BaseAgent)
```

Update test count.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md docs/status.md
git commit -m "docs: update for LoggingService (6th service)"
```

- [ ] **Step 4: Final verification**

Run: `source venv/bin/activate && python -c "import stoai" && python -m pytest tests/ -q`
Expected: All tests pass, no import errors
