# Project variables
APP_NAME := model-runner
GO_VERSION := 1.23.7
LLAMA_SERVER_VERSION := latest
LLAMA_SERVER_VARIANT := cpu
BASE_IMAGE := ubuntu:24.04
DOCKER_IMAGE := docker/model-runner:latest
PORT := 8080
MODELS_PATH := $(shell pwd)/models-store
LLAMA_ARGS ?=

# Main targets
.PHONY: build run clean test docker-build docker-run help

# Default target
.DEFAULT_GOAL := help

# Build the Go application
build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(APP_NAME) ./main.go

# Run the application locally
run: build
	LLAMA_ARGS="$(LLAMA_ARGS)" \
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
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg LLAMA_SERVER_VERSION=$(LLAMA_SERVER_VERSION) \
		--build-arg LLAMA_SERVER_VARIANT=$(LLAMA_SERVER_VARIANT) \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
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
		-e LLAMA_ARGS="$(LLAMA_ARGS)" \
		-e DMR_ORIGINS="$(DMR_ORIGINS)" \
		-e DO_NOT_TRACK=${DO_NOT_TRACK} \
		-e DEBUG=${DEBUG} \
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
	@echo ""
	@echo "Backend configuration options:"
	@echo "  LLAMA_ARGS    - Arguments for llama.cpp (e.g., \"--verbose --jinja -ngl 100 --ctx-size 2048\")"
	@echo ""
	@echo "Example usage:"
	@echo "  make run LLAMA_ARGS=\"--verbose --jinja -ngl 100 --ctx-size 2048\""
	@echo "  make docker-run LLAMA_ARGS=\"--verbose --jinja -ngl 100 --threads 4 --ctx-size 2048\""
