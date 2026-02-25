# Sophia's Eye: The Head of Unheaded - AI Model Stack Architecture

## Overview

Sophia's Eye is the first application running as the "head" inside Unheaded's suit of armor. It represents the decision-making and reasoning layer for the distributed Unheaded infrastructure, powered by a cutting-edge AI model stack optimized for both reasoning and code generation on consumer-grade AMD GPU hardware.

This document describes the complete architecture, deployment strategy, integration points, and operational characteristics of Sophia's Eye within the Unheaded ecosystem.

## Mission Statement

Sophia's Eye provides real-time AI inference capabilities to Unheaded's distributed services, enabling intelligent request routing, anomaly detection, and autonomous decision-making through:

1. **DeepSeek-R1 7B**: Deep reasoning model for complex decision logic
2. **Qwen 2.5 Coder 7B**: Code generation and script synthesis
3. **BGE-M3**: Multilingual embeddings for semantic search and RAG
4. **Qdrant**: Vector database for embedding storage and retrieval

All services operate in lockdown mode with strict network policies, full observability through eBPF tracing, and integration with Unheaded's Protocol API.

## Hardware Specifications

### Gaming Desktop (AI Compute Host)
- **CPU**: AMD Ryzen 5 7600X (6-core, 12-thread, 3.8-4.7 GHz)
- **GPU**: AMD Radeon RX 7700 XT (12GB GDDR6 VRAM, RDNA 2 architecture)
- **RAM**: 16GB DDR5-5600
- **Storage**:
  - 2TB HDD (/data/models) - Model weights
  - 1TB NVMe (/fast/qdrant) - Vector index (fast access)
- **Network**: 1Gbps Ethernet (EVPN-VXLAN capable)
- **OS**: NixOS with ROCm 6.0+

### Bare Metal Server (Unheaded Core)
- 4-core DDR3 system
- Role: Protocol API gateway, Wotan observability, Shield WAF, Anamnesis
- Connection: 1Gbps Ethernet via EVPN-VXLAN tunnel

## Network Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Gaming Desktop (NixOS)                        │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │  Sophia's Eye Container                                    │ │
│  │                                                             │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │ │
│  │  │  vLLM v1     │  │  vLLM v2     │  │    Qdrant    │    │ │
│  │  │ DeepSeek-R1  │  │   Qwen 2.5   │  │  Vector DB   │    │ │
│  │  │    7B        │  │    Coder 7B  │  │              │    │ │
│  │  │              │  │              │  │  ports:      │    │ │
│  │  │  port 8000   │  │  port 8001   │  │  6333 (HTTP) │    │ │
│  │  │              │  │              │  │  6334 (gRPC) │    │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘    │ │
│  │                                                             │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │ │
│  │  │ BGE-M3       │  │  Sophia's    │  │ Open WebUI   │    │ │
│  │  │ Embeddings   │  │  Eye Gateway │  │              │    │ │
│  │  │              │  │              │  │  port 20101  │    │ │
│  │  │ port 8002    │  │ port 20105   │  │              │    │ │
│  │  │              │  │              │  │ (dashboard)  │    │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘    │ │
│  │                                                             │ │
│  │  Docker-Compose Orchestration & systemd Services          │ │
│  └────────────────────────────────────────────────────────────┘ │
│                           ▲                                      │
│                           │ VXLAN Tunnel                        │
│                    vxlan0 (172.16.100.1/24)                    │
└─────────────────────────────────────┬──────────────────────────┘
                                      │
                    ┌─────────────────────────────────┐
                    │                                 │
                    ▼                                 ▼
      ┌──────────────────────┐      ┌──────────────────────────┐
      │  Bare Metal Server   │      │   External Services      │
      │  (Unheaded Core)     │      │                          │
      │                      │      │  • Internet connectivity │
      │  ┌────────────────┐  │      │  • Admin terminals      │
      │  │ Protocol API   │  │      │  • Monitoring dashboards│
      │  │ :17100/http    │  │      │                         │
      │  └────────────────┘  │      └──────────────────────────┘
      │                      │
      │  ┌────────────────┐  │
      │  │ Wotan gRPC     │  │
      │  │ :18001         │  │
      │  └────────────────┘  │
      │                      │
      │  ┌────────────────┐  │
      │  │ Shield WAF     │  │
      │  │ Rules Engine   │  │
      │  └────────────────┘  │
      │                      │
      │  ┌────────────────┐  │
      │  │ Anamnesis      │  │
      │  │ History Store  │  │
      │  └────────────────┘  │
      └──────────────────────┘
