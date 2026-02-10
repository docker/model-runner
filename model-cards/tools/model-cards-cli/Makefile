.PHONY: all build test clean lint

# Import env file if it exists
-include .env

# Build variables
BINARY_NAME=model-cards-cli
VERSION?=0.1.0

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

# Namespace for update command (optional)
NAMESPACE ?=

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

# This is needed to handle the arguments properly
%:
	@:

help:
	@echo "Available targets:"
	@echo "  all              - Clean, build, and test"
	@echo "  build            - Build the binary"
	@echo "  clean            - Clean build artifacts"
	@echo "  lint             - Run linters"
	@echo "  help             - Show this help message"
