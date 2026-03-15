"""Tests for the email capability (mailbox, reply, cc/bcc on top of mail)."""
import socket
import threading
from unittest.mock import MagicMock

from stoai.agent import BaseAgent
from stoai.config import AgentConfig


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def _get_free_port():
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.bind(("127.0.0.1", 0))
    port = sock.getsockname()[1]
    sock.close()
    return port


# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

def test_email_capability_registers_tool():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = agent.add_capability("email")
    assert "email" in agent._mcp_handlers
    assert mgr is not None


def test_email_capability_adds_system_prompt():
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    agent.add_capability("email")
    section = agent._prompt_manager.read_section("email_instructions")
    assert section is not None


# ---------------------------------------------------------------------------
# Receive interception
# ---------------------------------------------------------------------------

def test_email_receive_intercept():
    """Email capability should intercept mail receive and store in mailbox."""
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = agent.add_capability("email")
    mgr.on_mail_received({"from": "sender", "to": ["test"], "subject": "hi", "message": "body"})
    assert len(mgr._mailbox) == 1
    assert mgr._mailbox[0]["from"] == "sender"
    assert not agent.inbox.empty()
    notification = agent.inbox.get_nowait()
    assert "hi" in notification.content  # subject in notification


def test_email_receive_intercept_via_agent():
    """After add_capability('email'), agent._on_mail_received should route to mailbox."""
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = agent.add_capability("email")
    # Call via agent's method — should be intercepted
    agent._on_mail_received({"from": "sender", "to": ["test"], "subject": "hi", "message": "body"})
    assert len(mgr._mailbox) == 1


# ---------------------------------------------------------------------------
# Mailbox: check, read
# ---------------------------------------------------------------------------

def test_email_check_mailbox():
    """email check should return stored emails with IDs."""
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = agent.add_capability("email")
    mgr.on_mail_received({"from": "a", "to": ["test"], "subject": "s1", "message": "m1"})
    mgr.on_mail_received({"from": "b", "to": ["test"], "subject": "s2", "message": "m2"})
    result = mgr.handle({"action": "check"})
    assert result["total"] == 2
    assert result["showing"] == 2
    assert all("id" in e for e in result["emails"])


def test_email_read_by_id():
    """email read should return full content by ID."""
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = agent.add_capability("email")
    mgr.on_mail_received({"from": "sender", "to": ["test"], "subject": "topic", "message": "full body"})
    eid = mgr._mailbox[0]["id"]
    result = mgr.handle({"action": "read", "email_id": eid})
    assert result["status"] == "ok"
    assert result["message"] == "full body"
    assert result["subject"] == "topic"


def test_email_read_marks_as_read():
    """email read should mark the message as read."""
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = agent.add_capability("email")
    mgr.on_mail_received({"from": "sender", "message": "m"})
    eid = mgr._mailbox[0]["id"]
    assert mgr._mailbox[0]["unread"] is True
    mgr.handle({"action": "read", "email_id": eid})
    assert mgr._mailbox[0]["unread"] is False


# ---------------------------------------------------------------------------
# Send with multi-to, CC, BCC
# ---------------------------------------------------------------------------

def test_email_send_multi_to():
    """email send should deliver to multiple addresses."""
    from stoai.services.mail import TCPMailService

    received = {0: [], 1: []}
    events = [threading.Event(), threading.Event()]
    ports = [_get_free_port(), _get_free_port()]
    services = []
    for i in range(2):
        svc = TCPMailService(listen_port=ports[i])
        svc.listen(on_message=lambda msg, idx=i: (received[idx].append(msg), events[idx].set()))
        services.append(svc)

    try:
        sender_svc = TCPMailService()
        agent = BaseAgent(agent_id="sender", service=make_mock_service(), mail_service=sender_svc)
        mgr = agent.add_capability("email")
        addrs = [f"127.0.0.1:{p}" for p in ports]
        result = mgr.handle({"action": "send", "address": addrs, "message": "multi-to"})
        assert result["status"] == "delivered"
        for ev in events:
            assert ev.wait(timeout=5.0)
        for i in range(2):
            assert received[i][0]["message"] == "multi-to"
    finally:
        for svc in services:
            svc.stop()


def test_email_send_cc_visible():
    """CC addresses should receive the email with cc field visible."""
    from stoai.services.mail import TCPMailService

    received = {0: [], 1: []}
    events = [threading.Event(), threading.Event()]
    ports = [_get_free_port(), _get_free_port()]
    services = []
    for i in range(2):
        svc = TCPMailService(listen_port=ports[i])
        svc.listen(on_message=lambda msg, idx=i: (received[idx].append(msg), events[idx].set()))
        services.append(svc)

    try:
        sender_svc = TCPMailService()
        agent = BaseAgent(agent_id="sender", service=make_mock_service(), mail_service=sender_svc)
        mgr = agent.add_capability("email")
        to_addr = f"127.0.0.1:{ports[0]}"
        cc_addr = f"127.0.0.1:{ports[1]}"
        result = mgr.handle({"action": "send", "address": to_addr, "message": "cc test", "cc": [cc_addr]})
        assert result["status"] == "delivered"
        for ev in events:
            assert ev.wait(timeout=5.0)
        assert received[0][0]["cc"] == [cc_addr]
        assert received[1][0]["cc"] == [cc_addr]
    finally:
        for svc in services:
            svc.stop()


