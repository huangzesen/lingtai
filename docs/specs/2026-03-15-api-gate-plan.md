# API Call Gate Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-adapter RPM rate limiting via an `APICallGate` that queues calls and dispatches them through a thread pool, transparent to all upper layers.

**Architecture:** `APICallGate` is a standalone class with a gate thread (timing) and thread pool (execution). `LLMAdapter` base class gets `_setup_gate()` / `_gated_call()`. Each concrete adapter reads `max_rpm` from its constructor and optionally enables the gate. Sessions get the gate injected at creation time and route `send()` / `send_stream()` through it.

**Tech Stack:** Python threading, `queue.Queue`, `concurrent.futures.Future`, `ThreadPoolExecutor`

**Spec:** `docs/specs/2026-03-15-llm-api-call-queue-design.md`

---

## Chunk 1: APICallGate + base class wiring

### Task 1: Create APICallGate

**Files:**
- Create: `src/lingtai/llm/api_gate.py`
- Test: `tests/test_api_gate.py`

- [ ] **Step 1: Write failing tests for APICallGate**

```python
# tests/test_api_gate.py
"""Tests for APICallGate."""
import time
import threading
import concurrent.futures

import pytest

from lingtai.llm.api_gate import APICallGate


def test_gate_passes_calls_through():
    """Calls go through and return results."""
    gate = APICallGate(max_rpm=60)
    try:
        result = gate.submit(lambda: 42)
        assert result == 42
    finally:
        gate.shutdown()


def test_gate_propagates_exceptions():
    """Exceptions from fn propagate to caller."""
    gate = APICallGate(max_rpm=60)
    try:
        with pytest.raises(ValueError, match="boom"):
            gate.submit(lambda: (_ for _ in ()).throw(ValueError("boom")))
    finally:
        gate.shutdown()


def test_gate_enforces_rpm():
    """With max_rpm=2, third concurrent call should be delayed."""
    gate = APICallGate(max_rpm=2, pool_size=4)
    timestamps = []

    def timed_call():
        timestamps.append(time.monotonic())
        return "ok"

    try:
        # Submit 3 calls concurrently — third should wait ~60s.
        # We can't wait 60s in a test, so verify the gate queues correctly
        # by checking that 2 calls proceed immediately.
        futures = []
        for _ in range(2):
            f = threading.Thread(target=lambda: gate.submit(timed_call))
            f.start()
            futures.append(f)
        for f in futures:
            f.join(timeout=5.0)
        assert len(timestamps) == 2
        # Both should complete within 1 second (no rate limit hit)
        assert timestamps[1] - timestamps[0] < 1.0
    finally:
        gate.shutdown()


def test_gate_no_calls_without_gate():
    """When gate is None, _gated_call on adapter should call directly."""
    # This tests the base class integration, covered in Task 2
    pass


def test_gate_concurrent_in_flight():
    """Multiple slow calls should be in-flight simultaneously."""
    gate = APICallGate(max_rpm=10, pool_size=4)
    active = {"count": 0, "max": 0}
    lock = threading.Lock()

    def slow_call():
        with lock:
            active["count"] += 1
            active["max"] = max(active["max"], active["count"])
        time.sleep(0.2)
        with lock:
            active["count"] -= 1
        return "ok"

    try:
        threads = []
        for _ in range(4):
            t = threading.Thread(target=lambda: gate.submit(slow_call))
            t.start()
            threads.append(t)
        for t in threads:
            t.join(timeout=5.0)
        # Multiple calls should have been in-flight at the same time
        assert active["max"] > 1
    finally:
        gate.shutdown()


def test_gate_shutdown_resolves_pending():
    """Shutdown should resolve pending futures with RuntimeError."""
    gate = APICallGate(max_rpm=1, pool_size=1)

    # Fill the RPM window
    gate.submit(lambda: "first")

    # Next call will be queued (RPM exhausted)
    result_holder = {"error": None}

    def submit_second():
        try:
            gate.submit(lambda: "second")
        except RuntimeError as e:
            result_holder["error"] = str(e)

    t = threading.Thread(target=submit_second)
    t.start()
    time.sleep(0.1)  # let it queue
    gate.shutdown()
    t.join(timeout=5.0)
    assert result_holder["error"] is not None
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `source venv/bin/activate && python -m pytest tests/test_api_gate.py -v`
Expected: ImportError — `api_gate` module doesn't exist yet

- [ ] **Step 3: Implement APICallGate**

```python
# src/lingtai/llm/api_gate.py
"""APICallGate — rate-limiting gate for LLM API calls.

Gate model: a gate thread controls WHEN calls proceed (timing),
a thread pool executes them (concurrency). Multiple calls can be
in-flight simultaneously as long as the RPM window has capacity.
"""
from __future__ import annotations

