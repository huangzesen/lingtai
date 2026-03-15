# StoAI — Implementation Status

## Origin

StoAI was extracted from the xhelio project. The initial implementation lives at `xhelio-dev/packages/xhelio-agents/` with 80 passing tests.

## What Already Exists (in xhelio-dev/packages/xhelio-agents/)

All of this code needs to be **renamed** from `xhelio_agents` → `stoai` and moved here.

### Core
- [x] `agent.py` — BaseAgent (full lifecycle, tool dispatch, compaction, loop guard, streaming, session save/restore)
- [x] `types.py` — MCPTool, UnknownToolError, AgentNotConnectedError, event constants
- [x] `config.py` — AgentConfig dataclass
- [x] `prompt.py` — system prompt builder
- [x] `__init__.py` — public API exports

### Intrinsics (8)
- [x] `intrinsics/read.py` — read file contents
- [x] `intrinsics/edit.py` — string-replacement edit
- [x] `intrinsics/write.py` — create/overwrite file
- [x] `intrinsics/glob.py` — find files by pattern
- [x] `intrinsics/grep.py` — search file contents
- [x] `intrinsics/talk.py` — inter-agent messaging (needs rename to `email`)
- [x] `intrinsics/vision.py` — image understanding
- [x] `intrinsics/web_search.py` — web search
- [x] `intrinsics/manage_system_prompt.py` — Python API for system prompt sections

### Layers
- [x] `layers/diary.py` — immutable agent log (save, catalogue, view)
- [x] `layers/plan.py` — file-based planning (create, read, update, check_off)

### LLM
- [x] `llm/base.py` — LLMAdapter, ChatSession, LLMResponse, ToolCall, FunctionSchema
- [x] `llm/service.py` — LLMService
- [x] `llm/interface.py` — Interactions API interface
- [x] `llm/interface_converters.py` — format converters
- [x] `llm/rate_limiter.py` — rate limiting
- [x] 10 provider adapters: gemini, openai, anthropic, minimax, deepseek, grok, qwen, glm, kimi, custom

### Supporting Modules
- [x] `loop_guard.py` — repetitive tool call detection
- [x] `token_counter.py` — token counting
- [x] `tool_timing.py` — tool execution timing
- [x] `llm_utils.py` — LLM utilities (send_with_timeout, etc.)
- [x] `logging.py` — package logging

### Tests (80 passing)
- [x] `test_agent.py`
- [x] `test_types.py`
- [x] `test_prompt.py`
- [x] `test_layers.py`
- [x] `test_intrinsics_file.py`
- [x] `test_intrinsics_comm.py`
- [x] `test_llm_utils.py`
- [x] `test_loop_guard.py`
- [x] `test_token_counter.py`

## What Needs to Change (from today's discussion)

### 1. Rename: `xhelio_agents` → `stoai`
- Package name, imports, pyproject.toml, all references

### 2. `talk` → `email`
- Rename `intrinsics/talk.py` → `intrinsics/email.py`
- Change from `target_id` + `send_and_wait` to `address` (host:port) + fire-and-forget
- Remove `send_and_wait` (sync is an upper layer concern)
- Add inbox model (queue accepts incoming emails)

### 3. Services Architecture (NEW)
Create `services/` directory with abstract contracts:

- `FileIOService` (ABC) + `LocalFileIOService` — backs read, edit, write, glob, grep
- `EmailService` (ABC) + `TCPEmailService` — backs email intrinsic
- `VisionService` (ABC) + `LLMVisionService` — backs vision intrinsic
- `SearchService` (ABC) + `LLMSearchService` — backs web_search intrinsic

Update BaseAgent constructor:
- All 5 services optional (including LLMService)
- Missing service → intrinsics backed by it auto-disabled

### 4. New Layers (future, not blocking)
- `layers/bash.py` — shell execution
- `layers/delegate.py` — agent spawning + role injection + MCP injection

### 5. Inbox Model
- Base agent has a queue inbox for incoming emails
- No filtering at base level (that's a layer)
- No registry at base level (that's Forum)

## Migration Steps

1. **Copy** `xhelio-dev/packages/xhelio-agents/src/xhelio_agents/` → `stoai/src/stoai/`
2. **Copy** `xhelio-dev/packages/xhelio-agents/tests/` → `stoai/tests/`
3. **Rename** all `xhelio_agents` imports → `stoai`
4. **Update** `pyproject.toml` with new name
5. **Apply design changes** (services, email rename, inbox)
6. **Run tests** — all 80 should pass after rename (before design changes)
7. **Iterate** on services architecture

## Priority

Get the rename done first (mechanical), then layer in the services architecture incrementally. The existing code is functional — don't break what works.
