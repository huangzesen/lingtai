package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/globalmigrate"
	"github.com/anthropics/lingtai-tui/internal/headless"
	"github.com/anthropics/lingtai-tui/internal/preset"
	"github.com/anthropics/lingtai-tui/internal/process"
	"github.com/anthropics/lingtai-tui/internal/tui"
)

// version is set at build time via -ldflags "-X main.version=v0.4.2"
var version = "dev"

func isSubcommandHelpArg(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

func dispatchSelfUpdate(args []string, showHelp func(), runSelfUpdate func()) {
	if len(args) > 0 && isSubcommandHelpArg(args[0]) {
		showHelp()
		return
	}
	runSelfUpdate()
}

func dispatchDoctor(args []string, showHelp func(), runDoctor func()) {
	if len(args) > 0 && isSubcommandHelpArg(args[0]) {
		showHelp()
		return
	}
	runDoctor()
}

func printSubcommandHelp() {
	printWelcomeInfo()
	fmt.Println()
	printHelp()
}

func main() {
	// Handle flags
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "--help" || arg == "-h" {
			printWelcomeInfo()
			fmt.Println()
			printHelp()
			os.Exit(0)
		}
		if arg == "--version" || arg == "-v" || arg == "version" {
			fmt.Println("lingtai-tui " + version)
			os.Exit(0)
		}
		if arg == "purge" {
			purgeMain()
			return
		}
		if arg == "list" {
			listMain()
			return
		}
		if arg == "clean" {
			cleanMain()
			return
		}
		if arg == "suspend" {
			suspendMain()
			return
		}
		if arg == "bootstrap" {
			bootstrapMain()
			return
		}
		if arg == "presets" {
			// Revision is an explicit, read-only-by-default path. Keep it
			// before presetsMain so it cannot resolve global state or call
			// preset.Bootstrap before the manifest/input adapter runs.
			if len(os.Args) > 2 && os.Args[2] == "revise" {
				os.Exit(headless.RunPresetRevision(os.Args[3:], os.Stdout, os.Stderr))
			}
			presetsMain()
			return
		}
		if arg == "spawn" {
			spawnMain()
			return
		}
		if arg == "self-update" {
			dispatchSelfUpdate(os.Args[2:], printSubcommandHelp, selfUpdateMain)
			return
		}
		if arg == "doctor" {
			dispatchDoctor(os.Args[2:], printSubcommandHelp, doctorMain)
			return
		}
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'lingtai-tui --help' for usage.\n", arg)
		os.Exit(1)
	}

	// Record the running binary version so /doctor can report it and
	// detect TUI↔kernel version drift. Pure in-memory — safe before the
	// no-project gate.
	tui.SetTUIVersion(version)

	// Always start in current directory. Resolved with plain os.Getwd +
	// filepath.Abs only — no filesystem mutation.
	projectDir, _ := os.Getwd()
	projectDir, _ = filepath.Abs(projectDir)

	// ---- Shared startup-update preflight (TUI + kernel, read-only) ----
	//
	// Runs ONCE, before any Bubble Tea program starts and before the
	// no-project decision gate below, so every default interactive launch —
	// existing-project returning user, first-run/setup, and the empty-
	// directory no-project launcher — is covered by the identical read-only
	// TUI (config.CheckTUIUpgrade) and kernel (config.InspectKernel) version
	// checks. install.sh installs the kernel by default, so first-run is not
	// a reason to skip the kernel check (Jason, 2026-07-16). Because this
	// runs before ANY Bubble Tea program exists, the kernel-upgrade prompt
	// (if InspectKernel reports an actionable update AND stdin is a real
	// TTY) can safely block on stdin directly — no inProgram/fallback dance
	// is needed here the way prepareApp needs it for prompts that might run
	// inside the launcher's own program. A completed TUI self-upgrade always
	// tells the user to restart and exits main() here, before the no-project
	// gate or launcher ever starts. Uses config.GlobalDirPath() (pure, no
	// mkdir) for the read-only check phase; a consented mutation (TUI
	// self-upgrade or RunKernelUpdate) creates ~/.lingtai-tui itself as an
	// intentional side effect of that mutation, not of merely checking.
	if runVersionPreflight() {
		return
	}

	// ---- No-project decision gate (design doc: "no-project-launcher") ----
	//
	// Before ANY of: config.GlobalDir() (which os.MkdirAll's
	// ~/.lingtai-tui), globalmigrate.Run, MigrateLegacyLanguage/
	// LoadTUIConfig's write paths, ValidateCodexAuthOnStartup, showWelcome,
	// maybeShowAgentCount, process.InitProject, config.Register,
	// preset.PopulateBundledLibrary, or config.RuntimeReady/preset.Bootstrap —
	// do a PURE, read-only check: does <projectDir>/.lingtai exist? Uses
	// os.Lstat (never Stat) so a symlink there counts as "exists" without
	// being followed or created through.
	//
	// If it's missing, branch into the no-project launcher, which itself
	// performs zero filesystem writes until the user explicitly confirms
	// either "Open Existing" (hands a chosen project root to the SAME
	// existing pipeline below, exactly as if the user had cd'd there) or
	// "Start project" (runs the atomic staging→validate→rename commit in
	// project_create.go, then falls through to construct the real App).
	//
	// ProbeNoProjectPure now fails CLOSED: any Lstat error other than
	// "absent" (permission denied on a parent dir, I/O error, ...) is
	// surfaced and exits here, before config.GlobalDir()/any write below.
	// The previous version folded every such error into "project exists",
	// which silently fell through to the normal (write-heavy) startup path
	// without the launcher ever making a real decision — exactly the
	// eager-write bug this gate exists to prevent.
	noProject, probeErr := tui.ProbeNoProjectPure(projectDir)
	if probeErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", probeErr)
		os.Exit(1)
	}
	if noProject {
		result, ok, startup := runNoProjectLauncher(projectDir)
		if !ok {
			if startup.kind == startupFatal {
				fmt.Fprintln(os.Stderr, startup.err)
				os.Exit(1)
			}
			return
		}
		if result.Kind == tui.DecisionCreate {
			runCreatedProject(result)
			return
		}
		if startup.kind == startupFallback {
			runPreparedApp(startup.projectDir)
			return
		}
		if startup.kind == startupUpgradeExit || startup.kind == startupCanceled {
			return
		}
		if startup.kind == startupFatal {
			fmt.Fprintln(os.Stderr, startup.err)
			os.Exit(1)
		}
		// Open Existing and a successfully created project already run (and
		// exit) inside the launcher's Bubble Tea program; no second program starts.
		return
	}

	runPreparedApp(projectDir)

}

// startupReadyMsg carries the fully prepared App back to the single
// launcher/handoff Bubble Tea program.
type startupKind uint8

const (
	startupCanceled startupKind = iota
	startupReady
	startupFallback
	startupUpgradeExit
	startupFatal
)

type startupResult struct {
	kind       startupKind
	projectDir string
	app        tui.App
	err        error
}

type startupReadyMsg struct{ result startupResult }

type noProjectProgramModel struct {
	launcher        tui.LauncherRootModel
	app             tui.App
	appReady        bool
	loading         bool
	cancelRequested bool
	startup         startupResult
	width           int
	height          int
}

func launcherHandoffProject(result tui.LauncherResult) (string, bool) {
	switch result.Kind {
	case tui.DecisionOpenExisting, tui.DecisionCreate:
		return result.ProjectRoot, true
	default:
		return "", false
	}
}

func (m noProjectProgramModel) Init() tea.Cmd { return m.launcher.Init() }

