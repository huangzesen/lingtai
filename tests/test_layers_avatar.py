"""Tests for the avatar capability."""
import json
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
        result = mgr.handle({"name": "helper"})
        assert result["status"] == "ok"
        assert "address" in result
        assert result["address"]  # filesystem path (non-empty string)
        assert result["agent_name"] == "helper"

    def test_spawn_inherits_capabilities(self, tmp_path):
        """Spawned agent should get all of parent's capabilities."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities={"bash": {"yolo": True}, "avatar": {}})
        result = parent._tool_handlers["avatar"]({"name": "child"})
        assert result["status"] == "ok"
        child = parent.get_capability("avatar")._peers["child"]
        child_cap_names = [name for name, _ in child._capabilities]
        assert "bash" in child_cap_names
        assert "avatar" in child_cap_names

    def test_spawn_inherits_covenant(self, tmp_path):
        """Spawned agent should inherit parent's covenant."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"], covenant="Be helpful and concise.")
        mgr = parent.get_capability("avatar")
        result = mgr.handle({"name": "helper"})
        assert result["status"] == "ok"

    def test_spawn_no_admin(self, tmp_path):
        """Avatar should never get admin privileges, even if parent has them."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"], admin={"karma": True})
        mgr = parent.get_capability("avatar")
        result = mgr.handle({"name": "helper"})
        assert result["status"] == "ok"
        child = mgr._peers["helper"]
        assert child._admin == {}

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

    def test_spawn_duplicate_name_error(self, tmp_path):
        """Spawning a name that's already active should return already_active."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"])
        mgr = parent.get_capability("avatar")
        r1 = mgr.handle({"name": "helper"})
        assert r1["status"] == "ok"
        r2 = mgr.handle({"name": "helper"})
        assert r2["status"] == "already_active"

    def test_spawn_mirror_false_no_identity_files(self, tmp_path):
        """mirror=False (default) should not copy character/memory/library."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"])
        # Write identity files to parent
        system_dir = parent._working_dir / "system"
        system_dir.mkdir(parents=True, exist_ok=True)
        (system_dir / "character.md").write_text("I am the parent")
        (system_dir / "memory.md").write_text("Parent memory")
        lib_dir = parent._working_dir / "library"
        lib_dir.mkdir(parents=True, exist_ok=True)
        (lib_dir / "library.json").write_text('{"entries": []}')

        mgr = parent.get_capability("avatar")
        result = mgr.handle({"name": "blank"})
        assert result["status"] == "ok"
        child = mgr._peers["blank"]
        # Character and library should NOT be copied
        assert not (child._working_dir / "system" / "character.md").is_file()
        assert not (child._working_dir / "library" / "library.json").is_file()

    def test_spawn_mirror_true_copies_identity(self, tmp_path):
        """mirror=True should copy character, memory, library, and exports."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"])
        # Write identity files to parent
        system_dir = parent._working_dir / "system"
        system_dir.mkdir(parents=True, exist_ok=True)
        (system_dir / "character.md").write_text("I am the parent")
        (system_dir / "memory.md").write_text("Parent memory")
        lib_dir = parent._working_dir / "library"
        lib_dir.mkdir(parents=True, exist_ok=True)
        (lib_dir / "library.json").write_text('{"entries": []}')
        exports_dir = parent._working_dir / "exports"
        exports_dir.mkdir(parents=True, exist_ok=True)
        (exports_dir / "abc123.txt").write_text("exported knowledge")

        mgr = parent.get_capability("avatar")
        result = mgr.handle({"name": "clone", "mirror": True})
        assert result["status"] == "ok"
        child = mgr._peers["clone"]
        assert (child._working_dir / "system" / "character.md").read_text() == "I am the parent"
        assert (child._working_dir / "system" / "memory.md").read_text() == "Parent memory"
        assert (child._working_dir / "library" / "library.json").read_text() == '{"entries": []}'
        assert (child._working_dir / "exports" / "abc123.txt").read_text() == "exported knowledge"

    def test_spawn_mirror_missing_files_ok(self, tmp_path):
        """mirror=True with no identity files should not error."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"])
        mgr = parent.get_capability("avatar")
        result = mgr.handle({"name": "clone", "mirror": True})
        assert result["status"] == "ok"

    def test_ledger_records_mirror(self, tmp_path):
        """Ledger should record mirror flag."""
        from lingtai.agent import Agent
        parent = Agent(service=make_mock_service(), agent_name="parent", working_dir=tmp_path / "test",
                            capabilities=["avatar"])
        mgr = parent.get_capability("avatar")
        mgr.handle({"name": "clone", "mirror": True})
        ledger = (parent._working_dir / "delegates" / "ledger.jsonl").read_text().strip()
        record = json.loads(ledger)
        assert record["mirror"] is True
        assert record["name"] == "clone"


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
        assert "avatar" in agent._tool_handlers

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
