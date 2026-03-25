# tests/test_daemon.py
"""Tests for the daemon (神識) capability — subagent system."""
import queue
import threading
import time
from unittest.mock import MagicMock

from lingtai_kernel.config import AgentConfig
from lingtai_kernel.llm.base import ToolCall


def _make_agent(tmp_path, capabilities=None):
    """Create a minimal Agent with mock LLM service."""
    from lingtai.agent import Agent
    svc = MagicMock()
    svc.provider = "mock"
    svc.model = "mock-model"
    svc.create_session = MagicMock()
    svc.make_tool_result = MagicMock()
    agent = Agent(
        svc,
        working_dir=tmp_path / "daemon-agent",
        capabilities=capabilities or ["daemon"],
        config=AgentConfig(),
    )
    return agent


def test_daemon_registers_tool(tmp_path):
    agent = _make_agent(tmp_path, ["daemon"])
    tool_names = {s.name for s in agent._tool_schemas}
    assert "daemon" in tool_names


def test_build_tool_surface_expands_groups(tmp_path):
    """'file' group expands to read/write/edit/glob/grep."""
    agent = _make_agent(tmp_path, ["file", "daemon"])
    mgr = agent.get_capability("daemon")
    schemas, dispatch = mgr._build_tool_surface(["file"])
    names = {s.name for s in schemas}
    assert "read" in names
    assert "write" in names
    assert "edit" in names
    assert "glob" in names
    assert "grep" in names


def test_build_tool_surface_blacklist(tmp_path):
    """Blacklisted tools are silently excluded."""
    agent = _make_agent(tmp_path, ["file", "daemon", "avatar"])
    mgr = agent.get_capability("daemon")
    schemas, dispatch = mgr._build_tool_surface(["file", "avatar", "daemon"])
    names = {s.name for s in schemas}
    assert "daemon" not in names
    assert "avatar" not in names
    assert "read" in names


def test_build_tool_surface_unknown_tool(tmp_path):
    """Unknown tool name raises ValueError."""
    agent = _make_agent(tmp_path, ["daemon"])
    mgr = agent.get_capability("daemon")
    try:
        mgr._build_tool_surface(["nonexistent"])
        assert False, "Should have raised ValueError"
    except ValueError as e:
        assert "nonexistent" in str(e)


def test_build_tool_surface_inherits_mcp_tools(tmp_path):
    """MCP tools are automatically inherited without being requested."""
    agent = _make_agent(tmp_path, ["daemon"])
    # Simulate an MCP tool registered via connect_mcp
    agent._sealed = False
    agent.add_tool("my_mcp_tool", schema={"type": "object", "properties": {}},
                   handler=lambda args: {}, description="MCP tool")
    agent._sealed = True
    mgr = agent.get_capability("daemon")
    schemas, dispatch = mgr._build_tool_surface([])  # no explicit tools
    names = {s.name for s in schemas}
    assert "my_mcp_tool" in names


def test_build_emanation_prompt_includes_task(tmp_path):
    """System prompt includes the task description."""
    agent = _make_agent(tmp_path, ["file", "daemon"])
    mgr = agent.get_capability("daemon")
    schemas, _ = mgr._build_tool_surface(["file"])
    prompt = mgr._build_emanation_prompt("Find all TODOs", schemas)
    assert "Find all TODOs" in prompt
    assert "daemon emanation" in prompt.lower() or "分神" in prompt


def test_run_emanation_returns_text(tmp_path):
    """Emanation runs a tool loop and returns final text."""
    agent = _make_agent(tmp_path, ["file", "daemon"])
    mgr = agent.get_capability("daemon")

    mock_session = MagicMock()
    mock_response = MagicMock()
    mock_response.text = "Task done. Found 3 files."
    mock_response.tool_calls = []
    mock_session.send = MagicMock(return_value=mock_response)
    agent.service.create_session = MagicMock(return_value=mock_session)

    cancel = threading.Event()
    em_id = "em-test"
    mgr._emanations[em_id] = {
        "followup_buffer": "",
        "followup_lock": threading.Lock(),
    }
    result = mgr._run_emanation(em_id, "find stuff", ["file"], None, cancel)
    assert "Found 3 files" in result


