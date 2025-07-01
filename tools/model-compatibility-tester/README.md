# Model Compatibility Tester

A simple tool to test Docker AI model compatibility on your hardware.

## What it does

1. Pulls AI models from Docker registry
2. Runs a test prompt: "Write me a 10 word poem"
3. Records success/failure in CSV format
4. Cleans up models after testing

## Usage

```bash
# Test all repositories and tags from "ai" namespace
./test-model-compatibility.sh --namespace ai

# Test all models from local directory
./test-model-compatibility.sh

# Test specific models
./test-model-compatibility.sh --models ai/llama3.1,ai/qwen2.5

# Test specific model with variant
./test-model-compatibility.sh --models ai/llama3.2:latest

# Test namespace with custom prompt
./test-model-compatibility.sh --namespace ai --prompt "Hello world"
```

## Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Docker Hub namespace to test all repositories from |
| `-m, --models` | Comma-separated list of models to test |
| `-v, --variants` | Comma-separated list of variants to test |
| `--prompt` | Custom test prompt |
| `-h, --help` | Show help |

## Output

Results are saved to `results/results.csv` with columns:
- timestamp
- model
- variant
- hardware_type (macos/linux/nvidia)
- total_memory_mb
- gpu_memory_mb
- status (SUCCESS/FAILED/PULL_FAILED)
- duration_seconds
- error_type (MEMORY_ERROR/RUNTIME_ERROR/etc)
- error_message

## Error Types

- **SUCCESS**: Model worked correctly
- **MEMORY_ERROR**: Out of memory (model too large)
- **RUNTIME_ERROR**: General runtime failure
- **MODEL_NOT_FOUND**: Model not available
- **PULL_ERROR**: Failed to download model
