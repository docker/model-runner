#!/bin/bash

main() {
  set -eux -o pipefail

  apt-get update
  apt-get install -y cmake ninja-build git wget gnupg2 clang lldb lld
}

main "$@"

