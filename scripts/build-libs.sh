#!/usr/bin/env bash
# Build and bundle static libraries for all supported platforms.
# Requires: cargo, cargo cross (for cross-compilation)
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

build_target() {
    local triple="$1"
    local dest_dir="$2"

    echo "Building for $triple..."
    # Use workspace root + -p nono-ffi, consistent with the CI workflow.
    cargo build --release --manifest-path "$NONO_SRC/Cargo.toml" -p nono-ffi --target "$triple"
    mkdir -p "$dest_dir"
    cp "$NONO_SRC/target/$triple/release/libnono_ffi.a" "$dest_dir/libnono_ffi.a"
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
