# Mail Notifications & Message Concatenation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Uniform mail notifications (from "system", custom text per service), general message concatenation replacing mail-specific collapse, and removal of synchronous reply mechanism (`_reply_event`/`_reply_value`) in favor of async peer communication.

**Architecture:** Mail notifications become simple `[system]` messages with per-service format. The mail-specific `_collapse_mail_notifications` is deleted and replaced by a general `_concat_queued_messages` in the run loop that joins all pending messages into one LLM turn. The `send()` API loses its `wait`/`timeout` synchronous mode — all agents are peers communicating asynchronously via mail + `clock(wait)`.

**Tech Stack:** Python 3.11+, lingtai framework, pytest, unittest.mock

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `src/lingtai/base_agent.py` | **Modify** | Add `_mailbox_name`/`_mailbox_tool` attrs. Rewrite `_on_normal_mail` to use them. Delete `_collapse_mail_notifications`. Add `_concat_queued_messages` in run loop. Remove `_reply_event`/`_reply_value` from `send()`, `_run_loop`, `_deliver_result`. |
| `src/lingtai/message.py` | **Modify** | Remove `_reply_event`, `_reply_value`, `_mail_notification` fields. Remove `reply_event` param from `_make_message`. |
| `src/lingtai/capabilities/email.py` | **Modify** | Set `agent._mailbox_name = "email box"` and `agent._mailbox_tool = "email"` in `setup()`. Remove custom `on_normal_mail` — let BaseAgent's `_on_normal_mail` handle it. |
| `src/lingtai/addons/gmail/manager.py` | **Modify** | Update `on_gmail_received` notification to use `[system]` sender and "gmail box" format. Gmail has its own listener path (IMAP → `on_gmail_received`), separate from TCP mail's `_on_mail_received` → `_on_normal_mail`. So it keeps its own notification method but adopts the uniform format. |
| `tests/test_agent.py` | **Modify** | Update notification assertion tests. Remove `_reply_event` tests. Update `send()` tests. |
| `tests/test_message.py` | **Modify** | Remove `_reply_event` test. |
| `tests/test_silence_kill.py` | **Modify** | Minor — verify notifications still work after format change. |
| `tests/test_layers_email.py` | **Modify** | Update notification assertions for new format. |
| `tests/test_addon_gmail_manager.py` | **Modify** | Update notification assertions for new format. |

---

### Task 1: Uniform mail notifications via `_mailbox_name` / `_mailbox_tool`

**Files:**
- Modify: `src/lingtai/base_agent.py:202,489-515` (add attrs, rewrite `_on_normal_mail`)
- Modify: `src/lingtai/capabilities/email.py:688-692` (set mailbox identity in `setup()`, stop replacing `_on_normal_mail`)
- Modify: `src/lingtai/addons/gmail/manager.py:195-221` (delegate to BaseAgent notification)
- Modify: `tests/test_agent.py:163-177` (update notification content assertion)
- Modify: `tests/test_layers_email.py:69-84` (update email notification tests)
- Modify: `tests/test_addon_gmail_manager.py:85-94` (update gmail notification tests)

The in-flight changes to `base_agent.py` already add `_mailbox_name`/`_mailbox_tool` and rewrite `_on_normal_mail`. This task finalizes those changes and makes email/gmail capabilities delegate to BaseAgent instead of defining their own notification methods.

- [ ] **Step 1: Finalize `_on_normal_mail` in BaseAgent**

The current in-flight diff already has this. Verify `_on_normal_mail` uses `self._mailbox_name` and `self._mailbox_tool`:

```python
def _on_normal_mail(self, payload: dict) -> None:
    """Handle a normal mail — notify agent via inbox.

    The message is already persisted to mailbox/inbox/ by MailService.
    This method signals arrival and sends a uniform push notification.
    Capabilities configure ``_mailbox_name`` and ``_mailbox_tool``
    to change the notification text (e.g. "email box" / "email").
    """
    from uuid import uuid4

    email_id = payload.get("_mailbox_id") or str(uuid4())
    sender = payload.get("from", "unknown")
    subject = payload.get("subject", "(no subject)")
    message = payload.get("message", "")

    self._mail_arrived.set()

    preview = message[:100].replace("\n", " ")
    notification = (
        f'[system] 1 new message in {self._mailbox_name}.\n'
        f'  From: {sender} — {subject}\n'
        f'  {preview}...\n'
        f'Use {self._mailbox_tool}(action="check") to see your inbox.'
    )

    self._log("mail_received", sender=sender, subject=subject, message=message)
    msg = _make_message(MSG_REQUEST, "system", notification)
    self.inbox.put(msg)
```

