package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// RuntimeVenvDir returns ~/.lingtai-tui/runtime/venv/.
func RuntimeVenvDir(globalDir string) string {
	return filepath.Join(globalDir, "runtime", "venv")
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
	python := VenvPython(RuntimeVenvDir(globalDir))
	if _, err := os.Stat(python); err == nil {
		return python
	}
	// Fallback: python on PATH (dev mode)
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return python
}

// NeedsVenv returns true if no working runtime venv exists
// or if lingtai is not importable inside it.
func NeedsVenv(globalDir string) bool {
	python := VenvPython(RuntimeVenvDir(globalDir))
	if _, err := os.Stat(python); err != nil {
		return true
	}
	// Venv exists — verify lingtai is importable
	if err := exec.Command(python, "-c", "import lingtai").Run(); err != nil {
		return true
	}
	return false
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
		// uv can download Python automatically — request 3.13 to avoid conda/system conflicts
		cmd = exec.Command(uvCmd, "venv", "--python", "3.13", venvPath)
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

	// Verify Python version is 3.11+
	venvPython := VenvPython(venvPath)
	verOut, err := exec.Command(venvPython, "-c",
		"import sys; print(sys.version_info >= (3, 11))").Output()
	if err != nil || strings.TrimSpace(string(verOut)) != "True" {
		os.RemoveAll(venvPath)
		return fmt.Errorf("Python 3.11+ is required. Found older version in venv. Install python@3.13 and try again")
	}

	// Step 2: install lingtai
	progress("welcome.step_install")
	home, _ := os.UserHomeDir()
	kernelSrc := filepath.Join(home, "Documents", "GitHub", "lingtai-kernel")
	lingtaiSrc := filepath.Join(home, "Documents", "GitHub", "lingtai")
	_, hasKernel := os.Stat(filepath.Join(kernelSrc, "pyproject.toml"))
	_, hasLingtai := os.Stat(filepath.Join(lingtaiSrc, "pyproject.toml"))
	devMode := hasKernel == nil && hasLingtai == nil

	var install *exec.Cmd
	if uvCmd != "" {
		if devMode {
			install = exec.Command(uvCmd, "pip", "install", "-e", kernelSrc, "-e", lingtaiSrc, "-p", venvPath)
		} else {
			install = exec.Command(uvCmd, "pip", "install", "lingtai", "-p", venvPath)
		}
	} else {
		var pipCmd string
		if runtime.GOOS == "windows" {
			pipCmd = filepath.Join(venvPath, "Scripts", "pip.exe")
		} else {
			pipCmd = filepath.Join(venvPath, "bin", "pip")
		}
		if devMode {
			install = exec.Command(pipCmd, "install", "-e", kernelSrc, "-e", lingtaiSrc)
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

// CheckTUIUpgrade compares the running TUI version against the latest GitHub release.
// Returns the latest version string if an upgrade is available, or "" if up-to-date.
// Non-blocking: silently returns "" on any error (offline, timeout, etc.).
func CheckTUIUpgrade(currentVersion string) string {
	if currentVersion == "" || currentVersion == "dev" {
		return ""
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/huangzesen/lingtai/releases/latest")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ""
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ""
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(currentVersion, "v")

	if latest != current {
		return release.TagName
	}
	return ""
}

// CheckUpgrade compares installed lingtai version to PyPI latest.
// Runs pip install --upgrade if a newer version is available.
// Returns true if an upgrade was performed.
// Non-blocking: silently returns false on any error (offline, timeout, etc.).
func CheckUpgrade(globalDir string) bool {
	python := VenvPython(RuntimeVenvDir(globalDir))
	if _, err := os.Stat(python); err != nil {
		return false // no venv yet
	}

	// Get installed version
	out, err := exec.Command(python, "-c",
		"import lingtai; print(lingtai.__version__)").Output()
	if err != nil {
		return false
	}
	installed := strings.TrimSpace(string(out))

	// Get latest from PyPI (3s timeout)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://pypi.org/pypi/lingtai/json")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}

	var pypi struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pypi); err != nil {
		return false
	}

	if installed == pypi.Info.Version {
		return false
	}

	// Upgrade
	var pipCmd string
	if runtime.GOOS == "windows" {
		pipCmd = filepath.Join(filepath.Dir(python), "pip.exe")
	} else {
		pipCmd = filepath.Join(filepath.Dir(python), "pip")
	}
	uvCmd := findUV()
	if uvCmd != "" {
		exec.Command(uvCmd, "pip", "install", "--upgrade", "lingtai",
			"-p", RuntimeVenvDir(globalDir)).Run()
	} else {
		exec.Command(pipCmd, "install", "--upgrade", "lingtai").Run()
	}
	return true
}
