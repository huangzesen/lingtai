"""Tests for the avatar capability."""
import time
from unittest.mock import MagicMock

import pytest

from stoai.capabilities.bash import BashManager
from stoai.capabilities.avatar import AvatarManager, setup as setup_avatar


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


class TestAvatarManager:
    def test_spawn_returns_address(self, tmp_path):
        """Spawn should return a valid address."""
        from stoai.agent import Agent
        parent = Agent(agent_name="parent", service=make_mock_service(), base_dir=tmp_path,
                            capabilities=["avatar"])
        mgr = parent.get_capability("avatar")
        result = mgr.handle({})
        assert result["status"] == "ok"
        assert "address" in result
        assert "127.0.0.1:" in result["address"]
        assert "agent_id" in result
        assert "agent_name" in result

    def test_spawn_with_role(self, tmp_path):
        """Spawn with role override should create agent with that role."""
        from stoai.agent import Agent
        parent = Agent(agent_name="parent", service=make_mock_service(), base_dir=tmp_path,
                            capabilities=["avatar"])
        parent.update_system_prompt("role", "I am the parent", protected=True)
        mgr = parent.get_capability("avatar")
        result = mgr.handle({"role": "I am the researcher"})
        assert result["status"] == "ok"

    def test_spawn_copies_parent_role(self, tmp_path):
        """Spawn without role should copy parent's role."""
        from stoai.agent import Agent
        parent = Agent(agent_name="parent", service=make_mock_service(), base_dir=tmp_path,
                            capabilities=["avatar"])
        parent.update_system_prompt("role", "I am the parent", protected=True)
        mgr = parent.get_capability("avatar")
        result = mgr.handle({})
        assert result["status"] == "ok"

    def test_spawn_inherits_capabilities(self, tmp_path):
        """Spawned agent should get parent's capabilities (minus avatar)."""
        from stoai.agent import Agent
        parent = Agent(agent_name="parent", service=make_mock_service(), base_dir=tmp_path,
                            capabilities={"bash": {"yolo": True}, "avatar": {}})
        result = parent._mcp_handlers["avatar"]({})
        assert result["status"] == "ok"

    def test_spawn_with_ltm(self, tmp_path):
        """Spawn with ltm should inject it as a system prompt section."""
        from stoai.agent import Agent
        parent = Agent(agent_name="parent", service=make_mock_service(), base_dir=tmp_path,
                            capabilities=["avatar"])
        mgr = parent.get_capability("avatar")
        result = mgr.handle({"ltm": "Remember: always be concise"})
        assert result["status"] == "ok"

    def test_spawn_no_recursive_avatar(self, tmp_path):
        """Spawned agent should not get avatar capability (prevents recursion)."""
        from stoai.agent import Agent
        parent = Agent(agent_name="parent", service=make_mock_service(), base_dir=tmp_path,
                            capabilities={"bash": {"yolo": True}, "avatar": {}})
        # Parent has both bash and avatar in capabilities log
        cap_names = [name for name, _ in parent._capabilities]
        assert "bash" in cap_names
        assert "avatar" in cap_names


class TestSetupAvatar:
    def test_setup_avatar(self):
        agent = MagicMock()
        mgr = setup_avatar(agent)
        assert isinstance(mgr, AvatarManager)
        agent.add_tool.assert_called_once()


class TestAddCapability:
    def test_add_capability_avatar(self, tmp_path):
        from stoai.agent import Agent
        agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                           capabilities=["avatar"])
        mgr = agent.get_capability("avatar")
        assert isinstance(mgr, AvatarManager)
        assert "avatar" in agent._mcp_handlers

    def test_add_capability_unknown(self, tmp_path):
        from stoai.agent import Agent
        with pytest.raises(ValueError, match="Unknown capability"):
            Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["nonexistent"])

    def test_add_multiple_capabilities_separately(self, tmp_path):
        from stoai.agent import Agent
        agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                           capabilities={"bash": {"yolo": True}, "avatar": {}})
        bash_mgr = agent.get_capability("bash")
        avatar_mgr = agent.get_capability("avatar")
        assert isinstance(bash_mgr, BashManager)
        assert isinstance(avatar_mgr, AvatarManager)

    def test_capabilities_log(self, tmp_path):
        """Agent should record (name, kwargs) in _capabilities."""
        from stoai.agent import Agent
        agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                           capabilities={"bash": {"yolo": True}, "avatar": {}})
        assert len(agent._capabilities) == 2
        assert agent._capabilities[0] == ("bash", {"yolo": True})
        assert agent._capabilities[1] == ("avatar", {})
