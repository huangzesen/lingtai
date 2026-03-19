# Email Schedule Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `schedule` sub-object to the email capability for recurring sends with create/cancel/list actions and disk-persisted state with crash recovery.

**Architecture:** The `schedule` property is added to the email tool schema. `EmailManager` gains a `_schedule_events` dict (schedule_id → threading.Event) for in-memory cancellation, and schedule state is persisted to `mailbox/schedules/{id}/schedule.json`. A daemon thread per schedule loops send → sleep → check cancel. Recovery on `setup()` resumes incomplete schedules.

**Tech Stack:** Python 3.11+, threading, json, pathlib, uuid, datetime. Tests use pytest + unittest.mock.

**Spec:** `docs/superpowers/specs/2026-03-18-email-schedule-design.md`

---

### Task 1: Schema and routing — schedule object + handle() dispatch

**Files:**
- Modify: `src/stoai/capabilities/email.py:34-127` (SCHEMA)
- Modify: `src/stoai/capabilities/email.py:126` (`required`)
- Modify: `src/stoai/capabilities/email.py:129-146` (DESCRIPTION)
- Modify: `src/stoai/capabilities/email.py:264-291` (`handle()`)
- Test: `tests/test_layers_email.py`

- [ ] **Step 1: Write failing tests for schedule routing**

Add to `tests/test_layers_email.py`:

```python
# ---------------------------------------------------------------------------
# Schedule — schema and routing
# ---------------------------------------------------------------------------

def test_email_schedule_in_schema(tmp_path):
    """Email schema should include schedule property."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    schemas = {s.name: s for s in agent._mcp_schemas}
    props = schemas["email"].schema["properties"]
    assert "schedule" in props
    assert "create" in props["schedule"]["properties"]["action"]["enum"]


def test_email_handle_without_action_or_schedule(tmp_path):
    """Missing both action and schedule should return error."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({})
    assert "action is required" in result["error"]


def test_email_schedule_unknown_action(tmp_path):
    """Unknown schedule action should return error."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"schedule": {"action": "bogus"}})
    assert "error" in result
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_layers_email.py::test_email_schedule_in_schema tests/test_layers_email.py::test_email_handle_without_action_or_schedule tests/test_layers_email.py::test_email_schedule_unknown_action -v`
Expected: FAIL — no `schedule` in schema, `handle()` doesn't check for it.

- [ ] **Step 3: Add schedule to SCHEMA, update required and DESCRIPTION, update handle()**

In `src/stoai/capabilities/email.py`:

1. Add `schedule` property to `SCHEMA["properties"]`:

```python
        "schedule": {
            "type": "object",
            "properties": {
                "action": {
                    "type": "string",
                    "enum": ["create", "cancel", "list"],
                    "description": (
                        "create: start a recurring send (requires address, message + schedule.interval, schedule.count). "
                        "cancel: stop a running schedule (requires schedule.schedule_id). "
                        "list: show all schedules with progress."
                    ),
                },
                "interval": {
                    "type": "integer",
                    "description": "Seconds between each send (for create)",
                },
                "count": {
                    "type": "integer",
                    "description": "Total number of sends (for create)",
                },
                "schedule_id": {
                    "type": "string",
                    "description": "Schedule ID (for cancel)",
                },
            },
        },
```

2. Change `SCHEMA["required"]` from `["action"]` to `[]`.

3. Append to `DESCRIPTION`:

```python
    "Pass a 'schedule' object instead of 'action' for recurring sends. "
    "schedule.action='create': start recurring send (requires address, message, schedule.interval, schedule.count). "
    "schedule.action='cancel': stop a schedule (requires schedule.schedule_id). "
    "schedule.action='list': show all schedules with progress."
```

4. Update `handle()` to check `schedule` first:

```python
    def handle(self, args: dict) -> dict:
        # Schedule sub-object takes priority over action
        schedule = args.get("schedule")
        if schedule is not None:
            return self._handle_schedule(args, schedule)
        action = args.get("action")
        if not action:
            return {"error": "action is required (or pass a schedule object)"}
        # ... existing dispatch ...
```

5. Add `_handle_schedule` stub:

