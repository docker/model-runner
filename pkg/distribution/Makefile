.PHONY: all build test clean lint run

# Import env file if it exists
-include .env

# Build variables
BINARY_NAME=model-distribution-tool
VERSION?=0.1.0

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

# Run configuration
SOURCE?=
TAG?=
STORE_PATH?=./model-store

# Use linker flags to provide version/build information
LDFLAGS=-ldflags "-X main.Version=${VERSION}"

all: clean lint build test

build:
	@echo "Building ${BINARY_NAME}..."
	@mkdir -p ${GOBIN}
	@go build ${LDFLAGS} -o ${GOBIN}/${BINARY_NAME} ../../cmd/mdltool/

test:
	@echo "Running unit tests..."
	@go test -race ./... ../../cmd/mdltool/...

clean:
	@echo "Cleaning..."
	@rm -rf ${GOBIN}
	@rm -f ${BINARY_NAME}
	@rm -f *.test
	@rm -rf test/artifacts/*

lint:
	@echo "Running linters..."
	@gofmt -s -l . | tee /dev/stderr | xargs -r false
	@go vet ./...

run-pull:
	@echo "Pulling model from ${TAG}..."
	@${GOBIN}/${BINARY_NAME} --store-path ${STORE_PATH} pull ${TAG}

run-package:
	@echo "Pushing model ${SOURCE} to ${TAG}..."
	@${GOBIN}/${BINARY_NAME} --store-path ${STORE_PATH} package ${SOURCE} ${TAG} ${LICENSE:+--license ${LICENSE}}

run-list:
	@echo "Listing models..."
	@${GOBIN}/${BINARY_NAME} --store-path ${STORE_PATH} list

run-get:
	@echo "Getting model ${TAG}..."
	@${GOBIN}/${BINARY_NAME} --store-path ${STORE_PATH} get ${TAG}

run-get-path:
	@echo "Getting path for model ${TAG}..."
	@${GOBIN}/${BINARY_NAME} --store-path ${STORE_PATH} get-path ${TAG}

run-rm:
	@echo "Removing model ${TAG}..."
	@${GOBIN}/${BINARY_NAME} --store-path ${STORE_PATH} rm ${TAG}

run-tag:
	@echo "Tagging model ${SOURCE} as ${TAG}..."
	@${GOBIN}/${BINARY_NAME} --store-path ${STORE_PATH} tag ${SOURCE} ${TAG}

help:
	@echo "Available targets:"
	@echo "  all              - Clean, build, and test"
	@echo "  build            - Build the binary"
	@echo "  test             - Run unit tests"
	@echo "  clean            - Clean build artifacts"
	@echo "  run-pull         - Pull a model (TAG=registry/model:tag)"
	@echo "  run-package      - Package and push a model (SOURCE=path/to/model.gguf TAG=registry/model:tag LICENSE=path/to/license.txt)"
	@echo "  run-list         - List all models"
	@echo "  run-get          - Get model info (TAG=registry/model:tag)"
	@echo "  run-get-path     - Get model path (TAG=registry/model:tag)"
	@echo "  run-rm           - Remove a model (TAG=registry/model:tag)"
	@echo "  run-tag          - Tag a model (SOURCE=registry/model:tag TAG=registry/model:newtag)"
