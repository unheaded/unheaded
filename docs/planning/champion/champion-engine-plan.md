# Champion Engine — High-Level Implementation Plan

## Custom Inference Server: Rust Engine + Go Management Plane

**Date**: 2026-03-11
**Status**: HIGH-LEVEL PLAN — Feed to agent for verbose battle plan when ready
**Sprint**: Post-Demo (after S-CHAMPION RAG demo is live)
**Hardware Target**: AMD RX 7700 XT (12GB VRAM), 16GB DDR5, 1TB NVMe, 2TB HDD, B650 AM5
**Prerequisite**: S-CHAMPION battle plan Phases 0-8 complete (RAG demo operational)

---

## 1. STRATEGIC CONTEXT

### Why Build This

Ollama wraps llama.cpp in a Go HTTP server — three overhead layers before GPU compute.
For the Unheaded Kingdom, we need tighter integration: eBPF token tracing, Monad wire
format awareness, Wotan message bus connectivity, and zero-copy model loading. A custom
engine also eliminates the dependency on Ollama's release cycle and architectural choices.

### Architecture Decision: Two-Binary Split

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Champion Engine (Rust)                          │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────────────┐ │
│  │ GGUF Loader │  │ Inference    │  │ PagedAttention KV Cache    │ │
│  │ zero-copy   │  │ candle/GGML  │  │ continuous batching        │ │
│  │ mmap        │  │ ROCm/HIP     │  │ arena allocator            │ │
│  └─────────────┘  └──────────────┘  └────────────────────────────┘ │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────────────┐ │
│  │ Streaming   │  │ eBPF Hooks   │  │ /dev/shm Ring Buffer       │ │
│  │ gRPC+tonic  │  │ token trace  │  │ → Monad protocol bridge    │ │
│  └─────────────┘  └──────────────┘  └────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
        ↕ Unix socket / gRPC

┌─────────────────────────────────────────────────────────────────────┐
│                  Champion Manager (Go)                               │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────────────┐ │
│  │ Model       │  │ REST/gRPC    │  │ Health Checks              │ │
│  │ Registry    │  │ API Gateway  │  │ Liveness + Readiness       │ │
│  └─────────────┘  └──────────────┘  └────────────────────────────┘ │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────────────────┐ │
│  │ Download    │  │ Wotan        │  │ wotan-ctl champion         │ │
│  │ Manager     │  │ Integration  │  │ CLI subcommand             │ │
│  └─────────────┘  └──────────────┘  └────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

**Why Rust for engine**: Zero-cost abstractions, no GC pauses on inference hot path,
direct ROCm/HIP FFI, mmap zero-copy model loading, arena allocators for KV cache.

**Why Go for manager**: Consistent with Unheaded service ecosystem (all 10 services are Go),
`wotan-ctl` integration, familiar deployment patterns, goroutines for concurrent downloads.

### Port Allocation (Doom Range)

| Service | Port | Protocol | Description |
|---------|------|----------|-------------|
| champion-engine | 20100 | gRPC | Rust inference engine (internal) |
| champion-embedding | 20101 | HTTP | Embedding API (sentence-transformers) |
| champion-rag | 20102 | HTTP/gRPC | RAG pipeline API |
| champion-ui | 20103 | HTTP | Web UI (chat interface) |
| champion-manager | 20104 | HTTP/gRPC | Go management plane |
| champion-metrics | 20105 | HTTP | Prometheus /metrics endpoint |

---

## 2. PHASE OVERVIEW

