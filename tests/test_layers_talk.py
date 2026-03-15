"""Tests for the talk capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from stoai.capabilities.talk import TalkManager, setup as setup_talk


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    agent.service = MagicMock()
    return agent


class TestTalkManager:
    def test_tts_success(self, tmp_path):
        svc = MagicMock()
        svc.text_to_speech.return_value = b"FAKE_AUDIO"
        mgr = TalkManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"text": "Hello world"})
        assert result["status"] == "ok"
        path = Path(result["file_path"])
        assert path.exists()
        assert path.read_bytes() == b"FAKE_AUDIO"
        assert path.parent == tmp_path / "media" / "audio"

    def test_no_provider(self, tmp_path):
        svc = MagicMock()
        svc.text_to_speech.side_effect = RuntimeError("No tts_provider configured")
        mgr = TalkManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"text": "hello"})
        assert result["status"] == "error"

    def test_missing_text(self, tmp_path):
        svc = MagicMock()
        mgr = TalkManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({})
        assert result["status"] == "error"

    def test_empty_bytes_is_error(self, tmp_path):
        svc = MagicMock()
        svc.text_to_speech.return_value = b""
        mgr = TalkManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"text": "hello"})
        assert result["status"] == "error"


class TestSetupTalk:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mgr = setup_talk(agent)
        assert isinstance(mgr, TalkManager)
        agent.add_tool.assert_called_once()
