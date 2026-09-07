---
name: preset-skill-op-revision-pipeline
description: Deterministic, manifest-driven revision of an explicitly supplied preset document or bundle.
version: 1.2.0
last_changed_at: "2026-09-07T00:00:00Z"
related_files:
  - tui/internal/preset/revision.go
  - tui/internal/preset/revision_test.go
  - tui/internal/headless/preset_revision.go
  - tui/internal/headless/preset_revision_test.go
  - tui/internal/preset/ANATOMY.md
  - tui/CONTRACT.md
  - tui/internal/headless/ANATOMY.md
  - tui/main.go
  - tui/internal/preset/preset_skill_router_test.go
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# Revision pipeline

This operation is the shared mechanism for a future named-preset update. A
named update composes the relevant provider child (provider-specific facts)
with this child (deterministic revision procedure). PR1 provides the engine
and CLI contract only; it does not revise the current 13 built-in templates,
saved presets, picker entries, or a live/current model catalog.

## Command

Use explicit files and an explicit mode:

```text
lingtai-tui presets revise --manifest PATH --input PATH \
  --mode dry-run|check|apply [--output-dir PATH]
```

`--manifest`, `--input`, and `--mode` are required. There is no default mode.
`--output-dir` is forbidden for `dry-run` and `check`, and required for
`apply`. Apply accepts only an absolute, not-yet-existing output directory.
The command reads no global preset directory and is dispatched before
`preset.Bootstrap`; it makes no provider, OAuth, MCP, network, auth, or
runtime calls.

## Input contract

The manifest is a versioned JSON operation contract, not a copy of the kernel
`init.json` schema. It requires:

- `schema_version: "lingtai.preset.revision/v1"`;
- `policy.retain_generations: 2` and an explicit parser version;
- content-addressed `evidence[]`, each with URL/source type, one of the
  distinct scopes `public_api`, `provider_endpoint`, `native_sdk`,
  `chatgpt_oauth_codex`, or `account_observation`, retrieval/freshness times,
  content and its SHA-256, parser version, status, and claims;
- `targets[]`, each with one canonical name, preserved aliases, exact
  provider ID/case, exact route/API/transport/scope, Responses semantics when
  the route is Responses, and a typed `state` of `revise`, `unsupported`, or
  `no-op`. A revision target's typed route binding is either `direct`, with
  separate exact input pointers and expected values for API, transport, and
  scope, or `provider_child`, with only the exact provider pointer and target
  provider ID; the latter means the composed provider child supplies the route
  facts and does not permit an arbitrary manifest marker. Non-revision targets
  may omit the binding when the schema cannot represent the route, but require
  a deterministic `reason` and declare no models or changes;
- `input_contract.document`, the pinned input SHA-256, the expected post-image
  SHA-256, name/provider JSON pointers, and the exact owned JSON paths. Owned
  and change paths may not duplicate or overlap by ancestry.

Model records carry exact served IDs, family, explicit generation rank,
variant classification, provider ID, evidence references, and separate
tri-state capability facts (`supported`, `unsupported`, `unknown`) for model
support, TUI wiring, and runtime/account observation. Generation rank is never
guessed from a model name or timestamp. The engine retains every variant on
the latest two explicit generations in each family; tied, missing, ambiguous,
or incomplete generation evidence fails closed. A removal is an explicit
`kind: remove` change with an expected old value; nothing is silently deleted.

Evidence is an input boundary, not a fetch instruction. Unknown or absent
capability evidence never becomes `true`. Public API evidence cannot authorize
a ChatGPT OAuth Codex target. Provider IDs, aliases, and case are preserved;
an alias resolves to the canonical plan identity without creating a duplicate
canonical output. Only `revise` targets with a model/model-list change require
two evidenced generations and explicit retirements; free-text,
provider-resolved, single-generation, and deliberately unchanged targets use
an explicit non-revision state. A capability replacement names the exact model
records it describes in `model_refs`; a promotion requires `supported` evidence
for each named model, ignores unrelated records, and cannot reference a model
retired by the same revision.

## Responses preservation

Responses targets declare input modalities and reasoning vocabulary/omission
semantics plus separate requested and observed service-tier vocabularies.
Ordinary `service_tier` paths are request-side; explicitly observed paths use
only the observed vocabulary. Reasoning and service-tier replacements must be
JSON strings.
Generic OpenAI-compatible Responses targets retain the documented reasoning
vocabulary including omission-as-default. Codex and Codex-pool retain their
four current TUI levels and do not receive generic normalization. `wire_api`
and `responses_transport` are eligible only for custom OpenAI-compatible
Responses targets with an HTTP or explicit WebSocket transport. Existing
unknown/unowned fields, including Responses fields outside owned paths, remain
byte-preserved unconditionally by the splice engine.

Every owned change has a JSON pointer, kind, expected old value, evidence
reference, reason, and (for replacement) a valid new value. The engine
validates the old value before producing a plan and splices the original input
bytes, preserving unowned ordering and formatting. Dry-run, check, and apply
all consume the same pure plan.

## Output and failures

Valid output is one stable JSON document with `schema_version`, `mode`, input
and manifest hashes, target state/reason, status, changed, sorted plan, and
diagnostics. Non-revision targets therefore produce an explicit unchanged
result. Human diagnostics/errors are written to stderr. Exit codes are stable:

| Code | Meaning |
|---:|---|
| 0 | valid dry-run, clean check, or completed apply |
| 1 | check found a valid deterministic diff |
| 2 | malformed schema/input/options or hash mismatch |
| 3 | stale/incomplete/conflicting/out-of-scope evidence, undeclared/nonmatching or ambiguous target, or expected-old conflict |
| 4 | filesystem/output failure |

Apply validates the complete plan, stages the complete document/bundle in a
hidden sibling, and attempts one rename into the explicit output path. It
refuses an output path that already exists when inspected and never writes the
input in place or mutates the global preset library. A concurrent creator can
race the inspection and publication window; that race is not synchronized and
may have platform-dependent rename behavior.
