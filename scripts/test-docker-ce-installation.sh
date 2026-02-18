#!/bin/bash

set -eux -o pipefail

main() {
  local cli_version="${1:-}"

  # If no version argument provided, try to detect from git tags
  if [ -z "$cli_version" ]; then
    local remote
    remote=$(git remote -v | awk '/docker\/model-runner/ && /\(fetch\)/ {print $1; exit}')

    if [ -n "$remote" ]; then
      echo "Fetching tags from $remote (docker/model-runner)..."
      git fetch "$remote" --tags >/dev/null 2>&1 || echo "Warning: Failed to fetch tags from $remote. Continuing with local tags." >&2
    else
      echo "Warning: No remote found for docker/model-runner, using local tags only" >&2
    fi

    cli_version=$(git tag -l --sort=-version:refname "v*" | head -1)

    if [ -z "$cli_version" ]; then
      echo "Error: Could not determine CLI version from git tags. Pass version as argument: $0 <version>" >&2
      exit 1
    fi
  fi

  echo "Testing Docker CE installation with expected CLI version: $cli_version"

  if [ -z "${BASE_IMAGE:-}" ]; then
    echo "Error: BASE_IMAGE is not set" >&2
    exit 1
  fi

  echo "Using base image: $BASE_IMAGE"

  echo "Starting container and installing Docker CE..."

  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  docker run --rm \
    -e "EXPECTED_VERSION=$cli_version" \
    -v "$script_dir/test-docker-ce-in-container.sh:/test.sh:ro" \
    "$BASE_IMAGE" \
    /test.sh

  echo "✓ Docker CE installation test passed!"
}

main "$@"
