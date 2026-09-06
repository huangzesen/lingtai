---
name: lingtai-tui-help
description: >
  Anything you need to know about LingTai TUI. Read this first for the stable
  mental model, feature map, interface/slash-command help, or the right
  source/manual route for a TUI question. Exact command prose lives in the
  localized help assets, Go structure lives in `lingtai-tui-anatomy`, and
  presets, Portal, tutorials, updates, and addons keep their own top-level
  skills.
version: 1.1.0
tags: [tui, help, routing, lifecycle, slash-commands, reference, lingtai-tui]
last_changed_at: "2026-09-05T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# LingTai TUI help — the discoverable umbrella

## Mental model — read this first

`lingtai-tui` and `lingtai-portal` are control and presentation surfaces. The
running agent is a separate LingTai kernel process with its own heartbeat,
listeners, lifecycle, and durable working directory.

- Closing the TUI or its terminal quits the interface; it does **not** stop a
  running agent. Reopening the TUI observes the current state and does not
  silently revive an intentionally stopped agent.
- Agent lifecycle changes are explicit. Use `/sleep`, `/suspend`, `/cpr`, or
  `/refresh` according to the intended transition; use `lingtai-tui list
  --detailed` or `/projects` to inspect current state.
- `launchd` is **not** the default remedy for ordinary persistence after TUI
  exit. The agent already runs independently; adding another supervisor can
  create duplicate-process races. Use a separate scheduler/service contract only
  for a separately authorized advanced workflow.
- For source-backed proof, load `lingtai-tui-anatomy`, start at the repository
  root `ANATOMY.md`, and descend to `tui/ANATOMY.md` or `portal/ANATOMY.md`.
  Kernel-side process, heartbeat, listener, and lifecycle internals belong to
  `lingtai-kernel-anatomy`; cross-repo references stay narrative.

Sanitized regression question: **“关闭 TUI/终端后 agent 会不会停止？”** No. UI exit
is not an agent lifecycle command; the agent continues until an explicit
lifecycle action or an external process failure stops it. Do not install a
`launchd` job merely to keep an ordinary agent alive after closing the TUI.

## Feature route catalog

This YAML is the machine-readable map. Each `route` names the first owner to
load; `next` is optional deeper evidence.

```yaml
- feature: runtime-lifecycle-source
  route: lingtai-tui-anatomy
  next: lingtai-kernel-anatomy
- feature: slash-commands-keyboard-interaction
  route: assets/slash-commands.en.md
  localized_routes: [assets/slash-commands.zh.md, assets/slash-commands.wen.md]
- feature: presets-providers-capabilities
  route: lingtai-preset-skill
- feature: portal-visualization-replay
  route: lingtai-portal-guide
- feature: tutorial-orientation
  route: tutorial-guide
- feature: install-update-build
  route: lingtai-update
- feature: contributor-setup-source-troubleshooting
  route: lingtai-dev-guide
- feature: runtime-health-doctor
  route: lingtai-doctor
- feature: projects-and-running-agent-inventory
  route: assets/slash-commands.en.md
  localized_routes: [assets/slash-commands.zh.md, assets/slash-commands.wen.md]
  next: lingtai-tui-anatomy
- feature: addons-and-external-channels
  route: mcp-manual
```

## Human routing table

| Need | Start here | What remains authoritative |
|---|---|---|
| Does closing TUI stop an agent? How do attach, list, quit, or lifecycle controls work? | `lingtai-tui-anatomy` | Repo-root `ANATOMY.md` → `tui/ANATOMY.md` / `portal/ANATOMY.md`; kernel internals → `lingtai-kernel-anatomy` |
| What does a slash command do? Which keys drive the interface? | `assets/slash-commands.en.md` or its ZH/Wen sibling | `DefaultCommands()` and the localized help assets |
| How do presets, providers, model parameters, and capabilities work? | `lingtai-preset-skill` | That skill's provider and operation routes |
| How do Portal topology, APIs, recording, and replay work? | `lingtai-portal-guide` | Portal guide, then Portal Anatomy/source for implementation |
| I am new to LingTai or want the guided course | `tutorial-guide` | Its lesson sequence |
| How do I install, update, build, or diagnose install method? | `lingtai-update` | Its focused update/install references |
| How do I contribute, set up a dev checkout, or troubleshoot source? | `lingtai-dev-guide` | Its focused contributor/debug references |
| Is an agent offline, unreachable, or inconsistent across process/heartbeat/log surfaces? | `lingtai-doctor` | Its layered read-only health diagnostics |
| How do `/projects` and running-agent inventory work? | Localized slash-command asset | `lingtai-tui list --detailed`, `/projects`, then `lingtai-tui-anatomy` for source |
| How do I configure addons or external messaging channels? | `mcp-manual` | The curated addon documentation it routes to |

## Slash-command assets

The complete slash-command reference remains in three language assets so the
in-app `/help` view can render the current UI language directly:

- `assets/slash-commands.en.md` — English (canonical wording).
- `assets/slash-commands.zh.md` — 简体中文.
- `assets/slash-commands.wen.md` — 文言.

Keep these assets in sync with `DefaultCommands()` in
`tui/internal/tui/palette.go`. When a command is added, changed, or removed,
update all three assets together.

## How in-app `/help` resolves the language

The top-level skill is the discoverable umbrella; the in-app `/help` command is
a focused shortcut to the slash-command guide. It calls `i18n.Lang()` (`"en"`,
`"zh"`, or `"wen"`) and opens the matching concrete asset from the list above in the Markdown
viewer. Unknown locales fall back to `assets/slash-commands.en.md`.

## Ownership boundary

Keep general TUI orientation and feature routing here. Do not copy precise Go
architecture from Anatomy, provider contracts from `lingtai-preset-skill`,
Portal detail from `lingtai-portal-guide`, or tutorial/update/addon procedures
from their owners. Route, then load only the source that owns the question.
