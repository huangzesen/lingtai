# Three-Layer Agent Hierarchy

Split the BaseAgent monolith into a clean three-layer class hierarchy where nothing changes at runtime.

## Motivation

BaseAgent is a ~2175-line monolith that owns everything: intrinsics, capabilities, MCP tools, lifecycle, LLM session. Capabilities define what an agent IS, and changing them at runtime is like hot-swapping organs. The tool surface should be fully known at construction time.

The three layers:

```
BaseAgent              — kernel (intrinsics, immutable tool surface)
    |
StoAIAgent(BaseAgent)  — kernel + capabilities + domain tools (declared at construction)
    |
CustomAgent(StoAIAgent) — host's wrapper (subclass with domain logic)
```

## Design Decisions (from brainstorming)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| `add_tool()` location | BaseAgent (kernel) | It's the primitive that capabilities and domain tools both use |
| Seal after `start()` | Yes — `add_tool()` and `remove_tool()` raise `RuntimeError` after `start()` | Enforces "nothing changes at runtime" in code |
| `mcp_tools=` on BaseAgent | Removed | Domain tools go through `tools=` on StoAIAgent or `add_tool()` in subclasses |
| `update_system_prompt()` | Open at any time (not sealed) | Prompt sections are context, not tools — memory reload, host-injected context |
| `capabilities=` format | `list[str]` or `dict[str, dict]` — list is sugar for dict with empty kwargs | Clean one-liner for simple cases, dict when kwargs needed |
| Delegate spawning | Mirrors parent capabilities by default | Future delegate policy is delegate tool's concern, not StoAIAgent's |
| `[MCP]` tool labeling | Kept — StoAIAgent populates `_mcp_tool_names` from `tools=` param | Capabilities are capabilities, MCP tools are MCP tools — the LLM should know the difference |
| Shutdown reason | In the tool call args, visible in restored chat history | Reborn agent reads its own conversation history — no special property needed |
| Shutdown | `status(action="shutdown", reason="...")` intrinsic | Agent requests termination; host handles rebirth |
| File convenience methods | Stay on BaseAgent | Ergonomic wrappers backed by intrinsics, kernel-level |

## 1. File Structure

The current `agent.py` (~2175 lines) splits into focused modules:

| New file | Contents | ~Lines |
|----------|----------|--------|
| `base_agent.py` | `BaseAgent` — kernel (lifecycle, intrinsics, tool dispatch, LLM comms, compaction, token tracking, hooks) | ~1400 |
| `stoai_agent.py` | `StoAIAgent(BaseAgent)` — capabilities, `tools=` param, seal-after-start | ~80 |
| `message.py` | `Message`, `_make_message`, `MSG_REQUEST`, `MSG_USER_INPUT` | ~50 |
| `state.py` | `AgentState` enum | ~15 |

File-locking helpers (`_lock_fd`, `_unlock_fd`) stay in `base_agent.py` as private module-level functions.

## 2. BaseAgent (kernel)

**File:** `base_agent.py`

### Constructor

```python
BaseAgent(
    agent_id, service, *,
    file_io=None, mail_service=None, config=None,
    base_dir, context=None,
    enabled_intrinsics=None, disabled_intrinsics=None,
    admin=False, streaming=False, logging_service=None,
    role="", ltm="",
)
```

No `mcp_tools=`. No `_capabilities` state.

### Public API

| Method | Sealed? | Notes |
|--------|---------|-------|
| `start()` | — | Sets `self._sealed = True` |
| `stop()` | — | Graceful shutdown |
| `send()` | — | Message delivery |
| `mail()` | — | Programmatic mail send |
| `add_tool()` | Yes | Raises `RuntimeError` after `start()` |
| `remove_tool()` | Yes | Raises `RuntimeError` after `start()` |
| `update_system_prompt()` | No | Open at any time |
| `read_file()`, `write_file()`, `edit_file()`, `glob()`, `grep()` | — | Convenience wrappers |
| `get_chat_state()`, `restore_chat()`, `restore_token_state()` | — | Session persistence |
| `get_token_usage()`, `status()` | — | Introspection |

### Seal Guard

```python
# Set in start()
self._sealed = True

# Checked in add_tool() and remove_tool()
if self._sealed:
    raise RuntimeError("Cannot modify tools after start()")
```

### Hooks (unchanged)

- `_pre_request(msg)` — transform message before LLM send
- `_post_request(msg, result)` — side effects after LLM response
- `_on_tool_result_hook(tool_name, args, result)` — post-tool interception
- `_get_guard_limits()` — per-agent loop guard settings
- `_PARALLEL_SAFE_TOOLS` — class variable for concurrent execution

### Status Intrinsic — Shutdown Action

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
    self._shutdown.set()  # signals the run loop to exit
    return {
        "status": "ok",
        "message": "Shutdown initiated. A successor agent may resume from your working directory and conversation history.",
    }
```

The status intrinsic schema is updated to include `shutdown` action and `reason` parameter.

System prompt guidance:

> "Use `status(action='shutdown', reason='...')` when you need capabilities you don't have. Before shutting down, mail your admin explaining what you need and why. The admin will delegate a successor with the right tools, resuming from your working directory and conversation history."

Host/admin side: the admin reads the agent's mail, spawns a new StoAIAgent with the requested capabilities using the same working dir, and restores chat state. The successor agent sees the full conversation history including the shutdown call and its reason — it picks up where the predecessor left off.

### Status Intrinsic Schema Update

The `status` intrinsic schema in `intrinsics/status.py` is updated:
- `action` enum: `["show", "shutdown"]`
- New optional property: `reason` (string) — only used with `shutdown` action
- The shutdown guidance text is added to the base system prompt template in `prompt.py`

### `[MCP]` Labeling — Kept

The `_mcp_tool_names` set and `[MCP]` prefix logic in `_build_tool_schemas()` stay on BaseAgent. The set is just no longer populated in BaseAgent's constructor (since `mcp_tools=` is removed). Instead, StoAIAgent populates it when registering tools from the `tools=` param:

```python
# In StoAIAgent.__init__, when registering domain tools:
if tools:
    for tool in tools:
        self.add_tool(tool.name, schema=tool.schema,
                      handler=tool.handler, description=tool.description)
        self._mcp_tool_names.add(tool.name)
```

Capabilities are capabilities. MCP tools are MCP tools. The LLM should know which is which.

### Shutdown Reason — No Special Property

The `reason` lives in the tool call args: `status(action="shutdown", reason="need bash")`. It's logged to the event log AND it's part of the chat history. When a successor agent's chat session is restored from the same working dir, it sees the shutdown call and knows the context. No `shutdown_reason` property, no special mechanism. The succession is just normal delegation — the admin spawns a new agent that happens to resume from the same working dir.

`stop()` behavior is unchanged whether triggered by shutdown intrinsic or normal stop — it flushes LTM, writes manifest, releases lock.

## 3. StoAIAgent

**File:** `stoai_agent.py`

```python
class StoAIAgent(BaseAgent):
    def __init__(
        self,
        *args,
        capabilities: list[str] | dict[str, dict] | None = None,
        tools: list[MCPTool] | None = None,
        **kwargs,
    ):
        super().__init__(*args, **kwargs)

        # Normalize list to dict
        if isinstance(capabilities, list):
            capabilities = {name: {} for name in capabilities}

        # Track for delegate replay
        self._capabilities: list[tuple[str, dict]] = []

        # Register capabilities
        if capabilities:
            for name, cap_kwargs in capabilities.items():
                self._setup_capability(name, **cap_kwargs)

        # Register domain tools
        if tools:
            for tool in tools:
                self.add_tool(
                    tool.name, schema=tool.schema,
                    handler=tool.handler, description=tool.description,
                )

    def _setup_capability(self, name: str, **kwargs) -> Any:
        """Internal: load a named capability.

        Not directly sealed — but setup() calls add_tool() which checks the seal.
        Must only be called from __init__ (before start()).
        """
        from .capabilities import setup_capability
        self._capabilities.append((name, dict(kwargs)))
        return setup_capability(self, name, **kwargs)

    def get_capability(self, name: str) -> Any:
        """Return the manager instance for a registered capability, or None."""
        return self._capability_managers.get(name)
