# recipe.json Manifest and Imported Recipe Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a mandatory `recipe.json` manifest to every recipe and surface auto-detected imported recipes as a first-class picker option above "Adaptive".

**Architecture:** New `RecipeInfo` struct + `LoadRecipeInfo` loader in `preset/recipes.go`, bundled `recipe.json` files in each recipe asset, dynamic picker indices in `firstrun.go` that shift when an imported recipe is present, and i18n keys for the "Imported" label.

**Tech Stack:** Go, Bubble Tea v2, existing `preset` and `tui` packages.

---

## File Structure

| File | Responsibility |
|---|---|
| `tui/internal/preset/recipes.go` | Add `RecipeInfo`, `LoadRecipeInfo`, `RecipeImported` constant |
| `tui/internal/preset/recipes_test.go` | Tests for `LoadRecipeInfo` |
| `tui/internal/preset/recipe_assets/adaptive/recipe.json` | New — bundled manifest |
| `tui/internal/preset/recipe_assets/greeter/recipe.json` | New — bundled manifest |
| `tui/internal/preset/recipe_assets/plain/recipe.json` | New — bundled manifest |
| `tui/internal/preset/recipe_assets/tutorial/recipe.json` | New — bundled manifest |
| `tui/internal/tui/firstrun.go` | Imported recipe detection, dynamic picker, view rendering |
| `tui/i18n/en.json` | Add `recipe.imported` key |
| `tui/i18n/zh.json` | Add `recipe.imported` key |
| `tui/i18n/wen.json` | Add `recipe.imported` key |
| `tui/internal/preset/skills/lingtai-recipe/en/SKILL.md` | Document `recipe.json` |
| `tui/internal/preset/skills/lingtai-recipe/zh/SKILL.md` | Document `recipe.json` |

---

### Task 1: Add `RecipeInfo`, `LoadRecipeInfo`, and `RecipeImported`

**Files:**
- Modify: `tui/internal/preset/recipes.go`
- Modify: `tui/internal/preset/recipes_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `tui/internal/preset/recipes_test.go`:

```go
func TestLoadRecipeInfo_Valid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"Test Recipe","description":"A test"}`), 0o644)

	info, err := LoadRecipeInfo(dir, "en")
	if err != nil {
		t.Fatalf("LoadRecipeInfo error: %v", err)
	}
	if info.Name != "Test Recipe" {
		t.Errorf("Name = %q, want %q", info.Name, "Test Recipe")
	}
	if info.Description != "A test" {
		t.Errorf("Description = %q, want %q", info.Description, "A test")
	}
}

func TestLoadRecipeInfo_LangSpecific(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"Root","description":"root"}`), 0o644)
	os.MkdirAll(filepath.Join(dir, "zh"), 0o755)
	os.WriteFile(filepath.Join(dir, "zh", "recipe.json"), []byte(`{"name":"中文名","description":"中文描述"}`), 0o644)

	info, err := LoadRecipeInfo(dir, "zh")
	if err != nil {
		t.Fatalf("LoadRecipeInfo error: %v", err)
	}
	if info.Name != "中文名" {
		t.Errorf("Name = %q, want %q", info.Name, "中文名")
	}
}

func TestLoadRecipeInfo_FallbackToRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"Root Name","description":"root"}`), 0o644)

	info, err := LoadRecipeInfo(dir, "wen")
	if err != nil {
		t.Fatalf("LoadRecipeInfo error: %v", err)
	}
	if info.Name != "Root Name" {
		t.Errorf("Name = %q, want %q", info.Name, "Root Name")
	}
}

func TestLoadRecipeInfo_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadRecipeInfo(dir, "en")
	if err == nil {
		t.Errorf("LoadRecipeInfo should error when recipe.json missing")
	}
}

func TestLoadRecipeInfo_EmptyName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"","description":"has desc"}`), 0o644)

	_, err := LoadRecipeInfo(dir, "en")
	if err == nil {
		t.Errorf("LoadRecipeInfo should error when name is empty")
	}
}

func TestLoadRecipeInfo_ExtraFieldsIgnored(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"Test","description":"d","version":"1.0","author":"me"}`), 0o644)

	info, err := LoadRecipeInfo(dir, "en")
	if err != nil {
		t.Fatalf("LoadRecipeInfo error: %v", err)
	}
	if info.Name != "Test" {
		t.Errorf("Name = %q, want %q", info.Name, "Test")
	}
}

func TestLoadRecipeInfo_EmptyDir(t *testing.T) {
	_, err := LoadRecipeInfo("", "en")
	if err == nil {
		t.Errorf("LoadRecipeInfo should error on empty dir")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd tui && go test ./internal/preset/ -run TestLoadRecipeInfo -v`