```python
    def _handle_schedule(self, args: dict, schedule: dict) -> dict:
        action = schedule.get("action")
        if action == "create":
            return self._schedule_create(args, schedule)
        elif action == "cancel":
            return self._schedule_cancel(schedule)
        elif action == "list":
            return self._schedule_list()
        else:
            return {"error": f"Unknown schedule action: {action}"}
```

6. Add stubs that return `{"error": "not implemented"}`:

```python
    def _schedule_create(self, args: dict, schedule: dict) -> dict:
        return {"error": "not implemented"}

    def _schedule_cancel(self, schedule: dict) -> dict:
        return {"error": "not implemented"}

    def _schedule_list(self) -> dict:
        return {"error": "not implemented"}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_email.py::test_email_schedule_in_schema tests/test_layers_email.py::test_email_handle_without_action_or_schedule tests/test_layers_email.py::test_email_schedule_unknown_action -v`
Expected: PASS

- [ ] **Step 5: Run ALL existing email tests to verify no regressions**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL PASS — existing tests pass `action` so routing is unchanged.

- [ ] **Step 6: Smoke-test the module**

Run: `python -c "import stoai.capabilities.email"`
Expected: no errors.

- [ ] **Step 7: Commit**

```bash
git add src/stoai/capabilities/email.py tests/test_layers_email.py
git commit -m "feat(email): add schedule sub-object to schema and routing"
```

---

### Task 2: schedule.create — persist and spawn daemon thread

**Files:**
- Modify: `src/stoai/capabilities/email.py` (`EmailManager.__init__`, `_schedule_create`)
- Test: `tests/test_layers_email.py`

- [ ] **Step 1: Write failing tests for schedule create**

```python
# ---------------------------------------------------------------------------
# Schedule — create
# ---------------------------------------------------------------------------

def test_email_schedule_create_basic(tmp_path):
    """schedule.create should persist schedule.json and return schedule_id."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    assert sched["cancelled"] is False


def test_email_schedule_create_sends_messages(tmp_path):
    """schedule.create should send count messages with interval between them."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({
        "message": "hi",
        "schedule": {"action": "create", "interval": 10, "count": 3},
    })
    assert "error" in result
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_layers_email.py -k "schedule_create" -v`
Expected: FAIL — `_schedule_create` returns `{"error": "not implemented"}`.

- [ ] **Step 3: Implement _schedule_create and forward _schedule metadata in _send()**

In `src/stoai/capabilities/email.py`:

1. In `_send()`, after building `sent_record` (before writing to disk), add `_schedule` forwarding:

```python
        if args.get("_schedule"):
            sent_record["_schedule"] = args["_schedule"]
```

This ensures `_schedule` metadata appears in sent records on disk, making it visible to tests and recipients.

3. Add `_schedule_events` dict to `__init__`:

```python
    def __init__(self, agent: "BaseAgent", *, private_mode: bool = False):
        self._agent = agent
        self._private_mode = private_mode
        self._last_sent: dict[str, tuple[str, int]] = {}
        self._dup_free_passes = 2
        self._schedule_events: dict[str, threading.Event] = {}
```

4. Add `_schedules_dir` property:

```python
    @property
    def _schedules_dir(self) -> Path:
        return self._mailbox_path / "schedules"
```

5. Implement `_schedule_create`:

```python
    def _schedule_create(self, args: dict, schedule: dict) -> dict:
        interval = schedule.get("interval")
        count = schedule.get("count")
        if interval is None or count is None:
            return {"error": "schedule.interval and schedule.count are required"}
        if interval <= 0 or count <= 0:
            return {"error": "schedule.interval and schedule.count must be positive"}

        # Validate send params
        raw_address = args.get("address", "")
        if isinstance(raw_address, str):
            to_list = [raw_address] if raw_address else []
        else:
            to_list = list(raw_address)
        if not to_list:
            return {"error": "address is required"}

        # Build send payload snapshot
        send_payload = {
            "address": args.get("address"),
            "subject": args.get("subject", ""),
            "message": args.get("message", ""),
            "cc": args.get("cc") or [],
            "bcc": args.get("bcc") or [],
            "type": args.get("type", "normal"),
        }
        if args.get("attachments"):
            send_payload["attachments"] = args["attachments"]

        schedule_id = uuid4().hex[:12]
        now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        record = {
            "schedule_id": schedule_id,
            "send_payload": send_payload,
            "interval": interval,
            "count": count,
            "sent": 0,
            "cancelled": False,
            "created_at": now,
            "last_sent_at": None,
        }

        # Persist
        sched_dir = self._schedules_dir / schedule_id
        sched_dir.mkdir(parents=True, exist_ok=True)
        self._write_schedule(sched_dir / "schedule.json", record)

        # Spawn daemon
        self._spawn_schedule_thread(schedule_id, record)

        return {"status": "scheduled", "schedule_id": schedule_id, "interval": interval, "count": count}
```

