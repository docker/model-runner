#!/bin/bash

# Install additional system packages on top of the upstream llama.cpp image.
#
# The upstream image already ships GPU libraries (Vulkan, CUDA, ROCm) and
# libgomp1, so we only need:
#   - ca-certificates  (for HTTPS model downloads)
#   - mesa patches     (aarch64 + cpu variant only — Docker Desktop virtio-vulkan)

enable_source_repos() {
  # DEB822 format (Ubuntu 24.04+)
  for f in /etc/apt/sources.list.d/*.sources; do
    [ -f "$f" ] && sed -i 's/^Types: deb$/Types: deb deb-src/' "$f"
  done
  # Traditional format: uncomment existing deb-src lines
  [ -f /etc/apt/sources.list ] && sed -i '/^#\s*deb-src/s/^#\s*//' /etc/apt/sources.list
}

rebuild_and_install_mesa() {
  enable_source_repos
  apt-get update
  apt-get install -y dpkg-dev
  apt-get build-dep -y mesa

  local build_dir
  build_dir=$(mktemp -d)
  pushd "$build_dir"

  apt-get source mesa
  cd mesa-*/

  local script_dir
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  patch -p1 < "$script_dir/0001-Revert-venus-filter-out-venus-incapable-physical-dev.patch"
  patch -p1 < "$script_dir/0001-virtio-vulkan-force-16k-alignment-for-allocations-HA.patch"
  patch -p1 < "$script_dir/0002-virtio-vulkan-silence-stuck-in-wait-message-HACK.patch"

  dpkg-buildpackage -us -uc -b

  cd ..
  dpkg -i mesa-vulkan-drivers_*.deb

  popd
  rm -rf "$build_dir"
}

main() {
  set -eux -o pipefail

  apt-get update
  local packages=("ca-certificates")

  # On aarch64 CPU (Vulkan) builds, rebuild mesa with Docker Desktop
  # virtio-vulkan patches.
  if [ "$LLAMA_SERVER_VARIANT" = "cpu" ] && [ "$(uname -m)" = "aarch64" ]; then
    rebuild_and_install_mesa
  fi

  apt-get install -y "${packages[@]}"
  rm -rf /var/lib/apt/lists/*
}

main "$@"
