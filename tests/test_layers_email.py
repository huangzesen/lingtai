"""Tests for the email capability (filesystem-based mailbox)."""
import json
import threading
import time
from datetime import datetime, timezone
from pathlib import Path
from unittest.mock import MagicMock
from uuid import uuid4

from lingtai.agent import Agent


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


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
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    assert "email" in agent._tool_handlers
    assert "email" in [s.name for s in agent._tool_schemas]
    assert mgr is not None


# ---------------------------------------------------------------------------
# Receive interception
# ---------------------------------------------------------------------------

def test_email_receive_notification(tmp_path):
    """Incoming mail should send notification to agent inbox."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    agent._on_mail_received({
        "_mailbox_id": "abc123",
        "from": "sender",
        "to": ["test"],
        "subject": "hi",
        "message": "body",
    })
    assert not agent.inbox.empty()
    notification = agent.inbox.get_nowait()
    assert notification.sender == "system"
    assert "email box" in notification.content
    assert 'email(action=' in notification.content


def test_email_receive_fallback_id(tmp_path):
    """Notification should work even without _mailbox_id."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    agent._on_mail_received({"from": "sender", "message": "body"})
    assert not agent.inbox.empty()


def test_email_receive_via_agent(tmp_path):
    """After add_capability('email'), agent._on_mail_received should route to mailbox."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
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
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    _make_inbox_email(agent.working_dir, sender="a", subject="s1", message="m1")
    _make_inbox_email(agent.working_dir, sender="b", subject="s2", message="m2")
    result = mgr.handle({"action": "check"})
    assert result["status"] == "ok"
    assert result["total"] == 2
    assert all("id" in e for e in result["emails"])


def test_email_check_sent(tmp_path):
    """check with folder=sent should show sent emails."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    mgr.handle({"action": "send", "address": "someone", "message": "hello", "subject": "test"})
    result = mgr.handle({"action": "check", "folder": "sent"})
    assert result["total"] == 1
    assert result["emails"][0]["from"] == "me"


def test_email_check_empty_mailbox(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "check"})
    assert result["status"] == "ok"
    assert result["total"] == 0


def test_email_read_by_id(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="sender", subject="topic", message="full body")
    result = mgr.handle({"action": "read", "email_id": eid})
    assert result["status"] == "ok"
    assert len(result["emails"]) == 1
    assert result["emails"][0]["message"] == "full body"
    assert result["emails"][0]["subject"] == "topic"


def test_email_read_marks_as_read(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
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
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    eid = _make_inbox_email(agent.working_dir, subject="photo", message="look",
                            attachments=["/path/to/photo.png"])
    result = mgr.handle({"action": "read", "email_id": eid})
    assert result["status"] == "ok"
    assert "attachments" in result["emails"][0]
    assert any("photo.png" in p for p in result["emails"][0]["attachments"])


# ---------------------------------------------------------------------------
# Send — outbox → mailman pipeline
# ---------------------------------------------------------------------------

def test_email_send_through_mailman(tmp_path):
    """Email send goes through outbox → mailman → sent."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "hello", "subject": "test",
    })
    assert result["status"] == "sent"
    assert result["delay"] == 0
    time.sleep(0.5)
    sent_dir = agent.working_dir / "mailbox" / "sent"
    assert sent_dir.is_dir()
    sent_items = list(sent_dir.iterdir())
    assert len(sent_items) == 1
    msg = json.loads((sent_items[0] / "message.json").read_text())
    assert msg["message"] == "hello"
    assert msg["sent_at"]


def test_email_send_with_delay(tmp_path):
    """Email send with delay dispatches after waiting."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "delayed", "delay": 1,
    })
    assert result["status"] == "sent"
    assert result["delay"] == 1
    mail_svc.send.assert_not_called()
    time.sleep(1.5)
    mail_svc.send.assert_called_once()


