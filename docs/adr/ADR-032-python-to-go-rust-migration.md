# ADR-032: Python → Go/Rust Migration — Maximum Hardware Performance

**Status:** Planned
**Date:** 2026-04-03
**Deciders:** Scientist, Developer, Computermancer, BlackMage

## Context

Zhenai's web UI (zhen_app.py), RAG pipeline (zhen_rag.py), scheduler (zhen_scheduler.py), and MCP server (zhen_mcp_server.py) are all Python. Python works but wastes hardware:

- **GIL**: Python can't truly parallelize CPU work across 12 threads
- **Memory**: Python's object overhead adds ~2-3x to data structure sizes
- **Startup**: Flask + FAISS + sentence-transformers takes 30+ seconds to load
- **Throughput**: Python's per-request overhead limits ops/sec
- **Dependencies**: pip ecosystem is a security surface (bitsandbytes CVEs, etc.)

On a 14GB RAM machine with 12 CPU threads and a powerful GPU, every wasted byte and cycle matters.

## Decision

### Migration Plan

Replace all Python components with Go (for HTTP services) and Rust (for compute-intensive work):

| Component | Current (Python) | Target | Rationale |
|-----------|-----------------|--------|-----------|
| Web UI + API | Flask (zhen_app.py) | **Go** (pkg/service/ template) | Go excels at HTTP servers, goroutines for concurrency |
| RAG pipeline | FAISS + sentence-transformers | **Rust** (zhenai-forge) | FAISS is C++ anyway, Rust FFI is natural |
| Scheduler | schedule lib (zhen_scheduler.py) | **Go** (systemd timers + watchdog) | Go's time.Ticker, integrates with ADR-029 watchdog |
| MCP server | mcp Python SDK (zhen_mcp_server.py) | **Go** (custom MCP impl) | MCP is just JSON-RPC over stdio, trivial in Go |
| Embedding | sentence-transformers (Python) | **Rust** (zhenai-forge) | ONNX Runtime in Rust, or custom GGUF inference |
| Training | zhenai-forge (Rust) | **Already Rust** ✓ | Done |

### Scientist's Analysis: Performance Gains

**Memory:**
- Python Flask + FAISS + 1.76M vectors: ~7.5GB RSS
- Go HTTP + mmap'd FAISS: ~2.5GB RSS (3x reduction)
- Reason: Python stores everything as heap objects with 28-byte overhead per int, Go uses value types

**Latency:**
- Python RAG query: ~60ms (FAISS search) + ~50ms (Flask overhead) = ~110ms
- Go RAG query: ~60ms (FAISS search) + ~2ms (Go HTTP overhead) = ~62ms
- Reason: Go's net/http is compiled, no interpreter overhead

**Startup:**
- Python: 30-45 seconds (load model + index + imports)
- Go + mmap: 2-3 seconds (mmap index, load on first query)
- Reason: mmap is lazy — OS loads pages on demand, not upfront

**Concurrency:**
- Python: 1 true thread (GIL), async via gevent/uvicorn
- Go: 12 real goroutines across 12 CPU threads
- Reason: Go's runtime scheduler distributes goroutines across OS threads

### Migration Phases

#### Phase 1: Go Web Server + API (replace zhen_app.py)
- `cmd/zhenai/main.go` — main service binary
- Uses `pkg/service/` template (already built)
- HTTP endpoints mirror current Flask routes
- Embeds static files via `go:embed`
- FAISS via CGO FFI to libfaiss

#### Phase 2: Rust Embedding Engine (replace sentence-transformers)
- Part of zhenai-forge
- Load all-MiniLM-L6-v2 as ONNX or custom GGUF
- Batch embedding via GPU (hipBLAS)
- No Python, no PyTorch, no HuggingFace

#### Phase 3: Go Scheduler + Watchdog (replace zhen_scheduler.py)
- Merge into `cmd/watchdog/main.go` (ADR-029)
- systemd timer integration
- Wotan pub/sub for health reporting

#### Phase 4: Go MCP Server (replace zhen_mcp_server.py)
- JSON-RPC over stdio in Go
- All tools: corpus_search, file_read/write, runbook_execute, service_health

#### Phase 5: Remove Python Entirely
- Delete `raft/*.py` (web app, scheduler, MCP, RAG)
- Remove `~/.venv/zhen` and `~/.venv/zhen-rocm`
- Single binary: `zhenai` (Go) + `zhenai-forge` (Rust)
- Zero Python in the Kingdom

### BlackMage's Security Note

Every Python dependency is an attack surface:
- `pip install` downloads arbitrary code from PyPI
- sentence-transformers pulls from HuggingFace (network dependency)
- Flask has had CVEs (SSRF, session issues)

Go and Rust binaries are statically compiled — no runtime dependencies, no pip, no downloads. The attack surface drops from "every Python package on PyPI" to "the Go/Rust compiler."

## Consequences

### Positive
- 3x memory reduction (7.5GB → 2.5GB)
- 60% latency reduction on RAG queries
- 10x faster startup
- True 12-thread parallelism
- Zero Python attack surface
- Single-binary deployment (Go embed)

### Negative
- Significant migration effort (4-5 phases)
- FAISS CGO binding requires careful memory management
- Loss of rapid Python prototyping speed during migration
- sentence-transformers replacement needs ONNX Runtime in Rust

### Risks
- FAISS CGO may have performance overhead vs Python's direct C++ binding
  - Mitigate: benchmark early, fall back to custom Rust FAISS-equivalent if needed
- MCP specification may evolve — Go implementation needs to track changes
  - Mitigate: MCP is simple JSON-RPC, changes are minimal
