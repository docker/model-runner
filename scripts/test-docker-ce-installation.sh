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

  echo "Testing Docker CE installation with expected version: $cli_version"

  local base_image="${BASE_IMAGE:-ubuntu:24.04}"
  echo "Using base image: $base_image"

  local server_image="docker/model-runner:$cli_version"
  echo "Using server image: $server_image"

  # Start the model-runner server container
  echo "Starting model-runner server..."
  docker run -d --name dmr-version-test -p 12434:12434 "$server_image"

  # Ensure cleanup on exit
  cleanup() {
    echo "Stopping model-runner server..."
    docker stop dmr-version-test 2>/dev/null || true
    docker rm dmr-version-test 2>/dev/null || true
  }
  trap cleanup EXIT

  # Wait for server to be ready
  echo "Waiting for server to be ready..."
  for i in $(seq 1 30); do
    if curl -sf http://localhost:12434/version > /dev/null 2>&1; then
      echo "Server is ready"
      break
    fi
    if [ "$i" -eq 30 ]; then
      echo "Error: Server did not become ready in time" >&2
      docker logs dmr-version-test
      exit 1
    fi
    sleep 1
  done

  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  echo "Starting test container..."
  docker run --rm \
    --network host \
    -e "EXPECTED_VERSION=$cli_version" \
    -e "MODEL_RUNNER_HOST=http://localhost:12434" \
    -v "$script_dir/test-docker-ce-in-container.sh:/test.sh:ro" \
    "$base_image" \
    /test.sh

  echo "✓ Docker CE installation test passed!"
}

main "$@"
