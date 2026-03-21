# Lingtai CLI Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `lingtai` a cwd-based tool (like `git`) that uses `.lingtai/` in the current directory, with named combos for reusable provider configs.

**Architecture:** `lingtai` checks cwd for `.lingtai/`. If absent, runs setup wizard (with combo selection). If present, starts the agent and opens chat TUI. Combos are stored at `~/.lingtai/combos/<name>.json`. Config files live at `.lingtai/configs/`, agent working dirs at `.lingtai/<agent_id>/`.

**Tech Stack:** Go (bubbletea TUI), Python (agent runtime)

**Spec:** `docs/superpowers/specs/2026-03-20-lingtai-cli-redesign.md`

---

## Pre-existing State

Several changes have already been made in this session:
- `daemon/internal/config/loader.go` — already updated: `BaseDir`→`ProjectDir`, `Covenant` removed, `Language` added
- `daemon/internal/setup/wizard.go` — already updated: base_dir/covenant fields removed from StepGeneral, writes to `configs/` subdir, covenant to agent working dir
- `daemon/internal/tui/app.go` — already updated: `BaseDir`→`ProjectDir`
- `daemon/main.go` — has subcommands that need to be replaced with the new flow
- `app/__init__.py` — already updated: loads covenant from agent working dir, propagates language
- `app/config.py` — already updated: derives base_dir from config path

The remaining work is:
1. Combo system (read/write/list)
2. Wizard combo selection step
3. Rewrite `main.go` to cwd-based flow
4. Verify end-to-end

---

### Task 1: Combo Read/Write/List

**Files:**
- Create: `daemon/internal/combo/combo.go`

- [ ] **Step 1: Create combo package**

```go
package combo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Combo is a named snapshot of provider/model/config settings.
type Combo struct {
	Name   string                 `json:"name"`
	Model  map[string]interface{} `json:"model"`
	Config map[string]interface{} `json:"config"`
	Env    map[string]string      `json:"env"`
}

// Dir returns the combos directory (~/.lingtai/combos/).
func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lingtai", "combos")
}

// List returns all saved combos, sorted by name.
func List() ([]Combo, error) {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var combos []Combo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var c Combo
		if json.Unmarshal(data, &c) == nil && c.Name != "" {
			combos = append(combos, c)
		}
	}
	sort.Slice(combos, func(i, j int) bool { return combos[i].Name < combos[j].Name })
	return combos, nil
}

// Save writes a combo to ~/.lingtai/combos/<name>.json with mode 0600.
func Save(c Combo) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create combos dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, c.Name+".json")
	return os.WriteFile(path, data, 0600)
}

// Load reads a combo by name.
func Load(name string) (*Combo, error) {
	path := filepath.Join(Dir(), name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Combo
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
```

- [ ] **Step 2: Verify Go builds**

Run: `cd daemon && go build ./...`
Expected: clean build

- [ ] **Step 3: Commit**

```bash
git add daemon/internal/combo/combo.go
git commit -m "feat(combo): add combo read/write/list for reusable provider configs"
```

---

### Task 2: Wizard Combo Selection Step

**Files:**
- Modify: `daemon/internal/setup/wizard.go`

The wizard currently starts at `StepLang`. Add a new `StepCombo` before it. If a combo is selected, pre-fill model/config fields and skip `StepLang`, `StepModel`, `StepMultimodal`. At the end, ask for a combo name and save.

- [ ] **Step 1: Add StepCombo constant**

In the `step` const block, add `StepCombo` before `StepLang`:

```go
const (
	StepCombo step = iota
	StepLang
	StepModel
	StepMultimodal
	StepMessaging
	StepGeneral
	StepReview
)
```

- [ ] **Step 2: Add combo fields to wizardModel**

Add to `wizardModel` struct:

```go
	combos       []combo.Combo  // loaded from ~/.lingtai/combos/
	comboIdx     int            // selected combo index (-1 = create new)
	comboName    textinput.Model // name input for saving combo
```

- [ ] **Step 3: Initialize combo step in newWizardModel**

In `newWizardModel()`, load combos and set up the selection:

```go
	// Step: Combo selection
	combos, _ := combo.List()
	m.combos = combos
	m.comboIdx = -1 // default to "Create new"
	m.comboName = textinput.New()
	m.comboName.Placeholder = "my-combo"
	m.comboName.CharLimit = 40
```

