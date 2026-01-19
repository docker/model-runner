# Project variables
APP_NAME := model-runner
GO_VERSION := 1.24.3
LLAMA_SERVER_VERSION := latest
LLAMA_SERVER_VARIANT := cpu
BASE_IMAGE := ubuntu:24.04
VLLM_BASE_IMAGE := nvidia/cuda:13.0.2-runtime-ubuntu24.04
DOCKER_IMAGE := docker/model-runner:latest
DOCKER_IMAGE_VLLM := docker/model-runner:latest-vllm-cuda
DOCKER_IMAGE_SGLANG := docker/model-runner:latest-sglang
DOCKER_IMAGE_DIFFUSERS := docker/model-runner:latest-diffusers
DOCKER_TARGET ?= final-llamacpp
PORT := 8080
MODELS_PATH := $(shell pwd)/models-store
LLAMA_ARGS ?=
DOCKER_BUILD_ARGS := \
	--load \
	--platform linux/$(shell docker version --format '{{.Server.Arch}}') \
	--build-arg LLAMA_SERVER_VERSION=$(LLAMA_SERVER_VERSION) \
	--build-arg LLAMA_SERVER_VARIANT=$(LLAMA_SERVER_VARIANT) \
	--build-arg BASE_IMAGE=$(BASE_IMAGE) \
	--target $(DOCKER_TARGET) \
	-t $(DOCKER_IMAGE)

# Test configuration
BUILD_DMR ?= 1

# Main targets
.PHONY: build run clean test integration-tests test-docker-ce-installation docker-build docker-build-multiplatform docker-run docker-build-vllm docker-run-vllm docker-build-sglang docker-run-sglang docker-run-impl help validate lint docker-build-diffusers docker-run-diffusers
# Default target
.DEFAULT_GOAL := build

# Build the Go application
build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(APP_NAME) .

# Run the application locally
run: build
	@LLAMACPP_BIN="llamacpp/install/bin"; \
	if [ "$(LOCAL_LLAMA)" = "1" ]; then \
		echo "Using local llama.cpp build from $${LLAMACPP_BIN}"; \
		export LLAMA_SERVER_PATH="$$(pwd)/$${LLAMACPP_BIN}"; \
	fi; \
	LLAMA_ARGS="$(LLAMA_ARGS)" ./$(APP_NAME)

# Clean build artifacts
clean:
	rm -f $(APP_NAME)
	rm -f model-runner.sock
	rm -rf $(MODELS_PATH)

# Run tests
test:
	go test -v ./...

integration-tests:
	@echo "Running integration tests..."
	@echo "Note: This requires Docker to be running"
	@echo "Checking test naming conventions..."
	@INVALID_TESTS=$$(grep "^func Test" cmd/cli/commands/integration_test.go | grep -v "^func TestIntegration"); \
	if [ -n "$$INVALID_TESTS" ]; then \
		echo "Error: Found test functions that don't start with 'TestIntegration':"; \
		echo "$$INVALID_TESTS" | sed 's/func \([^(]*\).*/\1/'; \
		exit 1; \
	fi
	@BUILD_DMR=$(BUILD_DMR) go test -v -race -count=1 -tags=integration -run "^TestIntegration" -timeout=5m ./cmd/cli/commands
	@echo "Integration tests completed!"

test-docker-ce-installation:
	@echo "Testing Docker CE installation..."
	@echo "Note: This requires Docker to be running"
	BASE_IMAGE=$(BASE_IMAGE) scripts/test-docker-ce-installation.sh

validate:
	find . -type f -name "*.sh" | grep -v "pkg/go-containerregistry\|llamacpp/native/vendor" | xargs shellcheck
	@echo "✓ Shellcheck validation passed!"

lint:
	@echo "Running golangci-lint on root module..."
	golangci-lint run ./...
	@echo "Running golangci-lint on cmd/cli module..."
	cd cmd/cli && golangci-lint run ./...
	@echo "✓ Go linting passed!"

# Build Docker image
docker-build:
	docker buildx build $(DOCKER_BUILD_ARGS) .

# Build multi-platform Docker image
docker-build-multiplatform:
	docker buildx build --platform linux/amd64,linux/arm64 $(DOCKER_BUILD_ARGS) .

# Run in Docker container with TCP port access and mounted model storage
docker-run: docker-build
	@$(MAKE) -s docker-run-impl

