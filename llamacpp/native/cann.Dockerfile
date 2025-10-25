# syntax=docker/dockerfile:1

ARG CANN_VERSION=8.2.rc2-910b
ARG CANN_IMAGE_VARIANT=ubuntu22.04
ARG ASCEND_SOC_TYPE=Ascend910B3

FROM quay.io/ascend/cann:${CANN_VERSION}-${CANN_IMAGE_VARIANT}-py3.11 AS builder

ARG TARGETARCH
ARG CANN_IMAGE_VARIANT
ARG ASCEND_SOC_TYPE

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
    -DGGML_NATIVE=OFF \
    -DGGML_OPENMP=OFF \
    -DGGML_CANN=ON \
    -DLLAMA_CURL=OFF \
    -DSOC_TYPE=${ASCEND_SOC_TYPE} \
    -GNinja \
    -S ." > cmake-flags

RUN cmake $(cat cmake-flags)

RUN --mount=type=cache,target=/root/.ccache \
    cann_in_sys_path=/usr/local/Ascend/ascend-toolkit; \
    cann_in_user_path=$HOME/Ascend/ascend-toolkit; \
    uname_m=$(uname -m) && \
    if [ -f "${cann_in_sys_path}/set_env.sh" ]; then \
        source ${cann_in_sys_path}/set_env.sh; \
        export LD_LIBRARY_PATH=${cann_in_sys_path}/latest/lib64:${cann_in_sys_path}/latest/${uname_m}-linux/devlib:${LD_LIBRARY_PATH} ; \
    elif [ -f "${cann_in_user_path}/set_env.sh" ]; then \
        source "$HOME/Ascend/ascend-toolkit/set_env.sh"; \
        export LD_LIBRARY_PATH=${cann_in_user_path}/latest/lib64:${cann_in_user_path}/latest/${uname_m}-linux/devlib:${LD_LIBRARY_PATH}; \ 
    else \
        echo "No Ascend Toolkit found"; \
        exit 1; \
    fi && \
    cmake --build build --config Release && \
    cmake --install build --config Release --prefix install

RUN rm install/bin/*.py
RUN rm -r install/lib/cmake
RUN rm -r install/lib/pkgconfig
RUN rm -r install/include

FROM quay.io/ascend/cann:${CANN_VERSION}-${CANN_IMAGE_VARIANT}-py3.11 AS final
ARG TARGETARCH
ARG CANN_VERSION

COPY --from=builder /llama-server/install /com.docker.llama-server.native.linux.cann.${TARGETARCH}
