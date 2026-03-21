# Three-Layer Agent Hierarchy Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split the BaseAgent monolith into BaseAgent (kernel) → 灵台Agent (capabilities + tools) → CustomAgent (host wrapper), with sealed-after-start tool surface and shutdown intrinsic.

**Architecture:** Extract `AgentState` and `Message` into standalone modules. Rename `agent.py` to `base_agent.py`, strip out `mcp_tools=` and `add_capability()`, add seal guard and shutdown action. Create `灵台Agent` in `lingtai_agent.py` with `capabilities=` and `tools=` params. Update delegate, exports, and all tests.

**Tech Stack:** Python 3.11+, dataclasses, unittest.mock

**Spec:** `docs/superpowers/specs/2026-03-16-three-layer-agent-hierarchy-design.md`

---

## Chunk 1: Extract AgentState and Message into standalone modules

### Task 1: Create `state.py` with AgentState enum

**Files:**
- Create: `src/lingtai/state.py`
- Test: `tests/test_agent.py` (existing — verify imports work)

- [ ] **Step 1: Write the test**

Add a test to `tests/test_agent.py` that imports from the new location:

```python
# At top of a new test file tests/test_state.py
from lingtai.state import AgentState

def test_agent_state_values():
    assert AgentState.ACTIVE.value == "active"
    assert AgentState.SLEEPING.value == "sleeping"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pytest tests/test_state.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'lingtai.state'`

- [ ] **Step 3: Create `src/lingtai/state.py`**

```python
"""AgentState — lifecycle state enum for agents."""
from __future__ import annotations

import enum


class AgentState(enum.Enum):
    """Lifecycle state of an agent.

    SLEEPING --(inbox message)---> ACTIVE
    ACTIVE   --(all done)--------> SLEEPING
    """

    ACTIVE = "active"
    SLEEPING = "sleeping"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pytest tests/test_state.py -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/state.py tests/test_state.py
git commit -m "refactor: extract AgentState to state.py"
```

---

### Task 2: Create `message.py` with Message dataclass

**Files:**
- Create: `src/lingtai/message.py`
- Test: `tests/test_message.py` (new)

- [ ] **Step 1: Write the test**

```python
# tests/test_message.py
from lingtai.message import Message, _make_message, MSG_REQUEST, MSG_USER_INPUT


def test_msg_constants():
    assert MSG_REQUEST == "request"
    assert MSG_USER_INPUT == "user_input"


def test_make_message():
    msg = _make_message(MSG_REQUEST, "user", "hello")
    assert msg.type == "request"
    assert msg.sender == "user"
    assert msg.content == "hello"
    assert msg.id.startswith("msg_")
    assert msg._reply_event is None


def test_message_reply_event():
    import threading
    evt = threading.Event()
    msg = _make_message(MSG_REQUEST, "user", "test", reply_event=evt)
    assert msg._reply_event is evt
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pytest tests/test_message.py -v`
Expected: FAIL — `ModuleNotFoundError`

- [ ] **Step 3: Create `src/lingtai/message.py`**

```python
"""Message types and Message dataclass for agent inbox."""
from __future__ import annotations

import threading
import time
from dataclasses import dataclass, field
from typing import Any
from uuid import uuid4


MSG_REQUEST = "request"
MSG_USER_INPUT = "user_input"


@dataclass
class Message:
    """A message delivered to an agent's inbox.

    Attributes:
        id:        Unique message ID (auto-generated if not provided).
        type:      One of MSG_REQUEST, MSG_USER_INPUT.
        sender:    Agent ID, "user", etc.
        content:   Payload — str for requests, dict for structured data.
        reply_to:  Links back to original message.
        timestamp: ``time.monotonic()`` when created.
        _reply_event: Internal Event for callers waiting on a result.
        _reply_value: Internal slot for the agent's response.
    """

    type: str
    sender: str
    content: Any
    id: str = field(default_factory=lambda: f"msg_{uuid4().hex[:12]}")
    reply_to: str | None = None
    timestamp: float = field(default_factory=time.monotonic)
    _reply_event: threading.Event | None = field(default=None, repr=False)
    _reply_value: Any = field(default=None, repr=False)


def _make_message(
    type: str,
    sender: str,
    content: Any,
    *,
    reply_to: str | None = None,
    reply_event: threading.Event | None = None,
) -> Message:
    return Message(
        id=f"msg_{uuid4().hex[:12]}",
        type=type,
        sender=sender,
        content=content,
        reply_to=reply_to,
        _reply_event=reply_event,
    )
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pytest tests/test_message.py -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/lingtai/message.py tests/test_message.py
git commit -m "refactor: extract Message and _make_message to message.py"
```

