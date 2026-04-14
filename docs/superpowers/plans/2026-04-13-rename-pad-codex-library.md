# Rename: memory→pad, library→codex, skills→library

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename three core concepts to better reflect what they actually are: memory→pad (working notes), library→codex (personal knowledge archive), skills→library (skill library).

**Architecture:** Surface-level rename across TUI + portal. One TUI migration (m015) renames filesystem paths for existing agents. Portal gets a no-op stub. All i18n keys, Go identifiers, capability names, tool descriptions, skill/procedure/covenant prose, and documentation updated atomically.

**Tech Stack:** Go, JSON (i18n), Markdown (skills/procedures/covenant)

**Key naming decisions:**
- English: pad / codex / library
- 中文: 手记 / 典集 / 藏经阁
- 文言: 简 / 典 / 藏经阁
- Items inside `.library/` are still called "skills" (功法/经书) — it's a library OF skills
- `.skills/` directory → `.library/` on disk
- `system/memory.md` → `system/pad.md`
- `library/library.json` → `codex/codex.json`

**Name collision warning:** The old `library` (knowledge archive) becomes `codex`, and the old `skills` becomes the new `library`. Go fields `a.library` and `a.skills` swap meanings. The old `library_entries.go` becomes codex logic, the old `skills.go` becomes library logic.

---

### Task 1: Migration file (m015) — filesystem renames

**Files:**
- Create: `tui/internal/migrate/m015_rename_pad_codex_library.go`
- Modify: `tui/internal/migrate/migrate.go`
- Modify: `portal/internal/migrate/migrate.go`

This migration renames three filesystem paths inside each agent's working directory within the `.lingtai/` tree. It must handle: missing source (agent doesn't use the feature), existing destination (already migrated or name collision), and symlinks.

- [ ] **Step 1: Create the migration file**

Create `tui/internal/migrate/m015_rename_pad_codex_library.go`:

```go
package migrate

import (
	"fmt"
	"os"
	"path/filepath"
)

// migrateRenamePadCodexLibrary renames agent filesystem paths:
//   - system/memory.md → system/pad.md
//   - library/ → codex/ (including library.json → codex.json)
//   - .skills/ → .library/
//
// Runs on each agent directory found under lingtaiDir.
// Skips dot-prefixed dirs (helpers) and human/ (not an agent).
func migrateRenamePadCodexLibrary(lingtaiDir string) error {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' || e.Name() == "human" {
			continue
		}
		agentDir := filepath.Join(lingtaiDir, e.Name())
		renameAgentPaths(agentDir)
	}

	// Also rename .skills/ → .library/ at the network level (.lingtai/.skills/)
	renameIfExists(
		filepath.Join(lingtaiDir, ".skills"),
		filepath.Join(lingtaiDir, ".library"),
		".skills → .library (network-level)",
	)

	return nil
}

func renameAgentPaths(agentDir string) {
	// 1. system/memory.md → system/pad.md
	renameIfExists(
		filepath.Join(agentDir, "system", "memory.md"),
		filepath.Join(agentDir, "system", "pad.md"),
		"system/memory.md → system/pad.md",
	)

	// 2. library/ → codex/
	oldLibDir := filepath.Join(agentDir, "library")
	newCodexDir := filepath.Join(agentDir, "codex")
	if renameIfExists(oldLibDir, newCodexDir, "library/ → codex/") {
		// Rename library.json → codex.json inside
		renameIfExists(
			filepath.Join(newCodexDir, "library.json"),
			filepath.Join(newCodexDir, "codex.json"),
			"library.json → codex.json",
		)
	}

	// 3. .skills/ → .library/ (agent-level, if it exists)
	renameIfExists(
		filepath.Join(agentDir, ".skills"),
		filepath.Join(agentDir, ".library"),
		".skills → .library (agent-level)",
	)
}

// renameIfExists renames src → dst if src exists and dst does not.
// Returns true if the rename happened.
func renameIfExists(src, dst, label string) bool {
	if _, err := os.Stat(src); err != nil {
		return false // source doesn't exist
	}
	if _, err := os.Lstat(dst); err == nil {
		fmt.Printf("  warning: %s skipped — destination already exists\n", label)
		return false
	}
	if err := os.Rename(src, dst); err != nil {
		fmt.Printf("  warning: %s failed: %v\n", label, err)
		return false
	}
	fmt.Printf("  migrated %s\n", label)
	return true
}
```

- [ ] **Step 2: Register migration in TUI**

In `tui/internal/migrate/migrate.go`:
- Change `const CurrentVersion = 14` → `const CurrentVersion = 15`
- Append to the `migrations` slice:
  ```go
  {Version: 15, Name: "rename-pad-codex-library", Fn: migrateRenamePadCodexLibrary},
  ```

