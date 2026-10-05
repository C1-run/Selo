#!/bin/bash
# Selo installer — downloads the latest binary from GitHub releases.
# Usage: curl -fsSL https://raw.githubusercontent.com/desmondkam/openselo/main/install.sh | bash
set -euo pipefail

REPO="desmondkam/openselo"
BINARY="selo"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS and architecture
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Linux*)   OS="linux" ;;
    Darwin*)  OS="darwin" ;;
    *)        echo "Error: unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
    x86_64|amd64)   ARCH="amd64" ;;
    arm64|aarch64)   ARCH="arm64" ;;
    *)               echo "Error: unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest release version from GitHub API
echo "Fetching latest release..."
VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

if [ -z "$VERSION" ]; then
    echo "Error: could not determine latest version"
    exit 1
fi

echo "Latest version: $VERSION"

# Download binary
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/${BINARY}-${OS}-${ARCH}"
TEMP_FILE=$(mktemp)

echo "Downloading $DOWNLOAD_URL..."
curl -fsSL -o "$TEMP_FILE" "$DOWNLOAD_URL"

# Install
chmod +x "$TEMP_FILE"
sudo mv "$TEMP_FILE" "$INSTALL_DIR/$BINARY"

echo ""
echo "Selo $VERSION installed to $INSTALL_DIR/$BINARY"
echo "Run 'selo --help' to get started."
