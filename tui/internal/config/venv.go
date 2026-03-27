package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// RuntimeVenvDir returns ~/.lingtai/runtime/venv/.
func RuntimeVenvDir(globalDir string) string {
	return filepath.Join(globalDir, "runtime", "venv")
}

// VenvDir returns the old ~/.lingtai/env/ path for migration detection.
func VenvDir(globalDir string) string {
	return filepath.Join(globalDir, "env")
}

// VenvPython returns the Python executable path inside a venv directory.
func VenvPython(venvDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvDir, "Scripts", "python.exe")
	}
	return filepath.Join(venvDir, "bin", "python")
}

// LingtaiCmd returns the Python interpreter path for running lingtai.
// Callers should invoke as: LingtaiCmd(dir), "-m", "lingtai", "run", agentDir
func LingtaiCmd(globalDir string) string {
	// Try new path: ~/.lingtai/runtime/venv/
	python := VenvPython(RuntimeVenvDir(globalDir))
	if _, err := os.Stat(python); err == nil {
		return python
	}
	// Try legacy path: ~/.lingtai/env/
	python = VenvPython(VenvDir(globalDir))
	if _, err := os.Stat(python); err == nil {
		return python
	}
	// Fallback: python on PATH (dev mode)
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return VenvPython(RuntimeVenvDir(globalDir))
}

// NeedsVenv returns true if no working runtime venv exists.
func NeedsVenv(globalDir string) bool {
	// Check new path
	if _, err := os.Stat(VenvPython(RuntimeVenvDir(globalDir))); err == nil {
		return false
	}
	// Check legacy path
	if _, err := os.Stat(VenvPython(VenvDir(globalDir))); err == nil {
		return false
	}
	return true
}

func VerifyVenv(globalDir string) error {
	if NeedsVenv(globalDir) {
		return nil
	}
	python := LingtaiCmd(globalDir)
	cmd := exec.Command(python, "-c", "import lingtai")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("lingtai installation is broken. Delete ~/.lingtai/runtime/venv/ and restart to reinstall")
	}
	return nil
}

func EnsureVenv(globalDir string) error {
	if !NeedsVenv(globalDir) {
		return nil
	}
	venvPath := RuntimeVenvDir(globalDir)
	pythonCmd := findPython()
	if pythonCmd == "" {
		return fmt.Errorf("Python 3.11+ is required. Install it from python.org and try again")
	}
	os.MkdirAll(filepath.Dir(venvPath), 0o755)
	cmd := exec.Command(pythonCmd, "-m", "venv", venvPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create venv: %w", err)
	}
	var pipCmd string
	if runtime.GOOS == "windows" {
		pipCmd = filepath.Join(venvPath, "Scripts", "pip.exe")
	} else {
		pipCmd = filepath.Join(venvPath, "bin", "pip")
	}
	install := exec.Command(pipCmd, "install", "lingtai")
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("failed to install lingtai. Check your internet connection and try again: %w", err)
	}
	return nil
}

func findPython() string {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
