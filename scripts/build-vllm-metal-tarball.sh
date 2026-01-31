#!/bin/bash
# Build script for vllm-metal macOS tarball distribution
# Creates a tarball containing Python site-packages for vllm-metal and dependencies
#
# Usage: ./scripts/build-vllm-metal-tarball.sh <VLLM_METAL_RELEASE> <TARBALL>
#   VLLM_METAL_RELEASE - vllm-metal release tag (required)
#   TARBALL - Output tarball path (required)
#
# Requirements:
#   - macOS with Apple Silicon (ARM64)
#   - Python 3.12+ installed (standard on macOS 14+, or via Homebrew)
#   - uv (will be installed if missing)

set -e

VLLM_METAL_RELEASE="${1:?Usage: $0 <VLLM_METAL_RELEASE> <TARBALL>}"
TARBALL_ARG="${2:?Usage: $0 <VLLM_METAL_RELEASE> <TARBALL>}"
WORK_DIR=$(mktemp -d)
VENV_DIR="$WORK_DIR/venv"

# Convert tarball path to absolute before we cd elsewhere
TARBALL="$(cd "$(dirname "$TARBALL_ARG")" && pwd)/$(basename "$TARBALL_ARG")"

VLLM_VERSION="0.13.0"
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

# Python 3.12 required (vllm-metal wheel is built for cp312)
if command -v python3.12 &> /dev/null; then
    PYTHON_BIN="python3.12"
elif command -v python3 &> /dev/null && [ "$(python3 --version 2>&1 | grep -oE '[0-9]+\.[0-9]+')" = "3.12" ]; then
    PYTHON_BIN="python3"
else
    echo "Error: Python 3.12 is required (the vllm-metal wheel is built for cp312)"
    echo "Install with: brew install python@3.12"
    exit 1
fi

PYTHON_VERSION=$($PYTHON_BIN --version 2>&1 | grep -oE '[0-9]+\.[0-9]+')
echo "Using Python $PYTHON_VERSION from: $(which $PYTHON_BIN)"

echo "Creating Python venv..."
uv venv "$VENV_DIR" --python "$PYTHON_BIN"

export VIRTUAL_ENV="$VENV_DIR"
export PATH="$VENV_DIR/bin:$PATH"

echo "Installing vLLM $VLLM_VERSION from source (CPU requirements)..."
cd "$WORK_DIR"
curl -fsSL -O "https://github.com/vllm-project/vllm/releases/download/v$VLLM_VERSION/vllm-$VLLM_VERSION.tar.gz"
tar xf "vllm-$VLLM_VERSION.tar.gz"
cd "vllm-$VLLM_VERSION"
uv pip install -r requirements/cpu.txt --index-strategy unsafe-best-match
uv pip install .
cd "$WORK_DIR"
rm -rf "vllm-$VLLM_VERSION" "vllm-$VLLM_VERSION.tar.gz"

echo "Installing vllm-metal from pre-built wheel..."
curl -fsSL -O "$VLLM_METAL_WHEEL_URL"
uv pip install vllm_metal-*.whl
rm -f vllm_metal-*.whl

echo "Packaging site-packages..."
SITE_PACKAGES_DIR="$VENV_DIR/lib/python$PYTHON_VERSION/site-packages"
if [ ! -d "$SITE_PACKAGES_DIR" ]; then
    echo "Error: site-packages directory not found at $SITE_PACKAGES_DIR"
    exit 1
fi

tar -czf "$TARBALL" -C "$SITE_PACKAGES_DIR" .

SIZE=$(du -h "$TARBALL" | cut -f1)
echo "Created: $TARBALL ($SIZE)"
echo ""
echo "To use this tarball:"
echo "  1. Upload to GitHub releases"
echo "  2. Model-runner will auto-download on first use"
echo "  3. Or manually extract for testing:"
echo "     python3.12 -m venv ~/.model-runner/vllm-metal"
echo "     tar -xzf $TARBALL -C ~/.model-runner/vllm-metal/lib/python$PYTHON_VERSION/site-packages/"
