# Project variables
APP_NAME := model-runner
GO_VERSION := 1.25.6
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
.PHONY: build run clean test integration-tests test-docker-ce-installation docker-build docker-build-multiplatform docker-run docker-build-vllm docker-run-vllm docker-build-sglang docker-run-sglang docker-run-impl help validate lint docker-build-diffusers docker-run-diffusers vllm-metal-build vllm-metal-install vllm-metal-clean
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

# vllm-metal (macOS ARM64 only, requires Python 3.12 for wheel compatibility)
VLLM_METAL_VERSION ?= v0.1.0
VLLM_METAL_INSTALL_DIR := $(HOME)/.model-runner/vllm-metal
VLLM_METAL_TARBALL := vllm-metal-macos-arm64-$(VLLM_METAL_VERSION).tar.gz

vllm-metal-build:
	@echo "Building vllm-metal tarball..."
	@scripts/build-vllm-metal-tarball.sh $(VLLM_METAL_VERSION) .
	@echo "Tarball created: $(VLLM_METAL_TARBALL)"

vllm-metal-install:
	@if [ ! -f "$(VLLM_METAL_TARBALL)" ]; then \
		echo "Error: $(VLLM_METAL_TARBALL) not found. Run 'make vllm-metal-build' first."; \
		exit 1; \
	fi
	@echo "Installing vllm-metal to $(VLLM_METAL_INSTALL_DIR)..."
	@PYTHON_BIN=""; \
	if command -v python3.12 >/dev/null 2>&1; then \
		PYTHON_BIN="python3.12"; \
	elif command -v python3 >/dev/null 2>&1; then \
		version=$$(python3 --version 2>&1 | grep -oE '[0-9]+\.[0-9]+'); \
		if [ "$$version" = "3.12" ]; then \
			PYTHON_BIN="python3"; \
		fi; \
	fi; \
	if [ -z "$$PYTHON_BIN" ]; then \
		echo "Error: Python 3.12 required (vllm-metal wheel is built for cp312)"; \
		echo "Install with: brew install python@3.12"; \
		exit 1; \
	fi; \
	echo "Using Python 3.12 from $$(which $$PYTHON_BIN)"; \
	rm -rf "$(VLLM_METAL_INSTALL_DIR)"; \
	$$PYTHON_BIN -m venv "$(VLLM_METAL_INSTALL_DIR)"; \
	SITE_PACKAGES="$(VLLM_METAL_INSTALL_DIR)/lib/python3.12/site-packages"; \
	mkdir -p "$$SITE_PACKAGES"; \
	tar -xzf "$(VLLM_METAL_TARBALL)" -C "$$SITE_PACKAGES"; \
	echo "vllm-metal installed successfully!"
	@echo "Test with: $(VLLM_METAL_INSTALL_DIR)/bin/python3 -c 'import vllm_metal; print(vllm_metal.__version__)'"

vllm-metal-clean:
	@echo "Removing vllm-metal installation and build artifacts..."
	rm -rf "$(VLLM_METAL_INSTALL_DIR)"
	rm -f $(VLLM_METAL_TARBALL)
	@echo "vllm-metal cleaned!"

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
	@echo "  vllm-metal-build		- Build vllm-metal tarball locally (macOS ARM64)"
	@echo "  vllm-metal-install		- Install vllm-metal from local tarball"
	@echo "  vllm-metal-clean		- Clean vllm-metal installation and tarball"
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
	@echo ""
	@echo "vllm-metal (macOS ARM64 only):"
	@echo "  make vllm-metal-build && make vllm-metal-install && make run"
