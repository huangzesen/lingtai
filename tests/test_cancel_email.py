"""Tests for cancel email intrinsic — mail-based agent cancellation."""
from __future__ import annotations

import threading
from unittest.mock import MagicMock, patch

import pytest

from stoai.base_agent import BaseAgent
from stoai.state import AgentState
from stoai.message import MSG_REQUEST
from stoai.config import AgentConfig
from stoai.llm import LLMResponse, ToolCall


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Cancel event and mail storage
# ---------------------------------------------------------------------------


def test_cancel_email_sets_event_and_stores_mail(tmp_path):
    """Cancel-type email should set _cancel_event and store the payload."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert not agent._cancel_event.is_set()
    assert agent._cancel_mail is None

    payload = {
        "from": "boss",
        "to": "test",
        "subject": "stop now",
        "message": "halt all work",
        "type": "cancel",
    }
    agent._on_mail_received(payload)

    assert agent._cancel_event.is_set()
    assert agent._cancel_mail is payload


def test_cancel_email_bypasses_mail_queue(tmp_path):
    """Cancel-type email should NOT be added to the normal mail queue."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    payload = {
        "from": "boss",
        "to": "test",
        "subject": "stop",
        "message": "stop",
        "type": "cancel",
    }
    agent._on_mail_received(payload)

    assert len(agent._mail_queue) == initial_count


def test_normal_email_queued_as_usual(tmp_path):
    """Normal-type email should go through the regular queue path."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    payload = {
        "from": "colleague",
        "to": "test",
        "subject": "hello",
        "message": "hi there",
        "type": "normal",
    }
    agent._on_mail_received(payload)

    assert len(agent._mail_queue) == initial_count + 1
    assert not agent._cancel_event.is_set()


def test_missing_type_defaults_to_normal(tmp_path):
    """Mail without a type field should be treated as normal."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    payload = {
        "from": "colleague",
        "to": "test",
        "subject": "hello",
        "message": "hi there",
    }
    agent._on_mail_received(payload)

    assert len(agent._mail_queue) == initial_count + 1
    assert not agent._cancel_event.is_set()


def test_unrecognized_type_treated_as_normal(tmp_path):
    """Unrecognized mail type should be logged as warning and treated as normal."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    payload = {
        "from": "someone",
        "to": "test",
        "subject": "test",
        "message": "test",
        "type": "unknown_type",
    }
    agent._on_mail_received(payload)

    assert len(agent._mail_queue) == initial_count + 1
    assert not agent._cancel_event.is_set()


# ---------------------------------------------------------------------------
# Cancelling flag prevents re-entrant cancel
# ---------------------------------------------------------------------------


def test_second_cancel_overwrites_first(tmp_path):
    """A second cancel email overwrites the first — last writer wins."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent._cancel_mail = {"from": "first_boss"}

    second_payload = {
        "from": "second_boss",
        "to": "test",
        "subject": "stop again",
        "message": "stop",
        "type": "cancel",
    }
    agent._on_mail_received(second_payload)

    assert agent._cancel_mail["from"] == "second_boss"
    assert agent._cancel_event.is_set()


# ---------------------------------------------------------------------------
# Diary flow
# ---------------------------------------------------------------------------


def test_handle_cancel_diary_produces_llm_call(tmp_path):
    """Diary flow should make one LLM call and return the diary text."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent._cancel_mail = {
        "from": "boss",
        "subject": "stop",
        "message": "halt work",
    }
    agent._cancel_event.set()

    # Mock the chat session
    mock_chat = MagicMock()
    mock_response = MagicMock()
    mock_response.text = "I was working on task X, got through steps 1-3."
    mock_chat.send.return_value = mock_response
    agent._chat = mock_chat

    result = agent._handle_cancel_diary()

    assert result["text"] == "I was working on task X, got through steps 1-3."
    assert result["failed"] is False
    assert result["errors"] == []
    assert not agent._cancel_event.is_set()  # cleared
    assert agent._cancel_mail is None  # cleared
    # _cancelling removed — no re-entrancy flag needed
    mock_chat.send.assert_called_once()
    # Verify the prompt includes cancel email info
    prompt = mock_chat.send.call_args[0][0]
    assert "boss" in prompt
    assert "halt work" in prompt


def test_handle_cancel_diary_handles_llm_failure(tmp_path):
    """If the diary LLM call fails, return a fallback message."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent._cancel_mail = {
        "from": "boss",
        "subject": "stop",
        "message": "halt",
    }
    agent._cancel_event.set()

    mock_chat = MagicMock()
    mock_chat.send.side_effect = RuntimeError("LLM down")
    agent._chat = mock_chat

    result = agent._handle_cancel_diary()

    assert "boss" in result["text"]
    assert "LLM down" in result["text"]
    assert result["failed"] is False
    assert agent._cancel_mail is None


