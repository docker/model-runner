# syntax=docker/dockerfile:1

ARG GO_VERSION=1.24
ARG LLAMA_SERVER_VERSION=latest
ARG LLAMA_SERVER_VARIANT=cpu
ARG LLAMA_BINARY_PATH=/com.docker.llama-server.native.linux.${LLAMA_SERVER_VARIANT}.${TARGETARCH}

# only 25.10 for cpu variant for max hardware support with vulkan
ARG BASE_IMAGE=ubuntu:25.10

FROM docker.io/library/golang:${GO_VERSION}-bookworm AS builder

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
    CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o model-runner ./main.go

# --- Get llama.cpp binary ---
FROM docker/docker-model-backend-llamacpp:${LLAMA_SERVER_VERSION}-${LLAMA_SERVER_VARIANT} AS llama-server

# --- Final image ---
FROM docker.io/${BASE_IMAGE} AS final

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

# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner

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
ENV LD_LIBRARY_PATH=/app/lib

# Label the image so that it's hidden on cloud engines.
LABEL com.docker.desktop.service="model-runner"

ENTRYPOINT ["/app/model-runner"]

# --- vLLM variant ---
FROM final AS vllm

ARG VLLM_VERSION

USER root

RUN apt update && apt install -y python3 python3-venv python3-dev curl ca-certificates build-essential && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/vllm-env && chown -R modelrunner:modelrunner /opt/vllm-env

USER modelrunner

# Install uv and vLLM as modelrunner user
RUN curl -LsSf https://astral.sh/uv/install.sh | sh \
 && ~/.local/bin/uv venv --python /usr/bin/python3 /opt/vllm-env \
 && ~/.local/bin/uv pip install --python /opt/vllm-env/bin/python "vllm==${VLLM_VERSION}"
