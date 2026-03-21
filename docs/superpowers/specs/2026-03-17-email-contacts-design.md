# Email Contacts Design

**Goal:** Add a persistent contact book to the email capability so agents can explicitly register, update, remove, and list known peers.

**Assumes:** Agent identity refactor is complete (`agent_id` = auto-generated UUID, `agent_name` = human label).

---

## Storage

Single file: `working_dir/mailbox/contacts.json`

```json
[
  {"agent_id": "a1b2c3d4e5f6", "agent_name": "alice", "address": "127.0.0.1:8301", "note": "research specialist"},
  {"agent_id": "c3d4e5f6a1b2", "agent_name": "bob", "address": "127.0.0.1:8302", "note": ""}
]
```

Each contact has four fields:
- `agent_id` (string, required) — the contact's UUID, used as the unique key
- `agent_name` (string, required) — human-readable label
- `address` (string, required) — mail service address (e.g. `127.0.0.1:8301`)
- `note` (string, optional) — free-text note, defaults to `""`

## New actions on the email tool

| Action | Required params | Optional params | Behavior |
|--------|----------------|-----------------|----------|
| `add_contact` | `agent_id`, `agent_name`, `address` | `note` | Upsert by `agent_id`. If contact exists, overwrite all fields. |
| `remove_contact` | `agent_id` | — | Remove by `agent_id`. Error if not found. |
| `edit_contact` | `agent_id` | `agent_name`, `address`, `note` | Update only the provided fields on an existing contact. Error if `agent_id` not found. |
| `contacts` | — | — | Return the full contact list. |

## Schema changes

Add to the existing `SCHEMA["properties"]`:

```python
"action": {
    "enum": [...existing..., "add_contact", "remove_contact", "edit_contact", "contacts"],
}
"agent_id": {
    "type": "string",
    "description": "Contact's agent ID (for add_contact, remove_contact, edit_contact)",
}
"agent_name": {
    "type": "string",
    "description": "Contact's human-readable name (for add_contact, edit_contact)",
}
"note": {
    "type": "string",
    "description": "Free-text note about the contact (for add_contact, edit_contact)",
}
```

The existing `"address"` property already exists in the schema (used by `send`). It is reused for `add_contact` and `edit_contact`.

## What doesn't change

- `send`, `reply`, `reply_all` — no address resolution, no auto-population from contacts
- Mail intrinsic — contacts are email-capability only
- Contact management is purely explicit — the agent decides when to add/remove/edit

## File changes

| Action | Path |
|--------|------|
| Modify | `src/lingtai/capabilities/email.py` |

## Implementation notes

- Contacts file is read/written with the same atomic-write pattern used by `read.json` (tempfile + `os.replace`)
- `contacts.json` is created lazily on first `add_contact`, not at capability setup
- `contacts` action returns `{"status": "ok", "contacts": [...]}` (empty list if file doesn't exist)
