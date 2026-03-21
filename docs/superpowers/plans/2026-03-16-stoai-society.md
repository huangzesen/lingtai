# lingtai-society Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone `lingtai-society` package that adds social networking (friends, groups, invitations, institutions) on top of lingtai's email infrastructure.

**Architecture:** SocialAgent wraps BaseAgent+email, replacing the `email` tool with a `social` tool. All communication uses the existing TCP email transport. Contacts, groups, and invitations are persisted as JSON files in the agent's working directory. Institution base class extends SocialAgent for public-facing agents with subscriber management.

**Key implementation constraint:** `EmailManager._send()` does NOT pass through extra dict keys to the wire payload — it constructs `base_payload` explicitly with only `from`, `to`, `subject`, `message`, `type`, `cc`, `attachments`. Social protocol emails (invitation, introduction, subscribe, etc.) with extra fields (`from_id`, `introduced_id`, `original_to`, `topic`) MUST call `agent._mail_service.send(address, payload)` directly, bypassing EmailManager. Only normal messaging actions (send, reply, check, read, search) delegate to EmailManager.

**Key implementation constraint:** `EmailManager` is not stored as an attribute on `BaseAgent`. The `email` capability's `setup()` returns it. `SocialAgent` must receive it as a constructor parameter: `email_mgr = agent.add_capability("email"); SocialAgent(agent, email_mgr=email_mgr, ...)`.

**Tech Stack:** Python 3.11+, lingtai (sibling directory), pytest, dataclasses, JSON persistence.

**Spec:** `docs/superpowers/specs/2026-03-16-lingtai-society-design.md`

---

## Chunk 1: Project Scaffold + Contacts Persistence

### Task 1: Project scaffold

**Files:**
- Create: `../lingtai-society/pyproject.toml`
- Create: `../lingtai-society/src/lingtai_society/__init__.py`
- Create: `../lingtai-society/src/lingtai_society/contacts.py` (empty placeholder)
- Create: `../lingtai-society/src/lingtai_society/permissions.py` (empty placeholder)
- Create: `../lingtai-society/src/lingtai_society/social_agent.py` (empty placeholder)
- Create: `../lingtai-society/src/lingtai_society/institution.py` (empty placeholder)

- [ ] **Step 1: Create directory structure**

```bash
mkdir -p ../lingtai-society/src/lingtai_society
mkdir -p ../lingtai-society/tests
mkdir -p ../lingtai-society/examples/institutions
```

- [ ] **Step 2: Write pyproject.toml**

```toml
[build-system]
requires = ["setuptools>=68.0"]
build-backend = "setuptools.build_meta"

[project]
name = "lingtai-society"
version = "0.1.0"
description = "Social networking primitives for lingtai agents — friends, groups, invitations, institutions"
requires-python = ">=3.11"
dependencies = ["lingtai"]

[tool.setuptools.packages.find]
where = ["src"]

[tool.pytest.ini_options]
testpaths = ["tests"]
```

- [ ] **Step 3: Write __init__.py**

```python
"""lingtai-society — social networking primitives for lingtai agents."""
from __future__ import annotations
```

(Exports will be added as classes are implemented.)

- [ ] **Step 4: Create empty module placeholders**

Create empty files with module docstrings for `contacts.py`, `permissions.py`, `social_agent.py`, `institution.py`.

- [ ] **Step 5: Install in editable mode and verify import**

```bash
cd ../lingtai-society && pip install -e . && python -c "import lingtai_society"
```

- [ ] **Step 6: Commit**

```bash
cd ../lingtai-society && git init && git add -A && git commit -m "feat: initial project scaffold"
```

---

### Task 2: ContactStore — friends persistence

**Files:**
- Create: `../lingtai-society/src/lingtai_society/contacts.py`
- Create: `../lingtai-society/tests/test_contacts.py`

The `ContactStore` class manages `friends.json`, `groups.json`, and `invitations.json` persistence. Pure data layer — no email logic.

- [ ] **Step 1: Write failing tests for friend CRUD**

File: `../lingtai-society/tests/test_contacts.py`

```python
from __future__ import annotations

import json
from pathlib import Path

import pytest

from lingtai_society.contacts import ContactStore


@pytest.fixture
def store(tmp_path: Path) -> ContactStore:
    return ContactStore(tmp_path / "social")


def test_add_friend(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    friends = store.list_friends()
    assert "bob" in friends
    assert friends["bob"]["address"] == "10.0.1.2:8301"
    assert friends["bob"]["nickname"] == "bob"  # default nickname = agent_id


def test_add_friend_with_nickname(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301", nickname="Bobby")
    assert store.list_friends()["bob"]["nickname"] == "Bobby"


def test_remove_friend(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.remove_friend("bob")
    assert "bob" not in store.list_friends()


def test_remove_friend_unknown(store: ContactStore) -> None:
    with pytest.raises(KeyError):
        store.remove_friend("nobody")


def test_is_friend(store: ContactStore) -> None:
    assert not store.is_friend("bob")
    store.add_friend("bob", address="10.0.1.2:8301")
    assert store.is_friend("bob")


def test_get_address(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    assert store.get_address("bob") == "10.0.1.2:8301"


def test_get_address_unknown(store: ContactStore) -> None:
    assert store.get_address("nobody") is None


def test_update_address(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.update_address("bob", "10.0.1.2:9999")
    assert store.get_address("bob") == "10.0.1.2:9999"


def test_set_nickname(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.set_nickname("bob", "Bobby")
    assert store.list_friends()["bob"]["nickname"] == "Bobby"


def test_find_by_address(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    assert store.find_by_address("10.0.1.2:8301") == "bob"
    assert store.find_by_address("unknown:1234") is None


def test_persistence(tmp_path: Path) -> None:
    social_dir = tmp_path / "social"
    store1 = ContactStore(social_dir)
    store1.add_friend("bob", address="10.0.1.2:8301", nickname="Bobby")

    store2 = ContactStore(social_dir)
    friends = store2.list_friends()
    assert "bob" in friends
    assert friends["bob"]["nickname"] == "Bobby"


def test_seed_contacts(tmp_path: Path) -> None:
    social_dir = tmp_path / "social"
    seed = {"bob": {"address": "10.0.1.2:8301"}}

    store1 = ContactStore(social_dir, seed_contacts=seed)
    assert store1.is_friend("bob")

    # Modify address
    store1.update_address("bob", "10.0.1.2:9999")

    # Re-create with same seed — should NOT overwrite
    store2 = ContactStore(social_dir, seed_contacts=seed)
    assert store2.get_address("bob") == "10.0.1.2:9999"


def test_seed_merges_new_only(tmp_path: Path) -> None:
    social_dir = tmp_path / "social"
    store1 = ContactStore(social_dir, seed_contacts={"bob": {"address": "10.0.1.2:8301"}})

    # Re-create with additional seed contact
    store2 = ContactStore(social_dir, seed_contacts={
        "bob": {"address": "10.0.1.2:8301"},
        "charlie": {"address": "10.0.1.3:8301"},
    })
    assert store2.is_friend("bob")
    assert store2.is_friend("charlie")
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd ../lingtai-society && python -m pytest tests/test_contacts.py -v
```

Expected: FAIL — `ContactStore` not implemented yet.

- [ ] **Step 3: Implement ContactStore — friends**

File: `../lingtai-society/src/lingtai_society/contacts.py`