- [ ] **Step 4: Render StepCombo in View()**

Show list of combos with arrow selection + "Create new" option. Display provider/model next to each combo name.

- [ ] **Step 5: Handle StepCombo in Update()**

Up/down to navigate combos, Enter to select. If a combo is selected, pre-fill model/config fields from `combo.Model` and `combo.Config`, write env vars from `combo.Env` to `os.Setenv`, and advance to `StepGeneral` (skip lang/model/multimodal). If "Create new", advance to `StepLang`.

- [ ] **Step 6: Add combo name prompt to StepReview**

In the review view, add a text input: "Name this combo:" with the combo name field. If editing an existing combo, pre-fill with its name.

- [ ] **Step 7: Save combo in writeConfig()**

At the end of `writeConfig()`, build a `combo.Combo` from the wizard state and call `combo.Save()`. Include env vars (read from the .env content being written).

- [ ] **Step 8: Verify Go builds**

Run: `cd daemon && go build ./...`
Expected: clean build

- [ ] **Step 9: Commit**

```bash
git add daemon/internal/setup/wizard.go
git commit -m "feat(wizard): add combo selection step and save-on-complete"
```

---

### Task 3: Rewrite main.go — CWD-Based Flow

**Files:**
- Modify: `daemon/main.go`

Replace the current subcommand-based main.go with the cwd-based flow.

- [ ] **Step 1: Rewrite main.go**

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"lingtai-daemon/internal/i18n"
	"lingtai-daemon/internal/setup"
	"lingtai-daemon/internal/tui"
	"lingtai-daemon/internal/config"
)

