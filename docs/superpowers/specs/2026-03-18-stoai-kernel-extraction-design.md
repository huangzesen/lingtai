# stoai-kernel Extraction Design

**Date:** 2026-03-18
**Status:** Draft

## Motivation

BaseAgent is approaching maturity as a standalone agent kernel — the minimal runtime for an AI agent that can think (LLM), communicate (mail), remember (eigen), and host tools. It should be extractable as a standalone package (`stoai-kernel`) so that:

- Others can build on the kernel without pulling in all 16 capabilities, addons, and multimodal features
- The kernel's stability is enforced by repository separation — changes to the kernel are deliberate
- Third parties can implement their own LLM adapters, services, and capabilities against the kernel's protocols

`stoai` becomes a batteries-included wrapper that depends on `stoai-kernel`, providing adapters, capabilities, and re-exporting the kernel's public API for backward compatibility.

## Design Analogy

The Linux kernel analogy: `stoai-kernel` provides process scheduling (tool dispatch), IPC (mail), memory management (eigen/working dir), and device driver interfaces (LLM adapter protocol, service ABCs). No userland utilities — but a fully functional agent OS.

## Package Boundary

### `stoai-kernel` (separate repo)

**Import name:** `stoai_kernel`
**Dependencies:** None (zero hard dependencies)

| Layer | Modules | Purpose |
|-------|---------|---------|
| Data types | `state.py`, `types.py`, `message.py`, `config.py`, `tool_timing.py` | Enums, dataclasses, error types |
| Utilities | `logging.py`, `token_counter.py`, `llm_utils.py`, `loop_guard.py` | Shared helpers |
| Working dir | `workdir.py` | Git-backed agent filesystem |
| LLM protocol | `llm/base.py`, `llm/interface.py`, `llm/service.py`, `llm/api_gate.py`, `llm/streaming.py` | ABCs, data types, adapter registry, session management |
| Service protocols + impls | `services/mail.py` (MailService ABC + TCPMailService), `services/logging.py` (LoggingService ABC + JSONLLoggingService) | Only the services BaseAgent depends on |
| Intrinsics | `intrinsics/mail.py`, `intrinsics/clock.py`, `intrinsics/status.py`, `intrinsics/eigen.py` | 4 kernel tools (always present) |
| Core | `base_agent.py`, `session.py`, `tool_executor.py`, `prompt.py` | Kernel coordinator, LLM session lifecycle, tool execution, system prompt |

### `stoai` (this repo, after migration)

**Import name:** `stoai`
**Dependencies:** `stoai-kernel`

| Layer | Modules | Purpose |
|-------|---------|---------|
| LLM adapters | `llm/` (all 10 provider directories + `interface_converters.py` + `_register.py`) | Adapter implementations, registered with kernel's `LLMService` |
| Agent | `agent.py` | Layer 2: capabilities dispatcher, imports `BaseAgent` from `stoai_kernel` |
| Service ABCs + impls | `services/file_io.py`, `services/vision.py`, `services/search.py`, `services/mcp.py` | FileIO, Vision, Search, MCP — ABCs and default implementations |
| Capabilities | `capabilities/` (all 16) | Composable agent tools |
| Addons | `addons/` | Optional extensions (gmail, etc.) |
| Re-exports | `__init__.py` | Re-exports kernel's public API so `from stoai import BaseAgent` works |

### What does NOT go to kernel

- **FileIOService** — file I/O is a capability concern, not kernel
- **VisionService / SearchService** — multimodal/web capabilities
- **MCPClient** — extension mechanism, not kernel
- **LLM provider adapters** — kernel defines the protocol, implementations live outside
- **`interface_converters.py`** — provider-specific conversion logic
- **All 16 capabilities** — composable tools built on top of the kernel
- **All addons** — optional extensions

## LLMService Refactoring

The current `LLMService` has two problems for the kernel split:

### Problem 1: Hardcoded adapter imports

`_create_adapter()` has a hardcoded `if/elif` chain importing from relative paths. This must become a registry.

**Solution — adapter registry:**

```python
class LLMService:
    _adapter_registry: dict[str, Callable[..., LLMAdapter]] = {}

    @classmethod
    def register_adapter(cls, name: str, factory: Callable[..., LLMAdapter]) -> None:
        """Register an adapter factory by provider name.

        factory receives keyword arguments: api_key, base_url, max_rpm,
        and (for some providers) default_model.
        """
        cls._adapter_registry[name.lower()] = factory

    def _create_adapter(self, provider: str, api_key, base_url) -> LLMAdapter:
        p = provider.lower()
        defaults = self._get_provider_defaults(provider)
        # ... build kwargs ...

        if p in self._adapter_registry:
            return self._adapter_registry[p](**kwargs)

        # Fallback: try custom adapter protocol (if registered)
        raise RuntimeError(f"No adapter registered for provider {provider!r}")
```

**Registration in `stoai`:**

```python
# stoai/llm/_register.py
from stoai_kernel.llm import LLMService

def register_all_adapters():
    def _gemini(**kw):
        from .gemini.adapter import GeminiAdapter
        return GeminiAdapter(**kw)

    def _anthropic(**kw):
        from .anthropic.adapter import AnthropicAdapter
        return AnthropicAdapter(**kw)

    # ... etc for all 10 providers ...

    LLMService.register_adapter("gemini", _gemini)
    LLMService.register_adapter("anthropic", _anthropic)
    # ...
```

Lazy import is preserved — adapter SDKs are only imported when first used.

### Problem 2: Multimodal capability methods

`LLMService` currently has: `web_search()`, `generate_vision()`, `make_multimodal_message()`, `generate_image()`, `generate_music()`, `text_to_speech()`, `transcribe()`, `analyze_audio()`.

