#!/usr/bin/env bash
# Build and bundle static libraries for all supported platforms.
# Requires: cargo (for Apple targets), docker (for Linux targets via rust:latest image)
# Usage: ./scripts/build-libs.sh [--nono-src /path/to/nono]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CLIB_DIR="$REPO_ROOT/internal/clib"

NONO_SRC=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --nono-src) NONO_SRC="$2"; shift 2 ;;
        *) echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$NONO_SRC" ]]; then
    NONO_SRC="$(mktemp -d)"
    trap 'rm -rf "$NONO_SRC"' EXIT
    echo "Cloning nono repository..."
    git clone --depth=1 https://github.com/always-further/nono.git "$NONO_SRC"
fi

is_linux_target() {
    [[ "$1" == *-linux-* ]]
}

linux_docker_platform() {
    case "$1" in
        x86_64-*) echo "linux/amd64" ;;
        aarch64-*) echo "linux/arm64" ;;
        *) echo "Unknown linux architecture for triple: $1" >&2; exit 1 ;;
    esac
}

build_target() {
    local triple="$1"
    local dest_dir="$2"

    echo "Building for $triple..."
    if is_linux_target "$triple"; then
        # Build natively inside a Rust Docker image (run under emulation on macOS).
        # rust:latest is Debian Bookworm with GCC 12, avoiding the GCC 9 memcmp bug.
        # Clear the native target/release dir between Linux builds since both platforms
        # write to the same path (no target triple in the path for native builds).
        rm -rf "$NONO_SRC/target/release"
        local docker_platform
        docker_platform="$(linux_docker_platform "$triple")"
        docker run --rm \
            --platform "$docker_platform" \
            -v "$NONO_SRC:/src" \
            rust:latest \
            sh -c "apt-get update -qq && apt-get install -y -qq libdbus-1-dev pkg-config && cargo build --release --manifest-path /src/Cargo.toml -p nono-ffi"
        local lib_src="$NONO_SRC/target/release/libnono_ffi.a"
    else
        cargo build --release --manifest-path "$NONO_SRC/Cargo.toml" -p nono-ffi --target "$triple"
        local lib_src="$NONO_SRC/target/$triple/release/libnono_ffi.a"
    fi

    mkdir -p "$dest_dir"
    cp "$lib_src" "$dest_dir/libnono_ffi.a"
    echo "  -> $dest_dir/libnono_ffi.a"
    # Record which upstream commit was used, matching the format written by CI.
    local sha
    sha="$(git -C "$NONO_SRC" rev-parse HEAD)"
    {
        echo "# nono upstream commit used to build libnono_ffi.a for $triple"
        echo "# Update this file whenever the bundled library is rebuilt."
        echo "$sha"
    } > "$dest_dir/VERSION"
}

# Copy header (located at bindings/c/include/nono.h in the upstream repo)
cp "$NONO_SRC/bindings/c/include/nono.h" "$CLIB_DIR/nono.h"
echo "Copied nono.h"

# Build each target
build_target "aarch64-apple-darwin"  "$CLIB_DIR/darwin_arm64"
build_target "x86_64-apple-darwin"   "$CLIB_DIR/darwin_amd64"
build_target "x86_64-unknown-linux-gnu"  "$CLIB_DIR/linux_amd64"
build_target "aarch64-unknown-linux-gnu" "$CLIB_DIR/linux_arm64"

echo "Done. Libraries updated in $CLIB_DIR/"
