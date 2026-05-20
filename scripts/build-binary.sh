#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
command -v go >/dev/null 2>&1 || { echo "go is required to build texgo" >&2; exit 1; }

OS_NAME="$(go env GOOS)"
ARCH_NAME="$(go env GOARCH)"
ASSET_OS_NAME="$OS_NAME"
[ "$ASSET_OS_NAME" = "darwin" ] && ASSET_OS_NAME="macos"
OUTPUT="$ROOT_DIR/dist/texgo-$ASSET_OS_NAME-$ARCH_NAME"

print_usage() {
    cat <<'EOF'
Usage: scripts/build-binary.sh [--output PATH]

Builds the Go texgo CLI binary.
EOF
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --output)
            [ "$#" -ge 2 ] || { echo "--output requires a value" >&2; exit 1; }
            OUTPUT="$2"
            shift 2
            ;;
        -h|--help)
            print_usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

mkdir -p "$(dirname "$OUTPUT")"
export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.gomodcache}"
go build -trimpath -ldflags="-s -w" -o "$OUTPUT" "$ROOT_DIR/cmd/texgo"
chmod +x "$OUTPUT"
echo "Built binary: $OUTPUT"
