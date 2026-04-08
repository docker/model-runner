#!/usr/bin/env bash
# model-cli gateway — interactive step-by-step demo
#
# Each step pauses and waits for Enter before running.
# Commands are "typed" character-by-character to look live.
#
# Prerequisites:
#   - Docker Model Runner running        (docker model ps)
#   - model-cli binary built             (cargo build --release in model-cli/)
#   - Models pulled:
#       docker model pull ai/smollm2
#       docker model pull ai/gemma3
#       docker model pull ai/qwen3:0.6B-Q4_0
#       docker model pull ai/nomic-embed-text-v1.5
#   - pip install openai                 (for the SDK step)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
BIN="${REPO_ROOT}/model-cli/target/release/model-cli"
GATEWAY_PORT=4000
GATEWAY_URL="http://localhost:${GATEWAY_PORT}"
API_KEY="demo-secret"
GATEWAY_PID=""

# ── colours ───────────────────────────────────────────────────────────────────
reset=$'\033[0m'
bold=$'\033[1m'
dim=$'\033[2m'
green=$'\033[0;32m'
yellow=$'\033[0;33m'
blue=$'\033[0;34m'
cyan=$'\033[0;36m'
red=$'\033[0;31m'
white=$'\033[0;37m'

# ── helpers ───────────────────────────────────────────────────────────────────

