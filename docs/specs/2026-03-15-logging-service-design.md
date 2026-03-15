# LoggingService — Structured JSONL Logging for BaseAgent

**Date:** 2026-03-15
**Status:** Draft

## Problem

After removing the xhelio-specific event system (`on_event` callbacks, `EVENT_*` constants), BaseAgent has no structured observability. Python's `logging` module provides text logs for humans, but upper-layer apps like Forum need machine-readable, structured event streams to build communication graphs and monitor agent activity in real-time.

## Design

### LoggingService ABC

A 6th optional service injected into BaseAgent, following the same pattern as FileIOService, EmailService, VisionService, SearchService.

```python
# services/logging.py

class LoggingService(ABC):
    @abstractmethod
    def log(self, event: dict) -> None:
        """Log a structured event. Must be thread-safe."""

    def close(self) -> None:
        """Flush and close. Default no-op."""
```

### JSONLLoggingService

First implementation — appends JSON lines to a file.

```python
class JSONLLoggingService(LoggingService):
    def __init__(self, path: Path):
        self._file = open(path, "a")
        self._lock = threading.Lock()

    def log(self, event: dict) -> None:
        with self._lock:
            self._file.write(json.dumps(event, default=str) + "\n")
            self._file.flush()

    def close(self) -> None:
        self._file.close()
```

- `default=str` handles datetime, Path, and other non-serializable types gracefully.
- `flush()` after every write ensures events are visible immediately (for real-time tailing).
- Thread-safe via lock (BaseAgent dispatches tools in parallel).

### Event Envelope

Every event is a dict with a common envelope:

```json
{"type": "tool_call", "agent_id": "researcher", "ts": 1710504000.123, ...fields}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | str | Event type identifier |
| `agent_id` | str | Agent that produced the event |
| `ts` | float | Unix timestamp (`time.time()`) |

### Event Types

| Type | When emitted | Additional fields |
|------|-------------|-------------------|
| `agent_state` | `_set_state()` | `old`, `new`, `reason` |
| `agent_stop` | `stop()` — clean shutdown | *(none beyond envelope)* |
| `tool_call` | `_execute_single_tool()` — before dispatch | `tool_name`, `tool_args` |
| `tool_result` | `_execute_single_tool()` — after dispatch (including blocked path) | `tool_name`, `status`, `elapsed_ms` |
| `llm_call` | `_llm_send()` — before LLM API call | `model` |
| `llm_response` | `_track_usage()` — after response | `input_tokens`, `output_tokens`, `thinking_tokens`, `cached_tokens` |
| `llm_reset` | `_on_reset()` — session rollback after provider error | `entries_kept` |
| `compaction` | `_check_and_compact()` — after compaction | `before_tokens`, `after_tokens` |
| `email_sent` | `_handle_email()` — after send attempt | `address`, `status`, `message_preview` |
| `email_received` | `_on_email_received()` — on arrival | `sender`, `message_preview` |
| `error` | `_execute_single_tool()` exception + `_run_loop` unhandled | `source`, `message` |

`message_preview` fields are truncated to 200 characters.

Notes on specific events:
- **`tool_result` with `status="blocked"`**: When the loop guard blocks a duplicate tool call, a `tool_result` is still emitted with `status="blocked"` and `elapsed_ms=0`. No `tool_call` is emitted for blocked calls (they were never dispatched).
- **`email_sent`**: Fires for all outcomes — `status` field is `"delivered"` or `"refused"`. Consumers can filter.
- **`llm_call` model field**: Falls back to `"unknown"` if both `self._config.model` and `self.service.model` are None.
- **`compaction` token fields**: `before_tokens` and `after_tokens` are computed by calling `self._chat.interface.estimate_context_tokens()` before and after the compaction call. These calls are added to `_check_and_compact()`.
- **`agent_stop`**: Emitted in `stop()` before shutdown, so Forum can distinguish "agent idle" from "agent permanently stopped".

### BaseAgent Integration

**Constructor:**

```python
def __init__(self, ..., logging_service: LoggingService | None = None):
    self._log_service = logging_service
