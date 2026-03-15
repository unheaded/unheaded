# Unheaded Software Bill of Materials (SBOM)

**Generated:** 2026-03-15
**Format:** SPDX-2.3 compatible
**Repository:** github.com/unheaded/unheaded
**Main License:** GPL-3.0-or-later
**Protocol Specs:** GPL-3.0/Apache-2.0 (dual-licensed)

---

## Summary

| Ecosystem | Total Dependencies | Direct | Indirect | Key Licenses |
|-----------|-------------------|--------|----------|--------------|
| **Go** | 99 | 21 | 78 | BSD-3-Clause (40), MIT (29), Apache-2.0 (22) |
| **Rust** | 346 | -- | 346 | MIT OR Apache-2.0 (308), MIT (29), Apache-2.0 (8) |
| **Python** | 108 | 108 | -- | Apache-2.0, MIT, BSD (ML/AI tooling) |
| **JavaScript** | 0 | 0 | 0 | Vanilla JS, no npm dependencies |
| **Total** | **553** | | | |

---

## Go Dependencies (99 modules)

**Source:** `go list -m -json all` against `go.mod` (Go 1.24.0)
**Detail file:** `sbom/go-dependencies.json`

### License Breakdown

| License | Count | Notes |
|---------|-------|-------|
| BSD-3-Clause | 40 | golang.org/x/*, google/*, protobuf, modernc.org/* |
| MIT | 29 | zerolog, cilium/ebpf, BurntSushi/toml, goldmark |
| Apache-2.0 | 22 | Prometheus, gRPC, genproto, envoyproxy |
| BSD-2-Clause | 4 | gorilla/websocket, godbus/dbus, pkg/errors, gopkg.in/check |
| ISC | 1 | davecgh/go-spew |
| MPL-2.0 | 1 | hashicorp/golang-lru/v2 |
| GPL-2.0 | 1 | unheaded/doomgeneric (ISOLATED -- see GPL Boundary) |
| MIT AND Apache-2.0 | 1 | gopkg.in/yaml.v3 |

### Key Direct Dependencies (21)

| Module | Version | License |
|--------|---------|---------|
| github.com/BurntSushi/toml | v1.6.0 | MIT |
| github.com/cilium/ebpf | v0.20.0 | MIT |
| github.com/cloudflare/circl | v1.6.3 | BSD-3-Clause |
| github.com/fsnotify/fsnotify | v1.7.0 | BSD-3-Clause |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/gorilla/mux | v1.8.1 | BSD-3-Clause |
| github.com/gorilla/websocket | v1.5.3 | BSD-2-Clause |
| github.com/lib/pq | v1.10.9 | MIT |
| github.com/prometheus/client_golang | v1.18.0 | Apache-2.0 |
| github.com/rs/zerolog | v1.31.0 | MIT |
| github.com/yuin/goldmark | v1.7.16 | MIT |
| golang.org/x/crypto | v0.43.0 | BSD-3-Clause |
| golang.org/x/sys | v0.40.0 | BSD-3-Clause |
| golang.org/x/text | v0.30.0 | BSD-3-Clause |
| golang.org/x/time | v0.5.0 | BSD-3-Clause |
| google.golang.org/grpc | v1.65.0 | Apache-2.0 |
| google.golang.org/protobuf | v1.34.1 | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | MIT AND Apache-2.0 |
| modernc.org/sqlite | v1.44.3 | BSD-3-Clause |
| github.com/sony/gobreaker | v0.5.0 | MIT |
| github.com/unheaded/doomgeneric | v0.0.0 | GPL-2.0 |

---

## Rust Dependencies (346 external crates, 30 workspace crates)

**Source:** 8 Cargo.lock files parsed
**Detail file:** `sbom/rust-dependencies.json`

### License Breakdown

| License | Count | Notes |
|---------|-------|-------|
| MIT OR Apache-2.0 | 308 | Standard Rust dual license (99 verified, 209 assumed) |
| MIT | 29 | tokio, tonic, hyper, tower, tracing ecosystem |
| Apache-2.0 | 8 | prost family (gRPC codegen) |
| Unlicense OR MIT | 1 | memchr |

### Workspace Crates (30 -- Unheaded-owned)

af-xdp, af-xdp-common, anomaly-ebpf, canary-ebpf, collector, compliance-ebpf,
ebpf-common, ebpf-loader, ebpf-programs, failover-ebpf, firewall-ebpf,
flow-tracker, hop-ebpf, latency-probe, maglev-ebpf, monad-common,
monad-cpu-ebpf, monad-mbc, monad-mbc-fuzz, nfv-ebpf, packet-marker,
pqc-common, qos-ebpf, shield-ebpf, syscall-tracer, trace-collector,
unheaded-common, version-ebpf, xdp-redirect, yaldabaoth-ebpf

### Key External Crates

| Crate | Version | License | Purpose |
|-------|---------|---------|---------|
| aya / aya-ebpf | 0.13.1 | MIT OR Apache-2.0 | eBPF framework |
| tokio | 1.44.2 | MIT | Async runtime |
| tonic | 0.12.3 | MIT | gRPC framework |
| prost | 0.13.5 | Apache-2.0 | Protobuf codegen |
| serde / serde_json | 1.x | MIT OR Apache-2.0 | Serialization |
| clap | 4.x | MIT OR Apache-2.0 | CLI parsing |
| hyper | 1.6.0 | MIT | HTTP library |
| tracing | 0.1.x | MIT | Instrumentation |

### Cargo.lock Files

1. `cmd/ebpf-collector/Cargo.lock`
2. `cmd/ebpf-collector/ebpf-programs/Cargo.lock`
3. `cmd/trace-collector/Cargo.lock`
4. `cmd/ebpf-loader/Cargo.lock`
5. `crates/monad-mbc/Cargo.lock`
6. `crates/monad-mbc/fuzz/Cargo.lock`
7. `ebpf/Cargo.lock`
8. `ebpf/af-xdp/Cargo.lock`

---

## Python Dependencies (108 packages)

**Source:** `pip freeze` from `~/.venv/zhen/` virtual environment
**Detail file:** `sbom/python-requirements.txt`

### Purpose

The Python environment provides AI/ML inference tooling (vLLM, transformers, langchain)
for the Sophia knowledge graph service. These are runtime dependencies for the AI
subsystem only and do not affect the core Go/Rust platform.

### Key Packages

| Package | Version | License | Purpose |
|---------|---------|---------|---------|
| torch | 2.10.0 | BSD-3-Clause | PyTorch ML framework |
| transformers | 5.3.0 | Apache-2.0 | HuggingFace model library |
| langchain | 1.2.12 | MIT | LLM orchestration |
| langgraph | 1.1.2 | MIT | Agent graph framework |
| sentence-transformers | 5.3.0 | Apache-2.0 | Embedding models |
| Flask | 3.1.3 | BSD-3-Clause | HTTP API framework |
| scikit-learn | 1.8.0 | BSD-3-Clause | ML utilities |
| numpy | 2.4.3 | BSD-3-Clause | Numerical computing |
| pydantic | 2.12.5 | MIT | Data validation |
| faiss-cpu | 1.13.2 | MIT | Vector similarity search |

### NVIDIA CUDA Libraries (GPU inference)

nvidia-cublas-cu12, nvidia-cuda-cupti-cu12, nvidia-cuda-nvrtc-cu12,
nvidia-cuda-runtime-cu12, nvidia-cudnn-cu12, nvidia-cufft-cu12,
nvidia-cufile-cu12, nvidia-curand-cu12, nvidia-cusolver-cu12,
nvidia-cusparse-cu12, nvidia-cusparselt-cu12, nvidia-nccl-cu12,
nvidia-nvjitlink-cu12, nvidia-nvshmem-cu12, nvidia-nvtx-cu12

All NVIDIA CUDA libraries are proprietary (NVIDIA EULA) but are standard
redistributable runtime components for GPU workloads.

---

## JavaScript Dependencies

**Status:** Zero npm dependencies. The dashboard and kanban UI are vanilla JavaScript
with no build toolchain, no bundler, and no framework dependencies.

The only `package.json` files in the repository are under `llama.cpp/` (vendored
upstream code, not part of the Unheaded build).

---

## System Dependencies

| Component | Version | License | Notes |
|-----------|---------|---------|-------|
| Linux kernel | 6.x | GPL-2.0 | Required for eBPF (kernel boundary, not linked) |
| LXD | 5.x | Apache-2.0 | Container runtime |
| Docker | 24.x | Apache-2.0 | Container runtime (alternative) |
| containerd | 1.7.x | Apache-2.0 | Container runtime (alternative) |
| HAProxy | 2.8+ | GPL-2.0 | Edge + internal load balancer |
| Nginx | 1.25+ | BSD-2-Clause | Per-app sidecar proxy |
| ROCm/CUDA | varies | Proprietary | GPU compute (AI inference only) |
| Go | 1.24.0 | BSD-3-Clause | Build toolchain |
| Rust | nightly | MIT OR Apache-2.0 | Build toolchain (eBPF) |

---

## GPL Boundary Verification

### PASS -- No GPL leakage into core

The GPL boundary is clean. The only GPL-licensed dependency in the Go module graph
is `github.com/unheaded/doomgeneric`, which is the DOOM engine used for computational
completeness proof (DOOM-on-Monad). This dependency is architecturally isolated:

1. **DOOM MBC bytecode** runs inside the Linux kernel's eBPF VM sandbox
2. **Communication** is via BPF map reads/writes only (data protocol, not code linkage)
3. **No runtime linking** between GPL code and MIT/Apache-2.0 code
4. **Independent compilation** -- DOOM is compiled by monad-mbc (Rust), Go tools are separate

All other Go dependencies are permissive: BSD-3-Clause, MIT, Apache-2.0, BSD-2-Clause,
ISC, or MPL-2.0.

All Rust crate dependencies are permissive: MIT, Apache-2.0, or dual-licensed
MIT OR Apache-2.0.

### MPL-2.0 Note

`hashicorp/golang-lru/v2` uses MPL-2.0, which is a weak copyleft license. MPL-2.0
is file-level copyleft -- modifications to MPL-2.0 files must remain MPL-2.0, but
it does not require the larger work to be MPL-2.0. This is compatible with GPL-3.0.

### HAProxy Note

HAProxy (GPL-2.0) is a system dependency used as a load balancer. It runs as a
separate process and communicates via network sockets only. No linking occurs.

---

## License Summary

| License | Go | Rust | Python | System | Compatible with GPL-3.0? |
|---------|---:|-----:|-------:|-------:|:------------------------:|
| BSD-3-Clause | 40 | 0 | ~15 | 2 | Yes |
| MIT | 29 | 29 | ~30 | 0 | Yes |
| MIT OR Apache-2.0 | 1 | 308 | 0 | 1 | Yes |
| Apache-2.0 | 22 | 8 | ~40 | 3 | Yes |
| BSD-2-Clause | 4 | 0 | 0 | 1 | Yes |
| ISC | 1 | 0 | 0 | 0 | Yes |
| MPL-2.0 | 1 | 0 | 0 | 0 | Yes (weak copyleft) |
| GPL-2.0 | 1 | 0 | 0 | 2 | Yes (isolated) |
| NVIDIA EULA | 0 | 0 | 15 | 1 | N/A (runtime, not linked) |
| Unlicense OR MIT | 0 | 1 | 0 | 0 | Yes |

**Result:** All dependencies are GPL-3.0 compatible. No AGPL dependencies found.
No license conflicts detected.

---

## Regeneration

To regenerate this SBOM:

```bash
./scripts/generate-sbom.sh
```

This will refresh all JSON files and regenerate this document from current dependency data.

---

*Generated by scripts/generate-sbom.sh -- Do not edit manually*
