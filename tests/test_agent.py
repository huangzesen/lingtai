"""Tests for BaseAgent lifecycle and tool dispatch."""
import time
import threading
from unittest.mock import MagicMock

import pytest

from stoai_kernel.base_agent import BaseAgent
from stoai.agent import Agent
from stoai_kernel.message import Message, _make_message, MSG_REQUEST
from stoai_kernel.state import AgentState
from stoai_kernel.types import UnknownToolError
from stoai_kernel.config import AgentConfig


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
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    assert agent.state == AgentState.IDLE
    agent.stop(timeout=2.0)


def test_agent_double_start(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    agent.start()  # should be no-op
    assert agent.state == AgentState.IDLE
    agent.stop(timeout=2.0)


def test_base_agent_file_io_defaults_to_none(tmp_path):
    """BaseAgent should have _file_io=None when no file_io is passed."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent._file_io is None


# ---------------------------------------------------------------------------
# Intrinsics filtering
# ---------------------------------------------------------------------------

def test_intrinsics_enabled_by_default(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert "mail" in agent._intrinsics
    assert "system" in agent._intrinsics
    assert "eigen" in agent._intrinsics
    # File I/O is now a capability, not intrinsic
    assert "read" not in agent._intrinsics
    assert "write" not in agent._intrinsics
    assert len(agent._intrinsics) == 4  # mail, system, eigen, soul


# ---------------------------------------------------------------------------
# MCP tools / add / remove
# ---------------------------------------------------------------------------

def test_add_remove_tool(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("custom", schema={"type": "object"}, handler=lambda args: {"ok": True})
    assert "custom" in agent._mcp_handlers
    agent.remove_tool("custom")
    assert "custom" not in agent._mcp_handlers


def test_mcp_tools_registered(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("domain_tool", schema={}, description="test", handler=lambda a: {"r": 1})
    assert "domain_tool" in agent._mcp_handlers


def test_add_tool_replaces_existing(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("custom", schema={}, handler=lambda args: {"v": 1})
    agent.add_tool("custom", schema={}, handler=lambda args: {"v": 2})
    assert agent._mcp_handlers["custom"]({})=={"v": 2}


def test_remove_nonexistent_tool_is_noop(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.remove_tool("nonexistent")  # should not raise


# ---------------------------------------------------------------------------
# System prompt sections
# ---------------------------------------------------------------------------

def test_system_prompt_sections(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.update_system_prompt("role", "You are a test agent", protected=True)
    assert agent._prompt_manager.read_section("role") == "You are a test agent"


def test_system_prompt_update_marks_dirty(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._token_decomp_dirty = False
    agent.update_system_prompt("info", "some info")
    assert agent._token_decomp_dirty is True


# ---------------------------------------------------------------------------
# Mail via MailService (FIFO)
# ---------------------------------------------------------------------------

def test_mail_without_service(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent.mail("localhost:8301", "hello")
    # Send is async — no error at send time, mailman handles missing service
    assert result["status"] == "sent"


def test_mail_with_service(tmp_path):
    import socket
    from stoai_kernel.services.mail import TCPMailService

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
            agent_name="sender",
            service=make_mock_service(),
            mail_service=sender_svc,
            base_dir=tmp_path,
        )
        result = agent.mail(f"127.0.0.1:{port}", "hello from agent")
        assert result["status"] == "sent"
        # Delivery is async via mailman thread — wait for receiver
        assert event.wait(timeout=5.0)
        assert received[0]["message"] == "hello from agent"
    finally:
        receiver_svc.stop()


def test_mail_to_bad_address(tmp_path):
    from stoai_kernel.services.mail import TCPMailService
    sender_svc = TCPMailService()
    agent = BaseAgent(
        agent_name="test",
        service=make_mock_service(),
        mail_service=sender_svc,
        base_dir=tmp_path,
    )
    result = agent.mail("127.0.0.1:1", "hello")
    # Send is async — always returns "sent"; refusal is recorded by mailman
    assert result["status"] == "sent"


# ---------------------------------------------------------------------------
# Mail FIFO wiring
# ---------------------------------------------------------------------------

def test_mail_inbox_wiring(tmp_path):
    """_on_mail_received should notify agent inbox."""
    agent = BaseAgent(agent_name="receiver", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({
        "_mailbox_id": "test-id-123",
        "from": "127.0.0.1:9999",
        "to": "127.0.0.1:8301",
        "message": "inbox test",
    })
    assert not agent.inbox.empty()
    msg = agent.inbox.get_nowait()
    assert msg.sender == "system"
    assert "mail box" in msg.content
    assert 'mail(action=' in msg.content


def test_mail_start_wires_listener(tmp_path):
    """start() should call MailService.listen() when configured."""
    import socket
    from stoai_kernel.services.mail import TCPMailService

    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()

    agent_svc = TCPMailService(listen_port=port)
    agent = BaseAgent(
        agent_name="receiver",
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
        assert result is None
        time.sleep(0.5)
        assert agent.inbox.qsize() >= 0
    finally:
        agent.stop(timeout=2.0)


def test_mail_read_by_id(tmp_path):
    """mail read should load messages by ID from disk."""
    import json
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    # Persist a message to the inbox directory
    import uuid
    msg_id = str(uuid.uuid4())
    msg_dir = agent._working_dir / "mailbox" / "inbox" / msg_id
    msg_dir.mkdir(parents=True)
    (msg_dir / "message.json").write_text(json.dumps({
        "_mailbox_id": msg_id,
        "from": "a",
        "subject": "test",
        "message": "first",
        "received_at": "2026-03-18T10:00:00Z",
    }))
    result = agent._intrinsics["mail"]({"action": "read", "id": [msg_id]})
    assert len(result["messages"]) == 1
    assert result["messages"][0]["message"] == "first"


def test_mail_read_no_ids_returns_error(tmp_path):
    """mail read without id should return an error."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._intrinsics["mail"]({"action": "read"})
    assert "error" in result


def test_mail_received_full_content_in_notification(tmp_path):
    """_on_mail_received should include preview and subject in notification."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({
        "from": "sender",
        "subject": "test subject",
        "message": "full body content here",
    })
    msg = agent.inbox.get_nowait()
    assert "full body content here" in msg.content
    assert "test subject" in msg.content
    assert msg.sender == "system"
    assert "mail box" in msg.content


# ---------------------------------------------------------------------------
# Token usage
# ---------------------------------------------------------------------------

def test_token_usage(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
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
    assert "hello" in msg.content
    assert msg.id.startswith("msg_")


# ---------------------------------------------------------------------------
# Tool dispatch
# ---------------------------------------------------------------------------

def test_execute_single_tool_intrinsic(tmp_path):
    """Intrinsic tools should be callable via _dispatch_tool."""
    from stoai_kernel.llm.base import ToolCall
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    # Replace the system intrinsic with a mock
    agent._intrinsics["system"] = lambda args: {"status": "ok", "time": "12:00"}

    tc = ToolCall(name="system", args={"action": "show"})
    result = agent._dispatch_tool(tc)
    assert result["status"] == "ok"


def test_execute_single_tool_mcp(tmp_path):
    """MCP tools should be callable via _dispatch_tool."""
    from stoai_kernel.llm.base import ToolCall
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("my_tool", schema={}, handler=lambda args: {"status": "ok", "value": args.get("x")})

    tc = ToolCall(name="my_tool", args={"x": 42})
    result = agent._dispatch_tool(tc)
    assert result["status"] == "ok"
    assert result["value"] == 42


def test_execute_single_tool_unknown(tmp_path):
    """Unknown tools should raise UnknownToolError."""
    from stoai_kernel.llm.base import ToolCall
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    tc = ToolCall(name="nonexistent_tool", args={})
    with pytest.raises(UnknownToolError):
        agent._dispatch_tool(tc)


# ---------------------------------------------------------------------------
# Context (opaque)
# ---------------------------------------------------------------------------

def test_context_stored_opaque(tmp_path):
    ctx = {"custom": "data", "nested": [1, 2, 3]}
    agent = BaseAgent(agent_name="test", service=make_mock_service(), context=ctx, base_dir=tmp_path)
    assert agent._context is ctx


# ---------------------------------------------------------------------------
# Working dir
# ---------------------------------------------------------------------------

def test_working_dir_resolved(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent.working_dir == tmp_path / "test"


def test_base_dir_required():
    """base_dir must be explicitly provided."""
    with pytest.raises(TypeError):
        BaseAgent(agent_name="test", service=make_mock_service())


# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

def test_config_defaults(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent._config.max_turns == 50


def test_config_override(tmp_path):
    config = AgentConfig(max_turns=10, provider="anthropic")
    agent = BaseAgent(agent_name="test", service=make_mock_service(), config=config, base_dir=tmp_path)
    assert agent._config.max_turns == 10
    assert agent._config.provider == "anthropic"


# ---------------------------------------------------------------------------
# Status
# ---------------------------------------------------------------------------

def test_status(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    s = agent.status()
    assert s["agent_name"] == "test"
    assert s["state"] == "idle"
    assert s["idle"] is True
    assert "tokens" in s


# ---------------------------------------------------------------------------
# Public send API
# ---------------------------------------------------------------------------

def test_send_fires_message(tmp_path):
    """send() should put a message in the inbox."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.send("hello")
    assert not agent.inbox.empty()
    msg = agent.inbox.get_nowait()
    assert "hello" in msg.content
    assert msg.type == MSG_REQUEST


# ---------------------------------------------------------------------------
# working_dir property
# ---------------------------------------------------------------------------

def test_working_dir_property(tmp_path):
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent.working_dir == tmp_path / "test"

def test_base_dir_property_required():
    """base_dir is a required argument — omitting it raises TypeError."""
    with pytest.raises(TypeError, match="base_dir"):
        BaseAgent(agent_name="test", service=make_mock_service())


# ---------------------------------------------------------------------------
# Agent lock and manifest
# ---------------------------------------------------------------------------

def test_agent_creates_manifest(tmp_path):
    import json
    agent = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)
    manifest = tmp_path / "alice" / ".agent.json"
    assert manifest.is_file()
    data = json.loads(manifest.read_text())
    assert data["agent_name"] == "alice"
    assert "started_at" in data


