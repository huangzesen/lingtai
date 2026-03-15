"""Tests for stoai.llm_utils."""

from stoai.llm_utils import (
    get_context_limit,
    build_outcome_summary,
    track_llm_usage,
    execute_tools_batch,
    EVENT_DEBUG,
    EVENT_LLM_RESPONSE,
    EVENT_TOKEN_USAGE,
    EVENT_THINKING,
    MODEL_CONTEXT_LIMITS,
)


def test_get_context_limit_known_model():
    """Known models should return their hardcoded limit."""
    limit = get_context_limit("gemini-2.5-flash")
    assert limit == 1_048_576


def test_get_context_limit_prefix_match():
    """Models with version suffixes should match via longest prefix."""
    limit = get_context_limit("claude-opus-4-20250514")
    assert limit == 200_000


def test_get_context_limit_unknown():
    """Unknown models should return 0 (no match)."""
    limit = get_context_limit("totally-unknown-model-xyz")
    assert isinstance(limit, int)
    assert limit >= 0


def test_get_context_limit_empty():
    """Empty model name returns 0."""
    assert get_context_limit("") == 0


def test_build_outcome_summary_basic():
    """build_outcome_summary produces a readable string."""
    outcomes = [
        {"tool": "fetch_data", "status": "ok", "label": "B_GSM", "num_points": 1000, "units": "nT"},
        {"tool": "render_plot", "status": "ok"},
    ]
    summary = build_outcome_summary(outcomes)
    assert "fetch_data=ok" in summary
    assert "B_GSM" in summary
    assert "render_plot=ok" in summary


def test_build_outcome_summary_error_truncation():
    """Error messages longer than 500 chars should be truncated."""
    long_msg = "x" * 1000
    outcomes = [{"tool": "bad_tool", "status": "error", "message": long_msg}]
    summary = build_outcome_summary(outcomes)
    assert "..." in summary
    # Should not contain the full 1000-char message
    assert len(summary) < 1000


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
    events = []

    def capture_event(event_type, payload):
        events.append((event_type, payload))

    track_llm_usage(
        FakeLLMResponse(),
        state,
        "test_agent",
        "some_tool",
        on_event=capture_event,
    )

    assert state["input"] == 100
    assert state["output"] == 50
    assert state["thinking"] == 10
    assert state["cached"] == 20
    assert state["api_calls"] == 1

    # Should have emitted llm_response, token_usage, and thinking events
    event_types = [e[0] for e in events]
    assert EVENT_LLM_RESPONSE in event_types
    assert EVENT_TOKEN_USAGE in event_types
    assert EVENT_THINKING in event_types


def test_track_llm_usage_no_event_callback():
    """track_llm_usage with on_event=None should not raise."""
    state = {"input": 0, "output": 0, "thinking": 0, "cached": 0, "api_calls": 0}
    track_llm_usage(
        FakeLLMResponse(),
        state,
        "test_agent",
        "some_tool",
        on_event=None,
    )
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
        calls, executor, set(), False, 4, "test", None, on_event=None,
    )
    assert len(results) == 2
    assert results[0][1] == "tool_a"
    assert results[1][1] == "tool_b"
    assert execution_order == ["tool_a", "tool_b"]


def test_execute_tools_batch_parallel():
    """execute_tools_batch runs in parallel when all tools are safe."""
    calls = [FakeToolCall("safe_a", {}), FakeToolCall("safe_b", {})]
    events = []

    def executor(name, args, tc_id):
        return {"status": "ok"}

    def capture(et, payload):
        events.append((et, payload))

    results = execute_tools_batch(
        calls, executor, {"safe_a", "safe_b"}, True, 4, "test", None,
        on_event=capture,
    )
    assert len(results) == 2
    # Should have emitted a debug event about parallel execution
    assert any(et == EVENT_DEBUG for et, _ in events)
