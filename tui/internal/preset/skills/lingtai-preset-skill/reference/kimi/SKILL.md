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
tui/internal/preset/preset.go:1361 ships Kimi Code's k3,
KIMI_CODE_API_KEY, https://api.kimi.com/coding/v1, OpenAI compatibility,
tool use, web_search, and skills; it has no built-in LingTai vision key.
ProviderRegionURLs["kimi"] also exposes OpenCode Go and Custom.

## Template-specific settings

Read the official [Kimi Code docs](https://www.kimi.com/code/docs/en/),
[Kimi Code quickstart](https://platform.kimi.com/docs/guide/kimi-k2-7-code-quickstart),
and [provider configuration](https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/providers.html).
The native route's reviewed two-generation picker uses the exact IDs k3,
k3-256k, kimi-for-coding, and kimi-for-coding-highspeed. OpenCode Go is a
separate gateway: query its authenticated
GET https://opencode.ai/zen/go/v1/models and preserve its lowercase IDs.
The exact native route lookup makes only the Kimi Code row a picker; the Go
row remains free text, so a gateway catalog cannot become a Go picker or
prove native-route support. Moving from Kimi Code to Go clears a known native
Kimi id; enter an explicit Go id before saving. Arbitrary typed Go values and
Custom remain free text.
OpenCode Go's documented examples include lowercase kimi-k3 and
kimi-k2.7-code; check its live list rather than treating these examples as a
permanent catalog. Do not add Go IDs to the native picker or infer Go vision.

## TUI surfaces to revise

Start at kimiPreset in tui/internal/preset/preset.go. The provider-global
catalog remains absent, while the exact Kimi Code route override supplies the
native picker and k3 default. Keep OpenCode Go and Custom free text, preserve
their endpoint/credential behavior, and recheck the constructor's absent
vision capability. Relevant route lookup and editor behavior live in
tui/internal/tui/preset_editor.go; follow tui/CONTRACT.md.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment
curates the native two-generation picker while preserving the Go free-text
route and Custom behavior.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns Kimi route and model facts.
