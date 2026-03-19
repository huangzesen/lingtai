# Email Schedule Design

## Problem

Agents need a way to send recurring messages — heartbeats, periodic status reports, reminders, regular check-ups — without staying awake between sends. The existing `delay` parameter on `send` handles one-shot delayed delivery but cannot express "send this N times every M seconds."

## Design

### Schema

A new `schedule` property on the email tool schema. When `schedule` is present, it takes over routing (top-level `action` is ignored).

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
4. Spawn a daemon thread that loops:
   - Call `_send()` with the stored payload + schedule metadata.
   - Update `sent` count and `last_sent_at` in the schedule JSON.
   - Sleep for `interval` seconds.
   - Before each iteration, re-read the schedule JSON and check `cancelled`.
   - Exit when `sent == count` or `cancelled == true`.
5. Return `{"status": "scheduled", "schedule_id": "...", "interval": N, "count": N}`.

#### `schedule.action = "cancel"`

Requires `schedule.schedule_id`.

1. Load `mailbox/schedules/{schedule_id}/schedule.json`.
2. Set `cancelled: true`, write back.
3. The daemon thread picks up the flag on its next iteration check and exits.
4. Return `{"status": "cancelled", "schedule_id": "..."}`.

#### `schedule.action = "list"`

No required params.

1. Scan `mailbox/schedules/*/schedule.json`.
2. Return list of schedules with status, progress, and computed fields.
3. Each entry includes: `schedule_id`, `interval`, `count`, `sent`, `cancelled`, `created_at`, `last_sent_at`, and a summary of the send payload (to, subject).

### Storage

```
mailbox/schedules/{schedule_id}/schedule.json
```

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

Each email dispatched by a schedule carries a `_schedule` object in the payload, so recipients (and the duplicate guard) can identify it as part of a scheduled sequence:

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

The existing duplicate guard in `EmailManager._send()` tracks consecutive identical messages per recipient. Scheduled sends carry `_schedule` metadata — the guard skips any send that has a `_schedule` key in the args, since repetition is intentional.

### Recovery on Start

During `setup()`, scan `mailbox/schedules/*/schedule.json`. For any schedule where `sent < count` and `cancelled == false`:

- Compute how many sends remain.
- Spawn a new daemon thread to continue from where it left off (`sent` is already tracked).
- The first send happens immediately (no initial delay — the agent was down, so catches up), then resumes the normal interval cadence.

### Schema Description Update

Add to the email tool's action enum description:

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

## Files to Modify

- `src/stoai/capabilities/email.py` — add `schedule` property to SCHEMA, handle routing in `EmailManager.handle()`, implement `_schedule_create`, `_schedule_cancel`, `_schedule_list`, recovery logic in `setup()`, duplicate guard bypass.

## Files NOT Modified

- `src/stoai/intrinsics/mail.py` — no changes needed; schedule is entirely an email-capability feature that reuses the existing `_send()` path.
