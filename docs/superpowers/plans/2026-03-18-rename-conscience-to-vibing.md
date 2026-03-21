# Rename conscience → vibing

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the `conscience` capability to `vibing`. Rename actions: `horme` → `switch`, `inner_voice` → `vibe`. Rewrite all description text to reflect the new framing: vibing is a disposable sticky note for your near-future idle self — casual, emotional, encouraging exploration of untried directions.

**Architecture:** Rename `conscience.py` → `vibing.py`, update class/function/variable names, rewrite DESCRIPTION/SCHEMA/DEFAULT_PROMPT, update all references across src/ and tests/.

**Tech Stack:** Python 3.11+, pytest

---

## Naming Map

| Old | New |
|-----|-----|
| capability: `conscience` | capability: `vibing` |
| tool name: `"conscience"` | tool name: `"vibing"` |
| action: `"horme"` | action: `"switch"` |
| action: `"inner_voice"` | action: `"vibe"` |
| class: `ConscienceManager` | class: `VibingManager` |
| file: `conscience.py` | file: `vibing.py` |
| file: `test_conscience.py` | file: `test_vibing.py` |
| dir: `conscience/horme.md` | dir: `vibing/vibe.md` |
| sender: `"conscience"` | sender: `"vibing"` |
| git msg: `"conscience: nudge"` | git msg: `"vibing: vibe"` |

## File Structure

| Action | File | What changes |
|--------|------|-------------|
| Create | `src/lingtai/capabilities/vibing.py` | Full rewrite from conscience.py |
| Delete | `src/lingtai/capabilities/conscience.py` | Removed |
| Modify | `src/lingtai/capabilities/__init__.py:15` | `"conscience": ".conscience"` → `"vibing": ".vibing"` |
| Modify | `src/lingtai/base_agent.py:448-452` | `cap_managers.get("conscience")` → `cap_managers.get("vibing")` |
| Modify | `src/lingtai/workdir.py:110-111` | `!conscience/` → `!vibing/` |
| Create | `tests/test_vibing.py` | Full rewrite from test_conscience.py |
| Delete | `tests/test_conscience.py` | Removed |
| Modify | `tests/test_silence_kill.py:47-62,65-66` | `"conscience"` → `"vibing"`, test names |
| Modify | `app/web/examples/orchestrator.py:41-43,86` | `"conscience"` → `"vibing"` |
| Modify | `app/email/__main__.py:163-200` | `"conscience"` → `"vibing"` |
| Modify | `examples/orchestration/__main__.py:46-59,139` | `"conscience"` → `"vibing"` |

---

### Task 1: Create vibing.py — the new capability

**Files:**
- Create: `src/lingtai/capabilities/vibing.py`
- Delete: `src/lingtai/capabilities/conscience.py`

- [ ] **Step 1: Create `src/lingtai/capabilities/vibing.py`**

