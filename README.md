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
- `help` - Show available targets

### Examples

```sh
# Build the application
make build

# Run the application locally
make run

# Show all available targets
make help
```

## API Examples

The Model Runner exposes a REST API over a Unix socket. You can interact with it using curl commands with the `--unix-socket` option.

### Listing Models

To list all available models:

```sh
curl --unix-socket model-runner.sock localhost/models
```

### Creating a Model

To create a new model:

```sh
curl --unix-socket model-runner.sock localhost/models/create -X POST -d '{"from": "ai/smollm2"}'
```

### Getting Model Information

To get information about a specific model:

```sh
curl --unix-socket model-runner.sock localhost/models/ai/smollm2
```

### Chatting with a Model

To chat with a model, you can send a POST request to the model's chat endpoint:

```sh
curl --unix-socket model-runner.sock localhost/engines/llama.cpp/v1/chat/completions -X POST -d '{
  "model": "ai/smollm2",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello, how are you?"}
  ]
}'
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

### Deleting a model
To delete a model from the server, send a DELETE request to the model's endpoint:

```sh
curl --unix-socket model-runner.sock localhost/models/ai/smollm2 -X DELETE
```
