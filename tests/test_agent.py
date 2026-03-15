"""Tests for BaseAgent lifecycle and tool dispatch."""
import time
import threading
from unittest.mock import MagicMock

import pytest

from stoai.agent import BaseAgent, Message, AgentState, _make_message, MSG_REQUEST
from stoai.types import MCPTool, AgentNotConnectedError, UnknownToolError
from stoai.config import AgentConfig


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Lifecycle
# ---------------------------------------------------------------------------

def test_agent_starts_and_stops():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.start()
    assert agent.state == AgentState.SLEEPING
    agent.stop(timeout=2.0)


def test_agent_double_start():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.start()
    agent.start()  # should be no-op
    assert agent.state == AgentState.SLEEPING
    agent.stop(timeout=2.0)


# ---------------------------------------------------------------------------
# Intrinsics filtering
# ---------------------------------------------------------------------------

def test_intrinsics_enabled_by_default():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    assert "read" in agent._intrinsics
    assert "write" in agent._intrinsics
    assert "talk" in agent._intrinsics
    assert "vision" in agent._intrinsics
    assert "web_search" in agent._intrinsics
    # diary, plan, manage_system_prompt are now layers, not intrinsics
    assert "manage_system_prompt" not in agent._intrinsics
    assert "manage_diary" not in agent._intrinsics
    assert "plan" not in agent._intrinsics
    assert len(agent._intrinsics) == 8  # read, edit, write, glob, grep, talk, vision, web_search


def test_disabled_intrinsics():
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        disabled_intrinsics={"talk", "vision"},
    )
    assert "talk" not in agent._intrinsics
    assert "vision" not in agent._intrinsics
    assert "read" in agent._intrinsics


def test_enabled_intrinsics():
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        enabled_intrinsics={"read", "write"},
    )
    assert "read" in agent._intrinsics
    assert "write" in agent._intrinsics
    assert "talk" not in agent._intrinsics
    assert "vision" not in agent._intrinsics


def test_enabled_and_disabled_raises():
    with pytest.raises(ValueError, match="Cannot specify both"):
        BaseAgent(
            agent_id="test",
            service=make_mock_service(),
            enabled_intrinsics={"read"},
            disabled_intrinsics={"talk"},
        )


# ---------------------------------------------------------------------------
# MCP tools / add / remove
# ---------------------------------------------------------------------------

def test_add_remove_tool():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.add_tool("custom", schema={"type": "object"}, handler=lambda args: {"ok": True})
    assert "custom" in agent._mcp_handlers
    agent.remove_tool("custom")
    assert "custom" not in agent._mcp_handlers


def test_mcp_tools_registered():
    tool = MCPTool(name="domain_tool", schema={}, description="test", handler=lambda a: {"r": 1})
    agent = BaseAgent(agent_id="test", service=make_mock_service(), mcp_tools=[tool])
    assert "domain_tool" in agent._mcp_handlers


def test_add_tool_replaces_existing():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.add_tool("custom", schema={}, handler=lambda args: {"v": 1})
    agent.add_tool("custom", schema={}, handler=lambda args: {"v": 2})
    assert agent._mcp_handlers["custom"]({})=={"v": 2}


def test_remove_nonexistent_tool_is_noop():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.remove_tool("nonexistent")  # should not raise


# ---------------------------------------------------------------------------
# System prompt sections
# ---------------------------------------------------------------------------

def test_system_prompt_sections():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.update_system_prompt("role", "You are a test agent", protected=True)
    assert agent._prompt_manager.read_section("role") == "You are a test agent"


def test_system_prompt_update_marks_dirty():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent._token_decomp_dirty = False
    agent.update_system_prompt("info", "some info")
    assert agent._token_decomp_dirty is True


# ---------------------------------------------------------------------------
# Agent connections
# ---------------------------------------------------------------------------

def test_connect_agents():
    a = BaseAgent(agent_id="a", service=make_mock_service())
    b = BaseAgent(agent_id="b", service=make_mock_service())
    a.connect("b", b)
    assert "b" in a._connections


def test_talk_unconnected_raises():
    a = BaseAgent(agent_id="a", service=make_mock_service())
    with pytest.raises(AgentNotConnectedError):
        a.talk("nonexistent", "hello")


def test_talk_sends_to_connected():
    a = BaseAgent(agent_id="a", service=make_mock_service())
    b = BaseAgent(agent_id="b", service=make_mock_service())
    a.connect("b", b)
    a.talk("b", "hello")
    assert not b.inbox.empty()
    msg = b.inbox.get_nowait()
    assert msg.content == "hello"
    assert msg.sender == "a"


# ---------------------------------------------------------------------------
# Events
# ---------------------------------------------------------------------------