Note: `_mail_notification` metadata dict is no longer set on the message — it was only needed by the collapse logic which is being deleted in Task 2.

- [ ] **Step 2: Update email capability to delegate notifications**

In `src/lingtai/capabilities/email.py`, the `setup()` function currently replaces `_on_normal_mail`:

```python
agent._on_normal_mail = mgr.on_normal_mail
```

Change `setup()` to just configure the mailbox identity instead. **Do not** replace `_on_normal_mail`:

```python
def setup(agent: "BaseAgent", *, private_mode: bool = False) -> EmailManager:
    """Set up email capability — filesystem-based mailbox."""
    mgr = EmailManager(agent, private_mode=private_mode)
    agent.override_intrinsic("mail")  # remove mail tool; email reimplements fully
    agent._mailbox_name = "email box"
    agent._mailbox_tool = "email"
    agent.add_tool(
        "email", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt="Send, receive, reply, and search email.",
    )
    return mgr
```

Then delete the `on_normal_mail` method from `EmailManager` entirely — BaseAgent handles it now.

- [ ] **Step 3: Update gmail addon notification format**

Gmail has its own listener path: IMAP → `GmailManager.on_gmail_received`. It does NOT go through `_on_mail_received` / `_on_normal_mail`. So it keeps its own `on_gmail_received` method — just update the notification format to match the uniform style:

In `src/lingtai/addons/gmail/manager.py`, update `on_gmail_received` notification:
```python
preview = message[:100].replace("\n", " ")
notification = (
    f'[system] 1 new message in gmail box.\n'
    f'  From: {sender} — {subject}\n'
    f'  {preview}...\n'
    f'Use gmail(action="check") to see your inbox.'
)
# ...
msg = _make_message(MSG_REQUEST, "system", notification)
```

