---
name: preset-skill-grok
description: "Use when revising the built-in grok TUI preset."
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

# grok preset revision

Use this child for the named built-in grok preset. grokPreset in
tui/internal/preset/preset.go:1375 ships grok-4.5, provider grok, OpenAI
compatibility, https://opencode.ai/zen/go/v1, and OPENCODE_GO_API_KEY. This
is the TUI's only verified Grok route; it has no vision capability. Custom is
available for a user-owned xAI or other gateway, but is not the built-in route.

## Template-specific settings

For the built-in route, read the official [OpenCode Go docs](https://opencode.ai/docs/go/)
and call the authenticated gateway GET https://opencode.ai/zen/go/v1/models.
That served-model list is authoritative for this preset; the [xAI API docs](https://docs.x.ai/)
are only for model-level capability facts and do not establish an xAI route in
the TUI. Preserve lowercase IDs and the shared Go credential. Do not use an
xAI catalog entry as proof that OpenCode Go serves it.
GROK_API_KEY is only the provider-generic fallback; the shipped default keeps
OPENCODE_GO_API_KEY because the default endpoint is the shared Go gateway.

## TUI surfaces to revise

Start at grokPreset in tui/internal/preset/preset.go. Revise
ProviderRegionURLs["grok"] and ProviderDefaultEnv for route/env changes,
providerModels["grok"] for the picker, and modelHasVision["grok-4.5"] for a
reviewed vision fact in tui/internal/tui/preset_editor.go. Recheck the
constructor's absent vision key and the OpenCode Go default row. Follow
tui/internal/tui/SKILL.md and tui/CONTRACT.md.

The picker currently offers grok-4.5 only and modelHasVision records it false:
the Go endpoint's image-input mapping is not pinned. A Custom row does not
turn an xAI endpoint into the shipped route.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment does
not change the current grok values.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns Grok route and capability facts.
