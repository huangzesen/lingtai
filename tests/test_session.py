"""Tests for SessionManager — LLM session, token tracking, compaction."""
from __future__ import annotations

from unittest.mock import MagicMock, patch, PropertyMock

import pytest

from lingtai_kernel.session import SessionManager
from lingtai_kernel.config import AgentConfig


def make_session_manager(**kw):
    svc = MagicMock()
    svc.model = "test-model"
    mock_session = MagicMock()
    mock_session.context_window.return_value = 100000
    mock_session.interface.estimate_context_tokens.return_value = 5000
    mock_session.interface.current_system_prompt = "test prompt"
    mock_session.send.return_value = MagicMock(
        text="hello", tool_calls=[], thoughts=[], usage=MagicMock(
            input_tokens=100, output_tokens=50, thinking_tokens=10, cached_tokens=20,
        ),
    )
    svc.create_session.return_value = mock_session
    svc.check_and_compact.return_value = None  # no compaction by default
    config = kw.get("config", AgentConfig())
    return SessionManager(
        llm_service=svc,
        config=config,
        agent_id="abc123def456",
        agent_name="test",
        streaming=kw.get("streaming", False),
        build_system_prompt_fn=lambda: "test prompt",
        build_tool_schemas_fn=lambda: [],
        logger_fn=kw.get("logger_fn", None),
    ), svc, mock_session


# ------------------------------------------------------------------
# Session lifecycle
# ------------------------------------------------------------------

def test_ensure_session_creates_on_first_call():
    sm, svc, _ = make_session_manager()
    session = sm.ensure_session()
    assert session is not None
    assert sm.chat is not None
    svc.create_session.assert_called_once()


def test_ensure_session_reuses():
    sm, svc, _ = make_session_manager()
    s1 = sm.ensure_session()
    s2 = sm.ensure_session()
    assert s1 is s2
    assert svc.create_session.call_count == 1


def test_ensure_session_passes_interaction_id():
    sm, svc, _ = make_session_manager()
    sm.interaction_id = "int-123"
    sm.ensure_session()
    call_kwargs = svc.create_session.call_args[1]
    assert call_kwargs["interaction_id"] == "int-123"


# ------------------------------------------------------------------
# send() — the core operation
# ------------------------------------------------------------------

def test_send_happy_path():
    sm, svc, mock_session = make_session_manager()
    response = sm.send("hello")
    assert response.text == "hello"
    # Should have created session, called send_with_timeout, tracked usage
    svc.create_session.assert_called_once()


def test_send_tracks_usage():
    sm, _, _ = make_session_manager()
    sm.send("hello")
    usage = sm.get_token_usage()
    assert usage["input_tokens"] == 100
    assert usage["output_tokens"] == 50
    assert usage["api_calls"] == 1


def test_send_does_not_call_compaction():
    sm, svc, _ = make_session_manager()
    sm.send("hello")
    # Compaction is no longer auto-triggered from SessionManager.send()
    svc.check_and_compact.assert_not_called()


def test_send_stale_interaction_recovery():
    """When a stale interaction error occurs, send() should create a new session and retry."""
    sm, svc, mock_session = make_session_manager()
    sm.interaction_id = "stale-id"

    # Make send_with_timeout raise a stale interaction error on first call
    from lingtai_kernel.llm_utils import _is_stale_interaction_error
    stale_error = Exception("interaction not found")

    call_count = [0]
    def fake_send_with_timeout(**kwargs):
        call_count[0] += 1
        if call_count[0] == 1:
            raise stale_error
        return mock_session.send.return_value

    with patch("lingtai_kernel.session.send_with_timeout", side_effect=fake_send_with_timeout), \
         patch("lingtai_kernel.session._is_stale_interaction_error", return_value=True):
        response = sm.send("hello")

    assert response.text == "hello"
    # interaction_id should be cleared after stale error
    assert svc.create_session.call_count == 2  # initial + recovery


def test_send_non_stale_error_propagates():
    """Non-stale errors should propagate normally."""
    sm, _, _ = make_session_manager()

    with patch("lingtai_kernel.session.send_with_timeout", side_effect=ValueError("real error")), \
         patch("lingtai_kernel.session._is_stale_interaction_error", return_value=False):
        with pytest.raises(ValueError, match="real error"):
            sm.send("hello")


