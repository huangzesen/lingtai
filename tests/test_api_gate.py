"""Tests for APICallGate."""
import time
import threading

import pytest

from stoai_kernel.llm.api_gate import APICallGate


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
    """With max_rpm=2, 2 calls proceed immediately."""
    gate = APICallGate(max_rpm=2, pool_size=4)
    timestamps = []

    def timed_call():
        timestamps.append(time.monotonic())
        return "ok"

    try:
        threads = []
        for _ in range(2):
            t = threading.Thread(target=lambda: gate.submit(timed_call))
            t.start()
            threads.append(t)
        for t in threads:
            t.join(timeout=5.0)
        assert len(timestamps) == 2
        assert timestamps[1] - timestamps[0] < 1.0
    finally:
        gate.shutdown()


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
        assert active["max"] > 1
    finally:
        gate.shutdown()


def test_gate_shutdown_resolves_pending():
    """Shutdown should resolve pending futures with RuntimeError."""
    gate = APICallGate(max_rpm=1, pool_size=1)
    gate.submit(lambda: "first")

    result_holder = {"error": None}

    def submit_second():
        try:
            gate.submit(lambda: "second")
        except RuntimeError as e:
            result_holder["error"] = str(e)

    t = threading.Thread(target=submit_second)
    t.start()
    time.sleep(0.1)
    gate.shutdown()
    t.join(timeout=5.0)
    assert result_holder["error"] is not None


def test_gate_invalid_max_rpm():
    """max_rpm <= 0 should raise ValueError."""
    with pytest.raises(ValueError):
        APICallGate(max_rpm=0)
    with pytest.raises(ValueError):
        APICallGate(max_rpm=-1)


def test_gated_call_on_adapter():
    """_gated_call routes through gate when configured."""
    from stoai_kernel.llm.base import LLMAdapter

    # Create a minimal concrete adapter for testing
    class FakeAdapter(LLMAdapter):
        def __init__(self, max_rpm=0):
            self._setup_gate(max_rpm)
        def create_chat(self, *a, **kw): pass
        def generate(self, *a, **kw): pass
        def make_tool_result_message(self, *a, **kw): pass
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
