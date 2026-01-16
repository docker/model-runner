#!/bin/bash

add_accelerators() {
  # Add NVIDIA GPU support for CUDA variants and GPU-accelerated backends
  if [[ "${DOCKER_IMAGE-}" == *"-cuda" ]] || \
     [[ "${DOCKER_IMAGE-}" == *"-diffusers" ]] || \
     [[ "${DOCKER_IMAGE-}" == *"-sglang" ]]; then
      if docker info -f '{{range $k, $v := .Runtimes}}{{$k}}{{"\n"}}{{end}}' 2>/dev/null | grep -qx "nvidia"; then
        args+=("--gpus" "all" "--runtime=nvidia")
      fi
  fi

  # Add GPU/accelerator devices if present
  for i in /dev/dri /dev/kfd /dev/accel /dev/davinci* /dev/devmm_svm /dev/hisi_hdc; do
    if [ -e "$i" ]; then
      args+=("--device" "$i")
    fi
  done

  local render_gid
  render_gid=$(set +o pipefail; command getent group render 2>/dev/null | cut -d: -f3)
  if [[ -n "$render_gid" ]]; then
    args+=("--group-add" "$render_gid")
    if [ -e "/dev/davinci_manager" ]; then
      # ascend driver accessing group id is 1000(HwHiAiUser)
      args+=("--group-add" "$(getent group HwHiAiUser | cut -d: -f3)")
    fi
  fi
}

add_optional_args() {
  if [ -n "${PORT-}" ]; then
    args+=(-p "$PORT:$PORT" -e "MODEL_RUNNER_PORT=$PORT")
  fi

  if [ -n "${MODELS_PATH-}" ]; then
    args+=(-v "$MODELS_PATH:/models" -e MODELS_PATH=/models)
  fi

  for i in /usr/local/dcmi /usr/local/bin/npu-smi /usr/local/Ascend/driver/lib64/ /usr/local/Ascend/driver/version.info /etc/ascend_install.info; do
    if [ -e "$i" ]; then
      args+=(-v "$i:$i")
    fi
  done

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
    chmod a+rwx "$MODELS_PATH"
  fi

  if [ -z "${DOCKER_IMAGE-}" ]; then
    echo "DOCKER_IMAGE is required" >&2
    return 1
  fi

  "${args[@]}" "$DOCKER_IMAGE"
}

main "$@"