def test_email_send_cc_one_sent_record(tmp_path):
    """CC/BCC email produces one sent record, not one per recipient."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "action": "send", "address": ["a", "b"],
        "cc": ["c"], "bcc": ["d"],
        "message": "broadcast", "subject": "multi",
    })
    assert result["status"] == "sent"
    time.sleep(0.5)
    sent_dir = agent.working_dir / "mailbox" / "sent"
    sent_items = list(sent_dir.iterdir())
    assert len(sent_items) == 1  # ONE sent record
    msg = json.loads((sent_items[0] / "message.json").read_text())
    assert msg["bcc"] == ["d"]


# ---------------------------------------------------------------------------
# Send — saves to sent/
# ---------------------------------------------------------------------------

def test_email_send_saves_to_sent(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "hello", "subject": "test",
    })
    assert result["status"] == "sent"
    sent_dir = agent.working_dir / "mailbox" / "sent"
    assert sent_dir.is_dir()
    sent_emails = list(sent_dir.iterdir())
    assert len(sent_emails) == 1
    msg = json.loads((sent_emails[0] / "message.json").read_text())
    assert msg["message"] == "hello"
    assert msg["sent_at"]


def test_email_send_saves_bcc_in_sent(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    mgr.handle({
        "action": "send", "address": "someone",
        "message": "secret", "bcc": ["hidden"],
    })
    sent_dir = agent.working_dir / "mailbox" / "sent"
    msg = json.loads(list(sent_dir.iterdir())[0].joinpath("message.json").read_text())
    assert msg["bcc"] == ["hidden"]


def test_email_blocks_identical_consecutive_send(tmp_path):
    """Sending the exact same message twice to the same recipient is blocked."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    mgr._dup_free_passes = 1

    # First send — should work
    result = mgr.handle({
        "action": "send", "address": "127.0.0.1:8888",
        "subject": "hi", "message": "thumbs up",
    })
    assert result["status"] == "sent"

    # Identical send — should be blocked
    result = mgr.handle({
        "action": "send", "address": "127.0.0.1:8888",
        "subject": "hi", "message": "thumbs up",
    })
    assert result["status"] == "blocked"
    assert "warning" in result

    # Different message — should work
    result = mgr.handle({
        "action": "send", "address": "127.0.0.1:8888",
        "subject": "hi", "message": "Got it, thanks!",
    })
    assert result["status"] == "sent"


