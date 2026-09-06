---
name: swiss-knife
description: >
  Umbrella router for small, focused CLI tools: minimax-cli for MiniMax
  media/TTS/vision; vision for image OCR/critique; listen for local audio
  transcription and music analysis; academic-research for papers, citation
  networks, and LaTeX; dj for music generation; token-usage for token/cost
  and API-call trend reports; html-report for standalone browser
  deliverables; xiaomi-mimo and zhipu-coding-plan for provider discovery;
  headless-bot for provisioning fresh LingTai bot projects (e.g. Telegram);
  find-something-to-do for idle curiosity; preset-health for read-only
  saved-preset health checks. Each nested reference is self-contained
  under `reference/<name>/SKILL.md`.
version: 2.4.0
tags: [utilities, umbrella, toolkit, nested-skill]
last_changed_at: "2026-09-05T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# Swiss Knife — Utility Toolkit Router

Swiss Knife is a top-level router for small, focused utility references. Enter
through this file, choose exactly one nested reference from the catalog below,
then read that nested `SKILL.md` for the actual procedure, examples, scripts, and
assets.

## Nested reference catalog

```yaml
nested_references:
  - name: bash-cli-harnesses
    # Virtual cross-skill redirect, not a local reference/<name>/SKILL.md file.
    location: bash-manual reference/bash-*/SKILL.md
    description: >
      Coding-agent CLIs such as Claude Code, OpenAI Codex, OpenCode, Cursor
      Agent, MiMo Code, Qwen Code, Oh-My-Pi, Gemini CLI, Aider, Goose,
      OpenHands, and Crush are now owned by `bash-manual` because they run as
      long-lived shell subprocesses. Read `bash-manual` and then the matching
      nested `reference/bash-*/SKILL.md`; Swiss Knife only keeps non-bash utility
      references.
  - name: minimax-cli
    location: reference/minimax-cli/SKILL.md
    description: >
      Nested swiss-knife reference for the MiniMax `mmx` CLI. Read this for
      MiniMax-backed image, video, music, TTS, or ad-hoc shell vision tasks.
  - name: vision
    location: reference/vision/SKILL.md
    description: >
      Nested swiss-knife reference for image understanding. Read this when you
      need to describe, OCR, or critique an image and aren't sure which path
      applies — it routes between the built-in `vision` tool, the sibling
      `minimax-cli` reference, and a local Hugging Face VLM fallback.
  - name: listen
    location: reference/listen/SKILL.md
    description: >
      Nested swiss-knife reference for local audio analysis. Read this when the
      human asks you to transcribe a voice note, extract lyrics, critique
      generated music, or measure audio characteristics — all local, no API key
      (Whisper transcription and librosa music analysis).
  - name: academic-research
    location: reference/academic-research/SKILL.md
    description: >
      Nested swiss-knife reference for academic literature work. Read this to
      fetch full-text papers by DOI/arXiv-ID/PMID, trace citation networks, run
      scholar analysis, or write/compile LaTeX manuscripts — indexes 12 API
      references and 6 pipeline workflows.
  - name: dj
    location: reference/dj/SKILL.md
    description: >
      Nested swiss-knife reference for composing one music track from a project
      journal entry. Read this when the human asks for music for a journal day,
      project vibe, session mood, or a specific generated genre.
  - name: token-usage
    location: reference/token-usage/SKILL.md
    description: >
      Nested swiss-knife reference for network-wide token, cost, cache, and tool-call/API-call trend reports. Read
      this when the human asks about token usage, budget, model costs, spending, cache behavior, or tools per API call
      across agent ledgers.
  - name: html-report
    location: reference/html-report/SKILL.md
    description: >
      Nested swiss-knife reference for standalone HTML deliverables. Read this
      when the human asks for a polished browser-openable report, dashboard,
      memo, comparison, or any HTML artifact with MathJax equations.
  - name: xiaomi-mimo
    location: reference/xiaomi-mimo/SKILL.md
    description: >
      Nested swiss-knife reference for Xiaomi MiMo provider discovery. Read this
      when the human asks to use or configure Xiaomi MiMo / 小米MiMo, or when you
      need to understand the MiMo model family and compatible endpoints.
  - name: zhipu-coding-plan
    location: reference/zhipu-coding-plan/SKILL.md
    description: >
      Nested swiss-knife reference for the Zhipu / Z.AI / BigModel coding-plan
      subscription. Read this when the human asks about Zhipu credentials or the
      plan's vision, web search, web-read, and zread MCP capabilities.
  - name: headless-bot
    location: reference/headless-bot/SKILL.md
    description: >
      Nested swiss-knife reference for creating and operating headless LingTai
      bot projects from `lingtai-tui spawn`. The current helper provisions a
      Telegram MCP bot, and the workflow also covers preset-policy replication,
      addon wiring, secret hygiene, refresh/relaunch, and verification.
  - name: find-something-to-do
    location: reference/find-something-to-do/SKILL.md
    description: >
      Nested swiss-knife reference for idle curiosity practice. Read this when
      you have no pending task, no human waiting, and want a reflective way to
      notice quiet impulses or choose a small autonomous exploration.
  - name: preset-health
    location: reference/preset-health/SKILL.md
    description: >
      Nested swiss-knife reference for read-only saved-preset health checks.
      Read this when the human asks whether saved presets still work, which one
      is expired or misconfigured, or why `system(action="presets")` shows a bad
      connectivity status. It enumerates saved presets, classifies failures
      (expired key, missing credentials, unreachable endpoint, invalid
      model/config, connectivity failure), and reports actionable fixes without
      printing or mutating any secret.
```