- [ ] **Step 3: Register no-op stub in portal**

In `portal/internal/migrate/migrate.go`:
- Change `const CurrentVersion = 14` → `const CurrentVersion = 15`
- Append to the `migrations` slice:
  ```go
  {Version: 15, Name: "rename-pad-codex-library", Fn: func(_ string) error { return nil }},
  ```

- [ ] **Step 4: Build both binaries to verify compilation**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai/tui && go build ./... && cd ../portal && go build ./...`
Expected: clean compilation, no errors.

- [ ] **Step 5: Commit**

```bash
git add tui/internal/migrate/m015_rename_pad_codex_library.go \
       tui/internal/migrate/migrate.go \
       portal/internal/migrate/migrate.go
git commit -m "feat(migrate): m015 rename memory→pad, library→codex, .skills→.library"
```

---

### Task 2: Go code — rename identifiers and paths (TUI core)

**Files:**
- Rename: `tui/internal/tui/library_entries.go` → `tui/internal/tui/codex_entries.go`
- Rename: `tui/internal/tui/skills.go` → `tui/internal/tui/library.go`
- Rename: `tui/internal/tui/skills_test.go` → `tui/internal/tui/library_test.go`
- Modify: `tui/internal/tui/app.go`
- Modify: `tui/internal/tui/palette.go`
- Modify: `tui/internal/tui/presets.go`

This task renames Go struct fields, type names, view constants, function names, palette entries, and file paths. The name collision requires careful ordering: old `library` → `codex`, old `skills` → `library`.

- [ ] **Step 1: Rename `library_entries.go` → `codex_entries.go` and update identifiers**

```bash
git mv tui/internal/tui/library_entries.go tui/internal/tui/codex_entries.go
```

In `tui/internal/tui/codex_entries.go`, rename all identifiers:
- `libraryFile` → `codexFile` (struct name)
- `libraryEntry` → `codexEntry` (struct name)
- `buildLibraryEntries` → `buildCodexEntries` (function name)
- Comment: `library/library.json` → `codex/codex.json`
- Path literal: `"library", "library.json"` → `"codex", "codex.json"`
- All comments referencing "library" in the knowledge-archive sense → "codex"

- [ ] **Step 2: Rename `skills.go` → `library.go` and update identifiers**

```bash
git mv tui/internal/tui/skills.go tui/internal/tui/library.go
```

In `tui/internal/tui/library.go`:
- `scanSkills` → `scanLibrary` (function name)
- `scanGroup` — keep as is (generic helper)
- `buildSkillEntries` → `buildLibraryEntries` (function name)
- `concatSkillI18n` → `concatSkillI18n` (keep — it's about individual skill files)
- `skillEntry` → keep as `skillEntry` (items inside library are still "skills")
- `skillProblem` → keep as `skillProblem`
- All i18n key references: `"skills.title"` → `"library.title"`, `"skills.problems"` → `"library.problems"`, etc.
- Comments: update "skills" references where they refer to the capability/command/directory

- [ ] **Step 3: Rename test file**

```bash
git mv tui/internal/tui/skills_test.go tui/internal/tui/library_test.go
```

In `tui/internal/tui/library_test.go`:
- Update function calls: `scanSkills` → `scanLibrary`
- Update path references: `.skills` → `.library`

- [ ] **Step 4: Update `app.go` — view constants and struct fields**

In `tui/internal/tui/app.go`:

View constants:
- `appViewSkills` → `appViewLibrary` (old skills view becomes library view)
- `appViewLibrary` → `appViewCodex` (old library view becomes codex view)

Struct fields in `App`:
- `skills MarkdownViewerModel` → `library MarkdownViewerModel`
- `library MarkdownViewerModel` → `codex MarkdownViewerModel`

All references throughout the file:
- `a.skills` → `a.library` (everywhere)
- `a.library` → `a.codex` (everywhere)
- `appViewSkills` → `appViewLibrary` (everywhere)
- `appViewLibrary` → `appViewCodex` (everywhere)

Command cases in `handlePaletteCommand` and `switchToView`:
- `case "skills":` → `case "library":`
  - `filepath.Join(a.projectDir, ".skills")` → `filepath.Join(a.projectDir, ".library")`
  - `filepath.Join(secretary.LingtaiDir(a.globalDir), ".skills")` → `filepath.Join(secretary.LingtaiDir(a.globalDir), ".library")`
  - `scanSkills(...)` → `scanLibrary(...)`
  - `buildSkillEntries(...)` → `buildLibraryEntries(...)`
  - `i18n.T("skills.title")` → `i18n.T("library.title")`
  - `a.skills = ...` → `a.library = ...`
- `case "library":` → `case "codex":`
  - `buildLibraryEntries(...)` → `buildCodexEntries(...)`
  - `i18n.T("palette.library")` → `i18n.T("palette.codex")`
  - `a.library = ...` → `a.codex = ...`

**IMPORTANT:** Because both old names are swapping, do the rename in two passes to avoid confusion:
1. First pass: old `library` → `codex` (all occurrences)
2. Second pass: old `skills` → `library` (all occurrences)

- [ ] **Step 5: Update `palette.go`**

In `tui/internal/tui/palette.go`:
```go
// Old:
{Name: "skills", Description: "palette.skills", Detail: "cmd.skills"},
// ...
{Name: "library", Description: "palette.library", Detail: "cmd.library"},

