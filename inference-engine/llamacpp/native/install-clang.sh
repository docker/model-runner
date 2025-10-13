#!/bin/bash

main() {
  set -eux -o pipefail

  apt-get update && apt-get install -y cmake ninja-build git wget gnupg2
  wget -qO- https://apt.llvm.org/llvm-snapshot.gpg.key | tee /etc/apt/trusted.gpg.d/apt.llvm.org.asc

  if [ "$1" = "ubuntu22.04" ]; then
    echo "deb http://apt.llvm.org/jammy/ llvm-toolchain-jammy-20 main" >> /etc/apt/sources.list
    echo "deb-src http://apt.llvm.org/jammy/ llvm-toolchain-jammy-20 main" >> /etc/apt/sources.list
  elif [ "$1" = "ubuntu24.04" ]; then
    echo "deb http://apt.llvm.org/noble/ llvm-toolchain-noble-20 main" >> /etc/apt/sources.list
    echo "deb-src http://apt.llvm.org/noble/ llvm-toolchain-noble-20 main" >> /etc/apt/sources.list
  else
      echo "distro variant not supported yet"
      exit 1
  fi

  apt-get update && apt-get install -y clang-20 lldb-20 lld-20
}

main "$@"

