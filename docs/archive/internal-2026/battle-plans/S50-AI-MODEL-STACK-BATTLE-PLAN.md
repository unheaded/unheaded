# S50: AI MODEL STACK — FIRST "HEAD" IN ARMOR

**Date**: 2026-03-07
**Sprint**: S50 — vLLM + DeepSeek-R1 + Qwen 2.5 Coder as first application in Unheaded armor
**Prerequisite**: S49 complete (Protocol API operational)
**Target**: AI inference running inside Unheaded's suit of armor, fully traced and observed
**Estimated Duration**: ~10-14 hours
**Agent Strategy**: Phase 0→1→2 sequential, Phase 3-5 parallelizable, Phase 6→7 sequential
**Commit Cadence**: Every 5 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## PHASE 0: ENVIRONMENT & HARDWARE VALIDATION (Steps 1-15)

- [ ] **Step 1** [HARDWARE] ~2m: **Verify AMD ROCm installation on gaming desktop**
  ```bash
  rocminfo | grep -A 20 "RX 7700 XT"
  ```
  - If pass → Step 2
  - If fail → Step 1D [DEBUG_ROCM]

- [ ] **Step 1D** [DEBUG_ROCM] ~5m: **Debug ROCm installation**
  ```bash
  apt list --installed | grep -i rocm
  sudo apt update && sudo apt install -y rocm-core rocm-hip-sdk
  ```
  - If pass → Step 1
  - If fail → SKIP (manual ROCm reinstall required, escalate)

- [ ] **Step 2** [HARDWARE] ~2m: **Run rocminfo to confirm RX 7700 XT detected**
  ```bash
  rocminfo
  ```
  - Expected output: device name RX 7700 XT, compute capability RDNA2
  - If pass → Step 3
  - If fail → Step 2D [DEBUG_GPU]

- [ ] **Step 2D** [DEBUG_GPU] ~5m: **Debug GPU detection**
  ```bash
  lspci | grep AMD
  sudo dmesg | grep -i amd
  ```
  - If pass → Step 2
  - If fail → SKIP (hardware issue, escalate)

- [ ] **Step 3** [HARDWARE] ~2m: **Verify 12GB VRAM available**
  ```bash
  rocminfo | grep "Memory:"
  ```
  - Expected: 12000+ MB free
  - If pass → Step 4
  - If fail → Step 3D [DEBUG_VRAM]

- [ ] **Step 3D** [DEBUG_VRAM] ~3m: **Check VRAM usage**
  ```bash
  rocm-smi --showmeminfo
  ```
  - If pass → Step 3
  - If fail → SKIP (insufficient VRAM, cannot proceed)

- [ ] **Step 4** [DOCKER] ~3m: **Verify Docker with GPU passthrough capability**
  ```bash
  docker run --rm --device=/dev/kfd --device=/dev/dri rocm/rocm-terminal rocm-smi
  ```
  - Expected: rocm-smi output showing GPU
  - If pass → Step 5
  - If fail → Step 4D [DEBUG_DOCKER]

- [ ] **Step 4D** [DEBUG_DOCKER] ~5m: **Debug Docker GPU access**
  ```bash
  sudo usermod -aG render $(whoami)
  sudo usermod -aG video $(whoami)
  docker run --rm --device=/dev/kfd --device=/dev/dri rocm/rocm-terminal rocm-smi
  ```
  - If pass → Step 4
  - If fail → SKIP (Docker GPU passthrough broken, escalate)

- [ ] **Step 5** [STORAGE] ~2m: **Verify 2TB HDD mounted for model weights**
  ```bash
  mount | grep 2TB
  df -h | grep 2TB
  ```
  - Expected: /mnt/models or similar with ~2000GB available
  - If pass → Step 6
  - If fail → Step 5D [DEBUG_HDD]

- [ ] **Step 5D** [DEBUG_HDD] ~5m: **Debug HDD mounting**
  ```bash
  lsblk
  sudo mount /dev/sdX1 /mnt/models
  mkdir -p /mnt/models/weights
  chmod 755 /mnt/models/weights
  ```
  - If pass → Step 5
  - If fail → SKIP (HDD not available, escalate)

- [ ] **Step 6** [STORAGE] ~2m: **Verify 1TB NVMe for swap/overflow**
  ```bash
  mount | grep nvme
  df -h | grep nvme
  ```
  - Expected: /mnt/nvme with ~1000GB available
  - If pass → Step 7
  - If fail → Step 6D [DEBUG_NVME]

- [ ] **Step 6D** [DEBUG_NVME] ~5m: **Debug NVMe mounting**
  ```bash
  lsblk
  sudo mount /dev/nvme0n1p1 /mnt/nvme
  mkdir -p /mnt/nvme/swap
  chmod 755 /mnt/nvme/swap
  ```
  - If pass → Step 6
  - If fail → SKIP (NVMe not available, escalate)

- [ ] **Step 7** [NETWORK] ~3m: **Verify bare metal server reachable**
  ```bash
  ping <BARE_METAL_IP> -c 3
  ssh user@<BARE_METAL_IP> 'uname -a'
  ```
  - Expected: successful ping and SSH
  - If pass → Step 8
  - If fail → Step 7D [DEBUG_NETWORK]

- [ ] **Step 7D** [DEBUG_NETWORK] ~5m: **Debug network connectivity**
  ```bash
  ip addr show
  route -n
  netstat -tuln | grep -E ':(22|20000)'
  ```
  - If pass → Step 7
  - If fail → SKIP (network broken, escalate)

- [ ] **Step 8** [GIT] ~2m: **Create branch s50-ai-model-stack**
  ```bash
  cd /path/to/unheaded-repo
  git branch s50-ai-model-stack
  git checkout s50-ai-model-stack
  git log --oneline -1
  ```
  - If pass → PHASE 0 COMPLETE
  - If fail → SKIP (git broken, escalate)

---

## PHASE 1: vLLM + MODEL DEPLOYMENT (Steps 9-34)

- [ ] **Step 9** [VLLM] ~3m: **Pull vLLM Docker image with ROCm support**
  ```bash
  docker pull vllm/vllm-openai:latest-rocm
  docker images | grep vllm
  ```
  - Expected: vllm/vllm-openai image with rocm tag
  - If pass → Step 10
  - If fail → Step 9D [DEBUG_DOCKER_PULL]

- [ ] **Step 9D** [DEBUG_DOCKER_PULL] ~5m: **Debug Docker image pull**
  ```bash
  docker pull vllm/vllm-openai:v0.5.0-rocm
  docker pull rocm/rocm-terminal:latest
  ```
  - If pass → Step 9
  - If fail → SKIP (Docker registry inaccessible, escalate)

- [ ] **Step 10** [COMPOSE] ~5m: **Create docker-compose.yml for AI stack**
  ```bash
  cat > /path/to/docker-compose.yml << 'EOF'
version: '3.8'
services:
  vllm:
    image: vllm/vllm-openai:latest-rocm
    container_name: ai-vllm
    devices:
      - /dev/kfd
      - /dev/dri
    volumes:
      - /mnt/models/weights:/root/.cache/huggingface/hub
      - /mnt/nvme/swap:/tmp/swap
    ports:
      - "20100:8000"
    environment:
      - HSA_OVERRIDE_GFX_VERSION=gfx1102
      - HF_TOKEN=${HF_TOKEN}
    command: >
      python -m vllm.entrypoints.openai.api_server
      --model meta-llama/Llama-2-7b-chat-hf
      --gpu-memory-utilization 0.9
      --max-model-len 2048
      --dtype auto
      - If pass → Step 11
      - If fail → Step 10D [DEBUG_COMPOSE]

- [ ] **Step 10D** [DEBUG_COMPOSE] ~3m: **Debug docker-compose syntax**
  ```bash
  docker-compose config
  docker-compose validate
  ```
  - If pass → Step 10
  - If fail → SKIP (YAML syntax error, fix manually)

- [ ] **Step 11** [MODELS] ~15m: **Download DeepSeek-R1 7B distilled (Q4_K_M) to 2TB HDD**
  ```bash
  mkdir -p /mnt/models/weights/deepseek-r1-7b
  cd /mnt/models/weights/deepseek-r1-7b
  # Using llama.cpp GGUF format
  wget https://huggingface.co/deepseek-ai/deepseek-r1-distill-qwen-7b-gguf/resolve/main/deepseek-r1-distill-qwen-7b-q4_k_m.gguf
  ```
  - Expected: ~4.5GB file downloaded
  - If pass → Step 12
  - If fail → Step 11D [DEBUG_DL]

- [ ] **Step 11D** [DEBUG_DL] ~10m: **Debug model download**
  ```bash
  curl -I https://huggingface.co/deepseek-ai/deepseek-r1-distill-qwen-7b
  df -h /mnt/models/weights
  free -h
  ```
  - If pass → Step 11
  - If fail → SKIP (download failed, escalate)

- [ ] **Step 12** [MODELS] ~15m: **Download Qwen 2.5 Coder 7B (Q4_K_M) to 2TB HDD**
  ```bash
  mkdir -p /mnt/models/weights/qwen-coder-7b
  cd /mnt/models/weights/qwen-coder-7b
  wget https://huggingface.co/Qwen/Qwen2.5-Coder-7B-gguf/resolve/main/qwen2.5-coder-7b-q4_k_m.gguf
  ```
  - Expected: ~4.5GB file downloaded
  - If pass → Step 13
  - If fail → Step 12D [DEBUG_DL]

- [ ] **Step 12D** [DEBUG_DL] ~10m: **Debug second model download**
  ```bash
  curl -I https://huggingface.co/Qwen/Qwen2.5-Coder-7B
  df -h /mnt/models/weights
  ```
  - If pass → Step 12
  - If fail → SKIP (download failed, escalate)

- [ ] **Step 13** [VLLM] ~5m: **Configure vLLM server: model path, GPU memory fraction, max batch size**
  ```bash
  cat > /path/to/vllm-config.yaml << 'EOF'
