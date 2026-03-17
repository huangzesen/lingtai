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
        covenant="You are helpful",
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
        covenant="You are helpful",
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


def test_memory_diff_delegates_to_system(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        r = mgr.handle({"object": "library", "action": "submit",
                         "title": "T", "summary": "s.", "content": "test entry"})
        mgr.handle({"object": "memory", "action": "load", "ids": [r["id"]]})
        result = mgr.handle({"object": "memory", "action": "diff"})
        assert result["status"] == "ok"
    finally:
        agent.stop()


def test_library_to_memory_workflow(tmp_path):
    """Full workflow: submit → filter → view → load → verify prompt."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        # Submit entries
        r1 = mgr.handle({
            "object": "library", "action": "submit",
            "title": "Mail Protocol",
            "summary": "FIFO mail queue and TCP transport.",
            "content": "The mail service uses a FIFO queue with TCP transport.",
            "supplementary": "Detailed protocol spec...",
        })
        r2 = mgr.handle({
            "object": "library", "action": "submit",
            "title": "File I/O",
            "summary": "Local filesystem service for file operations.",
            "content": "FileIOService wraps read, write, edit, glob, grep.",
        })
        # Filter
        filtered = mgr.handle({"object": "library", "action": "filter",
                                 "pattern": "mail"})
        assert len(filtered["entries"]) == 1
        assert filtered["entries"][0]["id"] == r1["id"]
        # View at content depth
        viewed = mgr.handle({"object": "library", "action": "view",
                               "ids": [r1["id"]]})
        assert "FIFO queue" in viewed["entries"][0]["content"]
        assert "supplementary" not in viewed["entries"][0]
        # View at supplementary depth
        viewed_deep = mgr.handle({"object": "library", "action": "view",
                                    "ids": [r1["id"]], "depth": "supplementary"})
        assert "protocol spec" in viewed_deep["entries"][0]["supplementary"]
        # Load into memory
        loaded = mgr.handle({"object": "memory", "action": "load",
                               "ids": [r1["id"], r2["id"]]})
        assert loaded["status"] == "ok"
        section = agent._prompt_manager.read_section("memory")
        assert "Mail Protocol" in section
        assert "File I/O" in section
        # Summary should NOT be in memory
        assert "FIFO mail queue and TCP transport." not in section
        # Content should be in memory
        assert "FIFO queue with TCP transport" in section
    finally:
        agent.stop()


def test_memory_load_selective(tmp_path):
    """Memory load should inject only selected entries (id + title + content) into prompt."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        r1 = mgr.handle({"object": "library", "action": "submit",
                          "title": "Entry A", "summary": "sA.", "content": "Content A."})
        r2 = mgr.handle({"object": "library", "action": "submit",
                          "title": "Entry B", "summary": "sB.", "content": "Content B."})
        r3 = mgr.handle({"object": "library", "action": "submit",
                          "title": "Entry C", "summary": "sC.", "content": "Content C."})
        # Load only A and C
        result = mgr.handle({"object": "memory", "action": "load",
                              "ids": [r1["id"], r3["id"]]})
        assert result["status"] == "ok"
        section = agent._prompt_manager.read_section("memory")
        assert "Entry A" in section
        assert "Content A" in section
        assert "Entry C" in section
        assert "Content C" in section
        assert "Entry B" not in section
        # Summary should NOT be in memory section
        assert "sA." not in section
    finally:
        agent.stop()


def test_memory_load_requires_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "memory", "action": "load"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_memory_load_replaces_previous(tmp_path):
    """Each load replaces the entire memory section."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        r1 = mgr.handle({"object": "library", "action": "submit",
                          "title": "A", "summary": "s.", "content": "cA"})
        r2 = mgr.handle({"object": "library", "action": "submit",
                          "title": "B", "summary": "s.", "content": "cB"})
        # Load A
        mgr.handle({"object": "memory", "action": "load", "ids": [r1["id"]]})
        section = agent._prompt_manager.read_section("memory")
        assert "cA" in section
        # Load B (replaces A)
        mgr.handle({"object": "memory", "action": "load", "ids": [r2["id"]]})
        section = agent._prompt_manager.read_section("memory")
        assert "cB" in section
        assert "cA" not in section
    finally:
        agent.stop()


def test_memory_load_invalid_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "memory", "action": "load", "ids": ["nope"]})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_memory_load_writes_memory_md(tmp_path):
    """Load should also write memory.md to disk."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    agent.start()
    try:
        mgr = agent.get_capability("anima")
        r = mgr.handle({"object": "library", "action": "submit",
                         "title": "X", "summary": "s.", "content": "body"})
        mgr.handle({"object": "memory", "action": "load", "ids": [r["id"]]})
        md = (agent.working_dir / "system" / "memory.md").read_text()
        assert "X" in md
        assert "body" in md
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