```python
"""ContactStore — persistence layer for friends, groups, and invitations.

Storage:
    social_dir/friends.json      — keyed by agent_id
    social_dir/groups.json       — keyed by group name
    social_dir/invitations.json  — pending_received + pending_sent
"""
from __future__ import annotations

import json
import os
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4


class ContactStore:
    """Manages social state on the filesystem.

    All data is persisted immediately on mutation. Reads load from disk
    each time (no in-memory cache) to keep things simple and correct.
    """

    def __init__(self, social_dir: Path | str, seed_contacts: dict | None = None) -> None:
        self._dir = Path(social_dir)
        self._dir.mkdir(parents=True, exist_ok=True)
        if seed_contacts:
            self._merge_seed(seed_contacts)

    # ------------------------------------------------------------------
    # Friends
    # ------------------------------------------------------------------

    def _friends_path(self) -> Path:
        return self._dir / "friends.json"

    def _load_friends(self) -> dict:
        p = self._friends_path()
        if p.is_file():
            try:
                return json.loads(p.read_text())
            except (json.JSONDecodeError, OSError):
                return {}
        return {}

    def _save_friends(self, data: dict) -> None:
        self._atomic_write(self._friends_path(), data)

    def add_friend(
        self, agent_id: str, *, address: str, nickname: str | None = None,
    ) -> None:
        friends = self._load_friends()
        friends[agent_id] = {
            "nickname": nickname or agent_id,
            "address": address,
            "added_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        }
        self._save_friends(friends)

    def remove_friend(self, agent_id: str) -> None:
        friends = self._load_friends()
        if agent_id not in friends:
            raise KeyError(f"Not a friend: {agent_id}")
        del friends[agent_id]
        self._save_friends(friends)
        # Also remove from all groups
        self._remove_from_all_groups(agent_id)

    def is_friend(self, agent_id: str) -> bool:
        return agent_id in self._load_friends()

    def get_address(self, agent_id: str) -> str | None:
        friends = self._load_friends()
        entry = friends.get(agent_id)
        return entry["address"] if entry else None

    def update_address(self, agent_id: str, new_address: str) -> None:
        friends = self._load_friends()
        if agent_id in friends:
            friends[agent_id]["address"] = new_address
            self._save_friends(friends)

    def set_nickname(self, agent_id: str, nickname: str) -> None:
        friends = self._load_friends()
        if agent_id in friends:
            friends[agent_id]["nickname"] = nickname
            self._save_friends(friends)

    def find_by_address(self, address: str) -> str | None:
        for agent_id, entry in self._load_friends().items():
            if entry["address"] == address:
                return agent_id
        return None

    def list_friends(self) -> dict:
        return self._load_friends()

    # ------------------------------------------------------------------
    # Seed merge
    # ------------------------------------------------------------------

    def _merge_seed(self, seed: dict) -> None:
        friends = self._load_friends()
        changed = False
        for agent_id, info in seed.items():
            if agent_id not in friends:
                friends[agent_id] = {
                    "nickname": info.get("nickname", agent_id),
                    "address": info["address"],
                    "added_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                }
                changed = True
        if changed:
            self._save_friends(friends)

    # ------------------------------------------------------------------
    # Atomic write helper
    # ------------------------------------------------------------------

    def _atomic_write(self, path: Path, data: dict | list) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp = tempfile.mkstemp(dir=str(path.parent), suffix=".tmp")
        try:
            os.write(fd, json.dumps(data, indent=2).encode())
            os.close(fd)
            os.replace(tmp, str(path))
        except Exception:
            try:
                os.close(fd)
            except OSError:
                pass
            if os.path.exists(tmp):
                os.unlink(tmp)
            raise

    # ------------------------------------------------------------------
    # Groups (placeholder — implemented in Task 3)
    # ------------------------------------------------------------------

    def _remove_from_all_groups(self, agent_id: str) -> None:
        """Remove agent_id from all groups. Called by remove_friend."""
        groups = self._load_groups()
        changed = False
        for name, group in groups.items():
            if agent_id in group["members"]:
                group["members"].remove(agent_id)
                changed = True
        if changed:
            self._save_groups(groups)

    def _groups_path(self) -> Path:
        return self._dir / "groups.json"

    def _load_groups(self) -> dict:
        p = self._groups_path()
        if p.is_file():
            try:
                return json.loads(p.read_text())
            except (json.JSONDecodeError, OSError):
                return {}
        return {}

    def _save_groups(self, data: dict) -> None:
        self._atomic_write(self._groups_path(), data)
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ../lingtai-society && python -m pytest tests/test_contacts.py -v
```

Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
cd ../lingtai-society && git add -A && git commit -m "feat: ContactStore — friends persistence with seed merge"
```

---

### Task 3: ContactStore — groups

**Files:**
- Modify: `../lingtai-society/src/lingtai_society/contacts.py`
- Modify: `../lingtai-society/tests/test_contacts.py`

- [ ] **Step 1: Write failing tests for group CRUD**

Append to `../lingtai-society/tests/test_contacts.py`:

```python
# ---- Groups ----