func main() {
	args := os.Args[1:]

	// Parse flags
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--lang":
			if i+1 < len(args) {
				i18n.Lang = args[i+1]
				i++
			}
		default:
			positional = append(positional, args[i])
		}
	}

	// lingtai setup — (re)configure current directory
	if len(positional) > 0 && positional[0] == "setup" {
		cwd, _ := os.Getwd()
		lingtaiDir := filepath.Join(cwd, ".lingtai")
		os.MkdirAll(lingtaiDir, 0755)
		if err := setup.Run(lingtaiDir); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
			os.Exit(1)
		}
		return
	}

	if len(positional) > 0 {
		switch positional[0] {
		case "help", "--help", "-h":
			printHelp()
			return
		}
	}

	// Default: check cwd for .lingtai/
	cwd, _ := os.Getwd()
	lingtaiDir := filepath.Join(cwd, ".lingtai")
	configPath := filepath.Join(lingtaiDir, "configs", "config.json")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// No .lingtai/ — run setup wizard
		fmt.Printf("\n  \033[1m\033[36m灵台\033[0m  No .lingtai/ found — starting setup.\n\n")
		os.MkdirAll(lingtaiDir, 0755)
		if err := setup.Run(lingtaiDir); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
			os.Exit(1)
		}
		// After setup, fall through to start agent
	}

	// .lingtai/ exists — load config, start agent, open chat TUI
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mError loading config: %v\033[0m\n", err)
		os.Exit(1)
	}

	if err := tui.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf(`
  灵台 LingTai — agent framework

  Usage:
    lingtai              Start agent in current directory (setup if needed)
    lingtai setup        (Re)configure current directory

  Flags:
    --lang <code>        UI language (en, zh, lzh)

  Run lingtai in any directory. It uses .lingtai/ in the current
  directory — like git uses .git/.

  Provider configs are saved as "combos" at ~/.lingtai/combos/
  for reuse across projects.

`)
}
```

- [ ] **Step 2: Verify Go builds**

Run: `cd daemon && go build ./...`
Expected: clean build (may need to adjust tui.Run signature)

- [ ] **Step 3: Commit**

```bash
git add daemon/main.go
git commit -m "refactor(main): cwd-based flow — .lingtai/ in current directory"
```

---

### Task 4: Update TUI Entry Point

**Files:**
- Modify: `daemon/internal/tui/app.go`

The TUI's `Run` function (or equivalent) needs to accept a `*config.Config`, start the Python agent process, and open the chat view. Check the current entry point and adjust.

- [ ] **Step 1: Check current TUI entry point signature**

Read `daemon/internal/tui/app.go` and find the `Run` or `New` function. Ensure it accepts `*config.Config` and handles agent startup internally.

- [ ] **Step 2: Ensure TUI starts agent process**

The TUI should:
1. Start Python agent via `agent.Start()`
2. Connect mail client
3. Show chat view
4. On quit, stop agent process

- [ ] **Step 3: Verify Go builds and test manually**

Run: `cd daemon && go build ./...`

- [ ] **Step 4: Commit**

```bash
git add daemon/internal/tui/app.go
git commit -m "refactor(tui): accept config, start agent process on launch"
```

---

### Task 5: Update Config Loader for CWD Layout

**Files:**
- Modify: `daemon/internal/config/loader.go`

The `ProjectDir` derivation needs to account for the new layout: `.lingtai/configs/config.json` → `.lingtai/` is the project dir (base_dir for agents).

- [ ] **Step 1: Verify ProjectDir derivation is correct**

Currently: `projectDir = filepath.Dir(configDir)` where `configDir = filepath.Dir(absPath)`.
For path `.lingtai/configs/config.json`: `configDir = .lingtai/configs/`, `projectDir = .lingtai/`.
This is correct — `.lingtai/` is where agent working dirs live.

- [ ] **Step 2: Update WorkingDir() comment**

```go
// WorkingDir returns the agent's working directory: {.lingtai}/{agent_name}
func (c *Config) WorkingDir() string {
	return filepath.Join(c.ProjectDir, c.AgentName)
}
```

- [ ] **Step 3: Commit if changes made**

---

### Task 6: Update Python App for CWD Layout

**Files:**
- Modify: `app/config.py`
- Modify: `app/__init__.py`

- [ ] **Step 1: Verify config.py derives base_dir correctly**

Already done: `project_dir = config_dir.parent` where config_dir is `.lingtai/configs/`. So `project_dir = .lingtai/`. Agent working dirs will be `.lingtai/<agent_name>/`.

- [ ] **Step 2: Verify app/__init__.py covenant loading**

Already done: reads from `base_dir / agent_name / "covenant.md"` = `.lingtai/<agent_name>/covenant.md`.

- [ ] **Step 3: Verify language propagation**

Already done: `AgentConfig(language=cfg.get("language", "en"))`.

- [ ] **Step 4: Smoke-test**

Run: `cd /path/to/lingtai && source venv/bin/activate && python -c "import lingtai"`
Expected: clean

- [ ] **Step 5: Commit if changes made**

---

### Task 7: Clean Up Old Files

**Files:**
- Modify: `docs/superpowers/plans/2026-03-20-project-structure.md` (mark superseded)

- [ ] **Step 1: Mark old plan as superseded**

Add note at top: "Superseded by `2026-03-20-lingtai-cli-redesign.md`"

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/plans/2026-03-20-project-structure.md
git commit -m "docs: mark old project-structure plan as superseded"
```

---

### Task 8: End-to-End Verification

- [ ] **Step 1: Build Go daemon**

Run: `cd daemon && go build -o lingtai .`

- [ ] **Step 2: Run Go tests**

Run: `cd daemon && go test ./internal/config/ ./internal/setup/ ./internal/combo/`

- [ ] **Step 3: Run Python tests**

Run: `source venv/bin/activate && python -m pytest tests/test_layers_avatar.py -v`

- [ ] **Step 4: Manual test — new project**

```bash
mkdir /tmp/test-project && cd /tmp/test-project
/path/to/daemon/lingtai
# Should show "No .lingtai/ found — starting setup."
# Wizard should show combo selection (empty list + "Create new")
# Complete wizard
# Should create .lingtai/configs/ and .lingtai/<agent_name>/covenant.md
# Should save combo to ~/.lingtai/combos/
# Should launch chat TUI
```

- [ ] **Step 5: Manual test — existing project**

```bash
cd /tmp/test-project
/path/to/daemon/lingtai
# Should skip setup, load config, launch chat TUI
```

- [ ] **Step 6: Manual test — combo reuse**

```bash
mkdir /tmp/test-project-2 && cd /tmp/test-project-2
/path/to/daemon/lingtai
# Wizard should show the combo saved in step 4
# Select it — should skip model config steps
```