def test_handle_cancel_diary_without_chat(tmp_path):
    """Diary flow without an active chat session should return empty text."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent._cancel_mail = {"from": "boss", "subject": "stop", "message": "halt"}
    agent._cancel_event.set()
    agent._chat = None

    result = agent._handle_cancel_diary()

    assert result["text"] == ""
    assert result["failed"] is False


# ---------------------------------------------------------------------------
# Cancel check in sequential tool execution
# ---------------------------------------------------------------------------


def test_sequential_execution_stops_on_cancel(tmp_path):
    """Sequential tool execution should return empty when cancel event is set."""
    from stoai.loop_guard import LoopGuard
    from stoai.tool_executor import ToolExecutor

    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent._cancel_event.set()

    tc = ToolCall(name="read", args={"file_path": "/tmp/test"}, id="tc1")
    guard = LoopGuard(max_total_calls=10)
    errors: list[str] = []

    executor = ToolExecutor(
        dispatch_fn=agent._dispatch_tool,
        make_tool_result_fn=lambda name, result, **kw: agent.service.make_tool_result(
            name, result, provider=agent._config.provider, **kw
        ),
        guard=guard,
        known_tools=set(agent._intrinsics) | set(agent._mcp_handlers),
        logger_fn=agent._log,
    )
    results, intercepted, text = executor.execute(
        [tc], cancel_event=agent._cancel_event, collected_errors=errors,
    )

    assert results == []
    assert intercepted is False


# ---------------------------------------------------------------------------
# Admin privilege gate
# ---------------------------------------------------------------------------


def test_non_admin_cannot_send_cancel_mail(tmp_path):
    """Non-admin agent should get an error when trying to send cancel mail."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path, admin=False,
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send",
        "address": "127.0.0.1:8001",
        "subject": "stop",
        "message": "stop",
        "type": "cancel",
    })

    assert "error" in result
    assert "admin" in result["error"].lower()


def test_admin_can_send_cancel_mail(tmp_path):
    """Admin agent should be able to send cancel mail."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path, admin=True,
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    mock_mail.send.return_value = None
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send",
        "address": "127.0.0.1:8001",
        "subject": "stop",
        "message": "stop now",
        "type": "cancel",
    })

    assert result["status"] == "delivered"
    # Verify the payload includes type
    call_args = mock_mail.send.call_args
    payload = call_args[0][1]
    assert payload["type"] == "cancel"


def test_non_admin_can_send_normal_mail(tmp_path):
    """Non-admin agent should be able to send normal mail (default type)."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path, admin=False,
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    mock_mail.send.return_value = None
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send",
        "address": "127.0.0.1:8001",
        "subject": "hello",
        "message": "hi there",
    })

    assert result["status"] == "delivered"


# ---------------------------------------------------------------------------
# Internal cancel event is always created
# ---------------------------------------------------------------------------


def test_cancel_event_always_created(tmp_path):
    """Agent should always have an internal _cancel_event, no external injection."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert isinstance(agent._cancel_event, threading.Event)
    assert not agent._cancel_event.is_set()


def test_admin_flag_stored(tmp_path):
    """Admin flag should be stored on the agent."""
    agent_normal = BaseAgent(agent_id="a", service=make_mock_service(), base_dir=tmp_path)
    assert agent_normal._admin is False

    agent_admin = BaseAgent(agent_id="b", service=make_mock_service(), base_dir=tmp_path, admin=True)
    assert agent_admin._admin is True
