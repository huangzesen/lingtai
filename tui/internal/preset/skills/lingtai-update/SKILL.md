---
name: lingtai-update
description: >
  Use when installing, updating, building, or debugging lingtai-tui or
  lingtai-portal — Homebrew, source builds, install-method detection, or
  mainland-China connectivity.
version: 1.0.0
last_changed_at: "2026-09-05T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# LingTai TUI update

TUI-owned operational router for the two Go deliverables this repository ships:
`lingtai-tui` and `lingtai-portal`. Read only the focused reference that matches
the problem; code, tests, `ANATOMY.md`, and the current installer remain the
source of truth.

This skill documents existing paths; it does not add update machinery. Python
runtime and kernel install/update/nudge behavior belongs to the kernel's
`system-manual` runtime/kernel update manual; use that manual for kernel work,
then return here for the TUI or portal binary boundary.

## Nested reference catalog

```yaml
- name: lingtai-update-install
  location: reference/install/SKILL.md
  description: Public installer, release assets, source fallback, and manual builds for both Go binaries.
- name: lingtai-update-command
  location: reference/update-tui/SKILL.md
  description: /update-tui and self-update semantics, confirmation, restart, and ownership boundaries.
- name: lingtai-update-detection
  location: reference/detection/SKILL.md
  description: Native install.json, source/user-local, Homebrew, symlink, and unknown-install detection.
- name: lingtai-update-diagnosis
  location: reference/diagnosis/SKILL.md
  description: Failure triage for updater, installer, binary, portal, and runtime symptoms.
- name: lingtai-update-homebrew
  location: reference/homebrew/SKILL.md
  description: Supported formula use and safe exploration of the Lingtai Homebrew tap and build logic.
- name: lingtai-update-mainland
  location: reference/mainland-china/SKILL.md
  description: Mainland-China routing for Go, npm, GitHub, and Gitee, plus the provider-selected dependency index, without mirror guarantees.
```

## Routing table

| Need | Read |
|---|---|
| Install or build `lingtai-tui` and `lingtai-portal` | `reference/install/SKILL.md` |
| Understand `/update-tui` or `lingtai-tui self-update` | `reference/update-tui/SKILL.md` |
| Explain why the TUI chose source, Homebrew, or unknown | `reference/detection/SKILL.md` |
| Diagnose a failed or partial update | `reference/diagnosis/SKILL.md` |
| Inspect Homebrew formula/tap behavior | `reference/homebrew/SKILL.md` |
| Build or fetch from mainland China, or explain which package index the installer picked | `reference/mainland-china/SKILL.md` |

The concise `/help` entry stays in `lingtai-tui-help`; it routes deep questions
here rather than duplicating these procedures.
