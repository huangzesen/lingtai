---
name: lingtai-tui-anatomy
description: >
  Source-navigation guide for the LingTai Go monorepo that ships
  `lingtai-tui` and `lingtai-portal`. Read this for TUI/Portal structure,
  runtime-boundary evidence, exact Go ownership, or when updating an
  `ANATOMY.md`. The repository-root `ANATOMY.md` is normative; this skill
  routes into that graph without duplicating it. Kernel internals remain
  owned by `lingtai-kernel-anatomy`.
version: 0.2.0
tags: [tui, portal, go, anatomy, source-navigation, reference]
last_changed_at: "2026-09-05T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# LingTai Go Anatomy — navigation guide

## Canonical owner

The repository-root `ANATOMY.md` is both the normative anatomy-of-anatomy and the
top of the distributed code-navigation graph. Per-folder `ANATOMY.md` files are
the content. This bundled skill is the discoverable navigation aid: when it and
the repository root disagree, the repository root governs.

An Anatomy file maps structure, connections, composition, and state beside the
code it describes. It is not a user manual, behavior contract, design history,
or test specification. The paired root `CONTRACT.md` owns interfaces and expected
behavior; the repository-local dev guide owns contribution workflow.

## How to navigate

1. Open the repository-root `ANATOMY.md` and read `Components` plus
   `Composition`.
2. Choose `tui/ANATOMY.md` for terminal UI, subprocess launch, agent inventory,
   lifecycle controls, and `/viz`; choose `portal/ANATOMY.md` for the web server,
   embedded frontend, topology, and replay.
3. Descend to the named package Anatomy and open the cited `file:line` range.
4. Treat code as structural truth. If a citation or ownership claim drifted,
   update the relevant Anatomy in the same change.
5. Use search for enumeration only (every callsite/file); use Anatomy for
   structural ownership and data-flow questions.

## Runtime-boundary route

For questions such as “Does closing the TUI stop my agent?”, “How do I attach
again?”, “Who owns lifecycle?”, or “Should I add launchd?”:

1. Start at root `ANATOMY.md` → `tui/ANATOMY.md`, section
   `Runtime/control-surface boundary`.
2. Follow its same-repo anchors for normal Bubble Tea quit, kernel subprocess
   launch and duplicate-process prevention, startup attachment, explicit
   `/sleep`/`/suspend`/`/cpr`/`/refresh`, inventory, and Portal process release.
3. Use `portal/ANATOMY.md` for Portal's own process and shutdown boundary.
4. Load `lingtai-kernel-anatomy` for exact kernel heartbeat, listener, signal,
   and lifecycle internals. Do not cite sibling-repo files from TUI Anatomy.

Expected conclusion: TUI and Portal are control/presentation processes, not the
kernel agent runtime. Normal UI exit is not an agent lifecycle action. Ordinary
agent persistence therefore does not require `launchd`; an extra supervisor can
instead race the existing duplicate-launch guard.

## Question-to-node map

| Structural question | First Anatomy node |
|---|---|
| TUI startup, quit, attach, process launch, lifecycle commands, `list`, `/projects`, `/viz` | `tui/ANATOMY.md` |
| Bubble Tea screens, palette, `/help`, mail, first-run, preset editor | `tui/internal/tui/ANATOMY.md` |
| Preset templates, utility-skill embedding, recipe materialization | `tui/internal/preset/ANATOMY.md` |
| Running-agent inventory and process discovery | `tui/internal/inventory/ANATOMY.md`, then `tui/internal/processscan/ANATOMY.md` |
| Portal server, browser surface, topology, recording, replay | `portal/ANATOMY.md`, then `portal/internal/api/ANATOMY.md` |
| Filesystem observation on either binary | `tui/internal/fs/ANATOMY.md` or `portal/internal/fs/ANATOMY.md` |
| Python agent runtime, heartbeat, listeners, lifecycle | `lingtai-kernel-anatomy` (separate repository; narrative route only) |

## Citation discipline

The root Anatomy owns the full schema and maintenance contract. Keep these
working rules resident:

- Cite repo-root-relative same-repo paths with verified line ranges, for example
  `tui/internal/tui/app.go:640-670`.
- Never put sibling `lingtai-kernel` file citations in this repo's Anatomy; route
  there narratively.
- Keep parent/child Anatomy links reciprocal where the root convention requires
  them, and do not create empty leaf stubs.
- Update Anatomy only for real structural ownership/state/navigation facts; do
  not paste user guidance or duplicate well-named code.
- After edits, run the repository's architecture smoke tests, citation/graph
  checks, focused package tests, and `git diff --check`; then open every changed
  citation to verify semantics, not only path/range existence.

## Finding local agents

For “which agents are alive?”, “where is that agent?”, or “which channel handles
are advertised?”, start with `lingtai-tui list --detailed`; add `--admin` when
admin fields matter, or use `/projects` for the interactive view. Only then
descend to `tui/ANATOMY.md` and the inventory/processscan nodes for source.

## Cross-repo boundary

This Go repository and the Python kernel are separate source trees. TUI Anatomy
may say that the kernel writes a file the TUI reads, but exact Python paths,
heartbeat loops, channel listeners, signal handling, and agent lifecycle belong
in the kernel's own Anatomy graph. Use `lingtai-kernel-anatomy` rather than
copying or citing that implementation here.

---
> **Found a bug or issue?** If you encounter a problem in this navigation route,
> load `lingtai-issue-report`, assemble source evidence, and obtain the required
> human authorization before filing.
