package main

import (
	"bufio"
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

	// If .lingtai/ exists, run migrations before anything else
	if _, err := os.Stat(lingtaiDir); err == nil {
		if err := migrate.Run(lingtaiDir); err != nil {
			fmt.Fprintf(os.Stderr, "migration error: %v\n", err)
			os.Exit(1)
		}
		// One-time check: warn about legacy addon-instruction blocks in
		// agent comment.md files (left over from older TUI versions before
		// the skill system replaced WriteAddonComment). The check runs
		// once per project and self-suppresses via meta.json.
		notifyLegacyAddonComments(lingtaiDir)
	}

	// Init project (create human dir)
	if err := process.InitProject(lingtaiDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	// Register this project in the global registry for /projects discovery.
	// Non-fatal: TUI works even if registration fails.
	if err := config.Register(globalDir, projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to register project: %v\n", err)
	}
	// Bundled skills — always populate (idempotent, skips existing files).
	// Runs every startup so existing projects get new skills on TUI upgrade.
	preset.PopulateBundledSkills(lingtaiDir)

	// First run = no config.json in ~/.lingtai-tui/
	configPath := filepath.Join(globalDir, "config.json")
	_, configErr := os.Stat(configPath)
	needsFirstRun := os.IsNotExist(configErr)

	// Load TUI config (migrate language from legacy config.json if needed)
	config.MigrateLegacyLanguage(globalDir)
	tuiCfg := config.LoadTUIConfig(globalDir)
	i18n.SetLang(tuiCfg.Language)

	orchestrators := tui.DetectOrchestrators(lingtaiDir)

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
		// Resolve human location in background (ipinfo.io, cached 1h)
		humanDir := filepath.Join(lingtaiDir, "human")
		go fs.UpdateHumanLocation(humanDir)
	}
	// If needsFirstRun: welcome page goroutine handles everything

	// Also need first-run if no orchestrator in this project
	if len(orchestrators) == 0 {
		needsFirstRun = true
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
	app := tui.NewApp(globalDir, lingtaiDir, needsFirstRun, orchestrators, tuiCfg)
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
