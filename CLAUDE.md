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

### Three-Layer Agent Hierarchy

```
BaseAgent              — kernel (intrinsics, sealed tool surface)
    |
StoAIAgent(BaseAgent)  — kernel + capabilities + domain tools
    |
CustomAgent(StoAIAgent) — host's wrapper (subclass with domain logic)
```

- **BaseAgent** (`base_agent.py`) — pure kernel. 9 intrinsics, `add_tool()`/`remove_tool()` sealed after `start()`. No capabilities. `update_system_prompt()` stays open.
- **StoAIAgent** (`stoai_agent.py`) — accepts `capabilities=` (list or dict) and `tools=` (MCPTool list) at construction. `get_capability(name)` for manager access.
- **Custom agents** — subclass StoAIAgent, add domain tools via `add_tool()` or `_setup_capability()` in `__init__`.

### Four Services (all optional)

| Service | What it backs | First implementation |
|---------|--------------|---------------------|
| `LLMService` | Core agent loop (thinking) | Gemini adapter |
| `FileIOService` | read, edit, write, glob, grep | `LocalFileIOService` |
| `MailService` | mail (point-to-point FIFO messaging) | `TCPMailService` |
| `LoggingService` | structured JSONL event logging (auto-created in working dir) | `JSONLLoggingService` |

Missing service = intrinsics backed by it auto-disabled. `FileIOService` auto-creates `LocalFileIOService` for backward compat if not passed. `LoggingService` auto-creates `JSONLLoggingService` at `{working_dir}/logs/events.jsonl` if not passed. `VisionService` and `SearchService` are capability-level — passed via `capabilities={"vision": {"vision_service": svc}}`.

### Three-Tier Tool Model

| Tier | What | How added |
|------|------|-----------|
| **Intrinsics** | Core capabilities (read, edit, write, glob, grep, mail, clock, status+shutdown, memory) | Built-in, backed by services, can be disabled |
| **Capabilities** | Composable capabilities (bash, delegate, email, vision, web_search, talk, compose, draw, listen) | Declared at construction via `capabilities=` on StoAIAgent |
| **MCP tools** | Domain context from the host app | Passed as `tools=[MCPTool(...)]` on StoAIAgent, or `add_tool()` in subclass constructors |

### Key Modules

- **`base_agent.py`** — `BaseAgent` class (kernel). 2-state lifecycle (SLEEPING/ACTIVE), 4 optional services, persistent LLM session, 2-layer tool dispatch (intrinsics + tools), FIFO mail queue via MailService, structured JSONL logging, git-controlled working dir, context compaction, loop guard, parallel tool execution. Tool surface sealed after `start()`.
- **`stoai_agent.py`** — `StoAIAgent(BaseAgent)`. Accepts `capabilities=` and `tools=` at construction. Tracks `_capabilities` for delegate replay. `get_capability(name)` returns manager instances.
- **`state.py`** — `AgentState` enum (ACTIVE, SLEEPING).
- **`message.py`** — `Message` dataclass, `_make_message`, `MSG_REQUEST`, `MSG_USER_INPUT`.
- **`services/`** — Service ABCs + first implementations: `file_io.py`, `mail.py`, `vision.py`, `search.py`, `logging.py`.
- **`llm/interface.py`** — `ChatInterface`, the canonical provider-agnostic conversation history. Single source of truth — adapters rebuild provider formats from this. Content blocks: `TextBlock`, `ToolCallBlock`, `ToolResultBlock`, `ThinkingBlock`, `ImageBlock`.
- **`llm/base.py`** — `LLMAdapter` (ABC), `ChatSession` (ABC), `LLMResponse`, `ToolCall`, `FunctionSchema`. All agent code depends on these, never on provider SDKs directly.
- **`llm/service.py`** — `LLMService`. Adapter factory, session registry, one-shot generation gateway, context compaction orchestration. Decoupled from config files — uses injected `key_resolver` and `provider_defaults`.
- **`llm/interface_converters.py`** — Bidirectional converters between `ChatInterface` and provider-specific formats (Anthropic, OpenAI, Gemini).
- **`intrinsics/`** — Each file exports `SCHEMA`, `DESCRIPTION`, `handle_*`. Some (mail, clock, status, memory) have `handler=None` because they need agent state and are handled in `BaseAgent`. Status intrinsic supports `show` and `shutdown` actions.
- **`capabilities/`** — Each capability module exports `setup(agent, **kwargs)`. 9 built-in: bash, delegate, email, vision, web_search, talk, compose, draw, listen. The email capability upgrades the mail FIFO with a persistent mailbox, reply/reply_all, CC/BCC, and multi-to. Delegate spawns `StoAIAgent` with reasoning as first prompt.
- **`config.py`** — `AgentConfig` dataclass. Host app injects resolved values; no file-based config inside stoai.
- **`prompt.py`** — Builds system prompt from base template + `SystemPromptManager` sections + MCP tool descriptions.

