"""Tests for lingtai.network — host-level network topology builder."""

from __future__ import annotations

import json
import time
from pathlib import Path

import pytest

from lingtai.network import (
    AgentNetwork,
    AgentNode,
    AvatarEdge,
    ContactEdge,
    MailEdge,
    MailRecord,
    build_network,
)


# ---------------------------------------------------------------------------
# Helpers — create filesystem structures
# ---------------------------------------------------------------------------

def _write_manifest(agent_dir: Path, agent_id: str, agent_name: str,
                    address: str | None = None) -> None:
    agent_dir.mkdir(parents=True, exist_ok=True)
    manifest = {"agent_id": agent_id, "agent_name": agent_name}
    if address:
        manifest["address"] = address
    (agent_dir / ".agent.json").write_text(json.dumps(manifest))


def _write_ledger(agent_dir: Path, entries: list[dict]) -> None:
    ledger_dir = agent_dir / "delegates"
    ledger_dir.mkdir(parents=True, exist_ok=True)
    lines = [json.dumps(e) for e in entries]
    (ledger_dir / "ledger.jsonl").write_text("\n".join(lines) + "\n")


def _write_contacts(agent_dir: Path, contacts: list[dict]) -> None:
    mailbox = agent_dir / "mailbox"
    mailbox.mkdir(parents=True, exist_ok=True)
    (mailbox / "contacts.json").write_text(json.dumps(contacts))


def _write_mail(agent_dir: Path, folder: str, messages: list[dict]) -> None:
    """Write messages to mailbox/inbox/ or mailbox/sent/."""
    folder_dir = agent_dir / "mailbox" / folder
    folder_dir.mkdir(parents=True, exist_ok=True)
    for i, msg in enumerate(messages):
        msg_dir = folder_dir / f"msg-{i:04d}"
        msg_dir.mkdir(parents=True, exist_ok=True)
        (msg_dir / "message.json").write_text(json.dumps(msg))


# ---------------------------------------------------------------------------
# Tests — node discovery
# ---------------------------------------------------------------------------

def test_discover_agents(tmp_path):
    _write_manifest(tmp_path / "alice", "aaa111", "alice", "127.0.0.1:8001")
    _write_manifest(tmp_path / "bob", "bbb222", "bob")

    net = build_network(tmp_path)

    assert len(net.nodes) == 2
    assert "aaa111" in net.nodes
    assert net.nodes["aaa111"].agent_name == "alice"
    assert net.nodes["aaa111"].address == "127.0.0.1:8001"
    assert net.nodes["bbb222"].address is None


def test_empty_base_dir(tmp_path):
    net = build_network(tmp_path)
    assert len(net.nodes) == 0
    assert net.avatar_edges == []
    assert net.contact_edges == []
    assert net.mail_edges == []


def test_nonexistent_base_dir(tmp_path):
    net = build_network(tmp_path / "does_not_exist")
    assert len(net.nodes) == 0


def test_skip_bad_manifest(tmp_path):
    # Directory with no manifest
    (tmp_path / "no_manifest").mkdir()
    # Directory with invalid JSON
    bad = tmp_path / "bad_json"
    bad.mkdir()
    (bad / ".agent.json").write_text("{invalid}")
    # Directory with manifest missing agent_id
    no_id = tmp_path / "no_id"
    no_id.mkdir()
    (no_id / ".agent.json").write_text(json.dumps({"agent_name": "orphan"}))

    net = build_network(tmp_path)
    assert len(net.nodes) == 0


# ---------------------------------------------------------------------------
# Tests — avatar edges
# ---------------------------------------------------------------------------

def test_avatar_edges(tmp_path):
    _write_manifest(tmp_path / "parent", "ppp111", "parent")
    _write_manifest(tmp_path / "child", "ccc111", "child")
    _write_ledger(tmp_path / "parent", [
        {
            "ts": 1710000000.0,
            "event": "avatar",
            "name": "child",
            "agent_id": "ccc111",
            "address": "127.0.0.1:9001",
            "mission": "do research",
            "capabilities": ["file", "web_search"],
            "provider": "anthropic",
            "model": "claude-3",
        },
    ])

    net = build_network(tmp_path)

    assert len(net.avatar_edges) == 1
    edge = net.avatar_edges[0]
    assert edge.parent_id == "ppp111"
    assert edge.child_id == "ccc111"
    assert edge.child_name == "child"
    assert edge.mission == "do research"
    assert edge.capabilities == ["file", "web_search"]
    assert edge.provider == "anthropic"


def test_avatar_dead_child_added_to_nodes(tmp_path):
    """A child referenced in ledger but without its own manifest should be added."""
    _write_manifest(tmp_path / "parent", "ppp111", "parent")
    _write_ledger(tmp_path / "parent", [
        {
            "ts": 1710000000.0,
            "event": "avatar",
            "name": "ghost",
            "agent_id": "ggg111",
            "address": "127.0.0.1:9999",
        },
    ])

    net = build_network(tmp_path)

    assert "ggg111" in net.nodes
    assert net.nodes["ggg111"].agent_name == "ghost"
    assert net.nodes["ggg111"].working_dir is None  # no manifest dir


