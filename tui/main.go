package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/migrate"
	"github.com/anthropics/lingtai-tui/internal/preset"
	"github.com/anthropics/lingtai-tui/internal/process"
	"github.com/anthropics/lingtai-tui/internal/secretary"
	"github.com/anthropics/lingtai-tui/internal/tui"
)

// version is set at build time via -ldflags "-X main.version=v0.4.2"
var version = "dev"

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
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'lingtai-tui --help' for usage.\n", arg)
		os.Exit(1)
	}

	// Print version and check for updates (3s timeout).
	// Skip upgrade check for dev builds (version contains '-' suffix like v0.4.31-4-gabcdef).
	isDev := strings.Contains(version, "-")
	latestVersion := ""
	if !isDev {
		latestVersion = config.CheckTUIUpgrade(version)
	}
	if latestVersion != "" {
		fmt.Printf("lingtai-tui %s (latest: %s)\n", version, latestVersion)
		fmt.Printf("  Upgrade now? [Y/n] ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		answer := strings.TrimSpace(strings.ToLower(line))
		if answer != "n" && answer != "no" {
			fmt.Println("  Upgrading...")
			cmd := exec.Command("brew", "upgrade", "huangzesen/lingtai/lingtai-tui")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "  Upgrade failed: %v\n", err)
			} else {
				// Verify the upgrade actually changed the binary by re-checking
				// the version. Brew returns exit 0 even for "already installed".
				postUpgrade := config.CheckTUIUpgrade(version)
				if postUpgrade != "" {
					// Still outdated — brew formula not updated yet, don't loop
					fmt.Println("  Brew formula not yet updated. Run manually later:")
					fmt.Println("    brew update && brew upgrade huangzesen/lingtai/lingtai-tui")
				} else {
					fmt.Println("  Upgraded! Restarting...")
					self, _ := os.Executable()
					syscallExec(self, os.Args, os.Environ())
				}
			}
		}
	} else {
		fmt.Println("lingtai-tui " + version)
	}

	// Always start in current directory
	projectDir, _ := os.Getwd()
	projectDir, _ = filepath.Abs(projectDir)

	// Global config directory (~/.lingtai-tui)
	globalDir, err := config.GlobalDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// First-time welcome — show once, write .firstrun sentinel
	showWelcome(globalDir)

	// Periodic running-agent reminder (every 4 hours, gated by marker file).
	maybeShowAgentCount(globalDir)

	lingtaiDir := filepath.Join(projectDir, ".lingtai")

	// If .lingtai/ doesn't exist, check for phantom processes before creating it
	if _, err := os.Stat(lingtaiDir); os.IsNotExist(err) {
		self, _ := os.Executable()
		out, _ := exec.Command(self, "list", projectDir).Output()
		if len(out) > 0 && strings.Contains(string(out), "[PHANTOM]") {
			fmt.Print(string(out))
			os.Exit(1)
		}
	}

	// Check for phantom processes in the secretary's network too.
	secProjectDir := filepath.Join(globalDir, "secretary")
	if _, err := os.Stat(filepath.Join(secProjectDir, ".lingtai")); err == nil {
		self, _ := os.Executable()
		out, _ := exec.Command(self, "list", secProjectDir).Output()
		if len(out) > 0 && strings.Contains(string(out), "[PHANTOM]") {
			fmt.Println("Secretary has phantom processes:")
			fmt.Print(string(out))
			fmt.Printf("Run: lingtai-tui purge %s\n", secProjectDir)
			os.Exit(1)
		}
	}

	// Rehydration state: set below if the network needs rehydration (cloned
	// agora network with no init.json files but an intact .agent.json blueprint).
	var needsRehydration bool
	var rehydrateOrchDir, rehydrateOrchName string

	// If .lingtai/ exists, run migrations before anything else
	if _, err := os.Stat(lingtaiDir); err == nil {
		if err := migrate.Run(lingtaiDir); err != nil {
			fmt.Fprintf(os.Stderr, "migration error: %v\n", err)
			os.Exit(1)
		}
		// Sanity checks: init.json all-or-nothing, and exactly one orchestrator.
		// Both refuse to launch on failure rather than limp along with a
		// broken network. Run before any mutation so the on-disk state is
		// preserved exactly as the user left it.
		nr, err := checkInitJSONInvariant(lingtaiDir)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
		needsRehydration = nr
		if err := checkOrchestratorInvariant(lingtaiDir); err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
		// If the network needs rehydration, find the orchestrator's dir and
		// name from its .agent.json blueprint so the wizard can prefill them.
		if needsRehydration {
			rehydrateOrchDir, rehydrateOrchName = findOrchestratorBlueprint(lingtaiDir)
			if rehydrateOrchDir == "" {
				fmt.Fprintln(os.Stderr, "error: rehydration needed but could not locate orchestrator")
				os.Exit(1)
			}
		}
		// One-time check: warn about legacy addon-instruction blocks in
		// agent comment.md files (left over from older TUI versions before
		// the skill system replaced WriteAddonComment). The check runs
		// once per project and self-suppresses via meta.json.
		notifyLegacyAddonComments(lingtaiDir)
	}

	// Init project (create human dir)
	if err := process.InitProject(lingtaiDir, globalDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	// Register this project in the global registry for /projects discovery.
	// Non-fatal: TUI works even if registration fails.
	if err := config.Register(globalDir, projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to register project: %v\n", err)
	}
	// Bundled (canonical) skills — written to globalDir/bundled-skills/
	// and symlinked into .lingtai/.skills/intrinsic. Refreshed every
	// startup so projects pick up new or updated skills after a TUI upgrade.
	preset.PopulateBundledSkills(lingtaiDir, globalDir)

	// First run = no config.json in ~/.lingtai-tui/
	configPath := filepath.Join(globalDir, "config.json")
	_, configErr := os.Stat(configPath)
	needsFirstRun := os.IsNotExist(configErr)

	// Rehydration forces us into the first-run wizard regardless of whether
	// the user has a global config.json — cloned networks always need to be
	// walked through setup before they can launch.
	if needsRehydration {
		needsFirstRun = true
	}

	// Load TUI config (migrate language from legacy config.json if needed)
	config.MigrateLegacyLanguage(globalDir)
	tuiCfg := config.LoadTUIConfig(globalDir)
	i18n.SetLang(tuiCfg.Language)

	// Recipe skills — symlink skills from all known recipes into .lingtai/.skills/.
	// On first run, no custom recipe is set yet, so only bundled recipe skills are
	// linked. Custom/agora recipe skills are picked up on the next launch.
	// LoadRecipeState returns zero-value on error, so customDir is "" and bundled +
	// agora recipe skills are still linked even if .recipe is corrupt.
	recipeState, _ := preset.LoadRecipeState(lingtaiDir)
	preset.LinkRecipeSkills(lingtaiDir, globalDir, tuiCfg.Language, recipeState.Recipe, recipeState.CustomDir)
	preset.PruneStaleSkillSymlinks(lingtaiDir)

	orchestrators := tui.DetectOrchestrators(lingtaiDir)

	// Reconcile needsFirstRun with actual orchestrator state.
	// If there are zero orchestrators, force first-run. This catches the
	// "user ran `lingtai-tui clean` and relaunched in the same folder"
	// case: clean removed .lingtai/, so the invariant checks at the top
	// of main() were skipped (they only run if .lingtai/ already exists),
	// but process.InitProject then recreated an empty .lingtai/ with only
	// human/ inside. Without this fallback, a returning user (global
	// config.json exists, so needsFirstRun would otherwise be false) would
	// reach NewApp with no orchestrator to launch.
	needsRecovery := false
	if len(orchestrators) == 0 {
		needsFirstRun = true
	} else if needsFirstRun && !needsRehydration {
		// Existing orchestrators found in .lingtai/ but global config is
		// missing (e.g. user deleted ~/.lingtai-tui). The agents are real
		// and must not be duplicated — show setup for API keys only.
		needsFirstRun = false
		needsRecovery = true
	}

	if !needsFirstRun {
		// Returning user — ensure runtime + assets (fast no-ops if already exist)
		if config.NeedsVenv(globalDir) {
			fmt.Println("Setting up Python environment...")
			if err := config.EnsureVenv(globalDir); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Venv exists — check for lingtai upgrades
			if config.CheckUpgrade(globalDir) {
				fmt.Println("Upgraded lingtai to latest version.")
			}
		}
		if err := preset.Bootstrap(globalDir); err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap error: %v\n", err)
			os.Exit(1)
		}
		tui.ExportCommandsJSON(globalDir)
		// Resolve human location in background (ipinfo.io, cached 1h)
		humanDir := filepath.Join(lingtaiDir, "human")
		go fs.UpdateHumanLocation(humanDir)
	}
	// If needsFirstRun: welcome page goroutine handles everything

	// Secretary health check — if the secretary has been set up but is not
	// running, gently ask the user if they want to wake it up.
	if !needsFirstRun {
		maybeReviveSecretary(globalDir)
	}

	// Do NOT auto-relaunch stopped agents on TUI startup. The TUI's job is
	// to attach to whatever state the agent is in, not to second-guess why
	// it's stopped. Causes of stopped-at-rest are externally indistinguishable
	// (deliberate /suspend, crash, kill -9, machine reboot mid-run, …) and
	// auto-revival overrides the user's last explicit decision (typically
	// /suspend) without their consent. Users wake stopped agents explicitly
	// via /cpr or /refresh from inside the TUI. The only place we launch on
	// startup is the FirstRunDoneMsg handler in app.go, which fires when the
	// user creates a new agent through the first-run wizard.

	// Launch TUI
	app := tui.NewApp(globalDir, lingtaiDir, needsFirstRun, needsRecovery, orchestrators, tuiCfg, rehydrateOrchDir, rehydrateOrchName)
	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// notifyLegacyAddonComments performs a one-time scan of the project's agent
