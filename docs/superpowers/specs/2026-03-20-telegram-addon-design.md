# Telegram Addon Design Spec

## Overview

A Telegram Bot API addon for 灵台, enabling agents to interact with Telegram users for customer service. Follows the IMAP addon's three-class pattern (Account, Service, Manager). Supports multi-account (multiple bot tokens), text + images + documents, inline keyboards, and long polling for updates.

**Standalone tool** — no bridge to the inter-agent mail system. The `telegram` tool is a separate channel, like how humans treat email and Telegram as naturally different. `TelegramService` does not implement `MailService` — it is not a mail transport. There is no TCP bridge.

## Architecture

### File Structure

```
src/lingtai/addons/telegram/
├── __init__.py      # setup(agent, **kwargs), config parsing
├── account.py       # TelegramAccount — per-bot connection + polling
├── service.py       # TelegramService — multi-account orchestrator
└── manager.py       # TelegramManager — tool dispatch + filesystem
```

### Responsibility Split

| Class | Owns | Lifecycle |
|-------|------|-----------|
| `TelegramAccount` | One bot token, one polling thread, HTTP calls to Bot API | `start()` begins polling, `stop()` terminates |
| `TelegramService` | Registry of TelegramAccounts, routes sends to correct account | `start()` starts all accounts, `stop()` stops all |
| `TelegramManager` | Filesystem (inbox/sent/contacts), tool action dispatch, message persistence | `start()`/`stop()` delegate to service |

### Data Flow

1. `TelegramAccount` polls via `getUpdates` → receives update → calls `on_message` callback
2. `TelegramService.on_message` → passes to manager with account alias
3. `TelegramManager.on_incoming` → persists to disk, downloads media, notifies agent
4. Agent sees notification → uses `telegram` tool to read/reply
5. `TelegramManager.handle()` dispatches action → `TelegramService` routes to correct account → `TelegramAccount` sends via Bot API

### Dependency

`httpx` for Bot API HTTP calls. Chosen over stdlib `urllib.request` for multipart file upload ergonomics (photo/document sends) and connection pooling for long-poll requests. No Telegram SDK needed — the Bot API is straightforward REST. Added as optional: `pip install lingtai[telegram]`.

## Tool Schema

Single `telegram` tool with these actions:

| Action | Purpose | Key params |
|--------|---------|------------|
| `send` | Send message to a chat | `account` (optional), `chat_id`, `text`, `media` (optional), `reply_markup` (optional) |
| `check` | List recent conversations with unread counts | `account` (optional, all if omitted) |
| `read` | Read messages from a chat. Returns messages with compound IDs that can be used for `reply`. | `account` (optional), `chat_id`, `limit` (default 10) |
| `reply` | Reply to a specific message (use compound ID from `read` results) | `account` (optional), `message_id`, `text`, `media` (optional) |
| `search` | Search messages by text/sender | `account` (optional), `query`, `chat_id` (optional) |
| `delete` | Delete a message the bot sent. In groups, can delete user messages only if bot is admin. | `account` (optional), `message_id` |
| `edit` | Edit a text message or caption the bot sent. Uses `editMessageText` for text messages, `editMessageCaption` for media with captions. | `account` (optional), `message_id`, `text`, `reply_markup` (optional) |
| `contacts` | List known contacts/chats | `account` (optional) |
| `add_contact` | Save a chat with alias | `account` (optional), `chat_id`, `alias` |
| `remove_contact` | Remove a saved contact | `account` (optional), `alias` or `chat_id` |
| `accounts` | List configured bot accounts | (none) |

`account` is optional on all actions — defaults to the first configured account. In single-account setups the agent never needs to specify it.

### Message ID Format

Compound `{account}:{chat_id}:{message_id}` — mirrors IMAP's `{account}:{folder}:{uid}` pattern.

### Reply Markup (Inline Keyboards)

```json
{"inline_keyboard": [[{"text": "Yes", "callback_data": "yes"}, {"text": "No", "callback_data": "no"}]]}
```

Callback query updates (button presses) are received as a special message type so the agent can react to customer choices.

### Media Param (for send/reply)

```json
{"type": "photo|document", "path": "/path/to/file"}
```

## Filesystem Layout

