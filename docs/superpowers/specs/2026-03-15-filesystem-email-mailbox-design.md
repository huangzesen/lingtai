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
- `read.json` at the mailbox root tracks which email IDs have been read.

## Changes

### `services/mail.py`

- Change the persist path from `mailbox/{uuid}/` to `mailbox/inbox/{uuid}/`.
- Inject `_mailbox_id` (the UUID) into the payload dict before calling `on_message(payload)`, so the email capability can use the directory name as the email ID without generating a separate one.

### `capabilities/email.py` — `EmailManager`

**Remove:**
- `self._mailbox: list[dict]` (in-memory storage)
- `self._mailbox_lock` (threading lock)

**Add:**
- `_mailbox_dir` property → `self._agent._working_dir / "mailbox"`
- `_read_ids()` / `_mark_read(email_id)` — load/update `read.json`

**Modified actions:**

| Action | Current | New |
|--------|---------|-----|
| `_send()` | Delivers via mail service only | Also saves copy to `mailbox/sent/{uuid}/message.json` |
| `_check()` | Iterates in-memory list | Scans `mailbox/{folder}/*/message.json`, sorts by timestamp, returns newest N. Default folder: `inbox` |
| `_read()` | Linear scan of in-memory list | Loads `mailbox/{folder}/{id}/message.json` (searches both folders), marks read in `read.json` |
| `_lookup()` | Linear scan of in-memory list | Loads single `message.json` by ID across both folders |
| `on_mail_received()` | Appends to in-memory list, generates separate ID | Reads `_mailbox_id` from payload, sends notification to agent inbox. No storage needed (mail service already wrote the file). |

**New action:**

| Action | Behavior |
|--------|----------|
| `_search(query, folder=None)` | Regex scan across `message.json` files. Matches against `from`, `subject`, and `message` fields. Optional `folder` param filters to `inbox` or `sent`; omit to search both. Returns matching email summaries (same format as `check`). |

### Schema Changes

- Add `"search"` to the `action` enum (already partially done).
- Add `query` property: `{"type": "string", "description": "Regex pattern for search (matches from, subject, message)"}`.
- Add `folder` property: `{"type": "string", "enum": ["inbox", "sent"], "description": "Folder to check or search. Default: inbox for check, both for search."}`.
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

The in-memory lock is removed. Filesystem operations are inherently atomic at the directory level (each email is its own directory). `read.json` is the only shared mutable file — use atomic write (write to temp file, rename) to avoid corruption.

## Backward Compatibility

- The mail service currently writes to `mailbox/{uuid}/`. This changes to `mailbox/inbox/{uuid}/`. Existing mailbox directories from before this change won't be found in the new path. This is acceptable — agent mailboxes are ephemeral within a session.
- All existing actions (send, check, read, reply, reply_all) preserve their interface. Only internal implementation changes.
