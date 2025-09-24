#!/bin/bash

main() {
  set -eux -o pipefail

  local gpu_device_flag=()
  for i in /dev/dri /dev/kfd /dev/accel /dev/davinci* /dev/devmm_svm /dev/hisi_hdc; do
    if [ -e "$i" ]; then
      gpu_device_flag+=("--device" "$i")
    fi
  done

  mkdir -p "$MODELS_PATH"
  chmod a+rx "$MODELS_PATH"
  docker run --rm \
    -p "$PORT:$PORT" \
    -v "$MODELS_PATH:/models" \
    -e MODEL_RUNNER_PORT="$PORT" \
    -e LLAMA_SERVER_PATH=/app/bin \
    -e MODELS_PATH=/models \
    -e LLAMA_ARGS="$LLAMA_ARGS" \
    -e DMR_ORIGINS="$DMR_ORIGINS" \
    -e DO_NOT_TRACK="$DO_NOT_TRACK" \
    -e DEBUG="$DEBUG" \
    --gpus all \
    "${gpu_device_flag[@]}" \
    "$DOCKER_IMAGE"
}

main "$@"