model_dir: /mnt/models/weights
gpu_memory_utilization: 0.85
max_model_len: 2048
max_batch_size: 4
tensor_parallel_size: 1
dtype: auto
enable_lora: false
EOF
  ```
  - If pass → Step 14
  - If fail → SKIP (config syntax error)

- [ ] **Step 14** [VLLM] ~8m: **Test vLLM inference endpoint (single prompt)**
  ```bash
  docker-compose up -d vllm
  sleep 10
  curl -X POST http://localhost:20100/v1/completions \
    -H "Content-Type: application/json" \
    -d '{
      "model": "meta-llama/Llama-2-7b",
      "prompt": "What is 2+2?",
      "max_tokens": 50
    }'
  ```
  - Expected: JSON response with completion
  - If pass → Step 15
  - If fail → Step 14D [DEBUG_VLLM]

- [ ] **Step 14D** [DEBUG_VLLM] ~10m: **Debug vLLM inference**
  ```bash
  docker logs ai-vllm | tail -50
  docker exec ai-vllm nvidia-smi
  rocm-smi --showmeminfo
  ```
  - If pass → Step 14
  - If fail → SKIP (vLLM crash, escalate)

- [ ] **Step 15** [WEBUI] ~5m: **Set up Open WebUI container (connected to vLLM API)**
  ```bash
  cat >> /path/to/docker-compose.yml << 'EOF'
  open-webui:
    image: ghcr.io/open-webui/open-webui:latest
    container_name: ai-webui
    ports:
      - "20101:8080"
    environment:
      - OPENAI_API_BASE_URL=http://vllm:8000/v1
      - OPENAI_API_KEY=sk-local-test
    depends_on:
      - vllm
EOF
  docker-compose up -d open-webui
  sleep 5
  curl http://localhost:20101
  ```
  - Expected: HTTP 200 or redirect
  - If pass → Step 16
  - If fail → Step 15D [DEBUG_WEBUI]

- [ ] **Step 15D** [DEBUG_WEBUI] ~5m: **Debug Open WebUI**
  ```bash
  docker logs ai-webui | tail -30
  docker ps -a | grep webui
  ```
  - If pass → Step 15
  - If fail → SKIP (WebUI broken, skip to Step 17)

- [ ] **Step 16** [QDRANT] ~5m: **Deploy Qdrant vector DB container**
  ```bash
  cat >> /path/to/docker-compose.yml << 'EOF'
  qdrant:
    image: qdrant/qdrant:latest
    container_name: ai-qdrant
    ports:
      - "20102:6333"
    volumes:
      - qdrant-storage:/qdrant/storage
    environment:
      - QDRANT_API_KEY=test-key
volumes:
  qdrant-storage:
EOF
  docker-compose up -d qdrant
  sleep 5
  curl http://localhost:20102/health
  ```
  - Expected: HTTP 200 with health status
  - If pass → Step 17
  - If fail → Step 16D [DEBUG_QDRANT]

- [ ] **Step 16D** [DEBUG_QDRANT] ~5m: **Debug Qdrant**
  ```bash
  docker logs ai-qdrant | tail -20
  docker ps -a | grep qdrant
  ```
  - If pass → Step 16
  - If fail → SKIP (Qdrant broken, continue to Step 18)

- [ ] **Step 17** [EMBED] ~5m: **Deploy BGE-M3 embedding service**
  ```bash
  cat >> /path/to/docker-compose.yml << 'EOF'
  embedder:
    image: ghcr.io/huggingface/text-embeddings-inference:latest
    container_name: ai-embedder
    ports:
      - "20103:80"
    environment:
      - MODEL_ID=BAAI/bge-m3
      - CUDA_VISIBLE_DEVICES=0
    volumes:
      - /mnt/models/weights:/root/.cache
EOF
  docker-compose up -d embedder
  sleep 10
  curl -X POST http://localhost:20103/embed \
    -H "Content-Type: application/json" \
    -d '{"inputs":["test sentence"]}'
  ```
  - Expected: JSON with embeddings array
  - If pass → Step 18
  - If fail → Step 17D [DEBUG_EMBED]

- [ ] **Step 17D** [DEBUG_EMBED] ~5m: **Debug embedder service**
  ```bash
  docker logs ai-embedder | tail -30
  docker ps -a | grep embed
  ```
  - If pass → Step 17
  - If fail → SKIP (embedder broken, continue to Step 19)

- [ ] **Step 18** [EMBED] ~5m: **Test embedding generation**
  ```bash
  curl -X POST http://localhost:20103/embed \
    -H "Content-Type: application/json" \
    -d '{"inputs":["machine learning","artificial intelligence","deep learning"]}'
  ```
  - Expected: 3 embeddings with 1024+ dimensions
  - If pass → Step 19
  - If fail → Step 18D [DEBUG_EMBED_TEST]

- [ ] **Step 18D** [DEBUG_EMBED_TEST] ~3m: **Debug embedding test**
  ```bash
  curl http://localhost:20103/health
  docker logs ai-embedder | tail -10
  ```
  - If pass → Step 18
  - If fail → SKIP (continue)

- [ ] **Step 19** [QDRANT] ~5m: **Test vector store/search**
  ```bash
  curl -X PUT http://localhost:20102/collections/test-collection \
    -H "Content-Type: application/json" \
    -d '{
      "name": "test-collection",
      "vectors": {"size": 1024, "distance": "Cosine"}
    }'
  # Upsert vectors
  curl -X PUT http://localhost:20102/collections/test-collection/points \
    -H "Content-Type: application/json" \
    -d '{
      "points": [
        {"id": 1, "vector": [0.1, 0.2, ...], "payload": {"text": "test"}}
      ]
    }'
  ```
  - Expected: HTTP 200 on upsert
  - If pass → Step 20
  - If fail → Step 19D [DEBUG_QDRANT_WRITE]

- [ ] **Step 19D** [DEBUG_QDRANT_WRITE] ~3m: **Debug Qdrant write**
  ```bash
  curl http://localhost:20102/collections
  docker logs ai-qdrant | tail -20
  ```
  - If pass → Step 19
  - If fail → SKIP (continue)

- [ ] **Step 20** [CONFIG] ~3m: **Configure model switching (DeepSeek for reasoning, Qwen for code)**
  ```bash
  cat > /path/to/model-router.yaml << 'EOF'
models:
  reasoning:
    name: deepseek-r1-7b-distill
    type: reasoning
    max_tokens: 2048
    priority: high
  coding:
    name: qwen-2.5-coder-7b
    type: code-generation
    max_tokens: 4096
    priority: high
routing:
  - pattern: "^(explain|reason|think|analyze)"
    model: reasoning
  - pattern: "^(code|function|class|def|generate.*code)"
    model: coding
  - default: reasoning
EOF
  ```
  - If pass → Step 21
  - If fail → SKIP (config syntax error)

- [ ] **Step 21** [HEALTH] ~5m: **Health checks for all AI services**
  ```bash
  docker-compose ps
  curl -s http://localhost:20100/health | jq .
  curl -s http://localhost:20101/health || echo "WebUI up"
  curl -s http://localhost:20102/health | jq .
  curl -s http://localhost:20103/health | jq .
  ```
  - Expected: all services healthy or running
  - If pass → Step 22 (COMMIT)
  - If fail → Step 21D [DEBUG_HEALTH]

- [ ] **Step 21D** [DEBUG_HEALTH] ~5m: **Debug health checks**
  ```bash
  docker ps -a
  docker-compose logs --tail=20
  ```
  - If pass → Step 21
  - If fail → SKIP (continue to commit)

- [ ] **Step 22** [COMMIT] ~2m: **Commit checkpoint: Phase 1 complete**
  ```bash
  git add -A
  git commit -m "S50 Phase 1: vLLM + DeepSeek-R1 + Qwen Coder deployment complete"
  git log --oneline -3
  ```
  - If pass → PHASE 1 COMPLETE
  - If fail → SKIP (git issue)

---

## PHASE 2: UNHEADED ARMOR INTEGRATION (Steps 23-52)

- [ ] **Step 23** [SOPHIA] ~5m: **Register AI service in Sophia dictionary (service discovery)**
  ```bash
  cat > /path/to/ai-service-registration.json << 'EOF'
{
  "service_name": "ai-model-stack",
  "instance_id": "ai-001",
  "hostname": "gaming-desktop",
  "ports": {
    "vllm_api": 20100,
    "webui": 20101,
    "qdrant_db": 20102,
    "embedder": 20103
  },
  "health_check_url": "http://localhost:20100/health",
  "tags": ["ai", "inference", "head-application"],
  "metadata": {
    "model_reasoning": "deepseek-r1-7b-distill",
    "model_coding": "qwen-2.5-coder-7b",
    "vram_total": "12GB",
    "gpu": "RX 7700 XT"
  }
}
EOF
  # Register via Sophia API (assuming S49 Protocol API available)
  curl -X POST http://<SOPHIA_ADDR>/v1/sophia/dictionaries/services \
    -H "Content-Type: application/json" \
    -d @ai-service-registration.json
  ```
  - Expected: HTTP 201 with service ID
  - If pass → Step 24
  - If fail → Step 23D [DEBUG_SOPHIA]

- [ ] **Step 23D** [DEBUG_SOPHIA] ~5m: **Debug Sophia registration**
  ```bash
  curl http://<SOPHIA_ADDR>/v1/sophia/health
  curl http://<SOPHIA_ADDR>/v1/sophia/dictionaries
  ```
  - If pass → Step 23
  - If fail → SKIP (S49 Protocol API not running, escalate)

- [ ] **Step 24** [CONFIG] ~3m: **Assign ports: 20100 (vLLM API), 20101 (Open WebUI), 20102 (Qdrant), 20103 (BGE-M3)**
  ```bash
  cat > /path/to/port-allocation.txt << 'EOF'
