"""Host-level utility for discovering agent network topology.

Crawls a ``base_dir`` filesystem tree and builds an ``AgentNetwork`` object
with three edge layers:

* **avatar** — parent → child spawning tree  (from ``delegates/ledger.jsonl``)
* **contact** — declared "knows about" edges (from ``mailbox/contacts.json``)
* **mail** — actual communication history     (from ``mailbox/inbox/`` + ``mailbox/sent/``)

This is a *read-only* utility — it never modifies agent state.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path


# ---------------------------------------------------------------------------
# Data model
# ---------------------------------------------------------------------------

@dataclass
class AgentNode:
    """An agent discovered in the network."""
    agent_id: str
    agent_name: str
    address: str | None = None       # TCP address if listening
    working_dir: Path | None = None  # resolved filesystem path


@dataclass
class AvatarEdge:
    """A parent → child spawning relationship."""
    parent_id: str
    child_id: str
    child_name: str
    spawned_at: float                # timestamp from ledger
    mission: str = ""
    capabilities: list[str] = field(default_factory=list)
    provider: str | None = None
    model: str | None = None


@dataclass
class ContactEdge:
    """An agent's declared knowledge of another address."""
    owner_id: str                    # who has this contact
    target_address: str              # the contact's address
    target_name: str = ""            # display name in contacts
    note: str = ""


@dataclass
class MailRecord:
    """Metadata of a single email (body stripped)."""
    sender: str                      # address or agent_id
    recipients: list[str]            # to + cc
    subject: str = ""
    timestamp: str = ""              # ISO timestamp (sent_at or received_at)
    direction: str = ""              # "sent" or "received"
    mail_type: str = "normal"        # normal, silence, kill


@dataclass
class MailEdge:
    """Aggregated communication from sender → recipient."""
    sender: str                      # address or agent_id
    recipient: str
    count: int = 0
    last_at: str = ""                # ISO timestamp of most recent
    subjects: list[str] = field(default_factory=list)
    records: list[MailRecord] = field(default_factory=list)


@dataclass
class AgentNetwork:
    """Unified network topology with three edge layers."""
    nodes: dict[str, AgentNode] = field(default_factory=dict)
    avatar_edges: list[AvatarEdge] = field(default_factory=list)
    contact_edges: list[ContactEdge] = field(default_factory=list)
    mail_edges: list[MailEdge] = field(default_factory=list)

    # -- convenience queries --------------------------------------------------

    def children_of(self, agent_id: str) -> list[AgentNode]:
        """Return avatar nodes spawned by *agent_id*."""
        child_ids = [e.child_id for e in self.avatar_edges
                     if e.parent_id == agent_id]
        return [self.nodes[cid] for cid in child_ids if cid in self.nodes]

    def contacts_of(self, agent_id: str) -> list[ContactEdge]:
        """Return contacts declared by *agent_id*."""
        return [e for e in self.contact_edges if e.owner_id == agent_id]

    def mail_of(self, agent_id: str) -> list[MailEdge]:
        """Return all mail edges where *agent_id* is sender or recipient."""
        node = self.nodes.get(agent_id)
        if node is None:
            return []
        # Match on agent_id or address
        ids = {agent_id}
        if node.address:
            ids.add(node.address)
        return [e for e in self.mail_edges
                if e.sender in ids or e.recipient in ids]


# ---------------------------------------------------------------------------
# Builder
# ---------------------------------------------------------------------------

def _normalize_address_list(raw: str | list) -> list[str]:
    """Normalize a ``to`` or ``cc`` field into a flat list of address strings.

    Handles edge cases in real data where addresses may be stored as:
    - a plain string: ``"127.0.0.1:8001"``
    - a list of strings: ``["127.0.0.1:8001"]``
    - a list containing JSON-serialized lists: ``['["127.0.0.1:8001"]']``
    """
    if isinstance(raw, str):
        raw = [raw]
    result: list[str] = []
    for item in raw:
        if not isinstance(item, str):
            continue
        # Try to unwrap JSON-serialized list strings like '["127.0.0.1:8001"]'
        if item.startswith("["):
            try:
                parsed = json.loads(item)
                if isinstance(parsed, list):
                    result.extend(str(x) for x in parsed)
                    continue
            except (json.JSONDecodeError, ValueError):
                pass
        result.append(item)
    return result


def _read_json(path: Path) -> dict | list | None:
    """Read a JSON file, returning None on any failure."""
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError, UnicodeDecodeError):
        return None


def _discover_agents(base_dir: Path) -> dict[str, AgentNode]:
    """Pass 1 — scan for .agent.json manifests in subdirectories."""
    nodes: dict[str, AgentNode] = {}
    if not base_dir.is_dir():
        return nodes

    for child in base_dir.iterdir():
        if not child.is_dir():
            continue
        manifest_path = child / ".agent.json"
        manifest = _read_json(manifest_path)
        if manifest is None or not isinstance(manifest, dict):
            continue
        agent_id = manifest.get("agent_id", "")
        if not agent_id:
            continue
        nodes[agent_id] = AgentNode(
            agent_id=agent_id,
            agent_name=manifest.get("agent_name", ""),
            address=manifest.get("address"),
            working_dir=child,
        )
    return nodes


