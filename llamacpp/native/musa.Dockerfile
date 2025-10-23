# syntax=docker/dockerfile:1

ARG MUSA_VERSION=rc4.2.0
ARG MUSA_IMAGE_VARIANT=ubuntu22.04

FROM mthreads/musa:${MUSA_VERSION}-devel-${MUSA_IMAGE_VARIANT}amd64 AS builder

ARG TARGETARCH
ARG MUSA_IMAGE_VARIANT

RUN apt-get update && apt-get install -y cmake ninja-build git build-essential curl

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
    -DBUILD_SHARED_LIBS=ON \
    -DGGML_BACKEND_DL=ON \
    -DGGML_CPU_ALL_VARIANTS=ON \
    -DGGML_NATIVE=OFF \
    -DGGML_OPENMP=OFF \
    -DGGML_MUSA=ON \
    -DLLAMA_CURL=OFF \
    -GNinja \
    -S ." > cmake-flags
RUN cmake $(cat cmake-flags)
RUN cmake --build build --config Release
RUN cmake --install build --config Release --prefix install

RUN rm install/bin/*.py
RUN rm -r install/lib/cmake
RUN rm -r install/lib/pkgconfig
RUN rm -r install/include

FROM scratch AS final

ARG TARGETARCH
ARG MUSA_VERSION

COPY --from=builder /llama-server/install /com.docker.llama-server.native.linux.musa$MUSA_VERSION.$TARGETARCH
