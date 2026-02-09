# Docker Model Runner Complete Guide

## Overview

Docker Model Runner (DMR) is included with Docker Desktop and Docker Engine. It provides a seamless way to run AI models locally with an OpenAI-compatible API.

## Installation

### Docker Desktop (macOS and Windows)
Docker Model Runner is included in Docker Desktop. Install from: https://docs.docker.com/desktop/

### Docker Engine (Linux)
```bash
curl -fsSL https://get.docker.com | sudo bash
sudo usermod -aG docker $USER
```

### Verify Installation
```bash
docker model --help
docker model version
```

## CLI Commands

### Model Management

```bash
# Pull a model
docker model pull ai/gemma3

# List local models
docker model list

# Remove a model
docker model rm ai/gemma3

# Inspect model details
docker model inspect ai/gemma3

# Tag a model
docker model tag ai/gemma3 my-gemma
```

### Running Models

```bash
# Interactive chat mode
docker model run ai/smollm2

# Single prompt
docker model run ai/smollm2 "Explain Docker in one sentence"

# Pre-load model for faster subsequent requests
docker model run --detach ai/smollm2

# With debug logging
docker model run --debug ai/smollm2 "Hello"
```

### Model Discovery

```bash
# Search Docker Hub
docker model search llama

# Search HuggingFace
docker model search hf.co/bartowski

# Search with specific source
docker model search --source dockerhub llama
```

### Status and Monitoring

```bash
# Check runner status
docker model status

# Show running models
docker model ps

# View logs
docker model logs

# Check disk usage
docker model df

# Active requests
docker model requests
```

### Runner Management

```bash
# Start the runner
docker model start-runner

# Stop the runner
docker model stop-runner

# Restart the runner
docker model restart-runner
```

## OpenAI-Compatible API

Docker Model Runner exposes an OpenAI-compatible API on port 12434.

### Endpoints

- `POST /engines/llama.cpp/v1/chat/completions` - Chat completions
- `POST /engines/llama.cpp/v1/completions` - Text completions
- `GET /models` - List available models
- `GET /models/{model}` - Get model info
- `POST /models/create` - Pull a model
- `DELETE /models/{model}` - Delete a model

### Chat Completions

```bash
curl http://localhost:12434/engines/llama.cpp/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is Docker?"}
    ],
    "temperature": 0.7,
    "max_tokens": 500
  }'
```

### Streaming Responses

```bash
curl http://localhost:12434/engines/llama.cpp/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "messages": [{"role": "user", "content": "Tell me a story"}],
    "stream": true
  }'
```

## Language SDK Examples

### Python (OpenAI Library)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:12434/engines/llama.cpp/v1",
    api_key="not-needed"
)

# Chat completion
response = client.chat.completions.create(
    model="ai/smollm2",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello!"}
    ]
)
print(response.choices[0].message.content)

# Streaming
stream = client.chat.completions.create(
    model="ai/smollm2",
    messages=[{"role": "user", "content": "Tell me a joke"}],
    stream=True
)
for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
```

### JavaScript/TypeScript

```javascript
import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:12434/engines/llama.cpp/v1',
  apiKey: 'not-needed'
});

const response = await client.chat.completions.create({
  model: 'ai/smollm2',
  messages: [{ role: 'user', content: 'Hello!' }]
});

console.log(response.choices[0].message.content);
```

### Go

```go
package main

import (
    "context"
    "fmt"
    "github.com/sashabaranov/go-openai"
)

func main() {
    config := openai.DefaultConfig("not-needed")
    config.BaseURL = "http://localhost:12434/engines/llama.cpp/v1"
    client := openai.NewClientWithConfig(config)

    resp, err := client.CreateChatCompletion(
        context.Background(),
        openai.ChatCompletionRequest{
            Model: "ai/smollm2",
            Messages: []openai.ChatCompletionMessage{
                {Role: "user", Content: "Hello!"},
            },
        },
    )
    if err != nil {
        panic(err)
    }
    fmt.Println(resp.Choices[0].Message.Content)
}
```

## Popular Models

| Model | Size | Description |
|-------|------|-------------|
| `ai/smollm2` | ~1GB | Fast, small model for quick tasks |
| `ai/gemma3` | ~5GB | Google's Gemma 3 model |
| `ai/llama3.2` | ~4GB | Meta's Llama 3.2 |
| `ai/qwen3` | ~4GB | Alibaba's Qwen 3 |
| `ai/phi4` | ~9GB | Microsoft's Phi-4 |
| `ai/mistral` | ~4GB | Mistral AI's base model |

## Configuration

### Environment Variables

- `MODEL_RUNNER_HOST` - Override the model runner endpoint
- `MODEL_RUNNER_PORT` - Override the default port (12434)

### Model Configuration

```bash
# Configure model parameters
docker model configure ai/smollm2 --ctx-size 4096

# View configuration
docker model inspect ai/smollm2
```

## Troubleshooting

### Model Runner Not Found
```bash
# Check if Docker is from official repositories
docker version

# Reinstall from official repository if needed
curl -fsSL https://get.docker.com | sudo bash
```

### Model Loading Issues
```bash
# Check logs for errors
docker model logs

# Restart the runner
docker model restart-runner

# Check disk space
docker model df
```

### Performance Tips

1. **Use smaller models** for development and testing
2. **Pre-load models** with `--detach` before heavy usage
3. **Monitor memory** - larger models require more RAM
4. **Use GPU acceleration** when available (automatic on supported systems)

## Resources

- [Official Documentation](https://docs.docker.com/desktop/features/model-runner/)
- [Model Specification](https://github.com/docker/model-spec/blob/main/spec.md)
- [Community Slack](https://dockercommunity.slack.com/archives/C09H9P5E57B)
