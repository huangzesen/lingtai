package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/api"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/preset"
	"github.com/anthropics/lingtai-tui/internal/process"
	"github.com/anthropics/lingtai-tui/internal/tui"
)

func main() {
	// Handle flags
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "--help" || arg == "-h" {
			fmt.Println("Usage: lingtai-tui")
			fmt.Println("       lingtai-tui tutorial")
			fmt.Println("       lingtai-tui suspend")
			fmt.Println("       lingtai-tui purge [dir]")
			fmt.Println("       lingtai-tui list [dir]")
			fmt.Println()
			fmt.Println("  (no args)    Launch TUI in current directory")
			fmt.Println("  tutorial     Start or resume the guided tutorial")
			fmt.Println("  suspend      Suspend all agents in current directory")
			fmt.Println("  purge        Kill lingtai processes (all, or only those in <dir>)")
			fmt.Println("  list         Show running lingtai processes (all, or only those in <dir>)")
			os.Exit(0)
		}
		if arg == "--version" || arg == "-v" {
			fmt.Println("lingtai-tui 0.1.1")
			os.Exit(0)
		}
		if arg == "tutorial" {
			tutorialMain()
			return
		}
		if arg == "suspend" {
			suspendMain()
			return
		}
		if arg == "purge" {
			purgeMain()
			return
		}
		if arg == "list" {
			listMain()
			return
		}
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'lingtai-tui --help' for usage.\n", arg)
		os.Exit(1)
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

	// Init project (create human dir)
	if err := process.InitProject(lingtaiDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Start API server
	srv := api.NewServer(lingtaiDir, WebFS())
	portFile := filepath.Join(lingtaiDir, ".port")
	if err := srv.Start(portFile); err != nil {
		fmt.Fprintf(os.Stderr, "error starting server: %v\n", err)
		os.Exit(1)
	}
	defer srv.Stop(context.Background())

	// First run = no config.json in ~/.lingtai-tui/
	configPath := filepath.Join(globalDir, "config.json")
	_, configErr := os.Stat(configPath)
	needsFirstRun := os.IsNotExist(configErr)

	// Load global config and settings
	globalCfg, _ := config.LoadConfig(globalDir)
	settings := tui.LoadSettings(lingtaiDir)
	if globalCfg.Language != "" {
		i18n.SetLang(globalCfg.Language)
	} else {
		i18n.SetLang(settings.Language)
	}

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
	app := tui.NewApp(globalDir, lingtaiDir, srv.URL(), needsFirstRun, orchestrators, settings)
	p := tea.NewProgram(app)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func tutorialMain() {
	projectDir, _ := os.Getwd()
	projectDir, _ = filepath.Abs(projectDir)

	globalDir, err := config.GlobalDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Need setup first
	if config.NeedsSetup(globalDir) {
		fmt.Fprintf(os.Stderr, "Run lingtai-tui first to complete setup.\n")
		os.Exit(1)
	}

	// Ensure runtime
	if config.NeedsVenv(globalDir) {
		fmt.Println("Setting up Python environment...")
		if err := config.EnsureVenv(globalDir); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	} else {
		if config.CheckUpgrade(globalDir) {
			fmt.Println("Upgraded lingtai to latest version.")
		}
	}
	preset.Bootstrap(globalDir)

	lingtaiDir := filepath.Join(projectDir, ".lingtai")

	// Check for phantom processes before creating .lingtai/
	if _, err := os.Stat(lingtaiDir); os.IsNotExist(err) {
		self, _ := os.Executable()
		out, _ := exec.Command(self, "list", projectDir).Output()
		if len(out) > 0 && strings.Contains(string(out), "[PHANTOM]") {
			fmt.Print(string(out))
			os.Exit(1)
		}
	}

	if err := process.InitProject(lingtaiDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	tutorialDir := filepath.Join(lingtaiDir, "tutorial")

	// Kill old tutorial if running, then wipe
	fs.SuspendAndWait(tutorialDir, 3*time.Second)
	os.RemoveAll(tutorialDir)
	p := preset.First()
	globalCfg, _ := config.LoadConfig(globalDir)
	lang := globalCfg.Language
	if lang == "" {
		lang = "en"
	}
	if err := preset.GenerateTutorialInit(p, lingtaiDir, globalDir, lang); err != nil {
		fmt.Fprintf(os.Stderr, "error creating tutorial: %v\n", err)
		os.Exit(1)
	}
	humanAddr, _ := filepath.Abs(filepath.Join(lingtaiDir, "human"))
	fs.WritePrompt(tutorialDir, "You have just been created as the tutorial guide. A new user is waiting. Send them a welcome email to introduce yourself and begin Lesson 1. The human's email address is: "+humanAddr)
	config.MarkTutorialDone(globalDir)

	// Launch tutorial agent if not alive
	lingtaiCmd := config.LingtaiCmd(globalDir)
	if lingtaiCmd != "" && !fs.IsAlive(tutorialDir, 2.0) {
		if _, err := process.LaunchAgent(lingtaiCmd, tutorialDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to launch tutorial: %v\n", err)
		}
	}

	// Start API server
	srv := api.NewServer(lingtaiDir, WebFS())
	portFile := filepath.Join(lingtaiDir, ".port")
	if err := srv.Start(portFile); err != nil {
		fmt.Fprintf(os.Stderr, "error starting server: %v\n", err)
		os.Exit(1)
	}
	defer srv.Stop(context.Background())

	// Load settings
	settings := tui.LoadSettings(lingtaiDir)
	if globalCfg.Language != "" {
		i18n.SetLang(globalCfg.Language)
	} else {
		i18n.SetLang(settings.Language)
	}

	// Launch TUI directly into the tutorial agent
	app := tui.NewApp(globalDir, lingtaiDir, srv.URL(), false, []string{"tutorial"}, settings)
	prog := tea.NewProgram(app)
	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func suspendMain() {
	projectDir, _ := os.Getwd()
	projectDir, _ = filepath.Abs(projectDir)

	lingtaiDir := filepath.Join(projectDir, ".lingtai")
	if _, err := os.Stat(lingtaiDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "No .lingtai/ found in %s\n", projectDir)
		os.Exit(1)
	}

	agents, err := fs.DiscoverAgents(lingtaiDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error discovering agents: %v\n", err)
		os.Exit(1)
	}

	count := 0
	for _, agent := range agents {
		if agent.IsHuman {
			continue
		}
		if !fs.IsAlive(agent.WorkingDir, 3.0) {
			continue
		}
		fmt.Printf("Suspending %s...\n", agent.AgentName)
		fs.SuspendAndWait(agent.WorkingDir, 5*time.Second)
		count++
	}

	if count == 0 {
		fmt.Println("No active agents found.")
	} else {
		fmt.Printf("Suspended %d agent(s).\n", count)
	}
}

// purgeMain is defined in purge_unix.go / purge_windows.go
