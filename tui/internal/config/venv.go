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
	return ensureVenv(globalDir, false, nil)
}

// ProgressFunc is called with an i18n key to report setup progress.
type ProgressFunc func(key string)

// EnsureVenvQuiet creates the venv without writing to stdout/stderr.
// Used when running inside the TUI (alt-screen).
func EnsureVenvQuiet(globalDir string, progress ProgressFunc) error {
	return ensureVenv(globalDir, true, progress)
}

func ensureVenv(globalDir string, quiet bool, progress ProgressFunc) error {
	if progress == nil {
		progress = func(string) {}
	}
	if !NeedsVenv(globalDir) {
		return nil
	}
	venvPath := RuntimeVenvDir(globalDir)
	uvCmd := findUV()

	// Step 1: create venv
	progress("welcome.step_venv")
	os.MkdirAll(filepath.Dir(venvPath), 0o755)
	var cmd *exec.Cmd
	if uvCmd != "" {
		cmd = exec.Command(uvCmd, "venv", venvPath)
	} else {
		pythonCmd := findPython()
		if pythonCmd == "" {
			return fmt.Errorf("Python 3.11+ is required. Install it from python.org and try again")
		}
		cmd = exec.Command(pythonCmd, "-m", "venv", venvPath)
	}
	if !quiet {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create venv: %w", err)
	}

	// Step 2: install lingtai
	progress("welcome.step_install")
	home, _ := os.UserHomeDir()
	kernelSrc := filepath.Join(home, "Documents", "GitHub", "lingtai-kernel")
	lingtaiSrc := filepath.Join(home, "Documents", "GitHub", "lingtai")
	var install *exec.Cmd
	if uvCmd != "" {
		// uv pip install — use -p to target the venv
		if _, err := os.Stat(filepath.Join(lingtaiSrc, "pyproject.toml")); err == nil {
			args := []string{"pip", "install", "-e", lingtaiSrc, "-p", venvPath}
			if _, err := os.Stat(filepath.Join(kernelSrc, "pyproject.toml")); err == nil {
				args = []string{"pip", "install", "-e", kernelSrc, "-e", lingtaiSrc, "-p", venvPath}
			}
			install = exec.Command(uvCmd, args...)
		} else {
			install = exec.Command(uvCmd, "pip", "install", "lingtai", "-p", venvPath)
		}
	} else {
		// Fallback: pip from the venv
		var pipCmd string
		if runtime.GOOS == "windows" {
			pipCmd = filepath.Join(venvPath, "Scripts", "pip.exe")
		} else {
			pipCmd = filepath.Join(venvPath, "bin", "pip")
		}
		if _, err := os.Stat(filepath.Join(lingtaiSrc, "pyproject.toml")); err == nil {
			args := []string{"install", "-e", lingtaiSrc}
			if _, err := os.Stat(filepath.Join(kernelSrc, "pyproject.toml")); err == nil {
				args = []string{"install", "-e", kernelSrc, "-e", lingtaiSrc}
			}
			install = exec.Command(pipCmd, args...)
		} else {
			install = exec.Command(pipCmd, "install", "lingtai")
		}
	}
	if !quiet {
		install.Stdout = os.Stdout
		install.Stderr = os.Stderr
	}
	if err := install.Run(); err != nil {
		return fmt.Errorf("failed to install lingtai. Check your internet connection and try again: %w", err)
	}

	// Step 3: verify installation
	progress("welcome.step_verify")
	python := VenvPython(venvPath)
	verify := exec.Command(python, "-c", "import lingtai; print(lingtai.__version__)")
	if !quiet {
		verify.Stdout = os.Stdout
		verify.Stderr = os.Stderr
	}
	if err := verify.Run(); err != nil {
		return fmt.Errorf("lingtai installed but import failed — check for missing dependencies: %w", err)
	}
	return nil
}

func findUV() string {
	if path, err := exec.LookPath("uv"); err == nil {
		return path
	}
	return ""
}

func findPython() string {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}
