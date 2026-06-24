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

# Assert CUDA 12.x — required for Pascal sm_61/sm_62 offline compilation.
# The nvidia/cuda base image version is set by the CUDA_VERSION ARG above.
RUN /usr/local/cuda/bin/nvcc --version | grep -q "release 12" || \
    { echo "ERROR: CUDA 12.x is required for Pascal GPU support."; \
      /usr/local/cuda/bin/nvcc --version; exit 1; }

# Explicitly list target CUDA architectures to include Pascal (sm_61, sm_62).
# CMake's CUDA_ARCHITECTURES defaults on CUDA 12.9+ omit pre-Turing architectures,
# causing "no compatible GPU found" on GTX 10-series, P40, and similar Pascal GPUs.
#
# The list includes only architectures supported by the CUDA 12.x toolchain.
# CUDA 13+ drops pre-sm_75 support — stay on CUDA 12.x for Pascal compatibility.
RUN echo "-B build \
    -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=ON \
    -DGGML_BACKEND_DL=ON \
    -DGGML_CPU_ALL_VARIANTS=ON \
    -DGGML_NATIVE=OFF \
    -DGGML_OPENMP=OFF \
    -DGGML_CUDA=ON \
    -DCMAKE_CUDA_ARCHITECTURES=61;62;70;75;80;86;89 \
    -DCMAKE_CUDA_COMPILER=/usr/local/cuda/bin/nvcc \
    -DLLAMA_OPENSSL=OFF \
    -GNinja \
    -S ." > cmake-flags
RUN cmake $(cat cmake-flags)
RUN cmake --build build --config Release -j$(nproc --ignore=2)
RUN cmake --install build --config Release --prefix install

RUN rm -f install/bin/*.py
RUN rm -r install/lib/cmake
RUN rm -r install/lib/pkgconfig
RUN rm -r install/include

FROM scratch AS final

ARG TARGETARCH
ARG CUDA_VERSION

COPY --from=builder /llama-server/install /com.docker.llama-server.native.linux.cuda$CUDA_VERSION.$TARGETARCH