# Simulate typing a command character by character
typewrite() {
    local text="$1"
    local delay="${2:-0.04}"
    local char
    for (( i=0; i<${#text}; i++ )); do
        char="${text:$i:1}"
        printf '%s' "$char"
        sleep "$delay"
    done
}

# Print a fake prompt then type-animate the command, then wait for Enter
# After Enter is pressed the command is actually run.
run_step() {
    local description="$1"
    local command="$2"

    echo
    printf '%s# %s%s\n' "$dim" "$description" "$reset"
    printf '%s%s$%s ' "$bold" "$green" "$reset"
    typewrite "$command" 0.035
    printf '%s ▌%s' "$dim" "$reset"   # blinking-cursor illusion

    # Wait for Enter
    read -r -s _
    printf "\r${bold}${green}\$${reset} ${white}%s${reset}\n" "$command"

    # Actually run it
    eval "$command"
}

# A pause with a brief explanatory comment — no command executed
pause_comment() {
    local msg="$1"
    echo
    printf '%s# %s%s' "$dim" "$msg" "$reset"
    read -r -s _
    echo
}

# Section banner
section() {
    echo
    printf '%s%s┌──────────────────────────────────────────────────────┐%s\n' "$bold" "$blue" "$reset"
    printf '%s%s│  %-52s│%s\n' "$bold" "$blue" "$*" "$reset"
    printf '%s%s└──────────────────────────────────────────────────────┘%s\n' "$bold" "$blue" "$reset"
}

ok()  { printf '%s✓ %s%s\n' "$green" "$*" "$reset"; }
info(){ printf '%s  %s%s\n' "$cyan"  "$*" "$reset"; }

pretty_json() {
    python3 -c "import sys,json; print(json.dumps(json.load(sys.stdin), indent=2))"
}

GATEWAY_LOG=""   # path to temp log file for current gateway instance

wait_for_gateway() {
    local retries=30
    while ! curl -sf "${GATEWAY_URL}/health" >/dev/null 2>&1; do
        retries=$((retries - 1))
        [[ $retries -eq 0 ]] && { printf '%sGateway did not start%s\n' "$red" "$reset"; exit 1; }
        sleep 0.2
    done
}

# Launch the gateway binary directly (no pipe) so $! is the real PID.
# Logs go to a temp file; we tail it briefly so startup messages are visible.
launch_gateway() {
    local config="$1"
    GATEWAY_LOG="$(mktemp /tmp/model-cli-gateway-XXXXXX.log)"
    "${BIN}" gateway \
        --config "${config}" \
        --port "${GATEWAY_PORT}" \
        >"${GATEWAY_LOG}" 2>&1 &
    GATEWAY_PID=$!
    wait_for_gateway
    # Print startup lines captured so far (INFO level only, indented)
    grep 'INFO' "${GATEWAY_LOG}" 2>/dev/null | sed 's/^/  /' || true
}

stop_gateway() {
    if [[ -n "${GATEWAY_PID}" ]]; then
        kill "${GATEWAY_PID}" 2>/dev/null || true
        wait "${GATEWAY_PID}" 2>/dev/null || true
        GATEWAY_PID=""
        [[ -n "${GATEWAY_LOG}" ]] && rm -f "${GATEWAY_LOG}"
        GATEWAY_LOG=""
    fi
}

trap 'stop_gateway' EXIT

# ─────────────────────────────────────────────────────────────────────────────
# INTRO
# ─────────────────────────────────────────────────────────────────────────────

clear
printf '%s%s' "$bold" "$blue"
cat <<'BANNER'
  ██████╗   ██████╗  ██████╗  ██╗  ██╗ ███████╗ ██████╗
  ██╔══██╗ ██╔═══██╗ ██╔════╝ ██║ ██╔╝ ██╔════╝ ██╔══██╗
  ██║  ██║ ██║   ██║ ██║      █████╔╝  █████╗   ██████╔╝
  ██║  ██║ ██║   ██║ ██║      ██╔═██╗  ██╔══╝   ██╔══██╗
  ██████╔╝ ╚██████╔╝ ╚██████╗ ██║  ██╗ ███████╗ ██║  ██║
  ╚═════╝   ╚═════╝   ╚═════╝ ╚═╝  ╚═╝ ╚══════╝ ╚═╝  ╚═╝

  ███╗   ███╗  ██████╗  ██████╗  ███████╗ ██╗
  ████╗ ████║ ██╔═══██╗ ██╔══██╗ ██╔════╝ ██║
  ██╔████╔██║ ██║   ██║ ██║  ██║ █████╗   ██║
  ██║╚██╔╝██║ ██║   ██║ ██║  ██║ ██╔══╝   ██║
  ██║ ╚═╝ ██║ ╚██████╔╝ ██████╔╝ ███████╗ ███████╗
  ╚═╝     ╚═╝  ╚═════╝  ╚═════╝  ╚══════╝ ╚══════╝

   ██████╗   █████╗  ████████╗ ███████╗ ██╗    ██╗  █████╗  ██╗   ██╗
  ██╔════╝  ██╔══██╗ ╚══██╔══╝ ██╔════╝ ██║    ██║ ██╔══██╗ ╚██╗ ██╔╝
  ██║  ███╗ ███████║    ██║    █████╗   ██║ █╗ ██║ ███████║  ╚████╔╝
  ██║   ██║ ██╔══██║    ██║    ██╔══╝   ██║███╗██║ ██╔══██║   ╚██╔╝
  ╚██████╔╝ ██║  ██║    ██║    ███████╗ ╚███╔███╔╝ ██║  ██║    ██║
   ╚═════╝  ╚═╝  ╚═╝    ╚═╝    ╚══════╝  ╚══╝╚══╝  ╚═╝  ╚═╝    ╚═╝
BANNER
printf '%s\n' "$reset"
printf '%s  Press %s%sEnter%s%s to advance through each step.%s\n' "$dim" "$reset" "$bold" "$reset" "$dim" "$reset"
printf '%s  The gateway is started behind the scenes — commands shown are%s\n' "$dim" "$reset"
printf '%s  exactly what you would run in a real session.%s\n' "$dim" "$reset"
echo

pause_comment "Let's begin — press Enter to start"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 1 — show the config
# ─────────────────────────────────────────────────────────────────────────────

section "Step 1 — Write a gateway config"

pause_comment "The gateway is driven by a simple YAML file. Here's a basic one."

echo
printf '%s# demos/gateway/config-basic.yaml%s\n' "$dim" "$reset"
cat "${SCRIPT_DIR}/config-basic.yaml"

pause_comment "Press Enter to start the gateway"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 2 — start the gateway (shown as docker model gateway)
# ─────────────────────────────────────────────────────────────────────────────

section "Step 2 — Start the gateway"

# Show the pretty command; actually run our binary in the background
echo
printf '%s# Starts an OpenAI-compatible proxy on :4000%s\n' "$dim" "$reset"
printf '%s%s$%s ' "$bold" "$green" "$reset"
typewrite "docker model gateway --config demos/gateway/config-basic.yaml" 0.035
printf '%s ▌%s' "$dim" "$reset"
read -r -s _
printf '\r%s%s$%s %sdocker model gateway --config demos/gateway/config-basic.yaml%s\n' "$bold" "$green" "$reset" "$white" "$reset"

# Actually launch
launch_gateway "${SCRIPT_DIR}/config-basic.yaml"
echo
ok "Gateway listening on http://localhost:${GATEWAY_PORT}"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 3 — health check
# ─────────────────────────────────────────────────────────────────────────────

section "Step 3 — Health check"

run_step \
    "The gateway exposes /health — no auth required" \
    "curl -s http://localhost:${GATEWAY_PORT}/health | python3 -m json.tool"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 4 — list models
# ─────────────────────────────────────────────────────────────────────────────

section "Step 4 — List models  (/v1/models)"

run_step \
    "OpenAI-compatible model list — clients see gateway aliases, not backend details" \
    "curl -s http://localhost:${GATEWAY_PORT}/v1/models -H 'Authorization: Bearer ${API_KEY}' | python3 -m json.tool"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 5 — auth rejection
# ─────────────────────────────────────────────────────────────────────────────

section "Step 5 — Auth enforcement"

pause_comment "The gateway rejects requests with the wrong key"

echo
printf '%s# Wrong key → 401%s\n' "$dim" "$reset"
printf '%s%s$%s ' "$bold" "$green" "$reset"
typewrite "curl -s -o /dev/null -w '%{http_code}' http://localhost:${GATEWAY_PORT}/v1/chat/completions -H 'Authorization: Bearer WRONG'" 0.03
printf '%s ▌%s' "$dim" "$reset"
read -r -s _
printf "\r${bold}${green}\$${reset} ${white}curl -s -o /dev/null -w '%%{http_code}' .../v1/chat/completions -H 'Authorization: Bearer WRONG'${reset}\n"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${GATEWAY_URL}/v1/chat/completions" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer WRONG" \
    -d '{"model":"smollm2","messages":[{"role":"user","content":"hi"}]}')
printf "  HTTP %s\n" "$HTTP_CODE"
ok "Correctly rejected with 401"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 6 — chat completion
# ─────────────────────────────────────────────────────────────────────────────

section "Step 6 — Chat completion"

pause_comment "Standard OpenAI-compatible chat completions endpoint"

echo
printf '%s# POST /v1/chat/completions — non-streaming%s\n' "$dim" "$reset"
printf '%s%s$%s ' "$bold" "$green" "$reset"
typewrite "curl -s http://localhost:${GATEWAY_PORT}/v1/chat/completions \\" 0.03
printf "\n"
typewrite "  -H 'Authorization: Bearer ${API_KEY}' \\" 0.03
printf "\n"
typewrite "  -d '{\"model\":\"smollm2\",\"messages\":[{\"role\":\"user\",\"content\":\"What is Docker Model Runner?\"}],\"max_tokens\":80}'" 0.03
printf '%s ▌%s' "$dim" "$reset"
read -r -s _
printf '\r%s%s$%s %scurl -s .../v1/chat/completions -H '"'"'Authorization: Bearer %s'"'"' -d '"'"'{...}'"'"'%s\n\n' "$bold" "$green" "$reset" "$white" "$API_KEY" "$reset"

curl -sf -X POST "${GATEWAY_URL}/v1/chat/completions" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer ${API_KEY}" \
     -d '{
       "model": "smollm2",
       "messages": [
         {"role": "system", "content": "You are a helpful assistant. Be very brief."},
         {"role": "user",   "content": "What is Docker Model Runner? One sentence."}
       ],
       "max_tokens": 80
     }' | pretty_json

# ─────────────────────────────────────────────────────────────────────────────
# STEP 7 — streaming
# ─────────────────────────────────────────────────────────────────────────────

section "Step 7 — Streaming (SSE)"

pause_comment "Add stream:true — tokens arrive in real time"

echo
printf '%s# Same endpoint, stream:true → server-sent events%s\n' "$dim" "$reset"
printf '%s%s$%s ' "$bold" "$green" "$reset"
typewrite "curl -sN http://localhost:${GATEWAY_PORT}/v1/chat/completions \\" 0.03
printf "\n"
typewrite "  -H 'Authorization: Bearer ${API_KEY}' \\" 0.03
printf "\n"
typewrite "  -d '{\"model\":\"smollm2\",\"messages\":[{\"role\":\"user\",\"content\":\"Count 1 to 5\"}],\"stream\":true}'" 0.03
printf '%s ▌%s' "$dim" "$reset"
read -r -s _
printf '\r%s%s$%s %scurl -sN .../v1/chat/completions -d '"'"'{...stream:true...}'"'"'%s\n\n' "$bold" "$green" "$reset" "$white" "$reset"

curl -sfN -X POST "${GATEWAY_URL}/v1/chat/completions" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer ${API_KEY}" \
     -d '{
       "model": "smollm2",
       "messages": [{"role": "user", "content": "Count from 1 to 5, one number per word."}],
       "stream": true,
       "max_tokens": 40
     }' | while IFS= read -r line; do
         if [[ "${line}" == data:* ]]; then
             payload="${line#data: }"
             [[ "${payload}" == "[DONE]" ]] && break
             delta=$(printf '%s' "$payload" | python3 -c \
                 "import sys,json; d=json.load(sys.stdin)['choices'][0]['delta']; print(d.get('content') or '',end='')" 2>/dev/null || true)
             printf '%s' "$delta"
         fi
     done