def test_email_blocks_identical_reply(tmp_path):
    """Replying with the same message twice is blocked."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    mgr._dup_free_passes = 1

    # Create an inbox email to reply to
    _make_inbox_email(agent.working_dir, sender="127.0.0.1:8888", subject="hello", message="hi there")
    check = mgr.handle({"action": "check"})
    email_id = check["emails"][0]["id"]

    # First reply
    result = mgr.handle({"action": "reply", "email_id": email_id, "message": "thumbs up"})
    assert result["status"] == "sent"

    # Identical reply — blocked
    result = mgr.handle({"action": "reply", "email_id": email_id, "message": "thumbs up"})
    assert result["status"] == "blocked"


def test_email_send_with_attachments(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "127.0.0.1:9999"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "action": "send",
        "address": "127.0.0.1:8888",
        "subject": "file for you",
        "message": "see attached",
        "attachments": ["/path/to/file.png"],
    })
    assert result["status"] == "sent"
    time.sleep(0.5)
    sent = mail_svc.send.call_args[0][1]
    assert sent.get("attachments") == ["/path/to/file.png"]


# ---------------------------------------------------------------------------
# Send — Filesystem integration tests
# ---------------------------------------------------------------------------

def _setup_receiver(tmp_path, name, stop_event):
    """Create a receiver agent dir with heartbeat and FilesystemMailService."""
    from lingtai_kernel.services.mail import FilesystemMailService
    d = tmp_path / name
    d.mkdir(parents=True, exist_ok=True)
    (d / ".agent.json").write_text(json.dumps({"agent_name": name}))
    (d / ".agent.heartbeat").write_text(str(time.time()))
    def _hb():
        while not stop_event.is_set():
            try:
                (d / ".agent.heartbeat").write_text(str(time.time()))
            except OSError:
                pass
            stop_event.wait(0.5)
    threading.Thread(target=_hb, daemon=True).start()
    svc = FilesystemMailService(working_dir=d)
    return d, svc


def test_email_send_multi_to(tmp_path):
    """email send should deliver to multiple addresses."""
    from lingtai_kernel.services.mail import FilesystemMailService

    stop = threading.Event()
    received = {0: [], 1: []}
    events = [threading.Event(), threading.Event()]
    services = []
    addrs = []
    for i in range(2):
        d, svc = _setup_receiver(tmp_path, f"recv{i}", stop)
        svc.listen(on_message=lambda msg, idx=i: (received[idx].append(msg), events[idx].set()))
        services.append(svc)
        addrs.append(str(d))

    try:
        sender_dir = tmp_path / "sender"
        sender_dir.mkdir()
        sender_svc = FilesystemMailService(working_dir=sender_dir)
        agent = Agent(service=make_mock_service(), agent_name="sender", working_dir=tmp_path / "test", mail_service=sender_svc, capabilities=["email"])
        mgr = agent.get_capability("email")
        result = mgr.handle({"action": "send", "address": addrs, "message": "multi-to"})
        assert result["status"] == "sent"
        for ev in events:
            assert ev.wait(timeout=5.0)
        for i in range(2):
            assert received[i][0]["message"] == "multi-to"
    finally:
        for svc in services:
            svc.stop()
        stop.set()


def test_email_send_cc_visible(tmp_path):
    """CC addresses should receive the email with cc field visible."""
    from lingtai_kernel.services.mail import FilesystemMailService

    stop = threading.Event()
    received = {0: [], 1: []}
    events = [threading.Event(), threading.Event()]
    services = []
    addrs = []
    for i in range(2):
        d, svc = _setup_receiver(tmp_path, f"recv{i}", stop)
        svc.listen(on_message=lambda msg, idx=i: (received[idx].append(msg), events[idx].set()))
        services.append(svc)
        addrs.append(str(d))

    try:
        sender_dir = tmp_path / "sender"
        sender_dir.mkdir()
        sender_svc = FilesystemMailService(working_dir=sender_dir)
        agent = Agent(service=make_mock_service(), agent_name="sender", working_dir=tmp_path / "test", mail_service=sender_svc, capabilities=["email"])
        mgr = agent.get_capability("email")
        to_addr = addrs[0]
        cc_addr = addrs[1]
        result = mgr.handle({"action": "send", "address": to_addr, "message": "cc test", "cc": [cc_addr]})
        assert result["status"] == "sent"
        for ev in events:
            assert ev.wait(timeout=5.0)
        assert received[0][0]["cc"] == [cc_addr]
        assert received[1][0]["cc"] == [cc_addr]
    finally:
        for svc in services:
            svc.stop()
        stop.set()


def test_email_send_bcc_hidden(tmp_path):
    """BCC addresses should receive the email but bcc field should NOT be in payload."""
    from lingtai_kernel.services.mail import FilesystemMailService

    stop = threading.Event()
    received = {0: [], 1: []}
    events = [threading.Event(), threading.Event()]
    services = []
    addrs = []
    for i in range(2):
        d, svc = _setup_receiver(tmp_path, f"recv{i}", stop)
        svc.listen(on_message=lambda msg, idx=i: (received[idx].append(msg), events[idx].set()))
        services.append(svc)
        addrs.append(str(d))

    try:
        sender_dir = tmp_path / "sender"
        sender_dir.mkdir()
        sender_svc = FilesystemMailService(working_dir=sender_dir)
        agent = Agent(service=make_mock_service(), agent_name="sender", working_dir=tmp_path / "test", mail_service=sender_svc, capabilities=["email"])
        mgr = agent.get_capability("email")
        to_addr = addrs[0]
        bcc_addr = addrs[1]
        result = mgr.handle({"action": "send", "address": to_addr, "message": "bcc test", "bcc": [bcc_addr]})
        assert result["status"] == "sent"
        for ev in events:
            assert ev.wait(timeout=5.0)
        assert received[0][0]["message"] == "bcc test"
        assert received[1][0]["message"] == "bcc test"
        assert "bcc" not in received[0][0]
        assert "bcc" not in received[1][0]
    finally:
        for svc in services:
            svc.stop()
        stop.set()


# ---------------------------------------------------------------------------
# Reply
# ---------------------------------------------------------------------------

def test_email_reply(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="replier", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = None
    agent._mail_service = mock_svc
    mgr = agent.get_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="alice", subject="Original topic", message="Please respond")
    result = mgr.handle({"action": "reply", "email_id": eid, "message": "Here is my reply"})
    assert result["status"] == "sent"
    time.sleep(0.5)
    sent_payload = mock_svc.send.call_args[0][1]
    assert sent_payload["subject"] == "Re: Original topic"
    assert sent_payload["message"] == "Here is my reply"


def test_email_reply_no_double_re(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="replier", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = None
    agent._mail_service = mock_svc
    mgr = agent.get_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="other", subject="Re: Already replied", message="text")
    result = mgr.handle({"action": "reply", "email_id": eid, "message": "follow up"})
    time.sleep(0.5)
    sent_payload = mock_svc.send.call_args[0][1]
    assert sent_payload["subject"] == "Re: Already replied"


# ---------------------------------------------------------------------------
# Reply All
# ---------------------------------------------------------------------------

def test_email_reply_all(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="replier", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = None
    agent._mail_service = mock_svc
    mgr = agent.get_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="alice", to=["me", "bob"],
                            cc=["charlie"], subject="Group thread", message="discussion")
    result = mgr.handle({"action": "reply_all", "email_id": eid, "message": "my thoughts"})
    assert result["status"] == "sent"
    time.sleep(0.5)
    sent_addresses = [call[0][0] for call in mock_svc.send.call_args_list]
    assert "alice" in sent_addresses
    assert "bob" in sent_addresses
    assert "charlie" in sent_addresses
    assert "me" not in sent_addresses


def test_email_reply_all_excludes_self(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="replier", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mock_svc = MagicMock()
    mock_svc.address = "me"
    mock_svc.send.return_value = None
    agent._mail_service = mock_svc
    mgr = agent.get_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="alice", to=["me", "alice"],
                            subject="Self-cc", message="text")
    result = mgr.handle({"action": "reply_all", "email_id": eid, "message": "reply"})
    assert result["status"] == "sent"
    time.sleep(0.5)
    sent_addresses = [call[0][0] for call in mock_svc.send.call_args_list]
    assert sent_addresses.count("alice") == 1
    assert "me" not in sent_addresses


# ---------------------------------------------------------------------------
# Search
# ---------------------------------------------------------------------------

def test_email_search_by_subject(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    _make_inbox_email(agent.working_dir, subject="important meeting", message="body1")
    _make_inbox_email(agent.working_dir, subject="casual chat", message="body2")
    result = mgr.handle({"action": "search", "query": "important"})
    assert result["total"] == 1
    assert "important" in result["emails"][0]["subject"]


def test_email_search_by_sender(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    _make_inbox_email(agent.working_dir, sender="alice@test", message="hello")
    _make_inbox_email(agent.working_dir, sender="bob@test", message="world")
    result = mgr.handle({"action": "search", "query": "alice"})
    assert result["total"] == 1


def test_email_search_by_message_body(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    _make_inbox_email(agent.working_dir, message="the secret code is 42")
    _make_inbox_email(agent.working_dir, message="nothing interesting")
    result = mgr.handle({"action": "search", "query": "secret.*42"})
    assert result["total"] == 1


def test_email_search_folder_filter(tmp_path):
    """Search with folder param should only search that folder."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    _make_inbox_email(agent.working_dir, message="keyword in inbox")
    mgr.handle({"action": "send", "address": "someone", "message": "keyword in sent"})
    # Search both — should find 2
    result = mgr.handle({"action": "search", "query": "keyword"})
    assert result["total"] == 2
    # Search inbox only — should find 1
    result = mgr.handle({"action": "search", "query": "keyword", "folder": "inbox"})
    assert result["total"] == 1


