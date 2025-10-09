#!/bin/bash

main() {
  set -eux -o pipefail

  apt-get update
  local packages=("ca-certificates")
  if [ "$LLAMA_SERVER_VARIANT" = "generic" ] || [ "$LLAMA_SERVER_VARIANT" = "cpu" ]; then
    # Install Vulkan SDK
    local vulkan_version=1.4.321.1
    local arch
    arch=$(uname -m)
    apt-get install -y wget xz-utils
    wget -qO /tmp/vulkan-sdk.tar.xz https://sdk.lunarg.com/sdk/download/$vulkan_version/linux/vulkan-sdk-linux-"$arch"-$vulkan_version.tar.xz
    mkdir -p /opt/vulkan
    tar -xf /tmp/vulkan-sdk.tar.xz -C /tmp

    if [ "$arch" != "x86_64" ]; then
        # TODO: uninstall build time deps after building the SDK
        apt-get install -y libglm-dev cmake libxcb-dri3-0 libxcb-present0 libpciaccess0 \
            libpng-dev libxcb-keysyms1-dev libxcb-dri3-dev libx11-dev g++ gcc \
            libwayland-dev libxrandr-dev libxcb-randr0-dev libxcb-ewmh-dev \
            git python-is-python3 bison libx11-xcb-dev liblz4-dev libzstd-dev \
            ocaml-core ninja-build pkg-config libxml2-dev wayland-protocols python3-jsonschema \
            clang-format qtbase5-dev qt6-base-dev
        pushd /tmp/"${vulkan_version}"
        # TODO: we don't need the whole SDK to run stuff, so eventually only build necessary targets here
        ./vulkansdk --no-deps -j "$(nproc)"
    fi

    mv /tmp/"${vulkan_version}"/"$arch"/* /opt/vulkan/
    rm -rf /tmp/*
  fi

  apt-get install -y "${packages[@]}"
  rm -rf /var/lib/apt/lists/*
}

main "$@"

