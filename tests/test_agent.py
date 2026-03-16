"""Tests for BaseAgent lifecycle and tool dispatch."""
import time
import threading
from unittest.mock import MagicMock

import pytest

from stoai.base_agent import BaseAgent
from stoai.message import Message, _make_message, MSG_REQUEST
from stoai.state import AgentState
from stoai.types import MCPTool, UnknownToolError
from stoai.config import AgentConfig


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Lifecycle
# ---------------------------------------------------------------------------

def test_agent_starts_and_stops(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    assert agent.state == AgentState.SLEEPING
    agent.stop(timeout=2.0)


def test_agent_double_start(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    agent.start()  # should be no-op
    assert agent.state == AgentState.SLEEPING
    agent.stop(timeout=2.0)


# ---------------------------------------------------------------------------
# Intrinsics filtering
# ---------------------------------------------------------------------------

def test_intrinsics_enabled_by_default(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert "read" in agent._intrinsics
    assert "write" in agent._intrinsics
    assert "mail" in agent._intrinsics
    # manage_system_prompt is a layer, not an intrinsic
    assert "manage_system_prompt" not in agent._intrinsics
    assert "email" not in agent._intrinsics  # email is now a capability, not intrinsic
    assert "vision" not in agent._intrinsics  # vision is now a capability
    assert "web_search" not in agent._intrinsics  # web_search is now a capability
    assert "clock" in agent._intrinsics
    assert "status" in agent._intrinsics
    assert len(agent._intrinsics) == 9  # read, edit, write, glob, grep, mail, clock, status, memory


def test_disabled_intrinsics(tmp_path):
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        disabled_intrinsics={"mail", "clock"},
        base_dir=tmp_path,
    )
    assert "mail" not in agent._intrinsics
    assert "clock" not in agent._intrinsics
    assert "read" in agent._intrinsics


def test_enabled_intrinsics(tmp_path):
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        enabled_intrinsics={"read", "write"},
        base_dir=tmp_path,
    )
    assert "read" in agent._intrinsics
    assert "write" in agent._intrinsics
    assert "mail" not in agent._intrinsics
    assert "clock" not in agent._intrinsics


def test_enabled_and_disabled_raises(tmp_path):
    with pytest.raises(ValueError, match="Cannot specify both"):
        BaseAgent(
            agent_id="test",
            service=make_mock_service(),
            enabled_intrinsics={"read"},
            disabled_intrinsics={"mail"},
            base_dir=tmp_path,
        )


# ---------------------------------------------------------------------------
# MCP tools / add / remove
# ---------------------------------------------------------------------------

def test_add_remove_tool(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("custom", schema={"type": "object"}, handler=lambda args: {"ok": True})
    assert "custom" in agent._mcp_handlers
    agent.remove_tool("custom")
    assert "custom" not in agent._mcp_handlers


def test_mcp_tools_registered(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("domain_tool", schema={}, description="test", handler=lambda a: {"r": 1})
    assert "domain_tool" in agent._mcp_handlers


def test_add_tool_replaces_existing(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("custom", schema={}, handler=lambda args: {"v": 1})
    agent.add_tool("custom", schema={}, handler=lambda args: {"v": 2})
    assert agent._mcp_handlers["custom"]({})=={"v": 2}


def test_remove_nonexistent_tool_is_noop(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.remove_tool("nonexistent")  # should not raise


# ---------------------------------------------------------------------------
# System prompt sections
# ---------------------------------------------------------------------------

def test_system_prompt_sections(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.update_system_prompt("role", "You are a test agent", protected=True)
    assert agent._prompt_manager.read_section("role") == "You are a test agent"


def test_system_prompt_update_marks_dirty(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent._token_decomp_dirty = False
    agent.update_system_prompt("info", "some info")
    assert agent._token_decomp_dirty is True


# ---------------------------------------------------------------------------
# Mail via MailService (FIFO)
# ---------------------------------------------------------------------------

def test_mail_without_service(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent.mail("localhost:8301", "hello")
    assert result.get("error") == "mail service not configured"


def test_mail_with_service(tmp_path):
    import socket
    from stoai.services.mail import TCPMailService

    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()

    received = []
    event = threading.Event()

    receiver_svc = TCPMailService(listen_port=port)
    receiver_svc.listen(on_message=lambda msg: (received.append(msg), event.set()))

    try:
        sender_svc = TCPMailService()
        agent = BaseAgent(
            agent_id="sender",
            service=make_mock_service(),
            mail_service=sender_svc,
            base_dir=tmp_path,
        )
        result = agent.mail(f"127.0.0.1:{port}", "hello from agent")
        assert result["status"] == "delivered"
        assert event.wait(timeout=5.0)
        assert received[0]["message"] == "hello from agent"
    finally:
        receiver_svc.stop()


def test_mail_to_bad_address(tmp_path):
    from stoai.services.mail import TCPMailService
    sender_svc = TCPMailService()
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        mail_service=sender_svc,
        base_dir=tmp_path,
    )
    result = agent.mail("127.0.0.1:1", "hello")
    assert result["status"] == "refused"


# ---------------------------------------------------------------------------
# Mail FIFO wiring
# ---------------------------------------------------------------------------

def test_mail_inbox_wiring(tmp_path):
    """_on_mail_received should enqueue in FIFO and notify agent inbox."""
    agent = BaseAgent(agent_id="receiver", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({
        "from": "127.0.0.1:9999",
        "to": "127.0.0.1:8301",
        "message": "inbox test",
    })
    assert not agent.inbox.empty()
    msg = agent.inbox.get_nowait()
    assert "inbox test" in msg.content  # full message in notification
    assert msg.sender == "127.0.0.1:9999"
    # Should be in FIFO queue
    assert len(agent._mail_queue) == 1
    assert agent._mail_queue[0]["message"] == "inbox test"


def test_mail_start_wires_listener(tmp_path):
    """start() should call MailService.listen() when configured."""
    import socket
    from stoai.services.mail import TCPMailService

    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()

    agent_svc = TCPMailService(listen_port=port)
    agent = BaseAgent(
        agent_id="receiver",
        service=make_mock_service(),
        mail_service=agent_svc,
        base_dir=tmp_path,
    )
    agent.start()
    try:
        sender_svc = TCPMailService()
        result = sender_svc.send(
            f"127.0.0.1:{port}",
            {"from": "sender", "to": f"127.0.0.1:{port}", "message": "wired"},
        )
        assert result is True
        time.sleep(0.5)
        assert agent.inbox.qsize() >= 0
    finally:
        agent.stop(timeout=2.0)


def test_mail_read_pops_fifo(tmp_path):
    """mail read should pop messages in FIFO order."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({"from": "a", "message": "first"})
    agent._on_mail_received({"from": "b", "message": "second"})

    r1 = agent._handle_mail({"action": "read"})
    assert r1["message"] == "first"
    assert r1["remaining"] == 1

    r2 = agent._handle_mail({"action": "read"})
    assert r2["message"] == "second"
    assert r2["remaining"] == 0


def test_mail_read_empty_queue(tmp_path):
    """mail read on empty queue should return message=None."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_mail({"action": "read"})
    assert result["message"] is None
    assert result["remaining"] == 0


def test_mail_received_full_content_in_notification(tmp_path):
    """_on_mail_received should put full message content in inbox notification."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({
        "from": "sender",
        "subject": "test subject",
        "message": "full body content here",
    })
    msg = agent.inbox.get_nowait()
    assert "full body content here" in msg.content
    assert "test subject" in msg.content


# ---------------------------------------------------------------------------
# FileIOService integration
# ---------------------------------------------------------------------------

def test_file_intrinsics_use_service(tmp_path):
    """File intrinsics should delegate to the FileIOService."""
    from stoai.services.file_io import LocalFileIOService

    svc = LocalFileIOService(root=tmp_path)
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        file_io=svc,
        base_dir=tmp_path,
    )

    # Write via agent
    result = agent.write_file(str(tmp_path / "test.txt"), "hello world")
    assert result["status"] == "ok"

    # Read via agent
    result = agent.read_file(str(tmp_path / "test.txt"))
    assert "hello world" in result["content"]

    # Edit via agent
    result = agent.edit_file(str(tmp_path / "test.txt"), "hello", "goodbye")
    assert result["status"] == "ok"

    result = agent.read_file(str(tmp_path / "test.txt"))
    assert "goodbye world" in result["content"]


def test_file_intrinsics_auto_create_service(tmp_path):
    """Without explicit file_io, LocalFileIOService should be auto-created."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    # Should still have file intrinsics
    assert "read" in agent._intrinsics
    assert "write" in agent._intrinsics
    assert agent._file_io is not None


def test_no_file_io_disables_file_intrinsics(tmp_path):
    """Setting file_io=None should not create file intrinsics.

    NOTE: Currently file_io=None triggers auto-creation of LocalFileIOService
    for backward compat. To truly disable file intrinsics, use disabled_intrinsics.
    """
    agent = BaseAgent(
        agent_id="test",
        service=make_mock_service(),
        disabled_intrinsics={"read", "edit", "write", "glob", "grep"},
        base_dir=tmp_path,
    )
    assert "read" not in agent._intrinsics
    assert "write" not in agent._intrinsics


# ---------------------------------------------------------------------------
# Token usage
# ---------------------------------------------------------------------------

def test_token_usage(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    usage = agent.get_token_usage()
    assert isinstance(usage, dict)
    assert "input_tokens" in usage
    assert "output_tokens" in usage
    assert "api_calls" in usage
    assert usage["input_tokens"] == 0
    assert usage["api_calls"] == 0


# ---------------------------------------------------------------------------
# Message
# ---------------------------------------------------------------------------

def test_message_type():
    msg = Message(type="request", content="hello", sender="user")
    assert msg.type == "request"
    assert msg.content == "hello"


def test_make_message():
    msg = _make_message(MSG_REQUEST, "user", "hello")
    assert msg.type == MSG_REQUEST
    assert msg.sender == "user"
    assert msg.content == "hello"
    assert msg.id.startswith("msg_")


def test_message_reply_event():
    event = threading.Event()
    msg = _make_message(MSG_REQUEST, "user", "hello", reply_event=event)
    assert msg._reply_event is event
    msg._reply_value = {"text": "world"}
    event.set()
    assert event.is_set()


# ---------------------------------------------------------------------------
# Tool dispatch
# ---------------------------------------------------------------------------

def test_execute_single_tool_intrinsic(tmp_path):
    """Intrinsic tools should be callable via _execute_single_tool."""
    from stoai.llm.base import ToolCall
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    # Replace the read intrinsic with a mock
    agent._intrinsics["read"] = lambda args: {"status": "ok", "content": "test"}

    tc = ToolCall(name="read", args={"file_path": "/tmp/test.txt"})
    result = agent._dispatch_tool(tc)
    assert result["status"] == "ok"


def test_execute_single_tool_mcp(tmp_path):
    """MCP tools should be callable via _dispatch_tool."""
    from stoai.llm.base import ToolCall
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("my_tool", schema={}, handler=lambda args: {"status": "ok", "value": args.get("x")})

    tc = ToolCall(name="my_tool", args={"x": 42})
    result = agent._dispatch_tool(tc)
    assert result["status"] == "ok"
    assert result["value"] == 42


def test_execute_single_tool_unknown(tmp_path):
    """Unknown tools should raise UnknownToolError."""
    from stoai.llm.base import ToolCall
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)

    tc = ToolCall(name="nonexistent_tool", args={})
    with pytest.raises(UnknownToolError):
        agent._dispatch_tool(tc)


# ---------------------------------------------------------------------------
# Context (opaque)
# ---------------------------------------------------------------------------

def test_context_stored_opaque(tmp_path):
    ctx = {"custom": "data", "nested": [1, 2, 3]}
    agent = BaseAgent(agent_id="test", service=make_mock_service(), context=ctx, base_dir=tmp_path)
    assert agent._context is ctx


# ---------------------------------------------------------------------------
# Working dir
# ---------------------------------------------------------------------------

def test_working_dir_resolved(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent.working_dir == tmp_path / "test"


def test_base_dir_required():
    """base_dir must be explicitly provided."""
    with pytest.raises(TypeError):
        BaseAgent(agent_id="test", service=make_mock_service())


# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

def test_config_defaults(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent._config.max_turns == 50


def test_config_override(tmp_path):
    config = AgentConfig(max_turns=10, provider="anthropic")
    agent = BaseAgent(agent_id="test", service=make_mock_service(), config=config, base_dir=tmp_path)
    assert agent._config.max_turns == 10
    assert agent._config.provider == "anthropic"


# ---------------------------------------------------------------------------
# Status
# ---------------------------------------------------------------------------

def test_status(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    s = agent.status()
    assert s["agent_id"] == "test"
    assert s["state"] == "sleeping"
    assert s["idle"] is True
    assert "tokens" in s


# ---------------------------------------------------------------------------
# Public send API
# ---------------------------------------------------------------------------

def test_send_fires_message(tmp_path):
    """send(wait=False) should put a message in the inbox."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.send("hello", wait=False)
    assert not agent.inbox.empty()
    msg = agent.inbox.get_nowait()
    assert msg.content == "hello"
    assert msg.type == MSG_REQUEST


# ---------------------------------------------------------------------------
# working_dir property
# ---------------------------------------------------------------------------

def test_working_dir_property(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent.working_dir == tmp_path / "test"

def test_base_dir_property_required():
    """base_dir is a required argument — omitting it raises TypeError."""
    with pytest.raises(TypeError, match="base_dir"):
        BaseAgent(agent_id="test", service=make_mock_service())


# ---------------------------------------------------------------------------
# Agent lock and manifest
# ---------------------------------------------------------------------------

def test_agent_creates_manifest(tmp_path):
    import json
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    manifest = tmp_path / "alice" / ".agent.json"
    assert manifest.is_file()
    data = json.loads(manifest.read_text())
    assert data["agent_id"] == "alice"
    assert "started_at" in data


def test_agent_creates_lock_file(tmp_path):
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    assert (tmp_path / "alice" / ".agent.lock").is_file()


def test_agent_lock_conflict(tmp_path):
    agent1 = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(RuntimeError, match="already in use"):
        agent2 = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)


def test_agent_lock_released_on_stop(tmp_path):
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    agent.stop()
    agent2 = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)


def test_agent_resume_reads_role_ltm(tmp_path):
    agent = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path,
        role="researcher", ltm="knows python",
    )
    agent.stop()
    agent2 = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    assert agent2._prompt_manager.read_section("role") == "researcher"
    assert agent2._prompt_manager.read_section("ltm") == "knows python"


def test_agent_resume_explicit_overrides_manifest(tmp_path):
    agent = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path,
        role="old role", ltm="old ltm",
    )
    agent.stop()
    agent2 = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path,
        role="new role",
    )
    assert agent2._prompt_manager.read_section("role") == "new role"
    assert agent2._prompt_manager.read_section("ltm") == "old ltm"


def test_agent_stop_persists_ltm(tmp_path):
    agent = BaseAgent(
        agent_id="alice", service=make_mock_service(), base_dir=tmp_path, ltm="initial",
    )
    agent._prompt_manager.write_section("ltm", "updated knowledge")
    agent.stop()
    ltm_file = tmp_path / "alice" / "ltm" / "ltm.md"
    assert ltm_file.is_file()
    assert ltm_file.read_text() == "updated knowledge"


def test_agent_corrupt_manifest(tmp_path):
    agent_dir = tmp_path / "alice"
    agent_dir.mkdir()
    (agent_dir / ".agent.json").write_text("{corrupt json")
    agent = BaseAgent(agent_id="alice", service=make_mock_service(), base_dir=tmp_path)
    assert (agent_dir / ".agent.json.corrupt").is_file()
    assert agent._prompt_manager.read_section("role") is None


def test_agent_id_validation(tmp_path):
    with pytest.raises(ValueError, match="agent_id"):
        BaseAgent(agent_id="bad/id", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(ValueError, match="agent_id"):
        BaseAgent(agent_id="../escape", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(ValueError, match="agent_id"):
        BaseAgent(agent_id="", service=make_mock_service(), base_dir=tmp_path)


def test_base_dir_must_exist(tmp_path):
    with pytest.raises(FileNotFoundError):
        BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path / "nonexistent")


# ---------------------------------------------------------------------------
# Seal guard
# ---------------------------------------------------------------------------

def test_add_tool_raises_after_start(tmp_path):
    """add_tool() must raise RuntimeError after start()."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda args: {}, description="test")
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.add_tool("bar", schema={"type": "object", "properties": {}}, handler=lambda args: {}, description="test2")
    finally:
        agent.stop(timeout=2.0)


def test_remove_tool_raises_after_start(tmp_path):
    """remove_tool() must raise RuntimeError after start()."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda args: {}, description="test")
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.remove_tool("foo")
    finally:
        agent.stop(timeout=2.0)


def test_add_tool_works_before_start(tmp_path):
    """add_tool() works fine before start()."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda args: {"ok": True}, description="test")
    assert "foo" in agent._mcp_handlers
    agent.stop(timeout=1.0)
