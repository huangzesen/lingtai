"""Tests for the talk capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from stoai.capabilities.talk import TalkManager, setup as setup_talk


def make_mock_mcp(result=None):
    """Create a mock MCP client that returns the given result from call_tool."""
    mcp = MagicMock()
    mcp.call_tool.return_value = result or {"status": "success", "text": ""}
    return mcp


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    return agent


class TestTalkManager:
    def test_tts_via_saved_file(self, tmp_path):
        """MCP saves file to output_directory — manager finds it."""
        out_dir = tmp_path / "media" / "audio"
        out_dir.mkdir(parents=True)
        saved = out_dir / "speech.mp3"
        saved.write_bytes(b"FAKE_AUDIO_MP3")

        mcp = make_mock_mcp({"status": "success", "text": "Audio saved"})
        mgr = TalkManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"text": "Hello world"})
        assert result["status"] == "ok"
        assert result["file_path"] == str(saved)
        call_args = mcp.call_tool.call_args[0][1]
        assert call_args["text"] == "Hello world"
        assert call_args["output_directory"] == str(out_dir)

    def test_tts_via_url_fallback(self, tmp_path, monkeypatch):
        """MCP returns a URL — manager downloads it."""
        url = "https://example.com/audio.mp3"
        mcp = make_mock_mcp({"status": "success", "text": f"Success. Audio URL: {url}"})

        fake_resp = MagicMock()
        fake_resp.content = b"DOWNLOADED_AUDIO"
        fake_resp.raise_for_status = MagicMock()

        import requests as requests_mod
        monkeypatch.setattr(requests_mod, "get", lambda *a, **kw: fake_resp)

        mgr = TalkManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"text": "Hello"})
        assert result["status"] == "ok"
        path = Path(result["file_path"])
        assert path.exists()
        assert path.read_bytes() == b"DOWNLOADED_AUDIO"

    def test_optional_params_passed_through(self, tmp_path):
        out_dir = tmp_path / "media" / "audio"
        out_dir.mkdir(parents=True)
        (out_dir / "speech.mp3").write_bytes(b"FAKE")

        mcp = make_mock_mcp()
        mgr = TalkManager(working_dir=tmp_path, mcp_client=mcp)
        mgr.handle({"text": "Hi", "voice_id": "English_Trustworth_Man", "emotion": "sad", "speed": 1.5})
        call_args = mcp.call_tool.call_args[0][1]
        assert call_args["voice_id"] == "English_Trustworth_Man"
        assert call_args["emotion"] == "sad"
        assert call_args["speed"] == 1.5

    def test_mcp_error_response(self, tmp_path):
        mcp = make_mock_mcp({"status": "error", "message": "quota exceeded"})
        mgr = TalkManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"text": "hello"})
        assert result["status"] == "error"
        assert "quota exceeded" in result["message"]

    def test_mcp_call_exception(self, tmp_path):
        mcp = MagicMock()
        mcp.call_tool.side_effect = RuntimeError("connection lost")
        mgr = TalkManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({"text": "hello"})
        assert result["status"] == "error"
        assert "connection lost" in result["message"]

    def test_missing_text(self, tmp_path):
        mcp = make_mock_mcp()
        mgr = TalkManager(working_dir=tmp_path, mcp_client=mcp)
        result = mgr.handle({})
        assert result["status"] == "error"
        assert "text" in result["message"]


class TestSetupTalk:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mcp = make_mock_mcp()
        mgr = setup_talk(agent, mcp_client=mcp)
        assert isinstance(mgr, TalkManager)
        agent.add_tool.assert_called_once()

    def test_setup_auto_creates_mcp_client(self, tmp_path, monkeypatch):
        """Without explicit mcp_client, setup auto-creates one."""
        from stoai.llm.minimax import mcp_media_client
        mock_client = MagicMock()
        monkeypatch.setattr(mcp_media_client, "create_minimax_media_client", lambda **kw: mock_client)
        agent = make_mock_agent(tmp_path)
        mgr = setup_talk(agent)
        assert isinstance(mgr, TalkManager)
