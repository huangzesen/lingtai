"""Tests for the conscience capability (hormê — periodic inner voice)."""
from __future__ import annotations

import subprocess
import time
from unittest.mock import MagicMock, patch

import pytest

from stoai.agent import Agent
from stoai.capabilities.conscience import (
    ConscienceManager,
    DEFAULT_PROMPT,
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

def test_conscience_registered_as_capability(tmp_path):
    """conscience registers the 'conscience' tool."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    assert "conscience" in agent._mcp_handlers
    assert ("conscience", {}) in agent._capabilities
    agent.stop(timeout=1.0)


def test_conscience_get_capability(tmp_path):
    """get_capability returns ConscienceManager."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    assert isinstance(agent.get_capability("conscience"), ConscienceManager)
    agent.stop(timeout=1.0)


def test_conscience_custom_interval(tmp_path):
    """Custom interval is passed through."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 60}},
    )
    assert agent.get_capability("conscience")._interval == 60
    agent.stop(timeout=1.0)


def test_clock_intrinsic_unchanged(tmp_path):
    """conscience does NOT replace the clock intrinsic."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    assert callable(agent._intrinsics["clock"])
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Hormê toggle
# ---------------------------------------------------------------------------

def test_horme_on(tmp_path):
    """horme enabled=true activates the timer."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "horme", "enabled": True})
    assert result["status"] == "activated"
    assert mgr._horme_active is True
    assert mgr._timer is not None
    mgr.stop()
    agent.stop(timeout=1.0)


def test_horme_on_already_active(tmp_path):
    """horme on when already active returns already_active."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    mgr.handle({"action": "horme", "enabled": True})
    result = mgr.handle({"action": "horme", "enabled": True})
    assert result["status"] == "already_active"
    mgr.stop()
    agent.stop(timeout=1.0)


def test_horme_off(tmp_path):
    """horme enabled=false deactivates."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    mgr.handle({"action": "horme", "enabled": True})
    result = mgr.handle({"action": "horme", "enabled": False})
    assert result["status"] == "deactivated"
    assert mgr._horme_active is False
    assert mgr._timer is None
    agent.stop(timeout=1.0)


def test_horme_off_already_inactive(tmp_path):
    """horme off when inactive returns already_inactive."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "horme", "enabled": False})
    assert result["status"] == "already_inactive"
    agent.stop(timeout=1.0)


def test_horme_missing_enabled(tmp_path):
    """horme without enabled returns error."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "horme"})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Inner voice edit
# ---------------------------------------------------------------------------

def test_inner_voice_update(tmp_path):
    """inner_voice updates the prompt."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({
        "action": "inner_voice",
        "prompt": "What patterns remain?",
        "reasoning": "Early exploration",
    })
    assert result["status"] == "updated"
    assert result["prompt"] == "What patterns remain?"
    assert mgr._prompt == "What patterns remain?"
    agent.stop(timeout=1.0)


def test_inner_voice_missing_prompt(tmp_path):
    """inner_voice without prompt returns error."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "inner_voice"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_unknown_action(tmp_path):
    """Unknown action returns error."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "bogus"})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Nudge delivery + git
# ---------------------------------------------------------------------------

def test_nudge_fires_when_idle(tmp_path):
    """Nudge is sent via agent.send() after interval when idle."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.4)
        mgr.stop()

    mock_send.assert_called()
    call_args = mock_send.call_args
    assert call_args[0][0] == DEFAULT_PROMPT
    assert call_args[1]["sender"] == "conscience"
    assert call_args[1]["wait"] is False
    agent.stop(timeout=2.0)


def test_nudge_skips_when_active(tmp_path):
    """When agent is ACTIVE, nudge reschedules instead of sending."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    agent._idle.clear()  # Force ACTIVE

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    mock_send.assert_not_called()
    agent._idle.set()
    agent.stop(timeout=2.0)


def test_nudge_uses_updated_prompt(tmp_path):
    """Nudge uses the most recent inner_voice prompt."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "inner_voice", "prompt": "What now?", "reasoning": "test"})
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    mock_send.assert_called()
    assert mock_send.call_args[0][0] == "What now?"
    agent.stop(timeout=2.0)


def test_nudge_writes_horme_md(tmp_path):
    """Each nudge writes conscience/horme.md."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    with patch.object(agent, "send"):
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    horme_md = agent._working_dir / "conscience" / "horme.md"
    assert horme_md.is_file()
    content = horme_md.read_text()
    assert "Last nudge:" in content
    assert DEFAULT_PROMPT.strip() in content
    agent.stop(timeout=2.0)


def test_nudge_git_commits(tmp_path):
    """Each nudge creates a git commit."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    with patch.object(agent, "send"):
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.4)
        mgr.stop()

    # Check git log for nudge commits
    result = subprocess.run(
        ["git", "log", "--oneline", "--all"],
        cwd=str(agent._working_dir),
        capture_output=True, text=True,
    )
    assert "conscience: nudge" in result.stdout
    agent.stop(timeout=2.0)


# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

def test_stop_cancels_timer(tmp_path):
    """stop() cancels the timer and deactivates."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    mgr.handle({"action": "horme", "enabled": True})
    assert mgr._horme_active is True
    mgr.stop()
    assert mgr._horme_active is False
    assert mgr._timer is None
    agent.stop(timeout=1.0)
