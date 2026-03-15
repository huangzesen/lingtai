"""Tests for the delegate layer."""
from unittest.mock import MagicMock

import pytest

from stoai.layers.delegate import DelegateManager, add_delegate_layer


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


class TestAddDelegateLayer:
    def test_add_delegate_layer(self):
        agent = MagicMock()
        mgr = add_delegate_layer(agent)
        assert isinstance(mgr, DelegateManager)
        agent.add_tool.assert_called_once()
        agent.update_system_prompt.assert_called_once()
