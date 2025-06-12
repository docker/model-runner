# Aggregated Metrics Endpoint

The model-runner now exposes an aggregated `/metrics` endpoint that collects and labels metrics from all active llama.cpp runners.

## Overview

When llama.cpp models are running, each server automatically exposes Prometheus-compatible metrics at its `/metrics` endpoint. The model-runner now aggregates these metrics from all active runners, adds identifying labels, and serves them through a unified `/metrics` endpoint. This provides a comprehensive view of all running models with proper Prometheus labeling.

## Aggregated Metrics Format

Instead of exposing metrics from a single runner, the endpoint now aggregates metrics from all active runners and adds labels to identify the source:

### Example Output

```prometheus
# HELP llama_prompt_tokens_total Total number of prompt tokens processed
# TYPE llama_prompt_tokens_total counter
llama_prompt_tokens_total{backend="llama.cpp",model="llama3.2:latest",mode="completion"} 4934
llama_prompt_tokens_total{backend="llama.cpp",model="ai/mxbai-embed-large:335M-F16",mode="embedding"} 4525

# HELP llama_generation_tokens_total Total number of tokens generated
# TYPE llama_generation_tokens_total counter
llama_generation_tokens_total{backend="llama.cpp",model="llama3.2:latest",mode="completion"} 2156

# HELP llama_requests_total Total number of requests processed
# TYPE llama_requests_total counter
llama_requests_total{backend="llama.cpp",model="llama3.2:latest",mode="completion"} 127
llama_requests_total{backend="llama.cpp",model="ai/mxbai-embed-large:335M-F16",mode="embedding"} 89
```

### Labels Added

Each metric is automatically labeled with:
- **`backend`**: The inference backend (e.g., "llama.cpp")
- **`model`**: The model name (e.g., "llama3.2:latest")
- **`mode`**: The operation mode ("completion" or "embedding")

## Usage

### Enabling Metrics (Default)

By default, the aggregated metrics endpoint is enabled. When the model-runner starts with active runners, you can access metrics at:

```
GET /metrics
```

### Disabling Metrics

To disable the metrics endpoint, set the `DISABLE_METRICS` environment variable:

```bash
export DISABLE_METRICS=1
```

### TCP Port Access

If you're running the model-runner with a TCP port (using `MODEL_RUNNER_PORT`), you can access metrics via HTTP:

```bash
# If MODEL_RUNNER_PORT=8080
curl http://localhost:8080/metrics
```

### Unix Socket Access

If using Unix sockets (default), you'll need to use a tool that supports Unix socket HTTP requests:

```bash
# Using curl with Unix socket
curl --unix-socket model-runner.sock http://localhost/metrics
```

## Metrics Available

The aggregated endpoint exposes all metrics from active llama.cpp runners, typically including:

- **Request metrics**: Total requests, request duration, queue statistics
- **Token metrics**: Prompt tokens, generation tokens, tokens per second
- **Memory metrics**: Memory usage, cache statistics
- **Model metrics**: Model loading status, context usage
- **Performance metrics**: Processing latency, throughput

All metrics retain their original names and types but gain the additional identifying labels.