6. Add `_write_schedule` helper (atomic write):

```python
    def _write_schedule(self, path: Path, record: dict) -> None:
        """Atomically write schedule record to disk."""
        path.parent.mkdir(parents=True, exist_ok=True)
        fd, tmp = tempfile.mkstemp(dir=str(path.parent), suffix=".tmp")
        try:
            os.write(fd, json.dumps(record, indent=2, default=str).encode())
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
```

7. Add `_read_schedule` helper:

```python
    def _read_schedule(self, schedule_id: str) -> dict | None:
        """Read schedule record from disk."""
        path = self._schedules_dir / schedule_id / "schedule.json"
        if not path.is_file():
            return None
        try:
            return json.loads(path.read_text())
        except (json.JSONDecodeError, OSError):
            return None
```

8. Add `_spawn_schedule_thread`:

```python
    def _spawn_schedule_thread(self, schedule_id: str, record: dict) -> None:
        """Spawn a daemon thread for a schedule."""
        event = threading.Event()
        self._schedule_events[schedule_id] = event
        t = threading.Thread(
            target=self._schedule_loop,
            args=(schedule_id, record, event),
            name=f"schedule-{schedule_id}",
            daemon=True,
        )
        t.start()
```

9. Add `_schedule_loop`:

```python
    def _schedule_loop(self, schedule_id: str, record: dict, cancel_event: threading.Event) -> None:
        """Daemon thread — sends count messages with interval between each."""
        interval = record["interval"]
        count = record["count"]
        send_payload = record["send_payload"]
        sent = record["sent"]

        for seq_0 in range(sent, count):
            seq = seq_0 + 1  # 1-indexed

            # Check cancel
            if cancel_event.is_set():
                break

            # Increment sent BEFORE sending (at-most-once)
            sched_path = self._schedules_dir / schedule_id / "schedule.json"
            current = self._read_schedule(schedule_id)
            if current is None or current.get("cancelled"):
                break
            current["sent"] = seq
            self._write_schedule(sched_path, current)

            # Build send args with _schedule metadata
            now = datetime.now(timezone.utc)
            remaining = count - seq
            estimated_finish = (now + timedelta(seconds=remaining * interval)).strftime("%Y-%m-%dT%H:%M:%SZ")
            schedule_meta = {
                "schedule_id": schedule_id,
                "seq": seq,
                "total": count,
                "interval": interval,
                "scheduled_at": now.strftime("%Y-%m-%dT%H:%M:%SZ"),
                "estimated_finish": estimated_finish,
            }
            send_args = {**send_payload, "_schedule": schedule_meta}
            self._send(send_args)

            # Update last_sent_at (re-read to preserve any concurrent cancel)
            current = self._read_schedule(schedule_id)
            if current is not None:
                current["sent"] = seq
                current["last_sent_at"] = now.strftime("%Y-%m-%dT%H:%M:%SZ")
                self._write_schedule(sched_path, current)
                if current.get("cancelled"):
                    break

            # Wait for interval (or cancel)
            if seq < count:
                if cancel_event.wait(interval):
                    break

        # Cleanup event
        self._schedule_events.pop(schedule_id, None)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_email.py -k "schedule_create" -v`
Expected: PASS

- [ ] **Step 5: Run ALL existing email tests to verify no regressions**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL PASS

- [ ] **Step 6: Smoke-test the module**

Run: `python -c "import stoai.capabilities.email"`

- [ ] **Step 7: Commit**

```bash
git add src/stoai/capabilities/email.py tests/test_layers_email.py
git commit -m "feat(email): implement schedule.create with daemon thread and disk persistence"
```

---

### Task 3: Duplicate guard bypass for scheduled sends

**Files:**
- Modify: `src/stoai/capabilities/email.py:297-414` (`_send()`)
- Test: `tests/test_layers_email.py`

- [ ] **Step 1: Write failing test for duplicate guard bypass**

