#!/usr/bin/env bash
# ab_test_checkpoint.sh — A/B test kingdom-v5 epoch 1 checkpoint vs base model
#
# Usage:
#   ./ab_test_checkpoint.sh [checkpoint_path]
#
# If no checkpoint given, uses the latest kingdom-v5 checkpoint.
# Tests 30 Kingdom questions: base Mistral-7B vs LoRA-augmented.
# Detects: verbatim memorization vs real understanding.
#
# Output: /tmp/ab_test_results.md

set -euo pipefail

LLAMA_SERVER="/home/govan/tmp/unheaded/llama.cpp/build/bin/llama-server"
MODEL="/var/zhen/models/mistral-7b-instruct-q5_k_m.gguf"
ZLORA_TO_GGUF="/home/govan/tmp/unheaded/scripts/zlora-to-gguf.py"
RESULTS="/tmp/ab_test_results.md"
BASE_PORT=20200
LORA_PORT=20201

# Find checkpoint
if [[ $# -ge 1 ]]; then
    CHECKPOINT="$1"
else
    CHECKPOINT=$(ls /var/zhen/models/kingdom-v5.zlora.checkpoint-* 2>/dev/null | sort -t- -k2 -n | tail -1)
    if [[ -z "$CHECKPOINT" ]]; then
        echo "ERROR: No kingdom-v5 checkpoint found in /var/zhen/models/"
        exit 1
    fi
fi

echo "=== Zhenai A/B Test ==="
echo "Checkpoint: $CHECKPOINT"
echo "Results: $RESULTS"
echo ""

# Convert .zlora checkpoint to GGUF
LORA_GGUF="/tmp/ab_test_lora.gguf"
echo "[1/6] Converting $CHECKPOINT → $LORA_GGUF ..."
python3 "$ZLORA_TO_GGUF" -i "$CHECKPOINT" -o "$LORA_GGUF"
echo "      OK — $(du -h "$LORA_GGUF" | cut -f1)"

# Kill any existing llama-server instances on our test ports
echo "[2/6] Cleaning up any existing llama-server on ports $BASE_PORT/$LORA_PORT ..."
fuser -k "${BASE_PORT}/tcp" 2>/dev/null || true
fuser -k "${LORA_PORT}/tcp" 2>/dev/null || true
sleep 1

# Start base model server
echo "[3/6] Starting base model server on :$BASE_PORT ..."
"$LLAMA_SERVER" \
    --model "$MODEL" \
    --port "$BASE_PORT" \
    --ctx-size 4096 \
    --threads 4 \
    --n-gpu-layers 28 \
    --log-disable \
    > /tmp/ab_base.log 2>&1 &
BASE_PID=$!

# Start LoRA model server
echo "[4/6] Starting LoRA model server on :$LORA_PORT ..."
"$LLAMA_SERVER" \
    --model "$MODEL" \
    --lora "$LORA_GGUF" \
    --port "$LORA_PORT" \
    --ctx-size 4096 \
    --threads 4 \
    --n-gpu-layers 28 \
    --log-disable \
    > /tmp/ab_lora.log 2>&1 &
LORA_PID=$!

echo "      Base PID: $BASE_PID | LoRA PID: $LORA_PID"

# Wait for servers to be ready
wait_for_server() {
    local port=$1
    local name=$2
    local max_wait=60
    local waited=0
    echo -n "      Waiting for $name..."
    while ! curl -s "http://localhost:$port/health" >/dev/null 2>&1; do
        sleep 2
        waited=$((waited + 2))
        echo -n "."
        if [[ $waited -ge $max_wait ]]; then
            echo " TIMEOUT"
            echo "ERROR: $name failed to start. Check /tmp/ab_${name,,}.log"
            kill $BASE_PID $LORA_PID 2>/dev/null || true
            exit 1
        fi
    done
    echo " ready (${waited}s)"
}

wait_for_server $BASE_PORT "base"
wait_for_server $LORA_PORT "lora"

# 30 Kingdom test questions (mix of specific facts, operational, and edge cases)
QUESTIONS=(
    "What port does Wotan use for HTTP and gRPC?"
    "What is the Doom Range port allocation for Unheaded services?"
    "Explain the role of Akira in the Kingdom health monitoring system."
    "What is the consensus threshold for Akira to trigger a service restart?"
    "How does the RAFT training format differ from simple Q/A training?"
    "What is the purpose of format_prompt in zhenai-forge?"
    "What are the three layers of the Unheaded load balancer stack?"
    "What does kingdom-v5.zlora improve over v4?"
    "How does the Kanban task GUID relate to git commits?"
    "What is The Well and which databases does it contain?"
    "Explain the WAL store vs hybrid store in Wotan persistence."
    "What is the Wotan replication architecture for active-passive redundancy?"
    "How does Pi-hole integrate with LXD in the Kingdom DNS setup?"
    "What is the MBC ISA instruction format (opcode, dst, src, imm)?"
    "What IP address and port does the Unheaded dashboard run on?"
    "What is ADR-004 and what does it say about external dependencies?"
    "Explain the certificate lifetime policy for Kingdom PKI."
    "What is the unheaded-daemon reconciliation interval?"
    "What eBPF maps does doom-runner use to store the MBC ROM?"
    "What is the Inverse Mask formula for IPv6 capacity calculation?"
    "How does the S67 wire format freeze affect future Monad packets?"
    "What is Sophia's role in the Kingdom service mesh?"
    "Where is the FAISS index stored and how large is it?"
    "What does the zhenai-forge --accum-steps flag do?"
    "How does the Kanban soft delete work (deleted_at field)?"
    "What is the gRPC-first transport cascade order?"
    "What happens to Wotan when the primary fails in cluster mode?"
    "What language is zhenai-forge written in and why?"
    "What is the LXD bridge network address used by Unheaded?"
    "What question cannot be answered with Kingdom knowledge?"
)

query_model() {
    local port=$1
    local question=$2
    local prompt="<s>[INST] You are Zhenai, champion of the Unheaded Kingdom. Answer using your knowledge of the Unheaded infrastructure platform.

Question: $question [/INST]"

    curl -s "http://localhost:$port/completion" \
        -H "Content-Type: application/json" \
        -d "{
            \"prompt\": $(echo "$prompt" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))'),
            \"n_predict\": 300,
            \"temperature\": 0.1,
            \"stop\": [\"</s>\", \"[INST]\"]
        }" 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('content','ERROR').strip())" 2>/dev/null || echo "ERROR: query failed"
}

# Run 30 questions against both models
echo "[5/6] Running 30 questions against both models..."
echo ""

{
echo "# Zhenai A/B Test Results"
echo ""
echo "**Date:** $(date -u '+%Y-%m-%d %H:%M UTC')"
echo "**Checkpoint:** $CHECKPOINT"
echo "**Model:** mistral-7b-instruct-q5_k_m.gguf"
echo ""
echo "---"
echo ""

total=0
base_wins=0
lora_wins=0
memorized=0

for i in "${!QUESTIONS[@]}"; do
    q="${QUESTIONS[$i]}"
    num=$((i + 1))
    echo "## Q${num}: ${q}"
    echo ""

    base_ans=$(query_model $BASE_PORT "$q")
    lora_ans=$(query_model $LORA_PORT "$q")

    echo "**Base:**"
    echo "$base_ans"
    echo ""
    echo "**LoRA:**"
    echo "$lora_ans"
    echo ""

    # Check for memorization signal: exact substring match > 80 chars
    # If LoRA answer appears verbatim in training data, flag it
    lora_first_100=$(echo "$lora_ans" | head -c 100)
    if grep -qF "$lora_first_100" /var/zhen/distilled_qa_claude.jsonl 2>/dev/null || \
       grep -qF "$lora_first_100" /var/zhen/raft_dataset_v5.jsonl 2>/dev/null; then
        echo "**FLAG: POSSIBLE VERBATIM MEMORIZATION** — LoRA answer matches training data"
        memorized=$((memorized + 1))
    fi

    # Check for degenerate output (repetition)
    unique_words=$(echo "$lora_ans" | tr ' ' '\n' | sort -u | wc -l)
    total_words=$(echo "$lora_ans" | wc -w)
    if [[ $total_words -gt 20 ]] && [[ $unique_words -lt $((total_words / 3)) ]]; then
        echo "**FLAG: POSSIBLE DEGENERATE OUTPUT** — low unique word ratio ($unique_words/$total_words)"
    fi

    echo "---"
    echo ""
    total=$((total + 1))
    echo "Progress: $num/30" >&2
done

echo "## Summary"
echo ""
echo "- **Total questions:** $total"
echo "- **Memorization flags:** $memorized"
echo "- **Checkpoint:** $CHECKPOINT"
echo ""
echo "### Interpretation"
echo ""
echo "If LoRA answers are:"
echo "- Specific (port numbers, service names, architecture details): **Real learning**"
echo "- Verbatim copies of training data (memorization flags): **Overfitting**"
echo "- Generic/vague with no Kingdom specifics: **Underfitting**"
echo "- Repetitive/degenerate: **Training instability**"
echo ""
echo "**Decision:**"
if [[ $memorized -gt 10 ]]; then
    echo "STOP training — high memorization rate ($memorized/30). Reduce LR or add dropout."
elif [[ $memorized -gt 5 ]]; then
    echo "CAUTION — moderate memorization ($memorized/30). Test one more epoch, watch closely."
else
    echo "CONTINUE — low memorization ($memorized/30). Proceed to epoch 2+3 if answers are specific."
fi

} > "$RESULTS"

echo "      Done. Results: $RESULTS"

# Cleanup
echo "[6/6] Shutting down test servers ..."
kill $BASE_PID $LORA_PID 2>/dev/null || true
wait $BASE_PID $LORA_PID 2>/dev/null || true

echo ""
echo "=== A/B Test Complete ==="
echo "Results: $RESULTS"
echo ""
echo "Quick check (LoRA-only answers with Kingdom-specific terms):"
grep -c 'port\|Wotan\|Akira\|LXD\|RAFT\|LoRA\|Kingdom\|FAISS\|PostgreSQL' "$RESULTS" || echo "0 Kingdom-specific mentions found"
