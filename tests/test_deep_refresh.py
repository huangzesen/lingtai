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
        "soul": "",
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

    agent._setup_from_init()

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
    agent._setup_from_init()
    assert agent._capabilities == old_caps


def test_deep_refresh_at_boot_no_history(tmp_path):
    """_setup_from_init works at boot time (no session, not sealed)."""
    init = _make_init(capabilities={"read": {}})
    agent = _make_agent(tmp_path, init)
    assert agent._sealed is False

    agent._setup_from_init()

    cap_names = [name for name, _ in agent._capabilities]
    assert "read" in cap_names
    assert agent._sealed is True


def test_cli_build_agent_uses_refresh(tmp_path):
    """cli.build_agent() constructs agent via _setup_from_init from init.json."""
    from lingtai.cli import load_init, build_agent

    init = _make_init(capabilities={"read": {}}, covenant="Be helpful.")
    (tmp_path / "init.json").write_text(json.dumps(init))

    data = load_init(tmp_path)
    agent = build_agent(data, tmp_path)

    # Capabilities loaded from init.json via _setup_from_init
    cap_names = [name for name, _ in agent._capabilities]
    assert "read" in cap_names

    # Covenant loaded
    covenant_content = agent._prompt_manager.read_section("covenant")
    assert covenant_content is not None
    assert "Be helpful" in covenant_content

    # Cleanup
    agent._workdir.release_lock()


def test_deep_refresh_invalid_init_keeps_old_config(tmp_path):
    """If init.json is invalid, refresh logs error and keeps old state."""
    init = _make_init(capabilities={"read": {}})
    agent = _make_agent(tmp_path, init)
    agent._setup_from_init()  # initial setup

    agent._sealed = True
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = MagicMock()
    agent._session = mock_session

    # Write invalid init.json
    (tmp_path / "init.json").write_text("not json")

    old_caps = list(agent._capabilities)
    agent._setup_from_init()

    # Old capabilities preserved (refresh was a no-op)
    assert agent._capabilities == old_caps


def test_deep_refresh_removes_old_capabilities(tmp_path):
    """Capabilities removed from init.json are gone after refresh."""
    init = _make_init(capabilities={"read": {}, "write": {}})
    agent = _make_agent(tmp_path, init)
    agent._setup_from_init()  # initial setup

    agent._sealed = True
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = MagicMock()
    agent._session = mock_session

    assert len(agent._capabilities) == 2

    # Remove "write" from init.json
    new_init = _make_init(capabilities={"read": {}})
    (tmp_path / "init.json").write_text(json.dumps(new_init))

    agent._setup_from_init()

    cap_names = [name for name, _ in agent._capabilities]
    assert "read" in cap_names
    assert "write" not in cap_names


def test_deep_refresh_preserves_chat_history(tmp_path):
    """ChatInterface is passed through to _rebuild_session after refresh."""
    agent = _make_agent(tmp_path, _make_init())
    agent._sealed = True

    mock_interface = MagicMock()
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = mock_interface
    agent._session = mock_session

    agent._setup_from_init()

    mock_session._rebuild_session.assert_called_once_with(mock_interface)


def test_deep_refresh_clears_stale_prompt_sections(tmp_path):
    """Prompt sections from old capabilities don't survive refresh."""
    agent = _make_agent(tmp_path, _make_init())

    # Simulate a stale prompt section from a removed capability
    agent._prompt_manager.write_section("some_old_section", "stale content")
    assert agent._prompt_manager.read_section("some_old_section") is not None

    agent._setup_from_init()

    # Stale section should be gone
    assert agent._prompt_manager.read_section("some_old_section") is None


def test_deep_refresh_reseals(tmp_path):
    """Tool surface is re-sealed after refresh completes."""
    agent = _make_agent(tmp_path, _make_init())
    agent._sealed = True
    mock_session = MagicMock()
    mock_session.chat = MagicMock()
    mock_session.chat.interface = MagicMock()
    agent._session = mock_session

    agent._setup_from_init()

    assert agent._sealed is True