def test_anima_schema_has_library_fields():
    """Schema should include title, summary, supplementary, pattern, limit, depth."""
    from stoai.capabilities.anima import SCHEMA
    props = SCHEMA["properties"]
    assert "title" in props
    assert "summary" in props
    assert "supplementary" in props
    assert "pattern" in props
    assert "limit" in props
    assert "depth" in props
    assert props["depth"]["enum"] == ["content", "supplementary"]
    # object enum should include library
    assert "library" in props["object"]["enum"]


def test_library_submit_creates_structured_entry(tmp_path):
    """Submit should require title, summary, content and store structured entry."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "library", "action": "submit",
        "title": "TCP Retry Logic",
        "summary": "Covers retry backoff and failure modes.",
        "content": "The TCP mail service uses exponential backoff...",
    })
    assert result["status"] == "ok"
    assert "id" in result
    # Check library.json structure
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert len(data["entries"]) == 1
    entry = data["entries"][0]
    assert entry["title"] == "TCP Retry Logic"
    assert entry["summary"] == "Covers retry backoff and failure modes."
    assert entry["content"] == "The TCP mail service uses exponential backoff..."
    assert entry["supplementary"] == ""
    assert "created_at" in entry
    agent.stop(timeout=1.0)


def test_library_submit_with_supplementary(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "library", "action": "submit",
        "title": "Mail Protocol",
        "summary": "FIFO mail queue internals.",
        "content": "Main body here.",
        "supplementary": "Extended appendix data...",
    })
    assert result["status"] == "ok"
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert data["entries"][0]["supplementary"] == "Extended appendix data..."
    agent.stop(timeout=1.0)


def test_library_submit_requires_title_summary_content(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    # Missing title
    r1 = mgr.handle({"object": "library", "action": "submit",
                      "summary": "s", "content": "c"})
    assert "error" in r1
    # Missing summary
    r2 = mgr.handle({"object": "library", "action": "submit",
                      "title": "t", "content": "c"})
    assert "error" in r2
    # Missing content
    r3 = mgr.handle({"object": "library", "action": "submit",
                      "title": "t", "summary": "s"})
    assert "error" in r3
    agent.stop(timeout=1.0)


def test_anima_migrates_ltm_to_library(tmp_path):
    """If ltm is provided, anima should migrate it to a library entry."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        memory="I know about CDF format",
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "filter"})
    assert len(result["entries"]) == 1
    assert "CDF" in result["entries"][0]["summary"]
    agent.stop(timeout=1.0)


def test_anima_stop_does_not_overwrite_memory_md(tmp_path):
    """When anima is active, stop() should not write memory.md."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    # Write something to memory.md manually to simulate previous session state
    mem_file = agent.working_dir / "system" / "memory.md"
    mem_file.parent.mkdir(exist_ok=True)
    mem_file.write_text("previous session memory")
    agent.stop()
    assert mem_file.read_text() == "previous session memory"


def test_library_filter_all(tmp_path):
    """Filter with no pattern returns all entries."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    mgr.handle({"object": "library", "action": "submit",
                 "title": "Entry A", "summary": "About A.", "content": "Details A."})
    mgr.handle({"object": "library", "action": "submit",
                 "title": "Entry B", "summary": "About B.", "content": "Details B."})
    result = mgr.handle({"object": "library", "action": "filter"})
    assert result["status"] == "ok"
    assert len(result["entries"]) == 2
    for e in result["entries"]:
        assert "id" in e
        assert "title" in e
        assert "summary" in e
        assert "content" not in e
        assert "supplementary" not in e
    agent.stop(timeout=1.0)