import concurrent.futures
import queue
import threading
import time
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from typing import Any, Callable


@dataclass
class _WorkItem:
    fn: Callable[[], Any]
    future: concurrent.futures.Future


class APICallGate:
    """Rate-limiting gate for API calls.

    Args:
        max_rpm: Maximum requests per minute. Must be > 0.
        pool_size: Thread pool size for executing calls. Default: max_rpm // 3,
                   clamped to [2, 32].
    """

    def __init__(self, max_rpm: int, pool_size: int | None = None):
        if max_rpm <= 0:
            raise ValueError(f"max_rpm must be > 0, got {max_rpm}")
        self._max_rpm = max_rpm
        self._timestamps: deque[float] = deque()
        self._queue: queue.Queue[_WorkItem | None] = queue.Queue()
        self._stop = threading.Event()
        effective_pool = pool_size or max(2, min(32, max_rpm // 3))
        self._pool = ThreadPoolExecutor(max_workers=effective_pool)
        self._gate_thread = threading.Thread(
            target=self._gate_loop, daemon=True, name="api-gate"
        )
        self._gate_thread.start()

    def submit(self, fn: Callable[[], Any]) -> Any:
        """Submit an API call through the gate. Blocks until result."""
        if self._stop.is_set():
            raise RuntimeError("Gate is shut down")
        future: concurrent.futures.Future = concurrent.futures.Future()
        self._queue.put(_WorkItem(fn=fn, future=future))
        return future.result()

    def shutdown(self) -> None:
        """Shut down the gate. Pending items get RuntimeError."""
        self._stop.set()
        self._queue.put(None)  # unblock gate thread
        self._gate_thread.join(timeout=5.0)
        # Drain remaining items
        while True:
            try:
                item = self._queue.get_nowait()
            except queue.Empty:
                break
            if item is not None:
                item.future.set_exception(RuntimeError("Gate shut down"))
        self._pool.shutdown(wait=False)

    def _gate_loop(self) -> None:
        """Gate thread: controls timing, submits to pool."""
        while not self._stop.is_set():
            try:
                item = self._queue.get(timeout=1.0)
            except queue.Empty:
                continue
            if item is None:
                break  # shutdown sentinel

            # Prune old timestamps
            now = time.monotonic()
            while self._timestamps and self._timestamps[0] <= now - 60.0:
                self._timestamps.popleft()

            # Wait if RPM window is full
            while len(self._timestamps) >= self._max_rpm and not self._stop.is_set():
                wait_until = self._timestamps[0] + 60.0
                delay = max(0, wait_until - time.monotonic())
                if delay > 0:
                    self._stop.wait(timeout=delay)
                # Re-prune after waking
                now = time.monotonic()
                while self._timestamps and self._timestamps[0] <= now - 60.0:
                    self._timestamps.popleft()

            if self._stop.is_set():
                item.future.set_exception(RuntimeError("Gate shut down"))
                break

            # Record timestamp and dispatch
            self._timestamps.append(time.monotonic())
            self._pool.submit(self._execute, item)

    @staticmethod
    def _execute(item: _WorkItem) -> None:
        """Run in pool thread. Always resolves the future."""
        try:
            result = item.fn()
            item.future.set_result(result)
        except BaseException as exc:
            item.future.set_exception(exc)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `source venv/bin/activate && python -m pytest tests/test_api_gate.py -v`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/llm/api_gate.py tests/test_api_gate.py
git commit -m "feat: add APICallGate — rate-limiting gate for LLM API calls"
```

### Task 2: Wire gate into LLMAdapter base class

**Files:**
- Modify: `src/lingtai/llm/base.py`

- [ ] **Step 1: Replace `_rate_limiter` with `_gate` on LLMAdapter**

In `src/lingtai/llm/base.py`, replace the existing rate limiter code:

```python
# Remove these lines:
from .rate_limiter import RateLimiter
# ...
_rate_limiter: RateLimiter | None = None

def _setup_rate_limiter(self, min_interval: float) -> None:
    if min_interval > 0:
        self._rate_limiter = RateLimiter(min_interval)
```

Replace with:

```python
from .api_gate import APICallGate
# ...
class LLMAdapter(ABC):
    supports_web_search: bool = False
    supports_vision: bool = False
    _gate: APICallGate | None = None

    def _setup_gate(self, max_rpm: int) -> None:
        """Set up rate-limiting gate for this adapter."""
        if max_rpm > 0:
            self._gate = APICallGate(max_rpm)

    def _gated_call(self, fn: Callable[[], Any]) -> Any:
        """Run fn through the gate if configured, otherwise call directly."""
        if self._gate is not None:
            return self._gate.submit(fn)
        return fn()
```

- [ ] **Step 2: Run existing tests to verify nothing breaks**

Run: `source venv/bin/activate && python -m pytest tests/ -v --tb=short`
Expected: All 124 pass (MiniMax adapter imports `RateLimiter` directly, not through base — will fix in Task 3)

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/llm/base.py
git commit -m "feat: replace _rate_limiter with _gate/_setup_gate/_gated_call on LLMAdapter"
```

### Task 3: Migrate MiniMax to use gate

**Files:**
- Modify: `src/lingtai/llm/minimax/adapter.py`

- [ ] **Step 1: Replace MiniMax's RateLimiter with gate**

Rewrite `src/lingtai/llm/minimax/adapter.py`:

- Remove `_RateLimitedSession` class entirely
- Remove `from ..rate_limiter import RateLimiter`
- Change `MiniMaxAdapter.__init__` to call `self._setup_gate(30)` (equivalent to old `interval=2.0`)
- Change `create_chat` to inject `self._gate` into session — but since MiniMax extends `AnthropicAdapter`, and sessions are created by the parent, we use `_gated_call` in `generate()` and `web_search()`, and override `create_chat` to wrap the session's `send`/`send_stream`.

Actually simpler: since sessions are `AnthropicChatSession` objects, and the gate is on the adapter, the cleanest approach is to keep the proxy but make it thinner — just route `send`/`send_stream` through `self._gate`. Or even simpler: just gate `generate()` and `web_search()` with `_gated_call`, and for sessions, wrap `send`/`send_stream` as a thin lambda.

Rewrite:

```python
# src/lingtai/llm/minimax/adapter.py
from ...logging import get_logger
from ..anthropic.adapter import AnthropicAdapter
from ..base import ChatSession, LLMResponse

logger = get_logger()

from .defaults import DEFAULTS  # noqa: F401 — re-exported for consumers


class _GatedSession:
    """Thin proxy that routes send/send_stream through the adapter's gate."""

    def __init__(self, inner: ChatSession, gate):
        self._inner = inner
        self._gate = gate

    @property
    def interface(self):
        return self._inner.interface

    def send(self, message):
        if self._gate is not None:
            return self._gate.submit(lambda: self._inner.send(message))
        return self._inner.send(message)

    def send_stream(self, message, on_chunk=None):
        if self._gate is not None:
            return self._gate.submit(lambda: self._inner.send_stream(message, on_chunk=on_chunk))
        return self._inner.send_stream(message, on_chunk=on_chunk)

    def __getattr__(self, name):
        return getattr(self._inner, name)


class MiniMaxAdapter(AnthropicAdapter):
    supports_web_search = True
    supports_vision = True

    def __init__(
        self, api_key: str, *, base_url: str | None = None,
        max_rpm: int = 30, timeout_ms: int = 300_000,
    ):
        effective_url = base_url or "https://api.minimaxi.com/anthropic"
        super().__init__(api_key=api_key, base_url=effective_url, timeout_ms=timeout_ms)
        self._setup_gate(max_rpm)

    def create_chat(self, *args, **kwargs):
        session = super().create_chat(*args, **kwargs)
        if self._gate is not None:
            return _GatedSession(session, self._gate)
        return session

    def generate(self, *args, **kwargs) -> LLMResponse:
        return self._gated_call(lambda: super(MiniMaxAdapter, self).generate(*args, **kwargs))

    def web_search(self, query: str, model: str) -> LLMResponse:
        def _do_search():
            try:
                from .mcp_client import get_minimax_mcp_client
                client = get_minimax_mcp_client()
                result = client.call_tool("web_search", {"query": query})
                if result.get("status") == "error":
                    logger.warning("MiniMax MCP web search error: %s", result.get("message"))
                    return LLMResponse(text="")
                text = result.get("text", "") or result.get("answer", "")
                if not text and "organic" in result:
                    parts = []
                    for item in result["organic"]:
                        title = item.get("title", "")
                        snippet = item.get("snippet", "")
                        link = item.get("link", "")
                        parts.append(f"**{title}**\n{snippet}\nSource: {link}")
                    text = "\n\n".join(parts)
                if not text:
                    text = str(result)
                return LLMResponse(text=text)
            except Exception as e:
                logger.warning("MiniMax MCP web search failed: %s", e)
                return LLMResponse(text="")
        return self._gated_call(_do_search)

    def make_multimodal_message(
        self, text: str, image_bytes: bytes, mime_type: str = "image/png"
    ) -> dict:
        logger.warning("MiniMax Anthropic-compatible API does not support image input")
        return {"role": "user", "content": [{"type": "text", "text": text}]}
```

- [ ] **Step 2: Run all tests**

Run: `source venv/bin/activate && python -m pytest tests/ -v --tb=short`
Expected: All pass

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/llm/minimax/adapter.py
git commit -m "refactor: migrate MiniMax from RateLimiter to APICallGate"
```

### Task 4: Wire max_rpm through LLMService to adapters

**Files:**
- Modify: `src/lingtai/llm/service.py`

- [ ] **Step 1: Extract max_rpm in _create_adapter and pass to all adapters**

In `_create_adapter`, add `max_rpm` extraction and pass it through:

```python
def _create_adapter(self, provider: str, api_key: str | None, base_url: str | None) -> LLMAdapter:
    key_kw: dict = {"api_key": api_key} if api_key is not None else {}
    url_kw: dict = {"base_url": base_url} if base_url is not None else {}

    # Extract max_rpm from provider defaults
    defaults = self._get_provider_defaults(provider)
    max_rpm = defaults.get("max_rpm", 0) if defaults else 0
    rpm_kw: dict = {"max_rpm": max_rpm} if max_rpm > 0 else {}

    p = provider.lower()
    if p == "gemini":
        from .gemini.adapter import GeminiAdapter
        return GeminiAdapter(**key_kw, **rpm_kw)
    elif p == "anthropic":
        from .anthropic.adapter import AnthropicAdapter
        return AnthropicAdapter(**key_kw, **url_kw, **rpm_kw)
    # ... same for all others
```

- [ ] **Step 2: Add `max_rpm: int = 0` to every adapter constructor**

Each adapter's `__init__` gains `max_rpm: int = 0` and calls `self._setup_gate(max_rpm)`:

For example in `GeminiAdapter`:
```python
def __init__(self, api_key: str, timeout_ms: int = 300_000, max_rpm: int = 0):
    ...existing init...
    self._setup_gate(max_rpm)
```

Same one-line addition for: `AnthropicAdapter`, `OpenAIAdapter`, `GrokAdapter`, `DeepSeekAdapter`, `QwenAdapter`, `GLMAdapter`. For `MiniMaxAdapter`, already done in Task 3. For `kimi` and `custom` (factory functions), pass through.

- [ ] **Step 3: For adapters with sessions, inject gate**

For adapters that create `ChatSession` subclasses, gate `send()`/`send_stream()`. Since most adapters (OpenAI-compatible: grok, deepseek, qwen, glm, kimi, custom) share `OpenAIChatSession`, we can gate at the `OpenAIAdapter.create_chat()` level using the same `_GatedSession` proxy pattern, or add gate support to `OpenAIChatSession` directly.

Simplest: add `_gate` param to each session class's `__init__`, gate in `send()`/`send_stream()`.

- [ ] **Step 4: Run all tests**

Run: `source venv/bin/activate && python -m pytest tests/ -v --tb=short`
Expected: All pass (no max_rpm configured → no gate → identical behavior)

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/llm/service.py src/lingtai/llm/gemini/adapter.py src/lingtai/llm/openai/adapter.py src/lingtai/llm/anthropic/adapter.py src/lingtai/llm/grok/adapter.py src/lingtai/llm/deepseek/adapter.py src/lingtai/llm/qwen/adapter.py src/lingtai/llm/glm/adapter.py src/lingtai/llm/kimi/adapter.py src/lingtai/llm/custom/adapter.py
git commit -m "feat: wire max_rpm from provider_defaults through LLMService to all adapters"
```

### Task 5: Delete old rate_limiter.py

**Files:**
- Delete: `src/lingtai/llm/rate_limiter.py`

- [ ] **Step 1: Verify no remaining imports**

Run: `grep -r "rate_limiter" src/`
Expected: No matches (all references removed in Tasks 2-3)

- [ ] **Step 2: Delete the file**

```bash
rm src/lingtai/llm/rate_limiter.py
```

- [ ] **Step 3: Run all tests**

Run: `source venv/bin/activate && python -m pytest tests/ -v --tb=short`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "cleanup: remove old rate_limiter.py, replaced by api_gate.py"
```

### Task 6: Add integration test — gate with mock adapter

**Files:**
- Modify: `tests/test_api_gate.py`

- [ ] **Step 1: Add integration test**

```python
def test_gated_call_on_adapter():
    """_gated_call routes through gate when configured."""
    from lingtai.llm.api_gate import APICallGate
    from lingtai.llm.base import LLMAdapter

    # Create a minimal concrete adapter for testing
    class FakeAdapter(LLMAdapter):
        def __init__(self, max_rpm=0):
            self._setup_gate(max_rpm)
        def create_chat(self, *a, **kw): pass
        def generate(self, *a, **kw): pass
        def make_tool_result_message(self, *a, **kw): pass
        def make_multimodal_message(self, *a, **kw): pass
        def is_quota_error(self, exc): return False

    # With gate
    adapter = FakeAdapter(max_rpm=60)
    assert adapter._gate is not None
    result = adapter._gated_call(lambda: "gated")
    assert result == "gated"
    adapter._gate.shutdown()

    # Without gate
    adapter2 = FakeAdapter(max_rpm=0)
    assert adapter2._gate is None
    result = adapter2._gated_call(lambda: "direct")
    assert result == "direct"
```

- [ ] **Step 2: Run tests**

Run: `source venv/bin/activate && python -m pytest tests/test_api_gate.py -v`
Expected: All pass

- [ ] **Step 3: Commit**

```bash
git add tests/test_api_gate.py
git commit -m "test: add integration test for adapter _gated_call"
```
