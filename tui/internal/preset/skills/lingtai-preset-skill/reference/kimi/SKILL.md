---
name: preset-skill-kimi
description: "Use when revising the built-in kimi TUI preset."
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

# kimi preset revision

Use this child for the named built-in kimi preset. kimiPreset in
tui/internal/preset/preset.go:1361 ships Kimi Code's kimi-for-coding,
KIMI_CODE_API_KEY, https://api.kimi.com/coding/v1, OpenAI compatibility,
tool use, web_search, and skills; it has no built-in LingTai vision key.
ProviderRegionURLs["kimi"] also exposes OpenCode Go and Custom.

## Template-specific settings

Read the official [Kimi Code docs](https://www.kimi.com/code/docs/en/),
[Kimi Code quickstart](https://platform.kimi.com/docs/guide/kimi-k2-7-code-quickstart),
and [provider configuration](https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/providers.html).
The native route uses the documented CLI/provider alias kimi-for-coding; use
the provider's own current model surface, not a guessed dated API ID. OpenCode
Go is a separate gateway: query its authenticated
GET https://opencode.ai/zen/go/v1/models and preserve its lowercase IDs.
Because Kimi's row is deliberately free text, a gateway catalog does not
become a TUI picker or prove native-route support.
OpenCode Go's documented examples include lowercase kimi-k3 and
kimi-k2.7-code; check its live list rather than treating these examples as a
permanent catalog. The native Kimi Code row remains the subscription alias
kimi-for-coding.

## TUI surfaces to revise

Start at kimiPreset in tui/internal/preset/preset.go. Kimi deliberately has
no providerModels or modelHasVision entry, so keep the model row free text
unless a reviewed UX change says otherwise. Revise ProviderRegionURLs and
ProviderDefaultEnv for Kimi Code/OpenCode Go/Custom endpoint and credential
behavior, and recheck the constructor's absent vision capability. Relevant
picker behavior is openInline's free-text model/base_url path in
tui/internal/tui/preset_editor.go; follow tui/CONTRACT.md.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment does
not change the current kimi values.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns Kimi route and model facts.
