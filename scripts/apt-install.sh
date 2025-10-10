#!/bin/bash

main() {
  set -eux -o pipefail

  apt-get update
  local packages=("ca-certificates")
  if [ "$LLAMA_SERVER_VARIANT" = "generic" ] || [ "$LLAMA_SERVER_VARIANT" = "cpu" ]; then
    packages+=("libvulkan1" "mesa-vulkan-drivers")
  fi

  apt-get install -y "${packages[@]}"
  rm -rf /var/lib/apt/lists/*
}

main "$@"