Port Allocation for S50 AI Stack:
- 20100: vLLM OpenAI API (inference engine)
- 20101: Open WebUI (web interface)
- 20102: Qdrant Vector DB (knowledge store)
- 20103: BGE-M3 Embeddings (semantic search)
Total range: 20100-20199 reserved for Applications tier per S41 spec
EOF
  grep -r "20100\|20101\|20102\|20103" /path/to/config/ || echo "No conflicts"
  ```
  - If pass → Step 25
  - If fail → SKIP (port conflict, resolve manually)

- [ ] **Step 25** [CONFIG] ~5m: **Create service config YAML for AI stack**
  ```bash
  cat > /path/to/unheaded/services/ai-stack-service.yaml << 'EOF'
apiVersion: services.unheaded.io/v1
kind: Service
metadata:
  name: ai-model-stack
  namespace: unheaded
spec:
  serviceName: ai-model-stack
  instanceId: ai-001
  tier: applications
  portRange:
    start: 20100
    end: 20103
  containers:
    vllm:
      image: vllm/vllm-openai:latest-rocm
      port: 20100
      resources:
        gpu: RX7700XT
        vram: 9GB
        memory: 2GB
    webui:
      image: ghcr.io/open-webui/open-webui:latest
      port: 20101
      resources:
        memory: 1GB
    qdrant:
      image: qdrant/qdrant:latest
      port: 20102
      resources:
        memory: 2GB
        storage: 500GB
    embedder:
      image: ghcr.io/huggingface/text-embeddings-inference:latest
      port: 20103
      resources:
        gpu: RX7700XT
        memory: 2GB
EOF
  ```
  - If pass → Step 26
  - If fail → SKIP (YAML syntax error)

- [ ] **Step 26** [SHIELD] ~5m: **Configure Shield WAF rules: strict ingress/egress for AI service**
  ```bash
  cat > /path/to/shield-ai-rules.yaml << 'EOF'
apiVersion: shield.unheaded.io/v1
kind: FirewallPolicy
metadata:
  name: ai-model-stack-policy
spec:
  service: ai-model-stack
  rules:
    ingress:
      - from: dashboard
        to_port: 20100
        action: ALLOW
      - from: dashboard
        to_port: 20101
        action: ALLOW
      - from: api-gateway
        to_port: 20100
        action: ALLOW
      - from: "*"
        action: DENY
    egress:
      - to: qdrant
        port: 20102
        action: ALLOW
      - to: embedder
        port: 20103
        action: ALLOW
      - to: "*"
        action: DENY
EOF
  ```
  - If pass → Step 27
  - If fail → SKIP (config error)

- [ ] **Step 27** [SHIELD] ~3m: **Only allowed flows: dashboard→AI, API→AI, AI→Qdrant internal**
  ```bash
  cat >> /path/to/shield-ai-rules.yaml << 'EOF'
  allowedFlows:
    - source: dashboard
      destination: ai-model-stack
      ports: [20100, 20101]
    - source: api-gateway
      destination: ai-model-stack
      ports: [20100]
    - source: ai-model-stack
      destination: qdrant
      ports: [20102]
    - source: ai-model-stack
      destination: embedder
      ports: [20103]
  deniedFlows:
    - source: "*"
      destination: "*"
      comment: "Implicit deny all"
EOF
  ```
  - If pass → Step 28
  - If fail → SKIP (continue)

- [ ] **Step 28** [EBPF] ~5m: **eBPF trace attachment for AI inference requests**
  ```bash
  cat > /path/to/ebpf-ai-trace.c << 'EOF'
#include <uapi/linux/ptrace.h>
#include <net/sock.h>
#include <bcc/proto.h>

BPF_PERF_OUTPUT(events);

struct event_t {
  u64 timestamp;
  u32 saddr, daddr;
  u16 sport, dport;
  u64 bytes_sent;
  char model[32];
};

TRACEPOINT_PROBE(tcp, tcp_retransmit_skb) {
  struct event_t event = {};
  event.timestamp = bpf_ktime_get_ns();
  bpf_probe_read_kernel(&event.saddr, sizeof(u32), &args->skaddr->__sk_common.skc_rcv_saddr);
  bpf_probe_read_kernel(&event.daddr, sizeof(u32), &args->skaddr->__sk_common.skc_daddr);
  bpf_probe_read_kernel(&event.sport, sizeof(u16), &args->skaddr->__sk_common.skc_num);
  bpf_probe_read_kernel(&event.dport, sizeof(u16), &args->skaddr->__sk_common.skc_dport);
  events.perf_submit(ctx, &event, sizeof(event));
  return 0;
}
EOF
  ```
  - If pass → Step 29
  - If fail → SKIP (eBPF debug required, continue)

- [ ] **Step 29** [EBPF] ~5m: **Verify inference requests visible on dashboard Packet Flow tab**
  ```bash
  # Launch trace-collector (from Unheaded services)
  python3 /path/to/trace-collector/main.py \
    --ebpf-program /path/to/ebpf-ai-trace.c \
    --filter "port==20100" &

  sleep 2
  # Trigger inference request
  curl -X POST http://localhost:20100/v1/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"deepseek-r1-7b","prompt":"test","max_tokens":10}'

  # Check trace output
  ps aux | grep trace-collector
  ```
  - Expected: trace-collector running, capturing packets
  - If pass → Step 30
  - If fail → Step 29D [DEBUG_EBPF]

- [ ] **Step 29D** [DEBUG_EBPF] ~5m: **Debug eBPF tracing**
  ```bash
  dmesg | tail -20
  bpftool prog list
  bpftool map list
  ```
  - If pass → Step 29
  - If fail → SKIP (eBPF kernel support missing, escalate)

- [ ] **Step 30** [WOTAN] ~5m: **Wotan message bus integration: publish inference events**
  ```bash
  cat > /path/to/wotan-ai-publisher.py << 'EOF'
import grpc
from wotan_pb2 import Event, EventType

def publish_inference_event(model_name, status, latency_ms):
  event = Event(
    type=EventType.AI_INFERENCE,
    source="ai-model-stack",
    timestamp=int(time.time() * 1000),
    data={
      "model": model_name,
      "status": status,
      "latency_ms": latency_ms
    }
  )
  stub.PublishEvent(event)

@app.post("/v1/completions")
async def completion(request):
  start = time.time()
  result = await vllm_api.complete(request)
  latency = (time.time() - start) * 1000
  publish_inference_event(request.model, "success", latency)
  return result
EOF
  ```
  - If pass → Step 31
  - If fail → SKIP (Wotan integration, continue)

- [ ] **Step 31** [DASHBOARD] ~5m: **Dashboard: add AI inference metrics card**
  ```bash
  cat > /path/to/dashboard/metrics/ai-inference.json << 'EOF'
{
  "card_id": "ai-inference-metrics",
  "title": "AI Inference Metrics",
  "type": "metrics",
  "metrics": [
    {
      "name": "inference_latency_ms",
      "label": "Latency (ms)",
      "unit": "ms",
      "query": "histogram_quantile(0.95, ai_inference_latency_ms)"
    },
    {
      "name": "inference_throughput",
      "label": "Throughput (req/s)",
      "unit": "req/s",
      "query": "rate(ai_inference_total[1m])"
    },
    {
      "name": "inference_error_rate",
      "label": "Error Rate (%)",
      "unit": "%",
      "query": "rate(ai_inference_errors_total[1m]) * 100"
    }
  ],
  "refresh_interval": "5s"
}
EOF
  ```
  - If pass → Step 32
  - If fail → SKIP (dashboard config error)

- [ ] **Step 32** [PROTOCOL] ~5m: **Protocol API integration: AI uses /api/v1/monad/decode, /api/v1/sophia/dictionaries**
  ```bash
  cat > /path/to/ai-protocol-client.py << 'EOF'
import httpx

async def lookup_service(service_name):
  async with httpx.AsyncClient() as client:
    resp = await client.get(
      f"http://<UNHEADED_API>/api/v1/sophia/dictionaries/services/{service_name}"
    )
    return resp.json()

async def decode_monad(monad_packet):
  async with httpx.AsyncClient() as client:
    resp = await client.post(
      "http://<UNHEADED_API>/api/v1/monad/decode",
      json={"data": monad_packet}
    )
    return resp.json()

# Use in inference handler
@app.post("/v1/completions")
async def completion(request):
  monad = await lookup_service("ai-model-stack")
  # ... inference logic
EOF
  ```
  - If pass → Step 33
  - If fail → SKIP (Protocol API integration, continue)

- [ ] **Step 33** [TRACE] ~3m: **Verify trace-collector captures AI traffic**
  ```bash
  ps aux | grep trace-collector
  curl -s http://localhost:20100/v1/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"deepseek-r1-7b","prompt":"test","max_tokens":5}' &
  sleep 1
  # Check trace logs
  tail -30 /var/log/trace-collector.log | grep "20100"
  ```
  - Expected: packet traces logged with port 20100
  - If pass → Step 34
  - If fail → SKIP (continue)

- [ ] **Step 34** [ANAMNESIS] ~5m: **Verify Anamnesis events fire for inference start/complete**
  ```bash
  cat > /path/to/anamnesis-ai-events.yaml << 'EOF'
events:
  - id: ai_inference_start
    name: "AI Inference Started"
    source: ai-model-stack
    tags: [inference, ai]
  - id: ai_inference_complete
    name: "AI Inference Completed"
    source: ai-model-stack
    tags: [inference, ai]
  - id: ai_inference_error
    name: "AI Inference Error"
    source: ai-model-stack
    tags: [inference, ai, error]
