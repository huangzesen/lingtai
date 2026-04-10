# Reusable Markdown Viewer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract a reusable two-panel markdown viewer from `/skills`, then wire it for both `/skills` and recipe Ctrl+O preview.

**Architecture:** New `MarkdownViewerModel` in `mdviewer.go` — standalone `tea.Model` that takes a pre-built `[]MarkdownEntry` list. Callers build entries and pass them in. The viewer handles all layout, rendering, and keyboard navigation. `/skills` slim down to scan + entry building. Recipe preview in `firstrun.go` replaced with viewer delegation.

**Tech Stack:** Go, Bubble Tea v2, glamour, lipgloss, viewport.

---

## File Structure

| File | Responsibility |
|---|---|
| `tui/internal/tui/mdviewer.go` | **New.** `MarkdownEntry`, `MarkdownViewerModel`, two-panel layout, glamour rendering, viewport |
| `tui/internal/tui/mdviewer_test.go` | **New.** Tests for entry rendering, frontmatter stripping |
| `tui/internal/tui/skills.go` | **Slim down.** Keep scan + frontmatter. Add `buildSkillEntries`. Remove rendering/model. |
| `tui/internal/tui/firstrun.go` | **Slim down.** Remove recipe preview code. Add `recipeViewer`, `buildRecipeEntries`, delegation. |
| `tui/internal/tui/app.go` | Update `/skills` to use viewer. |

---

### Task 1: Create `MarkdownViewerModel` in `mdviewer.go`

This is the core extraction. Move the two-panel layout, viewport management, glamour rendering, and frontmatter stripping from `skills.go` into a generic viewer.

**Files:**
- Create: `tui/internal/tui/mdviewer.go`
- Create: `tui/internal/tui/mdviewer_test.go`

- [ ] **Step 1: Create `mdviewer.go` with types and constructor**

Create `tui/internal/tui/mdviewer.go`:

```go
package tui

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"

	"github.com/anthropics/lingtai-tui/i18n"
)

// MarkdownEntry is a single item in the markdown viewer's left panel.
type MarkdownEntry struct {
	Label   string // display name shown in list
	Group   string // section header (entries with same group are grouped)
	Path    string // absolute path to file (read on selection)
	Content string // pre-built content (used instead of Path if non-empty)
}

// MarkdownViewerCloseMsg is sent when the user exits the viewer.
type MarkdownViewerCloseMsg struct{}

// MarkdownViewerModel is a two-panel view: entry list (left) + rendered
// markdown content (right). It is a standalone tea.Model — callers build
// the entry list and pass it in.
type MarkdownViewerModel struct {
	entries []MarkdownEntry
	title   string
	width   int
	height  int
	cursor  int

	viewport viewport.Model
	ready    bool
}

const (
	mdvHeaderLines = 2
	mdvFooterLines = 2
)

// NewMarkdownViewer creates a viewer with the given entries and title.
func NewMarkdownViewer(entries []MarkdownEntry, title string) MarkdownViewerModel {
	return MarkdownViewerModel{
		entries: entries,
		title:   title,
	}
}

func (m MarkdownViewerModel) Init() tea.Cmd { return nil }
```

- [ ] **Step 2: Add Update method**

Append to `mdviewer.go`:

```go
func (m MarkdownViewerModel) Update(msg tea.Msg) (MarkdownViewerModel, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpHeight := m.height - mdvHeaderLines - mdvFooterLines
		if vpHeight < 1 {
			vpHeight = 1
		}
		if !m.ready {
			m.viewport = viewport.New()
			m.viewport.SetWidth(m.width)
			m.viewport.SetHeight(vpHeight)
			m.ready = true
		} else {
			m.viewport.SetWidth(m.width)
			m.viewport.SetHeight(vpHeight)
		}
		m.syncContent()

	case tea.MouseWheelMsg:
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			return m, func() tea.Msg { return MarkdownViewerCloseMsg{} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.syncContent()
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.syncContent()
			}
			return m, nil
		default:
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}
```

- [ ] **Step 3: Add rendering methods**

Append to `mdviewer.go`:

```go
func (m *MarkdownViewerModel) syncContent() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.renderBody())
}

func (m MarkdownViewerModel) renderBody() string {
	leftW := m.width / 3
	if leftW < 25 {
		leftW = 25
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW := m.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}
	if leftW+1+rightW > m.width && m.width > 1 {
		rightW = m.width - leftW - 1
		if rightW < 0 {
			rightW = 0
		}
	}

	leftContent := m.renderLeft(leftW)
	rightContent := m.renderRight(rightW)

	leftLines := strings.Split(leftContent, "\n")
	rightLines := strings.Split(rightContent, "\n")

	vpHeight := m.height - mdvHeaderLines - mdvFooterLines
	if vpHeight < 1 {
		vpHeight = 1
	}
	for len(leftLines) < vpHeight {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < vpHeight {
		rightLines = append(rightLines, "")
	}
	for len(leftLines) < len(rightLines) {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < len(leftLines) {
		rightLines = append(rightLines, "")
	}

	sep := lipgloss.NewStyle().Foreground(ColorTextFaint).Render("│")
	var body strings.Builder
	for i := 0; i < len(leftLines); i++ {
		l := padToWidth(leftLines[i], leftW)
		body.WriteString(l + sep + rightLines[i] + "\n")
	}
	return strings.TrimRight(body.String(), "\n")
}

func (m MarkdownViewerModel) renderLeft(maxW int) string {
	selectedStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(ColorText)
	sectionStyle := lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b"))

	var lines []string
	lastGroup := ""
	entryIdx := 0

	for i, e := range m.entries {
		if e.Group != lastGroup {
			if lastGroup != "" {
				lines = append(lines, "")
			}
			// Problems group uses warning style
			gs := sectionStyle
			if e.Group == "Problems" {
				gs = warnStyle
			}
			lines = append(lines, "  "+gs.Render(e.Group))
			lines = append(lines, "")
			lastGroup = e.Group
		}

		marker := "  "
		style := normalStyle
		if e.Group == "Problems" {
			style = warnStyle
		}
		if i == m.cursor {
			marker = "> "
			style = selectedStyle
		}
		lines = append(lines, "  "+marker+style.Render(e.Label))
		entryIdx++
	}

	if len(m.entries) == 0 {
		lines = append(lines, "  "+StyleFaint.Render("(empty)"))
	}

	return strings.Join(lines, "\n")
}

func (m MarkdownViewerModel) renderRight(maxW int) string {
	if len(m.entries) == 0 || m.cursor >= len(m.entries) {
		return "\n  " + StyleFaint.Render("(no content)")
	}

	e := m.entries[m.cursor]

	// Get raw content
	var raw string
	if e.Content != "" {
		raw = e.Content
	} else if e.Path != "" {
		data, err := os.ReadFile(e.Path)
		if err != nil {
			return "\n  " + StyleFaint.Render("(file not found)")
		}
		raw = string(data)
	} else {
		return "\n  " + StyleFaint.Render("(no content)")
	}

	// Strip YAML frontmatter if present
	if loc := fmRe.FindStringIndex(raw); loc != nil {
		raw = raw[loc[1]:]
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "\n  " + StyleFaint.Render("(empty)")
	}

	// Glamour markdown rendering
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(ActiveTheme().GlamourStyle),
		glamour.WithWordWrap(maxW-2),
	)
	if err == nil {
		if rendered, rerr := r.Render(raw); rerr == nil {
			return "\n" + rendered
		}
	}

	// Fallback: plain text wrap
	wrapped := lipgloss.NewStyle().Width(maxW - 2).Render(raw)
	var lines []string
	lines = append(lines, "")
	for _, line := range strings.Split(wrapped, "\n") {
		lines = append(lines, " "+line)
	}
	return strings.Join(lines, "\n")
}

func (m MarkdownViewerModel) View() string {
	title := StyleTitle.Render("  "+m.title) + "\n" + strings.Repeat("\u2500", m.width)

	scrollHint := ""
	if m.ready && !m.viewport.AtBottom() {
		scrollHint = " " + RuneBullet + " pgup/pgdn scroll"
	}
	footer := strings.Repeat("\u2500", m.width) + "\n" +
		StyleFaint.Render("  ↑↓ "+i18n.T("welcome.select_lang")+"  [Esc] "+i18n.T("firstrun.back")+scrollHint)

	return title + "\n" + m.viewport.View() + "\n" + footer
}
```

- [ ] **Step 4: Write tests**

Create `tui/internal/tui/mdviewer_test.go`:

```go
package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownViewer_EmptyEntries(t *testing.T) {
	m := NewMarkdownViewer(nil, "Test")
	if len(m.entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(m.entries))
	}
}

func TestMarkdownViewer_CursorBounds(t *testing.T) {
	entries := []MarkdownEntry{
		{Label: "a", Group: "G", Content: "hello"},
		{Label: "b", Group: "G", Content: "world"},
	}
	m := NewMarkdownViewer(entries, "Test")
	if m.cursor != 0 {
		t.Errorf("initial cursor = %d, want 0", m.cursor)
	}
}

func TestMarkdownViewer_ContentEntry(t *testing.T) {
	entries := []MarkdownEntry{
		{Label: "test", Group: "G", Content: "# Hello\n\nThis is content."},
	}
	m := NewMarkdownViewer(entries, "Test")
	m.width = 80
	m.height = 24
	// Verify renderRight doesn't crash and produces output
	right := m.renderRight(60)
	if right == "" {
		t.Error("renderRight returned empty for content entry")
	}
}

func TestMarkdownViewer_PathEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("# Test File\n\nContent here."), 0o644)

	entries := []MarkdownEntry{
		{Label: "test.md", Group: "G", Path: path},
	}
	m := NewMarkdownViewer(entries, "Test")
	m.width = 80
	m.height = 24
	right := m.renderRight(60)
	if right == "" {
		t.Error("renderRight returned empty for path entry")
	}
}

func TestMarkdownViewer_FrontmatterStripped(t *testing.T) {
	entries := []MarkdownEntry{
		{Label: "skill", Group: "G", Content: "---\nname: test\n---\n# Real Content"},
	}
	m := NewMarkdownViewer(entries, "Test")
	m.width = 80
	m.height = 24
	right := m.renderRight(60)
	if right == "" {
		t.Error("renderRight returned empty")
	}
	// Frontmatter should be stripped — "name: test" should not appear
	if contains(right, "name: test") {
		t.Error("frontmatter was not stripped")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestMarkdownViewer_GroupRendering(t *testing.T) {
	entries := []MarkdownEntry{
		{Label: "a", Group: "Skills", Content: "x"},
		{Label: "b", Group: "Skills", Content: "y"},
		{Label: "c", Group: "Imported", Content: "z"},
	}
	m := NewMarkdownViewer(entries, "Test")
	m.width = 80
	m.height = 24
	left := m.renderLeft(30)
	if left == "" {
		t.Error("renderLeft returned empty")
	}
	// Both group headers should appear
	if !containsHelper(left, "Skills") {
		t.Error("missing Skills group header")
	}
	if !containsHelper(left, "Imported") {
		t.Error("missing Imported group header")
	}
}
```

- [ ] **Step 5: Run tests**

Run: `cd tui && go test ./internal/tui/ -run TestMarkdownViewer -v`
Expected: PASS (all 6 tests).

- [ ] **Step 6: Build to verify**

Run: `cd tui && go build -o /dev/null .`
Expected: Build succeeds (mdviewer.go compiles but is not yet wired).

- [ ] **Step 7: Commit**

```bash
git add tui/internal/tui/mdviewer.go tui/internal/tui/mdviewer_test.go
git commit -m "feat(tui): add reusable MarkdownViewerModel in mdviewer.go"
```

---

### Task 2: Add `buildSkillEntries` and slim down `skills.go`

Replace `SkillsModel` with entry building. The rendering code is now in `mdviewer.go`.

**Files:**
- Modify: `tui/internal/tui/skills.go`
- Modify: `tui/internal/tui/app.go`

- [ ] **Step 1: Add `buildSkillEntries` to `skills.go`**

Add after `scanSkills` in `skills.go`:

