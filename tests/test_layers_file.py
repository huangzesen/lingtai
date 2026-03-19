"""Tests for file I/O capabilities (read, write, edit, glob, grep)."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from stoai.agent import Agent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_file_sugar_expands_to_five(tmp_path):
    """capabilities=["file"] should register all 5 file tools."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    for name in ("read", "write", "edit", "glob", "grep"):
        assert name in agent._mcp_handlers, f"{name} not registered"
    agent.stop(timeout=1.0)


def test_file_sugar_dict_form(tmp_path):
    """capabilities={"file": {}} (dict form) should also expand."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"file": {}},
    )
    for name in ("read", "write", "edit", "glob", "grep"):
        assert name in agent._mcp_handlers, f"{name} not registered (dict form)"
    agent.stop(timeout=1.0)


def test_individual_file_capability(tmp_path):
    """Each file capability can be loaded individually."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["read", "write"],
    )
    assert "read" in agent._mcp_handlers
    assert "write" in agent._mcp_handlers
    assert "edit" not in agent._mcp_handlers
    assert "glob" not in agent._mcp_handlers
    assert "grep" not in agent._mcp_handlers
    agent.stop(timeout=1.0)


def test_write_and_read_via_capability(tmp_path):
    """Write and read files through capability handlers."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    # Write
    write_result = agent._mcp_handlers["write"](
        {"file_path": str(agent.working_dir / "test.txt"), "content": "hello world"}
    )
    assert write_result["status"] == "ok"

    # Read
    read_result = agent._mcp_handlers["read"](
        {"file_path": str(agent.working_dir / "test.txt")}
    )
    assert "hello world" in read_result["content"]
    agent.stop(timeout=1.0)


def test_edit_via_capability(tmp_path):
    """Edit files through capability handler."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    (agent.working_dir / "test.txt").write_text("hello world")
    result = agent._mcp_handlers["edit"](
        {"file_path": str(agent.working_dir / "test.txt"), "old_string": "hello", "new_string": "goodbye"}
    )
    assert result["status"] == "ok"
    assert (agent.working_dir / "test.txt").read_text() == "goodbye world"
    agent.stop(timeout=1.0)


def test_glob_via_capability(tmp_path):
    """Glob files through capability handler."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    (agent.working_dir / "a.py").write_text("pass")
    (agent.working_dir / "b.py").write_text("pass")
    (agent.working_dir / "c.txt").write_text("text")
    result = agent._mcp_handlers["glob"](
        {"pattern": "*.py", "path": str(agent.working_dir)}
    )
    assert result["count"] == 2
    agent.stop(timeout=1.0)


def test_grep_via_capability(tmp_path):
    """Grep files through capability handler."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["file"],
    )
    (agent.working_dir / "test.py").write_text("def hello():\n    pass\n")
    result = agent._mcp_handlers["grep"](
        {"pattern": "def hello", "path": str(agent.working_dir)}
    )
    assert result["count"] >= 1
    agent.stop(timeout=1.0)


def test_base_agent_has_no_file_intrinsics(tmp_path):
    """BaseAgent should NOT have file intrinsics after phase 2."""
    from stoai_kernel.base_agent import BaseAgent
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    for name in ("read", "write", "edit", "glob", "grep"):
        assert name not in agent._intrinsics, f"{name} should not be in BaseAgent intrinsics"
    agent.stop(timeout=1.0)


def test_base_agent_kernel_only(tmp_path):
    """BaseAgent should have exactly 4 intrinsics: mail, system, eigen, soul."""
    from stoai_kernel.base_agent import BaseAgent
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert set(agent._intrinsics.keys()) == {"mail", "system", "eigen", "soul"}
    agent.stop(timeout=1.0)


def test_file_capability_uses_file_io_service(tmp_path):
    """File capabilities should use the agent's FileIOService."""
    from stoai.services.file_io import LocalFileIOService
    svc = LocalFileIOService(root=tmp_path)
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        file_io=svc,
        capabilities=["file"],
    )
    result = agent._mcp_handlers["write"](
        {"file_path": str(tmp_path / "test.txt"), "content": "via service"}
    )
    assert result["status"] == "ok"
    assert (tmp_path / "test.txt").read_text() == "via service"
    agent.stop(timeout=1.0)
