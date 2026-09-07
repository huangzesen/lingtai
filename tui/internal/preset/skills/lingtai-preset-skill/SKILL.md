---
name: lingtai-preset-skill
description: "Use when asking about built-in TUI presets or their shared operations."
version: 3.0.0
last_changed_at: "2026-09-07T00:00:00Z"
related_files:
  - tui/internal/preset/preset.go
  - tui/internal/tui/preset_editor.go
  - tui/internal/tui/SKILL.md
  - tui/internal/preset/ANATOMY.md
  - tui/CONTRACT.md
  - tui/internal/preset/revision.go
  - tui/internal/headless/preset_revision.go
  - tui/main.go
  - tui/internal/preset/skills/lingtai-preset-skill/reference/minimax/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/zhipu/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/mimo/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/deepseek/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/gemini/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/kimi/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/grok/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/nvidia/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/openrouter/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/codex/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/codex-pool/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/claude/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/custom/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/operations/saved-presets/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/operations/endpoint-capabilities/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/operations/availability-save-gate/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/operations/activation-session-refresh/SKILL.md
  - tui/internal/preset/skills/lingtai-preset-skill/reference/operations/troubleshooting-migration/SKILL.md
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# Built-in preset router

This router covers exactly the names returned by `BuiltinPresets()` in
`tui/internal/preset/preset.go`. It covers TUI-owned template presets only, not
arbitrary saved presets. Route a named-preset revision to the direct child with
the matching name under `reference/<name>/SKILL.md`. That child owns the
provider's official latest-model lookup, route distinctions, exact TUI surfaces,
vision/capability facts, and named-preset revision instructions.

## Direct children: the 13 BuiltinPresets names

| Name | Direct child | Route hint |
|---|---|---|
| minimax | reference/minimax/SKILL.md | MiniMax regional Anthropic route |
| zhipu | reference/zhipu/SKILL.md | Zhipu/Z.AI coding plan |
| mimo | reference/mimo/SKILL.md | Xiaomi MiMo OpenAI-compatible route |
| deepseek | reference/deepseek/SKILL.md | DeepSeek text route |
| gemini | reference/gemini/SKILL.md | Google native multimodal route |
| kimi | reference/kimi/SKILL.md | Kimi Code and Go gateway |
| grok | reference/grok/SKILL.md | Grok through OpenCode Go |
| nvidia | reference/nvidia/SKILL.md | NVIDIA NIM/API Catalog gateway |
| openrouter | reference/openrouter/SKILL.md | OpenRouter gateway |
| codex | reference/codex/SKILL.md | ChatGPT OAuth Codex route |
| codex-pool | reference/codex-pool/SKILL.md | pooled ChatGPT OAuth route |
| claude | reference/claude/SKILL.md | Claude Code CLI/OAuth aliases |
| custom | reference/custom/SKILL.md | user-supplied compatible endpoint |

```yaml
- name: preset-skill-minimax
  location: reference/minimax/SKILL.md
- name: preset-skill-zhipu
  location: reference/zhipu/SKILL.md
- name: preset-skill-mimo
  location: reference/mimo/SKILL.md
- name: preset-skill-deepseek
  location: reference/deepseek/SKILL.md
- name: preset-skill-gemini
  location: reference/gemini/SKILL.md
- name: preset-skill-kimi
  location: reference/kimi/SKILL.md
- name: preset-skill-grok
  location: reference/grok/SKILL.md
- name: preset-skill-nvidia
  location: reference/nvidia/SKILL.md
- name: preset-skill-openrouter
  location: reference/openrouter/SKILL.md
- name: preset-skill-codex
  location: reference/codex/SKILL.md
- name: preset-skill-codex-pool
  location: reference/codex-pool/SKILL.md
- name: preset-skill-claude
  location: reference/claude/SKILL.md
- name: preset-skill-custom
  location: reference/custom/SKILL.md
```

Do not merge gateway catalogs, CLI/OAuth aliases, and native provider catalogs.
`custom` has no universal latest model: inspect its configured endpoint and
provider evidence. The exact constructor in `preset.go` is always the first
source to inspect; picker, capability, and vision surfaces are listed by each
direct child.

## Shared operation children: five unchanged mechanics

| Question | Child |
|---|---|
| Saved templates/presets, load/save/delete, and bootstrap ordering | reference/operations/saved-presets/SKILL.md |
| Endpoint, API compatibility, provider/model/capability declarations, and Codex OAuth quota | reference/operations/endpoint-capabilities/SKILL.md |
| Whether Save calls a provider and where availability is diagnosed | reference/operations/availability-save-gate/SKILL.md |
| Activation, session state, and what refresh switches | reference/operations/activation-session-refresh/SKILL.md |
| Bounded troubleshooting and migration routing | reference/operations/troubleshooting-migration/SKILL.md |

```yaml
- name: preset-skill-op-saved-presets
  location: reference/operations/saved-presets/SKILL.md
- name: preset-skill-op-endpoint-capabilities
  location: reference/operations/endpoint-capabilities/SKILL.md
- name: preset-skill-op-availability-save-gate
  location: reference/operations/availability-save-gate/SKILL.md
- name: preset-skill-op-activation-session-refresh
  location: reference/operations/activation-session-refresh/SKILL.md
- name: preset-skill-op-troubleshooting-migration
  location: reference/operations/troubleshooting-migration/SKILL.md
```

These five children remain shared production mechanics. A named revision
composes its direct child with the mechanics it needs; it does not create a
second provider tree or a revision operation child.

## Deterministic revision CLI and engine

For a reviewed change, prepare an evidence-bound manifest and explicit input,
then run:

```text
lingtai-tui presets revise --manifest PATH --input PATH \
  --mode dry-run|check|apply [--output-dir PATH]
```

The headless adapter at tui/internal/headless/preset_revision.go reads only
those paths and dispatches before preset bootstrap, provider access, OAuth,
MCP, network, auth, or runtime reads. dry-run and check do not write; apply
requires a new explicit output directory. The pure engine at
tui/internal/preset/revision.go validates the typed target state, route
bindings, evidence, expected old values, and input/post-image hashes, then
splices only owned JSON bytes while preserving unowned bytes. Review the JSON
plan and diagnostics before apply. The amendment documents this production
mechanism and does not change any current built-in model, picker, capability,
or endpoint value.

## Boundaries and maintenance

Provider children record only reviewed facts and official lookup paths; a
failed direct route is not permission to switch providers, guess credentials,
or auto-load an MCP. Saved presets may change provider, model, endpoint,
credentials, or capabilities, so inspect their actual manifest.

When `BuiltinPresets()` gains a new template name, add exactly one matching
direct child and catalog row in the same PR. When a new cross-cutting mechanic is added, extend the five-operation catalog rather than adding a revision
child. When a relevant TUI preset/page is revised, check the matching child
and include its revision in the same PR when needed.
