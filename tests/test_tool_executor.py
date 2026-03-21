"""Tests for ToolExecutor — sequential and parallel tool execution."""
from __future__ import annotations

import threading
import time
from unittest.mock import MagicMock

import pytest

from lingtai_kernel.llm.base import ToolCall
from lingtai_kernel.loop_guard import LoopGuard
from lingtai_kernel.tool_executor import ToolExecutor
from lingtai_kernel.types import UnknownToolError


def make_executor(dispatch_fn=None, parallel_safe=None, known_tools=None):
    if dispatch_fn is None:
        dispatch_fn = lambda tc: {"status": "ok", "result": f"ran {tc.name}"}
    make_result = MagicMock(side_effect=lambda name, result, **kw: {"name": name, "result": result})
    guard = LoopGuard(max_total_calls=50)
    return ToolExecutor(
        dispatch_fn=dispatch_fn,
        make_tool_result_fn=make_result,
        guard=guard,
        known_tools=known_tools,
        parallel_safe_tools=parallel_safe or set(),
    )


def test_execute_single_tool():
    executor = make_executor()
    calls = [ToolCall(name="read", args={"path": "/tmp"}, id="tc1")]
    results, intercepted, text = executor.execute(calls)
    assert len(results) == 1
    assert not intercepted


def test_execute_sequential_multiple():
    order = []
    def dispatch(tc):
        order.append(tc.name)
        return {"status": "ok"}
    executor = make_executor(dispatch_fn=dispatch)
    calls = [
        ToolCall(name="a", args={}, id="1"),
        ToolCall(name="b", args={}, id="2"),
    ]
    results, intercepted, text = executor.execute(calls)
    assert len(results) == 2
    assert order == ["a", "b"]


def test_execute_parallel():
    def dispatch(tc):
        time.sleep(0.05)
        return {"status": "ok", "tool": tc.name}
    executor = make_executor(
        dispatch_fn=dispatch,
        parallel_safe={"a", "b"},
    )
    calls = [
        ToolCall(name="a", args={}, id="1"),
        ToolCall(name="b", args={}, id="2"),
    ]
    t0 = time.monotonic()
    results, intercepted, text = executor.execute(calls)
    elapsed = time.monotonic() - t0
    assert len(results) == 2
    assert elapsed < 0.15


def test_intercept_hook():
    executor = make_executor()
    hook = MagicMock(return_value="intercepted!")
    calls = [ToolCall(name="read", args={}, id="1")]
    results, intercepted, text = executor.execute(calls, on_result_hook=hook)
    assert intercepted
    assert text == "intercepted!"


def test_error_collected():
    def dispatch(tc):
        raise ValueError("something broke")
    executor = make_executor(dispatch_fn=dispatch)
    calls = [ToolCall(name="bad", args={}, id="1")]
    errors = []
    results, intercepted, text = executor.execute(calls, collected_errors=errors)
    assert len(results) == 1
    assert "bad" in errors[0]
    assert "something broke" in errors[0]


def test_cancel_event_stops_sequential():
    cancel = threading.Event()
    cancel.set()
    executor = make_executor()
    calls = [ToolCall(name="a", args={}, id="1")]
    results, intercepted, text = executor.execute(calls, cancel_event=cancel)
    assert results == []


def test_unknown_tool_with_known_tools():
    executor = make_executor(known_tools={"read", "write"})
    calls = [ToolCall(name="bogus", args={}, id="1")]
    errors = []
    results, intercepted, text = executor.execute(calls, collected_errors=errors)
    assert len(results) == 1
    assert any("bogus" in e for e in errors)


def test_guard_property():
    executor = make_executor()
    old_guard = executor.guard
    new_guard = LoopGuard(max_total_calls=10)
    executor.guard = new_guard
    assert executor.guard is new_guard


def test_reasoning_stripped_from_args():
    dispatched_args = []
    def dispatch(tc):
        dispatched_args.append(tc.args)
        return {"status": "ok"}
    executor = make_executor(dispatch_fn=dispatch)
    calls = [ToolCall(name="read", args={"path": "/tmp", "reasoning": "because"}, id="1")]
    executor.execute(calls)
    assert "reasoning" not in dispatched_args[0]
    assert dispatched_args[0].get("_reasoning") == "because"
