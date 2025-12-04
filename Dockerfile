# syntax=docker/dockerfile:1

ARG GO_VERSION=1.24
ARG LLAMA_SERVER_VERSION=latest
ARG LLAMA_SERVER_VARIANT=cpu
ARG LLAMA_BINARY_PATH=/com.docker.llama-server.native.linux.${LLAMA_SERVER_VARIANT}.${TARGETARCH}

# only 26.04 for cpu variant for max hardware support with vulkan
# use 22.04 for gpu variants to match ROCm/CUDA base images
ARG BASE_IMAGE=ubuntu:26.04

FROM docker.io/library/golang:${GO_VERSION}-bookworm AS builder

# Install git for go mod download if needed
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go mod/sum first for better caching
COPY --link go.mod go.sum ./

# Copy pkg/go-containerregistry for the replace directive in go.mod
COPY --link pkg/go-containerregistry ./pkg/go-containerregistry

# Download dependencies (with cache mounts)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Copy the rest of the source code
COPY --link . .

# Build the Go binary (static build)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o model-runner ./main.go

# --- Get llama.cpp binary ---
FROM docker/docker-model-backend-llamacpp:${LLAMA_SERVER_VERSION}-${LLAMA_SERVER_VARIANT} AS llama-server

# --- Final image ---
FROM docker.io/${BASE_IMAGE} AS llamacpp

ARG LLAMA_SERVER_VARIANT

# Create non-root user
RUN groupadd --system modelrunner && useradd --system --gid modelrunner -G video --create-home --home-dir /home/modelrunner modelrunner
# TODO: if the render group ever gets a fixed GID add modelrunner to it

COPY scripts/apt-install.sh apt-install.sh

# Install ca-certificates for HTTPS and vulkan
RUN ./apt-install.sh

WORKDIR /app

# Create directories for the socket file and llama.cpp binary, and set proper permissions
RUN mkdir -p /var/run/model-runner /app/bin /models && \
    chown -R modelrunner:modelrunner /var/run/model-runner /app /models && \
    chmod -R 755 /models

# Copy the llama.cpp binary from the llama-server stage
ARG LLAMA_BINARY_PATH
COPY --from=llama-server ${LLAMA_BINARY_PATH}/ /app/.
RUN chmod +x /app/bin/com.docker.llama-server

USER modelrunner

# Set the environment variable for the socket path and LLaMA server binary path
ENV MODEL_RUNNER_SOCK=/var/run/model-runner/model-runner.sock
ENV MODEL_RUNNER_PORT=12434
ENV LLAMA_SERVER_PATH=/app/bin
ENV HOME=/home/modelrunner
ENV MODELS_PATH=/models
ENV LD_LIBRARY_PATH=/app/lib:$LD_LIBRARY_PATH

# Label the image so that it's hidden on cloud engines.
LABEL com.docker.desktop.service="model-runner"

ENTRYPOINT ["/app/model-runner"]

# --- vLLM variant ---
FROM llamacpp AS vllm

ARG VLLM_VERSION=0.12.0
ARG VLLM_CUDA_VERSION=cu130
ARG VLLM_PYTHON_TAG=cp38-abi3
ARG TARGETARCH

USER root

RUN apt update && apt install -y python3 python3-venv python3-dev curl ca-certificates build-essential && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/vllm-env && chown -R modelrunner:modelrunner /opt/vllm-env

USER modelrunner

# Install uv and vLLM as modelrunner user
RUN curl -LsSf https://astral.sh/uv/install.sh | sh \
 && ~/.local/bin/uv venv --python /usr/bin/python3 /opt/vllm-env \
 && if [ "$TARGETARCH" = "amd64" ]; then \
      WHEEL_ARCH="manylinux_2_31_x86_64"; \
      WHEEL_URL="https://github.com/vllm-project/vllm/releases/download/v${VLLM_VERSION}/vllm-${VLLM_VERSION}%2B${VLLM_CUDA_VERSION}-${VLLM_PYTHON_TAG}-${WHEEL_ARCH}.whl"; \
      ~/.local/bin/uv pip install --python /opt/vllm-env/bin/python "$WHEEL_URL"; \
    else \
      ~/.local/bin/uv pip install --python /opt/vllm-env/bin/python "vllm==${VLLM_VERSION}"; \
    fi

RUN /opt/vllm-env/bin/python -c "import vllm; print(vllm.__version__)" > /opt/vllm-env/version

# --- SGLang variant ---
FROM llamacpp AS sglang

ARG SGLANG_VERSION=0.4.0

USER root

RUN apt update && apt install -y python3 python3-venv python3-dev curl ca-certificates build-essential && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/sglang-env && chown -R modelrunner:modelrunner /opt/sglang-env

USER modelrunner

# Install uv and SGLang as modelrunner user
RUN curl -LsSf https://astral.sh/uv/install.sh | sh \
 && ~/.local/bin/uv venv --python /usr/bin/python3 /opt/sglang-env \
 && ~/.local/bin/uv pip install --python /opt/sglang-env/bin/python "sglang==${SGLANG_VERSION}"

RUN /opt/sglang-env/bin/python -c "import sglang; print(sglang.__version__)" > /opt/sglang-env/version

FROM llamacpp AS final-llamacpp
# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner

FROM vllm AS final-vllm
# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner

FROM sglang AS final-sglang
# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner
