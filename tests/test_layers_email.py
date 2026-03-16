"""Tests for the email capability (filesystem-based mailbox)."""
import json
import socket
import threading
from datetime import datetime, timezone
from pathlib import Path
from unittest.mock import MagicMock
from uuid import uuid4

from stoai.agent import BaseAgent


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


def _make_inbox_email(working_dir, *, sender="sender", to=None, subject="test",
                       message="body", cc=None, attachments=None):
    """Create an email on disk in mailbox/inbox/{uuid}/message.json.
    Returns the email_id (directory name)."""
    email_id = str(uuid4())
    msg_dir = working_dir / "mailbox" / "inbox" / email_id
    msg_dir.mkdir(parents=True, exist_ok=True)
    data = {
        "_mailbox_id": email_id,
        "from": sender,
        "to": to or ["test"],
        "subject": subject,
        "message": message,
        "received_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    if cc:
        data["cc"] = cc
    if attachments:
        data["attachments"] = attachments
    (msg_dir / "message.json").write_text(json.dumps(data, indent=2))
    return email_id


# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

def test_email_capability_registers_tool(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    assert "email" in agent._mcp_handlers
    assert "email" in [s.name for s in agent._mcp_schemas]
    assert mgr is not None


# ---------------------------------------------------------------------------
# Receive interception
# ---------------------------------------------------------------------------

def test_email_receive_notification(tmp_path):
    """on_normal_mail should send notification to agent inbox."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    mgr.on_normal_mail({
        "_mailbox_id": "abc123",
        "from": "sender",
        "to": ["test"],
        "subject": "hi",
        "message": "body",
    })
    assert not agent.inbox.empty()
    notification = agent.inbox.get_nowait()
    assert "hi" in notification.content
    assert "abc123" in notification.content


def test_email_receive_fallback_id(tmp_path):
    """on_normal_mail should generate ID if _mailbox_id is absent."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    mgr.on_normal_mail({"from": "sender", "message": "body"})
    assert not agent.inbox.empty()


def test_email_receive_via_agent(tmp_path):
    """After add_capability('email'), agent._on_mail_received should route to mailbox."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_capability("email")
    agent._on_mail_received({
        "_mailbox_id": "xyz",
        "from": "sender",
        "to": ["test"],
        "subject": "hi",
        "message": "body",
    })
    assert not agent.inbox.empty()


# ---------------------------------------------------------------------------
# Mailbox: check, read
# ---------------------------------------------------------------------------

def test_email_check_inbox(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    _make_inbox_email(agent.working_dir, sender="a", subject="s1", message="m1")
    _make_inbox_email(agent.working_dir, sender="b", subject="s2", message="m2")
    result = mgr.handle({"action": "check"})
    assert result["status"] == "ok"
    assert result["total"] == 2
    assert all("id" in e for e in result["emails"])


def test_email_check_sent(tmp_path):
    """check with folder=sent should show sent emails."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    mgr.handle({"action": "send", "address": "someone", "message": "hello", "subject": "test"})
    result = mgr.handle({"action": "check", "folder": "sent"})
    assert result["total"] == 1
    assert result["emails"][0]["from"] == "me"


def test_email_check_empty_mailbox(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    result = mgr.handle({"action": "check"})
    assert result["status"] == "ok"
    assert result["total"] == 0


def test_email_read_by_id(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="sender", subject="topic", message="full body")
    result = mgr.handle({"action": "read", "email_id": eid})
    assert result["status"] == "ok"
    assert result["message"] == "full body"
    assert result["subject"] == "topic"


def test_email_read_marks_as_read(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(agent.working_dir, message="m")
    # First check — should be unread
    result = mgr.handle({"action": "check"})
    assert result["emails"][0]["unread"] is True
    # Read it
    mgr.handle({"action": "read", "email_id": eid})
    # Now should be read
    result = mgr.handle({"action": "check"})
    assert result["emails"][0]["unread"] is False


def test_email_read_shows_attachments(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(agent.working_dir, subject="photo", message="look",
                            attachments=["/path/to/photo.png"])
    result = mgr.handle({"action": "read", "email_id": eid})
    assert result["status"] == "ok"
    assert "attachments" in result
    assert any("photo.png" in p for p in result["attachments"])


# ---------------------------------------------------------------------------
# Send — saves to sent/
# ---------------------------------------------------------------------------

def test_email_send_saves_to_sent(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "hello", "subject": "test",
    })
    assert result["status"] == "delivered"
    sent_dir = agent.working_dir / "mailbox" / "sent"
    assert sent_dir.is_dir()
    sent_emails = list(sent_dir.iterdir())
    assert len(sent_emails) == 1
    msg = json.loads((sent_emails[0] / "message.json").read_text())
    assert msg["message"] == "hello"
    assert msg["sent_at"]


def test_email_send_saves_bcc_in_sent(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    mgr.handle({
        "action": "send", "address": "someone",
        "message": "secret", "bcc": ["hidden"],
    })
    sent_dir = agent.working_dir / "mailbox" / "sent"
    msg = json.loads(list(sent_dir.iterdir())[0].joinpath("message.json").read_text())
    assert msg["bcc"] == ["hidden"]


def test_email_blocks_identical_consecutive_send(tmp_path):
    """Sending the exact same message twice to the same recipient is blocked."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")

    # First send — should work
    result = mgr.handle({
        "action": "send", "address": "127.0.0.1:8888",
        "subject": "hi", "message": "\ud83d\udc4d",
    })
    assert result["status"] == "delivered"

    # Identical send — should be blocked
    result = mgr.handle({
        "action": "send", "address": "127.0.0.1:8888",
        "subject": "hi", "message": "\ud83d\udc4d",
    })
    assert result["status"] == "blocked"
    assert "warning" in result

    # Different message — should work
    result = mgr.handle({
        "action": "send", "address": "127.0.0.1:8888",
        "subject": "hi", "message": "Got it, thanks!",
    })
    assert result["status"] == "delivered"


def test_email_blocks_identical_reply(tmp_path):
    """Replying with the same message twice is blocked."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")

    # Create an inbox email to reply to
    _make_inbox_email(agent.working_dir, sender="127.0.0.1:8888", subject="hello", message="hi there")
    check = mgr.handle({"action": "check"})
    email_id = check["emails"][0]["id"]

    # First reply
    result = mgr.handle({"action": "reply", "email_id": email_id, "message": "\ud83d\udc4d"})
    assert result["status"] == "delivered"

    # Identical reply — blocked
    result = mgr.handle({"action": "reply", "email_id": email_id, "message": "\ud83d\udc4d"})
    assert result["status"] == "blocked"


def test_email_send_with_attachments(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    result = mgr.handle({
        "action": "send",
        "address": "127.0.0.1:8888",
        "subject": "file for you",
        "message": "see attached",
        "attachments": ["/path/to/file.png"],
    })
    assert result["status"] == "delivered"
    sent = mail_svc.send.call_args[0][1]
    assert sent.get("attachments") == ["/path/to/file.png"]


# ---------------------------------------------------------------------------
# Send — TCP integration tests
# ---------------------------------------------------------------------------

def test_email_send_multi_to(tmp_path):
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
        agent = BaseAgent(agent_id="sender", service=make_mock_service(), mail_service=sender_svc, base_dir=tmp_path)
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


def test_email_send_cc_visible(tmp_path):
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
        agent = BaseAgent(agent_id="sender", service=make_mock_service(), mail_service=sender_svc, base_dir=tmp_path)
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


def test_email_send_bcc_hidden(tmp_path):
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
        agent = BaseAgent(agent_id="sender", service=make_mock_service(), mail_service=sender_svc, base_dir=tmp_path)
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

def test_email_reply(tmp_path):
    agent = BaseAgent(agent_id="replier", service=make_mock_service(), base_dir=tmp_path)
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="alice", subject="Original topic", message="Please respond")
    result = mgr.handle({"action": "reply", "email_id": eid, "message": "Here is my reply"})
    assert result["status"] == "delivered"
    sent_payload = mock_svc.send.call_args[0][1]
    assert sent_payload["subject"] == "Re: Original topic"
    assert sent_payload["message"] == "Here is my reply"


def test_email_reply_no_double_re(tmp_path):
    agent = BaseAgent(agent_id="replier", service=make_mock_service(), base_dir=tmp_path)
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="other", subject="Re: Already replied", message="text")
    result = mgr.handle({"action": "reply", "email_id": eid, "message": "follow up"})
    sent_payload = mock_svc.send.call_args[0][1]
    assert sent_payload["subject"] == "Re: Already replied"


# ---------------------------------------------------------------------------
# Reply All
# ---------------------------------------------------------------------------

def test_email_reply_all(tmp_path):
    agent = BaseAgent(agent_id="replier", service=make_mock_service(), base_dir=tmp_path)
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="alice", to=["me", "bob"],
                            cc=["charlie"], subject="Group thread", message="discussion")
    result = mgr.handle({"action": "reply_all", "email_id": eid, "message": "my thoughts"})
    assert result["status"] == "delivered"
    sent_addresses = [call[0][0] for call in mock_svc.send.call_args_list]
    assert "alice" in sent_addresses
    assert "bob" in sent_addresses
    assert "charlie" in sent_addresses
    assert "me" not in sent_addresses


def test_email_reply_all_excludes_self(tmp_path):
    agent = BaseAgent(agent_id="replier", service=make_mock_service(), base_dir=tmp_path)
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = True
    agent._mail_service = mock_svc
    mgr = agent.add_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="alice", to=["me", "alice"],
                            subject="Self-cc", message="text")
    result = mgr.handle({"action": "reply_all", "email_id": eid, "message": "reply"})
    assert result["status"] == "delivered"
    sent_addresses = [call[0][0] for call in mock_svc.send.call_args_list]
    assert sent_addresses.count("alice") == 1
    assert "me" not in sent_addresses


# ---------------------------------------------------------------------------
# Search
# ---------------------------------------------------------------------------

def test_email_search_by_subject(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    _make_inbox_email(agent.working_dir, subject="important meeting", message="body1")
    _make_inbox_email(agent.working_dir, subject="casual chat", message="body2")
    result = mgr.handle({"action": "search", "query": "important"})
    assert result["total"] == 1
    assert "important" in result["emails"][0]["subject"]


def test_email_search_by_sender(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    _make_inbox_email(agent.working_dir, sender="alice@test", message="hello")
    _make_inbox_email(agent.working_dir, sender="bob@test", message="world")
    result = mgr.handle({"action": "search", "query": "alice"})
    assert result["total"] == 1


def test_email_search_by_message_body(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    _make_inbox_email(agent.working_dir, message="the secret code is 42")
    _make_inbox_email(agent.working_dir, message="nothing interesting")
    result = mgr.handle({"action": "search", "query": "secret.*42"})
    assert result["total"] == 1


def test_email_search_folder_filter(tmp_path):
    """Search with folder param should only search that folder."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = True
    agent._mail_service = mail_svc
    mgr = agent.add_capability("email")
    _make_inbox_email(agent.working_dir, message="keyword in inbox")
    mgr.handle({"action": "send", "address": "someone", "message": "keyword in sent"})
    # Search both — should find 2
    result = mgr.handle({"action": "search", "query": "keyword"})
    assert result["total"] == 2
    # Search inbox only — should find 1
    result = mgr.handle({"action": "search", "query": "keyword", "folder": "inbox"})
    assert result["total"] == 1


def test_email_search_invalid_regex(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    result = mgr.handle({"action": "search", "query": "[invalid"})
    assert "error" in result


# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------

def test_email_without_mail_service(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    result = mgr.handle({"action": "send", "address": "someone", "message": "hello"})
    assert "error" in result


def test_email_read_not_found(tmp_path):
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    mgr = agent.add_capability("email")
    result = mgr.handle({"action": "read", "email_id": "nonexistent"})
    assert "error" in result
