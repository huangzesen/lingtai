#!/usr/bin/env bash
# One-shot installer for lingtai-tui and lingtai-portal, plus the Python
# `lingtai` runtime venv at ~/.lingtai-tui/runtime/venv.
#
# Homebrew is NOT required. By default this installs the latest GitHub Release:
# it downloads a prebuilt per-platform binary tarball when one exists, and
# otherwise falls back to building the release source tarball with Go/npm. If
# the installed Go is missing or older than tui/go.mod requires (distro
# packages often are), the official Go toolchain tarball is downloaded for the
# build. It then creates or updates the Python runtime venv and installs the
# `lingtai` package into it. The no-argument path remains this official stable
# release flow; use --latest explicitly for current TUI main + kernel main.
#
# Public entry point (once served from the website):
#   curl -fsSL https://lingtai.ai/install.sh | bash
#
# Direct-from-repo equivalent:
#   curl -fsSL https://raw.githubusercontent.com/Lingtai-AI/lingtai/main/install.sh | bash
#
# Install a specific release:
#   ./install.sh --version v0.10.5
#
# Binary release assets follow this naming convention (also produced by
# .github/workflows/release.yml):
#   lingtai-<tag>-<os>-<arch>.tar.gz    e.g. lingtai-v0.10.5-linux-amd64.tar.gz
# where <os> is darwin|linux and <arch> is amd64|arm64. The tarball contains
# lingtai-tui and (when built) lingtai-portal at its top level. The bundle
# manifest may also list a lingtai-<tag>-windows-amd64.zip entry for the same
# release; this installer's strict manifest parser accepts that entry (so a
# Windows archive in the manifest never fails POSIX validation) but never
# selects or downloads it — native Windows installs use install.ps1 instead.
#
# Source policy (--source auto|github|mirror, or LINGTAI_SOURCE env; default
# auto): auto runs a bounded, fail-open public-IP country lookup and prefers
# the lingtai.ai download-acceleration mirror for mainland China. GitHub
# remains the sole release/version authority either way — the mirror never
# lists releases, resolves "latest", or publishes independently; it only
# re-serves exact bytes GitHub has already published, mirrored by
# huangzesen/lingtai-web after each publisher's own upload succeeds (see
# docs/release-mirror/CONTRACT.md there). This replaces the earlier Gitee mirror;
# --source gitee is explicitly retired (rejected with a pointer to
# --source mirror), since nothing keeps a separate Gitee release in sync any
# longer. Each release publishes a small "bundle manifest" binding one exact
# TUI tag to one exact pinned kernel release/version/artifacts/checksums —
# see RELEASING.md. A provider fallback (mirror unreachable, or missing an
# asset for this exact tag) always re-fetches the SAME resolved tag/bundle
# from GitHub; it never independently re-resolves "latest" on the fallback,
# and never accepts bytes that fail their checksum. The Python `lingtai`
# runtime is installed from that pinned kernel release artifact by explicit
# local file path — never `pip install lingtai` from any package index — with
# SHA256 verified before install. Those third-party dependencies resolve via
# exactly ONE package index, chosen by python_dependency_index_url: a
# non-empty LINGTAI_PYPI_INDEX_URL always wins, otherwise the provider that
# actually served the final bundle manifest picks a provider-aligned default
# (mirror -> Tsinghua TUNA, GitHub -> pypi.org). Only lingtai's own bytes are
# pinned. If no compatible platform wheel exists for the runtime's
# interpreter, the pinned sdist is used instead (may require a local build
# toolchain).
#
# Known gap, stated honestly: the mirror accelerates the kernel wheels/sdist
# and (once TUI ships one) a per-platform TUI archive — the actual uploaded
# release assets a publisher workflow can hook after upload. It does NOT
# mirror the GitHub-auto-generated source tarball
# (archive/refs/tags/vX.Y.Z.tar.gz) that build_from_source's tag path falls
# back to, since that tarball is generated on demand by GitHub itself, not an
# asset either publisher workflow uploads. On POSIX platforms with no
# published prebuilt (true for all platforms as of TUI v1.0.8, which ships a
# Windows-only prebuilt), that source-build fetch remains a plain GitHub
# fetch regardless of --source.
#
# LingTai is NEVER installed by requesting the package name "lingtai" from
# any index — there is no PyPI fallback. On the default one-command path a
# pinned bundle is mandatory: if none can be resolved (either provider,
# same-tag fallback attempted), or the resolved bundle's kernel artifact
# fails to verify/install, the installer FAILS LOUD with the exact
# provider/tag/error rather than degrading to a package-index install.
# --ref/source-ref builds have no bundle to pin against and fail loud the
# same way. --skip-python (alias --skip-venv) is the explicit opt-out for a
# TUI/portal-only install; you then provision the Python runtime yourself.
set -euo pipefail

REPO_SLUG="Lingtai-AI/lingtai"
REPO="https://github.com/${REPO_SLUG}.git"
KERNEL_REPO_SLUG="Lingtai-AI/lingtai-kernel"
KERNEL_REPO="https://github.com/${KERNEL_REPO_SLUG}.git"
API_BASE="https://api.github.com/repos/${REPO_SLUG}"
DOWNLOAD_BASE="https://github.com/${REPO_SLUG}/releases/download"
RAW_INSTALL_URL="https://raw.githubusercontent.com/${REPO_SLUG}/main/install.sh"
GO_DL_BASE="${LINGTAI_GO_DL_BASE:-https://go.dev/dl}"  # official Go toolchain downloads
NODE_DL_BASE="${LINGTAI_NODE_DL_BASE:-https://nodejs.org/dist}"
UV_INSTALLER_URL="${LINGTAI_UV_INSTALLER_URL:-https://astral.sh/uv/install.sh}"  # official uv bootstrap installer
NODE_TOOLCHAIN_VERSION="${LINGTAI_NODE_VERSION:-22.12.0}"
DESKTOP_VERSION="0.1.10"
DESKTOP_RAW_BASE="https://raw.githubusercontent.com/Lingtai-AI/lingtai-desktop"
DESKTOP_RELEASE_BASE="https://github.com/Lingtai-AI/lingtai-desktop/releases/download"
# Audited from lingtai-desktop v0.1.10 commit
# fd39dd61e4d123b2835064c1c148566d8b36ceb0. These are deliberately not
# environment-overridable: the registered lazy bootstrap accepts only these
# exact installer-support bytes before running them.
DESKTOP_INSTALLER_SHA256="d915162c41b144fad19cd47405c36ceb5f408ca15fabd342d3b3615c53f654c9"
DESKTOP_CLI_SHA256="0a681eacdf71daea137089e68204b780f6e065184689d9b56208a67c24facc95"
DESKTOP_VERIFIER_SHA256="745374c0634709fa235cd7b63af6cd78b79f99ef5d290157d6e7bd281b3e8fc2"
DESKTOP_BOOTSTRAP_SHA256="6c246f7af6602eeee0d697bcd5c830029939bd786ba3ecbf3cf8c41846ac02e6"

# The single package index used ONLY for third-party dependencies of the
# verified local LingTai artifact — see python_dependency_index_url. Tsinghua
# TUNA is a cloud-neutral domestic PyPI mirror; it is the mirror-path default
# because pypi.org is not reliably reachable from mainland-China hosts, which
# makes dependency resolution the remaining failure point of an otherwise
# checksum-verified mirror install.
PYPI_INDEX_URL_DEFAULT="https://pypi.org/simple"
PYPI_INDEX_URL_MIRROR_DEFAULT="https://mirrors.tuna.tsinghua.edu.cn/pypi/web/simple"

KERNEL_GH_API_BASE="https://api.github.com/repos/Lingtai-AI/lingtai-kernel"
BUNDLE_TUI_ARCHIVE_SHA=""

# Country-detection endpoints for auto source selection. Two independent,
# unauthenticated, no-signup providers so one outage doesn't force a GitHub
# fallback for every mainland user; each probe is short-timeout and its
# result is discarded (fail-open) on any error. Only the two-letter country
# code of the requester's public IP is requested — no identity, no
# credentials, no persistent client. Overridable for tests/offline use.
COUNTRY_DETECT_URL_1="${LINGTAI_COUNTRY_DETECT_URL_1:-https://ipapi.co/country/}"
COUNTRY_DETECT_URL_2="${LINGTAI_COUNTRY_DETECT_URL_2:-https://ifconfig.co/country-iso}"
MIRROR_TIMEOUT="${LINGTAI_MIRROR_TIMEOUT:-3}"

# Canonical URLs for the standalone maintenance scripts (update.sh/fix.sh/
# verify.sh/dev.sh). The lingtai-web sync-installers workflow publishes those
# under help/reference/installation/assets/ while install.sh/remove.sh stay at
# the web root -- so a hint that only names the script would lead a user to
# guess the root URL and hit a 404. Overridable for tests/offline use.
LINGTAI_WEB_BASE="${LINGTAI_WEB_BASE:-https://lingtai.ai}"
LINGTAI_SCRIPTS_ASSETS="$LINGTAI_WEB_BASE/help/reference/installation/assets"

TMPDIR="${TMPDIR:-/tmp}"
BUILD_DIR="$TMPDIR/lingtai-install-$$"

# --- flags / state -----------------------------------------------------------
REF=""               # explicit source ref (branch/tag/commit) => forces source build
VERSION=""           # explicit release tag to install (default: latest release)
LATEST_MAIN_MODE=0   # --latest: explicit current-main TUI + kernel source install
UPDATE_MODE=0        # --update: re-run for an existing source/user-local install
REINSTALL_OK=0       # 1 when the default one-command path finds an existing receipt: reinstall in place (binaries + runtime refreshed; credentials/config untouched)
INSTALL_PREFIX=""    # --prefix: install root (bin_dir = <prefix>/bin)
BIN_DIR_OVERRIDE=""  # --bin-dir: explicit bin directory
NON_INTERACTIVE=0    # --non-interactive: never prompt / never sudo-install packages
FROM_SOURCE=0        # --from-source: skip release-asset download, always build
SKIP_PORTAL=0        # --skip-portal: TUI only
SKIP_VENV=0          # --skip-python (alias: --skip-venv): don't touch the Python runtime venv
SKIP_DESKTOP=0       # --skip-desktop: don't register the macOS-only lazy Desktop command
INSTALL_KIND=""      # "release-asset" | "source-build" (recorded in metadata)
SOURCE_ARG="${LINGTAI_SOURCE:-auto}"  # --source auto|github|mirror (env LINGTAI_SOURCE)
BUNDLE_PROVIDER=""    # resolved by resolve_source_provider(): "github" | "mirror"
BUNDLE_TAG=""         # resolved release tag shared by the TUI archive + bundle manifest
BUNDLE_MANIFEST_JSON="" # raw bundle manifest body, once fetched
BUNDLE_REQUIRED=0     # 1 on the default release-asset one-command path (no --ref, no --update):
                      # a pinned kernel bundle is mandatory there, so a missing/incoherent/failed
                      # bundle or kernel install must fail loud rather than silently falling back
                      # to `pip install lingtai`. 0 for --ref/source-ref builds, where no bundle is
                      # expected to exist at all — those paths require --skip-python instead (see
                      # ensure_runtime_venv).
KERNEL_SOURCE=""      # "bundle" | "release-pin" | "" (recorded in install.json only on a verified kernel install; LingTai is never installed from a package index by name — see ensure_runtime_venv)
KERNEL_BUNDLE_ID=""
KERNEL_RELEASE_TAG=""  # set when KERNEL_SOURCE=="release-pin": the exact kernel tag installed from that pin
KERNEL_VERSION_INSTALLED=""
KERNEL_PROVIDER=""
KERNEL_MANIFEST_PROVIDER=""  # set by fetch_kernel_manifest(); which provider actually served the kernel manifest
KERNEL_MANIFEST_JSON=""      # set by fetch_kernel_manifest() in the same shell as the provider
BUNDLE_MANIFEST_KERNEL_TAG=""
BUNDLE_MANIFEST_KERNEL_VERSION=""
BUNDLE_MANIFEST_KERNEL_FILENAME=""
BUNDLE_MANIFEST_BUNDLE_ID=""
KERNEL_LATEST_TAG=""         # set by resolve_latest_kernel_release(); newest published kernel release
TUI_MAIN_SHA=""
KERNEL_MAIN_SHA=""
KERNEL_SOURCE_DIR=""
# kernel-release.json exact-tag pin state (source-only TUI releases with no
# dual bundle manifest): a second, still-strict, still-exact-tag kernel source
# tried only after the bundle manifest is unavailable — see fetch_kernel_pin.
KERNEL_PIN_JSON=""
KERNEL_PIN_TAG=""
KERNEL_PIN_PROVIDER=""
KERNEL_PIN_TUI_TAG=""
RUNTIME_VENV_DIR=""   # set by ensure_runtime_venv() on success; read by write_install_metadata

usage() {
  cat <<'EOF'
One-shot installer for lingtai-tui, lingtai-portal, the Python runtime, and
LingTai Desktop on macOS.

Homebrew is not required. By default the latest release bundle is installed from the selected source:
a prebuilt per-platform tarball when available, otherwise a source build.

Usage:
  curl -fsSL https://lingtai.ai/install.sh | bash
  curl -fsSL https://lingtai.ai/install.sh | bash -s -- --latest
  ./install.sh [--version <tag>] [--bin-dir <dir>|--prefix <dir>]
  ./install.sh --update --prefix <prefix> --version <tag> --non-interactive

Options:
  --latest             Explicitly build TUI main + kernel main from source;
                       records and prints both resolved full commit SHAs
  --version <tag>      Release tag to install (default: latest from --source)
  --ref <ref>          Build a specific git branch/tag/commit from source
  --bin-dir <dir>      Install binaries into <dir>
  --prefix <dir>       Install binaries into <dir>/bin (used by --update)
  --from-source        Always build from source (skip prebuilt release assets)
  --skip-portal        Install only lingtai-tui (no portal)
  --skip-python         Do not create/update the Python runtime venv (explicit
                         opt-out; required when a pinned kernel bundle is
                         unavailable and you still want TUI-only binaries).
                         If a legacy ~/.lingtai-tui/runtime directory already
                         exists without a native install receipt, it is
                         preserved unchanged so TUI/portal binaries can be
                         installed beside it. --skip-venv is a back-compat alias.
  --skip-desktop        On macOS, do not register the lazy lingtai-desktop
                         command. Registration is the default for an ordinary
                         stable install and a version-pinned --update; it
                         downloads no Desktop App data. The command's first
                         execution installs Desktop. Linux, WSL, Windows,
                         --latest, --ref, and ordinary existing-receipt
                         reinstalls are unaffected. This installer pins Desktop
                         v0.1.10 and its audited four-file installer-support
                         checksums as one trust set.
  --source <mode>       auto|github|mirror (default: auto, or $LINGTAI_SOURCE).
                         auto prefers the lingtai.ai download-acceleration
                         mirror for mainland-China public IPs via a bounded,
                         fail-open country lookup; an explicit override always
                         wins and skips detection. GitHub remains the sole
                         release/version authority either way.
                         --source gitee is retired; use --source mirror.
  --update             Update an existing source/user-local install in place;
                         on macOS, register or refresh the lazy Desktop command
  --non-interactive    Never prompt; never install OS packages; fail instead
  -h, --help           Show this help

Binaries install to --bin-dir/--prefix if given, otherwise a writable
/usr/local/bin, otherwise ~/.local/bin. The portal is skipped when it can be
built from source but npm is missing. The Python runtime venv lives at
~/.lingtai-tui/runtime/venv.

For a Homebrew-to-native migration with a legacy ~/.lingtai-tui/runtime but
no native install receipt, preserve that runtime and install only the native
TUI/portal targets with:
  curl -fsSL https://lingtai.ai/install.sh | bash -s -- --version vX.Y.Z --non-interactive --skip-python
This creates a native receipt without adopting or changing the legacy runtime.
Provision or repair a separate runtime only after reviewing the exact
postconditions. For an exact-artifact update, bounded repair, read-only
verification, an explicit editable development install, or full removal of an
existing installation, use the standalone maintenance entrypoints instead of
this script: update.sh, fix.sh, verify.sh, dev.sh, remove.sh (each has its own
--help). See ANATOMY.md for their exact preconditions, allowed writes, and
postconditions.
EOF
}

# --- messaging helpers -------------------------------------------------------
say()  { echo "==> $*"; }
warn() { echo "warning: $*" >&2; }
note() { echo "    $*"; }

# print_path_hint gives the user a shell-specific command without changing the
# current process PATH or writing a shell rc file. SHELL is the user's login
# shell on the supported macOS/Linux paths; use a direct export for an
# unrecognized or unset shell rather than guessing its startup file.
print_path_hint() {
  local bin_dir="$1" shell_name="${SHELL:-}" rc_file
  case ":${PATH}:" in
    *":${bin_dir}:"*) return 0 ;;
  esac
  case "${shell_name##*/}" in
    zsh)  rc_file="$HOME/.zshrc" ;;
    bash) rc_file="$HOME/.bashrc" ;;
    *)
      say "Note: $bin_dir is not on your PATH. Add this export to your shell startup file:"
      note "export PATH=\"$bin_dir:\$PATH\""
      return 0
      ;;
  esac
  say "Note: $bin_dir is not on your PATH. Add it with:"
  note "echo 'export PATH=\"$bin_dir:\$PATH\"' >> \"$rc_file\" && source \"$rc_file\""
}

# is_wsl reports whether we're running under Windows Subsystem for Linux.
is_wsl() {
  if [[ -n "${WSL_DISTRO_NAME:-}" || -n "${WSL_INTEROP:-}" ]]; then
    return 0
  fi
  if [[ -r /proc/version ]] && grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
    return 0
  fi
  return 1
}

# Print a platform-appropriate install hint for a missing tool. Maps tool
# names to the package each manager actually ships (go is golang-go on
# Debian/Ubuntu, golang on Fedora, etc.). Homebrew is only suggested on macOS,
# never as the primary Linux path.
suggest_install() {
  local tool="$1" pkg="$1"
  if command -v apt-get &>/dev/null; then
    [[ "$tool" == "go" ]] && pkg="golang-go"
    [[ "$tool" == "npm" ]] && pkg="nodejs npm"
    [[ "$tool" == "python3" ]] && pkg="python3 python3-venv python3-pip"
    echo "      sudo apt-get update && sudo apt-get install -y $pkg" >&2
  elif command -v dnf &>/dev/null; then
    [[ "$tool" == "go" ]] && pkg="golang"
    [[ "$tool" == "npm" ]] && pkg="nodejs npm"
    [[ "$tool" == "python3" ]] && pkg="python3 python3-pip"
    echo "      sudo dnf install -y $pkg" >&2
  elif command -v pacman &>/dev/null; then
    [[ "$tool" == "npm" ]] && pkg="nodejs npm"
    [[ "$tool" == "python3" ]] && pkg="python python-pip"
    echo "      sudo pacman -S --needed $pkg" >&2
  elif command -v apk &>/dev/null; then
    [[ "$tool" == "npm" ]] && pkg="nodejs npm"
    [[ "$tool" == "python3" ]] && pkg="python3 py3-pip"
    echo "      sudo apk add $pkg" >&2
  elif command -v zypper &>/dev/null; then
    [[ "$tool" == "npm" ]] && pkg="nodejs npm"
    [[ "$tool" == "python3" ]] && pkg="python3 python3-pip"
    echo "      sudo zypper install $pkg" >&2
  elif [[ "$(uname -s)" == "Darwin" ]] || command -v brew &>/dev/null; then
    echo "      brew install $tool" >&2
  else
    echo "      install '$tool' with your system package manager" >&2
  fi
}

