#!/usr/bin/env bash
set -e

echo "========================================="
echo "    Installing Isthmus Mesh Engine       "
echo "========================================="

# Detect OS and Arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l) ARCH="armv7" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

BINARY="isthmus-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
  BINARY="isthmus-windows-${ARCH}.exe"
fi

echo "[1/3] Detected platform: ${OS}-${ARCH}"
echo "[2/3] Downloading ${BINARY}..."

URL="https://github.com/AtlasResearch-Labs/Isthmus/releases/latest/download/${BINARY}"
TMP_FILE="/tmp/isthmus"

if curl -fsSL "$URL" -o "$TMP_FILE" 2>/dev/null; then
  echo "  -> Downloaded from GitHub Releases."
else
  FALLBACK_URL="https://raw.githubusercontent.com/AtlasResearch-Labs/Isthmus/main/dist/binaries/${BINARY}"
  echo "  -> Fetching from raw repository..."
  curl -fsSL "$FALLBACK_URL" -o "$TMP_FILE"
fi

chmod +x "$TMP_FILE"

echo "[3/3] Installing to /usr/local/bin/isthmus..."
if [ -w "/usr/local/bin" ]; then
  mv "$TMP_FILE" /usr/local/bin/isthmus
else
  sudo mv "$TMP_FILE" /usr/local/bin/isthmus
fi

echo "========================================="
echo " SUCCESS: Isthmus installed successfully!"
echo " Run 'isthmus' or 'isthmus daemon' to start."
echo "========================================="
