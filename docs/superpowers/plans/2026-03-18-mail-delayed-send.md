# Mail Delayed Send & Email Archive Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify all mail sending through an outbox → mailman → sent pipeline with optional delay, and add archive to the email capability.

**Architecture:** Every `send()` writes to `mailbox/outbox/`, spawns a daemon thread (`_mailman`) that sleeps for the delay, dispatches (TCP or self-send), then moves to `mailbox/sent/`. The email capability gains `archive`, `delete`, and `folder` support, and routes its per-recipient dispatch through the same mailman pipeline with `skip_sent=True`.

**Tech Stack:** Python 3.11+, `threading`, `pathlib`, `json`, `shutil`. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-03-18-mail-delayed-send-design.md`

---

### Task 1: Add outbox helpers and `_mailman` thread to mail intrinsic

**Files:**
- Modify: `src/lingtai/intrinsics/mail.py:1-17` (imports + docstring)
- Modify: `src/lingtai/intrinsics/mail.py:19-70` (SCHEMA)
- Create new functions after line 230 (after `_persist_to_inbox`)
- Test: `tests/test_mail_intrinsic.py`

- [ ] **Step 1: Write failing tests for outbox helpers and mailman**

Add to `tests/test_mail_intrinsic.py`:

```python
import time

# ---------------------------------------------------------------------------
# outbox / mailman tests
# ---------------------------------------------------------------------------

class TestOutboxAndMailman:
    def test_send_writes_to_outbox_then_sent(self, tmp_path):
        """Every send writes to outbox, mailman moves to sent."""
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "subject": "hello",
            "message": "world",
        })
        assert result["status"] == "sent"
        assert result["delay"] == 0
        # Give mailman thread time to dispatch
        time.sleep(0.2)
        # Outbox should be empty (mailman moved it)
        outbox = agent._working_dir / "mailbox" / "outbox"
        if outbox.exists():
            assert len(list(outbox.iterdir())) == 0
        # Sent should have one entry
        sent = agent._working_dir / "mailbox" / "sent"
        assert sent.is_dir()
        sent_items = list(sent.iterdir())
        assert len(sent_items) == 1
        msg = json.loads((sent_items[0] / "message.json").read_text())
        assert msg["message"] == "world"
        assert msg["sent_at"]
        assert msg["status"] == "delivered"

    def test_send_with_delay(self, tmp_path):
        """Delayed send writes to outbox, mailman waits before dispatch."""
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "subject": "delayed",
            "message": "later",
            "delay": 1,
        })
        assert result["status"] == "sent"
        assert result["delay"] == 1
        # Immediately after — should be in outbox, NOT yet dispatched
        agent._mail_service.send.assert_not_called()
        outbox = agent._working_dir / "mailbox" / "outbox"
        assert len(list(outbox.iterdir())) == 1
        # Wait for mailman
        time.sleep(1.5)
        agent._mail_service.send.assert_called_once()
        # Outbox empty, sent has entry
        assert len(list(outbox.iterdir())) == 0
        sent = agent._working_dir / "mailbox" / "sent"
        assert len(list(sent.iterdir())) == 1

    def test_send_returns_delay_zero(self, tmp_path):
        """Default delay=0 always appears in return."""
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "hi",
        })
        assert result["delay"] == 0

    def test_self_send_through_mailman(self, tmp_path):
        """Self-send goes through outbox → mailman → inbox + sent."""
        agent = _make_mock_agent(tmp_path)
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:9999",  # self
            "subject": "note",
            "message": "remember",
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        # Should have persisted to inbox
        inbox = agent._working_dir / "mailbox" / "inbox"
        assert len(list(inbox.iterdir())) == 1
        # Should have set mail_arrived
        assert agent._mail_arrived.is_set()
        # Should NOT have called mail_service.send
        agent._mail_service.send.assert_not_called()
        # Should have moved to sent
        sent = agent._working_dir / "mailbox" / "sent"
        assert len(list(sent.iterdir())) == 1

    def test_send_no_mail_service_external(self, tmp_path):
        """External send with no mail service — mailman writes refused to sent."""
        agent = _make_mock_agent(tmp_path)
        agent._mail_service = None
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "hello",
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        sent = agent._working_dir / "mailbox" / "sent"
        assert sent.is_dir()
        sent_items = list(sent.iterdir())
        assert len(sent_items) == 1
        msg = json.loads((sent_items[0] / "message.json").read_text())
        assert msg["status"] == "refused"

    def test_send_refused_external(self, tmp_path):
        """External send refused by mail service — mailman writes refused to sent."""
        agent = _make_mock_agent(tmp_path)
        agent._mail_service.send.return_value = "connection refused"
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "hello",
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        sent = agent._working_dir / "mailbox" / "sent"
        sent_items = list(sent.iterdir())
        msg = json.loads((sent_items[0] / "message.json").read_text())
        assert msg["status"] == "refused"

    def test_mailman_exception_still_moves_to_sent(self, tmp_path):
        """If mail_service.send() raises, mailman writes refused to sent (not stuck in outbox)."""
        agent = _make_mock_agent(tmp_path)
        agent._mail_service.send.side_effect = ConnectionError("boom")
        result = handle(agent, {
            "action": "send",
            "address": "127.0.0.1:8888",
            "message": "hello",
        })
        assert result["status"] == "sent"
        time.sleep(0.2)
        # Should NOT be stuck in outbox
        outbox = agent._working_dir / "mailbox" / "outbox"
        if outbox.exists():
            assert len(list(outbox.iterdir())) == 0
        # Should be in sent with refused status
        sent = agent._working_dir / "mailbox" / "sent"
        sent_items = list(sent.iterdir())
        assert len(sent_items) == 1
        msg = json.loads((sent_items[0] / "message.json").read_text())
        assert msg["status"] == "refused"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_mail_intrinsic.py::TestOutboxAndMailman -v`
Expected: FAIL — `"sent"` not in result status, no outbox/sent dirs

- [ ] **Step 3: Add new imports to mail.py**

In `src/lingtai/intrinsics/mail.py`, add `threading` and `timedelta` to imports:

```python
import threading
from datetime import datetime, timedelta, timezone
```

Fix docstring from "6 actions" to "5 actions" on line 1.

- [ ] **Step 4: Add `delay` to SCHEMA**

In `src/lingtai/intrinsics/mail.py`, add to the `"properties"` dict in SCHEMA:

```python
"delay": {
    "type": "integer",
    "description": "Delay in seconds before delivery (default: 0)",
},
```

- [ ] **Step 5: Add `_outbox_dir`, `_sent_dir`, `_persist_to_outbox`, `_move_to_sent`, `_mailman`**

Add after `_persist_to_inbox` (after line 230) in `src/lingtai/intrinsics/mail.py`:

```python
def _outbox_dir(agent) -> Path:
    """Return the outbox directory."""
    return _mailbox_dir(agent) / "outbox"


