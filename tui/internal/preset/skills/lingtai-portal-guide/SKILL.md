---
name: lingtai-portal-guide
description: Reference router for lingtai-portal internals — startup/opening, .portal/ files, topology/replay APIs, network response shape, and lifecycle/recording. Use when the human asks about visualization, network history, or the portal.
version: 1.1.0
last_changed_at: "2026-09-05T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# Portal Guide

This is a reference router. It describes how the LingTai portal works internally. Do not take action based on this skill alone — use it to choose the focused reference that matches the portal question.

## Quick orientation

`lingtai-portal` is a Go binary that serves a web visualization of the agent network from the same `.lingtai/` directory the TUI reads, on a random local port written to `.lingtai/.portal/port`.

## Nested reference catalog

```yaml
- name: portal-guide-overview
  location: reference/overview/SKILL.md
  description: Portal purpose, how to open it, the `.lingtai/.portal/port` contract, and the `.lingtai/.portal/` directory layout.
- name: portal-guide-topology-and-api
  location: reference/topology-and-api/SKILL.md
  description: `topology.jsonl` frame shape, replay chunk storage, API endpoints, and the live `Network` response schema.
- name: portal-guide-lifecycle-and-recording
  location: reference/lifecycle-and-recording/SKILL.md
  description: Portal lifecycle-state interpretation, heartbeat/staleness rule, 3-second recording loop, and reconstruction behavior.
```

## Routing table

| Need | Read |
|---|---|
| Explain what the portal is, how to open it, or where portal files live | `reference/overview/SKILL.md` |
| Inspect topology tape format, replay chunks, API endpoints, or network JSON | `reference/topology-and-api/SKILL.md` |
| Explain agent lifecycle states in portal output or how recording/rebuild works | `reference/lifecycle-and-recording/SKILL.md` |

## Safety notes

- Treat this skill as architecture reference, not an instruction to mutate portal state.
- If you need to change portal code, use `lingtai-dev-guide` and `lingtai-tui-anatomy` first, then read the cited Go code.
- If a portal URL, field name, or behavior here disagrees with the current code, load `lingtai-issue-report` and surface the stale documentation.
