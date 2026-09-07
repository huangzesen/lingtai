---
name: preset-skill-codex-pool
description: "Use when revising the built-in codex-pool TUI preset."
version: 3.0.0
last_changed_at: "2026-09-07T00:00:00Z"
related_files:
  - tui/internal/preset/preset.go
  - tui/internal/tui/preset_editor.go
  - tui/internal/tui/codex_pool_store.go
  - tui/internal/tui/SKILL.md
  - tui/CONTRACT.md
  - tui/internal/preset/revision.go
  - tui/internal/headless/preset_revision.go
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# codex-pool preset revision

Use this child for the named built-in codex-pool preset. codexPoolPreset in
tui/internal/preset/preset.go:1476 mirrors codex: gpt-5.6-sol,
https://chatgpt.com/backend-api/codex, empty api_key_env, thinking xhigh,
web_search, and vision. Pooling changes OAuth account selection, not the
model or endpoint. Keep provider codex-pool distinct from codex.

## Template-specific settings

Use the official [Codex models](https://developers.openai.com/codex/models) and
[authentication](https://developers.openai.com/codex/auth) pages, then verify
served acceptance against the same Codex route for each relevant pooled
account. Do not use the standard OpenAI API catalog. The pool is not a model
catalog. Do not turn one account's observation into a claim about every
account.

## TUI surfaces to revise

Start at codexPoolPreset in tui/internal/preset/preset.go. Keep
providerModels["codex-pool"] exactly aligned with providerModels["codex"],
including gpt-5.6-sol, gpt-6-astra, gpt-5.6-terra, and gpt-5.6-luna in that
default-first order, with matching modelHasVision entries and the Codex four-level
codexThinkingOptions in tui/internal/tui/preset_editor.go. Recheck the
constructor's vision, /codex endpoint, OAuth-only fields, and the pool store
only when pool selection or file behavior is part of the change. Follow
tui/internal/tui/SKILL.md and tui/CONTRACT.md.

## Pool data, selection, and edits

The pool file is `$LINGTAI_TUI_DIR/codex-auth-pool.json`, or
`~/.lingtai-tui/codex-auth-pool.json` when that variable is unset. The pool
stores only refs and integer weights.
The flat shape is `{"version": 1, "accounts": [{"path": "...", "weight": 1}]}`;
the classified shape is `{"version": 2, "models": {"<exact model>": [...]}}`.
Presence of the `models` key is what classifies the pool, not its size or
version. Classified pools refuse flat TUI edits with
`errCodexPoolModelClassified`. Invalid or missing pool data, or an unmatched exact model, falls back to the legacy single-account auth path. Weight 0 means the account is present but disabled. Selection happens at adapter/service construction, is weighted and sticky within one agent wake/session, and does **not** reselect an already-running session.
Configured weights are inputs, not measured shares, and the pool excludes `molt_count`.

Manual edits require Exact authorization; Timestamped backup; Exact-old-value or hash gate; same-directory atomic rename; validation with the live
`load_codex_auth_pool`; and preservation on failure. Preserve the original file on any validation failure. Display labels/email only; never token contents or absolute auth paths (the exact safety rule is: Never print token/auth contents or absolute auth paths). Source anchors:
`tui/internal/tui/codex_pool_store.go:11-330`,
`login.go:171-201,285-299,603-702`, `auth/codex_pool.py:72-323`, and
`_register.py:424-497`.

To inspect live OAuth quota for one pooled account, complete `initialize`, send
`account/rateLimits/read` with structurally `null` params, and optionally watch
`account/rateLimits/updated`. This is per-account and reports `usedPercent`,
`windowDurationMins`, and `resetsAt`; it is not token or credential data.

## Reviewed deterministic revision

Prepare an evidence-bound manifest and explicit input, then run
lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply
[--output-dir PATH]. Review the JSON plan, use dry-run/check before apply, and
apply only to a new explicit output directory. revision.go validates hashes,
route bindings, and evidence and preserves unowned bytes. This amendment does
update the picker and vision metadata while keeping the constructor/default at
gpt-5.6-sol behind the exact-route availability gate; pool data is unchanged.

Maintenance: If the relevant TUI preset/page is revised, check whether this sub-skill also needs revision and, if so, include it in the same PR.

## Operations

For save, endpoint/capability, availability, activation/refresh, or
troubleshooting, use the five shared operation children under
`reference/operations/`; use
`reference/operations/endpoint-capabilities/SKILL.md` for per-account OAuth
quota. This child owns Codex-pool selection and edit facts.
