---
name: lingtai-recipe
description: >
  Menu manual (not a tool) for recipes — the named payload that shapes an
  orchestrator's greeting, ongoing behaviour, and shipped library; every
  LingTai project uses one. Routes to `reference/recipe-format/SKILL.md`
  (authoring/customising) and `reference/export-recipe/SKILL.md`
  (publishing for new networks), and warns about the three different
  recipe-shaped artifacts that can co-exist in one project — easy to
  conflate. Do NOT use for one-off exports of a single agent (that's
  just `cp -r`), or for in-network behaviour edits to the live system.
version: 3.2.0
last_changed_at: "2026-09-05T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# lingtai-recipe: Recipes and How to Publish Them

> **Bundle root convention**: The bundle root is the directory that **contains** `.recipe/` at its top level (alongside the library folder). When pointing the TUI or any tool at a recipe, pass **this directory**, not `.recipe/` itself and not a parent of it. For recipes published via `lingtai-recipe` skill, this is `$HOME/lingtai-agora/recipes/<id>/`.

A **recipe bundle** is a required `.recipe/` behavioral layer (`recipe.json` manifest plus optional `greet/`, `comment/`, `covenant/`, `procedures/` layer dirs) beside an optional framework-agnostic skill library named by `recipe.json#library_name`. `reference/recipe-format/SKILL.md` is authoritative for that structure; this router does not restate it.

Every LingTai project uses a recipe — selected during `/setup`, inherited from a cloned network, or auto-discovered when a project already has `.recipe/` at its root.

This skill is the one place to look for anything recipe-related. Pick the sub-file that matches what you're doing, then read it in full before acting.

## Nested reference catalog

`lingtai-recipe` owns these nested references. They are parent-owned drill-down
files, not standalone top-level skills.

```yaml
- name: recipe-format-reference
  location: reference/recipe-format/SKILL.md
  description: |
    Authoritative recipe bundle format reference: directory structure,
    `recipe.json` schema, optional behavioral layers, locale fallback rules,
    library sibling mechanics, validator contract, and custom recipe testing.
- name: recipe-export-flow
  location: reference/export-recipe/SKILL.md
  description: |
    Standalone recipe-export procedure for distilling a live network's culture
    into a portable recipe bundle: scope disambiguation, metadata collection,
    authoring, validation, sensitivity sweep, git initialization, and handoff.
```

## Routing table

| If you need to... | Read |
|---|---|
| Understand or author a recipe bundle: structure, `recipe.json`, optional layers, locale fallback, library sibling, validation, testing | `reference/recipe-format/SKILL.md` |
| Export a standalone recipe: distill culture into optional `greet/comment/covenant/procedures` plus optional skill library; no agents, no mailboxes | `reference/export-recipe/SKILL.md` |

## Disambiguate scope BEFORE picking a sub-guide

A single project directory can hold up to three recipe-shaped artifacts at once, and they are NOT interchangeable. Identify which the human means before going further:

1. **The inner network** — the agents currently living in `.lingtai/` of the project you're invoked in (orchestrator + avatars, their mailboxes, their accumulated state). This is "your network." When the human says "export the recipe of this network," they want you to distil this network's *behaviour* into a fresh recipe — not ship the network itself.

2. **The outer project's own `.recipe/`** — a recipe bundle sitting at the project root (sibling of `.lingtai/`), put there because the project was *seeded* from that recipe at `/setup` time. This is the methodology / culture *that produced* the inner network. It is a separate artifact with its own identity, version, and library. **Do not conflate it with the recipe you author for the export.** If asked to "re-export this recipe," check whether the human means this one (just republish the existing bundle) or wants a fresh recipe distilled from the network's current behavior — ask if ambiguous.

3. **The applied-recipe snapshot at `.lingtai/.tui-asset/.recipe/`** — a copy of #2 captured by the TUI when the recipe was applied. Useful as *evidence* of what behavior is currently in force inside the network, but it is not the artifact to ship. The recipe you ship is freshly distilled from how the inner network *actually behaves now*, not a verbatim copy of what was originally applied.

## Layout of this skill

```
lingtai-recipe/
├── SKILL.md                         ← this menu
├── reference/
│   ├── recipe-format/SKILL.md       ← authoritative recipe format reference
│   └── export-recipe/SKILL.md       ← standalone recipe-export procedure
├── assets/
│   └── gitignore.template           ← canonical .gitignore for exported recipes
└── scripts/
    └── validate_recipe.py           ← invoked by the export flow before git-init
```

Installed at `~/.lingtai-tui/utilities/lingtai-recipe/`. Resolve absolute paths from there when invoking scripts.

## Ground rules for the export flow

- **Never skip the interactive steps.** The flow requires human judgment at specific points (recipe naming, inspecting validator findings). The whole point of a skill-driven export is human-in-the-loop.
- Resolve `$HOME` first, `mkdir -p` before every write, verify afterwards with `find` / `ls`, and talk to the human via `email` rather than text output. `reference/export-recipe/SKILL.md` ("How to talk to the human", "Critical: Filesystem Rules") is canonical for all four — follow it there in full before writing anything.

## Key structural rules that differ from older skills

If you have memory of an older version of this skill, these are the things that changed — old shape → new shape only. `reference/recipe-format/SKILL.md` specifies the new format in full; when in doubt, the validator (`scripts/validate_recipe.py`) is the source of truth.

- **Recipe bundles now have two siblings, not one.** Old: everything under `.lingtai-recipe/` at the repo root. New: `.recipe/` holds only LingTai-facing behavioral layers; libraries live at a sibling folder named by `recipe.json#library_name`.
- **`recipe.json` moved into `.recipe/`.** Old: `<repo-root>/recipe.json`. New: `<bundle-root>/.recipe/recipe.json`, with a grown schema.
- **All four behavioral layers are optional.** Old: `greet.md` and `comment.md` were required. New: every layer is optional. Absent greet → silent agent. Absent comment → no comment file in init.json. Absent covenant / procedures → kernel defaults.
- **Library is a sibling, not inside `.recipe/`.** Old: `.lingtai-recipe/skills/<name>/SKILL.md`. New: `<bundle>/<library_name>/<skill>/SKILL.md`, which makes libraries drop-in-usable by non-LingTai agent frameworks.
- **Library skills are monolingual.** No more `SKILL-en.md` / `SKILL-zh.md` variants. One `SKILL.md` per skill.
- **Layer directories have their own fallback structure.** Old: `.lingtai-recipe/<lang>/greet.md`. New: `.recipe/greet/<lang>/greet.md` (layer-then-lang, with `<layer>.md` at the layer dir root as the default).
- **`recipe.json` is single-canonical, never localized.** Localized display strings belong only in `greet.md` / `comment.md` / `covenant.md` / `procedures.md`.
- **Network exports are gone (v3.2).** Earlier versions of this skill described an `export-network` flow that shipped the live `.lingtai/` snapshot alongside the recipe. That flow has been retired — `/export` now means recipe-only. Recipes are the seed; the garden is grown fresh in each new project.

Now go read the relevant nested reference.

---
> **Found a bug or issue?** If you encounter any problems with this skill, load the `lingtai-issue-report` skill and follow its instructions to report it.