These are multimodal capability concerns that happen to route through LLM adapters.

**Solution:** Remove all multimodal methods from `LLMService`. Capabilities that need them call `service.get_adapter(provider)` directly and invoke adapter methods themselves.

**Kernel `LLMService` retains only:**
- `__init__()`, `_create_adapter()` (registry-based), `get_adapter()`
- `create_session()`, `resume_session()`, `get_session()`
- `generate()` (one-shot text generation)
- `check_and_compact()` (context compaction)
- `make_tool_result()`
- `register_adapter()` (class method)

## Repository Structure

### `stoai-kernel/`

```
stoai-kernel/
├── src/
│   └── stoai_kernel/
│       ├── __init__.py
│       ├── base_agent.py
│       ├── config.py
│       ├── state.py
│       ├── types.py
│       ├── message.py
│       ├── workdir.py
│       ├── session.py
│       ├── tool_executor.py
│       ├── prompt.py
│       ├── logging.py
│       ├── token_counter.py
│       ├── llm_utils.py
│       ├── loop_guard.py
│       ├── tool_timing.py
│       ├── intrinsics/
│       │   ├── __init__.py
│       │   ├── mail.py
│       │   ├── clock.py
│       │   ├── status.py
│       │   └── eigen.py
│       ├── services/
│       │   ├── __init__.py
│       │   ├── mail.py
│       │   └── logging.py
│       └── llm/
│           ├── __init__.py
│           ├── base.py
│           ├── interface.py
│           ├── service.py
│           ├── api_gate.py
│           └── streaming.py
├── tests/
├── pyproject.toml
├── CLAUDE.md
└── README.md
```

**`pyproject.toml`:**

```toml
[project]
name = "stoai-kernel"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = []
description = "Minimal agent kernel — think, communicate, remember, host tools"
```

### `stoai/` (this repo, after migration)

```
stoai/
├── src/
│   └── stoai/
│       ├── __init__.py
│       ├── agent.py
│       ├── capabilities/
│       │   ├── __init__.py
│       │   ├── read.py, write.py, edit.py, glob.py, grep.py
│       │   ├── psyche.py, bash.py, conscience.py, delegate.py
│       │   ├── email.py, vision.py, web_search.py
│       │   ├── draw.py, compose.py, talk.py, listen.py
│       ├── addons/
│       │   └── gmail/
│       ├── services/
│       │   ├── __init__.py
│       │   ├── file_io.py
│       │   ├── vision.py
│       │   ├── search.py
│       │   └── mcp.py
│       └── llm/
│           ├── __init__.py
│           ├── _register.py
│           ├── interface_converters.py
│           ├── anthropic/
│           ├── openai/
│           ├── gemini/
│           ├── minimax/
│           ├── deepseek/
│           ├── grok/
│           ├── qwen/
│           ├── glm/
│           ├── kimi/
│           └── custom/
├── tests/
├── pyproject.toml
├── CLAUDE.md
└── README.md
```

**`pyproject.toml`:**

```toml
[project]
name = "stoai"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = []

[project.optional-dependencies]
kernel = ["stoai-kernel"]
gemini = ["google-genai>=1.0"]
openai = ["openai>=1.0"]
anthropic = ["anthropic>=0.40"]
minimax = ["minimax>=0.1"]
all = ["stoai[kernel,gemini,openai,anthropic,minimax]"]

[tool.setuptools.packages.find]
where = ["src"]
```

During development, `stoai-kernel` is installed via path dep: `pip install -e ../stoai-kernel`.

## Re-export Strategy

`stoai.__init__.py` re-exports the kernel's public API:

```python
# stoai/__init__.py
from stoai_kernel import (
    BaseAgent,
    AgentConfig,
    AgentState,
    Message,
    MSG_REQUEST,
    MSG_USER_INPUT,
    UnknownToolError,
)
from stoai_kernel.llm import (
    LLMService,
    LLMAdapter,
    ChatSession,
    LLMResponse,
    ToolCall,
    FunctionSchema,
    ChatInterface,
)
from stoai_kernel.services.mail import MailService, TCPMailService
from stoai_kernel.services.logging import LoggingService, JSONLLoggingService

# Own exports
from .agent import Agent
from .services.file_io import FileIOService, LocalFileIOService
# ... etc

# Register adapters on import
from .llm._register import register_all_adapters
register_all_adapters()
```

This ensures `from stoai import BaseAgent` continues to work. The split is invisible to existing users.

## Migration Strategy

1. **Create `stoai-kernel` repo** — copy kernel modules, rename all internal imports to `stoai_kernel.*`
2. **Refactor `LLMService`** — remove hardcoded adapter imports (replace with registry), remove multimodal methods
3. **Update `stoai`** — add `stoai-kernel` as path dev dependency, remove kernel modules from `src/stoai/`, update imports
4. **Update capabilities** — capabilities that used `LLMService` multimodal methods now call `service.get_adapter(provider)` directly
5. **Add re-exports** in `stoai.__init__` for backward compatibility
6. **Split tests** — kernel tests go to `stoai-kernel/tests/`, capability/agent tests stay
7. **Verify** — `pip install -e ../stoai-kernel && pip install -e .` then `python -m pytest tests/`

## Third-Party Extension Points

After this split, third parties can:

```python
# Custom LLM adapter (separate package)
from stoai_kernel.llm import LLMAdapter, ChatSession, LLMService

class MistralAdapter(LLMAdapter):
    def create_chat(self, ...) -> ChatSession: ...

LLMService.register_adapter("mistral", lambda **kw: MistralAdapter(**kw))

# Custom agent built on kernel only — no stoai dependency
from stoai_kernel import BaseAgent

class MinimalBot(BaseAgent):
    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        self.add_tool("greet", schema={...}, handler=greet_handler)
```
