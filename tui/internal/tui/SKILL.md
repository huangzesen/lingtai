# SKILL.md — Preset model lists & provider integration

Bookkeeping notes for keeping `providerModels`, route-specific model lookup,
`modelHasVision`, and friends in `preset_editor.go` aligned with each
provider's real-world catalog. Read this **before** editing those maps so you
don't bork an agent network with a typo or a retired model.

## What this file is for

`providerModels` (preset_editor.go:108) is the canonical native/default-route
catalog. Every model picker, display, and free-text decision calls
`modelOptions(provider, base_url)`: exact route overrides keep a proven native
catalog separate from a protected gateway catalog, while an uncurated route
remains free text. The lookup is the source of truth for editor behavior.

Drift here causes one of two failures:

- **Silent staleness:** the picker doesn't show a model the user knows exists, so they have to free-text edit it. Annoying, recoverable.
- **Loud breakage:** the picker offers a model the provider has retired or doesn't actually serve on our chosen endpoint. Agents pick it from the list, get 4xx/5xx, escalate to STUCK/AED. The user blames Lingtai, not OpenAI.

The second failure mode is what this file exists to prevent.

## Authoritative sources per provider

| Provider | Canonical list | Cadence | Notes |
|---|---|---|---|
| `minimax` | https://platform.minimaxi.com/document/Models | Quarterly | Native CN M-series, current = M2.7; Go keeps its protected pre-PR list |
| `zhipu` | https://docs.bigmodel.cn/cn/guide/models | Quarterly | GLM-5.x family; mixed case list retained until exact native/Go evidence is pinned |
| `mimo` | https://www.xiaomi-ai.com/cn/models | Quarterly | Native V2.5 curation; Go keeps its protected pre-PR list |
| `deepseek` | https://api-docs.deepseek.com/quick_start/pricing | Quarterly | DS-V4 family |
| `grok` | https://opencode.ai/docs/go/ | Monthly | reached only through OpenCode Go; no native xAI route is shipped |
| `codex` | https://developers.openai.com/codex/models | Monthly | ChatGPT-OAuth only — not the standard OpenAI API list |

`kimi` is deliberately absent from provider-global `providerModels`: the exact
Kimi Code route has a native override picker for `k3`, `k3-256k`,
`kimi-for-coding`, and `kimi-for-coding-highspeed`, while OpenCode Go and Custom
remain free text so typed gateway ids survive a `→`. Its route boundaries are
documented in `internal/preset/skills/lingtai-preset-skill/reference/kimi/SKILL.md`.

For codex specifically, **do not** consult `https://platform.openai.com/docs/models`. That's the standard API model list, which includes models the codex backend (`chatgpt.com/backend-api/codex/responses`) doesn't accept (e.g. `gpt-5.5-pro` exists in the standard API but 4xx's on the codex endpoint).

## Curation rules

**Rule 0 — latest two generations only.** `tui/CONTRACT.md` ("Model list curation") caps every family at its latest two generations. Adding a new generation is the same change that removes the third-newest. Variants inside a generation (`-highspeed`, `-pro`, `-mini`, the `gpt-5.6-sol/-terra/-luna` routes, a lowercase respelling for another endpoint) are not generations and all stay. Read that section before touching the maps; the checklist below decides inclusion *within* the two generations the rule allows.

The native/default catalog is not automatically valid for every route. Keep a
route override only when the exact endpoint is evidenced; an unlisted route
must remain free text. The MiniMax and MiMo OpenCode Go overrides are locked
pre-PR compatibility lists. On a route change, reconcile only values already
known to a curated route: keep a value accepted by the destination, otherwise
use its first preserved picker option. Never overwrite arbitrary off-list text.
Kimi Go has no picker, so a known native Kimi id is cleared and must be replaced
through the existing free-text model edit before save. Route-only IDs do not
gain a global `modelHasVision` claim.

For each candidate model, decide inclusion against this checklist:

1. **Is it served on our endpoint?** Codex uses `/backend-api/codex/responses`. If a model is listed in OpenAI's general API docs but not in the Codex docs page above, **exclude it**. Same logic for any other provider where we use a non-standard endpoint.
2. **Is it stable, not preview/research?** Skip "Research Preview" / "Beta" tiers — they get yanked without notice and our list rots. Example: `gpt-5.3-codex-spark` is currently a Research Preview, so it's omitted.
3. **Is its vision capability documented?** Visit the model's page, find the "Modalities" / "Input" section, record `true`/`false` in `modelHasVision`. Don't guess — `gpt-5.3-codex` accepts images, which surprised us.
4. **Will it 401 on a free tier or rollout gate?** `gpt-6-astra` is
   documented, but exact authenticated Codex OAuth availability is not proven
   for every account. Keep it picker-visible with metadata, retain
   `gpt-5.6-sol` as the default-first/native default, and mention the gate
   beside the entry. Do not silently promote it.

## Why some models you might expect are missing

- **`gpt-5.5-pro`** — exists in OpenAI's standard API at `/api/docs/models/gpt-5.5-pro` ($30/$180 per 1M tokens), is available in ChatGPT for Pro/Business/Enterprise, but **is not listed under Codex models**. Adding it would cause 4xx on the codex endpoint. Excluded.
- **`gpt-5.3-codex-spark`** — Research Preview as of 2026-05. Excluded until promoted to GA.
- **`o3-pro` / `o4-mini` / older o-series** — none are in the Codex CLI catalog. Codex serves the documented GPT-6/GPT-5.6 line here.

## When you add a new model

```go
// In providerModels:
"codex": {"gpt-5.6-sol", "gpt-6-astra", "gpt-5.6-terra", "gpt-5.6-luna"}, // default first; Astra availability-gated

// In modelHasVision:
"gpt-5.6-sol":   true, // keep named routes aligned with the verified GPT-5.6 family
"gpt-5.6-terra": true,
"gpt-5.6-luna":  true,
"gpt-6-astra":   true, // docs metadata only; exact OAuth availability is not implied
```

Order matters in `providerModels` only for the picker UX — left-to-right is the cycle order with ←/→. Putting the desired default first keeps fresh templates and the picker aligned. The `templates/codex.json` (built from `preset.go:codexPreset()`) should also have its `llm.model` bumped when you change that default. Existing saved presets keep whatever model they already declared — that's a feature, not a bug.

## When you remove a retired model

1. Remove from `providerModels`.
2. Remove from `modelHasVision`.
3. **Don't** scan saved presets and rewrite their `llm.model`. Users may have very specific reasons for pinning. Migrating their saved/ files silently is worse than letting them hit the 4xx and choose for themselves. (If we ever do migrate, it's an explicit user-confirmed step — not a startup hook.)