def test_email_search_invalid_regex(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "search", "query": "[invalid"})
    assert "error" in result


# ---------------------------------------------------------------------------
# Error cases
# ---------------------------------------------------------------------------

def test_email_without_mail_service(tmp_path):
    """Send without mail service succeeds at send-time."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    agent._mail_service = None
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "hello",
    })
    assert result["status"] == "sent"
    sent_dir = agent.working_dir / "mailbox" / "sent"
    assert sent_dir.is_dir()


def test_email_read_not_found(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "read", "email_id": "nonexistent"})
    assert result["status"] == "ok"
    assert result["not_found"] == ["nonexistent"]


def test_email_removes_mail_intrinsic(tmp_path):
    """When email capability is active, mail intrinsic should be removed."""
    agent = Agent(
        service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
        capabilities=["email"],
    )
    assert "mail" not in agent._intrinsics
    # But email tool should exist
    assert "email" in agent._tool_handlers
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Private mode
# ---------------------------------------------------------------------------

def test_email_private_mode_blocks_send_to_non_contact(tmp_path):
    """Private mode should block sends to addresses not in contacts."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities={"email": {"private_mode": True}})
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "send", "address": "stranger", "message": "hi"})
    assert "error" in result
    assert "Private mode" in result["error"]
    assert "stranger" in result["error"]


def test_email_private_mode_allows_send_to_contact(tmp_path):
    """Private mode should allow sends to registered contacts."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities={"email": {"private_mode": True}})
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    # Register contact first
    mgr.handle({"action": "add_contact", "name": "Alice", "address": "alice:8000"})
    result = mgr.handle({"action": "send", "address": "alice:8000", "message": "hi"})
    assert result["status"] == "sent"


def test_email_private_mode_blocks_reply_to_non_contact(tmp_path):
    """Private mode should block reply to addresses not in contacts."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities={"email": {"private_mode": True}})
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    eid = _make_inbox_email(agent.working_dir, sender="stranger", subject="hi", message="hello")
    result = mgr.handle({"action": "reply", "email_id": eid, "message": "reply"})
    assert "error" in result
    assert "Private mode" in result["error"]


def test_email_private_mode_blocks_cc_to_non_contact(tmp_path):
    """Private mode should block if any CC address is not in contacts."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities={"email": {"private_mode": True}})
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    mgr.handle({"action": "add_contact", "name": "Alice", "address": "alice:8000"})
    result = mgr.handle({
        "action": "send", "address": "alice:8000", "message": "hi",
        "cc": ["unknown:9000"],
    })
    assert "error" in result
    assert "unknown:9000" in result["error"]


def test_email_private_mode_off_allows_anyone(tmp_path):
    """Without private mode, sends to any address should work."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "send", "address": "anyone", "message": "hi"})
    assert result["status"] == "sent"


# ---------------------------------------------------------------------------
# Archive
# ---------------------------------------------------------------------------

def test_email_archive_moves_to_archive(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="keep this")
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "archive", "email_id": [email_id]})
    assert result["status"] == "ok"
    assert email_id in result["archived"]
    inbox = agent.working_dir / "mailbox" / "inbox" / email_id
    assert not inbox.exists()
    archive = agent.working_dir / "mailbox" / "archive" / email_id
    assert archive.is_dir()
    msg = json.loads((archive / "message.json").read_text())
    assert msg["subject"] == "keep this"


