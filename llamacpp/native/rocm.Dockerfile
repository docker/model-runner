# syntax=docker/dockerfile:1

ARG ROCM_VERSION=7.0.2
ARG ROCM_IMAGE_VARIANT=ubuntu-22.04

FROM rocm/dev-${ROCM_IMAGE_VARIANT}:${ROCM_VERSION}-complete AS builder

ARG TARGETARCH
ARG ROCM_IMAGE_VARIANT

RUN apt-get update && apt-get install -y cmake ninja-build git

WORKDIR /llama-server

COPY .git .git
COPY llamacpp/native/CMakeLists.txt .
COPY llamacpp/native/src src
COPY llamacpp/native/vendor vendor

# Fix submodule .git file to point to correct location in container
RUN echo "gitdir: ../../.git/modules/llamacpp/native/vendor/llama.cpp" > vendor/llama.cpp/.git && \
    sed -i 's|worktree = ../../../../../../llamacpp/native/vendor/llama.cpp|worktree = /llama-server/vendor/llama.cpp|' .git/modules/llamacpp/native/vendor/llama.cpp/config

ENV HIPCXX=/opt/rocm/llvm/bin/clang++
ENV HIP_PATH=/opt/rocm
RUN cmake -B build \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=ON \
    -DGGML_BACKEND_DL=ON \
    -DGGML_CPU_ALL_VARIANTS=ON \
    -DGGML_NATIVE=OFF \
    -DGGML_OPENMP=OFF \
    -DGGML_HIP=ON \
    -DAMDGPU_TARGETS="gfx908;gfx90a;gfx942;gfx1010;gfx1030;gfx1100;gfx1200;gfx1201;gfx1151" \
    -DLLAMA_CURL=OFF \
    -GNinja \
    -S .
RUN cmake --build build --config Release
RUN cmake --install build --config Release --prefix install

RUN rm install/bin/*.py
RUN rm -r install/lib/cmake
RUN rm -r install/lib/pkgconfig
RUN rm -r install/include

FROM scratch AS final

ARG TARGETARCH
ARG ROCM_VERSION

COPY --from=builder /llama-server/install /com.docker.llama-server.native.linux.rocm$ROCM_VERSION.$TARGETARCH
