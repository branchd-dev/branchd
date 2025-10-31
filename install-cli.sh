#!/bin/bash
set -e

# Branchd CLI Installation Script
# Usage: curl -fsSL https://raw.githubusercontent.com/branchd-dev/branchd/main/install-cli.sh | bash

REPO="branchd-dev/branchd"
BINARY_NAME="branchd"
INSTALL_DIR="$HOME/.local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Normalize architecture names
case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo "Error: Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# Set binary name based on OS
if [ "$OS" = "windows" ] || [ "$OS" = "mingw"* ] || [ "$OS" = "msys"* ]; then
    OS="windows"
    BINARY_FILE="${BINARY_NAME}-${OS}-${ARCH}.exe"
    INSTALL_NAME="${BINARY_NAME}.exe"
elif [ "$OS" = "darwin" ]; then
    BINARY_FILE="${BINARY_NAME}-darwin-${ARCH}"
    INSTALL_NAME="${BINARY_NAME}"
elif [ "$OS" = "linux" ]; then
    BINARY_FILE="${BINARY_NAME}-linux-${ARCH}"
    INSTALL_NAME="${BINARY_NAME}"
else
    echo "Error: Unsupported operating system: $OS"
    exit 1
fi

echo "Installing Branchd CLI for ${OS}/${ARCH}..."

# Get latest release version
echo "Fetching latest release..."
LATEST_RELEASE=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')

if [ -z "$LATEST_RELEASE" ]; then
    echo "Error: Failed to fetch latest release"
    exit 1
fi

echo "Latest version: $LATEST_RELEASE"

# Download binary
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_RELEASE}/${BINARY_FILE}"
TMP_FILE="/tmp/${BINARY_FILE}"

echo "Downloading from: $DOWNLOAD_URL"
curl -fsSL -o "$TMP_FILE" "$DOWNLOAD_URL"

if [ ! -f "$TMP_FILE" ]; then
    echo "Error: Failed to download binary"
    exit 1
fi

# Download checksum
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${LATEST_RELEASE}/${BINARY_FILE}.sha256"
TMP_CHECKSUM="/tmp/${BINARY_FILE}.sha256"

echo "Downloading checksum..."
curl -fsSL -o "$TMP_CHECKSUM" "$CHECKSUM_URL"

# Verify checksum
echo "Verifying checksum..."
cd /tmp
if ! sha256sum -c "$TMP_CHECKSUM" 2>/dev/null; then
    echo "Error: Checksum verification failed"
    rm -f "$TMP_FILE" "$TMP_CHECKSUM"
    exit 1
fi
echo "Checksum verified!"

# Make binary executable
chmod +x "$TMP_FILE"

# Create install directory if it doesn't exist
mkdir -p "$INSTALL_DIR"

# Install binary
echo "Installing to ${INSTALL_DIR}/${INSTALL_NAME}..."
mv "$TMP_FILE" "${INSTALL_DIR}/${INSTALL_NAME}"

# Cleanup
rm -f "$TMP_CHECKSUM"

# Verify installation
if command -v "$BINARY_NAME" >/dev/null 2>&1; then
    echo ""
    echo "✓ Branchd CLI installed successfully!"
    echo ""
    echo "Version: $($BINARY_NAME version)"
    echo ""
    echo "Get started:"
    echo "  1. Run 'branchd init' to create a configuration file"
    echo "  2. Run 'branchd --help' to see available commands"
else
    echo ""
    echo "✓ Installation complete!"
    echo ""
    echo "To use branchd, add ~/.local/bin to your PATH:"
    echo ""
    if [ -n "$BASH_VERSION" ]; then
        echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc"
        echo "  source ~/.bashrc"
    elif [ -n "$ZSH_VERSION" ]; then
        echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc"
        echo "  source ~/.zshrc"
    else
        echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
        echo ""
        echo "  Add the above line to your shell's RC file (~/.bashrc, ~/.zshrc, etc.)"
    fi
    echo ""
    echo "Or run directly: ~/.local/bin/branchd"
fi
