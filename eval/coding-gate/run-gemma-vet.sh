#!/usr/bin/env bash
# run-gemma-vet.sh — Strong-Inference vetting of gemma-4-E2B-it as a chat model.
#
# Pre-registered 2026-05-04 by Stevie + unheaded-scientist:
#
#   H1: gemma's final answer (post-thinking-strip) beats qwen on >=4/14 prompts
#       FALSIFIED if gemma matches qwen on <=3/14
#   H2: gemma can finish its answer within max_tokens=2000 (no truncation)
#       FALSIFIED if truncation on >=3/14 prompts at max_tokens=2000
#   H3: gemma's wall-time-to-answer is not >=2x slower than qwen on >=4/14
#   H4: gemma's reasoning trace catches review-tier bugs qwen misses
#
# Decision rule (locked):
#   H1 PASS + H2 PASS + H3 not-FAIL  → adopt gemma
#   H2 FAIL                           → reject (truncation)
#   H1 FAIL + H3 FAIL                 → reject (verbose AND slow)
#   H4 alone PASS                     → keep qwen default, document gemma niche
#
# Run order:
#   1. Pass-A (qwen-7b @600tok)    : 14 prompts, model already loaded
#   2. Switch model: scripts/switch-model.sh gemma
#   3. Pass-B (gemma @600tok)      : 14 prompts (same as qwen for fairness)
#   4. Pass-C (gemma @2000tok)     : 14 prompts (Stevie's bump)
#   5. Roll back: scripts/switch-model.sh qwen-7b
#
# All raw responses captured under eval/coding-gate/gemma-vet-2026-05-04/
# for post-hoc grading + reproducibility.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="${ROOT}/eval/coding-gate/gemma-vet-2026-05-04"
PROMPTS="${ROOT}/eval/coding-gate/prompts.jsonl"
LLAMA="http://127.0.0.1:8081"

mkdir -p "$OUT_DIR"

# Ask one prompt against the live llama-server and write JSON+plaintext.
# args: pass-name prompt-json-file max-tokens
ask() {
    local pass="$1" pf="$2" max="$3"
    local pid
    pid=$(jq -r '.id' "$pf")
    local prompt
    prompt=$(jq -r '.prompt' "$pf")
    local out_json="$OUT_DIR/${pass}__${pid}.json"
    local out_txt="$OUT_DIR/${pass}__${pid}.txt"

    # Build request body. No system role — keeps the apples-to-apples
    # tight (some models reject system-role for non-OpenAI templates).
    local body
    body=$(jq -n --arg p "$prompt" --argjson max "$max" '{
        model: "vetting",
        messages: [{role:"user", content:$p}],
        max_tokens: $max,
        temperature: 0.1,
        stream: false
    }')

    local t0 t1 elapsed
    t0=$(date +%s.%N)
    curl -s -X POST "$LLAMA/v1/chat/completions" \
        -H 'Content-Type: application/json' \
        -d "$body" \
        -m 240 > "$out_json"
    t1=$(date +%s.%N)
    elapsed=$(awk "BEGIN {printf \"%.2f\", $t1 - $t0}")

    # Extract the model's reply text and timing meta.
    local content
    content=$(jq -r '.choices[0].message.content // ""' "$out_json")
    local toks
    toks=$(jq -r '.usage.completion_tokens // 0' "$out_json")
    local fin
    fin=$(jq -r '.choices[0].finish_reason // "?"' "$out_json")

    # Plain-text dump for hand grading.
    {
        echo "# pass=$pass  id=$pid  elapsed=${elapsed}s  tokens=$toks  finish=$fin"
        echo "# prompt:"
        echo "$prompt" | sed 's/^/#   /'
        echo
        echo "$content"
    } > "$out_txt"

    printf "  %-12s %-22s %5ss %5d tok  finish=%s\n" "$pass" "$pid" "$elapsed" "$toks" "$fin"
}

# ── Run a pass over all 14 textbook prompts ───────────────────────
run_pass() {
    local pass="$1" max="$2"
    echo "[run-gemma-vet] === Pass: $pass (max_tokens=$max) ==="
    while IFS= read -r line; do
        local id
        id=$(echo "$line" | jq -r '.id')
        case "$id" in
            syntax-*|review-*)
                # Only the 14 textbook-tier prompts.
                local case_count
                case_count=$(echo "$id" | grep -c '^\(syntax\|review\)-\(bash\|python\|go\|rust\|html\|css\|javascript\)$' || true)
                if [[ "$case_count" -eq 1 ]]; then
                    local pf
                    pf=$(mktemp)
                    echo "$line" > "$pf"
                    ask "$pass" "$pf" "$max"
                    rm -f "$pf"
                fi
                ;;
        esac
    done < "$PROMPTS"
    echo
}

# Confirm llama-server is up.
curl -s -m 3 "$LLAMA/health" > /dev/null || {
    echo "ERROR: llama-server not reachable at $LLAMA" >&2
    exit 1
}

# Determine which model is currently loaded (from /v1/models).
LOADED=$(curl -s "$LLAMA/v1/models" | jq -r '.data[0].id' | head -1)
echo "[run-gemma-vet] currently loaded: $LOADED"
echo "[run-gemma-vet] outputs: $OUT_DIR"
echo

# Driver: $1 = pass label, $2 = max_tokens
PASS_LABEL="${1:-passA-qwen-600}"
MAX_TOKENS="${2:-600}"
run_pass "$PASS_LABEL" "$MAX_TOKENS"
echo "[run-gemma-vet] pass complete: $PASS_LABEL"
