# Project variables
APP_NAME := model-runner
GO_VERSION := 1.23.7
LLAMA_SERVER_VERSION := v0.0.4-rc2-cpu
TARGET_OS := linux
TARGET_ARCH := amd64
ACCEL := cpu
DOCKER_IMAGE := go-model-runner:latest
LLAMA_BINARY := /com.docker.llama-server.native.$(TARGET_OS).$(ACCEL).$(TARGET_ARCH)
PORT := 8080
MODELS_PATH := $(shell pwd)/models

# Main targets
.PHONY: build run clean test docker-build docker-run help

# Default target
.DEFAULT_GOAL := help

# Build the Go application
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(APP_NAME) ./main.go

# Run the application locally
run: build
	./$(APP_NAME)

# Clean build artifacts
clean:
	rm -f $(APP_NAME)
	rm -f model-runner.sock
	rm -rf $(MODELS_PATH)

# Run tests
test:
	go test -v ./...

# Build Docker image
docker-build:
	docker build --platform linux/amd64 \
		--build-arg LLAMA_SERVER_VERSION=$(LLAMA_SERVER_VERSION) \
		--build-arg LLAMA_BINARY_PATH=$(LLAMA_BINARY) \
		-t $(DOCKER_IMAGE) .

# Run in Docker container with TCP port access and mounted model storage
docker-run: docker-build
	@echo ""
	@echo "Starting service on port $(PORT) with model storage at $(MODELS_PATH)..."
	@echo "Service will be available at: http://localhost:$(PORT)"
	@echo "Example usage: curl http://localhost:$(PORT)/models"
	@echo ""
	mkdir -p $(MODELS_PATH)
	docker run --rm \
		-p $(PORT):$(PORT) \
		-v "$(MODELS_PATH):/models" \
		-e MODEL_RUNNER_PORT=$(PORT) \
		-e LLAMA_SERVER_PATH=/app/bin \
		-e MODELS_PATH=/models \
		$(DOCKER_IMAGE)

# Show help
help:
	@echo "Available targets:"
	@echo "  build          	- Build the Go application"
	@echo "  run            	- Run the application locally"
	@echo "  clean          	- Clean build artifacts"
	@echo "  test           	- Run tests"
	@echo "  docker-build   	- Build Docker image"
	@echo "  docker-run     	- Run in Docker container with TCP port access and mounted model storage"
	@echo "  help           	- Show this help message"