### LLM Provider Adapters

10 adapters under `llm/`, each lazy-imported. Most use OpenAI-compatible SDK: Gemini (`google-genai`), OpenAI, Anthropic, MiniMax, DeepSeek, Grok, Qwen, GLM, Kimi, Custom. Each adapter subdirectory has `adapter.py` (implementation) and `defaults.py` (model defaults).

### Built-in Capabilities (9)

| Capability | Usage | What it adds |
|-----------|-------|-------------|
| `bash` | `capabilities={"bash": {"policy_file": "p.json"}}` or `{"bash": {"yolo": True}}` | Shell command execution with policy |
| `delegate` | `capabilities=["delegate"]` | Spawn peer agents (reasoning = first prompt) |
| `email` | `capabilities=["email"]` | Persistent mailbox — upgrades mail FIFO with reply, CC/BCC, search, check |
| `vision` | `capabilities=["vision"]` or `{"vision": {"vision_service": svc}}` | Image understanding (LLM multimodal or dedicated VisionService) |
| `web_search` | `capabilities=["web_search"]` or `{"web_search": {"search_service": svc}}` | Web search (LLM grounding or dedicated SearchService) |
| `talk` | `capabilities=["talk"]` | Text-to-speech via MiniMax MCP |
| `compose` | `capabilities=["compose"]` | Music generation via MiniMax MCP |
| `draw` | `capabilities=["draw"]` | Text-to-image via MiniMax MCP |
| `listen` | `capabilities=["listen"]` | Speech transcription + music analysis |

### Extension Pattern

```python
# Layer 2: StoAIAgent with capabilities
agent = StoAIAgent(
    agent_id="alice", service=svc, base_dir="/agents",
    capabilities=["vision", "web_search", "bash"],      # list form (no kwargs)
)
agent = StoAIAgent(
    agent_id="bob", service=svc, base_dir="/agents",
    capabilities={"bash": {"policy_file": "p.json"}},   # dict form (with kwargs)
    tools=[MCPTool(name="query_db", ...)],               # domain tools
)

# Layer 3: Custom agent subclass
class ResearchAgent(StoAIAgent):
    def __init__(self, **kwargs):
        super().__init__(capabilities=["vision", "web_search"], **kwargs)
        self._setup_capability("bash", policy_file="research.json")
        self.add_tool("query_db", schema={...}, handler=db_handler)

# Low-level API (on BaseAgent, sealed after start)
agent.add_tool(name, schema=schema, handler=handler)     # register tool
agent.remove_tool(name)                                   # unregister tool
agent.update_system_prompt(section, content)              # inject prompt section (open at any time)
```

Note: `capabilities=` accepts `list[str]` (no kwargs) or `dict[str, dict]` (with kwargs per capability). `add_tool()` and `remove_tool()` raise `RuntimeError` after `start()`.

### System Prompt Structure

Base prompt (hardcoded with intrinsic list + shutdown guidance) → Sections (injected by host/capabilities via `update_system_prompt`) → MCP tool descriptions (auto-generated). Protected sections cannot be modified by the LLM's `manage_system_prompt` intrinsic.

## Conventions

- Python 3.11+, `from __future__ import annotations` used throughout.
- Dataclasses preferred over dicts for structured data.
- No file-based config inside stoai — all config injected via constructor args.
- All services optional — missing service auto-disables backed intrinsics.
- Provider SDKs lazy-imported — only active provider needs installation.
- Tests use `unittest.mock.MagicMock` for LLM service mocking. Test functions follow `test_<what_is_tested>` naming.
- Migrations should be complete and clean — remove old code entirely. No backward-compatibility shims, no deprecated wrappers, no legacy aliases unless the user explicitly asks for them.
