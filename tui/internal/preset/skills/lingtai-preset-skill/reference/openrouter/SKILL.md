---
name: preset-skill-openrouter
description: "Use when revising the built-in openrouter TUI preset."
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

# openrouter preset revision

Use this child for the named built-in openrouter preset. openrouterPreset in
tui/internal/preset/preset.go:1418 ships gateway provider openrouter, model
z-ai/glm-5.1, provider-resolved base_url, OPENROUTER_API_KEY, web_search,
and skills; the stock manifest has no vision capability.

The stock template is text-only. A saved preset that explicitly adds
`capabilities.vision` may attempt the selected downstream model's endpoint
and only then may the vision tool attempt that endpoint; gateway-wide
multimodality alone does not change this template.

## Template-specific settings

Read the official [OpenRouter model guide](https://openrouter.ai/docs/guides/overview/models)
and [multimodal guide](https://openrouter.ai/docs/guides/overview/multimodal/image-understanding).
For the live gateway catalog, authenticated GET
https://openrouter.ai/api/v1/models is the lookup; inspect each returned id,
input_modalities, supported parameters, and endpoint details. The website is
useful for browsing but the API response is the served-list evidence. Do not
confuse a downstream provider model or gateway-wide multimodality with the
stock preset's capability wiring.

## TUI surfaces to revise

Start at openrouterPreset in tui/internal/preset/preset.go. OpenRouter
intentionally has no providerModels picker or modelHasVision binding, so the
model remains free text and a catalog update does not automatically edit the
TUI. Recheck the constructor's provider-resolved base_url, credential, and
absent vision key; inspect the fixed capability rendering only if that
constructor contract changes. Follow tui/CONTRACT.md.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment does
not change the current openrouter values.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns OpenRouter gateway and capability
facts.
