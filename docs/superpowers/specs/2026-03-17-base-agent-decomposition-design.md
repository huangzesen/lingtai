# BaseAgent Decomposition

## Problem

`base_agent.py` is a 1940-line monolith handling ~10 distinct responsibilities. This makes it hard to navigate, hard to test individual concerns in isolation, and hard to swap out subsystems.

## Solution

Decompose BaseAgent into four extracted components via composition, leaving BaseAgent as a ~500-line kernel coordinator.

### New Module Structure

```
src/lingtai/
  base_agent.py          (~500 lines — kernel coordinator)
  workdir.py             (NEW — WorkingDir class)
  session.py             (NEW — SessionManager class)
  tool_executor.py       (NEW — ToolExecutor class)
  intrinsics/
    mail.py              (add handle() function)
    clock.py             (add handle() function)
    status.py            (add handle() function)
    system.py            (add handle() function)
```

## Component 1: `WorkingDir` (`workdir.py`)

Owns the agent's working directory — filesystem locking, git initialization, manifest persistence, and git diff/commit operations.

### Interface

```python
class WorkingDir:
    def __init__(self, base_dir: Path, agent_id: str): ...

    @property
    def path(self) -> Path: ...

    # Lock lifecycle
    def acquire_lock(self) -> None: ...
    def release_lock(self) -> None: ...

    # Git operations
    def init_git(self) -> None: ...
    def diff(self, filepath: str) -> str: ...              # read-only git diff
    def diff_and_commit(self, filepath: str, message: str) -> tuple[str, str | None]: ...

    # Manifest
    def read_manifest(self) -> tuple[str, str]: ...      # (covenant, memory)
    def write_manifest(self, manifest: dict) -> None: ...
```

### What moves here

- `_acquire_lock()`, `_release_lock()` and the cross-platform `_lock_fd`/`_unlock_fd` helpers
- `_git_init_working_dir()`
- `_git_diff_and_commit()`
- `_read_manifest()`, `_write_manifest()`

### Dependencies

None on BaseAgent. Pure filesystem/subprocess operations on a `Path`. Also absorbs the `sys.platform` file-locking branch at module level.

## Component 2: `SessionManager` (`session.py`)

Owns the LLM chat session — creation, send/retry/reset, streaming, token tracking, and context compaction.

### Interface

```python
class SessionManager:
    def __init__(
        self,
        llm_service: LLMService,
        config: AgentConfig,
        prompt_manager: SystemPromptManager,
        build_system_prompt_fn: Callable[[], str],
        build_tool_schemas_fn: Callable[[], list[FunctionSchema]],
    ): ...

    @property
    def chat(self) -> ChatSession | None: ...

    # Core operations
    def ensure_session(self) -> ChatSession: ...
    def send(self, message: str) -> LLMResponse: ...
    def send_streaming(self, message: str, callbacks) -> LLMResponse: ...
    def reset(self, chat: ChatSession, failed_message) -> tuple[ChatSession, str]: ...

    # Token tracking
    def track_usage(self, response: LLMResponse) -> None: ...
    def get_token_usage(self) -> dict: ...
    def check_and_compact(self) -> None: ...
    def update_token_decomposition(self) -> None: ...

    # Persistence
    def get_chat_state(self) -> dict | None: ...
    def restore_chat(self, state: dict | None) -> None: ...
    def restore_token_state(self, state: dict) -> None: ...
```

### What moves here

- `_ensure_session()`
- `_llm_send()` → `send()`
- `_llm_send_streaming()` → `send_streaming()`
- `_on_reset()` → `reset()`
- `_check_and_compact()` → `check_and_compact()`
- `_update_token_decomposition()` → `update_token_decomposition()`
- `_track_usage()` → `track_usage()`
- `get_token_usage()`
- `get_chat_state()`, `restore_chat()`, `restore_token_state()`

### Dependencies

Receives `LLMService`, `AgentConfig`, `SystemPromptManager` at construction. Uses two callbacks (`build_system_prompt_fn`, `build_tool_schemas_fn`) to get current prompt/schemas without needing a reference to BaseAgent.

### Design notes

- `send()` encapsulates the timeout/retry/stale-interaction-recovery logic currently in `_llm_send()`. Tools are obtained internally via `build_tool_schemas_fn`, not passed per-call (matching current `_llm_send` which takes only `message`).
- `reset(chat, failed_message)` encapsulates the rollback-on-server-error logic currently in `_on_reset()`. Returns `(new_chat, rollback_msg)` tuple — the caller uses both.
- Token state (`_cumulative_input`, `_cumulative_output`, etc.) lives on SessionManager
- Compaction logic calls `llm_service.compact()` and updates the session's interface

## Component 3: `ToolExecutor` (`tool_executor.py`)

Executes tool calls — decides sequential vs parallel, handles timing, error wrapping, guard checks, and intercept hooks.

