# Lingtai Agora: Share Your Agent Network With the World

*April 9, 2026*

---

What if you could share an entire AI agent network the way you share a git repo?

Not just the code. The agents themselves — their roles, their memory of past conversations, the skills they've learned, the topology they've grown into. A living network, packaged and ready for someone else to bring to life on their machine.

That's what Lingtai Agora does.

## The problem

You've spent weeks building a network of agents that explains a legal dataset. Ten agents, each specialized: one for case structure, one for citations, one for search, one orchestrating the whole thing. They've had hundreds of conversations. They've developed working relationships through email. The orchestrator knows how to delegate.

Now your colleague wants to use it. What do you do?

You can't just copy the folder. It's full of API keys, absolute paths, heartbeat files, process locks, and your personal addon configs. Half of it is ephemeral runtime state that makes no sense on another machine.

You could write documentation. But the value isn't in the documentation — it's in the network itself. The agents, their blueprints, their mail archives, their skill configurations.

## The solution: `/export network`

Type `/export network` in the TUI. Your orchestrator will walk you through a 7-step publishing flow:

1. **Name your project** and copy it to a staging area (`~/lingtai-agora/networks/<name>/`)
2. **Scrub ephemeral state** — API keys, heartbeat files, process locks, logs, portal cache. Everything that's machine-specific gets removed. Everything that's network-specific stays.
3. **Process mail** — you decide how much conversation history to keep. Recent context that helps new users understand the network? Keep it. Years of back-and-forth? Trim to the last month.
4. **Review the project** — walk through every directory, flag large files, decide what to include. Nested git repos, datasets, credentials — you make the call on each one.
5. **Privacy scan** — automated detection of API key patterns, private key blocks, and PII. Hard matches block publishing until resolved.
6. **Write a launch recipe** — this is the new part, and it's important.
7. **Publish to GitHub** — one command, your network is a public (or private) repo.

The whole flow is interactive. The agent walks you through it, asks questions, waits for your judgment on every decision that matters. It's not a script you run blind — it's a conversation about what your network should look like when someone else opens it.

## Launch recipes: the first impression

When someone clones your published network and runs `lingtai-tui`, what happens?

Before Agora, the answer was: a generic greeting, or silence, depending on a boolean setting. Now, it's whatever you designed.

A **launch recipe** is two files:

- **`greet.md`** — the first thing the orchestrator says to the new user. Your elevator pitch, spoken by the agent itself. "Welcome to the OpenClaw Explainer Network! I'm the lead orchestrator of a team of 10 agents that can walk you through the legal dataset. Type `/cpr all` to wake everyone up, then tell me where you'd like to start."

- **`comment.md`** — persistent behavioral instructions injected into the orchestrator's system prompt. The playbook. "Walk users through these topics in order: case structure, citations, search, cross-references. Delegate citation questions to `citation-agent`. Be patient. Don't give legal advice."

The recipe lives at `.lingtai-recipe/` in the project root. When a recipient clones and sets up, the TUI discovers it automatically and pre-selects the Custom recipe pointing to that folder. Zero configuration on the recipient's end.

Placeholders like `{{time}}`, `{{location}}`, and `{{lang}}` are substituted at the recipient's setup time with their own values. You write the template; each user gets a personalized first contact.

## What the recipient experiences

```bash
git clone https://github.com/you/openclaw-network
cd openclaw-network
lingtai-tui
```

The TUI detects the imported network:

> **Imported network detected — 10 agents found (orchestrator: guide)**

The setup wizard walks them through language, API key, capabilities — the same first-run flow as a fresh install, but with agent names pre-filled from your blueprints. At the recipe step, your `.lingtai-recipe/` is pre-selected.

After setup, the orchestrator launches and speaks your `greet.md`. The recipient types `/cpr all` to wake the full team. Ten agents come online, each with the role and knowledge you designed.

They didn't write a single line of configuration. They didn't read your documentation. They talked to your orchestrator, and your orchestrator knew what to do — because you told it to.

## Browse and publish from the TUI

Two new commands in the palette:

- **`/agora`** — browse all your published networks. Same two-panel view as `/projects`, but scanning `~/lingtai-agora/networks/` instead of the registry. See agent counts, states, and network stats at a glance.

- **`/export network`** — kick off the publishing flow. The TUI writes a prompt to your orchestrator asking it to use the `lingtai-export-network` skill. The agent reaches out to you via email and walks you through the process.

## Try it

```bash
brew install huangzesen/lingtai/lingtai-tui
lingtai-tui
```

Build a network. Teach it something. Then type `/export network` and share it with the world.

The network IS the product. Now you can ship it.

---

*Lingtai Agora ships in lingtai-tui v0.4.37. The `/agora` browser, `/export network` and `/export recipe` commands, launch recipes, and the 7-step export skill are all included. No kernel changes required.*
