# syntax=docker/dockerfile:1

ARG CANN_VERSION=8.0.0-910b
ARG CANN_IMAGE_VARIANT=openeuler22.03

FROM quay.io/ascend/cann:{CANN_VERSION}-{CANN_IMAGE_VARIANT}-py3.10 AS builder

ARG TARGETARCH
ARG CANN_IMAGE_VARIANT

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
    -DGGML_CANN=ON \
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
ARG CANN_VERSION

COPY --from=builder /llama-server/install /com.docker.llama-server.native.linux.cann$CANN_VERSION.$TARGETARCH
