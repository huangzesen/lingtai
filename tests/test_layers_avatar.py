"""Tests for the avatar capability."""
import time
from unittest.mock import MagicMock

import pytest

from lingtai.capabilities.bash import BashManager
from lingtai.capabilities.avatar import AvatarManager, setup as setup_avatar


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


class TestAvatarManager:
    def test_spawn_returns_address(self, tmp_path):
        """Spawn should return a valid address."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"])
        mgr = parent.get_capability("avatar")
        result = mgr.handle({})
        assert result["status"] == "ok"
        assert "address" in result
        assert result["address"]  # filesystem path (non-empty string)
        assert "agent_name" in result

    def test_spawn_with_role(self, tmp_path):
        """Spawn with role override should create agent with that role."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"])
        parent.update_system_prompt("role", "I am the parent", protected=True)
        mgr = parent.get_capability("avatar")
        result = mgr.handle({"role": "I am the researcher"})
        assert result["status"] == "ok"

    def test_spawn_copies_parent_role(self, tmp_path):
        """Spawn without role should copy parent's role."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"])
        parent.update_system_prompt("role", "I am the parent", protected=True)
        mgr = parent.get_capability("avatar")
        result = mgr.handle({})
        assert result["status"] == "ok"

    def test_spawn_inherits_capabilities(self, tmp_path):
        """Spawned agent should get parent's capabilities (including avatar)."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities={"bash": {"yolo": True}, "avatar": {}})
        result = parent._mcp_handlers["avatar"]({})
        assert result["status"] == "ok"
        # Spawned avatar should have avatar capability (recursive spawning)
        child = parent.get_capability("avatar")._peers["avatar"]
        child_cap_names = [name for name, _ in child._capabilities]
        assert "bash" in child_cap_names
        assert "avatar" in child_cap_names

    def test_spawn_with_ltm(self, tmp_path):
        """Spawn with ltm should inject it as a system prompt section."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"])
        mgr = parent.get_capability("avatar")
        result = mgr.handle({"ltm": "Remember: always be concise"})
        assert result["status"] == "ok"

    def test_spawn_max_agents(self, tmp_path):
        """Spawning should be refused when max_agents is reached."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities={"avatar": {"max_agents": 2}})
        mgr = parent.get_capability("avatar")
        # First spawn should succeed
        r1 = mgr.handle({"name": "a1"})
        assert r1["status"] == "ok"
        # Parent + a1 = 2 manifests, next spawn should be refused
        r2 = mgr.handle({"name": "a2"})
        assert "error" in r2
        assert "total agents=2" in r2["error"]


class TestSetupAvatar:
    def test_setup_avatar(self):
        agent = MagicMock()
        mgr = setup_avatar(agent)
        assert isinstance(mgr, AvatarManager)
        agent.add_tool.assert_called_once()


class TestAddCapability:
    def test_add_capability_avatar(self, tmp_path):
        from lingtai.agent import Agent
        agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                           capabilities=["avatar"])
        mgr = agent.get_capability("avatar")
        assert isinstance(mgr, AvatarManager)
        assert "avatar" in agent._mcp_handlers

    def test_add_capability_unknown(self, tmp_path):
        from lingtai.agent import Agent
        with pytest.raises(ValueError, match="Unknown capability"):
            Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["nonexistent"])

    def test_add_multiple_capabilities_separately(self, tmp_path):
        from lingtai.agent import Agent
        agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                           capabilities={"bash": {"yolo": True}, "avatar": {}})
        bash_mgr = agent.get_capability("bash")
        avatar_mgr = agent.get_capability("avatar")
        assert isinstance(bash_mgr, BashManager)
        assert isinstance(avatar_mgr, AvatarManager)

    def test_capabilities_log(self, tmp_path):
        """Agent should record (name, kwargs) in _capabilities."""
        from lingtai.agent import Agent
        agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                           capabilities={"bash": {"yolo": True}, "avatar": {}})
        assert len(agent._capabilities) == 2
        assert agent._capabilities[0] == ("bash", {"yolo": True})
        assert agent._capabilities[1] == ("avatar", {})
