"""Tests for anima capability — self-knowledge management."""
from __future__ import annotations

import json
from unittest.mock import MagicMock

import pytest

from stoai.agent import Agent
from stoai.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------


def test_anima_setup_removes_system_intrinsic(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    assert "system" not in agent._intrinsics
    assert "anima" in agent._mcp_handlers
    agent.stop(timeout=1.0)


def test_anima_manager_accessible(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    assert mgr is not None
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Role actions
# ---------------------------------------------------------------------------


def test_role_update_writes_character(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        role="You are helpful",
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "role", "action": "update", "content": "I am a PDF specialist"})
    assert result["status"] == "ok"
    character = (agent.working_dir / "system" / "character.md").read_text()
    assert character == "I am a PDF specialist"
    agent.stop(timeout=1.0)


def test_role_update_empty_clears_character(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    mgr.handle({"object": "role", "action": "update", "content": "something"})
    mgr.handle({"object": "role", "action": "update", "content": ""})
    character = (agent.working_dir / "system" / "character.md").read_text()
    assert character == ""
    agent.stop(timeout=1.0)


def test_role_diff(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        mgr.handle({"object": "role", "action": "update", "content": "new character"})
        result = mgr.handle({"object": "role", "action": "diff"})
        assert result["status"] == "ok"
        assert "new character" in result["git_diff"]
    finally:
        agent.stop()


def test_role_load_combines_covenant_and_character(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        role="You are helpful",
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        mgr.handle({"object": "role", "action": "update", "content": "I specialize in PDFs"})
        mgr.handle({"object": "role", "action": "load"})
        section = agent._prompt_manager.read_section("covenant")
        assert "You are helpful" in section
        assert "I specialize in PDFs" in section
    finally:
        agent.stop()


# ---------------------------------------------------------------------------
# Memory actions
# ---------------------------------------------------------------------------


def test_memory_submit(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "memory", "action": "submit",
        "content": "Agent bob knows CDF format",
    })
    assert result["status"] == "ok"
    assert "id" in result
    assert len(result["id"]) == 8
    # Check JSON
    data = json.loads((agent.working_dir / "system" / "memory.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["content"] == "Agent bob knows CDF format"
    # Check rendered markdown
    md = (agent.working_dir / "system" / "memory.md").read_text()
    assert result["id"] in md
    assert "Agent bob knows CDF format" in md
    agent.stop(timeout=1.0)


def test_memory_submit_empty_rejected(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "memory", "action": "submit", "content": ""})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_memory_consolidate(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r1 = mgr.handle({"object": "memory", "action": "submit", "content": "fact A"})
    r2 = mgr.handle({"object": "memory", "action": "submit", "content": "fact B"})

    result = mgr.handle({
        "object": "memory", "action": "consolidate",
        "ids": [r1["id"], r2["id"]],
        "content": "combined fact AB",
    })
    assert result["status"] == "ok"
    assert result["removed"] == 2
    assert "id" in result

    # Only one entry left
    data = json.loads((agent.working_dir / "system" / "memory.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["content"] == "combined fact AB"
    agent.stop(timeout=1.0)


def test_memory_consolidate_invalid_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "memory", "action": "consolidate",
        "ids": ["nonexist"],
        "content": "merged",
    })
    assert "error" in result
    assert "nonexist" in result["error"]
    agent.stop(timeout=1.0)


def test_memory_consolidate_no_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "memory", "action": "consolidate", "content": "x"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_memory_diff_delegates_to_system(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        mgr.handle({"object": "memory", "action": "submit", "content": "test entry"})
        result = mgr.handle({"object": "memory", "action": "diff"})
        assert result["status"] == "ok"
    finally:
        agent.stop()


def test_memory_load_delegates_to_system(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        mgr.handle({"object": "memory", "action": "submit", "content": "test entry"})
        result = mgr.handle({"object": "memory", "action": "load"})
        assert result["status"] == "ok"
        section = agent._prompt_manager.read_section("memory")
        assert "test entry" in section
    finally:
        agent.stop()


# ---------------------------------------------------------------------------
# Context actions
# ---------------------------------------------------------------------------


def test_context_compact_requires_prompt(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "context", "action": "compact"})
    assert "error" in result
    assert "prompt" in result["error"]
    agent.stop(timeout=1.0)


def test_context_compact_no_session(tmp_path):
    """Compact without active chat should return error."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "context", "action": "compact", "prompt": ""})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------


def test_invalid_object(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "bogus", "action": "diff"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_invalid_action_for_object(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "role", "action": "submit"})
    assert "error" in result
    assert "update" in result["error"]  # should list valid actions
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Memory ID generation
# ---------------------------------------------------------------------------


def test_memory_id_deterministic(tmp_path):
    """Same content + timestamp should produce same ID."""
    from stoai.capabilities.anima import AnimaManager
    id1 = AnimaManager._make_id("hello", "2026-03-16T00:00:00Z")
    id2 = AnimaManager._make_id("hello", "2026-03-16T00:00:00Z")
    assert id1 == id2
    assert len(id1) == 8


def test_memory_id_differs_by_content(tmp_path):
    from stoai.capabilities.anima import AnimaManager
    id1 = AnimaManager._make_id("hello", "2026-03-16T00:00:00Z")
    id2 = AnimaManager._make_id("world", "2026-03-16T00:00:00Z")
    assert id1 != id2
