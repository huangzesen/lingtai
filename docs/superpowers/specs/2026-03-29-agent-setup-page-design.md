# Enhanced Agent Setup Page

**Date:** 2026-03-29
**Status:** Approved

## Problem

The agent setup page (Step 3/4: "Name your agent") exposes only 7 fields: name, folder, language, stamina, context_limit, soul_delay, molt_pressure. Several important configuration values are hardcoded and invisible to the user:

- **Covenant, principle, soul flow prompts** — auto-resolved by language, user cannot customize or even see the paths
- **Comment** — hardcoded inline string ("You are the 本我 orchestrator...") that shouldn't exist — the agent shouldn't be told it's an orchestrator. Remove entirely.
- **Karma / nirvana** — admin authority baked into the preset, user cannot toggle
- **Tutorial comment** — embedded in binary, not editable on disk for pedagogical customization

The page also lacks visual structure — it's a flat list of fields with minimal context about what each one does.

## Design

### 1. New fields on the setup page

Add 6 new fields (13 total), organized into sections:

```
  ── Identity ──
  Agent name: minimax_cn
  Folder name: minimax_cn
  Language: < en >

  ── Runtime ──
  Stamina: 36000                     (awake time before auto-sleep)
  Context limit: 200000              (max context window)
  Soul delay: 120                    (idle time before soul speaks)
  Molt pressure: 0.8                 (context % to trigger molt)

  ── Authority ──
  Karma: < true >                    (manage other agents)
  Nirvana: < false >                 (permanently destroy agents)

  ── Prompts ──
  Covenant: /.../.lingtai-tui/covenant/en/covenant.md
  Principle: /.../.lingtai-tui/principle/en/principle.md
  Soul flow: /.../.lingtai-tui/soul/en/soul-flow.md
  Comment:                           (custom system prompt)
```

**Field behaviors:**

- **Karma** — cycle selector (true/false), default true. Maps to `manifest.admin.karma`.
- **Nirvana** — cycle selector (true/false), default false. Maps to `manifest.admin.nirvana`. Only meaningful when karma is true.
- **Covenant** — text input, pre-filled with `CovenantPath(globalDir, lang)`. Auto-updates when language toggles, unless user has manually edited the field.
- **Principle** — text input, pre-filled with `PrinciplePath(globalDir, lang)`. Same auto-update behavior.
- **Soul flow** — text input, pre-filled with `SoulFlowPath(globalDir, lang)`. Same auto-update behavior.
- **Comment** — text input, empty by default. Optional. User can point to their own file.

**Language-reactive behavior:** When the user toggles the Language selector, the covenant, principle, and soul flow fields update their pre-filled values to match the new language — but only if the user has not manually typed a custom path. Track a `*Dirty` boolean per field: set to true on any keystroke, reset when language changes and the field still holds the auto-generated path.

### 2. Section headers

Add visual section headers (`── Identity ──`, `── Runtime ──`, `── Authority ──`, `── Prompts ──`) rendered in the accent color. These are non-interactive — the cursor skips over them.

### 3. Improved hint text

Replace terse hints with descriptive ones:

| Field | Old hint | New hint |
|-------|----------|----------|
| Stamina | `(seconds)` | `(awake time before auto-sleep)` |
| Context limit | `(tokens)` | `(max context window)` |
| Soul delay | `(seconds)` | `(idle time before soul speaks)` |
| Molt pressure | `(0-1)` | `(context % to trigger molt)` |
| Karma | — | `(manage other agents)` |
| Nirvana | — | `(permanently destroy agents)` |
| Comment | — | `(custom system prompt)` |

Covenant, principle, and soul flow do not need hints — the file paths are self-explanatory.

### 4. Soul flow prompt files — new embedded assets

Extract the soul flow system prompt from i18n `soul.system_prompt` into standalone files:

```
~/.lingtai-tui/
├── soul/
│   ├── en/soul-flow.md
│   ├── zh/soul-flow.md
│   └── wen/soul-flow.md
```

Content is the current `soul.system_prompt` value from each i18n JSON file, written as plain text (no JSON escaping).

