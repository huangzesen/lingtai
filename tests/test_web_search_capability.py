"""Tests for web_search capability."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from stoai.agent import Agent
from stoai.capabilities.web_search import WebSearchManager


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_web_search_added_by_capability(tmp_path):
    """capabilities=['web_search'] should register the web_search tool."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["web_search"])
    assert "web_search" in agent._mcp_handlers


def test_web_search_calls_adapter_directly():
    """WebSearchManager should call adapter.web_search() directly."""
    from stoai.llm.base import LLMResponse

    svc = MagicMock()
    adapter = MagicMock()
    adapter.web_search.return_value = LLMResponse(text="Python is a programming language...")
    svc.get_adapter.return_value = adapter
    svc._get_provider_defaults.return_value = {"model": "gemini-test"}

    agent = MagicMock()
    agent.service = svc

    mgr = WebSearchManager(agent, web_search_provider="gemini")
    result = mgr.handle({"query": "what is python"})
    assert result["status"] == "ok"
    assert "Python" in result["results"]
    adapter.web_search.assert_called_once()


def test_web_search_with_dedicated_service(tmp_path):
    """web_search capability should use SearchService if provided."""
    mock_result = MagicMock()
    mock_result.title = "Python"
    mock_result.url = "https://python.org"
    mock_result.snippet = "Python programming language"
    mock_search_svc = MagicMock()
    mock_search_svc.search.return_value = [mock_result]
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities={"web_search": {"search_service": mock_search_svc}})
    result = agent._mcp_handlers["web_search"]({"query": "python"})
    assert result["status"] == "ok"
    assert "Python" in result["results"]
    mock_search_svc.search.assert_called_once()


def test_web_search_missing_query(tmp_path):
    """web_search should return error for missing query."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["web_search"])
    result = agent._mcp_handlers["web_search"]({"query": ""})
    assert result.get("status") == "error"


def test_web_search_no_provider_returns_error(tmp_path):
    """web_search without provider or service should return a clear error."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["web_search"])
    result = agent._mcp_handlers["web_search"]({"query": "test query"})
    assert result["status"] == "error"
    assert "provider" in result["message"].lower()
