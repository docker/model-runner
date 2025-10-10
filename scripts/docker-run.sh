#!/bin/bash

add_accelerators() {
  # Add GPU/accelerator devices if present
  for i in /dev/dri /dev/kfd /dev/accel /dev/davinci* /dev/devmm_svm /dev/hisi_hdc; do
    if [ -e "$i" ]; then
      args+=("--device" "$i")
    fi
  done
  args+=("--group-add" "$(getent group render | cut -d: -f3)")
}

add_optional_args() {
  if [ -n "${PORT-}" ]; then
    args+=(-p "$PORT:$PORT" -e "MODEL_RUNNER_PORT=$PORT")
  fi

  if [ -n "${MODELS_PATH-}" ]; then
    args+=(-v "$MODELS_PATH:/models" -e MODELS_PATH=/models)
  fi

  if [ -n "${LLAMA_ARGS-}" ]; then
    args+=(-e "LLAMA_ARGS=$LLAMA_ARGS")
  fi

  if [ -n "${DMR_ORIGINS-}" ]; then
    args+=(-e "DMR_ORIGINS=$DMR_ORIGINS")
  fi

  if [ -n "${DO_NOT_TRACK-}" ]; then
    args+=(-e "DO_NOT_TRACK=$DO_NOT_TRACK")
  fi

  if [ -n "${DEBUG-}" ]; then
    args+=(-e "DEBUG=$DEBUG")
  fi

  add_accelerators
}

main() {
  set -eux -o pipefail

  local args=(docker run --rm -e LLAMA_SERVER_PATH=/app/bin)
  add_optional_args

  # Ensure model path exists only if provided
  if [ -n "${MODELS_PATH-}" ]; then
    mkdir -p "$MODELS_PATH"
    chmod a+rx "$MODELS_PATH"
  fi

  if [ -z "${DOCKER_IMAGE-}" ]; then
    echo "DOCKER_IMAGE is required" >&2
    return 1
  fi

  "${args[@]}" "$DOCKER_IMAGE"
}

main "$@"