| Phase | Title | Scope | Est. Time | Dependencies |
|-------|-------|-------|-----------|--------------|
| 0 | Rust Toolchain + ROCm SDK | Dev environment for Rust+HIP | 2-3h | None |
| 1 | GGUF Loader | Zero-copy mmap model loading | 4-6h | Phase 0 |
| 2 | Inference Core | Token generation via candle or llama-cpp-rs | 8-12h | Phase 1 |
| 3 | KV Cache + PagedAttention | Memory-efficient context management | 6-8h | Phase 2 |
| 4 | Streaming gRPC Server | tonic-based inference API | 4-6h | Phase 2 |
| 5 | Go Management Plane | Model registry, downloads, health | 6-8h | Phase 4 |
| 6 | eBPF Token Tracing | Per-token latency hooks, BPF maps | 6-8h | Phase 4 |
| 7 | Wotan Integration | Message bus bridge, /dev/shm ring | 4-6h | Phases 5, 6 |
| 8 | Continuous Batching | Multi-request scheduling | 8-12h | Phase 3 |
| 9 | RAG Pipeline Migration | Move RAG from FastAPI to native | 6-8h | Phase 4 |
| 10 | Dashboard + Observability | Grafana panels, token traces | 4-6h | Phases 6, 7 |
| 11 | Integration Testing + Hardening | Fuzz, load test, chaos | 8-12h | All |

**Critical Path**: 0 → 1 → 2 → 3 → 8 (batching) = ~30h
**Parallel Track A**: 0 → 1 → 2 → 4 → 5 (management) = ~24h
**Parallel Track B**: 0 → 1 → 2 → 4 → 6 → 7 (eBPF+Wotan) = ~28h

**Total Estimate**: 70-90h execution across 2-3 weeks

---

## 3. PHASE DETAILS

### Phase 0: Rust Toolchain + ROCm SDK

**Goal**: Working Rust build environment with ROCm/HIP access on WEST.

**Key tasks**:
- Install/verify ROCm 6.x SDK (amdgpu driver, rocm-hip-sdk, hipcc)
- Verify `rocminfo` shows gfx1101 (RDNA3 / RX 7700 XT)
- Install Rust nightly (candle requires nightly for some CUDA/HIP features)
- Scaffold `champion-engine/` crate with workspace Cargo.toml
- Verify `hipcc` can compile and run a trivial kernel on the 7700 XT
- Install protobuf compiler (tonic codegen dependency)

**Exit gate**: `cargo build` succeeds on empty champion-engine crate, `hipcc` sample runs on GPU.

### Phase 1: GGUF Loader

**Goal**: Load any GGUF model file via mmap with zero-copy semantics.

**Key tasks**:
- Implement GGUF header parser (magic bytes, version, tensor metadata)
- Memory-map model file — `mmap(PROT_READ, MAP_PRIVATE)`, no heap copies
- Parse tensor descriptors: name, dimensions, quantization type, file offset
- Support quantization formats: Q4_K_M, Q5_K_M, Q8_0, F16 (minimum viable set)
- Dequantization kernels for each format (CPU reference impl first, HIP later)
- Unit tests: load Mistral-7B-Q5_K_M.gguf, verify tensor count, shapes, metadata

**Reference**: `gguf-rs` crate exists but is read-only — we need write support for
future RAFT adapter merging. Fork or rewrite based on complexity assessment.

**Exit gate**: Load Mistral-7B GGUF, enumerate all tensors, verify shapes match HuggingFace config.

### Phase 2: Inference Core

**Goal**: Generate tokens from a loaded GGUF model on AMD GPU.

**Decision point — two paths**:

**Path A: candle backend (pure Rust)**
- HuggingFace's Rust ML framework
- Native GGUF support via `candle-transformers`
- ROCm support is experimental (candle-hip)
- Pro: pure Rust, no FFI. Con: ROCm maturity on RDNA3 uncertain

**Path B: llama-cpp-rs backend (Rust FFI to C++)**
- Thin Rust wrapper around battle-tested llama.cpp
- ROCm/HIP already works on RX 7700 XT (proven in S-CHAMPION Phase 2)
- Pro: known working, mature. Con: FFI boundary, C++ dependency

**Recommendation**: Start with Path B (llama-cpp-rs) for immediate functionality,
develop Path A (candle) in parallel for long-term pure-Rust goal. Trait-based
abstraction allows swapping backends without touching the server layer.

