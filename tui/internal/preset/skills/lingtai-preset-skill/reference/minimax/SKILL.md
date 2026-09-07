---
name: preset-skill-minimax
description: "Use when revising the built-in minimax TUI preset."
version: 3.0.0
last_changed_at: "2026-09-07T00:00:00Z"
related_files:
  - tui/internal/preset/preset.go
  - tui/internal/tui/preset_editor.go
  - tui/internal/tui/SKILL.md
  - tui/CONTRACT.md
  - tui/internal/preset/revision.go
  - tui/internal/headless/preset_revision.go
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# minimax preset revision

Use this child for the named built-in minimax preset. Its current constructor
is minimaxPreset in tui/internal/preset/preset.go:1248: it ships MiniMax-M2.7,
MINIMAX_API_KEY, the CN Anthropic-compatible default, and text/tool
capabilities. The regional rows are ProviderRegionURLs["minimax"]: CN, INTL,
and OpenCode Go.

## Template-specific settings

Read the official [MiniMax Models catalog](https://platform.minimaxi.com/document/Models)
and its [API overview](https://platform.minimaxi.com/docs/api-reference/api-overview).
For served IDs, call the selected official Anthropic route's authenticated
GET /anthropic/v1/models; use the catalog/API docs for modality and plan facts.
OpenCode Go is a gateway, so separately query its authenticated
GET https://opencode.ai/zen/go/v1/models and do not copy a MiniMax catalog into
the Go row. Preserve exact case and distinguish the Go credential from the
regional slots. Check the official Token Plan MCP guide only for its separate
manual image tool; it does not make the TUI vision capability change.
The official MiniMax-Coding-Plan-MCP repository ships the
minimax-coding-plan-mcp package and understand_image tool for a Token Plan
seat or purchased Credits; it is manual-only and not auto-wired here.

## TUI surfaces to revise

Start at minimaxPreset in tui/internal/preset/preset.go. For a model change,
revise providerModels["minimax"] and the matching modelHasVision entries in
tui/internal/tui/preset_editor.go; for endpoint or credential changes, revise
ProviderRegionURLs and ProviderDefaultEnv in preset.go. Confirm the fixed
capability rendering only if the constructor's vision wiring changes. Follow
the curation and retired-model rules in tui/internal/tui/SKILL.md and
tui/CONTRACT.md.

The native CN picker is MiniMax-M2.7, MiniMax-M2.7-highspeed, MiniMax-M2.5,
and MiniMax-M2.5-highspeed, all explicitly text-only for the reviewed
Anthropic-compatible route. INTL remains free text because native parity is
not proven. OpenCode Go keeps its exact pre-PR picker — MiniMax-M3,
MiniMax-M2.7, and MiniMax-M2.7-highspeed — and its prior behavior; native
modality metadata is not copied into that gateway route. Retired IDs remain
valid only when already pinned in a saved preset.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan and diagnostics, run dry-run/check
before apply, and apply only to a new explicit output directory. The adapter
reads only those paths; revision.go validates hashes, route bindings, and
evidence and preserves unowned JSON bytes. This amendment revises native CN
curation while preserving the OpenCode Go route and all endpoint/credential
semantics.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns MiniMax route, model, and vision
facts.