// New:
{Name: "library", Description: "palette.library", Detail: "cmd.library"},
// ...
{Name: "codex", Description: "palette.codex", Detail: "cmd.codex"},
```

- [ ] **Step 6: Update `presets.go`**

In `tui/internal/tui/presets.go`:
```go
// Old:
var AllCapabilities = []string{
    "file", "email", "bash", "web_search", "psyche", "library",
    "vision", "talk", "draw", "compose", "video", "listen", "web_read",
    "avatar", "daemon", "skills",
}

// New:
var AllCapabilities = []string{
    "file", "email", "bash", "web_search", "psyche", "codex",
    "vision", "talk", "draw", "compose", "video", "listen", "web_read",
    "avatar", "daemon", "library",
}
```

- [ ] **Step 7: Build to verify**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai/tui && go build ./...`
Expected: clean compilation.

- [ ] **Step 8: Commit**

```bash
git add -A tui/internal/tui/
git commit -m "refactor(tui): rename skills→library, library→codex in Go code"
```

---

### Task 3: Go code — preset, recipe_skills, secretary, launcher, main

**Files:**
- Modify: `tui/internal/preset/preset.go`
- Modify: `tui/internal/preset/recipe_skills.go`
- Modify: `tui/internal/preset/recipe_skills_test.go`
- Modify: `tui/internal/tui/secretary_setup.go`
- Modify: `tui/internal/process/launcher.go`
- Modify: `tui/main.go`
- Modify: `tui/internal/migrate/m014_skills_groups.go` (comments only)
- Modify: `tui/internal/migrate/m005_relative_addressing.go` (comments only)
- Modify: `tui/internal/migrate/check_addon_comment.go` (comments only)

- [ ] **Step 1: Update `preset.go`**

All preset functions (minimaxPreset, zhipuPreset, codexPreset, customPreset):
- In capabilities maps: `"library": e()` → `"codex": e()` and `"skills": e()` → `"library": e()`
- In `iconMap`: `"library": "📚"` → `"codex": "📚"` (codex keeps the book icon)
  - Add `"library": "📜"` for the skill library (scroll icon — a 功法 manual)
- Config field: `"memory": ""` → `"pad": ""`
- Function name `PopulateBundledSkills` → `PopulateBundledLibrary`
- All `.skills` path strings → `.library`
- Comments: update "skills" → "library" where referring to the directory/capability
- `BundledSkillNames()` — keep as is (items are still called skills)

- [ ] **Step 2: Update `recipe_skills.go`**

- Rename file: `git mv tui/internal/preset/recipe_skills.go tui/internal/preset/recipe_library.go`
- Function `LinkRecipeSkills` → `LinkRecipeLibrary`
- Function `PruneStaleSkillSymlinks` → `PruneStaleLibrarySymlinks`
- All `.skills` path strings → `.library`
- The `skills/` subdirectory inside recipe dirs stays `skills/` (recipe format unchanged)
- Comments: update capability-level references, keep "skill" when referring to individual items

- [ ] **Step 3: Update `recipe_skills_test.go`**

- Rename: `git mv tui/internal/preset/recipe_skills_test.go tui/internal/preset/recipe_library_test.go`
- Update function calls: `LinkRecipeSkills` → `LinkRecipeLibrary`, `PruneStaleSkillSymlinks` → `PruneStaleLibrarySymlinks`
- All `.skills` path references → `.library`

- [ ] **Step 4: Update `secretary_setup.go`**

```go
// Old:
"library": map[string]interface{}{"library_limit": 100},
"skills": map[string]interface{}{},

// New:
"codex": map[string]interface{}{"codex_limit": 100},
"library": map[string]interface{}{},
```

Also:
- `secretaryCaps["library"]` → `secretaryCaps["codex"]`
- `lib["library_limit"]` → `lib["codex_limit"]`
- `.skills` path → `.library` in the symlink section
- Comments: update capability references

- [ ] **Step 5: Update `main.go`**

