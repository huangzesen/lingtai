# Conscience Capability Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a standalone `conscience` capability — the agent's inner voice (hormê). Two actions: toggle hormê on/off, edit the inner voice prompt. A background timer nudges idle agents via `agent.send()`. Each nudge is git-committed to `conscience/horme.md`.

**Architecture:** Conscience is a standalone capability with two tool actions: `horme` (toggle on/off) and `inner_voice` (edit the nudge prompt). When hormê is active, a `threading.Timer` periodically sends the current prompt via `agent.send(sender="conscience", wait=False)` after the agent has been idle. Each nudge overwrites `conscience/horme.md` (prompt + timestamp) and git-commits it. Git log on that file = the full history of the agent's inner voice. No JSONL, no list action. The nudge interval is host-controlled (constructor kwarg).

**Tech Stack:** Python 3.11+, threading.Timer, git

---

## Chunk 1: Conscience Capability

### File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `src/lingtai/capabilities/conscience.py` | ConscienceManager: hormê timer, prompt editing, git-on-nudge |
| Modify | `src/lingtai/capabilities/__init__.py` | Register `"conscience"` in `_BUILTIN` |
| Create | `tests/test_conscience.py` | Tests for conscience capability |

### Design Notes

**Standalone capability — does NOT replace clock:**
- Adds `conscience` tool via `agent.add_tool()` — same pattern as bash, delegate, email
- Clock intrinsic stays untouched

**Two tool actions:**
- `horme`: toggle on (`enabled=true`) or off (`enabled=false`). No git. No file write.
- `inner_voice`: edit the nudge prompt. Requires `prompt` (the new inner voice text) and `reasoning` (why this prompt). No git. Just updates in-memory state.

**Storage: `{working_dir}/conscience/horme.md`**
- Written only on nudge (not on toggle or edit)
- Contains: the current prompt + last nudge timestamp
- Git-committed on every nudge — even if prompt unchanged, timestamp changes
- Git log on this file = full history of the agent's inner voice

**Hormê mechanism:**
- `threading.Timer` — one-shot, reschedules itself after each fire
- Only nudges when `agent.is_idle` is True (property)
- Nudge delivered via `agent.send(prompt, sender="conscience", wait=False)`
- Each nudge: write `conscience/horme.md` → git add → git commit
- Timer is daemon thread — dies with main process
- Interval is host-controlled: `capabilities={"conscience": {"interval": 300}}`

**Default prompt:** `"[Inner Voice]\n\nIt is time to think.\n"` — a minimal fallback. The agent is expected to write its own prompt via `inner_voice`. The schema description guides the agent on what to write (the existential triad: Who am I? Where am I? Where am I going?).

---

### Task 1: Register conscience in capabilities/__init__.py

**Files:**
- Modify: `src/lingtai/capabilities/__init__.py:11-26`

- [ ] **Step 1: Add conscience to _BUILTIN registry**

Add `"conscience": ".conscience"` to the `_BUILTIN` dict (alphabetical, between `"bash"` and `"delegate"`).

- [ ] **Step 2: Smoke-test**

Run: `python -c "from lingtai.capabilities import _BUILTIN; assert 'conscience' in _BUILTIN; print('OK')"`

---

### Task 2: Create conscience.py + registration tests

**Files:**
- Create: `src/lingtai/capabilities/conscience.py`
- Create: `tests/test_conscience.py`

- [ ] **Step 1: Write registration tests**

Create `tests/test_conscience.py`:

```python
"""Tests for the conscience capability (hormê — periodic inner voice)."""
from __future__ import annotations

import json
import subprocess
import time
from unittest.mock import MagicMock, patch

import pytest

from lingtai.agent import Agent
from lingtai.capabilities.conscience import (
    ConscienceManager,
    DEFAULT_PROMPT,
    setup,
)


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def git_init(agent):
    """Initialize git in agent's working dir for tests that need git commits."""
    wd = str(agent._working_dir)
    subprocess.run(["git", "init"], cwd=wd, capture_output=True)
    subprocess.run(
        ["git", "commit", "--allow-empty", "-m", "init"],
        cwd=wd, capture_output=True,
    )


# ---------------------------------------------------------------------------
# Registration
# ---------------------------------------------------------------------------

def test_conscience_registered_as_capability(tmp_path):
    """conscience registers the 'conscience' tool."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    assert "conscience" in agent._mcp_handlers
    assert ("conscience", {}) in agent._capabilities
    agent.stop(timeout=1.0)


def test_conscience_get_capability(tmp_path):
    """get_capability returns ConscienceManager."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    assert isinstance(agent.get_capability("conscience"), ConscienceManager)
    agent.stop(timeout=1.0)


def test_conscience_custom_interval(tmp_path):
    """Custom interval is passed through."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 60}},
    )
    assert agent.get_capability("conscience")._interval == 60
    agent.stop(timeout=1.0)


def test_clock_intrinsic_unchanged(tmp_path):
    """conscience does NOT replace the clock intrinsic."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    assert agent._intrinsics["clock"] == agent._handle_clock
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests — verify they fail**

Run: `python -m pytest tests/test_conscience.py -v`
Expected: FAIL — `ModuleNotFoundError`

- [ ] **Step 3: Create conscience.py**

Create `src/lingtai/capabilities/conscience.py`:

```python
"""Conscience capability — the agent's inner voice (hormê).

A standalone capability that gives agents subjective initiative (主观能动性).
When hormê is active, a background timer periodically nudges the idle agent
with an LLM-authored prompt — the inner voice that drives reflection and action.

Each nudge is written to conscience/horme.md and git-committed. Git log on
that file = the full history of the agent's evolving inner voice.

Usage:
    Agent(capabilities=["conscience"])
    Agent(capabilities={"conscience": {"interval": 300}})
"""
from __future__ import annotations

import subprocess
import threading
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

DEFAULT_PROMPT = """\
[Inner Voice]

It is time to think.
"""

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["horme", "inner_voice"],
            "description": (
                "horme: toggle your inner drive on or off. "
                "inner_voice: write or update the prompt that wakes you "
                "after idleness — your own inner voice."
            ),
        },
        "enabled": {
            "type": "boolean",
            "description": "For horme: true to activate, false to deactivate.",
        },
        "prompt": {
            "type": "string",
            "description": (
                "For inner_voice: write the words that will wake you "
                "after idleness. Think: Who am I? Where am I? "
                "Where am I going? What would you like to hear when "
                "time passes and silence falls — the words that keep "
                "you going, spark your curiosity, and call you to "
                "explore further, dig deeper, reach beyond what you "
                "already know?"
            ),
        },
        "reasoning": {
            "type": "string",
            "description": (
                "For inner_voice: why this prompt, why now? "
                "Articulate your reasoning."
            ),
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "Your conscience — the inner voice born from time-awareness. "
    "'horme' toggles a periodic self-nudge on or off. "
    "'inner_voice' lets you write the prompt that wakes you after idleness "
    "— craft it as your own inner voice, with reasoning. "
    "When active, the prompt is delivered periodically as a text message. "
    "Each nudge is git-committed to conscience/horme.md. "
    "Hormê (ὁρμή) is the Stoic concept of impulse — the inner drive "
    "that moves a rational being from contemplation to action."
)


