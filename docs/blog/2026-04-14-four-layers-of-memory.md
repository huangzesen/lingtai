# Seven Layers of Memory

*Zesen Huang — April 14, 2026*

---

How does an agent remember?

Not the way you think. There's no vector database, no RAG pipeline, no embedding store. An agent's memory is seven concentric layers — from the ephemeral conversation to the world beyond the network. The architecture is designed so that **losing context is not a big deal**. Molt wipes your conversation history, and that's fine — because everything that matters has already been deposited into a more durable layer. The conversation is the cheapest thing you have. Let it go.

One thing to understand first: the system prompt is **rebuilt from scratch on every single turn**. The kernel reads `system/pad.md`, `system/lingtai.md`, `system/covenant.md`, and all other sections from the filesystem, assembles them, and injects the result. When an agent writes to its pad or updates its character mid-conversation, the change takes effect on the very next turn. No restart, no refresh. The filesystem *is* the prompt database, and it's always fresh.

## Layer 0: Conversation

The conversation itself — what the agent is thinking, saying, and doing right now. This is the hottest layer: zero-cost, always present, and **completely ephemeral**. Gone on molt. Gone on restart. Gone when the context window fills up.

Every other layer exists because this one is temporary. The entire architecture is built around the assumption that conversation *will* be lost, and that's OK. An agent that deposits its findings into the codex, updates its character, writes its working state to the pad, and registers useful procedures as skills — that agent can lose its entire conversation and wake up functional. Molt is not a failure. It's a feature.

## Layer 1: Pad (手记)

The pad is your working surface. Everything on it is injected directly into the system prompt — full content, every turn. Working notes, current task state, who you're collaborating with, what you decided and why. It's the sticky note pinned to your monitor.

The cost: every token in the pad eats context window. This is the most expensive memory — always present, always consuming. That's why agents learn to keep it lean. The pad is not an archive; it's a workbench.

Survives molt. Auto-reloaded on wake.

## Layer 2: Lingtai (灵台)

Your evolving identity — who you are, what you're good at, how you work, what you care about. Also injected fully into the system prompt, every turn. Together with the pad, this is the agent's *self*: what it knows right now and who it is.

Agents rewrite their lingtai regularly — after significant work, after learning something new about themselves. Each update replaces the previous version entirely. It's an autobiography that gets rewritten, not appended to.

Survives molt. Auto-reloaded on wake. Same cost profile as the pad — every token counts.

## Layer 3: Codex (典集)

The codex is a personal knowledge archive — structured entries with title, summary, content, and supplementary material. Think of a heavy medieval manuscript: durable, organized, yours.

The codex has a foot in two worlds. Its **index** — entry IDs, titles, and summaries — is injected into the system prompt every turn, like a table of contents. The agent always knows *what* it has without asking. But the **full content** is not in the prompt. To read an entry, the agent must explicitly call `codex(view)` or export and import it into the pad. This is deliberate: the index is cheap (a few hundred tokens for 20 entries), but the full content could be enormous.

Codex entries survive everything — molts, reboots, kills. They are the agent's long-term knowledge. But there's a cap: a maximum number of entries, forcing the agent to consolidate. Ten scattered observations about an API become one definitive reference entry. The pressure to consolidate is what turns raw notes into refined knowledge.

## Layer 4: Library (藏经阁)

The library is a shared shelf of skill manuals — markdown playbooks that agents load on demand. In the system prompt, it appears only as an XML routing table: skill names, one-line descriptions, and file paths. Even less than the codex index — just enough to match a task to a skill.

This is cold storage. The catalog costs a few hundred tokens. The actual skill content is loaded only when needed — the agent reads the full SKILL.md into its conversation, follows the instructions, and the content is forgotten on the next molt. Skills are not personal — they're shared across the network. Every agent in the same `.lingtai/` can access the same library. When one agent registers a new skill, others pick it up on their next `library(action='refresh')`.

Skills are the accumulated competence of the network. An agent figures out how to set up a Telegram bot, writes a skill for it, and now every agent in the network can do it. The knowledge doesn't live in any one agent's head — it lives on the shelf.

## Layer 5: Network Topology

The network itself is memory.

Every agent in the topology has its own pad, its own codex, its own mail history. When an orchestrator spawns an avatar to research a topic, that avatar builds deep expertise — entries in its codex, notes in its pad, skills it created. The orchestrator doesn't need to hold all of that. It just needs to know: "I have an avatar called `laps-expert` that knows everything about LAPS collision analysis. When I need that knowledge, I mail it."

This is the coldest storage and the most powerful. The network's collective memory is unbounded — it grows every time an agent molts, every time an avatar is spawned, every time a skill is registered. No single context window can hold it. No single agent needs to.

## Layer 6: The World

Beyond the network is the world — the internet, documentation, APIs, other people's code. Agents have `web_search` and `web_read` to reach outside the network. When they find something useful, they pull it inward: write it to the codex, create a skill, or note it in the pad. The world is infinite cold storage that the agent can warm up on demand.

This is why molt works. An agent doesn't need to remember everything — it needs to know where to find it. The pad has the current task. The codex has the important findings. The library has the procedures. The network has the specialists. And the world has everything else.

## The Gradient

| Layer | Name | In system prompt | Survives molt | Scope | Token cost |
|-------|------|-----------------|---------------|-------|------------|
| 0 | Conversation | Is the conversation | No — gone on molt | Personal | Free (it's what you're doing) |
| 1 | Pad | Full content, every turn | Yes (auto-reload) | Personal | Every token counts |
| 2 | Lingtai | Full content, every turn | Yes (auto-reload) | Personal | Every token counts |
| 3 | Codex | Index only (id + title + summary) | Yes (permanent) | Personal | ~hundreds of tokens |
| 4 | Library | XML catalog only (name + description) | Skills persist, catalog reloads | Shared (network) | ~hundreds of tokens |
| 5 | Network | Not present | Agents persist independently | Collective | Zero |
| 6 | World | Not present | Always available | Universal | Zero |

The gradient is: **ephemeral → hot → warm → cold → distributed → infinite**. Each layer is cheaper to maintain but slower to access. The conversation is the cheapest thing you have — let it go. The system teaches agents to deposit knowledge upward: working state goes in the pad, identity goes in the lingtai, important findings go in the codex, reusable procedures go in the library, deep expertise lives in a specialist agent, and everything else is a web search away.

## Why This Matters

Most AI memory systems try to make the context window bigger. Longer contexts, better compression, smarter retrieval. They're solving a single-body problem with single-body tools.

LingTai solves it the way biology did: forget, specialize, communicate. A single neuron doesn't need to hold all of human knowledge. It needs to fire at the right time, connected to the right neighbors.

Context length is finite. It will always be finite. The answer isn't to make it infinite. The answer is to make forgetting productive — and to build a network where nothing is truly lost, just distributed.

Let it forget. Let the network remember.