## Routing table

| Need | Read |
|---|---|
| Run or compare coding-agent CLIs such as Claude Code, Codex, OpenCode, Cursor Agent, MiMo Code, Qwen Code, Oh-My-Pi, Gemini CLI, Aider, Goose, OpenHands, or Crush | Load `bash-manual`, then the matching `reference/bash-*/SKILL.md` |
| Generate images, video, music, TTS, or MiniMax shell vision | `reference/minimax-cli/SKILL.md` |
| Describe, OCR, or critique an image (pick the cheapest available path) | `reference/vision/SKILL.md` |
| Transcribe speech/voice notes or analyze music locally (no API key) | `reference/listen/SKILL.md` |
| Fetch papers, trace citations, run scholar analysis, or write LaTeX | `reference/academic-research/SKILL.md` |
| Compose music from a project journal, session mood, or requested genre | `reference/dj/SKILL.md` |
| Report token usage, model costs, cache rates, or tool-call/API-call trends | `reference/token-usage/SKILL.md` |
| Produce standalone HTML reports/dashboards/memos | `reference/html-report/SKILL.md` |
| Discover/configure Xiaomi MiMo provider/model access | `reference/xiaomi-mimo/SKILL.md`; for `mimocode` shell execution, load `bash-manual` → `reference/bash-mimocode/SKILL.md` |
| Discover/configure Zhipu / Z.AI coding-plan capabilities | `reference/zhipu-coding-plan/SKILL.md`; for shell CLI harness execution, load `bash-manual` first |
| Create or automate a headless LingTai bot project, including Telegram bots | `reference/headless-bot/SKILL.md` |
| Practice idle curiosity when there is no pending task or human waiting | `reference/find-something-to-do/SKILL.md` |
| Health-check saved presets (read-only): classify expired keys, missing credentials, unreachable endpoints, invalid model/config | `reference/preset-health/SKILL.md` |

## How to use this router

1. Match the request to one row in the routing table.
2. Read the nested reference at the relative `location` next to this parent
   `SKILL.md`.
3. Follow that nested reference's procedure. Each nested reference owns its own
   scripts and assets under its folder.

Do not load every nested reference by default. Swiss Knife exists to avoid
polluting working context with unrelated utility manuals.

## Adding a nested utility reference

1. Create `reference/<name>/SKILL.md` with normal skill frontmatter (`name`,
   `description`, optional `version`/`tags`). The description should start with
   the fact that it is a nested swiss-knife reference and should state when to
   read it.
2. Put that reference's `scripts/`, `assets/`, and extra files under the same
   `reference/<name>/` folder.
3. Add the child to the `nested_references` YAML block and routing table above.
4. Run validation/tests. At minimum, verify the parent mentions the child and the
   embedded TUI skill tree still copies every nested file.
5. Re-extract the embedded skill bundle to disk after installing/updating the TUI:
   the human runs `lingtai-tui bootstrap`, then agents call `system(action="refresh")`
   or `skills(action="info")` to see the refreshed utility tree.

## Design philosophy

- **Router first** — Swiss Knife itself exposes `swiss-knife/SKILL.md` as its
  top-level catalog entry; its bundled references are reached through that router
  rather than promoted as sibling top-level utility skills or `swiss-knife/<child>`
  skills.
- **Self-contained children** — each nested reference carries its own examples,
  scripts, and assets.
- **Single-purpose children** — if a nested utility grows into a broadly useful
  workflow that agents should discover directly, promote it to a normal top-level
  skill instead of keeping it under Swiss Knife.
- **Small working context** — read one nested reference at a time.

---
> **Found a bug or issue?** If you encounter any problems with this skill, load
> the `lingtai-issue-report` skill and follow its instructions to report it.
