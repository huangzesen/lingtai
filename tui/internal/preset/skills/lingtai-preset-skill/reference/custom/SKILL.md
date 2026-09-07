---
name: preset-skill-custom
description: "Use when revising the built-in custom TUI preset."
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

# custom preset revision

Use this child for the named built-in custom preset. customPreset in
tui/internal/preset/preset.go:1532 is an OpenAI-compatible user-supplied
template: empty model, LLM_API_KEY, user-supplied base_url, web_search,
skills, and vision inherited from the configured endpoint. It has no universal
latest model and no provider-wide model or vision promise.

## Template-specific settings

There is no universal official catalog for custom. Read the configured
endpoint provider's official model and capability documentation, then query
that endpoint's authenticated GET <base_url>/models when it implements the
OpenAI list operation. If it does not, use its own official catalog or CLI
surface and record the exact served ID, route, protocol, and capability
evidence. The OpenAI, Anthropic, and Gemini API references are protocol
references only; none identifies an unspecified endpoint.

## TUI surfaces to revise

Start at customPreset in tui/internal/preset/preset.go. Custom has no
providerModels or modelHasVision picker binding: model and base_url are free
text. For capability/transport behavior, inspect isCustomOpenAI,
isCustomOpenAIResponses, customResponsesThinkingOptions,
responsesTransportOptions, and mandatoryCapRow in
tui/internal/tui/preset_editor.go. Revise the constructor's inherited vision
declaration only when the configured contract changes; do not turn an
arbitrary relay into a universal native route.

## Responses details

For custom plus api_compat openai, wire_api responses exposes HTTP (omitted
default) or WebSocket (responses_transport websocket), and the reasoning
selector uses default omission or none/minimal/low/medium/high/xhigh. The
editor removes stale transport/thinking fields outside that exact scope; a
WebSocket rejection does not silently fall back to HTTP.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment does
not change the current custom values.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns Custom's endpoint-derived facts, so
inspect the actual saved manifest for user-owned provider, model, endpoint,
credential, and capability facts.
