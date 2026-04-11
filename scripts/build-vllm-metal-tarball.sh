#!/bin/bash
# Build script for vllm-metal macOS tarball distribution
# Creates a self-contained tarball with a standalone Python 3.12 + vllm-metal packages.
# The result can be extracted anywhere and run without any system Python dependency.
#
# Usage: ./scripts/build-vllm-metal-tarball.sh <VLLM_METAL_RELEASE> <TARBALL>
#   VLLM_METAL_RELEASE - vllm-metal release tag (required)
#   TARBALL - Output tarball path (required)
#
# Requirements:
#   - macOS with Apple Silicon (ARM64)
#   - uv (will be installed if missing)

set -e

VLLM_METAL_RELEASE="${1:?Usage: $0 <VLLM_METAL_RELEASE> <TARBALL>}"
TARBALL_ARG="${2:?Usage: $0 <VLLM_METAL_RELEASE> <TARBALL>}"
WORK_DIR=$(mktemp -d)

# Convert tarball path to absolute before we cd elsewhere
TARBALL="$(cd "$(dirname "$TARBALL_ARG")" && pwd)/$(basename "$TARBALL_ARG")"

VLLM_VERSION=$(grep '^VLLM_UPSTREAM_VERSION=' "$(cd "$(dirname "$0")/.." && pwd)/.versions" | cut -d= -f2 | sed 's/[[:space:]]*#.*//;s/[[:space:]]*$//')

# Extract wheel version from release tag (e.g., v0.1.0-20260126-121650 -> 0.1.0)
VLLM_METAL_WHEEL_VERSION=$(echo "$VLLM_METAL_RELEASE" | sed 's/^v//' | cut -d'-' -f1)
VLLM_METAL_WHEEL_URL="https://github.com/vllm-project/vllm-metal/releases/download/${VLLM_METAL_RELEASE}/vllm_metal-${VLLM_METAL_WHEEL_VERSION}-cp312-cp312-macosx_11_0_arm64.whl"

cleanup() {
    rm -rf "$WORK_DIR"
}
trap cleanup EXIT

if ! command -v uv &> /dev/null; then
    echo "Installing uv..."
    curl -LsSf https://astral.sh/uv/install.sh | sh
    export PATH="$HOME/.local/bin:$PATH"
fi

# Install standalone Python 3.12 via uv (from python-build-standalone, relocatable)
echo "Installing standalone Python 3.12 via uv..."
uv python install 3.12

PYTHON_BIN=$(uv python find 3.12)
PYTHON_PREFIX=$(cd "$(dirname "$PYTHON_BIN")/.." && pwd)
echo "Using standalone Python from: $PYTHON_PREFIX"

# Copy the standalone Python to our work area
PYTHON_DIR="$WORK_DIR/python"
cp -Rp "$PYTHON_PREFIX" "$PYTHON_DIR"

# Remove the externally-managed marker so we can install packages into it
rm -f "$PYTHON_DIR/lib/python3.12/EXTERNALLY-MANAGED"

echo "Installing vLLM $VLLM_VERSION from source (CPU requirements)..."
cd "$WORK_DIR"
curl -fsSL -O "https://github.com/vllm-project/vllm/releases/download/v$VLLM_VERSION/vllm-$VLLM_VERSION.tar.gz"
tar xf "vllm-$VLLM_VERSION.tar.gz"
cd "vllm-$VLLM_VERSION"
uv pip install --python "$PYTHON_DIR/bin/python3" --system -r requirements/cpu.txt --index-strategy unsafe-best-match
# TODO: remove -Wno-parentheses once vllm-project/vllm#38801 is in a release and VLLM_VERSION is bumped past it.
# Apple Clang 21 (Xcode 26+) promotes -Wparentheses to an error for chained comparisons like `0 < M <= 8` in
# vllm's CPU attention headers. Clang 17 (Xcode 16.x, used in CI) only warns.
CXXFLAGS="-Wno-parentheses" uv pip install --python "$PYTHON_DIR/bin/python3" --system .
cd "$WORK_DIR"
rm -rf "vllm-$VLLM_VERSION" "vllm-$VLLM_VERSION.tar.gz"

echo "Installing vllm-metal from pre-built wheel..."
curl -fsSL -O "$VLLM_METAL_WHEEL_URL"
uv pip install --python "$PYTHON_DIR/bin/python3" --system vllm_metal-*.whl
rm -f vllm_metal-*.whl

# Strip files not needed at runtime to reduce tarball size
echo "Stripping unnecessary files..."
rm -rf "$PYTHON_DIR/include"
rm -rf "$PYTHON_DIR/share"
PYLIB="$PYTHON_DIR/lib/python3.12"
rm -rf "$PYLIB/test" "$PYLIB/tests"
rm -rf "$PYLIB/idlelib" "$PYLIB/idle_test"
rm -rf "$PYLIB/tkinter" "$PYLIB/turtledemo"
rm -rf "$PYLIB/ensurepip"
# Remove Tcl/Tk native libraries (we don't need tkinter at runtime)
rm -f "$PYTHON_DIR"/lib/libtcl*.dylib "$PYTHON_DIR"/lib/libtk*.dylib
rm -rf "$PYTHON_DIR"/lib/tcl* "$PYTHON_DIR"/lib/tk*
# Remove dev tools not needed at runtime
rm -f "$PYTHON_DIR"/bin/*-config "$PYTHON_DIR"/bin/idle*
find "$PYTHON_DIR" -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true

echo "Packaging standalone Python with vllm-metal..."
tar -czf "$TARBALL" -C "$PYTHON_DIR" .

SIZE=$(du -h "$TARBALL" | cut -f1)
echo "Created: $TARBALL ($SIZE)"
echo ""
echo "This tarball is fully self-contained (includes Python 3.12 + all packages)."
echo "To use: extract to a directory and run bin/python3 -m vllm.entrypoints.openai.api_server --model <path>"
