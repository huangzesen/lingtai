package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/lingtai-tui/internal/api"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/process"
	"github.com/anthropics/lingtai-tui/internal/tui"
)

func main() {
	var projectDir string
	if len(os.Args) > 1 {
		projectDir = os.Args[1]
	} else {
		projectDir, _ = os.Getwd()
	}
	projectDir, _ = filepath.Abs(projectDir)

	globalDir, err := config.GlobalDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	lingtaiDir := filepath.Join(projectDir, ".lingtai")

	if err := process.InitProject(lingtaiDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	srv := api.NewServer(lingtaiDir, WebFS())
	portFile := filepath.Join(lingtaiDir, ".port")
	if err := srv.Start(portFile); err != nil {
		fmt.Fprintf(os.Stderr, "error starting server: %v\n", err)
		os.Exit(1)
	}
	defer srv.Stop(context.Background())

	fmt.Printf("Visualization: %s\n", srv.URL())

	if config.NeedsVenv(globalDir) {
		fmt.Println("Setting up Python environment (first run)...")
		if err := config.EnsureVenv(globalDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
	}

	needsSetup := config.NeedsSetup(globalDir)
	app := tui.NewApp(globalDir, lingtaiDir, srv.URL(), needsSetup)

	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
