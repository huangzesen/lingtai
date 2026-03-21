"""Tests for lingtai.llm_utils."""

from lingtai_kernel.llm_utils import (
    track_llm_usage,
    execute_tools_batch,
)


class FakeLLMResponse:
    """Minimal mock for LLMResponse."""
    class Usage:
        input_tokens = 100
        output_tokens = 50
        thinking_tokens = 10
        cached_tokens = 20
    usage = Usage()
    thoughts = ["I think therefore I am"]


def test_track_llm_usage_accumulates():
    """track_llm_usage should update token_state in place."""
    state = {"input": 0, "output": 0, "thinking": 0, "cached": 0, "api_calls": 0}

    track_llm_usage(
        FakeLLMResponse(),
        state,
        "test_agent",
        "some_tool",
    )

    assert state["input"] == 100
    assert state["output"] == 50
    assert state["thinking"] == 10
    assert state["cached"] == 20
    assert state["api_calls"] == 1


class FakeToolCall:
    def __init__(self, name, args, id=None):
        self.name = name
        self.args = args
        self.id = id


def test_execute_tools_batch_sequential():
    """execute_tools_batch runs sequentially when parallel is disabled."""
    calls = [FakeToolCall("tool_a", {"x": 1}), FakeToolCall("tool_b", {"y": 2})]
    execution_order = []

    def executor(name, args, tc_id):
        execution_order.append(name)
        return {"status": "ok", "tool": name}

    results = execute_tools_batch(
        calls, executor, set(), False, 4, "test", None,
    )
    assert len(results) == 2
    assert results[0][1] == "tool_a"
    assert results[1][1] == "tool_b"
    assert execution_order == ["tool_a", "tool_b"]


def test_execute_tools_batch_parallel():
    """execute_tools_batch runs in parallel when all tools are safe."""
    calls = [FakeToolCall("safe_a", {}), FakeToolCall("safe_b", {})]

    def executor(name, args, tc_id):
        return {"status": "ok"}

    results = execute_tools_batch(
        calls, executor, {"safe_a", "safe_b"}, True, 4, "test", None,
    )
    assert len(results) == 2
