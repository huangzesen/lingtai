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
			fmt.Println("Usage: lingtai-tui")
			fmt.Println("       lingtai-tui purge [dir]")
			fmt.Println("       lingtai-tui list [dir]")
			fmt.Println("       lingtai-tui clean")
			fmt.Println()
			fmt.Println("  (no args)    Launch TUI in current directory")
			fmt.Println("  purge        Kill lingtai processes (all, or only those in <dir>)")
			fmt.Println("  list         Show running lingtai processes (all, or only those in <dir>)")
			fmt.Println("  clean        Suspend agents in current directory, then remove .lingtai/")
			fmt.Println()
			// Show directories
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
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'lingtai-tui --help' for usage.\n", arg)
		os.Exit(1)
	}

	// Print version and check for updates (3s timeout)
	latestVersion := config.CheckTUIUpgrade(version)
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
	}

	// Init project (create human dir)
	if err := process.InitProject(lingtaiDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

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
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			}
		} else {
			// Venv exists — check for lingtai upgrades
			if config.CheckUpgrade(globalDir) {
				fmt.Println("Upgraded lingtai to latest version.")
			}
		}
		preset.Bootstrap(globalDir)
		// Resolve human location in background (ipinfo.io, cached 1h)
		humanDir := filepath.Join(lingtaiDir, "human")
		go fs.UpdateHumanLocation(humanDir)
	}
	// If needsFirstRun: welcome page goroutine handles everything

	// Also need first-run if no orchestrator in this project
	if len(orchestrators) == 0 {
		needsFirstRun = true
	}

	// If 本我 found but not alive, auto-launch it
	if !needsFirstRun && len(orchestrators) == 1 {
		orchDir := filepath.Join(lingtaiDir, orchestrators[0])
		if !fs.IsAlive(orchDir, 2.0) {
			lingtaiCmd := config.LingtaiCmd(globalDir)
			if lingtaiCmd != "" {
				if _, err := process.LaunchAgent(lingtaiCmd, orchDir); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to launch agent: %v\n", err)
				}
			}
		}
	}

	// Launch TUI
	app := tui.NewApp(globalDir, lingtaiDir, needsFirstRun, orchestrators, tuiCfg)
	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
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

	// Suspend all agents first
	for _, agent := range agents {
		if agent.IsHuman {
			continue
		}
		if fs.IsAlive(agent.WorkingDir, 3.0) {
			fmt.Printf("Suspending %s...\n", agent.AgentName)
			fs.SuspendAndWait(agent.WorkingDir, 5*time.Second)
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
