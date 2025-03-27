#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <oci-repo> or <oci-image-ref>"
  echo "Example: $0 ai/qwen2.5"
  echo "         $0 ai/qwen2.5:7B-Q4_K_M"
  exit 1
fi

INPUT="$1"

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

  # Optional license
  LICENSE_DIGEST=$(jq -r '.layers[]? | select(.mediaType == "application/vnd.docker.ai.license") | .digest' <<<"$MANIFEST_JSON" || true)
  if [ -n "${LICENSE_DIGEST:-}" ]; then
    LICENSE_CONTENT=$(crane blob "${IMAGE_REF%@*}@${LICENSE_DIGEST}")
    echo "📜 License (first 5 lines):"
    echo "$LICENSE_CONTENT" | head -n 5
  else
    echo "⚠️  No license blob found."
  fi

  echo "----------------------------------------"
done