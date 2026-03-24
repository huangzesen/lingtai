"""Integration test: lingtai run boots an agent, tests .sleep (asleep) and .suspend (shutdown)."""
from __future__ import annotations

import json
import time
from pathlib import Path
from unittest.mock import MagicMock, patch

from lingtai.cli import load_init, build_agent
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
            "stamina": 10,
            "context_limit": None,
            "molt_pressure": 0.8,
            "molt_prompt": "",
            "max_turns": 5,
            "admin": {},
            "streaming": False,
        },
        "principle": "",
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
def test_sleep_signal_triggers_asleep(mock_llm_cls, tmp_path):
    """Boot agent, touch .sleep, verify ASLEEP (sleep, not shutdown)."""
    _write_init(tmp_path)
    mock_llm_cls.return_value = _make_mock_service()

    data = load_init(tmp_path)
    agent = build_agent(data, tmp_path)

    agent.start()

    assert agent.state == AgentState.IDLE
    assert (tmp_path / ".agent.json").is_file()

    # Touch .sleep → ASLEEP (sleep, process stays alive)
    (tmp_path / ".sleep").touch()
    time.sleep(3)

    assert agent._asleep.is_set()
    assert not agent._shutdown.is_set(), ".sleep should NOT set _shutdown"
    assert agent.state == AgentState.ASLEEP
    assert not (tmp_path / ".sleep").exists(), "signal file should be deleted"

    agent._shutdown.set()  # clean up for test teardown
    agent.stop()


@patch("lingtai.cli.LLMService")
def test_suspend_triggers_shutdown(mock_llm_cls, tmp_path):
    """Boot agent, touch .suspend, verify SUSPENDED (full shutdown)."""
    _write_init(tmp_path)
    mock_llm_cls.return_value = _make_mock_service()

    data = load_init(tmp_path)
    agent = build_agent(data, tmp_path)

    agent.start()

    assert agent.state == AgentState.IDLE

    # Touch .suspend → SUSPENDED (process death)
    (tmp_path / ".suspend").touch()
    time.sleep(3)

    assert agent._shutdown.is_set()
    assert agent.state == AgentState.SUSPENDED
    assert not (tmp_path / ".suspend").exists(), "signal file should be deleted"

    agent.stop()


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
