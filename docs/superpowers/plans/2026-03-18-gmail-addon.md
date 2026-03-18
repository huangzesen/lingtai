# Gmail Addon Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `gmail` addon that gives a StoAI agent a `gmail` tool backed by real email via Gmail IMAP/SMTP. Separate mailbox from inter-agent `email`. An internal TCP bridge port lets other agents relay messages outward via Gmail.

**Architecture:**
- **Addon infrastructure** — new extension tier on `Agent`. Addons are set up after capabilities (may depend on them). Addon managers can implement `start()` and `stop()` hooks called by `Agent.start()`/`Agent.stop()`. This keeps listeners out of `__init__`.
- **`GoogleMailService(MailService)`** — IMAP poll + SMTP send, lives in `addons/gmail/`. Imports `MailService` ABC from core (to subclass it). Core never imports from addons.
- **`GmailManager`** — owns `gmail` tool, `working_dir/gmail/` mailbox, TCP bridge. Implements `start()` (begins IMAP poll + bridge listener) and `stop()` (cleans up both).
- **TCP bridge** — a `TCPMailService` with `working_dir=None` (no persistence). Receives TCP messages from other agents, reads `to` field as the external email address, forwards via SMTP. Other agents must format the `to` field with the real external address (not the bridge TCP address).
- The `email` capability and `BaseAgent` are completely unchanged.

**Tech Stack:** Python stdlib (`imaplib`, `smtplib`, `email`), existing `MailService` ABC, existing `TCPMailService`

---

## Design

### Two tools, two mailboxes

```
email tool  ← TCPMailService        ← inter-agent (core, unchanged)
gmail tool  ← GoogleMailService     ← external email (addon)
              + bridge TCPMailService (relay port, working_dir=None)
```

### Mailbox layout

```
working_dir/
  mailbox/           ← email tool's mailbox (unchanged)
    inbox/
    sent/
    read.json
    contacts.json
  gmail/             ← gmail addon's mailbox (separate)
    inbox/
    sent/
    read.json
    contacts.json
    gmail_state.json ← IMAP processed UIDs
```

### Bridge protocol

Other agents forward to Gmail by sending a TCP message to the bridge port with the **real external email address** in `to`:

```python
# From another agent:
email(action="send", address="127.0.0.1:8399", message="hello",
      subject="hi")
```

But this puts `to: ["127.0.0.1:8399"]` in the StoAI payload — wrong for SMTP routing. So the bridge convention is:

**The bridge reads `_external_to` from the payload. If absent, falls back to `to`.**

The agent itself uses `gmail(action="send")` for outbound — no bridge needed. The bridge is for **other agents** and **external scripts** that construct raw TCP payloads:

```python
# External script or agent constructing a raw TCP payload:
TCPMailService().send("127.0.0.1:8399", {
    "to": ["user@gmail.com"],
    "subject": "forwarded",
    "message": "hello from the inner system",
})
```

### Addon lifecycle

```
Agent.__init__()
  → setup addons (register tools, create services, but DON'T start listeners)
Agent.start()
  → super().start()  (starts TCP mail_service listener + agent thread)
  → addon_mgr.start() for each addon  (starts IMAP poll + bridge listener)
Agent.stop()
  → addon_mgr.stop() for each addon  (stops IMAP poll + bridge listener)
  → super().stop()  (stops TCP mail_service + agent thread)
```

### Gmail tool responses

Every response includes:

```json
{"tcp_alias": "127.0.0.1:8399", "account": "stoaiagent@gmail.com", ...}
```

---

## File Structure

| File | Responsibility |
|------|---------------|
| `src/stoai/addons/__init__.py` | Addon registry + `setup_addon()` |
| `src/stoai/addons/gmail/__init__.py` | `setup(agent, **kwargs)` — creates services + manager, registers tool |
| `src/stoai/addons/gmail/service.py` | `GoogleMailService(MailService)` — IMAP poll + SMTP send |
| `src/stoai/addons/gmail/manager.py` | `GmailManager` — mailbox, tool handler, bridge, start/stop lifecycle |
| `src/stoai/agent.py` | Add `addons=` parameter, start/stop hooks |
| `tests/test_addons.py` | Addon infrastructure tests |
| `tests/test_addon_gmail_service.py` | GoogleMailService unit tests |
| `tests/test_addon_gmail_manager.py` | GmailManager unit tests |
| `app/email/__main__.py` | Simplified launcher |
| `app/email/config.example.json` | Updated config template |

---

## Tasks

### Task 1: Addon infrastructure

**Files:**
- Create: `src/stoai/addons/__init__.py`
- Modify: `src/stoai/agent.py`
- Test: `tests/test_addons.py`

