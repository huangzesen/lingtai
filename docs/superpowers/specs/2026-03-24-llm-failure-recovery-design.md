# LLM Failure Recovery — Unified AED

## Problem

Three components independently damage conversation history on LLM failure:

1. **`_send_with_retry`** (`llm_utils.py`) — calls `on_reset` multiple times per send attempt, each invocation eats a conversation turn-pair. Different error types (timeout, desync, bad_request, precondition, retryable) have separate code paths with inconsistent guards. Cascade failure during a provider outage progressively strips the entire conversation.

2. **`_on_reset`** (`session.py`) — called by `_send_with_retry` on each reset, drops the trailing assistant turn + tool results every time. No protection against being called repeatedly.

3. **`_perform_aed`** (`base_agent.py`) — the heartbeat's stuck-agent recovery sets `self._session.chat = None`, wiping all conversation history.

Additionally, the rollback message in `_on_reset` says "Data already fetched is still available in memory" — this is false; the tool results are dropped from the conversation.

## Design

### Principle

**History is sacred.** LLM failure never destroys conversation history. The only thing that gets removed is the orphaned tool call from the failed request — the incomplete assistant turn and its tool results that were never successfully processed.

### Unified recovery flow

All LLM failures — regardless of type — follow the same process:

```
send fails (any reason)
  → drop orphaned tool call (trailing assistant turn + its tool_results, idempotent)
  → rebuild session via _rebuild_session (current config, preserved history)
  → retry with exponential backoff: 5s → 10s → 20s → 40s → 80s
  → total elapsed > 5 min → raise TimeoutError
  → agent goes STUCK → heartbeat detects → ASLEEP
  → history intact, wakes on next incoming message (mail/imap/telegram)
```

No special cases per error type. No separate code paths. One mechanism.

### Changes

#### 1. `_send_with_retry` (`llm_utils.py`)

**Current:** ~150 lines of branching logic with 5 error-type-specific paths, per-type guards (`_bad_request_reset_done`, `_desync_reset_done`), a `_SESSION_RESET_THRESHOLD` that allows multiple resets, and an `on_reset` callback that gets called repeatedly.

**New:** Simple loop with exponential backoff. On any failure:
- Call `on_reset(chat, message)` once to clean the orphaned tool call and rebuild session. The callback is idempotent — safe to call on every retry since it only drops trailing orphans (if none exist, it's a no-op).
- Wait with exponential backoff (5s, 10s, 20s, 40s, 80s).
- If total elapsed time exceeds 5 minutes (300s), stop retrying and raise `TimeoutError`.
- Remove: `_SESSION_RESET_THRESHOLD`, `_bad_request_reset_done`, `_desync_reset_done`, all per-error-type branches. Remove `_is_history_desync_error`, `_is_precondition_error`, `_is_bad_request_error` error classification functions (or keep them only for logging, not for flow control).

The `on_reset` callback signature stays the same: `(chat, message) -> (new_chat, new_message)`.

#### 2. `_on_reset` (`session.py`)

**Current:** Drops trailing assistant turn + tool results, rebuilds session. Not idempotent — calling twice drops two turn-pairs.

**New:** Make idempotent. Only drop trailing entries if they are orphaned (i.e., the trailing assistant turn contains tool calls whose results were not followed by a successful assistant response). If the history is already clean (ends with a complete user→assistant exchange), drop nothing. Rebuild session regardless (harmless — just refreshes the session object with current config).

Fix rollback message: "Your previous response was lost due to a server error. Tool results from that exchange were also lost."

#### 3. `_perform_aed` (`base_agent.py`)

**Current:** `self._session.chat = None` — wipes all history.

**New:** `self._session._rebuild_session(self._session.chat.interface)` — preserves history, refreshes session. Keep the recovery message injection.

#### 4. Constants

- Remove `_SESSION_RESET_THRESHOLD` (no longer needed).
- Add `_AED_TOTAL_TIMEOUT = 300` (5 minutes total retry budget).
- Add `_AED_BACKOFF_BASE = 5` (first retry delay in seconds).
- Keep `_LLM_MAX_RETRIES` as a hard cap on number of attempts (safety net), but the 5-minute total timeout is the primary control.

## Non-changes

- `cpr_timeout` (20 min default) stays as-is — it's the heartbeat's timer for STUCK → ASLEEP transition, separate from the retry timeout.
- `_rebuild_session` — already implemented, no changes needed.
- The agent state machine (ACTIVE → STUCK → ASLEEP) — no changes. The retry mechanism raises after 5 min, the message handler catches the exception and sets STUCK, heartbeat eventually transitions to ASLEEP.
