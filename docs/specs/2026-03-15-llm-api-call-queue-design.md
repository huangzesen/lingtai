# Adapter-Level API Call Gate — Design Spec

> **Goal:** Rate-limit LLM API calls per adapter using a gate + thread pool pattern, so many agents sharing one adapter respect provider quotas. Transparent to all upper layers — queue wait is indistinguishable from network latency.

## Problem

Current state: agents call `ChatSession.send()` directly. No rate limiting except MiniMax's adapter-level `sleep()`. With many agents and sessions sharing one provider API quota, this causes:

- 429 rate-limit errors under load
- No fair scheduling across agents
- MiniMax's `sleep()` blocks the calling thread (doesn't scale)

## Design

### Core Insight

Rate limiting is a **transport concern**, not an orchestration concern. The adapter knows its provider's constraints. Upper layers (`LLMService`, `BaseAgent`, `llm_utils.py`) don't change at all — queue wait looks like normal API latency to them, and the existing `send_with_timeout` handles timeouts the same way regardless of whether the delay is network or queue.

### APICallGate

A gate that controls when API calls are allowed to proceed. Lives on `LLMAdapter`. One gate per adapter instance (which maps 1:1 to a provider endpoint).

```python
class APICallGate:
    """Rate-limiting gate for API calls. Gate model: timing only, not execution."""

    def __init__(self, max_rpm: int, pool_size: int | None = None):
        self._max_rpm = max_rpm
        self._timestamps: deque[float] = deque()  # sliding window, gate-thread-private
        self._queue: queue.Queue[_WorkItem] = queue.Queue()
        self._stop = threading.Event()  # for clean shutdown
        # Pool size: enough for max concurrent in-flight calls.
        # With RPM=60 and avg call duration ~10s, ~10 calls in-flight.
        # Default: max_rpm // 3, clamped to [2, 32].
        effective_pool = pool_size or max(2, min(32, max_rpm // 3))
        self._pool: ThreadPoolExecutor = ThreadPoolExecutor(max_workers=effective_pool)
        self._gate_thread = threading.Thread(target=self._gate_loop, daemon=True)
        self._gate_thread.start()
```

