# Homebrew Distribution + Auto-Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Distribute lingtai-tui via Homebrew and auto-upgrade the Python package on every TUI launch.

**Architecture:** Homebrew tap repo (`huangzesen/homebrew-lingtai`) serves the TUI binary from GitHub releases. A GitHub Actions workflow cross-compiles the TUI for 4 platforms on tag push and creates a release. The TUI's `venv.go` gains a `CheckUpgrade()` function that queries PyPI on startup.

**Tech Stack:** Go (TUI), GitHub Actions (CI), Homebrew (distribution), PyPI JSON API (version check)

---

### Task 1: Add CheckUpgrade to venv.go

**Files:**
- Modify: `tui/internal/config/venv.go`

- [ ] **Step 1: Add imports**

At the top of `tui/internal/config/venv.go`, add the required imports. The existing import block has `fmt`, `os`, `os/exec`, `path/filepath`, `runtime`. Add `encoding/json`, `net/http`, `strings`, `time`:

```go
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
```

- [ ] **Step 2: Add CheckUpgrade function**

Append this function to `tui/internal/config/venv.go`, after the `findPython()` function (end of file):

```go
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
```

- [ ] **Step 3: Build to verify compilation**

Run from `tui/`:
```bash
cd tui && go build -o bin/lingtai-tui .
```
Expected: clean build, no errors.

- [ ] **Step 4: Commit**

```bash
git add tui/internal/config/venv.go
git commit -m "feat(tui): add CheckUpgrade — query PyPI and auto-upgrade lingtai on launch"
```

---

### Task 2: Call CheckUpgrade from main.go

**Files:**
- Modify: `tui/main.go`

- [ ] **Step 1: Add CheckUpgrade call after NeedsVenv block**

In `tui/main.go`, find the block (around line 119-128):

```go
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
```

Replace with:

```go
	if !needsFirstRun {
		// Returning user — ensure runtime + assets (fast no-ops if already exist)
		if config.NeedsVenv(globalDir) {
			fmt.Println("Setting up Python environment...")
			if err := config.EnsureVenv(globalDir); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			}
		} else {
			// Venv exists — check for lingtai upgrades
			if config.CheckUpgrade(globalDir) {
				fmt.Println("Upgraded lingtai to latest version.")
			}
		}
		preset.Bootstrap(globalDir)
	}
```

- [ ] **Step 2: Add CheckUpgrade call in tutorialMain path**

Find the similar `NeedsVenv` block in the `tutorialMain` function (around line 174-181):

```go
	if config.NeedsVenv(globalDir) {
		fmt.Println("Setting up Python environment...")
		if err := config.EnsureVenv(globalDir); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
	preset.Bootstrap(globalDir)
```

Replace with:

```go
	if config.NeedsVenv(globalDir) {
		fmt.Println("Setting up Python environment...")
		if err := config.EnsureVenv(globalDir); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	} else {
		if config.CheckUpgrade(globalDir) {
			fmt.Println("Upgraded lingtai to latest version.")
		}
	}
	preset.Bootstrap(globalDir)
```

- [ ] **Step 3: Build to verify**