EOF
  # Test event firing
  curl -X POST http://<UNHEADED_API>/api/v1/anamnesis/events \
    -H "Content-Type: application/json" \
    -d '{"event_id":"ai_inference_start","source":"ai-model-stack","timestamp":"2026-03-07T10:00:00Z"}'
  ```
  - Expected: HTTP 201
  - If pass → Step 35
  - If fail → SKIP (Anamnesis integration, continue)

- [ ] **Step 35** [SHIELD] ~5m: **Test locked-down network: attempts to reach unauthorized endpoints BLOCKED**
  ```bash
  # Attempt unauthorized access (should fail)
  curl -X POST http://localhost:20100/v1/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"test","prompt":"test","max_tokens":5}' \
    --connect-timeout 1 &

  sleep 1
  pkill -f "curl.*20100" || true

  # From unauthorized source (should be blocked by Shield)
  docker run --network host alpine:latest \
    curl -X POST http://localhost:20100/admin/shutdown 2>&1 | grep -i "connection\|refused\|timeout"
  ```
  - Expected: connection refused/timeout
  - If pass → Step 36
  - If fail → Step 35D [DEBUG_SHIELD]

- [ ] **Step 35D** [DEBUG_SHIELD] ~5m: **Debug Shield blocking**
  ```bash
  sudo iptables -L -n | grep 20100
  sudo ufw status
  docker ps | grep shield
  ```
  - If pass → Step 35
  - If fail → SKIP (Shield not active, continue)

- [ ] **Step 36** [COMMIT] ~2m: **Commit checkpoint: Phase 2 complete**
  ```bash
  git add -A
  git commit -m "S50 Phase 2: Unheaded armor integration, Shield, eBPF tracing, Wotan events"
  git log --oneline -3
  ```
  - If pass → PHASE 2 COMPLETE
  - If fail → SKIP (git issue)

---

## PHASE 3: NIXOS CONTAINER DEFINITIONS (Steps 37-45)

- [ ] **Step 37** [NIX] ~8m: **Create nix/containers/ai-vllm.nix**
  ```bash
  cat > /path/to/nix/containers/ai-vllm.nix << 'EOF'
{ config, pkgs, lib, ... }:

{
  containers.ai-vllm = {
    autoStart = true;
    ephemeral = false;
    privateNetwork = true;
    hostAddress = "192.168.200.1";
    localAddress = "192.168.200.2";
    config = { config, pkgs, ... }: {
      system.stateVersion = "23.11";
      services.openssh.enable = true;
      environment.systemPackages = with pkgs; [
        rocmPackages.llvm
        rocmPackages.rocm-core
        python311
        python311Packages.vllm
      ];
      systemd.services.vllm = {
        description = "vLLM OpenAI API Server";
        after = ["network.target"];
        wantedBy = ["multi-user.target"];
        serviceConfig = {
          Type = "simple";
          ExecStart = "${pkgs.python311Packages.vllm}/bin/python -m vllm.entrypoints.openai.api_server";
          User = "root";
          Restart = "always";
          RestartSec = 5;
        };
        environment = {
          HF_HOME = "/mnt/models/weights";
          HSA_OVERRIDE_GFX_VERSION = "gfx1102";
        };
      };
    };
  };
}
EOF
  ```
  - If pass → Step 38
  - If fail → SKIP (Nix syntax error)

- [ ] **Step 38** [NIX] ~5m: **Create nix/containers/ai-qdrant.nix**
  ```bash
  cat > /path/to/nix/containers/ai-qdrant.nix << 'EOF'
{ config, pkgs, lib, ... }:

{
  containers.ai-qdrant = {
    autoStart = true;
    ephemeral = false;
    privateNetwork = true;
    hostAddress = "192.168.201.1";
    localAddress = "192.168.201.2";
    config = { config, pkgs, ... }: {
      system.stateVersion = "23.11";
      services.qdrant = {
        enable = true;
        port = 6333;
        dataDir = "/var/lib/qdrant";
        apiKey = "test-key";
      };
    };
  };
}
EOF
  ```
  - If pass → Step 39
  - If fail → SKIP (Nix syntax error)

- [ ] **Step 39** [NIX] ~5m: **Create nix/containers/ai-webui.nix**
  ```bash
  cat > /path/to/nix/containers/ai-webui.nix << 'EOF'
{ config, pkgs, lib, ... }:

{
  containers.ai-webui = {
    autoStart = true;
    ephemeral = false;
    privateNetwork = true;
    hostAddress = "192.168.202.1";
    localAddress = "192.168.202.2";
    config = { config, pkgs, ... }: {
      system.stateVersion = "23.11";
      services.open-webui = {
        enable = true;
        port = 8080;
        openaiApiBase = "http://192.168.200.2:8000/v1";
        openaiApiKey = "sk-local-test";
      };
    };
  };
}
EOF
  ```
  - If pass → Step 40
  - If fail → SKIP (Nix syntax error)

- [ ] **Step 40** [NIX] ~5m: **Create nix/containers/ai-embedder.nix**
  ```bash
  cat > /path/to/nix/containers/ai-embedder.nix << 'EOF'
{ config, pkgs, lib, ... }:

{
  containers.ai-embedder = {
    autoStart = true;
    ephemeral = false;
    privateNetwork = true;
    hostAddress = "192.168.203.1";
    localAddress = "192.168.203.2";
    config = { config, pkgs, ... }: {
      system.stateVersion = "23.11";
      environment.systemPackages = with pkgs; [
        python311
        python311Packages.torch
        python311Packages.transformers
      ];
      systemd.services.embedder = {
        description = "BGE-M3 Text Embeddings Service";
        after = ["network.target"];
        wantedBy = ["multi-user.target"];
        serviceConfig = {
          Type = "simple";
          ExecStart = "${pkgs.python311}/bin/python -m text_embeddings_inference.server";
          User = "root";
        };
        environment = {
          MODEL_ID = "BAAI/bge-m3";
          HF_HOME = "/mnt/models/weights";
        };
      };
    };
  };
}
EOF
  ```
  - If pass → Step 41
  - If fail → SKIP (Nix syntax error)

- [ ] **Step 41** [NIX] ~5m: **NixOS module for AI stack configuration**
  ```bash
  cat > /path/to/nix/modules/ai-stack.nix << 'EOF'
{ config, pkgs, lib, options, ... }:

with lib;

{
  options.unheaded.ai-stack = {
    enable = mkEnableOption "AI Model Stack";
    models = mkOption {
      type = types.attrs;
      default = {
        reasoning = "deepseek-r1-7b-distill";
        coding = "qwen-2.5-coder-7b";
      };
    };
    vramGb = mkOption {
      type = types.int;
      default = 12;
    };
  };

  config = mkIf config.unheaded.ai-stack.enable {
    imports = [
      ./containers/ai-vllm.nix
      ./containers/ai-qdrant.nix
      ./containers/ai-webui.nix
      ./containers/ai-embedder.nix
    ];

    networking.nat.enable = true;
    networking.nat.forwardPorts = [
      { sourcePort = 20100; destination.port = 8000; }
      { sourcePort = 20101; destination.port = 8080; }
      { sourcePort = 20102; destination.port = 6333; }
      { sourcePort = 20103; destination.port = 80; }
    ];
  };
}
EOF
  ```
  - If pass → Step 42
  - If fail → SKIP (Nix syntax error)

- [ ] **Step 42** [NIX] ~3m: **Verify NixOS builds without errors**
  ```bash
  nix-build /path/to/nix/modules/ai-stack.nix --dry-run
  nixos-option unheaded.ai-stack.enable
  ```
  - Expected: no errors
  - If pass → PHASE 3 COMPLETE
  - If fail → Step 42D [DEBUG_NIX]

- [ ] **Step 42D** [DEBUG_NIX] ~5m: **Debug NixOS build**
  ```bash
  nix-build /path/to/nix/modules/ai-stack.nix 2>&1 | tail -30
  nix-shell -p nix-linter --run "nix-linter /path/to/nix/modules/"
  ```
  - If pass → Step 42
  - If fail → SKIP (Nix expertise required)

---

## PHASE 4: BARE METAL SERVER SETUP (Steps 43-58)

- [ ] **Step 43** [BAREMETAL] ~8m: **NixOS config for 4-core DDR3 machine**
  ```bash
  ssh user@<BARE_METAL_IP> 'cat > /etc/nixos/configuration.nix << "EOF"
{ config, pkgs, ... }:

{
  imports = [ <nixpkgs/nixos/modules/installer/cd-dvd/installation-cd-minimal.nix> ];

  system.stateVersion = "23.11";
  networking.hostName = "unheaded-baremetal";

  environment.systemPackages = with pkgs; [
    git
    docker
    grpcurl
    htop
    ethtool
  ];

  virtualisation.docker.enable = true;
  users.users.unheaded = {
    isSystemUser = true;
    group = "unheaded";
  };
  users.groups.unheaded = {};
}
EOF
  '
  ```
  - Expected: SSH success, config written
  - If pass → Step 44
  - If fail → Step 43D [DEBUG_SSH]

- [ ] **Step 43D** [DEBUG_SSH] ~3m: **Debug SSH connection**
  ```bash
  ssh -v user@<BARE_METAL_IP> 'uname -a'
  ```
  - If pass → Step 43
  - If fail → SKIP (network issue, escalate)

- [ ] **Step 44** [BAREMETAL] ~15m: **Deploy Unheaded daemon + control plane on bare metal**
  ```bash
  ssh user@<BARE_METAL_IP> << 'EOF'
  cd /home/user
  git clone https://github.com/unheaded/unheaded.git
  cd unheaded
  git checkout s50-ai-model-stack

  # Build and deploy control plane
  docker-compose -f docker-compose.baremetal.yml up -d
  sleep 10
  docker ps
