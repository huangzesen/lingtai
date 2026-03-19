"""Tests for the vibing capability — idle-breaking sticky notes."""
from __future__ import annotations

import subprocess
import time
from unittest.mock import MagicMock, patch

import pytest

from stoai.agent import Agent
from stoai.capabilities.vibing import (
    VibingManager,
    DEFAULT_VIBE,
    setup,
)


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def git_init(agent):
    """Initialize git in agent's working dir for tests that need git commits."""
    wd = str(agent._working_dir)
    subprocess.run(["git", "init"], cwd=wd, capture_output=True)
    subprocess.run(
        ["git", "commit", "--allow-empty", "-m", "init"],
        cwd=wd, capture_output=True,
    )


# ---------------------------------------------------------------------------
# Registration
# ---------------------------------------------------------------------------

def test_vibing_registered_as_capability(tmp_path):
    """vibing registers the 'vibing' tool."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    assert "vibing" in agent._mcp_handlers
    assert ("vibing", {}) in agent._capabilities
    agent.stop(timeout=1.0)


def test_vibing_get_capability(tmp_path):
    """get_capability returns VibingManager."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    assert isinstance(agent.get_capability("vibing"), VibingManager)
    agent.stop(timeout=1.0)


def test_vibing_custom_interval(tmp_path):
    """Custom interval is passed through."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 60}},
    )
    assert agent.get_capability("vibing")._interval == 60
    agent.stop(timeout=1.0)


def test_system_intrinsic_unchanged(tmp_path):
    """vibing does NOT replace the system intrinsic."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    assert callable(agent._intrinsics["system"])
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Switch toggle
# ---------------------------------------------------------------------------

def test_switch_on(tmp_path):
    """switch enabled=true activates the timer."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 9999}},
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "switch", "enabled": True})
    assert result["status"] == "activated"
    assert mgr._active is True
    assert mgr._timer is not None
    mgr.stop()
    agent.stop(timeout=1.0)


def test_switch_on_already_active(tmp_path):
    """switch on when already active returns already_active."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 9999}},
    )
    mgr = agent.get_capability("vibing")
    mgr.handle({"action": "switch", "enabled": True})
    result = mgr.handle({"action": "switch", "enabled": True})
    assert result["status"] == "already_active"
    mgr.stop()
    agent.stop(timeout=1.0)


def test_switch_off(tmp_path):
    """switch enabled=false deactivates."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 9999}},
    )
    mgr = agent.get_capability("vibing")
    mgr.handle({"action": "switch", "enabled": True})
    result = mgr.handle({"action": "switch", "enabled": False})
    assert result["status"] == "deactivated"
    assert mgr._active is False
    assert mgr._timer is None
    agent.stop(timeout=1.0)


def test_switch_off_already_inactive(tmp_path):
    """switch off when inactive returns already_inactive."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "switch", "enabled": False})
    assert result["status"] == "already_inactive"
    agent.stop(timeout=1.0)


def test_switch_missing_enabled(tmp_path):
    """switch without enabled returns error."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "switch"})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Vibe — write the sticky note
# ---------------------------------------------------------------------------

def test_vibe_update(tmp_path):
    """vibe updates the prompt."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({
        "action": "vibe",
        "prompt": "What patterns remain?",
        "reasoning": "Early exploration",
    })
    assert result["status"] == "updated"
    assert result["prompt"] == "What patterns remain?"
    assert mgr._prompt == "What patterns remain?"
    agent.stop(timeout=1.0)


def test_vibe_missing_prompt(tmp_path):
    """vibe without prompt returns error."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "vibe"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_unknown_action(tmp_path):
    """Unknown action returns error."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "bogus"})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Nudge delivery + git
# ---------------------------------------------------------------------------

def test_nudge_fires_when_idle(tmp_path):
    """Nudge is sent via agent.send() after interval when idle."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.4)
        mgr.stop()

    mock_send.assert_called()
    call_args = mock_send.call_args
    assert call_args[0][0] == DEFAULT_VIBE
    assert call_args[1]["sender"] == "vibing"
    agent.stop(timeout=2.0)


def test_nudge_skips_when_active(tmp_path):
    """When agent is ACTIVE, nudge reschedules instead of sending."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    agent._idle.clear()  # Force ACTIVE

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    mock_send.assert_not_called()
    agent._idle.set()
    agent.stop(timeout=2.0)


def test_nudge_uses_updated_prompt(tmp_path):
    """Nudge uses the most recent vibe prompt."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "vibe", "prompt": "What now?", "reasoning": "test"})
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    mock_send.assert_called()
    assert mock_send.call_args[0][0] == "What now?"
    agent.stop(timeout=2.0)


def test_nudge_writes_vibe_md(tmp_path):
    """Each nudge writes vibing/vibe.md."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    with patch.object(agent, "send"):
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    vibe_md = agent._working_dir / "vibing" / "vibe.md"
    assert vibe_md.is_file()
    content = vibe_md.read_text()
    assert "Last vibe:" in content
    assert DEFAULT_VIBE.strip() in content
    agent.stop(timeout=2.0)


def test_nudge_git_commits(tmp_path):
    """Each nudge creates a git commit."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    with patch.object(agent, "send"):
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.4)
        mgr.stop()

    # Check git log for vibe commits
    result = subprocess.run(
        ["git", "log", "--oneline", "--all"],
        cwd=str(agent._working_dir),
        capture_output=True, text=True,
    )
    assert "vibing: vibe" in result.stdout
    agent.stop(timeout=2.0)


# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

def test_stop_cancels_timer(tmp_path):
    """stop() cancels the timer and deactivates."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 9999}},
    )
    mgr = agent.get_capability("vibing")
    mgr.handle({"action": "switch", "enabled": True})
    assert mgr._active is True
    mgr.stop()
    assert mgr._active is False
    assert mgr._timer is None
    agent.stop(timeout=1.0)