```

## Port Allocation (Application Tier 20100-20199)

```
20100 (vLLM DeepSeek-R1)        Primary reasoning model
20101 (vLLM Qwen Coder)          Code generation model
20102 (Qdrant HTTP)              Vector DB REST API
20103 (Qdrant gRPC)              Vector DB RPC
20104 (BGE-M3 Embeddings)        Embedding inference
20105 (Sophia Gateway)           Protocol API integration
20106 (Prometheus Metrics)       Observability metrics
20107 (Open WebUI)               Web interface
20108 (Prometheus)               Metrics scraper
```

## Component Architecture

### 1. vLLM DeepSeek-R1 7B (Port 20100)

**Purpose**: Deep reasoning and complex decision-making

**Configuration**:
- Model: DeepSeek-R1 7B distilled
- Runtime: vLLM with ROCm backend
- GPU Memory: 6-8GB VRAM
- Max Context: 32K tokens
- Batch Size: Up to 256 sequences
- Attention Backend: FlashInfer (AMD optimized)

**Features**:
- OpenAI-compatible API (/v1/chat/completions)
- Prefix caching for repeated queries
- Streaming responses
- LoRA adapter support

**Use Cases**:
- Request routing logic
- Anomaly scoring
- Policy decision generation
- Security rule evaluation

### 2. vLLM Qwen 2.5 Coder 7B (Port 20101)

**Purpose**: Code generation and automation scripts

**Configuration**:
- Model: Qwen 2.5 Coder 7B
- Runtime: vLLM with ROCm backend
- GPU Memory: 4-6GB VRAM
- Max Context: 16K tokens
- Batch Size: Up to 128 sequences
- Specialization: Code, SQL, Bash generation

**Features**:
- Function signature generation
- SQL query synthesis
- Bash script automation
- Multi-language support

**Use Cases**:
- Microservice code generation
- Deployment script synthesis
- Configuration automation
- Database query optimization

### 3. Qdrant Vector Database (Ports 20102-20103)

**Purpose**: Semantic search and RAG corpus management

**Configuration**:
- Storage: /fast/qdrant on 1TB NVMe
- HTTP API: Port 6333 (mapped to 20102)
- gRPC API: Port 6334 (mapped to 20103)
- Collections:
  - `knowledge-base`: Unheaded docs (384-dim BGE-M3)
  - `audit-logs`: Request history (1024-dim BGE-M3)
  - `policy-rules`: WAF/Shield rules (384-dim BGE-M3)

**Features**:
- Hybrid search (dense + sparse)
- Payload filtering
- HNSW indexing
- Snapshot/restore

### 4. BGE-M3 Embeddings (Port 20104)

**Purpose**: Text-to-embedding conversion for RAG and semantic search

**Configuration**:
- Model: BGE-M3 (384-dim dense + sparse)
- Runtime: Text Embeddings Inference (TEI)
- Batch Size: Up to 128 texts
- Latency: <100ms p95 for 512-token input

**Features**:
- Multilingual support (50+ languages)
- Long document handling (8K tokens)
- Dense + sparse hybrid embeddings
- Batch inference

### 5. Sophia's Eye Gateway (Port 20105-20107)

**Purpose**: Integration layer between AI stack and Unheaded infrastructure

**Responsibilities**:
1. Register services in Sophia dictionary on startup
2. Proxy inference requests to vLLM instances
3. Record requests in Anamnesis via Protocol API
4. Report metrics to Wotan for observability
5. Expose Prometheus metrics on /metrics

**Key Endpoints**:
- `POST /v1/chat/completions` - Inference proxy
- `GET /health` - Health check
- `GET /ready` - Readiness probe
- `GET /models` - Available models list
- `GET /metrics` - Prometheus metrics

**Service Discovery Integration**:
Registers on startup in Sophia dictionary:
```json
{
  "name": "sophia-eye/vllm",
  "endpoint": "http://vllm:8000",
  "model": "deepseek-r1-7b",
  "type": "llm",
  "metadata": {
    "gpu": "amdgpu",
    "max_tokens": "32768",
    "memory_util": "0.85"
  }
}
```

### 6. Open WebUI (Port 20107)

**Purpose**: Web interface for interactive model testing and monitoring

**Features**:
- Chat interface for both models
- RAG integration with Qdrant
- Request history tracking
- Performance metrics dashboard
- Model comparison interface

## D2 Requirements Implementation

### Requirement 1: Service Registration in Sophia Dictionary

**Implementation**: Sophia Gateway registers on startup

```go
func (sg *SophiaGateway) registerInSophia(ctx context.Context) error {
    entries := []SophiaServiceEntry{
        {
            Name: "sophia-eye/vllm",
            Endpoint: sg.vllmAddr,
            Model: "deepseek-r1-7b",
            Type: "llm",
        },
        // ... more entries
    }
    // POST to http://bare-metal:17100/api/sophia/register
}
```

**Verification**: Query Sophia dictionary API:
```bash
curl http://bare-metal:17100/api/sophia/list?service=sophia-eye
```

### Requirement 2: eBPF Tracing of Inference Requests

**Implementation**: Shield WAF with eBPF filters

**Traced Flows**:
- Ingress: All TCP traffic to ports 20100-20105
- Egress: All TCP traffic to bare-metal:17100 (Protocol API)
- Egress: All gRPC to bare-metal:18001 (Wotan)

**Tracing Metadata**:
```json
{
  "flow_id": "deepseek-r1-req-12345",
  "src_ip": "172.16.0.100",
  "dst_port": 20100,
  "model": "deepseek-r1-7b",
  "prompt_tokens": 256,
  "completion_tokens": 512,
  "latency_ms": 2500,
  "timestamp": 1708876543210
}
```

**Dashboard Visualization**:
- Real-time flow graph (source → destination)
- Model inference latency heatmap
- Token generation rates
- Error rate trends

### Requirement 3: Shield WAF Network Lockdown

**Implementation**: Restrictive firewall rules in shield-rules.yaml

**Allowed Ingress Flows**:
- Source: VXLAN fabric (172.16.0.0/12) only
- Destination: Ports 20100-20105
- Protocol: TCP
- Action: ALLOW + TRACE

**Allowed Egress Flows**:
1. Protocol API: bare-metal:17100/tcp (ALLOW + TRACE)
2. Wotan: bare-metal:18001/tcp (ALLOW)
3. Qdrant internal: localhost:6333,6334 (ALLOW)

**Denied by Default**: All other traffic (DENY + LOG)

### Requirement 4: Wotan Message Bus Integration

**Implementation**: Sophia Gateway reports metrics asynchronously

**Metric Format**:
```json
{
  "timestamp": 1708876543210,
  "service_name": "sophia-eye",
  "event_type": "inference_complete",
  "model": "deepseek-r1-7b",
  "tokens": 768,
  "latency_ms": 2500,
  "metadata": {
    "prompt_tokens": "256",
    "completion_tokens": "512"
  }
}
```

**Reporting Endpoint**: POST to `http://bare-metal:18001/metrics/ingest`

