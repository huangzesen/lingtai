---
name: lingtai-dev-guide
description: >
  Router for contributing to LingTai. Use when changing code or docs, setting
  up a dev environment, navigating the TUI/portal repo or Python kernel,
  developing MCP addons, troubleshooting a running network, auditing
  security, running a runtime self-check, prepping a PR review, or
  stewarding a new skill. For developers and contributors; for end-user
  lessons, use tutorial-guide.
version: 2.8.0
last_changed_at: "2026-09-05T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# LingTai Developer Guide

The cross-repository developer router for LingTai. The root stays short on
purpose; detailed procedures live under `reference/<topic>/`.

## Repository-local developer guidance takes precedence

Before using this guide, look for developer guidance maintained by the current
repository and read it first. When it exists, follow it for repository-specific
rules; it takes precedence over this global guide. Use this guide afterward — or
when the local guide routes here — to select the deeper cross-repository topic
you need, then read that nested reference before touching code. Do not substitute
generic procedures here for local guidance, or restate local rules from memory.

## Non-negotiable rules

- **Progressive disclosure:** router → nested reference → anatomy skill → code +
  tests. Never jump from memory straight to edits.
- **Code is truth:** reference files route and summarize; cited source files,
  tests, and `ANATOMY.md` files are authoritative.
- **Anatomy travels with code:** if you move/rename/split/delete code cited by an
  `ANATOMY.md`, update the anatomy in the same commit, following the ANATOMY
  frontmatter contract below.
- **Explicit human authorization gates:** do not open/merge PRs, push commits,
  file issues, close/delete resources, or change config unless the human gave an
  imperative authorization for that side effect.
- **Human-facing deliverables prefer HTML:** substantial plans, audits, release
  notes, and PR-readiness reports should be standalone HTML unless waived.
- **Release/install docs rule:** LingTai runtime is normally managed by the
  TUI-created project venv; do not present bare `pip install/upgrade lingtai` as
  the standard user path. Manual pip/venv commands are for developer,
  diagnostic, or verification contexts only.
- **Patch-to-self refresh rule:** a merged PR or rebuilt checkout is not live in
  the agent until the runtime-imported source/package is updated, the agent is
  refreshed, and a live in-situ probe confirms the new behaviour. For kernel
  fixes, identify the actual import path and git HEAD first — do not assume the
  repo you edited is the one this agent imports.
- **Skill-size/progressive-disclosure rule:** when writing or updating skills,
  treat read/context limits as a reason to keep the router lean and link onward,
  not as a reason to paste large content into `SKILL.md`. Put dense material in
  related/nested reference files and link to `skills-manual` for current
  authoring mechanics and limit-aware structure.
- **Launch-session / PYTHONPATH hygiene rule:** never launch the TUI or an agent
  from a shell session that exports `PYTHONPATH`. A leftover `PYTHONPATH` pointing
  at another project's scratch/worktree/temp repo `src` shadows the venv's own
  `lingtai` import, and because the refresh watcher copies `os.environ` verbatim,
  the pollution survives refresh — a plain `refresh` re-inherits it. Debug
  sessions that `export PYTHONPATH=...` for one experiment silently infect every
  agent they launch. See `reference/gotchas/SKILL.md` ("PYTHONPATH pollution")
  and `reference/runtime-self-check/SKILL.md` §1 for the probe and clean-relaunch
  recipe.

## ANATOMY frontmatter contract

New or materially updated `ANATOMY.md` files should start with YAML frontmatter:

```yaml
---
related_files:
  - path/from/repo/root.ext
  - path/to/neighbor/ANATOMY.md
maintenance: |
  Keep related_files as repo-relative paths to real files. Include neighboring
  ANATOMY.md files so the anatomy graph stays connected rather than isolated;
  anatomy links must be bidirectional. If you create a new ANATOMY.md, copy this
  maintenance field. If you notice drift between this anatomy and the code,
  report it. See lingtai-dev-guide for details.
---
```

Rules:

- `related_files` uses repo-relative paths only; every entry must be a real file.
- Include the files the anatomy explains plus neighboring/parent/child
  `ANATOMY.md` files. Do not build a complete graph — choose meaningful links —
  but an isolated `ANATOMY.md` is invalid, and anatomy-to-anatomy links must be
  bidirectional: if A lists B, B should list A.
- `maintenance` carries the recursive instruction: copy it into new anatomy files
  and report drift when anatomy and code no longer match.

## Nested reference catalog

`lingtai-dev-guide` owns these nested references. They are parent-owned
drill-down files, not standalone top-level skills.