def test_send_preserves_interaction_id():
    sm, _, mock_session = make_session_manager()
    mock_session.interaction_id = "new-id"

    with patch("lingtai_kernel.session.send_with_timeout", return_value=mock_session.send.return_value):
        sm.send("hello")

    assert sm.interaction_id == "new-id"


def test_send_logs_llm_call():
    events = []
    def log_fn(event_type, **fields):
        events.append(event_type)
    sm, _, _ = make_session_manager(logger_fn=log_fn)
    sm.send("hello")
    assert "llm_call" in events
    assert "llm_response" in events


# ------------------------------------------------------------------
# _on_reset() — rollback on server error
# ------------------------------------------------------------------

def test_on_reset_creates_new_session():
    sm, svc, _ = make_session_manager()
    sm.ensure_session()

    # Build a mock chat with interface
    mock_chat = MagicMock()
    mock_iface = MagicMock()
    mock_iface.last_assistant_entry.return_value = None
    mock_iface.entries = []
    mock_chat.interface = mock_iface

    new_session = MagicMock()
    svc.create_session.return_value = new_session

    result_chat, rollback_msg = sm._on_reset(mock_chat, "failed msg")

    assert result_chat is new_session
    assert "server error" in rollback_msg
    assert "no tool calls found" in rollback_msg


def test_on_reset_summarizes_tool_calls():
    sm, svc, _ = make_session_manager()
    sm.ensure_session()

    from lingtai_kernel.llm.interface import ToolCallBlock

    mock_chat = MagicMock()
    mock_iface = MagicMock()
    # Simulate a failed assistant turn with tool calls
    tool_block = ToolCallBlock(name="read_file", args={"path": "/tmp/test"}, id="tc1")
    mock_entry = MagicMock()
    mock_entry.content = [tool_block]
    mock_iface.last_assistant_entry.return_value = mock_entry
    mock_iface.entries = []
    mock_chat.interface = mock_iface

    new_session = MagicMock()
    svc.create_session.return_value = new_session

    _, rollback_msg = sm._on_reset(mock_chat, "failed")

    assert "read_file" in rollback_msg
    assert "/tmp/test" in rollback_msg


def test_on_reset_drops_trailing_turns():
    sm, svc, _ = make_session_manager()
    sm.ensure_session()

    mock_chat = MagicMock()
    mock_iface = MagicMock()
    mock_iface.last_assistant_entry.return_value = None
    mock_iface.entries = []
    mock_chat.interface = mock_iface

    svc.create_session.return_value = MagicMock()
    sm._on_reset(mock_chat, "failed")

    # Should have called drop_trailing twice (assistant turn, then tool results)
    assert mock_iface.drop_trailing.call_count == 2


def test_on_reset_passes_interface_to_new_session():
    sm, svc, _ = make_session_manager()
    sm.ensure_session()

    mock_chat = MagicMock()
    mock_iface = MagicMock()
    mock_iface.last_assistant_entry.return_value = None
    mock_iface.entries = []
    mock_chat.interface = mock_iface

    svc.create_session.return_value = MagicMock()
    sm._on_reset(mock_chat, "failed")

    # The new session should receive the interface for history continuity
    call_kwargs = svc.create_session.call_args[1]
    assert call_kwargs["interface"] is mock_iface
    assert call_kwargs["tracked"] is False


# ------------------------------------------------------------------
# get_context_pressure()
# ------------------------------------------------------------------

def test_get_context_pressure_no_session():
    sm, svc, _ = make_session_manager()
    # No session yet — should return 0.0
    assert sm.get_context_pressure() == 0.0


def test_get_context_pressure_with_session():
    sm, svc, mock_session = make_session_manager()
    sm.ensure_session()
    mock_session.context_window.return_value = 100_000
    mock_session.interface.estimate_context_tokens.return_value = 85_000
    pressure = sm.get_context_pressure()
    assert pressure == 0.85


def test_get_context_pressure_zero_window():
    sm, svc, mock_session = make_session_manager()
    sm.ensure_session()
    mock_session.context_window.return_value = 0
    assert sm.get_context_pressure() == 0.0


def test_compaction_warnings_initialized():
    sm, _, _ = make_session_manager()
    assert sm._compaction_warnings == 0


