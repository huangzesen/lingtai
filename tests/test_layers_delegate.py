"""Tests for the delegate capability."""
from unittest.mock import MagicMock

import pytest

from stoai.capabilities.bash import BashManager
from stoai.capabilities.delegate import DelegateManager, setup as setup_delegate


class TestDelegateManager:
    def test_list_empty(self):
        mgr = DelegateManager()
        result = mgr.handle({"action": "list"})
        assert result["status"] == "ok"
        assert result["count"] == 0

    def test_spawn_without_factory(self):
        mgr = DelegateManager()
        result = mgr.handle({"action": "spawn", "role": "researcher"})
        assert "error" in result
        assert "agent_factory" in result["error"]

    def test_spawn_without_role_or_task(self):
        mgr = DelegateManager(agent_factory=MagicMock())
        result = mgr.handle({"action": "spawn"})
        assert "error" in result

    def test_send_to_nonexistent(self):
        mgr = DelegateManager()
        result = mgr.handle({"action": "send", "agent_id": "nope", "task": "do thing"})
        assert "error" in result

    def test_stop_nonexistent(self):
        mgr = DelegateManager()
        result = mgr.handle({"action": "stop", "agent_id": "nope"})
        assert "error" in result

    def test_unknown_action(self):
        mgr = DelegateManager()
        result = mgr.handle({"action": "explode"})
        assert "error" in result

    def test_send_missing_fields(self):
        mgr = DelegateManager()
        assert "error" in mgr.handle({"action": "send"})
        assert "error" in mgr.handle({"action": "send", "agent_id": "x"})


class TestSetupDelegate:
    def test_setup_delegate(self):
        agent = MagicMock()
        mgr = setup_delegate(agent)
        assert isinstance(mgr, DelegateManager)
        agent.add_tool.assert_called_once()
        agent.update_system_prompt.assert_called_once()


class TestAddCapability:
    def test_add_capability_delegate(self):
        from stoai.agent import BaseAgent
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = BaseAgent(agent_id="test", service=svc)
        mgr = agent.add_capability("delegate")
        assert isinstance(mgr, DelegateManager)
        assert "delegate" in agent._mcp_handlers

    def test_add_capability_unknown(self):
        from stoai.agent import BaseAgent
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = BaseAgent(agent_id="test", service=svc)
        with pytest.raises(ValueError, match="Unknown capability"):
            agent.add_capability("nonexistent")

    def test_add_multiple_capabilities(self):
        from stoai.agent import BaseAgent
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = BaseAgent(agent_id="test", service=svc)
        results = agent.add_capability("bash", "delegate")
        assert isinstance(results["bash"], BashManager)
        assert isinstance(results["delegate"], DelegateManager)
