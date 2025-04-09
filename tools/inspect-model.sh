#!/usr/bin/env bash
set -euo pipefail

# Initialize
VERBOSE=false
INPUT=""

# Parse arguments
for arg in "$@"; do
  case "$arg" in
    --verbose)
      VERBOSE=true
      ;;
    *)
      if [ -z "$INPUT" ]; then
        INPUT="$arg"
      else
        echo "❌ Unexpected argument: $arg"
        echo "Usage: $0 <oci-repo|oci-image-ref> [--verbose]"
        exit 1
      fi
      ;;
  esac
done

if [ -z "$INPUT" ]; then
  echo "Usage: $0 <oci-repo|oci-image-ref> [--verbose]"
  echo "Example: $0 ai/qwen2.5"
  echo "         $0 ai/qwen2.5:7B-Q4_K_M --verbose"
  exit 1
fi

# Determine if input contains a tag (e.g. repo:tag)
if [[ "$INPUT" == *:* ]]; then
  # Single image mode
  REPO="${INPUT%%:*}"
  TAG="${INPUT##*:}"
  IMAGE_REFS=("${REPO}:${TAG}")
else
  # Repository mode - list all tags
  echo "📦 Listing tags for repository: $INPUT"
  TAGS=$(crane ls "$INPUT")
  IMAGE_REFS=()
  for TAG in $TAGS; do
    IMAGE_REFS+=("${INPUT}:${TAG}")
  done
fi

echo ""

for IMAGE_REF in "${IMAGE_REFS[@]}"; do
  echo "🔍 Inspecting: $IMAGE_REF"

  RAW_JSON=$(crane manifest "$IMAGE_REF" 2>&1)

  if ! jq empty <<<"$RAW_JSON" > /dev/null 2>&1; then
    echo "❌ Invalid JSON manifest for $IMAGE_REF"
    continue
  fi

  MEDIA_TYPE=$(jq -r '.mediaType' <<<"$RAW_JSON")

  if [[ "$MEDIA_TYPE" == *"image.index"* ]]; then
    DIGEST=$(jq -r '.manifests[0].digest' <<<"$RAW_JSON")
    MANIFEST_JSON=$(crane manifest "${IMAGE_REF%@*}@${DIGEST}")
  else
    MANIFEST_JSON="$RAW_JSON"
  fi

  # Compute size
  TOTAL_SIZE=$(jq '[.layers[]?.size, .config.size] | map(select(. != null)) | add' <<<"$MANIFEST_JSON")
  BYTES="$TOTAL_SIZE"
  MB=$(awk "BEGIN {printf \"%.2f\", $BYTES / 1000 / 1000}")
  GB=$(awk "BEGIN {printf \"%.2f\", $BYTES / 1000 / 1000 / 1000}")

  CONFIG_DIGEST=$(jq -r '.config.digest' <<<"$MANIFEST_JSON")
  CONFIG_JSON=$(crane blob "${IMAGE_REF%@*}@${CONFIG_DIGEST}")

  # Try common paths for model metadata
  FORMAT=$(jq -r '.config.format // .format // "-"' <<<"$CONFIG_JSON")
  QUANT=$(jq -r '.config.quantization // .quantization // "-"' <<<"$CONFIG_JSON")
  PARAMS=$(jq -r '.config.parameters // .parameters // "-"' <<<"$CONFIG_JSON")
  ARCH=$(jq -r '.config.architecture // .architecture // "-"' <<<"$CONFIG_JSON")
  MODEL_SIZE=$(jq -r '.config.size // .size // "-"' <<<"$CONFIG_JSON")

  echo "🧠 Model Info:"
  printf "   • Image        : %s\n" "$IMAGE_REF"
  printf "   • Format       : %s\n" "$FORMAT"
  printf "   • Quantization : %s\n" "$QUANT"
  printf "   • Parameters   : %s\n" "$PARAMS"
  printf "   • Architecture : %s\n" "$ARCH"
  printf "   • Model Size   : %s\n" "$MODEL_SIZE"
  printf "   • Artifact Size: %s bytes (%s MB / %s GB)\n" "$BYTES" "$MB" "$GB"

  # GGUF model layer digest
  GGUF_DIGEST=$(jq -r '.layers[] | select(.mediaType == "application/vnd.docker.ai.gguf.v3") | .digest' <<<"$MANIFEST_JSON")
  if [ -n "$GGUF_DIGEST" ]; then
    echo "📦 GGUF Layer Digest:"
    echo "   • $GGUF_DIGEST"

    if [ "$VERBOSE" = true ]; then
      echo "🔎 Inspecting GGUF metadata with gguf-tools..."
      TEMP_GGUF=$(mktemp /tmp/model.XXXXXX.gguf)
      crane blob "${IMAGE_REF%@*}@${GGUF_DIGEST}" > "$TEMP_GGUF"
      echo ""
      gguf-tools show "$TEMP_GGUF" || echo "⚠️  gguf-tools failed"
      echo ""
      rm -f "$TEMP_GGUF"
    fi
  else
    echo "⚠️  No GGUF layer with mediaType application/vnd.docker.ai.gguf.v3 found."
  fi

  # License
LICENSE_DIGESTS=($(jq -r '.layers[]? | select(.mediaType == "application/vnd.docker.ai.license") | .digest' <<<"$MANIFEST_JSON"))

if [ "${#LICENSE_DIGESTS[@]}" -eq 0 ]; then
  echo "⚠️  No license blob found."
else
  echo "📜 License(s):"
  for DIGEST in "${LICENSE_DIGESTS[@]}"; do
    echo "   • Digest: $DIGEST"
    LICENSE_CONTENT=$(crane blob "${IMAGE_REF%@*}@${DIGEST}")
    echo "$LICENSE_CONTENT" | head -n 5
    echo "   ─────────────────────────────────────"
  done
fi

  echo "----------------------------------------"
done