```yaml
- name: dev-guide-architecture
  location: reference/architecture/SKILL.md
  description: |
    Project shape, repos, IPC boundaries, and runtime state layout.
- name: dev-guide-setup
  location: reference/setup/SKILL.md
  description: |
    Local dev environment for the Go TUI/portal repo, Python kernel, and MCP addons.
- name: dev-guide-contributing
  location: reference/contributing/SKILL.md
  description: |
    Contribution workflow, daemon decomposition, build/test commands, skill
    changes, anatomy maintenance, and worktree hygiene with exact-object
    approval gates.
- name: dev-guide-gotchas
  location: reference/gotchas/SKILL.md
  description: |
    Known footguns: venv assumptions, migrations, packaging, state files, i18n.
- name: dev-guide-debug-troubleshoot
  location: reference/debug-troubleshoot/SKILL.md
  description: |
    Diagnosing stuck, errored, quiet, or misbehaving LingTai networks.
- name: dev-guide-security-audit
  location: reference/security-audit/SKILL.md
  description: |
    Read-only audits of secrets, permissions, MCP config, channels, and data
    exposure, with severity classification and safe reporting.
- name: dev-guide-runtime-self-check
  location: reference/runtime-self-check/SKILL.md
  description: |
    Probe which lingtai code is actually running after a refresh/checkout/preset/
    MCP change — editable source, git HEAD, active binary and dev-mode symlinks,
    whether long-lived runtime objects really rebuilt — and report it safely.
- name: dev-guide-pr-review-deliverables
  location: reference/pr-review-deliverables/SKILL.md
  description: |
    PR readiness gates, independent review passes, the self-contained local HTML
    explainer, PR body hygiene, and maintainer authorization boundaries.
- name: dev-guide-skill-stewardship
  location: reference/skill-stewardship/SKILL.md
  description: |
    Turning experience into durable skills: when to write one, router-vs-nested
    structure, distillation (归一), de-privatization, a pre-publish benchmark,
    shared-library grooming, and PR-ready cleanup. Cross-links skills-manual.
- name: dev-guide-repo-watch
  location: reference/repo-watch/SKILL.md
  description: |
    Read-only sweep of the Lingtai-AI org for open issues/PRs and recent
    activity, plus non-self monitoring and stateful alerting.
- name: dev-guide-cache-hit-rate
  location: reference/cache-hit-rate/SKILL.md
  description: |
    Recent prompt-cache hit rate from token ledgers over rolling windows
    (1h/5h/1d/3d): ledger fields, the formula, the daemon double-count hazard,
    and the bundled read-only stdlib script.
```

## Routing table

| If you need to... | Read |
|---|---|
| Understand the project shape, repos, IPC, and state layout | `reference/architecture/SKILL.md` |
| Set up a local development environment | `reference/setup/SKILL.md` |
| Make a contribution in TUI, portal, kernel, addons, or skills | `reference/contributing/SKILL.md` |
| Avoid common footguns while coding | `reference/gotchas/SKILL.md` |
| Diagnose a stuck, errored, or misbehaving LingTai network | `reference/debug-troubleshoot/SKILL.md` |
| Audit secrets, permissions, MCP config, channels, or data exposure | `reference/security-audit/SKILL.md` |
| Verify which runtime/binary is actually running after a refresh or rebuild, or why a fix that's on disk still serves stale behaviour | `reference/runtime-self-check/SKILL.md` |
| Get a PR review-ready: review gates, HTML explainer, PR hygiene | `reference/pr-review-deliverables/SKILL.md` |
| Turn experience into a durable, de-privatized, PR-ready skill | `reference/skill-stewardship/SKILL.md` |
| Sweep the GitHub org read-only, or install a non-self PR/issue monitor | `reference/repo-watch/SKILL.md` |
| Measure the recent prompt-cache hit rate (1h/5h/1d/3d) from token ledgers | `reference/cache-hit-rate/SKILL.md` |

## Related skills to load instead or next

| Need | Skill |
|---|---|
| Navigate Go TUI/portal code structurally | `lingtai-tui-anatomy` |
| Navigate Python kernel code structurally | `lingtai-kernel-anatomy` |
| Develop, register, or troubleshoot MCP servers/addons | `mcp-manual` first, then `lingtai-kernel-anatomy` `reference/mcp-protocol.md` |
| Author or publish skills, including limit-aware router/reference structure | `skills-manual`, then `reference/skill-stewardship/SKILL.md` |
| Customize, export, or package project methodology as a recipe | `lingtai-recipe` |
| Work on portal APIs, topology recording, replay, or `.portal/` state | `lingtai-portal-guide` |
| Produce a standalone HTML deliverable (skeleton, MathJax, validation) | `swiss-knife` → `reference/html-report/SKILL.md` |
| Prepare for a consequential molt during long dev work | `psyche-manual` |
| Explain LingTai to an end user lesson-by-lesson | `tutorial-guide` |
| Report a LingTai bug or stale documentation | `lingtai-issue-report` |

## Orientation snapshot

| Repo / package | Stack | Main role | Where to start |
|---|---|---|---|
| `Lingtai-AI/lingtai` | Go + TypeScript | `lingtai-tui`, `lingtai-portal`, bundled utilities | `reference/architecture/SKILL.md`, then `lingtai-tui-anatomy` |
| `Lingtai-AI/lingtai-kernel` | Python | agent runtime, tools, mailbox, soul/molt, intrinsic capabilities | `lingtai-kernel-anatomy` |
| `lingtai-imap`, `lingtai-telegram`, `lingtai-feishu`, `lingtai-wechat`, `lingtai-whatsapp` | Python MCPs | channel/addon integrations | `mcp-manual` plus each addon's README |

## Common routing examples

Multi-hop routes the table above doesn't spell out:

- **"Change a TUI screen"** → `reference/contributing/SKILL.md` →
  `lingtai-tui-anatomy` → relevant Go files → focused `go test`.
- **"Update an ANATOMY.md"** → repo-specific anatomy skill → apply the ANATOMY
  frontmatter contract above, and report stale citations, dead paths, or claims
  that no longer match the code.
- **"Add a capability or inspect runtime behavior"** → `lingtai-kernel-anatomy` →
  kernel anatomy/code → kernel tests.
- **"This broad dev task needs triage"** → the read-only portfolio sweep in
  `reference/contributing/SKILL.md`, then ask for authorization before mutating
  GitHub state.
- **"Local worktrees are piling up"** → "Worktree hygiene" in
  `reference/contributing/SKILL.md`: inventory read-only, propose exact objects,
  remove only after the human or owning maintainer approves them.

Now read the nested reference that matches the task, then verify against current
repo state before acting.