def test_email_archive_not_found(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "archive", "email_id": ["nonexistent"]})
    assert result["not_found"] == ["nonexistent"]


def test_email_check_archive_folder(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="archived msg")
    mgr = agent.get_capability("email")
    mgr.handle({"action": "archive", "email_id": [email_id]})
    result = mgr.handle({"action": "check", "folder": "archive"})
    assert result["total"] == 1
    assert result["emails"][0]["id"] == email_id


def test_email_read_archive_folder(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="archived")
    mgr = agent.get_capability("email")
    mgr.handle({"action": "archive", "email_id": [email_id]})
    result = mgr.handle({"action": "read", "email_id": [email_id], "folder": "archive"})
    assert len(result["emails"]) == 1
    assert result["emails"][0]["subject"] == "archived"


def test_email_search_archive_folder(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="unique archived topic")
    mgr = agent.get_capability("email")
    mgr.handle({"action": "archive", "email_id": [email_id]})
    result = mgr.handle({"action": "search", "query": "unique archived", "folder": "archive"})
    assert result["total"] == 1


# ---------------------------------------------------------------------------
# Delete
# ---------------------------------------------------------------------------

def test_email_delete_from_inbox(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="delete me")
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "delete", "email_id": [email_id]})
    assert email_id in result["deleted"]
    inbox = agent.working_dir / "mailbox" / "inbox" / email_id
    assert not inbox.exists()


def test_email_delete_from_archive(tmp_path):
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="archive then delete")
    mgr = agent.get_capability("email")
    mgr.handle({"action": "archive", "email_id": [email_id]})
    result = mgr.handle({"action": "delete", "email_id": [email_id], "folder": "archive"})
    assert email_id in result["deleted"]
    archive = agent.working_dir / "mailbox" / "archive" / email_id
    assert not archive.exists()


def test_email_delete_from_sent_rejected(tmp_path):
    """Cannot delete from sent folder."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "delete", "email_id": ["x"], "folder": "sent"})
    assert "error" in result


def test_email_archive_already_archived(tmp_path):
    """Archiving a message that's already in archive returns not_found."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="move me")
    mgr = agent.get_capability("email")
    mgr.handle({"action": "archive", "email_id": [email_id]})
    result = mgr.handle({"action": "archive", "email_id": [email_id]})
    assert result["not_found"] == [email_id]


# ---------------------------------------------------------------------------
# Schedule — schema and routing
# ---------------------------------------------------------------------------

def test_email_schedule_in_schema(tmp_path):
    """Email schema should include schedule property."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    schemas = {s.name: s for s in agent._tool_schemas}
    props = schemas["email"].parameters["properties"]
    assert "schedule" in props
    assert "create" in props["schedule"]["properties"]["action"]["enum"]


def test_email_handle_without_action_or_schedule(tmp_path):
    """Missing both action and schedule should return error."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({})
    assert "action is required" in result["error"]


def test_email_schedule_unknown_action(tmp_path):
    """Unknown schedule action should return error."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"schedule": {"action": "bogus"}})
    assert "error" in result


# ---------------------------------------------------------------------------
# Schedule — create
# ---------------------------------------------------------------------------

def test_email_schedule_create_basic(tmp_path):
    """schedule.create should persist schedule.json and return schedule_id."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "address": "someone",
        "subject": "Heartbeat",
        "message": "alive",
        "schedule": {"action": "create", "interval": 1, "count": 3},
    })
    assert result["status"] == "scheduled"
    assert "schedule_id" in result
    assert result["interval"] == 1
    assert result["count"] == 3
    # schedule.json should exist on disk
    sched_dir = agent.working_dir / "mailbox" / "schedules" / result["schedule_id"]
    assert (sched_dir / "schedule.json").is_file()
    sched = json.loads((sched_dir / "schedule.json").read_text())
    assert sched["count"] == 3
    assert sched["sent"] == 0


def test_email_schedule_create_sends_messages(tmp_path):
    """schedule.create should send count messages with interval between them."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "address": "someone",
        "subject": "Beat",
        "message": "ping",
        "schedule": {"action": "create", "interval": 1, "count": 3},
    })
    sid = result["schedule_id"]
    # Wait for all 3 sends (3 sends * 1s interval + buffer)
    time.sleep(4.0)
    # Should have sent 3 times
    sched = json.loads((agent.working_dir / "mailbox" / "schedules" / sid / "schedule.json").read_text())
    assert sched["sent"] == 3
    # Sent folder should have 3 records
    sent_dir = agent.working_dir / "mailbox" / "sent"
    assert len(list(sent_dir.iterdir())) == 3


def test_email_schedule_create_includes_metadata(tmp_path):
    """Each scheduled send should include _schedule metadata."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "address": "someone",
        "message": "beat",
        "schedule": {"action": "create", "interval": 1, "count": 2},
    })
    time.sleep(3.0)
    # Check sent records for _schedule metadata
    sent_dir = agent.working_dir / "mailbox" / "sent"
    sent_msgs = []
    for d in sent_dir.iterdir():
        msg = json.loads((d / "message.json").read_text())
        sent_msgs.append(msg)
    # Sort by seq
    sent_msgs.sort(key=lambda m: m.get("_schedule", {}).get("seq", 0))
    assert len(sent_msgs) == 2
    assert sent_msgs[0]["_schedule"]["seq"] == 1
    assert sent_msgs[0]["_schedule"]["total"] == 2
    assert sent_msgs[1]["_schedule"]["seq"] == 2
    assert "estimated_finish" in sent_msgs[1]["_schedule"]
    assert "schedule_id" in sent_msgs[0]["_schedule"]


def test_email_schedule_create_missing_params(tmp_path):
    """schedule.create without interval or count should error."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "address": "someone", "message": "hi",
        "schedule": {"action": "create", "count": 3},
    })
    assert "error" in result
    result = mgr.handle({
        "address": "someone", "message": "hi",
        "schedule": {"action": "create", "interval": 10},
    })
    assert "error" in result


def test_email_schedule_create_invalid_params(tmp_path):
    """schedule.create with non-positive interval or count should error."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "address": "someone", "message": "hi",
        "schedule": {"action": "create", "interval": 0, "count": 3},
    })
    assert "error" in result
    result = mgr.handle({
        "address": "someone", "message": "hi",
        "schedule": {"action": "create", "interval": 10, "count": -1},
    })
    assert "error" in result


def test_email_schedule_create_missing_address(tmp_path):
    """schedule.create without address should error."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "message": "hi",
        "schedule": {"action": "create", "interval": 10, "count": 3},
    })
    assert "error" in result


# ---------------------------------------------------------------------------
# Schedule — cancel
# ---------------------------------------------------------------------------

def test_email_schedule_cancel(tmp_path):
    """cancel should stop a running schedule."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "address": "someone",
        "message": "beat",
        "schedule": {"action": "create", "interval": 60, "count": 100},
    })
    sid = result["schedule_id"]
    time.sleep(0.5)  # let first send go through

    cancel_result = mgr.handle({"schedule": {"action": "cancel", "schedule_id": sid}})
    assert cancel_result["status"] == "cancelled"

    # Verify sentinel file on disk
    assert (agent.working_dir / "mailbox" / "schedules" / sid / ".cancel").is_file()
    # Should NOT have sent all 100
    sched = json.loads(
        (agent.working_dir / "mailbox" / "schedules" / sid / "schedule.json").read_text()
    )
    assert sched["sent"] < 100


def test_email_schedule_cancel_not_found(tmp_path):
    """cancel on non-existent schedule should error."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"schedule": {"action": "cancel", "schedule_id": "nonexistent"}})
    assert "error" in result


def test_email_schedule_cancel_missing_id(tmp_path):
    """cancel without schedule_id should cancel ALL schedules (agent-level)."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"schedule": {"action": "cancel"}})
    assert result["status"] == "cancelled"


def test_email_schedule_cancel_already_stopped(tmp_path):
    """cancel on completed or already-cancelled schedule should return already_stopped."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    # Create a short schedule and let it complete
    result = mgr.handle({
        "address": "someone",
        "message": "beat",
        "schedule": {"action": "create", "interval": 1, "count": 1},
    })
    sid = result["schedule_id"]
    time.sleep(2.0)
    # Cancel after completion
    cancel_result = mgr.handle({"schedule": {"action": "cancel", "schedule_id": sid}})
    assert cancel_result["status"] == "already_stopped"


# ---------------------------------------------------------------------------
# Schedule — list
# ---------------------------------------------------------------------------

def test_email_schedule_list_empty(tmp_path):
    """list with no schedules should return empty list."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"schedule": {"action": "list"}})
    assert result["status"] == "ok"
    assert result["schedules"] == []


def test_email_schedule_list_shows_active(tmp_path):
    """list should show active schedules with progress."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "address": "someone",
        "subject": "Status",
        "message": "ok",
        "schedule": {"action": "create", "interval": 60, "count": 10},
    })
    sid = result["schedule_id"]
    time.sleep(0.5)  # let first send happen
    listing = mgr.handle({"schedule": {"action": "list"}})
    assert listing["status"] == "ok"
    assert len(listing["schedules"]) == 1
    entry = listing["schedules"][0]
    assert entry["schedule_id"] == sid
    assert entry["interval"] == 60
    assert entry["count"] == 10
    assert entry["to"] == "someone"
    assert entry["subject"] == "Status"
    assert entry["active"] is True
    # Cleanup
    mgr.handle({"schedule": {"action": "cancel", "schedule_id": sid}})


def test_email_schedule_list_shows_completed(tmp_path):
    """list should show completed schedules with active=False."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "address": "someone",
        "message": "done",
        "schedule": {"action": "create", "interval": 1, "count": 1},
    })
    time.sleep(2.0)
    listing = mgr.handle({"schedule": {"action": "list"}})
    entry = listing["schedules"][0]
    assert entry["active"] is False
    assert entry["sent"] == 1


# ---------------------------------------------------------------------------
# Schedule — recovery
# ---------------------------------------------------------------------------

def test_email_schedule_recovery_on_setup(tmp_path):
    """Incomplete schedules should resume when a new EmailManager is created."""
    agent1 = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                        capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent1._mail_service = mail_svc

    # Manually write a schedule.json that looks like it was interrupted at sent=1 of count=3
    sched_id = "recover12345"
    sched_dir = agent1.working_dir / "mailbox" / "schedules" / sched_id
    sched_dir.mkdir(parents=True, exist_ok=True)
    record = {
        "schedule_id": sched_id,
        "send_payload": {
            "address": "someone",
            "subject": "Resume",
            "message": "continued",
            "cc": [],
            "bcc": [],
            "type": "normal",
        },
        "interval": 1,
        "count": 3,
        "sent": 1,
        "created_at": "2026-03-18T10:00:00Z",
        "last_sent_at": "2026-03-18T10:00:00Z",
    }
    (sched_dir / "schedule.json").write_text(json.dumps(record, indent=2))

    # Release the first agent's lock so the second agent can use the same directory
    agent1.stop(timeout=1.0)

    # Create a NEW agent at the same base_dir — setup() should auto-recover
    agent2 = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                        mail_service=mail_svc, capabilities=["email"])
    # Recovery happens automatically in setup() — no manual call needed

    # Wait for remaining 2 sends
    time.sleep(3.0)
    final = json.loads((sched_dir / "schedule.json").read_text())
    assert final["sent"] == 3


def test_email_schedule_recovery_skips_cancelled(tmp_path):
    """Cancelled schedules should not be resumed."""
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None

    # First agent creates the schedule dir, then releases lock
    agent1 = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])

    sched_id = "cancelled1234"
    sched_dir = agent1.working_dir / "mailbox" / "schedules" / sched_id
    sched_dir.mkdir(parents=True, exist_ok=True)
    record = {
        "schedule_id": sched_id,
        "send_payload": {"address": "someone", "message": "x", "subject": "", "cc": [], "bcc": [], "type": "normal"},
        "interval": 1, "count": 5, "sent": 2,
        "created_at": "2026-03-18T10:00:00Z",
        "last_sent_at": "2026-03-18T10:00:00Z",
    }
    (sched_dir / "schedule.json").write_text(json.dumps(record, indent=2))
    (sched_dir / ".cancel").touch()
    agent1.stop(timeout=1.0)

    # Second agent picks up the same working dir — recovery should skip cancelled
    agent2 = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       mail_service=mail_svc, capabilities=["email"])

    time.sleep(2.0)

    # Should NOT have resumed — sent should still be 2
    final = json.loads((sched_dir / "schedule.json").read_text())
    assert final["sent"] == 2


# ---------------------------------------------------------------------------
# Schedule — end-to-end
# ---------------------------------------------------------------------------

def test_email_schedule_end_to_end(tmp_path):
    """Full lifecycle: create → sends happen → list shows progress → cancel → list shows cancelled."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")

    # Create
    result = mgr.handle({
        "address": "peer",
        "subject": "Status",
        "message": "System OK",
        "schedule": {"action": "create", "interval": 1, "count": 5},
    })
    assert result["status"] == "scheduled"
    sid = result["schedule_id"]

    # Let 2 sends happen
    time.sleep(2.5)

    # List — should be active with some progress
    listing = mgr.handle({"schedule": {"action": "list"}})
    entry = [s for s in listing["schedules"] if s["schedule_id"] == sid][0]
    assert entry["active"] is True
    assert entry["sent"] >= 2

    # Cancel
    cancel = mgr.handle({"schedule": {"action": "cancel", "schedule_id": sid}})
    assert cancel["status"] == "cancelled"

    # List — should show cancelled
    listing = mgr.handle({"schedule": {"action": "list"}})
    entry = [s for s in listing["schedules"] if s["schedule_id"] == sid][0]
    assert entry["active"] is False
    assert entry["cancelled"] is True
    assert entry["sent"] < 5


def test_email_private_mode_receive_unrestricted(tmp_path):
    """Private mode should not block receiving emails."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities={"email": {"private_mode": True}})
    agent._on_mail_received({
        "_mailbox_id": "abc",
        "from": "stranger",
        "to": ["me"],
        "subject": "hi",
        "message": "can you hear me",
    })
    assert not agent.inbox.empty()


# ---------------------------------------------------------------------------
# Disk-driven scheduler service
# ---------------------------------------------------------------------------

