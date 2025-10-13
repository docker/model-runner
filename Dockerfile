# syntax=docker/dockerfile:1

ARG GO_VERSION=1.24
ARG LLAMA_SERVER_VERSION=latest
ARG LLAMA_SERVER_VARIANT=cpu
ARG LLAMA_BINARY_PATH=/com.docker.llama-server.native.linux.${LLAMA_SERVER_VARIANT}.${TARGETARCH}
ARG BACKEND=llamacpp
ARG VLLM_VERSION=v0.11.0

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

# --- Common setup stage ---
FROM scratch AS setup-files
COPY scripts/setup-modelrunner.sh /tmp/setup-modelrunner.sh

# --- Final image for llama.cpp ---
FROM docker.io/${BASE_IMAGE} AS final-llamacpp

ARG LLAMA_SERVER_VARIANT

COPY scripts/apt-install.sh apt-install.sh
# Install ca-certificates for HTTPS and vulkan
RUN ./apt-install.sh

WORKDIR /app

# Copy the llama.cpp binary from the llama-server stage
ARG LLAMA_BINARY_PATH
COPY --from=llama-server ${LLAMA_BINARY_PATH}/ /app/.
RUN chmod +x /app/bin/com.docker.llama-server

# Setup modelrunner user, directories and permissions
COPY --from=setup-files /tmp/setup-modelrunner.sh /tmp/setup-modelrunner.sh
RUN chmod +x /tmp/setup-modelrunner.sh && /tmp/setup-modelrunner.sh && rm /tmp/setup-modelrunner.sh

# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner

USER modelrunner

# --- Final image for vLLM ---
FROM vllm/vllm-openai:${VLLM_VERSION} AS final-vllm

WORKDIR /app

# Setup modelrunner user, directories and permissions
COPY --from=setup-files /tmp/setup-modelrunner.sh /tmp/setup-modelrunner.sh
RUN chmod +x /tmp/setup-modelrunner.sh && /tmp/setup-modelrunner.sh && rm /tmp/setup-modelrunner.sh

# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner

USER modelrunner

# --- Select final stage based on backend ---
FROM final-${BACKEND} AS final

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
