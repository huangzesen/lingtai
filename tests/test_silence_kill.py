"""Tests for silence and kill mail types."""
from __future__ import annotations

import threading
from unittest.mock import MagicMock

from stoai.agent import Agent
from stoai.base_agent import BaseAgent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


# ---------------------------------------------------------------------------
# Silence — interrupt + idle
# ---------------------------------------------------------------------------


def test_silence_sets_cancel_event(tmp_path):
    """Silence-type email should set _cancel_event."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert not agent._cancel_event.is_set()

    agent._on_mail_received({
        "from": "boss", "to": "test", "type": "silence",
    })

    assert agent._cancel_event.is_set()


def test_silence_bypasses_mail_queue(tmp_path):
    """Silence-type email should NOT enter the normal mail queue."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({
        "from": "boss", "to": "test", "type": "silence",
    })

    assert len(agent._mail_queue) == initial_count


def test_silence_deactivates_conscience(tmp_path):
    """Silence should deactivate conscience timer if active."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    # Activate conscience manually
    mgr._activate()
    assert mgr._horme_active

    agent._on_mail_received({"from": "boss", "type": "silence"})

    assert not mgr._horme_active
    assert mgr._timer is None
    agent.stop(timeout=1.0)


def test_silence_without_conscience_still_works(tmp_path):
    """Silence should work fine when conscience capability is not present."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)

    agent._on_mail_received({"from": "boss", "type": "silence"})

    assert agent._cancel_event.is_set()


# ---------------------------------------------------------------------------
# Kill — hard stop
# ---------------------------------------------------------------------------


def test_kill_sets_shutdown_and_cancel(tmp_path):
    """Kill-type email should set both _shutdown and _cancel_event immediately."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert not agent._shutdown.is_set()
    assert not agent._cancel_event.is_set()

    agent._on_mail_received({"from": "boss", "type": "kill"})

    # Both must be set synchronously (before the stop thread runs)
    assert agent._shutdown.is_set()
    assert agent._cancel_event.is_set()


def test_kill_bypasses_mail_queue(tmp_path):
    """Kill-type email should NOT enter the normal mail queue."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({"from": "boss", "type": "kill"})

    assert len(agent._mail_queue) == initial_count


def test_kill_stops_running_agent(tmp_path):
    """Kill should cause a running agent to exit its run loop."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent.start()
    assert agent._thread.is_alive()

    agent._on_mail_received({"from": "boss", "type": "kill"})

    # Wait for the stop thread to complete (it calls agent.stop())
    agent._thread.join(timeout=5.0)
    assert not agent._thread.is_alive()


# ---------------------------------------------------------------------------
# Admin privilege gate (mail intrinsic)
# ---------------------------------------------------------------------------


def test_non_admin_cannot_send_silence_via_mail(tmp_path):
    """Non-admin should be blocked from sending silence mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin={},
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "message": "shh", "type": "silence",
    })
    assert "error" in result or result.get("status") == "error"


def test_non_admin_cannot_send_kill_via_mail(tmp_path):
    """Non-admin should be blocked from sending kill mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin={},
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "message": "die", "type": "kill",
    })
    assert "error" in result or result.get("status") == "error"


def test_admin_can_send_silence_via_mail(tmp_path):
    """Admin should be able to send silence mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin={"silence": True, "kill": True},
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    mock_mail.send.return_value = None
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "message": "shh", "type": "silence",
    })
    assert result["status"] == "delivered"
    payload = mock_mail.send.call_args[0][1]
    assert payload["type"] == "silence"


def test_admin_can_send_kill_via_mail(tmp_path):
    """Admin should be able to send kill mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin={"silence": True, "kill": True},
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    mock_mail.send.return_value = None
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "message": "die", "type": "kill",
    })
    assert result["status"] == "delivered"
    payload = mock_mail.send.call_args[0][1]
    assert payload["type"] == "kill"


# ---------------------------------------------------------------------------
# Admin privilege gate (email capability)
# ---------------------------------------------------------------------------


def test_non_admin_cannot_send_silence_via_email(tmp_path):
    """Non-admin should be blocked from sending silence via email."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["email"], admin={},
    )
    mock_mail = MagicMock()
    mock_mail.address = "me"
    agent._mail_service = mock_mail
    mgr = agent.get_capability("email")

    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "shh", "type": "silence",
    })
    assert "error" in result