```bash
cd tui && go build -o bin/lingtai-tui .
```
Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add tui/main.go
git commit -m "feat(tui): check for lingtai upgrades on every launch"
```

---

### Task 3: Delete install.sh

**Files:**
- Delete: `install.sh`

- [ ] **Step 1: Remove install.sh**

```bash
git rm install.sh
```

- [ ] **Step 2: Commit**

```bash
git commit -m "chore: remove install.sh — replaced by brew install huangzesen/lingtai/lingtai-tui"
```

---

### Task 4: Create GitHub Actions release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create the workflow file**

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  build:
    strategy:
      matrix:
        include:
          - goos: darwin
            goarch: arm64
            asset: lingtai-darwin-arm64
            runner: macos-14
          - goos: darwin
            goarch: amd64
            asset: lingtai-darwin-x64
            runner: macos-13
          - goos: linux
            goarch: amd64
            asset: lingtai-linux-x64
            runner: ubuntu-latest
          - goos: linux
            goarch: arm64
            asset: lingtai-linux-arm64
            runner: ubuntu-latest

    runs-on: ${{ matrix.runner }}

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: tui/go.mod

      - name: Build
        working-directory: tui
        env:
          CGO_ENABLED: '0'
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
        run: go build -o ../dist/${{ matrix.asset }} .

      - uses: actions/upload-artifact@v4
        with:
          name: ${{ matrix.asset }}
          path: dist/${{ matrix.asset }}

  release:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          path: dist
          merge-multiple: true

      - name: Create release
        uses: softprops/action-gh-release@v2
        with:
          files: dist/*
          generate_release_notes: true

  update-homebrew:
    needs: release
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          path: dist
          merge-multiple: true

      - name: Compute checksums
        id: sha
        run: |
          echo "darwin_arm64=$(shasum -a 256 dist/lingtai-darwin-arm64 | cut -d' ' -f1)" >> "$GITHUB_OUTPUT"
          echo "darwin_x64=$(shasum -a 256 dist/lingtai-darwin-x64 | cut -d' ' -f1)" >> "$GITHUB_OUTPUT"
          echo "linux_x64=$(shasum -a 256 dist/lingtai-linux-x64 | cut -d' ' -f1)" >> "$GITHUB_OUTPUT"
          echo "linux_arm64=$(shasum -a 256 dist/lingtai-linux-arm64 | cut -d' ' -f1)" >> "$GITHUB_OUTPUT"

      - name: Update Homebrew formula
        uses: actions/checkout@v4
        with:
          repository: huangzesen/homebrew-lingtai
          token: ${{ secrets.HOMEBREW_TAP_TOKEN }}
          path: homebrew-tap

      - name: Write formula
        env:
          TAG: ${{ github.ref_name }}
        run: |
          cat > homebrew-tap/lingtai-tui.rb << 'FORMULA'
          class LingtaiTui < Formula
            desc "Terminal UI for the Lingtai AI agent framework"
            homepage "https://github.com/huangzesen/lingtai"
            version "$VERSION"
            license "MIT"

            on_macos do
              on_arm do
                url "https://github.com/huangzesen/lingtai/releases/download/$TAG/lingtai-darwin-arm64"
                sha256 "$SHA_DARWIN_ARM64"
              end
              on_intel do
                url "https://github.com/huangzesen/lingtai/releases/download/$TAG/lingtai-darwin-x64"
                sha256 "$SHA_DARWIN_X64"
              end
            end

            on_linux do
              on_arm do
                url "https://github.com/huangzesen/lingtai/releases/download/$TAG/lingtai-linux-arm64"
                sha256 "$SHA_LINUX_ARM64"
              end
              on_intel do
                url "https://github.com/huangzesen/lingtai/releases/download/$TAG/lingtai-linux-x64"
                sha256 "$SHA_LINUX_X64"
              end
            end

            def install
              bin.install stable.url.split("/").last => "lingtai-tui"
            end

            test do
              assert_match "lingtai-tui", shell_output("#{bin}/lingtai-tui version 2>&1", 0)
            end
          end
          FORMULA
          # Substitute variables
          VERSION="${TAG#v}"
          sed -i "s/\$VERSION/$VERSION/g" homebrew-tap/lingtai-tui.rb
          sed -i "s/\$TAG/$TAG/g" homebrew-tap/lingtai-tui.rb
          sed -i "s/\$SHA_DARWIN_ARM64/${{ steps.sha.outputs.darwin_arm64 }}/g" homebrew-tap/lingtai-tui.rb
          sed -i "s/\$SHA_DARWIN_X64/${{ steps.sha.outputs.darwin_x64 }}/g" homebrew-tap/lingtai-tui.rb
          sed -i "s/\$SHA_LINUX_X64/${{ steps.sha.outputs.linux_x64 }}/g" homebrew-tap/lingtai-tui.rb
          sed -i "s/\$SHA_LINUX_ARM64/${{ steps.sha.outputs.linux_arm64 }}/g" homebrew-tap/lingtai-tui.rb

      - name: Push formula update
        working-directory: homebrew-tap
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add lingtai-tui.rb
          git commit -m "Update lingtai-tui to ${{ github.ref_name }}"
          git push
```