```rust
pub trait InferenceBackend: Send + Sync {
    fn load_model(&self, path: &Path, params: ModelParams) -> Result<ModelHandle>;
    fn generate(&self, handle: &ModelHandle, prompt: &str, params: GenParams) -> Result<TokenStream>;
    fn tokenize(&self, handle: &ModelHandle, text: &str) -> Result<Vec<u32>>;
    fn detokenize(&self, handle: &ModelHandle, tokens: &[u32]) -> Result<String>;
    fn model_info(&self, handle: &ModelHandle) -> ModelInfo;
}
```

**Key tasks**:
- Define `InferenceBackend` trait (above)
- Implement `LlamaCppBackend` wrapping llama-cpp-rs
- Implement sampling: temperature, top-p, top-k, repetition penalty, min-p
- Token streaming via `tokio::sync::mpsc` channel
- Prompt template handling (Mistral instruct format, ChatML, Llama-3 format)
- Benchmark: tokens/sec on RX 7700 XT vs raw llama.cpp CLI (target: <5% overhead)

**Exit gate**: Generate coherent 256-token response from Mistral-7B, streaming, on GPU.
Benchmark within 5% of raw llama.cpp throughput.

### Phase 3: KV Cache + PagedAttention

**Goal**: Memory-efficient context window management enabling longer contexts and
future multi-request batching.

**Key concepts**:
- Standard KV cache: pre-allocates `max_seq_len × n_layers × head_dim × 2` — wastes VRAM
- PagedAttention (Kwon et al., 2023): allocates KV cache in fixed-size pages (blocks),
  maps logical positions to physical blocks via page table. Only allocates what's used.
- On 12GB VRAM with 5.1GB model: ~6.9GB for KV cache. PagedAttention uses this 2-4x
  more efficiently than contiguous allocation.

**Key tasks**:
- Implement block allocator (fixed 16-token blocks, free list)
- Page table mapping: logical sequence position → physical block index
- KV cache read/write through page table indirection
- Copy-on-write for shared prefixes (system prompt reuse across requests)
- Preemption: evict lowest-priority sequence when blocks exhausted
- Memory pressure monitoring: track allocated/free blocks, trigger warnings

**Exit gate**: Run inference with 4096-token context, verify VRAM usage is <70% of
naive contiguous allocation. No OOM on 12GB card.

### Phase 4: Streaming gRPC Server

**Goal**: Production gRPC API for inference requests using tonic.

**Key tasks**:
- Define protobuf service:
  ```protobuf
  service ChampionEngine {
    rpc Generate(GenerateRequest) returns (stream GenerateResponse);
    rpc Tokenize(TokenizeRequest) returns (TokenizeResponse);
    rpc Health(HealthRequest) returns (HealthResponse);
    rpc ModelInfo(ModelInfoRequest) returns (ModelInfoResponse);
    rpc Metrics(MetricsRequest) returns (MetricsResponse);
  }
  ```
- Implement server with tonic + tokio runtime
- Server-side streaming for token-by-token generation
- Request cancellation via gRPC cancellation propagation
- Graceful shutdown (drain in-flight requests, release GPU)
- TLS support (rustls, optional for internal traffic)
- Prometheus metrics: request count, token latency histogram, VRAM usage gauge,
  active requests gauge, tokens/sec counter
- Listen on port 20100 (gRPC) with HTTP/2

**Exit gate**: `grpcurl` can call Generate, receive streaming tokens. Health check passes.
Prometheus metrics endpoint returns valid scrape data.

### Phase 5: Go Management Plane

**Goal**: Model lifecycle management, API gateway, Unheaded ecosystem integration.

**Key tasks**:
- Scaffold `cmd/wotan-ctl/champion.go` subcommand group:
  - `wotan-ctl champion status` — engine health, loaded model, VRAM usage
  - `wotan-ctl champion load <model>` — trigger model load via gRPC
  - `wotan-ctl champion pull <model>` — download from HuggingFace
  - `wotan-ctl champion list` — show local model registry
  - `wotan-ctl champion bench` — run quick benchmark (tokens/sec)
- Model registry: `~/.champion/models/` with manifest.json per model
- Download manager: resume-capable HTTP downloads with progress, checksum verify
- REST API gateway (port 20104):
  - `POST /api/v1/generate` — proxies to Rust engine gRPC
  - `GET /api/v1/models` — list available models
  - `GET /api/v1/health` — aggregated health (engine + embedding + RAG)
  - `GET /api/v1/status` — detailed system status (VRAM, RAM, disk)
