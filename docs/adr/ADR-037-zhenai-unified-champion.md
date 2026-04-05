# ADR-037: Zhenai Unified Champion — The Suit of Armor

## Status: PLANNED

## Date: 2026-04-05
## Decision Makers: Stevie Bellis (Principal), Claude (Advisor)

---

## Context

Zhenai currently exists as 5 separate components:

| Component | Language | Purpose | Port |
|-----------|----------|---------|------|
| `raft/zhen_app.py` | Python | Flask web UI + RAG pipeline + chat commands | 20103 |
| `raft/zhen_mcp_server.py` | Python | MCP server for Claude Code (8 tools) | stdio |
| `raft/zhen_scheduler.py` | Python | Cron-based runbook scheduler + heartbeat | N/A |
| `pkg/champion/` | Go | Trust Level 1+2, sandboxed R/W, Kanban CRUD | N/A |
| `crates/zhenai-forge/` | Rust | LoRA training pipeline (GPU) | N/A |

These are **five separate processes** with no unified lifecycle, no shared state beyond
PostgreSQL, and no single binary that embodies "Zhenai." The armor has no body.

### The Metaphor

Unheaded's core identity: *"The user brings their app (the head), we provide everything
else (unheaded)."* Zhenai is the suit of armor that moves with no strings or body —
autonomous infrastructure operations. But currently the armor is in pieces: a gauntlet
here (MCP), a breastplate there (Flask), greaves somewhere else (scheduler).

## Decision

**Unify all Zhenai components into a single Go application with Rust FFI for GPU operations.**

### Architecture: The Unified Champion

```
┌─────────────────────────────────────────────────────┐
│                  zhenai (single binary)               │
│                                                       │
│  ┌──────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │  Web UI      │  │  MCP Server │  │  Scheduler  │ │
│  │  (HTTP)      │  │  (stdio/SSE)│  │  (cron)     │ │
│  │  :20103      │  │             │  │             │ │
│  └──────┬───────┘  └──────┬──────┘  └──────┬──────┘ │
│         │                 │                 │        │
│  ┌──────┴─────────────────┴─────────────────┴──────┐ │
│  │              Champion Core                       │ │
│  │  Trust levels, action logging, sandboxed ops     │ │
│  │  Conversation memory, runbook execution          │ │
│  │  Health monitoring, drift detection              │ │
│  └──────────────────────┬──────────────────────────┘ │
│                         │                            │
│  ┌──────────────────────┴──────────────────────────┐ │
│  │              RAG Engine                          │ │
│  │  ONNX Runtime (Go/Rust FFI) → FAISS index       │ │
│  │  Corpus: 1.76M vectors, 2.6GB index             │ │
│  └──────────────────────┬──────────────────────────┘ │
│                         │                            │
│  ┌──────────────────────┴──────────────────────────┐ │
│  │              Inference Bridge                    │ │
│  │  Local: llama.cpp (Mistral-7B + LoRA)           │ │
│  │  Remote: Claude MCP (handoff for complex tasks) │ │
│  └─────────────────────────────────────────────────┘ │
│                                                       │
│  ┌─────────────────────────────────────────────────┐ │
│  │              Persistence (The Well)              │ │
│  │  PostgreSQL: conversations, memories, actions    │ │
│  │  Wotan: health reports, system events            │ │
│  └─────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
```

### Migration Path (5 Phases)

**Phase 1: Go Web Server** (replaces zhen_app.py Flask)
- HTTP server with same routes (/api/v1/query, /health, etc)
- Chat command engine (15 commands → Go functions)
- PostgreSQL conversation logging
- Wotan integration (publish health reports)
- Templates: serve existing static/index.html

**Phase 2: Go MCP Server** (replaces zhen_mcp_server.py)
- JSON-RPC stdio transport (MCP protocol)
- 8 tools → Go handlers calling Champion Core
- Claude Code integration preserved

**Phase 3: Go Scheduler** (replaces zhen_scheduler.py)
- Cron-based runbook execution (merge with Akira health daemon)
- Heartbeat monitor
- Emergency stop

**Phase 4: Rust Embedding Engine** (replaces sentence-transformers)
- ONNX Runtime for all-MiniLM-L6-v2 inference
- FAISS bindings (or custom ANN index in Rust)
- CGo FFI: Go calls Rust for embedding + search
- This is the hardest phase — Python's ML ecosystem is why it survives

**Phase 5: Single Binary**
- `go build -o zhenai ./cmd/zhenai/`
- Rust components linked via CGo
- One systemd unit: `unheaded-zhenai.service`
- One .deb package: `unheaded-zhenai`
- One health endpoint: `:20103/health`
- The armor moves as one piece

### Performance Gains (Expected)

| Metric | Current (Python) | Unified (Go+Rust) |
|--------|-----------------|-------------------|
| Startup | 30-45s (FAISS load) | 2-3s (mmap lazy) |
| Memory | 7.5GB (3 processes) | 2.5GB (one process, shared index) |
| RAG latency | 110ms (Python GIL) | 40-60ms (real concurrency) |
| Concurrency | 1 thread (GIL) | 12+ goroutines |
| Processes | 5 | 1 |
| .deb packages | 3+ | 1 |

### What Stays Python (Temporary)

- `raft/scripts/distill_qa.py` — one-shot batch tool, not a service
- `raft/scripts/05_generate_qa.py` — training data generation
- Embedding scripts — until Phase 4 Rust engine exists

## Consequences

### Positive
- Single binary, single process, single systemd unit
- 3x memory reduction (no Python interpreter overhead per process)
- Real concurrency (Go goroutines, no GIL)
- Consistent error handling and logging (zerolog)
- Unified trust model — Champion Core controls all access
- One .deb, one health check, one Akira target
- The armor moves as one piece — Zhenai is *one thing*

### Negative
- Major rewrite effort (~5-8 sessions for Phases 1-3, ~3-5 for Phase 4-5)
- Rust ONNX/FAISS FFI is non-trivial (Phase 4 is the hardest)
- Temporary regression: Python features must be ported 1:1 before switching
- Two codebases during migration (Python stays until Go+Rust is proven)

### Mitigations
- Phase-by-phase migration — each phase is independently deployable
- Python stays running until Go replacement passes all integration tests
- Feature flag: `ZHENAI_BACKEND=go|python` for A/B testing during migration
- Rust embedding engine is the long pole — can ship Phases 1-3 with Python embedding subprocess as bridge

## References

- ADR-032: Python → Go/Rust Migration (general strategy)
- ADR-031: Hybrid Model Handoff (inference bridge architecture)
- ADR-019: Zhen Champion Agent (trust levels, sandboxed ops)
- ADR-017: Zhen Hybrid Inference (Claude API → MCP)
- `pkg/champion/` — existing Go Champion implementation (18 tests)
- `raft/zhen_app.py` — current Python web UI (62KB)
- `raft/zhen_mcp_server.py` — current MCP server (8 tools)
