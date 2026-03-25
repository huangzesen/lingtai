# tests/test_deep_refresh.py
"""Tests for deep refresh (full agent reconstruct from init.json)."""
from __future__ import annotations

import json
import os
from pathlib import Path
from unittest.mock import MagicMock, patch


def test_resolve_env_fields_resolves_env_var(monkeypatch):
    """_resolve_env_fields replaces *_env keys with env var values."""
    from lingtai.config_resolve import _resolve_env_fields

    monkeypatch.setenv("TEST_SECRET", "hunter2")
    result = _resolve_env_fields({"api_key": None, "api_key_env": "TEST_SECRET"})
    assert result == {"api_key": "hunter2"}
    assert "api_key_env" not in result


def test_resolve_capabilities_resolves_env():
    """_resolve_capabilities applies _resolve_env_fields to each capability."""
    from lingtai.config_resolve import _resolve_capabilities

    caps = {"bash": {"policy_file": "p.json"}, "vision": {}}
    result = _resolve_capabilities(caps)
    assert result == {"bash": {"policy_file": "p.json"}, "vision": {}}


def test_resolve_addons_none():
    """_resolve_addons returns None for None/empty input."""
    from lingtai.config_resolve import _resolve_addons

    assert _resolve_addons(None) is None
    assert _resolve_addons({}) is None


def _make_init(
    capabilities: dict | None = None,
    addons: dict | None = None,
    provider: str = "openai",
    model: str = "gpt-4o",
    covenant: str = "",
    principle: str = "",
    memory: str = "",
) -> dict:
    """Build a minimal valid init.json dict."""
    data = {
        "manifest": {
            "agent_name": "test-agent",
            "language": "en",
            "llm": {
                "provider": provider,
                "model": model,
                "api_key": "test-key",
                "base_url": None,
            },
            "capabilities": capabilities or {},
            "soul": {"delay": 60},
            "stamina": 3600,
            "context_limit": None,
            "molt_pressure": 0.8,
            "molt_prompt": "",
            "max_turns": 100,
            "admin": {"karma": True},
            "streaming": False,
        },
        "principle": principle,
        "covenant": covenant,
        "memory": memory,
        "prompt": "",
    }
    if addons:
        data["addons"] = addons
    return data


def _make_agent(tmp_path: Path, init_data: dict | None = None):
    """Create a bare Agent with a mock LLM service in a temp working dir."""
    from lingtai.agent import Agent
    from lingtai_kernel.config import AgentConfig

    init = init_data or _make_init()
    (tmp_path / "init.json").write_text(json.dumps(init))

    service = MagicMock()
    service.provider = "openai"
    service.model = "gpt-4o"
    service._base_url = None

    agent = Agent(
        service,
        agent_name="test-agent",
        working_dir=tmp_path,
        config=AgentConfig(),
    )
    return agent


def test_deep_refresh_loads_new_capability(tmp_path):
    """After editing init.json to add a capability, refresh picks it up."""
    agent = _make_agent(tmp_path, _make_init(capabilities={}))
    agent._sealed = True

    mock_interface = MagicMock()
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = mock_interface
    agent._session = mock_session

    new_init = _make_init(capabilities={"read": {}})
    (tmp_path / "init.json").write_text(json.dumps(new_init))

    agent._perform_refresh()

    cap_names = [name for name, _ in agent._capabilities]
    assert "read" in cap_names
    assert agent._sealed is True


def test_deep_refresh_no_init_json_is_noop(tmp_path):
    """If init.json is missing, refresh is a no-op (no crash)."""
    agent = _make_agent(tmp_path)
    (tmp_path / "init.json").unlink()

    agent._sealed = True
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = MagicMock()
    agent._session = mock_session

    old_caps = list(agent._capabilities)
    agent._perform_refresh()
    assert agent._capabilities == old_caps


def test_deep_refresh_at_boot_no_history(tmp_path):
    """_perform_refresh works at boot time (no session, not sealed)."""
    init = _make_init(capabilities={"read": {}})
    agent = _make_agent(tmp_path, init)
    assert agent._sealed is False

    agent._perform_refresh()

    cap_names = [name for name, _ in agent._capabilities]
    assert "read" in cap_names
    assert agent._sealed is True
