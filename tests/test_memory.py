"""Tests for memory intrinsic — long-term memory management."""
from __future__ import annotations

import subprocess
from unittest.mock import MagicMock

import pytest

from stoai.base_agent import BaseAgent
from stoai.intrinsics import ALL_INTRINSICS


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_memory_in_all_intrinsics():
    assert "memory" in ALL_INTRINSICS
    info = ALL_INTRINSICS["memory"]
    assert "schema" in info
    assert "description" in info
    assert info["handler"] is None


def test_memory_wired_in_agent(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "memory" in agent._intrinsics


def test_memory_can_be_disabled(tmp_path):
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        disabled_intrinsics={"memory"},
        base_dir=tmp_path,
    )
    assert "memory" not in agent._intrinsics


def test_memory_load_empty_file(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result = agent._handle_memory({"action": "load"})
        assert result["status"] == "ok"
        assert result["size_bytes"] == 0
        assert result["diff"]["changed"] is False
    finally:
        agent.stop()


def test_memory_load_after_edit(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        ltm_file.write_text("# Memory\n\n- important fact\n")

        result = agent._handle_memory({"action": "load"})
        assert result["status"] == "ok"
        assert result["size_bytes"] > 0
        assert "important fact" in result["content_preview"]
        assert result["diff"]["changed"] is True
        assert result["diff"]["commit"] is not None
        assert len(result["diff"]["commit"]) == 7
    finally:
        agent.stop()


def test_memory_load_injects_into_system_prompt(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        ltm_file.write_text("# Memory\n\nI learned something\n")

        agent._handle_memory({"action": "load"})

        section = agent._prompt_manager.read_section("ltm")
        assert section is not None
        assert "I learned something" in section
    finally:
        agent.stop()


def test_memory_load_empty_removes_section(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        ltm_file.write_text("some content")
        agent._handle_memory({"action": "load"})
        assert agent._prompt_manager.read_section("ltm") is not None

        ltm_file.write_text("")
        agent._handle_memory({"action": "load"})
        section = agent._prompt_manager.read_section("ltm")
        assert section is None or section.strip() == ""
    finally:
        agent.stop()


def test_memory_load_no_change_no_commit(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        result1 = agent._handle_memory({"action": "load"})
        result2 = agent._handle_memory({"action": "load"})
        assert result2["diff"]["changed"] is False
        assert result2["diff"]["commit"] is None
    finally:
        agent.stop()


def test_memory_load_git_diff_content(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        ltm_file.write_text("first version\n")
        agent._handle_memory({"action": "load"})

        ltm_file.write_text("second version\n")
        result = agent._handle_memory({"action": "load"})
        assert result["diff"]["changed"] is True
        assert "first version" in result["diff"]["git_diff"]
        assert "second version" in result["diff"]["git_diff"]
    finally:
        agent.stop()


def test_memory_unknown_action(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_memory({"action": "bogus"})
    assert "error" in result


# ---------------------------------------------------------------------------
# LTM migration
# ---------------------------------------------------------------------------


def test_ltm_migration_from_manifest(tmp_path):
    """Agent with ltm in manifest should migrate to ltm/ltm.md on init."""
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        base_dir=tmp_path,
        ltm="old manifest ltm content",
    )
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        assert ltm_file.is_file()
        assert ltm_file.read_text() == "old manifest ltm content"

        section = agent._prompt_manager.read_section("ltm")
        assert "old manifest ltm content" in section
    finally:
        agent.stop()


def test_ltm_migration_does_not_overwrite_existing_file(tmp_path):
    """If ltm/ltm.md already exists, migration should not overwrite it."""
    working_dir = tmp_path / "test"
    working_dir.mkdir()
    ltm_dir = working_dir / "ltm"
    ltm_dir.mkdir()
    (ltm_dir / "ltm.md").write_text("existing file content")

    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        base_dir=tmp_path,
        ltm="manifest ltm",
    )
    agent.start()
    try:
        ltm_file = agent.working_dir / "ltm" / "ltm.md"
        assert ltm_file.read_text() == "existing file content"
    finally:
        agent.stop()


def test_manifest_no_longer_stores_ltm(tmp_path):
    """After migration, manifest should not contain ltm field."""
    import json
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        base_dir=tmp_path,
        ltm="some ltm",
    )
    agent.start()
    agent.stop()

    manifest = json.loads((agent.working_dir / ".agent.json").read_text())
    assert "ltm" not in manifest


def test_auto_load_ltm_on_resume(tmp_path):
    """On resume, ltm/ltm.md should be auto-loaded into system prompt."""
    agent1 = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        base_dir=tmp_path,
        ltm="initial memory",
    )
    agent1.start()
    # Simulate agent updating LTM (file + prompt manager, as memory intrinsic does)
    ltm_file = agent1.working_dir / "ltm" / "ltm.md"
    ltm_file.write_text("updated memory from file")
    agent1._prompt_manager.write_section("ltm", "updated memory from file")
    agent1.stop()

    agent2 = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    section = agent2._prompt_manager.read_section("ltm")
    assert section is not None
    assert "updated memory from file" in section


def test_memory_creates_ltm_if_missing(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    try:
        import shutil
        ltm_dir = agent.working_dir / "ltm"
        if ltm_dir.exists():
            shutil.rmtree(ltm_dir)

        result = agent._handle_memory({"action": "load"})
        assert result["status"] == "ok"
        assert (agent.working_dir / "ltm" / "ltm.md").is_file()
    finally:
        agent.stop()