def _sent_dir(agent) -> Path:
    """Return the sent directory."""
    return _mailbox_dir(agent) / "sent"


def _persist_to_outbox(agent, payload: dict, deliver_at: datetime) -> str:
    """Write a message to outbox/{uuid}/message.json. Returns the message ID."""
    msg_id = str(uuid.uuid4())
    msg_dir = _outbox_dir(agent) / msg_id
    msg_dir.mkdir(parents=True, exist_ok=True)

    payload = dict(payload)  # shallow copy
    payload["_mailbox_id"] = msg_id
    payload["deliver_at"] = deliver_at.strftime("%Y-%m-%dT%H:%M:%SZ")

    (msg_dir / "message.json").write_text(
        json.dumps(payload, indent=2, default=str)
    )
    return msg_id


def _move_to_sent(agent, msg_id: str, sent_at: str, status: str) -> None:
    """Move outbox/{uuid}/ → sent/{uuid}/, enriching with sent_at and status."""
    src = _outbox_dir(agent) / msg_id
    dst = _sent_dir(agent) / msg_id
    dst.parent.mkdir(parents=True, exist_ok=True)

    if not src.is_dir():
        return  # already moved or cleaned up

    # Enrich payload
    msg_file = src / "message.json"
    if msg_file.is_file():
        try:
            data = json.loads(msg_file.read_text())
        except (json.JSONDecodeError, OSError):
            data = {}
        data["sent_at"] = sent_at
        data["status"] = status
        msg_file.write_text(json.dumps(data, indent=2, default=str))

    shutil.move(str(src), str(dst))