**TUI side:**
- Embed the 3 soul flow files under `tui/internal/preset/soul/{en,zh,wen}/soul-flow.md`
- `Bootstrap()` populates them to `~/.lingtai-tui/soul/` (same pattern as covenant/principle)
- New `SoulFlowPath(globalDir, lang string) string` function returns the path

**init.json:** Add `"soul_file"` field pointing to the soul flow file path. Same pattern as `covenant_file` / `principle_file` — the kernel resolves it via `resolve_file()`.

**Kernel side:**
- `_build_soul_system_prompt(agent)` checks for a custom soul prompt stored on the agent (loaded from `soul_file` at boot). Falls back to `t(lang, "soul.system_prompt")` if not set.
- `_setup_from_init()` already resolves `*_file` fields in a loop — add `"soul"` to the list: `for key in ("covenant", "principle", "memory", "prompt", "comment", "soul")`.
- The resolved soul text is stored on the agent (e.g., `agent._soul_flow_prompt`).

**init_schema.py:** Add `"soul"` to the required text fields validation loop alongside covenant/principle/memory/prompt (inline or `_file`).

**Avatar inheritance:** Avatars inherit `soul_file` from parent's init.json (same as `covenant_file`). Add `"soul_file"` to the avatar path resolution list in `avatar.py`.

### 5. AgentOpts expansion

Add new fields to `AgentOpts`:

```go
type AgentOpts struct {
    Language      string
    Stamina       float64
    ContextLimit  int
    SoulDelay     float64
    MoltPressure  float64
    Karma         bool     // NEW
    Nirvana       bool     // NEW
    CovenantFile  string   // NEW
    PrincipleFile string   // NEW
    SoulFile      string   // NEW
    CommentFile   string   // NEW
}
```

`GenerateInitJSONWithOpts` uses these to build init.json instead of hardcoding paths and admin values.

### 6. Remove hardcoded orchestrator comment

`GenerateInitJSONWithOpts` currently writes a hardcoded inline `"comment"` string: "You are the 本我 (orchestrator) — the primary agent the human interacts with...". This is wrong — the agent shouldn't be told it's an orchestrator. Remove this entirely.

- If user provides a `comment_file` path → use `"comment_file"` in init.json.
- If user leaves comment empty → no comment in init.json.

### 7. Tutorial comment to its own folder

Move `tutorial.md` from `~/.lingtai-tui/tutorial.md` into `~/.lingtai-tui/tutorial/tutorial.md`. Update `TutorialCommentPath()` accordingly. The folder provides a home for future localized tutorials or supplementary materials.

The tutorial is a first-class editable file — users who want to customize the pedagogy can edit it directly.

## Files Changed

| File | Change |
|------|--------|
| `tui/internal/preset/preset.go` | Add `SoulFlowPath()`, expand `AgentOpts`, update `GenerateInitJSONWithOpts` and `GenerateTutorialInit` |
| `tui/internal/preset/soul/{en,zh,wen}/soul-flow.md` | NEW: embedded soul flow prompts |
| `tui/internal/tui/firstrun.go` | Add 6 fields, section headers, improved hints, language-reactive logic, dirty tracking |
| `tui/i18n/en.json` | Add i18n keys for new fields and section headers |
| `tui/i18n/zh.json` | Same |
| `tui/i18n/wen.json` | Same |
| `src/lingtai/init_schema.py` | Add `"soul"` to text field validation (optional) |
| `src/lingtai/agent.py` | Add `"soul"` to `_file` resolution loop, store resolved soul prompt on agent |
| `src/lingtai/capabilities/avatar.py` | Add `"soul_file"` to avatar path resolution list |
| `lingtai-kernel: intrinsics/soul.py` | `_build_soul_system_prompt` checks agent's custom soul prompt first |

## What This Achieves

- User sees and controls all prompt paths during agent creation
- Soul flow prompt is customizable per agent (not locked in i18n)
- Karma/nirvana authority is explicit, not hidden in presets
- Section headers and descriptive hints make the page self-documenting
- Language toggle updates all prompt paths reactively
- Hardcoded orchestrator comment removed — agent is no longer told its role
- Tutorial pedagogy is user-editable via `~/.lingtai-tui/tutorial.md`
