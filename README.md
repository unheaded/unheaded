# Unheaded

**Production-ready infrastructure in hours, not months.**

Unheaded is a configuration management automation platform. You bring your application ("the head"); we provide everything else ("unheaded") — service mesh, observability, security hardening, and control plane from declarative configuration.

The Unheaded Protocol enables distributed computation by encoding state in IPv6 headers and processing it at each hop via eBPF. Proof of concept: DOOM runs on the protocol computer.

## What It Does

| Layer | What | How |
|-------|------|-----|
| **Observability** | eBPF-based L2-L7 tracing | Rust/Aya XDP programs, zero user data access |
| **Service Mesh** | 10 microservices + Wotan message bus | Go, gRPC-first with mTLS, HTTP fallback |
| **Security** | Container hardening, PQ crypto, cert rotation | NixOS/Docker, SLH-DSA, 1-day certs via Akira |
| **AI Operations** | Zhenai autonomous champion | Mistral-7B + RAFT LoRA, 1.76M vector RAG, 51 runbooks |
| **Bare Metal** | Dual-host production cluster | WEST + EAST, .deb packages, systemd, APT repo |
| **Protocol** | IPv6 distributed computation | 20-byte Monad register, frozen wire format v0x01 |

## Quick Start

```bash
# Build
go build ./...

# Start services (Docker)
sudo docker compose up -d

# Start services (bare metal)
sudo apt install unheaded-wotan unheaded-timeguru unheaded-akira
sudo systemctl start unheaded-wotan
```

See [QUICKSTART.md](QUICKSTART.md) for the full walkthrough.

## Architecture

```
Layer 5  Presentation    Dashboard (vanilla JS), Kanban, Zhenai Web UI
Layer 4  Application     timeguru, captain, architect, micromanager, monad, sophia
Layer 3  Infrastructure  Wotan (message bus + gRPC), trace-collector, gateway, Akira (health)
Layer 2  Control Plane   unheaded-daemon (drift detection, reconciliation)
Layer 1  Data Plane      23 eBPF programs (XDP/TC, Rust/Aya, Go/cilium)
Layer 0  Host            LXD / Docker / NixOS / bare metal (WEST + EAST)
```

**Ports**: All services use "The Doom Range" (16666-26666). See `pkg/ports/ports.go`.

**Transport**: gRPC with mTLS default (ADR-034). 1-day service certs, 3-day CA, Akira auto-rotation.

## The Protocol

The Monad wire format encodes a 20-byte register file in the IPv6 Hop-by-Hop Options header. At each hop, eBPF code reads and writes registers — the packet becomes distributed working memory.

Protocol specifications (IETF Experimental track):

- [draft-bellis-unheaded-protocol-foundation-06](docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md) — Monad wire format (12 IANA registries)
- [draft-bellis-unheaded-sophia-dictionary-03](docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md) — BPF map dictionaries
- [draft-bellis-unheaded-wotan-memory-03](docs/protocol/draft-bellis-unheaded-wotan-memory-03.md) — Distributed memory model
- [draft-bellis-unheaded-mbc-isa-00](docs/protocol/draft-bellis-unheaded-mbc-isa-00.md) — MBC instruction set
- [draft-bellis-unheaded-shim-00](docs/protocol/draft-bellis-unheaded-shim-00.md) — Shim execution pipeline
- [draft-bellis-unheaded-pqc-authentication-00](docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md) — Post-quantum authentication

## Zhenai — The Kingdom Champion

Autonomous AI operations agent. Acts as SOC/NOC team with controlled trust levels:

- **RAG Pipeline**: 1.76M FAISS vectors, all-MiniLM-L6-v2 embeddings, Mistral-7B inference
- **LoRA Training**: Custom Rust pipeline (zhenai-forge), ROCm GPU acceleration, .zlora format
- **51 Runbooks**: 7 categories (infra, network, security, data, observe, doom, deploy)
- **Akira Health**: 66.67% consensus auto-restart, Wotan integration, per-host configs
- **15 Chat Commands**: kingdom status, health, drift check, runbook execution, recall, scheduling

## By the Numbers

| Metric | Value |
|--------|-------|
| Production LOC | ~385K |
| Total LOC (with tests) | ~941K |
| Active services | 34 |
| eBPF programs | 23 |
| ADRs | 39 |
| Runbooks | 55+ |
| Kingdom skills | 20 |
| Tests | 800+ |
| Commits | 1,200+ |
| Bare metal hosts | 2 (WEST + EAST) |
| FAISS vectors | 1.76M |
| Protocol specs | 6 Internet-Drafts |

## Technology Stack

| Component | Technology |
|-----------|-----------|
| Services | Go 1.24+ |
| eBPF | Rust (Aya) + Go (cilium/ebpf) |
| Message Bus | Wotan (Go, gRPC + HTTP, active-passive replication) |
| AI Inference | llama.cpp + Mistral-7B (ROCm/AMD GPU) |
| AI Training | zhenai-forge (Rust, hipBLAS, LoRA) |
| Containers | LXD, Docker, NixOS |
| Frontend | Vanilla JS (no framework) |
| Database | PostgreSQL (The Well) |
| Crypto | SLH-DSA (FIPS 205), X25519, mTLS |
| Packaging | .deb + local APT repo + systemd |

## DOOM-over-IPv6

Computational completeness proof: DOOM (1993) runs on the Unheaded Protocol Computer — a BPF-compliant CPU emulator in XDP. RV32IM → MBC translation, 320x200 framebuffer via BPF maps, browser rendering at localhost:16666. See [docs/doom/](docs/doom/).

## License

GPL-3.0-or-later. See [LICENSE](LICENSE).

Protocol specifications dual-licensed GPL-3.0-or-later / Apache-2.0 for ecosystem adoption.

## AI-Assisted Development

Built with AI pair programming:

- **Claude** (Anthropic) — Primary development partner. Architecture, implementation, protocol specs, security review.
- **Zhenai** — In-house RAG champion. Mistral-7B + RAFT-trained LoRA, 1.76M vectors, 20 Kingdom skills.
- **Gemini** (Google) — Research, analysis.
- **ChatGPT** (OpenAI) — Research, exploration.

All code is human-reviewed and human-approved. The humans make the decisions; the AIs accelerate the work.

## Author

Stevie Bellis — stevie@bellis.tech
