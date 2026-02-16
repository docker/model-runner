#!/bin/bash
# Build script for diffusers macOS/Linux tarball distribution
# Creates a self-contained tarball with a standalone Python 3.12 + diffusers packages.
# The result can be extracted anywhere and run without any system Python dependency.
#
# Usage: ./scripts/build-diffusers-tarball.sh <DIFFUSERS_RELEASE> <TARBALL>
#   DIFFUSERS_RELEASE - diffusers release tag (required)
#   TARBALL - Output tarball path (required)
#
# Requirements:
#   - uv (will be installed if missing)

set -e

DIFFUSERS_RELEASE="${1:?Usage: $0 <DIFFUSERS_RELEASE> <TARBALL>}"
TARBALL_ARG="${2:?Usage: $0 <DIFFUSERS_RELEASE> <TARBALL>}"
WORK_DIR=$(mktemp -d)

# Convert tarball path to absolute before we cd elsewhere
TARBALL="$(cd "$(dirname "$TARBALL_ARG")" && pwd)/$(basename "$TARBALL_ARG")"

# Directory containing this script (project root is one level up)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

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

echo "Installing diffusers and dependencies..."
uv pip install --python "$PYTHON_DIR/bin/python3" --system \
    diffusers \
    torch \
    torchvision \
    accelerate \
    transformers \
    safetensors \
    fastapi \
    uvicorn \
    pydantic

# Install the diffusers_server module from the project
echo "Installing diffusers_server module..."
SITE_PACKAGES="$PYTHON_DIR/lib/python3.12/site-packages"
cp -Rp "$PROJECT_ROOT/python/diffusers_server" "$SITE_PACKAGES/diffusers_server"

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

echo "Packaging standalone Python with diffusers..."
tar -czf "$TARBALL" -C "$PYTHON_DIR" .

SIZE=$(du -h "$TARBALL" | cut -f1)
echo "Created: $TARBALL ($SIZE)"
echo ""
echo "This tarball is fully self-contained (includes Python 3.12 + all packages)."
echo "To use: extract to a directory and run bin/python3 -m diffusers_server.server"