def _mailman(agent, msg_id: str, payload: dict, deliver_at: datetime,
             *, skip_sent: bool = False) -> None:
    """Daemon thread — one per message. Waits, dispatches, archives to sent."""
    import time as _time

    # Wait until deliver_at
    wait = (deliver_at - datetime.now(timezone.utc)).total_seconds()
    if wait > 0:
        _time.sleep(wait)

    # Dispatch (wrapped in try/except — unhandled exception in a daemon thread
    # would leave the message stranded in outbox forever)
    address = payload.get("to", "")
    if isinstance(address, list):
        address = address[0] if address else ""

    try:
        if _is_self_send(agent, address):
            _persist_to_inbox(agent, payload)
            agent._mail_arrived.set()
            status = "delivered"
        elif agent._mail_service is not None:
            err = agent._mail_service.send(address, payload)
            status = "delivered" if err is None else "refused"
        else:
            status = "refused"
    except Exception:
        status = "refused"

    sent_at = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    # Archive to sent (unless caller handles it)
    if not skip_sent:
        _move_to_sent(agent, msg_id, sent_at, status)
    else:
        # Clean up outbox entry even when skipping sent
        outbox_entry = _outbox_dir(agent) / msg_id
        if outbox_entry.is_dir():
            shutil.rmtree(outbox_entry)

    agent._log("mail_sent", address=address, subject=payload.get("subject", ""),
               status=status, message=payload.get("message", ""))
```

- [ ] **Step 6: Rewrite `_send` to use outbox + mailman**

Replace the `_send` function in `src/lingtai/intrinsics/mail.py` (lines 237-289) with:

```python
def _send(agent, args: dict) -> dict:
    """Send a message — validate, write to outbox, spawn mailman."""
    address = args.get("address", "")
    subject = args.get("subject", "")
    message_text = args.get("message", "")
    mail_type = args.get("type", "normal")
    delay = args.get("delay", 0)

    # Privilege gate for non-normal types
    if mail_type != "normal" and not agent._admin.get(mail_type):
        return {"error": f"Not authorized to send type={mail_type!r} mail (requires admin.{mail_type}=True)"}

    if not address:
        return {"error": "address is required"}

    payload = {
        "from": (agent._mail_service.address if agent._mail_service is not None and agent._mail_service.address else agent.agent_id),
        "to": address,
        "subject": subject,
        "message": message_text,
        "type": mail_type,
    }

    # Resolve attachments
    attachments = args.get("attachments", [])
    if attachments:
        resolved = []
        for p in attachments:
            path = Path(p)
            if not path.is_absolute():
                path = agent._working_dir / path
            if not path.is_file():
                return {"error": f"Attachment not found: {path}"}
            resolved.append(str(path))
        payload["attachments"] = resolved

    # Outbox → mailman
    deliver_at = datetime.now(timezone.utc) + timedelta(seconds=delay)
    msg_id = _persist_to_outbox(agent, payload, deliver_at)

    t = threading.Thread(
        target=_mailman,
        args=(agent, msg_id, payload, deliver_at),
        name=f"mailman-{msg_id[:8]}",
        daemon=True,
    )
    t.start()

    return {"status": "sent", "to": address, "delay": delay}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `python -m pytest tests/test_mail_intrinsic.py -v`
Expected: New `TestOutboxAndMailman` tests PASS.

- [ ] **Step 8: Update old tests that check for "delivered"/"refused" status**

The following tests in `tests/test_mail_intrinsic.py` need updating:

- `TestSend.test_send_delivers` (line 81): change `"delivered"` → `"sent"`, add `assert result["delay"] == 0`, remove `agent._mail_service.send.assert_called_once()` (mailman calls it async)
- `TestSend.test_send_no_mail_service` (line 91-100): remove entirely — no mail service is no longer an error at send time; covered by `test_send_no_mail_service_external`
- `TestSend.test_send_refused` (line 102-111): remove entirely — refused is no longer returned at send time; covered by `test_send_refused_external`
- `TestSend.test_send_privilege_gate` (line 133): change `"delivered"` → `"sent"`
- `TestSend.test_send_with_attachments` (line 147-148): change `"delivered"` → `"sent"`, remove `sent_payload` assertion (mailman calls async), add `time.sleep(0.2)` then check sent dir for attachment payload
- `TestSelfSend.test_self_send_persists_to_inbox` (line 176-177): change `"delivered"` → `"sent"`, remove `self_send` assertion, add `time.sleep(0.2)` before checking inbox
- `TestSelfSend.test_self_send_sets_mail_arrived` (line 197): add `time.sleep(0.2)` before assertion
- `TestSelfSend.test_self_send_no_mail_service_still_works` (line 209-210): change `"delivered"` → `"sent"`, remove `self_send` assertion, add `time.sleep(0.2)`