**Metrics Captured**:
- inference_latency_ms (histogram)
- inference_errors_total (counter)
- tokens_generated_total (counter)
- tokens_prompted_total (counter)
- requests_total (counter)

### Requirement 5: Dashboard Inference Metrics

**Prometheus Metrics Exposed** (port 20106):

```
# Inference latency percentiles
sophia_inference_latency_ms_bucket{le="50"}
sophia_inference_latency_ms_bucket{le="100"}
sophia_inference_latency_ms_bucket{le="250"}
sophia_inference_latency_ms_bucket{le="500"}
sophia_inference_latency_ms_bucket{le="1000"}
sophia_inference_latency_ms_bucket{le="2500"}
sophia_inference_latency_ms_bucket{le="5000"}

# Token throughput
sophia_tokens_generated_total
sophia_tokens_prompted_total

# Error tracking
sophia_inference_errors_total

# Service health
sophia_registration_status (1=registered, 0=not)
```

**Dashboard SLA Targets**:
- Latency p50: <500ms
- Latency p95: <2500ms
- Latency p99: <5000ms
- Throughput: >100 tokens/sec aggregate
- Error rate: <0.5%

### Requirement 6: Protocol API Communication

**Implementation**: All AI-to-Unheaded communication via HTTP

**Endpoints**:
1. Service Registration: `POST /api/sophia/register`
2. Inference Recording: `POST /api/anamnesis/record`
3. Policy Query: `GET /api/shield/rules?service=sophia-eye`

**Request Format**:
```json
{
  "service_name": "sophia-eye",
  "model": "deepseek-r1-7b",
  "prompt_tokens": 256,
  "completion_tokens": 512,
  "latency_ms": 2500,
  "status": "success"
}
```

**Never Direct BPF Map Access**: All observability goes through Protocol API

## Storage Layout

### /data/models (2TB HDD)

```
/data/models/
├── deepseek-r1-7b/
│   ├── config.json
│   ├── model.safetensors
│   ├── tokenizer.json
│   ├── tokenizer_config.json
│   └── special_tokens_map.json
├── qwen2.5-coder-7b/
│   ├── config.json
│   ├── model.safetensors
│   ├── tokenizer.model
│   └── tokenizer_config.json
└── bge-m3/
    ├── config.json
    ├── model.safetensors
    ├── tokenizer.json
    └── vocab.txt
```

