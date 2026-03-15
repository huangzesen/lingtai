# StoAI — Design Discussion Log (2026-03-15)

Key decisions from the brainstorming session that led to the current design.

## Naming

- Started as `xhelio-agents` (monorepo subfolder in xhelio-dev)
- User wanted a standalone identity, inspired by 百家争鸣 (A Hundred Schools of Thought Contend)
- Explored Greek equivalents: stoa, agora, symposium, dialectic, polyphony
- **Final name: StoAI** — Stoa (Greek philosophical porch) + AI
- PyPI: available. GitHub: `huangzesen/stoai` available.

## Three-Tier Model

Evolved through discussion:

1. **Intrinsics** (8 tools) — what the agent *is*. Irreducible core.
2. **Layers** — what the agent *can do*. Composable via `add_tool()` + `update_system_prompt()`.
3. **MCP tools** — what the agent *works on*. Domain context from host app.

Key decisions:
- `run_code` belongs to xhelio (domain), not base agent
- `manage_diary` and `plan` moved from intrinsics to layers ("is it possible to wrap layer after layer?" → yes)
- `manage_system_prompt` is Python API only, NOT an LLM-callable tool
- `role` is part of system prompt, not a constructor param
- No `diary_dir` or `ltm_dir` in constructor — inject via layers

## Services Architecture

User insight: "all of these services are optional. Compaction and inline suggestions are examples [of agents that need nothing]."

Key decisions:
- **5 services**: LLMService, FileIOService, EmailService, VisionService, SearchService
- **All optional** — even LLMService (an agent with no LLM is a valid message router)
- Vision and web_search abstracted out of LLMService even though current impl wraps LLM — "tomorrow vision could be a dedicated model, web_search could be a Brave API call"
- Same pattern as LLM adapters: abstract contract, implement one backend, add more later
- FileIOService starts text-only, later adds PDF etc. via `RichFileIOService`

## Email (formerly Talk)

User: "We can make an inbox, allow black list or white list, pretty much like xhelio's orchestrator inbox. Always allow talk, its the listeners responsibility to check the mail box. (maybe we should rename talk to email)"

Key decisions:
- **Renamed `talk` → `email`** — sets right expectations (async, fire-and-forget)
- **No request/response coupling** — a reply is just another email
- **No threading, no conversation IDs** — just `{from, to, message}`
- **No registry at base level** — "if you know the tcp port, talk, if you don't never mind"
- **Filtering (allowlist/blocklist) is a layer**, not base concern
- **EmailService** abstracts transport — TCP first, others later
- User: "enforce request/response or even sync feature are introduced in upper layer"

## TCP Transport

User: "is ip:tcp really the best approach? in long term this would be great, agents can even talk over internet"

Key decisions:
- Address format is the EmailService's concern, not BaseAgent's
- TCP is the first implementation
- BaseAgent passes opaque address strings to `email_service.send()`
- Later: Unix sockets, WebSocket, pipes, etc.
- User: "current performance won't matter" — favoring clean decoupling

## Agent OS Realization

The discussion converged on StoAI being an agent operating system:

| OS Concept | StoAI |
|------------|-------|
| Kernel | Services |
| System calls | Intrinsics |
| Userspace libs | Layers |
| Device drivers | MCP tools |
| Network stack | Forum (future) |
| Processes | Agent instances |
| IPC | Email |

User confirmed: "Yes I do think so, because ultimately the xhelio is just a collection of tools with context, nothing more."

## Disabling Intrinsics

User: "one last check, we can always disable these intrinsic tools right? like make it read only"

Three levels:
1. **No service** — don't pass the service, intrinsics never exist
2. **`disabled_intrinsics`** — construction-time, hidden from LLM
3. **`remove_tool()`** — runtime, layers can revoke access

## Future Vision

- **Persistent specialist agents** as services (librarian, watcher, etc.)
- **Forum** — registry + discovery + bulletin board (not coordination)
- **Emergent routing** — agents develop working relationships via diary
- **Location transparency** — same `email()` call works local or internet
- **Three packages**: stoai → forum → domain apps (xhelio, etc.)
