# syntax=docker/dockerfile:1

ARG CUDA_VERSION=12.9.0
ARG CUDA_IMAGE_VARIANT=ubuntu22.04

FROM nvidia/cuda:${CUDA_VERSION}-devel-${CUDA_IMAGE_VARIANT} AS builder

ARG TARGETARCH
ARG CUDA_IMAGE_VARIANT

COPY llamacpp/native/install-clang.sh .
RUN ./install-clang.sh "${CUDA_IMAGE_VARIANT}"

WORKDIR /llama-server

COPY .git .git
COPY llamacpp/native/CMakeLists.txt .
COPY llamacpp/native/src src
COPY llamacpp/native/vendor vendor

# Fix submodule .git file to point to correct location in container
RUN echo "gitdir: ../../.git/modules/llamacpp/native/vendor/llama.cpp" > vendor/llama.cpp/.git && \
    sed -i 's|worktree = ../../../../../../llamacpp/native/vendor/llama.cpp|worktree = /llama-server/vendor/llama.cpp|' .git/modules/llamacpp/native/vendor/llama.cpp/config

ENV CC=/usr/bin/clang
ENV CXX=/usr/bin/clang++
RUN echo "-B build \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=ON \
    -DGGML_BACKEND_DL=ON \
    -DGGML_CPU_ALL_VARIANTS=ON \
    -DGGML_NATIVE=OFF \
    -DGGML_OPENMP=OFF \
    -DGGML_CUDA=ON \
    -DCMAKE_CUDA_COMPILER=/usr/local/cuda/bin/nvcc \
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
ARG CUDA_VERSION

COPY --from=builder /llama-server/install /com.docker.llama-server.native.linux.cuda$CUDA_VERSION.$TARGETARCH