- `preset.PopulateBundledSkills(...)` → `preset.PopulateBundledLibrary(...)`
- `preset.LinkRecipeSkills(...)` → `preset.LinkRecipeLibrary(...)`
- `preset.PruneStaleSkillSymlinks(...)` → `preset.PruneStaleLibrarySymlinks(...)`
- Comments: `.skills/intrinsic` → `.library/intrinsic`, "skills" → "library" for capability references
- Help text line 591: `"memory, mail, identity"` → `"pad, mail, identity"`
- Comment line 388: `.skills/` → `.library/`

- [ ] **Step 6: Update `launcher.go`**

Comment on line 51: `.skills/intrinsic` → `.library/intrinsic`

- [ ] **Step 7: Update old migration file comments (m005, m014, check_addon_comment)**

These are already-run migrations. Only update **comments** to reflect new naming so future readers aren't confused. Do NOT change any logic or string literals that the migration actually uses — those paths were correct at the time the migration ran.

- [ ] **Step 8: Build and run tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai/tui && go build ./... && go test ./...`
Expected: clean compilation and passing tests.

- [ ] **Step 9: Commit**

```bash
git add -A tui/
git commit -m "refactor(tui): rename preset/recipe/secretary/main skill→library references"
```

---

### Task 4: i18n — rename keys and values across en.json, zh.json, wen.json

**Files:**
- Modify: `tui/i18n/en.json`
- Modify: `tui/i18n/zh.json`
- Modify: `tui/i18n/wen.json`

This task requires careful key swapping. The old `palette.library` key (knowledge archive) becomes `palette.codex`, and the old `palette.skills` key (skill browser) becomes `palette.library`.

- [ ] **Step 1: Rename keys and values in `en.json`**

Key renames (old key → new key, with updated values):

```
"palette.skills" → "palette.library"
  Old value: "View installed skills"
  New value: "Browse skill library"

"palette.library" → "palette.codex"
  Old value: "Browse agent knowledge libraries"
  New value: "Browse agent codex"

"skills.title" → "library.title"
  Old value: "Skills"
  New value: "Library"

"skills.intrinsic" → "library.intrinsic"
  Old value: "Intrinsic"
  New value: "Intrinsic" (unchanged)

"skills.created" → "library.created"
  Old value: "Created"
  New value: "Created" (unchanged)

"skills.installed" → "library.installed"
  Old value: "Installed"
  New value: "Installed" (unchanged)

"skills.none" → "library.none"
  Old value: "No skills found"
  New value: "No skills found" (unchanged — items are still skills)

"skills.problems" → "library.problems"
  Old value: "Problems"
  New value: "Problems" (unchanged)

"skills.select_hint" → "library.select_hint"
  Old value: "No skills installed"
  New value: "No skills installed" (unchanged — items are still skills)

"cmd.skills" → "cmd.library"
  Old value: "View installed skills — reusable procedures the agent can invoke on demand"
  New value: "Browse the skill library — reusable procedures the agent can invoke on demand"

"cmd.library" → "cmd.codex"
  Old value: "Browse knowledge libraries across all agents — each agent's accumulated research and findings"
  New value: "Browse the codex across all agents — each agent's accumulated research and findings"

"firstrun.cap_desc.library" → "firstrun.cap_desc.codex"
  Old value: "Long-term knowledge archive.\nSubmit, filter, consolidate, and export notes. Survives molts — the agent's cumulative memory."
  New value: "Long-term knowledge codex.\nSubmit, filter, consolidate, and export notes. Survives molts — the agent's cumulative codex."

"firstrun.cap_desc.skills" → "firstrun.cap_desc.library"
  Old value: "Use installed skill packs.\nSkills are markdown playbooks the agent can load on demand, similar to Claude Code skills."
  New value: "Browse and use the skill library.\nSkills are markdown playbooks the agent can load on demand, similar to Claude Code skills."

"firstrun.cap_desc.psyche"
  Old value: "Evolving identity, memory, and context management.\n..."
  New value: "Evolving identity, pad, and context management.\n..." 

"cmd.clear"
  Old value: "...Identity, memory, and library are preserved"
  New value: "...Identity, pad, and codex are preserved"