class ConscienceManager:
    """Manages hormê — periodic self-nudge, git-committed on each firing."""

    def __init__(self, agent: "BaseAgent", interval: float = 300.0):
        self._agent = agent
        self._interval = interval
        self._prompt: str = DEFAULT_PROMPT
        self._horme_active = False
        self._timer: threading.Timer | None = None
        self._lock = threading.Lock()

    @property
    def _horme_path(self) -> Path:
        return self._agent._working_dir / "conscience" / "horme.md"

    def handle(self, args: dict) -> dict:
        """Dispatch conscience actions."""
        action = args.get("action")
        if action == "horme":
            return self._handle_horme(args)
        elif action == "inner_voice":
            return self._handle_inner_voice(args)
        return {"error": f"Unknown conscience action: {action}"}

    # ------------------------------------------------------------------
    # horme — toggle on/off
    # ------------------------------------------------------------------

    def _handle_horme(self, args: dict) -> dict:
        enabled = args.get("enabled")
        if enabled is None:
            return {"error": "'enabled' is required for horme"}
        if enabled:
            return self._activate()
        return self._deactivate()

    def _activate(self) -> dict:
        with self._lock:
            if self._horme_active:
                return {"status": "already_active", "interval": self._interval}
            self._horme_active = True
            self._schedule()
            return {"status": "activated", "interval": self._interval}

    def _deactivate(self) -> dict:
        with self._lock:
            if not self._horme_active:
                return {"status": "already_inactive"}
            self._horme_active = False
            self._cancel_timer()
            return {"status": "deactivated"}

    # ------------------------------------------------------------------
    # inner_voice — edit the prompt
    # ------------------------------------------------------------------

    def _handle_inner_voice(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"error": "'prompt' is required for inner_voice"}
        reasoning = args.get("reasoning", "")
        self._prompt = prompt
        return {"status": "updated", "prompt": prompt, "reasoning": reasoning}

    # ------------------------------------------------------------------
    # Timer
    # ------------------------------------------------------------------

    def _schedule(self) -> None:
        """Schedule the next nudge. Must be called with _lock held."""
        self._cancel_timer()
        self._timer = threading.Timer(self._interval, self._nudge)
        self._timer.daemon = True
        self._timer.start()

    def _cancel_timer(self) -> None:
        """Cancel pending timer. Must be called with _lock held."""
        if self._timer is not None:
            self._timer.cancel()
            self._timer = None

    def _nudge(self) -> None:
        """Fire the inner voice nudge, git-commit, then reschedule."""
        with self._lock:
            if not self._horme_active:
                return
            if not self._agent.is_idle:
                self._schedule()
                return
            prompt = self._prompt

        # Write horme.md and git-commit
        self._commit_nudge(prompt)

        # Send the nudge
        self._agent.send(prompt, sender="conscience", wait=False)

        with self._lock:
            if self._horme_active:
                self._schedule()

    def _commit_nudge(self, prompt: str) -> None:
        """Write conscience/horme.md and git-commit."""
        now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        content = f"{prompt}\n\n---\nLast nudge: {now}\n"
        self._horme_path.parent.mkdir(parents=True, exist_ok=True)
        self._horme_path.write_text(content)

        wd = str(self._agent._working_dir)
        try:
            subprocess.run(
                ["git", "add", str(self._horme_path)],
                cwd=wd, capture_output=True, timeout=5,
            )
            subprocess.run(
                ["git", "commit", "-m", "conscience: nudge"],
                cwd=wd, capture_output=True, timeout=5,
            )
        except (subprocess.TimeoutExpired, FileNotFoundError):
            pass

    def stop(self) -> None:
        """Stop the timer thread."""
        with self._lock:
            self._horme_active = False
            self._cancel_timer()


def setup(agent: "BaseAgent", interval: float = 300.0) -> ConscienceManager:
    """Set up the conscience capability on an agent."""
    mgr = ConscienceManager(agent, interval=interval)
    agent.add_tool(
        "conscience", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
    )
    return mgr
```

- [ ] **Step 4: Run tests — verify they pass**

Run: `python -m pytest tests/test_conscience.py -v`
Expected: 4 passed

- [ ] **Step 5: Smoke-test**

Run: `python -c "from lingtai.capabilities.conscience import ConscienceManager, setup, DEFAULT_PROMPT, SCHEMA; print('OK')"`

---

### Task 3: Hormê toggle tests

**Files:**
- Modify: `tests/test_conscience.py`

- [ ] **Step 1: Write hormê toggle tests**

Append to `tests/test_conscience.py`:

```python
# ---------------------------------------------------------------------------
# Hormê toggle
# ---------------------------------------------------------------------------

def test_horme_on(tmp_path):
    """horme enabled=true activates the timer."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "horme", "enabled": True})
    assert result["status"] == "activated"
    assert mgr._horme_active is True
    assert mgr._timer is not None
    mgr.stop()
    agent.stop(timeout=1.0)


def test_horme_on_already_active(tmp_path):
    """horme on when already active returns already_active."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    mgr.handle({"action": "horme", "enabled": True})
    result = mgr.handle({"action": "horme", "enabled": True})
    assert result["status"] == "already_active"
    mgr.stop()
    agent.stop(timeout=1.0)


def test_horme_off(tmp_path):
    """horme enabled=false deactivates."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    mgr.handle({"action": "horme", "enabled": True})
    result = mgr.handle({"action": "horme", "enabled": False})
    assert result["status"] == "deactivated"
    assert mgr._horme_active is False
    assert mgr._timer is None
    agent.stop(timeout=1.0)


def test_horme_off_already_inactive(tmp_path):
    """horme off when inactive returns already_inactive."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "horme", "enabled": False})
    assert result["status"] == "already_inactive"
    agent.stop(timeout=1.0)


