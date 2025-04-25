# Model Runner

The backend library for the
[Docker Model Runner](https://docs.docker.com/desktop/features/model-runner/).

## Overview

> [!NOTE]
> This package is still under rapid development and its APIs should not be
> considered stable.

This package supports the Docker Model Runner in Docker Desktop (in conjunction
with [Model Distribution](https://github.com/docker/model-distribution) and the
[Model CLI](https://github.com/docker/model-cli)). It includes a `main.go` that
mimics its integration with Docker Desktop and allows the package to be run in a
standalone mode.

## Using the Makefile

This project includes a Makefile to simplify common development tasks. It requires Docker Desktop >= 4.41.0 
The Makefile provides the following targets:

- `build` - Build the Go application
- `run` - Run the application locally
- `clean` - Clean build artifacts
- `test` - Run tests
- `docker-build` - Build the Docker image
- `docker-run` - Run the application in a Docker container with TCP port access and mounted model storage
- `help` - Show available targets

### Running in Docker

The application can be run in Docker with the following features enabled by default:
- TCP port access (default port 8080)
- Persistent model storage in a local `models` directory

```sh
# Run with default settings
make docker-run

# Customize port and model storage location
make docker-run PORT=3000 MODELS_PATH=/path/to/your/models
```

This will:
- Create a `models` directory in your current working directory (or use the specified path)
- Mount this directory into the container
- Start the service on port 8080 (or the specified port)
- All models downloaded will be stored in the host's `models` directory and will persist between container runs

### llama.cpp integration

The Docker image includes the llama.cpp server binary from the `docker/docker-model-backend-llamacpp` image. You can specify the version of the image to use by setting the `LLAMA_SERVER_VERSION` variable. Additionally, you can configure the target OS, architecture, and acceleration type:

```sh
# Build with a specific llama.cpp server version
LLAMA_SERVER_VERSION=v0.0.4-rc2-cpu make docker-build

# Specify all parameters
LLAMA_SERVER_VERSION=v0.0.4-rc2-cpu TARGET_OS=linux TARGET_ARCH=amd64 ACCEL=cpu make docker-build
```

Default values:
- `LLAMA_SERVER_VERSION`: v0.0.4-rc2-cpu
- `TARGETOS`: linux
- `TARGETARCH`: amd64
- `ACCEL`: cpu

The binary path in the image follows this pattern: `/com.docker.llama-server.native.${TARGETOS}.${ACCEL}.${TARGETARCH}`

## API Examples

The Model Runner exposes a REST API that can be accessed via TCP port. You can interact with it using curl commands.

### Using the API

When running with `docker-run`, you can use regular HTTP requests:

```sh
# List all available models
curl http://localhost:8080/models

# Create a new model
curl http://localhost:8080/models/create -X POST -d '{"from": "ai/smollm2"}'

# Get information about a specific model
curl http://localhost:8080/models/ai/smollm2

# Chat with a model
curl http://localhost:8080/engines/llama.cpp/v1/chat/completions -X POST -d '{
  "model": "ai/smollm2",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello, how are you?"}
  ]
}'

# Delete a model
curl http://localhost:8080/models/ai/smollm2 -X DELETE
```

The response will contain the model's reply:

```json
{
  "id": "chat-12345",
  "object": "chat.completion",
  "created": 1682456789,
  "model": "ai/smollm2",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "I'm doing well, thank you for asking! How can I assist you today?"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 24,
    "completion_tokens": 16,
    "total_tokens": 40
  }
}
```