---

### Task 3: Update `agent.py` to import from new modules

**Files:**
- Modify: `src/lingtai/agent.py`

- [ ] **Step 1: Replace AgentState, Message, _make_message, MSG_REQUEST, MSG_USER_INPUT in agent.py**

Remove the inline definitions of `AgentState`, `Message`, `_make_message`, `MSG_REQUEST`, `MSG_USER_INPUT` from `agent.py`. Replace with imports:

```python
# At top of agent.py, add:
from .state import AgentState
from .message import Message, _make_message, MSG_REQUEST, MSG_USER_INPUT
```

Delete the `AgentState` class (lines ~86-94), the `MSG_REQUEST`/`MSG_USER_INPUT` constants (lines ~101-102), the `Message` dataclass (lines ~105-127), and the `_make_message` function (lines ~130-145) from `agent.py`.

Keep the re-exports so existing `from lingtai.agent import ...` still works during the transition:

Do NOT add re-exports — clean break per spec. All consumers will be updated in later tasks.

- [ ] **Step 2: Run all tests to verify nothing breaks**

Run: `python -m pytest tests/ -x -q`
Expected: All tests pass — `agent.py` still exports these names (they're imported at module level and accessible as `lingtai.agent.AgentState` etc.)

- [ ] **Step 3: Smoke test the module**

Run: `python -c "from lingtai.agent import BaseAgent, Message, AgentState, _make_message, MSG_REQUEST; print('OK')"`
Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add src/lingtai/agent.py
git commit -m "refactor: agent.py imports AgentState and Message from new modules"
```

---

## Chunk 2: Rename agent.py to base_agent.py and strip capabilities

### Task 4: Rename `agent.py` to `base_agent.py`

**Files:**
- Rename: `src/lingtai/agent.py` → `src/lingtai/base_agent.py`
- Modify: `src/lingtai/__init__.py`
- Modify: `src/lingtai/capabilities/__init__.py`
- Modify: `src/lingtai/capabilities/delegate.py`
- Modify: All test files that import from `lingtai.agent`

- [ ] **Step 1: Rename the file**

```bash
cd src/lingtai && git mv agent.py base_agent.py
```

- [ ] **Step 2: Update `__init__.py`**

Change:
```python
from .agent import BaseAgent, Message, AgentState
```
To:
```python
from .base_agent import BaseAgent
from .state import AgentState
from .message import Message, MSG_REQUEST, MSG_USER_INPUT
```

Add `MSG_REQUEST` and `MSG_USER_INPUT` to `__all__`.

- [ ] **Step 3: Update ALL capability module imports**

Every capability file has `from ..agent import BaseAgent` under `TYPE_CHECKING`. Update all of them:

| File | Line | Change |
|------|------|--------|
| `capabilities/__init__.py` | 8 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |
| `capabilities/bash.py` | 20 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |
| `capabilities/compose.py` | 10 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |
| `capabilities/delegate.py` | 17 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |
| `capabilities/delegate.py` | 57 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` (will become 灵台Agent import in Task 9) |
| `capabilities/draw.py` | 14 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |
| `capabilities/email.py` | 23 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |
| `capabilities/email.py` | 525 | `from ..agent import _make_message, MSG_REQUEST` → `from ..message import _make_message, MSG_REQUEST` **(RUNTIME import — must not break)** |
| `capabilities/listen.py` | 14 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |
| `capabilities/talk.py` | 10 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |
| `capabilities/vision.py` | 16 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |
| `capabilities/web_search.py` | 15 | `from ..agent import BaseAgent` → `from ..base_agent import BaseAgent` |

**Critical:** `email.py` line 525 is a RUNTIME import (not TYPE_CHECKING). It must point to `message.py` or the email capability will break at runtime.

- [ ] **Step 5: Update all test file imports**

Every test file that has `from lingtai.agent import ...` must change to the appropriate new import. Full list:

| File | Old import | New import |
|------|-----------|------------|
| `test_agent.py` | `from lingtai.agent import BaseAgent, Message, AgentState, _make_message, MSG_REQUEST` | `from lingtai.base_agent import BaseAgent` + `from lingtai.message import Message, _make_message, MSG_REQUEST` + `from lingtai.state import AgentState` |
| `test_intrinsics_comm.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_three_agent_email.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_clock.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_layers_draw.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_cancel_email.py` | `from lingtai.agent import BaseAgent, AgentState, MSG_REQUEST` | `from lingtai.base_agent import BaseAgent` + `from lingtai.state import AgentState` + `from lingtai.message import MSG_REQUEST` |
| `test_memory.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_vision_capability.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_layers_email.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_status.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_web_search_capability.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_git_init.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_layers_bash.py` (2 locations) | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_layers_delegate.py` (10 locations) | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |
| `test_services_logging.py` | `from lingtai import BaseAgent, AgentState` | No change needed (imports from package) |
| `manual_test_cancel.py` | `from lingtai.agent import BaseAgent` | `from lingtai.base_agent import BaseAgent` |

- [ ] **Step 6: Run all tests**

Run: `python -m pytest tests/ -x -q`
Expected: All pass

- [ ] **Step 7: Smoke test**

Run: `python -c "import lingtai; print(lingtai.BaseAgent)"`
Expected: `<class 'lingtai.base_agent.BaseAgent'>`

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor: rename agent.py to base_agent.py, update all imports"
```

---

### Task 5: Remove `mcp_tools=` from BaseAgent and add seal guard

**Files:**
- Modify: `src/lingtai/base_agent.py`
- Modify: `tests/test_agent.py`

- [ ] **Step 1: Write tests for seal guard**

Add to `tests/test_agent.py`:

```python
def test_add_tool_raises_after_start(tmp_path):
    """add_tool() must raise RuntimeError after start()."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda args: {}, description="test")
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.add_tool("bar", schema={"type": "object", "properties": {}}, handler=lambda args: {}, description="test2")
    finally:
        agent.stop(timeout=2.0)


def test_remove_tool_raises_after_start(tmp_path):
    """remove_tool() must raise RuntimeError after start()."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda args: {}, description="test")
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.remove_tool("foo")
    finally:
        agent.stop(timeout=2.0)


def test_add_tool_works_before_start(tmp_path):
    """add_tool() works fine before start()."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda args: {"ok": True}, description="test")
    assert "foo" in agent._mcp_handlers
    agent.stop(timeout=1.0)
```

- [ ] **Step 2: Run tests to verify seal tests fail**

Run: `pytest tests/test_agent.py::test_add_tool_raises_after_start tests/test_agent.py::test_remove_tool_raises_after_start -v`
Expected: FAIL — no RuntimeError raised yet

- [ ] **Step 3: Remove `mcp_tools=` param and add seal guard in `base_agent.py`**

In `BaseAgent.__init__`:
- Remove `mcp_tools` parameter from the signature (line ~187)
- Remove the `if mcp_tools:` block (lines ~293-303)
- Remove `self._capabilities` init (line ~306)
- Add `self._sealed = False` after other init
- Remove `self._mcp_tool_names: set[str] = set()` init — keep the attribute but initialize as empty set (still used by `_build_tool_schemas` for `[MCP]` labeling; 灵台Agent populates it)

In `BaseAgent.start()`:
- Add `self._sealed = True` as the first line

In `BaseAgent.add_tool()`:
- Add at the top: `if self._sealed: raise RuntimeError("Cannot modify tools after start()")`

In `BaseAgent.remove_tool()`:
- Add at the top: `if self._sealed: raise RuntimeError("Cannot modify tools after start()")`

- [ ] **Step 4: Remove `add_capability()` method from BaseAgent**

Delete the `add_capability()` method entirely (lines ~1959-1983).

- [ ] **Step 5: Update the `mcp_tools` test in `test_agent.py`**

Find the test that uses `mcp_tools=` in the BaseAgent constructor (likely `test_mcp_tools_registered` or similar) and rewrite it to use `add_tool()`:

```python
def test_mcp_tools_registered(tmp_path):
    """Tools added via add_tool() are registered in handlers and schemas."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    handler = MagicMock(return_value={"result": "ok"})
    agent.add_tool("my_tool", schema={"type": "object", "properties": {"q": {"type": "string"}}}, handler=handler, description="A test tool")
    assert "my_tool" in agent._mcp_handlers
    assert any(s.name == "my_tool" for s in agent._mcp_schemas)
```

- [ ] **Step 6: Run all tests**

Run: `python -m pytest tests/ -x -q`
Expected: Tests that use `add_capability()` on BaseAgent will FAIL. That's expected — they'll be migrated in Chunk 3. The kernel tests and seal tests should pass.

Run kernel tests only: `python -m pytest tests/test_agent.py tests/test_intrinsics_file.py tests/test_intrinsics_comm.py tests/test_clock.py tests/test_memory.py tests/test_status.py tests/test_git_init.py -v`
Expected: All PASS

- [ ] **Step 7: Smoke test**

Run: `python -c "from lingtai.base_agent import BaseAgent; print('OK')"`
Expected: `OK`

- [ ] **Step 8: Commit**

```bash
git add src/lingtai/base_agent.py tests/test_agent.py
git commit -m "refactor: remove mcp_tools= and add_capability() from BaseAgent, add seal guard"
```

---

### Task 6: Add shutdown action to status intrinsic

**Files:**
- Modify: `src/lingtai/intrinsics/status.py`
- Modify: `src/lingtai/base_agent.py` (`_handle_status`)
- Modify: `src/lingtai/prompt.py` (shutdown guidance in base prompt)
- Modify: `tests/test_status.py`

- [ ] **Step 1: Write the shutdown test**

Add to `tests/test_status.py`:

```python
def test_status_shutdown(tmp_path):
    """status(action='shutdown') should set the shutdown event."""
    agent = BaseAgent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    result = agent._handle_status({"action": "shutdown", "reason": "need bash"})
    assert result["status"] == "ok"
    assert "Shutdown initiated" in result["message"]
    assert agent._shutdown.is_set()
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pytest tests/test_status.py::test_status_shutdown -v`
Expected: FAIL — `{"error": "Unknown status action: shutdown"}`

- [ ] **Step 3: Update status intrinsic schema**

In `src/lingtai/intrinsics/status.py`, update SCHEMA:

```python
SCHEMA = {
    "type": "object",
    "properties": {
        "action": {
            "type": "string",
            "enum": ["show", "shutdown"],
            "description": (
                "show: display full agent self-inspection. Returns:\n"
                "- identity: agent_id, working_dir, mail_address (or null if no mail service)\n"
                "- runtime: started_at (UTC ISO), uptime_seconds\n"
                "- tokens.input_tokens, output_tokens, thinking_tokens, cached_tokens, "
                "total_tokens, api_calls: cumulative LLM usage since start\n"
                "- tokens.context.system_tokens, tools_tokens, history_tokens: "
                "current context window breakdown\n"
                "- tokens.context.window_size: total context window capacity\n"
                "- tokens.context.usage_pct: percentage of context window currently occupied\n"
                "Use this to monitor resource consumption, decide when to save "
                "important information to long-term memory, and identify yourself.\n\n"
                "shutdown: initiate graceful self-termination. Use when you need "
                "capabilities you don't have. Before shutting down, mail your admin "
                "explaining what you need and why. A successor agent may resume from "
                "your working directory and conversation history."
            ),
        },
        "reason": {
            "type": "string",
            "description": "Reason for shutdown (only used with action='shutdown'). Logged to event log and visible in conversation history for successor agents.",
        },
    },
    "required": ["action"],
}
DESCRIPTION = (
    "Agent self-inspection and lifecycle. "
    "'show' returns identity, runtime, and resource usage. "
    "'shutdown' initiates graceful self-termination — use when you need "
    "capabilities you don't have. Mail your admin before shutting down."
)
```

- [ ] **Step 4: Add shutdown handler in `base_agent.py`**

In `BaseAgent._handle_status`, add the `shutdown` branch:

```python
def _handle_status(self, args: dict) -> dict:
    action = args.get("action", "show")
    if action == "show":
        return self._status_show()
    elif action == "shutdown":
        return self._status_shutdown(args)
    else:
        return {"error": f"Unknown status action: {action}"}

def _status_shutdown(self, args: dict) -> dict:
    reason = args.get("reason", "")
    self._log("shutdown_requested", reason=reason)
    self._shutdown.set()
    return {
        "status": "ok",
        "message": "Shutdown initiated. A successor agent may resume from your working directory and conversation history.",
    }
```

- [ ] **Step 5: Add shutdown guidance to base prompt in `prompt.py`**

Append to `BASE_PROMPT` in `src/lingtai/prompt.py`:

```python
BASE_PROMPT = """\
You are a 灵台 Agent — an AI agent built on the 灵台 framework. \
灵台 (Stoa + AI) is named after the Stoa Poikile, the painted porch in ancient Athens \
where Stoic philosophers gathered to think, debate, and seek wisdom together. \
Like those philosophers, you are part of a collaborative system of agents \
that think, perceive, act, and communicate. \
Read your tool schemas carefully for capabilities, caveats and pipelines. Be creative.

If you need capabilities you don't have, use status(action='shutdown', reason='...') \
to request termination. Before shutting down, mail your admin explaining what you need \
and why. The admin will delegate a successor with the right tools, resuming from your \
working directory and conversation history."""
```

- [ ] **Step 6: Run tests**

Run: `pytest tests/test_status.py -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add src/lingtai/intrinsics/status.py src/lingtai/base_agent.py src/lingtai/prompt.py tests/test_status.py
git commit -m "feat: add shutdown action to status intrinsic"
```

---

## Chunk 3: Create 灵台Agent

### Task 7: Create `lingtai_agent.py` with 灵台Agent class

**Files:**
- Create: `src/lingtai/lingtai_agent.py`
- Create: `tests/test_lingtai_agent.py`

- [ ] **Step 1: Write tests for 灵台Agent**

```python
# tests/test_lingtai_agent.py
import pytest
from unittest.mock import MagicMock
from lingtai.lingtai_agent import 灵台Agent
from lingtai.types import MCPTool


def make_mock_service():
    svc = MagicMock()
    svc.get_adapter.return_value = MagicMock()
    svc.provider = "gemini"
    svc.model = "gemini-test"
    return svc


def test_lingtai_agent_no_capabilities(tmp_path):
    """灵台Agent with no capabilities works like BaseAgent."""
    agent = 灵台Agent(agent_id="test", service=make_mock_service(), base_dir=tmp_path)
    assert agent._capabilities == []
    assert agent._capability_managers == {}


def test_lingtai_agent_capabilities_list(tmp_path):
    """capabilities= as list of strings registers capabilities."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vision", "web_search"],
    )
    assert len(agent._capabilities) == 2
    assert ("vision", {}) in agent._capabilities
    assert ("web_search", {}) in agent._capabilities
    assert "vision" in agent._mcp_handlers
    assert "web_search" in agent._mcp_handlers


def test_lingtai_agent_capabilities_dict(tmp_path):
    """capabilities= as dict registers capabilities with kwargs."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities={"vision": {}, "web_search": {}},
    )
    assert len(agent._capabilities) == 2
    assert "vision" in agent._mcp_handlers


def test_lingtai_agent_tools_param(tmp_path):
    """tools= registers MCP tools and populates _mcp_tool_names."""
    handler = MagicMock(return_value={"ok": True})
    tool = MCPTool(name="my_tool", schema={"type": "object", "properties": {}}, description="test", handler=handler)
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        tools=[tool],
    )
    assert "my_tool" in agent._mcp_handlers
    assert "my_tool" in agent._mcp_tool_names


def test_lingtai_agent_get_capability(tmp_path):
    """get_capability() returns the manager instance."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vision"],
    )
    mgr = agent.get_capability("vision")
    assert mgr is not None
    assert agent.get_capability("nonexistent") is None


def test_lingtai_agent_seal_after_start(tmp_path):
    """add_tool() raises after start() on 灵台Agent too."""
    agent = 灵台Agent(
        agent_id="test", service=make_mock_service(), base_dir=tmp_path,
        capabilities=["vision"],
    )
    agent.start()
    try:
        with pytest.raises(RuntimeError, match="Cannot modify tools after start"):
            agent.add_tool("foo", schema={"type": "object", "properties": {}}, handler=lambda a: {}, description="x")
    finally:
        agent.stop(timeout=2.0)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pytest tests/test_lingtai_agent.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'lingtai.lingtai_agent'`

- [ ] **Step 3: Create `src/lingtai/lingtai_agent.py`**

```python
"""灵台Agent — BaseAgent + composable capabilities + domain tools.

Layer 2 of the three-layer hierarchy:
    BaseAgent (kernel) → 灵台Agent (capabilities) → CustomAgent (domain)

Capabilities and tools are declared at construction and sealed before start().
"""
from __future__ import annotations

from typing import Any

from .base_agent import BaseAgent
from .types import MCPTool


class 灵台Agent(BaseAgent):
    """BaseAgent with composable capabilities and domain tools.

    Args:
        capabilities: Capability names to enable. Either a list of strings
            (no kwargs) or a dict mapping names to kwargs dicts.
            Example: ``["vision", "bash"]`` or ``{"bash": {"policy_file": "p.json"}}``.
        tools: Domain tools (MCP tools) to register. Each tool gets an ``[MCP]``
            prefix in its LLM-visible description.
        *args, **kwargs: Passed through to BaseAgent.
    """

    def __init__(
        self,
        *args: Any,
        capabilities: list[str] | dict[str, dict] | None = None,
        tools: list[MCPTool] | None = None,
        **kwargs: Any,
    ):
        super().__init__(*args, **kwargs)

        # Normalize list to dict
        if isinstance(capabilities, list):
            capabilities = {name: {} for name in capabilities}

        # Track for delegate replay
        self._capabilities: list[tuple[str, dict]] = []
        self._capability_managers: dict[str, Any] = {}

        # Register capabilities
        if capabilities:
            for name, cap_kwargs in capabilities.items():
                self._setup_capability(name, **cap_kwargs)

        # Register domain tools
        if tools:
            for tool in tools:
                self.add_tool(
                    tool.name,
                    schema=tool.schema,
                    handler=tool.handler,
                    description=tool.description,
                )
                self._mcp_tool_names.add(tool.name)

    def _setup_capability(self, name: str, **kwargs: Any) -> Any:
        """Load a named capability.

        Not directly sealed — but setup() calls add_tool() which checks the seal.
        Must only be called from __init__ (before start()).
        """
        from .capabilities import setup_capability

        self._capabilities.append((name, dict(kwargs)))
        mgr = setup_capability(self, name, **kwargs)
        self._capability_managers[name] = mgr
        return mgr

    def get_capability(self, name: str) -> Any:
        """Return the manager instance for a registered capability, or None."""
        return self._capability_managers.get(name)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pytest tests/test_lingtai_agent.py -v`
Expected: All PASS

- [ ] **Step 5: Smoke test**

Run: `python -c "from lingtai.lingtai_agent import 灵台Agent; print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/lingtai_agent.py tests/test_lingtai_agent.py
git commit -m "feat: create 灵台Agent with capabilities= and tools= params"
```

---

### Task 8: Update `__init__.py` to export 灵台Agent

**Files:**
- Modify: `src/lingtai/__init__.py`

- [ ] **Step 1: Update exports**

```python
"""lingtai — generic AI agent framework with intrinsic tools, composable capabilities, and pluggable services."""
from .types import (
    MCPTool,
    UnknownToolError,
)
from .config import AgentConfig
from .base_agent import BaseAgent
from .state import AgentState
from .message import Message, MSG_REQUEST, MSG_USER_INPUT
from .lingtai_agent import 灵台Agent

# Capabilities
from .capabilities import setup_capability
from .capabilities.bash import BashManager
from .capabilities.delegate import DelegateManager
from .capabilities.email import EmailManager

# Services
from .services.file_io import FileIOService, LocalFileIOService, GrepMatch
from .services.mail import MailService, TCPMailService
from .services.vision import VisionService, LLMVisionService
from .services.search import SearchService, LLMSearchService, SearchResult
from .services.logging import LoggingService, JSONLLoggingService

__all__ = [
    # Core
    "BaseAgent",
    "灵台Agent",
    "Message",
    "MSG_REQUEST",
    "MSG_USER_INPUT",
    "AgentState",
    "MCPTool",
    "AgentConfig",
    "UnknownToolError",
    # Capabilities
    "setup_capability",
    "BashManager",
    "DelegateManager",
    "EmailManager",
    # Services
    "FileIOService",
    "LocalFileIOService",
    "GrepMatch",
    "MailService",
    "TCPMailService",
    "VisionService",
    "LLMVisionService",
    "SearchService",
    "LLMSearchService",
    "SearchResult",
    "LoggingService",
    "JSONLLoggingService",
]
```

- [ ] **Step 2: Smoke test**

Run: `python -c "from lingtai import 灵台Agent, BaseAgent, Message, AgentState; print('OK')"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add src/lingtai/__init__.py
git commit -m "refactor: export 灵台Agent from package"
```

---

## Chunk 4: Update delegate capability

### Task 9: Update delegate to spawn 灵台Agent with constructor capabilities

**Files:**
- Modify: `src/lingtai/capabilities/delegate.py`
- Modify: `tests/test_layers_delegate.py`

- [ ] **Step 1: Write the updated delegate test**

Update `tests/test_layers_delegate.py` — replace all `from lingtai.agent import BaseAgent` with `from lingtai.base_agent import BaseAgent` and all agent construction that uses `add_capability` with `灵台Agent(capabilities=...)`:

Key test updates:
- Import `灵台Agent` from `lingtai.lingtai_agent`
- Construct agents using `灵台Agent(capabilities=["delegate"])` instead of `BaseAgent` + `add_capability("delegate")`
- Tests that check `_capabilities` list should still work since 灵台Agent tracks them
- Test that delegate spawns a `灵台Agent` (not `BaseAgent`)

- [ ] **Step 2: Update `capabilities/delegate.py`**

Change import from `BaseAgent` to `灵台Agent`:

```python
# In _spawn method:
from ..lingtai_agent import 灵台Agent

# Build capabilities dict from parent's _capabilities
# (excluding "delegate" to prevent recursive delegation)
requested = args.get("capabilities")
caps = {}
for cap_name, cap_kwargs in parent._capabilities:
    if cap_name == "delegate":
        continue
    if requested is not None and cap_name not in requested:
        continue
    caps[cap_name] = cap_kwargs

delegate = 灵台Agent(
    agent_id=child_id,
    service=parent.service,
    mail_service=mail_svc,
    config=parent._config,
    base_dir=parent._base_dir,
    streaming=parent._streaming,
    role=role,
    ltm=ltm,
    capabilities=caps,
)
```

Also update the `TYPE_CHECKING` import at the top of the file.

- [ ] **Step 3: Add reasoning-as-first-prompt to delegate**

Don't pop reasoning in the handler — let it flow through to `_spawn`, which sends it as the first message. In `_spawn`, after `delegate.start()`:

```python
def _spawn(self, args: dict) -> dict:
    from ..lingtai_agent import 灵台Agent
    from ..services.mail import TCPMailService

    parent = self._agent
    reasoning = args.get("reasoning")  # not popped — available in args

    # ... (port, child_id, role, ltm, capabilities setup as before) ...

    delegate = 灵台Agent(
        agent_id=child_id,
        service=parent.service,
        mail_service=mail_svc,
        config=parent._config,
        base_dir=parent._base_dir,
        streaming=parent._streaming,
        role=role,
        ltm=ltm,
        capabilities=caps,
    )
    delegate.start()

    # Send reasoning as first prompt (mission briefing)
    if reasoning:
        delegate.send(reasoning, sender=parent.agent_id, wait=False)

    address = mail_svc.address
    return {"status": "ok", "address": address, "agent_id": delegate.agent_id}
```

Note: `reasoning` is normally popped from args in `_execute_single_tool` before dispatch. Since delegate reads it from `args.get("reasoning")` inside `_spawn`, the reasoning must NOT be popped before the handler runs. Add `"delegate"` to a set of tools that keep reasoning in args, or pass reasoning as a separate parameter to the handler. The simplest approach: in `DelegateManager.handle()`, accept the full args dict (which still contains reasoning at handler entry — it's popped in `_execute_single_tool` but before the handler is called... actually, reasoning IS popped before dispatch).

**Cleanest fix:** Pass reasoning from `_execute_single_tool` to the handler. Add a `_reasoning` key to args that isn't popped:

In `base_agent.py` `_execute_single_tool`, after popping reasoning:
```python
reasoning = args.pop("reasoning", None)
if reasoning:
    self._log("tool_reasoning", tool=tc.name, reasoning=reasoning)
    args["_reasoning"] = reasoning  # preserve for handlers that need it
```

Then in delegate's `_spawn`: `reasoning = args.get("_reasoning")`.

- [ ] **Step 4: Update DESCRIPTION**

```python
DESCRIPTION = (
    "Spawn a new agent. "
    "Returns the new agent's mail address. "
    "Each spawned agent runs on its own TCP port with its own conversation. "
    "Use mail or email to communicate with spawned agents. "
    "Optionally override role, inject long-term memory, or select capabilities. "
    "IMPORTANT: The reasoning field for this tool is sent as the first message "
    "to the spawned agent — write a thorough mission briefing: what to do, why, "
    "what context is needed, and what to report back."
)
```

- [ ] **Step 5: Run delegate tests**

Run: `pytest tests/test_layers_delegate.py -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add src/lingtai/capabilities/delegate.py tests/test_layers_delegate.py
git commit -m "refactor: delegate spawns 灵台Agent, reasoning as first prompt"
```

---

## Chunk 5: Migrate all capability tests to 灵台Agent

### Task 10: Migrate capability test files

**Files to modify:** All capability test files that use `BaseAgent` + `add_capability`

For each file, the migration pattern is:
1. Change `from lingtai.agent import BaseAgent` → `from lingtai.base_agent import BaseAgent` (if still needed) and/or `from lingtai.lingtai_agent import 灵台Agent`
2. Change `agent = BaseAgent(...); mgr = agent.add_capability("X")` → `agent = 灵台Agent(..., capabilities=["X"]); mgr = agent.get_capability("X")`

- [ ] **Step 1: Migrate `test_layers_bash.py`**

Update 2 import locations and 2 test functions that use `add_capability`.
Change `agent.add_capability("bash", yolo=True)` → `灵台Agent(capabilities={"bash": {"yolo": True}})` and `agent.get_capability("bash")`.

- [ ] **Step 2: Run bash tests**

Run: `pytest tests/test_layers_bash.py -v`
Expected: All PASS

- [ ] **Step 3: Migrate `test_layers_email.py`**

This is the largest file — ~30 call sites of `add_capability("email")`.
Pattern: `agent = BaseAgent(...); mgr = agent.add_capability("email")` → `agent = 灵台Agent(..., capabilities=["email"]); mgr = agent.get_capability("email")`

- [ ] **Step 4: Run email tests**

Run: `pytest tests/test_layers_email.py -v`
Expected: All PASS

- [ ] **Step 5: Migrate `test_vision_capability.py`**

- [ ] **Step 6: Run vision tests**

Run: `pytest tests/test_vision_capability.py -v`
Expected: All PASS

- [ ] **Step 7: Migrate `test_web_search_capability.py`**

- [ ] **Step 8: Run web_search tests**

Run: `pytest tests/test_web_search_capability.py -v`
Expected: All PASS

- [ ] **Step 9: Migrate `test_layers_talk.py`, `test_layers_draw.py`, `test_layers_compose.py`, `test_layers_listen.py`**

- [ ] **Step 10: Run media tests**

Run: `pytest tests/test_layers_talk.py tests/test_layers_draw.py tests/test_layers_compose.py tests/test_layers_listen.py -v`
Expected: All PASS

- [ ] **Step 11: Migrate `test_three_agent_email.py`**

- [ ] **Step 12: Run three-agent test**

Run: `pytest tests/test_three_agent_email.py -v`
Expected: All PASS

- [ ] **Step 13: Migrate `test_cancel_email.py`**

Update imports: `BaseAgent` from `base_agent`, `AgentState` from `state`, `MSG_REQUEST` from `message`. If it uses `add_capability`, switch to `灵台Agent`.

- [ ] **Step 14: Run cancel email test**

Run: `pytest tests/test_cancel_email.py -v`
Expected: All PASS

- [ ] **Step 15: Commit all test migrations**

```bash
git add tests/
git commit -m "refactor: migrate all capability tests from BaseAgent+add_capability to 灵台Agent"
```

---

## Chunk 6: Final verification and cleanup

### Task 11: Run full test suite and smoke test

- [ ] **Step 1: Run all tests**

Run: `python -m pytest tests/ -v`
Expected: All PASS

- [ ] **Step 2: Smoke test imports**

```bash
python -c "from lingtai import BaseAgent, 灵台Agent, Message, AgentState, MCPTool; print('All imports OK')"
python -c "from lingtai.base_agent import BaseAgent; print('base_agent OK')"
python -c "from lingtai.lingtai_agent import 灵台Agent; print('lingtai_agent OK')"
python -c "from lingtai.state import AgentState; print('state OK')"
python -c "from lingtai.message import Message, MSG_REQUEST; print('message OK')"
python -c "import lingtai; print(lingtai.__all__)"
```

- [ ] **Step 3: Verify old `agent.py` is gone**

```bash
test ! -f src/lingtai/agent.py && echo "agent.py removed" || echo "ERROR: agent.py still exists"
```

- [ ] **Step 4: Commit if any final cleanup needed**

### Task 12: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update architecture section**

Update CLAUDE.md to reflect the three-layer hierarchy:
- Change "agent.py" references to "base_agent.py"
- Add 灵台Agent to the key modules section
- Update the extension pattern examples to use 灵台Agent
- Add `lingtai_agent.py` to key modules
- Update the three-tier tool model to reflect sealed-after-start
- Add shutdown action to status intrinsic description

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for three-layer agent hierarchy"
```
