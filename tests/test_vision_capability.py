"""Tests for vision capability."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from stoai.agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_vision_added_by_capability(tmp_path):
    """add_capability('vision') should register the vision tool."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_capability("vision")
    assert "vision" in agent._mcp_handlers


def test_vision_analyzes_image(tmp_path):
    """Vision capability should analyze an image file."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mock_response = MagicMock()
    mock_response.text = "A cat sitting on a table"
    agent.service.generate_vision = MagicMock(return_value=mock_response)
    agent.add_capability("vision")
    img_path = agent.working_dir / "test.png"
    img_path.write_bytes(b"\x89PNG fake image data")
    result = agent._mcp_handlers["vision"]({"image_path": str(img_path)})
    assert result["status"] == "ok"
    assert "cat" in result["analysis"]


def test_vision_with_dedicated_service(tmp_path):
    """Vision capability should use VisionService if provided."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mock_vision_svc = MagicMock()
    mock_vision_svc.analyze_image.return_value = "A dog in the park"
    agent.add_capability("vision", vision_service=mock_vision_svc)
    img_path = agent.working_dir / "test.jpg"
    img_path.write_bytes(b"\xff\xd8\xff fake jpeg")
    result = agent._mcp_handlers["vision"]({"image_path": str(img_path)})
    assert result["status"] == "ok"
    assert "dog" in result["analysis"]
    mock_vision_svc.analyze_image.assert_called_once()


def test_vision_missing_image(tmp_path):
    """Vision should return error for missing image file."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_capability("vision")
    result = agent._mcp_handlers["vision"]({"image_path": "/nonexistent/image.png"})
    assert result.get("status") == "error"


def test_vision_relative_path(tmp_path):
    """Vision should resolve relative paths against working directory."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mock_response = MagicMock()
    mock_response.text = "An image"
    agent.service.generate_vision = MagicMock(return_value=mock_response)
    agent.add_capability("vision")
    img_path = agent.working_dir / "photo.png"
    img_path.write_bytes(b"\x89PNG fake")
    result = agent._mcp_handlers["vision"]({"image_path": "photo.png"})
    assert result["status"] == "ok"
