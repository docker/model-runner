# Project variables
APP_NAME := model-runner
GO_VERSION := 1.23.7

# Main targets
.PHONY: build run clean help

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

# Show help
help:
	@echo "Available targets:"
	@echo "  build          - Build the Go application"
	@echo "  run            - Run the application locally"
	@echo "  clean          - Clean build artifacts"
	@echo "  help           - Show this help message"
