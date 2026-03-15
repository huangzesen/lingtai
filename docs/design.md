# StoAI — Design Document

> **Stoa + AI.** Named after the ancient Greek stoa — the open porch where independent thinkers gathered, debated, and contended (百家争鸣). Agents that stand on their own, communicate freely, and compose into larger systems.

## Vision

StoAI is an **agent operating system**. It provides the minimal kernel for AI agents: thinking (LLM), perceiving (vision, search), acting (file I/O), and communicating (email). Everything else — domain tools, coordination, orchestration — is plugged in from outside.

```
┌─────────────────────────────────────────────┐
│              Applications                    │
│  xhelio (space data)  │  future apps...     │
│  = MCP tools + context │  = MCP tools + context│
├─────────────────────────────────────────────┤
│              Forum (future package)          │
│  Registry · Discovery · Bulletin board      │
├─────────────────────────────────────────────┤
│              StoAI                           │
│                                             │
│  Layers:  bash · diary · plan · delegate    │
│                                             │
│  Intrinsics:  read edit write glob grep     │
│               email vision web_search       │
│                                             │
│  Services: LLM · FileIO · Email · Vision ·  │
│            Search                           │
└─────────────────────────────────────────────┘
```

Building a new app:

```python
from stoai import BaseAgent, LLMService, LocalFileIOService
from stoai.layers import add_diary_layer, add_plan_layer

agent = BaseAgent(agent_id="my_agent", service=llm, file_io=fs)
add_diary_layer(agent)
add_plan_layer(agent)
agent.add_tool("my_domain_tool", schema=..., handler=...)
agent.update_system_prompt("role", "You are a ...")
agent.run()
```

No domain dependency. Just an agent with tools.

## Three-Tier Model

| Tier | What it is | Examples |
|------|-----------|----------|
| **Intrinsics** | What the agent *is* — irreducible core capabilities | read, edit, write, glob, grep, email, vision, web_search |
| **Layers** | What the agent *can do* — composable capabilities added via `add_tool()` + `update_system_prompt()` | diary, plan, bash, delegate |
| **MCP tools** | What the agent *works on* — domain context provided by the host app | xhelio data pipeline, cdaweb fetch, plotly render |

## Services Architecture

Every intrinsic is backed by a **service** — an abstract contract with pluggable implementations. Services are injected at construction. Missing service → intrinsics backed by it are automatically disabled.

| Service | What it abstracts | Intrinsics | First implementation |
|---------|------------------|------------|---------------------|
| `LLMService` | Thinking, generating text | *(core agent loop)* | Gemini adapter |
| `FileIOService` | File access | `read`, `edit`, `write`, `glob`, `grep` | `LocalFileIOService` (text files) |
| `EmailService` | Message transport | `email` | `TCPEmailService` |
| `VisionService` | Image understanding | `vision` | `LLMVisionService` (wraps LLM) |
| `SearchService` | Web search | `web_search` | `LLMSearchService` (wraps LLM) |

**All services are optional.** An agent with no services is still valid — maybe a pure message router or inbox collector.

- No `FileIOService` → `read`, `edit`, `write`, `glob`, `grep` disabled
- No `EmailService` → `email` disabled (but inbox can still receive)
- No `VisionService` → `vision` disabled
- No `SearchService` → `web_search` disabled
- No `LLMService` → no thinking, but can still process emails deterministically

Same pattern as LLM — abstract the contract, implement one backend first, add more later:

```python
class FileIOService(ABC):
    @abstractmethod
    def read(self, path: str) -> str: ...

    @abstractmethod
    def write(self, path: str, content: str) -> None: ...

    @abstractmethod
    def glob(self, pattern: str, root: str) -> list[str]: ...

    @abstractmethod
    def grep(self, pattern: str, path: str) -> list[Match]: ...

# First implementation — local filesystem, text only
class LocalFileIOService(FileIOService):
    def read(self, path: str) -> str:
        return Path(path).read_text()

# Later — PDF, images, remote filesystems, sandboxed access
class RichFileIOService(FileIOService):
    def read(self, path: str) -> str:
        if path.endswith(".pdf"):
            return extract_pdf_text(path)
        return Path(path).read_text()
```

```python
class EmailService(ABC):
    @abstractmethod
    def send(self, address: str, message: dict) -> bool: ...

    @abstractmethod
    def listen(self, on_message: Callable[[dict], None]) -> None: ...

    @abstractmethod
    def stop(self) -> None: ...

# First implementation
class TCPEmailService(EmailService):
    def __init__(self, listen_port: int | None = None): ...
```

Vision and Search are conceptually separate from "talk to an LLM" even if they use the LLM under the hood today. Tomorrow vision could be a dedicated model, web_search could be a Brave API call:

```python
class LLMVisionService(VisionService):
    """Uses multimodal LLM for vision — first impl."""
    def __init__(self, llm: LLMService): ...

class LLMSearchService(SearchService):
    """Uses LLM grounding for search — first impl."""
    def __init__(self, llm: LLMService): ...
```

## BaseAgent

### Constructor

```python
class BaseAgent:
    def __init__(
        self,
        *,
        agent_id: str,
        service: LLMService | None = None,          # optional — the brain
        file_io: FileIOService | None = None,        # optional — file access
        email: EmailService | None = None,            # optional — messaging
        vision: VisionService | None = None,          # optional — image understanding
        search: SearchService | None = None,          # optional — web search
        config: AgentConfig | None = None,
        on_event: Callable[[str, dict], None] | None = None,
        context: Any = None,                          # opaque, for host app
        enabled_intrinsics: set[str] | None = None,   # None = all enabled
        disabled_intrinsics: set[str] | None = None,  # takes precedence
    ):
```

- **No `SessionContext`** — replaced by opaque `context: Any`.
- **No `EventBus`** — replaced by `on_event(event_type, payload)` callback.
- **No `role` parameter** — role is a system prompt section injected via `update_system_prompt()`.
- **No file-based config** — host app passes `AgentConfig` with resolved values.

### Intrinsic Tools (8)

| Tool | LLM tool name | Python method | Service |
|------|--------------|---------------|---------|
| Read | `read` | `self.read_file(path)` | `FileIOService` |
| Edit | `edit` | `self.edit_file(path, old, new)` | `FileIOService` |
| Write | `write` | `self.write_file(path, content)` | `FileIOService` |
| Glob | `glob` | `self.glob(pattern)` | `FileIOService` |
| Grep | `grep` | `self.grep(pattern, path)` | `FileIOService` |
| Email | `email` | `self.email(address, message)` | `EmailService` |
| Vision | `vision` | `self.see_image(path)` | `VisionService` |
| Web Search | `web_search` | `self.web_search(query)` | `SearchService` |

**Dual interface:** Every intrinsic is both LLM-callable tool and Python method. The LLM calls `email` as a tool; a wrapper calls `self.email()` as a method.

### Disabling Intrinsics — Three Levels

| Level | How | When |
|-------|-----|------|
| No service | Don't pass `file_io=` | Construction — intrinsics never exist |
| `disabled_intrinsics` | `{"edit", "write"}` | Construction — intrinsics hidden from LLM |
| `remove_tool()` | Layer calls it | Runtime — dynamically revoke access |

```python
# Read-only agent
agent = BaseAgent(agent_id="auditor", service=llm, file_io=fs,
                  disabled_intrinsics={"edit", "write"})

# Pure thinker — no file access, no email
agent = BaseAgent(agent_id="oracle", service=llm)

# Sandbox layer removes write access after setup
def add_sandbox_layer(agent: BaseAgent):
    agent.remove_tool("edit")
    agent.remove_tool("write")
```

### Extension Points

```python
agent.add_tool(name, schema, handler)    # add tool (layer or MCP)
agent.remove_tool(name)                  # remove from LLM schema (Python method stays)
agent.update_system_prompt(section, content, protected=False)  # Python API, NOT an LLM tool
```

### Tool Dispatch — 2-Layer

```python
def _resolve_handler(self, tool_name: str):
    if tool_name in self._intrinsics:
        return self._intrinsics[tool_name]
    if tool_name in self._mcp_handlers:
        return self._mcp_handlers[tool_name]
    raise UnknownToolError(tool_name)
```

### System Prompt Structure

```
┌─────────────────────────────┐
│  Base prompt (hardcoded)     │  "You are an AI agent with these
│                              │   intrinsic tools: read, edit, ..."
├─────────────────────────────┤
│  Sections                    │  Injected by host app or layers
│  (via update_system_prompt)  │  Python API only
├─────────────────────────────┤
│  MCP tool descriptions       │  Auto-generated from mcp_tools schemas
└─────────────────────────────┘
```

Protected sections: host app marks them, LLM cannot modify.

## Email — Inter-Agent Messaging

`talk` is renamed to `email`. Fire-and-forget, no request/response coupling.

### Message Format

Every email is a one-way message:

```python
{
    "from": "localhost:8300",
    "to": "localhost:8301",
    "message": "What's the solar wind speed?"
}

# A reply is just another email
{
    "from": "localhost:8301",
    "to": "localhost:8300",
    "message": "450 km/s"
}
```

No threading. No conversation IDs. No request/response pairing. Just messages in, messages out.

### Intrinsic