Expected: FAIL — `LoadRecipeInfo` undefined.

- [ ] **Step 3: Implement**

Add to `tui/internal/preset/recipes.go`:

```go
const RecipeImported = "imported"

// RecipeInfo holds the metadata from a recipe's recipe.json manifest.
type RecipeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// LoadRecipeInfo reads recipe.json from a recipe directory, resolved via the
// standard i18n fallback (<lang>/recipe.json → recipe.json). Returns an error
// if the file is not found, unparseable, or has an empty name.
func LoadRecipeInfo(recipeDir, lang string) (RecipeInfo, error) {
	if recipeDir == "" {
		return RecipeInfo{}, fmt.Errorf("empty recipe directory")
	}
	path := resolveRecipeFile(recipeDir, lang, "recipe.json")
	if path == "" {
		return RecipeInfo{}, fmt.Errorf("recipe.json not found in %s", recipeDir)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RecipeInfo{}, fmt.Errorf("read recipe.json: %w", err)
	}
	var info RecipeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return RecipeInfo{}, fmt.Errorf("parse recipe.json: %w", err)
	}
	if info.Name == "" {
		return RecipeInfo{}, fmt.Errorf("recipe.json has empty name in %s", recipeDir)
	}
	return info, nil
}
```

Also add `"encoding/json"` to the imports in `recipes.go` if not already present.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd tui && go test ./internal/preset/ -run TestLoadRecipeInfo -v`
Expected: PASS (all 7 tests).

- [ ] **Step 5: Commit**

```bash
git add tui/internal/preset/recipes.go tui/internal/preset/recipes_test.go
git commit -m "feat(preset): add RecipeInfo, LoadRecipeInfo, and RecipeImported constant"
```

---

### Task 2: Add bundled recipe.json files

**Files:**
- Create: `tui/internal/preset/recipe_assets/adaptive/recipe.json`
- Create: `tui/internal/preset/recipe_assets/greeter/recipe.json`
- Create: `tui/internal/preset/recipe_assets/plain/recipe.json`
- Create: `tui/internal/preset/recipe_assets/tutorial/recipe.json`

- [ ] **Step 1: Create all four files**

`tui/internal/preset/recipe_assets/adaptive/recipe.json`:
```json
{
  "name": "Adaptive",
  "description": "Progressive feature discovery — introduces commands and capabilities as you need them"
}
```

`tui/internal/preset/recipe_assets/greeter/recipe.json`:
```json
{
  "name": "Greeter",
  "description": "Comprehensive guided greeting with full feature overview"
}
```

`tui/internal/preset/recipe_assets/plain/recipe.json`:
```json
{
  "name": "Plain",
  "description": "Minimal — no greeting, no behavioral constraints"
}
```

`tui/internal/preset/recipe_assets/tutorial/recipe.json`:
```json
{
  "name": "Tutorial",
  "description": "Step-by-step walkthrough of lingtai features"
}
```

- [ ] **Step 2: Build to verify embed picks them up**

Run: `cd tui && go build -o /dev/null .`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add tui/internal/preset/recipe_assets/adaptive/recipe.json \
       tui/internal/preset/recipe_assets/greeter/recipe.json \
       tui/internal/preset/recipe_assets/plain/recipe.json \
       tui/internal/preset/recipe_assets/tutorial/recipe.json
git commit -m "feat(preset): add recipe.json manifests to all bundled recipes"
```

---

### Task 3: Imported recipe detection and dynamic picker in firstrun.go

**Files:**
- Modify: `tui/internal/tui/firstrun.go`

This is the core task. It touches:
- Model fields (new `importedRecipe *preset.RecipeInfo`, `importedRecipeDir string`)
- Constructor (detect imported recipe via `LoadRecipeInfo`)
- `recipeNameToIdx` / `recipeIdxToName` (shift indices when imported present)
- Navigation bounds (up/down limits change)
- Enter handler (resolve imported to its dir)
- `viewRecipe()` (render imported option with separator)

- [ ] **Step 1: Add model fields**

In `firstrun.go`, in the recipe picker state block (around line 202), add:

```go
	importedRecipe    *preset.RecipeInfo // non-nil if .lingtai-recipe/ has valid recipe.json
	importedRecipeDir string             // path to .lingtai-recipe/ (only when importedRecipe != nil)
```

- [ ] **Step 2: Update constructor to detect imported recipe**

In `NewFirstRunModel`, replace the existing `.lingtai-recipe/` detection block (lines 351-357):