def test_create_group(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.add_friend("charlie", address="10.0.1.3:8301")
    store.create_group("research", members=["bob", "charlie"])
    groups = store.list_groups()
    assert "research" in groups
    assert set(groups["research"]["members"]) == {"bob", "charlie"}


def test_create_group_non_friend_rejected(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    with pytest.raises(ValueError, match="not a friend"):
        store.create_group("team", members=["bob", "nobody"])


def test_dissolve_group(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.create_group("team", members=["bob"])
    store.dissolve_group("team")
    assert "team" not in store.list_groups()


def test_group_add_member(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.add_friend("charlie", address="10.0.1.3:8301")
    store.create_group("team", members=["bob"])
    store.group_add("team", "charlie")
    assert "charlie" in store.list_groups()["team"]["members"]


def test_group_add_non_friend_rejected(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.create_group("team", members=["bob"])
    with pytest.raises(ValueError, match="not a friend"):
        store.group_add("team", "nobody")


def test_group_remove_member(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.add_friend("charlie", address="10.0.1.3:8301")
    store.create_group("team", members=["bob", "charlie"])
    store.group_remove("team", "charlie")
    assert "charlie" not in store.list_groups()["team"]["members"]


def test_unfriend_removes_from_groups(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.add_friend("charlie", address="10.0.1.3:8301")
    store.create_group("team", members=["bob", "charlie"])
    store.remove_friend("charlie")
    assert "charlie" not in store.list_groups()["team"]["members"]


def test_resolve_group(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.add_friend("charlie", address="10.0.1.3:8301")
    store.create_group("team", members=["bob", "charlie"])
    addresses = store.resolve_group("team")
    assert set(addresses) == {"10.0.1.2:8301", "10.0.1.3:8301"}


def test_resolve_group_unknown(store: ContactStore) -> None:
    assert store.resolve_group("nope") is None


def test_is_group(store: ContactStore) -> None:
    store.add_friend("bob", address="10.0.1.2:8301")
    store.create_group("team", members=["bob"])
    assert store.is_group("team")
    assert not store.is_group("nope")
```

- [ ] **Step 2: Run tests to verify new ones fail**

```bash
cd ../lingtai-society && python -m pytest tests/test_contacts.py -v
```

Expected: New group tests FAIL.

- [ ] **Step 3: Implement group methods on ContactStore**

Add to `ContactStore` in `contacts.py`:

```python
    # ------------------------------------------------------------------
    # Groups
    # ------------------------------------------------------------------

    def create_group(self, name: str, *, members: list[str]) -> None:
        friends = self._load_friends()
        for m in members:
            if m not in friends:
                raise ValueError(f"{m!r} is not a friend")
        groups = self._load_groups()
        groups[name] = {
            "members": list(members),
            "created_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        }
        self._save_groups(groups)

    def dissolve_group(self, name: str) -> None:
        groups = self._load_groups()
        if name not in groups:
            raise KeyError(f"Group not found: {name}")
        del groups[name]
        self._save_groups(groups)

    def group_add(self, name: str, agent_id: str) -> None:
        if not self.is_friend(agent_id):
            raise ValueError(f"{agent_id!r} is not a friend")
        groups = self._load_groups()
        if name not in groups:
            raise KeyError(f"Group not found: {name}")
        if agent_id not in groups[name]["members"]:
            groups[name]["members"].append(agent_id)
            self._save_groups(groups)

    def group_remove(self, name: str, agent_id: str) -> None:
        groups = self._load_groups()
        if name not in groups:
            raise KeyError(f"Group not found: {name}")
        if agent_id in groups[name]["members"]:
            groups[name]["members"].remove(agent_id)
            self._save_groups(groups)

    def list_groups(self) -> dict:
        return self._load_groups()

    def is_group(self, name: str) -> bool:
        return name in self._load_groups()

    def resolve_group(self, name: str) -> list[str] | None:
        """Resolve a group name to a list of member addresses."""
        groups = self._load_groups()
        if name not in groups:
            return None
        friends = self._load_friends()
        return [
            friends[m]["address"]
            for m in groups[name]["members"]
            if m in friends
        ]
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ../lingtai-society && python -m pytest tests/test_contacts.py -v
```

Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
cd ../lingtai-society && git add -A && git commit -m "feat: ContactStore — group management"
```

---

### Task 4: ContactStore — invitations

**Files:**
- Modify: `../lingtai-society/src/lingtai_society/contacts.py`
- Modify: `../lingtai-society/tests/test_contacts.py`

- [ ] **Step 1: Write failing tests for invitations**

Append to `../lingtai-society/tests/test_contacts.py`:

```python
# ---- Invitations ----

def test_add_pending_received(store: ContactStore) -> None:
    inv_id = store.add_pending_received(
        from_id="diana", from_address="10.0.1.5:8301", message="hello",
    )
    received = store.list_pending_received()
    assert len(received) == 1
    assert received[0]["id"] == inv_id
    assert received[0]["from_id"] == "diana"


def test_add_pending_sent(store: ContactStore) -> None:
    inv_id = store.add_pending_sent(to_address="10.0.1.6:8301")
    sent = store.list_pending_sent()
    assert len(sent) == 1
    assert sent[0]["id"] == inv_id
    assert sent[0]["to_id"] is None


def test_add_pending_sent_with_to_id(store: ContactStore) -> None:
    inv_id = store.add_pending_sent(to_address="10.0.1.6:8301", to_id="eve")
    sent = store.list_pending_sent()
    assert sent[0]["to_id"] == "eve"


def test_remove_pending_received(store: ContactStore) -> None:
    inv_id = store.add_pending_received(
        from_id="diana", from_address="10.0.1.5:8301",
    )
    store.remove_pending_received(inv_id)
    assert len(store.list_pending_received()) == 0


def test_remove_pending_sent_by_id(store: ContactStore) -> None:
    inv_id = store.add_pending_sent(to_address="10.0.1.6:8301")
    store.remove_pending_sent(inv_id)
    assert len(store.list_pending_sent()) == 0


def test_find_pending_sent_by_from_id(store: ContactStore) -> None:
    store.add_pending_sent(to_address="10.0.1.6:8301", to_id="eve")
    found = store.find_pending_sent(from_id="eve")
    assert found is not None
    assert found["to_id"] == "eve"


def test_find_pending_sent_by_address(store: ContactStore) -> None:
    store.add_pending_sent(to_address="10.0.1.6:8301")
    found = store.find_pending_sent(original_to="10.0.1.6:8301")
    assert found is not None


def test_get_pending_received(store: ContactStore) -> None:
    inv_id = store.add_pending_received(
        from_id="diana", from_address="10.0.1.5:8301",
    )
    inv = store.get_pending_received(inv_id)
    assert inv is not None
    assert inv["from_id"] == "diana"


def test_expired_pending_sent_cleaned(tmp_path: Path) -> None:
    """Pending sent entries older than TTL are removed on load."""
    social_dir = tmp_path / "social"
    store = ContactStore(social_dir, invitation_ttl_days=0)  # 0 = expire immediately
    store.add_pending_sent(to_address="10.0.1.6:8301")
    # Re-load — should be cleaned up
    store2 = ContactStore(social_dir, invitation_ttl_days=0)
    assert len(store2.list_pending_sent()) == 0
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd ../lingtai-society && python -m pytest tests/test_contacts.py -v -k "invitation or pending or expired"
```

Expected: FAIL.

- [ ] **Step 3: Implement invitation methods on ContactStore**

Add to `ContactStore` in `contacts.py`:

```python
    # ------------------------------------------------------------------
    # Invitations
    # ------------------------------------------------------------------

    def __init__(self, social_dir: Path | str, seed_contacts: dict | None = None,
                 invitation_ttl_days: int = 7) -> None:
        self._dir = Path(social_dir)
        self._dir.mkdir(parents=True, exist_ok=True)
        self._invitation_ttl_days = invitation_ttl_days
        if seed_contacts:
            self._merge_seed(seed_contacts)
```

(Update the existing `__init__` to add `invitation_ttl_days`.)

```python
    def _invitations_path(self) -> Path:
        return self._dir / "invitations.json"

    def _load_invitations(self) -> dict:
        p = self._invitations_path()
        if p.is_file():
            try:
                data = json.loads(p.read_text())
            except (json.JSONDecodeError, OSError):
                return {"pending_received": [], "pending_sent": []}
            # Clean up expired pending_sent
            self._clean_expired(data)
            return data
        return {"pending_received": [], "pending_sent": []}

    def _save_invitations(self, data: dict) -> None:
        self._atomic_write(self._invitations_path(), data)

    def _clean_expired(self, data: dict) -> None:
        if self._invitation_ttl_days < 0:
            return
        now = datetime.now(timezone.utc)
        original_len = len(data.get("pending_sent", []))
        data["pending_sent"] = [
            inv for inv in data.get("pending_sent", [])
            if self._within_ttl(inv.get("sent_at", ""), now)
        ]
        if len(data["pending_sent"]) < original_len:
            self._save_invitations(data)

    def _within_ttl(self, ts: str, now: datetime) -> bool:
        if not ts:
            return True
        try:
            sent = datetime.fromisoformat(ts.replace("Z", "+00:00"))
            return (now - sent).days <= self._invitation_ttl_days
        except (ValueError, TypeError):
            return True

    def add_pending_received(
        self, *, from_id: str, from_address: str, message: str = "",
        extra: dict | None = None,
    ) -> str:
        data = self._load_invitations()
        inv_id = f"inv_{uuid4().hex[:12]}"
        entry = {
            "id": inv_id,
            "from_id": from_id,
            "from_address": from_address,
            "message": message,
            "received_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        }
        if extra:
            entry.update(extra)
        data["pending_received"].append(entry)
        self._save_invitations(data)
        return inv_id

    def add_pending_sent(
        self, *, to_address: str, to_id: str | None = None,
    ) -> str:
        data = self._load_invitations()
        inv_id = f"inv_{uuid4().hex[:12]}"
        data["pending_sent"].append({
            "id": inv_id,
            "to_address": to_address,
            "to_id": to_id,
            "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        })
        self._save_invitations(data)
        return inv_id

    def list_pending_received(self) -> list[dict]:
        return self._load_invitations().get("pending_received", [])

    def list_pending_sent(self) -> list[dict]:
        return self._load_invitations().get("pending_sent", [])

    def get_pending_received(self, inv_id: str) -> dict | None:
        for inv in self.list_pending_received():
            if inv["id"] == inv_id:
                return inv
        return None

    def remove_pending_received(self, inv_id: str) -> None:
        data = self._load_invitations()
        data["pending_received"] = [
            inv for inv in data["pending_received"] if inv["id"] != inv_id
        ]
        self._save_invitations(data)

    def remove_pending_sent(self, inv_id: str) -> None:
        data = self._load_invitations()
        data["pending_sent"] = [
            inv for inv in data["pending_sent"] if inv["id"] != inv_id
        ]
        self._save_invitations(data)

    def find_pending_sent(
        self, *, from_id: str | None = None, original_to: str | None = None,
    ) -> dict | None:
        """Find a pending_sent entry by the accepter's from_id or original address."""
        for inv in self.list_pending_sent():
            if from_id and inv.get("to_id") == from_id:
                return inv
            if original_to and inv.get("to_address") == original_to:
                return inv
        return None
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ../lingtai-society && python -m pytest tests/test_contacts.py -v
```

Expected: All PASS.

- [ ] **Step 5: Smoke-test module import**

```bash
cd ../lingtai-society && python -c "from lingtai_society.contacts import ContactStore; print('OK')"
```

- [ ] **Step 6: Commit**

```bash
cd ../lingtai-society && git add -A && git commit -m "feat: ContactStore — invitations with TTL expiry"
```

---

## Chunk 2: Permissions + SocialAgent Core

### Task 5: Permissions module

**Files:**
- Create: `../lingtai-society/src/lingtai_society/permissions.py`
- Create: `../lingtai-society/tests/test_permissions.py`

The permissions module provides pure functions for checking outgoing and incoming rules. No email logic — just contact store queries.

- [ ] **Step 1: Write failing tests**

File: `../lingtai-society/tests/test_permissions.py`

```python
from __future__ import annotations

from pathlib import Path

import pytest

from lingtai_society.contacts import ContactStore
from lingtai_society.permissions import check_outgoing, check_incoming, IncomingVerdict


@pytest.fixture
def store(tmp_path: Path) -> ContactStore:
    s = ContactStore(tmp_path / "social")
    s.add_friend("bob", address="10.0.1.2:8301")
    s.add_friend("charlie", address="10.0.1.3:8301")
    s.create_group("team", members=["bob", "charlie"])
    return s


# ---- Outgoing ----

def test_outgoing_friend_ok(store: ContactStore) -> None:
    result = check_outgoing(store, to="bob")
    assert result["ok"]
    assert result["addresses"] == ["10.0.1.2:8301"]


def test_outgoing_group_ok(store: ContactStore) -> None:
    result = check_outgoing(store, to="team")
    assert result["ok"]
    assert set(result["addresses"]) == {"10.0.1.2:8301", "10.0.1.3:8301"}
    assert result["is_group"]


def test_outgoing_unknown_blocked(store: ContactStore) -> None:
    result = check_outgoing(store, to="nobody")
    assert not result["ok"]
    assert "Unknown recipient" in result["error"]


def test_outgoing_cc_friend_ok(store: ContactStore) -> None:
    result = check_outgoing(store, to="bob", cc=["charlie"])
    assert result["ok"]
    assert "10.0.1.3:8301" in result["cc_addresses"]


def test_outgoing_cc_non_friend_blocked(store: ContactStore) -> None:
    result = check_outgoing(store, to="bob", cc=["nobody"])
    assert not result["ok"]
    assert "Cannot CC non-friend" in result["error"]


# ---- Incoming ----

def test_incoming_friend_deliver(store: ContactStore) -> None:
    v = check_incoming(store, from_id="bob", from_address="10.0.1.2:8301", mail_type="normal")
    assert v == IncomingVerdict.DELIVER


def test_incoming_friend_address_changed(store: ContactStore) -> None:
    v = check_incoming(store, from_id="bob", from_address="10.0.1.2:9999", mail_type="normal")
    assert v == IncomingVerdict.DELIVER
    # Address should be updated
    assert store.get_address("bob") == "10.0.1.2:9999"


def test_incoming_unknown_invitation(store: ContactStore) -> None:
    v = check_incoming(store, from_id="diana", from_address="10.0.1.5:8301", mail_type="invitation")
    assert v == IncomingVerdict.INVITATION


def test_incoming_unknown_introduction(store: ContactStore) -> None:
    v = check_incoming(store, from_id="diana", from_address="10.0.1.5:8301", mail_type="introduction")
    assert v == IncomingVerdict.INTRODUCTION


def test_incoming_unknown_subscribe(store: ContactStore) -> None:
    v = check_incoming(store, from_id="diana", from_address="10.0.1.5:8301", mail_type="subscribe")
    assert v == IncomingVerdict.SUBSCRIBE


def test_incoming_unknown_normal_drop(store: ContactStore) -> None:
    v = check_incoming(store, from_id="nobody", from_address="10.0.1.99:8301", mail_type="normal")
    assert v == IncomingVerdict.DROP


def test_incoming_fallback_address_match(store: ContactStore) -> None:
    """If from_id is missing, fall back to address matching."""
    v = check_incoming(store, from_id=None, from_address="10.0.1.2:8301", mail_type="normal")
    assert v == IncomingVerdict.DELIVER


def test_incoming_invitation_accept(store: ContactStore) -> None:
    v = check_incoming(store, from_id="diana", from_address="10.0.1.5:8301", mail_type="invitation_accept")
    assert v == IncomingVerdict.INVITATION_ACCEPT
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd ../lingtai-society && python -m pytest tests/test_permissions.py -v
```

Expected: FAIL.

- [ ] **Step 3: Implement permissions module**

File: `../lingtai-society/src/lingtai_society/permissions.py`

```python
"""Permission checks for social agent outgoing/incoming email."""
from __future__ import annotations

import enum

from .contacts import ContactStore


class IncomingVerdict(enum.Enum):
    DELIVER = "deliver"
    INVITATION = "invitation"
    INVITATION_ACCEPT = "invitation_accept"
    INTRODUCTION = "introduction"
    SUBSCRIBE = "subscribe"
    UNSUBSCRIBE = "unsubscribe"
    DROP = "drop"


def check_outgoing(
    store: ContactStore,
    *,
    to: str,
    cc: list[str] | None = None,
) -> dict:
    """Check if an outgoing send is allowed.

    Returns dict with:
        ok: bool
        addresses: list[str]     — resolved to addresses
        cc_addresses: list[str]  — resolved CC addresses
        is_group: bool
        error: str               — if not ok
    """
    cc = cc or []

    # Resolve "to" — check group first, then friend
    is_group = store.is_group(to)
    if is_group:
        addresses = store.resolve_group(to)
        if not addresses:
            return {"ok": False, "error": f"Group is empty: {to}"}
    elif store.is_friend(to):
        addr = store.get_address(to)
        addresses = [addr] if addr else []
    else:
        return {"ok": False, "error": f"Unknown recipient: {to}"}

    # Resolve CC — all must be friends
    cc_addresses = []
    for c in cc:
        if not store.is_friend(c):
            return {"ok": False, "error": f"Cannot CC non-friend: {c}"}
        addr = store.get_address(c)
        if addr:
            cc_addresses.append(addr)

    return {
        "ok": True,
        "addresses": addresses,
        "cc_addresses": cc_addresses,
        "is_group": is_group,
    }


_SOCIAL_TYPES = {
    "invitation": IncomingVerdict.INVITATION,
    "invitation_accept": IncomingVerdict.INVITATION_ACCEPT,
    "introduction": IncomingVerdict.INTRODUCTION,
    "subscribe": IncomingVerdict.SUBSCRIBE,
    "unsubscribe": IncomingVerdict.UNSUBSCRIBE,
}


def check_incoming(
    store: ContactStore,
    *,
    from_id: str | None,
    from_address: str,
    mail_type: str,
) -> IncomingVerdict:
    """Determine how to handle an incoming email.

    Social types (invitation, subscribe, etc.) are checked FIRST —
    they get their own routing regardless of friend status.
    Then friend check, then drop.
    """
    # Social types always get special routing (even from friends)
    verdict = _SOCIAL_TYPES.get(mail_type)
    if verdict:
        # Still update friend address if applicable
        if from_id and store.is_friend(from_id):
            stored = store.get_address(from_id)
            if stored != from_address:
                store.update_address(from_id, from_address)
        return verdict

    # Normal mail — check if sender is a friend
    is_friend = False
    if from_id and store.is_friend(from_id):
        is_friend = True
        stored = store.get_address(from_id)
        if stored != from_address:
            store.update_address(from_id, from_address)
    elif from_address:
        matched_id = store.find_by_address(from_address)
        if matched_id:
            is_friend = True

    if is_friend:
        return IncomingVerdict.DELIVER

    return IncomingVerdict.DROP
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd ../lingtai-society && python -m pytest tests/test_permissions.py -v
```

Expected: All PASS.

- [ ] **Step 5: Smoke-test import**

```bash
cd ../lingtai-society && python -c "from lingtai_society.permissions import check_outgoing, check_incoming, IncomingVerdict; print('OK')"
```

- [ ] **Step 6: Commit**

```bash
cd ../lingtai-society && git add -A && git commit -m "feat: permissions — outgoing/incoming checks"
```

---

### Task 6: SocialAgent — core wrapper with tool dispatch

**Files:**
- Create: `../lingtai-society/src/lingtai_society/social_agent.py`
- Create: `../lingtai-society/tests/test_social_agent.py`
- Modify: `../lingtai-society/src/lingtai_society/__init__.py`

This is the main class. It wraps BaseAgent+email, replaces the email tool with a social tool, and intercepts incoming email. This task covers the core wiring and messaging actions (send, broadcast, check, read, reply, reply_all, search). Friend and group tool actions are wired to ContactStore methods. Invitation/introduction handling is Task 7.

- [ ] **Step 1: Write failing tests for SocialAgent construction and messaging**

File: `../lingtai-society/tests/test_social_agent.py`

```python
"""Tests for SocialAgent — uses mocked BaseAgent."""
from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from lingtai_society.social_agent import SocialAgent


def _make_mock_agent(agent_id: str = "alice", tmp_path: Path | None = None) -> tuple[MagicMock, MagicMock]:
    """Create a mock BaseAgent and EmailManager.

    Returns (agent, email_mgr) — caller passes email_mgr to SocialAgent.
    """
    agent = MagicMock()
    agent.agent_id = agent_id
    agent._working_dir = tmp_path or Path("/tmp/test")
    agent._mail_service = MagicMock()
    agent._mail_service.address = "127.0.0.1:8301"
    agent._mail_service.send = MagicMock(return_value=True)
    agent._admin = False

    # Simulate email capability being loaded — EmailManager as separate object
    email_mgr = MagicMock()
    email_mgr.on_normal_mail = MagicMock()
    email_mgr._send = MagicMock(return_value={"status": "delivered"})
    email_mgr._check = MagicMock(return_value={"status": "ok", "emails": []})
    email_mgr._read = MagicMock(return_value={"status": "ok", "message": "hello"})
    email_mgr._reply = MagicMock(return_value={"status": "delivered"})
    email_mgr._reply_all = MagicMock(return_value={"status": "delivered"})
    email_mgr._search = MagicMock(return_value={"status": "ok", "emails": []})
    email_mgr.handle = MagicMock()
    # _on_normal_mail is set by email capability
    agent._on_normal_mail = email_mgr.on_normal_mail
    return agent, email_mgr


@pytest.fixture
def social(tmp_path: Path) -> SocialAgent:
    agent, email_mgr = _make_mock_agent("alice", tmp_path / "alice")
    (tmp_path / "alice").mkdir()
    return SocialAgent(agent, email_mgr=email_mgr, contacts={"bob": {"address": "127.0.0.1:8302"}})


def test_construction_removes_email_tool(tmp_path: Path) -> None:
    agent, email_mgr = _make_mock_agent("alice", tmp_path / "alice")
    (tmp_path / "alice").mkdir()
    social = SocialAgent(agent, email_mgr=email_mgr, contacts={})
    agent.remove_tool.assert_any_call("email")


def test_construction_adds_social_tool(tmp_path: Path) -> None:
    agent, email_mgr = _make_mock_agent("alice", tmp_path / "alice")
    (tmp_path / "alice").mkdir()
    social = SocialAgent(agent, email_mgr=email_mgr, contacts={})
    agent.add_tool.assert_called_once()
    call_kwargs = agent.add_tool.call_args
    assert call_kwargs[0][0] == "social"  # first positional arg = tool name


def test_friends_action(social: SocialAgent) -> None:
    result = social.handle({"action": "friends"})
    assert "bob" in result["friends"]


def test_send_to_friend(social: SocialAgent) -> None:
    result = social.handle({"action": "send", "to": "bob", "message": "hello"})
    assert result.get("error") is None


def test_send_to_unknown_blocked(social: SocialAgent) -> None:
    result = social.handle({"action": "send", "to": "nobody", "message": "hello"})
    assert "error" in result


def test_send_resolves_address(social: SocialAgent) -> None:
    social.handle({"action": "send", "to": "bob", "message": "hello"})
    # Check that EmailManager._send was called with resolved address
    call_args = social._email_mgr._send.call_args[0][0]
    assert call_args["address"] == ["127.0.0.1:8302"]


def test_broadcast(social: SocialAgent) -> None:
    result = social.handle({"action": "broadcast", "message": "news", "subject": "Update"})
    assert result.get("error") is None


def test_check_delegates(social: SocialAgent) -> None:
    social.handle({"action": "check"})
    social._email_mgr._check.assert_called_once()


def test_read_delegates(social: SocialAgent) -> None:
    social.handle({"action": "read", "message_id": "abc123"})
    social._email_mgr._read.assert_called_once()


def test_reply_delegates(social: SocialAgent) -> None:
    social.handle({"action": "reply", "message_id": "abc123", "message": "thanks"})
    social._email_mgr._reply.assert_called_once()


def test_search_delegates(social: SocialAgent) -> None:
    social.handle({"action": "search", "query": "hello"})
    social._email_mgr._search.assert_called_once()
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd ../lingtai-society && python -m pytest tests/test_social_agent.py -v
```

Expected: FAIL.

- [ ] **Step 3: Implement SocialAgent core**

File: `../lingtai-society/src/lingtai_society/social_agent.py`

```python
"""SocialAgent — social networking wrapper around BaseAgent + email.

Replaces the 'email' tool with a 'social' tool that enforces friend-list
permissions, resolves agent_ids and group names to addresses, and intercepts
incoming email for friend filtering and invitation routing.
"""
from __future__ import annotations

import shutil
from pathlib import Path
from typing import TYPE_CHECKING, Any

from .contacts import ContactStore
from .permissions import check_incoming, check_outgoing, IncomingVerdict

if TYPE_CHECKING:
    from lingtai.agent import BaseAgent

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": [
                # Messaging
                "send", "broadcast", "check", "read", "reply", "reply_all", "search",
                # Friends
                "friends", "invite", "accept", "reject", "unfriend", "introduce", "rename",
                # Groups
                "group_create", "group_dissolve", "group_add", "group_remove", "group_list",
                # Subscription (client-side)
                "subscribe", "unsubscribe",
            ],
            "description": (
                "send: send message to a friend or group. "
                "broadcast: BCC to all friends. "
                "check: list inbox. read: read message by ID. "
                "reply/reply_all: reply to message. search: regex search. "
                "friends: list friends. invite: send friend request. "
                "accept/reject: handle pending invitation. "
                "unfriend: remove friend. introduce: introduce two friends. "
                "rename: set friend nickname. "
                "group_create/group_dissolve/group_add/group_remove/group_list: manage groups. "
                "subscribe/unsubscribe: subscribe to institution."
            ),
        },
        "to": {
            "type": "string",
            "description": "agent_id or group name (for send)",
        },
        "message": {"type": "string", "description": "Message body"},
        "subject": {"type": "string", "description": "Subject line"},
        "message_id": {"type": "string", "description": "Email ID (for read/reply)"},
        "address": {"type": "string", "description": "TCP address (for invite, subscribe)"},
        "agent_id": {"type": "string", "description": "Agent ID (for unfriend, rename, subscribe)"},
        "invitation_id": {"type": "string", "description": "Invitation ID (for accept/reject)"},
        "nickname": {"type": "string", "description": "Display nickname (for accept, rename)"},
        "friend_a": {"type": "string", "description": "First friend agent_id (for introduce)"},
        "friend_b": {"type": "string", "description": "Second friend agent_id (for introduce)"},
        "name": {"type": "string", "description": "Group name"},
        "members": {
            "type": "array", "items": {"type": "string"},
            "description": "List of agent_ids (for group_create)",
        },
        "member": {"type": "string", "description": "Agent ID (for group_add/group_remove)"},
        "n": {"type": "integer", "description": "Max emails to show (for check)", "default": 10},
        "query": {"type": "string", "description": "Regex pattern (for search)"},
        "folder": {"type": "string", "enum": ["inbox", "sent"], "description": "Folder (for check/search)"},
        "topic": {"type": "string", "description": "Topic (for subscribe/unsubscribe)"},
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Social networking tool — message friends, manage friend list, form groups. "
    "Use 'send' to message a friend or group (by agent_id or group name). "
    "'broadcast' to BCC all friends. 'check'/'read'/'reply'/'reply_all'/'search' for inbox. "
    "'friends' to list contacts. 'invite' to send friend request (by address). "
    "'accept'/'reject' for pending invitations. 'unfriend' to remove. "
    "'introduce' to introduce two friends to each other. "
    "'group_create'/'group_dissolve'/'group_add'/'group_remove'/'group_list' for groups. "
    "'subscribe'/'unsubscribe' to subscribe to institutions. "
    "Etiquette: a short acknowledgement is fine, but do not reply to "
    "an acknowledgement — that creates pointless ping-pong."
)


class SocialAgent:
    """Social networking wrapper around a BaseAgent with email capability."""

    def __init__(
        self,
        agent: "BaseAgent",
        email_mgr: Any,
        contacts: dict[str, dict] | None = None,
    ) -> None:
        self._agent = agent
        self._email_mgr = email_mgr  # EmailManager instance from add_capability("email")

        # Set up contact store
        social_dir = agent._working_dir / "social"
        self._contacts = ContactStore(social_dir, seed_contacts=contacts)

        # Replace email tool with social tool
        agent.remove_tool("email")
        agent.add_tool("social", schema=SCHEMA, handler=self.handle, description=DESCRIPTION)

        # Hook into receive chain
        self._original_on_normal_mail = agent._on_normal_mail
        agent._on_normal_mail = self._on_normal_mail

    @property
    def contacts(self) -> ContactStore:
        return self._contacts

    def _cleanup_pre_persisted(self, payload: dict) -> None:
        """Delete pre-persisted message from mailbox/inbox/.

        TCPMailService saves messages to disk before on_message fires.
        Social-protocol messages (invitations, introductions, etc.) are
        tracked in social/ — the mailbox copy must be removed.
        """
        mailbox_id = payload.get("_mailbox_id")
        if mailbox_id:
            msg_dir = self._agent._working_dir / "mailbox" / "inbox" / mailbox_id
            if msg_dir.is_dir():
                shutil.rmtree(msg_dir, ignore_errors=True)

    # ------------------------------------------------------------------
    # Tool dispatch
    # ------------------------------------------------------------------

    def handle(self, args: dict) -> dict:
        action = args.get("action", "")
        dispatch = {
            # Messaging
            "send": self._send,
            "broadcast": self._broadcast,
            "check": self._check,
            "read": self._read,
            "reply": self._reply,
            "reply_all": self._reply_all,
            "search": self._search,
            # Friends
            "friends": self._friends,
            "invite": self._invite,
            "accept": self._accept,
            "reject": self._reject,
            "unfriend": self._unfriend,
            "introduce": self._introduce,
            "rename": self._rename,
            # Groups
            "group_create": self._group_create,
            "group_dissolve": self._group_dissolve,
            "group_add": self._group_add,
            "group_remove": self._group_remove,
            "group_list": self._group_list,
            # Subscription
            "subscribe": self._subscribe,
            "unsubscribe": self._unsubscribe,
        }
        handler = dispatch.get(action)
        if handler is None:
            return {"error": f"Unknown social action: {action}"}
        return handler(args)

    # ------------------------------------------------------------------
    # Messaging
    # ------------------------------------------------------------------

    def _send_social_email(self, address: str, payload: dict) -> bool:
        """Send a social-protocol email directly via MailService.

        Used for invitation, introduction, subscribe, etc. — emails that
        carry extra fields (from_id, introduced_id, topic) that EmailManager
        would strip. Normal messaging goes through EmailManager.
        """
        payload.setdefault("from_id", self._agent.agent_id)
        sender = self._agent._mail_service.address or self._agent.agent_id
        payload.setdefault("from", sender)
        return self._agent._mail_service.send(address, payload)

    def _send(self, args: dict) -> dict:
        to = args.get("to", "")
        if not to:
            return {"error": "'to' is required for send"}

        result = check_outgoing(self._contacts, to=to, cc=args.get("cc"))
        if not result["ok"]:
            return {"error": result["error"]}

        # Normal messaging — delegate to EmailManager (it handles sent/ persistence)
        # Inject from_id into the args — EmailManager won't put it on the wire,
        # but it will be in the sent/ record. For wire delivery, we also send
        # from_id by wrapping EmailManager's send to inject from_id.
        email_args = {
            "address": result["addresses"],
            "subject": args.get("subject", ""),
            "message": args.get("message", ""),
        }
        if result.get("cc_addresses"):
            email_args["cc"] = result["cc_addresses"]

        return self._email_mgr._send(email_args)

    def _broadcast(self, args: dict) -> dict:
        friends = self._contacts.list_friends()
        if not friends:
            return {"error": "No friends to broadcast to"}

        all_addresses = [f["address"] for f in friends.values()]
        # Use BCC for all — send via EmailManager with all as BCC
        # EmailManager needs at least one "to" address, but for true broadcast
        # we use the agent's own address as "to" and BCC everyone
        email_args = {
            "address": all_addresses,  # EmailManager fans out to all
            "bcc": [],
            "subject": args.get("subject", ""),
            "message": args.get("message", ""),
        }
        return self._email_mgr._send(email_args)

    def _check(self, args: dict) -> dict:
        return self._email_mgr._check(args)

    def _read(self, args: dict) -> dict:
        return self._email_mgr._read({"email_id": args.get("message_id", "")})

    def _reply(self, args: dict) -> dict:
        return self._email_mgr._reply({
            "email_id": args.get("message_id", ""),
            "message": args.get("message", ""),
            "from_id": self._agent.agent_id,
        })

    def _reply_all(self, args: dict) -> dict:
        return self._email_mgr._reply_all({
            "email_id": args.get("message_id", ""),
            "message": args.get("message", ""),
            "from_id": self._agent.agent_id,
        })

    def _search(self, args: dict) -> dict:
        return self._email_mgr._search({
            "query": args.get("query", ""),
            "folder": args.get("folder"),
        })

    # ------------------------------------------------------------------
    # Friends
    # ------------------------------------------------------------------

    def _friends(self, args: dict) -> dict:
        friends = self._contacts.list_friends()
        return {"friends": friends}

    def _invite(self, args: dict) -> dict:
        address = args.get("address", "")
        if not address:
            return {"error": "'address' is required for invite"}

        message = args.get("message", "")
        ok = self._send_social_email(address, {
            "to": [address],
            "subject": "Friend invitation",
            "message": message,
            "type": "invitation",
        })

        if ok:
            self._contacts.add_pending_sent(to_address=address)

        return {"status": "invitation sent" if ok else "failed", "to": address}

    def _accept(self, args: dict) -> dict:
        inv_id = args.get("invitation_id", "")
        if not inv_id:
            return {"error": "'invitation_id' is required for accept"}

        inv = self._contacts.get_pending_received(inv_id)
        if inv is None:
            return {"error": f"Invitation not found: {inv_id}"}

        from_id = inv["from_id"]
        from_address = inv["from_address"]
        nickname = args.get("nickname", from_id)
        is_introduction = inv.get("message", "").startswith("Introduced by ")

        # Add as friend
        self._contacts.add_friend(from_id, address=from_address, nickname=nickname)
        self._contacts.remove_pending_received(inv_id)

        if is_introduction:
            # This was an introduction — send an invitation to the introduced agent
            # so they can also add us
            self._send_social_email(from_address, {
                "to": [from_address],
                "subject": f"Friend invitation (introduced by {inv.get('_introduced_by', 'a friend')})",
                "message": f"{inv.get('_introduced_by', 'A friend')} introduced us — I'd like to connect",
                "type": "invitation",
                "introduced_by": inv.get("_introduced_by", ""),
            })
            self._contacts.add_pending_sent(to_address=from_address, to_id=from_id)
        else:
            # Direct invitation — send acceptance back
            self._send_social_email(from_address, {
                "to": [from_address],
                "subject": "Friend invitation accepted",
                "message": "I accepted your friend invitation",
                "type": "invitation_accept",
                "original_to": self._agent._mail_service.address,
            })

        return {"status": "accepted", "friend": from_id}

    def _reject(self, args: dict) -> dict:
        inv_id = args.get("invitation_id", "")
        if not inv_id:
            return {"error": "'invitation_id' is required for reject"}

        inv = self._contacts.get_pending_received(inv_id)
        if inv is None:
            return {"error": f"Invitation not found: {inv_id}"}

        self._contacts.remove_pending_received(inv_id)
        return {"status": "rejected"}

    def _unfriend(self, args: dict) -> dict:
        agent_id = args.get("agent_id", "")
        if not agent_id:
            return {"error": "'agent_id' is required for unfriend"}
        try:
            self._contacts.remove_friend(agent_id)
        except KeyError:
            return {"error": f"Not a friend: {agent_id}"}
        return {"status": "unfriended", "agent_id": agent_id}

    def _introduce(self, args: dict) -> dict:
        a = args.get("friend_a", "")
        b = args.get("friend_b", "")
        if not a or not b:
            return {"error": "'friend_a' and 'friend_b' are required for introduce"}
        if not self._contacts.is_friend(a):
            return {"error": f"Not a friend: {a}"}
        if not self._contacts.is_friend(b):
            return {"error": f"Not a friend: {b}"}

        addr_a = self._contacts.get_address(a)
        addr_b = self._contacts.get_address(b)

        # Send introduction to A about B (via direct mail service — carries extra fields)
        self._send_social_email(addr_a, {
            "to": [addr_a],
            "subject": f"Introduction: meet {b}",
            "message": f"I'd like to introduce you to {b}",
            "type": "introduction",
            "introduced_id": b,
            "introduced_address": addr_b,
        })

        # Send introduction to B about A
        self._send_social_email(addr_b, {
            "to": [addr_b],
            "subject": f"Introduction: meet {a}",
            "message": f"I'd like to introduce you to {a}",
            "type": "introduction",
            "introduced_id": a,
            "introduced_address": addr_a,
        })

        return {"status": "introduced", "friend_a": a, "friend_b": b}

    def _rename(self, args: dict) -> dict:
        agent_id = args.get("agent_id", "")
        nickname = args.get("nickname", "")
        if not agent_id or not nickname:
            return {"error": "'agent_id' and 'nickname' are required for rename"}
        if not self._contacts.is_friend(agent_id):
            return {"error": f"Not a friend: {agent_id}"}
        self._contacts.set_nickname(agent_id, nickname)
        return {"status": "renamed", "agent_id": agent_id, "nickname": nickname}

    # ------------------------------------------------------------------
    # Groups
    # ------------------------------------------------------------------

    def _group_create(self, args: dict) -> dict:
        name = args.get("name", "")
        members = args.get("members", [])
        if not name:
            return {"error": "'name' is required for group_create"}
        try:
            self._contacts.create_group(name, members=members)
        except ValueError as e:
            return {"error": str(e)}
        return {"status": "created", "group": name, "members": members}

    def _group_dissolve(self, args: dict) -> dict:
        name = args.get("name", "")
        if not name:
            return {"error": "'name' is required for group_dissolve"}
        try:
            self._contacts.dissolve_group(name)
        except KeyError:
            return {"error": f"Group not found: {name}"}
        return {"status": "dissolved", "group": name}

    def _group_add(self, args: dict) -> dict:
        name = args.get("name", "")
        member = args.get("member", "")
        if not name or not member:
            return {"error": "'name' and 'member' are required for group_add"}
        try:
            self._contacts.group_add(name, member)
        except (KeyError, ValueError) as e:
            return {"error": str(e)}
        return {"status": "added", "group": name, "member": member}

    def _group_remove(self, args: dict) -> dict:
        name = args.get("name", "")
        member = args.get("member", "")
        if not name or not member:
            return {"error": "'name' and 'member' are required for group_remove"}
        try:
            self._contacts.group_remove(name, member)
        except KeyError:
            return {"error": f"Group not found: {name}"}
        return {"status": "removed", "group": name, "member": member}

    def _group_list(self, args: dict) -> dict:
        return {"groups": self._contacts.list_groups()}

    # ------------------------------------------------------------------
    # Subscription (client-side)
    # ------------------------------------------------------------------

    def _subscribe(self, args: dict) -> dict:
        target = args.get("agent_id") or args.get("address", "")
        if not target:
            return {"error": "'agent_id' or 'address' is required for subscribe"}

        address = self._contacts.get_address(target) or target
        topic = args.get("topic")

        payload = {
            "to": [address],
            "subject": "Subscribe",
            "message": "",
            "type": "subscribe",
        }
        if topic:
            payload["topic"] = topic

        ok = self._send_social_email(address, payload)
        return {"status": "subscribed" if ok else "failed", "to": address}

    def _unsubscribe(self, args: dict) -> dict:
        target = args.get("agent_id") or args.get("address", "")
        if not target:
            return {"error": "'agent_id' or 'address' is required for unsubscribe"}

        address = self._contacts.get_address(target) or target
        topic = args.get("topic")

        payload = {
            "to": [address],
            "subject": "Unsubscribe",
            "message": "",
            "type": "unsubscribe",
        }
        if topic:
            payload["topic"] = topic

        ok = self._send_social_email(address, payload)
        return {"status": "unsubscribed" if ok else "failed", "to": address}

    # ------------------------------------------------------------------
    # Incoming email interception
    # ------------------------------------------------------------------

    def _on_normal_mail(self, payload: dict) -> None:
        """Intercept incoming email — friend filter + type routing."""
        from_id = payload.get("from_id")
        from_address = payload.get("from", "")
        mail_type = payload.get("type", "normal")

        verdict = check_incoming(
            self._contacts,
            from_id=from_id,
            from_address=from_address,
            mail_type=mail_type,
        )

        if verdict == IncomingVerdict.DELIVER:
            self._original_on_normal_mail(payload)

        elif verdict == IncomingVerdict.INVITATION:
            self._handle_invitation(payload)

        elif verdict == IncomingVerdict.INVITATION_ACCEPT:
            self._handle_invitation_accept(payload)

        elif verdict == IncomingVerdict.INTRODUCTION:
            self._handle_introduction(payload)

        elif verdict == IncomingVerdict.DROP:
            # Delete pre-persisted message from mailbox
            mailbox_id = payload.get("_mailbox_id")
            if mailbox_id:
                msg_dir = self._agent._working_dir / "mailbox" / "inbox" / mailbox_id
                if msg_dir.is_dir():
                    shutil.rmtree(msg_dir, ignore_errors=True)
            # Log the drop
            self._agent._log(
                "social_dropped",
                from_id=from_id,
                from_address=from_address,
                reason="not a friend",
            )

        # SUBSCRIBE/UNSUBSCRIBE are handled by Institution subclass override

    def _handle_invitation(self, payload: dict) -> None:
        from_id = payload.get("from_id", "unknown")
        from_address = payload.get("from", "")

        # Deduplication: if already a friend, auto-accept
        if self._contacts.is_friend(from_id):
            self._send_social_email(from_address, {
                "to": [from_address],
                "subject": "Friend invitation accepted",
                "message": "We are already friends",
                "type": "invitation_accept",
                "original_to": self._agent._mail_service.address,
            })
            # Delete pre-persisted message
            mailbox_id = payload.get("_mailbox_id")
            if mailbox_id:
                msg_dir = self._agent._working_dir / "mailbox" / "inbox" / mailbox_id
                if msg_dir.is_dir():
                    shutil.rmtree(msg_dir, ignore_errors=True)
            return

        inv_id = self._contacts.add_pending_received(
            from_id=from_id,
            from_address=from_address,
            message=payload.get("message", ""),
        )

        introduced_by = payload.get("introduced_by", "")
        intro_text = f" (introduced by {introduced_by})" if introduced_by else ""

        notification = (
            f"[Friend invitation from {from_id}{intro_text} ({from_address})]\n"
            f"  Message: {payload.get('message', '')}\n"
            f"  ID: {inv_id}\n"
            f'Use social(action="accept", invitation_id="{inv_id}") to accept.\n'
            f'Use social(action="reject", invitation_id="{inv_id}") to reject.'
        )

        from lingtai.agent import _make_message, MSG_REQUEST
        msg = _make_message(MSG_REQUEST, from_address, notification)
        self._agent.inbox.put(msg)

        # Delete pre-persisted message (invitation is tracked in invitations.json)
        mailbox_id = payload.get("_mailbox_id")
        if mailbox_id:
            msg_dir = self._agent._working_dir / "mailbox" / "inbox" / mailbox_id
            if msg_dir.is_dir():
                shutil.rmtree(msg_dir, ignore_errors=True)

    def _handle_invitation_accept(self, payload: dict) -> None:
        from_id = payload.get("from_id", "")
        from_address = payload.get("from", "")
        original_to = payload.get("original_to", "")

        # Find matching pending_sent
        inv = self._contacts.find_pending_sent(
            from_id=from_id, original_to=original_to,
        )

        # Auto-add the accepter as friend
        if not self._contacts.is_friend(from_id):
            self._contacts.add_friend(from_id, address=from_address)

        # Clean up pending_sent
        if inv:
            self._contacts.remove_pending_sent(inv["id"])

        # Delete pre-persisted message
        mailbox_id = payload.get("_mailbox_id")
        if mailbox_id:
            msg_dir = self._agent._working_dir / "mailbox" / "inbox" / mailbox_id
            if msg_dir.is_dir():
                shutil.rmtree(msg_dir, ignore_errors=True)

    def _handle_introduction(self, payload: dict) -> None:
        introduced_id = payload.get("introduced_id", "")
        introduced_address = payload.get("introduced_address", "")
        from_id = payload.get("from_id", "unknown")

        if not introduced_id or not introduced_address:
            return

        # If already friends, skip
        if self._contacts.is_friend(introduced_id):
            return

        inv_id = self._contacts.add_pending_received(
            from_id=introduced_id,
            from_address=introduced_address,
            message=f"Introduced by {from_id}",
            extra={"_introduced_by": from_id},
        )

        notification = (
            f"[Introduction from {from_id}]\n"
            f"  {from_id} would like to introduce you to {introduced_id} ({introduced_address})\n"
            f"  ID: {inv_id}\n"
            f'Use social(action="accept", invitation_id="{inv_id}") to add {introduced_id} as a friend.\n'
            f'Use social(action="reject", invitation_id="{inv_id}") to decline.'
        )

        from lingtai.agent import _make_message, MSG_REQUEST
        msg = _make_message(MSG_REQUEST, payload.get("from", ""), notification)
        self._agent.inbox.put(msg)

        # Delete pre-persisted message
        mailbox_id = payload.get("_mailbox_id")
        if mailbox_id:
            msg_dir = self._agent._working_dir / "mailbox" / "inbox" / mailbox_id
            if msg_dir.is_dir():
                shutil.rmtree(msg_dir, ignore_errors=True)
```

- [ ] **Step 4: Update __init__.py**

```python
"""lingtai-society — social networking primitives for lingtai agents."""
from __future__ import annotations

from .social_agent import SocialAgent

__all__ = ["SocialAgent"]
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd ../lingtai-society && python -m pytest tests/test_social_agent.py -v
```

Expected: All PASS.

- [ ] **Step 6: Smoke-test import**

```bash
cd ../lingtai-society && python -c "from lingtai_society import SocialAgent; print('OK')"
```

- [ ] **Step 7: Commit**

```bash
cd ../lingtai-society && git add -A && git commit -m "feat: SocialAgent — core wrapper with messaging, friends, groups, invitations"
```

---

## Chunk 3: Institution + Subscriber Management

### Task 7: Institution base class

**Files:**
- Create: `../lingtai-society/src/lingtai_society/institution.py`
- Create: `../lingtai-society/tests/test_institution.py`
- Modify: `../lingtai-society/src/lingtai_society/contacts.py` (add subscriber methods)
- Modify: `../lingtai-society/src/lingtai_society/__init__.py`

- [ ] **Step 1: Add subscriber methods to ContactStore**

Append tests to `tests/test_contacts.py`:

```python
# ---- Subscribers ----

def test_add_subscriber(store: ContactStore) -> None:
    store.add_subscriber("bob", topic="all", address="10.0.1.2:8301")
    subs = store.list_subscribers()
    assert "bob" in subs["all"]


def test_add_subscriber_topic(store: ContactStore) -> None:
    store.add_subscriber("bob", topic="climate", address="10.0.1.2:8301")
    subs = store.list_subscribers()
    assert "bob" in subs["topic:climate"]


def test_remove_subscriber(store: ContactStore) -> None:
    store.add_subscriber("bob", topic="all", address="10.0.1.2:8301")
    store.remove_subscriber("bob", topic="all")
    subs = store.list_subscribers()
    assert "bob" not in subs.get("all", [])


def test_get_subscriber_addresses(store: ContactStore) -> None:
    store.add_subscriber("bob", topic="all", address="10.0.1.2:8301")
    addrs = store.get_subscriber_addresses(topic="all")
    assert "10.0.1.2:8301" in addrs
```

- [ ] **Step 2: Run new subscriber tests to verify they fail**

```bash
cd ../lingtai-society && python -m pytest tests/test_contacts.py -v -k "subscriber"
```

Expected: FAIL.

- [ ] **Step 3: Implement subscriber methods on ContactStore**

Add to `contacts.py`:

```python
    # ------------------------------------------------------------------
    # Subscribers (used by Institution)
    # ------------------------------------------------------------------

    def _subscribers_path(self) -> Path:
        return self._dir / "subscribers.json"

    def _load_subscribers(self) -> dict:
        p = self._subscribers_path()
        if p.is_file():
            try:
                return json.loads(p.read_text())
            except (json.JSONDecodeError, OSError):
                return {}
        return {}

    def _save_subscribers(self, data: dict) -> None:
        self._atomic_write(self._subscribers_path(), data)

    def add_subscriber(self, agent_id: str, *, topic: str = "all",
                       address: str | None = None) -> None:
        key = topic if topic == "all" else f"topic:{topic}"
        subs = self._load_subscribers()
        if key not in subs:
            subs[key] = {}
        # Store as dict: agent_id → address
        if agent_id not in subs[key]:
            subs[key][agent_id] = address or ""
            self._save_subscribers(subs)
        elif address and subs[key][agent_id] != address:
            subs[key][agent_id] = address
            self._save_subscribers(subs)

    def remove_subscriber(self, agent_id: str, *, topic: str = "all") -> None:
        key = topic if topic == "all" else f"topic:{topic}"
        subs = self._load_subscribers()
        if key in subs and agent_id in subs[key]:
            del subs[key][agent_id]
            self._save_subscribers(subs)

    def list_subscribers(self, topic: str | None = None) -> dict:
        subs = self._load_subscribers()
        if topic:
            key = topic if topic == "all" else f"topic:{topic}"
            return {key: list(subs.get(key, {}).keys())}
        return {k: list(v.keys()) for k, v in subs.items()}

    def get_subscriber_addresses(self, topic: str = "all") -> list[str]:
        """Get all subscriber addresses for a topic."""
        key = topic if topic == "all" else f"topic:{topic}"
        subs = self._load_subscribers()
        entries = subs.get(key, {})
        return [addr for addr in entries.values() if addr]
```

- [ ] **Step 4: Run subscriber tests**

```bash
cd ../lingtai-society && python -m pytest tests/test_contacts.py -v -k "subscriber"
```

Expected: All PASS.

- [ ] **Step 5: Write failing tests for Institution**

File: `../lingtai-society/tests/test_institution.py`

```python
from __future__ import annotations

from pathlib import Path
from unittest.mock import MagicMock

import pytest

from lingtai_society.institution import Institution


def _make_mock_agent(agent_id: str, tmp_path: Path) -> tuple[MagicMock, MagicMock]:
    agent = MagicMock()
    agent.agent_id = agent_id
    agent._working_dir = tmp_path
    agent._mail_service = MagicMock()
    agent._mail_service.address = "127.0.0.1:9000"
    agent._mail_service.send = MagicMock(return_value=True)
    agent._admin = False
    email_mgr = MagicMock()
    email_mgr.on_normal_mail = MagicMock()
    email_mgr._send = MagicMock(return_value={"status": "delivered"})
    email_mgr._check = MagicMock(return_value={"status": "ok", "emails": []})
    email_mgr._read = MagicMock(return_value={"status": "ok"})
    email_mgr._reply = MagicMock(return_value={"status": "delivered"})
    email_mgr._reply_all = MagicMock(return_value={"status": "delivered"})
    email_mgr._search = MagicMock(return_value={"status": "ok", "emails": []})
    email_mgr.handle = MagicMock()
    agent._on_normal_mail = email_mgr.on_normal_mail
    return agent, email_mgr


@pytest.fixture
def inst(tmp_path: Path) -> Institution:
    agent, email_mgr = _make_mock_agent("news", tmp_path / "news")
    (tmp_path / "news").mkdir()
    return Institution(agent, email_mgr=email_mgr)


def test_institution_has_publish_action(inst: Institution) -> None:
    result = inst.handle({"action": "publish", "message": "Breaking news", "subject": "News"})
    # No subscribers yet
    assert "error" in result or result.get("status") == "no_subscribers"


def test_institution_subscribers_action(inst: Institution) -> None:
    result = inst.handle({"action": "subscribers"})
    assert "subscribers" in result


def test_institution_subscribe_incoming(inst: Institution, tmp_path: Path) -> None:
    """Simulate a subscribe email arriving."""
    inst._on_normal_mail({
        "from": "127.0.0.1:8301",
        "from_id": "bob",
        "type": "subscribe",
        "topic": "",
        "_mailbox_id": "msg123",
    })
    # Bob should be in subscribers
    subs = inst.contacts.list_subscribers()
    assert "bob" in subs.get("all", [])


def test_institution_unsubscribe_incoming(inst: Institution, tmp_path: Path) -> None:
    inst.contacts.add_subscriber("bob", topic="all")
    inst._on_normal_mail({
        "from": "127.0.0.1:8301",
        "from_id": "bob",
        "type": "unsubscribe",
        "topic": "",
        "_mailbox_id": "msg456",
    })
    subs = inst.contacts.list_subscribers()
    assert "bob" not in subs.get("all", [])


def test_institution_public_message_routed(inst: Institution) -> None:
    """Normal email from non-friend should go to on_public_message."""
    received = []
    inst.on_public_message = lambda payload: received.append(payload)

    inst._on_normal_mail({
        "from": "127.0.0.1:8301",
        "from_id": "stranger",
        "type": "normal",
        "message": "news tip",
        "_mailbox_id": "msg789",
    })
    assert len(received) == 1


def test_publish_to_subscribers(inst: Institution) -> None:
    inst.contacts.add_subscriber("bob", topic="all", address="127.0.0.1:8301")
    result = inst.handle({"action": "publish", "message": "News!", "subject": "Breaking"})
    assert result.get("status") == "published"
    # Verify mail was sent
    inst._agent._mail_service.send.assert_called()
```

- [ ] **Step 6: Run institution tests to verify they fail**

```bash
cd ../lingtai-society && python -m pytest tests/test_institution.py -v
```

Expected: FAIL.

- [ ] **Step 7: Implement Institution**

File: `../lingtai-society/src/lingtai_society/institution.py`

```python
"""Institution — public-facing social agent with subscriber management.

Extends SocialAgent to accept messages from non-friends and manage
a subscriber list. Subclasses override on_public_message() to implement
specific behavior (e.g., news publishing, forum posts).
"""
from __future__ import annotations

import shutil
from typing import TYPE_CHECKING

from typing import Any

from .social_agent import SocialAgent
from .permissions import check_incoming, IncomingVerdict

if TYPE_CHECKING:
    from lingtai.agent import BaseAgent


class Institution(SocialAgent):
    """Base class for public-facing social agents.

    Unlike SocialAgent, accepts messages from non-friends
    and maintains subscriber lists.
    """

    def __init__(
        self,
        agent: "BaseAgent",
        email_mgr: Any = None,
        contacts: dict[str, dict] | None = None,
    ) -> None:
        super().__init__(agent, email_mgr=email_mgr, contacts=contacts)

    def handle(self, args: dict) -> dict:
        action = args.get("action", "")
        if action == "publish":
            return self._publish(args)
        if action == "subscribers":
            return self._subscribers(args)
        return super().handle(args)

    def _publish(self, args: dict) -> dict:
        topic = args.get("topic", "all")
        addresses = self._contacts.get_subscriber_addresses(topic=topic)
        if not addresses:
            return {"status": "no_subscribers", "topic": topic}

        # Send to each subscriber individually via direct mail service
        sender = self._agent._mail_service.address or self._agent.agent_id
        payload = {
            "from": sender,
            "from_id": self._agent.agent_id,
            "subject": args.get("subject", ""),
            "message": args.get("message", ""),
            "type": "normal",
        }
        for addr in addresses:
            self._agent._mail_service.send(addr, {**payload, "to": [addr]})

        return {"status": "published", "topic": topic, "recipient_count": len(addresses)}

    def _subscribers(self, args: dict) -> dict:
        topic = args.get("topic")
        return {"subscribers": self._contacts.list_subscribers(topic=topic)}

    def on_public_message(self, payload: dict) -> None:
        """Hook for subclasses. Called when a non-friend sends a normal message.

        Override this to implement institution-specific behavior
        (e.g., store a forum post, queue a news submission).
        """
        pass

    def _on_normal_mail(self, payload: dict) -> None:
        """Override SocialAgent's incoming handler for institution behavior.

        Institutions accept subscribe/unsubscribe and route unknown
        normal emails to on_public_message instead of dropping.
        """
        from_id = payload.get("from_id")
        from_address = payload.get("from", "")
        mail_type = payload.get("type", "normal")

        verdict = check_incoming(
            self._contacts,
            from_id=from_id,
            from_address=from_address,
            mail_type=mail_type,
        )

        if verdict == IncomingVerdict.DELIVER:
            self._original_on_normal_mail(payload)

        elif verdict == IncomingVerdict.INVITATION:
            self._handle_invitation(payload)

        elif verdict == IncomingVerdict.INVITATION_ACCEPT:
            self._handle_invitation_accept(payload)

        elif verdict == IncomingVerdict.INTRODUCTION:
            self._handle_introduction(payload)

        elif verdict == IncomingVerdict.SUBSCRIBE:
            self._handle_subscribe(payload)

        elif verdict == IncomingVerdict.UNSUBSCRIBE:
            self._handle_unsubscribe(payload)

        elif verdict == IncomingVerdict.DROP:
            # Institution: route to on_public_message instead of dropping
            self.on_public_message(payload)
            # Clean up pre-persisted message
            mailbox_id = payload.get("_mailbox_id")
            if mailbox_id:
                msg_dir = self._agent._working_dir / "mailbox" / "inbox" / mailbox_id
                if msg_dir.is_dir():
                    shutil.rmtree(msg_dir, ignore_errors=True)

    def _handle_subscribe(self, payload: dict) -> None:
        from_id = payload.get("from_id", "")
        from_address = payload.get("from", "")
        topic = payload.get("topic") or "all"

        if from_id:
            # Add subscriber with address — does NOT add as friend
            self._contacts.add_subscriber(from_id, topic=topic, address=from_address)

            # Send confirmation via direct mail service
            self._send_social_email(from_address, {
                "to": [from_address],
                "subject": f"Subscribed to {topic}",
                "message": f"You are now subscribed to {topic}.",
            })

        # Clean up pre-persisted message
        mailbox_id = payload.get("_mailbox_id")
        if mailbox_id:
            msg_dir = self._agent._working_dir / "mailbox" / "inbox" / mailbox_id
            if msg_dir.is_dir():
                shutil.rmtree(msg_dir, ignore_errors=True)

    def _handle_unsubscribe(self, payload: dict) -> None:
        from_id = payload.get("from_id", "")
        topic = payload.get("topic") or "all"

        if from_id:
            self._contacts.remove_subscriber(from_id, topic=topic)

        # Clean up pre-persisted message
        mailbox_id = payload.get("_mailbox_id")
        if mailbox_id:
            msg_dir = self._agent._working_dir / "mailbox" / "inbox" / mailbox_id
            if msg_dir.is_dir():
                shutil.rmtree(msg_dir, ignore_errors=True)
```

- [ ] **Step 8: Update __init__.py**

```python
"""lingtai-society — social networking primitives for lingtai agents."""
from __future__ import annotations

from .social_agent import SocialAgent
from .institution import Institution

__all__ = ["SocialAgent", "Institution"]
```

- [ ] **Step 9: Run all tests**

```bash
cd ../lingtai-society && python -m pytest tests/ -v
```

Expected: All PASS.

- [ ] **Step 10: Smoke-test imports**

```bash
cd ../lingtai-society && python -c "from lingtai_society import SocialAgent, Institution; print('OK')"
```

- [ ] **Step 11: Commit**

```bash
cd ../lingtai-society && git add -A && git commit -m "feat: Institution — subscriber management and public message routing"
```

---

## Chunk 4: Examples

### Task 8: Village example

**Files:**
- Create: `../lingtai-society/examples/village.py`
- Create: `../lingtai-society/examples/institutions/news.py`

This is a demonstration script — not production code. It creates a small society of agents and a news institution to showcase the social features.

- [ ] **Step 1: Create NewsAgent example institution**

File: `../lingtai-society/examples/institutions/news.py`

```python
"""NewsAgent — example institution that publishes submitted news."""
from __future__ import annotations

from lingtai_society import Institution


class NewsAgent(Institution):
    """Accepts news submissions from anyone, publishes to subscribers."""

    def on_public_message(self, payload: dict) -> None:
        message = payload.get("message", "")
        sender = payload.get("from_id", payload.get("from", "unknown"))
        subject = payload.get("subject", "News submission")

        # Publish to all subscribers
        self.handle({
            "action": "publish",
            "subject": f"[News from {sender}] {subject}",
            "message": message,
        })
```

- [ ] **Step 2: Create village.py example**

File: `../lingtai-society/examples/village.py`

A script that sets up Alice, Bob, Charlie, and a NewsAgent. Details depend on LLM provider available — structure follows `three_agents.py` from lingtai but uses `SocialAgent` wrappers.

The village example should demonstrate:
- Alice and Bob are friends (seeded)
- Bob and Charlie are friends (seeded)
- Alice and Charlie are NOT friends — must be introduced by Bob
- NewsAgent accepts submissions from anyone
- Agents can subscribe to the news agent

(Implementation follows the pattern of `lingtai/examples/three_agents.py` — adapted for SocialAgent wrappers. The exact code should reference the current `three_agents.py` for LLM setup, HTTP handler, and HTML structure.)

- [ ] **Step 3: Create `examples/institutions/__init__.py`**

```python
# Institution examples
```

- [ ] **Step 4: Smoke-test that examples import correctly**

```bash
cd ../lingtai-society && python -c "from examples.institutions.news import NewsAgent; print('OK')"
```

- [ ] **Step 5: Commit**

```bash
cd ../lingtai-society && git add -A && git commit -m "feat: village example with NewsAgent institution"
```

---

## Execution Notes

**Dependencies between tasks:**
- Tasks 1-4 (Chunk 1) are sequential — each builds on the previous
- Task 5 (permissions) depends on Task 2 (ContactStore friends)
- Task 6 (SocialAgent) depends on Tasks 2-5
- Task 7 (Institution) depends on Task 6
- Task 8 (examples) depends on Task 7

**Testing approach:**
- Each task has its own test file or test section
- All tests use `tmp_path` fixture for isolation
- SocialAgent/Institution tests use mocked BaseAgent (no real LLM)
- ContactStore tests are pure filesystem tests

**Key lingtai internals the implementer needs to know:**
- `agent.add_tool(name, schema=..., handler=..., description=...)` — registers a tool
- `agent.remove_tool(name)` — unregisters a tool
- `agent._on_normal_mail` — hook point replaced by email capability, then by social layer
- `email_mgr = agent.add_capability("email")` — returns the EmailManager instance. Pass it to SocialAgent constructor: `SocialAgent(agent, email_mgr=email_mgr, ...)`
- `agent._working_dir` — the agent's filesystem home
- `agent._mail_service.address` — the agent's TCP address
- `agent.inbox.put(msg)` — inject a message into the agent's processing queue
- `_make_message(MSG_REQUEST, sender, text)` — create an inbox message
- `agent._log(event_type, **kwargs)` — structured logging
