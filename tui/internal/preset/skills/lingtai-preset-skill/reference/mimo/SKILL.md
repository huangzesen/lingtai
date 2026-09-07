---
name: preset-skill-mimo
description: "Use when revising the built-in mimo TUI preset."
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

# mimo preset revision

Use this child for the named built-in mimo preset. mimoPreset in
tui/internal/preset/preset.go:1294 ships mimo-v2.5, XIAOMI_API_KEY, the
OpenAI-compatible Xiaomi endpoint, and vision scoped to that model. Its
regional rows are MiMo, OpenCode Go, and Custom.

## Template-specific settings

Read Xiaomi's official [MiMo developer introduction](https://platform.xiaomimimo.com/llms.txt)
and [OpenAI-compatible API documentation](https://platform.xiaomimimo.com/docs/zh-CN/api/chat/openai-api).
Use the official catalog and, when supported by the selected endpoint, its
authenticated GET /models response for served IDs. OpenCode Go is a gateway:
query its authenticated GET https://opencode.ai/zen/go/v1/models separately.
Do not infer vision from a name such as omni; verify image input on the exact
model and route. Custom is user-supplied and has no provider-wide latest
answer.

The picker carries the latest two generations: mimo-v2.5 and its text-only
mimo-v2.5-pro sibling, plus mimo-v2-pro and mimo-v2-omni. Only mimo-v2.5 has
verified LingTai-side vision; modelHasVision records the other three false.
All four are served by Xiaomi and OpenCode Go, but the wired vision service is
scoped to Xiaomi's own endpoint. No official MiMo vision MCP is established.

## TUI surfaces to revise

Start at mimoPreset in tui/internal/preset/preset.go. Revise
providerModels["mimo"] and the matching modelHasVision entries in
tui/internal/tui/preset_editor.go. Revise ProviderRegionURLs and
ProviderDefaultEnv only for endpoint/credential behavior, and confirm the
constructor's vision provider remains scoped to the intended model and route.
Follow the latest-two-generation rule in tui/internal/tui/SKILL.md and
tui/CONTRACT.md.

When an edited built-in is stamped a numbered key slot, the prefix is the
provider name: MIMO_1_API_KEY, not the ProviderDefaultEnv slot
XIAOMI_API_KEY. Keep this distinction when revising endpoint behavior.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan and diagnostics, use dry-run/check
before apply, and apply only to a new explicit output directory. revision.go
validates hashes, route bindings, and evidence and preserves unowned bytes. This
amendment does not change the current mimo values.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns MiMo route, model, and vision facts.