# Build vLLM Docker image
docker-build-vllm:
	@$(MAKE) docker-build \
		DOCKER_TARGET=final-vllm \
		DOCKER_IMAGE=$(DOCKER_IMAGE_VLLM) \
		LLAMA_SERVER_VARIANT=cuda \
		BASE_IMAGE=$(VLLM_BASE_IMAGE)

# Run vLLM Docker container with TCP port access and mounted model storage
docker-run-vllm: docker-build-vllm
	@$(MAKE) -s docker-run-impl DOCKER_IMAGE=$(DOCKER_IMAGE_VLLM)

# Build SGLang Docker image
docker-build-sglang:
	@$(MAKE) docker-build \
		DOCKER_TARGET=final-sglang \
		DOCKER_IMAGE=$(DOCKER_IMAGE_SGLANG) \
		LLAMA_SERVER_VARIANT=cuda \
		BASE_IMAGE=$(VLLM_BASE_IMAGE)

# Run SGLang Docker container with TCP port access and mounted model storage
docker-run-sglang: docker-build-sglang
	@$(MAKE) -s docker-run-impl DOCKER_IMAGE=$(DOCKER_IMAGE_SGLANG)

# Build Diffusers Docker image
docker-build-diffusers:
	@$(MAKE) docker-build \
		DOCKER_TARGET=final-diffusers \
		DOCKER_IMAGE=$(DOCKER_IMAGE_DIFFUSERS)

# Run Diffusers Docker container with TCP port access and mounted model storage
docker-run-diffusers: docker-build-diffusers
	@$(MAKE) -s docker-run-impl DOCKER_IMAGE=$(DOCKER_IMAGE_DIFFUSERS)

# Common implementation for running Docker container
docker-run-impl:
	@echo ""
	@echo "Starting service on port $(PORT) with model storage at $(MODELS_PATH)..."
	@echo "Service will be available at: http://localhost:$(PORT)"
	@echo "Example usage: curl http://localhost:$(PORT)/models"
	@echo ""
	PORT="$(PORT)" \
	MODELS_PATH="$(MODELS_PATH)" \
	DOCKER_IMAGE="$(DOCKER_IMAGE)" \
	LLAMA_ARGS="$(LLAMA_ARGS)" \
	DMR_ORIGINS="$(DMR_ORIGINS)" \
	DO_NOT_TRACK="${DO_NOT_TRACK}" \
	DEBUG="${DEBUG}" \
	scripts/docker-run.sh

# Show help
help:
	@echo "Available targets:"
	@echo "  build				- Build the Go application"
	@echo "  run				- Run the application locally"
	@echo "  clean				- Clean build artifacts"
	@echo "  test				- Run tests"
	@echo "  integration-tests		- Run integration tests"
	@echo "  test-docker-ce-installation	- Test Docker CE installation with CLI plugin"
	@echo "  validate			- Run shellcheck validation"
	@echo "  lint				- Run Go linting with golangci-lint"
	@echo "  docker-build			- Build Docker image for current platform"
	@echo "  docker-build-multiplatform	- Build Docker image for multiple platforms"
	@echo "  docker-run			- Run in Docker container with TCP port access and mounted model storage"
	@echo "  docker-build-vllm		- Build vLLM Docker image"
	@echo "  docker-run-vllm		- Run vLLM Docker container"
	@echo "  docker-build-sglang		- Build SGLang Docker image"
	@echo "  docker-run-sglang		- Run SGLang Docker container"
	@echo "  docker-build-diffusers	- Build Diffusers Docker image"
	@echo "  docker-run-diffusers		- Run Diffusers Docker container"
	@echo "  help				- Show this help message"
	@echo ""
	@echo "Backend configuration options:"
	@echo "  LLAMA_ARGS    - Arguments for llama.cpp (e.g., \"--verbose --jinja -ngl 999 --ctx-size 2048\")"
	@echo "  LOCAL_LLAMA   - Use local llama.cpp build from llamacpp/install/bin (set to 1 to enable)"
	@echo ""
	@echo "Example usage:"
	@echo "  make run LLAMA_ARGS=\"--verbose --jinja -ngl 999 --ctx-size 2048\""
	@echo "  make run LOCAL_LLAMA=1"
	@echo "  make docker-run LLAMA_ARGS=\"--verbose --jinja -ngl 999 --threads 4 --ctx-size 2048\""
