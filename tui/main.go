package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
			fmt.Println("Usage: lingtai-tui [project-dir]")
			fmt.Println("       lingtai-tui tutorial [project-dir]")
			fmt.Println()
			fmt.Println("  project-dir  Path to the project (default: current directory)")
			fmt.Println("  tutorial     Start or resume the guided tutorial")
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
	}

	// Resolve project directory
	var projectDir string
	if len(os.Args) > 1 {
		projectDir = os.Args[1]
	} else {
		projectDir, _ = os.Getwd()
	}
	projectDir, _ = filepath.Abs(projectDir)

	// Global config directory (~/.lingtai)
	globalDir, err := config.GlobalDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	lingtaiDir := filepath.Join(projectDir, ".lingtai")

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

	// First run = no config.json in ~/.lingtai/
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
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func tutorialMain() {
	// Resolve project directory (optional second arg)
	var projectDir string
	if len(os.Args) > 2 {
		projectDir = os.Args[2]
	} else {
		projectDir, _ = os.Getwd()
	}
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
	}
	preset.Bootstrap(globalDir)

	lingtaiDir := filepath.Join(projectDir, ".lingtai")
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
	prog := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := prog.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