EOF
  ```
  - Expected: Unheaded services running on bare metal
  - If pass → Step 45
  - If fail → Step 44D [DEBUG_BAREMETAL]

- [ ] **Step 44D** [DEBUG_BAREMETAL] ~5m: **Debug bare metal deployment**
  ```bash
  ssh user@<BARE_METAL_IP> 'docker ps -a'
  ssh user@<BARE_METAL_IP> 'docker logs <container_id> | tail -30'
  ```
  - If pass → Step 44
  - If fail → SKIP (deployment issue, escalate)

- [ ] **Step 45** [WOTAN] ~8m: **Deploy Wotan, trace-collector, dashboard, kanban, gateway**
  ```bash
  ssh user@<BARE_METAL_IP> << 'EOF'
  docker run -d \
    --name wotan \
    -p 50051:50051 \
    ghcr.io/unheaded/wotan:latest \
    --listen=0.0.0.0:50051

  docker run -d \
    --name trace-collector \
    -p 8889:8889 \
    -v /var/log:/var/log \
    ghcr.io/unheaded/trace-collector:latest

  docker run -d \
    --name dashboard \
    -p 3000:3000 \
    ghcr.io/unheaded/dashboard:latest \
    --wotan-addr=wotan:50051

  docker ps | grep -E "wotan|trace|dashboard"
EOF
  ```
  - Expected: services running
  - If pass → Step 46
  - If fail → Step 45D [DEBUG_SERVICES]

- [ ] **Step 45D** [DEBUG_SERVICES] ~5m: **Debug service deployment**
  ```bash
  ssh user@<BARE_METAL_IP> 'docker logs wotan | tail -20'
  ssh user@<BARE_METAL_IP> 'docker logs trace-collector | tail -20'
  ```
  - If pass → Step 45
  - If fail → SKIP (escalate)

- [ ] **Step 46** [NETWORK] ~10m: **EVPN-VXLAN tunnel configuration between gaming desktop and bare metal**
  ```bash
  # On gaming desktop
  cat > /path/to/vxlan-config.sh << 'EOF'
#!/bin/bash
BAREMETAL_IP="<BARE_METAL_IP>"
GAMING_IP="<GAMING_DESKTOP_IP>"
VNI=100

# Create VXLAN interface
ip link add vxlan100 type vxlan id $VNI remote $BAREMETAL_IP local $GAMING_IP dstport 4789
ip addr add 192.168.100.1/24 dev vxlan100
ip link set vxlan100 up

# Enable forwarding
sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv4.conf.all.rp_filter=0

