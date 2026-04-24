#!/bin/bash

# resolve_llama_upstream_image prints the official Linux image reference
# for the requested llama.cpp version and variant.
#
# When a specific bNNNN version is requested the script validates that the
# corresponding tag exists in GHCR.  If the tag is missing it lists the
# closest available versions and exits with an error.
#
# Dependencies: curl, jq

set -euo pipefail

REGISTRY="ghcr.io"
REPO="ggml-org/llama.cpp"

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

# --------------------------------------------------------------------------
# GHCR tag validation helpers
# --------------------------------------------------------------------------

# ghcr_token fetches an anonymous bearer token for the repository.
ghcr_token() {
  curl -sf \
    "https://${REGISTRY}/token?service=${REGISTRY}&scope=repository:${REPO}:pull" \
    | jq -r '.token'
}

# ghcr_fetch_all_tags retrieves every tag in the repository (paginated).
# Prints one tag per line on stdout.
ghcr_fetch_all_tags() {
  local token="$1"
  local url="https://${REGISTRY}/v2/${REPO}/tags/list?n=10000"
  local page=0

  while [ -n "$url" ] && [ "$page" -lt 50 ]; do
    page=$((page + 1))
    local response
    response=$(curl -si -H "Authorization: Bearer $token" "$url" 2>&1)

    # Body is the last line of the curl -si output.
    local body
    body=$(echo "$response" | tail -1)
    echo "$body" | jq -r '.tags[]'

    # Follow the Link header for the next page, if any.
    local link
    link=$(echo "$response" | grep -i '^link:' | sed 's/.*<\(.*\)>.*/\1/' || true)
    if [ -n "$link" ]; then
      url="https://${REGISTRY}${link}"
    else
      url=""
    fi
  done
}

# find_newer_versions filters available version numbers to those >= requested,
# sorted ascending, and returns up to $limit results.
find_newer_versions() {
  local requested_num="$1"
  shift
  local available=("$@")

  for v in "${available[@]}"; do
    if [ "$v" -ge "$requested_num" ]; then
      printf '%d\n' "$v"
    fi
  done | sort -n | head -5
}

# find_latest_versions returns the N most recent available version numbers
# (sorted descending).
find_latest_versions() {
  local available=("$@")

  for v in "${available[@]}"; do
    printf '%d\n' "$v"
  done | sort -rn | head -5
}

# validate_tag_exists checks whether the candidate tag exists in GHCR.
# If not, it fetches available tags for the variant and suggests newer ones.
validate_tag_exists() {
  local tag_base="$1"
  local version="$2"
  local candidate_tag="${tag_base}-${version}"

  # Obtain a bearer token.
  local token
  token=$(ghcr_token) || {
    echo "Warning: Could not fetch GHCR token. Skipping tag validation." >&2
    return 0
  }

  # Fetch all tags and check if the candidate exists.
  local all_tags
  all_tags=$(ghcr_fetch_all_tags "$token")

  if echo "$all_tags" | grep -qx "$candidate_tag"; then
    # Tag exists — nothing to do.
    return 0
  fi

  # Tag does not exist.  Extract available bNNNN numbers for this variant.
  local available_nums
  available_nums=$(echo "$all_tags" \
    | sed -n "s/^${tag_base}-b\([0-9][0-9]*\)$/\1/p" \
    | sort -un)

  if [ -z "$available_nums" ]; then
    echo "Error: Tag '${candidate_tag}' not found in ${REGISTRY}/${REPO}." >&2
    echo "Could not fetch available versions to suggest alternatives." >&2
    return 1
  fi

  local requested_num="${version#b}"

  echo "Error: Tag '${candidate_tag}' not found in ${REGISTRY}/${REPO}." >&2
  echo "" >&2

  local nums_array
  # shellcheck disable=SC2206
  nums_array=($available_nums)

  # Show newer versions first; fall back to latest available if none newer.
  local suggestions
  suggestions=$(find_newer_versions "$requested_num" "${nums_array[@]}")

  if [ -n "$suggestions" ]; then
    echo "Available newer versions for ${tag_base}:" >&2
  else
    suggestions=$(find_latest_versions "${nums_array[@]}")
    echo "No newer versions available. Latest published versions for ${tag_base}:" >&2
  fi

  while IFS= read -r num; do
    printf '  b%-6s →  %s/%s:%s-b%s\n' \
      "$num" "$REGISTRY" "$REPO" "$tag_base" "$num" >&2
  done <<< "$suggestions"

  return 1
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

  local image="${REGISTRY}/${REPO}:${tag_base}${version_suffix}"

  # Validate against GHCR for pinned versions (not "latest").
  if [ "$version" != "latest" ]; then
    validate_tag_exists "$tag_base" "$version"
  fi

  printf '%s\n' "$image"
}

main "$@"