Also add `agent._mail_arrived.set()` if not already present (fixes latent bug where `clock(wait)` wouldn't wake on gmail arrivals).

- [ ] **Step 4: Update test assertions**

**`tests/test_agent.py:test_mail_inbox_wiring`:**
```python
def test_mail_inbox_wiring(tmp_path):
    """_on_mail_received should notify agent inbox."""
    agent = BaseAgent(agent_name="receiver", service=make_mock_service(), base_dir=tmp_path)
    agent._on_mail_received({
        "_mailbox_id": "test-id-123",
        "from": "127.0.0.1:9999",
        "to": "127.0.0.1:8301",
        "message": "inbox test",
    })
    assert not agent.inbox.empty()
    msg = agent.inbox.get_nowait()
    assert msg.sender == "system"
    assert "mail box" in msg.content
    assert "mail(action=" in msg.content
```

**`tests/test_layers_email.py`** — 3 tests call `mgr.on_normal_mail(...)` which no longer exists. Rewrite to call `agent._on_mail_received(...)` instead:

`test_email_receive_notification` (line 69):
```python
def test_email_receive_notification(tmp_path):
    """Incoming mail should send notification to agent inbox."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    agent._on_mail_received({
        "_mailbox_id": "abc123",
        "from": "sender",
        "to": ["test"],
        "subject": "hi",
        "message": "body",
    })
    assert not agent.inbox.empty()
    notification = agent.inbox.get_nowait()
    assert notification.sender == "system"
    assert "email box" in notification.content
    assert "email(action=" in notification.content
```

`test_email_receive_fallback_id` (line 87):
```python
def test_email_receive_fallback_id(tmp_path):
    """Notification should work even without _mailbox_id."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities=["email"])
    agent._on_mail_received({"from": "sender", "message": "body"})
    assert not agent.inbox.empty()
```

`test_email_private_mode_receive_unrestricted` (line 656):
```python
def test_email_private_mode_receive_unrestricted(tmp_path):
    """Private mode should not block receiving emails."""
    agent = Agent(agent_name="test", service=make_mock_service(), base_dir=tmp_path,
                       capabilities={"email": {"private_mode": True}})
    agent._on_mail_received({
        "_mailbox_id": "abc",
        "from": "stranger",
        "to": ["me"],
        "subject": "hi",
        "message": "can you hear me",
    })
    assert not agent.inbox.empty()
```

**`tests/test_addon_gmail_manager.py:test_on_gmail_received_notifies_agent`:** Update to check for "gmail box" in content and sender == "system". Remove `_mail_notification` assertions.

- [ ] **Step 5: Run tests**

Run: `python -m pytest tests/test_agent.py tests/test_layers_email.py tests/test_addon_gmail_manager.py tests/test_silence_kill.py -v`
Expected: all PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/base_agent.py src/lingtai/capabilities/email.py src/lingtai/addons/gmail/manager.py \
    tests/test_agent.py tests/test_layers_email.py tests/test_addon_gmail_manager.py
git commit -m "feat: uniform mail notifications via _mailbox_name/_mailbox_tool, delegate to BaseAgent"
```

---

### Task 2: Replace mail-specific collapse with general message concatenation

**Files:**
- Modify: `src/lingtai/base_agent.py:555-558,623-658` (delete `_collapse_mail_notifications`, add `_concat_queued_messages` in run loop)
- Modify: `src/lingtai/message.py:38` (remove `_mail_notification` field)
- Test: `tests/test_agent.py` (add concat test)

- [ ] **Step 1: Write failing test for message concatenation**

Add to `tests/test_agent.py`:

```python
def test_queued_messages_concatenated(tmp_path):
    """Multiple queued messages should be concatenated into one."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    # Simulate 3 messages arriving while agent was busy
    msg1 = _make_message(MSG_REQUEST, "system", "[system] 1 new message in mail box.\n  From: alice — hello\n  preview...\nUse mail(action=\"check\") to see your inbox.")
    msg2 = _make_message(MSG_REQUEST, "system", "[system] 1 new message in mail box.\n  From: bob — world\n  preview...\nUse mail(action=\"check\") to see your inbox.")
    msg3 = _make_message(MSG_REQUEST, "system", "[system] 1 new message in gmail box.\n  From: charlie — meeting\n  preview...\nUse gmail(action=\"check\") to see your inbox.")
    agent.inbox.put(msg1)
    agent.inbox.put(msg2)
    agent.inbox.put(msg3)

    # Pick first, concat should drain the rest
    first = agent.inbox.get()
    result = agent._concat_queued_messages(first)
    assert "alice" in result.content
    assert "bob" in result.content
    assert "charlie" in result.content
    assert result.sender == "system"
    assert agent.inbox.empty()


def test_single_message_not_modified(tmp_path):
    """A single message with nothing queued should pass through unchanged."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    original = _make_message(MSG_REQUEST, "alice", "hello")
    result = agent._concat_queued_messages(original)
    assert result is original


def test_concat_preserves_non_system_sender(tmp_path):
    """If first message is from a real sender, concatenated result keeps that sender."""
    agent = BaseAgent(agent_name="test", service=make_mock_service(), base_dir=tmp_path)
    msg1 = _make_message(MSG_REQUEST, "alice", "task for you")
    msg2 = _make_message(MSG_REQUEST, "system", "[system] 1 new message in mail box.\n  ...")
    agent.inbox.put(msg1)
    agent.inbox.put(msg2)

    first = agent.inbox.get()
    result = agent._concat_queued_messages(first)
    assert "task for you" in result.content
    assert "mail box" in result.content
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_agent.py::test_queued_messages_concatenated -v`
Expected: FAIL — `_concat_queued_messages` doesn't exist

- [ ] **Step 3: Implement `_concat_queued_messages` and delete `_collapse_mail_notifications`**

In `src/lingtai/base_agent.py`:

Delete the entire `_collapse_mail_notifications` method (lines ~623-658).

Add `_concat_queued_messages`:

```python
def _concat_queued_messages(self, msg: Message) -> Message:
    """Drain any additional queued messages and concatenate into one.

    If nothing else is queued, returns the original message unchanged.
    Otherwise, joins all message contents with blank lines and returns
    a new merged message. Non-string content is converted via str().
    """
    extra: list[Message] = []
    while True:
        try:
            queued = self.inbox.get_nowait()
        except queue.Empty:
            break
        extra.append(queued)

    if not extra:
        return msg

    all_msgs = [msg] + extra
    parts = [m.content if isinstance(m.content, str) else str(m.content)
             for m in all_msgs]
    merged_content = "\n\n".join(parts)
    merged = _make_message(MSG_REQUEST, msg.sender, merged_content)
    self._log("messages_concatenated", count=len(all_msgs))
    return merged
```

**Note on behavioral change:** The old `_collapse_mail_notifications` only merged mail notifications and requeued non-notification messages. The new `_concat_queued_messages` merges ALL queued messages into one LLM turn. This is intentional — it's more efficient and treats all messages uniformly.

Update `_run_loop` call site (line ~558):

```python
# OLD: msg = self._collapse_mail_notifications(msg)
# NEW:
msg = self._concat_queued_messages(msg)
```

- [ ] **Step 4: Remove `_mail_notification` field from Message**

In `src/lingtai/message.py`, remove line 38:
```python
# DELETE: _mail_notification: dict | None = field(default=None, repr=False)
```

Grep for any remaining references to `_mail_notification` in `src/` and `tests/` and remove them. Known locations:
- `src/lingtai/base_agent.py` `_on_normal_mail` — remove the `msg._mail_notification = {...}` block
- `tests/test_agent.py` — remove assertions on `msg._mail_notification`
- `tests/test_addon_gmail_manager.py` — remove assertions on `msg._mail_notification`

- [ ] **Step 5: Run tests**

Run: `python -m pytest tests/ -v`
Expected: all PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/base_agent.py src/lingtai/message.py tests/test_agent.py tests/test_addon_gmail_manager.py
git commit -m "refactor: replace mail-specific collapse with general message concatenation"
```

---

### Task 3: Remove synchronous reply mechanism (`_reply_event` / `_reply_value`)

**Files:**
- Modify: `src/lingtai/base_agent.py:569-575,1065-1100,1187-1191` (remove `_reply_event`/`_reply_value` from `send()`, `_run_loop`, `_deliver_result`)
- Modify: `src/lingtai/message.py:26-27,36-37,47,55` (remove `_reply_event`, `_reply_value` fields and `reply_event` param)
- Modify: `tests/test_agent.py` (remove `test_message_reply_event`, update `test_send_fires_message`)
- Modify: `tests/test_message.py` (remove `test_message_reply_event`, update default assertions)

All agents are peers. If an agent wants a response, it sends mail and uses `clock(action="wait")` until a reply arrives. No synchronous request/response.

- [ ] **Step 1: Simplify `send()` API**

In `src/lingtai/base_agent.py`, rewrite `send()` to be fire-and-forget only:

```python
def send(
    self,
    content: str | dict,
    sender: str = "user",
) -> None:
    """Send a message to the agent (fire-and-forget).

    Args:
        content: Message content.
        sender: Message sender.
    """
    msg = _make_message(MSG_REQUEST, sender, content)
    self.inbox.put(msg)
```

- [ ] **Step 2: Remove `_reply_event` handling from `_run_loop`**

In `_run_loop`, remove the error handler's `_reply_event` block (lines ~569-575):

```python
# DELETE this block:
if msg._reply_event:
    msg._reply_value = {
        "text": f"Internal error: {err_desc}",
        "failed": True,
        "errors": [err_desc],
    }
    msg._reply_event.set()
```

- [ ] **Step 3: Remove `_deliver_result`**

Delete the `_deliver_result` method entirely (lines ~1187-1191).

Search for any callers of `_deliver_result` and remove those calls too.

- [ ] **Step 4: Clean up Message dataclass**

In `src/lingtai/message.py`:

Remove fields:
```python
# DELETE:
_reply_event: threading.Event | None = field(default=None, repr=False)
_reply_value: Any = field(default=None, repr=False)
```

Remove `reply_event` parameter from `_make_message`:
```python
def _make_message(
    type: str,
    sender: str,
    content: Any,
    *,
    reply_to: str | None = None,
) -> Message:
    return Message(
        id=f"msg_{uuid4().hex[:12]}",
        type=type,
        sender=sender,
        content=content,
        reply_to=reply_to,
    )
```

The `threading` import can be removed from `message.py` if no other fields use it.

- [ ] **Step 5: Update tests**

**`tests/test_message.py`:** Remove `test_message_reply_event`. Update `test_make_message` if it checks `_reply_event is None`.

**`tests/test_agent.py`:** Remove `test_message_reply_event` (line 285). Update `test_send_fires_message` — remove the `wait=False` argument since `send()` no longer accepts it. The call becomes `agent.send("hello")`.

Search for any code that calls `agent.send(..., wait=...)` in `src/` and `tests/` and remove the `wait` argument. Known callers that use `wait=False` (no change in behavior, just remove the arg):
- `src/lingtai/capabilities/delegate.py`
- `src/lingtai/capabilities/conscience.py`

- [ ] **Step 6: Run tests**

Run: `python -m pytest tests/ -v`
Expected: all PASS

- [ ] **Step 7: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/base_agent.py src/lingtai/message.py tests/test_agent.py tests/test_message.py
git commit -m "refactor: remove synchronous reply mechanism — all agents are async peers"
```

---

### Task 4: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update documentation**

Update CLAUDE.md to reflect:
- Mail notifications are uniform `[system]` messages with per-service mailbox name
- `_collapse_email_notifications` replaced by general `_concat_queued_messages`
- `send()` is fire-and-forget only (no `wait`/`timeout`)
- Message dataclass no longer has `_reply_event`, `_reply_value`, `_mail_notification`

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for notification unification and async-only messaging"
```
