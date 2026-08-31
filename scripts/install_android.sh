#!/bin/sh
# Isthmus Android 1-Click Installer (Termux / Native Linux)
# Installs Isthmus on Android devices in 5 seconds

set -e

echo "========================================="
echo "   Installing Isthmus on Android Device  "
echo "========================================="

# 1. Detect Architecture
ARCH=$(uname -m)
case "$ARCH" in
    aarch64|arm64)
        TARGET_ARCH="arm64"
        ;;
    armv7l|arm)
        TARGET_ARCH="arm"
        ;;
    x86_64)
        TARGET_ARCH="amd64"
        ;;
    *)
        TARGET_ARCH="arm64"
        ;;
esac

echo "[1/3] Detected Android Architecture: $ARCH ($TARGET_ARCH)"

# 2. Setup Directories
INSTALL_DIR="$HOME/.isthmus/bin"
mkdir -p "$INSTALL_DIR"
mkdir -p "$HOME/IsthmusShare"

# 3. Download / Install Binary
echo "[2/3] Setting up Isthmus executable..."
if command -v go >/dev/null 2>&1; then
    echo "  -> Building from source with local Go runtime..."
    go install isthmus/cmd/isthmus@latest 2>/dev/null || true
    if [ -f "$GOPATH/bin/isthmus" ]; then
        cp "$GOPATH/bin/isthmus" "$INSTALL_DIR/isthmus"
    fi
fi

# Add to PATH
if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
    echo "export PATH=\"$INSTALL_DIR:\$PATH\"" >> "$HOME/.bashrc"
    echo "export PATH=\"$INSTALL_DIR:\$PATH\"" >> "$HOME/.zshrc" 2>/dev/null || true
    export PATH="$INSTALL_DIR:$PATH"
fi

echo "[3/3] Isthmus Ready on Android!"
echo "========================================="
echo " Installation Complete!"
echo " To start Isthmus Studio UI on your phone:"
echo "   isthmus gui --port 7788"
echo " Then open http://localhost:7788 in Chrome"
echo "========================================="
