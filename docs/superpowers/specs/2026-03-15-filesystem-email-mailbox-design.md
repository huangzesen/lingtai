# Filesystem-based Email Mailbox

**Date:** 2026-03-15
**Status:** Approved

## Problem

The email capability stores all messages in-memory (`self._mailbox: list[dict]`), while the mail service already persists received messages to `working_dir/mailbox/{uuid}/message.json`. This creates duplication, loses messages on restart, and doesn't record sent emails. Sent emails are a natural form of agent long-term memory.

## Storage Layout

```
working_dir/
  mailbox/
    inbox/
      {uuid}/
        message.json
        attachments/
    sent/
      {uuid}/
        message.json
        attachments/
    read.json          ← JSON array of read email IDs
```

- Each email is a directory named by UUID, containing `message.json` and optionally an `attachments/` subdirectory.
- `inbox/` holds received messages (written by the mail service).
- `sent/` holds outgoing messages (written by the email capability on send).
- `read.json` at the mailbox root tracks which inbox email IDs have been read. Sent emails are inherently "read" and not tracked here.

## Changes

### `services/mail.py`

- Change the persist path from `mailbox/{uuid}/` to `mailbox/inbox/{uuid}/`.
- Inject `_mailbox_id` (the UUID) and `received_at` (ISO timestamp) into the payload dict before writing `message.json` and before calling `on_message(payload)`. This ensures every persisted message has a timestamp for sorting and an ID for lookup.
- If `_mailbox_id` is absent from the payload in `on_mail_received` (e.g. non-TCP mail service), fall back to generating a UUID.

### `capabilities/email.py` — `EmailManager`

**Remove:**
- `self._mailbox: list[dict]` (in-memory storage)
- `self._mailbox_lock` (threading lock)

**Add:**
- `_mailbox_dir` property → `self._agent._working_dir / "mailbox"`
- `_read_ids()` / `_mark_read(email_id)` — load/update `read.json` with atomic write (write temp, rename)
- `_load_email(email_id)` — direct path check: try `inbox/{id}/message.json`, then `sent/{id}/message.json`. O(1) per folder, not a directory scan.

**Modified actions:**

| Action | Current | New |
|--------|---------|-----|
| `_send()` | Delivers via mail service only | Also saves copy to `mailbox/sent/{uuid}/message.json` with timestamp, BCC (for sender's records), and email ID |
| `_check(folder="inbox")` | Iterates in-memory list | Scans `mailbox/{folder}/*/message.json`, sorts by `received_at` (or `sent_at`), returns newest N |
| `_read(email_id)` | Linear scan of in-memory list | Direct path lookup across both folders, marks read in `read.json` (inbox only) |
| `_lookup(email_id)` | Linear scan of in-memory list | Direct path lookup: `inbox/{id}/message.json` else `sent/{id}/message.json` |
| `on_mail_received()` | Appends to in-memory list, generates separate ID | Reads `_mailbox_id` from payload (falls back to uuid4 if absent), sends notification to agent inbox. No storage needed — mail service already wrote the file. |

**New action:**

| Action | Behavior |
|--------|----------|
| `_search(query, folder=None)` | Regex scan across `message.json` files. Matches against `from`, `subject`, and `message` fields. Optional `folder` param filters to `inbox` or `sent`; omit to search both. Returns matching email summaries (same format as `check`). |

### Schema Changes

- Add `"search"` to the `action` enum.
- Add `query` property: `{"type": "string", "description": "Regex pattern for search (matches from, subject, message)"}`.
- Add `folder` property: `{"type": "string", "enum": ["inbox", "sent"], "description": "Folder for check/search. Default: inbox for check, both for search."}`.
- Update action descriptions to mention folder and search.

### DESCRIPTION Update

Mention search capability and inbox/sent folder structure.

## Search Implementation

Simple Python `re.search()` over each `message.json`:
1. Scan `mailbox/{folder}/*/message.json` (or both folders if folder is None).
2. For each file, load JSON, concatenate `from + subject + message` fields.
3. Apply `re.search(query, combined_text, re.IGNORECASE)`.
4. Return matching emails as summaries (id, from, to, subject, preview, time, unread).

No external dependencies. Sufficient for agent-to-agent message volumes.

## Thread Safety

The in-memory lock is removed. Filesystem operations are inherently atomic at the directory level (each email is its own directory). `read.json` is the only shared mutable file — use atomic write (write to temp file, rename) to avoid corruption. Tool calls are serialized within a single agent (sequential processing loop), so concurrent read.json updates cannot occur.

## Directory Creation

Directories are created lazily on first use. `_check()` and `_search()` handle the case where `inbox/` or `sent/` doesn't exist yet by returning empty results.

## Architectural Note

The email capability writes directly to the filesystem for sent emails (via `_send()`), rather than going through the mail service. This is intentional — the mail service is a transport layer (TCP delivery), not a storage layer. Sent email persistence is a capability concern.

## Test Migration

Existing tests in `test_layers_email.py` directly access `mgr._mailbox` and populate it via `on_mail_received`. All such tests need rewriting to use `tmp_path` fixtures with the filesystem layout. Tests for `on_mail_received` must either write `message.json` to disk first (simulating what the mail service does) or inject `_mailbox_id` into the payload.

## Backward Compatibility

- The mail service persist path changes from `mailbox/{uuid}/` to `mailbox/inbox/{uuid}/`. Existing directories won't be found. Acceptable — agent mailboxes are ephemeral within a session.
- All existing actions (send, check, read, reply, reply_all) preserve their interface. Only internal implementation changes.
- The base agent's `_mail_queue` (used by the `mail` intrinsic when email capability is not installed) is unchanged.
