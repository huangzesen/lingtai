---
name: preset-skill-gemini
description: "Use when revising the built-in gemini TUI preset."
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

# gemini preset revision

Use this child for the named built-in gemini preset. geminiPreset in
tui/internal/preset/preset.go:1332 uses Google's native gemini adapter,
the stable gemini-3.8-flash default, GEMINI_API_KEY, web_search, skills, and a
native vision capability. gemini-3-flash-preview was the previous preview
default. It has no base_url or OpenAI-compatibility override.

Native inline image input is distinct from the explicit LingTai `vision`
capability; both use the shipped Gemini provider rather than a fallback.
The explicit LingTai `vision` capability is a separate tool path from native
multimodality.

## Template-specific settings

Read Google's official [Gemini model guide](https://ai.google.dev/gemini-api/docs/models)
and [image understanding guide](https://ai.google.dev/gemini-api/docs/image-understanding).
For the actual account-served list, call the official Gemini API
models.list endpoint, GET https://generativelanguage.googleapis.com/v1beta/models,
or use the equivalent official SDK method; follow pagination and filter for
models supporting the operation used by this adapter. This is a native
provider catalog, not a gateway alias. Keep preview/stability and image-input
facts separate, and do not infer a LingTai MCP from native multimodality.

## TUI surfaces to revise

Start at geminiPreset in tui/internal/preset/preset.go. Gemini intentionally
has no providerModels picker or modelHasVision entry in
tui/internal/tui/preset_editor.go, so revise only the constructor's model,
credential, and vision capability unless that design changes. If capability
display changes, inspect the fixed mandatoryCapRow rendering; there is no
Gemini base_url surface. Follow tui/CONTRACT.md for free-text providers.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment
revises the constructor default to gemini-3.8-flash; the model remains free
text and is intentionally absent from providerModels and modelHasVision.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns Gemini preset facts.
