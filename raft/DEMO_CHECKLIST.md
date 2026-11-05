# Zhen Pre-Demo Checklist

Run through this checklist before any live demonstration.

---

## System Checks

- [ ] **GPU available**: `rocm-smi --showmeminfo vram` shows room for the qwen-coder-7b GGUF
- [ ] **Inference server running**: `curl http://localhost:8081/v1/models` returns 200 with model data
- [ ] **Retrieval running**: `curl http://localhost:20110/api/health` returns 200 from vor
- [ ] **Web UI running**: `curl http://localhost:20103/` returns HTML containing "Zhenai"
- [ ] **RAG healthy**: `curl http://localhost:20103/health` shows `rag_ready: true`
- [ ] **Backends reported**: `curl http://localhost:20103/api/v1/stats` shows `vor_health` filled in and `inference_model` naming the loaded model
- [ ] **Python venv active**: `source ~/.venv/zhen/bin/activate`

## Startup Commands (if services are not running)

```bash
# 1. Activate venv
source ~/.venv/zhen/bin/activate

# 2. Check the backends. start-zhen.sh does not launch them: with one down it
#    stops and prints how to bring it up.
curl -s http://localhost:8081/v1/models
curl -s http://localhost:20110/api/health

# 3. Start zhen-agentd and the web app. Chat answers 503 without the daemon.
cd ~/tmp/unheaded/raft
./start-zhen.sh -gated

# 4. Verify: rag_ready is true
curl -s http://localhost:20103/health
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
- [ ] **No port conflicts**: Ports 8081, 20103, 20110 and 20111 are not used by other services
- [ ] **Firewall**: If presenting from a projector/external display, ensure localhost still resolves

## Fallback Plan

If the **inference server** goes down mid-demo:
1. Show `/api/v1/search` — retrieval through vor works without the LLM
2. Show `/api/v1/stats` — names the retrieval backend and shows its health
3. Talk through the architecture using DEMO_SCRIPT.md

If the **web UI** goes down mid-demo:
1. Use `curl` commands directly against the API
2. Pipe output through `python3 -m json.tool` for readable formatting

If **response quality** is poor:
1. Rephrase the question with more specific terms
2. Fall back to the curated questions in DEMO_QUESTIONS.md
3. Use `/api/v1/search` to show relevant chunks, then explain what the LLM would do

## Final Verification (5 minutes before demo)

```bash
curl -s http://localhost:8081/v1/models
curl -s http://localhost:20110/api/health
curl -s http://localhost:20103/health
curl -s http://localhost:20103/api/v1/stats
```

Each should answer as System Checks above says. If one does not, check that service and restart it as needed. The ten-test integration script moved to `unheaded-forge` with the rest of the training work.