def test_on_event_callback():
    events = []
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        on_event=lambda t, p: events.append((t, p)),
    )
    agent._emit_event("test_event", {"data": 42})
    assert len(events) == 1
    assert events[0][0] == "test_event"
    assert events[0][1]["data"] == 42
    assert events[0][1]["agent_id"] == "test"


def test_no_event_callback_is_noop():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent._emit_event("test_event", {"data": 42})  # should not raise


# ---------------------------------------------------------------------------
# Token usage
# ---------------------------------------------------------------------------

def test_token_usage():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    usage = agent.get_token_usage()
    assert isinstance(usage, dict)
    assert "input_tokens" in usage
    assert "output_tokens" in usage
    assert "api_calls" in usage
    assert usage["input_tokens"] == 0
    assert usage["api_calls"] == 0


# ---------------------------------------------------------------------------
# Message
# ---------------------------------------------------------------------------

def test_message_type():
    msg = Message(type="request", content="hello", sender="user")
    assert msg.type == "request"
    assert msg.content == "hello"


def test_make_message():
    msg = _make_message(MSG_REQUEST, "user", "hello")
    assert msg.type == MSG_REQUEST
    assert msg.sender == "user"
    assert msg.content == "hello"
    assert msg.id.startswith("msg_")


def test_message_reply_event():
    event = threading.Event()
    msg = _make_message(MSG_REQUEST, "user", "hello", reply_event=event)
    assert msg._reply_event is event
    msg._reply_value = {"text": "world"}
    event.set()
    assert event.is_set()


# ---------------------------------------------------------------------------
# Tool dispatch
# ---------------------------------------------------------------------------

def test_execute_single_tool_intrinsic():
    """Intrinsic tools should be callable via _execute_single_tool."""
    from stoai.llm.base import ToolCall
    agent = BaseAgent(agent_id="test", service=make_mock_service())

    # Replace the read intrinsic with a mock
    agent._intrinsics["read"] = lambda args: {"status": "ok", "content": "test"}

    tc = ToolCall(name="read", args={"file_path": "/tmp/test.txt"})
    result = agent._dispatch_tool(tc)
    assert result["status"] == "ok"


def test_execute_single_tool_mcp():
    """MCP tools should be callable via _dispatch_tool."""
    from stoai.llm.base import ToolCall
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.add_tool("my_tool", schema={}, handler=lambda args: {"status": "ok", "value": args.get("x")})

    tc = ToolCall(name="my_tool", args={"x": 42})
    result = agent._dispatch_tool(tc)
    assert result["status"] == "ok"
    assert result["value"] == 42


def test_execute_single_tool_unknown():
    """Unknown tools should raise UnknownToolError."""
    from stoai.llm.base import ToolCall
    agent = BaseAgent(agent_id="test", service=make_mock_service())

    tc = ToolCall(name="nonexistent_tool", args={})
    with pytest.raises(UnknownToolError):
        agent._dispatch_tool(tc)


# ---------------------------------------------------------------------------
# Context (opaque)
# ---------------------------------------------------------------------------

def test_context_stored_opaque():
    ctx = {"custom": "data", "nested": [1, 2, 3]}
    agent = BaseAgent(agent_id="test", service=make_mock_service(), context=ctx)
    assert agent._context is ctx


# ---------------------------------------------------------------------------
# Working dir
# ---------------------------------------------------------------------------

def test_working_dir_resolved():
    agent = BaseAgent(agent_id="test", service=make_mock_service(), working_dir="/tmp/test")
    assert str(agent._working_dir) == "/tmp/test"


def test_working_dir_default():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    # Default working dir should be current directory
    assert agent._working_dir is not None


# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

def test_config_defaults():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    assert agent._config.max_turns == 50


def test_config_override():
    config = AgentConfig(max_turns=10, provider="anthropic")
    agent = BaseAgent(agent_id="test", service=make_mock_service(), config=config)
    assert agent._config.max_turns == 10
    assert agent._config.provider == "anthropic"


# ---------------------------------------------------------------------------
# Status
# ---------------------------------------------------------------------------

def test_status():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    s = agent.status()
    assert s["agent_id"] == "test"
    assert s["state"] == "sleeping"
    assert s["idle"] is True
    assert "tokens" in s


# ---------------------------------------------------------------------------
# Public send API
# ---------------------------------------------------------------------------

def test_send_fires_message():
    """send(wait=False) should put a message in the inbox."""
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.send("hello", wait=False)
    assert not agent.inbox.empty()
    msg = agent.inbox.get_nowait()
    assert msg.content == "hello"
    assert msg.type == MSG_REQUEST