- [ ] **Step 9: Run full test suite**

Run: `python -m pytest tests/test_mail_intrinsic.py -v`
Expected: ALL tests pass

- [ ] **Step 10: Smoke test**

Run: `python -c "import lingtai.intrinsics.mail; print('ok')"`
Expected: `ok`

- [ ] **Step 11: Commit**

```bash
git add src/lingtai/intrinsics/mail.py tests/test_mail_intrinsic.py
git commit -m "feat: unify mail send through outbox → mailman → sent pipeline"
```

---

### Task 2: Add archive and delete to email capability

**Files:**
- Modify: `src/lingtai/capabilities/email.py:23-26` (imports)
- Modify: `src/lingtai/capabilities/email.py:31-121` (SCHEMA)
- Modify: `src/lingtai/capabilities/email.py:240-263` (handle dispatch)
- Add new methods to `EmailManager`
- Test: `tests/test_layers_email.py`

- [ ] **Step 1: Write failing tests for archive**

Add to `tests/test_layers_email.py`:

```python
def test_email_archive_moves_to_archive(tmp_path):
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="keep this")
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "archive", "email_id": [email_id]})
    assert result["status"] == "ok"
    assert email_id in result["archived"]
    # Should be gone from inbox
    inbox = agent.working_dir / "mailbox" / "inbox" / email_id
    assert not inbox.exists()
    # Should be in archive
    archive = agent.working_dir / "mailbox" / "archive" / email_id
    assert archive.is_dir()
    msg = json.loads((archive / "message.json").read_text())
    assert msg["subject"] == "keep this"


def test_email_archive_not_found(tmp_path):
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "archive", "email_id": ["nonexistent"]})
    assert result["not_found"] == ["nonexistent"]


def test_email_check_archive_folder(tmp_path):
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="archived msg")
    mgr = agent.get_capability("email")
    mgr.handle({"action": "archive", "email_id": [email_id]})
    result = mgr.handle({"action": "check", "folder": "archive"})
    assert result["total"] == 1
    assert result["emails"][0]["id"] == email_id


def test_email_read_archive_folder(tmp_path):
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="archived")
    mgr = agent.get_capability("email")
    mgr.handle({"action": "archive", "email_id": [email_id]})
    result = mgr.handle({"action": "read", "email_id": [email_id], "folder": "archive"})
    assert len(result["emails"]) == 1
    assert result["emails"][0]["subject"] == "archived"


def test_email_search_archive_folder(tmp_path):
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="unique archived topic")
    mgr = agent.get_capability("email")
    mgr.handle({"action": "archive", "email_id": [email_id]})
    result = mgr.handle({"action": "search", "query": "unique archived", "folder": "archive"})
    assert result["total"] == 1


def test_email_delete_from_inbox(tmp_path):
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="delete me")
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "delete", "email_id": [email_id]})
    assert email_id in result["deleted"]
    inbox = agent.working_dir / "mailbox" / "inbox" / email_id
    assert not inbox.exists()


def test_email_delete_from_archive(tmp_path):
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"action": "delete", "email_id": ["x"], "folder": "sent"})
    assert "error" in result


def test_email_archive_already_archived(tmp_path):
    """Archiving a message that's already in archive returns not_found (inbox only)."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    email_id = _make_inbox_email(agent.working_dir, subject="move me")
    mgr = agent.get_capability("email")
    mgr.handle({"action": "archive", "email_id": [email_id]})
    result = mgr.handle({"action": "archive", "email_id": [email_id]})
    assert result["not_found"] == [email_id]
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_layers_email.py::test_email_archive_moves_to_archive tests/test_layers_email.py::test_email_delete_from_inbox -v`
Expected: FAIL — no `archive` or `delete` action

- [ ] **Step 3: Update email SCHEMA**

In `src/lingtai/capabilities/email.py`, update the SCHEMA:

Add `"archive"` and `"delete"` to the action enum (line 37):
```python
"enum": [
    "send", "check", "read", "reply", "reply_all", "search", "archive", "delete",
    "contacts", "add_contact", "remove_contact", "edit_contact",
],
```

Update action descriptions to include:
```python
"archive: move email(s) from inbox to archive (requires email_id). "
"delete: remove email(s) from inbox or archive (requires email_id, optional folder). "
```

