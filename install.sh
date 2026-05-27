#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
PREFIX="/usr/local"
SOURCE_BINARY=""
BUILD_FROM_SOURCE=0
INSTALL_DEPS=1
ASSUME_YES=0
REPO="${TEXGO_REPO:-Yaho7/texgo}"
VERSION="${TEXGO_VERSION:-latest}"
DOWNLOAD_TMP_DIR=""
STYLE_BOLD=""
STYLE_GREEN=""
STYLE_DIM=""
STYLE_RESET=""

print_usage() {
    cat <<'EOF'
Usage: ./install.sh [--prefix DIR] [--version VERSION] [--repo OWNER/REPO] [--from PATH] [--build-from-source] [--yes] [--no-deps]

Options:
  --prefix DIR          Install under DIR/bin. Default: /usr/local
  --version VERSION     GitHub release version to install. Default: latest
  --repo OWNER/REPO     GitHub repository to download from. Default: Yaho7/texgo
  --from PATH           Install an existing texgo binary.
  --build-from-source   Build from the current source checkout instead of downloading a release binary.
  --yes                 Do not prompt before installing missing dependencies.
  --no-deps             Skip dependency checks and automatic dependency installation.

Examples:
  curl -fsSL https://raw.githubusercontent.com/Yaho7/texgo/main/install.sh | bash -s -- --yes
  ./install.sh --from dist/texgo-macos-arm64 --prefix "$HOME/.local"
  ./install.sh --build-from-source --prefix "$HOME/.local"
EOF
}

cleanup() {
    [ -z "$DOWNLOAD_TMP_DIR" ] || rm -rf "$DOWNLOAD_TMP_DIR"
}
trap cleanup EXIT

contains_word() {
    local needle="$1"
    shift
    local item
    for item in "$@"; do
        [ "$item" = "$needle" ] && return 0
    done
    return 1
}

command_exists() {
    local command_name="$1"
    if [ -n "${TEXGO_TEST_MISSING_COMMANDS:-}" ]; then
        case " $TEXGO_TEST_MISSING_COMMANDS " in
            *" $command_name "*) return 1 ;;
        esac
    fi
    command -v "$command_name" >/dev/null 2>&1
}

detect_package_manager() {
    if [ -n "${TEXGO_PACKAGE_MANAGER:-}" ]; then
        printf '%s\n' "$TEXGO_PACKAGE_MANAGER"
        return
    fi

    if command_exists brew; then
        printf 'brew\n'
    elif command_exists apt-get; then
        printf 'apt-get\n'
    elif command_exists apt; then
        printf 'apt\n'
    elif command_exists dnf; then
        printf 'dnf\n'
    elif command_exists yum; then
        printf 'yum\n'
    elif command_exists pacman; then
        printf 'pacman\n'
    else
        printf 'unknown\n'
    fi
}

sudo_prefix() {
    if [ "$(id -u)" -eq 0 ]; then
        printf ''
    elif command_exists sudo; then
        printf 'sudo '
    else
        printf ''
    fi
}

package_for_dependency() {
    local manager="$1"
    local dep="$2"

    case "$manager:$dep" in
        brew:gm) printf 'graphicsmagick' ;;
        brew:latexmk) printf 'mactex-no-gui' ;;
        brew:go) printf 'go' ;;
        apt-get:gm|apt:gm) printf 'graphicsmagick' ;;
        apt-get:latexmk|apt:latexmk) printf 'latexmk' ;;
        apt-get:go|apt:go) printf 'golang-go' ;;
        dnf:gm|yum:gm) printf 'GraphicsMagick' ;;
        dnf:latexmk|yum:latexmk) printf 'latexmk' ;;
        dnf:go|yum:go) printf 'golang' ;;
        pacman:gm) printf 'graphicsmagick' ;;
        pacman:latexmk) printf 'texlive-binextra' ;;
        pacman:go) printf 'go' ;;
        *) return 1 ;;
    esac
}

run_install_command() {
    if [ -n "${TEXGO_INSTALL_LOG:-}" ]; then
        printf '%s\n' "$*" >> "$TEXGO_INSTALL_LOG"
        return 0
    fi
    "$@"
}

setup_terminal_styles() {
    [ -t 1 ] || return 0
    [ -z "${NO_COLOR:-}" ] || return 0

    STYLE_BOLD="$(printf '\033[1m')"
    STYLE_GREEN="$(printf '\033[32m')"
    STYLE_DIM="$(printf '\033[2m')"
    STYLE_RESET="$(printf '\033[0m')"
}

