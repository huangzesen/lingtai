# Recipe-Shipped Skills Design

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Allow recipes to ship skills that are automatically symlinked into `.lingtai/.skills/` on TUI startup, making recipes a full capability profile (greet + comment + skills) rather than just a greeting and behavioral prompt.

**Architecture:** Each recipe directory can contain a `skills/` subdirectory with per-language skill directories. On every TUI startup, the TUI iterates all known recipe directories, resolves the user's language, and creates symlinks from `.lingtai/.skills/<recipe>-<skill>[-<lang>]` to the resolved skill directory. Broken symlinks are pruned automatically. A new bundled `lingtai-recipe` skill self-documents the recipe contract.

**Tech Stack:** Go, `os.Symlink`, existing `preset` and `tui` packages.

---

## 1. Recipe Directory Structure (Extended)

A recipe directory gains an optional `skills/` subdirectory:

```
recipes/<recipe-name>/
  en/
    greet.md
    comment.md
  zh/
    greet.md
    comment.md
  skills/
    <skill-name>/
      en/
        SKILL.md
        scripts/
        assets/
      zh/
        SKILL.md
        scripts/
        assets/
      SKILL.md          # optional root fallback (language-agnostic)
```

- `greet.md` and `comment.md` behavior is unchanged.
- Each skill under `skills/` is self-contained per language directory.
- `SKILL.md` frontmatter contract is unchanged: `name`, `description` required, `version` optional.
- Scripts and assets are per-language (co-located with `SKILL.md`). If a script is language-independent, the skill author either duplicates it or uses only a root-level directory (no lang subdirs).

## 2. i18n Resolution for Recipe Skills

Same fallback pattern as `ResolveGreetPath` / `ResolveCommentPath`:

1. Try `skills/<skill>/<lang>/` — if it contains a `SKILL.md`, use this directory.
2. Try `skills/<skill>/` — if root contains a `SKILL.md`, use this directory.
3. Skip if neither exists.

New function in `preset/recipes.go`:

```go
func ResolveSkillDir(recipeDir, skillName, lang string) string
```

Returns the absolute path to the resolved skill directory, or empty string if not found.

## 3. Symlink Naming Convention

Symlinks are created in `.lingtai/.skills/` with the name:

```
<recipe-dirname>-<skill-name>-<lang>     # when lang-specific dir was resolved
<recipe-dirname>-<skill-name>            # when root fallback was used
```

Examples:
- `adaptive-progressive-discovery-en` symlinks to `~/.lingtai-tui/recipes/adaptive/skills/progressive-discovery/en/`
- `tutorial-step-by-step-zh` symlinks to `~/.lingtai-tui/recipes/tutorial/skills/step-by-step/zh/`
- `openclaw-citation-guide` symlinks to `~/lingtai-agora/projects/openclaw/.lingtai-recipe/skills/citation-guide/` (root fallback, no lang suffix)
- `my-custom-research-en` symlinks to `/path/to/my-custom/skills/research/en/`

The recipe dirname is:
- For bundled recipes: the recipe folder name (`adaptive`, `greeter`, `tutorial`, `plain`)
- For custom recipes: `filepath.Base(customDir)`
- For agora recipes: the agora project folder name

## 4. Symlink Lifecycle

### 4.1 Creation: `LinkRecipeSkills()`

New function in `preset/preset.go`:

```go
func LinkRecipeSkills(lingtaiDir, globalDir, lang, customDir string)
```

Called on every TUI startup, after `PopulateBundledSkills()`.

Algorithm:
1. Collect all recipe directories:
   - All bundled: `globalDir/recipes/*/` (every subdirectory)
   - Custom recipe dir if non-empty (from `.tui-asset/.recipe` state)
   - All agora projects: `$HOME/lingtai-agora/projects/*/` that contain a `.lingtai-recipe/` directory (resolved via `os.UserHomeDir()`)
2. For each recipe dir that has a `skills/` subdirectory:
   - `os.ReadDir(skills/)` to list skill directories
   - For each skill: call `ResolveSkillDir(recipeDir, skillName, lang)`
   - If resolved: compute symlink name, create symlink in `.lingtai/.skills/`
   - If symlink already exists and points to the correct target, skip (idempotent)
   - If symlink exists but points elsewhere (e.g., lang changed), remove and recreate
   - **Collision detection:** If a symlink name is already claimed by a different recipe (e.g., user project named `adaptive` collides with bundled `adaptive`), skip the conflicting symlink and log a warning. Bundled recipes are linked first and take priority. The collision is surfaced as a `skillProblem` in the `/skills` view, advising the user to rename their project or recipe.

### 4.2 Pruning: `PruneStaleSkillSymlinks()`

New function in `preset/preset.go`:

```go
func PruneStaleSkillSymlinks(lingtaiDir string)
```