Add `"archive"` to the `folder` enum (line 94):
```python
"folder": {
    "type": "string",
    "enum": ["inbox", "sent", "archive"],
    "description": "Folder for check/read/search/delete. Default: inbox for check, both inbox+sent for search.",
},
```

Add `"delay"` property:
```python
"delay": {
    "type": "integer",
    "description": "Delay in seconds before delivery (default: 0)",
},
```

- [ ] **Step 4: Update DESCRIPTION**

```python
DESCRIPTION = (
    "Full email client — filesystem-based mailbox with inbox/sent/archive folders, "
    "reply, reply-all, CC/BCC, attachments, regex search, and contacts. "
    "Use 'send' for outgoing email (optional delay for scheduled delivery). "
    "'check' to list inbox, sent, or archive (optional folder param). "
    "'read' to read by ID. "
    "'reply'/'reply_all' to respond. "
    "'search' to find emails by regex (searches from, subject, message). "
    "'archive' to move emails from inbox to archive. "
    "'delete' to remove emails from inbox or archive. "
    "'contacts' to list saved contacts. "
    "'add_contact' to register a peer (address, name, optional note). "
    "'remove_contact' to delete a contact by address. "
    "'edit_contact' to update fields on an existing contact. "
    "Attachments are stored alongside emails in the mailbox. "
    "Etiquette: a short acknowledgement is fine, but do not reply to "
    "an acknowledgement — that creates pointless ping-pong."
)
```

- [ ] **Step 5: Add archive and delete dispatch to `handle`**

In `EmailManager.handle` (line 240-263), add:

```python
elif action == "archive":
    return self._archive(args)
elif action == "delete":
    return self._delete(args)
```

- [ ] **Step 6: Implement `_archive` and `_delete` methods**

Add to `EmailManager` class, after `_search` and before contacts section:

```python
# ------------------------------------------------------------------
# Archive
# ------------------------------------------------------------------

def _archive(self, args: dict) -> dict:
    """Move email(s) from inbox to archive."""
    ids = args.get("email_id", [])
    if isinstance(ids, str):
        ids = [ids]
    if not ids:
        return {"error": "email_id is required"}

    archived = []
    not_found = []
    archive_dir = self._mailbox_path / "archive"
    inbox_dir = self._mailbox_path / "inbox"

    for eid in ids:
        src = inbox_dir / eid
        if not src.is_dir():
            not_found.append(eid)
            continue
        dst = archive_dir / eid
        dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.move(str(src), str(dst))
        archived.append(eid)

    # Clean read tracking for archived messages
    if archived:
        read_set = _read_ids(self._agent)
        read_set -= set(archived)
        _save_read_ids(self._agent, read_set)

    result: dict = {"status": "ok", "archived": archived}
    if not_found:
        result["not_found"] = not_found
    return result

# ------------------------------------------------------------------
# Delete
# ------------------------------------------------------------------

def _delete(self, args: dict) -> dict:
    """Remove email(s) from inbox or archive."""
    ids = args.get("email_id", [])
    if isinstance(ids, str):
        ids = [ids]
    if not ids:
        return {"error": "email_id is required"}

    folder = args.get("folder", "inbox")
    if folder not in ("inbox", "archive"):
        return {"error": f"Cannot delete from folder: {folder}"}

    folder_dir = self._mailbox_path / folder
    deleted = []
    not_found = []

    for eid in ids:
        target = folder_dir / eid
        if target.is_dir():
            shutil.rmtree(target)
            deleted.append(eid)
        else:
            not_found.append(eid)

    # Clean read tracking
    if deleted:
        read_set = _read_ids(self._agent)
        read_set -= set(deleted)
        _save_read_ids(self._agent, read_set)

    result: dict = {"status": "ok", "deleted": deleted}
    if not_found:
        result["not_found"] = not_found
    return result
```

Also add these to the top-level imports of `email.py`:

```python
import shutil

from ..intrinsics.mail import (
    _list_inbox, _load_message, _read_ids, _mark_read, _save_read_ids,
    _message_summary, _mailbox_dir,
)
```

(Adding `_save_read_ids` to the existing import line — `_archive` and `_delete` need it. Do NOT use inline imports.)

- [ ] **Step 7: Update `_load_email` to check archive**

In `EmailManager._load_email` (line 160-178), add archive check after the sent check and before the final `return None` on line 178:

```python
# Archive
path = self._mailbox_path / "archive" / email_id / "message.json"
if path.is_file():
    try:
        data = json.loads(path.read_text())
    except (json.JSONDecodeError, OSError):
        return None
    data["_folder"] = "archive"
    data.setdefault("_mailbox_id", email_id)
    return data
```

- [ ] **Step 8: Update `_list_emails` to handle archive folder**

In `EmailManager._list_emails` (line 180-207), the `else` branch (line 189+) already handles non-inbox folders generically by folder name. Since archive is structured identically to sent, this works for `_list_emails("archive")` with no changes needed. Verify this by reading the code.

- [ ] **Step 9: Update `_email_summary` to handle archive folder**

In `EmailManager._email_summary`, the inbox branch (line 213) and the sent branch (line 221+) handle folder tagging. The sent branch is generic (uses `e.get("_folder", "")`), so archive summaries will work. But inbox messages have `unread` tracking. Archive messages should also show unread status. Add after the inbox check:

```python
if e.get("_folder") == "archive":
    summary = _message_summary(e, read_set)
    summary["folder"] = "archive"
    if e.get("cc"):
        summary["cc"] = e["cc"]
    return summary
```

- [ ] **Step 10: Update `_read` to accept folder param**

In `EmailManager._read` (line 406-440), add folder-aware lookup. When `folder` is specified, look in that folder specifically instead of the default inbox-then-sent cascade:

After `ids` validation, add:
```python
folder = args.get("folder")
```

Then in the loop, if `folder` is specified, load directly from that folder directory:
```python
for eid in ids:
    if folder:
        path = self._mailbox_path / folder / eid / "message.json"
        if path.is_file():
            try:
                data = json.loads(path.read_text())
                data["_folder"] = folder
                data.setdefault("_mailbox_id", eid)
            except (json.JSONDecodeError, OSError):
                errors.append(eid)
                continue
        else:
            errors.append(eid)
            continue
    else:
        data = self._load_email(eid)
        if data is None:
            errors.append(eid)
            continue
    if data.get("_folder") == "inbox":
        _mark_read(self._agent, eid)
    # ... rest of entry building
```

- [ ] **Step 11: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL tests pass (new archive/delete tests + existing tests)

- [ ] **Step 12: Smoke test**

Run: `python -c "import lingtai.capabilities.email; print('ok')"`
Expected: `ok`

- [ ] **Step 13: Commit**

```bash
git add src/lingtai/capabilities/email.py tests/test_layers_email.py
git commit -m "feat: add archive and delete to email capability"
```

---

### Task 3: Route email send through outbox → mailman pipeline

**Files:**
- Modify: `src/lingtai/capabilities/email.py:23-26` (imports)
- Modify: `src/lingtai/capabilities/email.py:269-386` (EmailManager._send)
- Test: `tests/test_layers_email.py`

- [ ] **Step 1: Write failing tests for email send through mailman**

Add to `tests/test_layers_email.py`:

```python
import time

def test_email_send_through_mailman(tmp_path):
    """Email send goes through outbox → mailman → sent."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    time.sleep(0.2)
    # Sent record should exist
    sent_dir = agent.working_dir / "mailbox" / "sent"
    assert sent_dir.is_dir()
    sent_items = list(sent_dir.iterdir())
    assert len(sent_items) == 1
    msg = json.loads((sent_items[0] / "message.json").read_text())
    assert msg["message"] == "hello"
    assert msg["sent_at"]


def test_email_send_with_delay(tmp_path):
    """Email send with delay dispatches after waiting."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    time.sleep(0.2)
    sent_dir = agent.working_dir / "mailbox" / "sent"
    sent_items = list(sent_dir.iterdir())
    assert len(sent_items) == 1  # ONE sent record
    msg = json.loads((sent_items[0] / "message.json").read_text())
    assert msg["bcc"] == ["d"]
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_layers_email.py::test_email_send_through_mailman tests/test_layers_email.py::test_email_send_cc_one_sent_record -v`
Expected: FAIL — status is still `"delivered"`, not `"sent"`

- [ ] **Step 3: Update email.py imports**

Add to imports in `src/lingtai/capabilities/email.py` (extending the existing import from `..intrinsics.mail`):

```python
import threading

from datetime import datetime, timedelta, timezone

from ..intrinsics.mail import (
    _list_inbox, _load_message, _read_ids, _mark_read, _save_read_ids,
    _message_summary, _mailbox_dir,
    _persist_to_outbox, _mailman,
)
```