def test_non_admin_cannot_send_kill_via_email(tmp_path):
    """Non-admin should be blocked from sending kill via email."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["email"], admin={},
    )
    mock_mail = MagicMock()
    mock_mail.address = "me"
    agent._mail_service = mock_mail
    mgr = agent.get_capability("email")

    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "die", "type": "kill",
    })
    assert "error" in result


# ---------------------------------------------------------------------------
# Normal mail — unchanged behavior
# ---------------------------------------------------------------------------


def test_normal_email_queued(tmp_path):
    """Normal-type email should go through the regular queue path."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({
        "from": "colleague", "to": "test", "subject": "hello",
        "message": "hi there", "type": "normal",
    })

    assert len(agent._mail_queue) == initial_count + 1
    assert not agent._cancel_event.is_set()


def test_missing_type_defaults_to_normal(tmp_path):
    """Mail without a type field should be treated as normal."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({
        "from": "colleague", "to": "test", "message": "hi",
    })

    assert len(agent._mail_queue) == initial_count + 1
    assert not agent._cancel_event.is_set()


def test_unrecognized_type_treated_as_normal(tmp_path):
    """Unrecognized mail type should be treated as normal."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    initial_count = len(agent._mail_queue)

    agent._on_mail_received({
        "from": "someone", "type": "bogus", "message": "test",
    })

    assert len(agent._mail_queue) == initial_count + 1


def test_non_admin_can_send_normal_mail(tmp_path):
    """Non-admin should be able to send normal mail."""
    agent = BaseAgent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path, admin={},
    )
    mock_mail = MagicMock()
    mock_mail.address = "127.0.0.1:8000"
    mock_mail.send.return_value = None
    agent._mail_service = mock_mail

    result = agent._intrinsics["mail"]({
        "action": "send", "address": "127.0.0.1:8001",
        "subject": "hello", "message": "hi there",
    })
    assert result["status"] == "delivered"


# ---------------------------------------------------------------------------
# Internal state
# ---------------------------------------------------------------------------


def test_cancel_event_always_created(tmp_path):
    """Agent should always have _cancel_event (no external injection)."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    assert isinstance(agent._cancel_event, threading.Event)
    assert not agent._cancel_event.is_set()


def test_admin_dict_stored(tmp_path):
    """Admin dict should be stored on the agent."""
    agent_normal = BaseAgent(agent_name="a", service=make_mock_service(), base_dir=tmp_path)
    assert agent_normal._admin == {}

    agent_admin = BaseAgent(agent_name="b", service=make_mock_service(), base_dir=tmp_path,
                            admin={"silence": True, "kill": True})
    assert agent_admin._admin == {"silence": True, "kill": True}

    agent_peer = BaseAgent(agent_name="c", service=make_mock_service(), base_dir=tmp_path,
                           admin={"silence": True})
    assert agent_peer._admin.get("silence") is True
    assert not agent_peer._admin.get("kill")


# ---------------------------------------------------------------------------
# Tool executor cancel check
# ---------------------------------------------------------------------------


def test_sequential_execution_stops_on_cancel(tmp_path):
    """Sequential tool execution should return empty when cancel event is set."""
    from stoai.loop_guard import LoopGuard
    from stoai.tool_executor import ToolExecutor
    from stoai.llm import ToolCall

    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    agent._cancel_event.set()

    tc = ToolCall(name="clock", args={"action": "check"}, id="tc1")
    guard = LoopGuard(max_total_calls=10)

    executor = ToolExecutor(
        dispatch_fn=agent._dispatch_tool,
        make_tool_result_fn=lambda name, result, **kw: agent.service.make_tool_result(
            name, result, provider=agent._config.provider, **kw
        ),
        guard=guard,
        known_tools=set(agent._intrinsics) | set(agent._mcp_handlers),
        logger_fn=agent._log,
    )
    results, intercepted, text = executor.execute(
        [tc], cancel_event=agent._cancel_event, collected_errors=[],
    )

    assert results == []
    assert intercepted is False
