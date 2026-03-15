# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is StoAI

StoAI (Stoa + AI) is a generic agent framework — an "agent operating system" providing the minimal kernel for AI agents: thinking (LLM), perceiving (vision, search), acting (file I/O), and communicating (inter-agent email). Domain tools, coordination, and orchestration are plugged in from outside via MCP-compatible interfaces.

The primary consumer is **xhelio** (space data platform), which provides MCP tools, memory, and session management on top of StoAI.

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

### Three-Tier Tool Model

| Tier | What | How added |
|------|------|-----------|
| **Intrinsics** | Core capabilities the agent *is* (read, edit, write, glob, grep, talk, vision, web_search) | Built-in, can be disabled at construction |
| **Layers** | Composable capabilities (diary, plan) added via `add_tool()` + `update_system_prompt()` | `add_diary_layer(agent)`, `add_plan_layer(agent)` |
| **MCP tools** | Domain context from the host app | Passed as `mcp_tools=[MCPTool(...)]` at construction |

### Key Modules

- **`agent.py`** — `BaseAgent` class. 2-state lifecycle (SLEEPING/ACTIVE), persistent LLM session, 2-layer tool dispatch (intrinsics + MCP handlers), inbox-based inter-agent messaging, context compaction, loop guard, parallel tool execution.
- **`llm/interface.py`** — `ChatInterface`, the canonical provider-agnostic conversation history. Single source of truth — adapters rebuild provider formats from this. Content blocks: `TextBlock`, `ToolCallBlock`, `ToolResultBlock`, `ThinkingBlock`, `ImageBlock`.
- **`llm/base.py`** — `LLMAdapter` (ABC), `ChatSession` (ABC), `LLMResponse`, `ToolCall`, `FunctionSchema`. All agent code depends on these, never on provider SDKs directly.
- **`llm/service.py`** — `LLMService`. Adapter factory, session registry, one-shot generation gateway, context compaction orchestration. Decoupled from config files — uses injected `key_resolver` and `provider_defaults`.
- **`llm/interface_converters.py`** — Bidirectional converters between `ChatInterface` and provider-specific formats (Anthropic, OpenAI, Gemini).
- **`intrinsics/`** — Each file exports `SCHEMA`, `DESCRIPTION`, `handle_*`. Some (talk, vision, web_search) have `handler=None` because they need agent state and are handled in `BaseAgent`.
- **`layers/`** — Each layer is a function that takes an agent and wires in a tool + system prompt section. Layers compose independently.
- **`config.py`** — `AgentConfig` dataclass. Host app injects resolved values; no file-based config inside stoai.
- **`prompt.py`** — Builds system prompt from base template + `SystemPromptManager` sections + MCP tool descriptions.

### LLM Provider Adapters

10 adapters under `llm/`, each lazy-imported. Most use OpenAI-compatible SDK: Gemini (`google-genai`), OpenAI, Anthropic, MiniMax, DeepSeek, Grok, Qwen, GLM, Kimi, Custom. Each adapter subdirectory has `adapter.py` (implementation) and `defaults.py` (model defaults).

### Extension Pattern

```python
agent.add_tool(name, schema, handler)          # add tool (layer or MCP)
agent.remove_tool(name)                        # remove from LLM schema
agent.update_system_prompt(section, content)   # inject system prompt section (Python API, NOT an LLM tool)
```

### System Prompt Structure

Base prompt (hardcoded with intrinsic list) → Sections (injected by host/layers via `update_system_prompt`) → MCP tool descriptions (auto-generated). Protected sections cannot be modified by the LLM's `manage_system_prompt` intrinsic.

## Conventions

- Python 3.11+, `from __future__ import annotations` used throughout.
- Dataclasses preferred over dicts for structured data.
- No file-based config inside stoai — all config injected via constructor args.
- All services optional — missing service auto-disables backed intrinsics.
- Provider SDKs lazy-imported — only active provider needs installation.
- Tests use `unittest.mock.MagicMock` for LLM service mocking. Test functions follow `test_<what_is_tested>` naming.
- Event system uses string constants (`EVENT_TOOL_CALL`, `EVENT_TEXT_DELTA`, etc.) with `on_event(type, payload)` callback.