```go
	// Pre-fill custom path from project-local convention.
	// The projectDir is one level up from baseDir (.lingtai/).
	projectDir := filepath.Dir(baseDir)
	if local := preset.ProjectLocalRecipeDir(projectDir); local != "" {
		m.recipeCustomInput.SetValue(local)
		m.localRecipeDir = local
	}
```

With:

```go
	// Detect imported recipe (.lingtai-recipe/ with valid recipe.json).
	projectDir := filepath.Dir(baseDir)
	if local := preset.ProjectLocalRecipeDir(projectDir); local != "" {
		lang := "en"
		if m.pendingAgentOpts.Language != "" {
			lang = m.pendingAgentOpts.Language
		}
		if info, err := preset.LoadRecipeInfo(local, lang); err == nil {
			m.importedRecipe = &info
			m.importedRecipeDir = local
		} else {
			// Has .lingtai-recipe/ but no valid recipe.json — ignore
			m.localRecipeDir = local // keep for custom pre-fill fallback
			m.recipeCustomInput.SetValue(local)
		}
	}
```

- [ ] **Step 3: Replace `recipeNameToIdx` and `recipeIdxToName` with methods**

These need access to `importedRecipe` to shift indices. Convert from free functions to methods on `FirstRunModel`:

```go
// hasImportedRecipe returns true if an imported recipe was detected.
func (m FirstRunModel) hasImportedRecipe() bool {
	return m.importedRecipe != nil
}

// recipeMaxIdx returns the maximum recipe index.
func (m FirstRunModel) recipeMaxIdx() int {
	if m.hasImportedRecipe() {
		return 5 // 0=imported, 1=adaptive, 2=greeter, 3=plain, 4=tutorial, 5=custom
	}
	return 4 // 0=adaptive, 1=greeter, 2=plain, 3=tutorial, 4=custom
}

func (m FirstRunModel) recipeNameToIdx(name string) int {
	offset := 0
	if m.hasImportedRecipe() {
		if name == preset.RecipeImported {
			return 0
		}
		offset = 1
	}
	switch name {
	case preset.RecipeGreeter:
		return 1 + offset
	case preset.RecipePlain:
		return 2 + offset
	case preset.RecipeTutorial:
		return 3 + offset
	case preset.RecipeCustom:
		return 4 + offset
	default:
		return offset // adaptive (default), or 0 if no imported
	}
}

func (m FirstRunModel) recipeIdxToName(idx int) string {
	if m.hasImportedRecipe() {
		if idx == 0 {
			return preset.RecipeImported
		}
		idx-- // shift down for the rest
	}
	switch idx {
	case 1:
		return preset.RecipeGreeter
	case 2:
		return preset.RecipePlain
	case 3:
		return preset.RecipeTutorial
	case 4:
		return preset.RecipeCustom
	default:
		return preset.RecipeAdaptive
	}
}
```

Delete the old free functions `recipeNameToIdx` and `recipeIdxToName` (lines 2529-2557).

Update all call sites from `recipeNameToIdx(x)` to `m.recipeNameToIdx(x)` and `recipeIdxToName(x)` to `m.recipeIdxToName(x)`. Search for these in firstrun.go and update each occurrence.

- [ ] **Step 4: Update navigation bounds**

In the `stepRecipe` up/down handlers (lines 1304-1326), replace hardcoded `4` with `m.recipeMaxIdx()`:

Up handler:
```go
			case "up":
				if m.recipeIdx > 0 {
					m.recipeIdx--
					m.recipeCustomErr = ""
				}
				if m.recipeIdx == m.recipeMaxIdx() {
					m.recipeCustomInput.Focus()
				} else {
					m.recipeCustomInput.Blur()
				}
				return m, nil
```

Down handler:
```go
			case "down":
				if m.recipeIdx < m.recipeMaxIdx() {
					m.recipeIdx++
					m.recipeCustomErr = ""
				}
				if m.recipeIdx == m.recipeMaxIdx() {
					m.recipeCustomInput.Focus()
				} else {
					m.recipeCustomInput.Blur()
				}
				return m, nil
```

- [ ] **Step 5: Update enter handler**

In the enter handler (around line 1336), update to handle imported recipe:

```go
			case "enter":
				recipeName := m.recipeIdxToName(m.recipeIdx)
				customDir := ""
				if recipeName == preset.RecipeImported {
					customDir = m.importedRecipeDir
				} else if recipeName == preset.RecipeCustom {
					customDir = m.recipeCustomInput.Value()
					if err := preset.ValidateCustomDir(customDir); err != nil {
						m.recipeCustomErr = err.Error()
						return m, nil
					}
				}

				// Mid-life recipe change detection -> route to swap confirm
				if m.setupMode && recipeChanged(m.currentRecipe, m.currentCustomDir, recipeName, customDir) {
					m.pendingRecipeName = recipeName
					m.pendingCustomDir = customDir
					m.step = stepRecipeSwapConfirm
					m.swapConfirmIdx = 0
					return m, nil
				}

				return m.performRecipeSave(recipeName, customDir)
```

Also update the custom input forwarding check — change `m.recipeIdx == 4` to `m.recipeIdxToName(m.recipeIdx) == preset.RecipeCustom`:

```go
			default:
				if m.recipeIdxToName(m.recipeIdx) == preset.RecipeCustom {
					var cmd tea.Cmd
					m.recipeCustomInput, cmd = m.recipeCustomInput.Update(msg)
					return m, cmd
				}
				return m, nil
```

- [ ] **Step 6: Update `viewRecipe()` rendering**

Replace the recipe list rendering block (lines 2636-2663) with:

```go
	var leftBlock strings.Builder

	if m.hasImportedRecipe() {
		// Imported recipe — shown first, above separator
		importedStyle := lipgloss.NewStyle().Foreground(ColorActive)
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if m.recipeIdx == 0 {
			cursor = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		}
		leftBlock.WriteString(cursor + style.Render(m.importedRecipe.Name) + "  " + importedStyle.Render(i18n.T("recipe.imported")) + "\n")
		leftBlock.WriteString("    " + StyleFaint.Render(m.importedRecipe.Description) + "\n")
		leftBlock.WriteString("\n  " + StyleFaint.Render("───────────────────────────") + "\n")
	}

	recommendedStyle := lipgloss.NewStyle().Foreground(ColorAgent)
	adaptiveIdx := 0
	if m.hasImportedRecipe() {
		adaptiveIdx = 1
	}
	leftBlock.WriteString("  " + recommendedStyle.Render(i18n.T("recipe.recommended")) + "\n")

	bundledRecipes := preset.BundledRecipes() // [adaptive, greeter, plain, tutorial]
	for i, name := range bundledRecipes {
		globalIdx := adaptiveIdx + i
		// Separator between adaptive (recommended) and the rest
		if i == 1 {
			leftBlock.WriteString("\n  " + StyleFaint.Render(i18n.T("recipe.others")) + "\n")
		}
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if globalIdx == m.recipeIdx {
			cursor = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		}
		label := i18n.T("recipe.name." + name)
		desc := i18n.T("recipe.desc." + name)
		leftBlock.WriteString(cursor + style.Render(label) + "\n")
		leftBlock.WriteString("    " + StyleFaint.Render(desc) + "\n")
	}

	// Custom entry
	customIdx := m.recipeMaxIdx()
	{
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if customIdx == m.recipeIdx {
			cursor = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		}
		label := i18n.T("recipe.name.custom")
		desc := i18n.T("recipe.desc.custom")
		leftBlock.WriteString(cursor + style.Render(label) + "\n")
		leftBlock.WriteString("    " + StyleFaint.Render(desc) + "\n")
	}

	if m.recipeIdxToName(m.recipeIdx) == preset.RecipeCustom {
		leftBlock.WriteString("\n  " + i18n.T("recipe.custom_path") + "\n")
		leftBlock.WriteString("  " + m.recipeCustomInput.View() + "\n")
		if m.recipeCustomErr != "" {
			errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
			leftBlock.WriteString("  " + errStyle.Render(m.recipeCustomErr) + "\n")
		}
	}
```

Also remove the `localRecipeDir` hint from the top of `viewRecipe()` (lines 2629-2632) — the imported recipe now has its own slot, so the "local recipe found" notice is redundant:

```go
	// Remove these lines:
	// if m.localRecipeDir != "" {
	//     foundStyle := lipgloss.NewStyle().Foreground(ColorActive)
	//     b.WriteString("  " + foundStyle.Render(i18n.T("recipe.local_found")) + "\n")
	// }
```

- [ ] **Step 7: Update `renderRecipeSidePane` for imported recipe**

The side pane resolves greet/comment paths for preview. It needs to handle the imported recipe. Find where it resolves the recipe directory for preview (it likely uses `recipeIdxToName` to get the recipe dir). Ensure that when `RecipeImported` is selected, it uses `m.importedRecipeDir` as the recipe directory — same pattern as custom.

- [ ] **Step 8: Update preselectedRecipe handling**