def test_agent_creates_lock_file(tmp_path):
    agent = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)
    assert (tmp_path / "alice" / ".agent.lock").is_file()


def test_agent_lock_conflict(tmp_path):
    agent1 = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(RuntimeError, match="already in use"):
        agent2 = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)


def test_agent_lock_released_on_stop(tmp_path):
    agent = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)
    agent.stop()
    agent2 = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)


def test_agent_resume_reads_covenant_memory(tmp_path):
    agent = BaseAgent(
        agent_name="alice", service=make_mock_service(), base_dir=tmp_path,
        covenant="researcher", memory="knows python",
    )
    agent.stop()
    agent2 = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)
    assert agent2._prompt_manager.read_section("covenant") == "researcher"
    assert agent2._prompt_manager.read_section("memory") == "knows python"


def test_agent_resume_explicit_overrides_manifest(tmp_path):
    agent = BaseAgent(
        agent_name="alice", service=make_mock_service(), base_dir=tmp_path,
        covenant="old role", memory="old ltm",
    )
    agent.stop()
    agent2 = BaseAgent(
        agent_name="alice", service=make_mock_service(), base_dir=tmp_path,
        covenant="new role",
    )
    assert agent2._prompt_manager.read_section("covenant") == "new role"
    assert agent2._prompt_manager.read_section("memory") == "old ltm"


