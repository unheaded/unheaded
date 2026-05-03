#!/bin/bash
# Start Zhenai — WAVE15 post-rewire shape.
#
# Pre-rewire this script launched its own llama-server with Mistral-7B
# on port 20100. Post-rewire (WAVE15, commit 45d7aeb7-era) the inference
# substrate is the SHARED qwen-coder-7b llama-server on :8081 — the same
# binary the Go agent stack uses. We do NOT launch our own llama-server
# from this script. If :8081 isn't up, this script fails fast and tells
# the operator to bring it up via the Go-stack runbook.
#
# Pre-rewire this script also expected a 1.76M-vector FAISS index;
# post-rewire retrieval is via vor (cs serve) at :9876. We do NOT
# launch vor either; same fail-fast posture.
#
# What this script DOES launch:
#   • raft/zhen_app.py on :20103 (the Python web UI)
#
# Optional: also start cmd/zhen-agentd if -gated is passed. Without it,
# /api/v1/runbooks/<n>/execute returns 503 "agentd unreachable" and the
# UI degrades gracefully. Production posture should run the daemon.

set -e

echo "=== Starting Zhenai (WAVE15 post-rewire) ==="

# Phase 5 / WAVE15: context-size config preserved for Python's RAG
# pipeline budget calculations even though we don't launch llama-server.
CONTEXT_SIZE="${ZHEN_CONTEXT_SIZE:-16384}"

# 1. Pre-flight: backends MUST already be up.
echo "[1/3] Pre-flight backend checks..."

if ! curl -sf --max-time 3 http://127.0.0.1:8081/health > /dev/null; then
    echo "  ✗ llama-server :8081 not reachable"
    echo "    Bring it up first. Reference invocation:"
    echo ""
    echo "    cd \$HOME/tmp/unheaded/llama.cpp/build && \\"
    echo "    nohup ./bin/llama-server \\"
    echo "      -m /var/zhen/models/qwen2.5-coder-7b-instruct-q4_k_m.gguf \\"
    echo "      --host 127.0.0.1 --port 8081 \\"
    echo "      --n-gpu-layers 999 --ctx-size $CONTEXT_SIZE \\"
    echo "      --threads 6 \\"
    echo "      &> /tmp/llama-server.log &"
    echo ""
    exit 1
fi
echo "  ✓ llama-server :8081 (qwen-coder-7b)"

if ! curl -sf --max-time 3 http://127.0.0.1:9876/api/health > /dev/null; then
    echo "  ✗ vor :9876 not reachable"
    echo "    Bring it up first: 'cs serve' (or wherever your cs binary lives)."
    echo "    Verify the unheaded source symlink:"
    echo "    ls -la ~/.config/cs/sources/unheaded → /home/govan/tmp/unheaded"
    exit 1
fi
echo "  ✓ vor :9876"

# 2. Optional: start zhen-agentd for gated mutations (runbook execute,
#    future kanban-from-chat). Without it the UI is read-only on the
#    runbook surface (returns 503). With it, mutations route through
#    pkg/champion's three-rule gate (T6b closed).
GATED_FLAG="${1:-}"
if [[ "$GATED_FLAG" == "-gated" ]]; then
    if ! ss -lnt | grep -q ':20105'; then
        echo "[2/3] Starting cmd/zhen-agentd on :20105 (mutation gating)..."
        cd "$HOME/tmp/unheaded"
        nohup ./bin/zhen-agentd \
            -port 20105 \
            -project-root "$HOME/tmp/unheaded" \
            > /tmp/zhen-agentd.log 2>&1 &
        echo "  zhen-agentd PID: $!"
        for i in $(seq 1 10); do
            curl -sf http://127.0.0.1:20105/health > /dev/null && break
            sleep 1
        done
        if curl -sf http://127.0.0.1:20105/health > /dev/null; then
            echo "  ✓ zhen-agentd ready"
        else
            echo "  ✗ zhen-agentd did not bind in 10s; see /tmp/zhen-agentd.log"
            exit 1
        fi
    else
        echo "[2/3] zhen-agentd already running on :20105 — leaving alone"
    fi