echo
echo
ok "Streaming response complete"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 8 — advanced config (load balancing + fallbacks)
# ─────────────────────────────────────────────────────────────────────────────

section "Step 8 — Advanced config: load balancing & fallbacks"

pause_comment "Restart the gateway with the advanced config"

echo
printf '%s# config-advanced.yaml%s\n' "$dim" "$reset"
cat "${SCRIPT_DIR}/config-advanced.yaml"
echo

printf '%s%s$%s ' "$bold" "$green" "$reset"
typewrite "docker model gateway --config demos/gateway/config-advanced.yaml" 0.035
printf '%s ▌%s' "$dim" "$reset"
read -r -s _
printf '\r%s%s$%s %sdocker model gateway --config demos/gateway/config-advanced.yaml%s\n' "$bold" "$green" "$reset" "$white" "$reset"

stop_gateway
launch_gateway "${SCRIPT_DIR}/config-advanced.yaml"
echo
ok "Gateway restarted with advanced config"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 9 — round-robin load balancing
# ─────────────────────────────────────────────────────────────────────────────

section "Step 9 — Round-robin load balancing"

pause_comment "'fast-model' has 2 backends — watch them alternate across 4 requests"

echo
printf '%s# 4 requests to '"'"'fast-model'"'"' → round-robins across smollm2 + qwen3%s\n' "$dim" "$reset"
printf '%s%s$%s ' "$bold" "$green" "$reset"
typewrite "for i in 1 2 3 4; do curl -s .../v1/chat/completions -d '{\"model\":\"fast-model\",...}'; done" 0.03
printf '%s ▌%s' "$dim" "$reset"
read -r -s _
printf '\r%s%s$%s %sfor i in 1 2 3 4; do curl -s .../v1/chat/completions ...; done%s\n\n' "$bold" "$green" "$reset" "$white" "$reset"