- [ ] **Step 1: Write failing test**

```python
# tests/test_addons.py
from __future__ import annotations
from unittest.mock import MagicMock


def test_addon_registry():
    from stoai.addons import _BUILTIN
    assert "gmail" in _BUILTIN


def test_agent_addon_lifecycle():
    """Addon managers with start/stop should be called at the right time."""
    from stoai.agent import Agent

    # Verify Agent accepts addons parameter
    # (can't actually construct without LLM, just check the class)
    import inspect
    sig = inspect.signature(Agent.__init__)
    assert "addons" in sig.parameters
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python -m pytest tests/test_addons.py::test_addon_registry -v`
Expected: FAIL

- [ ] **Step 3: Create addon registry**

```python
# src/stoai/addons/__init__.py
"""Add-ons — optional extensions that may depend on capabilities.

Add-ons are set up after capabilities. They use the same
setup(agent, **kwargs) interface but live separately to signal
they are optional and may have external dependencies.

Addon managers can implement start() and stop() methods for
lifecycle hooks — called by Agent.start() and Agent.stop().

Usage:
    agent = Agent(
        capabilities=["email", "file"],
        addons={"gmail": {"gmail_address": "...", "gmail_password": "..."}},
    )
"""
from __future__ import annotations

import importlib
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

_BUILTIN: dict[str, str] = {
    "gmail": ".gmail",
}


def setup_addon(agent: "BaseAgent", name: str, **kwargs: Any) -> Any:
    """Look up an addon by name and call its setup(agent, **kwargs).

    Returns whatever the addon's setup function returns (typically a manager).
    Raises ValueError if the name is unknown.
    """
    module_path = _BUILTIN.get(name)
    if module_path is None:
        raise ValueError(
            f"Unknown addon: {name!r}. "
            f"Available: {', '.join(sorted(_BUILTIN))}"
        )
    mod = importlib.import_module(module_path, package=__package__)
    setup_fn = getattr(mod, "setup", None)
    if setup_fn is None:
        raise ValueError(
            f"Addon module {name!r} does not export a setup() function"
        )
    return setup_fn(agent, **kwargs)
```

- [ ] **Step 4: Add addons to Agent**

In `src/stoai/agent.py`, add `addons` parameter to `__init__`, set up after capabilities. Override `start()` and `stop()` to call addon lifecycle hooks.

```python
# In Agent.__init__ signature:
def __init__(
    self,
    *args: Any,
    capabilities: list[str] | dict[str, dict] | None = None,
    addons: dict[str, dict] | None = None,
    **kwargs: Any,
):

# After capabilities setup, add:
    # Add-ons — set up after capabilities (may depend on them)
    self._addon_managers: dict[str, Any] = {}
    if addons:
        from .addons import setup_addon
        for addon_name, addon_kwargs in addons.items():
            mgr = setup_addon(self, addon_name, **(addon_kwargs or {}))
            self._addon_managers[addon_name] = mgr

# Override start():
def start(self) -> None:
    super().start()
    for name, mgr in self._addon_managers.items():
        if hasattr(mgr, "start"):
            mgr.start()

# Override stop():
def stop(self, timeout: float = 5.0) -> None:
    # Stop addons first (before mail service and agent thread)
    for name, mgr in self._addon_managers.items():
        if hasattr(mgr, "stop"):
            try:
                mgr.stop()
            except Exception:
                pass
    super().stop(timeout=timeout)
```

- [ ] **Step 5: Run tests**

Run: `python -m pytest tests/test_addons.py -v`
Expected: PASS

- [ ] **Step 6: Smoke-test**

Run: `python -c "from stoai.addons import setup_addon; print('OK')"`
Expected: `OK`

Run: `python -c "import stoai"`
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add src/stoai/addons/__init__.py src/stoai/agent.py tests/test_addons.py
git commit -m "feat: add addon infrastructure — extension tier with lifecycle hooks"
```

---

### Task 2: GoogleMailService

**Files:**
- Create: `src/stoai/addons/gmail/service.py`
- Create: `src/stoai/addons/gmail/__init__.py` (empty placeholder)
- Test: `tests/test_addon_gmail_service.py`

- [ ] **Step 1: Write failing tests**

```python
# tests/test_addon_gmail_service.py
from __future__ import annotations
from unittest.mock import patch, MagicMock
from stoai.addons.gmail.service import GoogleMailService


def test_construction():
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="xxxx xxxx xxxx xxxx",
    )
    assert svc.address == "agent@gmail.com"


