#!/usr/bin/env bash
# Install lingtai-tui and lingtai-portal from source.
# Builds from main branch and installs to Homebrew's bin directory.
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/huangzesen/lingtai/main/install.sh | bash
#
# To install a specific branch/tag:
#   curl -sSL https://raw.githubusercontent.com/huangzesen/lingtai/main/install.sh | bash -s -- --ref v0.4.43
#
set -euo pipefail

REF="main"
REPO="https://github.com/huangzesen/lingtai.git"
TMPDIR="${TMPDIR:-/tmp}"
BUILD_DIR="$TMPDIR/lingtai-install-$$"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --ref) REF="$2"; shift 2 ;;
    *) echo "unknown flag: $1"; exit 1 ;;
  esac
done

# Detect install path — use Homebrew prefix if available, else /usr/local/bin
if command -v brew &>/dev/null; then
  BIN_DIR="$(brew --prefix)/bin"
else
  BIN_DIR="/usr/local/bin"
fi

# Check dependencies — install via brew if missing
if ! command -v git &>/dev/null; then
  echo "error: git is required but not found"
  exit 1
fi

if ! command -v go &>/dev/null; then
  if command -v brew &>/dev/null; then
    echo "==> Installing Go via Homebrew ..."
    brew install go
  else
    echo "error: go is required but not found (install with: brew install go)"
    exit 1
  fi
fi

echo "==> Cloning lingtai ($REF) ..."
git clone --depth 1 --branch "$REF" "$REPO" "$BUILD_DIR" 2>/dev/null || \
  git clone --depth 1 "$REPO" "$BUILD_DIR"

if [[ "$REF" != "main" ]]; then
  cd "$BUILD_DIR" && git fetch --depth 1 origin "$REF" && git checkout FETCH_HEAD 2>/dev/null || true
fi

VERSION=$(cd "$BUILD_DIR" && git describe --tags --always 2>/dev/null || echo "dev")

echo "==> Building lingtai-tui ($VERSION) ..."
cd "$BUILD_DIR/tui"
CGO_ENABLED=0 go build -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/lingtai-tui" .

echo "==> Building lingtai-portal ($VERSION) ..."
cd "$BUILD_DIR/portal"
if command -v npm &>/dev/null; then
  cd web && npm ci --silent && npm run build --silent && cd ..
  CGO_ENABLED=0 go build -ldflags "-X main.version=$VERSION" -o "$BUILD_DIR/lingtai-portal" .
else
  echo "    (skipping portal — npm not found)"
fi

echo "==> Installing to $BIN_DIR ..."
install -m 755 "$BUILD_DIR/lingtai-tui" "$BIN_DIR/lingtai-tui"
if [[ -f "$BUILD_DIR/lingtai-portal" ]]; then
  install -m 755 "$BUILD_DIR/lingtai-portal" "$BIN_DIR/lingtai-portal"
fi

echo "==> Cleaning up ..."
rm -rf "$BUILD_DIR"

echo "==> Done. $(lingtai-tui version 2>&1 || echo "$VERSION")"
echo "    To revert to Homebrew version later: brew reinstall lingtai-tui"