# --- platform detection ------------------------------------------------------

# detect_os prints darwin|linux, or "unsupported".
detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux)  echo "linux" ;;
    *)      echo "unsupported" ;;
  esac
}

# Desktop has its own release train and verified installer. The public LingTai
# installer only registers its lazy command on an ordinary stable macOS install
# or its version-pinned internal update; current-main/arbitrary-ref workflows
# keep their existing ownership.
should_install_desktop() {
  [[ "$SKIP_DESKTOP" != "1" ]] || return 1
  [[ "$(detect_os)" == "darwin" ]] || return 1
  [[ "$LATEST_MAIN_MODE" != "1" && "$REINSTALL_OK" != "1" && -z "$REF" ]]
}

# Register only a self-contained lazy command. It performs no Desktop network
# access and creates no Desktop managed state. On its first execution the
# command downloads the four exact, SHA-pinned installer-support files audited
# above; that existing Desktop code retains exclusive ownership of release API,
# archive/manifest verification, atomic App publication, and command semantics.
# A stable update may atomically refresh only this installer's marked lazy
# command; complete official Desktop state and every unowned target stay intact.
register_desktop_bootstrap() {
  local target="$BIN_DIR/lingtai-desktop"
  local app_executable="$HOME/.local/share/lingtai-desktop/current/LingTai.app/Contents/MacOS/LingTai"
  local template="$BUILD_DIR/lingtai-desktop-bootstrap.py.in"
  local staged="$BUILD_DIR/lingtai-desktop-bootstrap.py"
  local refresh_lazy=0

  if [[ -f "$target" && -x "$target" && ! -L "$target" ]]; then
    if grep -Fq '# lingtai-desktop-owned-v1' "$target"; then
      if [[ -f "$app_executable" && -x "$app_executable" && ! -L "$app_executable" ]]; then
        note "Existing complete LingTai Desktop command is already installed; keeping it unchanged: $target"
        return 0
      fi
      echo "error: existing Desktop command target was found; refusing to overwrite it: $target" >&2
      return 1
    fi
    if grep -Fq 'lingtai-desktop-lazy-bootstrap-v1' "$target"; then
      if [[ "$UPDATE_MODE" == "1" ]]; then
        refresh_lazy=1
      else
        note "Existing LingTai Desktop lazy command is already registered; keeping it unchanged: $target"
        return 0
      fi
    fi
  fi
  if [[ "$refresh_lazy" != "1" && ( -e "$target" || -L "$target" ) ]]; then
    echo "error: existing Desktop command target was found; refusing to overwrite it: $target" >&2
    return 1
  fi
  mkdir -p "$BUILD_DIR"
  cat > "$template" <<'PY'
#!/usr/bin/env python3
"""LingTai Desktop lazy bootstrap, registered by the public LingTai installer."""

from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

DESKTOP_VERSION = "@DESKTOP_VERSION@"
RAW_BASE = "@DESKTOP_RAW_BASE@"
RELEASE_BASE = "@DESKTOP_RELEASE_BASE@"
RELEASE_ASSETS = {"desktop_user_cli.py", "verify-app-archive.py"}
SUPPORT = {
    "install-macos-app.py": "@DESKTOP_INSTALLER_SHA256@",
    "desktop_user_cli.py": "@DESKTOP_CLI_SHA256@",
    "verify-app-archive.py": "@DESKTOP_VERIFIER_SHA256@",
    "support_bootstrap.py": "@DESKTOP_BOOTSTRAP_SHA256@",
}
BOOTSTRAP_MARKER = "lingtai-desktop-lazy-bootstrap-v1"


def fail(message: str) -> int:
    print(f"lingtai-desktop: {message}", file=sys.stderr)
    return 1


def invoked_path() -> Path:
    candidate = sys.argv[0]
    if os.sep not in candidate:
        candidate = shutil.which(candidate) or candidate
    return Path(candidate).resolve(strict=True)


def installed_paths() -> tuple[Path, Path]:
    home = Path.home()
    launcher = home / ".local/bin/lingtai-desktop"
    executable = (
        home / ".local/share/lingtai-desktop/current/"
        "LingTai.app/Contents/MacOS/LingTai"
    )
    return launcher, executable


def is_same_file(left: Path, right: Path) -> bool:
    try:
        return os.path.samefile(left, right)
    except OSError:
        return False


def exec_installed(launcher: Path, arguments: list[str]) -> None:
    os.execv(os.fspath(launcher), [os.fspath(launcher), *arguments])


def main() -> int:
    arguments = sys.argv[1:]
    bootstrap = invoked_path()
    launcher, app_executable = installed_paths()
    if launcher.is_file() and app_executable.is_file() and not is_same_file(bootstrap, launcher):
        exec_installed(launcher, arguments)

    curl = shutil.which("curl")
    if curl is None:
        return fail("curl is required for the first Desktop command execution")

    with tempfile.TemporaryDirectory(prefix="lingtai-desktop-bootstrap-") as temporary:
        support_root = Path(temporary)
        scripts_root = support_root / "scripts"
        scripts_root.mkdir(mode=0o700)
        for name, expected_sha in SUPPORT.items():
            destination = scripts_root / name
            if name in RELEASE_ASSETS:
                url = f"{RELEASE_BASE}/v{DESKTOP_VERSION}/{name}"
            else:
                url = f"{RAW_BASE}/v{DESKTOP_VERSION}/scripts/{name}"
            result = subprocess.run(
                [curl, "-fsSL", "--max-time", "30", "-o", os.fspath(destination), url],
                check=False,
            )
            if result.returncode != 0:
                return fail(
                    f"Desktop installer support is unavailable for v{DESKTOP_VERSION}; "
                    "retry after confirming access to the public release"
                )
            actual_sha = hashlib.sha256(destination.read_bytes()).hexdigest()
            if actual_sha != expected_sha:
                return fail(f"Desktop installer support checksum mismatch: {name}")
            destination.chmod(0o600)

        backup: Path | None = None
        if launcher.exists() and is_same_file(bootstrap, launcher):
            backup = launcher.with_name(f".lingtai-desktop.bootstrap.{os.getpid()}")
            if backup.exists() or backup.is_symlink():
                return fail("Desktop bootstrap backup path is unexpectedly occupied")
            os.replace(launcher, backup)

        result = subprocess.run(
            [sys.executable, os.fspath(scripts_root / "install-macos-app.py"),
             "--version", DESKTOP_VERSION],
            check=False,
        )
        if result.returncode != 0:
            retryable = backup is None
            if backup is not None and not launcher.exists() and not launcher.is_symlink():
                os.replace(backup, launcher)
                retryable = True
            suffix = (
                "the lazy command remains retryable"
                if retryable else "inspect the Desktop installer error above"
            )
            return fail(f"verified Desktop installation failed; {suffix}")
        if not launcher.is_file() or not app_executable.is_file():
            retryable = backup is None
            if backup is not None and not launcher.exists() and not launcher.is_symlink():
                os.replace(backup, launcher)
                retryable = True
            suffix = (
                "the lazy command remains retryable"
                if retryable else "incomplete Desktop state requires inspection"
            )
            return fail(f"Desktop installer returned success without a complete managed installation; {suffix}")
        if backup is not None:
            backup.unlink()
    exec_installed(launcher, arguments)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
PY
  sed \
    -e "s|@DESKTOP_VERSION@|$DESKTOP_VERSION|g" \
    -e "s|@DESKTOP_RAW_BASE@|$DESKTOP_RAW_BASE|g" \
    -e "s|@DESKTOP_RELEASE_BASE@|$DESKTOP_RELEASE_BASE|g" \
    -e "s|@DESKTOP_INSTALLER_SHA256@|$DESKTOP_INSTALLER_SHA256|g" \
    -e "s|@DESKTOP_CLI_SHA256@|$DESKTOP_CLI_SHA256|g" \
    -e "s|@DESKTOP_VERIFIER_SHA256@|$DESKTOP_VERIFIER_SHA256|g" \
    -e "s|@DESKTOP_BOOTSTRAP_SHA256@|$DESKTOP_BOOTSTRAP_SHA256|g" \
    "$template" > "$staged"
  chmod 755 "$staged"
  install_binary_atomically "$staged" "$target" || return 1
  if [[ "$refresh_lazy" == "1" ]]; then
    say "Refreshed lazy LingTai Desktop command at $target"
  else
    say "Registered lazy LingTai Desktop command at $target"
  fi
  note "The Desktop App will be downloaded and independently verified only when lingtai-desktop is first run."
}

# detect_arch prints amd64|arm64, or "unsupported".
detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64)          echo "amd64" ;;
    arm64 | aarch64)         echo "arm64" ;;
    *)                       echo "unsupported" ;;
  esac
}

# asset_name builds the release asset filename for a tag/os/arch triple. Keep
# this identical to the workflow's packaging step.
asset_name() {
  local tag="$1" os="$2" arch="$3"
  printf 'lingtai-%s-%s-%s.tar.gz' "$tag" "$os" "$arch"
}

# --- release metadata --------------------------------------------------------

# release_tag_name echoes its argument only when it is a strict vX.Y.Z tag,
# tolerating a refs/tags/ prefix. Empty output means "not an exact release tag".
release_tag_name() {
  local ref="${1#refs/tags/}"
  if [[ "$ref" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    printf '%s' "$ref"
  fi
}

# latest_release_tag queries the GitHub API for the latest published release
# tag. Falls back to the newest v* git tag if the API is unreachable.
latest_release_tag() {
  local body tag
  if command -v curl &>/dev/null; then
    body="$(curl -fsSL --max-time 15 "$API_BASE/releases/latest" 2>/dev/null || true)"
    tag="$(printf '%s' "$body" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"
    if [[ -n "$tag" ]]; then
      printf '%s' "$tag"
      return 0
    fi
  fi
  # Fallback: newest semver-looking tag from the git remote.
  if command -v git &>/dev/null; then
    tag="$(git ls-remote --tags "$REPO" 'v*' 2>/dev/null \
      | sed 's#.*refs/tags/##; s/\^{}//' \
      | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
      | sort -t. -k1,1V | tail -1)"
    if [[ -n "$tag" ]]; then
      printf '%s' "$tag"
      return 0
    fi
  fi
  return 1
}

# release_asset_url echoes the download URL for an asset if the release exposes
# it, otherwise nothing. Uses the release API listing so a 404 tarball is not
# mistaken for a present asset.
release_asset_url() {
  local tag="$1" name="$2" body
  command -v curl &>/dev/null || return 1
  body="$(curl -fsSL --max-time 15 "$API_BASE/releases/tags/$tag" 2>/dev/null || true)"
  [[ -n "$body" ]] || return 1
  if printf '%s' "$body" | grep -q "\"name\"[[:space:]]*:[[:space:]]*\"$name\""; then
    printf '%s/%s/%s' "$DOWNLOAD_BASE" "$tag" "$name"
    return 0
  fi
  return 1
}

# --- source policy: country detection + GitHub/mirror provider selection ---

# json_string_field extracts the first string value of a top-level JSON key
# from stdin using the same grep/sed idiom as release_asset_url/latest_release_tag
# above (no jq dependency). Not a general JSON parser — sufficient for the
# flat manifest/API shapes this script reads.
json_string_field() {
  local key="$1"
  grep -o "\"$key\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 \
    | sed "s/.*\"$key\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/"
}

# detect_country_cn returns 0 if a bounded, best-effort public-IP lookup says
# the requester is in mainland China, 1 otherwise (including "could not tell"
# — this function is fail-open by contract: a lookup failure or ambiguous
# result must never be treated as CN). Two independent unauthenticated
# providers are tried in order; each is capped at MIRROR_TIMEOUT seconds.
# Only the two-letter country code is requested — no identity, no
# credentials, no persistent client, no request body beyond a plain GET.
detect_country_cn() {
  command -v curl &>/dev/null || return 1
  local cc
  cc="$(curl -fsSL --max-time "$MIRROR_TIMEOUT" "$COUNTRY_DETECT_URL_1" 2>/dev/null | tr -d '[:space:]' || true)"
  if [[ -z "$cc" ]]; then
    cc="$(curl -fsSL --max-time "$MIRROR_TIMEOUT" "$COUNTRY_DETECT_URL_2" 2>/dev/null | tr -d '[:space:]' || true)"
  fi
  [[ "$cc" == "CN" ]]
}

# mirror_reachable is a cheap liveness probe for the lingtai.ai host, bounded
# the same way as the GitHub API calls above. This only answers "is the host
# up" — per-asset availability is a separate question answered by
# mirror_release_asset_url below, since the mirror has no listing API.
mirror_reachable() {
  command -v curl &>/dev/null || return 1
  curl -fsSL --max-time "$MIRROR_TIMEOUT" -o /dev/null "$LINGTAI_WEB_BASE/" 2>/dev/null
}

github_reachable() {
  command -v curl &>/dev/null || return 1
  curl -fsSL --max-time "$MIRROR_TIMEOUT" -o /dev/null "$API_BASE" 2>/dev/null
}

# resolve_source_provider sets BUNDLE_PROVIDER to "github" or "mirror" per the
# --source policy:
#   explicit override (github|mirror) -> that provider, no detection, no
#     reachability fallback (an explicit choice is honored even if degraded;
#     the caller still gets a clear error later if that provider truly has no
#     usable release). --source gitee is explicitly retired; see parse_args.
#   auto -> bounded country lookup; CN -> prefer mirror, else github; a failed
#     or ambiguous lookup fails open to github. The preferred provider is then
#     probed for reachability; if unreachable, falls back to the other
#     provider for the SAME resolved tag/bundle (never re-resolves "latest").
resolve_source_provider() {
  case "$SOURCE_ARG" in
    github) BUNDLE_PROVIDER="github"; return 0 ;;
    mirror) BUNDLE_PROVIDER="mirror"; return 0 ;;
  esac

  local preferred="github"
  if detect_country_cn; then
    preferred="mirror"
  fi

  if [[ "$preferred" == "mirror" ]]; then
    if mirror_reachable; then
      BUNDLE_PROVIDER="mirror"
    else
      note "lingtai.ai mirror unreachable; using GitHub for this install."
      BUNDLE_PROVIDER="github"
    fi
  else
    BUNDLE_PROVIDER="github"
  fi
}

# python_dependency_index_url echoes the ONE package index used to resolve the
# third-party dependencies of the verified local LingTai artifact. LingTai
# itself is never requested by package name from this or any other index.
#
# Precedence, deliberately narrow:
#   1. a non-empty LINGTAI_PYPI_INDEX_URL always wins (an empty value is not an
#      override and falls through, so it can never blank out the index);
#   2. otherwise the FINAL bundle provider — the one that actually served the
#      bundle manifest, after any same-tag fallback moved BUNDLE_PROVIDER —
#      picks the default it can reach: mirror -> Tsinghua TUNA, GitHub ->
#      official PyPI.
# Callers pass the result as a single `--index-url <url>` argv pair; there is
# deliberately no --extra-index-url, so exactly one index is ever consulted.
python_dependency_index_url() {
  if [[ -n "${LINGTAI_PYPI_INDEX_URL:-}" ]]; then
    printf '%s' "$LINGTAI_PYPI_INDEX_URL"
  elif [[ "${BUNDLE_PROVIDER:-github}" == "mirror" ]]; then
    printf '%s' "$PYPI_INDEX_URL_MIRROR_DEFAULT"
  else
    printf '%s' "$PYPI_INDEX_URL_DEFAULT"
  fi
}

# --- lingtai.ai mirror asset resolution -------------------------------------

# mirror_release_asset_url echoes the deterministic download URL for a named
# asset of a source repo/tag on the lingtai.ai mirror, or nothing if that
# exact URL is not (yet) reachable. Unlike GitHub/the retired Gitee mirror,
# there is no separate listing API: the mirror route
# (huangzesen/lingtai-web's docs/release-mirror/CONTRACT.md) is a single exact key
# in, one exact object out, so "does this exist" is answered by probing that
# same URL directly — never by inventing or guessing a nearby key.
mirror_release_asset_url() {
  local repo_slug="$1" tag="$2" name="$3" url
  command -v curl &>/dev/null || return 1
  url="$LINGTAI_WEB_BASE/dl/$repo_slug/$tag/$name"
  curl -fsSL --max-time "$MIRROR_TIMEOUT" --head -o /dev/null "$url" 2>/dev/null || return 1
  printf '%s' "$url"
}

# --- bundle manifest resolution (schema lingtai.tui.bundle/v1) --------------

# bundle_manifest_url_for_provider echoes the bundle manifest asset URL for a
# tag on the given provider, or nothing if unavailable.
bundle_manifest_url_for_provider() {
  local provider="$1" tag="$2"
  case "$provider" in
    github) release_asset_url "$tag" "lingtai-bundle-manifest.json" ;;
    mirror) mirror_release_asset_url "$REPO_SLUG" "$tag" "lingtai-bundle-manifest.json" ;;
    *) return 1 ;;
  esac
}

