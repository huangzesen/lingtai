# Mail Delayed Send & Email Archive

**Date:** 2026-03-18

## Overview

Unify all mail sending through an outbox → mailman → sent pipeline. Every message — immediate or delayed — follows the same path. The email capability gains an archive folder.

## The Mailman Model

Three roles:

1. **Sender (agent)** — calls `send()`, gets `{"status": "sent", "to": addr, "delay": N}`. Fire-and-forget. No cancel. Send is send.
2. **Mailman (daemon thread)** — one per message. Picks up from outbox, waits, dispatches, moves to sent.
3. **Mailbox (disk)** — `inbox/`, `outbox/`, `sent/`. Outbox is transient. Sent is system audit trail.

## Mail Intrinsic Changes

### Schema

`send` action gains one optional parameter:

```json
"delay": {
  "type": "integer",
  "description": "Delay in seconds before delivery (default: 0)"
}
```

The 5 existing actions are unchanged: `send`, `check`, `read`, `search`, `delete`. All remain inbox-only. Outbox and sent are not exposed to the agent.

### Send Pipeline

Every `send()` follows this path — no special case for `delay=0`:

```
_send(agent, args)
    │
    ├─ Validate, build payload, resolve attachments (same as today)
    ├─ Compute deliver_at = now + timedelta(seconds=delay)
    ├─ Write to mailbox/outbox/{uuid}/message.json
    ├─ Spawn daemon thread: _mailman(agent, msg_id, payload, deliver_at)
    └─ Return {"status": "sent", "to": address, "delay": N}

_mailman thread:
    │
    ├─ sleep(remaining seconds until deliver_at)
    ├─ Dispatch:
    │   ├─ self-send → _persist_to_inbox() + _mail_arrived.set()
    │   └─ external  → _mail_service.send(address, payload)
    ├─ Enrich payload with sent_at (UTC) and status ("delivered" / "refused")
    ├─ Move outbox/{uuid}/ → sent/{uuid}/
    ├─ Log via agent._log()
    └─ Thread exits
```

### Disk Layout (mail intrinsic)

```
mailbox/
  inbox/              ← received messages (unchanged)
    {uuid}/
      message.json
  outbox/             ← staged, awaiting dispatch (transient)
    {uuid}/
      message.json    ← payload + deliver_at
  sent/               ← dispatched messages (system audit, not exposed)
    {uuid}/
      message.json    ← payload + deliver_at + sent_at + status
  read.json           ← read tracking (unchanged)
```

### Return Value

All sends return the same shape:

```json
{"status": "sent", "to": "<address>", "delay": 0}
```

No "delivered" / "refused" distinction — the agent doesn't know or care about dispatch outcome. It sent a letter.

### New Helpers

- `_persist_to_outbox(agent, payload, deliver_at) → msg_id` — write to `outbox/{uuid}/message.json`
- `_mailman(agent, msg_id, payload, deliver_at, skip_sent=False)` — daemon thread, one per message. `skip_sent=True` skips the move-to-sent step (used by email capability which writes its own sent record).
- `_move_to_sent(agent, msg_id, sent_at, status)` — move `outbox/{uuid}/` → `sent/{uuid}/`, enrich with metadata

### What Gets Removed from `_send`

- Direct calls to `_mail_service.send()` — mailman handles this
- Direct calls to `_persist_to_inbox()` — mailman handles self-send
- Self-send branching in `_send` — mailman checks `_is_self_send` at dispatch time
- The `"delivered"` / `"refused"` return values — replaced by `"sent"`

### Thread Lifecycle

- `daemon=True` — dies with the process
- No crash recovery — if the process dies, outbox messages are lost. Mail gets lost in real life too.
- No cancellation — send is send
- No startup scan — no resume from outbox on restart
- Thread spawned per message, sleeps for delay duration, dispatches, exits
- Thread name includes message ID for debuggability: `mailman-{uuid[:8]}`

### Edge Cases

**`mail_service` is None + external send:** The `_mailman` thread handles this at dispatch time. If `_is_self_send` is false and `_mail_service is None`, the mailman writes status `"refused"` to `sent/` and exits. The `_send` function does NOT check `mail_service` — validation happens at dispatch. This keeps the pipeline uniform (even a refused message passes through outbox → sent).

**Self-send notification:** `_mailman` calls `_persist_to_inbox()` + `agent._mail_arrived.set()` for self-send — same as the current direct self-send path. No push notification via `_on_normal_mail`. The agent must check its mailbox to see self-sent notes. This is intentional — self-send is for persistent notes, not urgent alerts.