def _build_avatar_edges(nodes: dict[str, AgentNode]) -> list[AvatarEdge]:
    """Pass 2 — read delegates/ledger.jsonl for each node."""
    edges: list[AvatarEdge] = []
    for agent_id, node in list(nodes.items()):
        if node.working_dir is None:
            continue
        ledger_path = node.working_dir / "delegates" / "ledger.jsonl"
        if not ledger_path.is_file():
            continue
        try:
            lines = ledger_path.read_text(encoding="utf-8").splitlines()
        except OSError:
            continue
        for line in lines:
            line = line.strip()
            if not line:
                continue
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                continue
            if record.get("event") != "avatar":
                continue
            child_id = record.get("agent_id", "")
            if not child_id:
                continue
            # Ensure child node exists (may be from a dead avatar)
            if child_id not in nodes:
                nodes[child_id] = AgentNode(
                    agent_id=child_id,
                    agent_name=record.get("name", ""),
                    address=record.get("address"),
                    working_dir=None,
                )
            edges.append(AvatarEdge(
                parent_id=agent_id,
                child_id=child_id,
                child_name=record.get("name", ""),
                spawned_at=record.get("ts", 0.0),
                mission=record.get("mission", ""),
                capabilities=record.get("capabilities", []),
                provider=record.get("provider"),
                model=record.get("model"),
            ))
    return edges


def _build_contact_edges(nodes: dict[str, AgentNode]) -> list[ContactEdge]:
    """Pass 3 — read mailbox/contacts.json for each node."""
    edges: list[ContactEdge] = []
    for agent_id, node in nodes.items():
        if node.working_dir is None:
            continue
        contacts_path = node.working_dir / "mailbox" / "contacts.json"
        contacts = _read_json(contacts_path)
        if not isinstance(contacts, list):
            continue
        for entry in contacts:
            if not isinstance(entry, dict):
                continue
            edges.append(ContactEdge(
                owner_id=agent_id,
                target_address=entry.get("address", ""),
                target_name=entry.get("name", ""),
                note=entry.get("note", ""),
            ))
    return edges


def _scan_mail_folder(folder: Path, direction: str) -> list[MailRecord]:
    """Scan a mailbox folder (inbox or sent) and extract mail metadata."""
    records: list[MailRecord] = []
    if not folder.is_dir():
        return records
    for msg_dir in folder.iterdir():
        if not msg_dir.is_dir():
            continue
        msg_file = msg_dir / "message.json"
        msg = _read_json(msg_file)
        if not isinstance(msg, dict):
            continue

        sender = msg.get("from", "")
        to_list = _normalize_address_list(msg.get("to", []))
        cc_list = _normalize_address_list(msg.get("cc", []))
        recipients = to_list + cc_list

        # Determine timestamp
        timestamp = msg.get("sent_at", "") or msg.get("received_at", "")

        records.append(MailRecord(
            sender=sender,
            recipients=recipients,
            subject=msg.get("subject", ""),
            timestamp=timestamp,
            direction=direction,
            mail_type=msg.get("type", "normal"),
        ))
    return records


def _build_mail_edges(nodes: dict[str, AgentNode]) -> list[MailEdge]:
    """Pass 4 — crawl inbox + sent folders, aggregate into directed edges."""
    # Collect all records first
    all_records: list[MailRecord] = []
    for node in nodes.values():
        if node.working_dir is None:
            continue
        mailbox = node.working_dir / "mailbox"
        all_records.extend(_scan_mail_folder(mailbox / "inbox", "received"))
        all_records.extend(_scan_mail_folder(mailbox / "sent", "sent"))

    # Aggregate by (sender, recipient)
    edge_map: dict[tuple[str, str], MailEdge] = {}
    for rec in all_records:
        for recipient in rec.recipients:
            key = (rec.sender, recipient)
            if key not in edge_map:
                edge_map[key] = MailEdge(sender=rec.sender, recipient=recipient)
            edge = edge_map[key]
            edge.count += 1
            edge.records.append(rec)
            if rec.subject and rec.subject not in edge.subjects:
                edge.subjects.append(rec.subject)
            if rec.timestamp > edge.last_at:
                edge.last_at = rec.timestamp

    return list(edge_map.values())


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------

def build_network(base_dir: str | Path) -> AgentNetwork:
    """Build an ``AgentNetwork`` by crawling the filesystem under *base_dir*.

    Parameters
    ----------
    base_dir:
        The root directory containing agent working directories.
        Each subdirectory with a ``.agent.json`` manifest is treated as an agent.

    Returns
    -------
    AgentNetwork
        Unified network object with nodes and three edge layers.
    """
    base = Path(base_dir)
    nodes = _discover_agents(base)
    avatar_edges = _build_avatar_edges(nodes)  # may add nodes for dead avatars
    contact_edges = _build_contact_edges(nodes)
    mail_edges = _build_mail_edges(nodes)

    return AgentNetwork(
        nodes=nodes,
        avatar_edges=avatar_edges,
        contact_edges=contact_edges,
        mail_edges=mail_edges,
    )
