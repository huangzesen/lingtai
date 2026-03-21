# 灵台 — Implementation Status

## Core
- [x] `agent.py` — BaseAgent (full lifecycle, tool dispatch, compaction, loop guard, streaming, session save/restore, 6-service architecture)
- [x] `types.py` — UnknownToolError
- [x] `config.py` — AgentConfig dataclass
- [x] `prompt.py` — system prompt builder
- [x] `__init__.py` — public API exports

## Intrinsics (8)
- [x] `intrinsics/read.py` — read file contents (backed by FileIOService)
- [x] `intrinsics/edit.py` — string-replacement edit (backed by FileIOService)
- [x] `intrinsics/write.py` — create/overwrite file (backed by FileIOService)
- [x] `intrinsics/glob.py` — find files by pattern (backed by FileIOService)
- [x] `intrinsics/grep.py` — search file contents (backed by FileIOService)
- [x] `intrinsics/email.py` — fire-and-forget inter-agent messaging (backed by EmailService)
- [x] `intrinsics/vision.py` — image understanding (backed by VisionService, falls back to LLM)
- [x] `intrinsics/web_search.py` — web search (backed by SearchService, falls back to LLM)
- [x] `intrinsics/manage_system_prompt.py` — Python API for system prompt sections

## Services (5 + LLM)
- [x] `services/file_io.py` — FileIOService ABC + LocalFileIOService (wired into BaseAgent)
- [x] `services/email.py` — EmailService ABC + TCPEmailService (wired into BaseAgent)
- [x] `services/vision.py` — VisionService ABC + LLMVisionService (wired into BaseAgent)
- [x] `services/search.py` — SearchService ABC + LLMSearchService (wired into BaseAgent)
- [x] `services/logging.py` — LoggingService ABC + JSONLLoggingService (wired into BaseAgent)

## Layers (4)
- [x] `layers/diary.py` — immutable agent log (save, catalogue, view)
- [x] `layers/plan.py` — file-based planning (create, read, update, check_off)
- [x] `layers/bash.py` — shell command execution
- [x] `layers/delegate.py` — agent spawning + role injection + MCP injection (stub — needs full implementation)

## LLM
- [x] `llm/base.py` — LLMAdapter, ChatSession, LLMResponse, ToolCall, FunctionSchema
- [x] `llm/service.py` — LLMService
- [x] `llm/interface.py` — ChatInterface (canonical conversation history)
- [x] `llm/interface_converters.py` — format converters
- [x] `llm/rate_limiter.py` — rate limiting
- [x] 10 provider adapters: gemini, openai, anthropic, minimax, deepseek, grok, qwen, glm, kimi, custom

## Supporting Modules
- [x] `loop_guard.py` — repetitive tool call detection
- [x] `token_counter.py` — token counting
- [x] `tool_timing.py` — tool execution timing
- [x] `llm_utils.py` — LLM utilities (send_with_timeout, etc.)
- [x] `logging.py` — package logging

## Tests (121 passing)
- [x] `test_agent.py` — lifecycle, intrinsics, services, email, file I/O
- [x] `test_types.py`
- [x] `test_prompt.py`
- [x] `test_layers.py` — diary, plan
- [x] `test_layers_bash.py`
- [x] `test_layers_delegate.py`
- [x] `test_intrinsics_file.py`
- [x] `test_intrinsics_comm.py`
- [x] `test_llm_utils.py`
- [x] `test_loop_guard.py`
- [x] `test_token_counter.py`
- [x] `test_services_email.py`
- [x] `test_services_file_io.py`
- [x] `test_services_logging.py`

## What Remains

### Delegate layer — full implementation
- [ ] Wire `agent_factory` to actually spawn BaseAgent instances
- [ ] Connect spawned agents via EmailService
- [ ] Implement `stop` (lifecycle management)
- [ ] Add `allowed_contacts` / `blocked_senders` to spawn args

### Forum package (future)
- [ ] Registry — agents register, others discover by capability
- [ ] Bulletin board — agents post findings, others subscribe
- [ ] Reputation tracking