```python
# LLM calls
email(host="localhost", port=8301, message="What's the solar wind speed?")
# Returns immediately: {"status": "delivered"} or {"status": "refused"}
```

The sender doesn't block. The receiver checks when ready. The address is opaque to BaseAgent — passed to `EmailService.send()`.

### Inbox

Every agent has an inbox — a queue that accepts incoming emails. The agent processes them on its own schedule, like xhelio's orchestrator inbox (`queue.Queue` with priority drain).

Filtering (allowlist/blocklist) is a **layer**, not base agent's concern. The base inbox accepts everything.

### No Registry at Base Level

The caller must know the address. If you don't know it, you can't email. Discovery is Forum's job (upper layer).

- **Delegate layer** knows addresses because it spawns agents
- **Forum** (future) provides registry and discovery
- **Config file** can list known agent addresses

## Layers — Composable Capabilities

Layers are added via `add_tool()` + `update_system_prompt()`. Each layer is a function that takes an agent and wires itself in:

```python
# layers/diary.py
def add_diary_layer(agent: BaseAgent, diary_dir: Path) -> None:
    mgr = DiaryManager(diary_dir=diary_dir)
    agent.add_tool("manage_diary", schema=DIARY_SCHEMA, handler=mgr.handle)
    agent.update_system_prompt("diary_instructions",
        "You have a diary tool. Use it to record observations...")

# layers/plan.py
def add_plan_layer(agent: BaseAgent, working_dir: Path | None = None) -> None:
    mgr = PlanManager(working_dir=working_dir)
    agent.add_tool("plan", schema=PLAN_SCHEMA, handler=mgr.handle)
    agent.update_system_prompt("plan_instructions",
        "You have a plan tool. Use it to create and track plans.")

# layers/bash.py (future)
def add_bash_layer(agent: BaseAgent) -> None:
    agent.add_tool("bash", schema=BASH_SCHEMA, handler=run_bash)
    agent.update_system_prompt("bash_instructions",
        "You can execute shell commands via the bash tool.")

# layers/delegate.py (future)
def add_delegate_layer(agent: BaseAgent, ...) -> None:
    # talk + agent spawning + role injection + MCP tool injection
    ...
```

Layers compose: one layer gives diary, next gives bash, next gives delegate. Each `add_tool` + `update_system_prompt` pair is independent.

**Adding a layer = making a more powerful agent.**
**Adding MCP tools = giving it context to work on.**

### Built-in Layers (4)

| Layer | What it adds |
|-------|-------------|
| `diary` | Immutable append-only log (save, catalogue, view) |
| `plan` | File-based planning (create, read, update, check_off) |
| `bash` | Shell command execution |
| `delegate` | Agent spawning + role injection + MCP injection + email wiring |

### Diary Layer

Immutable, append-only log for agent observations:

```
Actions:
  save       → requires: title (str), summary (one-line str), content (full text)
  catalogue  → list recent N entries (titles + summaries)
  view       → read full content by title
```

Entries are timestamped markdown files:
```
diary_dir/
  2026-03-14T10-23-45_session-resume-bug.md
```

### Plan Layer

File-based planning:

```
Actions:
  create     → create a plan file
  read       → read current plan
  update     → edit plan sections
  check_off  → mark a step complete
```

## Core Types

### MCPTool

```python
@dataclass
class MCPTool:
    """A domain tool provided via MCP-compatible interface."""
    name: str
    schema: dict
    description: str
    handler: Callable[[dict], dict]
```

### AgentConfig

```python
@dataclass
class AgentConfig:
    """Configuration injected at construction, not read from files."""
    max_turns: int = 50
    provider: str = "gemini"
    model: str | None = None
    api_key: str | None = None
    base_url: str | None = None
    retry_timeout: float = 30.0
    thinking_budget: int | None = None
    data_dir: str | None = None
```

## What BaseAgent Handles

| Feature | Reason |
|---------|--------|
| Thread lifecycle (start/stop, inbox, sleep/active) | Core agent concern |
| LLM conversation loop | Core agent concern |
| 2-layer tool dispatch (intrinsics + MCP) | Core agent concern |
| Context compaction | Core agent concern |
| Loop guard | Core agent concern |
| Token tracking + decomposition | Core agent concern |
| Streaming support | Core agent concern |
| Session save/restore | Core agent concern |
| Pre/post request hooks | Extension point |
| Parallel tool execution | Performance concern |
| Turn limits | Safety concern |

## LLM Adapters

10 provider adapters, each lazy-imported:

| Provider | Package |
|----------|---------|
| Gemini | `google-genai` |
| OpenAI | `openai` |
| Anthropic | `anthropic` |
| MiniMax | `minimax` |
| DeepSeek | `openai` (compatible) |
| Grok | `openai` (compatible) |
| Qwen | `openai` (compatible) |
| GLM | `openai` (compatible) |
| Kimi | `openai` (compatible) |
| Custom | `openai` (compatible) |

No hard dependencies — only the active provider's SDK needs to be installed.

## Package Structure

```
stoai/
  pyproject.toml
  tests/
  src/stoai/
    __init__.py

    agent.py                  ← BaseAgent class
    config.py                 ← AgentConfig dataclass
    types.py                  ← MCPTool, errors, event constants

    services/
      __init__.py
      file_io.py              ← FileIOService ABC + LocalFileIOService
      email.py                ← EmailService ABC + TCPEmailService
      vision.py               ← VisionService ABC + LLMVisionService
      search.py               ← SearchService ABC + LLMSearchService

    intrinsics/
      __init__.py
      read.py                 ← read file contents
      edit.py                 ← string-replacement file edit
      write.py                ← create/overwrite file
      glob.py                 ← find files by pattern
      grep.py                 ← search file contents by regex
      email.py                ← fire-and-forget message
      vision.py               ← image understanding
      web_search.py           ← web search

    layers/
      __init__.py
      diary.py                ← immutable agent log
      plan.py                 ← file-based planning
      bash.py                 ← shell execution (future)
      delegate.py             ← agent spawning + wiring (future)

    llm/
      __init__.py
      base.py                 ← LLMAdapter, ChatSession, LLMResponse, ToolCall
      service.py              ← LLMService
      interface.py            ← Interactions API
      interface_converters.py
      rate_limiter.py
      gemini/                 ← adapter + defaults
      openai/
      anthropic/
      minimax/
      deepseek/
      grok/
      qwen/
      glm/
      kimi/
      custom/

    # Supporting modules
    loop_guard.py
    token_counter.py
    tool_timing.py
    llm_utils.py
    prompt.py
    logging.py
```

## The Agent OS Analogy

| OS Concept | StoAI Equivalent |
|------------|-----------------|
| Kernel | Services (FileIO, Email, Vision, Search, LLM) |
| System calls | Intrinsics (read, write, email, ...) |
| Userspace libraries | Layers (diary, plan, bash, delegate) |
| Device drivers | MCP tools (domain-specific) |
| Network stack | Forum (future — registry, discovery) |
| Processes | Individual agent instances |
| IPC | Email (fire-and-forget messages) |

## Emergent Patterns

Because agents are independent services that communicate via email:

- **Persistent specialist agents** — a librarian agent maintains a knowledge base, others email it questions
- **Watcher agents** — monitors data sources, emails relevant agents when something happens
- **Working relationships** — agents that frequently interact learn each other's addresses
- **Location transparency** — local or across the internet, same `email()` call
- **Fault tolerance** — if an agent is down, emails bounce, sender handles it
- **Observable** — every interaction is a message you can log/audit

## Relationship to xhelio

xhelio becomes a **consumer** of StoAI. It provides:

- **MCP tools** — data pipeline, Plotly renderer, custom operations
- **Memory system** — domain-specific long-term memory
- **Session management** — session lifecycle, persistence
- **EventBus** — translates StoAI's `on_event` to xhelio's pub/sub system

```python
from stoai import BaseAgent, AgentConfig
from stoai.layers import add_diary_layer

# xhelio creates agents with domain tools
agent = BaseAgent(
    agent_id="orchestrator",
    service=llm_service,
    file_io=local_fs,
    config=AgentConfig(max_turns=80),
    mcp_tools=get_xhelio_tools("orchestrator", session_ctx),
    on_event=lambda t, p: event_bus.emit(translate(t, p)),
    context=session_ctx,
)
agent.update_system_prompt("role", orchestrator_role, protected=True)
agent.update_system_prompt("domain_memory", memory_content, protected=True)
add_diary_layer(agent, diary_dir=session_dir / "diary")
```

## Future: Forum Package

A coordination layer built on top of StoAI:

- **Registry** — agents register themselves, others discover by capability
- **Bulletin board** — agents post findings, others subscribe
- **Reputation** — track which agents give good answers
- **Hiring** — "I need an agent that can do X" → Forum spawns or finds one

Forum doesn't coordinate agents. Agents coordinate themselves. Forum just makes discovery easier.

## Dependencies

```toml
[project]
name = "stoai"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = []  # no hard deps

[project.optional-dependencies]
gemini = ["google-genai>=1.0"]
openai = ["openai>=1.0"]
anthropic = ["anthropic>=0.40"]
minimax = ["minimax>=0.1"]
all = ["stoai[gemini,openai,anthropic,minimax]"]
```