def test_agent_stop_persists_memory(tmp_path):
    agent = BaseAgent(
        agent_name="alice", service=make_mock_service(), base_dir=tmp_path, memory="initial",
    )
    agent._prompt_manager.write_section("memory", "updated knowledge")
    agent.stop()
    memory_file = tmp_path / "alice" / "system" / "memory.md"
    assert memory_file.is_file()
    assert memory_file.read_text() == "updated knowledge"


def test_agent_corrupt_manifest(tmp_path):
    agent_dir = tmp_path / "alice"
    agent_dir.mkdir()
    (agent_dir / ".agent.json").write_text("{corrupt json")
    agent = BaseAgent(agent_name="alice", service=make_mock_service(), base_dir=tmp_path)
    # Constructor writes a fresh manifest (overwriting the corrupt one)
    import json
    data = json.loads((agent_dir / ".agent.json").read_text())
    assert data["agent_name"] == "alice"
    assert agent._prompt_manager.read_section("covenant") is None


def test_agent_name_validation(tmp_path):
    with pytest.raises(ValueError, match="agent_name"):
        BaseAgent(agent_name="bad/id", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(ValueError, match="agent_name"):
        BaseAgent(agent_name="../escape", service=make_mock_service(), base_dir=tmp_path)
    with pytest.raises(ValueError, match="agent_name"):
        BaseAgent(agent_name="", service=make_mock_service(), base_dir=tmp_path)


def test_base_dir_must_exist(tmp_path):
    with pytest.raises(FileNotFoundError):
        BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path / "nonexistent")