# fetch_bundle_manifest resolves BUNDLE_TAG (explicit VERSION, else latest)
# and BUNDLE_MANIFEST_JSON for BUNDLE_PROVIDER. Version identity always comes
# from GitHub: the mirror has no independent "latest" concept (no listing
# API), so an unresolved tag is always resolved via GitHub's latest-release
# endpoint regardless of BUNDLE_PROVIDER — the mirror only ever serves bytes
# for an already-known tag. If the preferred provider has no manifest for the
# resolved tag, falls back to the OTHER provider for the SAME tag (never
# re-resolves "latest" on the second provider — see the module header
# contract). Returns nonzero if neither provider has a usable manifest for
# the resolved tag.
fetch_bundle_manifest() {
  local tag="$VERSION" body url

  if [[ -z "$tag" ]]; then
    tag="$(latest_release_tag || true)"
  fi
  [[ -n "$tag" ]] || return 1

  # Keep the exact resolved TUI tag even when its bundle is absent or
  # malformed: the source-only-release kernel-pin fallback (fetch_kernel_pin,
  # called by main() when this function returns nonzero) consumes this same
  # tag without ever resolving "latest" a second time.
  BUNDLE_TAG="$tag"

  url="$(bundle_manifest_url_for_provider "$BUNDLE_PROVIDER" "$tag" || true)"
  if [[ -z "$url" ]]; then
    local other="github"
    [[ "$BUNDLE_PROVIDER" == "github" ]] && other="mirror"
    note "$BUNDLE_PROVIDER has no bundle manifest for $tag; trying $other for the SAME tag."
    url="$(bundle_manifest_url_for_provider "$other" "$tag" || true)"
    [[ -n "$url" ]] || return 1
    BUNDLE_PROVIDER="$other"
  fi

  body="$(curl -fsSL --max-time 30 "$url" 2>/dev/null || true)"
  [[ -n "$body" ]] || return 1
  if ! load_bundle_manifest "$body" "$tag"; then
    echo "error: bundle manifest at $url failed strict validation" >&2
    return 1
  fi

  BUNDLE_MANIFEST_JSON="$body"
  return 0
}

# Validate the complete bundle contract at the trust boundary and print the
# canonical digest for this host's archive when the release publishes one.
# A host archive is optional: the same manifest still binds the exact kernel
# while the TUI/Portal binaries fall back to an exact-tag source build.
parse_bundle_manifest() {
  local body="$1" expected_tag="$2"
  BODY="$body" python3 - "$expected_tag" "$(detect_os)" "$(detect_arch)" <<'PY'
import datetime, json, os, re, sys
expected_tag, os_name, arch = sys.argv[1:]
def pairs(items):
    result = {}
    for key, value in items:
        if key in result:
            raise ValueError(f"duplicate JSON key: {key}")
        result[key] = value
    return result
def exact(value, keys, label):
    if not isinstance(value, dict) or set(value) != set(keys):
        raise ValueError(f"{label} has the wrong object shape")
def string(value, label):
    if not isinstance(value, str) or not value:
        raise ValueError(f"{label} must be a nonempty string")
    return value
try:
    data = json.loads(os.environ["BODY"], object_pairs_hook=pairs)
    exact(data, ("schema", "bundle_id", "tui_tag", "tui_commit", "generated_at", "kernel_tag", "kernel_version", "kernel_manifest_filename", "archives", "providers"), "manifest")
    if data["schema"] != "lingtai.tui.bundle/v1": raise ValueError("unexpected schema")
    for key in ("bundle_id", "tui_tag", "tui_commit", "kernel_tag", "kernel_version", "kernel_manifest_filename"): string(data[key], key)
    if data["bundle_id"] != data["tui_tag"] or data["tui_tag"] != expected_tag: raise ValueError("bundle_id/tui_tag does not equal resolved tag")
    if not re.fullmatch(r"[0-9a-f]{40}", data["tui_commit"]): raise ValueError("tui_commit must be a 40-character lowercase commit SHA")
    generated_at = data["generated_at"]
    if not isinstance(generated_at, str) or not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z", generated_at): raise ValueError("generated_at must be YYYY-MM-DDTHH:MM:SSZ")
    datetime.datetime.strptime(generated_at, "%Y-%m-%dT%H:%M:%SZ")
    if not isinstance(data["archives"], list) or not data["archives"]: raise ValueError("archives must be a nonempty array")
    names = set()
    for archive in data["archives"]:
        exact(archive, ("filename", "sha256"), "archive entry")
        name = string(archive["filename"], "archive filename")
        if name in names: raise ValueError("archives contains duplicate filenames")
        names.add(name)
        if not (re.fullmatch(r"lingtai-[^/]+-(?:darwin|linux)-(?:amd64|arm64)\.tar\.gz", name) or re.fullmatch(r"lingtai-[^/]+-windows-amd64\.zip", name)): raise ValueError("archive filename is invalid")
        if not isinstance(archive["sha256"], str) or not re.fullmatch(r"[0-9a-f]{64}", archive["sha256"]): raise ValueError("archive sha256 must be lowercase 64-hex")
    target = f"lingtai-{expected_tag}-{os_name}-{arch}.tar.gz"
    hits = [archive for archive in data["archives"] if archive["filename"] == target]
    exact(data["providers"], ("github", "gitee"), "providers")
    exact(data["providers"]["github"], ("repo",), "github provider")
    exact(data["providers"]["gitee"], ("owner", "repo"), "gitee provider")
    string(data["providers"]["github"]["repo"], "github repo")
    string(data["providers"]["gitee"]["owner"], "gitee owner")
    string(data["providers"]["gitee"]["repo"], "gitee repo")
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", data["providers"]["github"]["repo"]): raise ValueError("github repo is invalid")
    if not re.fullmatch(r"[A-Za-z0-9_.-]+", data["providers"]["gitee"]["owner"]): raise ValueError("gitee owner is invalid")
    if not re.fullmatch(r"[A-Za-z0-9_.-]+", data["providers"]["gitee"]["repo"]): raise ValueError("gitee repo is invalid")
except (ValueError, TypeError, json.JSONDecodeError) as exc:
    raise SystemExit(f"invalid strict bundle manifest: {exc}")
print(hits[0]["sha256"] if hits else "")
print(data["kernel_tag"])
print(data["kernel_version"])
print(data["kernel_manifest_filename"])
print(data["bundle_id"])
PY
}

validate_bundle_manifest() { parse_bundle_manifest "$1" "$2" | sed -n '1p'; }

load_bundle_manifest() {
  local body="$1" expected_tag="$2" fields=() field
  while IFS= read -r field; do fields+=("$field"); done < <(parse_bundle_manifest "$body" "$expected_tag")
  [[ "${#fields[@]}" == 5 ]] || return 1
  BUNDLE_TUI_ARCHIVE_SHA="${fields[0]}"
  BUNDLE_MANIFEST_KERNEL_TAG="${fields[1]}"
  BUNDLE_MANIFEST_KERNEL_VERSION="${fields[2]}"
  BUNDLE_MANIFEST_KERNEL_FILENAME="${fields[3]}"
  BUNDLE_MANIFEST_BUNDLE_ID="${fields[4]}"
}

# bundle_manifest_field returns values populated by the strict parser; it
# never reparses raw manifest text.
bundle_manifest_field() {
  case "$1" in
    bundle_id) printf '%s\n' "$BUNDLE_MANIFEST_BUNDLE_ID" ;;
    kernel_tag) printf '%s\n' "$BUNDLE_MANIFEST_KERNEL_TAG" ;;
    kernel_version) printf '%s\n' "$BUNDLE_MANIFEST_KERNEL_VERSION" ;;
    kernel_manifest_filename) printf '%s\n' "$BUNDLE_MANIFEST_KERNEL_FILENAME" ;;
    *) return 1 ;;
  esac
}

# run_manifest_python picks any available system python3, or bootstraps a
# managed uv Python 3.13 on a machine with neither — manifest validation is a
# read-only boundary that happens before the owned runtime venv exists, so it
# must not depend on that venv already being usable.
run_manifest_python() {
  local body="$1" uv
  shift
  if command -v python3 >/dev/null 2>&1; then
    BODY="$body" python3 "$@"
    return
  fi
  ensure_uv >/dev/null || return 1
  uv="$(find_uv 2>/dev/null || true)"
  [[ -n "$uv" && -x "$uv" ]] || return 1
  BODY="$body" "$uv" run --no-project --managed-python --python 3.13 -- python "$@"
}

# parse_kernel_pin_manifest strictly validates kernel-release.json, the small
# source-owned pin committed at an exact TUI release tag for releases with no
# dual bundle manifest (source-only TUI releases). Keeping the parser strict
# prevents an accidental "latest" or provider-specific shape from selecting a
# kernel outside the TUI release's explicit contract.
parse_kernel_pin_manifest() {
  local body="$1"
  run_manifest_python "$body" - <<'PY'
import json
import os
import re


def pairs(items):
    result = {}
    for key, value in items:
        if key in result:
            raise ValueError(f"duplicate JSON key: {key}")
        result[key] = value
    return result

try:
    data = json.loads(os.environ["BODY"], object_pairs_hook=pairs)
    if not isinstance(data, dict) or set(data) != {"schema", "kernel_tag", "comment"}:
        raise ValueError("unexpected top-level keys")
    if data["schema"] != "lingtai.tui.kernel-pin/v1":
        raise ValueError("unexpected schema")
    if not isinstance(data["kernel_tag"], str) or not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", data["kernel_tag"]):
        raise ValueError("kernel_tag must be a versioned vX.Y.Z tag")
    if not isinstance(data["comment"], str) or not data["comment"].strip():
        raise ValueError("comment must be a non-empty string")
except (ValueError, TypeError, json.JSONDecodeError) as exc:
    raise SystemExit(f"invalid kernel pin manifest: {exc}")

print(data["kernel_tag"])
PY
}

# kernel_pin_url_for_provider returns kernel-release.json from the exact TUI
# tag; unlike latest-release helpers, it never resolves another tag.
# kernel-release.json is a repo-committed file at the tag, not a GitHub
# Releases asset, so the mirror (which only re-serves uploaded release
# assets) has no copy of it: the "mirror" case always fails, letting the
# fetch_kernel_pin loop below fall through to GitHub for this rare
# source-only-release fallback.
kernel_pin_url_for_provider() {
  local provider="$1" tag="$2"
  case "$provider" in
    github) printf 'https://raw.githubusercontent.com/%s/%s/kernel-release.json' "$REPO_SLUG" "$tag" ;;
    mirror) return 1 ;;
    *) return 1 ;;
  esac
}

# fetch_kernel_pin fetches and strictly validates kernel-release.json from the
# exact resolved TUI tag. A missing/malformed pin on the selected provider is
# retried on the other provider for that SAME tag only — this is the
# source-only-release fallback tried when fetch_bundle_manifest cannot resolve
# a dual bundle for this tag (see BUNDLE_REQUIRED's caller in main()).
fetch_kernel_pin() {
  local tui_tag="$1" provider="${BUNDLE_PROVIDER:-github}" other url body kernel_tag candidate
  KERNEL_PIN_JSON=""
  KERNEL_PIN_TAG=""
  KERNEL_PIN_PROVIDER=""
  KERNEL_PIN_TUI_TAG=""

  other="github"
  [[ "$provider" == "github" ]] && other="mirror"
  for candidate in "$provider" "$other"; do
    url="$(kernel_pin_url_for_provider "$candidate" "$tui_tag" || true)"
    [[ -n "$url" ]] || continue
    body="$(curl -fsSL --max-time 30 "$url" 2>/dev/null || true)"
    [[ -n "$body" ]] || continue
    if ! kernel_tag="$(parse_kernel_pin_manifest "$body" 2>/dev/null)"; then
      echo "error: kernel pin at $url failed strict validation" >&2
      continue
    fi
    KERNEL_PIN_JSON="$body"
    KERNEL_PIN_TAG="$kernel_tag"
    KERNEL_PIN_PROVIDER="$candidate"
    KERNEL_PIN_TUI_TAG="$tui_tag"
    return 0
  done
  return 1
}

# kernel_tag_for_install preserves the existing bundle as the first-priority
# source and otherwise returns the exact release pin selected above.
kernel_tag_for_install() {
  if [[ -n "$BUNDLE_MANIFEST_JSON" ]]; then
    bundle_manifest_field kernel_tag
  else
    printf '%s\n' "$KERNEL_PIN_TAG"
  fi
}

# kernel_source_for_install returns "bundle" when a dual bundle manifest was
# resolved, else "release-pin" when the exact-tag kernel-release.json pin was
# resolved instead, else nothing (caller must fail loud).
kernel_source_for_install() {
  if [[ -n "$BUNDLE_MANIFEST_JSON" ]]; then
    printf '%s\n' "bundle"
  elif [[ -n "$KERNEL_PIN_TAG" ]]; then
    printf '%s\n' "release-pin"
  fi
}

# verify_sha256 checks a file against an expected lowercase hex digest using
# whichever checksum tool is available. Returns nonzero on mismatch or if no
# checksum tool exists (callers must treat "no tool" as a hard failure, not a
# skip — this installer never installs unverified release bytes).
verify_sha256() {
  local file="$1" expected="$2" actual
  if command -v sha256sum &>/dev/null; then
    actual="$(sha256sum "$file" | cut -d' ' -f1)"
  elif command -v shasum &>/dev/null; then
    actual="$(shasum -a 256 "$file" | cut -d' ' -f1)"
  else
    echo "error: no sha256sum/shasum tool available to verify $file" >&2
    return 1
  fi
  [[ "$actual" == "$expected" ]]
}

# --- git checkout version helpers (used by source build + tests) -------------

is_exact_checkout_tag() {
  local repo_dir="$1" tag="$2" tag_commit head_commit
  tag_commit="$(git -C "$repo_dir" rev-parse --verify --quiet "refs/tags/$tag^{commit}" 2>/dev/null || true)"
  if [[ -z "$tag_commit" ]]; then
    return 1
  fi
  head_commit="$(git -C "$repo_dir" rev-parse --verify HEAD 2>/dev/null || true)"
  if [[ -z "$head_commit" ]]; then
    return 1
  fi
  [[ "$head_commit" == "$tag_commit" ]]
}

version_for_checkout() {
  local repo_dir="$1" requested_ref="$2" requested_tag
  requested_tag="$(release_tag_name "$requested_ref")"
  if [[ -n "$requested_tag" ]] && is_exact_checkout_tag "$repo_dir" "$requested_tag"; then
    printf '%s\n' "$requested_tag"
    return
  fi
  git -C "$repo_dir" describe --tags --always 2>/dev/null || echo "dev"
}

resolved_ref_for_checkout() {
  local repo_dir="$1" exact_tag branch
  exact_tag="$(git -C "$repo_dir" describe --tags --exact-match 2>/dev/null || true)"
  if [[ -n "$exact_tag" ]]; then
    printf '%s\n' "$exact_tag"
    return
  fi
  branch="$(git -C "$repo_dir" symbolic-ref --quiet --short HEAD 2>/dev/null || true)"
  if [[ -n "$branch" ]]; then
    printf '%s\n' "$branch"
    return
  fi
  git -C "$repo_dir" rev-parse --short HEAD
}

# --- bin dir / prefix helpers ------------------------------------------------

# resolve_main_branch_sha resolves one exact full commit before either source
# checkout begins. The subsequent clones are checked against these pins; a
# moving main branch therefore fails loudly instead of building a mixed pair.
resolve_main_branch_sha() {
  local repo_url="$1" sha
  command -v git &>/dev/null || return 1
  sha="$(git ls-remote "$repo_url" refs/heads/main 2>/dev/null | awk 'NF { print $1; exit }')"
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  printf '%s\n' "$sha"
}

prefix_for_bin_dir() {
  local bin_dir="$1"
  if [[ "$(basename "$bin_dir")" == "bin" ]]; then
    dirname "$bin_dir"
  else
    printf '%s\n' "$bin_dir"
  fi
}

bin_dir_for_prefix() {
  local prefix="$1"
  printf '%s/bin\n' "${prefix%/}"
}

install_binary_atomically() {
  local src="$1" dst="$2" dir base tmp
  dir="$(dirname "$dst")"
  base="$(basename "$dst")"
  tmp="$dir/.$base.tmp.$$"
  install -m 755 "$src" "$tmp"
  mv -f "$tmp" "$dst"
}

verify_tui_binary_version() {
  local binary="$1" want="$2" output
  output="$("$binary" version 2>&1)"
  case "$output" in
    *"$want"*) ;;
    *)
      echo "error: built lingtai-tui reports '$output', expected '$want'" >&2
      return 1
      ;;
  esac
}

ensure_lingtai_alias() {
  local bin_dir="$1"
  if [[ ! -e "$bin_dir/lingtai" ]] || [[ -L "$bin_dir/lingtai" && "$(readlink "$bin_dir/lingtai")" == "$bin_dir/lingtai-tui" ]]; then
    ln -sfn "$bin_dir/lingtai-tui" "$bin_dir/lingtai"
  else
    echo "  (skipping 'lingtai' alias — $bin_dir/lingtai already exists)"
  fi
}

# --- arg parsing -------------------------------------------------------------

parse_args() {
  local saw_latest=0 saw_ref=0 saw_version=0 saw_from_source=0 saw_skip_python=0 saw_source=0 saw_update=0
  LATEST_MAIN_MODE=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --latest) LATEST_MAIN_MODE=1; saw_latest=1; shift ;;
      --ref) REF="${2:?error: --ref requires a value}"; saw_ref=1; shift 2 ;;
      --version) VERSION="${2:?error: --version requires a value}"; saw_version=1; shift 2 ;;
      --prefix) INSTALL_PREFIX="${2:?error: --prefix requires a value}"; shift 2 ;;
      --bin-dir) BIN_DIR_OVERRIDE="${2:?error: --bin-dir requires a value}"; shift 2 ;;
      --from-source) FROM_SOURCE=1; saw_from_source=1; shift ;;
      --skip-portal) SKIP_PORTAL=1; shift ;;
      --skip-python|--skip-venv) SKIP_VENV=1; saw_skip_python=1; shift ;;
      --skip-desktop) SKIP_DESKTOP=1; shift ;;
      --source) SOURCE_ARG="${2:?error: --source requires a value}"; saw_source=1; shift 2 ;;
      --update) UPDATE_MODE=1; saw_update=1; shift ;;
      --non-interactive) NON_INTERACTIVE=1; shift ;;
      -h|--help) usage; exit 0 ;;
      *) echo "error: unknown flag: $1" >&2; usage >&2; exit 1 ;;
    esac
  done

  if [[ "$saw_latest" == "1" ]]; then
    if [[ "$saw_ref" == "1" || "$saw_version" == "1" || "$saw_from_source" == "1" || "$saw_skip_python" == "1" || "$saw_source" == "1" || "$saw_update" == "1" || "$SOURCE_ARG" != "auto" ]]; then
      echo "error: --latest cannot be combined with --ref, --version, --from-source, --skip-python/--skip-venv, --source/LINGTAI_SOURCE, or --update" >&2
      usage >&2
      exit 1
    fi
  fi

  # --update is the TUI source updater contract: it passes --prefix and
  # --version and expects an in-place source-compatible update.
  if [[ "$UPDATE_MODE" == "1" ]]; then
    if [[ -z "$INSTALL_PREFIX" ]]; then
      echo "error: --update requires --prefix <prefix>" >&2
      usage >&2
      exit 1
    fi
    if [[ -z "$(release_tag_name "$VERSION")" ]]; then
      echo "error: --update requires --version <release-tag>" >&2
      usage >&2
      exit 1
    fi
  fi

  case "$SOURCE_ARG" in
    auto|github|mirror) ;;
    gitee)
      echo "error: --source gitee has been retired; lingtai.ai now provides the China-accelerated mirror." >&2
      echo "       Use --source mirror or --source auto." >&2
      usage >&2
      exit 1
      ;;
    *) echo "error: --source must be one of auto|github|mirror, got: $SOURCE_ARG" >&2; usage >&2; exit 1 ;;
  esac
}

