"""Tests for the bash capability."""
import json
from pathlib import Path

import pytest
from unittest.mock import MagicMock

from stoai.capabilities.bash import BashManager, BashPolicy, setup as setup_bash


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

    def test_allow_and_deny(self, tmp_path):
        """Must be in allow AND not in deny."""
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["git", "rm"], "deny": ["rm"]}))
        policy = BashPolicy.from_file(str(policy_file))
        assert policy.is_allowed("git status")
        assert not policy.is_allowed("rm file")  # in allow but also in deny

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
        assert "error" in result

    def test_timeout(self):
        mgr = BashManager(policy=BashPolicy.yolo(), working_dir="/tmp")
        result = mgr.handle({"command": "sleep 10", "timeout": 0.5})
        assert "error" in result
        assert "timed out" in result["error"]

    def test_policy_denies(self, tmp_path):
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"deny": ["rm"]}))
        policy = BashPolicy.from_file(str(policy_file))
        mgr = BashManager(policy=policy, working_dir="/tmp")
        result = mgr.handle({"command": "rm -rf /"})
        assert "error" in result
        assert "not allowed" in result["error"]

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

    def test_setup_requires_policy_or_yolo(self):
        agent = MagicMock()
        agent._working_dir = Path("/tmp")
        with pytest.raises(ValueError, match="policy_file"):
            setup_bash(agent)


# ---------------------------------------------------------------------------
# add_capability integration
# ---------------------------------------------------------------------------

class TestAddCapability:
    def test_add_capability_bash_yolo(self):
        from stoai.agent import BaseAgent
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = BaseAgent(agent_id="test", service=svc, working_dir="/tmp")
        mgr = agent.add_capability("bash", yolo=True)
        assert isinstance(mgr, BashManager)
        assert "bash" in agent._mcp_handlers

    def test_add_capability_bash_with_policy(self, tmp_path):
        from stoai.agent import BaseAgent
        policy_file = tmp_path / "policy.json"
        policy_file.write_text(json.dumps({"allow": ["echo"]}))
        svc = MagicMock()
        svc.get_adapter.return_value = MagicMock()
        svc.provider = "gemini"
        svc.model = "gemini-test"
        agent = BaseAgent(agent_id="test", service=svc, working_dir="/tmp")
        mgr = agent.add_capability("bash", policy_file=str(policy_file))
        assert isinstance(mgr, BashManager)
