# Zhen Pre-Demo Checklist

Run through this checklist before any live demonstration.

---

## System Checks

- [ ] **GPU available**: `nvidia-smi` shows GPU with free VRAM (need ~6GB for Mistral-7B)
- [ ] **Inference server running**: `curl http://localhost:20100/v1/models` returns 200 with model data
- [ ] **Web UI running**: `curl http://localhost:20103/` returns HTML containing "ZHEN"
- [ ] **RAG healthy**: `curl http://localhost:20103/health` shows `rag_ready: true`
- [ ] **Index loaded**: `curl http://localhost:20103/api/v1/stats` shows `index_vectors > 20000`
- [ ] **Python venv active**: `source ~/.venv/zhen/bin/activate`

## Startup Commands (if services are not running)

```bash
# 1. Activate venv
source ~/.venv/zhen/bin/activate

# 2. Start inference server (if not running)
# Check: curl http://localhost:20100/v1/models
# Start: (refer to llama.cpp server launch command with Mistral-7B model)

# 3. Start Zhen web app
cd ~/tmp/unheaded/raft
python zhen_app.py &

# 4. Verify
python scripts/04_integration_tests.py
```

## Performance Baseline

- [ ] **Warm up the model**: Send one test query before the demo to load weights into GPU cache
  ```bash
  curl -s -X POST http://localhost:20103/api/v1/query \
    -H "Content-Type: application/json" \
    -d '{"question": "What is Unheaded?"}' | python3 -m json.tool
  ```
- [ ] **Check response time**: First query may take 15-30s (cold), subsequent queries should be 5-15s
- [ ] **Verify answer quality**: Confirm the warm-up answer is coherent and mentions key concepts

## Browser Setup

- [ ] **Open browser tab**: http://localhost:20103
- [ ] **Font loaded**: JetBrains Mono should be rendering (check header)
- [ ] **Status indicator**: Shows "Online" (green) in the header
- [ ] **No console errors**: Open DevTools (F12) and check for errors
- [ ] **Window size**: Full screen or at least 960px wide for optimal layout

## Network

- [ ] **No VPN interference**: Ensure localhost ports are not being intercepted
- [ ] **No port conflicts**: Ports 20100 and 20103 are not used by other services
- [ ] **Firewall**: If presenting from a projector/external display, ensure localhost still resolves

## Fallback Plan

If the **inference server** goes down mid-demo:
1. Show `/api/v1/search` — vector search works without the LLM
2. Show `/api/v1/stats` — proves the 21K vector index is loaded
3. Show integration test results from the last successful run
4. Talk through the architecture using DEMO_SCRIPT.md

If the **web UI** goes down mid-demo:
1. Use `curl` commands directly against the API
2. Pipe output through `python3 -m json.tool` for readable formatting

If **response quality** is poor:
1. Rephrase the question with more specific terms
2. Fall back to the curated questions in DEMO_QUESTIONS.md
3. Use `/api/v1/search` to show relevant chunks, then explain what the LLM would do

## Final Verification (5 minutes before demo)

```bash
source ~/.venv/zhen/bin/activate
cd ~/tmp/unheaded/raft
python scripts/04_integration_tests.py
```

All 10 tests should pass. If any fail, check the specific service and restart as needed.