### Interface

```python
class ToolExecutor:
    def __init__(
        self,
        dispatch_fn: Callable[[ToolCall], str],
        make_tool_result_fn: Callable,
        guard: LoopGuard,
        parallel_safe_tools: set[str],
        logger_fn: Callable | None = None,
    ): ...

    def execute(
        self,
        tool_calls: list[ToolCall],
        *,
        on_result_hook: Callable | None = None,
    ) -> list[dict]: ...
```

### What moves here

- `_execute_single_tool()` → `_execute_single()`
- `_execute_tools_sequential()` → `_execute_sequential()`
- `_execute_tools_parallel()` → `_execute_parallel()`

### What stays on BaseAgent

- `_dispatch_tool()` — the 2-layer routing table (intrinsics + MCP handlers). This is passed to ToolExecutor as `dispatch_fn`.
- `_process_response()` — the outer loop that calls `executor.execute()` and feeds results back to the LLM.

### Dependencies

Receives `dispatch_fn` (a closure over BaseAgent's tool tables), `make_tool_result_fn` (wraps `service.make_tool_result` — formats results for the LLM provider), `LoopGuard`, and `parallel_safe_tools` at construction. No reference to BaseAgent.

### Design notes

- Single public method `execute()`. Internally decides sequential vs parallel based on tool call count and `parallel_safe_tools` membership.
- `make_tool_result_fn` wraps `service.make_tool_result(name, result, tool_call_id, provider)` — every execution path needs to format results for the LLM provider. BaseAgent passes a closure: `lambda name, result, tc_id: self.service.make_tool_result(name, result, tool_call_id=tc_id, provider=self._config.provider)`.
- `on_result_hook` replaces the current `_on_tool_result_hook` call — ToolExecutor calls it after each tool execution, and if it returns a non-None string, that string replaces the tool result (intercept pattern). This is a callback, so subclass overrides work via late binding.
- Owns `ThreadPoolExecutor` lifecycle — creates per-batch (matching current behavior).
- Error handling (UnknownToolError, guard blocks, exceptions) is encapsulated — results always come back as dicts with `result` or `error` fields.

## Component 4: Intrinsic Handlers in `intrinsics/*.py`

Each intrinsic module gains a `handle(agent, args) -> str` function alongside its existing `SCHEMA` and `DESCRIPTION`.

### Pattern

```python
# intrinsics/mail.py
# existing: SCHEMA, DESCRIPTION

def handle(agent, args: dict) -> str:
    """Handle mail intrinsic calls."""
    action = args.get("action")
    if action == "send":
        return _send(agent, args)
    elif action == "read":
        return _read(agent)
    raise ValueError(f"Unknown mail action: {action}")

def _send(agent, args: dict) -> str:
    # Body of current BaseAgent._mail_send
    ...

def _read(agent) -> str:
    # Body of current BaseAgent._mail_read
    ...
```

### What moves where

| Current BaseAgent method | Destination |
|---|---|
| `_handle_mail`, `_mail_send`, `_mail_read` | `intrinsics/mail.py` |
| `_handle_clock`, `_clock_check`, `_clock_wait` | `intrinsics/clock.py` |
| `_handle_status`, `_status_shutdown`, `_status_show` | `intrinsics/status.py` |
| `_handle_system`, `_system_diff`, `_system_load` | `intrinsics/system.py` |

### Wiring in BaseAgent

```python
def _wire_intrinsics(self):
    for name, mod in ALL_INTRINSICS.items():
        if hasattr(mod, 'handle'):
            self._intrinsic_handlers[name] = lambda args, m=mod: m.handle(self, args)
```

### Agent state accessed by handlers

Each handler accesses agent state via the `agent` parameter. The specific attributes each handler needs:

- **mail**: `agent._mail_service`, `agent._mail_queue`, `agent._mail_queue_lock`, `agent.agent_id`, `agent._admin`, `agent._working_dir`, `agent._log()`
- **clock**: `agent._cancel_event`, `agent._mail_arrived`, `agent._log()`
- **status**: `agent.agent_id`, `agent._state`, `agent._chat` (→ `agent._session.chat` after SessionManager extraction), `agent._mail_service`, `agent._uptime_anchor`, `agent._started_at`, `agent._shutdown` (threading.Event), `agent._working_dir`, `agent.get_token_usage()`, `agent._log()`
- **system**: `agent._working_dir` (for `diff()` and `diff_and_commit()`), `agent.update_system_prompt()`, `agent._prompt_manager`, `agent._chat` (→ `agent._session.chat`), `agent._token_decomp_dirty`, `agent._build_system_prompt()`, `agent._log()`

This is an explicit dependency — each handler documents exactly what agent surface it touches.

## What Stays on BaseAgent (~500 lines)

### Initialization
- `__init__` — creates `WorkingDir`, `SessionManager`, `ToolExecutor`, wires intrinsics, sets up mail queue and state

### Lifecycle
- `start()`, `stop()`

### Main loop and message routing
- `_run_loop()` — wait for inbox, process messages
- `_handle_message()` — route by message type
- `_handle_request()` — send to LLM, process response
- `_process_response()` — tool call loop, delegates execution to ToolExecutor, feeds results back via SessionManager
- `_handle_cancel_diary()` — sends LLM call to write diary on cancellation (called from `_process_response`)

### Mail routing (MailService callbacks)
- `_on_mail_received()`, `_on_normal_mail()`

### State
- `_set_state()`, `_log()`, properties (`is_idle`, `state`, `working_dir`)

### Public API
- `add_tool()`, `remove_tool()`, `override_intrinsic()`, `update_system_prompt()`
- `send()`, `mail()`, `status()`

### Schema building
- `_build_tool_schemas()`, `_build_system_prompt()` — these know about the agent's tool registry and prompt sections, so they stay here and are passed as callbacks to SessionManager

### Hooks (overridable by subclasses)
- `_pre_request()`, `_post_request()`, `_on_tool_result_hook()`, `_deliver_result()`

### Session delegation (thin wrappers)
- `get_chat_state()` → `self._session.get_chat_state()`
- `restore_chat()` → `self._session.restore_chat()`
- `restore_token_state()` → `self._session.restore_token_state()`
- `get_token_usage()` → `self._session.get_token_usage()`

## Dependency Flow

```
BaseAgent (kernel coordinator)
  ├── owns WorkingDir        (no back-reference to agent)
  ├── owns SessionManager    (no back-reference; callbacks for prompt/schemas)
  ├── owns ToolExecutor      (no back-reference; dispatch_fn callback)
  └── wires intrinsics       (handlers receive agent as explicit parameter)
```

No component imports or references BaseAgent. Communication is via:
- Constructor injection (services, config, callbacks)
- Return values
- The `agent` parameter for intrinsic handlers (documented interface)

## Migration Strategy

Each extraction is independent and can be done as a separate commit. Order:

1. **`WorkingDir`** — zero coupling to other extractions, simplest boundary
2. **Intrinsic handlers** — zero coupling, mostly mechanical move
3. **`ToolExecutor`** — zero coupling, clean dispatch_fn interface
4. **`SessionManager`** — most intertwined with message loop, do last

Each step: extract code → update BaseAgent to delegate → run full test suite → commit.

## Testing Impact

- Existing tests continue to work unchanged (BaseAgent's public API doesn't change)
- New unit tests can be added for each component in isolation:
  - `WorkingDir`: give it a temp directory, verify git init/manifest/lock behavior
  - `ToolExecutor`: give it a mock dispatch function, verify sequential/parallel execution
  - `SessionManager`: give it a mock LLMService, verify send/retry/reset/compaction
  - Intrinsic handlers: give them a mock agent, verify each action

## Migration Warnings

### Anima capability accesses `agent._chat` directly

The anima capability (`capabilities/anima.py`) reads `self._agent._chat`, `self._agent._interaction_id`, and `self._agent._token_decomp_dirty` in multiple places. It also calls `self._agent._build_system_prompt()` and `self._agent.service.check_and_compact()` — its compaction logic partially duplicates BaseAgent's own `_check_and_compact()`. When SessionManager is extracted (step 4), these references must be updated to `self._agent._session.chat` etc., and the duplicated compaction logic should be reconciled. This is the highest-risk migration step.

### Tests call intrinsic handlers by method name

Tests call `agent._handle_mail()`, `agent._handle_clock()`, `agent._handle_status()`, and `agent._handle_system()` directly across `test_agent.py`, `test_clock.py`, `test_status.py`, `test_system.py`, and `test_conscience.py`. After extraction, these methods no longer exist on BaseAgent. Additionally, `test_conscience.py` asserts `agent._intrinsics["clock"] == agent._handle_clock` (identity check against bound method). Either:
- Update all tests to call via `intrinsics/module.handle(agent, args)` directly, or
- Keep thin forwarding methods on BaseAgent (less clean but less churn)

### Email capability monkey-patches `_on_normal_mail()`

The email capability replaces `agent._on_normal_mail` at runtime. This method stays on BaseAgent, so no issue — but worth documenting since it's a runtime mutation of agent behavior.

### `_system_diff()` does raw subprocess, not `_git_diff_and_commit()`

The system intrinsic's `diff` action runs its own `git diff` and `git status --porcelain` via subprocess. This is read-only (no commit). WorkingDir needs a separate `diff(filepath)` method for this, distinct from `diff_and_commit()`.

## Non-goals

- Not changing the `Agent` subclass or capabilities layer
- Not changing any public API signatures
- Not changing the tool dispatch model (2-layer intrinsics + MCP)
- Not introducing async — this stays synchronous/threaded
