"""Tests for status intrinsic — agent self-inspection."""
from __future__ import annotations

import time
from unittest.mock import MagicMock

import pytest

from stoai.base_agent import BaseAgent
from stoai.intrinsics import ALL_INTRINSICS


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_status_in_all_intrinsics():
    assert "status" in ALL_INTRINSICS
    info = ALL_INTRINSICS["status"]
    assert "schema" in info
    assert "description" in info
    assert callable(info["handle"])


def test_status_wired_in_agent(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "status" in agent._intrinsics


def test_status_show_returns_identity(tmp_path):
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._intrinsics["status"]({"action": "show"})
        assert result["status"] == "ok"
        identity = result["identity"]
        assert identity["agent_id"] == "alice"
        assert "alice" in identity["working_dir"]
        assert identity["mail_address"] is None
    finally:
        agent.stop()


def test_status_show_returns_runtime(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        time.sleep(0.1)
        result = agent._intrinsics["status"]({"action": "show"})
        runtime = result["runtime"]
        assert "T" in runtime["started_at"]
        assert runtime["uptime_seconds"] >= 0.05
    finally:
        agent.stop()


def test_status_show_returns_tokens(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._intrinsics["status"]({"action": "show"})
        tokens = result["tokens"]
        assert "input_tokens" in tokens
        assert "output_tokens" in tokens
        assert "total_tokens" in tokens
        assert "api_calls" in tokens
        assert "context" in tokens
        ctx = tokens["context"]
        assert "window_size" in ctx
        assert "usage_pct" in ctx
    finally:
        agent.stop()


def test_status_show_with_mail_service(tmp_path):
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8301"
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        mail_service=mock_mail,
        base_dir=tmp_path,
    )
    agent.start()
    try:
        result = agent._intrinsics["status"]({"action": "show"})
        assert result["identity"]["mail_address"] == "127.0.0.1:8301"
    finally:
        agent.stop()


def test_status_show_context_null_without_session(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["status"]({"action": "show"})
    ctx = result["tokens"]["context"]
    assert ctx["window_size"] is None
    assert ctx["usage_pct"] is None


def test_status_unknown_action(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["status"]({"action": "bogus"})
    assert "error" in result


def test_status_shutdown(tmp_path):
    """status(action='shutdown') should set the shutdown event."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["status"]({"action": "shutdown", "reason": "need bash"})
    assert result["status"] == "ok"
    assert "Shutdown initiated" in result["message"]
    assert agent._shutdown.is_set()
    agent.stop(timeout=1.0)
