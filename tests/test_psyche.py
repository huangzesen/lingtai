"""Tests for psyche capability — identity, memory, and context management."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from lingtai.agent import Agent
from lingtai_kernel.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------


def test_psyche_setup_removes_eigen_intrinsic(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    assert "eigen" not in agent._intrinsics
    assert "psyche" in agent._tool_handlers
    agent.stop(timeout=1.0)


def test_psyche_manager_accessible(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    assert mgr is not None
    agent.stop(timeout=1.0)


def test_anima_alias_removed(tmp_path):
    """'anima' alias was removed — should raise ValueError."""
    import pytest
    with pytest.raises(ValueError, match="Unknown capability"):
        Agent(
            service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
            capabilities=["anima"],
        )


# ---------------------------------------------------------------------------
# Character actions
# ---------------------------------------------------------------------------


def test_character_update_writes_character(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        covenant="You are helpful",
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "character", "action": "update", "content": "I am a PDF specialist"})
    assert result["status"] == "ok"
    character = (agent.working_dir / "system" / "character.md").read_text()
    assert character == "I am a PDF specialist"
    agent.stop(timeout=1.0)


def test_character_update_empty_clears_character(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    mgr.handle({"object": "character", "action": "update", "content": "something"})
    mgr.handle({"object": "character", "action": "update", "content": ""})
    character = (agent.working_dir / "system" / "character.md").read_text()
    assert character == ""
    agent.stop(timeout=1.0)


def test_character_load_combines_covenant_and_character(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        covenant="You are helpful",
        capabilities=["psyche"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("psyche")
        mgr.handle({"object": "character", "action": "update", "content": "I specialize in PDFs"})
        mgr.handle({"object": "character", "action": "load"})
        section = agent._prompt_manager.read_section("covenant")
        assert "You are helpful" in section
        assert "I specialize in PDFs" in section
    finally:
        agent.stop()


# ---------------------------------------------------------------------------
# Memory edit (upgraded with files support)
# ---------------------------------------------------------------------------


def test_memory_edit_content_only(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "memory", "action": "edit", "content": "my notes"})
    assert result["status"] == "ok"
    md = (agent.working_dir / "system" / "memory.md").read_text()
    assert "my notes" in md
    agent.stop(timeout=1.0)


def test_memory_edit_with_files(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    # Create files to import
    (agent.working_dir / "export1.txt").write_text("knowledge from export 1")
    (agent.working_dir / "export2.txt").write_text("knowledge from export 2")

    mgr = agent.get_capability("psyche")
    result = mgr.handle({
        "object": "memory", "action": "edit",
        "content": "My working notes.",
        "files": ["export1.txt", "export2.txt"],
    })
    assert result["status"] == "ok"
    md = (agent.working_dir / "system" / "memory.md").read_text()
    assert "My working notes." in md
    assert "[file-1]" in md
    assert "knowledge from export 1" in md
    assert "[file-2]" in md
    assert "knowledge from export 2" in md
    agent.stop(timeout=1.0)


def test_memory_edit_files_only(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    (agent.working_dir / "data.txt").write_text("file data")

    mgr = agent.get_capability("psyche")
    result = mgr.handle({
        "object": "memory", "action": "edit",
        "files": ["data.txt"],
    })
    assert result["status"] == "ok"
    md = (agent.working_dir / "system" / "memory.md").read_text()
    assert "[file-1]" in md
    assert "file data" in md
    agent.stop(timeout=1.0)


def test_memory_edit_missing_file_errors(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({
        "object": "memory", "action": "edit",
        "content": "notes",
        "files": ["nonexistent.txt"],
    })
    assert "error" in result
    assert "nonexistent.txt" in result["error"]
    agent.stop(timeout=1.0)


def test_memory_edit_empty_errors(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "memory", "action": "edit"})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Memory load (delegates to eigen)
# ---------------------------------------------------------------------------


def test_memory_load_delegates_to_eigen(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("psyche")
        system_dir = agent._working_dir / "system"
        system_dir.mkdir(exist_ok=True)
        (system_dir / "memory.md").write_text("loaded via eigen")

        result = mgr.handle({"object": "memory", "action": "load"})
        assert result["status"] == "ok"
        section = agent._prompt_manager.read_section("memory")
        assert "loaded via eigen" in section
    finally:
        agent.stop()


# ---------------------------------------------------------------------------
# Molt (delegates to eigen)
# ---------------------------------------------------------------------------


def test_molt_delegates_to_eigen(tmp_path):
    from lingtai_kernel.llm.interface import ChatInterface, TextBlock

    svc = make_mock_service()

    def fake_create_session(**kwargs):
        mock_chat = MagicMock()
        iface = ChatInterface()
        iface.add_system("You are helpful.")
        mock_chat.interface = iface
        mock_chat.context_window.return_value = 100_000
        return mock_chat

    svc.create_session.side_effect = fake_create_session

    agent = Agent(
        service=svc, agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    agent.start()
    try:
        agent._session.ensure_session()
        agent._session._chat.interface.add_user_message("Hello")
        agent._session._chat.interface.add_assistant_message(
            [TextBlock(text="Hi there.")],
        )

        mgr = agent.get_capability("psyche")
        result = mgr.handle({
            "object": "context",
            "action": "molt",
            "summary": "Key findings: X=42. Current task: analyze dataset Z.",
        })

        assert result["status"] == "ok"
        iface = agent._session._chat.interface
        entries = [e for e in iface.entries if e.role == "user"]
        assert any("X=42" in str(e.content) for e in entries)
    finally:
        agent.stop()


# ---------------------------------------------------------------------------
# Schema checks
# ---------------------------------------------------------------------------


def test_psyche_schema_has_correct_objects():
    from lingtai.capabilities.psyche import get_schema
    SCHEMA = get_schema("en")
    objects = SCHEMA["properties"]["object"]["enum"]
    assert set(objects) == {"character", "memory", "context"}


def test_psyche_schema_has_correct_actions():
    from lingtai.capabilities.psyche import get_schema
    SCHEMA = get_schema("en")
    actions = SCHEMA["properties"]["action"]["enum"]
    assert set(actions) == {"update", "load", "edit", "molt"}


def test_psyche_schema_has_files_field():
    from lingtai.capabilities.psyche import get_schema
    SCHEMA = get_schema("en")
    assert "files" in SCHEMA["properties"]


# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------


def test_invalid_object(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "bogus", "action": "diff"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_invalid_action_for_object(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "character", "action": "submit"})
    assert "error" in result
    assert "update" in result["error"]
    agent.stop(timeout=1.0)


def test_psyche_stop_does_not_overwrite_memory_md(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["psyche"],
    )
    mem_file = agent.working_dir / "system" / "memory.md"
    mem_file.parent.mkdir(exist_ok=True)
    mem_file.write_text("previous session memory")
    agent.stop()
    assert mem_file.read_text() == "previous session memory"
