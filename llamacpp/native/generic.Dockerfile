# syntax=docker/dockerfile:1

ARG BASE_IMAGE=ubuntu:25.10

FROM ${BASE_IMAGE} AS builder

ARG TARGETARCH

RUN apt-get update && apt-get install -y cmake ninja-build git build-essential curl

COPY llamacpp/native/install-vulkan.sh .
RUN ./install-vulkan.sh

ENV VULKAN_SDK=/opt/vulkan
ENV PATH=$VULKAN_SDK/bin:$PATH
ENV LD_LIBRARY_PATH=$VULKAN_SDK/lib
ENV CMAKE_PREFIX_PATH=$VULKAN_SDK
ENV PKG_CONFIG_PATH=$VULKAN_SDK/lib/pkgconfig

WORKDIR /llama-server

COPY .git .git
COPY llamacpp/native/CMakeLists.txt .
COPY llamacpp/native/src src
COPY llamacpp/native/vendor vendor

# Fix submodule .git file to point to correct location in container
RUN echo "gitdir: ../../.git/modules/llamacpp/native/vendor/llama.cpp" > vendor/llama.cpp/.git && \
    sed -i 's|worktree = ../../../../../../llamacpp/native/vendor/llama.cpp|worktree = /llama-server/vendor/llama.cpp|' .git/modules/llamacpp/native/vendor/llama.cpp/config

RUN echo "-B build \
          -DCMAKE_BUILD_TYPE=Release \
          -DGGML_NATIVE=OFF \
          -DGGML_OPENMP=OFF \
          -DLLAMA_CURL=OFF \
          -DGGML_VULKAN=ON \
          -GNinja \
    -S ." > cmake-flags
RUN if [ "${TARGETARCH}" = "amd64" ]; then \
      echo " -DBUILD_SHARED_LIBS=ON \
             -DGGML_BACKEND_DL=ON \
             -DGGML_CPU_ALL_VARIANTS=ON" >> cmake-flags; \
    elif [ "${TARGETARCH}" = "arm64" ]; then \
      echo " -DBUILD_SHARED_LIBS=OFF" >> cmake-flags; \
    else \
      echo "${TARGETARCH} is not supported"; \
      exit 1; \
    fi
RUN cmake $(cat cmake-flags)
RUN cmake --build build --config Release -j 4
RUN cmake --install build --config Release --prefix install

RUN rm install/bin/*.py
RUN rm -r install/lib/cmake
RUN rm -r install/lib/pkgconfig
RUN rm -r install/include

FROM scratch AS final

ARG TARGETARCH

COPY --from=builder /llama-server/install /com.docker.llama-server.native.linux.cpu.$TARGETARCH
