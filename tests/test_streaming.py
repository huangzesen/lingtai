"""Tests for StreamingAccumulator."""

from __future__ import annotations

from lingtai_kernel.llm.streaming import StreamingAccumulator
from lingtai_kernel.llm.base import ToolCall, UsageMetadata


# -- Text accumulation ------------------------------------------------------

def test_text_accumulation():
    acc = StreamingAccumulator()
    acc.add_text("Hello")
    acc.add_text(" world")
    result = acc.finalize()
    assert result.text == "Hello world"
    assert result.tool_calls == []
    assert result.thoughts == []


def test_empty_finalize():
    acc = StreamingAccumulator()
    result = acc.finalize()
    assert result.text == ""
    assert result.tool_calls == []
    assert result.thoughts == []


# -- Sequential tool calls (Anthropic / OpenAI Responses) -------------------

def test_sequential_tool_single():
    acc = StreamingAccumulator()
    acc.start_tool(id="toolu_1", name="read_file")
    acc.add_tool_args('{"path":')
    acc.add_tool_args(' "foo.py"}')
    acc.finish_tool()
    result = acc.finalize()
    assert len(result.tool_calls) == 1
    tc = result.tool_calls[0]
    assert tc.name == "read_file"
    assert tc.args == {"path": "foo.py"}
    assert tc.id == "toolu_1"


def test_sequential_tool_multiple():
    acc = StreamingAccumulator()
    acc.start_tool(id="t1", name="read")
    acc.add_tool_args('{"a": 1}')
    acc.finish_tool()
    acc.start_tool(id="t2", name="write")
    acc.add_tool_args('{"b": 2}')
    acc.finish_tool()
    result = acc.finalize()
    assert len(result.tool_calls) == 2
    assert result.tool_calls[0].name == "read"
    assert result.tool_calls[1].name == "write"


def test_sequential_tool_empty_args():
    acc = StreamingAccumulator()
    acc.start_tool(id="t1", name="noop")
    acc.finish_tool()
    result = acc.finalize()
    assert result.tool_calls[0].args == {}


def test_sequential_tool_malformed_json():
    acc = StreamingAccumulator()
    acc.start_tool(id="t1", name="broken")
    acc.add_tool_args("{not valid json")
    acc.finish_tool()
    result = acc.finalize()
    assert result.tool_calls[0].args == {}


def test_finish_tool_noop_when_no_pending():
    """finish_tool() is safe to call when there's no pending tool."""
    acc = StreamingAccumulator()
    acc.finish_tool()  # should not raise
    result = acc.finalize()
    assert result.tool_calls == []


# -- Index-keyed tool calls (OpenAI Completions) ----------------------------

def test_index_keyed_single():
    acc = StreamingAccumulator()
    acc.add_tool_delta(0, id="call_1", name="search", args_delta='{"q":')
    acc.add_tool_delta(0, args_delta=' "hello"}')
    acc.finish_all_tools()
    result = acc.finalize()
    assert len(result.tool_calls) == 1
    assert result.tool_calls[0].name == "search"
    assert result.tool_calls[0].args == {"q": "hello"}
    assert result.tool_calls[0].id == "call_1"


def test_index_keyed_concurrent():
    acc = StreamingAccumulator()
    acc.add_tool_delta(0, id="c1", name="read", args_delta='{"p')
    acc.add_tool_delta(1, id="c2", name="write", args_delta='{"')
    acc.add_tool_delta(0, args_delta='ath": "a.py"}')
    acc.add_tool_delta(1, args_delta='path": "b.py"}')
    acc.finish_all_tools()
    result = acc.finalize()
    assert len(result.tool_calls) == 2
    assert result.tool_calls[0].id == "c1"
    assert result.tool_calls[0].args == {"path": "a.py"}
    assert result.tool_calls[1].id == "c2"
    assert result.tool_calls[1].args == {"path": "b.py"}


def test_index_keyed_late_id():
    """ID may arrive in a later delta than the first one."""
    acc = StreamingAccumulator()
    acc.add_tool_delta(0, name="foo", args_delta='{"x": 1}')
    acc.add_tool_delta(0, id="late_id")
    acc.finish_all_tools()
    result = acc.finalize()
    assert result.tool_calls[0].id == "late_id"


# -- Atomic tool calls (Gemini Interactions) --------------------------------

def test_atomic_tool():
    acc = StreamingAccumulator()
    acc.add_tool(ToolCall(name="search", args={"q": "test"}, id="g1"))
    result = acc.finalize()
    assert len(result.tool_calls) == 1
    assert result.tool_calls[0].name == "search"


# -- Thoughts ---------------------------------------------------------------

def test_thought_single_block():
    acc = StreamingAccumulator()
    acc.add_thought("Let me ")
    acc.add_thought("think...")
    acc.finish_thought()
    result = acc.finalize()
    assert result.thoughts == ["Let me think..."]


def test_thought_multiple_blocks():
    """Multiple thought blocks get consolidated into one entry."""
    acc = StreamingAccumulator()
    acc.add_thought("First thought.")
    acc.finish_thought()
    acc.add_thought("Second thought.")
    acc.finish_thought()
    result = acc.finalize()
    assert result.thoughts == ["First thought.Second thought."]


def test_thought_auto_closed_on_finalize():
    """Unfinished thought block is closed by finalize()."""
    acc = StreamingAccumulator()
    acc.add_thought("unfinished")
    result = acc.finalize()
    assert result.thoughts == ["unfinished"]


def test_finish_thought_noop_when_empty():
    """finish_thought() is safe when no thought deltas accumulated."""
    acc = StreamingAccumulator()
    acc.finish_thought()  # should not raise
    result = acc.finalize()
    assert result.thoughts == []


# -- Mixed content ----------------------------------------------------------

def test_text_tools_and_thoughts():
    acc = StreamingAccumulator()
    acc.add_thought("thinking...")
    acc.finish_thought()
    acc.add_text("Here is ")
    acc.start_tool(id="t1", name="search")
    acc.add_tool_args('{"q": "test"}')
    acc.finish_tool()
    acc.add_text("the answer")
    result = acc.finalize()
    assert result.text == "Here is the answer"
    assert result.thoughts == ["thinking..."]
    assert len(result.tool_calls) == 1


# -- Usage ------------------------------------------------------------------

def test_usage_passthrough():
    acc = StreamingAccumulator()
    acc.add_text("hi")
    usage = UsageMetadata(input_tokens=100, output_tokens=50, thinking_tokens=10, cached_tokens=20)
    result = acc.finalize(usage=usage)
    assert result.usage.input_tokens == 100
    assert result.usage.output_tokens == 50
    assert result.usage.thinking_tokens == 10
    assert result.usage.cached_tokens == 20


def test_default_usage():
    acc = StreamingAccumulator()
    result = acc.finalize()
    assert result.usage.input_tokens == 0
    assert result.usage.output_tokens == 0


# -- Properties -------------------------------------------------------------

def test_text_property_during_accumulation():
    acc = StreamingAccumulator()
    acc.add_text("a")
    acc.add_text("b")
    assert acc.text == "ab"


def test_tool_calls_property_during_accumulation():
    acc = StreamingAccumulator()
    acc.start_tool(id="t1", name="foo")
    acc.add_tool_args("{}")
    acc.finish_tool()
    assert len(acc.tool_calls) == 1


def test_thoughts_property_includes_unfinished():
    acc = StreamingAccumulator()
    acc.add_thought("done")
    acc.finish_thought()
    acc.add_thought("in progress")
    assert acc.thoughts == ["done", "in progress"]