When an imported recipe is detected and no `preselectedRecipe` is set, default `recipeIdx` to 0 (imported). In the constructor, after detecting the imported recipe:

```go
	// Default to imported recipe if detected and no explicit preselection
	if m.importedRecipe != nil && preselectedRecipe == "" {
		m.recipeIdx = 0
	} else {
		m.recipeIdx = m.recipeNameToIdx(preselectedRecipe)
	}
```

This replaces the existing line 360: `m.recipeIdx = recipeNameToIdx(preselectedRecipe)`.

- [ ] **Step 9: Build to verify**

Run: `cd tui && go build -o /dev/null .`
Expected: Build succeeds.

- [ ] **Step 10: Commit**

```bash
git add tui/internal/tui/firstrun.go
git commit -m "feat(tui): imported recipe picker slot with dynamic indices"
```

---

### Task 4: Add i18n keys

**Files:**
- Modify: `tui/i18n/en.json`
- Modify: `tui/i18n/zh.json`
- Modify: `tui/i18n/wen.json`

- [ ] **Step 1: Add keys**

In each file, add the `recipe.imported` key after the existing `recipe.local_found` key:

**`en.json`:**
```json
"recipe.imported": "Imported",
```

**`zh.json`:**
```json
"recipe.imported": "已导入",
```

**`wen.json`:**
```json
"recipe.imported": "已导入",
```

- [ ] **Step 2: Build to verify**

Run: `cd tui && go build -o /dev/null .`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add tui/i18n/en.json tui/i18n/zh.json tui/i18n/wen.json
git commit -m "feat(i18n): add recipe.imported key for imported recipe label"
```

---

### Task 5: Update `lingtai-recipe` skill docs

**Files:**
- Modify: `tui/internal/preset/skills/lingtai-recipe/en/SKILL.md`
- Modify: `tui/internal/preset/skills/lingtai-recipe/zh/SKILL.md`

- [ ] **Step 1: Update English SKILL.md**

In the "Recipe Directory Structure" section, add `recipe.json` to the tree:

```
my-recipe/
  recipe.json             # Required — name and description
  en/
    recipe.json           # Optional — lang-specific override
    greet.md
    comment.md
  zh/
    recipe.json
    greet.md
    comment.md
  skills/
    ...
```

Add a new section after "The Three Components" and before "i18n Fallback Rules":

```markdown
## recipe.json — Recipe Manifest

Every recipe must contain a `recipe.json` at root level (language-specific overrides are optional):

` ` `json
{
  "name": "My Recipe Name",
  "description": "One-line description of what this recipe does"
}
` ` `

- `name` — **required**, displayed in the TUI recipe picker
- `description` — **required**, shown as hint text in the picker
- Extra fields are ignored but tolerated (forward-compatible)

Without a valid `recipe.json`, the recipe will not be recognized as importable. The TUI only auto-detects `.lingtai-recipe/` directories that contain a valid manifest.
```

(Use actual triple backticks in the file, not the escaped ` ` ` shown here.)

- [ ] **Step 2: Update Chinese SKILL.md**

Same changes in Chinese:

Add `recipe.json` to the directory structure tree.

Add section:

```markdown
## recipe.json — 配方清单

每个配方的根目录必须包含 `recipe.json`（语言特定版本可选）：

` ` `json
{
  "name": "配方名称",
  "description": "一行描述"
}
` ` `

- `name` — **必须**，显示在 TUI 配方选择器中
- `description` — **必须**，作为提示文本显示
- 额外字段会被忽略但不会报错（向前兼容）

没有有效 `recipe.json` 的配方不会被识别为可导入。TUI 仅自动检测包含有效清单的 `.lingtai-recipe/` 目录。
```

- [ ] **Step 3: Commit**

```bash
git add tui/internal/preset/skills/lingtai-recipe/en/SKILL.md \
       tui/internal/preset/skills/lingtai-recipe/zh/SKILL.md
git commit -m "docs(skills): document recipe.json manifest in lingtai-recipe skill"
```

---

### Task 6: Integration test — full build and verify

**Files:**
- No new files.

- [ ] **Step 1: Run all preset tests**

Run: `cd tui && go test ./internal/preset/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 2: Run all tui tests**

Run: `cd tui && go test ./internal/tui/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 3: Build the binary**

Run: `cd tui && make build`
Expected: Build succeeds.

- [ ] **Step 4: Verify bundled recipe.json files exist in embed**

Run: `ls tui/internal/preset/recipe_assets/*/recipe.json`
Expected: Four files (adaptive, greeter, plain, tutorial).

- [ ] **Step 5: Final commit (if any fixups needed)**

Only if previous steps required changes.
