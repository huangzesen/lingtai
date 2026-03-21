# Email Schedule Design

## Problem

Agents need a way to send recurring messages — heartbeats, periodic status reports, reminders, regular check-ups — without staying awake between sends. The existing `delay` parameter on `send` handles one-shot delayed delivery but cannot express "send this N times every M seconds."

## Design

### Schema

A new `schedule` property on the email tool schema. When `schedule` is present, it takes over routing (top-level `action` is ignored). The top-level `required` changes from `["action"]` to `[]` — `action` is validated inside `handle()` when `schedule` is absent.

`handle()` checks for `schedule` first, before dispatching on `action`.

```json
"schedule": {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["create", "cancel", "list"],
            "description": "create: start a recurring send. cancel: stop a schedule. list: show all schedules."
        },
        "interval": {
            "type": "integer",
            "description": "Seconds between each send (for create)"
        },
        "count": {
            "type": "integer",
            "description": "Total number of sends (for create)"
        },
        "schedule_id": {
            "type": "string",
            "description": "Schedule ID (for cancel)"
        }
    }
}
```

### Actions

#### `schedule.action = "create"`

Requires all normal send parameters (address, message, subject, cc, bcc, attachments, type) at the top level, plus `schedule.interval` and `schedule.count`.

1. Validate send params (same as `_send`).
2. Generate a `schedule_id` (uuid4 hex, 12 chars).
3. Persist schedule record to `mailbox/schedules/{schedule_id}/schedule.json`.
4. Create a `threading.Event` for this schedule (stored in `self._schedule_events[schedule_id]`).
5. Spawn a daemon thread that loops:
   - Increment `sent` count in the schedule JSON (at-most-once — increment before send).
   - Call `self._send()` with the stored payload + `_schedule` metadata.
   - Update `last_sent_at` in the schedule JSON.
   - Wait on the cancel event for `interval` seconds (`event.wait(interval)`).
   - If the event is set, exit.
   - Exit when `sent == count`.
6. Return `{"status": "scheduled", "schedule_id": "...", "interval": N, "count": N}`.

#### `schedule.action = "cancel"`

Requires `schedule.schedule_id`.

1. Load `mailbox/schedules/{schedule_id}/schedule.json`.
2. Set `cancelled: true`, write back atomically (write-to-temp + `os.replace`).
3. If a `threading.Event` exists for this schedule (in-memory), set it — the daemon thread wakes immediately and exits.
4. Return `{"status": "cancelled", "schedule_id": "..."}`.

#### `schedule.action = "list"`

No required params.

1. Scan `mailbox/schedules/*/schedule.json`.
2. Return list of schedules with status, progress, and computed fields.
3. Each entry includes: `schedule_id`, `interval`, `count`, `sent`, `cancelled`, `created_at`, `last_sent_at`, `active` (true if `sent < count` and not `cancelled`), and a summary of the send payload (to, subject).

### Storage

```
mailbox/schedules/{schedule_id}/schedule.json
```

All writes use atomic file operations (write-to-temp + `os.replace`).

```json
{
    "schedule_id": "a1b2c3d4e5f6",
    "send_payload": {
        "address": "127.0.0.1:8301",
        "subject": "Heartbeat",
        "message": "System status: all green.",
        "cc": [],
        "bcc": [],
        "type": "normal"
    },
    "interval": 1800,
    "count": 10,
    "sent": 0,
    "cancelled": false,
    "created_at": "2026-03-18T10:00:00Z",
    "last_sent_at": null
}
```

### Schedule Metadata on Each Send

The schedule daemon calls `self._send({**stored_payload, '_schedule': {...}})`. Each email dispatched carries a `_schedule` object in the payload:

```json
{
    "_schedule": {
        "schedule_id": "a1b2c3d4e5f6",
        "seq": 3,
        "total": 10,
        "interval": 1800,
        "scheduled_at": "2026-03-18T11:30:00Z",
        "estimated_finish": "2026-03-18T14:30:00Z"
    }
}
```

- `seq`: 1-indexed sequence number for this send.
- `scheduled_at`: UTC timestamp when this particular send was dispatched.
- `estimated_finish`: computed dynamically as `now + (remaining * interval)` where `remaining = total - seq`.

### Duplicate Guard Bypass

In `_send()`, the duplicate guard checks `if args.get('_schedule'):` and skips the duplicate check when present — scheduled repetition is intentional.

### Cancellation Mechanism

Two-layer cancellation:

1. **In-memory**: `threading.Event` per schedule stored in `self._schedule_events`. `cancel` sets the event, daemon thread's `event.wait(interval)` returns immediately. This handles the fast path.
2. **On-disk**: `cancelled: true` in `schedule.json`. This handles recovery — if the process restarts, resumed schedules check this flag.

### Recovery on Start

During `setup()`, scan `mailbox/schedules/*/schedule.json`. For any schedule where `sent < count` and `cancelled == false`:

- Compute how many sends remain.
- Create a new `threading.Event` for this schedule.
- Spawn a new daemon thread to continue from where it left off (`sent` is already tracked).
- The first send happens immediately (no initial delay — the agent was down, so catches up), then resumes the normal interval cadence.

**Delivery semantics**: at-most-once. The `sent` counter is incremented before `_send()` is called. If the process crashes after incrementing but before sending, that iteration is skipped on recovery. For heartbeat/status use cases, a missed beat is preferable to a duplicate.

### Schema Description Update

Add to the email tool's DESCRIPTION and action description:

```
"Pass a 'schedule' object instead of 'action' for recurring sends. "
"schedule.action='create': start recurring send (requires address, message, schedule.interval, schedule.count). "
"schedule.action='cancel': stop a schedule (requires schedule.schedule_id). "
"schedule.action='list': show all schedules with progress."
```

### Return Values

**create:**
```json
{"status": "scheduled", "schedule_id": "a1b2c3d4e5f6", "interval": 1800, "count": 10}
```

**cancel:**
```json
{"status": "cancelled", "schedule_id": "a1b2c3d4e5f6"}
```

**list:**
```json
{
    "status": "ok",
    "schedules": [
        {
            "schedule_id": "a1b2c3d4e5f6",
            "to": "127.0.0.1:8301",
            "subject": "Heartbeat",
            "interval": 1800,
            "count": 10,
            "sent": 3,
            "cancelled": false,
            "created_at": "2026-03-18T10:00:00Z",
            "last_sent_at": "2026-03-18T11:00:00Z",
            "active": true
        }
    ]
}
```

### Error Cases

- `create` without `interval` or `count`: `{"error": "schedule.interval and schedule.count are required"}`
- `create` with `count <= 0` or `interval <= 0`: `{"error": "schedule.interval and schedule.count must be positive"}`
- `cancel` without `schedule_id`: `{"error": "schedule.schedule_id is required"}`
- `cancel` on non-existent schedule: `{"error": "Schedule not found: {id}"}`
- `cancel` on already-cancelled or completed schedule: `{"status": "already_stopped", "schedule_id": "..."}`
- Missing `action` when `schedule` is not present: `{"error": "action is required"}`

## Files to Modify

- `src/lingtai/capabilities/email.py` — add `schedule` property to SCHEMA, change `required` from `["action"]` to `[]`, check for `schedule` first in `handle()`, implement `_schedule_create`, `_schedule_cancel`, `_schedule_list`, add `_schedule_events` dict and recovery logic in `setup()`, duplicate guard bypass in `_send()`.

## Files NOT Modified

- `src/lingtai/intrinsics/mail.py` — no changes needed; schedule is entirely an email-capability feature that reuses the existing `_send()` path.