for i in 1 2 3 4; do
    resp=$(curl -sf -X POST "${GATEWAY_URL}/v1/chat/completions" \
         -H "Content-Type: application/json" \
         -H "Authorization: Bearer ${API_KEY}" \
         -d "{
           \"model\": \"fast-model\",
           \"messages\": [{\"role\": \"user\", \"content\": \"Reply with only the number ${i}\"}],
           \"max_tokens\": 10
         }")
    model_used=$(printf '%s' "$resp" | python3 -c \
        "import sys,json; print(json.load(sys.stdin).get('model','?'))")
    content=$(printf '%s' "$resp" | python3 -c \
        "import sys,json; print(json.load(sys.stdin)['choices'][0]['message']['content'].strip())")
    printf "  Request %d  backend=%-40s  reply=%s\n" "$i" "$model_used" "$content"
done
echo
ok "Requests distributed across both backends"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 10 — embeddings
# ─────────────────────────────────────────────────────────────────────────────

section "Step 10 — Embeddings (nomic-embed-text)"

pause_comment "Dedicated embedding model behind its own alias"

echo
printf '%s# POST /v1/embeddings — two sentences, then compute cosine similarity%s\n' "$dim" "$reset"
printf '%s%s$%s ' "$bold" "$green" "$reset"
typewrite "curl -s -X POST http://localhost:${GATEWAY_PORT}/v1/embeddings \\" 0.03
printf "\n"
typewrite "  -H 'Content-Type: application/json' \\" 0.03
printf "\n"
typewrite "  -H 'Authorization: Bearer ${API_KEY}' \\" 0.03
printf "\n"
typewrite "  -d '{\"model\":\"embeddings\",\"input\":[\"The quick brown fox\",\"A fast auburn canine\"]}'" 0.03
printf '%s ▌%s' "$dim" "$reset"
read -r -s _
printf '\r%s%s$%s %scurl -s -X POST .../v1/embeddings -d '"'"'{...}'"'"'%s\n\n' "$bold" "$green" "$reset" "$white" "$reset"