```

The `_capability_managers: dict[str, Any]` is populated by `_setup_capability` with the return value of each `setup()` call. This lets host code and tests access managers after construction:

```python
agent = StoAIAgent(capabilities=["bash", "email"], ...)
bash_mgr = agent.get_capability("bash")   # BashManager
email_mgr = agent.get_capability("email") # EmailManager
```

### Usage

```python
# List form — no kwargs
agent = StoAIAgent(
    agent_id="alice", service=svc, base_dir="/agents",
    capabilities=["vision", "web_search", "bash"],
)

# Dict form — with kwargs
agent = StoAIAgent(
    agent_id="bob", service=svc, base_dir="/agents",
    capabilities={
        "vision": {},
        "web_search": {},
        "bash": {"policy_file": "policy.json"},
    },
)

# Subclass
class ResearchAgent(StoAIAgent):
    def __init__(self, **kwargs):
        super().__init__(capabilities=["vision", "web_search"], **kwargs)
        self._setup_capability("bash", policy_file="research_policy.json")
        self.add_tool("query_db", schema={...}, handler=db_handler)
```

## 4. Delegate Capability Changes

DelegateManager spawns `StoAIAgent` instead of `BaseAgent`. The key change is that capabilities must be passed through the constructor, not replayed post-construction.

### Updated Spawn Logic

```python
def _spawn(self, agent_id: str, role: str, ...) -> StoAIAgent:
    # Build capabilities dict from parent's _capabilities log
    # (excluding "delegate" to prevent recursive delegation)
    caps = {
        name: kwargs
        for name, kwargs in self._agent._capabilities
        if name != "delegate"
    }

    delegate = StoAIAgent(
        agent_id=agent_id,
        service=self._agent.service,
        base_dir=self._agent._base_dir,
        capabilities=caps,
        role=role,
        mail_service=...,
        config=...,
    )
    delegate.start()
    return delegate
```

**Key points:**
- Capabilities dict is built from `parent._capabilities` before construction — no post-construction replay
- `delegate` capability is excluded from the delegate's capabilities (prevents infinite recursion)
- Future delegate capability policy (e.g., restricting what delegates can do) is the delegate tool's own concern, not StoAIAgent's

### Delegate Reasoning as First Prompt

Every tool already has a `reasoning` parameter injected by `_build_tool_schemas()` — it's popped from args before dispatch and logged as diary. For the delegate tool, `reasoning` serves double duty:

1. **Diary** — logged as `tool_reasoning` event, same as all other tools
2. **First prompt** — sent as the initial message to the delegated agent via `delegate.send(reasoning, sender=parent.agent_id)`

This means the delegate's reasoning is the mission briefing. The delegate schema description should guide the LLM to write a thorough reasoning for this tool — not a one-liner but a multi-line explanation of what the delegated agent should do, why, and what context it needs.

Updated delegate schema description:

```python
DESCRIPTION = (
    "Spawn a new agent cloned from this agent. "
    "Returns the new agent's mail address. "
    "IMPORTANT: The reasoning field for this tool is sent as the first message "
    "to the spawned agent — write a thorough mission briefing: what to do, why, "
    "what context is needed, and what to report back."
)
```

The spawn logic sends the reasoning as the first message after `start()`:

```python
delegate.start()
if reasoning:
    delegate.send(reasoning, sender=parent.agent_id, wait=False)