Called on every TUI startup, after `LinkRecipeSkills()`.

Algorithm:
1. `os.ReadDir(lingtaiDir/.skills/)`
2. For each entry, `os.Lstat()` to check if it's a symlink (`ModeSymlink`)
3. If symlink, `os.Stat()` to check if target exists
4. If target does not exist (broken symlink), `os.Remove()`

Non-symlink entries (bundled skills written by `PopulateBundledSkills`) are never touched.

### 4.3 All recipes linked simultaneously

All recipes' skills are symlinked at the same time. Switching recipes via `/setup` only changes `greet.md` and `comment.md` — skills from all recipes remain available. This is intentional: skills are capabilities, and removing capabilities on recipe swap would be surprising.

## 5. Fix `scanSkills()` for Symlinks

Current code in `tui/skills.go`:

```go
if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
    continue
}
```

`os.ReadDir()` returns `DirEntry` where `IsDir()` returns `false` for symlinks to directories. Fix: use `os.Stat()` (follows symlinks) to check if the entry is a directory:

```go
if strings.HasPrefix(e.Name(), ".") {
    continue
}
info, err := os.Stat(filepath.Join(skillsDir, e.Name()))
if err != nil || !info.IsDir() {
    continue
}
```

## 6. New Bundled Skill: `lingtai-recipe`

A self-documenting skill that teaches agents (and users) the recipe contract.

```
skills/lingtai-recipe/
  en/
    SKILL.md
  zh/
    SKILL.md
```

Contents cover:
- Recipe directory layout (`greet.md`, `comment.md`, `skills/`)
- i18n fallback rules (lang-specific dir -> root dir -> skip)
- Placeholder contract for `greet.md`: `{{time}}`, `{{addr}}`, `{{lang}}`, `{{location}}`, `{{soul_delay}}`
- Skills subdirectory structure and naming convention
- `SKILL.md` frontmatter requirements (`name`, `description`, `version`)
- How to test locally: point `/setup` custom recipe at your directory
- How to export: `/export network` packages the recipe with skills automatically

## 7. Call Sites

### `main.go` and `launcher.go`

After the existing `PopulateBundledSkills(lingtaiDir)` call, add:

```go
preset.LinkRecipeSkills(lingtaiDir, globalDir, lang, customDir)
preset.PruneStaleSkillSymlinks(lingtaiDir)
```

The `lang` and `customDir` values are read from the existing config / recipe state at startup.

Note: `LinkRecipeSkills` is called after `InitProject()` (which creates `.lingtai/`) and after `PopulateBundledSkills()` (which creates `.lingtai/.skills/`), so the target directory is guaranteed to exist. On first run, the wizard hasn't set a recipe yet, but bundled recipe skills are still linked unconditionally. Custom/agora recipe skills are picked up on the next TUI launch after the wizard completes.

### `applyRecipe()` in `recipe_save.go`

No changes needed. Symlinks are managed on startup, not at recipe-apply time. The next TUI launch picks up any new recipe's skills automatically.

## 8. Agora Integration

The agora SKILL.md already documents `.lingtai-recipe/` as the recipe directory for published networks. Recipe skills are automatically included because:

1. The agora publish flow copies the full project (including `.lingtai-recipe/skills/`)
2. The recipient's TUI startup discovers `~/lingtai-agora/projects/<name>/.lingtai-recipe/` and symlinks its skills

No changes to the agora skill or publishing flow are needed. The `lingtai-recipe` skill documents how to author skills within a recipe, which is sufficient guidance for publishers.

## 9. What This Design Does NOT Include

- **Recipe content refactoring** (moving adaptive comment.md content into skills) — separate future work.
- **Tutorial skill extraction** — to be discussed separately.
- **Central skill registry or deduplication** — YAGNI.
- **Kernel changes** — none needed. The kernel sees flat directories in `.lingtai/.skills/`.
- **Migration** — none needed. Symlinks are created fresh on every startup. Existing projects gain recipe skills automatically on next TUI launch.

## 10. Files Changed

| File | Change |
|---|---|
| `tui/internal/preset/preset.go` | Add `LinkRecipeSkills()`, `PruneStaleSkillSymlinks()` |
| `tui/internal/preset/recipes.go` | Add `ResolveSkillDir()` |
| `tui/internal/tui/skills.go` | Fix `scanSkills()` to follow symlinks |
| `tui/main.go` | Call `LinkRecipeSkills()` + `PruneStaleSkillSymlinks()` after `PopulateBundledSkills()` |
| `tui/internal/tui/launcher.go` | Same call site addition |
| `tui/internal/preset/skills/lingtai-recipe/en/SKILL.md` | New bundled skill |
| `tui/internal/preset/skills/lingtai-recipe/zh/SKILL.md` | New bundled skill (Chinese) |
