# Agent Isolation Design — Path as Identity, Kernel as Protocol

**Date:** 2026-03-22
**Scope:** lingtai-kernel + lingtai
**Motivation:** Each agent instance should be a fully independent program running in its own directory. The kernel defines protocols (rules), not implementations. Unix philosophy: the kernel provides syscall interfaces, userspace provides the drivers.

---

## Design Principles

1. **Path IS identity.** The working directory path is the agent's unique identifier. No separate `agent_id`. Mail address = path. Lock = path. Manifest = path.
2. **Kernel defines protocols, not implementations.** `LLMService` and `ChatSession` are ABCs in the kernel. How they're fulfilled (adapters, API keys, rate limiting) is the app's concern.
3. **Each agent is self-sufficient within its directory.** The kernel's contract: "give me a `working_dir` and a `service`, I'll run in it." It doesn't know how the directory was created or how the service was configured.
4. **Agent name (真名) is a display name.** Stored in manifest, used by agents/humans for convenience. Kernel stores it, never routes by it.

---

## Change 1 — WorkingDir Takes a Path

### Current (kernel decides path structure)

```python
# WorkingDir.__init__
def __init__(self, base_dir: Path | str, agent_id: str) -> None:
    self._path = self._base_dir / agent_id      # kernel's opinion

# BaseAgent.__init__
def __init__(self, service, *, agent_id: str, base_dir: str | Path, ...):
    self._workdir = WorkingDir(base_dir=base_dir, agent_id=self.agent_id)
```

### New (caller owns the path)

```python
# WorkingDir.__init__
def __init__(self, working_dir: Path | str) -> None:
    self._path = Path(working_dir)
    self._path.mkdir(parents=True, exist_ok=True)
    # no agent_id validation — caller's responsibility

# BaseAgent.__init__
def __init__(self, service, *, working_dir: str | Path, ...):
    self._workdir = WorkingDir(working_dir)
    self._working_dir = self._workdir.path
```

### What moves to lingtai / orchestrator

- The `.lingtai/{hex_id}/` directory convention
- `agent_id` generation (6-char hex in Go, 12-char hex in Python)
- Path-safety validation (no slashes, etc.)
- `base_dir` concept

### Affected files in lingtai-kernel

| File | Change |
|------|--------|
| `workdir.py` | `__init__(working_dir)` instead of `__init__(base_dir, agent_id)`. Remove `_agent_id` field, `_base_dir` field, path-safety validation. |
| `base_agent.py` | Constructor takes `working_dir: Path` instead of `base_dir: Path` + `agent_id: str`. Remove `self.agent_id` property. Remove `self._base_dir`. Billboard path (`{agent_id}.json`) changes to use working_dir basename. Thread names (`agent-{agent_id}`, `soul-{agent_id}`, `heartbeat-{agent_id}`) change to use `agent_name` or working_dir basename. `status()` and `_log()` output `"address"` instead of `"agent_id"`. |
| `session.py` | `SessionManager.__init__` — remove `agent_id` parameter. Display name fallback changes from `agent_id` to working_dir basename. |
| `intrinsics/mail.py` | `_is_self_send()` currently checks `address == agent.agent_id` — change to check against `str(agent._working_dir)`. `"from"` field fallback uses `agent_id` when no mail service — change to `str(agent._working_dir)`. |
| `intrinsics/soul.py` | Grep for `agent_id` — replace with working_dir path or agent_name. |
| `intrinsics/eigen.py` | Grep for `agent_id` — replace with working_dir path or agent_name. |
| `intrinsics/system.py` | Grep for `agent_id` — replace with working_dir path or agent_name. |
| `services/mail.py` | `MailService` ABC has `expected_agent_id` parameter on `send()`. `FilesystemMailService.send()` validates `agent_meta.get("agent_id")`. Remove `agent_id`-based verification — verify by address (path) match only. |
| `handshake.py` | No direct `agent_id` logic, but `manifest()` returns dict that callers check `agent_id` from. After manifest no longer has `agent_id`, callers are unaffected (they use `address` instead). |
| `config.py` | `AgentConfig` has no `agent_id` field — no change needed. |

### Affected files in lingtai

