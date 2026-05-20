#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

release_dir="$TMP_DIR/releases"
prefix_dir="$TMP_DIR/prefix"
log_file="$TMP_DIR/install.log"
mkdir -p "$release_dir" "$prefix_dir"

cat > "$release_dir/texgo-macos-arm64" <<'BIN'
#!/usr/bin/env bash
echo "texgo test binary"
BIN
chmod +x "$release_dir/texgo-macos-arm64"

TEXGO_OS=macos \
TEXGO_ARCH=arm64 \
TEXGO_RELEASE_BASE_URL="file://$release_dir" \
TEXGO_TEST_MISSING_COMMANDS="gm latexmk" \
TEXGO_PACKAGE_MANAGER=brew \
TEXGO_INSTALL_LOG="$log_file" \
    "$ROOT_DIR/install.sh" --prefix "$prefix_dir" --yes

"$prefix_dir/bin/texgo" | grep -q "texgo test binary"
grep -q "brew install graphicsmagick mactex-no-gui" "$log_file"

if grep -q " go" "$log_file"; then
    echo "release installation should not install Go" >&2
    exit 1
fi

echo "install release test passed"