# BFD for fast failover
bfd_session=$(echo "
session {
  ip_address = $BAREMETAL_IP;
  interface = eth0;
  detect_mult = 3;
  desired_min_tx_interval = 300;
  required_min_rx_interval = 300;
}")

echo "$bfd_session" > /etc/bfd.conf
systemctl restart bfd || echo "BFD not available"

ip link show vxlan100
EOF
  chmod +x /path/to/vxlan-config.sh
  sudo bash /path/to/vxlan-config.sh
  ```
  - Expected: VXLAN interface up
  - If pass → Step 47
  - If fail → Step 46D [DEBUG_VXLAN]

- [ ] **Step 46D** [DEBUG_VXLAN] ~5m: **Debug VXLAN**
  ```bash
  ip link show | grep vxlan
  ip addr show | grep 192.168.100
  ping 192.168.100.2 -c 3
  ```
  - If pass → Step 46
  - If fail → SKIP (VXLAN kernel support issue, continue)

- [ ] **Step 47** [WOTAN] ~5m: **Verify Wotan gRPC works cross-host**
  ```bash
  # From gaming desktop
  grpcurl -plaintext <BARE_METAL_IP>:50051 list
  # Should list Wotan service methods
  ```
  - Expected: gRPC services listed
  - If pass → Step 48
  - If fail → Step 47D [DEBUG_GRPC]

- [ ] **Step 47D** [DEBUG_GRPC] ~5m: **Debug gRPC connectivity**
  ```bash
  nc -zv <BARE_METAL_IP> 50051
  telnet <BARE_METAL_IP> 50051
  ssh user@<BARE_METAL_IP> 'netstat -tuln | grep 50051'
  ```
  - If pass → Step 47
  - If fail → SKIP (firewall issue)

- [ ] **Step 48** [EBPF] ~5m: **Verify eBPF runs on bare metal**
  ```bash
  ssh user@<BARE_METAL_IP> << 'EOF'
  bpftool prog list
  bpftool map list
  # Should show loaded BPF programs
EOF
  ```
  - Expected: BPF programs listed
  - If pass → Step 49
  - If fail → Step 48D [DEBUG_EBPF_BM]

- [ ] **Step 48D** [DEBUG_EBPF_BM] ~3m: **Debug eBPF on bare metal**
  ```bash
  ssh user@<BARE_METAL_IP> 'uname -r | grep -i 5\.'
  ssh user@<BARE_METAL_IP> 'cat /boot/config-$(uname -r) | grep CONFIG_BPF'
  ```
  - If pass → Step 48
  - If fail → SKIP (kernel too old, escalate)

- [ ] **Step 49** [SPLIT-BRAIN] ~5m: **Split-brain verification: AI on gaming box, services on bare metal**
  ```bash
  # From gaming desktop
  curl -s http://localhost:20100/health | jq .
  # Should return vLLM health

  # From bare metal (via gRPC)
  grpcurl -plaintext <BARE_METAL_IP>:50051 wotan.v1.Wotan/Health
  # Should return healthy

  # Verify AI doesn't run on bare metal
  ssh user@<BARE_METAL_IP> 'docker ps | grep -i vllm' || echo "vLLM not on baremetal (correct)"
  ```
  - Expected: AI on gaming, control plane on bare metal
  - If pass → PHASE 4 COMPLETE
  - If fail → SKIP (architecture issue, escalate)

---

## PHASE 5: NETWORK UNDERLAY (Steps 50-59)

- [ ] **Step 50** [BGP] ~8m: **eBGP configuration between hosts (RFC 7938)**
  ```bash
  cat > /path/to/frr-gaming.conf << 'EOF'
frr defaults traditional
hostname unheaded-gaming
password zebra
!
router bgp 65001
 bgp router-id 192.168.0.1
 neighbor 192.168.0.2 remote-as 65002
 neighbor 192.168.0.2 description "Bare Metal Server"
 !
 address-family ipv4 unicast
  network 192.168.100.0/24
  neighbor 192.168.0.2 activate
  neighbor 192.168.0.2 soft-reconfiguration inbound
 exit-address-family
!
line vty
!
EOF

cat > /path/to/frr-baremetal.conf << 'EOF'
frr defaults traditional
hostname unheaded-baremetal
password zebra
!
router bgp 65002
 bgp router-id 192.168.0.2
 neighbor 192.168.0.1 remote-as 65001
 neighbor 192.168.0.1 description "Gaming Desktop"
 !
 address-family ipv4 unicast
  network 192.168.101.0/24
  neighbor 192.168.0.1 activate
  neighbor 192.168.0.1 soft-reconfiguration inbound
 exit-address-family
!
line vty
!
EOF
  ```
  - If pass → Step 51
  - If fail → SKIP (BGP config error)

- [ ] **Step 51** [BGP] ~5m: **BFD sub-second failover**
  ```bash
  cat >> /path/to/frr-gaming.conf << 'EOF'
!
interface eth0
 ip address 192.168.0.1/30
!
bfd
 peer 192.168.0.2
  interval 300
  detect-multiplier 3
!
EOF

cat >> /path/to/frr-baremetal.conf << 'EOF'
!
interface eth0
 ip address 192.168.0.2/30
!
bfd
 peer 192.168.0.1
  interval 300
  detect-multiplier 3
!
EOF
  ```
  - If pass → Step 52
  - If fail → SKIP (BFD config)

- [ ] **Step 52** [BGP] ~5m: **ECMP paths if multiple interfaces**
  ```bash
  cat >> /path/to/frr-gaming.conf << 'EOF'
!
router bgp 65001
 bgp bestpath as-path multipath-relax
 maximum-paths 2
!
EOF

cat >> /path/to/frr-baremetal.conf << 'EOF'
!
router bgp 65002
 bgp bestpath as-path multipath-relax
 maximum-paths 2
!
EOF
  ```
  - If pass → Step 53
  - If fail → SKIP (continue)

- [ ] **Step 53** [BGP] ~3m: **Route reflector config for iBGP**
  ```bash
  cat > /path/to/route-reflector.conf << 'EOF'
! Optional route reflector on bare metal if need iBGP clusters
router bgp 65000
 bgp cluster-id 192.168.0.2
 neighbor 192.168.10.0/24 peer-group IBGP_CLIENTS
 neighbor 192.168.10.0/24 remote-as 65000
 neighbor 192.168.10.0/24 route-reflector-client
!
EOF
  ```
  - If pass → Step 54
  - If fail → SKIP (optional, continue)

- [ ] **Step 54** [EVPN] ~8m: **EVPN-VXLAN overlay for container communication**
  ```bash
  cat > /path/to/evpn-config.yaml << 'EOF'
vxlan:
  networks:
    - name: ai-stack-overlay
      vni: 100
      vtep_list:
        - 192.168.0.1  # Gaming desktop
        - 192.168.0.2  # Bare metal
  local_vtep: 192.168.0.1

evpn:
  as: 65001
  router_id: 192.168.0.1
  local_vlan_map:
    ai-vllm: 10
    ai-qdrant: 11
    ai-webui: 12
    ai-embedder: 13

l2vpn:
  enable: true
  rd: 65001:100
  rt_import:
    - 65001:100
  rt_export:
    - 65001:100
EOF
  ```
  - If pass → Step 55
  - If fail → SKIP (EVPN config)

- [ ] **Step 55** [VXLAN] ~5m: **Verify L2 connectivity between containers across hosts**
  ```bash
  # Create test containers on both hosts
  docker run -d --name test-gaming --network ai-overlay alpine sleep 3600
  ssh user@<BARE_METAL_IP> 'docker run -d --name test-baremetal --network ai-overlay alpine sleep 3600'

  # Get IPs
  GAMING_IP=$(docker inspect test-gaming -f '{{.NetworkSettings.Networks.ai-overlay.IPAddress}}')
  BAREMETAL_IP=$(ssh user@<BARE_METAL_IP> 'docker inspect test-baremetal -f "{{.NetworkSettings.Networks.ai-overlay.IPAddress}}"')

  # Test connectivity
  docker exec test-gaming ping -c 3 $BAREMETAL_IP
  ```
  - Expected: ICMP reply
  - If pass → Step 56
  - If fail → Step 55D [DEBUG_L2]

- [ ] **Step 55D** [DEBUG_L2] ~5m: **Debug L2 connectivity**
  ```bash
  docker exec test-gaming ip addr show
  docker exec test-gaming ip route show
  docker network inspect ai-overlay
  ```
  - If pass → Step 55
  - If fail → SKIP (overlay networking broken, continue)

- [ ] **Step 56** [ISIS] ~5m: **IS-IS alternative config documented**
  ```bash
  cat > /path/to/ISIS-ALTERNATIVE.md << 'EOF'
# IS-IS Alternative Configuration (S50 Appendix)

If BGP not available or preferred, use IS-IS:

```
router isis UNHEADED
 net 49.0001.1921.6800.0001.00
 redistribute connected level-1
!
interface eth0
 ip router isis UNHEADED
 isis circuit-type level-1-2
!
```

IS-IS Key Differences:
- Simpler configuration than BGP for small networks
- Automatic neighbor discovery
- Convergence time: similar to BGP with BFD
- Scalability: suitable for <100 routers

Port Mapping:
- IS-IS packet port: 493
- Flooding: LSP Type 1-3
- DR election: per link

Metric Recommendation for AI traffic:
- VXLAN tunnel: 10
- Container links: 20
- Management links: 50
EOF
  ```
  - If pass → PHASE 5 COMPLETE
  - If fail → SKIP (documentation error)

---

## PHASE 6: DOCUMENTATION (Steps 57-66)

- [ ] **Step 57** [DOCS] ~8m: **Create docs/architecture/AI_STACK.md**
  ```bash
  cat > /path/to/docs/architecture/AI_STACK.md << 'EOF'
# S50: AI Model Stack Architecture

## Overview
First "head" application running inside Unheaded armor. AI inference engine with integrated vector search.

## Components

### Inference Engine (vLLM)
- Model: DeepSeek-R1 7B distilled (Q4_K_M)
- Model: Qwen 2.5 Coder 7B (Q4_K_M)
- Port: 20100
- VRAM: 9GB
- API: OpenAI-compatible

### Web Interface (Open WebUI)
- Port: 20101
- Backend: vLLM OpenAI API
- Features: Chat, model selection, history

### Vector Database (Qdrant)
- Port: 20102
- Storage: 500GB SSD
- Collections: embeddings, knowledge-base

### Embeddings (BGE-M3)
- Port: 20103
- Dimensions: 1024
- Models: multilingual, Arabic, Chinese support

## Network Topology
- Gaming Desktop: vLLM + embedder (high VRAM)
- Bare Metal: control plane, dashboard, trace-collector
- Link: VXLAN over eBGP, BFD sub-second failover
- Overlay: EVPN layer 2 fabric

## Service Registration
All services registered in Sophia dictionary with tags: ["ai", "inference", "head-application"]

## Observability
- eBPF tracing: packet-level inference metrics
- Wotan events: inference start/complete/error
- Dashboard cards: latency, throughput, error rate
- Anamnesis: event log for replay/audit

## Security
- Shield WAF: only dashboard/API→AI, AI→internal
- Implicit deny all other traffic
- Port range: 20100-20199 (Applications tier)

## Performance Targets
- Inference latency: <500ms (reasoning), <200ms (coding)
- Throughput: 4 req/s (batch size 4)
- 10-minute stability: zero errors
EOF
  ```
  - If pass → Step 58
  - If fail → SKIP (docs error)

- [ ] **Step 58** [DOCS] ~5m: **Gorgonia replacement documentation**
  ```bash
  cat > /path/to/docs/migration/GORGONIA_REPLACEMENT.md << 'EOF'
# Gorgonia Replacement Strategy (S50)

## Why Gorgonia is Dead
- Go generics in v1.18+ broke Gorgonia's type system
- No active maintenance since 2023
- Incompatible with modern Go toolchain

## vLLM as Replacement

### Advantages
- Python-first design (ML native)
- Optimized for LLM inference
- ROCm support for AMD GPUs
- OpenAI API compatibility
- Active development and community

### Integration Pattern
```
Go Application
    ↓
HTTP REST (port 20100)
    ↓
vLLM OpenAI API
    ↓
DeepSeek-R1 / Qwen 2.5 Coder
```

### Example Go Client
```go
import "github.com/sashabaranov/go-openai"

client := openai.NewClient("sk-local")
client.BaseURL = "http://localhost:20100/v1"

resp, _ := client.CreateChatCompletion(ctx,
  openai.ChatCompletionRequest{
    Model: "deepseek-r1-7b",
    Messages: []openai.ChatCompletionMessage{...},
  })
```

### Migration Checklist
- [ ] Remove gorgonia dependencies
- [ ] Replace `gorgonia.Tensor` with vLLM REST calls
- [ ] Implement caching layer for embeddings
- [ ] Test with DeepSeek-R1 and Qwen
- [ ] Benchmark latency vs original gorgonia
EOF
  ```
  - If pass → Step 59
  - If fail → SKIP (docs error)

- [ ] **Step 59** [DOCS] ~5m: **Hardware requirements documented**
  ```bash
  cat > /path/to/docs/setup/HARDWARE_REQUIREMENTS.md << 'EOF'
# Hardware Requirements (S50)

## Gaming Desktop (AI Inference Node)
- CPU: AMD Ryzen 5 7600X (6c/12t)
- GPU: RX 7700 XT (12GB VRAM)
- RAM: 16GB DDR5
- Storage: 2TB HDD (models), 1TB NVMe (swap/temp)
- Power: 550W PSU min

### VRAM Budget (12GB total)
- vLLM base: 1.5GB
- DeepSeek-R1 7B (Q4_K_M): 4.5GB
- Qwen Coder 7B (Q4_K_M): 4.5GB
- Embedder (BGE-M3): 0.5GB
- Reserve: 0.5GB
- Total: ~11.5GB (safe margin)

## Bare Metal Server (Control Plane)
- CPU: 4-core DDR3 minimum
- RAM: 8GB
- Storage: 256GB (OS, traces, metrics)
- Network: 1Gbps uplink min

## Network
- Gaming Desktop ↔ Bare Metal: 1Gbps+ link
- Latency: <10ms preferred (VXLAN overhead)
- Protocol: Ethernet, supports VXLAN/eBGP

## Power Considerations
- GPU idle power: ~50W
- GPU peak (inference): ~250W
- Sustained load (10 req/s): ~200W avg
- Daily energy: ~4.8 kWh (20h operation)
EOF
  ```
  - If pass → Step 60
  - If fail → SKIP (docs error)

- [ ] **Step 60** [DOCS] ~5m: **ROCm setup guide**
  ```bash
  cat > /path/to/docs/setup/ROCM_SETUP.md << 'EOF'
# ROCm Setup Guide (S50)

## Installation on Ubuntu 22.04

### Step 1: Add ROCm repository
```bash
wget -q -O - https://repo.radeon.com/rocm/rocm.gpg.key | sudo apt-key add -
echo "deb [arch=amd64] https://repo.radeon.com/rocm/apt/debian focal main" \
  | sudo tee /etc/apt/sources.list.d/rocm.list
sudo apt update
```

### Step 2: Install ROCm
```bash
sudo apt install -y rocm-dkms rocm-libs rocm-hip-sdk
```

### Step 3: Add user to groups
```bash
sudo usermod -aG render $USER
sudo usermod -aG video $USER
newgrp render
```

### Step 4: Verify installation
```bash
rocminfo       # Lists GPU info
rocm-smi       # GPU memory, utilization
clang --version  # Should show AMD LLVM
```

### Step 5: Docker GPU support
```bash
docker run --device=/dev/kfd --device=/dev/dri rocm/rocm-terminal rocm-smi
```

### Troubleshooting

**GPU not detected:**
```bash
lspci | grep -i amd
dmesg | grep -i amdgpu
```

**VRAM not showing 12GB:**
```bash
rocminfo | grep "Size:"
# Should show 12288 MB (12GB)
```

**Docker device access denied:**
```bash
sudo usermod -aG render $(whoami)
sudo usermod -aG video $(whoami)
sudo usermod -aG docker $(whoami)
# Logout/login required
```
EOF
  ```
  - If pass → Step 61
  - If fail → SKIP (docs error)

- [ ] **Step 61** [DOCS] ~5m: **Model selection guide (what fits in 12GB VRAM)**
  ```bash
  cat > /path/to/docs/setup/MODEL_SELECTION.md << 'EOF'
# Model Selection Guide (S50)

## Quantization Impact on VRAM

### 7B Parameter Models
| Quantization | VRAM (GB) | Quality | Speed | Recommended |
|--------------|-----------|---------|-------|-------------|
| FP16         | 14-15     | Excellent | Slow | ❌ OOM |
| FP32         | 28-30     | Best | Very slow | ❌ OOM |
| Q8_0         | 7-8       | Excellent | Fast | ✓ Pair with small model |
| Q6_K         | 5-6       | Very Good | Fast | ✓ Recommended |
| Q5_K_M       | 4.5-5     | Good | Faster | ✓ Recommended |
| Q4_K_M       | 3.5-4     | Good | Very fast | ✓ Dual-model setup |

### Recommended Setup (12GB VRAM)
```
Primary: DeepSeek-R1 7B (Q4_K_M) = 4.5GB
Secondary: Qwen Coder 7B (Q4_K_M) = 4.5GB
Embedding: BGE-M3 (ONNX quantized) = 0.5GB
Overhead: 2.5GB (system, batch buffers)
Total: ~12GB
```

### Alternative Setups

#### More Reasoning Power (14B Reasoning)
```
Primary: DeepSeek-V3 14B (Q4_K_M) = 9GB
Secondary: (skip)
Embedding: BGE-M3 = 0.5GB
Overhead: 2.5GB
Total: ~12GB
```

#### More Coding Power (Hybrid)
```
Primary: Qwen 2.5 Coder 32B (Q3_K) = 9GB (quantized)
Secondary: (skip)
Embedding: BGE-M3 = 0.5GB
Overhead: 2.5GB
Total: ~12GB
```

## Model Recommendations by Task

**Reasoning/Analysis**: DeepSeek-R1 > Qwen 32B > Llama 2
**Code Generation**: Qwen Coder > DeepSeek Coder > CodeLlama
**Embeddings**: BGE-M3 > Voyage-Large > All-MiniLM
**RAG Completeness**: DeepSeek + BGE-M3 + Qdrant

## Switching at Runtime
```bash
curl -X POST http://localhost:20100/v1/model/switch \
  -d '{"model_name": "deepseek-r1-7b"}'
# Unloads current, loads new (few seconds)
```
EOF
  ```
  - If pass → PHASE 6 COMPLETE
  - If fail → SKIP (docs error)

---

## PHASE 7: END-TO-END VERIFICATION (Steps 62-80)

- [ ] **Step 62** [E2E] ~10m: **Full pipeline: user→dashboard→API→AI→inference→trace→dashboard**
  ```bash
  # 1. User sends request to dashboard
  curl -X POST http://localhost:3000/api/inference \
    -H "Content-Type: application/json" \
    -d '{
      "query": "Explain how transformers work",
      "model": "deepseek-r1-7b"
    }' &
  INFER_PID=$!

  # 2. Trace the inference request
  sleep 1
  tail -30 /var/log/trace-collector.log | grep "20100"

  # 3. Check vLLM received it
  docker logs ai-vllm | grep "Completion request" | tail -1

  # 4. Verify response returned
  wait $INFER_PID
  echo "Inference completed"

  # 5. Check Wotan published event
  grpcurl -plaintext <BARE_METAL_IP>:50051 wotan.v1.Wotan/GetEvents \
    | grep ai_inference_complete
  ```
  - Expected: inference completes, traces captured, events published
  - If pass → Step 63
  - If fail → Step 62D [DEBUG_E2E]

- [ ] **Step 62D** [DEBUG_E2E] ~10m: **Debug end-to-end**
  ```bash
  # Check each component
  curl http://localhost:20100/health
  curl http://localhost:3000/health || echo "Dashboard may not have health"
  docker logs ai-vllm | tail -30
  grpcurl -plaintext <BARE_METAL_IP>:50051 list
  ```
  - If pass → Step 62
  - If fail → SKIP (escalate)

- [ ] **Step 63** [EBPF] ~5m: **Verify eBPF traces show AI inference packets**
  ```bash
  # Filter traces for vLLM API port
  grep "20100" /var/log/trace-collector.log | head -20
  # Should show packet details: src, dst, port, timestamp, size

  # Count inference requests
  grep "20100" /var/log/trace-collector.log | grep "dport=20100" | wc -l
  ```
  - Expected: >0 packets traced
  - If pass → Step 64
  - If fail → Step 63D [DEBUG_TRACE]

- [ ] **Step 63D** [DEBUG_TRACE] ~5m: **Debug eBPF tracing**
  ```bash
  bpftool prog list | grep -i trace
  bpftool map list | head -10
  ps aux | grep trace-collector
  tail -50 /var/log/trace-collector.log | grep -i error
  ```
  - If pass → Step 63
  - If fail → SKIP (eBPF not working, continue)

- [ ] **Step 64** [DASHBOARD] ~5m: **Verify dashboard shows AI metrics**
  ```bash
  # Query dashboard metrics API
  curl http://localhost:3000/api/metrics/ai-inference-metrics
  # Should return: latency_ms, throughput_req_s, error_rate_percent

  # Visual verification (browser)
  echo "Open http://localhost:3000 and check AI Inference Metrics card"
  ```
  - Expected: metrics returned with non-zero values
  - If pass → Step 65
  - If fail → Step 64D [DEBUG_DASHBOARD]

- [ ] **Step 64D** [DEBUG_DASHBOARD] ~5m: **Debug dashboard metrics**
  ```bash
  docker logs dashboard | tail -30
  curl http://localhost:3000/api/health
  curl http://localhost:3000/api/metrics
  ```
  - If pass → Step 64
  - If fail → SKIP (dashboard issue, continue)

- [ ] **Step 65** [SHIELD] ~5m: **Verify Shield blocks unauthorized access**
  ```bash
  # Attempt to connect from unauthorized source
  timeout 2 nc -zv localhost 20100 || echo "Access potentially blocked"

  # Try from different namespace (should be denied)
  docker run --rm alpine:latest \
    sh -c "timeout 2 nc -zv 192.168.1.1 20100" 2>&1 | grep -i "refused\|timeout"

  # Check Shield logs
  docker logs shield | tail -20 | grep -i "denied\|blocked"
  ```
  - Expected: connection refused or timeout
  - If pass → Step 66
  - If fail → Step 65D [DEBUG_SHIELD_BLOCK]

- [ ] **Step 65D** [DEBUG_SHIELD_BLOCK] ~3m: **Debug Shield blocking**
  ```bash
  docker logs shield | tail -50 | tail -20
  sudo iptables -L -n | grep 20100
  ```
  - If pass → Step 65
  - If fail → SKIP (Shield not blocking, may be permissive)

- [ ] **Step 66** [WOTAN] ~5m: **Verify Wotan events fire**
  ```bash
  # Query recent events from Wotan
  grpcurl -plaintext <BARE_METAL_IP>:50051 \
    -d '{"limit": 10, "filter": "ai_inference"}' \
    wotan.v1.Wotan/GetEvents | jq '.events[].type'

  # Should show: AI_INFERENCE_START, AI_INFERENCE_COMPLETE, AI_INFERENCE_ERROR
  ```
  - Expected: events listed
  - If pass → Step 67
  - If fail → Step 66D [DEBUG_WOTAN]

- [ ] **Step 66D** [DEBUG_WOTAN] ~3m: **Debug Wotan events**
  ```bash
  grpcurl -plaintext <BARE_METAL_IP>:50051 wotan.v1.Wotan/Health | jq .
  docker logs wotan | tail -20 | grep -i error
  ```
  - If pass → Step 66
  - If fail → SKIP (Wotan event publishing not working)

- [ ] **Step 67** [STABILITY] ~10m: **10-minute stability test**
  ```bash
  # Run inference requests for 10 minutes
  cat > /tmp/stability-test.sh << 'EOF'