# --- install metadata --------------------------------------------------------

json_escape() {
  local s="$1" ch ord
  local LC_ALL=C
  # LC_ALL=C makes Bash indexing byte-wise: UTF-8 metadata bytes pass through; JSON controls are escaped.
  local i

  for (( i = 0; i < ${#s}; i++ )); do
    ch="${s:i:1}"
    case "$ch" in
      \\) printf '\\\\' ;;
      '"') printf '\\"' ;;
      $'\b') printf '\\b' ;;
      $'\f') printf '\\f' ;;
      $'\n') printf '\\n' ;;
      $'\r') printf '\\r' ;;
      $'\t') printf '\\t' ;;
      *)
        printf -v ord '%d' "'$ch"
        (( ord < 0 )) && ord=$(( ord + 256 ))
        if (( ord < 32 )); then
          printf '\\u%04x' "$ord"
        else
          printf '%s' "$ch"
        fi
        ;;
    esac
  done
}

# write_install_metadata records the install so `lingtai-tui`'s source updater
# can re-run this script for a newer tag. install_method stays "source" for
# updater compatibility regardless of whether we downloaded a prebuilt asset or
# built from source; install_kind records which path was taken (additive field).
write_install_metadata() {
  local global_dir="$1" prefix="$2" bin_dir="$3" repo_url="$4" requested_ref="$5"
  local resolved_ref="$6" resolved_commit="$7" stamped_version="$8" tui_path="$9"
  local portal_path="${10:-}" metadata_path tmp_path installed_at portal_json=""
  local install_kind="${INSTALL_KIND:-source-build}"
  # Kernel/runtime provenance is read from globals (set by install_kernel_from_bundle
  # / ensure_runtime_venv during this run) rather than added as more positional
  # params — this function already has 10. KERNEL_SOURCE is "" (no verified
  # kernel install happened this run — e.g. --skip-python), "bundle", "main"
  # (--latest), or "release-pin" (source-only TUI release with no dual bundle
  # manifest — see fetch_kernel_pin). The block is omitted entirely, not
  # written as empty strings, when KERNEL_SOURCE is "" — old readers see
  # exactly the same install.json shape as before this field existed.
  local bundle_json="" runtime_json=""
  if [[ -n "${RUNTIME_VENV_DIR:-}" && "$SKIP_VENV" != "1" ]]; then
    if ! canonical_runtime_venv "$RUNTIME_VENV_DIR" "$HOME/.lingtai-tui/runtime" >/dev/null; then
      echo "error: refusing to persist a runtime pointer outside the canonical owned runtime root: $RUNTIME_VENV_DIR" >&2
      return 1
    fi
    runtime_json="$(printf ',\n  "runtime_venv": "%s"' "$(json_escape "${RUNTIME_VENV_DIR%/}")")"
  fi
  if [[ "$KERNEL_SOURCE" == "bundle" ]]; then
    bundle_json="$(printf ',\n  "kernel_source": "bundle",\n  "kernel_bundle_id": "%s",\n  "kernel_version": "%s",\n  "kernel_provider": "%s"' \
      "$(json_escape "$KERNEL_BUNDLE_ID")" "$(json_escape "$KERNEL_VERSION_INSTALLED")" "$(json_escape "$KERNEL_PROVIDER")")"
    bundle_json="$(printf '%s,\n  "bundle_provider": "%s"' "$bundle_json" "$(json_escape "$BUNDLE_PROVIDER")")"
  elif [[ "$KERNEL_SOURCE" == "release-pin" ]]; then
    local release_tag="${KERNEL_RELEASE_TAG:-$KERNEL_PIN_TAG}"
    local tui_release_tag="${KERNEL_PIN_TUI_TAG:-$BUNDLE_TAG}"
    bundle_json="$(printf ',\n  "kernel_source": "release-pin",\n  "kernel_release_tag": "%s",\n  "kernel_version": "%s",\n  "kernel_provider": "%s",\n  "tui_release_tag": "%s"' \
      "$(json_escape "$release_tag")" "$(json_escape "$KERNEL_VERSION_INSTALLED")" \
      "$(json_escape "$KERNEL_PROVIDER")" "$(json_escape "$tui_release_tag")")"
  elif [[ "$KERNEL_SOURCE" == "main" ]]; then
    bundle_json="$(printf ',\n  "source_mode": "latest-main",\n  "tui_commit": "%s",\n  "kernel_source": "main",\n  "kernel_commit": "%s",\n  "kernel_version": "%s",\n  "kernel_provider": "github"' \
      "$(json_escape "$TUI_MAIN_SHA")" "$(json_escape "$KERNEL_MAIN_SHA")" "$(json_escape "$KERNEL_VERSION_INSTALLED")")"
  fi

  metadata_path="$global_dir/install.json"
  installed_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

  if [[ -L "$global_dir" ]]; then
    echo "error: install metadata directory is a symlink; refusing to write redirected state: $global_dir" >&2
    return 1
  fi
  mkdir -p "$global_dir"
  if [[ -n "$portal_path" ]]; then
    portal_json="$(printf ',\n    "%s"' "$(json_escape "$portal_path")")"
  fi

  # UPDATE_MODE re-publishes over its own existing receipt by design (an
  # in-place update of the same target), so the exclusive-create/no-clobber
  # publication below applies only to a fresh (non-update) install — see
  # validate_fresh_install_state, which already refuses ordinary install over
  # an existing receipt before this function is ever reached on that path.
  # LATEST_MAIN_MODE (--latest) is the other exception: it is an explicit,
  # already-documented re-runnable current-main dev install, so a second run
  # must republish its own receipt in place exactly like --update, not refuse
  # to record newly-installed binaries/runtime it already mutated.
  if [[ "$UPDATE_MODE" != "1" && "$LATEST_MAIN_MODE" != "1" && "$REINSTALL_OK" != "1" ]]; then
    if [[ -e "$metadata_path" || -L "$metadata_path" ]]; then
      echo "error: install receipt appeared before metadata creation; refusing to replace it: $metadata_path" >&2
      return 1
    fi
    tmp_path="$(mktemp "$global_dir/.install.json.XXXXXX")" || {
      echo "error: could not create an owned metadata staging file under $global_dir" >&2
      return 1
    }
    chmod 600 "$tmp_path" || { rm -f "$tmp_path"; return 1; }
  else
    tmp_path="$metadata_path.tmp.$$"
    : > "$tmp_path" || { echo "error: could not create receipt republish staging file: $tmp_path" >&2; return 1; }
    chmod 600 "$tmp_path" || { rm -f "$tmp_path"; return 1; }
  fi

  cat > "$tmp_path" <<EOF
{
  "schema": "lingtai.tui.install/v1",
  "schema_version": 1,
  "install_method": "source",
  "install_kind": "$(json_escape "$install_kind")",
  "prefix": "$(json_escape "$prefix")",
  "bin_dir": "$(json_escape "$bin_dir")",
  "repo_url": "$(json_escape "$repo_url")",
  "requested_ref": "$(json_escape "$requested_ref")",
  "resolved_ref": "$(json_escape "$resolved_ref")",
  "resolved_commit": "$(json_escape "$resolved_commit")",
  "stamped_version": "$(json_escape "$stamped_version")",
  "installed_at": "$(json_escape "$installed_at")",
  "managed_binaries": [
    "$(json_escape "$tui_path")"$portal_json
  ]$bundle_json$runtime_json
}
EOF

  if [[ "$UPDATE_MODE" != "1" && "$LATEST_MAIN_MODE" != "1" && "$REINSTALL_OK" != "1" ]]; then
    if [[ -L "$global_dir" || -e "$metadata_path" || -L "$metadata_path" ]]; then
      rm -f "$tmp_path"
      echo "error: install receipt appeared during metadata creation; refusing to replace it: $metadata_path" >&2
      return 1
    fi
    # Same-directory hard-link publication is atomic and no-clobber: if
    # another writer creates install.json first, ln fails without replacing
    # its bytes.
    if ! ln "$tmp_path" "$metadata_path"; then
      rm -f "$tmp_path"
      echo "error: install receipt could not be published exclusively; existing state was preserved: $metadata_path" >&2
      return 1
    fi
    rm -f "$tmp_path"
  else
    mv "$tmp_path" "$metadata_path"
  fi
}

# --- OS package installation (Linux/WSL) -------------------------------------

# have_sudo reports whether we can run sudo non-interactively-ish. Root needs no
# sudo; otherwise sudo must exist.
have_root_or_sudo() {
  [[ "$(id -u)" == "0" ]] && return 0
  command -v sudo &>/dev/null
}