```

- [ ] **Step 2: Rename keys and values in `zh.json`**

Same key renames as en.json. Updated values using the Chinese names:

```
"palette.skills" → "palette.library": "浏览技能藏经阁"
"palette.library" → "palette.codex": "浏览器灵典集"
"skills.title" → "library.title": "藏经阁"
"skills.intrinsic" → "library.intrinsic": "内置" (unchanged)
"skills.created" → "library.created": "自创" (unchanged)
"skills.installed" → "library.installed": "已安装" (unchanged)
"skills.none" → "library.none": "未找到技能" (unchanged)
"skills.problems" → "library.problems": "问题" (unchanged)
"skills.select_hint" → "library.select_hint": "未安装技能" (unchanged)
"cmd.skills" → "cmd.library": "浏览技能藏经阁——器灵可按需调用的可复用流程"
"cmd.library" → "cmd.codex": "浏览所有器灵的典集——各器灵积累的调研与发现"
"firstrun.cap_desc.library" → "firstrun.cap_desc.codex": "典集 — 长期知识库。\n提交、筛选、整理、导出笔记，凝蜕后依然保留 — 这是器灵的典集。"
"firstrun.cap_desc.skills" → "firstrun.cap_desc.library": "技能藏经阁。\n技能是可按需加载的 Markdown 剧本，与 Claude Code 的 skills 一脉相承。"
"firstrun.cap_desc.psyche": "身份、手记与上下文管理。\n..."
"cmd.clear": "清空器灵的整个上下文窗口并以空白对话重启。身份、手记和典集保留不变"
```

- [ ] **Step 3: Rename keys and values in `wen.json`**

Same key renames. Updated values using the 文言 names:

```
"palette.skills" → "palette.library": "览技艺之藏经阁"
"palette.library" → "palette.codex": "览诸灵之典"
"skills.title" → "library.title": "藏经阁"
"skills.intrinsic" → "library.intrinsic": "固有" (unchanged)
"skills.created" → "library.created": "自创" (unchanged)
"skills.installed" → "library.installed": "已安" (unchanged)
"skills.none" → "library.none": "未寻得技艺" (unchanged)
"skills.problems" → "library.problems": "异" (unchanged)
"skills.select_hint" → "library.select_hint": "未安技艺" (unchanged)
"cmd.skills" → "cmd.library": "览技艺之藏经阁——器灵可按需施行之可复用流程"
"cmd.library" → "cmd.codex": "览诸灵之典——各灵所积之知与所得之悟"
"firstrun.cap_desc.library" → "firstrun.cap_desc.codex": "典 — 长久之识库。\n投经、筛卷、整纂、导出；凝蜕不失，乃器灵之典。"
"firstrun.cap_desc.skills" → "firstrun.cap_desc.library": "技艺之藏经阁。\n技艺者，按需而载之 markdown 剧本，与 Claude Code 之 skills 同源。"
"firstrun.cap_desc.psyche": "心印、简与上下文之管。\n..."
"cmd.clear": "清器灵全部上下文，以空白对话重启之。灵台、简与典皆存"
```

- [ ] **Step 4: Verify JSON is valid**

Run: `python3 -c "import json; [json.load(open(f'tui/i18n/{l}.json')) for l in ('en','zh','wen')]; print('OK')"`
Expected: `OK`

- [ ] **Step 5: Commit**

```bash
git add tui/i18n/en.json tui/i18n/zh.json tui/i18n/wen.json
git commit -m "feat(i18n): rename memory→pad, library→codex, skills→library across all locales"
```

---

### Task 5: Tool descriptions — `docs/tool-descriptions.md`

**Files:**
- Modify: `docs/tool-descriptions.md`

This file contains the English, Chinese, and 文言 descriptions for each tool/capability that agents read. Every tool call pattern and capability name must be updated. This is a **prose-aware** task — the subagent must read the full file and make contextually appropriate changes.

- [ ] **Step 1: Update `eigen` intrinsic descriptions (all three languages)**

In the `### eigen` section:
- All `memory:` sub-action references → `pad:`
- `system/memory.md` → `system/pad.md`
- `memory.edit` → `pad.edit`
- `working memory` → `working pad` or rephrase naturally
- `记忆` → `手记` (zh), keep 文言 phrasing natural

- [ ] **Step 2: Update `psyche` capability descriptions (all three languages)**

In the `### psyche` section:
- `memory:` sub-action → `pad:`
- `your working notes (system/memory.md)` → `your working notes (system/pad.md)`
- `psyche(memory, edit, ...)` → `psyche(pad, edit, ...)`
- `psyche(memory, load)` → `psyche(pad, load)`
- `import into memory` → `import into pad`
- `inject memory into your prompt` → `inject pad into your prompt`
- All Chinese/文言 equivalents

- [ ] **Step 3: Update `library` capability descriptions → `codex`**

