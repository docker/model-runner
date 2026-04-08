# model-cli gateway demo

Demonstrates the `model-cli gateway` command — a lightweight,
OpenAI-compatible LLM proxy that sits in front of Docker Model Runner
(and other providers) and adds routing, load balancing, retries,
fallbacks, and auth.

## Prerequisites

1. Docker Desktop with Model Runner enabled
2. The `model-cli` binary built:
   ```bash
   cd model-cli && cargo build --release
   ```
3. Models pulled:
   ```bash
   docker model pull ai/smollm2
   docker model pull ai/gemma3
   docker model pull ai/qwen3:0.6B-Q4_0
   docker model pull ai/nomic-embed-text-v1.5
   ```
4. Python `openai` package (for step 11):
   ```bash
   pip install openai
   ```

## Run the demo

```bash
./demos/gateway/demo.sh
```

The script starts the gateway on `http://localhost:4000`, runs through
every feature, then shuts the gateway down on exit.

## Files

| File | Purpose |
|------|---------|
| `config-basic.yaml`    | Single-provider config with two models and bearer-token auth |
| `config-advanced.yaml` | Multi-deployment config showing load balancing and fallbacks |
| `demo.sh`              | Full end-to-end demo script |

## What is demonstrated

| # | Feature | Config |
|---|---------|--------|
| 1 | Start gateway | basic |
| 2 | `/health` endpoint | basic |
| 3 | `/v1/models` — OpenAI-compatible model list | basic |
| 4 | Auth rejection with wrong key (HTTP 401) | basic |
| 5 | Non-streaming chat completion | basic |
| 6 | Streaming chat completion (SSE) | basic |
| 7 | Embeddings via chat model | basic |
| 8 | Switch to advanced config | advanced |
| 9 | Round-robin load balancing across two deployments | advanced |
| 10 | Dedicated embedding model (`nomic-embed-text`) | advanced |
| 11 | OpenAI Python SDK — zero code changes required | advanced |

## Config anatomy

```yaml
model_list:
  # Alias the client uses       Provider / actual model on DMR
  - model_name: fast-model
    params:
      model: docker_model_runner/ai/smollm2

  # Second entry with same alias → round-robin load balancing
  - model_name: fast-model
    params:
      model: docker_model_runner/ai/qwen3:0.6B-Q4_0

  - model_name: big-model
    params:
      model: docker_model_runner/ai/gemma3

general_settings:
  master_key: demo-secret   # Bearer token required on all requests
  num_retries: 2            # retry up to 2 times before fallback
  fallbacks:
    - fast-model: [big-model]   # automatic fallback chain
```

## Manual curl examples

```bash
GW="http://localhost:4000"
KEY="demo-secret"

# Health
curl "${GW}/health"

# List models
curl -H "Authorization: Bearer ${KEY}" "${GW}/v1/models"

# Chat completion
curl -X POST "${GW}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${KEY}" \
  -d '{"model":"smollm2","messages":[{"role":"user","content":"Hello!"}]}'

# Streaming
curl -N -X POST "${GW}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${KEY}" \
  -d '{"model":"smollm2","messages":[{"role":"user","content":"Count to 5"}],"stream":true}'

# Embeddings
curl -X POST "${GW}/v1/embeddings" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${KEY}" \
  -d '{"model":"embeddings","input":["hello world"]}'
```
