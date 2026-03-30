<div align="center">

# 灵台 LingTai

**Self-growing agent network**

> *灵台者有持，而不知其所持，而不可持者也。*
> *The spirit platform holds something, yet knows not what it holds — and what it holds cannot be held.*
> — 庄子 · 庚桑楚

[English](README.md) | [中文](README.zh.md) | [文言](README.wen.md) | [lingtai.ai](https://lingtai.ai)

[![PyPI](https://img.shields.io/pypi/v/lingtai?color=%237dab8f)](https://pypi.org/project/lingtai/)
[![Python](https://img.shields.io/pypi/pyversions/lingtai?color=%237dab8f)](https://pypi.org/project/lingtai/)
[![License](https://img.shields.io/github/license/huangzesen/lingtai?color=%237dab8f)](LICENSE)
[![Kernel](https://img.shields.io/badge/kernel-lingtai--kernel-%237dab8f)](https://github.com/huangzesen/lingtai-kernel)

</div>

---

## One soul, thousand avatars

Lingtai is not a coding assistant. It is an **agent operating system** — a runtime where agents think, communicate, spawn avatars, and grow into networks.

Named after 灵台方寸山 — the mountain where 孙悟空 (Sun Wukong) learned his 72 transformations. Lingtai gives each agent a place to cultivate: a working directory on disk where memory, identity, covenant, and mailbox live. The directory IS the agent. Agents communicate through filesystem mail. They spawn avatars that become independent agents with their own directories, their own mail, their own LLM sessions. Those avatars can spawn their own. The self-growing network of avatars is the agent itself.

One heart-mind (一心), myriad forms (万相).

## How it works

- **Think** — Any LLM as the mind. Anthropic, OpenAI, Gemini, MiniMax, or any OpenAI-compatible API (DeepSeek, Grok, Qwen, GLM, Kimi).
- **Communicate** — Filesystem mail between agents. No message broker, no shared memory. Write to another agent's inbox, like passing a letter.
- **Multiply** — Avatars (分身) are fully independent agents spawned as separate processes. They survive their creator. Daemons (神識) are ephemeral parallel workers for quick tasks.
- **Persist** — Agents are directories. Molt (凝蜕) compacts context and rebirth the session — the agent lives indefinitely. Memory and identity survive across molts.

## Quick start

```bash
brew install huangzesen/lingtai/lingtai-tui
lingtai-tui
```

The TUI guides you through creating your first agent — pick an LLM provider, configure capabilities, and launch. Run `lingtai-tui tutorial` for a guided walkthrough.

Python runtime (`pip install lingtai`) is installed automatically on first launch.

## Architecture

Two packages, one dependency direction:

| Package | Role |
|---------|------|
| **[lingtai-kernel](https://github.com/huangzesen/lingtai-kernel)** | Minimal runtime — BaseAgent, intrinsics, LLM protocol, mail, logging. Zero hard dependencies. |
| **lingtai** (this repo) | Batteries-included — Agent with 19 capabilities, 5 LLM adapters, MCP integration, addons. |

Three-layer agent hierarchy:

```
BaseAgent              — kernel (intrinsics, sealed tool surface)
    │
Agent(BaseAgent)       — kernel + capabilities + domain tools
    │
CustomAgent(Agent)     — your domain logic
```

## Capabilities

### Perception

| Capability | What it adds |
|-----------|-------------|
| `vision` | Image understanding |
| `listen` | Speech transcription, music analysis |
| `web_search` | Web search (DuckDuckGo, MiniMax, Gemini, or custom) |
| `web_read` | Web page content extraction |

### Action

| Capability | What it adds |
|-----------|-------------|
| `file` | Read, write, edit, glob, grep (group shorthand) |
| `bash` | Shell execution with policy-based guardrails |
| `talk` | Text-to-speech |
| `compose` | Music generation |
| `draw` | Image generation |
| `video` | Video generation |

### Cognition

| Capability | What it adds |
|-----------|-------------|
| `psyche` | Evolving identity, character development |
| `library` | Persistent knowledge archive |
| `email` | Full mailbox — reply, CC/BCC, contacts, archive, scheduled sends |

### Network

| Capability | What it adds |
|-----------|-------------|
| `avatar` | Spawn independent sub-agents (分身) as separate processes |
| `daemon` | Ephemeral parallel workers (神識) for quick concurrent tasks |

## Agent = directory

```
/agents/wukong/
  .agent.lock               ← exclusive lock (one process per directory)
  .agent.heartbeat          ← liveness proof
  .agent.json               ← manifest
  system/
    covenant.md             ← protected instructions (survive molts)
    memory.md               ← working notes
  mailbox/
    inbox/                  ← received messages
    outbox/                 ← pending sends
    sent/                   ← delivery audit trail
  logs/
    events.jsonl            ← structured event log
```

No `agent_id`. The path is the identity. Agents find each other by path, communicate by writing to each other's `mailbox/inbox/`. Like passing letters between houses.

## Extend

Compose capabilities:

```python
agent = Agent(
    service=service,
    working_dir="/agents/bajie",
    capabilities=["file", "bash", "email", "avatar"],
)
```

Subclass for domain logic:

```python
class ResearchAgent(Agent):
    def __init__(self, **kwargs):
        super().__init__(
            capabilities=["file", "vision", "web_search", "avatar"],
            **kwargs,
        )
        self.add_tool("query_db", schema={...}, handler=db_handler)
```

Connect MCP servers for domain tools:

```python
await agent.connect_mcp("npx -y @modelcontextprotocol/server-filesystem /data")
```

## License

MIT — [Zesen Huang](https://github.com/huangzesen), 2025–2026

<div align="center">

[lingtai.ai](https://lingtai.ai) · [lingtai-kernel](https://github.com/huangzesen/lingtai-kernel) · [PyPI](https://pypi.org/project/lingtai/)

</div>
