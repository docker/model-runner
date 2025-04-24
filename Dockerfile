# syntax=docker/dockerfile:1

ARG GO_VERSION=1.23.7
ARG LLAMA_SERVER_VERSION=latest
ARG LLAMA_BINARY_PATH=/com.docker.llama-server.native.linux.cpu.amd64

FROM golang:${GO_VERSION}-alpine AS builder

# Install git for go mod download if needed
RUN apk add --no-cache git

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
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o model-runner ./main.go

# --- Get llama.cpp binary ---
FROM docker/docker-model-backend-llamacpp:${LLAMA_SERVER_VERSION} AS llama-server

# --- Final image ---
FROM alpine:latest AS final

# Create non-root user
RUN addgroup -S modelrunner && adduser -S modelrunner -G modelrunner

# Install ca-certificates for HTTPS
RUN --mount=type=cache,target=/var/cache/apk \
    apk add --no-cache ca-certificates

WORKDIR /app

# Create directories for the socket file and llama.cpp binary, and set proper permissions
RUN mkdir -p /var/run/model-runner /app/bin && \
    chown -R modelrunner:modelrunner /var/run/model-runner /app/bin

# Copy the built binary from builder
COPY --from=builder /app/model-runner /app/model-runner

# Copy the llama.cpp binary from the llama-server stage
ARG LLAMA_BINARY_PATH
COPY --from=llama-server ${LLAMA_BINARY_PATH} /app/bin/com.docker.llama-server

USER modelrunner

# Set the environment variable for the socket path and LLaMA server binary path
ENV MODEL_RUNNER_SOCK=/var/run/model-runner/model-runner.sock
ENV LLAMA_SERVER_PATH=/app/bin

ENTRYPOINT ["/app/model-runner"]
