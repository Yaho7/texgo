#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

release_dir="$TMP_DIR/releases"
prefix_dir="$TMP_DIR/prefix"
log_file="$TMP_DIR/install.log"
install_output="$TMP_DIR/install.out"
stdin_output="$TMP_DIR/stdin-install.out"
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
    "$ROOT_DIR/install.sh" --prefix "$prefix_dir" --yes > "$install_output"

"$prefix_dir/bin/texgo" | grep -q "texgo test binary"
grep -q "brew install graphicsmagick mactex-no-gui" "$log_file"
grep -q "texgo installation complete" "$install_output"
grep -q "Install path : $prefix_dir/bin/texgo" "$install_output"
grep -q "Dependencies : verified" "$install_output"
grep -q "Next step    : texgo --help" "$install_output"

stdin_prefix_dir="$TMP_DIR/stdin-prefix"
mkdir -p "$stdin_prefix_dir"
TEXGO_OS=macos \
TEXGO_ARCH=arm64 \
TEXGO_RELEASE_BASE_URL="file://$release_dir" \
TEXGO_TEST_MISSING_COMMANDS="" \
    bash -s -- --prefix "$stdin_prefix_dir" --yes < "$ROOT_DIR/install.sh" > "$stdin_output" 2>&1
"$stdin_prefix_dir/bin/texgo" | grep -q "texgo test binary"
grep -q "texgo installation complete" "$stdin_output"
if grep -q "unbound variable" "$stdin_output"; then
    echo "stdin installation should not print shell variable errors" >&2
    exit 1
fi

if grep -q " go" "$log_file"; then
    echo "release installation should not install Go" >&2
    exit 1
fi

echo "install release test passed"