```python
def test_email_schedule_bypasses_duplicate_guard(tmp_path):
    """Scheduled sends should not be blocked by the duplicate guard."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc
    mgr = agent.get_capability("email")
    mgr._dup_free_passes = 1  # block after 1 identical send

    result = mgr.handle({
        "address": "someone",
        "message": "heartbeat",
        "schedule": {"action": "create", "interval": 1, "count": 3},
    })
    assert result["status"] == "scheduled"
    time.sleep(4.0)
    # All 3 should have been sent despite identical content
    sched = json.loads(
        (agent.working_dir / "mailbox" / "schedules" / result["schedule_id"] / "schedule.json").read_text()
    )
    assert sched["sent"] == 3
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_layers_email.py::test_email_schedule_bypasses_duplicate_guard -v`
Expected: FAIL — duplicate guard blocks after `_dup_free_passes`.

- [ ] **Step 3: Add bypass in _send()**

In `_send()`, wrap BOTH the duplicate check block AND the duplicate tracking update in a `_schedule` guard:

1. Before the duplicate check block (lines 318-335), add:

```python
        # Bypass duplicate guard for scheduled sends
        if not args.get("_schedule"):
            # existing duplicate guard code (check + block)
            ...
```

Restructure the existing duplicate check to be inside this `if not` branch.

2. Also wrap the duplicate tracking update (near end of `_send()`, lines 402-407) in the same guard:

```python
        # Track duplicates (skip for scheduled sends)
        if not args.get("_schedule"):
            for addr in all_recipients:
                prev = self._last_sent.get(addr)
                ...
```

This prevents scheduled sends from polluting the duplicate counter for future non-scheduled sends.

- [ ] **Step 4: Run test to verify it passes**

Run: `python -m pytest tests/test_layers_email.py::test_email_schedule_bypasses_duplicate_guard -v`
Expected: PASS

- [ ] **Step 5: Run ALL existing email tests**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add src/stoai/capabilities/email.py tests/test_layers_email.py
git commit -m "feat(email): bypass duplicate guard for scheduled sends"
```

---

### Task 4: schedule.cancel

**Files:**
- Modify: `src/stoai/capabilities/email.py` (`_schedule_cancel`)
- Test: `tests/test_layers_email.py`

- [ ] **Step 1: Write failing tests for schedule cancel**

```python
# ---------------------------------------------------------------------------
# Schedule — cancel
# ---------------------------------------------------------------------------

def test_email_schedule_cancel(tmp_path):
    """cancel should stop a running schedule."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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

    # Verify on disk
    sched = json.loads(
        (agent.working_dir / "mailbox" / "schedules" / sid / "schedule.json").read_text()
    )
    assert sched["cancelled"] is True
    # Should NOT have sent all 100
    assert sched["sent"] < 100


def test_email_schedule_cancel_not_found(tmp_path):
    """cancel on non-existent schedule should error."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"schedule": {"action": "cancel", "schedule_id": "nonexistent"}})
    assert "error" in result


def test_email_schedule_cancel_missing_id(tmp_path):
    """cancel without schedule_id should error."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"schedule": {"action": "cancel"}})
    assert "error" in result


def test_email_schedule_cancel_already_stopped(tmp_path):
    """cancel on completed or already-cancelled schedule should return already_stopped."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_layers_email.py -k "schedule_cancel" -v`
Expected: FAIL — `_schedule_cancel` returns `{"error": "not implemented"}`.

- [ ] **Step 3: Implement _schedule_cancel**

```python
    def _schedule_cancel(self, schedule: dict) -> dict:
        schedule_id = schedule.get("schedule_id")
        if not schedule_id:
            return {"error": "schedule.schedule_id is required"}

        record = self._read_schedule(schedule_id)
        if record is None:
            return {"error": f"Schedule not found: {schedule_id}"}

        # Already done?
        if record.get("cancelled") or record.get("sent", 0) >= record.get("count", 0):
            return {"status": "already_stopped", "schedule_id": schedule_id}

        # Set cancelled on disk
        record["cancelled"] = True
        sched_path = self._schedules_dir / schedule_id / "schedule.json"
        self._write_schedule(sched_path, record)

        # Signal in-memory event
        event = self._schedule_events.get(schedule_id)
        if event is not None:
            event.set()

        return {"status": "cancelled", "schedule_id": schedule_id}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_email.py -k "schedule_cancel" -v`
