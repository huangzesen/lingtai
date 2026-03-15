"""Tests for the bash layer."""
import pytest

from stoai.layers.bash import BashManager, add_bash_layer


class TestBashManager:
    def test_echo(self):
        mgr = BashManager()
        result = mgr.handle({"command": "echo hello"})
        assert result["status"] == "ok"
        assert result["exit_code"] == 0
        assert "hello" in result["stdout"]

    def test_nonexistent_command(self):
        mgr = BashManager()
        result = mgr.handle({"command": "definitely_not_a_real_command_xyz"})
        assert result["status"] == "ok"
        assert result["exit_code"] != 0

    def test_empty_command(self):
        mgr = BashManager()
        result = mgr.handle({"command": ""})
        assert "error" in result

    def test_timeout(self):
        mgr = BashManager()
        result = mgr.handle({"command": "sleep 10", "timeout": 0.5})
        assert "error" in result
        assert "timed out" in result["error"]

    def test_allowed_commands(self):
        mgr = BashManager(allowed_commands=["echo", "ls"])
        # Allowed
        result = mgr.handle({"command": "echo ok"})
        assert result["status"] == "ok"
        # Not allowed
        result = mgr.handle({"command": "rm -rf /"})
        assert "error" in result
        assert "not allowed" in result["error"]

    def test_working_dir(self, tmp_path):
        mgr = BashManager(working_dir=str(tmp_path))
        result = mgr.handle({"command": "pwd"})
        assert result["status"] == "ok"
        assert str(tmp_path) in result["stdout"]

    def test_output_truncation(self):
        mgr = BashManager(max_output=20)
        result = mgr.handle({"command": "echo 'a very long output string that exceeds the limit'"})
        assert "truncated" in result["stdout"]


class TestAddBashLayer:
    def test_add_bash_layer(self):
        from unittest.mock import MagicMock
        agent = MagicMock()
        mgr = add_bash_layer(agent)
        assert isinstance(mgr, BashManager)
        agent.add_tool.assert_called_once()
        agent.update_system_prompt.assert_called_once()

    def test_add_bash_layer_with_restrictions(self):
        from unittest.mock import MagicMock
        agent = MagicMock()
        mgr = add_bash_layer(agent, allowed_commands=["git", "npm"])
        assert isinstance(mgr, BashManager)
        # Verify the system prompt mentions restrictions
        call_args = agent.update_system_prompt.call_args
        assert "git" in call_args[0][1]