```go
// buildSkillEntries converts scan results into MarkdownEntry items for the
// markdown viewer. Bundled (non-symlink) skills go under "Skills", symlinked
// skills under "Imported", and broken folders under "Problems".
func buildSkillEntries(skillsDir string, skills []skillEntry, problems []skillProblem) []MarkdownEntry {
	var entries []MarkdownEntry

	// Separate bundled vs imported (symlinked)
	var bundled, imported []skillEntry
	for _, sk := range skills {
		dir := filepath.Dir(sk.Path) // .skills/<name>/
		info, err := os.Lstat(dir)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			imported = append(imported, sk)
		} else {
			bundled = append(bundled, sk)
		}
	}

	for _, sk := range bundled {
		label := sk.Name
		if sk.Version != "" {
			label += " " + sk.Version
		}
		entries = append(entries, MarkdownEntry{
			Label: label,
			Group: i18n.T("skills.installed"),
			Path:  sk.Path,
		})
	}

	for _, sk := range imported {
		label := sk.Name
		if sk.Version != "" {
			label += " " + sk.Version
		}
		entries = append(entries, MarkdownEntry{
			Label: label,
			Group: i18n.T("recipe.imported"),
			Path:  sk.Path,
		})
	}

	for _, p := range problems {
		entries = append(entries, MarkdownEntry{
			Label:   p.Folder,
			Group:   i18n.T("skills.problems"),
			Content: p.Reason,
		})
	}

	return entries
}
```

- [ ] **Step 2: Remove rendering code from `skills.go`**

Delete the following from `skills.go`:
- `SkillsModel` struct (lines 33-46)
- `NewSkillsModel` function (lines 48-52)
- `skillsLoadMsg` type (lines 54-58)
- `skillsHeaderLines`, `skillsFooterLines` constants (lines 60-63)
- `loadData` method (lines 142-145)
- `Init` method (line 147)
- `Update` method (lines 149-206)
- `syncViewportContent` method (lines 210-216)
- `renderBody` method (lines 218-270)
- `renderLeft` method (lines 272-313)
- `renderRight` method (lines 315-350)
- `View` method (lines 352-363)

Remove unused imports: `tea`, `viewport`, `glamour`, `lipgloss`. Keep: `os`, `path/filepath`, `regexp`, `sort`, `strings`, and the `i18n` import.

The file should keep only: `skillEntry`, `skillProblem`, `fmRe`, `kvRe`, `parseFrontmatter`, `scanSkills`, and the new `buildSkillEntries`.

- [ ] **Step 3: Update `app.go` to use `MarkdownViewerModel` for skills**

In `app.go`, the `skills` field is currently `SkillsModel`. Change it:

Replace the field declaration:
```go
skills      SkillsModel
```
With:
```go
skills      MarkdownViewerModel
```

Replace the two `/skills` switch-to blocks (around lines 545-547 and 716-718):
```go
// OLD:
a.currentView = appViewSkills
a.skills = NewSkillsModel(a.projectDir)
return a, tea.Batch(a.skills.Init(), a.sendSize())
```
With:
```go
// NEW:
a.currentView = appViewSkills
skillsDir := filepath.Join(a.projectDir, ".skills")
skills, problems := scanSkills(skillsDir)
entries := buildSkillEntries(skillsDir, skills, problems)
a.skills = NewMarkdownViewer(entries, i18n.T("skills.title"))
return a, a.sendSize()
```

Update the Update handler for skills (around line 366):
```go
// OLD:
case appViewSkills:
    updated, cmd := a.skills.Update(msg)
    a.skills = updated
```
This stays the same — `MarkdownViewerModel` has the same Update signature pattern.

Add handling for `MarkdownViewerCloseMsg` in the app's Update (where `ViewChangeMsg` is handled):
```go
case MarkdownViewerCloseMsg:
    a.currentView = appViewMail
    return a, a.sendSize()
```

The View case stays the same:
```go
case appViewSkills:
    content = a.skills.View()
```

- [ ] **Step 4: Build and test**

Run: `cd tui && go build -o /dev/null .`
Expected: Build succeeds.

Run: `cd tui && go test ./internal/tui/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add tui/internal/tui/skills.go tui/internal/tui/app.go
git commit -m "refactor(tui): replace SkillsModel with MarkdownViewerModel for /skills"
```

---

### Task 3: Replace recipe preview with `MarkdownViewerModel`

Remove all recipe preview code from `firstrun.go` and replace with viewer delegation.

**Files:**
- Modify: `tui/internal/tui/firstrun.go`

- [ ] **Step 1: Add `buildRecipeEntries` function**

