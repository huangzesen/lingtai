#!/bin/sh
# Install lingtai — downloads the TUI binary and sets up the Python runtime.
# Usage: curl -fsSL https://raw.githubusercontent.com/huangzesen/lingtai/main/install.sh | sh

set -e

REPO="huangzesen/lingtai"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="lingtai-tui"
RUNTIME_DIR="$HOME/.lingtai-tui/runtime/venv"

# Detect platform
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Darwin)  PLATFORM="darwin" ;;
    Linux)   PLATFORM="linux" ;;
    *)       echo "Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
    x86_64|amd64)  ARCH_SUFFIX="x64" ;;
    arm64|aarch64) ARCH_SUFFIX="arm64" ;;
    *)             echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

ASSET="lingtai-${PLATFORM}-${ARCH_SUFFIX}"

# ── Step 1: Download TUI binary ──────────────────────────────────────────

TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed 's/.*: "//;s/".*//')
if [ -z "$TAG" ]; then
    echo "Failed to find latest release"
    exit 1
fi

URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

echo "Downloading ${ASSET} (${TAG})..."
curl -fsSL -o "/tmp/${ASSET}" "$URL"
chmod +x "/tmp/${ASSET}"

if [ -w "$INSTALL_DIR" ]; then
    mv "/tmp/${ASSET}" "${INSTALL_DIR}/${BINARY_NAME}"
else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "/tmp/${ASSET}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"

# ── Step 2: Set up Python runtime ────────────────────────────────────────

# Find Python 3.11+
find_python() {
    for cmd in python3 python; do
        if command -v "$cmd" >/dev/null 2>&1; then
            ver=$("$cmd" -c "import sys; print(sys.version_info >= (3, 11))" 2>/dev/null)
            if [ "$ver" = "True" ]; then
                echo "$cmd"
                return
            fi
        fi
    done
}

PYTHON=$(find_python)
if [ -z "$PYTHON" ]; then
    echo ""
    echo "Warning: Python 3.11+ not found. Install it from python.org."
    echo "The TUI will set up the Python environment on first run."
    echo ""
    echo "Run:  lingtai-tui"
    exit 0
fi

if [ -f "$RUNTIME_DIR/bin/python" ] || [ -f "$RUNTIME_DIR/Scripts/python.exe" ]; then
    echo "Python runtime already exists, skipping."
else
    echo "Creating Python environment..."
    mkdir -p "$(dirname "$RUNTIME_DIR")"

    # Prefer uv if available
    if command -v uv >/dev/null 2>&1; then
        uv venv "$RUNTIME_DIR"
        echo "Installing lingtai..."
        uv pip install lingtai -p "$RUNTIME_DIR"
    else
        "$PYTHON" -m venv "$RUNTIME_DIR"
        echo "Installing lingtai..."
        "$RUNTIME_DIR/bin/pip" install lingtai
    fi

    # Verify
    "$RUNTIME_DIR/bin/python" -c "import lingtai; print('lingtai', lingtai.__version__)" || {
        echo "Warning: lingtai installed but import failed."
    }
fi

echo ""
echo "Ready. Run:  lingtai-tui"