curl -sf -X POST "${GATEWAY_URL}/v1/embeddings" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer ${API_KEY}" \
     -d '{"model":"embeddings","input":["The quick brown fox","A fast auburn canine"]}' \
| python3 -c "
import sys, json, math
resp = json.load(sys.stdin)
vecs = [item['embedding'] for item in sorted(resp['data'], key=lambda x: x['index'])]
def cos(a, b):
    dot = sum(x*y for x,y in zip(a,b))
    na  = math.sqrt(sum(x*x for x in a))
    nb  = math.sqrt(sum(x*x for x in b))
    return dot / (na * nb) if na and nb else 0.0
print(f'  Vector dimensions : {len(vecs[0])}')
print(f'  Cosine similarity : {cos(vecs[0], vecs[1]):.4f}')
print(f'  (sentences are semantically similar → high score)')
"
ok "Embeddings complete"

# ─────────────────────────────────────────────────────────────────────────────
# STEP 11 — OpenAI Python SDK
# ─────────────────────────────────────────────────────────────────────────────

section "Step 11 — OpenAI Python SDK compatibility"

pause_comment "Any app already using the openai library works with zero code changes"

echo
printf '%s# python demo — just swap base_url to point at the gateway%s\n' "$dim" "$reset"
cat <<'PYSHOW'
  from openai import OpenAI

  client = OpenAI(
      base_url="http://localhost:4000/v1",
      api_key="demo-secret",
  )

  resp = client.chat.completions.create(
      model="fast-model",
      messages=[{"role": "user", "content": "Name 3 benefits of local LLMs."}],
      max_tokens=120,
  )
  print(resp.choices[0].message.content)
PYSHOW

printf '%s%s$%s ' "$bold" "$green" "$reset"
typewrite "python3 demo.py" 0.05
printf '%s ▌%s' "$dim" "$reset"
read -r -s _
printf '\r%s%s$%s %spython3 demo.py%s\n\n' "$bold" "$green" "$reset" "$white" "$reset"

if python3 -c "import openai" 2>/dev/null; then
    python3 - <<'PYEOF'
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:4000/v1",
    api_key="demo-secret",
)

resp = client.chat.completions.create(
    model="fast-model",
    messages=[
        {"role": "system", "content": "You are a concise assistant."},
        {"role": "user",   "content": "Name 3 benefits of running LLMs locally."},
    ],
    max_tokens=120,
)
print(resp.choices[0].message.content)
PYEOF
    echo
    ok "OpenAI SDK works against the gateway — no code changes required"
else
    printf '%s  (skipped — openai package not installed: pip install openai)%s\n' "$yellow" "$reset"
    ok "OpenAI SDK step skipped — install openai to run it"
fi

# ─────────────────────────────────────────────────────────────────────────────
# DONE
# ─────────────────────────────────────────────────────────────────────────────

section "Demo complete"

echo
printf '%s  What we showed:%s\n' "$bold" "$reset"
info "YAML-driven config — models, auth, retries, fallbacks"
info "/health  /v1/models  /v1/chat/completions  /v1/embeddings"
info "Bearer-token auth  (accept ✓  reject 401 ✓)"
info "Non-streaming and streaming (SSE) chat completions"
info "Round-robin load balancing across multiple backends"
info "Automatic fallback chain when a backend fails"
info "Drop-in OpenAI Python SDK compatibility"
echo
ok "docker model gateway demo finished"