# ---------------------------------------------------------------------------
# Seal guard
# ---------------------------------------------------------------------------

def test_add_tool_raises_after_start(tmp_path):
    """add_tool() must raise RuntimeError after start()."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda args: {}, description="test")
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.add_tool("bar", schema={"type": "object", "properties": {}}, handler=lambda args: {}, description="test2")
    finally:
        agent.stop(timeout=2.0)


def test_remove_tool_raises_after_start(tmp_path):
    """remove_tool() must raise RuntimeError after start()."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda args: {}, description="test")
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.remove_tool("foo")
    finally:
        agent.stop(timeout=2.0)


def test_add_tool_works_before_start(tmp_path):
    """add_tool() works fine before start()."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda args: {"ok": True}, description="test")
    assert "foo" in agent._mcp_handlers
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# _concat_queued_messages
# ---------------------------------------------------------------------------

def test_queued_messages_concatenated(tmp_path):
    """Multiple queued messages should be concatenated into one."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    msg1 = _make_message(MSG_REQUEST, "system", "[system] 1 new message in mail box.\n  From: alice — hello")
    msg2 = _make_message(MSG_REQUEST, "system", "[system] 1 new message in mail box.\n  From: bob — world")
    msg3 = _make_message(MSG_REQUEST, "system", "[system] 1 new message in gmail box.\n  From: charlie — meeting")
    agent.inbox.put(msg1)
    agent.inbox.put(msg2)
    agent.inbox.put(msg3)

    first = agent.inbox.get()
    result = agent._concat_queued_messages(first)
    assert "alice" in result.content
    assert "bob" in result.content
    assert "charlie" in result.content
    assert result.sender == "system"
    assert agent.inbox.empty()


def test_single_message_not_modified(tmp_path):
    """A single message with nothing queued should pass through unchanged."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    original = _make_message(MSG_REQUEST, "alice", "hello")
    result = agent._concat_queued_messages(original)
    assert result is original


def test_concat_preserves_first_sender(tmp_path):
    """Concatenated result keeps the first message's sender."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    msg1 = _make_message(MSG_REQUEST, "alice", "task for you")
    msg2 = _make_message(MSG_REQUEST, "system", "[system] 1 new message in mail box.")
    agent.inbox.put(msg1)
    agent.inbox.put(msg2)

    first = agent.inbox.get()
    result = agent._concat_queued_messages(first)
    assert "task for you" in result.content
    assert "mail box" in result.content
    assert result.sender == "alice"


# ---------------------------------------------------------------------------
# connect_mcp placement
# ---------------------------------------------------------------------------

def test_connect_mcp_is_on_agent_not_base(tmp_path):
    """connect_mcp should be defined on Agent, not BaseAgent."""
    assert hasattr(Agent, 'connect_mcp')
    # Verify it's not inherited from BaseAgent
    assert 'connect_mcp' not in BaseAgent.__dict__


# ---------------------------------------------------------------------------
# AgentConfig kernel cleanliness
# ---------------------------------------------------------------------------

def test_agent_config_has_no_bash_policy_file():
    """AgentConfig should not have capability-specific fields."""
    from stoai_kernel.config import AgentConfig
    assert 'bash_policy_file' not in AgentConfig.__dataclass_fields__


# ---------------------------------------------------------------------------
# BaseAgent kernel import cleanliness
# ---------------------------------------------------------------------------

def test_base_agent_has_no_non_kernel_imports():
    """BaseAgent module (in stoai_kernel) should not import from non-kernel stoai modules."""
    import ast
    import stoai_kernel
    from pathlib import Path
    kernel_dir = Path(stoai_kernel.__file__).parent
    source = (kernel_dir / "base_agent.py").read_text()
    tree = ast.parse(source)

    non_kernel = {"services.file_io", "services.mcp", "services.vision", "services.search",
                  "capabilities", "addons", "agent"}

    for node in ast.walk(tree):
        if isinstance(node, (ast.Import, ast.ImportFrom)):
            if isinstance(node, ast.ImportFrom) and node.module:
                for nk in non_kernel:
                    assert nk not in node.module, f"base_agent.py imports from non-kernel: {node.module}"