- Ollama API compatibility layer (optional stretch goal):
  - `POST /api/generate` — Ollama-compatible endpoint
  - `POST /api/chat` — Ollama-compatible chat endpoint
  - Enables drop-in replacement for tools expecting Ollama
- Systemd service file: `champion-manager.service`
- Wotan registration: announce champion services on system.discovery topic

**Exit gate**: `wotan-ctl champion status` shows engine health. REST API serves
generation requests proxied to Rust engine. Model download + load cycle works end to end.

### Phase 6: eBPF Token Tracing

**Goal**: Per-token latency tracing via eBPF, feeding Kingdom observability pipeline.

**Key tasks**:
- Design BPF program: kprobe/uprobe on inference hot path entry/exit
- BPF map: per-request trace ring buffer (request_id, token_idx, timestamp_ns, latency_ns)
- Userspace reader (Go): poll BPF map, emit as Wotan Anamnesis events
- Metrics derived from traces:
  - Time-to-first-token (TTFT) histogram
  - Inter-token latency (ITL) histogram
  - Total generation time per request
  - Tokens/sec real-time gauge
- Integration with existing trace-collector (port 16670)
- Dashboard panel: token latency flame graph (per-request waterfall)

**eBPF attachment strategy**:
- Option A: uprobe on Rust binary's `generate_next_token()` — requires symbol visibility
- Option B: USDT probes compiled into Rust binary — cleaner, requires `probe-rs` or manual
- Option C: tracepoint on GPU command submission (ROCm runtime) — hardware-level

**Exit gate**: Generate a response, observe per-token latency data in BPF map.
Trace-collector receives Anamnesis events. Dashboard shows token waterfall.

### Phase 7: Wotan Integration

**Goal**: Champion Engine as a first-class Wotan service — message bus connectivity,
shared memory ring, Kingdom Mode compliance.

**Key tasks**:
- Implement Wotan gRPC client in Go manager (subscribe/publish topics)
- Champion-specific Wotan topics:
  - `champion.inference.request` — incoming generation requests via bus
  - `champion.inference.response` — streaming token responses via bus
  - `champion.inference.metrics` — periodic metrics broadcast
  - `champion.model.status` — model load/unload events
- `/dev/shm/champion-ring` — shared memory ring buffer for zero-copy token transfer
  between Rust engine and Go manager (avoid gRPC serialization for local traffic)
- Monad protocol bridge: tag Champion inference packets with Monad headers
  (flow action = CHAMPION_INFERENCE, service ID registered in Kingdom)
- Kingdom Mode awareness: respect pause/resume/drain commands from wotan-ctl

**Exit gate**: Send inference request via Wotan topic, receive streaming response.
`wotan-ctl status` shows Champion service registered and healthy.

### Phase 8: Continuous Batching

**Goal**: Handle multiple concurrent inference requests efficiently.

**Key tasks**:
- Implement request queue with priority levels
- Continuous batching scheduler (iteration-level scheduling):
  - Each iteration processes tokens from ALL active sequences
  - New requests join mid-batch (no waiting for current request to finish)
  - Completed sequences release their KV cache blocks immediately
- Preemption policy: when VRAM full, pause lowest-priority sequence, swap KV to CPU RAM
- Fairness: round-robin within same priority, prevent starvation
- Benchmark: throughput (tokens/sec aggregate) with 1, 2, 4, 8 concurrent requests
- Back-pressure: reject requests when queue depth exceeds threshold

**Note**: On 12GB VRAM with Mistral-7B Q5_K_M, realistic concurrent capacity is
2-4 requests depending on context length. This phase is more about correctness
and architecture than massive parallelism — sets foundation for future GPU upgrades.

**Exit gate**: 4 concurrent requests complete without OOM. Throughput scales >1.5x
versus sequential processing.

### Phase 9: RAG Pipeline Migration

