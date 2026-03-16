# Cancel Email Intrinsic — Design Spec

## Problem

StoAI agents run synchronous tool-call loops. There is no way to externally interrupt an agent mid-work via the mail system. The current `cancel_event` is an externally injected `threading.Event` — a low-level threading primitive that doesn't fit the StoAI communication model where all interactions (including with users) happen via email.

## Goal

Allow any authorized party (host app, orchestrator, or admin agent) to send a special `type: "cancel"` email that immediately stops the agent's current work. The agent writes a diary entry summarizing where it left off, then returns to waiting for new messages.

## Design

### 1. Mail `type` Field

Mail payloads gain a `type` field with two values:

- `"normal"` (default) — regular inter-agent communication. Queued in FIFO, agent reads when it chooses.
- `"cancel"` — control signal. Triggers immediate cancellation of the agent's current work.

The field is extensible for future control signal types.

**Wire format change:**

The `type` key is added to the message dict payload (the second argument to `MailService.send(address, message)`). This is consistent with how `subject`, `message`, `from`, etc. are already keys inside the dict. No change to the `MailService.send()` ABC signature.

```python
# Caller includes type in the message dict
mail_service.send(address, {"from": "boss", "subject": "stop", "message": "halt work", "type": "cancel"})
```

Default is `"normal"` — all existing code that omits the key works unchanged (`payload.get("type", "normal")`).

### 2. Cancel Event Refactor

**Removed:**
- `cancel_event` constructor parameter on `BaseAgent`
- `MSG_CANCEL` message type
- `_handle_cancel()` method
- `cancel_event` parameter from `llm_utils.py` functions (`send_with_timeout`, `send_with_timeout_stream`, `_send_with_retry`)

**Added:**
- `self._cancel_event = threading.Event()` — always created internally by the agent
- `self._cancel_mail: dict | None = None` — stores the cancel email payload for the diary flow

The `_cancel_event` is checked at these interruption points:
- In `_process_response()` — before executing tool calls (between LLM rounds) — **existing check**
- In `_execute_tools_parallel()` — between futures — **existing check**
- In `_execute_tools_sequential()` — between individual tool calls — **new check to add** (not currently present)

**Not checked during LLM calls.** The `cancel_event` parameter is removed entirely from `llm_utils.py` functions, so the LLM polling loop no longer checks for cancellation. LLM calls always run to completion or timeout naturally. This is an intentional trade-off: we prefer a clean cancellation model (check between rounds) over mid-LLM-call interruption. If a cancel email arrives while an LLM call is in flight, the call finishes, then the cancel check fires on the next loop iteration.

### 3. `_on_mail_received` Changes

```python
def _on_mail_received(self, payload: dict) -> None:
    mail_type = payload.get("type", "normal")

    if mail_type == "cancel":
        if self._cancelling:
            return  # already in diary flow, ignore
        # Control signal — don't queue, don't notify via inbox
        self._cancel_mail = payload  # store for diary
        self._cancel_event.set()
        self._log("cancel_received", sender=payload.get("from"), subject=payload.get("subject"))
        return

    # Normal mail — existing behavior unchanged
    # (queue in _mail_queue, put MSG_REQUEST in inbox)
    ...
```

Cancel emails:
- Bypass the `_mail_queue` entirely
- Bypass the inbox notification
- Store the payload in `self._cancel_mail` for the diary prompt
- Set `self._cancel_event` to signal the tool loop

### 4. Diary Flow

When `_cancel_event` fires during `_process_response()`:

1. **Strip pending tool calls** — discard any tool results from the current batch. Don't commit them to chat history. In `_execute_tools_parallel`, when `_cancel_event` is detected, return empty results (don't build result messages from completed futures). In `_execute_tools_sequential`, break out of the loop and return empty results.
2. **Set `_cancelling` flag** — set `self._cancelling = True` to prevent re-entrant cancellation (a second cancel email arriving during the diary call). Reset `_cancel_event` so the diary LLM call isn't itself cancelled.
3. **One final LLM call** — send a user message to the LLM via `self._chat.send()` directly (not through `_process_response`). Extract only the text from the response; ignore any tool calls the LLM may return. Log a `cancel_diary` event.

   ```
   [CANCELLED] You have been stopped by a cancel email.
   From: {sender}
   Subject: {subject}
   Message: {content}

   Write a brief diary entry summarizing what you were working on
   and where you left off, so you can resume later.
   ```

4. **Return the diary text** as the response: `{"text": diary_text, "failed": False, "errors": []}`.
5. **Clear state** — set `self._cancel_mail = None`, `self._cancelling = False`.

The agent then naturally returns to waiting for the next inbox message (SLEEPING state).

### 5. `admin` Privilege Gate

A new `admin: bool = False` parameter on the `BaseAgent` constructor controls whether the agent can send cancel emails.

- `admin=False` (default): The mail intrinsic does not expose the `type` field. If the LLM passes `type: "cancel"`, it is silently downgraded to `"normal"` or returns an error.
- `admin=True`: The mail intrinsic schema includes the `type` field, allowing the LLM to send cancel emails to other agents.

This is a creation-time privilege — cannot be escalated at runtime. The host app decides which agents are supervisors.

**The host app can always send cancel emails directly** via `MailService.send(address, {"type": "cancel", ...})`, regardless of the `admin` flag. The privilege gate only applies to the LLM-facing mail intrinsic.

**Email capability:** If an admin agent uses the `email` capability (which replaces the mail intrinsic's send action), the `type` field gate must apply there too. The `email` capability's `setup()` should read the agent's `admin` flag to decide whether to expose the `type` field.

### 6. MailService Protocol Changes

**No ABC signature change.** The `type` field is a key inside the message dict (second argument to `send(address, message)`). The ABC `MailService.send(self, address: str, message: dict) -> bool` is unchanged.

**`TCPMailService`** passes the `type` key through in the wire payload (it already forwards the full dict).

**Receive side:** No changes needed — `_on_mail_received` already receives the full payload dict.

**Unrecognized types:** `_on_mail_received` should log a warning for unrecognized `type` values and treat them as `"normal"`.

## Scope of Changes

| File | Change |
|------|--------|
| `agent.py` | Remove `cancel_event` param, add `admin` param, create internal `_cancel_event`, add `_cancel_mail` and `_cancelling`, modify `_on_mail_received`, modify cancel check in `_process_response` for diary flow, add cancel check in `_execute_tools_sequential`, modify `_execute_tools_parallel` to return empty on cancel, remove `MSG_CANCEL` / `_handle_cancel`, update `_llm_send` and `_llm_send_streaming` to stop passing `cancel_event` to `llm_utils` |
| `llm_utils.py` | Remove `cancel_event` param from `send_with_timeout`, `send_with_timeout_stream`, `_send_with_retry`. Remove cancel polling loop and `_CancelledDuringLLM` exception class in `_send_with_retry` |
| `services/mail.py` | No ABC change. Document that `type` is a key in the message dict |
| `intrinsics/mail.py` (or agent mail handler) | Conditionally expose `type` field in schema based on `admin` flag, enforce privilege gate |
| `capabilities/email.py` | Respect `admin` flag — only expose `type` field for admin agents |
| Tests | Update existing cancel tests, add tests for cancel email flow, diary flow, admin privilege gate |

## Non-Goals

- Interrupting mid-tool-call (e.g., killing a running bash command). The cancel takes effect between tool calls.
- Interrupting mid-LLM-call. The LLM call finishes, then the cancel fires.
- Multi-level cancel types (e.g., soft stop vs hard kill). One type: `"cancel"`.
- Persisting the diary to disk. It's returned as the response text — the caller can persist if needed.
