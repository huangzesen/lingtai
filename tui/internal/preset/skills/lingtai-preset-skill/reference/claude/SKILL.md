---
name: preset-skill-claude
description: "Use when revising the built-in claude TUI preset."
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

# claude preset revision

Use this child for the named built-in claude preset. claudePreset in
tui/internal/preset/preset.go:1501 uses canonical provider claude-code and
the local Claude Code CLI's OAuth login, with model alias opus and no API-key
env, base_url, web_search override, or LingTai vision capability. The editor
also offers fable, sonnet, and haiku aliases.

## Template-specific settings

Read the official [Claude Code workflows](https://code.claude.com/docs/en/common-workflows)
and [Claude vision guide](https://platform.claude.com/docs/en/build-with-claude/vision).
This preset uses CLI/OAuth aliases, not an Anthropic API model catalog: use the
installed claude CLI's documented model-selection/help surface to see aliases
that the local CLI serves, and verify an alias with its normal print-mode
selection. Do not replace an alias with a dated API ID or inspect credential
contents. CLI alias support and API model availability are distinct facts.
The current Claude Code mapping is fable to claude-fable-5-1; the TUI still
stores the alias, not that full API identifier. Underlying CLI image support
does not establish forwarding through LingTai's CLI adapter.

## TUI surfaces to revise

Start at claudePreset in tui/internal/preset/preset.go. Revise the
claude-code entries in providerModels in tui/internal/tui/preset_editor.go
when CLI aliases change. There is intentionally no modelHasVision entry and
no constructor vision capability; inspect only the model alias picker and
fixed capability rendering if that contract changes. Follow the CLI-alias
exemption in tui/CONTRACT.md.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment does
not change the current claude values.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns Claude CLI alias facts.