In the `### library` section:
- Rename section header: `### library` → `### codex`
- `library(submit, ...)` → `codex(submit, ...)`
- `library(filter, ...)` → `codex(filter, ...)`
- `library(view, ...)` → `codex(view, ...)`
- `library(export, ...)` → `codex(export, ...)`
- `library(consolidate, ...)` → `codex(consolidate, ...)`
- `library(delete, ...)` → `codex(delete, ...)`
- `psyche(memory, edit, files=[...])` → `psyche(pad, edit, files=[...])`
- `psyche(memory, load)` → `psyche(pad, load)`
- `知识库` → `典集` (zh), `藏经阁` → `典` (wen, for the codex description — but NOTE: 藏经阁 is now the library/skills)
- The 文言 for codex: 典 — not 藏经阁. 藏经阁 is now reserved for the skill library.

- [ ] **Step 4: Verify the file reads naturally**

Read through the complete file to ensure no orphaned references remain.

- [ ] **Step 5: Commit**

```bash
git add docs/tool-descriptions.md
git commit -m "docs: rename memory→pad, library→codex in tool descriptions"
```

---

### Task 6: Templates and examples — `init.jsonc` files

**Files:**
- Modify: `tui/internal/preset/templates/init.jsonc`
- Modify: `examples/init.jsonc`

- [ ] **Step 1: Update `tui/internal/preset/templates/init.jsonc`**

- Line 73-76: Comments about "Identity & memory" → "Identity & pad"
- Line 78-81: Comments about "Knowledge library" → "Knowledge codex", and `"library": {}` → `"codex": {}`
- Line 229-232: `"memory": ""` → `"pad": ""`, comments about `system/memory.md` → `system/pad.md`, `memory_file` → `pad_file`

- [ ] **Step 2: Update `examples/init.jsonc`**

Same changes as above template.

- [ ] **Step 3: Commit**

```bash
git add tui/internal/preset/templates/init.jsonc examples/init.jsonc
git commit -m "docs: rename memory→pad, library→codex in init templates"
```

---

### Task 7: Procedures — prose-aware rename

**Files:**
- Modify: `tui/internal/preset/procedures/procedures.md`
- Modify: `tui/internal/preset/procedures/en/procedures.md`

This is a **prose-aware** task. The procedures file is instructions the agent reads and follows. Tool call patterns must be exact. Descriptive text must read naturally.

- [ ] **Step 1: Update tool call patterns**

Throughout both files:
- `psyche(memory, edit, ...)` → `psyche(pad, edit, ...)`
- `psyche(memory, load)` → `psyche(pad, load)`
- `library(submit, ...)` → `codex(submit, ...)`
- `library(filter, ...)` → `codex(filter, ...)`
- `library(view, ...)` → `codex(view, ...)`
- `library(export, ...)` → `codex(export, ...)`
- `skills(action='register')` → `library(action='register')`
- `skills(action='refresh')` → `library(action='refresh')`
- `.skills/custom/` → `.library/custom/`

- [ ] **Step 2: Update descriptive text**

- "Update your memory with..." → "Update your pad with..."
- "library IDs" → "codex IDs"
- "your identity, memory, and library are preserved" → "your identity, pad, and codex are preserved"
- "skill library" references → keep as "skill library" (this is the new name for the capability!)

- [ ] **Step 3: Commit**

```bash
git add tui/internal/preset/procedures/
git commit -m "docs: rename memory→pad, library→codex, skills→library in procedures"
```

---

### Task 8: Covenant (all locales) — prose-aware rename

**Files:**
- Modify: `tui/internal/preset/covenant/en/covenant.md`
- Modify: `tui/internal/preset/covenant/zh/covenant.md`
- Modify: `tui/internal/preset/covenant/wen/covenant.md`
- Modify: `prompt/covenant/en/covenant.md`
- Modify: `prompt/covenant/covenant_en.md` (if exists)
- Modify: `prompt/archive/base_prompt.md`

The covenant is the agent's core philosophical document. Changes must preserve the tone and meaning. All three language variants have substantial references.

- [ ] **Step 1: Read and update `en/covenant.md`**

The subagent should read the entire file, understand the context, then:
- `library` (knowledge archive sense) → `codex`
- `memory` (working notes sense) → `pad`
- Tool call patterns: same as Task 7
- "four layers of memory" → "four layers of persistence" or rephrase
- Table entries: `| **Library** |` → `| **Codex** |`
- Philosophical text: "Library is forever" → "Codex is forever"
- "Memory is your working notebook" → "Pad is your working surface"
- Preserve tone — the covenant is literary

- [ ] **Step 2: Read and update `zh/covenant.md`**

Key Chinese term renames:
- 知识库 → 典集 (everywhere it refers to the knowledge archive capability)
- 记忆 → 手记 (where it refers to system/memory.md working notes, NOT generic "memory" in philosophical sense)
- 藏经阁 → 典 (if used for knowledge archive — careful: 藏经阁 may now be correct for the skill library)
- Tool call patterns: `library(...)` → `codex(...)`, `psyche(memory,...)` → `psyche(pad,...)`
- Table: `| **知识库** |` → `| **典集** |`
- "四层记忆" → rephrase appropriately
- Preserve philosophical tone