**Goal**: Move RAG from Python FastAPI prototype to native Go service backed by
Champion Engine gRPC.

**Key tasks**:
- Port embedding service: Go wrapper around sentence-transformers (Python sidecar or
  ONNX runtime in Go via `onnxruntime-go`)
- Port FAISS search: Go bindings (`go-faiss`) or rewrite with custom ANN index
- RAG orchestration in Go:
  1. Receive query → embed → search FAISS → retrieve top-k chunks
  2. Construct prompt with retrieved context
  3. Call Champion Engine gRPC Generate (streaming)
  4. Stream response tokens back to client
- REST API on port 20102:
  - `POST /api/v1/ask` — RAG query with streaming response
  - `POST /api/v1/search` — semantic search only (no generation)
  - `GET /api/v1/corpus/stats` — index statistics
- Circuit breaker: if engine is down, return graceful error (not hang)
- Caching: LRU cache for repeated queries (embedding + search results)

**Exit gate**: RAG query returns grounded response via Go pipeline. Latency within
20% of Python prototype. Circuit breaker tested.

### Phase 10: Dashboard + Observability

**Goal**: Real-time Champion monitoring integrated with Kingdom dashboard.

**Key tasks**:
- Grafana dashboard panels (or custom Unheaded dashboard):
  - GPU VRAM utilization gauge
  - Tokens/sec real-time counter
  - TTFT + ITL latency histograms
  - Active requests gauge
  - Model info card (name, quant, context length)
  - Token trace waterfall (from eBPF Phase 6)
- Prometheus scrape config for champion-metrics (port 20105)
- Alert rules:
  - VRAM > 90% for > 30s
  - TTFT > 5s (degraded inference)
  - Engine health check failed for > 10s
  - Disk < 50GB free on model storage
- Log aggregation: structured JSON logs from both Rust engine and Go manager
  → Fluentd → ELK pipeline (existing Kingdom infra)

**Exit gate**: Dashboard shows live inference metrics. Alerts fire on simulated failures.

### Phase 11: Integration Testing + Hardening

**Goal**: Production-grade reliability. Break it, fuzz it, fix it.

**Key tasks**:
- Unit tests: 100% coverage on GGUF loader, page table, scheduler, gRPC handlers
- Integration tests:
  - Full request lifecycle: HTTP → Go manager → gRPC → Rust engine → GPU → response
  - Model hot-swap: load model A, swap to model B mid-flight, verify no corruption
  - Graceful shutdown: in-flight requests drain before exit
  - OOM recovery: trigger VRAM exhaustion, verify engine recovers
- Fuzz testing:
  - GGUF loader: malformed headers, truncated files, corrupt tensor data
  - gRPC server: invalid protobuf, oversized messages, rapid connect/disconnect
  - Prompt injection: adversarial prompts, UTF-8 edge cases, null bytes
- Load testing:
  - Sustained 4 concurrent requests for 1 hour — no memory leak, no degradation
  - Burst: 20 requests in 1 second — verify back-pressure and graceful rejection
- Chaos testing:
  - Kill Rust engine mid-inference — Go manager detects and reports
  - Fill VRAM with dummy data — verify preemption and recovery
  - Network partition between manager and engine — verify timeout + reconnect
- Security hardening:
  - Input sanitization on all API endpoints
  - Rate limiting on public-facing endpoints
  - No model file path traversal
  - gRPC reflection disabled in production
- BlackMage review: full attack surface assessment (invoke skill before shipping)

**Exit gate**: All tests pass. 1-hour soak test clean. Fuzz campaigns find no crashes.
BlackMage sign-off.

---

## 4. TECHNOLOGY CHOICES

### Rust Crates (Engine)

| Crate | Purpose | Version |
|-------|---------|---------|
| `tokio` | Async runtime | 1.x |
| `tonic` | gRPC server/client | 0.12+ |
| `prost` | Protobuf codegen | 0.13+ |
| `llama-cpp-rs` | llama.cpp FFI (Path B) | latest |
| `candle-core` | Rust ML framework (Path A) | 0.8+ |
| `candle-transformers` | Model implementations | 0.8+ |
| `memmap2` | Memory-mapped file I/O | 0.9+ |
| `prometheus` | Metrics exposition | 0.13+ |
| `tracing` | Structured logging | 0.1+ |
| `rustls` | TLS (optional) | 0.23+ |