**Total Size**: ~20GB (6.5GB + 7.5GB + 6GB)

### /fast/qdrant (1TB NVMe)

```
/fast/qdrant/
├── storage/
│   ├── knowledge-base/
│   │   ├── segments/
│   │   └── segment_0/
│   ├── audit-logs/
│   │   ├── segments/
│   │   └── segment_0/
│   └── policy-rules/
│       ├── segments/
│       └── segment_0/
└── snapshots/
    └── knowledge-base-2025-02-24.snapshot
```

**Index Configuration**:
- Vector Size: 384 (BGE-M3 dense)
- Index Type: HNSW
- ef_construct: 200
- ef_search: 100
- M: 16

## Model Selection Rationale

### Why DeepSeek-R1 7B?

1. **Reasoning Performance**: Superior CoT reasoning in 7B parameter class
2. **VRAM Efficiency**: Fits in 12GB VRAM at fp16 quantization
3. **License**: Open weights under Apache 2.0
4. **Speed/Quality Trade-off**: Better than larger models on commodity hardware
5. **Latency**: 200-3000ms inference time (acceptable for policy decisions)

### Why Qwen 2.5 Coder 7B?

1. **Code Generation**: State-of-the-art for 7B parameter code models
2. **Multi-language**: Python, Go, Bash, SQL, YAML
3. **Context Window**: 32K (can be reduced to 16K for speed)
4. **Training Data**: Includes latest AI/ML practices
5. **Apache 2.0 License**: Commercial-friendly

### Why Not Gorgonia or Local Fine-tuning?

**Gorgonia (Dead as a Project)**:
- Last meaningful commit: 2023
- Go generics (1.18+) killed the motivation
- ONNX runtime matured as better alternative
- vLLM OpenAI API is the industry standard

**Conclusion**: vLLM with pre-trained models is the only sensible choice.

### Why BGE-M3?

1. **Multilingual**: Covers all Unheaded documentation languages
2. **Hybrid Search**: Dense + sparse embeddings for flexibility
3. **Long Documents**: Can handle 8K token chunks
4. **Performance**: Fast embedding generation (<100ms)
5. **Dimension**: 384-dim dense is small but capable

## ROCm Setup & GPU Drivers

### AMD GPU Support Stack

```
Hardware: RX 7700 XT (RDNA 2, gfx1032)
         │
         ▼
ROCm Core 6.0+
         │
         ├─ HIP Runtime
         ├─ ROCm OpenCL ICDs
         ├─ AMD GPU Device Libs
         └─ ROCM-managed firmware
         │
         ▼
PyTorch + ROCm Backend
         │
         ▼
vLLM (ROCm variant)
         │
         ├─ hipBLAS (matrix ops)
         ├─ hipRAND (randomness)
         ├─ rocFFT (FFT operations)
         └─ MIOpen (neural network)
```

### NixOS ROCm Configuration

```nix
services.rocm = {
  enable = true;
  gpuTargets = [ "gfx1032" ];  # RX 7700 XT
  rocmPackages = with pkgs.rocmPackages; [
    rocm-runtime
    rocm-opencl-icd
    rocm-device-libs
    rocminfo
    rocm-smi
  ];
};

environment.variables = {
  ROCM_HOME = "${pkgs.rocmPackages.rocm-core}";
  HIP_PLATFORM = "amd";
  VLLM_WORKER_MULTIPROC_METHOD = "spawn";
};
```

### Verification Commands

```bash
# Check HIP runtime
rocminfo | grep gfx1032

# Monitor GPU
rocm-smi

# Verify vLLM GPU detection
python -c "import torch; print(torch.cuda.is_available()); print(torch.cuda.get_device_name(0))"
```

## Startup Sequence

```
1. VXLAN Tunnel Establishment
   └─> Verify bare-metal:172.16.100.254 reachable
       └─> Wait max 30s

2. Health Check Protocol API
   └─> GET http://bare-metal:17100/health
       └─> Must return 200 within 10s

3. Register Services in Sophia
   └─> POST /api/sophia/register (4 services)
       └─> Each must return 200/201
           └─> Set registration_status=1

4. Start vLLM DeepSeek-R1
   └─> Wait for /health endpoint 200
       └─> Max 60s startup time

5. Start vLLM Qwen Coder
   └─> Wait for /health endpoint 200
       └─> Max 60s startup time

6. Start Qdrant
   └─> Initialize vector indices
       └─> Load snapshots if available

7. Start BGE-M3 Embeddings
   └─> Warmup with test batch
       └─> Max 30s

8. Start Sophia Gateway
   └─> Begin proxying requests
       └─> Start Wotan metric reporting

9. Start Open WebUI
   └─> Connect to vLLM endpoints
       └─> Dashboard operational

Total startup time: 4-5 minutes (cold start with model loading)
```