#!/bin/bash
DURATION=600  # 10 minutes
INTERVAL=5    # 5 seconds between requests
MODELS=("deepseek-r1-7b" "qwen-2.5-coder-7b")
START=$(date +%s)
COUNT=0
ERRORS=0

while [ $(($(date +%s) - START)) -lt $DURATION ]; do
  MODEL=${MODELS[$((RANDOM % 2))]}
  RESPONSE=$(curl -s -X POST http://localhost:20100/v1/completions \
    -H "Content-Type: application/json" \
    -d "{\"model\":\"$MODEL\",\"prompt\":\"test\",\"max_tokens\":20}")

  if echo "$RESPONSE" | grep -q "choices"; then
    ((COUNT++))
  else
    ((ERRORS++))
    echo "Error on request $((COUNT + ERRORS)): $RESPONSE"
  fi

  sleep $INTERVAL
done

echo "Stability Test Results:"
echo "  Duration: 10 minutes"
echo "  Requests: $COUNT"
echo "  Errors: $ERRORS"
echo "  Error Rate: $(echo "scale=2; 100*$ERRORS/($COUNT+$ERRORS)" | bc)%"

[ $ERRORS -eq 0 ] && echo "PASS" || echo "FAIL"
EOF

  bash /tmp/stability-test.sh
  ```
  - Expected: <5% error rate
  - If pass → Step 68
  - If fail → Step 67D [DEBUG_STABILITY]

- [ ] **Step 67D** [DEBUG_STABILITY] ~5m: **Debug stability issues**
  ```bash
  # Check for OOM
  dmesg | grep -i "oom\|killed" | tail -5

  # Check vLLM logs
  docker logs ai-vllm | tail -50 | grep -i "error\|exception"

  # Check system resources
  free -h
  rocm-smi --showmeminfo
  docker ps --format "table {{.Names}}\t{{.CPUPerc}}\t{{.MemUsage}}"
  ```
  - If pass → Step 67
  - If fail → SKIP (stability issue, escalate)

- [ ] **Step 68** [TESTS] ~5m: **Run go test ./... -race**
  ```bash
  cd /path/to/unheaded-repo
  git status
  go test ./... -race -timeout 5m
  # Should show: ok [all packages]
  ```
  - Expected: all tests pass
  - If pass → Step 69
  - If fail → Step 68D [DEBUG_TESTS]

- [ ] **Step 68D** [DEBUG_TESTS] ~5m: **Debug test failures**
  ```bash
  go test ./... -race -v 2>&1 | grep -i "fail\|error" | head -20
  go test ./... -race -short  # Run only short tests
  ```
  - If pass → Step 68
  - If fail → SKIP (test failure, may be pre-existing)

- [ ] **Step 69** [COMMIT] ~2m: **Final commit: S50 complete**
  ```bash
  git status
  git add -A
  git commit -m "S50 Complete: AI Model Stack first head application in Unheaded armor

  - vLLM with DeepSeek-R1 7B and Qwen 2.5 Coder 7B
  - Integrated in Sophia service registry
  - Shield WAF, eBPF tracing, Wotan events
  - NixOS container definitions
  - Cross-host VXLAN + eBGP networking
  - Full observability: dashboard, trace-collector
  - 10-minute stability test: 0% error rate
  - All tests passing"

  git log --oneline -5
  ```
  - If pass → PHASE 7 COMPLETE
  - If fail → SKIP (git issue)

---

## APPENDIX A: EMERGENCY PROCEDURES

### EMERGENCY A1: ROCm Not Detected

**Symptoms**: `rocminfo` shows "0 GPU" or errors

**Recovery**:
```bash
# Step 1: Check hardware
lspci | grep -i amd
lspci | grep -i vga

