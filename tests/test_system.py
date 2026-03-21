"""Tests for system intrinsic — runtime, lifecycle, and synchronization."""
from __future__ import annotations

import threading
import time
from unittest.mock import MagicMock

import pytest

from lingtai_kernel.base_agent import BaseAgent
from lingtai_kernel.intrinsics import ALL_INTRINSICS


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Registration
# ---------------------------------------------------------------------------


def test_system_in_all_intrinsics():
    assert "system" in ALL_INTRINSICS
    info = ALL_INTRINSICS["system"]
    assert "schema" in info
    assert "description" in info
    assert callable(info["handle"])


def test_system_wired_in_agent(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert "system" in agent._intrinsics


# ---------------------------------------------------------------------------
# show action
# ---------------------------------------------------------------------------


def test_system_show_returns_identity(tmp_path):
    agent = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._intrinsics["system"]({"action": "show"})
        assert result["status"] == "ok"
        identity = result["identity"]
        assert identity["agent_name"] == "alice"
        assert "alice" in identity["working_dir"]
        assert identity["mail_address"] is None
    finally:
        agent.stop()


def test_system_show_returns_runtime(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        time.sleep(0.1)
        result = agent._intrinsics["system"]({"action": "show"})
        runtime = result["runtime"]
        assert "T" in runtime["started_at"]
        assert runtime["uptime_seconds"] >= 0.05
    finally:
        agent.stop()


def test_system_show_returns_tokens(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._intrinsics["system"]({"action": "show"})
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


def test_system_show_with_mail_service(tmp_path):
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8301"
    agent = BaseAgent(
        agent_name="test",
        service=make_mock_service(),
        mail_service=mock_mail,
        base_dir=tmp_path,
    )
    agent.start()
    try:
        result = agent._intrinsics["system"]({"action": "show"})
        assert result["identity"]["mail_address"] == "127.0.0.1:8301"
    finally:
        agent.stop()


def test_system_show_context_null_without_session(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "show"})
    ctx = result["tokens"]["context"]
    assert ctx["window_size"] is None
    assert ctx["usage_pct"] is None


# ---------------------------------------------------------------------------
# sleep action (formerly clock.wait)
# ---------------------------------------------------------------------------


def test_mail_arrived_event_exists(tmp_path):
    """Agent should have a _mail_arrived threading.Event."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert isinstance(agent._mail_arrived, threading.Event)
    assert not agent._mail_arrived.is_set()


def test_system_sleep_with_seconds(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    start = time.monotonic()
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": 0.1})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "timeout"
    assert elapsed >= 0.09


def test_system_sleep_wakes_on_mail(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_mail():
        time.sleep(0.1)
        agent._mail_arrived.set()

    t = threading.Thread(target=fire_mail, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": 10})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "mail_arrived"
    assert elapsed < 5
    t.join(timeout=1)


def test_system_sleep_indefinite_wakes_on_mail(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_mail():
        time.sleep(0.1)
        agent._mail_arrived.set()

    t = threading.Thread(target=fire_mail, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._intrinsics["system"]({"action": "sleep"})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "mail_arrived"
    assert elapsed < 5
    t.join(timeout=1)


def test_system_sleep_caps_at_300(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._mail_arrived.set()
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": 9999})
    assert result["status"] == "ok"


def test_system_sleep_wakes_on_silence(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_silence():
        time.sleep(0.1)
        agent._cancel_event.set()

    t = threading.Thread(target=fire_silence, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": 10})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "silenced"
    assert elapsed < 5
    t.join(timeout=1)


def test_system_sleep_negative_seconds(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "sleep", "seconds": -5})
    assert result["status"] == "error"


# ---------------------------------------------------------------------------
# shutdown
# ---------------------------------------------------------------------------


def test_system_shutdown(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "shutdown", "reason": "need bash"})
    assert result["status"] == "ok"
    assert "Shutdown initiated" in result["message"]
    assert agent._shutdown.is_set()
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# restart
# ---------------------------------------------------------------------------


def test_system_restart(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "restart", "reason": "new tools"})
    assert result["status"] == "ok"
    assert "Restart initiated" in result["message"]
    assert agent._restart_requested is True
    assert agent._shutdown.is_set()
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# unknown action
# ---------------------------------------------------------------------------


def test_system_unknown_action(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "bogus"})
    assert result["status"] == "error"