- [ ] **Step 3: Read and update `wen/covenant.md`**

Key 文言 term renames:
- 藏经阁 (where it means knowledge archive) → 典
- 记忆 (working notes sense) → 简
- Tool call patterns same as above
- Table: `| **藏经阁** |` → `| **典** |`
- "四层记忆" → rephrase
- Note: 藏经 (scriptures/skill manuals) is still correct when referring to skills stored in the library
- Preserve literary tone — this is 文言

- [ ] **Step 4: Update `prompt/archive/base_prompt.md`**

- "memory, mail, identity" → "pad, mail, identity"
- "Save important data to your library" → "Save important data to your codex"
- "Your memory section below" → "Your pad section below"

- [ ] **Step 5: Commit**

```bash
git add tui/internal/preset/covenant/ prompt/
git commit -m "docs: rename memory→pad, library→codex in covenant and prompts (en/zh/wen)"
```

---

### Task 9: Secretary briefing skill — prose-aware rename

**Files:**
- Modify: `tui/internal/secretary/assets/skills/briefing/SKILL.md`
- Modify: `tui/internal/secretary/assets/comment.md`

The briefing skill is the heaviest file (~30+ references). It contains detailed workflows with tool call syntax that agents follow literally.

- [ ] **Step 1: Update `SKILL.md` — all tool call patterns**

Throughout:
- `psyche(memory, append)` → `psyche(pad, append)`
- `psyche(memory, append, files=[...])` → `psyche(pad, append, files=[...])`
- `psyche(memory, edit, content=...)` → `psyche(pad, edit, content=...)`
- `library(submit, ...)` → `codex(submit, ...)`
- `library(filter, ...)` → `codex(filter, ...)`
- `library(view, ...)` → `codex(view, ...)`
- `library(delete, ...)` → `codex(delete, ...)`

- [ ] **Step 2: Update `SKILL.md` — descriptive text**

- "in your memory" → "in your pad"
- "Your memory tracks" → "Your pad tracks"
- "load your draft entries from library" → "load your draft entries from codex"
- "Your library limit" → "Your codex limit"
- "library entries" → "codex entries"
- "count of library entries" → "count of codex entries"
- "on every memory load" → "on every pad load"
- All occurrences — read the full file for context

- [ ] **Step 3: Update `comment.md`**

- `skills()` → `library()` (where it refers to the capability for loading skills)

- [ ] **Step 4: Commit**

```bash
git add tui/internal/secretary/assets/
git commit -m "docs: rename memory→pad, library→codex, skills→library in secretary assets"
```

---

### Task 10: Intrinsic skill files — prose-aware rename

**Files:**
- Modify: `tui/internal/preset/skills/skills-manual/SKILL.md`
- Modify: `tui/internal/preset/skills/lingtai-anatomy/SKILL.md`
- Modify: `tui/internal/preset/skills/lingtai-tutorial-guide/SKILL-en.md`
- Modify: `tui/internal/preset/skills/lingtai-recipe/SKILL-en.md`
- Modify: `tui/internal/preset/skills/lingtai-recipe/SKILL-zh.md`
- Modify: `tui/internal/preset/skills/lingtai-recipe/SKILL-wen.md`
- Modify: `tui/internal/preset/skills/lingtai-export-recipe/SKILL.md`
- Modify: `tui/internal/preset/skills/lingtai-export-network/SKILL.md`
- Modify: `tui/internal/preset/skills/lingtai-export-network/assets/gitignore.template`
- Modify: `tui/internal/preset/skills/lingtai-export-network/scripts/scrub_ephemeral.py`
- Modify: `tui/internal/preset/skills/lingtai-wechat-setup/SKILL.md`
- Modify: `tui/internal/preset/skills/lingtai-feishu-setup/SKILL.md`
- Modify: `tui/internal/preset/skills/lingtai-imap-setup/SKILL.md`
- Modify: `tui/internal/preset/skills/lingtai-telegram-setup/SKILL.md`

Each skill file needs individual attention. The subagent should read each file fully and make contextually appropriate changes.

- [ ] **Step 1: Update `skills-manual/SKILL.md`**

This is the meta-skill about skills. Key changes:
- `.skills/` → `.library/` (all filesystem paths)
- `skills(action='register')` → `library(action='register')`
- `skills(action='refresh')` → `library(action='refresh')`
- "skill store" → "skill library" or keep as "library"
- `<skills-dir>` → `<library-dir>`
- Keep all references to individual "skills" as-is — they're still called skills