// directories for legacy addon-instruction blocks left over from older TUI
// versions, prints a notice with cleanup suggestions if any are found, and
// marks meta.json so the check is not repeated. Always marks notified after
// running, even when no matches are found, so the scan happens at most once
// per project per upgrade.
func notifyLegacyAddonComments(lingtaiDir string) {
	notified, err := migrate.IsAddonCommentNotified(lingtaiDir)
	if err != nil || notified {
		return
	}
	matches, err := migrate.CheckAddonComment(lingtaiDir)
	if err != nil {
		// Non-fatal: skip the check if we can't read .lingtai/
		return
	}
	if len(matches) > 0 {
		fmt.Println()
		fmt.Printf("⚠ Found legacy addon-instruction blocks in %d agent comment file(s):\n", len(matches))
		for _, p := range matches {
			fmt.Printf("   %s\n", p)
		}
		fmt.Println()
		fmt.Println("These blocks were generated by an older TUI to tell agents how addons")
		fmt.Println("work. The skill system now handles this natively, and the blocks have")
		fmt.Println("become slightly harmful:")
		fmt.Println()
		fmt.Println("  - They duplicate (sometimes contradict) what's in init.json and the")
		fmt.Println("    addon SKILL.md files")
		fmt.Println("  - They prime every conversation toward addon setup, even when you're")
		fmt.Println("    not asking about addons")
		fmt.Println("  - They're English-only — Chinese and wen agents see English text in")
		fmt.Println("    their otherwise-localized system prompt")
		fmt.Println("  - If you manually edit init.json's addon paths, the comment.md still")
		fmt.Println("    has the old path baked in — two sources of truth that can disagree")
		fmt.Println()
		fmt.Println("Recommended cleanup:")
		fmt.Println("   rm <path>   (if you don't have custom content in those files)")
		fmt.Println()
		fmt.Println("   Or: open each file and delete the \"## Add-ons\" section while")
		fmt.Println("   keeping any custom content above it.")
		fmt.Println()
		fmt.Print("This message will not appear again. Press Enter to continue...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Println()
	}
	// Mark notified even when no matches, so the scan never repeats.
	if err := migrate.MarkAddonCommentNotified(lingtaiDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to mark addon comment notification: %v\n", err)
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
// Dot-prefixed directories under .lingtai/ (.skills/, .portal/, .addons/,
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
	fmt.Println("       lingtai-tui list [dir]")
	fmt.Println("       lingtai-tui suspend [dir]")
	fmt.Println("       lingtai-tui clean")
	fmt.Println()
	fmt.Println("  (no args)    Launch TUI in current directory")
	fmt.Println("  purge        Kill all lingtai agent processes on this machine.")
	fmt.Println("               Agents are autonomous — they keep running after you")
	fmt.Println("               exit the TUI. Use purge when you need them all dead.")
	fmt.Println("  list         Show running lingtai processes (all, or only those in <dir>)")
	fmt.Println("  suspend      Gracefully suspend agents via signal files (all, or those in <dir>)")
	fmt.Println("  clean        Suspend agents in current directory, then remove .lingtai/")
	fmt.Println()
	fmt.Println("  You are responsible for all .lingtai/ folders on this machine.")
	fmt.Println("  They are the bodies of your agents — files, memory, mail, identity.")
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
	fmt.Println("    • Agents act on their own after idle timeout (soul flow)")
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
	marker := filepath.Join(globalDir, ".last_agent_check")
	if info, err := os.Stat(marker); err == nil {
		if time.Since(info.ModTime()) < agentCheckInterval {
			return // checked recently, stay quiet
		}
	}

	n := countRunningAgents()

	// Refresh marker regardless of count, so we don't rescan for another
	// interval even when nothing is running.
	os.MkdirAll(globalDir, 0o755)
	now := time.Now()
	if err := os.WriteFile(marker, nil, 0o644); err == nil {
		os.Chtimes(marker, now, now)
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

// maybeReviveSecretary checks whether the secretary agent has been set up
// but is not currently running. If so, it asks the user whether to wake it.
func maybeReviveSecretary(globalDir string) {
	secDir := secretary.AgentDir(globalDir)
	if _, err := os.Stat(filepath.Join(secDir, ".agent.json")); os.IsNotExist(err) {
		return // secretary never set up — nothing to do
	}
	if fs.IsAlive(secDir, 3.0) {
		return // already running
	}

	fmt.Print("Secretary is not running. Wake up? [Y/n] ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer == "n" || answer == "no" {
		return
	}

	lingtaiCmd := config.LingtaiCmd(globalDir)
	if _, err := process.LaunchAgent(lingtaiCmd, secDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to wake secretary: %v\n", err)
		return
	}
	fs.WritePrompt(secDir, secretary.GreetContent())
	fmt.Println("Secretary is waking up.")
}

func cleanMain() {
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

	// Signal all agents at once (touch .suspend in every folder)
	var alive []string
	for _, agent := range agents {
		if agent.IsHuman {
			continue
		}
		suspendFile := filepath.Join(agent.WorkingDir, ".suspend")
		os.WriteFile(suspendFile, []byte(""), 0o644)
		if fs.IsAlive(agent.WorkingDir, 3.0) {
			alive = append(alive, agent.WorkingDir)
		}
	}
	// Wait for all to die (poll, max 10s)
	if len(alive) > 0 {
		fmt.Printf("Suspending %d agent(s)...\n", len(alive))
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			allDead := true
			for _, dir := range alive {
				if fs.IsAlive(dir, 3.0) {
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

	// Remove .lingtai/
	if err := os.RemoveAll(lingtaiDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", lingtaiDir, err)
		os.Exit(1)
	}
	fmt.Printf("Removed %s\n", lingtaiDir)
	fmt.Println()
	fmt.Println("To also remove global config, run:")
	fmt.Println("  rm -rf ~/.lingtai-tui")
}

// purgeMain is defined in purge_unix.go / purge_windows.go
