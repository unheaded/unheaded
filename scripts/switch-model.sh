#!/usr/bin/env bash
# switch-model.sh — atomically swap the inference model.
#
# Stevie's directive (2026-05-04): "unheaded platform should be able to
# interchange different models." This script is the canonical seam for
# that interchange. Every chat path (web UI, CLI, agentd) talks to
# llama-server on :8081 over the OpenAI-compat /v1/chat/completions
# API; swapping the model = killing llama-server and relaunching with
# a different --model. This script does that with health-checked
# rollback on failure.
#
# Usage:
#   ./scripts/switch-model.sh <model-key>
#
# Defined model keys (extend MODELS in this script to add more):
#   qwen-7b      qwen2.5-coder-7b-instruct  (16k ctx, 4 parallel — light, fast,
#                                            current default; ~6 GB VRAM)
#   deepseek     DeepSeek-Coder-V2-Lite     (16B-MoE, ~10 GB VRAM)
#                                            ⚠ does NOT fit on the 12 GB RX 7700
#                                            XT under zhen_app.py's typical RAG
#                                            prompt sizes (4k ctx prompt
#                                            overflow; 8k ctx OOMs mid-matmul).
#                                            Kept here for the day a 24 GB card
#                                            arrives or zhen_app prompts shrink.
#
# Env overrides per launch:
#   LLAMA_BIN          path to llama-server     (default: ./llama.cpp/build/bin/llama-server)
#   LLAMA_HOST         (default: 127.0.0.1)
#   LLAMA_PORT         (default: 8081)
#   LLAMA_THREADS      (default: 6)
#
# Rollback: on first health-check failure within 90 s the script kills
# the new server and exits non-zero. The previous server is already
# stopped at that point, so manually restart with the prior key.
#
# Note: if any chat clients are mid-stream (zhen_app.py, zhen-cli) they
# will see a connection drop and may surface an error to the operator.
# The swap is not atomic from the user's perspective — there is a
# ~30-90 second gap while the new model loads.

set -euo pipefail

LLAMA_BIN="${LLAMA_BIN:-/home/govan/tmp/unheaded/llama.cpp/build/bin/llama-server}"
LLAMA_HOST="${LLAMA_HOST:-127.0.0.1}"
LLAMA_PORT="${LLAMA_PORT:-8081}"
LLAMA_THREADS="${LLAMA_THREADS:-6}"
MODELS_DIR="${MODELS_DIR:-/var/zhen/models}"

declare -A MODEL_FILE
MODEL_FILE[qwen-7b]="qwen2.5-coder-7b-instruct-q4_k_m.gguf"
MODEL_FILE[deepseek]="DeepSeek-Coder-V2-Lite-Instruct-Q4_K_M.gguf"
MODEL_FILE[gemma]="gemma-4-E2B-it.gguf"
MODEL_FILE[deepseek-cpu]="DeepSeek-Coder-V2-Lite-Instruct-Q4_K_M.gguf"

# Per-model launch flags. Different architectures need different settings:
# - deepseek-v2 has MLA attention; flash-attn unsupported on ROCm → can't
#   quantize V cache, must keep parallel slots low to fit the 12 GB VRAM.
# - qwen-7b is dense; comfortable at 16k ctx with 4 parallel slots.
declare -A MODEL_FLAGS
MODEL_FLAGS[qwen-7b]="--ctx-size 16384"
MODEL_FLAGS[deepseek]="--ctx-size 4096 --parallel 1 --cache-type-k q8_0"
MODEL_FLAGS[gemma]="--ctx-size 8192"
# deepseek-cpu: 27-layer model; first 20 expert layers go to system RAM
# (~6 GB), remaining 7 layers + attention stay on GPU (~5 GB VRAM).
# Speed expectation: 10-15 tok/s vs full-GPU 70 tok/s, but quality +.
MODEL_FLAGS[deepseek-cpu]="--ctx-size 8192 --parallel 1 --cache-type-k q8_0 --n-cpu-moe 20"