def test_run_emanation_dispatches_tools(tmp_path):
    """Emanation dispatches tool calls and feeds results back."""
    agent = _make_agent(tmp_path, ["file", "daemon"])
    agent.inbox = queue.Queue()
    mgr = agent.get_capability("daemon")

    mock_handler = MagicMock(return_value={"content": "file text"})
    agent._tool_handlers["read"] = mock_handler

    tc = ToolCall(name="read", args={"file_path": "/tmp/x"}, id="tc-1")
    resp1 = MagicMock()
    resp1.text = ""
    resp1.tool_calls = [tc]
    resp2 = MagicMock()
    resp2.text = "Task done. Read the file."
    resp2.tool_calls = []

    mock_session = MagicMock()
    mock_session.send = MagicMock(side_effect=[resp1, resp2])
    agent.service.create_session = MagicMock(return_value=mock_session)
    agent.service.make_tool_result = MagicMock(return_value="mock_result")

    cancel = threading.Event()
    em_id = "em-test"
    mgr._emanations[em_id] = {
        "followup_buffer": "",
        "followup_lock": threading.Lock(),
    }
    result = mgr._run_emanation(em_id, "read a file", ["file"], None, cancel)
    assert "Read the file" in result
    assert mock_handler.called


def test_run_emanation_respects_cancel_before_first_send(tmp_path):
    """Emanation exits immediately if pre-cancelled (before first LLM call)."""
    agent = _make_agent(tmp_path, ["file", "daemon"])
    mgr = agent.get_capability("daemon")

    mock_session = MagicMock()
    agent.service.create_session = MagicMock(return_value=mock_session)

    cancel = threading.Event()
    cancel.set()
    em_id = "em-test"
    mgr._emanations[em_id] = {
        "followup_buffer": "",
        "followup_lock": threading.Lock(),
    }
    result = mgr._run_emanation(em_id, "do stuff", ["file"], None, cancel)
    assert result == "[cancelled]"
    mock_session.send.assert_not_called()


def test_handle_emanate_dispatches_and_returns_ids(tmp_path):
    """emanate dispatches tasks and returns sequential IDs."""
    agent = _make_agent(tmp_path, ["file", "daemon"])
    agent.inbox = queue.Queue()
    mgr = agent.get_capability("daemon")

    mock_session = MagicMock()
    mock_resp = MagicMock()
    mock_resp.text = "done"
    mock_resp.tool_calls = []
    mock_session.send = MagicMock(return_value=mock_resp)
    agent.service.create_session = MagicMock(return_value=mock_session)

    result = mgr.handle({"action": "emanate", "tasks": [
        {"task": "task A", "tools": ["file"]},
        {"task": "task B", "tools": ["file"]},
    ]})
    assert result["status"] == "dispatched"
    assert result["count"] == 2
    assert result["ids"] == ["em-1", "em-2"]

    time.sleep(1)

    messages = []
    while not agent.inbox.empty():
        messages.append(agent.inbox.get_nowait())
    assert len(messages) == 2


def test_handle_emanate_rejects_over_limit(tmp_path):
    """emanate rejects when max_emanations would be exceeded."""
    agent = _make_agent(tmp_path, {"daemon": {"max_emanations": 1}})
    mgr = agent.get_capability("daemon")

    mgr._emanations["em-0"] = {"future": MagicMock(done=MagicMock(return_value=False))}
    result = mgr.handle({"action": "emanate", "tasks": [
        {"task": "x", "tools": ["file"]},
    ]})
    assert result["status"] == "error"
    assert "Too many" in result["message"] or "running" in result["message"]


def test_handle_list_shows_status(tmp_path):
    """list returns emanation statuses."""
    agent = _make_agent(tmp_path, ["daemon"])
    mgr = agent.get_capability("daemon")

    done_future = MagicMock()
    done_future.done.return_value = True
    done_future.exception.return_value = None
    running_future = MagicMock()
    running_future.done.return_value = False

    mgr._emanations = {
        "em-1": {"future": done_future, "task": "task A", "start_time": time.time() - 10, "cancel_event": threading.Event()},
        "em-2": {"future": running_future, "task": "task B", "start_time": time.time() - 5, "cancel_event": threading.Event()},
    }
    result = mgr._handle_list()
    assert len(result["emanations"]) == 2
    statuses = {e["id"]: e["status"] for e in result["emanations"]}
    assert statuses["em-1"] == "done"
    assert statuses["em-2"] == "running"


def test_handle_ask_sends_followup(tmp_path):
    """ask buffers a follow-up for a running emanation."""
    agent = _make_agent(tmp_path, ["daemon"])
    mgr = agent.get_capability("daemon")
    mgr._emanations["em-1"] = {
        "future": MagicMock(done=MagicMock(return_value=False)),
        "task": "x",
        "followup_buffer": "",
        "followup_lock": threading.Lock(),
    }
    result = mgr._handle_ask("em-1", "also check tests/")
    assert result["status"] == "sent"
    assert mgr._emanations["em-1"]["followup_buffer"] == "also check tests/"


