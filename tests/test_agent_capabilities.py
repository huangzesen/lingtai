"""Tests for Agent — capabilities layer."""
from __future__ import annotations

import pytest
from unittest.mock import MagicMock
from lingtai.agent import Agent
from lingtai.services.vision import VisionService
from lingtai.services.websearch import SearchService, SearchResult


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


class FakeVisionService(VisionService):
    def analyze_image(self, image_path, prompt=None):
        return "fake"


class FakeSearchService(SearchService):
    def search(self, query, max_results=5):
        return [SearchResult(title="t", url="u", snippet="s")]


def test_agent_no_capabilities(tmp_path):
    """Agent with no capabilities works like BaseAgent."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    assert agent._capabilities == []
    assert agent._capability_managers == {}
    agent.stop(timeout=1.0)


def test_agent_capabilities_list(tmp_path):
    """capabilities= as list of strings registers capabilities (using file caps that need no key)."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["read", "write"],
    )
    assert len(agent._capabilities) == 2
    assert ("read", {}) in agent._capabilities
    assert ("write", {}) in agent._capabilities
    agent.stop(timeout=1.0)


def test_agent_capabilities_dict(tmp_path):
    """capabilities= as dict registers capabilities with kwargs."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities={
            "vision": {"vision_service": FakeVisionService()},
            "web_search": {"search_service": FakeSearchService()},
        },
    )
    assert len(agent._capabilities) == 2
    assert "vision" in agent._tool_handlers
    assert "web_search" in agent._tool_handlers
    agent.stop(timeout=1.0)


def test_agent_get_capability(tmp_path):
    """get_capability() returns the manager instance."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities={"vision": {"vision_service": FakeVisionService()}},
    )
    mgr = agent.get_capability("vision")
    assert mgr is not None
    assert agent.get_capability("nonexistent") is None
    agent.stop(timeout=1.0)


def test_agent_seal_after_start(tmp_path):
    """add_tool() raises after start() on Agent too."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities={"vision": {"vision_service": FakeVisionService()}},
    )
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda a: {}, description="x")
    finally:
        agent.stop(timeout=2.0)


def test_vision_requires_provider(tmp_path):
    """Vision capability raises ValueError without provider or service."""
    with pytest.raises(ValueError, match="vision capability requires"):
        Agent(
            service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
            capabilities=["vision"],
        )


def test_web_search_requires_provider(tmp_path):
    """Web search capability raises ValueError without provider or service."""
    with pytest.raises(ValueError, match="web_search capability requires"):
        Agent(
            service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
            capabilities=["web_search"],
        )