```

**Lifecycle wiring in `stop()`:**

`BaseAgent.stop()` calls `self._log_service.close()` after the agent thread has joined, ensuring all events are flushed.

**Helper method:**

```python
def _log(self, event_type: str, **fields) -> None:
    if self._log_service:
        self._log_service.log({
            "type": event_type,
            "agent_id": self.agent_id,
            "ts": time.time(),
            **fields,
        })
```

**Call sites (13 total):**

| Method | Event | Fields |
|--------|-------|--------|
| `_set_state()` | `agent_state` | `old=old.value, new=new_state.value, reason=reason` |
| `stop()` | `agent_stop` | *(envelope only)* |
| `_execute_single_tool()` blocked path | `tool_result` | `tool_name=tc.name, status="blocked", elapsed_ms=0` |
| `_execute_single_tool()` before dispatch | `tool_call` | `tool_name=tc.name, tool_args=args` |
| `_execute_single_tool()` after dispatch | `tool_result` | `tool_name=tc.name, status=..., elapsed_ms=timer.elapsed_ms` |
| `_execute_single_tool()` exception | `error` | `source=tc.name, message=str(e)` |
| `_run_loop()` unhandled exception | `error` | `source="message_handler", message=str(e)` |
| `_llm_send()` before call | `llm_call` | `model=self._config.model or self.service.model or "unknown"` |
| `_track_usage()` | `llm_response` | `input_tokens, output_tokens, thinking_tokens, cached_tokens` |
| `_on_reset()` | `llm_reset` | `entries_kept=len(iface.entries)` |
| `_check_and_compact()` | `compaction` | `before_tokens, after_tokens` |
| `_handle_email()` | `email_sent` | `address, status, message_preview` |
| `_on_email_received()` | `email_received` | `sender, message_preview` |

### What This Does NOT Cover

- **LLM retry/timeout details** — stay in Python `logging` module (`_logger.warning()` calls in `llm_utils.py`). The JSONL captures high-level `llm_call`/`llm_response`; the text log has retry-level details.
- **Streaming text deltas** — UI concern, not a debugging/monitoring event.
- **Thinking content** — UI concern.
- **Commentary text** — UI concern.

### Differences From Old Event System

| Old (`on_event`) | New (`LoggingService`) |
|------------------|----------------------|
| Raw callback `Callable[[str, dict], None]` | Proper ABC with contract |
| Transient — events lost if no listener | Persistent — written to JSONL |
| 10 event types, mixed UI + framework | 11 event types, debugging + monitoring only |
| Duplicate constants in `types.py` + `llm_utils.py` | No constants — event types are string literals at call sites |
| `on_event` threaded through `llm_utils` params | `llm_utils` untouched — only BaseAgent has the service |
| Single callback, no multiple subscribers | Forum can subclass `LoggingService` to both log AND build graph |

### Public API Surface

Exported from `stoai`:

- `LoggingService` (ABC)
- `JSONLLoggingService` (first implementation)

### Testing

**File:** `tests/test_services_logging.py`

1. **JSONLLoggingService unit tests**
   - Write events, read back JSONL, verify structure
   - Verify `close()` flushes and closes
   - Verify thread safety (concurrent writes don't corrupt)

2. **BaseAgent integration**
   - Construct agent with `JSONLLoggingService`
   - Execute a tool via `_execute_single_tool`
   - Verify JSONL contains `tool_call` + `tool_result` entries

3. **No-service is no-op**
   - Construct agent without `logging_service`
   - Execute tools — no errors raised

### Files Changed

| File | Change |
|------|--------|
| `services/logging.py` | **New** — `LoggingService` ABC + `JSONLLoggingService` |
| `agent.py` | Add `logging_service` param, `_log()` helper, 13 call sites, `close()` in `stop()` |
| `__init__.py` | Export `LoggingService`, `JSONLLoggingService` |
| `tests/test_services_logging.py` | **New** — unit + integration tests |
| `CLAUDE.md` | Update services table (6 services) |
| `docs/status.md` | Add logging service entry |