## Codex preset specifics

Codex is the odd one out — it uses ChatGPT OAuth instead of an API key. A few things only apply to it:

- **`api_key_env: ""`** in the preset. Don't change to a placeholder env var name; the kernel's `_codex` factory in `lingtai-kernel/src/lingtai/llm/_register.py` ignores `api_key` entirely and reads the OAuth token from the account file named by `manifest.llm.codex_auth_path`, falling back to the legacy `~/.lingtai-tui/codex-auth.json` when that field is absent/empty.
- **Multiple accounts via `codex_auth_path`.** A codex preset may bind to a specific ChatGPT account by setting the non-secret `manifest.llm.codex_auth_path` to a token file (e.g. `~/.lingtai-tui/codex-auth/work.json`). Additional accounts are added from Setup → Credentials (`listCodexAccounts` / `newCodexAuthPath` in `codex_auth_store.go`); the editor's API-key row doubles as an account selector (←/→) for codex. An empty/absent field means the legacy single-account file — existing presets keep working unchanged. Token files are 0600 and never logged.
- **`base_url: "https://chatgpt.com/backend-api/codex"`** — note the `/codex` suffix. Without it, requests hit `/backend-api` (the generic ChatGPT backend) and fail with HTML / Cloudflare responses. Source: `lingtai-kernel/discussions/codex-oauth-stateless-patch.md`.
- **No model picker on `stepPresetKey`.** The codex flow used to render a model strip on the API-key page; that picker was removed in 2026-05 in favor of the standard editor model row. The first-run wizard's stepPresetKey for codex is now pure OAuth-status display. If you find yourself wanting to add a picker there again, you've hit a different bug — fix the editor instead.
- **Two login methods, one completion path.** Codex login first shows a method chooser: browser OAuth/localhost for same-machine use, or device code for remote/headless use. `CodexOAuthDoneMsg` writes the token bundle after either method completes; stale completions are epoch-gated so cancelled attempts cannot overwrite `codex-auth.json`.
- **Empty email is valid.** OpenAI's id_token JWT sometimes ships without the profile claim. We treat `RefreshToken != ""` as the canonical "session is usable" signal and fall back to `(logged in)` for display. Don't gate any logic on `Email != ""`.

## Verification when bumping the codex list

After editing `providerModels["codex"]` / `modelHasVision`:

1. **Build:** `cd tui && go vet ./... && go test ./... && make build`
2. **Manual:** open the preset editor on the codex template, cycle through models with ←/→. Each one should render in the model row, the vision row should toggle correctly with the model.
3. **Live test:** restart an agent on each model in the new list. If you can't run all of them, at least run the new latest and the previous default to confirm the codex endpoint accepts both.
4. **Don't** assume the docs page is canonical for the Codex backend's actual acceptance. The docs sometimes list models still rolling out. If a model 4xx's, it's not in your account yet — leave it in the list (it'll work for users who have it) but note the rollout status in the comment.

## Cross-references

- `preset_editor.go:108` — `providerModels` map
- `preset_editor.go:217` — `modelHasVision` map
- `preset_editor.go:1647` — `mandatoryCapRow` (fixed, informational capabilities rendering)
- `internal/preset/preset.go:codexPreset()` — built-in template, sets default model
- `firstrun.go` `startCodexLogin` — first-run Codex browser/device-code login launcher
- `firstrun.go` / `login.go` `CodexOAuthDoneMsg` handlers — save tokens after matching-epoch browser/device-code completion
- `oauth.go` — browser OAuth, device-code login, token exchange, JWT email parser
- `lingtai-kernel/discussions/codex-oauth-stateless-patch.md` — kernel-side stateless responses contract

When in doubt, search the OpenAI Codex docs and the codex-rs Rust source (https://github.com/openai/codex) for ground truth on what the chatgpt.com endpoint actually accepts.
