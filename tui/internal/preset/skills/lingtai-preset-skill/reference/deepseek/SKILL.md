---
name: preset-skill-deepseek
description: "Use when revising the built-in deepseek TUI preset."
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

# deepseek preset revision

Use this child for the named built-in deepseek preset. deepseekPreset in
tui/internal/preset/preset.go:1321 uses provider deepseek, model
deepseek-v4-pro, https://api.deepseek.com, DEEPSEEK_API_KEY, OpenAI
compatibility, web_search, and skills. It has no built-in vision capability.
The editor also offers DeepSeek API, OpenCode Go, and Custom base_url rows.

## Template-specific settings

Read the official [DeepSeek models and pricing](https://api-docs.deepseek.com/quick_start/pricing)
and [models API](https://api-docs.deepseek.com/api/list-models). For the native
route, authenticated GET https://api.deepseek.com/models is the served-list
check. OpenCode Go is a separate gateway: query its authenticated
GET https://opencode.ai/zen/go/v1/models and keep only DeepSeek IDs served
there; do not use the native list as proof for Go. Preserve the route-specific
credential and model spelling.

The picker currently carries deepseek-v4-pro and deepseek-v4-flash, both
modelHasVision false. The experimental deepseek-v4-flash-vision-exp entry is
not part of the stock picker or vision contract. OpenCode Go is scoped to
DeepSeek IDs for this provider;
other Go models belong in Custom. The native API row and Go row select
DEEPSEEK_API_KEY and OPENCODE_GO_API_KEY respectively; an edited native
built-in receives a numbered DEEPSEEK_1_API_KEY-style slot.

## TUI surfaces to revise

Start at deepseekPreset in tui/internal/preset/preset.go. Revise
providerModels["deepseek"] and its modelHasVision entries in
tui/internal/tui/preset_editor.go for picker or vision changes. Revise
ProviderRegionURLs and ProviderDefaultEnv for base_url/env behavior; the
constructor remains text-only unless a reviewed native vision route is wired.
Follow tui/internal/tui/SKILL.md and tui/CONTRACT.md.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment does
not change the current deepseek values; it records the experimental vision
exclusion.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns DeepSeek preset facts.
