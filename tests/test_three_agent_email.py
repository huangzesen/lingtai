"""Three-agent email integration test.

Tests end-to-end email flows between three agents (Alice, Bob, Charlie)
using real FilesystemMailService instances and the email capability.

Scenarios:
1. Alice sends to Bob and Charlie (multi-to)
2. Bob replies to Alice
3. Charlie reply-alls (reaches Alice + Bob)
4. Alice sends to Bob with Charlie on CC
"""
from __future__ import annotations

import json
import shutil
import tempfile
import threading
import time
from pathlib import Path
from unittest.mock import MagicMock

from lingtai.agent import Agent
from lingtai_kernel.config import AgentConfig
from lingtai_kernel.services.mail import FilesystemMailService


def _make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def _setup_agent_dir(path: Path) -> None:
    """Create a minimal agent directory with manifest and heartbeat."""
    path.mkdir(parents=True, exist_ok=True)
    (path / ".agent.json").write_text(json.dumps({
        "agent_id": path.name,
        "agent_name": path.name,
    }))
    (path / ".agent.heartbeat").write_text(str(time.time()))


def _make_agent(name: str, base_dir: Path):
    """Create an agent with a real FilesystemMailService and email capability."""
    agent = Agent(
        service=_make_mock_service(),
        agent_name=name,
        working_dir=base_dir / name,
        capabilities=["email"],
    )
    # Wire up mail service after construction (needs working_dir from agent)
    mail_svc = FilesystemMailService(working_dir=agent.working_dir)
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    return agent, mgr


def _inbox_count(working_dir: Path) -> int:
    """Count emails in the inbox directory."""
    inbox = working_dir / "mailbox" / "inbox"
    if not inbox.is_dir():
        return 0
    return sum(1 for d in inbox.iterdir() if d.is_dir() and (d / "message.json").is_file())


def _inbox_emails(working_dir: Path) -> list[dict]:
    """Load all inbox emails sorted by received_at."""
    inbox = working_dir / "mailbox" / "inbox"
    if not inbox.is_dir():
        return []
    emails = []
    for d in inbox.iterdir():
        msg_file = d / "message.json"
        if d.is_dir() and msg_file.is_file():
            data = json.loads(msg_file.read_text())
            data.setdefault("_mailbox_id", d.name)
            emails.append(data)
    emails.sort(key=lambda e: e.get("received_at", ""))
    return emails