- [ ] **Step 2: Commit**

```bash
mkdir -p .github/workflows
git add .github/workflows/release.yml
git commit -m "ci: add release workflow — cross-compile TUI + GitHub release + update Homebrew"
```

---

### Task 5: Create Homebrew tap repository

**Files:**
- Create: `huangzesen/homebrew-lingtai` repo on GitHub
- Create: `lingtai-tui.rb` in that repo

This task is manual (GitHub web UI or `gh` CLI) — not in the lingtai repo.

- [ ] **Step 1: Create the repository**

```bash
gh repo create huangzesen/homebrew-lingtai --public --description "Homebrew tap for lingtai-tui"
```

- [ ] **Step 2: Clone and add initial formula**

```bash
cd /tmp
git clone https://github.com/huangzesen/homebrew-lingtai.git
cd homebrew-lingtai
```

Create `lingtai-tui.rb` with placeholder SHA values (the release workflow will overwrite these on first tag):

```ruby
class LingtaiTui < Formula
  desc "Terminal UI for the Lingtai AI agent framework"
  homepage "https://github.com/huangzesen/lingtai"
  version "0.3.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/huangzesen/lingtai/releases/download/v0.3.0/lingtai-darwin-arm64"
      sha256 "PLACEHOLDER"
    end
    on_intel do
      url "https://github.com/huangzesen/lingtai/releases/download/v0.3.0/lingtai-darwin-x64"
      sha256 "PLACEHOLDER"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/huangzesen/lingtai/releases/download/v0.3.0/lingtai-linux-arm64"
      sha256 "PLACEHOLDER"
    end
    on_intel do
      url "https://github.com/huangzesen/lingtai/releases/download/v0.3.0/lingtai-linux-x64"
      sha256 "PLACEHOLDER"
    end
  end

  def install
    bin.install stable.url.split("/").last => "lingtai-tui"
  end

  test do
    assert_match "lingtai-tui", shell_output("#{bin}/lingtai-tui version 2>&1", 0)
  end
end
```

- [ ] **Step 3: Commit and push**

```bash
git add lingtai-tui.rb
git commit -m "Initial formula for lingtai-tui"
git push
```

- [ ] **Step 4: Create HOMEBREW_TAP_TOKEN secret**

In the `huangzesen/lingtai` repo settings → Secrets → Actions, add `HOMEBREW_TAP_TOKEN` — a GitHub personal access token with `repo` scope for `huangzesen/homebrew-lingtai`.

---

### Task 6: Test the full release flow

- [ ] **Step 1: Push all changes**

```bash
git push origin main
```

- [ ] **Step 2: Create a tag and push it**

```bash
git tag v0.3.0
git push origin v0.3.0
```

- [ ] **Step 3: Verify the GitHub Actions workflow**

Go to https://github.com/huangzesen/lingtai/actions — the release workflow should:
1. Build 4 binaries (check all 4 matrix jobs pass)
2. Create a GitHub release at https://github.com/huangzesen/lingtai/releases with 4 assets
3. Update the formula in `huangzesen/homebrew-lingtai` with correct SHA values

- [ ] **Step 4: Test brew install**

```bash
brew install huangzesen/lingtai/lingtai-tui
which lingtai-tui
lingtai-tui version
```

Expected: installs successfully, binary on PATH, prints version.

- [ ] **Step 5: Commit any fixes**

If any adjustments were needed during testing, commit them.

---

### Task 7: Update website install command

**Files:**
- Modify: lingtai-web repo (the website at lingtai.ai)

- [ ] **Step 1: Update the install command on the homepage**

The website currently shows:
```
$ curl -fsSL https://raw.githubusercontent.com/huangzesen/lingtai/main/install.sh | sh
```

Change to:
```
$ brew install huangzesen/lingtai/lingtai-tui
```

- [ ] **Step 2: Commit and deploy**

Commit the change and deploy the website.