Add to `firstrun.go` (or a new `recipe_entries.go` if preferred — but keeping in `firstrun.go` is fine since it's only used there):

```go
// buildRecipeEntries scans a recipe directory and returns MarkdownEntry items
// for the markdown viewer. Discovers all lang variants of greet.md, comment.md,
// recipe.json, and skills.
func buildRecipeEntries(recipeDir string) []MarkdownEntry {
	if recipeDir == "" {
		return nil
	}
	var entries []MarkdownEntry

	// Helper: scan for a file across root and lang subdirs
	addFile := func(filename, group string) {
		// Root version
		rootPath := filepath.Join(recipeDir, filename)
		if info, err := os.Stat(rootPath); err == nil && !info.IsDir() {
			entries = append(entries, MarkdownEntry{
				Label: filename,
				Group: group,
				Path:  rootPath,
			})
		}
		// Lang variants
		dirEntries, err := os.ReadDir(recipeDir)
		if err != nil {
			return
		}
		for _, e := range dirEntries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "skills" {
				continue
			}
			langPath := filepath.Join(recipeDir, e.Name(), filename)
			if info, err := os.Stat(langPath); err == nil && !info.IsDir() {
				entries = append(entries, MarkdownEntry{
					Label: filename + " (" + e.Name() + ")",
					Group: group,
					Path:  langPath,
				})
			}
		}
	}

	addFile("greet.md", "greet.md")
	addFile("comment.md", "comment.md")
	addFile("recipe.json", "recipe.json")

	// Skills
	skillsRoot := filepath.Join(recipeDir, "skills")
	skillDirs, err := os.ReadDir(skillsRoot)
	if err == nil {
		for _, sd := range skillDirs {
			if !sd.IsDir() || strings.HasPrefix(sd.Name(), ".") {
				continue
			}
			skillName := sd.Name()
			// Root SKILL.md
			rootSkill := filepath.Join(skillsRoot, skillName, "SKILL.md")
			if info, err := os.Stat(rootSkill); err == nil && !info.IsDir() {
				entries = append(entries, MarkdownEntry{
					Label: skillName + "/SKILL.md",
					Group: i18n.T("skills.title"),
					Path:  rootSkill,
				})
			}
			// Lang variants
			langDirs, err := os.ReadDir(filepath.Join(skillsRoot, skillName))
			if err != nil {
				continue
			}
			for _, ld := range langDirs {
				if !ld.IsDir() || strings.HasPrefix(ld.Name(), ".") {
					continue
				}
				langSkill := filepath.Join(skillsRoot, skillName, ld.Name(), "SKILL.md")
				if info, err := os.Stat(langSkill); err == nil && !info.IsDir() {
					entries = append(entries, MarkdownEntry{
						Label: skillName + "/SKILL.md (" + ld.Name() + ")",
						Group: i18n.T("skills.title"),
						Path:  langSkill,
					})
				}
			}
		}
	}

	return entries
}
```

- [ ] **Step 2: Replace recipe preview fields**

In FirstRunModel, replace:
```go
	// Recipe preview sub-view (reuses skills-style two-panel layout)
	recipePreview       bool
	recipePreviewFile   int
	recipePreviewVP     viewport.Model
	recipePreviewReady  bool
```

With:
```go
	// Recipe viewer (Ctrl+O from recipe picker)
	recipeViewer *MarkdownViewerModel
```

- [ ] **Step 3: Update Ctrl+O handler**

In the `stepRecipe` keyboard handler, replace the `"ctrl+o"` case:

```go
			case "ctrl+o":
				recipeDir := m.resolveCurrentRecipeDir()
				if recipeDir == "" {
					return m, nil
				}
				entries := buildRecipeEntries(recipeDir)
				if len(entries) == 0 {
					return m, nil
				}
				viewer := NewMarkdownViewer(entries, i18n.T("recipe.preview"))
				m.recipeViewer = &viewer
				return m, nil
```

Add a helper method to resolve the current recipe directory:

```go
func (m FirstRunModel) resolveCurrentRecipeDir() string {
	recipeName := m.recipeIdxToName(m.recipeIdx)
	switch recipeName {
	case preset.RecipeImported:
		return m.importedRecipeDir
	case preset.RecipeCustom:
		dir := m.recipeCustomInput.Value()
		if dir == "" {
			return ""
		}
		if err := preset.ValidateCustomDir(dir); err != nil {
			return ""
		}
		return dir
	default:
		return preset.RecipeDir(m.globalDir, recipeName)
	}
}
```

- [ ] **Step 4: Add viewer delegation in Update**

At the top of `stepRecipe` handling in Update (before the keyboard handler), add:

```go
		case stepRecipe:
			// Delegate to recipe viewer if active
			if m.recipeViewer != nil {
				switch msg := msg.(type) {
				case MarkdownViewerCloseMsg:
					m.recipeViewer = nil
					return m, nil
				case tea.WindowSizeMsg:
					updated, cmd := m.recipeViewer.Update(msg)
					m.recipeViewer = &updated
					// Also update own dimensions
					m.width = msg.Width
					m.height = msg.Height
					return m, cmd
				default:
					updated, cmd := m.recipeViewer.Update(msg)
					m.recipeViewer = &updated
					return m, cmd
				}
			}
			// ... existing stepRecipe handling below
```

- [ ] **Step 5: Update View**

In the View method, replace the recipe preview early return:
```go
// OLD:
if m.recipePreview {
    return m.viewRecipePreview()
}
```

With:
```go
if m.recipeViewer != nil {
    return m.recipeViewer.View()
}
```

- [ ] **Step 6: Delete old recipe preview code**

Delete from `firstrun.go`:
- `enterRecipePreview()` function
- `viewRecipePreview()` function
- `syncRecipePreviewContent()` function
- `renderRecipeFileContent()` function
- The recipe preview keyboard handling block (the `if m.recipePreview { ... }` block in stepRecipe)
- Window resize handling for `recipePreviewReady`/`recipePreviewVP`

Delete from `firstrun.go` the inline side pane:
- `renderRecipeSidePane()` function
- `recipeFilePreview()` function
- `recipeWidePaneThreshold` constant
- The `wide` logic block in `viewRecipe()` that calls `renderRecipeSidePane`

Simplify `viewRecipe()` to just show the left panel list without the side pane.

- [ ] **Step 7: Remove unused imports**

After deleting the above, remove unused imports from `firstrun.go`. The `viewport` import may no longer be needed if no other code uses it. The `glamour` import should be gone. Check and clean.

- [ ] **Step 8: Build and test**

Run: `cd tui && go build -o /dev/null .`
Expected: Build succeeds.

Run: `cd tui && go test ./internal/tui/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 9: Commit**

```bash
git add tui/internal/tui/firstrun.go
git commit -m "refactor(tui): replace recipe preview with MarkdownViewerModel"
```

---

### Task 4: Add i18n key for recipe preview title

**Files:**
- Modify: `tui/i18n/en.json`
- Modify: `tui/i18n/zh.json`
- Modify: `tui/i18n/wen.json`

- [ ] **Step 1: Add keys**

Add `recipe.preview` key (used as viewer title when Ctrl+O from recipe picker):

**`en.json`:**
```json
"recipe.preview": "Recipe Preview",
```

**`zh.json`:**
```json
"recipe.preview": "配方预览",
```

**`wen.json`:**
```json
"recipe.preview": "配方预览",
```

Note: There may already be a `recipe.preview` key — check and reuse if it exists, or add alongside existing `recipe.preview_greet` etc.

- [ ] **Step 2: Build**

Run: `cd tui && go build -o /dev/null .`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add tui/i18n/en.json tui/i18n/zh.json tui/i18n/wen.json
git commit -m "feat(i18n): add recipe.preview key for viewer title"
```

---

### Task 5: Integration test — full build and verify

**Files:**
- No new files.

- [ ] **Step 1: Run all tests**

Run: `cd tui && go test ./... -count=1`
Expected: All packages pass.

- [ ] **Step 2: Build binary**

Run: `cd tui && make build`
Expected: Binary built at `tui/bin/lingtai-tui`.

- [ ] **Step 3: Verify skills.go is slim**

Run: `wc -l tui/internal/tui/skills.go`
Expected: Significantly smaller than before (~140 lines down from ~364).

- [ ] **Step 4: Verify no old recipe preview references remain**

Run: `grep -n "recipePreview\b\|recipePreviewFile\|recipePreviewVP\|recipePreviewReady\|enterRecipePreview\|viewRecipePreview\|syncRecipePreviewContent\|renderRecipeFileContent\|renderRecipeSidePane\|recipeFilePreview\|recipeWidePaneThreshold" tui/internal/tui/firstrun.go`
Expected: No matches.

- [ ] **Step 5: Verify no old SkillsModel references remain**

Run: `grep -n "SkillsModel\|skillsLoadMsg\|skillsHeaderLines\|skillsFooterLines" tui/internal/tui/skills.go tui/internal/tui/app.go`
Expected: No matches.
