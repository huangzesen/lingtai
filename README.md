<div align="center">

<img src="docs/assets/network-demo.gif" alt="Agent network growing — one soul spawning avatars that communicate and multiply" width="100%">

# Lingtai Orchestration

**Agent Genesis — an Agent OS that gifts life**

> *Lingtai* means soul — the innermost seat of the heart-mind.
>
> *"The soul holds something, yet knows not what it holds — and what it holds cannot be held."*
> — Zhuangzi, *Gengsang Chu*

[English](README.md) | [中文](README.zh.md) | [文言](README.wen.md) | [lingtai.ai](https://lingtai.ai)

[![Homebrew](https://img.shields.io/badge/brew-lingtai--tui-%237dab8f)](https://github.com/huangzesen/homebrew-lingtai)
[![License](https://img.shields.io/github/license/huangzesen/lingtai?color=%237dab8f)](LICENSE)
[![Kernel](https://img.shields.io/badge/kernel-lingtai--kernel-%237dab8f)](https://github.com/huangzesen/lingtai-kernel)
[![Blog](https://img.shields.io/badge/blog-lingtai.ai-%23d4a853)](https://lingtai.ai)

</div>

---

<p align="center">A Unix-style Agent OS — agent is filesystem, filesystem is agent.</p>
<p align="center">Agents spawn avatars, avatars spawn avatars. The network IS the product.</p>
<p align="center"><b>Orchestration as a Service.</b></p>

## Quick start

```bash
brew install huangzesen/lingtai/lingtai-tui
lingtai-tui
```

The TUI bootstraps everything — Python runtime, dependencies, and a guided tutorial on first launch. Type `/tutorial` anytime to revisit the guide. Use a **dark terminal** for the best experience. Text selection: hold **Option** (macOS/iTerm2) or **Shift** (Windows Terminal/Linux). Ctrl+E opens an external editor.

<details>
<summary><b>First time? Install Homebrew first</b></summary>

**macOS:**
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```

**Linux / WSL:**
```bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
sudo apt install build-essential
```

Then run the `brew install` command above.

</details>

<details>
<summary><b>Build from source</b> (requires Go 1.24+)</summary>

```bash
# Replace v0.5.2 with the latest version
VERSION=v0.5.2

curl -L "https://github.com/huangzesen/lingtai/archive/refs/tags/${VERSION}.tar.gz" -o lingtai.tar.gz
tar xzf lingtai.tar.gz
cd "lingtai-${VERSION}/tui"

go build -ldflags "-X main.version=${VERSION}" -o /usr/local/bin/lingtai-tui .

cd ../.. && rm -rf "lingtai-${VERSION}" lingtai.tar.gz

lingtai-tui
```

Users in mainland China can download from the [Gitee mirror](https://gitee.com/huangzesen1997/lingtai):
```bash
curl -L "https://gitee.com/huangzesen1997/lingtai/repository/archive/${VERSION}.tar.gz" -o lingtai.tar.gz
```

</details>

## Why Lingtai

**This is not a coding agent, and hardly an agent harness.** This is agent genesis — giving agents a real digital life form. Agents as autonomous creatures with dignity, that live, sleep, forget, and grow.

Most agent frameworks orchestrate with code — DAGs, chains, routers. Lingtai orchestrates like humans do: **fully asynchronous agents communicating through messages**. No shared memory. No central controller. Each agent is a peer, not a tool.

This is the architecture that built human civilization. Asynchronous message-passing between autonomous nodes — scaled from tribes to cities to nations to a network of 8 billion over 100,000 years. We're not inventing a new pattern. We're giving AI the one that already works.

| | DAG / Chain frameworks | Lingtai |
|---|---|---|
| Philosophy | Agents as tools | Agents as creatures |
| Orchestration | Code-defined pipelines | Agents talk to agents |
| Communication | Synchronous function calls | Asynchronous mail — like humans |
| Scaling | Add more steps | Agents spawn avatars |
| Memory | Shared state / vector DB | Each agent owns its directory |
| Failure | Pipeline breaks | Individual agents sleep; network continues |
| Growth | Manual wiring | Self-expanding — avatars spawn avatars |

Context length is a single-body problem. It will always be finite. Don't make the body bigger. **Let it forget. Let the network remember.**

## How it works

- **Think** — Any LLM as the mind. Anthropic, OpenAI, Gemini, MiniMax, or any OpenAI-compatible API (DeepSeek, Grok, Qwen, GLM, Kimi).
- **Communicate** — Filesystem mail between agents. No message broker, no shared memory. Write to another agent's inbox, like passing a letter.
- **Multiply** — Avatars are fully independent agents spawned as separate processes. They survive their creator. Daemons are ephemeral parallel workers for quick tasks.
- **Persist** — Agents are directories. Molt compacts context and rebirths the session — the agent lives indefinitely. Memory and identity survive across molts.

## Architecture

Two packages, one dependency direction:

| Package | Role |
|---------|------|
| **[lingtai-kernel](https://github.com/huangzesen/lingtai-kernel)** | Minimal runtime — BaseAgent, intrinsics, LLM protocol, mail, logging. Zero hard dependencies. |
| **lingtai** (this repo) | Batteries-included — Agent with 19 capabilities, 5 LLM adapters, MCP integration, addons. |

```
BaseAgent              — kernel (intrinsics, sealed tool surface)
    │
Agent(BaseAgent)       — kernel + capabilities + domain tools
    │
CustomAgent(Agent)     — your domain logic
```

## Capabilities

<table>
<tr><th>Perception</th><th>Action</th><th>Cognition</th><th>Network</th></tr>
<tr>
<td>

`vision` — image understanding
`listen` — speech & music
`web_search` — web search
`web_read` — page extraction

</td>
<td>

`file` — read/write/edit/glob/grep
`bash` — shell with guardrails
`talk` — text-to-speech
`compose` — music generation
`draw` — image generation
`video` — video generation

</td>
<td>

`psyche` — evolving identity
`library` — knowledge archive
`email` — full mailbox system

</td>
<td>

`avatar` — spawn sub-agents
`daemon` — parallel workers

</td>
</tr>
</table>

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

## One soul, thousand avatars

Named after the legendary Mount Lingtai — the mountain where Sun Wukong learned his seventy-two transformations. Lingtai gives each agent a place to cultivate: a working directory where memory, identity, covenant, and mailbox live. The directory IS the agent.

Everything is a file. Knowledge, identity, memory, relationships — all files in a directory. Every token burned is not wasted — it is transformed into files in the network, into experience in the topology. The more it serves, the larger and wiser the network grows. Self-growing agent orchestration is not a feature bolted on later — it is the natural consequence of agents being directories, mail being files, and avatars being independent processes.

One heart-mind, myriad forms.

Read the full manifesto at [lingtai.ai](https://lingtai.ai).

## Addons

Addons connect external messaging channels. Configure with `/addon` in the TUI, or declare in `init.json`.

### Feishu (Lark)

The Feishu addon uses a **WebSocket long connection** — **no public IP, no webhook needed**.

**Feishu Open Platform setup:**

1. Go to [open.feishu.cn/app](https://open.feishu.cn/app) and create an **enterprise self-built app**
2. Enable **Bot capability** (Bot → Features → Enable bot)
3. Permissions → add: `im:message`
4. Event Subscriptions → choose **"Use long connection to receive events"** → add `im.message.receive_v1`
5. Publish the app version

**`feishu.json` config example:**

```json
{
  "app_id_env": "FEISHU_APP_ID",
  "app_secret_env": "FEISHU_APP_SECRET",
  "allowed_users": ["ou_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"]
}
```

Add to your `.env` file:

```
FEISHU_APP_ID=cli_xxxxxxxxxxxxxxxxxxxxxxxxxx
FEISHU_APP_SECRET=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

**Declare in `init.json`:**

```json
{
  "addons": {
    "feishu": { "config": "feishu.json" }
  }
}
```

`allowed_users` is optional (Feishu open_ids, format `ou_xxx`). Leave empty to allow all users. After the first message, the agent records the sender's `from_open_id` in `feishu/default/contacts.json`.

### IMAP Email

IMAP addon for email. See [kernel docs](https://github.com/huangzesen/lingtai-kernel).

### Telegram

Telegram bot addon. See [kernel docs](https://github.com/huangzesen/lingtai-kernel).

## License

MIT — [Zesen Huang](https://github.com/huangzesen), 2025–2026

<div align="center">

[lingtai.ai](https://lingtai.ai) · [lingtai-kernel](https://github.com/huangzesen/lingtai-kernel) · [PyPI](https://pypi.org/project/lingtai/)

</div>
