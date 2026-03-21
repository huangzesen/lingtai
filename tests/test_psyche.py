"""Tests for psyche capability — self-knowledge management."""
from __future__ import annotations

import json
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
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    assert "eigen" not in agent._intrinsics
    assert "psyche" in agent._mcp_handlers
    agent.stop(timeout=1.0)


def test_psyche_manager_accessible(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    assert mgr is not None
    agent.stop(timeout=1.0)


def test_anima_alias_works(tmp_path):
    """'anima' should be an alias for 'psyche'."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    assert "eigen" not in agent._intrinsics
    assert "psyche" in agent._mcp_handlers
    mgr = agent.get_capability("anima")
    assert mgr is not None
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Character actions
# ---------------------------------------------------------------------------


def test_character_update_writes_character(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    mgr.handle({"object": "character", "action": "update", "content": "something"})
    mgr.handle({"object": "character", "action": "update", "content": ""})
    character = (agent.working_dir / "system" / "character.md").read_text()
    assert character == ""
    agent.stop(timeout=1.0)


def test_character_diff(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("psyche")
        mgr.handle({"object": "character", "action": "update", "content": "new character"})
        result = mgr.handle({"object": "character", "action": "diff"})
        assert result["status"] == "ok"
        assert "new character" in result["git_diff"]
    finally:
        agent.stop()


def test_character_load_combines_covenant_and_character(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
# Memory construct
# ---------------------------------------------------------------------------


def test_memory_construct_with_ids_and_notes(tmp_path):
    """construct builds memory from library entries + free text."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("psyche")
        r1 = mgr.handle({
            "object": "library", "action": "submit",
            "title": "Finding A", "summary": "About A.", "content": "Details about A.",
        })
        result = mgr.handle({
            "object": "memory", "action": "construct",
            "ids": [r1["id"]], "notes": "Working on task X.",
        })
        assert result["status"] == "ok"
        assert result["entries"] == 1

        md = (agent.working_dir / "system" / "memory.md").read_text()
        assert "Working on task X." in md
        assert "Finding A" in md
        assert "Details about A." in md
    finally:
        agent.stop()


def test_memory_construct_notes_only(tmp_path):
    """construct with only notes (no ids) works."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("psyche")
        result = mgr.handle({
            "object": "memory", "action": "construct",
            "notes": "Just some free text notes.",
        })
        assert result["status"] == "ok"
        assert result["entries"] == 0

        md = (agent.working_dir / "system" / "memory.md").read_text()
        assert "Just some free text notes." in md
    finally:
        agent.stop()


def test_memory_construct_ids_only(tmp_path):
    """construct with only ids (no notes) works."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("psyche")
        r1 = mgr.handle({
            "object": "library", "action": "submit",
            "title": "Entry X", "summary": "About X.", "content": "Content X.",
        })
        result = mgr.handle({
            "object": "memory", "action": "construct",
            "ids": [r1["id"]],
        })
        assert result["status"] == "ok"
        assert result["entries"] == 1

        md = (agent.working_dir / "system" / "memory.md").read_text()
        assert "Entry X" in md
        assert "Content X." in md
    finally:
        agent.stop()


def test_memory_construct_empty_rejects(tmp_path):
    """construct with no ids and no notes returns error."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "memory", "action": "construct"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_memory_construct_invalid_ids(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "memory", "action": "construct", "ids": ["nope"]})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Memory load (delegates to eigen)
# ---------------------------------------------------------------------------


def test_memory_load_delegates_to_eigen(tmp_path):
    """memory load should delegate to eigen's handler."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("psyche")
        # Write memory file
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
    """psyche molt calls through to eigen's handler."""
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
        agent_name="test", service=svc, base_dir=tmp_path,
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
# Library actions (same as anima, migrated)
# ---------------------------------------------------------------------------


def test_library_submit_creates_structured_entry(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({
        "object": "library", "action": "submit",
        "title": "TCP Retry Logic",
        "summary": "Covers retry backoff and failure modes.",
        "content": "The TCP mail service uses exponential backoff...",
    })
    assert result["status"] == "ok"
    assert "id" in result
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["title"] == "TCP Retry Logic"
    agent.stop(timeout=1.0)


def test_library_filter_with_pattern(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    mgr.handle({"object": "library", "action": "submit",
                 "title": "TCP Retry", "summary": "About TCP.", "content": "Backoff logic."})
    mgr.handle({"object": "library", "action": "submit",
                 "title": "HTTP Caching", "summary": "About HTTP.", "content": "Cache rules."})
    result = mgr.handle({"object": "library", "action": "filter", "pattern": "TCP"})
    assert len(result["entries"]) == 1
    assert result["entries"][0]["title"] == "TCP Retry"
    agent.stop(timeout=1.0)


def test_library_consolidate(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    r1 = mgr.handle({"object": "library", "action": "submit",
                      "title": "A", "summary": "s1.", "content": "c1"})
    r2 = mgr.handle({"object": "library", "action": "submit",
                      "title": "B", "summary": "s2.", "content": "c2"})
    result = mgr.handle({
        "object": "library", "action": "consolidate",
        "ids": [r1["id"], r2["id"]],
        "title": "AB Combined",
        "summary": "Merged A and B.",
        "content": "Combined content.",
    })
    assert result["status"] == "ok"
    assert result["removed"] == 2
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["title"] == "AB Combined"
    agent.stop(timeout=1.0)


def test_library_delete(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    r1 = mgr.handle({"object": "library", "action": "submit",
                      "title": "A", "summary": "s.", "content": "c"})
    r2 = mgr.handle({"object": "library", "action": "submit",
                      "title": "B", "summary": "s.", "content": "c"})
    result = mgr.handle({"object": "library", "action": "delete",
                          "ids": [r1["id"]]})
    assert result["status"] == "ok"
    assert result["removed"] == 1
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["id"] == r2["id"]
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Schema checks
# ---------------------------------------------------------------------------


def test_forget_not_in_schema():
    """forget is not exposed in psyche's SCHEMA actions."""
    from lingtai.capabilities.psyche import SCHEMA
    actions = SCHEMA["properties"]["action"]["enum"]
    assert "forget" not in actions


def test_psyche_schema_has_construct():
    """Schema should include construct action and notes/ids fields."""
    from lingtai.capabilities.psyche import SCHEMA
    actions = SCHEMA["properties"]["action"]["enum"]
    assert "construct" in actions
    assert "molt" in actions
    props = SCHEMA["properties"]
    assert "ids" in props
    assert "notes" in props


def test_psyche_schema_has_library_fields():
    """Schema should include title, summary, supplementary, pattern, limit, depth."""
    from lingtai.capabilities.psyche import SCHEMA
    props = SCHEMA["properties"]
    assert "title" in props
    assert "summary" in props
    assert "supplementary" in props
    assert "pattern" in props
    assert "limit" in props
    assert "depth" in props


# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------


def test_invalid_object(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "bogus", "action": "diff"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_invalid_action_for_object(tmp_path):
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "character", "action": "submit"})
    assert "error" in result
    assert "update" in result["error"]
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Memory ID generation
# ---------------------------------------------------------------------------


def test_memory_id_deterministic(tmp_path):
    from lingtai.capabilities.psyche import PsycheManager
    id1 = PsycheManager._make_id("hello", "2026-03-16T00:00:00Z")
    id2 = PsycheManager._make_id("hello", "2026-03-16T00:00:00Z")
    assert id1 == id2
    assert len(id1) == 8


def test_memory_id_differs_by_content(tmp_path):
    from lingtai.capabilities.psyche import PsycheManager
    id1 = PsycheManager._make_id("hello", "2026-03-16T00:00:00Z")
    id2 = PsycheManager._make_id("world", "2026-03-16T00:00:00Z")
    assert id1 != id2


# ---------------------------------------------------------------------------
# Migration from existing memory
# ---------------------------------------------------------------------------


def test_psyche_migrates_memory_to_library(tmp_path):
    """If memory.md exists, psyche should migrate it to a library entry."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        memory="I know about CDF format",
        capabilities=["psyche"],
    )
    mgr = agent.get_capability("psyche")
    result = mgr.handle({"object": "library", "action": "filter"})
    assert len(result["entries"]) == 1
    assert "CDF" in result["entries"][0]["summary"]
    agent.stop(timeout=1.0)


def test_psyche_stop_does_not_overwrite_memory_md(tmp_path):
    """When psyche is active, stop() should not write memory.md."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["psyche"],
    )
    mem_file = agent.working_dir / "system" / "memory.md"
    mem_file.parent.mkdir(exist_ok=True)
    mem_file.write_text("previous session memory")
    agent.stop()
    assert mem_file.read_text() == "previous session memory"