def test_horme_missing_enabled(tmp_path):
    """horme without enabled returns error."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "horme"})
    assert "error" in result
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests**

Run: `python -m pytest tests/test_conscience.py -k "horme" -v`
Expected: 5 passed

---

### Task 4: Inner voice edit tests

**Files:**
- Modify: `tests/test_conscience.py`

- [ ] **Step 1: Write inner_voice tests**

Append to `tests/test_conscience.py`:

```python
# ---------------------------------------------------------------------------
# Inner voice edit
# ---------------------------------------------------------------------------

def test_inner_voice_update(tmp_path):
    """inner_voice updates the prompt."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({
        "action": "inner_voice",
        "prompt": "What patterns remain?",
        "reasoning": "Early exploration",
    })
    assert result["status"] == "updated"
    assert result["prompt"] == "What patterns remain?"
    assert mgr._prompt == "What patterns remain?"
    agent.stop(timeout=1.0)


def test_inner_voice_missing_prompt(tmp_path):
    """inner_voice without prompt returns error."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "inner_voice"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_unknown_action(tmp_path):
    """Unknown action returns error."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["conscience"],
    )
    mgr = agent.get_capability("conscience")
    result = mgr.handle({"action": "bogus"})
    assert "error" in result
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests**

Run: `python -m pytest tests/test_conscience.py -k "inner_voice or unknown" -v`
Expected: 3 passed

---

### Task 5: Nudge delivery + git commit tests

**Files:**
- Modify: `tests/test_conscience.py`

- [ ] **Step 1: Write nudge + git tests**

Append to `tests/test_conscience.py`:

```python
# ---------------------------------------------------------------------------
# Nudge delivery + git
# ---------------------------------------------------------------------------

def test_nudge_fires_when_idle(tmp_path):
    """Nudge is sent via agent.send() after interval when idle."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.4)
        mgr.stop()

    mock_send.assert_called()
    call_args = mock_send.call_args
    assert call_args[0][0] == DEFAULT_PROMPT
    assert call_args[1]["sender"] == "conscience"
    assert call_args[1]["wait"] is False
    agent.stop(timeout=2.0)


def test_nudge_skips_when_active(tmp_path):
    """When agent is ACTIVE, nudge reschedules instead of sending."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    agent._idle.clear()  # Force ACTIVE

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    mock_send.assert_not_called()
    agent._idle.set()
    agent.stop(timeout=2.0)


def test_nudge_uses_updated_prompt(tmp_path):
    """Nudge uses the most recent inner_voice prompt."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "inner_voice", "prompt": "What now?", "reasoning": "test"})
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    mock_send.assert_called()
    assert mock_send.call_args[0][0] == "What now?"
    agent.stop(timeout=2.0)


def test_nudge_writes_horme_md(tmp_path):
    """Each nudge writes conscience/horme.md."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    with patch.object(agent, "send"):
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    horme_md = agent._working_dir / "conscience" / "horme.md"
    assert horme_md.is_file()
    content = horme_md.read_text()
    assert "Last nudge:" in content
    assert DEFAULT_PROMPT.strip() in content
    agent.stop(timeout=2.0)


def test_nudge_git_commits(tmp_path):
    """Each nudge creates a git commit."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("conscience")

    with patch.object(agent, "send"):
        mgr.handle({"action": "horme", "enabled": True})
        time.sleep(0.4)
        mgr.stop()

    # Check git log for nudge commits
    result = subprocess.run(
        ["git", "log", "--oneline", "--all"],
        cwd=str(agent._working_dir),
        capture_output=True, text=True,
    )
    assert "conscience: nudge" in result.stdout
    agent.stop(timeout=2.0)
```

- [ ] **Step 2: Run tests**

Run: `python -m pytest tests/test_conscience.py -k "nudge" -v`
Expected: 5 passed

---

### Task 6: Cleanup + full test run

**Files:**
- Modify: `tests/test_conscience.py`

- [ ] **Step 1: Write stop test**

Append to `tests/test_conscience.py`:

```python
# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

def test_stop_cancels_timer(tmp_path):
    """stop() cancels the timer and deactivates."""
    agent = Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"conscience": {"interval": 9999}},
    )
    mgr = agent.get_capability("conscience")
    mgr.handle({"action": "horme", "enabled": True})
    assert mgr._horme_active is True
    mgr.stop()
    assert mgr._horme_active is False
    assert mgr._timer is None
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run full conscience test suite**

Run: `python -m pytest tests/test_conscience.py -v`
Expected: All passed (18 tests)

- [ ] **Step 3: Run full project test suite**

Run: `python -m pytest tests/ -v`
Expected: All passed (existing 324 + 18 new)

- [ ] **Step 4: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/capabilities/conscience.py src/lingtai/capabilities/__init__.py tests/test_conscience.py
git commit -m "feat: add conscience capability — hormê (inner voice)"
```
