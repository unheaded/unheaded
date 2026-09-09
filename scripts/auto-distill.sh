#!/bin/bash
# auto-distill.sh — Wait for training to finish, then run local distillation
# Run with: nohup bash scripts/auto-distill.sh > /tmp/auto-distill.log 2>&1 &

set -e
cd ~/tmp/unheaded

echo "=== Auto-Distill: Waiting for training v3 to finish ==="
echo "  $(date)"

# Wait for zhenai-forge to exit
while pgrep -f "zhenai-forge" > /dev/null 2>&1; do
    STEP=$(tail -1 /tmp/forge-v3.log 2>/dev/null | grep -oP 'Step \K[0-9]+' || echo "?")
    echo "  Training still running (step $STEP)... waiting 60s"
    sleep 60
done

echo "  Training complete at $(date)"
echo ""

# Start llama-server for local inference
echo "=== Starting llama-server for distillation ==="
nohup ~/tmp/unheaded/llama.cpp/build/bin/llama-server \
    -m /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
    --host 0.0.0.0 --port 20100 \
    -ngl 33 -c 4096 \
    </dev/null >/dev/null 2>&1 &
LLAMA_PID=$!
echo "  llama-server PID: $LLAMA_PID"

# Wait for it to be ready
echo "  Waiting for llama-server to load..."
for _ in $(seq 1 60); do
    if curl -sf http://localhost:20100/health > /dev/null 2>&1; then
        echo "  llama-server ready!"
        break
    fi
    sleep 5
done

if ! curl -sf http://localhost:20100/health > /dev/null 2>&1; then
    echo "  ERROR: llama-server failed to start"
    exit 1
fi

# Run local distillation on full repo
echo ""
echo "=== Running Local Distillation (2378 files) ==="
echo "  Started: $(date)"
python3 raft/scripts/distill_qa.py \
    --local \
    --repo \
    --output /var/zhen/distilled_local_repo.jsonl

echo ""
echo "=== Distillation Complete ==="
echo "  Finished: $(date)"
echo "  Output: /var/zhen/distilled_local_repo.jsonl"
wc -l /var/zhen/distilled_local_repo.jsonl 2>/dev/null

# Merge with existing dataset
echo ""
echo "=== Merging Datasets ==="
cat /var/zhen/raft_dataset_combined.jsonl /var/zhen/distilled_local_repo.jsonl \
    > /var/zhen/raft_dataset_v4.jsonl
echo "  Combined: $(wc -l < /var/zhen/raft_dataset_v4.jsonl) QA pairs"

echo ""
echo "=== DONE — Ready for kingdom-v4 training ==="
echo "  Dataset: /var/zhen/raft_dataset_v4.jsonl"
echo "  Next: kill llama-server, run zhenai-forge train --data /var/zhen/raft_dataset_v4.jsonl"