def test_library_filter_with_pattern(tmp_path):
    """Filter with regex pattern matches against title, summary, and content."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    mgr.handle({"object": "library", "action": "submit",
                 "title": "TCP Retry", "summary": "About TCP.", "content": "Backoff logic."})
    mgr.handle({"object": "library", "action": "submit",
                 "title": "HTTP Caching", "summary": "About HTTP.", "content": "Cache rules."})
    result = mgr.handle({"object": "library", "action": "filter", "pattern": "TCP"})
    assert len(result["entries"]) == 1
    assert result["entries"][0]["title"] == "TCP Retry"
    agent.stop(timeout=1.0)


def test_library_filter_matches_content(tmp_path):
    """Filter should match against content field, not just title."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    mgr.handle({"object": "library", "action": "submit",
                 "title": "Entry A", "summary": "About A.", "content": "Uses exponential backoff."})
    mgr.handle({"object": "library", "action": "submit",
                 "title": "Entry B", "summary": "About B.", "content": "Simple linear scan."})
    result = mgr.handle({"object": "library", "action": "filter", "pattern": "exponential"})
    assert len(result["entries"]) == 1
    assert result["entries"][0]["title"] == "Entry A"
    agent.stop(timeout=1.0)


def test_library_filter_with_limit(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    for i in range(5):
        mgr.handle({"object": "library", "action": "submit",
                     "title": f"Entry {i}", "summary": f"About {i}.", "content": f"Details {i}."})
    result = mgr.handle({"object": "library", "action": "filter", "limit": 3})
    assert len(result["entries"]) == 3
    agent.stop(timeout=1.0)


def test_library_filter_invalid_regex(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "filter", "pattern": "[invalid"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_view_content_depth(tmp_path):
    """View with depth=content returns id, title, summary, content."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r = mgr.handle({"object": "library", "action": "submit",
                     "title": "A", "summary": "S.", "content": "C.",
                     "supplementary": "Supp."})
    result = mgr.handle({"object": "library", "action": "view",
                          "ids": [r["id"]], "depth": "content"})
    assert result["status"] == "ok"
    assert len(result["entries"]) == 1
    e = result["entries"][0]
    assert e["title"] == "A"
    assert e["summary"] == "S."
    assert e["content"] == "C."
    assert "supplementary" not in e
    agent.stop(timeout=1.0)


def test_library_view_supplementary_depth(tmp_path):
    """View with depth=supplementary returns everything."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r = mgr.handle({"object": "library", "action": "submit",
                     "title": "A", "summary": "S.", "content": "C.",
                     "supplementary": "Supp."})
    result = mgr.handle({"object": "library", "action": "view",
                          "ids": [r["id"]], "depth": "supplementary"})
    assert result["status"] == "ok"
    e = result["entries"][0]
    assert e["supplementary"] == "Supp."
    agent.stop(timeout=1.0)


def test_library_view_default_depth_is_content(tmp_path):
    """View without explicit depth defaults to content."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r = mgr.handle({"object": "library", "action": "submit",
                     "title": "A", "summary": "S.", "content": "C.",
                     "supplementary": "Supp."})
    result = mgr.handle({"object": "library", "action": "view", "ids": [r["id"]]})
    assert result["status"] == "ok"
    e = result["entries"][0]
    assert "content" in e
    assert "supplementary" not in e
    agent.stop(timeout=1.0)


def test_library_view_requires_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "view"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_view_unknown_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "view", "ids": ["nope"]})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_consolidate(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
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
    assert "id" in result
    data = json.loads((agent.working_dir / "system" / "library.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["title"] == "AB Combined"
    agent.stop(timeout=1.0)


def test_library_consolidate_requires_title_summary_content(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    r = mgr.handle({"object": "library", "action": "submit",
                     "title": "X", "summary": "s.", "content": "c"})
    # Missing title
    result = mgr.handle({"object": "library", "action": "consolidate",
                          "ids": [r["id"]], "summary": "s", "content": "c"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_consolidate_invalid_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({
        "object": "library", "action": "consolidate",
        "ids": ["nonexist"], "title": "T", "summary": "s.", "content": "c",
    })
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_delete(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
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


def test_library_delete_invalid_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "delete", "ids": ["nope"]})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_library_delete_requires_ids(tmp_path):
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["anima"],
    )
    mgr = agent.get_capability("anima")
    result = mgr.handle({"object": "library", "action": "delete"})
    assert "error" in result
    agent.stop(timeout=1.0)
