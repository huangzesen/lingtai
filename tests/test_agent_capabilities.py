"""Tests for Agent — capabilities layer."""
from __future__ import annotations

import pytest
from unittest.mock import MagicMock
from lingtai.agent import Agent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_agent_no_capabilities(tmp_path):
    """Agent with no capabilities works like BaseAgent."""
    agent = Agent(service=make_mock_service(), agent_name="test", base_dir=tmp_path)
    assert agent._capabilities == []
    assert agent._capability_managers == {}
    agent.stop(timeout=1.0)


def test_agent_capabilities_list(tmp_path):
    """capabilities= as list of strings registers capabilities."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", base_dir=tmp_path,
        capabilities=["vision", "web_search"],
    )
    assert len(agent._capabilities) == 2
    assert ("vision", {}) in agent._capabilities
    assert ("web_search", {}) in agent._capabilities
    assert "vision" in agent._mcp_handlers
    assert "web_search" in agent._mcp_handlers
    agent.stop(timeout=1.0)


def test_agent_capabilities_dict(tmp_path):
    """capabilities= as dict registers capabilities with kwargs."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", base_dir=tmp_path,
        capabilities={"vision": {}, "web_search": {}},
    )
    assert len(agent._capabilities) == 2
    assert "vision" in agent._mcp_handlers
    agent.stop(timeout=1.0)


def test_agent_get_capability(tmp_path):
    """get_capability() returns the manager instance."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", base_dir=tmp_path,
        capabilities=["vision"],
    )
    mgr = agent.get_capability("vision")
    assert mgr is not None
    assert agent.get_capability("nonexistent") is None
    agent.stop(timeout=1.0)


def test_agent_seal_after_start(tmp_path):
    """add_tool() raises after start() on Agent too."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", base_dir=tmp_path,
        capabilities=["vision"],
    )
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda a: {}, description="x")
    finally:
        agent.stop(timeout=2.0)
