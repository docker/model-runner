# Docker Model Runner
<img width="2071" height="269" alt="docker-model-runner-white" src="https://github.com/user-attachments/assets/f3b2ed27-ea0e-47d5-8ed0-82ee338f3eae" />

Docker Model Runner (DMR) makes it easy to manage, run, and deploy AI models using Docker. Designed for developers, Docker Model Runner streamlines the process of pulling, running, and serving large language models (LLMs) and other AI models directly from Docker Hub or any OCI-compliant registry.

## Overview

This package supports the Docker Model Runner in Docker Desktop and Docker Engine.

### Installation

### Docker Desktop (macOS and Windows)

For macOS and Windows, install Docker Desktop:

https://docs.docker.com/desktop/

Docker Model Runner is included in Docker Desktop.

### Docker Engine (Linux)

For Linux, install Docker Engine from the official Docker repository:

```bash
curl -fsSL https://get.docker.com | sudo bash
sudo usermod -aG docker $USER # give user permission to access docker daemon, relogin to take effect
```

Docker Model Runner is included in Docker Engine when installed from Docker's official repositories.

### Verifying Your Installation

To verify that Docker Model Runner is available:

```bash
# Check if the Docker CLI plugin is available
docker model --help

# Check Docker version
docker version

# Check Docker Model Runner version
docker model version

# Run a model to test the full setup
docker model run ai/gemma3 "Hello"
```

If `docker model` is not available, see the troubleshooting section below.

### Troubleshooting: Docker Installation Source

If you encounter errors like `Package 'docker-model-plugin' has no installation candidate` or `docker model` command is not found:

1. **Check your Docker installation source:**
   ```bash
   # Check Docker version
   docker version

   # Check Docker Model Runner version
   docker model version
   ```

   Look for the source in the output. If it shows a package from your distro, you'll need to reinstall from Docker's official repositories.

2. **Remove the distro version and install from Docker's official repository:**
   ```bash
   # Remove distro version (Ubuntu/Debian example)
   sudo apt-get purge docker docker.io containerd runc

   # Install from Docker's official repository
   curl -fsSL https://get.docker.com | sudo bash

   # Verify Docker Model Runner is available
   docker model --help
   ```

3. **For NVIDIA DGX systems:** If Docker came pre-installed, verify it's from Docker's official repositories. If not, follow the reinstallation steps above.

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
- [Community Slack Channel](https://dockercommunity.slack.com/archives/C09H9P5E57B)

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

Available variants:
- `cpu`: CPU-optimized version
- `cuda`: CUDA-accelerated version for NVIDIA GPUs
- `rocm`: ROCm-accelerated version for AMD GPUs
- `musa`: MUSA-accelerated version for MTHREADS GPUs
- `cann`: CANN-accelerated version for Ascend NPUs

The binary path in the image follows this pattern: `/com.docker.llama-server.native.linux.${LLAMA_SERVER_VARIANT}.${TARGETARCH}`

### vLLM integration

The Docker image also supports vLLM as an alternative inference backend.

#### Building the vLLM variant

To build a Docker image with vLLM support:

```sh
# Build with default settings (vLLM 0.12.0)
make docker-build DOCKER_TARGET=final-vllm BASE_IMAGE=nvidia/cuda:13.0.2-runtime-ubuntu24.04 LLAMA_SERVER_VARIANT=cuda

# Build for specific architecture
docker buildx build \
  --platform linux/amd64 \
  --target final-vllm \
  --build-arg BASE_IMAGE=nvidia/cuda:13.0.2-runtime-ubuntu24.04 \
  --build-arg LLAMA_SERVER_VARIANT=cuda \
  --build-arg VLLM_VERSION=0.12.0 \
  -t docker/model-runner:vllm .
```

#### Build Arguments

The vLLM variant supports the following build arguments:

- **VLLM_VERSION**: The vLLM version to install (default: `0.12.0`)
- **VLLM_CUDA_VERSION**: The CUDA version suffix for the wheel (default: `cu130`)
- **VLLM_PYTHON_TAG**: The Python compatibility tag (default: `cp38-abi3`, compatible with Python 3.8+)

#### Multi-Architecture Support

The vLLM variant supports both x86_64 (amd64) and aarch64 (arm64) architectures. The build process automatically selects the appropriate prebuilt wheel:

- **linux/amd64**: Uses `manylinux1_x86_64` wheels
- **linux/arm64**: Uses `manylinux2014_aarch64` wheels

To build for multiple architectures:

```sh
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --target final-vllm \
  --build-arg BASE_IMAGE=nvidia/cuda:12.9.0-runtime-ubuntu24.04 \
  --build-arg LLAMA_SERVER_VARIANT=cuda \
  -t docker/model-runner:vllm .
```

#### Updating to a New vLLM Version

To update to a new vLLM version:

```sh
docker buildx build \
  --target final-vllm \
  --build-arg VLLM_VERSION=0.11.1 \
  -t docker/model-runner:vllm-0.11.1 .
```

The vLLM wheels are sourced from the official vLLM GitHub Releases at `https://github.com/vllm-project/vllm/releases`, which provides prebuilt wheels for each release version.

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

### Features

- **Automatic GPU Detection**: Automatically configures NVIDIA GPU support if available
- **Persistent Caching**: Models are cached in `~/.cache/nim` (or `$LOCAL_NIM_CACHE` if set)
- **Interactive Chat**: Supports both single prompt and interactive chat modes
- **Container Reuse**: Existing NIM containers are reused across runs

### Example Usage

**Single prompt:**
```bash
docker model run nvcr.io/nim/google/gemma-3-1b-it:latest "Explain quantum computing"
```

**Interactive chat:**
```bash
docker model run nvcr.io/nim/google/gemma-3-1b-it:latest
> Tell me a joke
...
> /bye
```

### Configuration

- **NGC_API_KEY**: Set this environment variable to authenticate with NVIDIA's services
- **LOCAL_NIM_CACHE**: Override the default cache location (default: `~/.cache/nim`)

### Technical Details

NIM containers:
- Run on port 8000 (localhost only)
- Use 16GB shared memory by default
- Mount `~/.cache/nim` for model caching
- Support NVIDIA GPU acceleration when available

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

##  Kubernetes

Experimental support for running in Kubernetes is available
in the form of [a Helm chart and static YAML](charts/docker-model-runner/README.md).

If you are interested in a specific Kubernetes use-case, please start a
discussion on the issue tracker.

## Community

For general questions and discussion, please use [Docker Model Runner's Slack channel](https://dockercommunity.slack.com/archives/C09H9P5E57B).

For discussions about issues/bugs and features, you can use GitHub Issues and Pull requests.