| File | Change |
|------|--------|
| `agent.py` | Pass `working_dir=` to `super().__init__()`. Own the `base_dir / agent_id` convention. Update `revive()` — currently reads `agent_meta.get("agent_id")` and passes to constructor. |
| `capabilities/avatar.py` | Construct avatar `working_dir` path (currently uses `agent_id=avatar_id`, `parent._base_dir`). Own child directory convention. Update sender identity from `parent.agent_id` to `str(parent._working_dir)`. |
| `capabilities/email.py` | Uses `self._agent.agent_id` as fallback (lines 609, 801). Contacts schema stores `agent_id` field with i18n strings — change to store `address` (path). Update tool schema and i18n (en.json, zh.json, wen.json). |
| `network.py` | `AgentNode.agent_id` field, `_discover_agents()` keys by `agent_id` from manifest. All query methods take `agent_id`. Rekey entirely by `address` (path) or directory basename. |
| Go: `config/loader.go` | Remove `AgentID` from `Config` struct. Remove agent_id validation. `WorkingDir()` returns a direct path field instead of computing `ProjectDir/AgentID`. `DisplayName()` fallback changes. |
| Go: `setup/wizard.go` | `generateAgentID()` still generates a hex ID for directory naming, but it becomes a directory name convention in the Go layer, not a kernel identity concept. |
| Go: `tui/root.go`, `tui/status.go` | `activeID` tracks by path or directory basename instead of `agent_id`. Status scan reads `address` from manifest instead of `agent_id`. |
| Go: `agent/process.go` | Passes full `working_dir` path. No `agent_id` in process metadata. |
| Go: `internal/mail.go` | Writes `address` to manifest instead of `agent_id`. |
| Examples (`chat_agent.py`, `contemplate.py`, `chat_web.py`) | Replace `agent_id=secrets.token_hex(3)` + `base_dir=` with `working_dir=`. |

### Manifest changes

Current `.agent.json`:
```json
{
  "agent_id": "185c8e",
  "agent_name": "alice",
  "address": "/path/to/agents/185c8e",
  ...
}
```

New `.agent.json`:
```json
{
  "agent_name": "alice",
  "address": "/path/to/agents/185c8e",
  ...
}
```

`agent_id` field removed. `address` is the working directory path (already is today). `agent_name` is optional display name.

---

## Change 2 — LLMService: ABC in Kernel, Concrete in Lingtai

### Current state

`lingtai_kernel.llm.service` contains a **concrete** `LLMService` class that mixes protocol with implementation:

- Protocol: `create_session()`, `resume_session()`, `generate()`, `make_tool_result()`, `.model`, `.provider`
- Implementation: adapter registry (class-level), adapter cache, key resolution, litellm context window fetching, session ID generation, session tracking

`lingtai_kernel.llm.base` contains `LLMAdapter` ABC and `APICallGate` import — adapter concepts in the kernel.

### New design

**Kernel keeps** (`lingtai_kernel.llm`):

```python
# lingtai_kernel/llm/service.py — ABC only

class LLMService(ABC):
    """Protocol for LLM access. Kernel depends only on this."""

    @property
    @abstractmethod
    def model(self) -> str:
        """Default model identifier."""

    @property
    @abstractmethod
    def provider(self) -> str:
        """Provider name."""

    @abstractmethod
    def create_session(
        self,
        system_prompt: str,
        tools: list[FunctionSchema] | None = None,
        *,
        model: str | None = None,
        thinking: str = "default",
        agent_type: str = "",
        tracked: bool = True,
        interaction_id: str | None = None,
        json_schema: dict | None = None,
        force_tool_call: bool = False,
        provider: str | None = None,
        interface: ChatInterface | None = None,
    ) -> ChatSession: ...

    @abstractmethod
    def resume_session(
        self, saved_state: dict, *, thinking: str = "high"
    ) -> ChatSession: ...

    @abstractmethod
    def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        system_prompt: str | None = None,
        temperature: float | None = None,
        json_schema: dict | None = None,
        max_output_tokens: int | None = None,
        provider: str | None = None,
    ) -> LLMResponse: ...

    @abstractmethod
    def make_tool_result(
        self, tool_name: str, result: dict, *, tool_call_id: str | None = None,
        provider: str | None = None,
    ) -> ToolResultBlock: ...
```

**Kernel also keeps** (unchanged):
- `ChatSession` ABC — `lingtai_kernel.llm.base`
- Data types — `LLMResponse`, `ToolCall`, `UsageMetadata`, `FunctionSchema`, `ToolResultBlock`
- `ChatInterface` — canonical conversation history

**Moves to lingtai** (`lingtai.llm`):

| What | From | To |
|------|------|----|
| Concrete `LLMService` (adapter-based) | `lingtai_kernel.llm.service` | `lingtai.llm.service` |
| `LLMAdapter` ABC | `lingtai_kernel.llm.base` | `lingtai.llm.base` |
| `APICallGate` | `lingtai_kernel.llm.api_gate` | `lingtai.llm.api_gate` |
| `_setup_gate()` / `_gated_call()` | On `LLMAdapter` in kernel | On `LLMAdapter` in lingtai |
| Adapter registry + cache | In `LLMService` | In lingtai's concrete `LLMService` |
| litellm context window fetching | `lingtai_kernel.llm.service` | `lingtai.llm.service` |
| `get_context_limit()` | `lingtai_kernel.llm.service` | `lingtai.llm.service` |
| Session ID generation | `lingtai_kernel.llm.service` | `lingtai.llm.service` |