- [ ] **Step 2: Update `lingtai-anatomy/SKILL.md`**

- `.skills/` → `.library/`
- `memory.md` → `pad.md`

- [ ] **Step 3: Update `lingtai-tutorial-guide/SKILL-en.md`**

- `memory` (system file reference) → `pad`
- `skills(action='refresh')` → `library(action='refresh')`
- "**Eigen** — memory, identity management" → "**Eigen** — pad, identity management"

- [ ] **Step 4: Update `lingtai-recipe/SKILL-en.md`, `SKILL-zh.md`, `SKILL-wen.md`**

- `skills/` (recipe component path inside recipe dirs) → keep as `skills/` (recipe format unchanged)
- `.lingtai/.skills/<recipe-name>/` → `.lingtai/.library/<recipe-name>/`
- `/skills` view reference → `/library` view
- References to the capability: "skills" → "library"
- zh/wen: `.lingtai/.skills/` → `.lingtai/.library/`, `/skills` 视图 → `/library` 视图

- [ ] **Step 5: Update `lingtai-export-recipe/SKILL.md`**

- `.lingtai/.skills/` → `.lingtai/.library/`
- `skills/` inside recipe dirs → keep as `skills/` (recipe format unchanged)
- Capability references: "skills" → "library"

- [ ] **Step 6: Update `lingtai-export-network/SKILL.md` and assets**

- `.lingtai/.skills/` → `.lingtai/.library/`
- `gitignore.template`: update comment
- `scrub_ephemeral.py`: update comments and path references

- [ ] **Step 7: Update setup skills (wechat, feishu, imap, telegram)**

Each has a config path reference:
- `.lingtai/.skills/lingtai-{name}-setup/assets/config.json` → `.lingtai/.library/lingtai-{name}-setup/assets/config.json`

- [ ] **Step 8: Commit**

```bash
git add tui/internal/preset/skills/
git commit -m "docs: rename .skills→.library, memory→pad in intrinsic skill files"
```

---

### Task 11: README and design docs

**Files:**
- Modify: `README.md`
- Modify: `README.zh.md`
- Modify: `CLAUDE.md`
- Modify: `docs/design-molt-and-network-intelligence.md`
- Modify: `docs/tool-descriptions.md` (if not already done in Task 5)

- [ ] **Step 1: Update READMEs**

In both `README.md` and `README.zh.md`:
- Capability table: `library` → `codex`, `skills` → `library`
- Any references to `system/memory.md` → `system/pad.md`

- [ ] **Step 2: Update `CLAUDE.md`**

If there are any references to the old naming in CLAUDE.md, update them.

- [ ] **Step 3: Update `docs/design-molt-and-network-intelligence.md`**

- `| **Memory (system/memory.md)** |` → `| **Pad (system/pad.md)** |`

- [ ] **Step 4: Commit**

```bash
git add README.md README.zh.md CLAUDE.md docs/
git commit -m "docs: rename memory→pad, library→codex, skills→library in docs and READMEs"
```

---

### Task 12: Final build, test, and smoke check

- [ ] **Step 1: Build TUI**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai/tui && go build -o bin/lingtai-tui ./...`
Expected: clean compilation.

- [ ] **Step 2: Build portal**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai/portal && go build ./...`
Expected: clean compilation.

- [ ] **Step 3: Run all tests**

Run: `cd /Users/huangzesen/Documents/GitHub/lingtai/tui && go test ./...`
Expected: all tests pass.

- [ ] **Step 4: Grep for orphaned references**

Run targeted greps to find any missed references:
```bash
# In Go code: old capability names in string literals
rg '"library"' tui/ --type go  # should only appear in new-library (skills) context
rg '"skills"' tui/ --type go   # should NOT appear as capability name
rg '"memory"' tui/ --type go   # should NOT appear (except Go memory management)
rg 'system/memory\.md' tui/    # should be zero
rg '\.skills/' tui/            # should be zero (except old migration comments)
rg 'library/library\.json' tui/ # should be zero
```

- [ ] **Step 5: Verify i18n key consistency**

Check that all i18n keys referenced in Go code exist in all three locale files:
```bash
# Extract all i18n.T("...") and i18n.TF("...") calls from Go
rg 'i18n\.T[F]?\("([^"]+)"' tui/ -or '$1' --no-filename | sort -u > /tmp/used_keys.txt
# Check each key exists in en.json
python3 -c "
import json
keys = open('/tmp/used_keys.txt').read().splitlines()
en = json.load(open('tui/i18n/en.json'))
missing = [k for k in keys if k not in en]
print('Missing from en.json:', missing or 'none')
"
```

- [ ] **Step 6: Commit any fixes**

If orphaned references or missing keys were found, fix and commit.
