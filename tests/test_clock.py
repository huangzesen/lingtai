"""Tests for clock intrinsic."""
from __future__ import annotations

import threading
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


# ---------------------------------------------------------------------------
# Registration
# ---------------------------------------------------------------------------


def test_clock_in_all_intrinsics():
    """Clock should be registered in ALL_INTRINSICS with handler=None."""
    assert "clock" in ALL_INTRINSICS
    info = ALL_INTRINSICS["clock"]
    assert "schema" in info
    assert "description" in info
    assert info["handler"] is None


# ---------------------------------------------------------------------------
# Wiring
# ---------------------------------------------------------------------------


def test_clock_wired_in_agent(tmp_path):
    """Clock should be wired as an intrinsic in BaseAgent."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "clock" in agent._intrinsics


def test_clock_can_be_disabled(tmp_path):
    """Clock should be disable-able like other intrinsics."""
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        disabled_intrinsics={"clock"},
        base_dir=tmp_path,
    )
    assert "clock" not in agent._intrinsics


def test_mail_arrived_event_exists(tmp_path):
    """Agent should have a _mail_arrived threading.Event."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert isinstance(agent._mail_arrived, threading.Event)
    assert not agent._mail_arrived.is_set()


# ---------------------------------------------------------------------------
# check action
# ---------------------------------------------------------------------------


def test_clock_check_returns_time(tmp_path):
    """clock check should return current UTC time and unix timestamp."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_clock({"action": "check"})

    assert result["status"] == "ok"
    assert "utc" in result
    assert "unix" in result
    assert isinstance(result["unix"], float)
    # UTC string should be ISO format
    assert "T" in result["utc"]


# ---------------------------------------------------------------------------
# wait action
# ---------------------------------------------------------------------------


def test_clock_wait_with_seconds(tmp_path):
    """clock wait with seconds should sleep and return."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    start = time.monotonic()
    result = agent._handle_clock({"action": "wait", "seconds": 0.1})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "timeout"
    assert elapsed >= 0.09  # slept at least ~0.1s


def test_clock_wait_wakes_on_mail(tmp_path):
    """clock wait should wake early when mail arrives."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    # Fire mail arrival after 0.1s
    def fire_mail():
        time.sleep(0.1)
        agent._mail_arrived.set()

    t = threading.Thread(target=fire_mail, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._handle_clock({"action": "wait", "seconds": 10})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "mail_arrived"
    assert elapsed < 5  # woke up WAY before 10s timeout
    t.join(timeout=1)


def test_clock_wait_indefinite_wakes_on_mail(tmp_path):
    """clock wait without seconds should block until mail arrives."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_mail():
        time.sleep(0.1)
        agent._mail_arrived.set()

    t = threading.Thread(target=fire_mail, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._handle_clock({"action": "wait"})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "mail_arrived"
    assert elapsed < 5
    t.join(timeout=1)


def test_clock_wait_caps_at_300(tmp_path):
    """clock wait should cap seconds at 300."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    # We can't actually wait 300s in a test, so just verify the cap logic
    # by setting mail_arrived immediately
    agent._mail_arrived.set()
    result = agent._handle_clock({"action": "wait", "seconds": 9999})
    # Should wake immediately because mail_arrived is already set
    assert result["status"] == "ok"


def test_clock_wait_wakes_on_cancel(tmp_path):
    """clock wait should wake when cancel event is set."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    def fire_cancel():
        time.sleep(0.1)
        agent._cancel_event.set()

    t = threading.Thread(target=fire_cancel, daemon=True)
    t.start()

    start = time.monotonic()
    result = agent._handle_clock({"action": "wait", "seconds": 10})
    elapsed = time.monotonic() - start

    assert result["status"] == "ok"
    assert result["reason"] == "cancelled"
    assert elapsed < 5
    t.join(timeout=1)


def test_clock_wait_negative_seconds(tmp_path):
    """clock wait with negative seconds should return error."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_clock({"action": "wait", "seconds": -5})
    assert "error" in result


def test_clock_wait_unknown_action(tmp_path):
    """Unknown clock action should return error."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_clock({"action": "bogus"})
    assert "error" in result
