"""Tests for web_search capability."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from stoai.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_web_search_added_by_capability(tmp_path):
    """add_capability('web_search') should register the web_search tool."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_capability("web_search")
    assert "web_search" in agent._mcp_handlers


def test_web_search_returns_results(tmp_path):
    """web_search capability should return search results."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mock_response = MagicMock()
    mock_response.text = "Python is a programming language..."
    agent.service.web_search = MagicMock(return_value=mock_response)
    agent.add_capability("web_search")
    result = agent._mcp_handlers["web_search"]({"query": "what is python"})
    assert result["status"] == "ok"
    assert "Python" in result["results"]


def test_web_search_with_dedicated_service(tmp_path):
    """web_search capability should use SearchService if provided."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mock_result = MagicMock()
    mock_result.title = "Python"
    mock_result.url = "https://python.org"
    mock_result.snippet = "Python programming language"
    mock_search_svc = MagicMock()
    mock_search_svc.search.return_value = [mock_result]
    agent.add_capability("web_search", search_service=mock_search_svc)
    result = agent._mcp_handlers["web_search"]({"query": "python"})
    assert result["status"] == "ok"
    assert "Python" in result["results"]
    mock_search_svc.search.assert_called_once()


def test_web_search_missing_query(tmp_path):
    """web_search should return error for missing query."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_capability("web_search")
    result = agent._mcp_handlers["web_search"]({"query": ""})
    assert result.get("status") == "error"
