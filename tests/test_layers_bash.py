"""Tests for the bash capability."""
import json
from pathlib import Path

import pytest
from unittest.mock import MagicMock

from lingtai.capabilities.bash import BashManager, BashPolicy, setup as setup_bash


# ---------------------------------------------------------------------------
# BashPolicy
# ---------------------------------------------------------------------------

class TestBashPolicy:
    def test_load_from_file(self, tmp_path):
        """Policy should load allow/deny from JSON file."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["git", "ls"], "deny": ["rm"]}))
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("git status")
        assert policy.is_allowed("ls -la")
        assert not policy.is_allowed("rm -rf /")

    def test_allow_only(self, tmp_path):
        """With only allow list, unlisted commands are denied."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["git", "echo"]}))
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("git push")
        assert not policy.is_allowed("curl http://evil.com")

    def test_deny_only(self, tmp_path):
        """With only deny list, unlisted commands are allowed."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"deny": ["rm", "sudo"]}))
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("ls -la")
        assert not policy.is_allowed("rm file.txt")
        assert not policy.is_allowed("sudo apt install")

    def test_allow_ignores_deny(self, tmp_path):
        """When allow is present, deny is ignored (allowlist mode)."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["git", "rm"], "deny": ["rm"]}))
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("git status")
        assert policy.is_allowed("rm file")  # allow mode — rm is in allow, deny ignored
        assert not policy.is_allowed("curl http://x")  # not in allow → blocked

    def test_pipe_awareness(self, tmp_path):
        """Should check all commands in a pipe chain."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"deny": ["rm"]}))
        policy = BashPolicy.from_file(str(policy_file))
        assert not policy.is_allowed("ls | rm -rf /")
        assert not policy.is_allowed("echo hello && rm file")
        assert not policy.is_allowed("echo hello; rm file")
        assert policy.is_allowed("ls | grep foo | sort")

    def test_subshell_awareness(self, tmp_path):
        """Should check commands inside $()."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"deny": ["rm"]}))
        policy = BashPolicy.from_file(str(policy_file))
        assert not policy.is_allowed("echo $(rm file)")

    def test_yolo_allows_everything(self):
        """Yolo policy should allow all commands."""
        policy = BashPolicy.yolo()
        assert policy.is_allowed("rm -rf /")
        assert policy.is_allowed("sudo shutdown -h now")

    def test_missing_file_raises(self):
        """Loading from nonexistent file should raise."""
        with pytest.raises(FileNotFoundError):
            BashPolicy.from_file("/nonexistent/policy.json")

    def test_empty_policy_file(self, tmp_path):
        """Empty policy (no allow, no deny) should allow everything."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({}))
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("anything")


# ---------------------------------------------------------------------------
# BashManager
# ---------------------------------------------------------------------------

class TestBashManager:
    def test_echo(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp")
        result = mgr.handle({"command": "echo hello"})
        assert result["status"] == "ok"
        assert result["exit_code"] == 0
        assert "hello" in result["stdout"]

    def test_nonexistent_command(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp")
        result = mgr.handle({"command": "definitely_not_a_real_command_xyz"})
        assert result["status"] == "ok"
        assert result["exit_code"] != 0

    def test_empty_command(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp")
        result = mgr.handle({"command": ""})
        assert result["status"] == "error"

    def test_timeout(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp")
        result = mgr.handle({"command": "sleep 10", "timeout": 0.5})
        assert result["status"] == "error"
        assert "timed out" in result["message"]

    def test_policy_denies(self, tmp_path):
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"deny": ["rm"]}))
        policy = BashPolicy.from_file(str(policy_file))
        mgr = BashManager(policy=policy, working_dir="/tmp")
        result = mgr.handle({"command": "rm -rf /"})
        assert result["status"] == "error"
        assert "not allowed" in result["message"]

    def test_policy_allows(self, tmp_path):
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["echo", "ls"]}))
        policy = BashPolicy.from_file(str(policy_file))
        mgr = BashManager(policy=policy, working_dir="/tmp")
        result = mgr.handle({"command": "echo ok"})
        assert result["status"] == "ok"

    def test_working_dir(self, tmp_path):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir=str(tmp_path))
        result = mgr.handle({"command": "pwd"})
        assert result["status"] == "ok"
        assert str(tmp_path) in result["stdout"]

    def test_output_truncation(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp", max_output=20)
        result = mgr.handle({"command": "echo 'a very long output string that exceeds the limit'"})
        assert "truncated" in result["stdout"]


# ---------------------------------------------------------------------------
# setup()
# ---------------------------------------------------------------------------

class TestSetupBash:
    def test_setup_with_policy_file(self, tmp_path):
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["echo"]}))
        agent = MagicMock()
        agent._working_dir = Path("/tmp")
        mgr = setup_bash(agent, policy_file=str(policy_file))
        assert isinstance(mgr, BashManager)
        agent.add_tool.assert_called_once()

    def test_setup_yolo(self):
        agent = MagicMock()
        agent._working_dir = Path("/tmp")
        mgr = setup_bash(agent, yolo=True)
        assert isinstance(mgr, BashManager)
        agent.add_tool.assert_called_once()

    def test_setup_uses_default_policy_when_none_specified(self):
        agent = MagicMock()
        agent._working_dir = Path("/tmp")
        agent._config.bash_policy_file = None  # no config fallback
        # Should succeed — falls back to bundled default policy
        setup_bash(agent)
        agent.add_tool.assert_called_once()


# ---------------------------------------------------------------------------
# add_capability integration
# ---------------------------------------------------------------------------

class TestAddCapability:
    def test_add_capability_bash_yolo(self, tmp_path):
        from lingtai.agent import Agent
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = Agent(service=svc, agent_name="test", working_dir=tmp_path / "test",
                           capabilities={"bash": {"yolo": True}})
        mgr = agent.get_capability("bash")
        assert isinstance(mgr, BashManager)
        assert "bash" in agent._tool_handlers

    def test_add_capability_bash_with_policy(self, tmp_path):
        from lingtai.agent import Agent
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["echo"]}))
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = Agent(service=svc, agent_name="test", working_dir=tmp_path / "test",
                           capabilities={"bash": {"policy_file": str(policy_file)}})
        mgr = agent.get_capability("bash")
        assert isinstance(mgr, BashManager)