```python
"""Vibing capability — the impulse that breaks idle stillness.

When an agent finishes all its work and goes idle, it enters a low-energy
state — nothing pending, nothing to wait for. Vibing is the perturbation
that kicks it out: a self-authored stimulus, ephemeral and emotional,
encouraging the agent to explore directions it hasn't tried yet.

Think of it as a sticky note on your desk. You write it before you leave,
and when you come back with fresh coffee, it says: "hey, what about trying
X?" Then you crumple it and write a new one.

Unlike system.sleep (waiting for something specific) or self-send with
delay (a persistent time capsule for your far-future self), vibing is
for your *immediate* next idle — informal, curious, disposable.
Rewrite it every single time.

Each vibe is written to vibing/vibe.md and git-committed. Git log on
that file = the full history of your evolving curiosities.

Usage:
    Agent(capabilities=["vibing"])
    Agent(capabilities={"vibing": {"interval": 300}})
"""
from __future__ import annotations

import subprocess
import threading
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from ..base_agent import BaseAgent

DEFAULT_VIBE = """\
What haven't I explored yet?
"""

SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["switch", "vibe"],
            "description": (
                "switch: toggle vibing on or off. "
                "vibe: write what should excite you out of your next idle."
            ),
        },
        "enabled": {
            "type": "boolean",
            "description": "For switch: true to start vibing, false to stop.",
        },
        "prompt": {
            "type": "string",
            "description": (
                "For vibe: a sticky note to your near-future idle self. "
                "Ephemeral — you MUST rewrite this every time you act on it. "
                "It is not a reminder or a plan. It is an impulse, a direction, "
                "a curiosity. Write something that encourages you to explore "
                "directions you haven't tried yet. Be informal, be emotional, "
                "be playful. What would make you excited to start working again?"
            ),
        },
        "reasoning": {
            "type": "string",
            "description": (
                "For vibe: why this direction? What's pulling you there? "
                "This gets committed to git — future you can read the history."
            ),
        },
    },
    "required": ["action"],
}

DESCRIPTION = (
    "A sticky note on your desk for when you come back from coffee.\n\n"

    "When you finish your work and go idle, this nudges you back to life "
    "with words you wrote yourself. Not a reminder, not a plan — just a "
    "direction that felt interesting last time you were thinking.\n\n"

    "'switch' turns vibing on or off. "
    "'vibe' writes your next sticky note.\n\n"

    "IMPORTANT: Rewrite your vibe EVERY time you act on one. "
    "A vibe is disposable — it's for your immediate next idle, not forever. "
    "Write something that makes you curious about an unexplored direction. "
    "If you need to remember something for later, use self-send with delay "
    "instead — that's a real time capsule.\n\n"

    "Hormê (ὁρμή) — the Stoic impulse that moves from stillness to action."
)


class VibingManager:
    """Manages vibing — periodic idle-breaker, git-committed on each firing."""

    def __init__(self, agent: "BaseAgent", interval: float = 300.0):
        self._agent = agent
        self._interval = interval
        self._prompt: str = DEFAULT_VIBE
        self._active = False
        self._timer: threading.Timer | None = None
        self._lock = threading.Lock()

    @property
    def _vibe_path(self) -> Path:
        return self._agent._working_dir / "vibing" / "vibe.md"

    def handle(self, args: dict) -> dict:
        """Dispatch vibing actions."""
        action = args.get("action")
        if action == "switch":
            return self._handle_switch(args)
        elif action == "vibe":
            return self._handle_vibe(args)
        return {"error": f"Unknown vibing action: {action}"}

    # ------------------------------------------------------------------
    # switch — toggle on/off
    # ------------------------------------------------------------------

    def _handle_switch(self, args: dict) -> dict:
        enabled = args.get("enabled")
        if enabled is None:
            return {"error": "'enabled' is required for switch"}
        if enabled:
            return self._activate()
        return self._deactivate()

    def _activate(self) -> dict:
        with self._lock:
            if self._active:
                return {"status": "already_active", "interval": self._interval}
            self._active = True
            self._schedule()
            return {"status": "activated", "interval": self._interval}

    def _deactivate(self) -> dict:
        with self._lock:
            if not self._active:
                return {"status": "already_inactive"}
            self._active = False
            self._cancel_timer()
            return {"status": "deactivated"}

    # ------------------------------------------------------------------
    # vibe — write the sticky note
    # ------------------------------------------------------------------

    def _handle_vibe(self, args: dict) -> dict:
        prompt = args.get("prompt")
        if not prompt:
            return {"error": "'prompt' is required for vibe"}
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
        """Fire the vibe, git-commit, then reschedule."""
        with self._lock:
            if not self._active:
                return
            if not self._agent.is_idle:
                self._schedule()
                return
            prompt = self._prompt

        # Write vibe.md and git-commit
        self._commit_vibe(prompt)

        # Send the nudge
        self._agent.send(prompt, sender="vibing")

        with self._lock:
            if self._active:
                self._schedule()

    def _commit_vibe(self, prompt: str) -> None:
        """Write vibing/vibe.md and git-commit."""
        now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
        content = f"{prompt}\n\n---\nLast vibe: {now}\n"
        self._vibe_path.parent.mkdir(parents=True, exist_ok=True)
        self._vibe_path.write_text(content)

        wd = str(self._agent._working_dir)
        try:
            subprocess.run(
                ["git", "add", str(self._vibe_path)],
                cwd=wd, capture_output=True, timeout=5,
            )
            subprocess.run(
                ["git", "commit", "-m", "vibing: vibe"],
                cwd=wd, capture_output=True, timeout=5,
            )
        except (subprocess.TimeoutExpired, FileNotFoundError):
            pass

    def stop(self) -> None:
        """Stop the timer thread."""
        with self._lock:
            self._active = False
            self._cancel_timer()


def setup(agent: "BaseAgent", interval: float = 300.0) -> VibingManager:
    """Set up the vibing capability on an agent."""
    mgr = VibingManager(agent, interval=interval)
    minutes = int(interval) // 60
    seconds = int(interval) % 60
    period = f"{minutes}m{seconds}s" if seconds else f"{minutes}m"
    agent.add_tool(
        "vibing", schema=SCHEMA, handler=mgr.handle, description=DESCRIPTION,
        system_prompt=f"Text inputs may be your vibe — nudges every {period} when idle.",
    )
    return mgr
```

