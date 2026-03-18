"""Tests for the compose capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from stoai.capabilities.compose import ComposeManager, setup as setup_compose


def make_mock_mcp(result=None):
    """Create a mock MCP client that returns the given result from call_tool."""
    mcp = MagicMock()
    mcp.call_tool.return_value = result or {"status": "success", "text": ""}
    return mcp


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    return agent


class TestComposeManager:
    def test_generate_music_via_saved_file(self, tmp_path):
        """MCP saves file to output_directory — manager finds it."""
        out_dir = tmp_path / "media" / "music"
        out_dir.mkdir(parents=True)
        saved = out_dir / "music.mp3"
        saved.write_bytes(b"FAKE_MP3")

        mcp = make_mock_mcp({"status": "success", "text": "Music saved"})
        mgr = ComposeManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"prompt": "jazz piano", "lyrics": "La la la"})
        assert result["status"] == "ok"
        assert result["file_path"] == str(saved)
        call_args = mcp.call_tool.call_args[0][1]
        assert call_args["prompt"] == "jazz piano"
        assert call_args["lyrics"] == "La la la"
        assert call_args["output_directory"] == str(out_dir)

    def test_generate_music_via_url_fallback(self, tmp_path, monkeypatch):
        """MCP returns a URL — manager downloads it."""
        url = "https://example.com/music.mp3"
        mcp = make_mock_mcp({"status": "success", "text": f"Success. Music url: {url}"})

        fake_resp = MagicMock()
        fake_resp.content = b"DOWNLOADED_MUSIC"
        fake_resp.raise_for_status = MagicMock()

        import requests as requests_mod
        monkeypatch.setattr(requests_mod, "get", lambda *a, **kw: fake_resp)

        mgr = ComposeManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"prompt": "jazz", "lyrics": "Do re mi"})
        assert result["status"] == "ok"
        path = Path(result["file_path"])
        assert path.exists()
        assert path.read_bytes() == b"DOWNLOADED_MUSIC"

    def test_mcp_error_response(self, tmp_path):
        mcp = make_mock_mcp({"status": "error", "message": "rate limited"})
        mgr = ComposeManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"prompt": "jazz", "lyrics": "La"})
        assert result["status"] == "error"
        assert "rate limited" in result["message"]

    def test_mcp_call_exception(self, tmp_path):
        mcp = MagicMock()
        mcp.call_tool.side_effect = RuntimeError("connection lost")
        mgr = ComposeManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"prompt": "jazz", "lyrics": "La"})
        assert result["status"] == "error"
        assert "connection lost" in result["message"]

    def test_missing_prompt(self, tmp_path):
        mcp = make_mock_mcp()
        mgr = ComposeManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"lyrics": "La la la"})
        assert result["status"] == "error"
        assert "prompt" in result["message"]

    def test_missing_lyrics(self, tmp_path):
        mcp = make_mock_mcp()
        mgr = ComposeManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"prompt": "jazz"})
        assert result["status"] == "error"
        assert "lyrics" in result["message"]


class TestSetupCompose:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mcp = make_mock_mcp()
        mgr = setup_compose(agent, mcp_client=mcp)
        assert isinstance(mgr, ComposeManager)
        agent.add_tool.assert_called_once()

    def test_setup_auto_creates_mcp_client(self, tmp_path, monkeypatch):
        """Without explicit mcp_client, setup auto-creates one."""
        from stoai.llm.minimax import mcp_media_client
        mock_client = MagicMock()
        monkeypatch.setattr(mcp_media_client, "create_minimax_media_client", lambda **kw: mock_client)
        agent = make_mock_agent(tmp_path)
        mgr = setup_compose(agent)
        assert isinstance(mgr, ComposeManager)
