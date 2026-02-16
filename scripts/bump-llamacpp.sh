#!/usr/bin/env bash

set -euo pipefail

# Bump llama.cpp submodule to the latest (or specified) tagged release.
# Usage: ./scripts/bump-llamacpp.sh [<tag>]
#   <tag>  - a specific llama.cpp tag (e.g. b8068). Defaults to the latest b* tag.

REPO_ROOT="$(git rev-parse --show-toplevel)"
SUBMODULE_PATH="llamacpp/native/vendor/llama.cpp"
SUBMODULE_DIR="$REPO_ROOT/$SUBMODULE_PATH"

if git -C "$REPO_ROOT" submodule status -- "$SUBMODULE_PATH" | grep -q '^-'; then
    echo "Submodule not initialized. Initializing..."
    git -C "$REPO_ROOT" submodule update --init --recursive -- "$SUBMODULE_PATH"
fi

echo "Fetching latest tags from llama.cpp..."
git -C "$SUBMODULE_DIR" fetch --tags origin --quiet

CURRENT_SHA=$(git -C "$REPO_ROOT" rev-parse HEAD:"$SUBMODULE_PATH")
CURRENT_TAG=$(git -C "$SUBMODULE_DIR" describe --tags --exact-match "$CURRENT_SHA" 2>/dev/null || \
              git -C "$SUBMODULE_DIR" describe --tags "$CURRENT_SHA" 2>/dev/null || \
              echo "$CURRENT_SHA")

if [[ -n "${1:-}" ]]; then
    TARGET_TAG="$1"
    # Verify the specified <tag> exists.
    if ! git -C "$SUBMODULE_DIR" rev-parse --verify "refs/tags/$TARGET_TAG" >/dev/null 2>&1; then
        echo "Error: tag '$TARGET_TAG' not found in llama.cpp" >&2
        exit 1
    fi
else
    # Find the latest b* tag by sorting numerically on the part after 'b'
    TARGET_TAG=$(git -C "$SUBMODULE_DIR" tag -l 'b[0-9]*' --sort=-v:refname | head -1 || true)
    if [[ -z "$TARGET_TAG" ]]; then
        echo "Error: no b* tags found in llama.cpp" >&2
        exit 1
    fi
fi

TARGET_SHA=$(git -C "$SUBMODULE_DIR" rev-parse "refs/tags/$TARGET_TAG")

echo "Current: $CURRENT_TAG ($CURRENT_SHA)"
echo "Target:  $TARGET_TAG ($TARGET_SHA)"

if [[ "$CURRENT_SHA" == "$TARGET_SHA" ]]; then
    echo "Already up to date."
    exit 0
fi

echo ""
echo "Updating submodule to $TARGET_TAG..."
git -C "$SUBMODULE_DIR" checkout --quiet "$TARGET_SHA"

echo "Staging submodule change..."
git -C "$REPO_ROOT" add -f "$SUBMODULE_PATH"

COMMIT_MSG="chore: bump llama.cpp (https://github.com/ggml-org/llama.cpp/releases/$TARGET_TAG)"
echo ""
echo "Done. Commit with:"
echo "  git commit --signoff -S -m '$COMMIT_MSG'"
