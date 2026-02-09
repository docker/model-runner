---
name: docker-model-runner
description: Skills for using Docker Model Runner to run local LLM inference
---

# Docker Model Runner

Docker Model Runner (DMR) makes it easy to run AI models locally using Docker. This skill helps you effectively use Docker Model Runner for local LLM inference in your development workflow.

## Workflow

When helping users with local LLM inference using Docker Model Runner:

1. **Check if Docker Model Runner is available** by running `docker model version`

2. **List available models** with `docker model list` to see what's already pulled

3. **Search for models** on Docker Hub or HuggingFace:
   - `docker model search <query>` to find models
   - Popular models include: `ai/gemma3`, `ai/llama3.2`, `ai/smollm2`, `ai/qwen3`

4. **Pull models** before running: `docker model pull <model>`

5. **Run models** for inference:
   - One-time prompt: `docker model run ai/smollm2 "Your prompt here"`
   - Interactive chat: `docker model run ai/smollm2`
   - Pre-load model: `docker model run --detach ai/smollm2`

6. **Use the OpenAI-compatible API** for programmatic access:
   - Endpoint: `http://localhost:12434/engines/llama.cpp/v1/chat/completions`
   - This is compatible with OpenAI client libraries

## API Usage

Docker Model Runner exposes an OpenAI-compatible REST API:

```bash
# Chat completions
curl http://localhost:12434/engines/llama.cpp/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ai/smollm2",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Hello!"}
    ]
  }'
```

For Python with the OpenAI library:

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:12434/engines/llama.cpp/v1",
    api_key="not-needed"  # API key not required for local inference
)

response = client.chat.completions.create(
    model="ai/smollm2",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

## Key Commands

| Command | Description |
|---------|-------------|
| `docker model run <model> [prompt]` | Run a model with optional prompt |
| `docker model pull <model>` | Pull a model from registry |
| `docker model list` | List downloaded models |
| `docker model search <query>` | Search for models |
| `docker model ps` | Show running models |
| `docker model rm <model>` | Remove a model |
| `docker model inspect <model>` | Show model details |

## Best Practices

- Use smaller models (like `ai/smollm2`) for faster responses during development
- Pre-load models with `--detach` for better performance in scripts
- Models stay loaded until another model is requested or timeout (5 min)
- Use the OpenAI-compatible API for integration with existing tools

## References

See [references/docker-model-guide.md](references/docker-model-guide.md) for detailed documentation.
