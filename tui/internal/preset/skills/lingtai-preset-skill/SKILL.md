---
name: lingtai-preset-skill
description: >
  Dual-axis router for built-in preset questions: which of the 13
  TUI-shipped provider templates, and which cross-cutting lifecycle
  mechanic — saving, checking availability, activating/refreshing,
  endpoint/capability facts, or troubleshooting. Read a child only when
  relevant; this does not describe arbitrary saved presets.
version: 2.1.0
last_changed_at: "2026-09-05T00:00:00Z"
related_files:
  - tui/internal/preset/skills/lingtai-preset-skill/SKILL.md
  - tui/internal/preset/preset.go
  - tui/internal/preset/ANATOMY.md
  - tui/internal/preset/preset_skill_router_test.go
  - tui/internal/preset/skill_metadata_test.go
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

# Built-in preset manuals

This router covers exactly the names returned by `BuiltinPresets()` in
`tui/internal/preset/preset.go`, plus the cross-cutting mechanics that apply
across all of them. It is for TUI-owned template presets only.

Preset questions split along two independent axes:

- **Provider axis** — "which template, and what are its unique facts?" (model,
  endpoint, credential env-var, official links). One child per
  `BuiltinPresets()` name under `reference/<provider>/SKILL.md`.
- **Operation axis** — "what happens when I save / check / activate / refresh
  / troubleshoot a preset, regardless of provider?" One child per mechanic
  under `reference/operations/<operation>/SKILL.md`.

A concrete question usually composes both: read the operation child for the
mechanic, then the provider child for the provider-specific fact (exact
model, endpoint, credential). Provider pages own volatile model, pricing,
endpoint, protocol, and plan facts and route to the relevant operation
child instead of restating shared mechanics; operation pages own the shared
lifecycle mechanics and never encode provider-specific facts.

## Provider catalog (13 direct children)

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

| Preset | Child manual | Stable routing hint |
|---|---|---|
| `minimax` | `reference/minimax/SKILL.md` | MiniMax, Anthropic-shaped endpoint |
| `zhipu` | `reference/zhipu/SKILL.md` | Zhipu/Z.AI GLM Coding Plan |
| `mimo` | `reference/mimo/SKILL.md` | Xiaomi MiMo, OpenAI-compatible |
| `deepseek` | `reference/deepseek/SKILL.md` | DeepSeek, OpenAI-compatible text route |
| `gemini` | `reference/gemini/SKILL.md` | Google Gemini native multimodal route |
| `kimi` | `reference/kimi/SKILL.md` | Kimi Code, OpenAI-compatible |
| `grok` | `reference/grok/SKILL.md` | Grok (xAI) via OpenCode Go, OpenAI-compatible |
| `nvidia` | `reference/nvidia/SKILL.md` | NVIDIA NIM/API Catalog gateway |
| `openrouter` | `reference/openrouter/SKILL.md` | OpenRouter gateway |
| `codex` | `reference/codex/SKILL.md` | ChatGPT OAuth Codex route |
| `codex-pool` | `reference/codex-pool/SKILL.md` | pooled ChatGPT OAuth Codex route |
| `claude` | `reference/claude/SKILL.md` | local Claude Code login |
| `custom` | `reference/custom/SKILL.md` | user-supplied compatible endpoint |

## Operation catalog (5 nested children)

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

| Question shape | Child manual |
|---|---|
| How do saved presets differ from templates? Load/Save/Delete/Bootstrap order and atomicity? | `reference/operations/saved-presets/SKILL.md` |
| What base URL / API compatibility / provider / model / capability declarations does a preset carry, distinct from credentials? Codex OAuth quota inspection. | `reference/operations/endpoint-capabilities/SKILL.md` |
| Does Save make a live provider call, and where does real availability diagnosis live? | `reference/operations/availability-save-gate/SKILL.md` |
| How does a saved preset become the running default, and what does `/refresh` actually switch? | `reference/operations/activation-session-refresh/SKILL.md` |
| Something looks broken/stale — bounded triage before routing to a deeper runtime/update/migration skill | `reference/operations/troubleshooting-migration/SKILL.md` |

## Boundaries and maintenance

The manuals describe direct provider-native or current-preset
OpenAI-compatible vision conservatively. A missing or failed direct route is
not a reason to switch providers, guess credentials, or auto-load/invoke an
MCP. Reviewed official evidence positively identifies optional plan-level
vision MCPs for Zhipu/Z.AI and MiniMax; their child manuals link them as
explicit, manual-only methods. Unknowns remain unknown.

Saved presets are different: a clone under `~/.lingtai-tui/presets/saved/`
may change provider, model, endpoint, credentials, or capabilities. Inspect
its actual manifest instead of treating this catalog as coverage for it.

When `BuiltinPresets()` gains a new template name, the same change must add a
`reference/<name>/SKILL.md` child and both provider-catalog entries. When a
new cross-cutting mechanic is added, add a `reference/operations/<name>/SKILL.md`
child and both operation-catalog entries instead of folding it into a
provider page. The focused router test keeps the source list, embedded
children, parent metadata, and extracted paths in bijection for both axes.
