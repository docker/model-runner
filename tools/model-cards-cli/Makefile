.PHONY: all build test clean lint run

# Import env file if it exists
-include .env

# Build variables
BINARY_NAME=model-cards-cli
VERSION?=0.1.0

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

# Use linker flags to provide version/build information
LDFLAGS=-ldflags "-X main.Version=${VERSION}"

all: clean lint build

build: lint
	@echo "Building ${BINARY_NAME}..."
	@mkdir -p ${GOBIN}
	@cd $(GOBASE) && go build ${LDFLAGS} -o ${GOBIN}/${BINARY_NAME}

clean:
	@echo "Cleaning..."
	@rm -rf ${GOBIN}
	@rm -f ${BINARY_NAME}

lint:
	@echo "Running linters..."
	@gofmt -s -l . | tee /dev/stderr | xargs -r false
	@go vet ./...

run:
	@echo "Running ${BINARY_NAME} for all models..."
	@${GOBIN}/${BINARY_NAME} update

run-single:
	@if [ -z "$(MODEL)" ]; then \
		echo "Error: MODEL parameter is required. Usage: make run-single MODEL=<model-file.md>"; \
		exit 1; \
	fi
	@echo "Running ${BINARY_NAME} for single model: $(MODEL)..."
	@${GOBIN}/${BINARY_NAME} update --model-file=$(MODEL)

# Define variables for inspect command
REPOSITORY ?=
TAG ?=
OPTIONS ?=

inspect:
	@if [ -z "$(REPOSITORY)" ]; then \
		echo "Error: REPOSITORY parameter is required. Usage: make inspect REPOSITORY=<repository> [TAG=<tag>] [OPTIONS=<options>]"; \
		exit 1; \
	fi
	@echo "Inspecting model: $(REPOSITORY)$(if $(TAG),:$(TAG),)"
	@${GOBIN}/${BINARY_NAME} inspect-model $(if $(TAG),--tag=$(TAG),) $(if $(OPTIONS),$(OPTIONS),) $(REPOSITORY)

# This is needed to handle the arguments properly
%:
	@:

help:
	@echo "Available targets:"
	@echo "  all              - Clean, build, and test"
	@echo "  build            - Build the binary"
	@echo "  clean            - Clean build artifacts"
	@echo "  lint             - Run linters"
	@echo "  run              - Run the binary to update all model files"
	@echo "  run-single       - Run the binary to update a single model file (Usage: make run-single MODEL=<model-file.md>)"
	@echo "  inspect          - Inspect a model repository (Usage: make inspect REPOSITORY=<repository> [TAG=<tag>] [OPTIONS=<options>])"
	@echo "                     Example: make inspect REPOSITORY=ai/smollm2"
	@echo "                     Example: make inspect REPOSITORY=ai/smollm2 TAG=360M-Q4_K_M"
	@echo "                     Example: make inspect REPOSITORY=ai/smollm2 OPTIONS=\"--all\""
	@echo "  help             - Show this help message"