### Go Packages (Manager)

| Package | Purpose |
|---------|---------|
| `google.golang.org/grpc` | gRPC client to engine |
| `github.com/gin-gonic/gin` or `net/http` | REST API |
| `unheaded/pkg/transport` | Wotan gRPC-first transport |
| `unheaded/pkg/discovery` | Service registration |
| `cilium/ebpf` | BPF map reader |
| `prometheus/client_golang` | Metrics |

### Build + Deploy

- Rust engine: `cargo build --release` with ROCm/HIP feature flag
- Go manager: standard `go build` within unheaded workspace
- Systemd units: `champion-engine.service`, `champion-manager.service`
- NixOS module: `services.champion.enable = true` (future)

---

## 5. UPGRADE PATH + FUTURE WORK

### Hardware Scaling

| Upgrade | Impact on Champion Engine |
|---------|--------------------------|
| +32GB DDR5 (→48-64GB) | Larger FAISS indexes, RAFT training batches, no swap |
| +2TB NVMe | Dedicated model/dataset drive, faster checkpoint I/O |
| RX 7900 XTX (24GB VRAM) | 13B models, higher quant (Q8), 8+ concurrent requests |
| Second GPU (needs new mobo) | Tensor parallelism, train while serving |

### Software Roadmap

1. **RAFT Integration** — After RAG demo, plug QLoRA training loop into Champion
   Engine. Train adapter → merge → hot-swap model without restart.
2. **Speculative Decoding** — Use small draft model (1.5B) to predict tokens,
   verify with 7B model. 2-3x speedup on acceptance.
3. **Pure Candle Backend** — Migrate from llama-cpp-rs to pure Rust candle once
   ROCm/RDNA3 support matures. Eliminates C++ dependency entirely.
4. **Multi-Model Serving** — Load multiple models, route by query type
   (code model for code questions, general model for conversation).
5. **Distributed Inference** — Split model across WEST + EAST over P2P link.
   Tensor parallelism over 192.168.13.0/30. Requires Monad flow awareness.

---

## 6. RISK REGISTER

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| candle ROCm on RDNA3 immature | High | Medium | Path B (llama-cpp-rs) as primary, candle as parallel experiment |
| PagedAttention complexity | Medium | High | Start with simple contiguous cache, add paging incrementally |
| 12GB VRAM limits concurrency | High | Medium | Aggressive quantization (Q4_K_M), preemption, CPU offload |
| eBPF uprobe on Rust binary | Medium | Low | USDT probes or tracepoint on ROCm runtime as fallback |
| Go-Rust IPC overhead | Low | Medium | Unix socket + /dev/shm shared memory, benchmark early |
| Scope creep into training | Medium | High | Hard boundary: engine is inference-only until RAG demo ships |

---

## 7. AGENT EXECUTION NOTES

When expanding this plan into a verbose battle plan:

- **Split across 3-4 agents** to avoid token cap:
  - Agent 1: Phases 0-3 (Rust foundation)
  - Agent 2: Phases 4-6 (server + management + eBPF)
  - Agent 3: Phases 7-9 (integration + RAG migration)
  - Agent 4: Phases 10-11 + appendices (observability + hardening)
- **Commit cadence**: Every 4 steps (per Warmonger formula)
- **Skip protocol**: 3x time estimate or 2 failed debug attempts → skip + log
- **Path B first**: llama-cpp-rs backend before candle. Get something working, then optimize.
- **Test every layer**: TDD throughout — red-green-refactor per Developer skill
- **Benchmark constantly**: Every phase exit gate includes performance comparison vs baseline

---

*Champion Engine Plan — Drafted 2026-03-11*
*11 Phases. Rust forges the blade. Go wields it. eBPF traces its arc.*
*The Kingdom's own inference engine. No dependencies. No compromises.*