func (m noProjectProgramModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Window size belongs to the root program across Launcher -> Loading -> App.
	// Bubble Tea does not resend it merely because this model changes phase.
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
	}
	if m.loading {
		switch msg := msg.(type) {
		case startupReadyMsg:
			if m.cancelRequested {
				m.loading = false
				m.startup = startupResult{kind: startupCanceled, projectDir: msg.result.projectDir}
				return m, tea.Quit
			}
			m.loading = false
			m.startup = msg.result
			if msg.result.kind != startupReady {
				return m, tea.Quit
			}
			m.app = msg.result.app
			m.appReady = true
			updated, sizeCmd := m.app.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.app = updated.(tui.App)
			return m, tea.Batch(sizeCmd, m.app.Init())
		case tea.KeyPressMsg:
			if msg.String() == "ctrl+c" {
				m.cancelRequested = true
				return m, nil
			}
		}
		return m, nil
	}

	if m.appReady {
		updated, cmd := m.app.Update(msg)
		m.app = updated.(tui.App)
		return m, cmd
	}

	updated, cmd := m.launcher.Update(msg)
	m.launcher = updated.(tui.LauncherRootModel)
	if m.launcher.Done() {
		result := m.launcher.Result()
		if projectRoot, ok := launcherHandoffProject(result); ok {
			m.loading = true
			return m, func() tea.Msg {
				startup := prepareApp(projectRoot, true)
				startup.projectDir = projectRoot
				return startupReadyMsg{result: startup}
			}
		}
	}
	return m, cmd
}

func (m noProjectProgramModel) View() tea.View {
	if m.appReady {
		return m.app.View()
	}
	if m.loading {
		v := tea.NewView(tui.StartupLoadingView(m.width, m.height))
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		tui.ApplyThemeToView(&v)
		v.ReportFocus = true
		return v
	}
	return m.launcher.View()
}

// runNoProjectLauncher keeps Open Existing or a successful Create, the
// canonical loading screen, and the real App in one Bubble Tea program. This
// prevents the first program's alternate-screen teardown from exposing the shell
// during startup.
// It performs a pure, non-mutating global-dir path resolution
// (config.GlobalDirPath, never config.GlobalDir/EnsureGlobalDir) so
// launching the picker itself cannot create ~/.lingtai-tui.
//
// lingtaiCmd is resolved via a best-effort PURE read of whatever venv might
// already exist (config.LingtaiCmd only stats a path, it does not create
// anything). An empty result is intentionally allowed: after a successful
// atomic project publication, project_create.go checks runtime readiness
// (config.RuntimeReady — read-only, never installs/repairs), resolves the
// command again, and only then bootstraps/launches. Those post-commit steps
// are best-effort and never roll back project creation.
//
// Returns (result, true) when the launcher reached a terminal decision, or
// (zero, false) when the Bubble Tea program itself failed. The third return
// preserves that failure as startupFatal; only an actual user cancellation is
// a clean zero-write return.
func runNoProjectLauncher(projectDir string) (tui.LauncherResult, bool, startupResult) {
	globalDirPath, err := config.GlobalDirPath()
	if err != nil {
		return tui.LauncherResult{Kind: tui.DecisionCancel}, false, startupResult{kind: startupFatal, err: err}
	}
	lingtaiCmd := config.LingtaiCmd(globalDirPath)
	model := noProjectProgramModel{launcher: tui.NewLauncherRootModel(projectDir, globalDirPath, lingtaiCmd)}
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return tui.LauncherResult{Kind: tui.DecisionCancel}, false, startupResult{kind: startupFatal, err: err}
	}
	pm, ok := finalModel.(noProjectProgramModel)
	if !ok {
		return tui.LauncherResult{Kind: tui.DecisionCancel}, false, startupResult{kind: startupFatal, err: errors.New("launcher returned an unexpected model")}
	}
	if pm.appReady {
		return tui.LauncherResult{Kind: tui.DecisionOpenExisting}, true, pm.startup
	}
	if pm.startup.kind != startupReady {
		return tui.LauncherResult{Kind: tui.DecisionOpenExisting, ProjectRoot: pm.startup.projectDir}, true, pm.startup
	}
	if !pm.launcher.Done() {
		return tui.LauncherResult{Kind: tui.DecisionCancel}, false, startupResult{kind: startupCanceled}
	}
	return pm.launcher.Result(), true, startupResult{kind: startupReady}
}

// runCreatedProject handles the DecisionCreate outcome: RunProjectCreate
// already ran (inside the launcher's own program, on ProjectDraftConfirmedMsg)
// and — since main.go only reaches this function when result.Kind ==
// DecisionCreate — successfully committed the rename. This function's job
// is exactly what the normal returning-user path already does once a
// project exists: report post-commit warnings honestly (never silently
// swallow a "created, but ..." state), then construct and run the real App
// for the newly created project.
func runCreatedProject(result tui.LauncherResult) {
	globalDir, err := config.GlobalDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	// Resolve the configured locale BEFORE printing anything — this must
	// happen before the post-commit warning block below, mirroring how the
	// normal (non-launcher) startup path resolves i18n.SetLang before its
	// own first user-visible print (showWelcome/maybeShowAgentCount). A
	// parent review found this function's i18n.SetLang call used to run
	// AFTER two hardcoded-English fmt.Println/Printf calls — moving it
	// earlier is necessary (not just routing the strings through i18n.T)
	// because calling i18n.T before SetLang runs would still render in
	// whatever the DEFAULT locale is, not the user's configured one.
	tuiCfg := config.LoadTUIConfig(globalDir)
	if err := i18n.SetLang(tuiCfg.Language); err != nil {
		tuiCfg.Language = i18n.Lang()
	}
	if result.CreateResult != nil && len(result.CreateResult.PostCommitWarnings) > 0 {
		fmt.Println(i18n.T("launcher.postcommit.incomplete_header"))
		for _, w := range result.CreateResult.PostCommitWarnings {
			fmt.Printf("  - %s\n", w)
		}
		fmt.Println(i18n.T("launcher.postcommit.retryable_notice"))
	}

	lingtaiDir := filepath.Join(result.ProjectRoot, ".lingtai")
	orchestrators := tui.DetectOrchestrators(lingtaiDir)
	needsFirstRun := len(orchestrators) == 0
	app := tui.NewApp(globalDir, lingtaiDir, needsFirstRun, false, false, orchestrators, tuiCfg, "", "")
	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// isAgentDir returns true if entryName under lingtaiDir is a real agent
// directory (has .agent.json AND .agent.json's admin field is not nil).
//
// The human/ placeholder directory has .agent.json with "admin": null,
// which distinguishes it from all real agents (who have admin as a map,
// possibly empty). This is the canonical rule used by both invariant
// checks to avoid counting human as an agent.
//
// Returns (isAgent bool, manifest map, err error). manifest is the parsed
// .agent.json body (useful to callers that need to read other fields like
// the admin flags for orchestrator detection). If the file is unreadable
// or unparseable, returns (false, nil, nil) — not an agent.
func isAgentDir(lingtaiDir, entryName string) (bool, map[string]interface{}, error) {
	manifestPath := filepath.Join(lingtaiDir, entryName, ".agent.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return false, nil, nil
	}
	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return false, nil, nil
	}
	// admin == nil (missing or explicit null) means this is the human
	// placeholder, not an agent.
	adminRaw, hasAdmin := manifest["admin"]
	if !hasAdmin || adminRaw == nil {
		return false, manifest, nil
	}
	return true, manifest, nil
}

// checkInitJSONInvariant enforces the all-or-nothing rule for per-agent
// init.json files. A healthy network is one of:
//
//   - every agent has init.json (normal running state), or
//   - no agent has init.json (cloned network awaiting rehydration; the
//     rehydration path runs the first-run wizard with agent names pre-
//     filled from each .agent.json), or
//   - no agents exist at all (checkOrchestratorInvariant will catch this).
//
// Only mixed state (some agents with init.json, some without) is corrupt.
//
// Returns (needsRehydration, error). needsRehydration is true when at
// least one agent exists and every agent is missing init.json — the
// caller (main.go) routes into the rehydration wizard in that case.
//
// Dot-prefixed directories under .lingtai/ (.library/, .portal/, .addons/,
// .tui-asset/) are helper dirs and are skipped. The human/ placeholder
// (which has .agent.json but with admin: null) is also skipped via
// isAgentDir — it's not an agent, so it doesn't need init.json.
func checkInitJSONInvariant(lingtaiDir string) (needsRehydration bool, err error) {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return false, nil // missing .lingtai/ is handled elsewhere
	}
	var withInit, withoutInit []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		isAgent, _, err := isAgentDir(lingtaiDir, entry.Name())
		if err != nil {
			return false, err
		}
		if !isAgent {
			continue
		}
		agentDir := filepath.Join(lingtaiDir, entry.Name())
		initPath := filepath.Join(agentDir, "init.json")
		if _, err := os.Stat(initPath); err == nil {
			withInit = append(withInit, entry.Name())
		} else if os.IsNotExist(err) {
			withoutInit = append(withoutInit, entry.Name())
		} else {
			return false, fmt.Errorf("sanity check: cannot stat %s: %w", initPath, err)
		}
	}

	// Mixed state is the only failure mode. All-present and all-absent
	// are both legitimate; the caller figures out which one.
	if len(withInit) > 0 && len(withoutInit) > 0 {
		var msg strings.Builder
		msg.WriteString("\nerror: corrupted network — init.json is present in some agents but missing in others\n\n")
		msg.WriteString(fmt.Sprintf("  with init.json (%d):\n", len(withInit)))
		for _, n := range withInit {
			msg.WriteString(fmt.Sprintf("    %s\n", n))
		}
		msg.WriteString(fmt.Sprintf("\n  missing init.json (%d):\n", len(withoutInit)))
		for _, n := range withoutInit {
			msg.WriteString(fmt.Sprintf("    %s\n", n))
		}
		msg.WriteString("\nA healthy network has init.json in either every agent or none.\n")
		msg.WriteString("Mixed state usually means an interrupted rehydration, a partial\n")
		msg.WriteString("publish, or manual tampering.\n")
		msg.WriteString("\nTo recover, run:  lingtai-tui clean\n")
		msg.WriteString("This suspends any running agents and removes .lingtai/ so you can start over.\n")
		return false, fmt.Errorf("%s", msg.String())
	}
	// All-absent with at least one agent: rehydration needed.
	if len(withInit) == 0 && len(withoutInit) > 0 {
		return true, nil
	}
	return false, nil
}

// checkOrchestratorInvariant enforces "exactly one orchestrator per network".
//
// A healthy network has exactly one agent whose .agent.json declares at least
// one truthy admin flag (the same definition tui.IsOrchestrator uses). Any
// other count is corruption:
//
//   - zero agents in .lingtai/             → empty network, no root will
//   - agents present but zero orchestrators → headless network
//   - two or more orchestrators            → competing wills
//
// All three cases refuse to launch. The error message points the user at
// `lingtai-tui clean` for recovery, which suspends running agents and
// removes .lingtai/ so they can re-run the first-run wizard cleanly.
//
// Dot-prefixed directories under .lingtai/ are helper dirs and are skipped,
// matching checkInitJSONInvariant.
func checkOrchestratorInvariant(lingtaiDir string) error {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return nil // missing .lingtai/ is handled elsewhere
	}
	var allAgents, orchestrators []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		isAgent, manifest, err := isAgentDir(lingtaiDir, entry.Name())
		if err != nil {
			return err
		}
		if !isAgent {
			continue // not an agent (no .agent.json, or human placeholder)
		}
		allAgents = append(allAgents, entry.Name())
		if tui.IsOrchestrator(manifest) {
			orchestrators = append(orchestrators, entry.Name())
		}
	}

	// Zero agents: corrupt under strict rules. A complete network must
	// have at least one orchestrator. An empty .lingtai/ means something
	// created the directory without finishing setup (most commonly: the
	// user cancelled the first-run wizard mid-flow).
	if len(allAgents) == 0 {
		var msg strings.Builder
		msg.WriteString("\nerror: corrupted network — .lingtai/ exists but contains no agents\n\n")
		msg.WriteString("A complete network must have at least one orchestrator agent. An empty\n")
		msg.WriteString(".lingtai/ usually means the first-run wizard was cancelled mid-flow,\n")
		msg.WriteString("leaving behind a partially-created directory.\n")
		msg.WriteString("\nTo recover, run:  lingtai-tui clean\n")
		msg.WriteString("Then re-run lingtai-tui to start the first-run wizard from scratch.\n")
		return fmt.Errorf("%s", msg.String())
	}

	// Zero orchestrators among existing agents: headless network.
	if len(orchestrators) == 0 {
		var msg strings.Builder
		msg.WriteString("\nerror: corrupted network — no orchestrator found\n\n")
		msg.WriteString(fmt.Sprintf("Found %d agent(s), but none has admin privileges:\n", len(allAgents)))
		for _, n := range allAgents {
			msg.WriteString(fmt.Sprintf("    %s\n", n))
		}
		msg.WriteString("\nEvery network must have exactly one orchestrator — an agent whose\n")
		msg.WriteString(".agent.json contains an `admin` field with at least one truthy value\n")
		msg.WriteString("(e.g. `\"admin\": {\"karma\": true}`). Without an orchestrator, there is\n")
		msg.WriteString("no root will to launch.\n")
		msg.WriteString("\nTo recover, run:  lingtai-tui clean\n")
		msg.WriteString("Then re-run lingtai-tui to start the first-run wizard from scratch.\n")
		return fmt.Errorf("%s", msg.String())
	}

	// Two or more orchestrators: competing wills.
	if len(orchestrators) > 1 {
		var msg strings.Builder
		msg.WriteString("\nerror: corrupted network — multiple orchestrators found\n\n")
		msg.WriteString(fmt.Sprintf("Found %d orchestrator agents (a network must have exactly one):\n", len(orchestrators)))
		for _, n := range orchestrators {
			msg.WriteString(fmt.Sprintf("    %s\n", n))
		}
		msg.WriteString("\nA network has exactly one root will. Multiple orchestrators usually\n")
		msg.WriteString("mean two networks were merged, or someone manually edited an agent's\n")
		msg.WriteString(".agent.json to add an admin flag.\n")
		msg.WriteString("\nTo recover, run:  lingtai-tui clean\n")
		msg.WriteString("Then re-run lingtai-tui to start the first-run wizard from scratch.\n")
		msg.WriteString("\nIf you want to keep the existing agents, edit each non-orchestrator's\n")
		msg.WriteString(".agent.json to set `\"admin\": {}` (empty map) before re-running.\n")
		return fmt.Errorf("%s", msg.String())
	}

	// Exactly one orchestrator: healthy.
	return nil
}

// findOrchestratorBlueprint returns the (dirName, agentName) of the single
// orchestrator in .lingtai/. Assumes checkOrchestratorInvariant has already
// passed (so exactly one orchestrator exists). Returns empty strings if no
// orchestrator is found.
//
// dirName is the filesystem directory name (what the dir is called on disk).
// agentName is the value of the .agent.json's agent_name field (may differ
// from dirName if the user renamed the agent via the wizard).
func findOrchestratorBlueprint(lingtaiDir string) (dirName, agentName string) {
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		return "", ""
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		isAgent, manifest, err := isAgentDir(lingtaiDir, entry.Name())
		if err != nil || !isAgent {
			continue
		}
		if !tui.IsOrchestrator(manifest) {
			continue
		}
		dirName = entry.Name()
		if name, ok := manifest["agent_name"].(string); ok && name != "" {
			agentName = name
		} else {
			agentName = dirName
		}
		return dirName, agentName
	}
	return "", ""
}

func printHelp() {
	fmt.Println("Usage: lingtai-tui")
	fmt.Println("       lingtai-tui purge [dir]")
	fmt.Println("       lingtai-tui list [--detailed|-d] [--admin] [dir]")
	fmt.Println("       lingtai-tui suspend [dir]")
	fmt.Println("       lingtai-tui clean [--force]")
	fmt.Println("       lingtai-tui bootstrap")
	fmt.Println("       lingtai-tui presets [--saved-only] [--templates-only]")
	fmt.Println("       lingtai-tui presets revise --manifest PATH --input PATH --mode dry-run|check|apply [--output-dir PATH]")
	fmt.Println("       lingtai-tui spawn <dir> --preset <name> [--agent-name <name>] [--language <code>]")
	fmt.Println("       lingtai-tui self-update")
	fmt.Println("       lingtai-tui doctor")
	fmt.Println()
	fmt.Println("  (no args)    Launch TUI in current directory")
	fmt.Println("  purge        Kill all lingtai agent processes on this machine.")
	fmt.Println("               Agents are autonomous — they keep running after you")
	fmt.Println("               exit the TUI. Use purge when you need them all dead.")
	fmt.Println("  list         Show running agents as a decentralized contact-book view.")
	fmt.Println("               Marks main agents; --detailed adds names/state/path; --admin adds admin flags.")
	fmt.Println("  suspend      Gracefully suspend agents via signal files (all, or those in <dir>)")
	fmt.Println("  clean        Suspend agents in current directory, then remove .lingtai/.")
	fmt.Println("               Refuses to delete while agents are still alive; --force overrides.")
	fmt.Println("  bootstrap       Re-extract embedded skills to ~/.lingtai-tui/utilities/")
	fmt.Println("  presets      List available presets as JSON (for agent consumption)")
	fmt.Println("  presets revise  Plan/check/apply an explicit deterministic preset revision")
	fmt.Println("  spawn        Create a new project and launch an agent headlessly (JSON output)")
	fmt.Println("  self-update  Run the TUI binary updater for the detected install method")
	fmt.Println("  doctor       Force-check + update TUI/kernel/venv. Use when the TUI cannot start.")
	fmt.Println()
	fmt.Println("  You are responsible for all .lingtai/ folders on this machine.")
	fmt.Println("  They are the bodies of your agents — files, pad, mail, identity.")
	fmt.Println("  Always purge or suspend before deleting them.")
	fmt.Println()
	home, _ := os.UserHomeDir()
	globalDir := filepath.Join(home, ".lingtai-tui")
	fmt.Printf("  Global config: %s\n", globalDir)
	cwd, _ := os.Getwd()
	localDir := filepath.Join(cwd, ".lingtai")
	if _, err := os.Stat(localDir); err == nil {
		fmt.Printf("  Working dir:   %s\n", localDir)
	} else {
		fmt.Printf("  Working dir:   (no .lingtai/ in %s)\n", cwd)
	}
}

func maybePromptRustToolchain(globalDir string) {
	if os.Getenv("LINGTAI_SKIP_RUST_PROMPT") == "1" {
		return
	}
	if info, err := os.Stdin.Stat(); err != nil || (info.Mode()&os.ModeCharDevice) == 0 {
		return
	}

	promptPath := filepath.Join(globalDir, "runtime", "rust-toolchain-prompted")
	if _, err := os.Stat(promptPath); err == nil {
		return
	}

	status, err := config.FileSearchNativeStatus(globalDir, nil)
	if err != nil {
		// The probe failed (slow/broken/old runtime). Mark the prompt seen so
		// we don't re-spawn the Python probe on every startup forever.
		markRustPromptSeen(promptPath, "probe-error\n")
		return
	}
	if status.Unsupported {
		// Installed runtime predates the Rust sidecar diagnostics. Nothing the
		// user can act on here, and the probe will keep failing until they
		// upgrade lingtai — so record it once and stop prompting.
		markRustPromptSeen(promptPath, "unsupported-runtime\n")
		return
	}
	if status.SidecarPath != "" || status.Backend == "RustFileIOBackend" {
		return
	}
	if cargo, err := exec.LookPath("cargo"); err == nil && cargo != "" {
		return
	}

	fmt.Println()
	fmt.Println("LingTai is using the pure-Python file search fallback; Rust/Cargo is not installed.")
	fmt.Println("Rust is optional, but installing it lets source installs build the accelerated glob/grep sidecar.")
	fmt.Print("Install Rust now via rustup.rs? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		markRustPromptSeen(promptPath, "declined\n")
		return
	}

	if runtime.GOOS == "windows" {
		fmt.Println("Please install Rust from https://rustup.rs, then reinstall/upgrade the LingTai Python runtime if you need the native sidecar.")
		markRustPromptSeen(promptPath, "manual-windows\n")
		return
	}

	installCmd := "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --profile minimal"
	fmt.Printf("Running: %s\n", installCmd)
	cmd := exec.Command("sh", "-c", installCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Rust installer failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "You can install manually from https://rustup.rs and then reinstall/upgrade the LingTai Python runtime.")
		return
	}
	fmt.Println("Rust installed. Open a new shell if cargo is not on PATH yet; reinstall/upgrade the LingTai Python runtime to rebuild the native sidecar if this install currently falls back to Python.")
	markRustPromptSeen(promptPath, "installed\n")
}

func markRustPromptSeen(path, content string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(content), 0o644)
}

func printWelcomeInfo() {
	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("  ║               Welcome to 灵台 LingTai Agent                 ║")
	fmt.Println("  ╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("  LingTai agents are autonomous digital beings. They have a")
	fmt.Println("  heartbeat, a lifecycle, and they keep running after you exit")
	fmt.Println("  this TUI. You talk to them via async email — not direct chat.")
	fmt.Println()
	fmt.Println("  Important:")
	fmt.Println("    • Exiting the TUI does NOT stop agents — use /suspend all first")
	fmt.Println("    • Agent files live in .lingtai/ — deleting it without stopping")
	fmt.Println("      agents creates phantoms. Use lingtai-tui purge to clean up")
	fmt.Println("    • Agents act on their own after idle only when soul flow is opted in")
}

// agentCheckInterval is how often maybeShowAgentCount re-scans for running
// agents on TUI startup.
const agentCheckInterval = 4 * time.Hour

// maybeShowAgentCount prints a one-line reminder of how many `lingtai run`
// processes are currently alive on this machine, but only if the marker
// file at ~/.lingtai-tui/.last_agent_check is missing or older than
// agentCheckInterval. After any scan the marker's mtime is refreshed so
// the next check is suppressed until another interval has passed.
//
// When any agents are found, the user must press Enter to continue — this
// is the whole point of the reminder: agents keep running after the TUI
// exits, so it's worth making sure the human sees the count before diving
// back into the interface.
func maybeShowAgentCount(globalDir string) {
	n, scanned := scanAgentCount(globalDir, time.Now(), os.Stat, countRunningAgents,
		os.MkdirAll, os.WriteFile, os.Chtimes)
	if !scanned {
		return
	}

	if n == 0 {
		return
	}

	fmt.Printf("%d agent(s) running. Use 'lingtai-tui list' to see.\n", n)
	fmt.Print("Press Enter to continue...")
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')
}

// showWelcome displays a one-time welcome page for first-time users.
// Writes .firstrun sentinel to globalDir after confirmation.
func showWelcome(globalDir string) {
	sentinel := filepath.Join(globalDir, ".firstrun")
	if _, err := os.Stat(sentinel); err == nil {
		return // already seen
	}

	os.MkdirAll(globalDir, 0o755)

	printWelcomeInfo()
	fmt.Println()
	printHelp()
	fmt.Println()
	fmt.Println("  Run lingtai-tui --help to see this info again.")
	fmt.Println()

	fmt.Print("  Press Enter to continue...")
	reader := bufio.NewReader(os.Stdin)
	reader.ReadString('\n')

	os.WriteFile(sentinel, []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644)
}

func cleanMain() {
	force := false
	for _, arg := range os.Args[2:] {
		switch arg {
		case "--force", "-f":
			force = true
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag for clean: %s\n", arg)
			os.Exit(1)
		}
	}

	projectDir, _ := os.Getwd()
	projectDir, _ = filepath.Abs(projectDir)
	lingtaiDir := filepath.Join(projectDir, ".lingtai")

	if _, err := os.Stat(lingtaiDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "No .lingtai/ found in %s\n", projectDir)
		os.Exit(1)
	}

	// Count agents
	agents, _ := fs.DiscoverAgents(lingtaiDir)
	agentCount := 0
	for _, agent := range agents {
		if !agent.IsHuman {
			agentCount++
		}
	}

	// Confirm
	if agentCount > 0 {
		fmt.Printf("This will suspend %d agent(s) and remove %s\n", agentCount, lingtaiDir)
	} else {
		fmt.Printf("This will remove %s\n", lingtaiDir)
	}
	fmt.Print("Proceed? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted.")
		return
	}

	if err := cleanProject(lingtaiDir, force, 10*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Removed %s\n", lingtaiDir)
	fmt.Println()
	fmt.Println("To also remove global config, run:")
	fmt.Println("  rm -rf ~/.lingtai-tui")
}

