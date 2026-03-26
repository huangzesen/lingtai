package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func VenvDir(globalDir string) string {
	return filepath.Join(globalDir, "env")
}

func LingtaiCmd(globalDir string) string {
	// Prefer managed venv
	venv := VenvDir(globalDir)
	var venvCmd string
	if runtime.GOOS == "windows" {
		venvCmd = filepath.Join(venv, "Scripts", "lingtai.exe")
	} else {
		venvCmd = filepath.Join(venv, "bin", "lingtai")
	}
	if _, err := os.Stat(venvCmd); err == nil {
		return venvCmd
	}
	// Fallback: lingtai on PATH (dev mode)
	if path, err := exec.LookPath("lingtai"); err == nil {
		return path
	}
	return venvCmd // return venv path anyway — caller handles missing
}

func NeedsVenv(globalDir string) bool {
	_, err := os.Stat(LingtaiCmd(globalDir))
	return os.IsNotExist(err)
}

func VerifyVenv(globalDir string) error {
	if NeedsVenv(globalDir) {
		return nil
	}
	venv := VenvDir(globalDir)
	var pythonCmd string
	if runtime.GOOS == "windows" {
		pythonCmd = filepath.Join(venv, "Scripts", "python.exe")
	} else {
		pythonCmd = filepath.Join(venv, "bin", "python")
	}
	cmd := exec.Command(pythonCmd, "-c", "import lingtai")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("lingtai installation is broken. Delete ~/.lingtai/env/ and restart to reinstall")
	}
	return nil
}

func EnsureVenv(globalDir string) error {
	if !NeedsVenv(globalDir) {
		return nil
	}
	venvPath := VenvDir(globalDir)
	pythonCmd := findPython()
	if pythonCmd == "" {
		return fmt.Errorf("Python 3.11+ is required. Install it from python.org and try again")
	}
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
	install := exec.Command(pipCmd, "install", "lingtai[minimax]")
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