```
working_dir/telegram/
└── {account_alias}/
    ├── inbox/
    │   └── {uuid}/
    │       ├── message.json
    │       └── attachments/
    │           └── photo.jpg
    ├── sent/
    │   └── {uuid}/
    │       └── message.json
    ├── contacts.json          # per-account contacts
    ├── read.json              # set of read message compound IDs
    └── state.json             # last update_id + bot info (getMe cache)
```

Callback queries (inline keyboard button presses) are stored as regular inbox messages with a `callback_query` field — no separate file needed.

### Inbox message.json

```json
{
  "id": "bot1:12345:678",
  "from": {"id": 12345, "username": "customer1", "first_name": "Alice"},
  "chat": {"id": 12345, "type": "private"},
  "date": "2026-03-20T10:30:00Z",
  "text": "Hello, I need help with my order",
  "media": null,
  "reply_to_message_id": null,
  "callback_query": null
}
```

### Sent message.json

```json
{
  "id": "bot1:12345:679",
  "to": {"chat_id": 12345},
  "date": "2026-03-20T10:30:15Z",
  "text": "Hi Alice! I'd be happy to help. What's your order number?",
  "media": null,
  "reply_markup": null,
  "sent_at": "2026-03-20T10:30:15Z",
  "status": "sent"
}
```

### contacts.json (per account)

```json
{
  "alice": {"chat_id": 12345, "username": "customer1", "first_name": "Alice"},
  "support_group": {"chat_id": -100999, "type": "group"}
}
```

## TelegramAccount

```python
class TelegramAccount:
    def __init__(self, alias: str, bot_token: str, allowed_users: list[int] | None,
                 poll_interval: float = 1.0, on_message: Callable):
        ...
    def start(self) -> None: ...   # calls getMe (caches bot info), starts polling thread
    def stop(self) -> None: ...    # signals polling thread to stop, joins
```

Constructor stores config only — no threads, no API calls. `start()` calls `getMe` to cache the bot's username/id in `state.json`, then spawns the polling daemon thread. `stop()` sets a stop event and joins the thread.

### Polling

- Uses `getUpdates` with `offset = last_update_id + 1` and `timeout = 30` (long poll)
- `poll_interval` is the delay between poll cycles (not the long-poll timeout). Default 1s for customer service responsiveness.
- Filters by `allowed_users` (if set), calls `on_message(update)` for each accepted update
- Updates `last_update_id` after processing

### Update Types Handled

- `message` — text, photo, document
- `callback_query` — inline keyboard button press (auto-answers the callback to dismiss the spinner)
- `edited_message` — customer edits their message (stored as update to existing message)

### Sending Methods

- `send_message(chat_id, text, reply_markup=None, reply_to_message_id=None)` → `sendMessage`
- `send_photo(chat_id, photo_path, caption=None)` → `sendPhoto` (multipart upload)
- `send_document(chat_id, doc_path, caption=None)` → `sendDocument`
- `edit_message(chat_id, message_id, text, reply_markup=None)` → `editMessageText`
- `delete_message(chat_id, message_id)` → `deleteMessage`

### Media Download

Incoming photos/documents: call `getFile` → download via `file_path` URL → save to attachments dir.

### Error Handling

- HTTP 429 (rate limit): respect `retry_after` from Telegram's response
- Connection errors: log warning, continue polling after backoff
- `allowed_users` filter: silently drop messages from unknown users

### Thread Model

One daemon thread per account running the poll loop, started/stopped by `start()`/`stop()`.

## TelegramService

```python
class TelegramService:
    def __init__(self, working_dir: Path, accounts_config: list[dict],
                 on_message: Callable):
        ...
```

### Responsibilities

- Creates one `TelegramAccount` per config entry
- Routes outbound sends to the correct account by alias
- Provides unified `on_message` callback that tags each incoming update with the account alias before passing to manager

### Account Config Format

```python
[
    {
        "alias": "support_bot",
        "bot_token": "123456:ABC...",
        "allowed_users": [111, 222],   # optional, None = accept all
        "poll_interval": 1.0,          # optional, default 1.0
    },
    {
        "alias": "sales_bot",
        "bot_token": "789012:DEF...",
    }
]
```

### API Surface

- `start()` — starts all accounts' polling threads
- `stop()` — stops all accounts
- `get_account(alias) -> TelegramAccount` — for routing sends
- `list_accounts() -> list[str]` — return aliases

