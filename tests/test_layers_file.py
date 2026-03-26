"""Tests for file I/O capabilities (read, write, edit, glob, grep)."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from lingtai.agent import Agent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_file_sugar_expands_to_five(tmp_path):
    """capabilities=["file"] should register all 5 file tools."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["file"],
    )
    for name in ("read", "write", "edit", "glob", "grep"):
        assert name in agent._tool_handlers, f"{name} not registered"
    agent.stop(timeout=1.0)


def test_file_sugar_dict_form(tmp_path):
    """capabilities={"file": {}} (dict form) should also expand."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities={"file": {}},
    )
    for name in ("read", "write", "edit", "glob", "grep"):
        assert name in agent._tool_handlers, f"{name} not registered (dict form)"
    agent.stop(timeout=1.0)


def test_individual_file_capability(tmp_path):
    """Each file capability can be loaded individually."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["read", "write"],
    )
    assert "read" in agent._tool_handlers
    assert "write" in agent._tool_handlers
    assert "edit" not in agent._tool_handlers
    assert "glob" not in agent._tool_handlers
    assert "grep" not in agent._tool_handlers
    agent.stop(timeout=1.0)


def test_write_and_read_via_capability(tmp_path):
    """Write and read files through capability handlers."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["file"],
    )
    # Write
    write_result = agent._tool_handlers["write"](
        {"file_path": str(agent.working_dir / "test.txt"), "content": "hello world"}
    )
    assert write_result["status"] == "ok"

    # Read
    read_result = agent._tool_handlers["read"](
        {"file_path": str(agent.working_dir / "test.txt")}
    )
    assert "hello world" in read_result["content"]
    agent.stop(timeout=1.0)


def test_edit_via_capability(tmp_path):
    """Edit files through capability handler."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["file"],
    )
    (agent.working_dir / "test.txt").write_text("hello world")
    result = agent._tool_handlers["edit"](
        {"file_path": str(agent.working_dir / "test.txt"), "old_string": "hello", "new_string": "goodbye"}
    )
    assert result["status"] == "ok"
    assert (agent.working_dir / "test.txt").read_text() == "goodbye world"
    agent.stop(timeout=1.0)


def test_glob_via_capability(tmp_path):
    """Glob files through capability handler."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["file"],
    )
    (agent.working_dir / "a.py").write_text("pass")
    (agent.working_dir / "b.py").write_text("pass")
    (agent.working_dir / "c.txt").write_text("text")
    result = agent._tool_handlers["glob"](
        {"pattern": "*.py", "path": str(agent.working_dir)}
    )
    assert result["count"] == 2
    agent.stop(timeout=1.0)


def test_grep_via_capability(tmp_path):
    """Grep files through capability handler."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["file"],
    )
    (agent.working_dir / "test.py").write_text("def hello():\n    pass\n")
    result = agent._tool_handlers["grep"](
        {"pattern": "def hello", "path": str(agent.working_dir)}
    )
    assert result["count"] >= 1
    agent.stop(timeout=1.0)


def test_base_agent_has_no_file_intrinsics(tmp_path):
    """BaseAgent should NOT have file intrinsics after phase 2."""
    from lingtai_kernel.base_agent import BaseAgent
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    for name in ("read", "write", "edit", "glob", "grep"):
        assert name not in agent._intrinsics, f"{name} should not be in BaseAgent intrinsics"
    agent.stop(timeout=1.0)


def test_base_agent_kernel_only(tmp_path):
    """BaseAgent should have exactly 4 intrinsics: mail, system, eigen, soul."""
    from lingtai_kernel.base_agent import BaseAgent
    agent = BaseAgent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test")
    assert set(agent._intrinsics.keys()) == {"mail", "system", "eigen", "soul"}
    agent.stop(timeout=1.0)


def test_file_capability_uses_file_io_service(tmp_path):
    """File capabilities should use the agent's FileIOService."""
    from lingtai.services.file_io import LocalFileIOService
    svc = LocalFileIOService(root=tmp_path)
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        file_io=svc,
        capabilities=["file"],
    )
    result = agent._tool_handlers["write"](
        {"file_path": str(tmp_path / "test.txt"), "content": "via service"}
    )
    assert result["status"] == "ok"
    assert (tmp_path / "test.txt").read_text() == "via service"
    agent.stop(timeout=1.0)
