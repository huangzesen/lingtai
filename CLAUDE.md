# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is StoAI

StoAI (Stoa + AI) is a generic agent framework — an "agent operating system" providing the minimal kernel for AI agents: thinking (LLM), perceiving (vision, search), acting (file I/O), and communicating (inter-agent email). Domain tools, coordination, and orchestration are plugged in from outside via MCP-compatible interfaces.

## Build & Test

```bash
# Activate the venv (required — stoai is installed in editable mode)
source venv/bin/activate

# Run all tests
python -m pytest tests/

# Run a single test file
python -m pytest tests/test_agent.py

# Run a single test
python -m pytest tests/test_agent.py::test_agent_starts_and_stops -v

# Smoke-test after editing a module
python -c "import stoai"
```

No hard dependencies — only the active LLM provider's SDK needs to be installed. Optional deps: `pip install stoai[gemini]`, `stoai[openai]`, `stoai[anthropic]`, `stoai[minimax]`, or `stoai[all]`.

## Architecture

### Five Services (all optional)

| Service | What it backs | First implementation |
|---------|--------------|---------------------|
| `LLMService` | Core agent loop (thinking) | Gemini adapter |
| `FileIOService` | read, edit, write, glob, grep | `LocalFileIOService` |
| `EmailService` | email (inter-agent messaging) | `TCPEmailService` |
| `VisionService` | vision | `LLMVisionService` |
| `SearchService` | web_search | `LLMSearchService` |

Missing service = intrinsics backed by it auto-disabled. `FileIOService` auto-creates `LocalFileIOService` for backward compat if not passed.

### Three-Tier Tool Model

| Tier | What | How added |
|------|------|-----------|
| **Intrinsics** | Core capabilities (read, edit, write, glob, grep, email, vision, web_search) | Built-in, backed by services, can be disabled |
| **Capabilities** | Composable capabilities (bash, delegate) via `add_capability()` | `agent.add_capability("bash")`, `agent.add_capability("delegate")` |
| **MCP tools** | Domain context from the host app | Passed as `mcp_tools=[MCPTool(...)]` at construction |

### Key Modules

- **`agent.py`** — `BaseAgent` class. 2-state lifecycle (SLEEPING/ACTIVE), 5 optional services, persistent LLM session, 2-layer tool dispatch (intrinsics + MCP), inbox-based inter-agent messaging via EmailService, context compaction, loop guard, parallel tool execution.
- **`services/`** — Service ABCs + first implementations: `file_io.py`, `email.py`, `vision.py`, `search.py`.
- **`llm/interface.py`** — `ChatInterface`, the canonical provider-agnostic conversation history. Single source of truth — adapters rebuild provider formats from this. Content blocks: `TextBlock`, `ToolCallBlock`, `ToolResultBlock`, `ThinkingBlock`, `ImageBlock`.
- **`llm/base.py`** — `LLMAdapter` (ABC), `ChatSession` (ABC), `LLMResponse`, `ToolCall`, `FunctionSchema`. All agent code depends on these, never on provider SDKs directly.
- **`llm/service.py`** — `LLMService`. Adapter factory, session registry, one-shot generation gateway, context compaction orchestration. Decoupled from config files — uses injected `key_resolver` and `provider_defaults`.
- **`llm/interface_converters.py`** — Bidirectional converters between `ChatInterface` and provider-specific formats (Anthropic, OpenAI, Gemini).
- **`intrinsics/`** — Each file exports `SCHEMA`, `DESCRIPTION`, `handle_*`. Some (email, vision, web_search) have `handler=None` because they need agent state and are handled in `BaseAgent`.
- **`capabilities/`** — Each capability module exports `setup(agent, **kwargs)`. Added via `agent.add_capability("name")`. 2 built-in: bash, delegate.
- **`config.py`** — `AgentConfig` dataclass. Host app injects resolved values; no file-based config inside stoai.
- **`prompt.py`** — Builds system prompt from base template + `SystemPromptManager` sections + MCP tool descriptions.

### LLM Provider Adapters

10 adapters under `llm/`, each lazy-imported. Most use OpenAI-compatible SDK: Gemini (`google-genai`), OpenAI, Anthropic, MiniMax, DeepSeek, Grok, Qwen, GLM, Kimi, Custom. Each adapter subdirectory has `adapter.py` (implementation) and `defaults.py` (model defaults).

### Extension Pattern

```python
agent.add_capability("bash")                   # add a named capability (bash, delegate, ...)
agent.add_capability("bash", "delegate")       # add multiple at once
agent.add_tool(name, schema, handler)          # add a custom/MCP tool (low-level)
agent.remove_tool(name)                        # remove from LLM schema
agent.update_system_prompt(section, content)   # inject system prompt section (Python API, NOT an LLM tool)
```

### System Prompt Structure

Base prompt (hardcoded with intrinsic list) → Sections (injected by host/capabilities via `update_system_prompt`) → MCP tool descriptions (auto-generated). Protected sections cannot be modified by the LLM's `manage_system_prompt` intrinsic.

## Conventions

- Python 3.11+, `from __future__ import annotations` used throughout.
- Dataclasses preferred over dicts for structured data.
- No file-based config inside stoai — all config injected via constructor args.
- All services optional — missing service auto-disables backed intrinsics.
- Provider SDKs lazy-imported — only active provider needs installation.
- Tests use `unittest.mock.MagicMock` for LLM service mocking. Test functions follow `test_<what_is_tested>` naming.
- Legacy `talk` intrinsic renamed to `email`. Legacy `connect()`/`talk()` API kept for backward compat but new code should use `EmailService`.