```

Note: `reasoning` is popped from args in `_execute_single_tool` before dispatch. The delegate handler needs access to it. Two options: (a) pass it through a side channel, or (b) don't pop `reasoning` for the delegate tool. Option (b) is simpler — the delegate handler reads `args.get("reasoning")`, uses it as the first prompt, and ignores it for schema dispatch. The implementation should choose the cleanest approach.

## 5. Package Exports

```python
# __init__.py updates
from .stoai_agent import StoAIAgent   # NEW
from .base_agent import BaseAgent     # was from .agent
from .message import Message, MSG_REQUEST, MSG_USER_INPUT  # was from .agent
from .state import AgentState         # was from .agent

__all__ = [
    # Core
    "BaseAgent",
    "StoAIAgent",          # NEW
    "Message",
    "AgentState",
    "MCPTool",
    "AgentConfig",
    "UnknownToolError",
    # Capabilities
    "setup_capability",
    "BashManager",
    "DelegateManager",
    "EmailManager",
    # Services (unchanged)
    ...
]
```

`StoAIAgent` becomes the primary public-facing class. `BaseAgent` still exported for advanced use.

### Import Path Migration

All internal imports change from `from .agent import ...` to the new locations:

| Symbol | Old location | New location |
|--------|-------------|--------------|
| `BaseAgent` | `agent.py` | `base_agent.py` |
| `Message`, `_make_message`, `MSG_REQUEST`, `MSG_USER_INPUT` | `agent.py` | `message.py` |
| `AgentState` | `agent.py` | `state.py` |

Affected internal files:
- `capabilities/__init__.py` — `TYPE_CHECKING` import of `BaseAgent`
- `capabilities/delegate.py` — imports `BaseAgent`, changes to import `StoAIAgent`
- All test files that import from `stoai.agent`

No backward-compatibility re-exports from `base_agent.py`. Clean break.

## 6. Test Migration

| Test file | Currently uses | Changes to |
|-----------|---------------|------------|
| `test_agent.py` | `BaseAgent` | Stays `BaseAgent` — kernel tests |
| `test_layers_bash.py` | `BaseAgent` + `add_capability` | `StoAIAgent(capabilities=["bash"])` |
| `test_layers_email.py` | `BaseAgent` + `add_capability` | `StoAIAgent(capabilities=["email"])` |
| `test_layers_delegate.py` | `BaseAgent` + `add_capability` | `StoAIAgent(capabilities=["delegate"])` |
| `test_vision_capability.py` | `BaseAgent` + `add_capability` | `StoAIAgent(capabilities=["vision"])` |
| `test_web_search_capability.py` | `BaseAgent` + `add_capability` | `StoAIAgent(capabilities=["web_search"])` |
| `test_layers_talk.py` | `BaseAgent` + `add_capability` | `StoAIAgent(capabilities=["talk"])` |
| `test_layers_draw.py` | `BaseAgent` + `add_capability` | `StoAIAgent(capabilities=["draw"])` |
| `test_layers_compose.py` | `BaseAgent` + `add_capability` | `StoAIAgent(capabilities=["compose"])` |
| `test_layers_listen.py` | `BaseAgent` + `add_capability` | `StoAIAgent(capabilities=["listen"])` |
| `test_three_agent_email.py` | `BaseAgent` + capabilities | `StoAIAgent` with capabilities |
| `test_intrinsics_file.py` | `BaseAgent` | Stays `BaseAgent` |
| `test_intrinsics_comm.py` | `BaseAgent` | Stays `BaseAgent` |

**Note:** `test_agent.py` stays on `BaseAgent` but its `mcp_tools=` test must be rewritten to use `add_tool()` directly (since `mcp_tools=` is removed from BaseAgent). Tests that import `Message`, `AgentState`, etc. from `stoai.agent` must update import paths.

### New Tests

- `test_stoai_agent.py` — StoAIAgent construction, capabilities dict/list normalization, `tools=` param, seal-after-start enforcement
- `test_shutdown.py` — status shutdown action, reason logging, `_shutdown_reason` set
