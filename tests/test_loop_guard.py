from lingtai_kernel.loop_guard import LoopGuard


def test_loop_guard_no_loop():
    """Distinct tool calls with different args should never be blocked."""
    guard = LoopGuard(max_total_calls=10)
    for i in range(5):
        verdict = guard.record_tool_call(f"tool_{i}", {"arg": i})
        assert not verdict.blocked


def test_loop_guard_detects_identical():
    """Identical calls at or beyond dup_hard_block should be blocked."""
    guard = LoopGuard(max_total_calls=20, dup_hard_block=3)
    guard.record_tool_call("read", {"path": "/foo"})
    guard.record_tool_call("read", {"path": "/foo"})
    verdict = guard.record_tool_call("read", {"path": "/foo"})  # 3rd = hard block
    assert verdict.blocked
    assert verdict.count == 3


def test_loop_guard_warning_before_block():
    """Calls between free passes and hard block should get a warning but not be blocked."""
    guard = LoopGuard(max_total_calls=20, dup_free_passes=1, dup_hard_block=5)
    guard.record_tool_call("read", {"path": "/foo"})  # count=1 (free pass)
    verdict = guard.record_tool_call("read", {"path": "/foo"})  # count=2 (warn)
    assert not verdict.blocked
    assert verdict.warning is not None


def test_loop_guard_check_limit():
    """check_limit returns a reason when total would be exceeded."""
    guard = LoopGuard(max_total_calls=3)
    guard.record_calls(3)
    reason = guard.check_limit(1)
    assert reason is not None
    assert "3" in reason


def test_loop_guard_invalid_tool():
    """Repeated invalid tool names should trigger a stop reason."""
    guard = LoopGuard(max_total_calls=20, invalid_tool_limit=2)
    guard.record_invalid_tool("ghost_tool")
    guard.record_invalid_tool("ghost_tool")
    guard.record_invalid_tool("ghost_tool")  # count=3 > limit=2
    reason = guard.check_invalid_tool_limit()
    assert reason is not None
    assert "ghost_tool" in reason
