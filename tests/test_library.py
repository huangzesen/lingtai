"""Tests for library capability — standalone knowledge store."""
from __future__ import annotations

import json
from unittest.mock import MagicMock

import pytest

from lingtai.agent import Agent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------


def test_library_setup_registers_tool(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    assert "library" in agent._tool_handlers
    agent.stop(timeout=1.0)


def test_library_manager_accessible(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    assert mgr is not None
    agent.stop(timeout=1.0)


def test_library_independent_of_psyche(tmp_path):
    """Library can be used without psyche — eigen stays intact."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    assert "eigen" in agent._intrinsics
    assert "library" in agent._tool_handlers
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Submit
# ---------------------------------------------------------------------------


def test_submit_creates_entry(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    result = mgr.handle({
        "action": "submit",
        "title": "TCP Retry Logic",
        "summary": "Covers retry backoff and failure modes.",
        "content": "The TCP mail service uses exponential backoff...",
    })
    assert result["status"] == "ok"
    assert "id" in result
    data = json.loads((agent.working_dir / "library" / "library.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["title"] == "TCP Retry Logic"
    agent.stop(timeout=1.0)


def test_submit_requires_title(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    result = mgr.handle({"action": "submit", "summary": "s", "content": "c"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_submit_enforces_limit(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities={"library": {"library_limit": 2}},
    )
    mgr = agent.get_capability("library")
    mgr.handle({"action": "submit", "title": "A", "summary": "s", "content": "c"})
    mgr.handle({"action": "submit", "title": "B", "summary": "s", "content": "c"})
    result = mgr.handle({"action": "submit", "title": "C", "summary": "s", "content": "c"})
    assert "error" in result
    assert "full" in result["error"].lower()
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Filter
# ---------------------------------------------------------------------------


def test_filter_with_pattern(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    mgr.handle({"action": "submit", "title": "TCP Retry", "summary": "About TCP.", "content": "Backoff logic."})
    mgr.handle({"action": "submit", "title": "HTTP Caching", "summary": "About HTTP.", "content": "Cache rules."})
    result = mgr.handle({"action": "filter", "pattern": "TCP"})
    assert len(result["entries"]) == 1
    assert result["entries"][0]["title"] == "TCP Retry"
    agent.stop(timeout=1.0)


def test_filter_all(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    mgr.handle({"action": "submit", "title": "A", "summary": "s", "content": "c"})
    mgr.handle({"action": "submit", "title": "B", "summary": "s", "content": "c"})
    result = mgr.handle({"action": "filter"})
    assert len(result["entries"]) == 2
    agent.stop(timeout=1.0)


def test_filter_with_limit(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    mgr.handle({"action": "submit", "title": "A", "summary": "s", "content": "c"})
    mgr.handle({"action": "submit", "title": "B", "summary": "s", "content": "c"})
    result = mgr.handle({"action": "filter", "limit": 1})
    assert len(result["entries"]) == 1
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# View
# ---------------------------------------------------------------------------


def test_view_returns_content(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    r = mgr.handle({"action": "submit", "title": "X", "summary": "s", "content": "full content here"})
    result = mgr.handle({"action": "view", "ids": [r["id"]]})
    assert result["status"] == "ok"
    assert result["entries"][0]["content"] == "full content here"
    agent.stop(timeout=1.0)


def test_view_invalid_id(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    result = mgr.handle({"action": "view", "ids": ["nope"]})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Consolidate
# ---------------------------------------------------------------------------


def test_consolidate(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    r1 = mgr.handle({"action": "submit", "title": "A", "summary": "s1.", "content": "c1"})
    r2 = mgr.handle({"action": "submit", "title": "B", "summary": "s2.", "content": "c2"})
    result = mgr.handle({
        "action": "consolidate",
        "ids": [r1["id"], r2["id"]],
        "title": "AB Combined",
        "summary": "Merged A and B.",
        "content": "Combined content.",
    })
    assert result["status"] == "ok"
    assert result["removed"] == 2
    data = json.loads((agent.working_dir / "library" / "library.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["title"] == "AB Combined"
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Delete
# ---------------------------------------------------------------------------


def test_delete(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    r1 = mgr.handle({"action": "submit", "title": "A", "summary": "s.", "content": "c"})
    r2 = mgr.handle({"action": "submit", "title": "B", "summary": "s.", "content": "c"})
    result = mgr.handle({"action": "delete", "ids": [r1["id"]]})
    assert result["status"] == "ok"
    assert result["removed"] == 1
    data = json.loads((agent.working_dir / "library" / "library.json").read_text())
    assert len(data["entries"]) == 1
    assert data["entries"][0]["id"] == r2["id"]
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Export
# ---------------------------------------------------------------------------


def test_export_creates_files(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    r = mgr.handle({
        "action": "submit", "title": "TCP Retry",
        "summary": "Backoff logic.", "content": "Exponential backoff details.",
    })
    result = mgr.handle({"action": "export", "ids": [r["id"]]})
    assert result["status"] == "ok"
    assert result["count"] == 1
    assert len(result["files"]) == 1

    # File should exist and contain the entry content
    export_path = agent.working_dir / result["files"][0]
    assert export_path.is_file()
    text = export_path.read_text()
    assert "TCP Retry" in text
    assert "Exponential backoff details." in text
    agent.stop(timeout=1.0)


def test_export_includes_supplementary(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    r = mgr.handle({
        "action": "submit", "title": "Deep Dive",
        "summary": "s.", "content": "Main content.",
        "supplementary": "Extended material here.",
    })
    result = mgr.handle({"action": "export", "ids": [r["id"]]})
    text = (agent.working_dir / result["files"][0]).read_text()
    assert "Extended material here." in text
    agent.stop(timeout=1.0)


def test_export_returns_relative_paths(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    r = mgr.handle({"action": "submit", "title": "A", "summary": "s.", "content": "c"})
    result = mgr.handle({"action": "export", "ids": [r["id"]]})
    # Paths should be relative (for use in memory.edit files param)
    for f in result["files"]:
        assert not f.startswith("/")
        assert f.startswith("exports/")
    agent.stop(timeout=1.0)


def test_export_invalid_id(tmp_path):
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library"],
    )
    mgr = agent.get_capability("library")
    result = mgr.handle({"action": "export", "ids": ["nope"]})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_export_to_memory_edit_workflow(tmp_path):
    """Full workflow: library export → psyche memory.edit with files."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["library", "psyche"],
    )
    lib = agent.get_capability("library")
    psy = agent.get_capability("psyche")

    r = lib.handle({
        "action": "submit", "title": "Key Finding",
        "summary": "Important.", "content": "The answer is 42.",
    })
    export_result = lib.handle({"action": "export", "ids": [r["id"]]})

    mem_result = psy.handle({
        "object": "memory", "action": "edit",
        "content": "My working notes.",
        "files": export_result["files"],
    })
    assert mem_result["status"] == "ok"

    md = (agent.working_dir / "system" / "memory.md").read_text()
    assert "My working notes." in md
    assert "[file-1]" in md
    assert "The answer is 42." in md
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Schema
# ---------------------------------------------------------------------------


def test_schema_has_all_fields():
    from lingtai.capabilities.library import get_schema
    SCHEMA = get_schema("en")
    actions = SCHEMA["properties"]["action"]["enum"]
    assert set(actions) == {"submit", "filter", "view", "consolidate", "delete", "export"}
    props = SCHEMA["properties"]
    assert "title" in props
    assert "summary" in props
    assert "content" in props
    assert "supplementary" in props
    assert "ids" in props
    assert "pattern" in props
    assert "limit" in props
    assert "depth" in props


# ---------------------------------------------------------------------------
# ID generation
# ---------------------------------------------------------------------------


def test_id_deterministic():
    from lingtai.capabilities.library import LibraryManager
    id1 = LibraryManager._make_id("hello", "2026-03-16T00:00:00Z")
    id2 = LibraryManager._make_id("hello", "2026-03-16T00:00:00Z")
    assert id1 == id2
    assert len(id1) == 8


def test_id_differs_by_content():
    from lingtai.capabilities.library import LibraryManager
    id1 = LibraryManager._make_id("hello", "2026-03-16T00:00:00Z")
    id2 = LibraryManager._make_id("world", "2026-03-16T00:00:00Z")
    assert id1 != id2
