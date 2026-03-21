"""Tests for the draw capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai.capabilities.draw import DrawManager, setup as setup_draw


def make_mock_mcp(result=None):
    """Create a mock MCP client that returns the given result from call_tool."""
    mcp = MagicMock()
    mcp.call_tool.return_value = result or {"status": "success", "text": ""}
    return mcp


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    return agent


class TestDrawManager:
    def test_generate_image_via_saved_file(self, tmp_path):
        """MCP saves file to output_directory — manager finds it."""
        out_dir = tmp_path / "media" / "images"
        out_dir.mkdir(parents=True)
        saved = out_dir / "generated.jpeg"
        saved.write_bytes(b"\xff\xd8JPEG_FAKE")

        mcp = make_mock_mcp({"status": "success", "text": "Image saved"})
        mgr = DrawManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"prompt": "a cute cat"})
        assert result["status"] == "ok"
        assert result["file_path"] == str(saved)
        mcp.call_tool.assert_called_once()
        call_args = mcp.call_tool.call_args
        assert call_args[0][0] == "text_to_image"
        assert call_args[0][1]["prompt"] == "a cute cat"
        assert call_args[0][1]["output_directory"] == str(out_dir)

    def test_generate_image_via_url_fallback(self, tmp_path, monkeypatch):
        """MCP returns a URL — manager downloads it."""
        url = "https://example.com/image.jpeg"
        mcp = make_mock_mcp({"status": "success", "text": f"Success. Image URLs: ['{url}']"})

        fake_resp = MagicMock()
        fake_resp.content = b"\xff\xd8JPEG_DOWNLOADED"
        fake_resp.raise_for_status = MagicMock()

        import lingtai.capabilities.draw as draw_mod
        monkeypatch.setattr(draw_mod.requests, "get", lambda *a, **kw: fake_resp)

        mgr = DrawManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"prompt": "a sunset"})
        assert result["status"] == "ok"
        path = Path(result["file_path"])
        assert path.exists()
        assert path.read_bytes() == b"\xff\xd8JPEG_DOWNLOADED"

    def test_aspect_ratio_passed_through(self, tmp_path):
        out_dir = tmp_path / "media" / "images"
        out_dir.mkdir(parents=True)
        (out_dir / "img.jpeg").write_bytes(b"FAKE")

        mcp = make_mock_mcp()
        mgr = DrawManager(working_dir=tmp_path, mcp_client=mcp)
        mgr.handle({"prompt": "wide shot", "aspect_ratio": "16:9"})
        call_args = mcp.call_tool.call_args[0][1]
        assert call_args["aspect_ratio"] == "16:9"

    def test_mcp_error_response(self, tmp_path):
        mcp = make_mock_mcp({"status": "error", "message": "rate limited"})
        mgr = DrawManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"prompt": "a cat"})
        assert result["status"] == "error"
        assert "rate limited" in result["message"]

    def test_mcp_call_exception(self, tmp_path):
        mcp = MagicMock()
        mcp.call_tool.side_effect = RuntimeError("connection lost")
        mgr = DrawManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"prompt": "a cat"})
        assert result["status"] == "error"
        assert "connection lost" in result["message"]

    def test_missing_prompt(self, tmp_path):
        mcp = make_mock_mcp()
        mgr = DrawManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({})
        assert result["status"] == "error"
        assert "prompt" in result["message"]


class TestSetupDraw:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mcp = make_mock_mcp()
        mgr = setup_draw(agent, mcp_client=mcp)
        assert isinstance(mgr, DrawManager)
        agent.add_tool.assert_called_once()

    def test_setup_auto_creates_mcp_client(self, tmp_path, monkeypatch):
        """Without explicit mcp_client, setup auto-creates one."""
        from lingtai.llm.minimax import mcp_media_client
        mock_client = MagicMock()
        monkeypatch.setattr(mcp_media_client, "create_minimax_media_client", lambda **kw: mock_client)
        agent = make_mock_agent(tmp_path)
        mgr = setup_draw(agent)
        assert isinstance(mgr, DrawManager)


class TestAddCapabilityIntegration:
    def test_add_capability_draw(self, tmp_path):
        from unittest.mock import MagicMock
        from lingtai.agent import Agent
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        mcp = make_mock_mcp()
        agent = Agent(agent_name="test", service=svc, base_dir=tmp_path,
                           capabilities={"draw": {"mcp_client": mcp}})
        assert "draw" in agent._mcp_handlers