## TelegramManager

```python
class TelegramManager:
    def __init__(self, agent, service: TelegramService, working_dir: Path):
        ...
```

### Incoming Message Flow

1. Service calls `manager.on_incoming(account_alias, update)`
2. Manager persists to `telegram/{account}/inbox/{uuid}/message.json`
3. Downloads any media attachments to `telegram/{account}/inbox/{uuid}/attachments/`
4. Notifies agent using the same pattern as IMAP:
   - Set `self._agent._mail_arrived.set()` to wake the agent from sleep
   - Create system notification via `_make_message(MSG_REQUEST, "system", "New telegram message from {username} via {account}")`
   - Put it on `self._agent.inbox`
   - Log via `self._agent._log("telegram_received", ...)`

### Action Dispatch

| Action | Filesystem | Bot API |
|--------|-----------|---------|
| `send` | Write to `sent/{uuid}/` | `sendMessage/sendPhoto/sendDocument` |
| `check` | Scan `inbox/`, cross-ref `read.json` | — |
| `read` | Read from `inbox/`, mark in `read.json` | — |
| `reply` | Write to `sent/{uuid}/` | `sendMessage` with `reply_to_message_id` |
| `search` | Scan `inbox/` messages by text/sender | — |
| `delete` | — | `deleteMessage` |
| `edit` | Update `sent/{uuid}/` | `editMessageText` |
| `contacts` | Read `contacts.json` | — |
| `add_contact` | Write `contacts.json` | — |
| `remove_contact` | Write `contacts.json` | — |
| `accounts` | — | — (return config) |

### Duplicate Send Protection

Count-based blocking (same pattern as IMAP): track `_last_sent` dict keyed by `(account, chat_id, text)` with a count. Block repeated identical sends beyond a configurable free-pass threshold.

### Lifecycle

- `start()` → calls `service.start()`
- `stop()` → calls `service.stop()`

## Configuration

### Setup Function

```python
def setup(agent, *, accounts: list[dict] | None = None,
          bot_token: str | None = None, allowed_users: list[int] | None = None,
          poll_interval: float = 1.0, **kwargs) -> TelegramManager:
    ...
```

`setup()` normalizes config (single-account shorthand → accounts list), creates `TelegramService` and `TelegramManager`, then calls `agent.add_tool("telegram", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION)` to register the tool. Returns the manager (which has `start()`/`stop()` lifecycle hooks called by `Agent`).

### Single Account Shorthand

```python
addons={"telegram": {
    "bot_token": "123456:ABC...",
    "allowed_users": [111, 222],
    "poll_interval": 1.0,
}}
```

Auto-wrapped into `accounts=[{"alias": "default", "bot_token": ..., ...}]`.

### Multi-Account

```python
addons={"telegram": {
    "accounts": [
        {"alias": "support", "bot_token": "123:ABC", "allowed_users": [111]},
        {"alias": "sales", "bot_token": "789:DEF"},
    ]
}}
```

### Addon Registration

```python
# src/lingtai/addons/__init__.py
_BUILTIN = {
    "imap": ".imap",
    "telegram": ".telegram",
}
```

### Optional Dependency

`pip install lingtai[telegram]` adds `httpx` in `pyproject.toml`.

## Testing Strategy

### Test Files

- `tests/test_addon_telegram_account.py` — TelegramAccount unit tests
- `tests/test_addon_telegram_service.py` — TelegramService orchestration tests
- `tests/test_addon_telegram_manager.py` — TelegramManager tool dispatch + filesystem tests

### Mocking

- Mock `httpx` responses for all Bot API calls
- Use `tmp_path` for filesystem assertions
- Mock `agent` as `MagicMock()` with `agent._working_dir = tmp_path`

### Key Test Cases

| Area | Tests |
|------|-------|
| Account | Polling receives updates, filters by allowed_users, handles rate limits, downloads media, send/edit/delete API calls |
| Service | Multi-account routing, start/stop lifecycle, account lookup |
| Manager | Each action (send, check, read, reply, search, delete, edit, contacts), message persistence, read state tracking, duplicate send protection, incoming message notification, compound ID parsing |
| Setup | Single-account shorthand, multi-account config, missing bot_token error |