## Monitoring & Observability

### Prometheus Scrape Configuration

Add to bare-metal Prometheus config:
```yaml
scrape_configs:
  - job_name: 'sophia-eye'
    static_configs:
      - targets: ['172.16.100.2:20106']
    scrape_interval: 15s
    scrape_timeout: 10s
    metrics_path: '/metrics'
```

### Log Aggregation

Container logs available via:
```bash
docker logs -f sophia-vllm-deepseek
docker logs -f sophia-vllm-qwen
docker logs -f sophia-qdrant
docker logs -f sophia-gateway
```

Journal access on host:
```bash
journalctl -u vllm-deepseek -f
journalctl -u sophia-gateway -f
```

### Alerting Rules

```promql
# High inference latency
sophia_inference_latency_ms{quantile="0.95"} > 3000

# Inference error spike
rate(sophia_inference_errors_total[5m]) > 0.01

# Registration lost
sophia_registration_status == 0

# GPU memory near limit
nvidia_smi_memory_used > 11GB (for Nvidia reference)
```

## Future Roadmap

### Phase 1.1 (Next Sprint)
- Multi-model routing (smart model selection based on request)
- Fine-tuning pipeline for Unheaded-specific policies
- RAG corpus auto-expansion from audit logs

### Phase 2 (Q2)
- Speculative decoding (smaller draft models for faster inference)
- LoRA adapter loading for per-customer customization
- Model quantization (4-bit/8-bit) for faster inference

### Phase 3 (Q3)
- Distributed inference (model sharding across multiple GPUs)
- Mixture of Experts (MoE) for increased reasoning capacity
- Continuous learning from Anamnesis feedback

### Phase 4 (Q4)
- On-device fine-tuning (GPU training during off-peak hours)
- Ensemble voting (multiple models for critical decisions)
- Federated learning integration

## Troubleshooting

### vLLM Won't Start

**Symptom**: Health check timeout on port 8000

**Diagnosis**:
```bash
docker logs sophia-vllm-deepseek | tail -50
nvidia-smi  # Check GPU availability
```

**Common Causes**:
1. Model weights not found: Check `/data/models/deepseek-r1-7b/model.safetensors`
2. Insufficient VRAM: Need 8GB+ for fp16, 12GB+ with batch
3. ROCm not initialized: `rocminfo` should list GPU

**Fix**:
```bash
# Verify GPU
rocm-smi

# Restart container
docker restart sophia-vllm-deepseek

# Check logs
docker logs -f sophia-vllm-deepseek
```

### Qdrant Collection Empty

**Symptom**: Vector searches return no results

**Diagnosis**:
```bash
curl http://localhost:20102/collections
```

**Fix**: Import embeddings from knowledge base:
```bash
python3 -c "
from qdrant_client import QdrantClient
client = QdrantClient('localhost', port=20102)
# Import from /data/knowledge-base.json
"
```

### Gateway Registration Failed

**Symptom**: `sophia_registration_status == 0`

**Diagnosis**:
```bash
curl http://bare-metal:17100/health
docker logs sophia-gateway | grep "register"
```

**Fix**: Check bare-metal server connectivity:
```bash
ping bare-metal
curl -v http://bare-metal:17100/api/sophia/register
```

### High Inference Latency

**Symptom**: p95 latency >3000ms

**Diagnosis**:
```bash
# Check GPU utilization
rocm-smi

# Check vLLM queue
docker logs sophia-vllm-deepseek | grep "queue"

# Check system load
top
```

**Optimization**:
1. Reduce `max-num-seqs` if GPU memory constrained
2. Reduce prompt length (context token limit)
3. Enable prefix caching (enabled by default)
4. Consider offloading to Qwen (lighter model)

## Reference

- **vLLM Documentation**: https://docs.vllm.ai
- **ROCm Installation**: https://rocmdocs.amd.com
- **Qdrant Vector DB**: https://qdrant.tech/documentation
- **BGE Embeddings**: https://huggingface.co/BAAI/bge-m3
- **DeepSeek-R1**: https://huggingface.co/deepseek-ai/DeepSeek-R1
- **Qwen 2.5 Coder**: https://huggingface.co/Qwen/Qwen2.5-Coder-7B

---

**Version**: 1.0.0
**Last Updated**: 2025-02-24
**Status**: Production Ready (Gaming Desktop Host)
**Maintained By**: Unheaded Infrastructure Team
