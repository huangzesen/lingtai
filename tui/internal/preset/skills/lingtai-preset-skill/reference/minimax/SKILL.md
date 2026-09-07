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
is minimaxPreset in tui/internal/preset/preset.go:1248: it ships MiniMax-M3,
MINIMAX_API_KEY, the CN Anthropic-compatible default, and native vision. The
regional rows are ProviderRegionURLs["minimax"]: CN, INTL, and OpenCode Go.

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

The current picker is MiniMax-M3 plus MiniMax-M2.7 and its -highspeed variant;
the corresponding modelHasVision entries are true. M3 accepts image/video
blocks natively on the selected endpoint. Retired M2.5/M2.1/M2 IDs remain
valid only when already pinned in a saved preset.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan and diagnostics, run dry-run/check
before apply, and apply only to a new explicit output directory. The adapter
reads only those paths; revision.go validates hashes, route bindings, and
evidence and preserves unowned JSON bytes. This amendment does not change the
current minimax values.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns MiniMax route, model, and vision
facts.