// cleanProject signals every agent under lingtaiDir to suspend, waits up to
// waitTimeout for their heartbeats to go stale, and removes lingtaiDir.
// If any agent is still alive after the timeout — including agents whose
// .suspend signal could not be written — nothing is removed and the
// survivors are listed in the returned error: deleting a live agent's
// working directory strands the process and destroys state it is still
// writing (issue #488). force skips the survivor guard, not the suspend
// attempt; it also overrides a failed agent discovery, which otherwise
// refuses to remove anything because live agents could be hiding in an
// unlistable directory.
func cleanProject(lingtaiDir string, force bool, waitTimeout time.Duration) error {
	agents, err := fs.DiscoverAgents(lingtaiDir)
	if err != nil {
		if !force {
			return fmt.Errorf("Cannot list agents under %s: %v\nNot removing it: a live agent could be hiding in there. Fix the error, or re-run with --force to delete anyway.", lingtaiDir, err)
		}
		fmt.Fprintf(os.Stderr, "Warning: cannot list agents under %s (%v); removing anyway (--force)\n", lingtaiDir, err)
	}

	// Signal all agents at once (touch .suspend in every folder)
	var alive []string
	for _, agent := range agents {
		if agent.IsHuman {
			continue
		}
		suspendFile := filepath.Join(agent.WorkingDir, ".suspend")
		if err := os.WriteFile(suspendFile, []byte(""), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to signal %s: %v\n", agent.WorkingDir, err)
		}
		if fs.IsAlive(agent.WorkingDir, fs.AgentAliveThresholdSec()) {
			alive = append(alive, agent.WorkingDir)
		}
	}
	// Wait for all to die (poll, max waitTimeout)
	if len(alive) > 0 {
		fmt.Printf("Suspending %d agent(s)...\n", len(alive))
		deadline := time.Now().Add(waitTimeout)
		for time.Now().Before(deadline) {
			allDead := true
			for _, dir := range alive {
				if fs.IsAlive(dir, fs.AgentAliveThresholdSec()) {
					allDead = false
					break
				}
			}
			if allDead {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
	}

	// Survivor guard: refuse to delete a live agent's body. Re-discover
	// before checking so agents that appeared during the wait window are
	// guarded too, not just the ones signaled above.
	guarded := append([]string(nil), alive...)
	if current, err := fs.DiscoverAgents(lingtaiDir); err == nil {
		seen := make(map[string]bool, len(guarded))
		for _, dir := range guarded {
			seen[dir] = true
		}
		for _, agent := range current {
			if !agent.IsHuman && !seen[agent.WorkingDir] {
				guarded = append(guarded, agent.WorkingDir)
			}
		}
	} else if !force {
		return fmt.Errorf("Cannot re-list agents under %s before deleting: %v\nNot removing it. Fix the error, or re-run with --force to delete anyway.", lingtaiDir, err)
	}
	var survivors []string
	for _, dir := range guarded {
		if fs.IsAlive(dir, fs.AgentAliveThresholdSec()) {
			survivors = append(survivors, dir)
		}
	}
	if len(survivors) > 0 {
		if !force {
			var b strings.Builder
			fmt.Fprintf(&b, "%d agent(s) still alive after %s:\n", len(survivors), waitTimeout)
			for _, dir := range survivors {
				fmt.Fprintf(&b, "  %s\n", dir)
			}
			fmt.Fprintf(&b, "Not removing %s.\nWait for them to stop, or re-run with --force to delete anyway.", lingtaiDir)
			return errors.New(b.String())
		}
		fmt.Fprintf(os.Stderr, "Warning: removing %s with %d live agent(s) (--force)\n",
			lingtaiDir, len(survivors))
	}

	if err := os.RemoveAll(lingtaiDir); err != nil {
		return fmt.Errorf("Failed to remove %s: %v", lingtaiDir, err)
	}
	return nil
}

func bootstrapMain() {
	globalDir, err := config.GlobalDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	preset.PopulateBundledLibrary(globalDir)
	fmt.Printf("Bootstrapped skills to %s/utilities/\n", globalDir)
}

func doctorMain() {
	globalDir, err := config.GlobalDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	globalmigrate.Run(globalDir)

	fmt.Println("LingTai doctor: forced update + bootstrap check")
	fmt.Printf("Global config: %s\n", globalDir)
	fmt.Println()

	report := config.RunDoctorUpdate(globalDir, config.DoctorOptions{
		CurrentTUIVersion: version,
		ForceTUI:          true,
		ForcePython:       true,
	})
	for _, line := range report.Lines {
		fmt.Printf("%s %s\n", doctorCLIIndicator(line.Severity), line.Text)
	}

	if err := preset.Bootstrap(globalDir); err != nil {
		report.Healthy = false
		fmt.Printf("✗ Bootstrap assets refresh failed: %v\n", err)
	} else {
		fmt.Println("✓ Bootstrap assets refreshed")
	}
	preset.PopulateBundledLibrary(globalDir)
	fmt.Println("✓ Utility skills refreshed")
	tui.ExportCommandsJSON(globalDir)
	fmt.Println("✓ commands.json refreshed")

	// The startup decision table (D1-D5, tui/CONTRACT.md) is the
	// TUI-can't-start diagnostic set. The interactive /doctor view emits it;
	// the CLI `lingtai-tui doctor` path (the escape hatch when the TUI cannot
	// start) must surface it too, otherwise D1/D3 — the checks whose failure
	// forces the first-run/recovery wizards — are unreachable exactly when
	// they matter (fable F5). Resolve the project's .lingtai dir the same way
	// prepareApp does; when no project is present, only the global checks run.
	if projectDir, err := os.Getwd(); err == nil {
		lingtaiDir := filepath.Join(projectDir, ".lingtai")
		if _, statErr := os.Stat(lingtaiDir); statErr == nil {
			fmt.Println()
			for _, line := range tui.CheckStartupDecisionCLI(lingtaiDir, globalDir) {
				fmt.Printf("%s\n", line)
			}
		}
	}

	if report.Healthy {
		fmt.Println()
		fmt.Println("Doctor completed: no unrecoverable update/bootstrap failures detected.")
		return
	}
	fmt.Println()
	fmt.Println("Doctor completed with failures. Review the lines above; if the TUI binary was upgraded, restart lingtai-tui and run doctor again.")
	os.Exit(1)
}

func selfUpdateMain() {
	globalDir, err := config.GlobalDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	report := config.RunManualTUIUpdate(globalDir, config.ManualTUIUpdateOptions{
		CurrentTUIVersion: version,
	})
	for _, line := range report.Lines {
		fmt.Printf("%s %s\n", doctorCLIIndicator(line.Severity), line.Text)
	}

	if report.Healthy {
		return
	}
	if report.Err != nil {
		fmt.Fprintf(os.Stderr, "Self-update failed: %v\n", report.Err)
	}
	os.Exit(1)
}

func doctorCLIIndicator(sev config.DoctorSeverity) string {
	switch sev {
	case config.DoctorOK:
		return "✓"
	case config.DoctorFail:
		return "✗"
	case config.DoctorWarn:
		return "!"
	default:
		return "•"
	}
}

func presetsMain() {
	globalDir, err := config.GlobalDir()
	if err != nil {
		headless.ExitError("cannot resolve global dir: "+err.Error(), "init_failed")
	}
	if err := preset.Bootstrap(globalDir); err != nil {
		headless.ExitError("bootstrap failed: "+err.Error(), "bootstrap_failed")
	}

	savedOnly := false
	templatesOnly := false
	for _, a := range os.Args[2:] {
		switch a {
		case "--saved-only":
			savedOnly = true
		case "--templates-only":
			templatesOnly = true
		default:
			headless.ExitError("unknown flag: "+a, "invalid_args")
		}
	}
	headless.RunPresets(os.Stdout, os.Stderr, savedOnly, templatesOnly)
}

func spawnMain() {
	if len(os.Args) < 3 {
		headless.ExitError(
			"usage: lingtai-tui spawn <directory> --preset <name> [--agent-name <name>] [--language <en|zh|wen>] [--wait-ready-timeout <duration>]",
			"invalid_args")
	}

	opts := headless.SpawnOpts{Dir: os.Args[2], Language: "en"}

	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--preset":
			if i+1 >= len(os.Args) {
				headless.ExitError("--preset requires a value", "invalid_args")
			}
			i++
			opts.Preset = os.Args[i]
		case "--agent-name":
			if i+1 >= len(os.Args) {
				headless.ExitError("--agent-name requires a value", "invalid_args")
			}
			i++
			opts.AgentName = os.Args[i]
		case "--language":
			if i+1 >= len(os.Args) {
				headless.ExitError("--language requires a value", "invalid_args")
			}
			i++
			lang := os.Args[i]
			if lang != "en" && lang != "zh" && lang != "wen" {
				headless.ExitError("--language must be en, zh, or wen", "invalid_args")
			}
			opts.Language = lang
		case "--wait-ready-timeout":
			if i+1 >= len(os.Args) {
				headless.ExitError("--wait-ready-timeout requires a value", "invalid_args")
			}
			i++
			timeout, err := time.ParseDuration(os.Args[i])
			if err != nil || timeout <= 0 {
				headless.ExitError("--wait-ready-timeout must be a positive duration like 10s", "invalid_args")
			}
			opts.ReadyTimeout = timeout
		default:
			headless.ExitError("unknown flag: "+os.Args[i], "invalid_args")
		}
	}

	if opts.Preset == "" {
		headless.ExitError("--preset is required", "invalid_args")
	}

	code := headless.RunSpawn(os.Stdout, os.Stderr, opts)
	os.Exit(code)
}