Expected: PASS

- [ ] **Step 5: Run ALL existing email tests**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add src/stoai/capabilities/email.py tests/test_layers_email.py
git commit -m "feat(email): implement schedule.cancel with event-based wakeup"
```

---

### Task 5: schedule.list

**Files:**
- Modify: `src/stoai/capabilities/email.py` (`_schedule_list`)
- Test: `tests/test_layers_email.py`

- [ ] **Step 1: Write failing tests for schedule list**

```python
# ---------------------------------------------------------------------------
# Schedule — list
# ---------------------------------------------------------------------------

def test_email_schedule_list_empty(tmp_path):
    """list with no schedules should return empty list."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mgr = agent.get_capability("email")
    result = mgr.handle({"schedule": {"action": "list"}})
    assert result["status"] == "ok"
    assert result["schedules"] == []


def test_email_schedule_list_shows_active(tmp_path):
    """list should show active schedules with progress."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_layers_email.py -k "schedule_list" -v`
Expected: FAIL

- [ ] **Step 3: Implement _schedule_list**

```python
    def _schedule_list(self) -> dict:
        schedules_dir = self._schedules_dir
        if not schedules_dir.is_dir():
            return {"status": "ok", "schedules": []}

        entries = []
        for sched_dir in schedules_dir.iterdir():
            if not sched_dir.is_dir():
                continue
            sched_file = sched_dir / "schedule.json"
            if not sched_file.is_file():
                continue
            try:
                record = json.loads(sched_file.read_text())
            except (json.JSONDecodeError, OSError):
                continue

            payload = record.get("send_payload", {})
            address = payload.get("address", "")
            if isinstance(address, list):
                address = ", ".join(address)

            sent = record.get("sent", 0)
            count = record.get("count", 0)
            cancelled = record.get("cancelled", False)
            active = sent < count and not cancelled

            entries.append({
                "schedule_id": record.get("schedule_id", sched_dir.name),
                "to": address,
                "subject": payload.get("subject", ""),
                "interval": record.get("interval", 0),
                "count": count,
                "sent": sent,
                "cancelled": cancelled,
                "created_at": record.get("created_at", ""),
                "last_sent_at": record.get("last_sent_at"),
                "active": active,
            })

        # Sort by created_at descending
        entries.sort(key=lambda e: e.get("created_at", ""), reverse=True)
        return {"status": "ok", "schedules": entries}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_email.py -k "schedule_list" -v`
Expected: PASS

- [ ] **Step 5: Run ALL existing email tests**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add src/stoai/capabilities/email.py tests/test_layers_email.py
git commit -m "feat(email): implement schedule.list"
```

---

### Task 6: Recovery on start

**Files:**
- Modify: `src/stoai/capabilities/email.py` (`setup()`, `EmailManager`)
- Test: `tests/test_layers_email.py`

- [ ] **Step 1: Write failing test for recovery**

```python
# ---------------------------------------------------------------------------
# Schedule — recovery
# ---------------------------------------------------------------------------

def test_email_schedule_recovery_on_setup(tmp_path):
    """Incomplete schedules should resume when a new EmailManager is created."""
    # First agent creates a schedule and "crashes" (we just don't let it finish)
    agent1 = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
        "cancelled": False,
        "created_at": "2026-03-18T10:00:00Z",
        "last_sent_at": "2026-03-18T10:00:00Z",
    }
    (sched_dir / "schedule.json").write_text(json.dumps(record, indent=2))

    # Create a NEW agent at the same base_dir — setup() should auto-recover
    agent2 = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                        mail_service=mail_svc, capabilities=["email"])
    # Recovery happens automatically in setup() — no manual call needed

    # Wait for remaining 2 sends
    time.sleep(3.0)
    final = json.loads((sched_dir / "schedule.json").read_text())
    assert final["sent"] == 3


