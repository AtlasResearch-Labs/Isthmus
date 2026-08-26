#!/usr/bin/env bash
set -e

echo "=================================================="
echo "Starting Isthmus Multi-Platform Compilation Pipeline"
echo "=================================================="

TARGETS=(
    "windows/amd64/.exe"
    "windows/arm64/.exe"
    "linux/amd64/"
    "linux/arm64/"
    "darwin/amd64/"
    "darwin/arm64/"
)

for target in "${TARGETS[@]}"; do
    IFS="/" read -r OS ARCH EXT <<< "$target"
    OUT_DIR="bin/${OS}-${ARCH}"
    mkdir -p "$OUT_DIR"

    BIN_ISTHMUS="${OUT_DIR}/isthmus${EXT}"
    BIN_COORD="${OUT_DIR}/isthmus-coord${EXT}"

    echo "[BUILD] Target: ${OS} / ${ARCH} -> ${BIN_ISTHMUS}"
    GOOS="$OS" GOARCH="$ARCH" CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN_ISTHMUS" ./cmd/isthmus

    if [ "$OS" = "linux" ] || ([ "$OS" = "windows" ] && [ "$ARCH" = "amd64" ]); then
        echo "[BUILD] Target: ${OS} / ${ARCH} -> ${BIN_COORD}"
        GOOS="$OS" GOARCH="$ARCH" CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN_COORD" ./cmd/isthmus-coord
    fi
done

echo "=================================================="
echo "Multi-Platform Compilation Complete."
echo "=================================================="
