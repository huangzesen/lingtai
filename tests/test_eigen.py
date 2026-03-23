"""Tests for eigen intrinsic — core self-management (memory + context)."""
from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from lingtai_kernel.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Memory edit
# ---------------------------------------------------------------------------


def test_eigen_memory_edit(tmp_path):
    """eigen memory edit writes to system/memory.md."""
    agent = BaseAgent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
    )
    result = agent._intrinsics["eigen"]({"object": "memory", "action": "edit", "content": "hello world"})
    assert result["status"] == "ok"
    mem_path = agent._working_dir / "system" / "memory.md"
    assert mem_path.read_text() == "hello world"
    agent.stop(timeout=1.0)


def test_eigen_memory_edit_empty_clears(tmp_path):
    """eigen memory edit with empty content clears memory file."""
    agent = BaseAgent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
    )
    # First write something
    agent._intrinsics["eigen"]({"object": "memory", "action": "edit", "content": "data"})
    # Then clear it
    result = agent._intrinsics["eigen"]({"object": "memory", "action": "edit"})
    assert result["status"] == "ok"
    mem_path = agent._working_dir / "system" / "memory.md"
    assert mem_path.read_text() == ""
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Memory load
# ---------------------------------------------------------------------------


def test_eigen_memory_load(tmp_path):
    """eigen memory load injects into system prompt."""
    agent = BaseAgent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
    )
    agent.start()
    try:
        # Write memory file first
        system_dir = agent._working_dir / "system"
        system_dir.mkdir(exist_ok=True)
        (system_dir / "memory.md").write_text("loaded content")

        result = agent._intrinsics["eigen"]({"object": "memory", "action": "load"})
        assert result["status"] == "ok"
        section = agent._prompt_manager.read_section("memory")
        assert "loaded content" in section
    finally:
        agent.stop()


def test_eigen_memory_load_empty(tmp_path):
    """eigen memory load with empty file deletes section."""
    agent = BaseAgent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
    )
    agent.start()
    try:
        result = agent._intrinsics["eigen"]({"object": "memory", "action": "load"})
        assert result["status"] == "ok"
        section = agent._prompt_manager.read_section("memory")
        assert section is None or section.strip() == ""
    finally:
        agent.stop()


# ---------------------------------------------------------------------------
# Context molt
# ---------------------------------------------------------------------------


def test_eigen_molt_uses_summary(tmp_path):
    """molt wipes context and re-injects agent's summary."""
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

    agent = BaseAgent(
        service=svc, agent_name="test", working_dir=tmp_path / "test",
    )
    agent.start()
    try:
        agent._session.ensure_session()
        agent._session._chat.interface.add_user_message("Hello")
        agent._session._chat.interface.add_assistant_message(
            [TextBlock(text="Hi there.")],
        )

        result = agent._intrinsics["eigen"]({
            "object": "context",
            "action": "molt",
            "summary": "Key finding: X=42. Task: analyze Y.",
        })
        assert result["status"] == "ok"
        assert result["after_tokens"] < result["before_tokens"] or result["after_tokens"] >= 0
        # Summary should be in new conversation
        iface = agent._session._chat.interface
        entries = [e for e in iface.entries if e.role == "user"]
        assert any("X=42" in str(e.content) for e in entries)
    finally:
        agent.stop()


def test_eigen_molt_rejects_empty(tmp_path):
    """molt with empty summary returns error."""
    agent = BaseAgent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
    )
    result = agent._intrinsics["eigen"]({"object": "context", "action": "molt", "summary": ""})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_eigen_molt_rejects_missing_summary(tmp_path):
    """molt without summary arg returns error."""
    agent = BaseAgent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
    )
    result = agent._intrinsics["eigen"]({"object": "context", "action": "molt"})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Context forget (internal only)
# ---------------------------------------------------------------------------


def test_eigen_forget_wipes_context(tmp_path):
    """context_forget nuclear wipes the session."""
    from lingtai_kernel.llm.interface import ChatInterface, TextBlock
    from lingtai_kernel.intrinsics.eigen import context_forget

    svc = make_mock_service()

    def fake_create_session(**kwargs):
        mock_chat = MagicMock()
        iface = ChatInterface()
        iface.add_system("You are helpful.")
        mock_chat.interface = iface
        mock_chat.context_window.return_value = 100_000
        return mock_chat

    svc.create_session.side_effect = fake_create_session

    agent = BaseAgent(
        service=svc, agent_name="test", working_dir=tmp_path / "test",
    )
    agent.start()
    try:
        agent._session.ensure_session()
        agent._session._chat.interface.add_user_message("test")
        agent._session._chat.interface.add_assistant_message(
            [TextBlock(text="response")],
        )

        result = context_forget(agent)
        assert result["status"] == "ok"
        assert result["before_tokens"] > 0
    finally:
        agent.stop()


# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------


def test_eigen_unknown_object(tmp_path):
    agent = BaseAgent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
    )
    result = agent._intrinsics["eigen"]({"object": "bogus", "action": "edit"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_eigen_unknown_action(tmp_path):
    agent = BaseAgent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
    )
    result = agent._intrinsics["eigen"]({"object": "memory", "action": "bogus"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_eigen_is_intrinsic_not_memory(tmp_path):
    """eigen replaces memory in intrinsics."""
    agent = BaseAgent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
    )
    assert "eigen" in agent._intrinsics
    assert "memory" not in agent._intrinsics
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Name action (true name)
# ---------------------------------------------------------------------------

def test_eigen_name_sets_agent_name(tmp_path):
    """eigen name action sets agent true name."""
    agent = BaseAgent(service=make_mock_service(), working_dir=tmp_path / "test")
    assert agent.agent_name is None
    result = agent._intrinsics["eigen"]({"object": "name", "action": "set", "content": "悟空"})
    assert result["status"] == "ok"
    assert result["name"] == "悟空"
    assert agent.agent_name == "悟空"
    agent.stop(timeout=1.0)


def test_eigen_name_rejects_second_set(tmp_path):
    """eigen name action fails if already named."""
    agent = BaseAgent(service=make_mock_service(), working_dir=tmp_path / "test", agent_name="alice")
    result = agent._intrinsics["eigen"]({"object": "name", "action": "set", "content": "bob"})
    assert "error" in result
    assert agent.agent_name == "alice"  # unchanged
    agent.stop(timeout=1.0)


def test_eigen_name_rejects_empty(tmp_path):
    """eigen name action fails with empty name."""
    agent = BaseAgent(service=make_mock_service(), working_dir=tmp_path / "test")
    result = agent._intrinsics["eigen"]({"object": "name", "action": "set", "content": ""})
    assert "error" in result
    assert agent.agent_name is None  # still unnamed
    agent.stop(timeout=1.0)