def test_handle_ask_collapses_multiple(tmp_path):
    """Multiple asks collapse into one buffer."""
    agent = _make_agent(tmp_path, ["daemon"])
    mgr = agent.get_capability("daemon")
    mgr._emanations["em-1"] = {
        "future": MagicMock(done=MagicMock(return_value=False)),
        "task": "x",
        "followup_buffer": "",
        "followup_lock": threading.Lock(),
    }
    mgr._handle_ask("em-1", "first")
    mgr._handle_ask("em-1", "second")
    assert mgr._emanations["em-1"]["followup_buffer"] == "first\n\nsecond"


def test_handle_reclaim_cancels_all(tmp_path):
    """reclaim sets cancel events and clears registry."""
    agent = _make_agent(tmp_path, ["daemon"])
    mgr = agent.get_capability("daemon")
    cancel = threading.Event()
    pool = MagicMock()
    mgr._pools = [(pool, cancel)]
    mgr._emanations = {
        "em-1": {"future": MagicMock(done=MagicMock(return_value=False)), "cancel_event": cancel},
    }
    result = mgr._handle_reclaim()
    assert result["status"] == "reclaimed"
    assert result["cancelled"] == 1
    assert cancel.is_set()
    assert len(mgr._emanations) == 0
    pool.shutdown.assert_called_once()


def test_run_emanation_respects_cancel_mid_loop(tmp_path):
    """Emanation exits on cancel event between tool-call rounds."""
    agent = _make_agent(tmp_path, ["file", "daemon"])
    mgr = agent.get_capability("daemon")

    tc = ToolCall(name="read", args={}, id="tc-1")
    resp = MagicMock()
    resp.text = ""
    resp.tool_calls = [tc]

    mock_session = MagicMock()
    agent.service.create_session = MagicMock(return_value=mock_session)
    agent.service.make_tool_result = MagicMock(return_value="mock_result")
    agent._tool_handlers["read"] = MagicMock(return_value={})

    cancel = threading.Event()
    call_count = [0]
    def send_and_cancel(*args, **kwargs):
        call_count[0] += 1
        if call_count[0] >= 2:
            cancel.set()
        return resp
    mock_session.send = send_and_cancel

    em_id = "em-test"
    mgr._emanations[em_id] = {
        "followup_buffer": "",
        "followup_lock": threading.Lock(),
    }
    result = mgr._run_emanation(em_id, "do stuff", ["file"], None, cancel)
    assert result == "[cancelled]"


def test_end_to_end_emanate_list_ask_reclaim(tmp_path):
    """Full lifecycle: emanate → list → ask → results arrive → reclaim."""
    agent = _make_agent(tmp_path, ["file", "daemon"])
    agent.inbox = queue.Queue()
    mgr = agent.get_capability("daemon")

    tc = ToolCall(name="read", args={"file_path": "/tmp/x"}, id="tc-1")
    resp1 = MagicMock()
    resp1.text = "Checking files..."
    resp1.tool_calls = [tc]
    resp2 = MagicMock()
    resp2.text = "Task done. Summarized architecture."
    resp2.tool_calls = []

    mock_session = MagicMock()
    mock_session.send = MagicMock(side_effect=[resp1, resp2])
    agent.service.create_session = MagicMock(return_value=mock_session)
    agent.service.make_tool_result = MagicMock(return_value="mock_result")

    result = mgr.handle({"action": "emanate", "tasks": [
        {"task": "summarize architecture", "tools": ["file"]},
    ]})
    assert result["status"] == "dispatched"
    assert result["ids"] == ["em-1"]

    time.sleep(0.1)
    time.sleep(2)

    list_result = mgr._handle_list()
    statuses = {e["id"]: e["status"] for e in list_result["emanations"]}
    assert statuses.get("em-1") == "done"

    messages = []
    while not agent.inbox.empty():
        messages.append(agent.inbox.get_nowait())
    assert len(messages) >= 1
    texts = [m.content for m in messages]
    assert any("Task done" in t for t in texts)

    reclaim_result = mgr._handle_reclaim()
    assert reclaim_result["status"] == "reclaimed"


def test_sequential_emanate_increments_ids(tmp_path):
    """Multiple emanate calls produce sequential IDs."""
    agent = _make_agent(tmp_path, ["file", "daemon"])
    agent.inbox = queue.Queue()
    mgr = agent.get_capability("daemon")

    mock_session = MagicMock()
    mock_resp = MagicMock()
    mock_resp.text = "done"
    mock_resp.tool_calls = []
    mock_session.send = MagicMock(return_value=mock_resp)
    agent.service.create_session = MagicMock(return_value=mock_session)

    r1 = mgr.handle({"action": "emanate", "tasks": [{"task": "a", "tools": ["file"]}]})
    time.sleep(0.5)
    r2 = mgr.handle({"action": "emanate", "tasks": [{"task": "b", "tools": ["file"]}]})

    assert r1["ids"] == ["em-1"]
    assert r2["ids"] == ["em-2"]