def test_avatar_reactivate_event_skipped(tmp_path):
    """Only 'avatar' events create edges, not 'reactivate'."""
    _write_manifest(tmp_path / "parent", "ppp111", "parent")
    _write_ledger(tmp_path / "parent", [
        {"ts": 1.0, "event": "avatar", "name": "c1", "agent_id": "c11"},
        {"ts": 2.0, "event": "reactivate", "name": "c1", "agent_id": "c11"},
    ])

    net = build_network(tmp_path)
    assert len(net.avatar_edges) == 1


def test_children_of(tmp_path):
    _write_manifest(tmp_path / "parent", "ppp111", "parent")
    _write_manifest(tmp_path / "c1", "c11", "c1")
    _write_manifest(tmp_path / "c2", "c22", "c2")
    _write_ledger(tmp_path / "parent", [
        {"ts": 1.0, "event": "avatar", "name": "c1", "agent_id": "c11"},
        {"ts": 2.0, "event": "avatar", "name": "c2", "agent_id": "c22"},
    ])

    net = build_network(tmp_path)
    children = net.children_of("ppp111")
    assert len(children) == 2
    assert {c.agent_id for c in children} == {"c11", "c22"}


# ---------------------------------------------------------------------------
# Tests — contact edges
# ---------------------------------------------------------------------------

def test_contact_edges(tmp_path):
    _write_manifest(tmp_path / "alice", "aaa111", "alice")
    _write_contacts(tmp_path / "alice", [
        {"address": "127.0.0.1:9001", "name": "Bob", "note": "researcher"},
        {"address": "127.0.0.1:9002", "name": "Carol", "note": ""},
    ])

    net = build_network(tmp_path)

    assert len(net.contact_edges) == 2
    bob_edge = [e for e in net.contact_edges if e.target_name == "Bob"][0]
    assert bob_edge.owner_id == "aaa111"
    assert bob_edge.target_address == "127.0.0.1:9001"
    assert bob_edge.note == "researcher"


def test_contacts_of(tmp_path):
    _write_manifest(tmp_path / "alice", "aaa111", "alice")
    _write_manifest(tmp_path / "bob", "bbb222", "bob")
    _write_contacts(tmp_path / "alice", [
        {"address": "127.0.0.1:9001", "name": "Bob", "note": ""},
    ])
    _write_contacts(tmp_path / "bob", [
        {"address": "127.0.0.1:8001", "name": "Alice", "note": ""},
    ])

    net = build_network(tmp_path)
    alice_contacts = net.contacts_of("aaa111")
    assert len(alice_contacts) == 1
    assert alice_contacts[0].target_name == "Bob"


def test_no_contacts_file(tmp_path):
    _write_manifest(tmp_path / "alice", "aaa111", "alice")
    net = build_network(tmp_path)
    assert net.contact_edges == []


# ---------------------------------------------------------------------------
# Tests — mail edges
# ---------------------------------------------------------------------------

def test_mail_edges_from_sent(tmp_path):
    _write_manifest(tmp_path / "alice", "aaa111", "alice", "127.0.0.1:8001")
    _write_mail(tmp_path / "alice", "sent", [
        {
            "from": "127.0.0.1:8001",
            "to": ["127.0.0.1:9001"],
            "subject": "hello",
            "sent_at": "2026-03-20T10:00:00Z",
            "type": "normal",
        },
        {
            "from": "127.0.0.1:8001",
            "to": ["127.0.0.1:9001"],
            "subject": "follow up",
            "sent_at": "2026-03-20T11:00:00Z",
            "type": "normal",
        },
    ])

    net = build_network(tmp_path)

    assert len(net.mail_edges) == 1
    edge = net.mail_edges[0]
    assert edge.sender == "127.0.0.1:8001"
    assert edge.recipient == "127.0.0.1:9001"
    assert edge.count == 2
    assert edge.last_at == "2026-03-20T11:00:00Z"
    assert "hello" in edge.subjects
    assert "follow up" in edge.subjects


def test_mail_edges_from_inbox(tmp_path):
    _write_manifest(tmp_path / "bob", "bbb222", "bob", "127.0.0.1:9001")
    _write_mail(tmp_path / "bob", "inbox", [
        {
            "from": "127.0.0.1:8001",
            "to": ["127.0.0.1:9001"],
            "subject": "hello",
            "received_at": "2026-03-20T10:00:00Z",
            "type": "normal",
        },
    ])

    net = build_network(tmp_path)

    assert len(net.mail_edges) == 1
    edge = net.mail_edges[0]
    assert edge.sender == "127.0.0.1:8001"
    assert edge.recipient == "127.0.0.1:9001"
    assert edge.count == 1


