"""Tests for the compose capability."""
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from stoai.capabilities.compose import ComposeManager, setup as setup_compose


def make_mock_agent(tmp_path):
    agent = MagicMock()
    agent.working_dir = tmp_path
    agent.service = MagicMock()
    return agent


class TestComposeManager:
    def test_generate_music_success(self, tmp_path):
        svc = MagicMock()
        svc.generate_music.return_value = b"FAKE_MP3_BYTES"
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "jazz piano"})
        assert result["status"] == "ok"
        path = Path(result["file_path"])
        assert path.exists()
        assert path.read_bytes() == b"FAKE_MP3_BYTES"
        assert path.parent == tmp_path / "media" / "music"

    def test_generate_music_with_duration(self, tmp_path):
        svc = MagicMock()
        svc.generate_music.return_value = b"AUDIO"
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "jazz", "duration_seconds": 30.0})
        assert result["status"] == "ok"
        svc.generate_music.assert_called_once_with("jazz", duration_seconds=30.0)

    def test_no_provider(self, tmp_path):
        svc = MagicMock()
        svc.generate_music.side_effect = RuntimeError("No music_provider configured")
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "jazz"})
        assert result["status"] == "error"

    def test_missing_prompt(self, tmp_path):
        svc = MagicMock()
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({})
        assert result["status"] == "error"

    def test_empty_bytes_is_error(self, tmp_path):
        svc = MagicMock()
        svc.generate_music.return_value = b""
        mgr = ComposeManager(working_dir=tmp_path, llm_service=svc)
        result = mgr.handle({"prompt": "jazz"})
        assert result["status"] == "error"


class TestSetupCompose:
    def test_setup_registers_tool(self, tmp_path):
        agent = make_mock_agent(tmp_path)
        mgr = setup_compose(agent)
        assert isinstance(mgr, ComposeManager)
        agent.add_tool.assert_called_once()
