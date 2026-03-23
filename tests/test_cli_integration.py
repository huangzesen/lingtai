"""Integration test: lingtai run boots an agent and shuts down via .quell."""
from __future__ import annotations

import json
import time
from pathlib import Path
from unittest.mock import MagicMock, patch

from lingtai.cli import load_init, build_agent, write_pid, remove_pid
from lingtai_kernel.state import AgentState


def _write_init(tmp_path: Path) -> None:
    """Write a minimal init.json into tmp_path."""
    data = {
        "manifest": {
            "agent_name": "integration-test",
            "language": "en",
            "llm": {
                "provider": "gemini",
                "model": "test-model",
                "api_key": "fake-key",
                "base_url": None,
            },
            "capabilities": {},
            "soul": {"delay": 5},
            "vigil": 10,
            "max_turns": 5,
            "admin": {},
            "streaming": False,
        },
        "covenant": "You are a test agent.",
        "memory": "",
        "prompt": "",
    }
    (tmp_path / "init.json").write_text(json.dumps(data))


def _make_mock_service():
    """Build a mock LLMService that satisfies BaseAgent's contract."""
    svc = MagicMock()
    svc.provider = "gemini"
    svc.model = "test-model"
    svc.get_adapter.return_value = MagicMock()
    return svc


@patch("lingtai.cli.LLMService")
def test_full_boot_and_quell_shutdown(mock_llm_cls, tmp_path):
    """Boot agent, touch .quell, verify clean shutdown."""
    _write_init(tmp_path)
    mock_llm_cls.return_value = _make_mock_service()

    data = load_init(tmp_path)
    agent = build_agent(data, tmp_path)
    write_pid(tmp_path)

    agent.start()

    # Verify agent is running and files are created
    assert agent.state == AgentState.IDLE
    assert (tmp_path / ".agent.pid").is_file()
    assert (tmp_path / ".agent.json").is_file()

    # Touch .quell to trigger shutdown via heartbeat
    (tmp_path / ".quell").touch()

    # Wait for heartbeat to pick it up (beats every ~1s)
    time.sleep(3)

    assert agent._shutdown.is_set()
    assert agent.state == AgentState.DORMANT
    assert not (tmp_path / ".quell").exists(), "signal file should be deleted"

    remove_pid(tmp_path)
    assert not (tmp_path / ".agent.pid").is_file()


@patch("lingtai.cli.LLMService")
def test_load_init_and_build_agent(mock_llm_cls, tmp_path):
    """load_init + build_agent produce a valid Agent without crashing."""
    _write_init(tmp_path)
    mock_llm_cls.return_value = _make_mock_service()

    data = load_init(tmp_path)
    agent = build_agent(data, tmp_path)

    assert agent.agent_name == "integration-test"
    assert agent._config.max_turns == 5
    assert agent._config.language == "en"


@patch("lingtai.cli.LLMService")
def test_pid_lifecycle(mock_llm_cls, tmp_path):
    """write_pid creates .agent.pid, remove_pid deletes it."""
    _write_init(tmp_path)

    write_pid(tmp_path)
    pid_file = tmp_path / ".agent.pid"
    assert pid_file.is_file()
    assert pid_file.read_text().strip().isdigit()

    remove_pid(tmp_path)
    assert not pid_file.is_file()

    # remove_pid on missing file is a no-op
    remove_pid(tmp_path)