def test_mail_edges_with_cc(tmp_path):
    _write_manifest(tmp_path / "alice", "aaa111", "alice", "127.0.0.1:8001")
    _write_mail(tmp_path / "alice", "sent", [
        {
            "from": "127.0.0.1:8001",
            "to": ["127.0.0.1:9001"],
            "cc": ["127.0.0.1:9002"],
            "subject": "team update",
            "sent_at": "2026-03-20T10:00:00Z",
            "type": "normal",
        },
    ])

    net = build_network(tmp_path)

    # Should create two edges: to 9001 and cc 9002
    assert len(net.mail_edges) == 2
    recipients = {e.recipient for e in net.mail_edges}
    assert recipients == {"127.0.0.1:9001", "127.0.0.1:9002"}


def test_mail_deduplication_across_inbox_and_sent(tmp_path):
    """Same message in sender's sent/ and recipient's inbox/ counts as 2 records.

    This is correct — each folder is a separate view. The mail_edge aggregates
    by (sender, recipient) key so the count reflects total observations.
    """
    _write_manifest(tmp_path / "alice", "aaa111", "alice", "127.0.0.1:8001")
    _write_manifest(tmp_path / "bob", "bbb222", "bob", "127.0.0.1:9001")
    _write_mail(tmp_path / "alice", "sent", [
        {
            "from": "127.0.0.1:8001",
            "to": ["127.0.0.1:9001"],
            "subject": "hello",
            "sent_at": "2026-03-20T10:00:00Z",
        },
    ])
    _write_mail(tmp_path / "bob", "inbox", [
        {
            "from": "127.0.0.1:8001",
            "to": ["127.0.0.1:9001"],
            "subject": "hello",
            "received_at": "2026-03-20T10:00:00Z",
        },
    ])

    net = build_network(tmp_path)

    edge = net.mail_edges[0]
    assert edge.count == 2  # observed in both sent and inbox


def test_mail_of(tmp_path):
    _write_manifest(tmp_path / "alice", "aaa111", "alice", "127.0.0.1:8001")
    _write_manifest(tmp_path / "bob", "bbb222", "bob", "127.0.0.1:9001")
    _write_mail(tmp_path / "alice", "sent", [
        {
            "from": "127.0.0.1:8001",
            "to": ["127.0.0.1:9001"],
            "subject": "hello",
            "sent_at": "2026-03-20T10:00:00Z",
        },
    ])

    net = build_network(tmp_path)

    # mail_of matches by agent_id or address
    alice_mail = net.mail_of("aaa111")
    assert len(alice_mail) == 1
    assert alice_mail[0].sender == "127.0.0.1:8001"


def test_mail_of_unknown_agent(tmp_path):
    net = build_network(tmp_path)
    assert net.mail_of("unknown") == []


# ---------------------------------------------------------------------------
# Tests — full integration
# ---------------------------------------------------------------------------

def test_full_network(tmp_path):
    """Build a complete network with all three layers."""
    # Set up agents
    _write_manifest(tmp_path / "boss", "boss01", "boss", "127.0.0.1:8000")
    _write_manifest(tmp_path / "worker", "work01", "worker", "127.0.0.1:8001")

    # Boss spawned worker
    _write_ledger(tmp_path / "boss", [
        {
            "ts": 1710000000.0,
            "event": "avatar",
            "name": "worker",
            "agent_id": "work01",
            "address": "127.0.0.1:8001",
            "mission": "process data",
            "capabilities": ["file"],
        },
    ])

    # Mutual contacts
    _write_contacts(tmp_path / "boss", [
        {"address": "127.0.0.1:8001", "name": "Worker", "note": "data processor"},
    ])
    _write_contacts(tmp_path / "worker", [
        {"address": "127.0.0.1:8000", "name": "Boss", "note": "supervisor"},
    ])

    # Mail exchange
    _write_mail(tmp_path / "boss", "sent", [
        {
            "from": "127.0.0.1:8000",
            "to": ["127.0.0.1:8001"],
            "subject": "start task",
            "sent_at": "2026-03-20T10:00:00Z",
        },
    ])
    _write_mail(tmp_path / "worker", "sent", [
        {
            "from": "127.0.0.1:8001",
            "to": ["127.0.0.1:8000"],
            "subject": "task done",
            "sent_at": "2026-03-20T11:00:00Z",
        },
    ])

    net = build_network(tmp_path)

    assert len(net.nodes) == 2
    assert len(net.avatar_edges) == 1
    assert len(net.contact_edges) == 2
    assert len(net.mail_edges) == 2  # bidirectional

    # Verify boss→worker and worker→boss mail
    senders = {e.sender for e in net.mail_edges}
    assert senders == {"127.0.0.1:8000", "127.0.0.1:8001"}