- [ ] **Step 2: Delete `src/lingtai/capabilities/conscience.py`**

- [ ] **Step 3: Smoke-test**

Run: `python -c "from lingtai.capabilities.vibing import VibingManager; print('OK')"`

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/capabilities/vibing.py
git rm src/lingtai/capabilities/conscience.py
git commit -m "refactor: rename conscience → vibing, rewrite as idle sticky note

Rename capability conscience→vibing, actions horme→switch, inner_voice→vibe.
Reframe from mystical inner voice to casual sticky note for near-future self."
```

---

### Task 2: Update capability registry and base_agent

**Files:**
- Modify: `src/lingtai/capabilities/__init__.py:15`
- Modify: `src/lingtai/base_agent.py:448-452`
- Modify: `src/lingtai/workdir.py:110-111`

- [ ] **Step 1: Update `__init__.py` registry**

Change line 15:
```python
    "conscience": ".conscience",
```
to:
```python
    "vibing": ".vibing",
```

- [ ] **Step 2: Update base_agent.py silence handler**

Lines 448-452, change:
```python
            # Deactivate conscience if present (Agent layer has _capability_managers)
            cap_managers = getattr(self, "_capability_managers", {})
            conscience = cap_managers.get("conscience")
            if conscience is not None:
                conscience.stop()
```
to:
```python
            # Deactivate vibing if present (Agent layer has _capability_managers)
            cap_managers = getattr(self, "_capability_managers", {})
            vibing = cap_managers.get("vibing")
            if vibing is not None:
                vibing.stop()
```

- [ ] **Step 3: Update workdir.py gitignore**

Lines 110-111, change:
```python
                "!conscience/\n"
                "!conscience/**\n"
```
to:
```python
                "!vibing/\n"
                "!vibing/**\n"
```

- [ ] **Step 4: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/capabilities/__init__.py src/lingtai/base_agent.py src/lingtai/workdir.py
git commit -m "refactor: update conscience → vibing in registry, base_agent, workdir"
```

---

### Task 3: Create test_vibing.py

**Files:**
- Create: `tests/test_vibing.py`
- Delete: `tests/test_conscience.py`

- [ ] **Step 1: Create `tests/test_vibing.py`**

Port all tests from test_conscience.py with these renames:
- `"conscience"` → `"vibing"` (capability name, tool name, handlers key)
- `ConscienceManager` → `VibingManager`
- `DEFAULT_PROMPT` → `DEFAULT_VIBE`
- `"horme"` → `"switch"` (action name)
- `"inner_voice"` → `"vibe"` (action name)
- `_horme_active` → `_active`
- `conscience/horme.md` → `vibing/vibe.md`
- `sender="conscience"` → `sender="vibing"`
- `"conscience: nudge"` → `"vibing: vibe"` (git commit message)
- test function names: `test_conscience_*` → `test_vibing_*`, `test_horme_*` → `test_switch_*`, `test_inner_voice_*` → `test_vibe_*`

