# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25
ARG LLAMA_SERVER_VERSION=b8967
ARG LLAMA_SERVER_VARIANT=cpu
ARG LLAMA_UPSTREAM_IMAGE=ghcr.io/ggml-org/llama.cpp:server-vulkan-b8967

ARG VERSION=dev

FROM docker.io/library/golang:${GO_VERSION}-bookworm AS builder

ARG VERSION

# Install git for go mod download if needed
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go mod/sum first for better caching
COPY --link go.mod go.sum ./

# Download dependencies (with cache mounts)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the rest of the source code
COPY --link . .

# Build the Go binary (static build)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w -X main.Version=${VERSION}" -o model-runner .

# Build the Go binary for SGLang (without vLLM)
FROM builder AS builder-sglang
ARG VERSION
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -tags=novllm -ldflags="-s -w -X main.Version=${VERSION}" -o model-runner .

# --- Final image: directly FROM the upstream llama.cpp image ---
FROM ${LLAMA_UPSTREAM_IMAGE} AS llamacpp

ARG LLAMA_SERVER_VARIANT

# Create non-root user
RUN groupadd --system modelrunner && useradd --system --gid modelrunner -G video --create-home --home-dir /home/modelrunner modelrunner
# TODO: if the render group ever gets a fixed GID add modelrunner to it

COPY scripts/ /scripts/

# Install additional packages not shipped by the upstream image
# (e.g. ca-certificates for HTTPS, mesa patches for aarch64 virtio-vulkan).
RUN /scripts/apt-install.sh && rm -rf /scripts

WORKDIR /app

# Create directories for the socket file and set proper permissions
RUN mkdir -p /var/run/model-runner /models && \
    chown -R modelrunner:modelrunner /var/run/model-runner /app /models && \
    chmod -R 755 /models

USER modelrunner

# Set the environment variable for the socket path and LLamA server binary path.
# LLAMA_SERVER_PATH points at the directory containing the llama-server binary
# and its ggml backend plugins — keeping them together lets llama.cpp discover
# backends via its default search path (relative to the binary).
ENV MODEL_RUNNER_SOCK=/var/run/model-runner/model-runner.sock
ENV MODEL_RUNNER_PORT=12434
ENV LLAMA_SERVER_PATH=/app
# LD_LIBRARY_PATH is required so that backend plugins loaded via dlopen()
# (e.g. libggml-cpu-*.so, libggml-vulkan.so) can resolve their transitive
# dependencies on libggml-base.so and other shared libraries in /app.
ENV LD_LIBRARY_PATH=/app
ENV HOME=/home/modelrunner
ENV MODELS_PATH=/models

# Label the image so that it's hidden on cloud engines.
LABEL com.docker.desktop.service="model-runner"

ENTRYPOINT ["/app/model-runner"]

# --- vLLM variant ---
FROM llamacpp AS vllm

ARG VLLM_VERSION=0.19.1
ARG VLLM_CUDA_VERSION=cu130
ARG VLLM_PYTHON_TAG=cp38-abi3
ARG TARGETARCH

USER root

RUN apt update && apt install -y python3.12 python3.12-venv python3.12-dev curl ca-certificates build-essential && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/vllm-env && chown -R modelrunner:modelrunner /opt/vllm-env

USER modelrunner

# Install uv and vLLM as modelrunner user
RUN curl -LsSf https://astral.sh/uv/install.sh | sh \
    && ~/.local/bin/uv venv --python 3.12 /opt/vllm-env \
    && . /opt/vllm-env/bin/activate \
    && ~/.local/bin/uv pip install vllm --torch-backend auto

RUN /opt/vllm-env/bin/python3.12 -c "import vllm; print(vllm.__version__)" > /opt/vllm-env/version

# --- SGLang variant ---
FROM llamacpp AS sglang

ARG SGLANG_VERSION=0.5.6

USER root

# Install CUDA toolkit 13 for nvcc (needed for flashinfer JIT compilation)
RUN apt update && apt install -y \
    python3.12 python3.12-venv python3.12-dev \
    curl ca-certificates build-essential \
    libnuma1 libnuma-dev numactl ninja-build \
    wget gnupg \
    && wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb \
    && dpkg -i cuda-keyring_1.1-1_all.deb \
    && apt update && apt install -y cuda-toolkit-13-0 \
    && rm cuda-keyring_1.1-1_all.deb \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/sglang-env && chown -R modelrunner:modelrunner /opt/sglang-env