# ------------------------------------------------------------------
# _track_usage()
# ------------------------------------------------------------------

def test_track_usage_accumulates():
    sm, _, _ = make_session_manager()
    response = MagicMock()
    response.usage.input_tokens = 100
    response.usage.output_tokens = 50
    response.usage.thinking_tokens = 10
    response.usage.cached_tokens = 20

    sm._track_usage(response)
    usage = sm.get_token_usage()
    assert usage["input_tokens"] == 100
    assert usage["output_tokens"] == 50
    assert usage["thinking_tokens"] == 10
    assert usage["cached_tokens"] == 20
    assert usage["api_calls"] == 1

    # Second call accumulates
    sm._track_usage(response)
    usage = sm.get_token_usage()
    assert usage["input_tokens"] == 200
    assert usage["api_calls"] == 2


def test_track_usage_triggers_decomposition_update():
    sm, _, _ = make_session_manager()
    assert sm.token_decomp_dirty  # starts dirty
    response = MagicMock()
    response.usage.input_tokens = 100
    response.usage.output_tokens = 50
    response.usage.thinking_tokens = 0
    response.usage.cached_tokens = 0
    sm._track_usage(response)
    assert not sm.token_decomp_dirty  # updated during track_usage


# ------------------------------------------------------------------
# Token usage
# ------------------------------------------------------------------

def test_get_token_usage_default():
    sm, _, _ = make_session_manager()
    usage = sm.get_token_usage()
    assert usage["input_tokens"] == 0
    assert usage["output_tokens"] == 0
    assert usage["total_tokens"] == 0
    assert usage["api_calls"] == 0


def test_restore_token_state():
    sm, _, _ = make_session_manager()
    sm.restore_token_state({
        "input_tokens": 500, "output_tokens": 200,
        "thinking_tokens": 50, "cached_tokens": 100, "api_calls": 3,
    })
    usage = sm.get_token_usage()
    assert usage["input_tokens"] == 500
    assert usage["output_tokens"] == 200
    assert usage["api_calls"] == 3


# ------------------------------------------------------------------
# Session persistence
# ------------------------------------------------------------------

def test_get_chat_state_empty():
    sm, _, _ = make_session_manager()
    assert sm.get_chat_state() == {}


def test_get_chat_state_with_session():
    sm, _, mock_session = make_session_manager()
    sm.ensure_session()
    mock_session.interface.to_dict.return_value = [{"role": "user", "content": "hi"}]
    state = sm.get_chat_state()
    assert "messages" in state
    assert state["messages"] == [{"role": "user", "content": "hi"}]


def test_restore_chat_with_state():
    sm, svc, _ = make_session_manager()
    restored = MagicMock()
    svc.resume_session.return_value = restored
    sm.restore_chat({"messages": [{"role": "user"}]})
    assert sm.chat is restored


def test_restore_chat_fallback_on_error():
    sm, svc, _ = make_session_manager()
    svc.resume_session.side_effect = ValueError("bad state")
    sm.restore_chat({"messages": [{"role": "user"}]})
    # Should fallback to ensure_session
    assert sm.chat is not None
    svc.create_session.assert_called_once()


def test_restore_chat_empty_state():
    sm, svc, _ = make_session_manager()
    sm.restore_chat({})
    # Should call ensure_session
    assert sm.chat is not None
    svc.create_session.assert_called_once()


# ------------------------------------------------------------------
# Properties
# ------------------------------------------------------------------

def test_token_decomp_dirty_flag():
    sm, _, _ = make_session_manager()
    assert sm.token_decomp_dirty
    sm.token_decomp_dirty = False
    assert not sm.token_decomp_dirty


def test_interaction_id_property():
    sm, _, _ = make_session_manager()
    assert sm.interaction_id is None
    sm.interaction_id = "int-456"
    assert sm.interaction_id == "int-456"


def test_intermediate_text_streamed_property():
    sm, _, _ = make_session_manager()
    assert not sm.intermediate_text_streamed
    sm.intermediate_text_streamed = True
    assert sm.intermediate_text_streamed


# ------------------------------------------------------------------
# Cleanup
# ------------------------------------------------------------------

def test_close_shuts_down_pool():
    sm, _, _ = make_session_manager()
    sm.close()
    # Should not raise on second close
    sm.close()
