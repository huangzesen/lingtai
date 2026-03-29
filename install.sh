#!/bin/sh
# Install lingtai-tui — downloads the latest release binary for your platform.
# Usage: curl -fsSL https://raw.githubusercontent.com/huangzesen/lingtai/main/install.sh | sh

set -e

REPO="huangzesen/lingtai"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="lingtai-tui"

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

# Get latest release tag
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed 's/.*: "//;s/".*//')
if [ -z "$TAG" ]; then
    echo "Failed to find latest release"
    exit 1
fi

URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

echo "Downloading ${ASSET} (${TAG})..."
curl -fsSL -o "/tmp/${ASSET}" "$URL"
chmod +x "/tmp/${ASSET}"

# Install
if [ -w "$INSTALL_DIR" ]; then
    mv "/tmp/${ASSET}" "${INSTALL_DIR}/${BINARY_NAME}"
else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "/tmp/${ASSET}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
echo ""
echo "Run:  lingtai-tui"
