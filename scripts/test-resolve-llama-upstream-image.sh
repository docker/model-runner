#!/bin/bash

# test-resolve-llama-upstream-image verifies the upstream image resolver
# mappings used by Docker builds.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVER="$SCRIPT_DIR/resolve-llama-upstream-image.sh"

# assert_resolves checks a successful version and variant mapping.
assert_resolves() {
  local version="$1"
  local variant="$2"
  local want="$3"
  local got

  got="$(bash "$RESOLVER" "$version" "$variant")"
  if [ "$got" != "$want" ]; then
    echo "Unexpected upstream image for $version/$variant." >&2
    echo "Got:  $got" >&2
    echo "Want: $want" >&2
    exit 1
  fi
}

# assert_fails checks an invalid version or variant combination.
assert_fails() {
  local version="$1"
  local variant="$2"

  if bash "$RESOLVER" "$version" "$variant" >/dev/null 2>&1; then
    echo "Expected resolver to fail for $version/$variant." >&2
    exit 1
  fi
}

# main runs the resolver smoke tests.
main() {
  assert_resolves \
    latest cpu \
    ghcr.io/ggml-org/llama.cpp:server-vulkan
  assert_resolves \
    b8840 cpu \
    ghcr.io/ggml-org/llama.cpp:server-vulkan-b8840
  assert_resolves \
    latest cuda \
    ghcr.io/ggml-org/llama.cpp:server-cuda13
  assert_resolves \
    b8840 cuda \
    ghcr.io/ggml-org/llama.cpp:server-cuda13-b8840
  assert_resolves \
    latest rocm \
    ghcr.io/ggml-org/llama.cpp:server-rocm
  assert_resolves \
    b8840 rocm \
    ghcr.io/ggml-org/llama.cpp:server-rocm-b8840

  assert_fails latest generic
  assert_fails latest musa
  assert_fails latest cann
  assert_fails v0.0.4 cpu
}

main
