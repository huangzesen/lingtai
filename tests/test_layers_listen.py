"""Tests for the listen capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from stoai.capabilities.listen import ListenManager, setup as setup_listen


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    agent.service = MagicMock()
    return agent


class TestListenManager:
    def test_transcribe_success(self, tmp_path):
        svc = MagicMock()
        svc.transcribe.return_value = "Hello world"
        audio_file = tmp_path / "test.mp3"
        audio_file.write_bytes(b"FAKE_AUDIO")
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"audio_path": str(audio_file)})
        assert result["status"] == "ok"
        assert result["text"] == "Hello world"

    def test_transcribe_relative_path(self, tmp_path):
        svc = MagicMock()
        svc.transcribe.return_value = "Hi"
        audio_file = tmp_path / "test.mp3"
        audio_file.write_bytes(b"AUDIO")
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"audio_path": "test.mp3"})
        assert result["status"] == "ok"
        assert result["text"] == "Hi"

    def test_analyze_mode(self, tmp_path):
        svc = MagicMock()
        svc.analyze_audio.return_value = "This is jazz music"
        audio_file = tmp_path / "song.mp3"
        audio_file.write_bytes(b"AUDIO")
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({
            "audio_path": str(audio_file),
            "mode": "analyze",
            "prompt": "What genre is this?",
        })
        assert result["status"] == "ok"
        assert result["text"] == "This is jazz music"
        svc.analyze_audio.assert_called_once()

    def test_file_not_found(self, tmp_path):
        svc = MagicMock()
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"audio_path": "/nonexistent/file.mp3"})
        assert result["status"] == "error"
        assert "not found" in result["message"]

    def test_missing_audio_path(self, tmp_path):
        svc = MagicMock()
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({})
        assert result["status"] == "error"

    def test_no_provider(self, tmp_path):
        svc = MagicMock()
        svc.transcribe.side_effect = RuntimeError("No audio_provider configured")
        audio_file = tmp_path / "test.mp3"
        audio_file.write_bytes(b"AUDIO")
        mgr = ListenManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"audio_path": str(audio_file)})
        assert result["status"] == "error"


class TestSetupListen:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mgr = setup_listen(agent)
        assert isinstance(mgr, ListenManager)
        agent.add_tool.assert_called_once()
