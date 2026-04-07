//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/lingtai-tui/internal/fs"
)

func suspendMain() {
	// Optional project dir from os.Args[2]; defaults to cwd
	var projectDir string
	if len(os.Args) > 2 {
		projectDir, _ = filepath.Abs(os.Args[2])
	} else {
		projectDir, _ = os.Getwd()
		projectDir, _ = filepath.Abs(projectDir)
	}
	lingtaiDir := filepath.Join(projectDir, ".lingtai")

	if _, err := os.Stat(lingtaiDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "No .lingtai\\ found in %s\n", projectDir)
		os.Exit(1)
	}

	// Discover agents by scanning subdirs that have .agent.json
	entries, err := os.ReadDir(lingtaiDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot read .lingtai/: %v\n", err)
		os.Exit(1)
	}

	suspended := 0
	skipped := 0
	failed := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agentDir := filepath.Join(lingtaiDir, entry.Name())
		manifestPath := filepath.Join(agentDir, ".agent.json")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		// Skip human
		node, err := fs.ReadAgent(agentDir)
		if err == nil && node.IsHuman {
			skipped++
			continue
		}

		suspendFile := filepath.Join(agentDir, ".suspend")
		if err := os.WriteFile(suspendFile, []byte{}, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to suspend %s: %v\n", entry.Name(), err)
			continue
		}

		// Wait for the agent to actually stop before reporting success
		fs.SuspendAndWait(agentDir, 5*time.Second)
		if fs.IsAlive(agentDir, 2.0) {
			fmt.Printf("Warning: %s did not stop (still alive after 5s)\n", entry.Name())
			failed++
		} else {
			fmt.Printf("Suspended: %s\n", entry.Name())
			suspended++
		}
	}

	if suspended == 0 && failed == 0 {
		fmt.Println("No agents to suspend.")
	} else {
		if failed > 0 {
			fmt.Printf("Suspended %d agent(s). %d did not stop.\n", suspended, failed)
		} else {
			fmt.Printf("Suspended %d agent(s). %d skipped (human).\n", suspended, skipped)
		}
		fmt.Println("Run 'lingtai-tui list' to check status.")
	}
}
