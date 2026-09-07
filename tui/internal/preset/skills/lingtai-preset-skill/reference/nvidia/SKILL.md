---
name: preset-skill-nvidia
description: "Use when revising the built-in nvidia TUI preset."
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

# nvidia preset revision

Use this child for the named built-in nvidia preset. nvidiaPreset in
tui/internal/preset/preset.go:1397 ships the bounded NVIDIA gateway snapshot
nvidia/nemotron-3-ultra-550b-a55b, https://integrate.api.nvidia.com/v1,
NVIDIA_API_KEY, OpenAI compatibility, web_search, and skills; it has no
default vision capability. This is a gateway catalog, not a single NVIDIA
model family.

## Template-specific settings

Read the official [NVIDIA API Catalog](https://build.nvidia.com/), [model catalog](https://build.nvidia.com/models),
and [NIM LLM API reference](https://docs.api.nvidia.com/nim/reference/llm-apis).
Use the catalog's exact model slug and, for the selected account, query the
authenticated OpenAI-compatible GET https://integrate.api.nvidia.com/v1/models
when available. Check the model page for modality and tool support; a VLM in
the gateway catalog does not automatically add this preset's vision key.
Keep catalog slugs distinct from provider names and never copy a volatile
catalog wholesale into the picker.

The curated picker is exactly the reviewed stable snapshot: Nemotron Ultra,
Nemotron Super/Nano Omni, DeepSeek v4 pro/flash gateway slugs, Kimi K3,
MiniMax M3, Mistral Nemotron, GPT-OSS 20B, and the Nemotron Ultra 253B entry.
This is not an all-model universal generation ladder. The kernel's nvidia registration disables
prompt_cache_key because NIM rejects that OpenAI-only field.

## TUI surfaces to revise

Start at nvidiaPreset in tui/internal/preset/preset.go. Revise
providerModels["nvidia"] in tui/internal/tui/preset_editor.go for the exact
route-served snapshot. This target is unsupported by the universal
two-generation JSON revision engine; maintain it as manual code curation. Do
not add modelHasVision entries unless the TUI constructor
also wires a verified vision route; recheck the constructor's absent vision
capability and explicit base_url/env fields. Follow the gateway exemption and
curation rules in tui/internal/tui/SKILL.md and tui/CONTRACT.md.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment is
unsupported for universal JSON generation; the constructor and exact picker
are manual route-served curation with stock no vision.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; this child owns NVIDIA gateway and capability facts.
