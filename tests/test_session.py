"""Tests for SessionManager."""
from __future__ import annotations
from unittest.mock import MagicMock
from stoai.session import SessionManager
from stoai.config import AgentConfig


def make_session_manager(**kw):
    svc = MagicMock()
    svc.model = "test-model"
    mock_session = MagicMock()
    mock_session.context_window.return_value = 100000
    mock_session.interface.estimate_context_tokens.return_value = 5000
    svc.create_session.return_value = mock_session
    config = kw.get("config", AgentConfig())
    return SessionManager(
        llm_service=svc,
        config=config,
        agent_id="test",
        streaming=False,
        build_system_prompt_fn=lambda: "test prompt",
        build_tool_schemas_fn=lambda: [],
        logger_fn=None,
    )


def test_ensure_session_creates_on_first_call():
    sm = make_session_manager()
    session = sm.ensure_session()
    assert session is not None
    assert sm.chat is not None


def test_ensure_session_reuses():
    sm = make_session_manager()
    s1 = sm.ensure_session()
    s2 = sm.ensure_session()
    assert s1 is s2


def test_get_chat_state_empty():
    sm = make_session_manager()
    assert sm.get_chat_state() == {}


def test_restore_token_state():
    sm = make_session_manager()
    sm.restore_token_state({
        "input_tokens": 500, "output_tokens": 200,
        "thinking_tokens": 50, "cached_tokens": 100, "api_calls": 3,
    })
    usage = sm.get_token_usage()
    assert usage["input_tokens"] == 500
    assert usage["api_calls"] == 3


def test_token_decomp_dirty_flag():
    sm = make_session_manager()
    assert sm.token_decomp_dirty
    sm.token_decomp_dirty = False
    assert not sm.token_decomp_dirty
