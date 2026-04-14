package timemachine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// initGit initializes a git repo in lingtaiDir if one doesn't exist.
// Configures user identity for the time machine commits.
func initGit(lingtaiDir string) error {
	gitDir := filepath.Join(lingtaiDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return nil // already initialized
	}

	if err := git(lingtaiDir, "init"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if err := git(lingtaiDir, "config", "user.email", "timemachine@lingtai"); err != nil {
		return fmt.Errorf("git config email: %w", err)
	}
	if err := git(lingtaiDir, "config", "user.name", "灵台 Time Machine"); err != nil {
		return fmt.Errorf("git config name: %w", err)
	}

	// Initial commit (empty or with .gitignore if it exists)
	gitignore := filepath.Join(lingtaiDir, ".gitignore")
	if _, err := os.Stat(gitignore); err == nil {
		git(lingtaiDir, "add", ".gitignore")
	}
	return git(lingtaiDir, "commit", "--allow-empty", "-m", "init: time machine")
}

// snapshot stages all changes and commits. Returns true if a commit was made.
func snapshot(lingtaiDir string) (bool, error) {
	if err := git(lingtaiDir, "add", "-A"); err != nil {
		return false, fmt.Errorf("git add: %w", err)
	}

	// Check if anything is staged
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = lingtaiDir
	if err := cmd.Run(); err == nil {
		return false, nil // nothing staged
	}

	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	if err := git(lingtaiDir, "commit", "-m", "snapshot "+ts); err != nil {
		return false, fmt.Errorf("git commit: %w", err)
	}
	return true, nil
}

// scanLargeFiles walks lingtaiDir for files exceeding maxBytes and appends
// them to .gitignore. Paths are relative to lingtaiDir.
func scanLargeFiles(lingtaiDir string, maxBytes int64) {
	gitignorePath := filepath.Join(lingtaiDir, ".gitignore")
	existing, _ := os.ReadFile(gitignorePath)

	// Build set of existing gitignore lines for exact matching
	ignoredLines := make(map[string]bool)
	for _, line := range strings.Split(string(existing), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			ignoredLines[line] = true
		}
	}

	var toAdd []string

	filepath.Walk(lingtaiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if info.Size() > maxBytes {
			rel, err := filepath.Rel(lingtaiDir, path)
			if err != nil {
				return nil
			}
			if !ignoredLines[rel] {
				toAdd = append(toAdd, rel)
			}
		}
		return nil
	})

	if len(toAdd) > 0 {
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		for _, rel := range toAdd {
			f.WriteString(rel + "\n")
		}
	}
}

// git runs a git command in the given directory.
func git(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}
