#!/bin/bash

main() {
  set -eux -o pipefail

  apt-get update
  local packages=("ca-certificates")
  if [ "$LLAMA_SERVER_VARIANT" = "generic" ] || [ "$LLAMA_SERVER_VARIANT" = "cpu" ]; then
    apt-get install -y libvulkan1
  fi

  apt-get install -y "${packages[@]}"
  rm -rf /var/lib/apt/lists/*
}

main "$@"