**Kernel's `llm/` package after cleanup:**

```
lingtai_kernel/llm/
├── __init__.py          # re-exports
├── service.py           # LLMService ABC only
├── base.py              # ChatSession ABC + data types (LLMResponse, ToolCall, etc.)
├── interface.py         # ChatInterface + content blocks
└── streaming.py         # StreamingAccumulator (stays — used by adapter implementations)
```

No `api_gate.py`. No adapter concept. No litellm. No key resolution.

**Note on `streaming.py`:** `StreamingAccumulator` is a utility used by adapter implementations. It stays in the kernel as a shared building block — it has no adapter dependencies, just processes chunks into `LLMResponse` objects.

**Note on `generate()`:** The kernel's agent loop never calls `generate()` — SessionManager uses `create_session()` + `session.send()`. Soul whisper also uses `create_session()`. `generate()` is a one-shot convenience method used by integration tests and capability-level code. It belongs in the ABC for completeness, but implementors should know the core agent loop only requires `create_session()`, `resume_session()`, and `make_tool_result()`.

### Migration path

1. Create `LLMService` ABC in kernel (extract from current concrete class)
2. Move concrete class to `lingtai.llm.service` — import ABC, subclass it
3. Move `LLMAdapter`, `APICallGate` to `lingtai.llm`
4. Update all lingtai adapter imports
5. Update `lingtai_kernel.llm.__init__` exports
6. Verify: `python -c "import lingtai_kernel"` — no adapter/gate imports
7. Verify: `python -c "import lingtai"` — everything still works

---

## What Does NOT Change

- **LLMService injection pattern** — caller creates it, passes to `BaseAgent`. Kernel doesn't police sharing.
- **Mail service** — already per-agent, address = working_dir path.
- **Heartbeat** — already per-agent, `.agent.heartbeat` in working_dir.
- **Logging service** — already per-agent, `logs/events.jsonl` in working_dir.
- **Lock mechanism** — `fcntl.flock` on `.agent.lock`, unchanged.
- **ChatSession ABC** — stays in kernel.
- **Data types** — stay in kernel.
- **Git operations in WorkingDir** — unchanged, operate on `self._path`.

---

## Isolation Summary

After these changes, each agent is fully independent:

| Resource | Owner | Isolation |
|----------|-------|-----------|
| Working directory | Caller creates, kernel runs in it | Exclusive `.agent.lock` |
| LLM service | Caller creates, injects | One per agent (convention) |
| Rate limiting | LLM service implementation | Per-service instance |
| Chat session | Created by LLM service | Per-agent |
| Mail | Per-agent, address = working_dir | Filesystem IPC |
| Heartbeat | Per-agent, in working_dir | Liveness proof |
| Logging | Per-agent, in working_dir | Own event log |
| Identity | working_dir path | Path = ID |
| Display name | agent_name (真名) | Optional, cosmetic |

The kernel is a runtime that takes a directory and a service, runs in it, and communicates with peers via filesystem mail. Everything else is the caller's responsibility.

---

## Test Migration

All tests that construct `BaseAgent` or `Agent` will break. The change is mechanical:

**Before:**
```python
agent = BaseAgent(service, agent_id="test", base_dir=tmp_path, ...)
```

**After:**
```python
agent = BaseAgent(service, working_dir=tmp_path / "test", ...)
```

### Kernel tests (~10 files)

- `test_workdir.py` — Remove `test_invalid_agent_id_raises` and `test_arbitrary_agent_id`. Add tests for `WorkingDir(path)` with various path types.
- `test_base_agent.py`, `test_heartbeat.py`, `test_intrinsics_comm.py`, and all other agent-constructing tests — update constructor calls.
- Tests that assert `agent.agent_id` — change to assert `agent.working_dir` or remove.

### Lingtai tests (~15 files)

- `test_agent_capabilities.py`, `test_git_init.py`, `test_eigen.py`, `test_compaction.py`, `test_layers_draw.py`, etc. — all update constructor calls from `agent_id=` + `base_dir=` to `working_dir=`.

### Go tests

- All Go tests with hardcoded `agent_id` in manifest maps — remove the field, verify by `address`.

---

## No Backward Compatibility

This is a clean break. Old agent directories with `agent_id` in their manifests are not migrated. Recreate agents if needed.