def test_email_schedule_recovery_skips_cancelled(tmp_path):
    """Cancelled schedules should not be resumed."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    mail_svc = MagicMock()
    mail_svc.address = "me"
    mail_svc.send.return_value = None
    agent._mail_service = mail_svc

    sched_id = "cancelled1234"
    sched_dir = agent.working_dir / "mailbox" / "schedules" / sched_id
    sched_dir.mkdir(parents=True, exist_ok=True)
    record = {
        "schedule_id": sched_id,
        "send_payload": {"address": "someone", "message": "x", "cc": [], "bcc": [], "type": "normal"},
        "interval": 1, "count": 5, "sent": 2,
        "cancelled": True,
        "created_at": "2026-03-18T10:00:00Z",
        "last_sent_at": "2026-03-18T10:00:00Z",
    }
    (sched_dir / "schedule.json").write_text(json.dumps(record, indent=2))

    # Recovery runs automatically in setup() — cancelled schedules should be skipped
    time.sleep(2.0)

    # Should NOT have resumed — sent should still be 2
    final = json.loads((sched_dir / "schedule.json").read_text())
    assert final["sent"] == 2
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_layers_email.py -k "schedule_recovery" -v`
Expected: FAIL — `resume_schedules` doesn't exist.

- [ ] **Step 3: Implement resume_schedules**

1. Add `resume_schedules` to `EmailManager`:

```python
    def resume_schedules(self) -> None:
        """Resume any incomplete, non-cancelled schedules from disk."""
        schedules_dir = self._schedules_dir
        if not schedules_dir.is_dir():
            return
        for sched_dir in schedules_dir.iterdir():
            if not sched_dir.is_dir():
                continue
            sched_file = sched_dir / "schedule.json"
            if not sched_file.is_file():
                continue
            try:
                record = json.loads(sched_file.read_text())
            except (json.JSONDecodeError, OSError):
                continue
            if record.get("cancelled"):
                continue
            if record.get("sent", 0) >= record.get("count", 0):
                continue
            # Resume
            self._spawn_schedule_thread(record["schedule_id"], record)
```

2. Call `resume_schedules()` at the end of `setup()`, after the tool is registered:

```python
def setup(agent: "BaseAgent", *, private_mode: bool = False) -> EmailManager:
    """Set up email capability — filesystem-based mailbox."""
    mgr = EmailManager(agent, private_mode=private_mode)
    agent.override_intrinsic("mail")
    agent._mailbox_name = "email box"
    agent._mailbox_tool = "email"
    agent.add_tool(
        "email", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt="Send, receive, reply, and search email.",
    )
    mgr.resume_schedules()
    return mgr
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `python -m pytest tests/test_layers_email.py -k "schedule_recovery" -v`
Expected: PASS

- [ ] **Step 5: Run ALL existing email tests**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL PASS

- [ ] **Step 6: Smoke-test the module**

Run: `python -c "import stoai.capabilities.email"`

- [ ] **Step 7: Commit**

```bash
git add src/stoai/capabilities/email.py tests/test_layers_email.py
git commit -m "feat(email): add schedule recovery on setup"
```

---

### Task 7: Final integration test + CLAUDE.md update

**Files:**
- Test: `tests/test_layers_email.py`
- Modify: `CLAUDE.md` (update email capability docs)

- [ ] **Step 1: Write an end-to-end integration test**

```python
# ---------------------------------------------------------------------------
# Schedule — end-to-end
# ---------------------------------------------------------------------------

def test_email_schedule_end_to_end(tmp_path):
    """Full lifecycle: create → sends happen → list shows progress → cancel → list shows cancelled."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
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
```

- [ ] **Step 2: Run the integration test**

Run: `python -m pytest tests/test_layers_email.py::test_email_schedule_end_to_end -v`
Expected: PASS

- [ ] **Step 3: Run ALL tests**

Run: `python -m pytest tests/test_layers_email.py -v`
Expected: ALL PASS

- [ ] **Step 4: Update CLAUDE.md email capability entry**

In the `Built-in Capabilities` table, update the `email` row description to mention schedule:

```
| `email` | `capabilities=["email"]` | Upgrades mail intrinsic with reply/reply_all, CC/BCC, contacts, sent/archive folders, archive (inbox→archive), delete (inbox/archive), delayed send (`delay`), private mode, and scheduled recurring sends (`schedule` sub-object with create/cancel/list). |
```

- [ ] **Step 5: Commit**

```bash
git add tests/test_layers_email.py CLAUDE.md
git commit -m "feat(email): add schedule integration test and update docs"
```

- [ ] **Step 6: Run full test suite as final check**

Run: `python -m pytest tests/ -v`
Expected: ALL PASS
