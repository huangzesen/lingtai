package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"lingtai-daemon/internal/agent"
	"lingtai-daemon/internal/config"
	"lingtai-daemon/internal/i18n"
	"lingtai-daemon/internal/manage"
	"lingtai-daemon/internal/setup"
	"lingtai-daemon/internal/tui"
)

func main() {
	args := os.Args[1:]
	configPath := "config.json"
	headless := false

	// Parse flags
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 < len(args) {
				configPath = args[i+1]
				i++
			}
		case "--headless":
			headless = true
		case "--lang":
			if i+1 < len(args) {
				i18n.Lang = args[i+1]
				i++
			}
		default:
			positional = append(positional, args[i])
		}
	}

	// Subcommands
	if len(positional) > 0 {
		switch positional[0] {
		case "setup":
			outputDir := "."
			if len(positional) > 1 {
				outputDir = positional[1]
			}
			if err := setup.Run(outputDir); err != nil {
				fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
				os.Exit(1)
			}
			return
		case "manage":
			baseDir := "~/.lingtai"
			for i, arg := range args {
				if arg == "--base-dir" && i+1 < len(args) {
					baseDir = args[i+1]
				}
			}
			if strings.HasPrefix(baseDir, "~") {
				home, _ := os.UserHomeDir()
				baseDir = filepath.Join(home, baseDir[1:])
			}
			spirits := manage.ScanSpirits(baseDir)
			fmt.Print(manage.FormatTable(spirits))
			return
		}
	}

	// Load config — if missing, run setup automatically
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("\n  \033[1m\033[36m灵台\033[0m  No config found — starting setup wizard.\n\n")
		if err := setup.Run(filepath.Dir(configPath)); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
			os.Exit(1)
		}
		fmt.Println()
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
		os.Exit(1)
	}

	// Start agent process
	proc, err := agent.Start(agent.StartOptions{
		ConfigPath: configPath,
		AgentPort:  cfg.AgentPort,
		WorkingDir: cfg.WorkingDir(),
		Headless:   headless,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
		os.Exit(1)
	}

	if headless {
		// Print meta and block
		printMeta(cfg, proc)
		fmt.Printf("  \033[2mLog: %s/daemon.log\033[0m\n", cfg.WorkingDir())
		fmt.Printf("  \033[2m%s\033[0m\n\n", i18n.S("press_ctrl_c"))

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		fmt.Printf("\n  %s\n", i18n.S("shutting_down"))
		proc.Stop()
	} else {
		// Interactive TUI
		tui.Run(cfg, proc)
		proc.Stop()
	}
}

func printMeta(cfg *config.Config, proc *agent.Process) {
	title := i18n.S("title")
	fmt.Printf("\n  \033[1m\033[36m%s\033[0m\n\n", title)
	fmt.Printf("  \033[1mAgent:\033[0m      %s\n", cfg.AgentName)
	fmt.Printf("  \033[1mWorking:\033[0m    %s\n", cfg.WorkingDir())
	fmt.Printf("  \033[1mPort:\033[0m       %d\n", cfg.AgentPort)
	fmt.Printf("  \033[1mPID:\033[0m        %d\n", proc.PID())

	if cfg.IMAP != nil {
		addr, _ := cfg.IMAP["email_address"].(string)
		fmt.Printf("  \033[1mIMAP:\033[0m       \033[32m● %s\033[0m\n", addr)
	} else {
		fmt.Printf("  \033[1mIMAP:\033[0m       \033[2mdisabled\033[0m\n")
	}

	if cfg.Telegram != nil {
		fmt.Printf("  \033[1mTelegram:\033[0m   \033[32m● enabled\033[0m\n")
	} else {
		fmt.Printf("  \033[1mTelegram:\033[0m   \033[2mdisabled\033[0m\n")
	}

	if cfg.CLI {
		fmt.Printf("  \033[1mCLI:\033[0m        \033[32m● interactive\033[0m\n")
	} else {
		fmt.Printf("  \033[1mCLI:\033[0m        \033[2mdisabled\033[0m\n")
	}
	fmt.Println()
}
