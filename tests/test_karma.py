"""Tests for karma/nirvana lifecycle control via system intrinsic."""
from __future__ import annotations

import threading
import time
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai_kernel.base_agent import BaseAgent
from lingtai_kernel.state import AgentState


def _make_agent(tmp_path, **kwargs):
    """Create a minimal BaseAgent for testing."""
    svc = MagicMock()
    svc.create_session.return_value = MagicMock()
    kwargs.setdefault("working_dir", str(tmp_path / "test000000ab"))
    agent = BaseAgent(svc, **kwargs)
    return agent


class TestSignalFiles:
    """Signal file detection in heartbeat loop."""

    def test_interrupt_signal_sets_cancel_event(self, tmp_path):
        agent = _make_agent(tmp_path)
        agent.start()
        try:
            # Write .interrupt signal file
            (agent.working_dir / ".interrupt").write_text("")
            # Wait for heartbeat to detect it
            time.sleep(2.0)
            assert agent._cancel_event.is_set()
            assert not (agent.working_dir / ".interrupt").exists(), "signal file should be deleted"
        finally:
            agent.stop()

    def test_sleep_signal_sets_asleep(self, tmp_path):
        agent = _make_agent(tmp_path)
        agent.start()
        # Write .sleep signal file
        (agent.working_dir / ".sleep").write_text("")
        # Wait for agent to detect it
        time.sleep(3.0)
        assert agent._asleep.is_set()
        assert agent.state == AgentState.ASLEEP
        assert not (agent.working_dir / ".sleep").exists(), "signal file should be deleted"


class TestSystemIntrinsicKarma:
    """Karma actions in system intrinsic."""

    def test_interrupt_requires_karma_admin(self, tmp_path):
        agent = _make_agent(tmp_path, admin={})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "interrupt", "address": "/some/path"})
        assert "error" in result

    def test_interrupt_with_karma_admin(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')
        (target_dir / ".agent.heartbeat").write_text(str(time.time()))

        sender_base = tmp_path / "sender"
        sender_base.mkdir()
        agent = _make_agent(sender_base, admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "interrupt", "address": str(target_dir)})
        assert result["status"] == "interrupted"
        assert (target_dir / ".interrupt").is_file()

    def test_lull_writes_signal_file(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')
        (target_dir / ".agent.heartbeat").write_text(str(time.time()))

        sender_base = tmp_path / "sender"
        sender_base.mkdir()
        agent = _make_agent(sender_base, admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "lull", "address": str(target_dir)})
        assert result["status"] == "asleep"
        assert (target_dir / ".sleep").is_file()

    def test_lull_rejects_asleep_target(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')

        sender_base = tmp_path / "sender"
        sender_base.mkdir()
        agent = _make_agent(sender_base, admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "lull", "address": str(target_dir)})
        assert "error" in result

    def test_interrupt_self_rejected(self, tmp_path):
        agent = _make_agent(tmp_path, admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "interrupt", "address": str(agent.working_dir)})
        assert "error" in result

    def test_nirvana_requires_nirvana_admin(self, tmp_path):
        sender_base = tmp_path / "sender"
        sender_base.mkdir()
        agent = _make_agent(sender_base, admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "nirvana", "address": "/some/path"})
        assert "error" in result

    def test_nirvana_with_nirvana_admin(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')

        sender_base = tmp_path / "sender"
        sender_base.mkdir()
        agent = _make_agent(sender_base, admin={"karma": True, "nirvana": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "nirvana", "address": str(target_dir)})
        assert result["status"] == "nirvana"
        assert not target_dir.exists()

    def test_nirvana_self_rejected(self, tmp_path):
        agent = _make_agent(tmp_path, admin={"karma": True, "nirvana": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "nirvana", "address": str(agent.working_dir)})
        assert "error" in result

    def test_cpr_rejects_alive_target(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        (target_dir / ".agent.json").write_text('{"agent_id": "t1"}')
        (target_dir / ".agent.heartbeat").write_text(str(time.time()))

        sender_base = tmp_path / "sender"
        sender_base.mkdir()
        agent = _make_agent(sender_base, admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "cpr", "address": str(target_dir)})
        assert "error" in result
        assert "already running" in result["message"]

    def test_cpr_without_handler_returns_error(self, tmp_path):
        target_dir = tmp_path / "target"
        target_dir.mkdir()
        # admin={} so is_human returns False (admin is not None)
        (target_dir / ".agent.json").write_text('{"agent_id": "t1", "admin": {}}')

        sender_base = tmp_path / "sender"
        sender_base.mkdir()
        agent = _make_agent(sender_base, admin={"karma": True})
        from lingtai_kernel.intrinsics.system import handle
        result = handle(agent, {"action": "cpr", "address": str(target_dir)})
        assert "error" in result
        assert "not supported" in result["message"].lower()


class TestCPRLingtai:
    """CPR via lingtai Agent (full reconstruction)."""

    def test_cpr_reconstructs_agent(self, tmp_path):
        from lingtai.agent import Agent

        svc = MagicMock()
        svc.create_session.return_value = MagicMock()
        svc.provider = "mock"
        svc.model = "test-model"
        svc._base_url = None

        # Create an agent — this should persist LLM config
        agent = Agent(svc, working_dir=tmp_path / "alice000001",
                      agent_name="alice", admin={"karma": True})

        # Verify LLM config was persisted to working dir
        import json
        llm_config_path = agent.working_dir / "system" / "llm.json"
        assert llm_config_path.is_file()
        llm_config = json.loads(llm_config_path.read_text())
        assert llm_config["provider"] == "mock"
        assert llm_config["model"] == "test-model"

    def test_cpr_agent_hook_returns_agent(self, tmp_path):
        from lingtai.agent import Agent
        from unittest.mock import patch

        svc = MagicMock()
        svc.create_session.return_value = MagicMock()
        svc.provider = "mock"
        svc.model = "test-model"
        svc._base_url = None

        # Create an "asleep" agent — construct, persist, don't start
        agents_dir = tmp_path / "agents"
        agents_dir.mkdir()
        target = Agent(svc, working_dir=agents_dir / "bobbob000001",
                       agent_name="bob")
        target_dir = str(target.working_dir)

        # Create the agent that will perform CPR
        reviver_dir = tmp_path / "reviver"
        reviver_dir.mkdir()
        reviver = Agent(svc, working_dir=reviver_dir / "admin000001",
                        agent_name="admin", admin={"karma": True})

        # Release the lock on the target (simulate an asleep/dead agent)
        target._workdir.release_lock()

        # Patch LLMService so reconstruction doesn't fail (no adapter registered for "mock")
        mock_svc = MagicMock()
        mock_svc.create_session.return_value = MagicMock()
        mock_svc.provider = "mock"
        mock_svc.model = "test-model"
        mock_svc._base_url = None

        with patch("lingtai.agent.LLMService", return_value=mock_svc):
            resuscitated = reviver._cpr_agent(target_dir)

        assert resuscitated is not None
        assert resuscitated.agent_name == "bob"