def test_send_via_smtp():
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )
    with patch("stoai.addons.gmail.service.smtplib.SMTP") as MockSMTP:
        mock_smtp = MagicMock()
        MockSMTP.return_value.__enter__ = MagicMock(return_value=mock_smtp)
        MockSMTP.return_value.__exit__ = MagicMock(return_value=False)

        result = svc.send("user@gmail.com", {
            "subject": "test",
            "message": "hello",
        })

        assert result is None
        mock_smtp.starttls.assert_called_once()
        mock_smtp.login.assert_called_once_with("agent@gmail.com", "test")
        mock_smtp.send_message.assert_called_once()


def test_deliver_email_persists(tmp_path):
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
        working_dir=tmp_path,
    )
    received = []
    svc._deliver_email(lambda p: received.append(p), "user@gmail.com", "hi", "hello")

    assert len(received) == 1
    assert received[0]["from"] == "user@gmail.com"
    mid = received[0]["_mailbox_id"]
    msg_file = tmp_path / "gmail" / "inbox" / mid / "message.json"
    assert msg_file.is_file()


def test_send_empty_rejected():
    svc = GoogleMailService(
        gmail_address="agent@gmail.com",
        gmail_password="test",
    )
    result = svc.send("user@gmail.com", {"subject": "", "message": ""})
    assert result is not None  # error string
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_addon_gmail_service.py -v`

- [ ] **Step 3: Implement GoogleMailService**

Create `src/stoai/addons/gmail/__init__.py` as empty placeholder:
```python
# Setup function implemented after manager is ready.
```

Create `src/stoai/addons/gmail/service.py` with the `GoogleMailService` class. Key points:
- Subclasses `MailService` from `...services.mail`
- `send()` uses SMTP with TLS
- `listen()` starts IMAP poll thread
- `stop()` stops poll thread
- `_deliver_email()` persists to `working_dir/gmail/inbox/{uuid}/` before calling `on_message`
- `_poll_loop` connects, polls at interval, reconnects on error
- `_check_new_emails` uses `imap.uid("SEARCH"/"FETCH")` for stable UIDs
- `_save_state`/`_load_state` persist processed UIDs to `working_dir/gmail/gmail_state.json`
- UID list sorted numerically: `sorted(uids, key=lambda x: int(x) if x.isdigit() else 0)`

Full implementation in the code block in the previous plan revision — same code, with these fixes:
- `working_dir` is a constructor arg (not post-construction mutation)
- Persistence path is `working_dir/gmail/inbox/` (not `working_dir/mailbox/inbox/`)
- UID sort is numeric
- `_state_file` derived from `working_dir/gmail/gmail_state.json`

- [ ] **Step 4: Run tests**

Run: `python -m pytest tests/test_addon_gmail_service.py -v`
Expected: PASS

- [ ] **Step 5: Smoke-test**

Run: `python -c "from stoai.addons.gmail.service import GoogleMailService; print('OK')"`

- [ ] **Step 6: Commit**

```bash
git add src/stoai/addons/gmail/__init__.py src/stoai/addons/gmail/service.py tests/test_addon_gmail_service.py
git commit -m "feat: GoogleMailService — Gmail IMAP/SMTP MailService implementation"
```

---

### Task 3: GmailManager + addon setup

**Files:**
- Create: `src/stoai/addons/gmail/manager.py`
- Modify: `src/stoai/addons/gmail/__init__.py`
- Test: `tests/test_addon_gmail_manager.py`

- [ ] **Step 1: Write failing tests**

```python
# tests/test_addon_gmail_manager.py
from __future__ import annotations
import json
from pathlib import Path
from unittest.mock import MagicMock
from stoai.addons.gmail.manager import GmailManager


