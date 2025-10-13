#!/bin/bash

main() {
  set -eux -o pipefail

  apt-get install -y glslc libvulkan-dev
}

main "$@"

