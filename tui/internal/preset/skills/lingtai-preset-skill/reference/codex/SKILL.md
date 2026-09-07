---
name: preset-skill-codex
description: "Use when revising the built-in codex TUI preset."
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

# codex preset revision

Use this child for the named built-in codex preset. codexPreset in
tui/internal/preset/preset.go:1439 ships provider codex, gpt-5.6-sol,
https://chatgpt.com/backend-api/codex, ChatGPT OAuth with an empty
api_key_env, thinking xhigh, web_search, and provider-native vision. It is
not the standard OpenAI API preset.

## Template-specific settings

Read the official [Codex authentication](https://developers.openai.com/codex/auth)
and [Codex models](https://developers.openai.com/codex/models) pages. For actual
served acceptance, compare that Codex-specific catalog with a fresh
ChatGPT-OAuth request/live selection against the Codex route; never use the
standard OpenAI model list to populate this picker. Keep account rollout and
subscription gates as evidence, and verify image input for each exact model.
Never inspect or print OAuth token contents.

## TUI surfaces to revise

Start at codexPreset in tui/internal/preset/preset.go. Revise
providerModels["codex"], matching modelHasVision entries, codexThinkingOptions,
and the model/capability rows in tui/internal/tui/preset_editor.go when the
Codex catalog, vision, or reasoning choices change. Preserve the /codex
base_url suffix, empty api_key_env, OAuth identity, and explicit xhigh default.
Follow the Codex-specific checklist in tui/internal/tui/SKILL.md and the
latest-two-generation rule in tui/CONTRACT.md.

The current picker is gpt-5.6-sol, gpt-5.6-terra, gpt-5.6-luna, and gpt-5.5;
older gpt-5.4, gpt-5.4-mini, gpt-5.3-codex, and gpt-5.2 are not offered.
Saved presets are not rewritten. To inspect live OAuth quota, complete the
app-server initialize handshake, send account/rateLimits/read with
structurally `null` params, and optionally observe account/rateLimits/updated;
read usedPercent, windowDurationMins, and resetsAt without exposing secrets.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment does
not change the current codex values.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; for live OAuth quota/rate limits, use
`reference/operations/endpoint-capabilities/SKILL.md` and never expose auth
paths or tokens. This child owns Codex preset facts.
