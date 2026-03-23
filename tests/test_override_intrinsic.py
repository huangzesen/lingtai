"""Tests for BaseAgent.override_intrinsic() — capability upgrade mechanism."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from lingtai_kernel.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_override_intrinsic_removes_from_dict(tmp_path):
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    assert "eigen" in agent._intrinsics
    agent.override_intrinsic("eigen")
    assert "eigen" not in agent._intrinsics
    agent.stop(timeout=1.0)


def test_override_intrinsic_returns_original_handler(tmp_path):
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    original = agent._intrinsics["eigen"]
    returned = agent.override_intrinsic("eigen")
    assert returned is original
    agent.stop(timeout=1.0)


def test_override_intrinsic_raises_after_start(tmp_path):
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.override_intrinsic("eigen")
    finally:
        agent.stop(timeout=2.0)


def test_override_intrinsic_raises_unknown(tmp_path):
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    with pytest.raises(KeyError):
        agent.override_intrinsic("nonexistent")
    agent.stop(timeout=1.0)


def test_override_intrinsic_tool_no_longer_visible(tmp_path):
    """After override, the intrinsic should not appear in tool schemas."""
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    agent.override_intrinsic("eigen")
    schemas = agent._build_tool_schemas()
    schema_names = [s.name for s in schemas]
    assert "eigen" not in schema_names
    agent.stop(timeout=1.0)
