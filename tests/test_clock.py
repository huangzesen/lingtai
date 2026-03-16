"""Tests for clock intrinsic."""
from __future__ import annotations

import threading
import time
from unittest.mock import MagicMock

import pytest

from stoai.agent import BaseAgent
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
