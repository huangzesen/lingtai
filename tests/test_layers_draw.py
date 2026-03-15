"""Tests for the draw capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from stoai.capabilities.draw import DrawManager, setup as setup_draw


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    agent.service = MagicMock()
    return agent


class TestDrawManager:
    def test_generate_image_success(self, tmp_path):
        svc = MagicMock()
        svc.generate_image.return_value = b"\x89PNG_FAKE_BYTES"
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "ok"
        assert "file_path" in result
        path = Path(result["file_path"])
        assert path.exists()
        assert path.read_bytes() == b"\x89PNG_FAKE_BYTES"
        assert path.parent == tmp_path / "media" / "images"

    def test_generate_image_no_provider(self, tmp_path):
        svc = MagicMock()
        svc.generate_image.side_effect = RuntimeError("No image_provider configured")
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "error"
        assert "image_provider" in result["message"]

    def test_generate_image_not_implemented(self, tmp_path):
        svc = MagicMock()
        svc.generate_image.side_effect = NotImplementedError
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "error"

    def test_missing_prompt(self, tmp_path):
        svc = MagicMock()
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({})
        assert result["status"] == "error"

    def test_empty_bytes_is_error(self, tmp_path):
        svc = MagicMock()
        svc.generate_image.return_value = b""
        mgr = DrawManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "error"


class TestSetupDraw:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mgr = setup_draw(agent)
        assert isinstance(mgr, DrawManager)
        agent.add_tool.assert_called_once()


class TestAddCapabilityIntegration:
    def test_add_capability_draw(self, tmp_path):
        from stoai.agent import BaseAgent
        from unittest.mock import MagicMock
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = BaseAgent(agent_id="test", service=svc, working_dir=str(tmp_path))
        mgr = agent.add_capability("draw")
        assert "draw" in agent._mcp_handlers
