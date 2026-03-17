"""Tests for system intrinsic — agent memory management."""
from __future__ import annotations

import subprocess
from unittest.mock import MagicMock

import pytest

from stoai.intrinsics import ALL_INTRINSICS
from stoai.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Schema tests
# ---------------------------------------------------------------------------


def test_system_in_all_intrinsics():
    assert "system" in ALL_INTRINSICS
    info = ALL_INTRINSICS["system"]
    schema = info["schema"]
    # Only memory object
    assert schema["properties"]["object"]["enum"] == ["memory"]
    # Only diff and load actions
    assert schema["properties"]["action"]["enum"] == ["diff", "load"]


def test_system_wired_in_agent(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "system" in agent._intrinsics
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Constructor args (covenant / memory file paths)
# ---------------------------------------------------------------------------


def test_covenant_constructor_arg_writes_to_system(tmp_path):
    """covenant= constructor arg should write to system/covenant.md."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        covenant="You are a helpful agent",
    )
    covenant_file = agent.working_dir / "system" / "covenant.md"
    assert covenant_file.is_file()
    assert covenant_file.read_text() == "You are a helpful agent"
    agent.stop(timeout=1.0)


def test_memory_constructor_arg_writes_to_system(tmp_path):
    """memory= constructor arg should write to system/memory.md."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        memory="initial memory",
    )
    memory_file = agent.working_dir / "system" / "memory.md"
    assert memory_file.is_file()
    assert memory_file.read_text() == "initial memory"
    agent.stop(timeout=1.0)


def test_covenant_is_protected_section(tmp_path):
    """Covenant should be a protected prompt section."""
    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        covenant="researcher",
    )
    sections = agent._prompt_manager.list_sections()
    covenant_section = [s for s in sections if s["name"] == "covenant"]
    assert len(covenant_section) == 1
    assert covenant_section[0]["protected"] is True
    agent.stop(timeout=1.0)


def test_existing_system_files_not_overwritten(tmp_path):
    """If system/memory.md already exists, constructor arg should not overwrite it."""
    working_dir = tmp_path / "test"
    working_dir.mkdir()
    system_dir = working_dir / "system"
    system_dir.mkdir()
    (system_dir / "memory.md").write_text("existing content")

    agent = BaseAgent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        memory="constructor ltm",
    )
    assert (agent.working_dir / "system" / "memory.md").read_text() == "existing content"
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Handler tests (memory object only: diff / load)
# ---------------------------------------------------------------------------


def test_system_diff_memory(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        memory_file = agent.working_dir / "system" / "memory.md"
        memory_file.write_text("first version\n")
        agent._intrinsics["system"]({"action": "load", "object": "memory"})
        memory_file.write_text("second version\n")
        result = agent._intrinsics["system"]({"action": "diff", "object": "memory"})
        assert result["status"] == "ok"
        assert "first version" in result["git_diff"] or "second version" in result["git_diff"]
    finally:
        agent.stop()


def test_system_load_memory(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        memory_file = agent.working_dir / "system" / "memory.md"
        memory_file.write_text("# Memory\n\nimportant fact\n")
        result = agent._intrinsics["system"]({"action": "load", "object": "memory"})
        assert result["status"] == "ok"
        assert result["diff"]["changed"] is True
        section = agent._prompt_manager.read_section("memory")
        assert "important fact" in section
    finally:
        agent.stop()


def test_system_load_empty_removes_section(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        memory_file = agent.working_dir / "system" / "memory.md"
        memory_file.write_text("some content")
        agent._intrinsics["system"]({"action": "load", "object": "memory"})
        assert agent._prompt_manager.read_section("memory") is not None
        memory_file.write_text("")
        agent._intrinsics["system"]({"action": "load", "object": "memory"})
        section = agent._prompt_manager.read_section("memory")
        assert section is None or section.strip() == ""
    finally:
        agent.stop()


def test_system_diff_no_changes(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._intrinsics["system"]({"action": "diff", "object": "memory"})
        assert result["status"] == "ok"
        assert result["git_diff"] == ""
    finally:
        agent.stop()


def test_system_load_no_change_no_commit(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        agent._intrinsics["system"]({"action": "load", "object": "memory"})
        result = agent._intrinsics["system"]({"action": "load", "object": "memory"})
        assert result["diff"]["changed"] is False
    finally:
        agent.stop()


def test_system_unknown_action(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "view", "object": "memory"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_system_unknown_object(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["system"]({"action": "diff", "object": "role"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_system_creates_files_if_missing(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        import shutil
        system_dir = agent.working_dir / "system"
        if system_dir.exists():
            shutil.rmtree(system_dir)
        result = agent._intrinsics["system"]({"action": "diff", "object": "memory"})
        assert result["status"] == "ok"
        assert (agent.working_dir / "system" / "memory.md").is_file()
    finally:
        agent.stop()
