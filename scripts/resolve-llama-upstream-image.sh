#!/bin/bash

# resolve_llama_upstream_image prints the official Linux image reference
# for the requested llama.cpp version and variant.

set -euo pipefail

# usage prints the supported arguments and exits.
usage() {
  cat >&2 <<'EOF'
Usage: resolve-llama-upstream-image.sh <version> <variant>

Supported versions:
  - latest
  - bNNNN

Supported variants:
  - cpu
  - cuda
  - rocm
EOF
  exit 1
}

# resolve_tag_base maps the model-runner variant to the upstream tag base.
resolve_tag_base() {
  local variant="${1-}"

  case "$variant" in
    cpu)
      printf '%s\n' 'server-vulkan'
      ;;
    cuda)
      printf '%s\n' 'server-cuda13'
      ;;
    rocm)
      printf '%s\n' 'server-rocm'
      ;;
    *)
      echo "Unsupported LLAMA_SERVER_VARIANT: $variant" >&2
      usage
      ;;
  esac
}

# resolve_version_suffix validates the requested version and formats the
# upstream tag suffix.
resolve_version_suffix() {
  local version="${1-}"

  if [ "$version" = "latest" ]; then
    printf '%s' ''
    return
  fi

  if [[ "$version" =~ ^b[0-9]+$ ]]; then
    printf -- '-%s\n' "$version"
    return
  fi

  echo "Unsupported LLAMA_SERVER_VERSION: $version" >&2
  usage
}

# main validates arguments and prints the upstream image reference.
main() {
  if [ "$#" -ne 2 ]; then
    usage
  fi

  local version="$1"
  local variant="$2"
  local tag_base
  local version_suffix

  tag_base="$(resolve_tag_base "$variant")"
  version_suffix="$(resolve_version_suffix "$version")"

  printf '%s\n' \
    "ghcr.io/ggml-org/llama.cpp:${tag_base}${version_suffix}"
}

main "$@"
