package main

import (
	"fmt"
	"os"
	"path/filepath"

	"lingtai-daemon/internal/i18n"
	"lingtai-daemon/internal/setup"
	"lingtai-daemon/internal/tui"
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

	cwd, _ := os.Getwd()
	lingtaiDir := filepath.Join(cwd, ".lingtai")
	configPath := filepath.Join(lingtaiDir, "configs", "config.json")

	// lingtai setup — standalone wizard
	if len(positional) > 0 {
		switch positional[0] {
		case "setup":
			os.MkdirAll(lingtaiDir, 0755)
			if err := setup.Run(lingtaiDir); err != nil {
				fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
				os.Exit(1)
			}
			return
		case "help", "--help", "-h":
			printHelp()
			return
		}
	}

	// Determine initial view
	opts := tui.RootOpts{
		LingtaiDir: lingtaiDir,
		ConfigPath: configPath,
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// No .lingtai/ — start with wizard
		opts.InitialView = tui.ViewWizard
	} else {
		// .lingtai/ exists — start with status
		opts.InitialView = tui.ViewStatus
	}

	tui.RunTUI(opts)
}

func printHelp() {
	fmt.Printf(`
  灵台 LingTai — agent framework

  Usage:
    lingtai              Start TUI (setup if first time)
    lingtai setup        (Re)configure current directory

  Flags:
    --lang <code>        UI language (en, zh, lzh)

  Run lingtai in any directory. It uses .lingtai/ in the current
  directory — like git uses .git/.

  Provider configs are saved as "combos" at ~/.lingtai/combos/
  for reuse across projects.

`)
}
