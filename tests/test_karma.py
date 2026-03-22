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
    kwargs.setdefault("agent_id", "test000000ab")
    agent = BaseAgent(svc, base_dir=str(tmp_path), **kwargs)
    return agent


class TestSignalFiles:
    """Signal file detection in heartbeat loop."""

    def test_silence_signal_sets_cancel_event(self, tmp_path):
        agent = _make_agent(tmp_path)
        agent.start()
        try:
            # Write .silence signal file
            (agent.working_dir / ".silence").write_text("")
            # Wait for heartbeat to detect it
            time.sleep(2.0)
            assert agent._cancel_event.is_set()
            assert not (agent.working_dir / ".silence").exists(), "signal file should be deleted"
        finally:
            agent.stop()

    def test_quell_signal_sets_shutdown(self, tmp_path):
        agent = _make_agent(tmp_path)
        agent.start()
        # Write .quell signal file
        (agent.working_dir / ".quell").write_text("")
        # Wait for agent to shut down
        time.sleep(3.0)
        assert agent._shutdown.is_set()
        assert agent.state == AgentState.DORMANT
        assert not (agent.working_dir / ".quell").exists(), "signal file should be deleted"