class TestThreeAgentEmail:
    """Integration tests for email flows between three agents."""

    def setup_method(self):
        """Set up three agents with real filesystem mail services."""
        self.base_dir = Path(tempfile.mkdtemp())
        self.agents = {}
        self.managers = {}
        self._heartbeat_stop = threading.Event()

        for name in ("alice", "bob", "charlie"):
            agent, mgr = _make_agent(name, self.base_dir)
            self.agents[name] = agent
            self.managers[name] = mgr

        # Keep heartbeats alive for all agents
        def _heartbeat_loop():
            while not self._heartbeat_stop.is_set():
                for agent in self.agents.values():
                    try:
                        hb = agent.working_dir / ".agent.heartbeat"
                        hb.write_text(str(time.time()))
                    except OSError:
                        pass
                self._heartbeat_stop.wait(0.5)
        self._hb_thread = threading.Thread(target=_heartbeat_loop, daemon=True)
        self._hb_thread.start()

        # Start listening on all agents
        for name, agent in self.agents.items():
            agent._mail_service.listen(
                on_message=lambda msg, a=agent: a._on_mail_received(msg)
            )

    def teardown_method(self):
        """Stop all mail services and clean up temp dirs."""
        self._heartbeat_stop.set()
        for agent in self.agents.values():
            agent._mail_service.stop()
        shutil.rmtree(self.base_dir, ignore_errors=True)

    def _addr(self, name: str) -> str:
        return str(self.agents[name].working_dir)

    def _dir(self, name: str) -> Path:
        return self.agents[name].working_dir

    def _wait_for_inbox(self, name: str, count: int, timeout: float = 5.0):
        """Wait until the agent's inbox has at least `count` messages."""
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if _inbox_count(self._dir(name)) >= count:
                return True
            time.sleep(0.05)
        return False

    # -------------------------------------------------------------------
    # Test 1: Alice sends to Bob and Charlie
    # -------------------------------------------------------------------

    def test_alice_sends_to_bob_and_charlie(self):
        """Alice sends a single email to both Bob and Charlie."""
        result = self.managers["alice"].handle({
            "action": "send",
            "address": [self._addr("bob"), self._addr("charlie")],
            "subject": "Team update",
            "message": "Hello team, here is the update.",
        })
        assert result["status"] == "sent"

        assert self._wait_for_inbox("bob", 1), "Bob did not receive the email"
        assert self._wait_for_inbox("charlie", 1), "Charlie did not receive the email"

        # Verify content
        bob_mail = _inbox_emails(self._dir("bob"))[0]
        charlie_mail = _inbox_emails(self._dir("charlie"))[0]
        assert bob_mail["message"] == "Hello team, here is the update."
        assert charlie_mail["subject"] == "Team update"
        assert bob_mail["from"] == self._addr("alice")
        assert charlie_mail["from"] == self._addr("alice")

    # -------------------------------------------------------------------
    # Test 2: Bob replies to Alice
    # -------------------------------------------------------------------

    def test_bob_replies_to_alice(self):
        """Alice emails Bob, then Bob replies back to Alice."""
        # Alice -> Bob
        self.managers["alice"].handle({
            "action": "send",
            "address": self._addr("bob"),
            "subject": "Question",
            "message": "What is the status?",
        })
        assert self._wait_for_inbox("bob", 1)

        # Bob reads and replies
        bob_email_id = _inbox_emails(self._dir("bob"))[0]["_mailbox_id"]
        result = self.managers["bob"].handle({
            "action": "reply",
            "email_id": bob_email_id,
            "message": "Everything is on track.",
        })
        assert result["status"] == "sent"

        # Alice should receive Bob's reply
        assert self._wait_for_inbox("alice", 1), "Alice did not receive Bob's reply"
        alice_mail = _inbox_emails(self._dir("alice"))[0]
        assert alice_mail["message"] == "Everything is on track."
        assert alice_mail["subject"] == "Re: Question"
        assert alice_mail["from"] == self._addr("bob")

    # -------------------------------------------------------------------
    # Test 3: Charlie reply-alls (reaches Alice + Bob)
    # -------------------------------------------------------------------

    def test_charlie_reply_all(self):
        """Alice sends to Bob and Charlie, Charlie reply-alls.

        Per-recipient dispatch means each recipient sees only their own
        address in the ``to`` field, so reply_all resolves to the original
        sender (Alice) only -- Bob is not included.
        """
        # Alice -> Bob + Charlie
        self.managers["alice"].handle({
            "action": "send",
            "address": [self._addr("bob"), self._addr("charlie")],
            "subject": "Group discussion",
            "message": "Let's plan the sprint.",
        })
        assert self._wait_for_inbox("charlie", 1)

        # Charlie reply-alls -- goes to Alice (reply-to) and Bob (CC from original)
        charlie_email_id = _inbox_emails(self._dir("charlie"))[0]["_mailbox_id"]
        result = self.managers["charlie"].handle({
            "action": "reply_all",
            "email_id": charlie_email_id,
            "message": "I have some ideas to share.",
        })
        assert result["status"] == "sent"

        # Alice should receive Charlie's reply
        assert self._wait_for_inbox("alice", 1), "Alice did not receive Charlie's reply_all"

        # Verify Alice got the reply
        alice_reply = _inbox_emails(self._dir("alice"))[0]
        assert alice_reply["message"] == "I have some ideas to share."
        assert alice_reply["from"] == self._addr("charlie")
        assert alice_reply["subject"] == "Re: Group discussion"

        # Bob should receive both the original from Alice and Charlie's reply_all
        assert self._wait_for_inbox("bob", 2), "Bob did not receive Charlie's reply_all"
        bob_emails = _inbox_emails(self._dir("bob"))
        assert len(bob_emails) == 2

    # -------------------------------------------------------------------
    # Test 4: Alice sends to Bob with Charlie on CC
    # -------------------------------------------------------------------

    def test_alice_sends_to_bob_with_charlie_cc(self):
        """Alice sends to Bob with Charlie on CC -- both receive, CC field visible."""
        result = self.managers["alice"].handle({
            "action": "send",
            "address": self._addr("bob"),
            "subject": "FYI",
            "message": "Bob, please review. Charlie for visibility.",
            "cc": [self._addr("charlie")],
        })
        assert result["status"] == "sent"

        assert self._wait_for_inbox("bob", 1), "Bob did not receive the email"
        assert self._wait_for_inbox("charlie", 1), "Charlie did not receive the CC"

        # Both should see the CC field
        bob_mail = _inbox_emails(self._dir("bob"))[0]
        charlie_mail = _inbox_emails(self._dir("charlie"))[0]
        assert bob_mail["cc"] == [self._addr("charlie")]
        assert charlie_mail["cc"] == [self._addr("charlie")]
        assert bob_mail["message"] == "Bob, please review. Charlie for visibility."
        assert charlie_mail["message"] == "Bob, please review. Charlie for visibility."

    # -------------------------------------------------------------------
    # Test 5: Full conversation flow
    # -------------------------------------------------------------------

    def test_full_conversation_flow(self):
        """End-to-end: Alice starts thread, Bob replies, Charlie reply-alls."""
        # Step 1: Alice -> Bob + Charlie
        self.managers["alice"].handle({
            "action": "send",
            "address": [self._addr("bob"), self._addr("charlie")],
            "subject": "Project kickoff",
            "message": "Welcome to the project!",
        })
        assert self._wait_for_inbox("bob", 1)
        assert self._wait_for_inbox("charlie", 1)

        # Step 2: Bob replies to Alice only
        bob_email_id = _inbox_emails(self._dir("bob"))[0]["_mailbox_id"]
        self.managers["bob"].handle({
            "action": "reply",
            "email_id": bob_email_id,
            "message": "Thanks, excited to start!",
        })
        assert self._wait_for_inbox("alice", 1)
        assert _inbox_emails(self._dir("alice"))[0]["subject"] == "Re: Project kickoff"

        # Step 3: Charlie reply-alls -- goes to Alice + Bob
        charlie_email_id = _inbox_emails(self._dir("charlie"))[0]["_mailbox_id"]
        self.managers["charlie"].handle({
            "action": "reply_all",
            "email_id": charlie_email_id,
            "message": "Looking forward to it!",
        })
        # Alice gets Charlie's reply (2nd message)
        assert self._wait_for_inbox("alice", 2), "Alice didn't get Charlie's reply_all"
        # Bob gets Charlie's reply_all (2nd message)
        assert self._wait_for_inbox("bob", 2), "Bob didn't get Charlie's reply_all"

        # Verify final mailbox states
        assert _inbox_count(self._dir("alice")) == 2  # Bob's reply + Charlie's reply_all
        assert _inbox_count(self._dir("bob")) == 2    # Alice's original + Charlie's reply_all
        assert _inbox_count(self._dir("charlie")) == 1  # Alice's original only