print_success_summary() {
    local dependency_status="$1"
    local source_label="$2"
    local install_path="$PREFIX/bin/texgo"

    setup_terminal_styles

    printf '\n'
    printf '%s%s%s\n' "$STYLE_GREEN$STYLE_BOLD" "texgo installation complete" "$STYLE_RESET"
    printf '%s\n' "--------------------------------"
    printf '  %-12s : %s\n' "Install path" "$install_path"
    printf '  %-12s : %s\n' "Source" "$source_label"
    printf '  %-12s : %s\n' "Dependencies" "$dependency_status"
    printf '  %-12s : %s\n' "Next step" "texgo --help"
    printf '%s\n' "--------------------------------"

    case ":$PATH:" in
        *":$PREFIX/bin:"*) ;;
        *)
            printf '%s\n' "${STYLE_DIM}Tip: add $PREFIX/bin to PATH if texgo is not found in a new shell.${STYLE_RESET}"
            ;;
    esac
}

confirm_dependency_install() {
    [ "$ASSUME_YES" -eq 1 ] && return 0
    [ -t 0 ] || {
        echo "Cannot prompt to install missing dependencies in non-interactive mode. Re-run with --yes or --no-deps." >&2
        return 1
    }

    local answer
    printf 'Install missing dependencies now? [Y/n]: '
    IFS= read -r answer || answer=""
    case "$answer" in
        ""|y|Y|yes|YES) return 0 ;;
        *) return 1 ;;
    esac
}

install_missing_dependencies() {
    local manager="$1"
    shift
    local missing=("$@")
    local packages=()
    local dep package

    [ "${#missing[@]}" -eq 0 ] && return 0

    echo "Missing dependencies: ${missing[*]}"

    if [ "$manager" = "unknown" ]; then
        echo "No supported package manager found. Install these manually: ${missing[*]}" >&2
        return 1
    fi

    for dep in "${missing[@]}"; do
        package="$(package_for_dependency "$manager" "$dep")" || {
            echo "No package mapping for dependency '$dep' with package manager '$manager'." >&2
            return 1
        }
        if [ "${#packages[@]}" -eq 0 ] || ! contains_word "$package" "${packages[@]}"; then
            packages+=("$package")
        fi
    done

    echo "Package manager: $manager"
    echo "Packages to install: ${packages[*]}"

    confirm_dependency_install || {
        echo "Dependency installation cancelled." >&2
        return 1
    }

    local sudo_cmd
    sudo_cmd="$(sudo_prefix)"

    case "$manager" in
        brew)
            run_install_command brew install "${packages[@]}"
            ;;
        apt-get)
            if [ -n "$sudo_cmd" ]; then
                run_install_command sudo apt-get update
                run_install_command sudo apt-get install -y "${packages[@]}"
            else
                run_install_command apt-get update
                run_install_command apt-get install -y "${packages[@]}"
            fi
            ;;
        apt)
            if [ -n "$sudo_cmd" ]; then
                run_install_command sudo apt update
                run_install_command sudo apt install -y "${packages[@]}"
            else
                run_install_command apt update
                run_install_command apt install -y "${packages[@]}"
            fi
            ;;
        dnf|yum)
            if [ -n "$sudo_cmd" ]; then
                run_install_command sudo "$manager" install -y "${packages[@]}"
            else
                run_install_command "$manager" install -y "${packages[@]}"
            fi
            ;;
        pacman)
            if [ -n "$sudo_cmd" ]; then
                run_install_command sudo pacman -Sy --needed --noconfirm "${packages[@]}"
            else
                run_install_command pacman -Sy --needed --noconfirm "${packages[@]}"
            fi
            ;;
    esac
}

ensure_dependencies() {
    [ "$INSTALL_DEPS" -eq 1 ] || {
        echo "Skipping dependency checks because --no-deps was provided."
        return 0
    }

    local required=(gm latexmk)
    if [ "$BUILD_FROM_SOURCE" -eq 1 ]; then
        required+=(go)
    fi

    local missing=()
    local dep
    for dep in "${required[@]}"; do
        if ! command_exists "$dep"; then
            missing+=("$dep")
        fi
    done

    [ "${#missing[@]}" -eq 0 ] && {
        echo "All required dependencies are available."
        return 0
    }

    local manager
    manager="$(detect_package_manager)"
    install_missing_dependencies "$manager" "${missing[@]}"

    if [ -z "${TEXGO_INSTALL_LOG:-}" ]; then
        local still_missing=()
        for dep in "${required[@]}"; do
            if ! command_exists "$dep"; then
                still_missing+=("$dep")
            fi
        done
        [ "${#still_missing[@]}" -eq 0 ] || {
            echo "Dependencies are still missing after installation: ${still_missing[*]}" >&2
            return 1
        }
    fi
}

detect_os() {
    if [ -n "${TEXGO_OS:-}" ]; then
        printf '%s\n' "$TEXGO_OS"
        return
    fi

    case "$(uname -s)" in
        Darwin) printf 'macos\n' ;;
        Linux) printf 'linux\n' ;;
        MINGW*|MSYS*|CYGWIN*) printf 'windows\n' ;;
        *)
            echo "Unsupported operating system: $(uname -s)" >&2
            return 1
            ;;
    esac
}

