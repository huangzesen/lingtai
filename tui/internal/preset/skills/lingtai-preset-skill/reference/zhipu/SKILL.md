---
name: preset-skill-zhipu
description: "Use when revising the built-in zhipu TUI preset."
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

# zhipu preset revision

Use this child for the named built-in zhipu preset. zhipuPreset in
tui/internal/preset/preset.go:1275 ships GLM-5.2, ZHIPU_API_KEY, OpenAI
compatibility, CN/INTL coding endpoints, web_search, and skills; it does not
ship a vision capability. ProviderRegionURLs["zhipu"] also has an OpenCode Go
row whose model spelling is lowercase.

## Template-specific settings

Read the official [Zhipu model overview](https://docs.bigmodel.cn/cn/guide/start/model-overview)
and [Z.AI model docs](https://docs.z.ai/) for the applicable region. For the
native CN/INTL route, use that provider catalog and its documented API model
list/endpoint; for OpenCode Go, separately call the gateway's authenticated
GET https://opencode.ai/zen/go/v1/models and preserve the lowercase IDs it
serves. Do not treat GLM-4.6V or the optional [Z.AI Vision MCP](https://docs.z.ai/devpack/mcp/vision-mcp-server)
as the text preset's native vision. Preserve the exact case required by the
selected route.
The optional official server uses package @z_ai/mcp-server and backing model
GLM-4.6V; it reads Z_AI_API_KEY and Z_AI_MODE, while this preset reads
ZHIPU_API_KEY. Vision Understanding consumes a separate 5-hour prompt pool;
never install, register, or invoke that MCP automatically.

## TUI surfaces to revise

Start at zhipuPreset in tui/internal/preset/preset.go. Revise
providerModels["zhipu"] and all matching modelHasVision entries in
tui/internal/tui/preset_editor.go when the picker or text/vision fact changes.
For endpoint or credential changes, revise ProviderRegionURLs and
ProviderDefaultEnv in preset.go. Recheck the constructor's intentionally
absent vision key and the model-row/base_url behavior; follow
tui/internal/tui/SKILL.md and tui/CONTRACT.md.

The picker retains uppercase GLM-5.2/GLM-5.1 and lowercase glm-5.2/glm-5.1
in one mixed list. This is an evidence-backed no-op: the reviewed sources do
not pin a native Coding Plan GLM-5.3 ID or casing strongly enough to split the
native and Go routes safely, so no native GLM-5.3 entry is invented. The
constructor's absent vision key is intentional: the shipped GLM coding models
are text-only.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned JSON bytes. This amendment
records the native-picker no-op and preserves the current constructor, Go
behavior, endpoint, credential, and capability values.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns Zhipu route, model, and vision facts.