USER modelrunner

# Set CUDA paths for nvcc (needed during flashinfer compilation)
ENV PATH=/usr/local/cuda-13.0/bin:$PATH
ENV LD_LIBRARY_PATH=/usr/local/cuda-13.0/lib64:$LD_LIBRARY_PATH

# Install uv and SGLang as modelrunner user
RUN curl -LsSf https://astral.sh/uv/install.sh | sh \
    && ~/.local/bin/uv venv --python 3.12 /opt/sglang-env \
    && . /opt/sglang-env/bin/activate \
    && ~/.local/bin/uv pip install "sglang==${SGLANG_VERSION}"

RUN /opt/sglang-env/bin/python3.12 -c "import sglang; print(sglang.__version__)" > /opt/sglang-env/version

FROM llamacpp AS final-llamacpp
# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner

FROM vllm AS final-vllm
# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner

FROM sglang AS final-sglang
# Copy the built binary from builder-sglang (without vLLM)
COPY --from=builder-sglang /app/model-runner /app/model-runner

# --- vLLM ROCm: builder stage ---
# Builds upstream vLLM from source on AMD's pre-built ROCm dev image, which
# already contains PyTorch ROCm, Triton, flash-attention, and the ROCm SDK
# (see https://hub.docker.com/r/rocm/vllm-dev). vLLM is checked out at the
# tagged release matching VLLM_VERSION — no fork, no custom wheels.
FROM rocm/vllm-dev:base AS vllm-rocm-builder

ARG VLLM_VERSION=0.19.1
# Target GPU architectures officially supported by vLLM ROCm:
# gfx90a (MI200), gfx942 (MI300), gfx1100/1101 (RDNA3 7900/7800).
ARG PYTORCH_ROCM_ARCH="gfx90a;gfx942;gfx1100;gfx1101"
ENV PYTORCH_ROCM_ARCH=${PYTORCH_ROCM_ARCH}

RUN git clone --depth 1 --branch v${VLLM_VERSION} \
    https://github.com/vllm-project/vllm.git /vllm-src

WORKDIR /vllm-src
RUN python3 -m pip install --no-cache-dir -r requirements/rocm.txt \
    && python3 setup.py bdist_wheel --dist-dir=/wheels

# --- vLLM ROCm: runtime stage ---
# Mirrors the /opt/vllm-env layout that pkg/inference/backends/vllm/vllm.go
# expects (binary at /opt/vllm-env/bin/vllm, version file at
# /opt/vllm-env/version). Symlinks are used instead of a real venv because
# rocm/vllm-dev:base installs Python dependencies system-wide and recreating
# a venv would break the PyTorch ROCm / Triton ROCm wiring.
#
# Note: unlike the CUDA vllm stage, this image does NOT include llama.cpp.
# The base image is incompatible (different ROCm runtime versions), and the
# rocm vllm image is intended as a vLLM-only artifact.
FROM rocm/vllm-dev:base AS vllm-rocm

COPY --from=vllm-rocm-builder /wheels/*.whl /tmp/
RUN python3 -m pip install --no-cache-dir /tmp/*.whl && rm /tmp/*.whl

RUN groupadd --system modelrunner \
    && useradd --system --gid modelrunner -G video \
        --create-home --home-dir /home/modelrunner modelrunner

RUN mkdir -p /opt/vllm-env/bin \
    && ln -s "$(command -v vllm)" /opt/vllm-env/bin/vllm \
    && python3 -c "import vllm; print(vllm.__version__)" > /opt/vllm-env/version \
    && chown -R modelrunner:modelrunner /opt/vllm-env

RUN mkdir -p /var/run/model-runner /models /app \
    && chown -R modelrunner:modelrunner /var/run/model-runner /app /models \
    && chmod -R 755 /models

USER modelrunner

ENV MODEL_RUNNER_SOCK=/var/run/model-runner/model-runner.sock
ENV MODEL_RUNNER_PORT=12434
ENV HOME=/home/modelrunner
ENV MODELS_PATH=/models

LABEL com.docker.desktop.service="model-runner"

ENTRYPOINT ["/app/model-runner"]

FROM vllm-rocm AS final-vllm-rocm
# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner
