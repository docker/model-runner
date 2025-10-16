# Docker Model Runner

Docker Model Runner (DMR) makes it easy to manage, run, and deploy AI models using Docker. Designed for developers, Docker Model Runner streamlines the process of pulling, running, and serving large language models (LLMs) and other AI models directly from Docker Hub or any OCI-compliant registry.

## Overview

This package supports the Docker Model Runner in Docker Desktop and Docker Engine.

### Installation

For macOS and Windows, install Docker Desktop:

https://docs.docker.com/desktop/

For Linux, install Docker Engine:

```bash
curl -fsSL https://get.docker.com | sudo bash
```

Docker Model Runner is included in the above tools.

For more details refer to:

https://docs.docker.com/ai/model-runner/get-started/

### Prerequisites

Before building from source, ensure you have the following installed:

- **Go 1.24+** - Required for building both model-runner and model-cli
- **Git** - For cloning repositories
- **Make** - For using the provided Makefiles
- **Docker** (optional) - For building and running containerized versions
- **CGO dependencies** - Required for model-runner's GPU support:
  - On macOS: Xcode Command Line Tools (`xcode-select --install`)
  - On Linux: gcc/g++ and development headers
  - On Windows: MinGW-w64 or Visual Studio Build Tools

### Building the Complete Stack

#### Step 1: Clone and Build model-runner (Server/Daemon)

```bash
# Clone the model-runner repository
git clone https://github.com/docker/model-runner.git
cd model-runner

# Build the model-runner binary
make build

# Or build with specific backend arguments
make run LLAMA_ARGS="--verbose --jinja -ngl 999 --ctx-size 2048"

# Run tests to verify the build
make test
```

The `model-runner` binary will be created in the current directory. This is the backend server that manages models.

#### Step 2: Build model-cli (Client)

```bash
# From the root directory, navigate to the model-cli directory
cd cmd/cli

# Build the CLI binary
make build

# The binary will be named 'model-cli'
# Optionally, install it as a Docker CLI plugin
make install  # This will link it to ~/.docker/cli-plugins/docker-model
```

### Testing the Complete Stack End-to-End

> **Note:** We use port 13434 in these examples to avoid conflicts with Docker Desktop's built-in Model Runner, which typically runs on port 12434.

#### Option 1: Local Development (Recommended for Contributors)

1. **Start model-runner in one terminal:**
```bash
cd model-runner
MODEL_RUNNER_PORT=13434 ./model-runner
# The server will start on port 13434
```

2. **Use model-cli in another terminal:**
```bash
cd cmd/cli
# List available models (connecting to port 13434)
MODEL_RUNNER_HOST=http://localhost:13434 ./model-cli list

# Pull and run a model
MODEL_RUNNER_HOST=http://localhost:13434 ./model-cli run ai/smollm2 "Hello, how are you?" 
```

#### Option 2: Using Docker

1. **Build and run model-runner in Docker:**
```bash
cd model-runner
make docker-build
make docker-run PORT=13434 MODELS_PATH=/path/to/models
```

2. **Connect with model-cli:**
```bash
cd cmd/cli
MODEL_RUNNER_HOST=http://localhost:13434 ./model-cli list
```

### Additional Resources

- [Model Runner Documentation](https://docs.docker.com/desktop/features/model-runner/)
- [Model CLI README](./cmd/cli/README.md)
- [Model Specification](https://github.com/docker/model-spec/blob/main/spec.md)
- [Community Slack Channel](https://app.slack.com/client/T0JK1PCN6/C09H9P5E57B)

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
make docker-build LLAMA_SERVER_VERSION=v0.0.4

# Specify all parameters
make docker-build LLAMA_SERVER_VERSION=v0.0.4 LLAMA_SERVER_VARIANT=cpu
```

Default values:
- `LLAMA_SERVER_VERSION`: latest
- `LLAMA_SERVER_VARIANT`: cpu

The binary path in the image follows this pattern: `/com.docker.llama-server.native.linux.${LLAMA_SERVER_VARIANT}.${TARGETARCH}`

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

# Get metrics
curl http://localhost:8080/metrics
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

## Metrics

The Model Runner exposes [the metrics endpoint](https://github.com/ggml-org/llama.cpp/tree/master/tools/server#get-metrics-prometheus-compatible-metrics-exporter) of llama.cpp server at the `/metrics` endpoint. This allows you to monitor model performance, request statistics, and resource usage.

### Accessing Metrics

```sh
# Get metrics in Prometheus format
curl http://localhost:8080/metrics
```

### Configuration

- **Enable metrics (default)**: Metrics are enabled by default
- **Disable metrics**: Set `DISABLE_METRICS=1` environment variable
- **Monitoring integration**: Add the endpoint to your Prometheus configuration

Check [METRICS.md](./METRICS.md) for more details.

## ollama Integration

Docker Model Runner supports running ollama as an alternative runner. This allows you to use ollama's model format and API alongside Docker Model Runner.

### Installing ollama Runner

To install the ollama runner instead of the default Docker Model Runner:

```bash
docker model install-runner --ollama
```

This will:
- Start an ollama container on port 11434 (the standard ollama port)
- Create an `ollama` volume for model storage (instead of `docker-model-runner-models`)
- Use the `ollama/ollama:latest` image

### GPU Support for ollama

ollama supports both NVIDIA CUDA and AMD ROCm GPUs:

```bash
# For AMD GPUs with ROCm
docker model install-runner --ollama --gpu rocm

# For NVIDIA GPUs (auto-detected)
docker model install-runner --ollama --gpu auto
```

The `--gpu rocm` flag will use the `ollama/ollama:rocm` image which includes ROCm support.

### Managing ollama Runner

All standard runner commands support the `--ollama` flag:

```bash
# Start ollama runner
docker model start-runner --ollama

# Stop ollama runner
docker model stop-runner --ollama

# Restart ollama runner
docker model restart-runner --ollama

# Reinstall ollama runner
docker model reinstall-runner --ollama

# Uninstall ollama runner (optionally remove images)
docker model uninstall-runner --ollama --images
```

### Using ollama Models

Models from `ollama.com` are automatically detected:

```bash
# Pull an ollama model (will auto-start ollama runner)
docker model pull ollama.com/library/smollm:135m

# Run an ollama model
docker model run ollama.com/library/smollm:135m
```

**Note:** Direct ollama API integration is currently in development. For now, you can interact with ollama models using:

```bash
# Pull a model
docker exec docker-ollama-runner ollama pull smollm:135m

# Run a model interactively
docker exec -it docker-ollama-runner ollama run smollm:135m

# List ollama models
docker exec docker-ollama-runner ollama list
```

### ollama vs Docker Model Runner

Key differences:

- **Port**: ollama uses port 11434, Docker Model Runner uses 12434 (Docker Engine) or 12435 (Cloud)
- **Volume**: ollama uses the `ollama` volume, Docker Model Runner uses `docker-model-runner-models`
- **Image**: ollama uses `ollama/ollama:latest` or `ollama/ollama:rocm`
- **Platform support**: ollama controller containers are used on all platforms (including Docker Desktop)

##  Kubernetes

Experimental support for running in Kubernetes is available
in the form of [a Helm chart and static YAML](charts/docker-model-runner/README.md).

If you are interested in a specific Kubernetes use-case, please start a
discussion on the issue tracker.

## Community

For general questions and discussion, please use [Docker Model Runner's Slack channel](https://app.slack.com/client/T0JK1PCN6/C09H9P5E57B).

For discussions about issues/bugs and features, you can use GitHub Issues and Pull requests.