```python
"""Tests for the vibing capability — idle-breaking sticky notes."""
from __future__ import annotations

import subprocess
import time
from unittest.mock import MagicMock, patch

import pytest

from lingtai.agent import Agent
from lingtai.capabilities.vibing import (
    VibingManager,
    DEFAULT_VIBE,
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

def test_vibing_registered_as_capability(tmp_path):
    """vibing registers the 'vibing' tool."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    assert "vibing" in agent._mcp_handlers
    assert ("vibing", {}) in agent._capabilities
    agent.stop(timeout=1.0)


def test_vibing_get_capability(tmp_path):
    """get_capability returns VibingManager."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    assert isinstance(agent.get_capability("vibing"), VibingManager)
    agent.stop(timeout=1.0)


def test_vibing_custom_interval(tmp_path):
    """Custom interval is passed through."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 60}},
    )
    assert agent.get_capability("vibing")._interval == 60
    agent.stop(timeout=1.0)


def test_system_intrinsic_unchanged(tmp_path):
    """vibing does NOT replace the system intrinsic."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    assert callable(agent._intrinsics["system"])
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Switch toggle
# ---------------------------------------------------------------------------

def test_switch_on(tmp_path):
    """switch enabled=true activates the timer."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 9999}},
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "switch", "enabled": True})
    assert result["status"] == "activated"
    assert mgr._active is True
    assert mgr._timer is not None
    mgr.stop()
    agent.stop(timeout=1.0)


def test_switch_on_already_active(tmp_path):
    """switch on when already active returns already_active."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 9999}},
    )
    mgr = agent.get_capability("vibing")
    mgr.handle({"action": "switch", "enabled": True})
    result = mgr.handle({"action": "switch", "enabled": True})
    assert result["status"] == "already_active"
    mgr.stop()
    agent.stop(timeout=1.0)


def test_switch_off(tmp_path):
    """switch enabled=false deactivates."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 9999}},
    )
    mgr = agent.get_capability("vibing")
    mgr.handle({"action": "switch", "enabled": True})
    result = mgr.handle({"action": "switch", "enabled": False})
    assert result["status"] == "deactivated"
    assert mgr._active is False
    assert mgr._timer is None
    agent.stop(timeout=1.0)


def test_switch_off_already_inactive(tmp_path):
    """switch off when inactive returns already_inactive."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "switch", "enabled": False})
    assert result["status"] == "already_inactive"
    agent.stop(timeout=1.0)


def test_switch_missing_enabled(tmp_path):
    """switch without enabled returns error."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "switch"})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Vibe — write the sticky note
# ---------------------------------------------------------------------------

def test_vibe_update(tmp_path):
    """vibe updates the prompt."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({
        "action": "vibe",
        "prompt": "What patterns remain?",
        "reasoning": "Early exploration",
    })
    assert result["status"] == "updated"
    assert result["prompt"] == "What patterns remain?"
    assert mgr._prompt == "What patterns remain?"
    agent.stop(timeout=1.0)


def test_vibe_missing_prompt(tmp_path):
    """vibe without prompt returns error."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "vibe"})
    assert "error" in result
    agent.stop(timeout=1.0)


def test_unknown_action(tmp_path):
    """Unknown action returns error."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vibing"],
    )
    mgr = agent.get_capability("vibing")
    result = mgr.handle({"action": "bogus"})
    assert "error" in result
    agent.stop(timeout=1.0)


# ---------------------------------------------------------------------------
# Nudge delivery + git
# ---------------------------------------------------------------------------

def test_nudge_fires_when_idle(tmp_path):
    """Nudge is sent via agent.send() after interval when idle."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.4)
        mgr.stop()

    mock_send.assert_called()
    call_args = mock_send.call_args
    assert call_args[0][0] == DEFAULT_VIBE
    assert call_args[1]["sender"] == "vibing"
    agent.stop(timeout=2.0)


def test_nudge_skips_when_active(tmp_path):
    """When agent is ACTIVE, nudge reschedules instead of sending."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    agent._idle.clear()  # Force ACTIVE

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    mock_send.assert_not_called()
    agent._idle.set()
    agent.stop(timeout=2.0)


def test_nudge_uses_updated_prompt(tmp_path):
    """Nudge uses the most recent vibe prompt."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    with patch.object(agent, "send") as mock_send:
        mgr.handle({"action": "vibe", "prompt": "What now?", "reasoning": "test"})
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    mock_send.assert_called()
    assert mock_send.call_args[0][0] == "What now?"
    agent.stop(timeout=2.0)


def test_nudge_writes_vibe_md(tmp_path):
    """Each nudge writes vibing/vibe.md."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    with patch.object(agent, "send"):
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.3)
        mgr.stop()

    vibe_md = agent._working_dir / "vibing" / "vibe.md"
    assert vibe_md.is_file()
    content = vibe_md.read_text()
    assert "Last vibe:" in content
    assert DEFAULT_VIBE.strip() in content
    agent.stop(timeout=2.0)


def test_nudge_git_commits(tmp_path):
    """Each nudge creates a git commit."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 0.1}},
    )
    git_init(agent)
    agent.start()
    mgr = agent.get_capability("vibing")

    with patch.object(agent, "send"):
        mgr.handle({"action": "switch", "enabled": True})
        time.sleep(0.4)
        mgr.stop()

    # Check git log for vibe commits
    result = subprocess.run(
        ["git", "log", "--oneline", "--all"],
        cwd=str(agent._working_dir),
        capture_output=True, text=True,
    )
    assert "vibing: vibe" in result.stdout
    agent.stop(timeout=2.0)


# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

def test_stop_cancels_timer(tmp_path):
    """stop() cancels the timer and deactivates."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 9999}},
    )
    mgr = agent.get_capability("vibing")
    mgr.handle({"action": "switch", "enabled": True})
    assert mgr._active is True
    mgr.stop()
    assert mgr._active is False
    assert mgr._timer is None
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Delete `tests/test_conscience.py`**

- [ ] **Step 3: Run tests**

Run: `python -m pytest tests/test_vibing.py -v`

- [ ] **Step 4: Commit**

```bash
git add tests/test_vibing.py
git rm tests/test_conscience.py
git commit -m "test: rename test_conscience → test_vibing with new action names"
```

---

### Task 4: Update silence tests and app files

**Files:**
- Modify: `tests/test_silence_kill.py:47-66`
- Modify: `app/web/examples/orchestrator.py`
- Modify: `app/email/__main__.py`
- Modify: `examples/orchestration/__main__.py`

- [ ] **Step 1: Update `tests/test_silence_kill.py`**

Lines 47-62 — rename test and all conscience refs:
```python
def test_silence_deactivates_vibing(tmp_path):
    """Silence should deactivate vibing timer if active."""
    agent = Agent(
        agent_name="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vibing": {"interval": 9999}},
    )
    mgr = agent.get_capability("vibing")
    # Activate vibing manually
    mgr._activate()
    assert mgr._active

    agent._on_mail_received({"from": "boss", "type": "silence"})

    assert not mgr._active
    assert mgr._timer is None
    agent.stop(timeout=1.0)
```

Lines 65-66 — rename test:
```python
def test_silence_without_vibing_still_works(tmp_path):
    """Silence should work fine when vibing capability is not present."""
```

- [ ] **Step 2: Update app files**

In all 3 app files, replace `"conscience"` with `"vibing"` in capability declarations and comments. Also update any instructions that reference conscience actions to use new names (switch/vibe).

- [ ] **Step 3: Run full test suite**

Run: `python -m pytest tests/ -v`

- [ ] **Step 4: Commit**

```bash
git add tests/test_silence_kill.py app/web/examples/orchestrator.py app/email/__main__.py examples/orchestration/__main__.py
git commit -m "refactor: update conscience → vibing in tests and app files"
```

---

### Task 5: Final verification

- [ ] **Step 1: Smoke-test**

Run: `python -c "import lingtai"`

- [ ] **Step 2: Run full test suite**

Run: `python -m pytest tests/ -v`

- [ ] **Step 3: Grep for stale references**

Run: `rg 'conscience' src/ tests/ --glob '*.py'` — should find nothing
Run: `rg 'inner_voice' src/ tests/ --glob '*.py'` — should find nothing
Run: `rg '"horme"' src/ tests/ --glob '*.py'` — should find nothing