def test_email_send_bcc_hidden():
    """BCC addresses should receive the email but bcc field should NOT be in payload."""
    from stoai.services.mail import TCPMailService

    received = {0: [], 1: []}
    events = [threading.Event(), threading.Event()]
    ports = [_get_free_port(), _get_free_port()]
    services = []
    for i in range(2):
        svc = TCPMailService(listen_port=ports[i])
        svc.listen(on_message=lambda msg, idx=i: (received[idx].append(msg), events[idx].set()))
        services.append(svc)

    try:
        sender_svc = TCPMailService()
        agent = BaseAgent(agent_id="sender", service=make_mock_service(), mail_service=sender_svc)
        mgr = agent.add_capability("email")
        to_addr = f"127.0.0.1:{ports[0]}"
        bcc_addr = f"127.0.0.1:{ports[1]}"
        result = mgr.handle({"action": "send", "address": to_addr, "message": "bcc test", "bcc": [bcc_addr]})
        assert result["status"] == "delivered"
        for ev in events:
            assert ev.wait(timeout=5.0)
        assert received[0][0]["message"] == "bcc test"
        assert received[1][0]["message"] == "bcc test"
        assert "bcc" not in received[0][0]
        assert "bcc" not in received[1][0]
    finally:
        for svc in services:
            svc.stop()


# ---------------------------------------------------------------------------
# Reply
# ---------------------------------------------------------------------------

def test_email_reply():
    """reply should auto-fill address from original sender and prefix subject."""
    agent = BaseAgent(agent_id="replier", service=make_mock_service())
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")

    mgr.on_mail_received({
        "from": "alice",
        "to": ["me"],
        "subject": "Original topic",
        "message": "Please respond",
    })
    eid = mgr._mailbox[0]["id"]
    result = mgr.handle({"action": "reply", "email_id": eid, "message": "Here is my reply"})
    assert result["status"] == "delivered"
    sent_payload = mock_svc.send.call_args[0][1]
    assert sent_payload["subject"] == "Re: Original topic"
    assert sent_payload["message"] == "Here is my reply"


def test_email_reply_no_double_re():
    """reply should not stack Re: prefix."""
    agent = BaseAgent(agent_id="replier", service=make_mock_service())
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")

    mgr.on_mail_received({
        "from": "other",
        "to": ["me"],
        "subject": "Re: Already replied",
        "message": "text",
    })
    eid = mgr._mailbox[0]["id"]
    result = mgr.handle({"action": "reply", "email_id": eid, "message": "follow up"})
    assert result["status"] == "delivered"
    sent_payload = mock_svc.send.call_args[0][1]
    assert sent_payload["subject"] == "Re: Already replied"


# ---------------------------------------------------------------------------
# Reply All
# ---------------------------------------------------------------------------

def test_email_reply_all():
    """reply_all should send to sender + CC all other recipients minus self."""
    agent = BaseAgent(agent_id="replier", service=make_mock_service())
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")

    mgr.on_mail_received({
        "from": "alice",
        "to": ["me", "bob"],
        "cc": ["charlie"],
        "subject": "Group thread",
        "message": "discussion",
    })
    eid = mgr._mailbox[0]["id"]
    result = mgr.handle({"action": "reply_all", "email_id": eid, "message": "my thoughts"})
    assert result["status"] == "delivered"

    sent_addresses = [call[0][0] for call in mock_svc.send.call_args_list]
    assert "alice" in sent_addresses
    assert "bob" in sent_addresses
    assert "charlie" in sent_addresses
    assert "me" not in sent_addresses


def test_email_reply_all_excludes_self():
    """reply_all should not duplicate the sender address."""
    agent = BaseAgent(agent_id="replier", service=make_mock_service())
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")

    mgr.on_mail_received({
        "from": "alice",
        "to": ["me", "alice"],
        "subject": "Self-cc",
        "message": "text",
    })
    eid = mgr._mailbox[0]["id"]
    result = mgr.handle({"action": "reply_all", "email_id": eid, "message": "reply"})
    assert result["status"] == "delivered"
    sent_addresses = [call[0][0] for call in mock_svc.send.call_args_list]
    assert sent_addresses.count("alice") == 1
    assert "me" not in sent_addresses


# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------

def test_email_without_mail_service():
    """email send should return error if mail service not configured."""
    agent = BaseAgent(agent_id="test", service=make_mock_service())
    mgr = agent.add_capability("email")
    result = mgr.handle({"action": "send", "address": "someone", "message": "hello"})
    assert "error" in result