def test_scheduler_service_sends_due_messages(tmp_path):
    """The scheduler service thread should send messages when due."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")

    result = mgr.handle({
        "address": "someone",
        "subject": "ping",
        "message": "hello",
        "schedule": {"action": "create", "interval": 1, "count": 3},
    })
    sid = result["schedule_id"]
    time.sleep(4.0)

    sched = json.loads(
        (agent.working_dir / "mailbox" / "schedules" / sid / "schedule.json").read_text()
    )
    assert sched["sent"] == 3


def test_scheduler_cancel_via_sentinel_file(tmp_path):
    """Touching .cancel in schedule folder should stop delivery."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")

    result = mgr.handle({
        "address": "someone",
        "message": "beat",
        "schedule": {"action": "create", "interval": 60, "count": 100},
    })
    sid = result["schedule_id"]
    time.sleep(1.5)  # let first send go

    (agent.working_dir / "mailbox" / "schedules" / sid / ".cancel").touch()
    time.sleep(2.0)

    sched = json.loads(
        (agent.working_dir / "mailbox" / "schedules" / sid / "schedule.json").read_text()
    )
    assert sched["sent"] < 100


def test_scheduler_agent_level_cancel(tmp_path):
    """Touching .cancel in schedules/ dir should stop ALL schedules."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")

    r1 = mgr.handle({
        "address": "a", "message": "x",
        "schedule": {"action": "create", "interval": 60, "count": 50},
    })
    r2 = mgr.handle({
        "address": "b", "message": "y",
        "schedule": {"action": "create", "interval": 60, "count": 50},
    })
    time.sleep(1.5)

    (agent.working_dir / "mailbox" / "schedules" / ".cancel").touch()
    time.sleep(2.0)

    for sid in [r1["schedule_id"], r2["schedule_id"]]:
        sched = json.loads(
            (agent.working_dir / "mailbox" / "schedules" / sid / "schedule.json").read_text()
        )
        assert sched["sent"] < 50


def test_scheduler_respects_interval_on_resume(tmp_path):
    """On resume, scheduler should wait remaining interval, not send immediately."""
    from datetime import timedelta

    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc

    sched_id = "resume-test"
    sched_dir = agent.working_dir / "mailbox" / "schedules" / sched_id
    sched_dir.mkdir(parents=True, exist_ok=True)
    now = datetime.now(timezone.utc)
    record = {
        "schedule_id": sched_id,
        "send_payload": {
            "address": "someone", "subject": "Resume", "message": "continued",
            "cc": [], "bcc": [], "type": "normal",
        },
        "interval": 5,
        "count": 3,
        "sent": 1,
        "created_at": "2026-03-18T10:00:00Z",
        "last_sent_at": (now - timedelta(seconds=1)).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    (sched_dir / "schedule.json").write_text(json.dumps(record, indent=2))

    mgr = agent.get_capability("email")
    time.sleep(2.0)

    sched = json.loads((sched_dir / "schedule.json").read_text())
    assert sched["sent"] == 1  # should not have sent yet — 4s remaining


def test_schedule_cancel_action_creates_sentinel(tmp_path):
    """schedule.cancel action should create .cancel file."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")

    result = mgr.handle({
        "address": "someone", "message": "beat",
        "schedule": {"action": "create", "interval": 60, "count": 100},
    })
    sid = result["schedule_id"]
    time.sleep(1.5)

    cancel_result = mgr.handle({"schedule": {"action": "cancel", "schedule_id": sid}})
    assert cancel_result["status"] == "cancelled"
    assert (agent.working_dir / "mailbox" / "schedules" / sid / ".cancel").is_file()


def test_schedule_cancel_all_creates_agent_sentinel(tmp_path):
    """schedule.cancel without schedule_id should create agent-level .cancel."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mgr = agent.get_capability("email")

    result = mgr.handle({"schedule": {"action": "cancel"}})
    assert result["status"] == "cancelled"
    assert (agent.working_dir / "mailbox" / "schedules" / ".cancel").is_file()


def test_schedule_sends_inbox_notification(tmp_path):
    """After a scheduled send fires, agent should get an inbox notification."""
    agent = Agent(service=make_mock_service(), agent_name="test", working_dir=tmp_path / "test",
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")

    result = mgr.handle({
        "address": "someone",
        "subject": "alarm",
        "message": "wake up",
        "schedule": {"action": "create", "interval": 1, "count": 2},
    })
    assert result["status"] == "scheduled"

    # Wait for both sends to fire
    time.sleep(3.5)

    # Drain inbox for schedule notifications
    notifications = []
    while not agent.inbox.empty():
        msg = agent.inbox.get_nowait()
        if hasattr(msg, "content") and "[schedule" in str(msg.content):
            notifications.append(str(msg.content))

    assert len(notifications) >= 1, f"Expected schedule notifications, got {notifications}"
    assert "[schedule 1/2]" in notifications[0]
    assert "alarm" in notifications[0]
    # Last notification should say "schedule complete"
    assert "schedule complete" in notifications[-1]
