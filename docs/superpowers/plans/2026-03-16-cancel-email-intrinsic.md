# Cancel Email Intrinsic — Implementation Plan

Spec: `docs/superpowers/specs/2026-03-16-cancel-email-intrinsic-design.md`

## Steps

### Step 1: Remove `cancel_event` from `llm_utils.py`

**Files:** `src/lingtai/llm_utils.py`

- Remove `cancel_event` param from `_send_with_retry`, `send_with_timeout`, `send_with_timeout_stream`
- Remove the `_CancelledDuringLLM` exception class
- Remove the cancel polling check inside `_send_with_retry` (the `if cancel_event and cancel_event.is_set()` block)
- Remove the `except _CancelledDuringLLM: raise` handler

### Step 2: Refactor `cancel_event` in `agent.py`

**Files:** `src/lingtai/agent.py`

- Remove `cancel_event` constructor parameter
- Add `admin: bool = False` constructor parameter, store as `self._admin`
- Create `self._cancel_event = threading.Event()` internally (always)
- Add `self._cancel_mail: dict | None = None`
- Add `self._cancelling: bool = False`
- Remove `MSG_CANCEL` constant and `_handle_cancel()` method
- Remove `MSG_CANCEL` dispatch in `_run_loop`
- Update `_llm_send` and `_llm_send_streaming` to stop passing `cancel_event` to `llm_utils` functions

### Step 3: Modify `_on_mail_received` for cancel emails

**Files:** `src/lingtai/agent.py`

- At the top of `_on_mail_received`, check `payload.get("type", "normal")`
- If `"cancel"` and not `self._cancelling`: store payload in `self._cancel_mail`, set `self._cancel_event`, log `cancel_received`, return early
- If unrecognized type: log warning, treat as normal
- Normal mail path unchanged

### Step 4: Implement diary flow in `_process_response`

**Files:** `src/lingtai/agent.py`

- At the existing cancel check (line ~923), when `_cancel_event` is set and `_cancel_mail` is not None:
  - Set `self._cancelling = True`
  - Clear `self._cancel_event`
  - Build the diary prompt from `_cancel_mail` (sender, subject, message)
  - Send one LLM call via `self._chat.send()` directly, extract text only (ignore tool calls)
  - Log `cancel_diary`
  - Clear `self._cancel_mail`, set `self._cancelling = False`
  - Return `{"text": diary_text, "failed": False, "errors": []}`

### Step 5: Add cancel checks in tool execution paths

**Files:** `src/lingtai/agent.py`

- In `_execute_tools_sequential`: add `_cancel_event` check between individual tool calls. If set, return empty results `([], False, "")`
- In `_execute_tools_parallel`: when cancel is detected (existing break), return empty results instead of building partial results

### Step 6: Admin privilege gate for mail intrinsic

**Files:** `src/lingtai/agent.py` (mail handler), `src/lingtai/capabilities/email.py`

- In `_handle_mail` (send action): if `self._admin` is True, allow `type` field in args; if False, strip or ignore `type` field
- In email capability `setup()`: read agent's `_admin` flag, conditionally include `type` in schema
- Update mail intrinsic SCHEMA to conditionally include `type` (or use two schema variants)

### Step 7: Tests

**Files:** `tests/test_cancel_email.py` (new)

- Test: cancel email sets `_cancel_event` and stores mail
- Test: cancel email bypasses mail queue
- Test: diary flow produces LLM call and returns text
- Test: cancel during sequential tool execution stops between calls
- Test: cancel during parallel tool execution returns empty
- Test: `_cancelling` flag prevents re-entrant cancel
- Test: non-admin agent cannot send cancel emails
- Test: admin agent can send cancel emails
- Test: unrecognized mail type logged as warning, treated as normal