Note: `_is_self_send` is NOT imported — the mailman handles self-send detection internally. `threading` and `timedelta` are added as top-level imports (not inline). `_save_read_ids` was already added in Task 2.

- [ ] **Step 4: Rewrite `EmailManager._send` to use mailman pipeline**

Replace `EmailManager._send` (lines 269-386) with:

```python
def _send(self, args: dict) -> dict:
    raw_address = args.get("address", "")
    subject = args.get("subject", "")
    message_text = args.get("message", "")
    mail_type = args.get("type", "normal")
    cc = args.get("cc") or []
    bcc = args.get("bcc") or []
    delay = args.get("delay", 0)

    # Privilege gate
    if mail_type != "normal" and not self._agent._admin.get(mail_type):
        return {"error": f"Not authorized to send type={mail_type!r} mail (requires admin.{mail_type}=True)"}

    if isinstance(raw_address, str):
        to_list = [raw_address] if raw_address else []
    else:
        to_list = list(raw_address)

    if not to_list:
        return {"error": "address is required"}

    # Block identical consecutive messages
    all_targets = to_list + cc + bcc
    duplicates = [
        addr for addr in all_targets
        if (prev := self._last_sent.get(addr)) is not None
        and prev[0] == message_text
        and prev[1] >= self._dup_free_passes
    ]
    if duplicates:
        return {
            "status": "blocked",
            "warning": (
                "Identical message already sent to: "
                f"{', '.join(duplicates)}. "
                "This looks like a repetitive loop — "
                "think twice before sending."
            ),
        }

    # Private mode
    if self._private_mode:
        contact_addresses = {c["address"] for c in self._load_contacts()}
        not_in_contacts = [a for a in all_targets if a not in contact_addresses]
        if not_in_contacts:
            return {
                "error": (
                    "Private mode: recipient not in contacts: "
                    f"{', '.join(not_in_contacts)}. "
                    "Register them first with add_contact."
                ),
            }

    sender = (self._agent._mail_service.address
              if self._agent._mail_service is not None and self._agent._mail_service.address
              else self._agent.agent_id)

    # Build visible payload (no bcc)
    base_payload = {
        "from": sender,
        "to": to_list,
        "subject": subject,
        "message": message_text,
        "type": mail_type,
    }
    if cc:
        base_payload["cc"] = cc
    attachments = args.get("attachments", [])
    if attachments:
        base_payload["attachments"] = attachments

    # Dispatch each recipient through outbox → mailman (skip_sent=True)
    deliver_at = datetime.now(timezone.utc) + timedelta(seconds=delay)
    all_recipients = to_list + cc + bcc

    for addr in all_recipients:
        # Override "to" for dispatch — mailman uses payload["to"] for routing
        dispatch_payload = dict(base_payload)
        dispatch_payload["to"] = addr
        msg_id = _persist_to_outbox(self._agent, dispatch_payload, deliver_at)
        t = threading.Thread(
            target=_mailman,
            args=(self._agent, msg_id, dispatch_payload, deliver_at),
            kwargs={"skip_sent": True},
            name=f"mailman-{msg_id[:8]}",
            daemon=True,
        )
        t.start()

    # Write ONE sent record (email-level, preserving the "one email" view)
    sent_id = str(uuid4())
    sent_dir = self._mailbox_path / "sent" / sent_id
    sent_dir.mkdir(parents=True, exist_ok=True)
    sent_record = {
        **base_payload,
        "_mailbox_id": sent_id,
        "sent_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "delay": delay,
    }
    if bcc:
        sent_record["bcc"] = bcc
    (sent_dir / "message.json").write_text(
        json.dumps(sent_record, indent=2, default=str)
    )

    # Track duplicates
    for addr in all_recipients:
        prev = self._last_sent.get(addr)
        if prev is not None and prev[0] == message_text:
            self._last_sent[addr] = (message_text, prev[1] + 1)
        else:
            self._last_sent[addr] = (message_text, 1)

    self._agent._log(
        "email_sent", to=to_list, cc=cc, bcc=bcc,
        subject=subject, message=message_text, delay=delay,
    )

    return {"status": "sent", "to": to_list, "cc": cc, "bcc": bcc, "delay": delay}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: New mailman tests PASS.

- [ ] **Step 6: Update old email tests that check for "delivered" status or inspect async side effects**

**Status changes** (`"delivered"` → `"sent"`):

- `test_email_send_saves_to_sent` (line 204)
- `test_email_blocks_identical_consecutive_send` (line 247)
- `test_email_send_with_attachments` (line ~306)
- `test_email_send_multi_to` (line ~335)
- `test_email_send_cc_visible` (line ~365)
- `test_email_send_bcc_hidden` (line ~405)
- `test_email_private_mode_allows_send_to_contact` (line ~610)
- `test_email_private_mode_off_allows_anyone` (line ~655)

**Async-aware assertion updates** (add `time.sleep(0.5)` before inspecting `mail_svc.send.call_args` or `call_args_list`):

- `test_email_send_with_attachments` — inspects sent payload via `call_args`
- `test_email_reply` (line ~423) — calls `_send` via reply, inspects `call_args`
- `test_email_reply_no_double_re` (line ~438) — same
- `test_email_reply_all` (line ~458) — inspects `call_args_list`
- `test_email_reply_all_excludes_self` (line ~477) — inspects `call_args_list`
- `test_email_blocks_identical_reply` (line ~265) — status change + async

**TCP integration tests** (`test_email_send_multi_to`, `test_email_send_cc_visible`, `test_email_send_bcc_hidden`): These use real TCPMailService instances. After the migration, each recipient gets `dispatch_payload["to"] = single_addr` (string) instead of `to_list` (list). Review assertions on the `"to"` field in received payloads.

**Full rewrite — `test_email_without_mail_service`** (line 548):

The current test sends to "someone" (external) with no mail service and expects an error. After the migration, send succeeds (fire-and-forget), and the mailman writes `"refused"` to sent. Rewrite:

```python
def test_email_without_mail_service(tmp_path):
    """Send without mail service succeeds at send-time; mailman writes refused to sent."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    agent._mail_service = None
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "action": "send", "address": "someone",
        "message": "hello",
    })
    assert result["status"] == "sent"
    # Email writes its own sent record immediately (not via mailman)
    sent_dir = agent.working_dir / "mailbox" / "sent"
    assert sent_dir.is_dir()
```

- [ ] **Step 7: Run full test suite**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL tests pass

- [ ] **Step 8: Smoke test**

Run: `python -c "import lingtai.capabilities.email; print('ok')"`
Expected: `ok`

- [ ] **Step 9: Commit**

```bash
git add src/lingtai/capabilities/email.py tests/test_layers_email.py
git commit -m "feat: route email send through outbox → mailman pipeline, add delay support"
```

---

### Task 4: Run full test suite and fix breakage

**Files:**
- Possibly: any test file that references old `"delivered"` / `"refused"` status from mail sends
- Test: all tests

- [ ] **Step 1: Run full test suite**

Run: `python -m pytest tests/ -v`
Expected: Identify any remaining failures from the status change or mailman async behavior.

Likely failures in:
- `tests/test_agent.py` — `test_mail_with_service`, `test_mail_to_bad_address`, `test_mail_inbox_wiring`
- `tests/test_silence_kill.py` — tests that check `"delivered"` status from mail/email sends
- `tests/test_intrinsics_comm.py` — `test_mail_send_passes_attachments`
- `tests/test_three_agent_email.py` — integration tests

- [ ] **Step 2: Fix each failing test**

For each test:
- Change `"delivered"` → `"sent"` in assertions
- Add `time.sleep(0.2)` where tests check side effects of async dispatch
- Remove assertions on `"refused"` status at send time (now in sent/ record)
- Remove assertions on `"self_send"` key (no longer returned)

- [ ] **Step 3: Run full test suite again**

Run: `python -m pytest tests/ -v`
Expected: ALL tests pass

- [ ] **Step 4: Smoke test the whole package**

Run: `python -c "import lingtai; print('ok')"`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add -u tests/
git commit -m "test: update all tests for outbox → mailman → sent pipeline"
```

---

### Task 5: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update mail intrinsic description in CLAUDE.md**

Update the mail intrinsic description to reflect:
- Outbox → mailman → sent pipeline
- `delay` parameter on send
- `sent/` as system audit trail
- `{"status": "sent"}` return value

- [ ] **Step 2: Update email capability description in CLAUDE.md**

Update to reflect:
- `archive` and `delete` actions
- `folder` param includes `archive`
- `delay` parameter
- Send goes through outbox → mailman pipeline

- [ ] **Step 3: Update the capabilities table**

Add `archive`, `delete`, `delay` to the email capability row.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for outbox/mailman pipeline, email archive"
```