def test_check_returns_tcp_alias(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({"action": "check"})
    assert result["tcp_alias"] == "127.0.0.1:8399"
    assert result["account"] == "agent@gmail.com"


def test_check_lists_gmail_inbox(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    # Write test email to gmail inbox
    eid = "test-email-1"
    msg_dir = tmp_path / "gmail" / "inbox" / eid
    msg_dir.mkdir(parents=True)
    (msg_dir / "message.json").write_text(json.dumps({
        "from": "user@gmail.com", "to": ["agent@gmail.com"],
        "subject": "hello", "message": "hi there",
        "_mailbox_id": eid, "received_at": "2026-03-18T12:00:00Z",
    }))

    result = mgr.handle({"action": "check"})
    assert len(result["emails"]) == 1
    assert result["emails"][0]["from"] == "user@gmail.com"


def test_send_uses_gmail_service(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    svc.send.return_value = None
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    result = mgr.handle({
        "action": "send", "address": "user@gmail.com",
        "subject": "test", "message": "hello",
    })
    assert result["status"] == "delivered"
    svc.send.assert_called_once()


def test_every_response_has_meta(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")

    for action in ["check", "contacts"]:
        result = mgr.handle({"action": action})
        assert "tcp_alias" in result
        assert "account" in result

    result = mgr.handle({"action": "read", "email_id": "nope"})
    assert "tcp_alias" in result


def test_start_stop_lifecycle(tmp_path):
    agent = MagicMock()
    agent._working_dir = tmp_path
    svc = MagicMock()
    svc.address = "agent@gmail.com"
    mgr = GmailManager(agent, gmail_service=svc, tcp_alias="127.0.0.1:8399")
    mgr._bridge = MagicMock()

    mgr.start()
    svc.listen.assert_called_once()
    mgr._bridge.listen.assert_called_once()

    mgr.stop()
    svc.stop.assert_called_once()
    mgr._bridge.stop.assert_called_once()
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `python -m pytest tests/test_addon_gmail_manager.py -v`

- [ ] **Step 3: Implement GmailManager**

Create `src/stoai/addons/gmail/manager.py` with:
- Tool schema (same actions as email: send, check, read, reply, search, contacts, add/remove/edit_contact)
- `GmailManager` class with:
  - `__init__(agent, gmail_service, tcp_alias)` — stores refs, no listeners started
  - `start()` — starts IMAP poll via `gmail_service.listen()`, starts bridge TCP listener
  - `stop()` — stops both listeners
  - `handle(args)` — action dispatch, every response wrapped with `_inject_meta()`
  - `on_gmail_received(payload)` — notification to agent inbox (like email's `on_normal_mail`)
  - Same filesystem helpers as EmailManager but rooted at `working_dir/gmail/`
  - `_inject_meta(result)` — adds `tcp_alias` and `account` to every result dict

The `on_gmail_received` method should also set `msg._email_notification` for batching consistency.

- [ ] **Step 4: Implement addon setup**

Replace `src/stoai/addons/gmail/__init__.py`:

```python
"""Gmail addon — real email via Gmail IMAP/SMTP.

Adds a `gmail` tool with its own mailbox (working_dir/gmail/).
An internal TCP bridge port lets other agents relay messages outward.

Usage:
    agent = Agent(
        capabilities=["email", "file"],
        addons={"gmail": {
            "gmail_address": "agent@gmail.com",
            "gmail_password": "xxxx xxxx xxxx xxxx",
            "allowed_senders": ["you@gmail.com"],
        }},
    )
"""
from __future__ import annotations

import logging
from pathlib import Path
from typing import TYPE_CHECKING

from ...services.mail import TCPMailService
from .manager import GmailManager, SCHEMA, DESCRIPTION
from .service import GoogleMailService

if TYPE_CHECKING:
    from ...base_agent import BaseAgent

log = logging.getLogger(__name__)


def setup(
    agent: "BaseAgent",
    *,
    gmail_address: str,
    gmail_password: str,
    allowed_senders: list[str] | None = None,
    poll_interval: int = 30,
    bridge_port: int = 8399,
    imap_host: str = "imap.gmail.com",
    imap_port: int = 993,
    smtp_host: str = "smtp.gmail.com",
    smtp_port: int = 587,
) -> GmailManager:
    """Set up gmail addon — registers gmail tool, creates services.

    Listeners are NOT started here — they start in GmailManager.start(),
    which is called by Agent.start() via the addon lifecycle.
    """
    working_dir = Path(agent._working_dir)
    tcp_alias = f"127.0.0.1:{bridge_port}"

    # Create GoogleMailService (IMAP/SMTP)
    gmail_svc = GoogleMailService(
        gmail_address=gmail_address,
        gmail_password=gmail_password,
        allowed_senders=allowed_senders,
        poll_interval=poll_interval,
        working_dir=working_dir,
        imap_host=imap_host,
        imap_port=imap_port,
        smtp_host=smtp_host,
        smtp_port=smtp_port,
    )

    # Create bridge TCPMailService (no working_dir = no persistence)
    bridge = TCPMailService(listen_port=bridge_port)

    # Create manager (holds both services, owns lifecycle)
    mgr = GmailManager(agent, gmail_service=gmail_svc, tcp_alias=tcp_alias)
    mgr._bridge = bridge

    # Register gmail tool (tool registration happens at __init__ time, before seal)
    agent.add_tool(
        "gmail", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt=(
            f"Gmail account: {gmail_address}\n"
            f"Internal TCP alias: {tcp_alias} "
            f"(other agents can send to this address to relay via Gmail)\n"
            f"Use gmail(action=...) for external email. "
            f"Use email(action=...) for inter-agent communication."
        ),
    )

    log.info("Gmail addon configured: %s (bridge: %s)", gmail_address, tcp_alias)
    return mgr
```

- [ ] **Step 5: Add bridge relay logic to GmailManager.start()**

In `GmailManager.start()`:
```python
def start(self) -> None:
    """Start IMAP polling and bridge TCP listener."""
    # Start IMAP poll
    self._gmail_service.listen(on_message=self.on_gmail_received)

    # Start bridge — relay TCP messages to Gmail SMTP
    def on_bridge_mail(payload: dict) -> None:
        to = payload.get("to", [])
        if isinstance(to, str):
            to = [to]
        if not to:
            return
        for addr in to:
            self._gmail_service.send(addr, payload)

    self._bridge.listen(on_message=on_bridge_mail)

def stop(self) -> None:
    """Stop IMAP polling and bridge."""
    self._gmail_service.stop()
    if self._bridge:
        self._bridge.stop()
```

- [ ] **Step 6: Run tests**

Run: `python -m pytest tests/test_addon_gmail_manager.py -v`
Expected: PASS

- [ ] **Step 7: Smoke-test**

Run: `python -c "from stoai.addons.gmail import setup; print('OK')"`
Run: `python -c "import stoai"`

- [ ] **Step 8: Commit**

```bash
git add src/stoai/addons/gmail/manager.py src/stoai/addons/gmail/__init__.py tests/test_addon_gmail_manager.py
git commit -m "feat: gmail addon — GmailManager + tool + TCP bridge"
```

---

### Task 4: Update app/email launcher

**Files:**
- Modify: `app/email/__main__.py`
- Modify: `app/email/config.example.json`
- Delete: `app/email/bridge.py`

- [ ] **Step 1: Rewrite `__main__.py` to use addons**

The launcher creates an Agent with `capabilities=["email", ...]` and `addons={"gmail": {...}}`. No bridge code — the addon handles everything.

Key sections:
- Load config, extract gmail settings
- Create `LLMService`
- Create `TCPMailService` for inter-agent mail
- Create `Agent` with capabilities + addons
- `agent.start()` — starts everything (agent thread, TCP listener, IMAP poll, bridge)
- Signal handler for clean shutdown

See Task 4 in the previous plan revision for the full `__main__.py` code.

- [ ] **Step 2: Update config.example.json**

```json
{
  "_comment": "Gmail email app config. Copy to config.json and fill in.",

  "gmail_address": "your-agent@gmail.com",
  "gmail_password": "xxxx xxxx xxxx xxxx",
  "_gmail_note": "Use a Gmail App Password (2FA required). myaccount.google.com → App Passwords",

  "allowed_senders": ["your-personal@gmail.com"],

  "agent_name": "agent",
  "agent_port": 8301,
  "bridge_port": 8399,
  "poll_interval": 30,

  "provider": "minimax",
  "model": "MiniMax-M2.5-highspeed",
  "api_key_env": "MINIMAX_API_KEY",
  "max_turns": 20,

  "capabilities": {
    "email": {},
    "file": {},
    "web_search": {},
    "anima": {}
  }
}
```

- [ ] **Step 3: Delete bridge.py**

```bash
rm app/email/bridge.py
```

- [ ] **Step 4: Smoke-test**

Run: `python -c "from app.email.__main__ import load_config; print('OK')"`

- [ ] **Step 5: Commit**

```bash
git add app/email/__main__.py app/email/config.example.json
git rm app/email/bridge.py
git commit -m "refactor: email launcher uses gmail addon, remove bridge"
```

---

### Task 5: End-to-end manual test

- [ ] **Step 1: Clear old state**

```bash
rm -rf ~/.stoai/email
```

- [ ] **Step 2: Launch**

```bash
source venv/bin/activate && python -m app.email
```

Expected:
```
  Agent:      agent
  TCP:        127.0.0.1:8301 (inter-agent)
  Gmail:      stoaiagent@gmail.com
  Bridge:     127.0.0.1:8399 (TCP → Gmail)
  Accepts:    huangzs1997@gmail.com
  Data:       /Users/huangzesen/.stoai/email
```

- [ ] **Step 3: Send email from personal Gmail to stoaiagent@gmail.com**

- [ ] **Step 4: Verify:**
- IMAP connects and picks up the email
- Agent receives notification via `gmail` tool
- Agent processes and replies via `gmail(action="reply")`
- Reply arrives in personal Gmail inbox

- [ ] **Step 5: Send a second email while running — verify pickup within 30s**

- [ ] **Step 6: Verify agent has both tools available:**
- `email` — for inter-agent
- `gmail` — for external