else
    echo "[2/3] Skipping zhen-agentd (run with '-gated' to enable mutation gating)"
fi

# 3. Start the Python web UI.
if ss -lnt 2>/dev/null | grep -q ':20103'; then
    echo "[3/3] Web UI already bound on :20103 — leaving alone (use 'pkill -f zhen_app.py' to restart)"
else
    echo "[3/3] Starting Zhen Web UI on :20103..."
    source "$HOME/.venv/zhen/bin/activate" 2>/dev/null || true
    export ZHEN_LOCAL_MAX_TOKENS="$CONTEXT_SIZE"

    # Defaults the rewired RAGPipeline already prefers; setting them
    # explicitly here makes the deployment posture self-documenting.
    export ZHEN_INFERENCE_URL="${ZHEN_INFERENCE_URL:-http://localhost:8081}"
    export ZHEN_AGENTD_URL="${ZHEN_AGENTD_URL:-http://localhost:20105}"
    export ZHEN_MODEL="${ZHEN_MODEL:-qwen2.5-coder-7b-instruct}"
    export VOR_URL="${VOR_URL:-http://localhost:9876}"

    # The Well (PostgreSQL) — required for memory cache, conversation log,
    # and live-context kanban/audit/runbook intent injection. Without these,
    # the chat path falls back to vor-only retrieval and the model has to
    # guess at "how many tasks are in progress?" instead of reading PG.
    # Defaults match db/migrations/002_create_databases.sql + 008_zhen_memories.
    export ZHEN_DB_NAME="${ZHEN_DB_NAME:-unheaded_app}"
    export ZHEN_DB_USER="${ZHEN_DB_USER:-app_zhen}"
    export ZHEN_DB_PASSWORD="${ZHEN_DB_PASSWORD:-zhen_dev}"
    export ZHEN_DB_HOST="${ZHEN_DB_HOST:-localhost}"
    export ZHEN_DB_PORT="${ZHEN_DB_PORT:-5432}"

    cd "$HOME/tmp/unheaded/raft"
    nohup python3 zhen_app.py &> /tmp/zhen-webapp.log &
    WEBAPP_PID=$!
    echo "  webapp PID: $WEBAPP_PID"
fi

sleep 3
echo ""
echo "=== Zhenai Status ==="
curl -s http://localhost:8081/v1/models 2>/dev/null \
    | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Inference: {d[\"models\"][0][\"name\"]}')" \
    2>/dev/null || echo "Inference: NOT READY"
curl -s http://localhost:9876/api/health 2>/dev/null \
    | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Retrieval: vor — {d[\"sheets\"]} sheets, {d[\"categories\"]} categories')" \
    2>/dev/null || echo "Retrieval: NOT READY"
curl -s http://localhost:20103/health 2>/dev/null \
    | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Web UI:    {d[\"status\"]} (ready={d[\"rag_ready\"]}, well_connected={d[\"well_connected\"]})')" \
    2>/dev/null || echo "Web UI: NOT READY"
if ss -lnt 2>/dev/null | grep -q ':20105'; then
    curl -s http://localhost:20105/health 2>/dev/null \
        | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Daemon:    {d[\"status\"]} (mutation gating ON)')" \
        2>/dev/null || echo "Daemon: present but not responding"
else
    echo "Daemon:    not running (mutation gating OFF; run with '-gated' to enable)"
fi
echo ""
echo "Browse:        http://localhost:20103"
echo "Chat API:      POST http://localhost:20103/api/v1/query"
echo "Runbooks API:  POST http://localhost:20103/api/v1/runbooks/<n>/execute"
echo "Daemon API:    POST http://localhost:20105/api/v1/tool/exec  (when -gated)"
echo ""
echo "Stop:  pkill -f zhen_app.py; pkill -f bin/zhen-agentd"