**Gate thread loop (timing only — does NOT execute calls):**
1. Dequeue next `_WorkItem` (blocks on `queue.get()`)
2. Prune timestamps older than 60s from the sliding window
3. If `len(timestamps) >= max_rpm`: wait until oldest timestamp + 60s, using `self._stop.wait(timeout=delay)` so shutdown can interrupt the sleep
4. Re-prune timestamps after waking (in case more have aged out)
5. Record current timestamp
6. Submit `work_item.fn` to `self._pool` wrapped in a safety harness
7. Loop immediately (don't wait for the call to finish)

This means multiple calls can be **in-flight concurrently** as long as RPM quota allows. The gate only controls *when* a call starts, not *when* it finishes.

**Work item:**
```python
@dataclass
class _WorkItem:
    fn: Callable[[], Any]       # the actual API call
    future: concurrent.futures.Future
```

**Submitting a call:**
```python
def submit(self, fn: Callable[[], Any]) -> Any:
    """Submit an API call through the gate. Blocks until result is ready."""
    future = concurrent.futures.Future()
    self._queue.put(_WorkItem(fn=fn, future=future))
    return future.result()  # agent thread blocks here — same as blocking on network I/O
```

**Pool execution wrapper (ensures Future is always resolved):**
```python
def _execute_in_pool(self, item: _WorkItem) -> None:
    """Submitted to the pool. Always resolves the future, even on crash."""
    try:
        result = item.fn()
        item.future.set_result(result)
    except BaseException as exc:
        item.future.set_exception(exc)
```

### Integration with LLMAdapter

`LLMAdapter` base class gets:

```python
class LLMAdapter(ABC):
    _gate: APICallGate | None = None

    def _setup_gate(self, max_rpm: int) -> None:
        """Set up rate-limiting gate. Called by adapter subclass __init__."""
        if max_rpm > 0:
            self._gate = APICallGate(max_rpm)

    def _gated_call(self, fn: Callable[[], Any]) -> Any:
        """Run fn through the gate if configured, otherwise call directly."""
        if self._gate is not None:
            return self._gate.submit(fn)
        return fn()
```

The existing `_rate_limiter` attribute and `_setup_rate_limiter()` method are removed, replaced by `_gate` and `_setup_gate()`.

### Integration with ChatSession

Adapters inject the gate into sessions at creation time. Both `send()` and `send_stream()` route through the gate:

```python
# In adapter's create_chat():
class GeminiChatSession(ChatSession):
    def __init__(self, ..., gate: APICallGate | None = None):
        self._gate = gate

    def send(self, message) -> LLMResponse:
        def _do_send():
            # existing send logic (provider SDK call)
            ...
        if self._gate is not None:
            return self._gate.submit(_do_send)
        return _do_send()

    def send_stream(self, message, on_chunk=None) -> LLMResponse:
        def _do_send_stream():
            # existing send_stream logic — on_chunk fires on the pool thread
            ...
        if self._gate is not None:
            return self._gate.submit(_do_send_stream)
        return _do_send_stream()
```

The gate controls *when* the streaming call starts. Once started, `on_chunk` callbacks fire on the pool thread during execution — same threading model as today (where `on_chunk` fires on the `timeout_pool` thread). The `submit()` call blocks until the entire streaming response is complete, which is the same behavior as the current `send_stream()`.

For one-shot calls (`adapter.generate()`, `adapter.web_search()`, `adapter.generate_vision()`), the adapter uses `self._gated_call()` directly.

### Double Thread Pool — Acceptable Overhead

`send_with_timeout` already submits `chat.send()` to a `ThreadPoolExecutor(max_workers=1)` per agent. With the gate, the call chain becomes:

```
timeout_pool thread → chat.send() → gate.submit() → blocks on Future → gate pool thread → SDK call
```

The `timeout_pool` thread is idle while blocking on the Future — no CPU cost, just one thread waiting. This is the same cost as blocking on a network call. The alternative (restructuring `send_with_timeout` to accept gate Futures) would violate the "zero changes to upper layers" design goal. The overhead is one idle thread per in-flight call, which is identical to the current behavior.

### Config

Rate limits configured via `provider_defaults`, which `LLMService` already supports:

```python
LLMService(
    provider="gemini",
    model="gemini-2.5-flash",
    provider_defaults={
        "gemini": {"model": "gemini-2.5-flash", "max_rpm": 60},
        "anthropic": {"model": "claude-sonnet-4-20250514", "max_rpm": 30},
    },
)
```

**Plumbing `max_rpm` to adapters:**

`LLMService._create_adapter()` extracts `max_rpm` from `provider_defaults` and passes it to the adapter constructor:

```python
def _create_adapter(self, provider, api_key, base_url):
    defaults = self._get_provider_defaults(provider)
    max_rpm = defaults.get("max_rpm", 0) if defaults else 0
    ...
    if p == "gemini":
        from .gemini.adapter import GeminiAdapter
        return GeminiAdapter(**key_kw, max_rpm=max_rpm)
    ...
```

Every adapter constructor gains an optional `max_rpm: int = 0` parameter. When `max_rpm > 0`, the adapter calls `self._setup_gate(max_rpm)` in `__init__`.

**Rules:**
- No `max_rpm` key in defaults → no gate (backward compat)
- `max_rpm: 0` → no gate (explicitly disabled)
- `max_rpm: N` where N > 0 → gate with N RPM

### Call Flow

```
Agent thread            ChatSession          APICallGate              Pool thread        Provider API
     |                       |              (gate thread)                  |                  |
     |-- chat.send(msg) ---> |                    |                       |                  |
     |                       |-- submit(fn) ----> |                       |                  |
     |                       |   (blocks on       |-- prune timestamps    |                  |
     |                       |    Future)         |-- check RPM window    |                  |
     |                       |                    |-- wait if needed      |                  |
     |                       |                    |   (Event.wait, not    |                  |
     |                       |                    |    sleep — shutdown   |                  |
     |                       |                    |    can interrupt)     |                  |
     |                       |                    |-- re-prune            |                  |
     |                       |                    |-- record timestamp    |                  |
     |                       |                    |-- pool.submit(fn) --> |                  |
     |                       |                    |   (loop immediately)  |-- fn() --------> |
     |                       |                    |                       |   <-- response -- |
     |                       |                    |                       |-- future.set()    |
     |   <-- response ------ | <----------------- | <------------------- |                  |
```

Same flow for `send_stream()` — the `fn` closure contains the entire streaming call including `on_chunk` callbacks.

### What Changes

| File | Change |
|------|--------|
| `src/lingtai/llm/api_gate.py` | **New.** `APICallGate`, `_WorkItem`, `_execute_in_pool` |
| `src/lingtai/llm/base.py` | Replace `_rate_limiter`/`_setup_rate_limiter` with `_gate`/`_setup_gate`/`_gated_call` on `LLMAdapter` |
| `src/lingtai/llm/service.py` | `_create_adapter()` extracts `max_rpm` from `provider_defaults`, passes to adapter constructors (internal change, no public API change) |
| `src/lingtai/llm/gemini/adapter.py` | Add `max_rpm` param to constructor, call `_setup_gate()`. Inject `self._gate` into sessions. Gate `send()`, `send_stream()`, `generate()`. |
| `src/lingtai/llm/openai/adapter.py` | Same pattern. |
| `src/lingtai/llm/anthropic/adapter.py` | Same pattern. |
| `src/lingtai/llm/minimax/adapter.py` | Remove `_RateLimitedSession`, remove old `RateLimiter` usage. Add `max_rpm` param, use gate. Old default `interval=2.0` → equivalent `max_rpm=30`. |
| Other adapters (deepseek, grok, qwen, glm, kimi, custom) | Add `max_rpm` param, call `_setup_gate()`. Gate `send()`, `send_stream()`, `generate()`. |
| `src/lingtai/llm/rate_limiter.py` | **Delete.** Replaced by `APICallGate`. |
| `tests/test_api_gate.py` | **New.** Unit tests for gate behavior. |

### What Doesn't Change

- `LLMService` public API — no new methods, no new constructor params (uses existing `provider_defaults`)
- `ChatSession` abstract interface — unchanged (gate is injected into concrete sessions)
- `BaseAgent` — zero changes
- `llm_utils.py` — zero changes
- `send_with_timeout` — unchanged, timeouts work the same

### MiniMax Migration

MiniMax currently has `_RateLimitedSession` (duck-typed proxy) and per-call `self._rate_limiter.wait()`:

- Remove `_RateLimitedSession` class entirely
- Remove `self._limiter` / `RateLimiter` usage
- Add `max_rpm: int = 0` to `MiniMaxAdapter.__init__()`
- Default: if no `max_rpm` in config, use `max_rpm=30` (equivalent to old `interval=2.0`)
- Call `self._setup_gate(max_rpm)` in `__init__`
- Inject `self._gate` into sessions
- `LLMService.create_session()` sets `chat.session_id` and `chat._agent_type` directly on the session (no longer through the `_RateLimitedSession` proxy's `__getattr__`)

### Shutdown

- Gate thread uses `self._stop.wait(timeout=...)` instead of `time.sleep()` — shutdown can interrupt the wait immediately
- `APICallGate.shutdown()`: sets `self._stop`, drains remaining queue items by setting their futures with a `RuntimeError("Gate shutting down")`
- Shuts down the pool with `self._pool.shutdown(wait=False)`
- `LLMAdapter` can call `self._gate.shutdown()` in cleanup if needed

### Error Handling

- Pool execution wrapper (`_execute_in_pool`) catches `BaseException` and always calls `future.set_exception()` — the future is guaranteed to resolve even if the call crashes
- The agent thread sees the exception via `future.result()` — identical to a direct call that raised
- 429 errors from the provider are NOT handled by the gate — that's `send_with_timeout`'s retry logic. The gate prevents 429s proactively; retries handle them reactively
- Per-process RPM tracking only — if multiple processes share one API key, they rate-limit independently and may exceed quota in aggregate. Cross-process rate limiting is a future concern.

### Thread Safety

- `_timestamps` deque: gate-thread-private, never accessed from other threads. No lock needed.
- `queue.Queue`: thread-safe for `put()`/`get()`.
- `concurrent.futures.Future`: `.set_result()` and `.result()` are internally synchronized.
- Gate thread never waits on anything the agent thread holds. No deadlock risk.
- Pool threads only touch the Future they were given. No shared mutable state.

## Testing

- Gate enforces RPM: submit `max_rpm + 1` concurrent calls, verify the last one starts after ~1s delay
- No gate: calls go through immediately (zero overhead when `max_rpm=0` or not set)
- Concurrent calls: multiple calls in-flight simultaneously when RPM window has capacity
- Streaming: `send_stream()` gated the same as `send()`, `on_chunk` fires during execution
- Exception propagation: adapter error propagates to caller via Future
- Pool safety: crashing `fn` still resolves the Future with exception
- Shutdown: pending items get `RuntimeError`, gate thread exits promptly
- MiniMax migration: old `RateLimiter` removed, gate used, session attributes set directly

## Future Extensions (not in this spec)

- **Priority queues** — swap `queue.Queue` for `queue.PriorityQueue`, add priority to work items
- **TPM (tokens per minute)** — add token estimate to work items, track in sliding window
- **Backpressure** — expose queue depth so agents can skip optional calls when congested
- **Per-agent fairness** — round-robin or weighted fair queuing across agents
- **Dynamic RPM adjustment** — reduce effective RPM on 429 errors, increase when successful
- **Cancellation** — add cancellation token to `_WorkItem`, allow queued (not yet dispatched) items to be cancelled