// purgeMain is defined in purge_unix.go / purge_windows.go

func runPreparedApp(projectDir string) {
	result := prepareApp(projectDir, false)
	if result.kind == startupUpgradeExit || result.kind == startupCanceled {
		return
	}
	if result.kind != startupReady {
		if result.err == nil {
			result.err = errors.New("startup did not produce an App")
		}
		fmt.Fprintln(os.Stderr, result.err)
		os.Exit(1)
	}
	p := tea.NewProgram(result.app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func startupPromptNeeded(globalDir string) bool {
	if _, err := os.Stat(filepath.Join(globalDir, ".firstrun")); os.IsNotExist(err) {
		return true
	}
	return agentCountPromptNeeded(globalDir, time.Now(), os.Stat, countRunningAgents,
		os.MkdirAll, os.WriteFile, os.Chtimes)
}

// scanAgentCount belongs to the outside-program prompt. It refreshes the
// marker before maybeShowAgentCount renders its Enter prompt.
func scanAgentCount(globalDir string, now time.Time,
	stat func(string) (os.FileInfo, error), count func() int,
	mkdirAll func(string, os.FileMode) error,
	writeFile func(string, []byte, os.FileMode) error,
	chtimes func(string, time.Time, time.Time) error) (int, bool) {
	marker := filepath.Join(globalDir, ".last_agent_check")
	info, err := stat(filepath.Join(globalDir, ".last_agent_check"))
	if err == nil && info != nil && now.Sub(info.ModTime()) < agentCheckInterval {
		return 0, false
	}
	n := count()
	_ = mkdirAll(globalDir, 0o755)
	if err := writeFile(marker, nil, 0o644); err == nil {
		_ = chtimes(marker, now, now)
	}
	return n, true
}

// agentCountPromptNeeded is the in-program preflight. A positive scan result
// deliberately leaves the marker untouched so the outside maybeShowAgentCount
// remains the sole owner of refreshing it and reading Enter. A zero result
// refreshes it here; if that refresh cannot complete, fallback is safer than
// allowing an interactive prompt to run inside a tea.Cmd.
func agentCountPromptNeeded(globalDir string, now time.Time,
	stat func(string) (os.FileInfo, error), count func() int,
	mkdirAll func(string, os.FileMode) error,
	writeFile func(string, []byte, os.FileMode) error,
	chtimes func(string, time.Time, time.Time) error) bool {
	marker := filepath.Join(globalDir, ".last_agent_check")
	info, err := stat(marker)
	if err == nil && info != nil && now.Sub(info.ModTime()) < agentCheckInterval {
		return false
	}
	if count() > 0 {
		return true
	}
	if err := mkdirAll(globalDir, 0o755); err != nil {
		return true
	}
	if err := writeFile(marker, nil, 0o644); err != nil {
		return true
	}
	if err := chtimes(marker, now, now); err != nil {
		return true
	}
	return false
}

func rustToolchainPromptNeeded(globalDir string) bool {
	if os.Getenv("LINGTAI_SKIP_RUST_PROMPT") == "1" {
		return false
	}
	info, err := os.Stdin.Stat()
	if err != nil || (info.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	if _, err := os.Stat(filepath.Join(globalDir, "runtime", "rust-toolchain-prompted")); err == nil {
		return false
	}
	status, err := config.FileSearchNativeStatus(globalDir, nil)
	if err != nil || status.Unsupported || status.SidecarPath != "" || status.Backend == "RustFileIOBackend" {
		return false
	}
	_, err = exec.LookPath("cargo")
	return err != nil
}

// kernelUpgradePromptNeeded reports whether the returning-user startup path
// running inside the launcher's Bubble Tea program should fall back out to
// read stdin for kernel-upgrade consent. Mirrors rustToolchainPromptNeeded's
// interactive-only gate: a non-TTY stdin (piped, redirected, headless) never
// prompts and never upgrades.
func kernelUpgradePromptNeeded() bool {
	info, err := os.Stdin.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

// kernelUpgradePromptOptions injects stdin/stdout, the interactivity gate,
// and the read-only inspect / mutating update functions for deterministic
// tests.
type kernelUpgradePromptOptions struct {
	Input             io.Reader
	Output            io.Writer
	Interactive       func() bool
	InspectKernelFunc func(string) config.KernelStatus
	RunKernelUpdate   func(string, bool) config.DoctorReport
}

// maybeCheckAndPromptKernelUpgrade performs a read-only kernel version check
// on EVERY default interactive launch (see
// docs/superpowers/specs/2026-06-23-launch-kernel-upgrade-confirm-design.md
// and .../2026-06-23-update-command-design.md, which define InspectKernel /
// RunKernelUpdate). install.sh installs the kernel by default, so the TUI
// assumes it is already present; this function's job is ONLY to ask before
// mutating, never to silently install/repair/upgrade on its own. It only
// ever prompts when InspectKernel reports an actionable state
// (NeedsUpdate=true); an already-current, editable, or bundle-pinned kernel
// — or a failed lookup — never prompts and never mutates. The prompt wording
// distinguishes the two actionable cases per the human's explicit state
// machine: an absent/unimportable/broken runtime (status.Installed=="")
// gets an INSTALL/REPAIR prompt, never the generic "update available"
// phrasing; a present-but-stale runtime gets an UPDATE prompt showing
// installed → latest. When a prompt IS shown, it never persists or reuses
// the answer: a "no", EOF, or non-interactive stdin all fall through to "no"
// and perform no mutation. Only an explicit "y"/"yes" on THIS invocation
// calls the mutating RunKernelUpdate (force=false, matching CheckUpgrade's
// routine semantics) — the single install/repair/update path for this
// launch. Returns true when an install/repair/update was actually performed.
func maybeCheckAndPromptKernelUpgrade(globalDir string) bool {
	return maybeCheckAndPromptKernelUpgradeWithOptions(globalDir, kernelUpgradePromptOptions{})
}

func maybeCheckAndPromptKernelUpgradeWithOptions(globalDir string, opts kernelUpgradePromptOptions) bool {
	if opts.Input == nil {
		opts.Input = os.Stdin
	}
	if opts.Output == nil {
		opts.Output = os.Stdout
	}
	if opts.Interactive == nil {
		opts.Interactive = kernelUpgradePromptNeeded
	}
	if opts.InspectKernelFunc == nil {
		opts.InspectKernelFunc = config.InspectKernel
	}
	if opts.RunKernelUpdate == nil {
		opts.RunKernelUpdate = config.RunKernelUpdate
	}

	// Read-only check runs unconditionally — even non-interactively — so the
	// human requirement "every default interactive launch performs read-only
	// version checks" holds regardless of TTY. Only the PROMPT and the
	// mutating apply step are interactive-gated.
	status := opts.InspectKernelFunc(globalDir)
	if !status.NeedsUpdate {
		return false
	}
	if !opts.Interactive() {
		return false
	}
	if status.Installed == "" {
		// Absent, unimportable, or otherwise broken — InspectKernel's own
		// contract (missing venv or failed import) never populates
		// Installed in this case. This is an install/repair decision, not a
		// version-diff upgrade, so it must never be phrased as one.
		fmt.Fprint(opts.Output, "lingtai kernel is not installed (or is broken). Install/repair it now? [y/N] ")
	} else if status.Latest != "" {
		fmt.Fprintf(opts.Output, "lingtai kernel %s → %s. Update now? [y/N] ", status.Installed, status.Latest)
	} else {
		fmt.Fprint(opts.Output, "lingtai kernel update available. Update now? [y/N] ")
	}
	if !answerYes(readLineLower(opts.Input)) {
		return false
	}
	report := opts.RunKernelUpdate(globalDir, false)
	return report.Healthy
}

// runVersionPreflight performs the shared read-only TUI + kernel version
// check exactly once, before any Bubble Tea program starts and before the
// no-project decision gate branches into "existing project" vs. "no-project
// launcher" vs. (via the launcher) "first-run" vs. "create." Every one of
// those default interactive launch shapes reaches this single call site, so
// none of them can silently skip it. Returns true when main() should exit
// immediately: a TUI self-upgrade completed (the user was already told to
// restart) or resolving the global dir path failed.
//
// Dev builds (version containing "-", e.g. v0.4.31-4-gabcdef) skip the TUI
// check, matching the long-standing convention this preflight is extracted
// from. A failed/offline TUI or kernel lookup is handled by each check's own
// existing non-blocking contract (CheckTUIUpgrade returns "" on any error;
// InspectKernel reports NeedsUpdate=false on a failed PyPI lookup) — this
// function does not add its own error handling on top.
func runVersionPreflight() bool {
	return runVersionPreflightWithOptions(preflightOptions{})
}

// preflightOptions injects every side-effecting call for deterministic
// tests: none of them may reach the real network, global config, or a
// mutating command unless a test explicitly wants to assert that path. Nil
// fields (production) use the real config.GlobalDirPath (pure, no mkdir),
// config.CheckTUIUpgrade, config.DetectCurrentTUIInstall, handleTUIUpgrade,
// and maybeCheckAndPromptKernelUpgrade. A consented mutation (TUI
// self-upgrade or RunKernelUpdate) creates ~/.lingtai-tui itself as an
// intentional side effect of that mutation, not of merely checking.
type preflightOptions struct {
	GlobalDirPath           func() (string, error)
	CheckTUIUpgradeFunc     func(string) string
	DetectCurrentTUIInstall func(string) config.TUIInstallInfo
	HandleTUIUpgrade        func(config.TUIInstallInfo, string, string, string) bool
	CheckAndPromptKernel    func(string) bool
	PrintOutput             io.Writer
}

func runVersionPreflightWithOptions(opts preflightOptions) bool {
	if opts.GlobalDirPath == nil {
		opts.GlobalDirPath = config.GlobalDirPath
	}
	if opts.CheckTUIUpgradeFunc == nil {
		opts.CheckTUIUpgradeFunc = config.CheckTUIUpgrade
	}
	if opts.DetectCurrentTUIInstall == nil {
		opts.DetectCurrentTUIInstall = config.DetectCurrentTUIInstall
	}
	if opts.HandleTUIUpgrade == nil {
		opts.HandleTUIUpgrade = handleTUIUpgrade
	}
	if opts.CheckAndPromptKernel == nil {
		opts.CheckAndPromptKernel = maybeCheckAndPromptKernelUpgrade
	}
	if opts.PrintOutput == nil {
		opts.PrintOutput = os.Stdout
	}

	globalDir, err := opts.GlobalDirPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return true
	}

	// TUI version check (read-only). Skip for dev builds, matching the
	// pre-existing convention this block is extracted from verbatim.
	isDev := strings.Contains(version, "-")
	latestVersion := ""
	if !isDev {
		latestVersion = opts.CheckTUIUpgradeFunc(version)
	}
	if latestVersion != "" {
		install := opts.DetectCurrentTUIInstall(globalDir)
		switch install.Method {
		case config.TUIInstallMethodHomebrew, config.TUIInstallMethodSource:
			if opts.HandleTUIUpgrade(install, version, latestVersion, globalDir) {
				// A successful self-upgrade has always told the user to
				// restart. Exit main() here, before the no-project gate or
				// launcher ever starts — never construct or run an App.
				return true
			}
		default:
			fmt.Fprintln(opts.PrintOutput, "lingtai-tui "+version)
		}
	} else {
		fmt.Fprintln(opts.PrintOutput, "lingtai-tui "+version)
	}

	// Kernel version check (read-only) + consent-gated prompt. Always runs
	// on a plain terminal here (no Bubble Tea program exists yet), so the
	// prompt can safely block on stdin directly.
	if opts.CheckAndPromptKernel(globalDir) {
		fmt.Fprintln(opts.PrintOutput, "Upgraded lingtai to latest version.")
	}
	return false
}

// prepareApp runs the normal post-decision startup pipeline and returns the
// real App without starting a second Bubble Tea program. When inProgram is
// true, terminal-coupled prompts and fatal paths become typed outcomes for
// the caller to handle after Bubble Tea has released the terminal.
// startupDecision computes the launch mode from the R1/R2/R3 requirement
// state (tui/CONTRACT.md).
//   - R1  orchestrators present (zero agents → real first-run setup; also
//        catches the `lingtai-tui clean` recreated-empty-.lingtai case)
//   - R2  API keys resolvable (resolvedKeys = .env authoritative + config.json
//        mirror fills gaps)
//   - R3.1  the config.json keys mirror is present (configOK) and non-empty
//
// Returns:
//   firstRun — no agents (or rehydration, applied by the caller)
//   recovery — R2 fail: no API key anywhere → real setup wizard
//   degraded — R3.1 loss while R2 ok → launch with banner, keys from .env
//
// Content-based (fable F7): a present-but-keyless config.json degrades exactly
// like an absent one, so the original config-wipe incident cannot silently
// recur through a keyless-but-present mirror.
func startupDecision(orchestrators int, resolvedKeys map[string]string, configOK bool, mirrorKeys map[string]string) (firstRun, recovery, degraded bool) {
	if orchestrators == 0 {
		return true, false, false
	}
	if !config.HasAPIKeys(resolvedKeys) {
		return false, true, false
	}
	if !configOK || !config.HasAPIKeys(mirrorKeys) {
		return false, false, true
	}
	return false, false, false
}

func prepareApp(projectDir string, inProgram bool) startupResult {
	// TUI + kernel version checks already ran once in main(), via
	// runVersionPreflight, before any Bubble Tea program started and before
	// the no-project decision gate — including for this call, whichever of
	// the three default-launch shapes (existing project, first-run via the
	// no-project launcher, or a fresh Create) led here. prepareApp never
	// repeats them; doing so here would be a second, redundant read-only
	// check per launch (and, if triggered inside the launcher's Bubble Tea
	// program, a duplicate prompt).
	globalDir, err := config.GlobalDir()
	if err != nil {
		return startupResult{kind: startupFatal, err: fmt.Errorf("error: %w", err)}
	}

	// Global per-machine migrations (versioned in ~/.lingtai-tui/meta.json).
	// Best-effort housekeeping — failures don't abort startup.
	globalmigrate.Run(globalDir)

	// Resolve the UI language early so every user-visible startup string
	// (codex banner, welcome, agent-count reminder) renders in the configured
	// locale rather than the i18n default. tuiCfg is reused below.
	config.MigrateLegacyLanguage(globalDir)
	tuiCfg := config.LoadTUIConfig(globalDir)
	if err := i18n.SetLang(tuiCfg.Language); err != nil {
		if !inProgram {
			fmt.Fprintf(os.Stderr, "warning: invalid configured language %q: %v; continuing with %q\n", tuiCfg.Language, err, i18n.Lang())
		} else {
			return startupResult{kind: startupFallback, projectDir: projectDir}
		}
		tuiCfg.Language = i18n.Lang()
	}

	// Test Codex OAuth validity on every launch across ALL stored accounts
	// (legacy ~/.lingtai-tui/codex-auth.json plus per-account files under
	// codex-auth/). For each account whose access token is expired (or
	// near-expired), this round-trips the refresh token through
	// auth.openai.com and writes the refreshed bundle back atomically. A
	// 401/403 response means that account's grant was revoked server-side
	// (password changed, "log out everywhere", etc.) — surface that as a
	// startup banner naming the account so the user re-OAuths via /setup.
	// Transient errors (offline, 5xx) leave local tokens untouched and stay
	// silent.
	codexBanner := tui.ValidateCodexAuthOnStartup(globalDir)
	if codexBanner != "" {
		if inProgram {
			return startupResult{kind: startupFallback, projectDir: projectDir}
		}
		fmt.Println(codexBanner)
	}

	lingtaiDir := filepath.Join(projectDir, ".lingtai")

	// Welcome, running-agent, and legacy-addon notices own stdin. Let the
	// renderer shut down before the original outside-Program path handles any
	// of them.
	if inProgram && startupPromptNeeded(globalDir) {
		return startupResult{kind: startupFallback}
	}

	// First-time welcome — show once, write .firstrun sentinel
	showWelcome(globalDir)

	// Periodic running-agent reminder (every 4 hours, gated by marker file).
	maybeShowAgentCount(globalDir)

	// If .lingtai/ doesn't exist, check for phantom processes before creating it
	if _, err := os.Stat(lingtaiDir); os.IsNotExist(err) {
		self, _ := os.Executable()
		out, _ := exec.Command(self, "list", projectDir).Output()
		if len(out) > 0 && strings.Contains(string(out), "[PHANTOM]") {
			return startupResult{kind: startupFatal, err: errors.New(string(out))}
		}
	}

	// Rehydration state: set below if the network needs rehydration (cloned
	// agora network with no init.json files but an intact .agent.json blueprint).
	var needsRehydration bool
	var rehydrateOrchDir, rehydrateOrchName string

	// Existing projects are opened exactly as stored. Runtime project
	// migrations and migration-progress stamps are retired; compatibility
	// diagnosis/repair belongs to the kernel reader and explicit Agent edits.
	if _, err := os.Stat(lingtaiDir); err == nil {
		// Sanity checks: init.json all-or-nothing, and exactly one orchestrator.
		// Both refuse to launch on failure rather than limp along with a
		// broken network. Run before any mutation so the on-disk state is
		// preserved exactly as the user left it.
		nr, err := checkInitJSONInvariant(lingtaiDir)
		if err != nil {
			return startupResult{kind: startupFatal, err: err}
		}
		needsRehydration = nr
		if err := checkOrchestratorInvariant(lingtaiDir); err != nil {
			return startupResult{kind: startupFatal, err: err}
		}
		// If the network needs rehydration, find the orchestrator's dir and
		// name from its .agent.json blueprint so the wizard can prefill them.
		if needsRehydration {
			rehydrateOrchDir, rehydrateOrchName = findOrchestratorBlueprint(lingtaiDir)
			if rehydrateOrchDir == "" {
				return startupResult{kind: startupFatal, err: errors.New("error: rehydration needed but could not locate orchestrator")}
			}
		}
	}

	// Init project (create human dir)
	if err := process.InitProject(lingtaiDir); err != nil {
		return startupResult{kind: startupFatal, err: fmt.Errorf("error: %w", err)}
	}
	// Register this project in the global registry for /projects discovery.
	// Non-fatal: TUI works even if registration fails.
	if err := config.Register(globalDir, projectDir); err != nil {
		if !inProgram {
			fmt.Fprintf(os.Stderr, "warning: failed to register project: %v\n", err)
		} else {
			return startupResult{kind: startupFallback, projectDir: projectDir}
		}
	}
	// TUI utility skills — extracted to <globalDir>/utilities/ on every
	// startup. Agents reach these via the library.paths default in init.json.
	preset.PopulateBundledLibrary(globalDir)

	// Compute launch mode from R1/R2/R3 requirement state (tui/CONTRACT.md).
	// startupDecision is content-based (fable F7): a present-but-keyless
	// config.json — e.g. only the legacy `language` key — is an R3.1 mirror
	// loss and degrades exactly like an absent file, so the original incident
	// cannot silently recur. Zero orchestrators force first-run (the
	// `lingtai-tui clean` recreated-empty-.lingtai case).
	orchestrators := tui.DetectOrchestrators(lingtaiDir)
	keys, configOK := config.ResolveKeys(globalDir)
	mirror, _ := config.LoadConfigReadOnly(globalDir)
	needsFirstRun, needsRecovery, degradedConfig := startupDecision(len(orchestrators), keys, configOK, mirror.Keys)

	// Rehydration forces us into the first-run wizard regardless of whether
	// the user has a global config.json — cloned networks always need to be
	// walked through setup before they can launch.
	if needsRehydration {
		needsFirstRun = true
		needsRecovery = false
		degradedConfig = false
	}

	if !needsFirstRun {
		// Returning user — assets (fast no-ops if already exist). The runtime
		// itself is NOT ensured/installed/repaired here: install.sh installs
		// the kernel by default, and the ONLY automatic opportunity to
		// install or repair it is main()'s runVersionPreflight (before the
		// no-project gate), gated on explicit current-launch human consent.
		// A declined or absent runtime must not be silently repaired behind
		// that decision — config.RuntimeReady is a pure readiness check
		// (never creates/repairs/upgrades); a not-ready runtime surfaces as
		// an actionable error instead.
		if err := config.RuntimeReady(globalDir); err != nil {
			return startupResult{kind: startupFatal, err: err}
		}
		if err := preset.Bootstrap(globalDir); err != nil {
			return startupResult{kind: startupFatal, err: fmt.Errorf("bootstrap error: %w", err)}
		}
		tui.ExportCommandsJSON(globalDir)
		if inProgram && rustToolchainPromptNeeded(globalDir) {
			return startupResult{kind: startupFallback}
		}
		maybePromptRustToolchain(globalDir)

		// Recipe reconciliation: if the project carries a recipe bundle at
		// its root (.recipe/) and the contents differ from the last-applied
		// snapshot under .lingtai/.tui-asset/.recipe/, re-apply so each
		// agent's .prompt, library.paths, and snapshot stay in sync with
		// the currently-selected recipe. No-op when .recipe/ is absent
		// (pre-redesign projects, or projects that haven't gone through
		// /setup yet) or when the snapshot already matches.
		//
		// Greet substitution intentionally uses the startup humanDir/addr/
		// lang/soulDelay defaults; a proper re-apply via /setup gives the
		// user full control over those fields. This path is just the "you
		// edited .recipe/<layer>/<layer>.md by hand, we'll redo the
		// .prompt" convenience.
		projectRoot := filepath.Dir(lingtaiDir)
		if preset.RecipeNeedsApply(projectRoot) {
			if inProgram {
				return startupResult{kind: startupFallback, projectDir: projectDir}
			}
			humanDir := filepath.Join(lingtaiDir, "human")
			humanAddr := "human"
			if humanNode, err := fs.ReadAgent(humanDir); err == nil && humanNode.Address != "" {
				humanAddr = humanNode.Address
			}
			lang := tuiCfg.Language
			if lang == "" {
				lang = "en"
			}
			subst := func(tmpl string) string {
				return tui.SubstituteGreetPlaceholders(tmpl, humanAddr, humanDir, lang, "120")
			}
			if _, err := preset.ApplyRecipe(projectRoot, lang, subst); err != nil {
				if !inProgram {
					fmt.Fprintf(os.Stderr, "warning: recipe reconcile failed: %v\n", err)
				} else {
					return startupResult{kind: startupFallback, projectDir: projectDir}
				}
			}
		}
		// Resolve human location in background (ipinfo.io, cached 1h)
		humanDir := filepath.Join(lingtaiDir, "human")
		go fs.UpdateHumanLocation(humanDir)
	}
	// If needsFirstRun: welcome page goroutine handles everything

	// Do NOT auto-relaunch stopped agents on TUI startup. The TUI's job is
	// to attach to whatever state the agent is in, not to second-guess why
	// it's stopped. Causes of stopped-at-rest are externally indistinguishable
	// (deliberate /suspend, crash, kill -9, machine reboot mid-run, …) and
	// auto-revival overrides the user's last explicit decision (typically
	// /suspend) without their consent. Users wake stopped agents explicitly
	// via /cpr or /refresh from inside the TUI. The only place we launch on
	// startup is the FirstRunDoneMsg handler in app.go, which fires when the
	// user creates a new agent through the first-run wizard.

	app := tui.NewApp(globalDir, lingtaiDir, needsFirstRun, needsRecovery, degradedConfig, orchestrators, tuiCfg, rehydrateOrchDir, rehydrateOrchName)
	return startupResult{kind: startupReady, app: app}
}