detect_arch() {
    if [ -n "${TEXGO_ARCH:-}" ]; then
        printf '%s\n' "$TEXGO_ARCH"
        return
    fi

    case "$(uname -m)" in
        x86_64|amd64) printf 'amd64\n' ;;
        arm64|aarch64) printf 'arm64\n' ;;
        *)
            echo "Unsupported CPU architecture: $(uname -m)" >&2
            return 1
            ;;
    esac
}

release_base_url() {
    if [ -n "${TEXGO_RELEASE_BASE_URL:-}" ]; then
        printf '%s\n' "$TEXGO_RELEASE_BASE_URL"
    elif [ "$VERSION" = "latest" ]; then
        printf 'https://github.com/%s/releases/latest/download\n' "$REPO"
    else
        printf 'https://github.com/%s/releases/download/%s\n' "$REPO" "$VERSION"
    fi
}

download_file() {
    local url="$1"
    local output="$2"

    case "$url" in
        file://*)
            cp "${url#file://}" "$output"
            return
            ;;
    esac

    if command_exists curl; then
        curl -fL "$url" -o "$output"
    elif command_exists wget; then
        wget -O "$output" "$url"
    else
        echo "curl or wget is required to download texgo." >&2
        return 1
    fi
}

download_release_binary() {
    local os arch ext asset base_url url
    os="$(detect_os)"
    arch="$(detect_arch)"
    ext=""
    [ "$os" = "windows" ] && ext=".exe"

    asset="texgo-$os-$arch$ext"
    base_url="$(release_base_url)"
    url="${base_url%/}/$asset"

    DOWNLOAD_TMP_DIR="$(mktemp -d)"
    local output="$DOWNLOAD_TMP_DIR/$asset"

    echo "Downloading texgo from $url" >&2
    download_file "$url" "$output"
    chmod +x "$output"
    printf '%s\n' "$output"
}

install_binary() {
    local source="$1"
    local install_dir="$PREFIX/bin"
    local sudo_cmd
    sudo_cmd="$(sudo_prefix)"

    if ! mkdir -p "$install_dir" 2>/dev/null; then
        [ -n "$sudo_cmd" ] || {
            echo "Cannot create $install_dir. Re-run with --prefix DIR or install sudo." >&2
            return 1
        }
        run_install_command sudo mkdir -p "$install_dir"
    fi

    if [ -w "$install_dir" ]; then
        install -m 0755 "$source" "$install_dir/texgo"
    else
        [ -n "$sudo_cmd" ] || {
            echo "Cannot write to $install_dir. Re-run with --prefix DIR or install sudo." >&2
            return 1
        }
        run_install_command sudo install -m 0755 "$source" "$install_dir/texgo"
    fi
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --prefix)
            [ "$#" -ge 2 ] || { echo "--prefix requires a value" >&2; exit 1; }
            PREFIX="$2"
            shift 2
            ;;
        --from)
            [ "$#" -ge 2 ] || { echo "--from requires a value" >&2; exit 1; }
            SOURCE_BINARY="$2"
            shift 2
            ;;
        --build-from-source)
            BUILD_FROM_SOURCE=1
            shift
            ;;
        --version)
            [ "$#" -ge 2 ] || { echo "--version requires a value" >&2; exit 1; }
            VERSION="$2"
            shift 2
            ;;
        --repo)
            [ "$#" -ge 2 ] || { echo "--repo requires a value" >&2; exit 1; }
            REPO="$2"
            shift 2
            ;;
        --yes)
            ASSUME_YES=1
            shift
            ;;
        --no-deps)
            INSTALL_DEPS=0
            shift
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

[ -z "$SOURCE_BINARY" ] || [ "$BUILD_FROM_SOURCE" -eq 0 ] || {
    echo "--from and --build-from-source cannot be used together." >&2
    exit 1
}

ensure_dependencies

DEPENDENCY_STATUS="skipped"
[ "$INSTALL_DEPS" -eq 1 ] && DEPENDENCY_STATUS="verified"

if [ -n "$SOURCE_BINARY" ]; then
    SOURCE="$SOURCE_BINARY"
    SOURCE_LABEL="local binary"
elif [ "$BUILD_FROM_SOURCE" -eq 1 ]; then
    [ -x "$ROOT_DIR/scripts/build-binary.sh" ] || {
        echo "--build-from-source must be run from a texgo source checkout." >&2
        exit 1
    }
    SOURCE="$ROOT_DIR/dist/texgo-install"
    "$ROOT_DIR/scripts/build-binary.sh" --output "$SOURCE"
    SOURCE_LABEL="source build"
else
    SOURCE="$(download_release_binary)"
    SOURCE_LABEL="release $VERSION from $REPO"
fi

[ -f "$SOURCE" ] || { echo "Install source not found: $SOURCE" >&2; exit 1; }
install_binary "$SOURCE"

print_success_summary "$DEPENDENCY_STATUS" "$SOURCE_LABEL"