as_root() {
  if [[ "$(id -u)" == "0" ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

# apt_install installs packages when interactive and root/sudo is available;
# otherwise prints the exact command and returns non-zero.
apt_install() {
  local why="$1"; shift
  if [[ "$NON_INTERACTIVE" == "1" ]] || ! have_root_or_sudo; then
    warn "$why — install the packages first:"
    echo "      sudo apt-get update && sudo apt-get install -y $*" >&2
    return 1
  fi
  say "Installing $why via apt: $*"
  as_root apt-get update
  as_root apt-get install -y "$@"
}

# --- Python runtime venv -----------------------------------------------------

find_uv() {
  if command -v uv &>/dev/null; then command -v uv; return 0; fi
  [[ -n "${UV_INSTALL_DIR:-}" && -x "$UV_INSTALL_DIR/uv" ]] && { echo "$UV_INSTALL_DIR/uv"; return 0; }
  [[ -x "$HOME/.local/bin/uv" ]] && { echo "$HOME/.local/bin/uv"; return 0; }
  return 1
}

# ensure_uv resolves an executable uv, bootstrapping it if necessary. uv can
# download its own Python toolchain (uv venv --python 3.13), which is the only
# reliable way to get Python 3.11+ on distros whose system packages are older
# (e.g. Ubuntu jammy ships Python 3.10). If uv is already present it is reused;
# otherwise the official installer is downloaded to a temp file and run with an
# explicit UV_INSTALL_DIR so the result lands in a known location. On success it
# echoes the uv path and returns 0; on failure it warns loudly and returns 1
# without aborting the overall install.
ensure_uv() {
  local uv installer rc
  uv="$(find_uv 2>/dev/null || true)"
  if [[ -n "$uv" ]]; then
    echo "$uv"
    return 0
  fi

  if ! command -v curl &>/dev/null; then
    warn "curl is required to bootstrap uv but was not found."
    return 1
  fi

  local install_dir="${UV_INSTALL_DIR:-$HOME/.local/bin}"
  say "Bootstrapping uv (for a self-contained Python runtime) ..."
  mkdir -p "$install_dir"

  installer="$BUILD_DIR/uv-install.sh"
  mkdir -p "$BUILD_DIR"
  # Download to a temp file first so the script is fetched (and can be inspected)
  # before it is executed, rather than piping an unseen body straight into sh.
  if ! curl -fsSL --retry 3 --max-time 120 -o "$installer" "$UV_INSTALLER_URL"; then
    warn "failed to download the uv installer from $UV_INSTALLER_URL"
    return 1
  fi

  # UV_INSTALL_DIR pins where the uv binary lands; UV_NO_MODIFY_PATH keeps the
  # installer from editing shell rc files during a one-shot install.
  UV_INSTALL_DIR="$install_dir" UV_NO_MODIFY_PATH=1 sh "$installer" >/dev/null 2>&1
  rc=$?
  if [[ "$rc" -ne 0 ]]; then
    warn "the uv installer exited with status $rc"
    return 1
  fi

  # The freshly-installed uv may not be on PATH yet; find_uv also probes
  # ~/.local/bin, and we fold in an explicit install_dir check for custom dirs.
  uv="$(find_uv 2>/dev/null || true)"
  if [[ -z "$uv" && -x "$install_dir/uv" ]]; then
    uv="$install_dir/uv"
  fi
  if [[ -z "$uv" || ! -x "$uv" ]]; then
    warn "uv installer ran but no executable uv was found under $install_dir."
    return 1
  fi
  say "Bootstrapped uv at $uv."
  echo "$uv"
  return 0
}

# python_ok reports whether a python3 with venv/ensurepip support and >=3.11 is present.
python_ok() {
  command -v python3 &>/dev/null || return 1
  python3 -c 'import sys; sys.exit(0 if sys.version_info >= (3, 11) else 1)' 2>/dev/null || return 1
  python3 -c 'import venv, ensurepip' 2>/dev/null || return 1
  return 0
}

# ensure_python makes a usable Python interpreter available for the runtime venv.
# uv is preferred because it can download its own Python 3.13 toolchain, which is
# the only reliable path on distros whose packages are too old (Ubuntu jammy ships
# Python 3.10, so `apt install python3` does NOT yield a usable interpreter here).
# Order of preference:
#   1. an existing uv                       -> done (uv downloads Python itself)
#   2. an already-adequate system python3   -> done
#   3. bootstrap uv via the official installer (needs curl) -> done
#   4. apt-install python3/venv/pip and re-check python_ok (for distros where it
#      actually yields Python 3.11+, or where curl is unavailable for step 3)
ensure_python() {
  if find_uv >/dev/null 2>&1; then
    return 0  # uv can download Python itself
  fi
  if python_ok; then
    return 0
  fi
  # Try to bootstrap uv before falling back to system packages: on jammy the apt
  # python3 is 3.10, so uv is the only way to reach Python 3.11+.
  if ensure_uv >/dev/null; then
    return 0
  fi
  if command -v apt-get &>/dev/null; then
    apt_install "Python 3.11+ with venv/pip" python3 python3-venv python3-pip || return 1
    python_ok && return 0
    warn "apt-installed python3 is still older than 3.11 (or lacks venv); uv bootstrap is required."
  fi
  warn "Python 3.11+ (via uv or system packages) is required for the runtime venv. Install uv or Python 3.11+ with:"
  suggest_install python3
  return 1
}

# canonical_runtime_venv resolves a candidate runtime venv path to its
# physical location and requires that physical location to be canonically
# contained under $HOME/.lingtai-tui/runtime — not merely lexically prefixed.
# A symlinked venv directory (or a symlinked ancestor) whose real target
# escapes the owned runtime root is rejected outright: this installer must
# never adopt or mutate a venv outside the root it claims to own. Prints the
# physical path and returns 0 only when containment holds.
canonical_runtime_venv() {
  local dir="$1" runtime_root="$2" physical_root physical_dir physical_home expected_root
  local parent base physical_parent root_parent root_base root_grandparent root_parent_base

  physical_home="$(cd "$HOME" 2>/dev/null && pwd -P)" || return 1
  expected_root="$physical_home/.lingtai-tui/runtime"

  # Ownership is both lexical and physical: a path outside the declared root
  # is not adopted merely because a symlink happens to point back inside it.
  [[ "$dir" == "$runtime_root"/* ]] || return 1
  [[ ! -L "$runtime_root" ]] || return 1

  if [[ -d "$runtime_root" ]]; then
    physical_root="$(cd "$runtime_root" 2>/dev/null && pwd -P)" || return 1
  elif [[ -e "$runtime_root" ]]; then
    return 1
  else
    # Resolve the not-yet-created owned root without mkdir. A completely fresh
    # install may also lack its .lingtai-tui parent, so append at most those two
    # fixed missing components to an existing physical HOME ancestor.
    root_parent="$(dirname "$runtime_root")"
    root_base="$(basename "$runtime_root")"
    if [[ -d "$root_parent" ]]; then
      physical_root="$(cd "$root_parent" 2>/dev/null && pwd -P)/$root_base" || return 1
    else
      [[ ! -e "$root_parent" && ! -L "$root_parent" ]] || return 1
      root_grandparent="$(dirname "$root_parent")"
      root_parent_base="$(basename "$root_parent")"
      [[ -d "$root_grandparent" ]] || return 1
      physical_root="$(cd "$root_grandparent" 2>/dev/null && pwd -P)/$root_parent_base/$root_base" || return 1
    fi
  fi
  # `$HOME` itself may be a symlink, but `.lingtai-tui` and `runtime` may not
  # redirect ownership elsewhere. The resolved root must be exactly beneath the
  # canonical physical HOME, not merely whatever `pwd -P` found through an
  # ancestor symlink.
  [[ "$physical_root" == "$expected_root" ]] || return 1

  if [[ -d "$dir" ]]; then
    physical_dir="$(cd "$dir" 2>/dev/null && pwd -P)" || return 1
  else
    # A file or symlink (including dangling) is occupied untrusted state, not a
    # free final child that venv creation may replace or follow.
    [[ ! -e "$dir" && ! -L "$dir" ]] || return 1
    parent="$(dirname "$dir")"
    base="$(basename "$dir")"
    [[ "$base" != "." && "$base" != ".." ]] || return 1
    if [[ "$parent" == "$runtime_root" && ! -e "$runtime_root" ]]; then
      physical_parent="$physical_root"
    else
      physical_parent="$(cd "$parent" 2>/dev/null && pwd -P)" || return 1
    fi
    physical_dir="$physical_parent/$base"
  fi
  [[ "$physical_dir" == "$physical_root"/* ]] || return 1
  printf '%s\n' "$physical_dir"
}

# runtime_python_for_venv resolves the python/python3 launcher under a venv
# directory, or nothing if neither exists.
runtime_python_for_venv() {
  local venv_dir="$1"
  if [[ -x "$venv_dir/bin/python" ]]; then
    printf '%s\n' "$venv_dir/bin/python"
  elif [[ -x "$venv_dir/bin/python3" ]]; then
    printf '%s\n' "$venv_dir/bin/python3"
  fi
}

# A launcher located under the selected venv is not enough ownership proof: it
# may be a symlink to another environment. Check sys.prefix before any pip/uv
# operation so an external interpreter is never mutated and rejected only later.
runtime_prefix_matches_venv() {
  local py="$1" venv_dir="$2" selected_prefix
  selected_prefix="$(cd "$venv_dir" 2>/dev/null && pwd -P)" || return 1
  PYTHONPATH= "$py" - "$selected_prefix" <<'PY' >/dev/null 2>&1
import os
import sys

selected_prefix = os.path.realpath(sys.argv[1])
raise SystemExit(0 if os.path.realpath(sys.prefix) == selected_prefix else 1)
PY
}

# ensure_runtime_pip repairs pip only inside the brand-new owned venv created by
# this invocation. It first asks that exact interpreter to seed itself, then (if
# available) uses the already selected uv scoped to the same venv. Prefix checks
# before and after the attempt prevent either fallback from reaching another
# interpreter. Existing runtimes are rejected before this helper is called.
ensure_runtime_pip() {
  local py="$1" venv_dir="$2" uv="${3:-}" index_url
  runtime_prefix_matches_venv "$py" "$venv_dir" || return 1
  "$py" -m pip --version >/dev/null 2>&1 && return 0

  warn "pip is missing from the new owned runtime; trying that interpreter's ensurepip."
  "$py" -m ensurepip --upgrade || warn "ensurepip could not seed pip in the new owned runtime."
  "$py" -m pip --version >/dev/null 2>&1 && return 0

  if [[ -n "$uv" ]]; then
    index_url="$(python_dependency_index_url)"
    warn "pip is still missing; using selected uv only inside the new owned runtime."
    "$uv" pip install --index-url "$index_url" -p "$venv_dir" pip || warn "uv could not seed pip in the new owned runtime."
  fi

  runtime_prefix_matches_venv "$py" "$venv_dir" || return 1
  "$py" -m pip --version >/dev/null 2>&1
}

# runtime_venv_state classifies an existing venv path as missing/broken/healthy
# without mutating it: missing (no directory at all), broken (no interpreter,
# too old, prefix does not match, or no working pip), or healthy. Ordinary
# install uses this to refuse silently reusing an existing runtime — see the
# guard at the top of ensure_runtime_venv below.
runtime_venv_state() {
  local venv_dir="$1" py
  [[ -d "$venv_dir" ]] || { printf '%s\n' missing; return 0; }
  py="$(runtime_python_for_venv "$venv_dir")"
  [[ -n "$py" ]] || { printf '%s\n' broken; return 0; }
  "$py" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 11) else 1)' 2>/dev/null || { printf '%s\n' broken; return 0; }
  runtime_prefix_matches_venv "$py" "$venv_dir" || { printf '%s\n' broken; return 0; }
  "$py" -m pip --version >/dev/null 2>&1 || { printf '%s\n' broken; return 0; }
  printf '%s\n' healthy
}

# runtime_health_check is the install postcondition: both the public package
# and its kernel module must import from the selected interpreter, the
# package version must equal the exact manifest/pin version, AND both module
# __file__ paths must resolve physically underneath the selected venv's own
# canonical prefix — not merely be importable. This rejects a same-version
# package injected through an external `.pth` entry, system site-packages, or
# any other interpreter path configuration that would let a bare `import
# lingtai` check pass while the kernel actually loads from outside the venv
# this installer claims is healthy. PYTHONPATH= alone does not cover that
# case, so the check is done in Python against sys.prefix/realpath.
runtime_health_check() {
  local py="$1" expected="${2:-}" output selected_prefix
  selected_prefix="$(cd "$(dirname "$py")/.." 2>/dev/null && pwd -P)" || return 1
  output="$(PYTHONPATH= "$py" - "$expected" "$selected_prefix" <<'PY'
import importlib
import os
import sys

expected = sys.argv[1]
selected_prefix = os.path.realpath(sys.argv[2])
prefix = os.path.realpath(sys.prefix)
if prefix != selected_prefix:
    raise SystemExit(1)
module = importlib.import_module("lingtai")
kernel = importlib.import_module("lingtai.kernel")
version = str(getattr(module, "__version__", ""))
if not version or (expected and version.lstrip("v") != expected.lstrip("v")):
    raise SystemExit(1)
for mod in (module, kernel):
    mod_path = os.path.realpath(getattr(mod, "__file__", "") or "")
    if not mod_path or not (mod_path == selected_prefix or mod_path.startswith(selected_prefix + os.sep)):
        raise SystemExit(1)
print(f"{version}\t{module.__file__}")
PY
  )" || return 1
  [[ "$output" == *$'\t'* ]] || return 1
  printf '%s\n' "$output"
}

# ensure_runtime_venv creates or updates ~/.lingtai-tui/runtime/venv and
# installs the `lingtai` package into it from the pinned release-bundle
# kernel artifact, by explicit local file path — LingTai itself is NEVER
# requested from a package index by name (only third-party dependencies
# resolve via the configured index; see install_kernel_from_bundle). This is
# mirrored by the TUI's own EnsureVenv logic (uv venv --python 3.13 if uv
# exists, else python3 -m venv; verify import; stamp env marker; symlink
# lingtai-agent).
#
# Ordinary (non---update) install refuses to silently reuse an existing
# runtime venv at all — healthy or broken — pointing to fix.sh instead: see
# the guard immediately below, which runs before canonical's own
# repair-loop. That existing loop's own venv-repair-$$-N recreation covers a
# DIFFERENT case (a transient failure discovered DURING this run's own venv
# creation/kernel-install attempt), not an already-occupied runtime from a
# prior run.
#
# On the default release-asset one-command path (BUNDLE_REQUIRED=1), a
# resolved bundle + a successful kernel-artifact install are MANDATORY: any
# failure (no bundle manifest, incoherent manifest, no compatible wheel/sdist,
# checksum mismatch, install failure) is a fail-loud error, not a fallback.
# On a --ref source build (BUNDLE_REQUIRED=0), no bundle is expected to
# exist at all for an arbitrary ref, so this function fails loud with a
# distinct "pass --skip-python" message instead of silently reaching for
# PyPI. --skip-python (alias --skip-venv) is the only way to skip the Python
# runtime entirely; venv creation/repair problems below remain best-effort
# (they warn and defer to the TUI's own venv repair) since those are
# genuinely transient environment issues, not a LingTai-source violation.
ensure_runtime_venv() {
  local bin_dir="$1"
  local venv_dir="$HOME/.lingtai-tui/runtime/venv"
  local uv py repair_attempt

  if [[ "$SKIP_VENV" == "1" ]]; then
    note "Skipping Python runtime venv (--skip-python)."
    return 0
  fi

  if [[ "$LATEST_MAIN_MODE" != "1" && -z "$BUNDLE_MANIFEST_JSON" ]]; then
    if [[ "$BUNDLE_REQUIRED" == "1" ]]; then
      # TUI/kernel decoupled: source-only TUI releases publish no bundle
      # manifest. install_kernel_from_bundle resolves the LATEST kernel
      # release directly; only fail here if even that is impossible.
      if ! resolve_latest_kernel_release; then
        echo "error: no kernel release could be resolved for this install." >&2
        echo "       Tried the latest lingtai-kernel release on $BUNDLE_PROVIDER (with the other provider)." >&2
        echo "       LingTai's Python runtime is installed only from a verified pinned release" >&2
        echo "       artifact, never from PyPI/an index by package name - so this is a hard stop," >&2
        echo "       not a silent fallback." >&2
        echo "       Options:" >&2
        echo "         - Retry (the latest kernel release may not be published yet)." >&2
        echo "         - Pass --skip-python to install the TUI/portal binaries only, then set up the" >&2
        echo "           Python runtime yourself (e.g. from an editable lingtai-kernel checkout)." >&2
        return 1
      fi
    else
      echo "error: --ref/source-ref builds have no pinned kernel release bundle to install from." >&2
      echo "       LingTai's Python runtime is installed only from a verified pinned release" >&2
      echo "       artifact, never from PyPI/an index by package name, so this build cannot" >&2
      echo "       provision the Python runtime automatically." >&2
      echo "       Pass --skip-python to install the TUI/portal binaries only, then set up the" >&2
      echo "       Python runtime yourself — for example an editable install against a local" >&2
      echo "       lingtai-kernel checkout (see RELEASING.md / CLAUDE.md \"Agent venv\")." >&2
      return 1
    fi
  fi

  if [[ "$UPDATE_MODE" != "1" && "$LATEST_MAIN_MODE" != "1" && "$REINSTALL_OK" != "1" ]]; then
    local runtime_root="$HOME/.lingtai-tui/runtime" existing_state
    if ! canonical_runtime_venv "$venv_dir" "$runtime_root" >/dev/null; then
      echo "error: selected runtime venv is not a canonical child of the owned runtime root: $venv_dir" >&2
      echo "       Refusing to adopt or create a venv outside the root this installer owns." >&2
      return 1
    fi
    if [[ -L "$runtime_root" ]]; then
      echo "error: runtime root is a symlink: $runtime_root" >&2
      return 1
    fi
    existing_state="$(runtime_venv_state "$venv_dir")"
    if [[ "$existing_state" != "missing" ]]; then
      echo "error: existing runtime at $venv_dir is $existing_state; ordinary install will not adopt or repair it." >&2
      echo "       Use the standalone fix.sh ($LINGTAI_SCRIPTS_ASSETS/fix.sh) to repair an existing installation, or update.sh ($LINGTAI_SCRIPTS_ASSETS/update.sh) to update one." >&2
      return 1
    fi
  fi

  say "Setting up Python runtime venv at $venv_dir ..."
  if ! ensure_python; then
    if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
      echo "error: --latest requires Python prerequisites to provision kernel main; refusing a partial install." >&2
      return 1
    fi
    warn "Skipping Python runtime venv — Python prerequisites are missing."
    warn "Re-run install after installing Python, or the TUI will create the venv on first launch."
    return 0
  fi

  mkdir -p "$(dirname "$venv_dir")"
  repair_attempt=0

  while true; do
    uv="$(find_uv 2>/dev/null || true)"
    py=""
    if [[ -x "$venv_dir/bin/python" ]]; then
      py="$venv_dir/bin/python"
    elif [[ -x "$venv_dir/bin/python3" ]]; then
      py="$venv_dir/bin/python3"
    fi

    local recreate_reason=""
    if [[ -d "$venv_dir" && -z "$py" ]]; then
      recreate_reason="runtime venv Python is missing"
    elif [[ -n "$py" ]] && ! "$py" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 11) else 1)' 2>/dev/null; then
      recreate_reason="runtime venv Python is older than 3.11"
    fi

    if [[ -n "$recreate_reason" ]]; then
      if [[ "$repair_attempt" != "0" ]]; then
        if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
          echo "error: $recreate_reason after recreate; --latest cannot leave the kernel runtime unprovisioned." >&2
          return 1
        fi
        warn "$recreate_reason after recreate; leaving runtime venv repair to the TUI."
        return 0
      fi
      warn "$recreate_reason; retaining it and provisioning a new runtime venv path."
      venv_dir="$HOME/.lingtai-tui/runtime/venv-repair-$$-1"
      repair_attempt=1
      py=""
    fi

    if [[ -z "$py" ]]; then
      if [[ -n "$uv" ]]; then
        if ! "$uv" venv --python 3.13 "$venv_dir"; then
          if python_ok; then
            warn "uv venv failed; falling back to python3 -m venv"
            uv=""
          else
            if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
              echo "error: uv could not create the kernel-main runtime venv and no supported Python fallback is available." >&2
              return 1
            fi
            warn "uv venv failed and no Python 3.11+ with venv/ensurepip is available; skipping runtime setup."
            warn "Install uv or Python with venv/ensurepip support, then re-run the installer."
            return 0
          fi
        fi
      fi
      if [[ ! -x "$venv_dir/bin/python" && ! -x "$venv_dir/bin/python3" && -z "$uv" ]]; then
        if python_ok; then
          if ! python3 -m venv "$venv_dir"; then
            if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
              echo "error: failed to create the kernel-main runtime venv." >&2
              return 1
            fi
            warn "failed to create venv"
            return 0
          fi
        else
          if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
            echo "error: no supported Python/uv runtime is available for kernel main." >&2
            return 1
          fi
          warn "Cannot create runtime venv: uv is unavailable and no Python 3.11+ with venv/ensurepip is available."
          warn "Install uv or Python with venv/ensurepip support, then re-run the installer."
          return 0
        fi
      fi
      if [[ -x "$venv_dir/bin/python" ]]; then
        py="$venv_dir/bin/python"
      elif [[ -x "$venv_dir/bin/python3" ]]; then
        py="$venv_dir/bin/python3"
      else
        if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
          echo "error: kernel-main runtime venv has no Python interpreter at $venv_dir." >&2
          return 1
        fi
        warn "venv python not found at $venv_dir; skipping runtime setup."
        return 0
      fi
      # Re-check Python version after creating/recreating the venv.
      continue
    fi

    if ! ensure_runtime_pip "$py" "$venv_dir" "$uv"; then
      if [[ "$repair_attempt" == "0" ]]; then
        warn "runtime venv pip is missing and could not be self-healed; retaining it and provisioning a new runtime venv path."
        venv_dir="$HOME/.lingtai-tui/runtime/venv-repair-$$-1"
        repair_attempt=1
        continue
      fi
      if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
        echo "error: kernel-main runtime venv has no pip/uv installer after recreate." >&2
        return 1
      fi
      warn "runtime venv pip is missing after recreate; TUI will repair it on first launch."
      return 0
    fi

    local install_ok=0
    # The pinned release-bundle kernel artifact is the ONLY LingTai install
    # source (guaranteed present at this point — see the BUNDLE_MANIFEST_JSON
    # guard above). Any failure here (incoherent manifest, no compatible
    # wheel/sdist, checksum mismatch, install command failure) is retried
    # once after a venv recreate (a legitimate transient-environment repair,
    # the same pattern every other step in this loop uses), then FAILS LOUD —
    # it never falls back to `pip install lingtai` from an index.
    if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
      if install_kernel_from_main "$py" "$uv"; then
        install_ok=1
      fi
    elif install_kernel_from_bundle "$py" "$uv"; then
      install_ok=1
    fi
    if [[ "$install_ok" != "1" ]]; then
      if [[ "$repair_attempt" == "0" ]]; then
        warn "failed to install the pinned kernel bundle artifact; retaining the venv and provisioning a new runtime venv path."
        venv_dir="$HOME/.lingtai-tui/runtime/venv-repair-$$-1"
        repair_attempt=1
        continue
      fi
      if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
        echo "error: failed to install kernel main commit $KERNEL_MAIN_SHA into the runtime venv after recreate." >&2
      else
        echo "error: failed to install the pinned kernel bundle artifact into the runtime venv after recreate." >&2
        echo "       bundle: $(bundle_manifest_field bundle_id 2>/dev/null || echo "?") kernel: $(bundle_manifest_field kernel_tag 2>/dev/null || echo "?") via $KERNEL_MANIFEST_PROVIDER" >&2
      fi
      echo "       LingTai's Python runtime is never installed from PyPI/an index by package name," >&2
      if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
        echo "       so this is a hard stop rather than a silent fallback. Re-run --latest after fixing the error above." >&2
      else
        echo "       so this is a hard stop rather than a silent fallback. Re-run the installer, or" >&2
        echo "       pass --skip-python to install the TUI/portal binaries only." >&2
      fi
      return 1
    fi

    # Postcondition: version + BOTH modules' __file__ must resolve physically
    # inside this exact venv's prefix — not merely importable — so a
    # same-version package reachable through an external .pth entry or system
    # site-packages can never be mistaken for a healthy owned install.
    if ! runtime_health_check "$py" "$KERNEL_VERSION_INSTALLED" >/dev/null; then
      if [[ "$repair_attempt" == "0" ]]; then
        warn "runtime venv failed import/provenance check; retaining it and provisioning a new runtime venv path."
        venv_dir="$HOME/.lingtai-tui/runtime/venv-repair-$$-1"
        repair_attempt=1
        continue
      fi
      if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
        echo "error: kernel main import/provenance check failed after reinstall; refusing a partial --latest install." >&2
        return 1
      fi
      warn "runtime venv is still unhealthy after reinstall; TUI will repair it on first launch."
      return 0
    fi
    break
  done

  RUNTIME_VENV_DIR="$venv_dir"

  # Stamp the env marker (best-effort — older kernels may lack the subcommand).
  "$py" -m lingtai.venv_resolve env-marker stamp --venv "$venv_dir" >/dev/null 2>&1 || true

  # Symlink lingtai-agent into the chosen bin dir (best-effort).
  if [[ -x "$venv_dir/bin/lingtai-agent" ]]; then
    ln -sfn "$venv_dir/bin/lingtai-agent" "$bin_dir/lingtai-agent" 2>/dev/null \
      || warn "could not symlink lingtai-agent into $bin_dir"
  fi
  return 0
}


# --- kernel bundle artifact install (schema lingtai.kernel.release/v1) ------
#
# Installs the Python `lingtai` runtime from the release-pinned kernel
# artifact named in the TUI bundle manifest, by explicit local file path —
# never `pip install lingtai` against any package index. One package index
# (python_dependency_index_url: an explicit non-empty LINGTAI_PYPI_INDEX_URL,
# else the final bundle provider's selected default) is used ONLY to resolve
# lingtai's own third-party dependencies during that local-path install;
# lingtai itself is never requested from an index.

# kernel_manifest_url_for_provider echoes the kernel release manifest asset
# URL on the given provider/tag, or nothing if unavailable.
kernel_manifest_url_for_provider() {
  local provider="$1" tag="$2" body
  case "$provider" in
    github)
      command -v curl &>/dev/null || return 1
      body="$(curl -fsSL --max-time 15 "${KERNEL_GH_API_BASE}/releases/tags/$tag" 2>/dev/null || true)"
      [[ -n "$body" ]] || return 1
      if printf '%s' "$body" | grep -q '"name"[[:space:]]*:[[:space:]]*"lingtai-kernel-release-manifest.json"'; then
        printf 'https://github.com/Lingtai-AI/lingtai-kernel/releases/download/%s/lingtai-kernel-release-manifest.json' "$tag"
      else
        return 1
      fi
      ;;
    mirror)
      mirror_release_asset_url "$KERNEL_REPO_SLUG" "$tag" "lingtai-kernel-release-manifest.json"
      ;;
    *) return 1 ;;
  esac
}

# update_validate_manifest strictly validates a kernel release manifest
# (schema lingtai.kernel.release/v1): every required top-level key present
# and no others, exact schema/tag/version match, and every artifact's shape,
# digest, and (for wheels) filename/tag self-consistency — this replaces a
# substring schema check that could be satisfied by any JSON containing that
# string anywhere, including in an unrelated field, with a real parser that
# rejects duplicate keys, wrong shapes, and mismatched artifact metadata.
update_validate_manifest() {
  local py="$1" manifest_file="$2" expected_tag="$3"
  "$py" - "$manifest_file" "$expected_tag" <<'PY'
import json
import re
import sys

path, expected_tag = sys.argv[1:]
expected_version = expected_tag[1:] if expected_tag.startswith("v") else expected_tag

def pairs(items):
    result = {}
    for key, value in items:
        if key in result:
            raise ValueError(f"duplicate JSON key: {key}")
        result[key] = value
    return result

def fail(message):
    raise SystemExit(f"invalid kernel release manifest: {message}")

try:
    with open(path, encoding="utf-8") as stream:
        data = json.load(stream, object_pairs_hook=pairs)
except (OSError, ValueError, json.JSONDecodeError) as exc:
    fail(str(exc))

required = {"schema", "kernel_version", "kernel_tag", "commit", "generated_at", "artifacts", "sdist_fallback"}
if not isinstance(data, dict) or set(data) != required:
    fail("unexpected top-level keys")
if data["schema"] != "lingtai.kernel.release/v1":
    fail("unexpected schema")
for key in ("kernel_version", "kernel_tag", "commit", "generated_at", "sdist_fallback"):
    if not isinstance(data[key], str) or not data[key]:
        fail(f"{key} must be a non-empty string")
if data["kernel_tag"] != expected_tag or data["kernel_version"] != expected_version:
    fail(f"manifest is for {data['kernel_tag']}/{data['kernel_version']}, expected {expected_tag}/{expected_version}")
if not isinstance(data["artifacts"], list) or not data["artifacts"]:
    fail("artifacts must be a non-empty list")

seen = set()
has_sdist = False
for index, artifact in enumerate(data["artifacts"]):
    if not isinstance(artifact, dict) or set(artifact) != {"filename", "sha256", "kind", "python_tag", "abi_tag", "platform_tag"}:
        fail(f"artifacts[{index}] has the wrong shape")
    filename = artifact["filename"]
    digest = artifact["sha256"]
    kind = artifact["kind"]
    if not isinstance(filename, str) or not filename or filename in seen:
        fail(f"artifacts[{index}] has an invalid or duplicate filename")
    seen.add(filename)
    if not isinstance(digest, str) or not re.fullmatch(r"[0-9a-f]{64}", digest):
        fail(f"artifacts[{index}].sha256 is not lowercase 64-hex")
    if kind == "wheel":
        if any(not isinstance(artifact[key], str) or not artifact[key] for key in ("python_tag", "abi_tag", "platform_tag")):
            fail(f"artifacts[{index}] wheel tags must be non-empty strings")
        parts = filename[:-4].split("-") if filename.endswith(".whl") else []
        if len(parts) != 5 or parts[0] != "lingtai" or parts[1] != expected_version:
            fail(f"artifacts[{index}] filename is not the selected lingtai version")
        if tuple(parts[2:]) != (artifact["python_tag"], artifact["abi_tag"], artifact["platform_tag"]):
            fail(f"artifacts[{index}] filename tags disagree with metadata")
    elif kind == "sdist":
        has_sdist = True
        if filename != f"lingtai-{expected_version}.tar.gz":
            fail(f"artifacts[{index}] sdist filename is not the selected version")
        if any(artifact[key] is not None for key in ("python_tag", "abi_tag", "platform_tag")):
            fail(f"artifacts[{index}] sdist has wheel tags")
    else:
        fail(f"artifacts[{index}] has unsupported kind {kind!r}")
if not has_sdist or data["sdist_fallback"] not in seen:
    fail("sdist fallback is not a listed sdist")

print(json.dumps(data, sort_keys=True, separators=(",", ":")))
PY
}

# fetch_kernel_manifest resolves the pinned kernel tag/manifest for the
# CURRENT BUNDLE_PROVIDER + the bundle's or release pin's kernel_tag. Falls
# back to the other provider for the SAME kernel tag only (same-tag-fallback
# contract). Populates KERNEL_MANIFEST_JSON and KERNEL_MANIFEST_PROVIDER in
# this shell; returns nonzero if unavailable (or invalid) on either provider.
# The optional second argument is the python3/uv-managed interpreter used for
# strict manifest validation; a caller with no interpreter yet (before the
# runtime venv exists) may omit it to fall back to any system python3.
fetch_kernel_manifest() {
  local kernel_tag="$1" provider="$BUNDLE_PROVIDER" url body other
  local validator="${2:-$(command -v python3 || true)}" manifest_file
  KERNEL_MANIFEST_PROVIDER=""
  KERNEL_MANIFEST_JSON=""

  url="$(kernel_manifest_url_for_provider "$provider" "$kernel_tag" || true)"
  if [[ -z "$url" ]]; then
    other="github"
    [[ "$provider" == "github" ]] && other="mirror"
    # Keep fallback diagnostics on stderr; stdout remains reserved for normal
    # installer output while the manifest is returned through explicit state.
    echo "    $provider has no kernel manifest for $kernel_tag; trying $other for the SAME kernel tag." >&2
    url="$(kernel_manifest_url_for_provider "$other" "$kernel_tag" || true)"
    [[ -n "$url" ]] || return 1
    provider="$other"
  fi

  body="$(curl -fsSL --max-time 30 "$url" 2>/dev/null || true)"
  [[ -n "$body" ]] || return 1
  [[ -n "$validator" ]] || {
    echo "error: Python is required to validate the kernel release manifest at $url" >&2
    return 1
  }
  manifest_file="$(mktemp "${TMPDIR:-/tmp}/lingtai-kernel-manifest-validate.XXXXXX")"
  printf '%s' "$body" > "$manifest_file"
  if ! update_validate_manifest "$validator" "$manifest_file" "$kernel_tag" >/dev/null 2>&1; then
    rm -f "$manifest_file"
    echo "error: kernel manifest at $url failed strict validation" >&2
    return 1
  fi
  rm -f "$manifest_file"

  KERNEL_MANIFEST_PROVIDER="$provider"
  KERNEL_MANIFEST_JSON="$body"
}

# python_platform_tags asks the venv's own Python for compatible wheel tags,
# one per line, most-specific first. Fresh `uv venv` environments intentionally
# contain neither packaging nor pip, so use their implementations when present
# and otherwise emit a conservative dependency-free CPython/OS/arch set for the
# platform wheels this release pipeline publishes. The installer still lets uv
# enforce final wheel compatibility during installation.
python_platform_tags() {
  local py="$1"
  "$py" - <<'PY' 2>/dev/null
import platform
import sys

sys_tags = None
try:
    from packaging.tags import sys_tags
except ModuleNotFoundError:
    try:
        from pip._vendor.packaging.tags import sys_tags  # type: ignore
    except ModuleNotFoundError:
        pass

if sys_tags is not None:
    for tag in sys_tags():
        print(f"{tag.interpreter}-{tag.abi}-{tag.platform}")
    raise SystemExit(0)

interpreter = f"cp{sys.version_info.major}{sys.version_info.minor}"
abi = interpreter
machine = platform.machine().lower()

def emit(platform_tag):
    print(f"{interpreter}-{abi}-{platform_tag}")

if sys.platform == "darwin":
    arch = "arm64" if machine in {"arm64", "aarch64"} else "x86_64"
    version = platform.mac_ver()[0]
    try:
        major, minor = (int(part) for part in version.split(".")[:2])
    except (TypeError, ValueError):
        major, minor = (11, 0) if arch == "arm64" else (10, 13)
    if major >= 11:
        for compatible_major in range(major, 10, -1):
            emit(f"macosx_{compatible_major}_0_{arch}")
        minor = 16
    if arch == "x86_64" and major >= 10:
        for compatible_minor in range(min(minor, 16), 8, -1):
            emit(f"macosx_10_{compatible_minor}_x86_64")
elif sys.platform.startswith("linux"):
    arch = "aarch64" if machine in {"arm64", "aarch64"} else "x86_64"
    libc_name, libc_version = platform.libc_ver()
    try:
        libc_major, libc_minor = (int(part) for part in libc_version.split(".")[:2])
    except (TypeError, ValueError):
        libc_major, libc_minor = 0, 0
    if libc_name == "glibc" and libc_major == 2 and libc_minor >= 17:
        for compatible_minor in range(libc_minor, 16, -1):
            tag = f"manylinux_2_{compatible_minor}_{arch}"
            if compatible_minor == 17:
                tag += f".manylinux2014_{arch}"
            emit(tag)
elif sys.platform == "win32":
    emit("win_amd64" if machine in {"amd64", "x86_64"} else "win_arm64")
PY
}

# select_kernel_wheel picks the first artifact from a kernel manifest JSON
# body whose "<python_tag>-<abi_tag>-<platform_tag>" combination appears in
# the venv's compatible-tag list (most-specific tags are tried first, so an
# exact match wins over a compatible-but-looser one). Echoes
# "<filename> <sha256>" on a match; returns nonzero (and prints nothing) if no
# wheel matches — the caller falls back to the sdist.
select_kernel_wheel() {
  local manifest_json="$1" py="$2" tags combo manifest_file
  tags="$(python_platform_tags "$py")"
  [[ -n "$tags" ]] || return 1

  manifest_file="$(mktemp "${TMPDIR:-/tmp}/lingtai-kernel-manifest.XXXXXX")"
  printf '%s' "$manifest_json" > "$manifest_file"

  while IFS= read -r combo; do
    [[ -n "$combo" ]] || continue
    # Each artifact object is small and single-line-safe to grep for its tag
    # triple; scope the match to one object at a time via a python one-liner
    # for correctness instead of hand-rolled brace matching across wheels.
    # Manifest is passed by FILE PATH (not stdin) so this command can't
    # collide with a heredoc's stdin takeover.
    local hit
    hit="$(python3 - "$manifest_file" "$combo" <<'PY'
import json, sys
data = json.loads(open(sys.argv[1]).read())
combo = sys.argv[2]
for art in data.get("artifacts", []):
    if art.get("kind") != "wheel":
        continue
    if f"{art['python_tag']}-{art['abi_tag']}-{art['platform_tag']}" == combo:
        print(f"{art['filename']} {art['sha256']}")
        break
PY
)"
    if [[ -n "$hit" ]]; then
      printf '%s' "$hit"
      return 0
    fi
  done <<<"$tags"
  return 1
}

# kernel_sdist_fallback echoes "<filename> <sha256>" for the manifest's
# declared sdist_fallback artifact.
kernel_sdist_fallback() {
  local manifest_json="$1" manifest_file
  manifest_file="$(mktemp "${TMPDIR:-/tmp}/lingtai-kernel-manifest.XXXXXX")"
  printf '%s' "$manifest_json" > "$manifest_file"
  python3 - "$manifest_file" <<'PY'
import json, sys
data = json.loads(open(sys.argv[1]).read())
name = data.get("sdist_fallback", "")
for art in data.get("artifacts", []):
    if art.get("filename") == name:
        print(f"{art['filename']} {art['sha256']}")
        break
PY
}

# kernel_artifact_download_url echoes the download URL for a named kernel
# artifact on the given provider/tag.
kernel_artifact_download_url() {
  local provider="$1" tag="$2" name="$3"
  case "$provider" in
    github) printf 'https://github.com/Lingtai-AI/lingtai-kernel/releases/download/%s/%s' "$tag" "$name" ;;
    mirror) printf '%s/dl/%s/%s/%s' "$LINGTAI_WEB_BASE" "$KERNEL_REPO_SLUG" "$tag" "$name" ;;
    *) return 1 ;;
  esac
}

# resolve_latest_kernel_release queries GitHub for the newest published
# kernel release that carries a release manifest. Populates
# KERNEL_LATEST_TAG; returns nonzero if unavailable. Version identity always
# comes from GitHub: the mirror has no listing API and so can never answer
# "what is the latest kernel release" — it only re-serves bytes for an
# already-known tag (see kernel_artifact_download_url above).
resolve_latest_kernel_release() {
  local body tag has_manifest
  KERNEL_LATEST_TAG=""
  body="$(curl -fsSL --max-time 15 "${KERNEL_GH_API_BASE}/releases/latest" 2>/dev/null || true)"
  [[ -n "$body" ]] || return 1
  tag="$(printf '%s' "$body" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
  has_manifest="$(printf '%s' "$body" | grep -c 'lingtai-kernel-release-manifest.json' || true)"
  if [[ -n "$tag" && "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ && "$has_manifest" != "0" ]]; then
    KERNEL_LATEST_TAG="$tag"
    return 0
  fi
  return 1
}


# install_kernel_from_bundle installs the Python `lingtai` runtime from the
# bundle's or exact release pin's kernel release, by explicit local file path
# — this is the ONLY way this script installs LingTai; it is never requested
# from a package index by name. Sets KERNEL_SOURCE
# ("bundle"|"release-pin")/KERNEL_BUNDLE_ID/KERNEL_RELEASE_TAG/
# KERNEL_VERSION_INSTALLED/KERNEL_PROVIDER on success. Returns nonzero
# (installs nothing, KERNEL_SOURCE left untouched) on any failure
# (missing/incoherent kernel manifest, no compatible wheel/sdist, checksum
# mismatch, install command failure) — the caller (ensure_runtime_venv)
# treats that as a fail-loud install error, not a signal to try any other
# source.
install_kernel_from_bundle() {
  local py="$1" uv="$2"

  local kernel_tag kernel_manifest artifact_line fname sha download_url dest index_url kernel_source
  kernel_tag="$(kernel_tag_for_install || true)"

  # TUI release and kernel runtime are DECOUPLED on the default one-command
  # path (BUNDLE_REQUIRED=1): the kernel ALWAYS comes from the latest
  # lingtai-kernel release, independent of the TUI bundle. --ref/source-ref
  # builds (BUNDLE_REQUIRED=0) keep the bundle pin when a bundle is present.
  if [[ "$BUNDLE_REQUIRED" == "1" ]]; then
    if resolve_latest_kernel_release; then
      if [[ -n "$kernel_tag" && "$KERNEL_LATEST_TAG" != "$kernel_tag" ]]; then
        say "Resolving latest kernel release: $KERNEL_LATEST_TAG (TUI/kernel decoupled, bundle pinned $kernel_tag)"
      elif [[ -z "$kernel_tag" ]]; then
        say "Resolving latest kernel release: $KERNEL_LATEST_TAG (no TUI bundle for this release)"
      fi
      kernel_tag="$KERNEL_LATEST_TAG"
      BUNDLE_MANIFEST_KERNEL_FILENAME="lingtai-kernel-release-manifest.json"
    elif [[ -n "$kernel_tag" ]]; then
      warn "Could not resolve the latest kernel release; falling back to the bundle's pinned kernel $kernel_tag."
    else
      note "Could not resolve a kernel release (no bundle, no reachable latest kernel release)."
      return 1
    fi
  fi
  [[ -n "$kernel_tag" ]] || return 1
  kernel_source="$(kernel_source_for_install || true)"
  # When the default path resolved the LATEST kernel release (no bundle, no
  # exact-tag pin), record it as a release-pin source so the metadata writer
  # emits the precise kernel_release_tag.
  if [[ "$BUNDLE_REQUIRED" == "1" && -z "$kernel_source" && -n "$KERNEL_LATEST_TAG" && "$kernel_tag" == "$KERNEL_LATEST_TAG" ]]; then
    kernel_source="release-pin"
  fi
  [[ -n "$kernel_source" ]] || return 1

  if ! fetch_kernel_manifest "$kernel_tag" "$py"; then
    note "Could not fetch the pinned kernel release manifest ($kernel_tag) from GitHub or the lingtai.ai mirror."
    return 1
  fi
  kernel_manifest="$KERNEL_MANIFEST_JSON"
  [[ -n "$kernel_manifest" && -n "$KERNEL_MANIFEST_PROVIDER" ]] || {
    note "Kernel manifest resolution returned incomplete provider state."
    return 1
  }

  artifact_line="$(select_kernel_wheel "$kernel_manifest" "$py" || true)"
  if [[ -z "$artifact_line" ]]; then
    note "No platform wheel in kernel release $kernel_tag matches this Python; using the sdist fallback (extra build toolchain may be required)."
    artifact_line="$(kernel_sdist_fallback "$kernel_manifest" || true)"
  fi
  [[ -n "$artifact_line" ]] || return 1
  fname="${artifact_line%% *}"
  sha="${artifact_line##* }"

  download_url="$(kernel_artifact_download_url "$KERNEL_MANIFEST_PROVIDER" "$kernel_tag" "$fname" || true)"
  [[ -n "$download_url" ]] || return 1

  mkdir -p "$BUILD_DIR/kernel-artifact"
  dest="$BUILD_DIR/kernel-artifact/$fname"
  say "Downloading kernel artifact: $fname (from $KERNEL_MANIFEST_PROVIDER, release $kernel_tag) ..."
  if ! curl -fsSL --max-time 300 -o "$dest" "$download_url"; then
    # Same-tag GitHub fallback on mirror transport unavailability only: the
    # manifest was already validated (possibly from the mirror), so this
    # retries the exact same kernel_tag/fname/sha on GitHub — never a
    # different release, never a re-resolved "latest". No bytes have been
    # accepted yet (the checksum gate below still runs), so this is a
    # transport retry, not a downgrade of an already-verified artifact.
    if [[ "$KERNEL_MANIFEST_PROVIDER" == "mirror" ]]; then
      note "lingtai.ai mirror transport unavailable for $fname; retrying the SAME release from GitHub."
      download_url="$(kernel_artifact_download_url "github" "$kernel_tag" "$fname" || true)"
      if [[ -z "$download_url" ]] || ! curl -fsSL --max-time 300 -o "$dest" "$download_url"; then
        warn "download failed for $fname on both the mirror and GitHub"
        return 1
      fi
      KERNEL_MANIFEST_PROVIDER="github"
    else
      warn "download failed for $download_url"
      return 1
    fi
  fi
  if ! verify_sha256 "$dest" "$sha"; then
    echo "error: checksum mismatch for $fname — refusing to install an unverified kernel artifact." >&2
    echo "       retained mismatched artifact for diagnosis: $dest" >&2
    return 1
  fi
  note "Verified SHA256 for $fname."

  index_url="$(python_dependency_index_url)"
  say "Installing lingtai from local artifact (dependencies resolved via $index_url) ..."
  # Explicit local path: pip/uv never requests the package name "lingtai"
  # from any index here — only third-party dependency resolution uses
  # --index-url. This is the "no pip install lingtai from index" guarantee.
  # One quoted `--index-url "$index_url"` pair and no --extra-index-url, so
  # dependency resolution consults exactly one index.
  if [[ -n "$uv" ]]; then
    "$uv" pip install --index-url "$index_url" -p "$(dirname "$(dirname "$py")")" "$dest" || return 1
  else
    "$py" -m pip install --index-url "$index_url" "$dest" || return 1
  fi

  if ! "$py" -c 'import lingtai; print("lingtai", getattr(lingtai, "__version__", "?"))'; then
    warn "lingtai import failed after bundle install."
    return 1
  fi

  KERNEL_SOURCE="$kernel_source"
  KERNEL_BUNDLE_ID=""
  if [[ "$kernel_source" == "bundle" ]]; then
    KERNEL_BUNDLE_ID="$(bundle_manifest_field bundle_id)"
  fi
  KERNEL_RELEASE_TAG="$kernel_tag"
  KERNEL_VERSION_INSTALLED="$(printf '%s' "$kernel_manifest" | json_string_field kernel_version)"
  KERNEL_PROVIDER="$KERNEL_MANIFEST_PROVIDER"
  return 0
}

# install_kernel_from_main installs the checked-out kernel main source tree by
# local path. Dependencies may use the configured index, but the LingTai source
# itself is never looked up by package name and never falls back to PyPI.
install_kernel_from_main() {
  local py="$1" uv="$2" index_url="${LINGTAI_PYPI_INDEX_URL:-https://pypi.org/simple}"
  [[ -n "$KERNEL_SOURCE_DIR" && -d "$KERNEL_SOURCE_DIR" ]] || return 1
  [[ "$KERNEL_MAIN_SHA" =~ ^[0-9a-f]{40}$ ]] || return 1
  [[ "$(git -C "$KERNEL_SOURCE_DIR" rev-parse HEAD 2>/dev/null || true)" == "$KERNEL_MAIN_SHA" ]] || {
    echo "error: kernel source checkout no longer matches resolved main commit $KERNEL_MAIN_SHA" >&2
    return 1
  }
  say "Installing lingtai from kernel main source ($KERNEL_MAIN_SHA; dependencies via $index_url) ..."
  if [[ -n "$uv" ]]; then
    "$uv" pip install --index-url "$index_url" -p "$(dirname "$(dirname "$py")")" "$KERNEL_SOURCE_DIR" || return 1
  else
    "$py" -m pip install --index-url "$index_url" "$KERNEL_SOURCE_DIR" || return 1
  fi
  "$py" -c 'import lingtai; print("lingtai", getattr(lingtai, "__version__", "?"))' || return 1
  KERNEL_SOURCE="main"
  KERNEL_VERSION_INSTALLED="$("$py" -c 'import lingtai; print(getattr(lingtai, "__version__", "?"))' 2>/dev/null || true)"
  KERNEL_PROVIDER="github"
  return 0
}

# --- install flows -----------------------------------------------------------

# resolve_bin_dir picks the install bin directory honoring --bin-dir/--prefix
# and, for --update, the existing prefix. Prefers user-writable locations; never
# prefers Homebrew.
resolve_bin_dir() {
  if [[ "$UPDATE_MODE" == "1" ]]; then
    BIN_DIR="$(bin_dir_for_prefix "$INSTALL_PREFIX")"
    if [[ ! -d "$BIN_DIR" ]]; then
      echo "error: update target bin dir does not exist: $BIN_DIR" >&2
      exit 1
    fi
    return
  fi
  if [[ -n "$BIN_DIR_OVERRIDE" ]]; then
    BIN_DIR="$BIN_DIR_OVERRIDE"
  elif [[ -n "$INSTALL_PREFIX" ]]; then
    BIN_DIR="$(bin_dir_for_prefix "$INSTALL_PREFIX")"
  elif [[ -w /usr/local/bin ]]; then
    BIN_DIR="/usr/local/bin"
  else
    BIN_DIR="$HOME/.local/bin"
  fi
  mkdir -p "$BIN_DIR"
}

# validate_install_target refuses ordinary (non---update) install over an
# existing managed binary at the selected bin dir — ordinary install is
# first-install-only; adopting or overwriting an existing target requires an
# explicit standalone maintenance asset (update.sh/fix.sh) instead.
validate_install_target() {
  [[ "$BIN_DIR" == /* && "$BIN_DIR" != *$'\n'* && "$BIN_DIR" != *$'\t'* && "$BIN_DIR" != */../* && "$BIN_DIR" != */./* ]] || {
    echo "error: install target is not an exact absolute directory: $BIN_DIR" >&2; return 1;
  }
  [[ ! -L "$BIN_DIR" ]] || { echo "error: install target is a symlink: $BIN_DIR" >&2; return 1; }
  local managed
  for managed in lingtai-tui lingtai-portal lingtai lingtai-agent; do
    if [[ -e "$BIN_DIR/$managed" || -L "$BIN_DIR/$managed" ]]; then
      echo "error: existing managed target $BIN_DIR/$managed was found; ordinary install will not adopt or overwrite it." >&2
      echo "       Use the standalone fix.sh ($LINGTAI_SCRIPTS_ASSETS/fix.sh) to repair an existing installation, or update.sh ($LINGTAI_SCRIPTS_ASSETS/update.sh) to update one." >&2
      return 1
    fi
  done
}

# validate_fresh_install_state refuses ordinary install over an existing
# install receipt or runtime root — checked before any target creation,
# release resolution, download, or binary/runtime mutation, so a different
# empty --bin-dir cannot turn ordinary install into silent adoption of
# pre-existing state elsewhere under $HOME/.lingtai-tui. A deliberate
# --skip-python TUI-only install is the one safe exception for an existing,
# real runtime directory: it does not inspect, adopt, repair, or overwrite that
# legacy runtime, and records no runtime pointer until a later explicit setup.
validate_fresh_install_state() {
  local state_root="$HOME/.lingtai-tui"
  local metadata="$state_root/install.json"
  local runtime_root="$state_root/runtime"
  if [[ -e "$metadata" || -L "$metadata" ]]; then
    echo "error: existing install receipt $metadata was found; ordinary install is first-install-only." >&2
    echo "       Use the standalone update.sh ($LINGTAI_SCRIPTS_ASSETS/update.sh), fix.sh ($LINGTAI_SCRIPTS_ASSETS/fix.sh), or verify.sh ($LINGTAI_SCRIPTS_ASSETS/verify.sh) for an existing installation." >&2
    return 1
  fi
  if [[ -e "$runtime_root" || -L "$runtime_root" ]]; then
    if [[ "$SKIP_VENV" == "1" && -d "$runtime_root" && ! -L "$runtime_root" ]]; then
      note "Preserving existing runtime state at $runtime_root (--skip-python); no runtime will be adopted or changed."
      return 0
    fi
    echo "error: existing runtime state $runtime_root was found; ordinary install will not adopt or repair it." >&2
    echo "       Use the standalone fix.sh ($LINGTAI_SCRIPTS_ASSETS/fix.sh) to repair an existing installation, or pass --skip-python for a TUI/portal-only install that preserves the runtime." >&2
    return 1
  fi
}

# try_release_asset attempts to install prebuilt binaries for the tag. Returns 0
# on success (binaries installed to BIN_DIR), 1 if no asset was usable so the
# caller should fall back to a source build.
try_release_asset() {
  local tag="$1" os arch name url tarball extract_dir provider
  os="$(detect_os)"
  arch="$(detect_arch)"
  if [[ "$os" == "unsupported" || "$arch" == "unsupported" ]]; then
    note "No prebuilt asset for $(uname -s)/$(uname -m); will build from source."
    return 1
  fi
  command -v curl &>/dev/null || { note "curl unavailable; will build from source."; return 1; }

  name="$(asset_name "$tag" "$os" "$arch")"
  if [[ -z "$BUNDLE_MANIFEST_JSON" ]] || [[ "$BUNDLE_TAG" != "$tag" ]]; then
    warn "no validated bundle manifest is bound to TUI tag $tag; refusing the release asset."
    return 1
  fi
  if ! load_bundle_manifest "$BUNDLE_MANIFEST_JSON" "$tag"; then
    warn "validated bundle manifest could not be loaded for $name; refusing the release asset."
    return 1
  fi
  if [[ -z "$BUNDLE_TUI_ARCHIVE_SHA" ]]; then
    note "Validated bundle manifest does not list $name; will build TUI/Portal binaries from source."
    return 1
  fi
  if [[ ! "$BUNDLE_TUI_ARCHIVE_SHA" =~ ^[0-9a-f]{64}$ ]]; then
    warn "validated bundle manifest has no usable digest for $name; refusing the release asset."
    return 1
  fi
  provider="${BUNDLE_PROVIDER:-github}"
  if [[ "$provider" == "mirror" ]]; then
    url="$(mirror_release_asset_url "$REPO_SLUG" "$tag" "$name" || true)"
    if [[ -z "$url" ]]; then
      note "lingtai.ai mirror has no prebuilt asset ($name) for $tag; trying GitHub for the SAME tag."
      url="$(release_asset_url "$tag" "$name" || true)"
      provider="github"
    fi
  else
    url="$(release_asset_url "$tag" "$name" || true)"
  fi
  if [[ -z "$url" ]]; then
    note "Release $tag has no prebuilt asset ($name) on GitHub or the lingtai.ai mirror; will build from source."
    return 1
  fi

  say "Downloading prebuilt binaries: $name (from $provider)"
  mkdir -p "$BUILD_DIR"
  tarball="$BUILD_DIR/$name"
  extract_dir="$BUILD_DIR/asset"
  mkdir -p "$extract_dir"
  if ! curl -fsSL --max-time 120 -o "$tarball" "$url"; then
    warn "download failed for $url; will build from source."
    return 1
  fi

  # Checksum verification: the sidecar .sha256 is fetched from the SAME
  # provider/URL as the tarball itself so a fallback never mixes providers
  # mid-artifact. A missing/unfetchable sidecar is a hard stop for this
  # asset (not silently trusted) — the caller falls back to a source build.
  local sha_url sha_expected
  sha_url="${url}.sha256"
  sha_expected="$(curl -fsSL --max-time 30 "$sha_url" 2>/dev/null | cut -d' ' -f1 || true)"
  if [[ ! "$sha_expected" =~ ^[0-9a-f]{64}$ ]]; then
    warn "could not fetch checksum sidecar for $name; will build from source rather than install unverified bytes."
    return 1
  fi
  if [[ "$sha_expected" != "$BUNDLE_TUI_ARCHIVE_SHA" ]]; then
    warn "provider checksum sidecar disagrees with bundle manifest for $name; refusing mixed provenance."
    return 2
  fi
  if ! verify_sha256 "$tarball" "$BUNDLE_TUI_ARCHIVE_SHA"; then
    echo "error: downloaded bytes for $name disagree with the bundle manifest; refusing this tag." >&2
    return 2
  fi
  note "Verified SHA256 for $name."

  if ! tar -xzf "$tarball" -C "$extract_dir"; then
    warn "could not extract $tarball; will build from source."
    return 1
  fi

  local tui portal
  tui="$(find "$extract_dir" -type f -name lingtai-tui | head -1)"
  if [[ -z "$tui" ]]; then
    warn "asset $name did not contain lingtai-tui; will build from source."
    return 1
  fi

  install -m 755 "$tui" "$BIN_DIR/lingtai-tui"
  PORTAL_PATH=""
  if [[ "$SKIP_PORTAL" != "1" ]]; then
    portal="$(find "$extract_dir" -type f -name lingtai-portal | head -1)"
    if [[ -n "$portal" ]]; then
      install -m 755 "$portal" "$BIN_DIR/lingtai-portal"
      PORTAL_PATH="$BIN_DIR/lingtai-portal"
    fi
  fi

  ensure_lingtai_alias "$BIN_DIR"
  VERSION="$tag"
  RESOLVED_REF="$tag"
  RESOLVED_COMMIT=""
  INSTALL_KIND="release-asset"
  # Verify the downloaded binary reports the expected version.
  verify_tui_binary_version "$BIN_DIR/lingtai-tui" "$tag" || {
    warn "prebuilt lingtai-tui version mismatch; will rebuild from source."
    return 1
  }
  return 0
}

# build_from_source clones REF (or the release source tarball for a tag) and
# builds both binaries. Installs to BIN_DIR. Sets VERSION/RESOLVED_*/PORTAL_PATH.
build_from_source() {
  local ref="$1" requested_tag source_tarball

  requested_tag="$(release_tag_name "$ref")"
  mkdir -p "$(dirname "$BUILD_DIR")"
  rm -rf "$BUILD_DIR"

  if [[ -n "$requested_tag" ]]; then
    # Release installs must stay GitHub-Release based even when no prebuilt
    # asset exists. Use the release source tarball instead of cloning raw main.
    ensure_build_deps 0
    command -v curl &>/dev/null || { echo "error: curl is required to download the release source tarball" >&2; exit 1; }
    command -v tar &>/dev/null || { echo "error: tar is required to extract the release source tarball" >&2; exit 1; }
    say "Downloading lingtai release source ($requested_tag) ..."
    source_tarball="$TMPDIR/lingtai-$requested_tag-src-$$.tar.gz"
    curl -fsSL --max-time 120 \
      -o "$source_tarball" \
      "https://github.com/${REPO_SLUG}/archive/refs/tags/${requested_tag}.tar.gz"
    mkdir -p "$BUILD_DIR"
    tar -xzf "$source_tarball" -C "$BUILD_DIR" --strip-components 1
    rm -f "$source_tarball"
    VERSION="$requested_tag"
    RESOLVED_REF="$requested_tag"
    RESOLVED_COMMIT="$(git ls-remote --tags "$REPO" "refs/tags/$requested_tag" 2>/dev/null | awk '{print $1}' | head -1 || true)"
  else
    ensure_build_deps 1
    say "Cloning lingtai ($ref) ..."
    if ! git clone --depth 1 --branch "$ref" "$REPO" "$BUILD_DIR" 2>/dev/null; then
      # --branch only resolves branches and tags; fall back to a default clone
      # plus an explicit fetch for commit SHAs and other refs.
      git clone --depth 1 "$REPO" "$BUILD_DIR"
      if [[ "$ref" != "main" ]]; then
        if ! (cd "$BUILD_DIR" && git fetch --depth 1 origin "$ref" && git checkout --quiet FETCH_HEAD); then
          echo "error: ref '$ref' not found in $REPO" >&2
          exit 1
        fi
      fi
    fi

    VERSION="$(version_for_checkout "$BUILD_DIR" "$ref")"
    RESOLVED_REF="$(resolved_ref_for_checkout "$BUILD_DIR")"
    RESOLVED_COMMIT="$(git -C "$BUILD_DIR" rev-parse HEAD)"
  fi
  INSTALL_KIND="source-build"

  ensure_go_for_source "$BUILD_DIR"

  say "Building lingtai-tui ($VERSION) ..."
  (cd "$BUILD_DIR/tui" && CGO_ENABLED=0 go build -buildvcs=false -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/lingtai-tui" .)

  PORTAL_BUILT=0
  if [[ "$SKIP_PORTAL" == "1" ]]; then
    note "Skipping portal (--skip-portal)."
  else
    if ensure_node_for_portal; then
      say "Building lingtai-portal ($VERSION) ..."
      if (cd "$BUILD_DIR/portal/web" && npm ci --silent && npm run build --silent) &&          (cd "$BUILD_DIR/portal" && CGO_ENABLED=0 go build -buildvcs=false -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/lingtai-portal" .); then
        PORTAL_BUILT=1
      else
        warn "Skipping portal — portal build failed; continuing with lingtai-tui only."
        note "$(portal_node_requirement_note)"
      fi
    else
      warn "Skipping portal — could not prepare a supported Node.js/npm toolchain."
      note "$(portal_node_requirement_note)"
    fi
  fi

  # Install binaries.
  PORTAL_PATH=""
  if [[ "$UPDATE_MODE" == "1" ]]; then
    local stage_bin="$BUILD_DIR/stage/bin"
    mkdir -p "$stage_bin"
    install -m 755 "$BUILD_DIR/lingtai-tui" "$stage_bin/lingtai-tui"
    verify_tui_binary_version "$stage_bin/lingtai-tui" "$VERSION"
    say "Installing update to $BIN_DIR ..."
    install_binary_atomically "$stage_bin/lingtai-tui" "$BIN_DIR/lingtai-tui"
    if [[ "$PORTAL_BUILT" == "1" ]]; then
      install -m 755 "$BUILD_DIR/lingtai-portal" "$stage_bin/lingtai-portal"
      install_binary_atomically "$stage_bin/lingtai-portal" "$BIN_DIR/lingtai-portal"
      PORTAL_PATH="$BIN_DIR/lingtai-portal"
    fi
  else
    say "Installing to $BIN_DIR ..."
    install -m 755 "$BUILD_DIR/lingtai-tui" "$BIN_DIR/lingtai-tui"
    if [[ "$PORTAL_BUILT" == "1" ]]; then
      install -m 755 "$BUILD_DIR/lingtai-portal" "$BIN_DIR/lingtai-portal"
      PORTAL_PATH="$BIN_DIR/lingtai-portal"
    fi
  fi
  ensure_lingtai_alias "$BIN_DIR"
  if [[ "$UPDATE_MODE" == "1" ]]; then
    verify_tui_binary_version "$BIN_DIR/lingtai-tui" "$VERSION"
  fi
}

# build_latest_from_main resolves and pins both repositories before building. It
# deliberately does not consult release metadata: --latest is a separate,
# explicit current-main mode and never falls back to a stable tag or bundle.
build_latest_from_main() {
  local actual_kernel_sha
  TUI_MAIN_SHA="$(resolve_main_branch_sha "$REPO")" || {
    echo "error: could not resolve the full TUI main commit from $REPO" >&2
    return 1
  }
  KERNEL_MAIN_SHA="$(resolve_main_branch_sha "$KERNEL_REPO")" || {
    echo "error: could not resolve the full kernel main commit from $KERNEL_REPO" >&2
    return 1
  }
  say "Resolved TUI main commit: $TUI_MAIN_SHA"
  say "Resolved kernel main commit: $KERNEL_MAIN_SHA"

  # Reuse the existing source-build path for the TUI. Its shallow clone is
  # accepted only when it lands on the exact SHA resolved above.
  build_from_source main
  if [[ "${RESOLVED_COMMIT:-}" != "$TUI_MAIN_SHA" ]]; then
    echo "error: TUI main moved during checkout (resolved $TUI_MAIN_SHA, cloned ${RESOLVED_COMMIT:-unknown})" >&2
    return 1
  fi

  KERNEL_SOURCE_DIR="$BUILD_DIR/kernel"
  ensure_build_deps 1
  say "Cloning lingtai-kernel (main at $KERNEL_MAIN_SHA) ..."
  git clone --depth 1 --branch main "$KERNEL_REPO" "$KERNEL_SOURCE_DIR"
  actual_kernel_sha="$(git -C "$KERNEL_SOURCE_DIR" rev-parse HEAD)"
  if [[ "$actual_kernel_sha" != "$KERNEL_MAIN_SHA" ]]; then
    echo "error: kernel main moved during checkout (resolved $KERNEL_MAIN_SHA, cloned $actual_kernel_sha)" >&2
    return 1
  fi
  TUI_MAIN_SHA="$RESOLVED_COMMIT"
  KERNEL_MAIN_SHA="$actual_kernel_sha"
  say "Using TUI main commit: $TUI_MAIN_SHA"
  say "Using kernel main commit: $KERNEL_MAIN_SHA"
}

# normalize_go_version prints MAJOR.MINOR.PATCH for Go language/toolchain
# versions (for example: 1.26 -> 1.26.0, go1.26.1 -> 1.26.1).
normalize_go_version() {
  local version="${1#go}"
  if [[ "$version" =~ ^([0-9]+)\.([0-9]+)$ ]]; then
    printf '%s.%s.0\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}"
    return 0
  fi
  if [[ "$version" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    printf '%s.%s.%s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}"
    return 0
  fi
  return 1
}

# go_version_ge returns success when $1 >= $2 using numeric major/minor/patch
# comparison. Both inputs may optionally include the leading "go" prefix.
go_version_ge() {
  local have required hmaj hmin hpatch rmaj rmin rpatch
  have="$(normalize_go_version "$1")" || return 1
  required="$(normalize_go_version "$2")" || return 1
  IFS=. read -r hmaj hmin hpatch <<<"$have"
  IFS=. read -r rmaj rmin rpatch <<<"$required"
  (( hmaj > rmaj )) && return 0
  (( hmaj < rmaj )) && return 1
  (( hmin > rmin )) && return 0
  (( hmin < rmin )) && return 1
  (( hpatch >= rpatch ))
}

installed_go_version() {
  command -v go &>/dev/null || return 1
  go version 2>/dev/null | sed -n 's/^go version go\([0-9][0-9.]*\).*/\1/p' | head -1
}

required_go_version_for_source() {
  local source_dir="$1" version
  version="$(awk '$1 == "go" { print $2; exit }' "$source_dir/tui/go.mod" 2>/dev/null || true)"
  [[ -n "$version" ]] || return 1
  normalize_go_version "$version"
}

go_toolchain_archive_name() {
  local version="$1" os="$2" arch="$3"
  printf 'go%s.%s-%s.tar.gz\n' "$version" "$os" "$arch"
}

go_toolchain_download_url() {
  local version="$1" os="$2" arch="$3"
  printf '%s/%s\n' "${GO_DL_BASE%/}" "$(go_toolchain_archive_name "$version" "$os" "$arch")"
}

install_go_toolchain() {
  local version="$1" os arch root archive url fallback_url installed
  os="$(detect_os)"
  arch="$(detect_arch)"
  if [[ "$os" == "unsupported" || "$arch" == "unsupported" ]]; then
    echo "error: Go $version is required, but automatic Go toolchain download is unsupported on $(uname -s)/$(uname -m)." >&2
    echo "Install Go $version or newer manually, then re-run this installer." >&2
    exit 1
  fi
  command -v curl &>/dev/null || { echo "error: curl is required to download Go $version" >&2; exit 1; }
  command -v tar &>/dev/null || { echo "error: tar is required to extract Go $version" >&2; exit 1; }

  root="$BUILD_DIR/go-toolchain"
  archive="$root/$(go_toolchain_archive_name "$version" "$os" "$arch")"
  rm -rf "$root"
  mkdir -p "$root"
  url="$(go_toolchain_download_url "$version" "$os" "$arch")"
  fallback_url="https://dl.google.com/go/$(go_toolchain_archive_name "$version" "$os" "$arch")"

  say "Downloading Go $version toolchain for source build ($os/$arch) ..."
  if ! curl -fsSL --retry 3 --max-time 300 -o "$archive" "$url"; then
    if [[ "$url" != "$fallback_url" ]]; then
      warn "Go download failed from $url; retrying $fallback_url"
      curl -fsSL --retry 3 --max-time 300 -o "$archive" "$fallback_url"
    else
      return 1
    fi
  fi
  tar -xzf "$archive" -C "$root"
  export PATH="$root/go/bin:$PATH"
  installed="$(installed_go_version || true)"
  if ! go_version_ge "$installed" "$version"; then
    echo "error: downloaded Go toolchain is $installed, expected $version or newer" >&2
    exit 1
  fi
}

ensure_go_for_source() {
  local source_dir="$1" required installed
  required="$(required_go_version_for_source "$source_dir")" || {
    echo "error: could not read required Go version from $source_dir/tui/go.mod" >&2
    exit 1
  }
  installed="$(installed_go_version || true)"
  if [[ -n "$installed" ]] && go_version_ge "$installed" "$required"; then
    note "Using Go $installed for source build (requires >= $required)."
    return 0
  fi
  if [[ -n "$installed" ]]; then
    note "Installed Go $installed is older than required $required; using official Go toolchain for this build."
  else
    note "Go is not installed; using official Go $required toolchain for this build."
  fi
  install_go_toolchain "$required"
}

normalize_node_version() {
  local version="${1#v}"
  if [[ "$version" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    printf '%s.%s.%s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}" "${BASH_REMATCH[3]}"
    return 0
  fi
  return 1
}

installed_node_version() {
  command -v node &>/dev/null || return 1
  node --version 2>/dev/null | sed -n 's/^v\([0-9][0-9.]*\)$/\1/p' | head -1
}

portal_node_supported() {
  local version major minor patch
  version="$(normalize_node_version "$1")" || return 1
  IFS=. read -r major minor patch <<<"$version"
  if (( major == 20 )); then
    (( minor >= 19 ))
    return
  fi
  if (( major == 22 )); then
    (( minor >= 12 ))
    return
  fi
  (( major > 22 ))
}

portal_node_requirement_note() {
  echo "Node.js 20.19+ or 22.12+ is required to build lingtai-portal. The installer can use an official temporary Node toolchain; if that download fails, upgrade Node and re-run the installer to add the portal binary."
}

node_toolchain_arch() {
  case "$(detect_arch)" in
    amd64) echo "x64" ;;
    arm64) echo "arm64" ;;
    *) echo "unsupported" ;;
  esac
}

node_toolchain_archive_name() {
  local version="$1" os="$2" arch="$3"
  printf 'node-v%s-%s-%s.tar.gz\n' "$version" "$os" "$arch"
}

node_toolchain_download_url() {
  local version="$1" os="$2" arch="$3"
  printf '%s/v%s/%s\n' "${NODE_DL_BASE%/}" "$version" "$(node_toolchain_archive_name "$version" "$os" "$arch")"
}

install_node_toolchain() {
  local version="${1:-$NODE_TOOLCHAIN_VERSION}" os arch root archive url dirname installed
  os="$(detect_os)"
  arch="$(node_toolchain_arch)"
  if [[ "$os" == "unsupported" || "$arch" == "unsupported" ]]; then
    warn "Automatic Node.js toolchain download is unsupported on $(uname -s)/$(uname -m)."
    return 1
  fi
  command -v curl &>/dev/null || { warn "curl is required to download Node.js $version"; return 1; }
  command -v tar &>/dev/null || { warn "tar is required to extract Node.js $version"; return 1; }

  root="$BUILD_DIR/node-toolchain"
  archive="$root/$(node_toolchain_archive_name "$version" "$os" "$arch")"
  dirname="node-v${version}-${os}-${arch}"
  rm -rf "$root"
  mkdir -p "$root"
  url="$(node_toolchain_download_url "$version" "$os" "$arch")"

  say "Downloading Node.js $version toolchain for portal build ($os/$arch) ..."
  if ! curl -fsSL --retry 3 --max-time 300 -o "$archive" "$url"; then
    warn "Node.js download failed from $url"
    return 1
  fi
  if ! tar -xzf "$archive" -C "$root"; then
    warn "Node.js archive extraction failed"
    return 1
  fi
  export PATH="$root/$dirname/bin:$PATH"
  installed="$(installed_node_version || true)"
  if [[ -z "$installed" ]] || ! portal_node_supported "$installed"; then
    warn "Downloaded Node.js toolchain is ${installed:-unavailable}, expected $version or another supported version"
    return 1
  fi
}

ensure_node_for_portal() {
  local installed
  installed="$(installed_node_version || true)"
  if [[ -n "$installed" ]] && portal_node_supported "$installed" && command -v npm &>/dev/null; then
    note "Using Node.js $installed for portal build."
    return 0
  fi
  if [[ -n "$installed" ]]; then
    warn "Node.js $installed is not supported for portal build; using official Node.js $NODE_TOOLCHAIN_VERSION for this build."
  else
    warn "Node.js is not available; using official Node.js $NODE_TOOLCHAIN_VERSION for the portal build."
  fi
  install_node_toolchain "$NODE_TOOLCHAIN_VERSION"
}


# ensure_build_deps checks/installs non-Go source-build dependencies. Go is
# validated after the source tree is available, because tui/go.mod declares the
# required version and distro packages (for example Ubuntu jammy Go 1.18) may be
# too old.
ensure_build_deps() {
  local need_git="${1:-1}"
  if [[ "$need_git" == "1" ]] && ! command -v git &>/dev/null; then
    if command -v apt-get &>/dev/null && apt_install "git (build dependency)" git; then
      :
    else
      echo "error: git is required for --ref source builds but not found. Install it with:" >&2
      suggest_install git
      exit 1
    fi
  fi
}

# --- main --------------------------------------------------------------------

main() {
parse_args "$@"

if [[ -L "$HOME/.lingtai-tui" ]]; then
  echo "error: $HOME/.lingtai-tui is a symlink; refusing redirected install state." >&2
  exit 1
fi

# Remove the build directory even when a build or install step fails midway.
cleanup() {
  cd / 2>/dev/null || true
  rm -rf "$BUILD_DIR"
}
trap cleanup EXIT

if is_wsl; then
  say "Detected Windows Subsystem for Linux (WSL)."
  note "Binaries and the Python runtime install into your Linux home ($HOME)."
  note "Run lingtai-tui from your WSL shell, not Windows PowerShell."
fi

# Auto-detect CN-restricted networks. If proxy.golang.org is unreachable
# within 3 seconds (typical on mainland China without VPN), fall back to
# CN-accessible mirrors for Go modules, the Go checksum database, and npm.
# Only relevant when we build from source, but harmless otherwise. Explicit
# pre-set env vars are preserved.
if command -v curl &>/dev/null && \
   [ -z "${GOPROXY:-}" ] && \
   ! curl -sSfL --max-time 3 -o /dev/null \
     "https://proxy.golang.org/github.com/golang/go/@latest" 2>/dev/null; then
  say "proxy.golang.org unreachable; using China-friendly build mirrors."
  export GOPROXY="https://goproxy.cn,direct"
  export GOSUMDB="sum.golang.google.cn"
  export NPM_CONFIG_REGISTRY="https://registry.npmmirror.com"
fi

resolve_bin_dir
if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
  build_latest_from_main || exit 1
else
  # Ordinary install is first-install-only: it never adopts, overwrites, or
  # repairs an existing TUI target or receipt/runtime state. --update is the
  # one existing, tested exception (an explicit in-place re-run against a
  # known --prefix). --latest never reaches this branch at all (it returned
  # via build_latest_from_main above), so this specific pair of checks is
  # --latest-exempt structurally, not by the UPDATE_MODE test below — but
  # --latest is exempted explicitly, the same way --update is, at every other
  # guard downstream that DOES run for both modes: ensure_runtime_venv's
  # existing-runtime check and both write_install_metadata no-clobber sites.
  # --latest is a separate, already-documented re-runnable current-main dev
  # install with its own re-run semantics, identical in spirit to --update's.
  #
  # REINSTALL_OK is the plain re-run path Jason asked for: when the default
  # one-command path (no --update/--ref/--latest) finds an existing receipt,
  # re-running install.sh reinstalls in place — binaries and runtime are
  # refreshed to the resolved release, the receipt is republished, and local
  # credentials/config (under $HOME/.lingtai, .secrets, etc.) are never
  # touched: the installer only ever manages BIN_DIR, the runtime venv, and
  # install.json. If no receipt exists yet, REINSTALL_OK stays 0 and the
  # first-install-only guards below behave exactly as before.
  if [[ "$UPDATE_MODE" != "1" && "$LATEST_MAIN_MODE" != "1" && -z "$REF" && -e "$HOME/.lingtai-tui/install.json" ]]; then
    REINSTALL_OK=1
    say "Existing installation detected; reinstalling in place (binaries and runtime refreshed, credentials untouched)."
  fi
  if [[ "$UPDATE_MODE" != "1" && "$REINSTALL_OK" != "1" ]]; then
    validate_fresh_install_state || exit 1
    validate_install_target || exit 1
  fi
  resolve_source_provider
if [[ "$BUNDLE_PROVIDER" == "mirror" ]]; then
  say "Source: lingtai.ai download-acceleration mirror ($LINGTAI_WEB_BASE) — override with --source github or LINGTAI_SOURCE=github."
fi

# Resolve one bundle (TUI tag + bundle manifest, which pins an exact kernel
# release) up front, on BUNDLE_PROVIDER, once. Every subsequent step —
# try_release_asset, build_from_source's tag-based source-tarball path, and
# the kernel artifact install in ensure_runtime_venv — reuses this same
# BUNDLE_TAG/BUNDLE_MANIFEST_JSON.
#
# This is the default release-asset one-command path (no --ref, not
# --update): a pinned kernel bundle is REQUIRED here. LingTai must never be
# installed from a package index by name, so if no bundle manifest can be
# resolved, ensure_runtime_venv below fails loud instead of silently
# installing from PyPI — see BUNDLE_REQUIRED.
if [[ -z "$REF" ]]; then
  BUNDLE_REQUIRED=1
  if fetch_bundle_manifest; then
    note "Resolved bundle $BUNDLE_TAG via $BUNDLE_PROVIDER (kernel $(bundle_manifest_field kernel_tag))."
  else
    warn "No bundle manifest available for $([[ -n "$VERSION" ]] && echo "$VERSION" || echo "the latest release") on GitHub or the lingtai.ai mirror."
    # Source-only TUI releases (no dual bundle manifest) instead commit an
    # exact kernel-release.json pin at the same tag — try that before failing
    # loud. Never re-resolves "latest" a second time; consumes BUNDLE_TAG,
    # which fetch_bundle_manifest sets even on its own failure.
    if [[ -n "$(release_tag_name "$BUNDLE_TAG")" ]] && fetch_kernel_pin "$BUNDLE_TAG"; then
      note "Resolved kernel release pin $KERNEL_PIN_TAG from TUI $KERNEL_PIN_TUI_TAG via $KERNEL_PIN_PROVIDER."
    else
      warn "No valid kernel release pin for exact TUI tag $BUNDLE_TAG."
    fi
  fi
fi

# Decide what to install.
#   --update      : source-compatible in-place update for the given tag
#   --ref         : explicit source build of that ref
#   --version tag : that release (asset, else source tarball)
#   default       : latest release (asset, else source tarball)
if [[ "$UPDATE_MODE" == "1" ]]; then
  # The TUI source updater expects a source-compatible update. Try the prebuilt
  # asset first (fast), then fall back to building the tag from source.
  TARGET_TAG="${VERSION:-$BUNDLE_TAG}"
  [[ -n "$TARGET_TAG" ]] || { echo "error: --update could not resolve a release tag" >&2; exit 1; }
  VERSION="$TARGET_TAG"
  if [[ "$FROM_SOURCE" != "1" ]]; then
    if try_release_asset "$TARGET_TAG"; then
      :
    else
      asset_rc=$?
      [[ "$asset_rc" != "2" ]] || exit 1
      build_from_source "$TARGET_TAG"
    fi
  else
    build_from_source "$TARGET_TAG"
  fi
elif [[ -n "$REF" ]]; then
  build_from_source "$REF"
else
  TARGET_TAG="$VERSION"
  if [[ -z "$TARGET_TAG" ]]; then
    if [[ -n "$BUNDLE_TAG" ]]; then
      # Reuse the tag already resolved by fetch_bundle_manifest above instead
      # of re-querying "latest" a second time (which could race to a newer
      # tag between the two calls and silently combine a bundle from one
      # release with TUI binaries from another).
      TARGET_TAG="$BUNDLE_TAG"
      say "Latest release is $TARGET_TAG"
    else
      say "Resolving latest release ..."
      # Version identity always comes from GitHub: the mirror has no listing
      # API and so cannot answer "latest" (see resolve_latest_kernel_release
      # for the same rule on the kernel side).
      TARGET_TAG="$(latest_release_tag || true)"
      if [[ -z "$TARGET_TAG" ]]; then
        echo "error: could not determine the latest release tag from GitHub." >&2
        echo "       Pass one explicitly: ./install.sh --version vX.Y.Z" >&2
        exit 1
      fi
      say "Latest release is $TARGET_TAG"
    fi
  fi
  if [[ -z "$(release_tag_name "$TARGET_TAG")" ]]; then
    warn "'$TARGET_TAG' is not a vX.Y.Z release tag; treating it as a source ref."
    build_from_source "$TARGET_TAG"
  elif [[ "$FROM_SOURCE" != "1" ]]; then
    if try_release_asset "$TARGET_TAG"; then
      :
    else
      asset_rc=$?
      [[ "$asset_rc" != "2" ]] || exit 1
      build_from_source "$TARGET_TAG"
    fi
  else
    build_from_source "$TARGET_TAG"
  fi
fi

fi

# Provision the pinned runtime before recording install metadata. This makes
# kernel_source and its bundle fields a postcondition of verified provisioning,
# never a claim about a partially completed install.
if ! ensure_runtime_venv "$BIN_DIR"; then
  echo "error: LingTai install incomplete — the TUI/portal binaries installed, but the" >&2
  if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
    echo "       Python runtime could not be provisioned from kernel main commit $KERNEL_MAIN_SHA." >&2
  else
    echo "       Python runtime could not be provisioned from a verified pinned bundle." >&2
  fi
  if [[ "$LATEST_MAIN_MODE" == "1" ]]; then
    echo "       See the error above and re-run --latest after fixing it." >&2
  else
    echo "       See the error above. Re-run, or pass --skip-python if TUI-only is intended." >&2
  fi
  exit 1
fi

# Record install metadata for the TUI source updater only after the runtime
# gate above succeeds (or --skip-python explicitly opted out).
GLOBAL_DIR="$HOME/.lingtai-tui"
PREFIX="$(prefix_for_bin_dir "$BIN_DIR")"
REQUESTED_REF="${REF:-${VERSION:-main}}"
write_install_metadata \
  "$GLOBAL_DIR" \
  "$PREFIX" \
  "$BIN_DIR" \
  "$REPO" \
  "$REQUESTED_REF" \
  "${RESOLVED_REF:-$VERSION}" \
  "${RESOLVED_COMMIT:-}" \
  "$VERSION" \
  "$BIN_DIR/lingtai-tui" \
  "$PORTAL_PATH"
say "Wrote install metadata to $GLOBAL_DIR/install.json"

if should_install_desktop; then
  if ! register_desktop_bootstrap; then
    echo "error: LingTai TUI/runtime installation succeeded and its receipt is valid," >&2
    echo "       but the lazy macOS Desktop command could not be registered." >&2
    echo "       No Desktop App state or Desktop network access occurred; use --skip-desktop" >&2
    echo "       to keep the completed TUI/Portal installation without that command." >&2
    exit 1
  fi
fi

say "Done. $("$BIN_DIR/lingtai-tui" version 2>&1 || echo "$VERSION")"

# Tell the user how to put BIN_DIR on PATH if it isn't already.
print_path_hint "$BIN_DIR"
}

if [[ "${LINGTAI_INSTALL_SH_SOURCE_ONLY:-0}" != "1" ]]; then
  main "$@"
fi
