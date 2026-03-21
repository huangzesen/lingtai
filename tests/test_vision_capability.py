"""Tests for vision capability."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from lingtai.agent import Agent
from lingtai.capabilities.vision import VisionManager


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_vision_added_by_capability(tmp_path):
    """capabilities=['vision'] should register the vision tool."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["vision"])
    assert "vision" in agent._mcp_handlers


def test_vision_analyzes_image_via_adapter(tmp_path):
    """VisionManager should call adapter.generate_vision() directly."""
    svc = make_mock_service()
    adapter = MagicMock()
    mock_response = MagicMock()
    mock_response.text = "A cat sitting on a table"
    adapter.generate_vision.return_value = mock_response
    svc.get_adapter.return_value = adapter
    svc._get_provider_defaults.return_value = {"model": "gemini-test"}

    agent = MagicMock()
    agent.service = svc
    agent._working_dir = tmp_path

    mgr = VisionManager(agent, vision_provider="gemini")
    img_path = tmp_path / "test.png"
    img_path.write_bytes(b"\x89PNG fake image data")
    result = mgr.handle({"image_path": str(img_path)})
    assert result["status"] == "ok"
    assert "cat" in result["analysis"]
    adapter.generate_vision.assert_called_once()


def test_vision_with_dedicated_service(tmp_path):
    """Vision capability should use VisionService if provided."""
    mock_vision_svc = MagicMock()
    mock_vision_svc.analyze_image.return_value = "A dog in the park"
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities={"vision": {"vision_service": mock_vision_svc}})
    img_path = agent.working_dir / "test.jpg"
    img_path.write_bytes(b"\xff\xd8\xff fake jpeg")
    result = agent._mcp_handlers["vision"]({"image_path": str(img_path)})
    assert result["status"] == "ok"
    assert "dog" in result["analysis"]
    mock_vision_svc.analyze_image.assert_called_once()


def test_vision_missing_image(tmp_path):
    """Vision should return error for missing image file."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["vision"])
    result = agent._mcp_handlers["vision"]({"image_path": "/nonexistent/image.png"})
    assert result.get("status") == "error"


def test_vision_relative_path_via_adapter(tmp_path):
    """VisionManager should resolve relative paths against working directory."""
    svc = make_mock_service()
    adapter = MagicMock()
    mock_response = MagicMock()
    mock_response.text = "An image"
    adapter.generate_vision.return_value = mock_response
    svc.get_adapter.return_value = adapter
    svc._get_provider_defaults.return_value = {"model": "gemini-test"}

    agent = MagicMock()
    agent.service = svc
    agent._working_dir = tmp_path

    mgr = VisionManager(agent, vision_provider="gemini")
    img_path = tmp_path / "photo.png"
    img_path.write_bytes(b"\x89PNG fake")
    result = mgr.handle({"image_path": "photo.png"})
    assert result["status"] == "ok"


def test_vision_falls_back_to_main_provider(tmp_path):
    """Vision without explicit provider should fall back to agent's main provider."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["vision"])
    img_path = agent.working_dir / "test.png"
    img_path.write_bytes(b"\x89PNG fake")
    adapter = agent.service.get_adapter.return_value
    adapter.generate_vision.return_value = MagicMock(text="a photo")
    result = agent._mcp_handlers["vision"]({"image_path": str(img_path)})
    assert result["status"] == "ok"
    agent.service.get_adapter.assert_called_with("gemini")