# Step 2: Check BIOS
# - Reboot into BIOS, enable PCIe GPU
# - Enable BAR1 64-bit (IOMMU not required for guest)
# - Save and reboot

# Step 3: Reinstall ROCm
sudo apt purge -y rocm-* amdgpu-*
sudo apt autoremove
wget -q -O - https://repo.radeon.com/rocm/rocm.gpg.key | sudo apt-key add -
echo "deb [arch=amd64] https://repo.radeon.com/rocm/apt/debian jammy main" \
  | sudo tee /etc/apt/sources.list.d/rocm.list
sudo apt update
sudo apt install -y rocm-dkms rocm-libs rocm-hip-sdk

# Step 4: Verify
dmesg | tail -20
rocminfo | head -30

# If still failing → ESCALATE
```

### EMERGENCY A2: VRAM Out of Memory (OOM)

**Symptoms**: vLLM crashes with "RuntimeError: out of memory"

**Recovery**:
```bash
# Step 1: Check current VRAM usage
rocm-smi --showmeminfo
docker ps --format "table {{.Names}}\t{{.MemUsage}}"

# Step 2: Reduce model size
# Current: DeepSeek-R1 Q4_K_M (4.5GB) + Qwen Coder Q4_K_M (4.5GB) = 9GB
# Option A: Use single 7B model + unload secondary
# Option B: Quantize to Q3_K (saves ~1GB per model)
# Option C: Reduce max_batch_size from 4 to 2

# Step 3: Modify vLLM config
cat > docker-compose.override.yml << 'EOF'
version: '3.8'
services:
  vllm:
    command: >
      python -m vllm.entrypoints.openai.api_server
      --model meta-llama/Llama-2-7b
      --gpu-memory-utilization 0.8
      --max-model-len 1024
      --max-batch-size 2
EOF

docker-compose -f docker-compose.yml -f docker-compose.override.yml up -d vllm

# Step 4: Monitor
watch rocm-smi --showmeminfo

# If still OOM → Use Q3_K quantized model → ESCALATE
```

### EMERGENCY A3: vLLM Server Crash

**Symptoms**: vLLM container exits, no inference possible

**Recovery**:
```bash
# Step 1: Check logs
docker logs ai-vllm | tail -100

# Step 2: Common causes and fixes
# Cause A: Model format mismatch
# Fix: Ensure GGUF format, compatible with vLLM
# Alternative: Convert to safetensors format
#   huggingface-cli download deepseek-ai/deepseek-r1-distill-qwen-7b \
#     --repo-type model --revision main

# Cause B: GPU memory fragmentation
# Fix: Restart docker container
docker restart ai-vllm
docker logs ai-vllm

# Cause C: Model file corrupted or incomplete
# Fix: Re-download
rm -rf /mnt/models/weights/deepseek-r1-7b/*
wget https://huggingface.co/.../q4_k_m.gguf -O /mnt/models/weights/deepseek-r1-7b/model.gguf
docker restart ai-vllm

# Cause D: CUDA/ROCm version mismatch
# Fix: Pull latest vLLM image
docker pull vllm/vllm-openai:latest-rocm
docker-compose up -d vllm

# Step 3: Test inference
curl -X POST http://localhost:20100/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-r1-7b","prompt":"test","max_tokens":10}'

# If still crashing → Escalate with docker logs output
```

### EMERGENCY A4: Model File Too Large

**Symptoms**: Insufficient space on /mnt/models, download incomplete

**Recovery**:
```bash
# Step 1: Check available space
df -h /mnt/models /mnt/nvme

# Step 2: Options
# Option A: Use smaller quantization (Q3_K instead of Q4_K_M)
#   Size: ~3GB vs 4.5GB (saves 1.5GB)
#   Quality: still acceptable, slight degradation

# Option B: Use single model (skip secondary)
#   Free: 4.5GB (secondary model removal)

# Option C: Expand storage
#   Mount additional HDD
#   sudo mkdir /mnt/models2
#   sudo mount /dev/sdX /mnt/models2
#   mv /mnt/models/weights/* /mnt/models2/
#   mount --bind /mnt/models2 /mnt/models/weights

# Step 3: Download smaller model
mkdir -p /mnt/models/weights/qwen-coder-3b
wget https://huggingface.co/Qwen/Qwen2.5-Coder-3B-Instruct-gguf/resolve/main/qwen2.5-coder-3b-q3_k.gguf \
  -O /mnt/models/weights/qwen-coder-3b/model.gguf

# Step 4: Update config
cat >> docker-compose.override.yml << 'EOF'
services:
  vllm:
    environment:
      - VLLM_MODEL=qwen-coder-3b  # Switch to smaller model
EOF

# Step 5: Restart
docker-compose up -d vllm

# If still space issues → Escalate storage planning
```

---

## APPENDIX B: AGENT MATRIX

| Phase | Steps | Duration | Parallelizable | Critical Path | Dependencies |
|-------|-------|----------|---|---|---|
| 0 | 1-8 | ~2h | No (sequential hwvalidation) | Yes | None |
| 1 | 9-22 | ~2h | Partial (models in parallel) | Yes | Phase 0 |
| 2 | 23-36 | ~3h | No (Shield before trace) | Yes | Phase 1 |
| 3 | 37-42 | ~30m | Yes (nix files) | No | None |
| 4 | 43-49 | ~2h | No (baremetal must be sequential) | Yes | Phases 0,2 |
| 5 | 50-56 | ~1h | Yes (BGP, VXLAN parallel) | No | Phase 4 |
| 6 | 57-61 | ~30m | Yes (docs) | No | Phases 1,2,4 |
| 7 | 62-69 | ~1.5h | No (sequential E2E test) | Yes | All |

**Total**: 10-14 hours
**Critical Path**: 0→1→2→4→7 (8 hours sequential)
**Parallelizable**: Phases 3,5,6 can run during 2,4 (~2-3h saved)

---

## APPENDIX C: QUICK REFERENCE

### Model Sizes & VRAM
```
DeepSeek-R1 7B Q4_K_M:    4.5GB
Qwen 2.5 Coder 7B Q4_K_M: 4.5GB
BGE-M3 ONNX:              0.5GB
vLLM overhead:            1.5GB
Docker/system:            0.5GB
Total:                    ~11.5GB (fits in 12GB)
```

### Port Allocation (Applications Tier: 20000-20999)
```
20100: vLLM OpenAI API (inference)
20101: Open WebUI (web interface)
20102: Qdrant Vector DB (knowledge)
20103: BGE-M3 Embeddings (semantic)
20104-20199: Reserved for future AI services
```

### Service Names (Sophia Dictionary)
```
ai-model-stack          (main service group)
ai-vllm                 (inference engine)
ai-qdrant               (vector database)
ai-embedder             (semantic embeddings)
ai-webui                (web interface)
```

### Health Check Commands
```bash
# vLLM
curl http://localhost:20100/health | jq .

# Qdrant
curl http://localhost:20102/health | jq .

# Embedder
curl http://localhost:20103/health | jq .

# WebUI
curl http://localhost:20101

# Wotan (cross-host)
grpcurl -plaintext <BARE_METAL_IP>:50051 wotan.v1.Wotan/Health
```

### GPU Monitoring
```bash
# Continuous monitoring
watch rocm-smi --showmeminfo

# One-shot
rocm-smi
rocminfo | grep "Size:"

# Power draw
rocm-smi --showpower
```

### Trace Inspection
```bash
# Real-time
tail -f /var/log/trace-collector.log | grep "20100"

# Summary
grep "20100" /var/log/trace-collector.log | wc -l

# Per-packet detail
grep "20100" /var/log/trace-collector.log | head -5 | jq .
```

### Docker Shortcuts
```bash
# Logs all AI services
docker-compose logs -f --tail=50

# Restart single service
docker-compose up -d ai-vllm

# Execute in container
docker exec ai-vllm rocm-smi
docker exec ai-qdrant qdrant-cli

# Network inspection
docker network inspect ai-overlay
```

---

## APPENDIX D: MODEL SELECTION MATRIX

| Model | Params | Quant | VRAM | Reasoning | Coding | Quality | Speed | Notes |
|-------|--------|-------|------|-----------|--------|---------|-------|-------|
| DeepSeek-R1 Distill | 7B | Q4_K_M | 4.5GB | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | Excellent | 50ms avg | PRIMARY: Best reasoning |
| Qwen 2.5 Coder | 7B | Q4_K_M | 4.5GB | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | Excellent | 40ms avg | PRIMARY: Best coding |
| DeepSeek-V3 | 14B | Q3_K | 9GB | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | Excellent | 100ms avg | Alternative: Reasoning |
| Qwen 2.5 Coder | 32B | Q3_K | 9GB | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | Excellent | 150ms avg | Alternative: Coding |
| Llama 2 Chat | 7B | Q4_K_M | 4.5GB | ⭐⭐⭐ | ⭐⭐⭐ | Good | 40ms avg | Fallback: General purpose |
| Mistral | 7B | Q4_K_M | 4.5GB | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | Good | 45ms avg | Fallback: Balanced |

**Recommendation for S50**: DeepSeek-R1 7B (Q4_K_M) + Qwen 2.5 Coder 7B (Q4_K_M)
- Rationale: Best for mixed reasoning + coding workload
- VRAM: Fits perfectly in 12GB
- Latency: <100ms combined
- Quality: Production-grade

---

**END OF S50 BATTLE PLAN**

Generated: 2026-03-07
Total Steps: 145
Estimated Duration: 10-14 hours
Status: Ready for execution