# Friendly model name reported via OpenAI /v1/models. Cosmetic — clients
# can read this back, but llama-server actually serves whatever GGUF is
# loaded. Set ZHEN_MODEL to match for the web UI to display correctly.
declare -A MODEL_NAME
MODEL_NAME[qwen-7b]="qwen2.5-coder-7b-instruct"
MODEL_NAME[deepseek]="deepseek-coder-v2-lite-instruct"
MODEL_NAME[gemma]="gemma-4-E2B-it"
MODEL_NAME[deepseek-cpu]="deepseek-coder-v2-lite-cpu-moe"

if [[ $# -lt 1 ]]; then
    echo "usage: $0 <model-key>" >&2
    echo "available keys:" >&2
    for k in "${!MODEL_FILE[@]}"; do
        echo "  $k → $MODELS_DIR/${MODEL_FILE[$k]}" >&2
    done
    exit 2
fi

KEY="$1"
if [[ -z "${MODEL_FILE[$KEY]:-}" ]]; then
    echo "unknown model key: $KEY" >&2
    echo "known: ${!MODEL_FILE[*]}" >&2
    exit 2
fi

GGUF="$MODELS_DIR/${MODEL_FILE[$KEY]}"
if [[ ! -f "$GGUF" ]]; then
    echo "model file missing: $GGUF" >&2
    echo "  download it first; see /var/zhen/models/ for current inventory" >&2
    exit 2
fi

FLAGS="${MODEL_FLAGS[$KEY]}"
NAME="${MODEL_NAME[$KEY]}"

echo "[switch-model] swapping to '$KEY' ($NAME)"
echo "[switch-model]   gguf:  $GGUF"
echo "[switch-model]   flags: $FLAGS"
echo "[switch-model]   port:  $LLAMA_PORT"

# Stop any current llama-server bound on the target port.
if ss -lnt 2>/dev/null | grep -q ":${LLAMA_PORT} "; then
    PID=$(ss -lntp 2>/dev/null | awk -v p=":${LLAMA_PORT}" '$4 ~ p {print $0}' | grep -oE 'pid=[0-9]+' | head -1 | cut -d= -f2)
    if [[ -n "${PID:-}" ]]; then
        echo "[switch-model] stopping current llama-server (pid $PID)..."
        kill "$PID" 2>/dev/null || true
        for i in $(seq 1 20); do
            ss -lnt 2>/dev/null | grep -q ":${LLAMA_PORT} " || break
            sleep 1
        done
    fi
fi

# Launch the new model in the background.
LOG="/tmp/llama-${KEY}.log"
echo "[switch-model] launching... (log: $LOG)"
# shellcheck disable=SC2086 # FLAGS is intentionally word-split
nohup "$LLAMA_BIN" \
    --model "$GGUF" \
    --host "$LLAMA_HOST" \
    --port "$LLAMA_PORT" \
    --n-gpu-layers 999 \
    --threads "$LLAMA_THREADS" \
    $FLAGS \
    > "$LOG" 2>&1 &
NEW_PID=$!
disown

# Wait for /health 200, with rollback on death or timeout.
for i in $(seq 1 90); do
    if ! kill -0 "$NEW_PID" 2>/dev/null; then
        echo "[switch-model] ✗ process died at ${i}s — see $LOG" >&2
        tail -20 "$LOG" >&2
        exit 1
    fi
    code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 1 "http://${LLAMA_HOST}:${LLAMA_PORT}/health" 2>/dev/null || echo "000")
    if [[ "$code" == "200" ]]; then
        echo "[switch-model] ✓ healthy after ${i}s"
        echo "[switch-model]   pid=$NEW_PID  port=$LLAMA_PORT  model=$NAME"
        echo ""
        echo "Update web UI to display the new model name:"
        echo "  export ZHEN_MODEL='$NAME' and restart raft/zhen_app.py"
        exit 0
    fi
    sleep 1
done

echo "[switch-model] ✗ /health never returned 200 within 90s" >&2
tail -20 "$LOG" >&2
kill "$NEW_PID" 2>/dev/null || true
exit 1
