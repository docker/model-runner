# Unified memory configuration for integrated GPUs

Docker Model Runner uses llama.cpp under the hood. On systems with integrated GPUs (for example AMD APUs), available shared memory may be reported incorrectly unless unified memory is enabled.

## Docker Compose

Set environment variables on the model runner service:

```yaml
services:
  model-runner:
    image: docker/model-runner:latest
    environment:
      GGML_CUDA_ENABLE_UNIFIED_MEMORY: "1"
```

## docker model run

```shell
GGML_CUDA_ENABLE_UNIFIED_MEMORY=1 docker model run ai/gemma3 "Hello"
```

## Standalone dmr

```shell
export GGML_CUDA_ENABLE_UNIFIED_MEMORY=1
dmr serve &
dmr run ai/gemma3 "Hello"
```

See [ggml-org/llama.cpp#18159](https://github.com/ggml-org/llama.cpp/issues/18159) for background.