**Attachments with self-send:** Self-send writes raw file paths (no base64 encoding). Same filesystem, same agent — no transport encoding needed. This matches current behavior.

**Attachments with delay:** Resolved to absolute paths at `_send` time (before outbox write). If the file is moved/deleted before the mailman dispatches, TCPMailService will fail at send time — the mailman writes status `"refused"` to sent.

## Email Capability Changes

### New: Archive

`archive` action added to the email capability's 10 existing actions (`send`, `check`, `read`, `reply`, `reply_all`, `search`, `contacts`, `add_contact`, `remove_contact`, `edit_contact`). Moves `inbox/{uuid}/` → `archive/{uuid}/`. One-way, no unarchive.

Schema addition:

```json
"archive": "Move email(s) from inbox to archive (requires email_id)"
```

The `folder` parameter gains `"archive"` as a valid value for `check`, `read`, `search`.

A `delete` action is also added to the email capability (it does not currently have one), supporting both inbox and archive via the `folder` param.

### New: Delay

`delay` parameter added to email's `send` schema. Passed through to the outbox → mailman pipeline.

### Sent Folder Migration

Email capability currently writes to `sent/` directly in `EmailManager._send` (lines 352-365). This is replaced by the mailman pipeline — the mailman writes to `sent/` after dispatch.

Email capability's `_list_emails("sent")` and `_load_email()` (which checks sent/) continue to work — they read from the same `mailbox/sent/` directory, now populated by the mailman instead of by `EmailManager._send`.

### Email `_send` Refactor

Instead of calling `self._agent._mail_service.send(addr, payload)` directly per recipient (line 345), each recipient's delivery goes through the outbox → mailman pipeline. Email imports `_persist_to_outbox` and `_mailman` from `intrinsics.mail` (extending its existing imports from that module).

Per-recipient dispatch, one sent record:

1. For each recipient in `to + cc + bcc`: write to outbox, spawn mailman thread (one thread per recipient)
2. Email writes ONE sent record to `sent/` itself (preserving the "one email" semantic — a CC-to-5 email is one sent item, not five). The mailman threads for email sends skip the `_move_to_sent` step.
3. Keep: privilege gate, duplicate detection, private mode, contacts validation, CC/BCC splitting
4. Return shape changes to `{"status": "sent", "to": [...], "cc": [...], "bcc": [...], "delay": N}`
5. Remove the `mail_service is None` guard — self-sends work without a mail service. The mailman handles the `mail_service is None` case at dispatch time.

To distinguish mail-intrinsic sends (mailman writes to sent/) from email-capability sends (email writes to sent/), the `_mailman` function accepts an optional `skip_sent=True` flag. When true, the mailman dispatches but does not move to sent — the caller handles sent-record creation.

### Disk Layout (email capability)

```
mailbox/
  inbox/              ← received (unchanged)
  outbox/             ← transient (mailman)
  sent/               ← dispatched (now populated by mailman, read by email for folder="sent")
  archive/            ← agent-archived emails (new)
    {uuid}/
      message.json
  read.json           ← read tracking (unchanged)
  contacts.json       ← contact book (unchanged)
```

## What Does NOT Change

- `inbox/` handling — all inbox actions in mail intrinsic untouched
- `_is_self_send`, `_persist_to_inbox` — still exist, called by `_mailman` instead of `_send`
- `TCPMailService` — no changes
- `BaseAgent` — no changes to `_on_mail_received`, `_on_normal_mail`, notification pipeline
- `BaseAgent.mail()` — public API still calls `_send` via intrinsic, gets new return shape
- Email contacts, reply, reply_all — unchanged (reply/reply_all call `_send` which now goes through outbox)
- Gmail addon — out of scope, future work

## Files Changed

| File | Change |
|------|--------|
| `intrinsics/mail.py` | Add `delay` to SCHEMA. Rewrite `_send` → outbox + spawn `_mailman`. Add `_persist_to_outbox`, `_mailman`, `_move_to_sent`. Fix docstring (5 actions, not 6). New imports: `threading`, `timedelta`. |
| `capabilities/email.py` | Add `archive` + `delete` actions, `folder="archive"`. Add `delay` to SCHEMA. Import `_persist_to_outbox`, `_mailman` from mail intrinsic. Route per-recipient dispatch through outbox → `_mailman(skip_sent=True)`. Write one sent record per logical email. Remove direct `_mail_service.send()` calls and sent-folder write. Remove `mail_service is None` guard. |
