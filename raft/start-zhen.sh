#!/bin/bash
# Start Zhenai Champion — all services
set -e

echo "=== Starting Zhenai Champion ==="

# Context window size — benchmark found zero degradation up to 32K on 7700 XT
# 16384 chosen for 4.8 GB VRAM headroom under concurrent load
CONTEXT_SIZE="${ZHEN_CONTEXT_SIZE:-16384}"

# 1. Start inference server (llama.cpp with ROCm)
echo "[1/2] Starting llama-server on port 20100 (context: $CONTEXT_SIZE)..."
cd ~/tmp/unheaded/llama.cpp/build
export LD_LIBRARY_PATH="$(pwd)/bin:$(pwd)/src:$(pwd)/ggml/src:$(pwd)/ggml/src/ggml-hip:$(pwd)/ggml/src/ggml-cpu:$LD_LIBRARY_PATH"
nohup ./bin/llama-server \
  -m ~/tmp/unheaded/raft/models/mistral-7b-instruct-q5_k_m.gguf \
  -ngl 40 -c "$CONTEXT_SIZE" --port 20100 \
  &> /tmp/llama-server.log &
echo "  PID: $!"

# Wait for model to load
echo "  Waiting for model load..."
for i in $(seq 1 30); do
  if curl -s http://localhost:20100/v1/models &>/dev/null; then
    echo "  Model loaded!"
    break
  fi
  sleep 2
done

# 2. Start Zhen Web UI + RAG
echo "[2/2] Starting Zhen Web UI on port 20103..."
source ~/.venv/zhen/bin/activate

# Pass local max tokens matching context size
export ZHEN_LOCAL_MAX_TOKENS="$CONTEXT_SIZE"

cd ~/tmp/unheaded/raft
nohup python3 zhen_app.py &> /tmp/zhen-webapp.log &
echo "  PID: $!"

sleep 3
echo ""
echo "=== Zhen Status ==="
curl -s http://localhost:20100/v1/models | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Inference: {d[\"models\"][0][\"name\"]}')" 2>/dev/null || echo "Inference: NOT READY"
curl -s http://localhost:20103/health | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'RAG: {d[\"status\"]} (ready={d[\"rag_ready\"]})')" 2>/dev/null || echo "RAG: NOT READY"
echo ""
echo "Web UI:  http://localhost:20103"
echo "API:     http://localhost:20103/api/v1/query"
echo "MCP:     python3 ~/tmp/unheaded/raft/zhen_mcp_server.py (stdio transport)"
echo ""
echo "=== Claude Code MCP Config ==="
echo 'Add to ~/.claude/settings.json or project settings:'
echo '  "mcpServers": {'
echo '    "zhen": {'
echo '      "command": "python3",'
echo '      "args": ["'$HOME'/tmp/unheaded/raft/zhen_mcp_server.py"]'
echo '    }'
echo '  }'
