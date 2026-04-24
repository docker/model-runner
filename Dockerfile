# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25
ARG LLAMA_SERVER_VERSION=b8882
ARG LLAMA_SERVER_VARIANT=cpu
ARG LLAMA_UPSTREAM_IMAGE=ghcr.io/ggml-org/llama.cpp:server-vulkan-b8882

# Use 26.04 for the default Vulkan-backed Linux image.
# GPU variants should pair this with a compatible runtime base image.
ARG BASE_IMAGE=ubuntu:26.04

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

# --- Get llama.cpp binary ---
FROM ${LLAMA_UPSTREAM_IMAGE} AS llama-server

# --- Final image ---
FROM docker.io/${BASE_IMAGE} AS llamacpp

ARG LLAMA_SERVER_VARIANT

# Create non-root user
RUN groupadd --system modelrunner && useradd --system --gid modelrunner -G video --create-home --home-dir /home/modelrunner modelrunner
# TODO: if the render group ever gets a fixed GID add modelrunner to it

COPY scripts/ /scripts/

# Install ca-certificates for HTTPS and vulkan
RUN /scripts/apt-install.sh && rm -rf /scripts

WORKDIR /app

# Create directories for the socket file and set proper permissions
RUN mkdir -p /var/run/model-runner /models && \
    chown -R modelrunner:modelrunner /var/run/model-runner /app /models && \
    chmod -R 755 /models

# Copy the upstream llama.cpp /app layout as-is.  The Go binary-resolver
# (resolveLlamaServerBin) discovers "llama-server" automatically when the
# Docker-convention "com.docker.llama-server" is absent.
COPY --from=llama-server /app/ /app/

# Verify that every shared library copied from the upstream image can resolve
# its runtime dependencies.  This catches missing system packages (e.g.
# libgomp1) at build time instead of letting them surface as cryptic
# "no CPU backend found" errors at runtime.
RUN set -e; missing=""; \
    export LD_LIBRARY_PATH=/app; \
    for f in /app/llama-server /app/*.so; do \
      out=$(ldd "$f" 2>&1) || true; \
      not_found=$(echo "$out" | grep "not found" || true); \
      if [ -n "$not_found" ]; then \
        missing="$missing\n$f:\n$not_found"; \
      fi; \
    done; \
    if [ -n "$missing" ]; then \
      printf "ERROR: unresolved shared-library dependencies:\n%b\n" "$missing" >&2; \
      exit 1; \
    fi

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
    && ~/.local/bin/uv pip install vllm \
         --extra-index-url https://wheels.vllm.ai/${VLLM_VERSION}/cu130 \
         --extra-index-url https://download.pytorch.org/whl/cu130 \
         --index-strategy unsafe-best-match

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
