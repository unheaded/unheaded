

# Unheaded

Configuration management automation platform. Provisions backend infrastructure — service mesh, observability, security, control plane — from declarative configuration. The Unheaded Protocol encodes state in IPv6 Hop-by-Hop headers, processed at each hop via eBPF.

**Status:** Age 1 (Alpha) and Age 2 (Beta) complete; Age 3 (Public Release) in progress. Dual bare metal (WEST + EAST) online with cross-host BPF flow graph. Wire format frozen at v0x01. **ASCEND-LINUX Phase 1 complete (2026-07-02): interactive xv6 shell on the UPC** — fork/exec/wait, per-pid MMU isolation, in-BPF filesystem reader (`ls`/`cat`/`echo`/`wc` over a real `fs.img`). Doom runs on the same interpreter, playable in-browser via `doom-runner`. Next: Unheaded Linux, an own-built minimal OS on the UPC (ADR-081).

## Building

Requires Linux (kernel 5.15+ baseline; kernel 6.17+ for the ASCEND-LINUX BPF verifier features), Go 1.25+, Rust nightly, Docker.

```bash
go build ./...
sudo docker compose up -d
```

See [QUICKSTART.md](QUICKSTART.md) for details.

## Architecture

```
Layer 5  Dashboard, Kanban, Zhenai Web UI
Layer 4  timeguru, captain, architect, micromanager, monad, sophia
Layer 3  Wotan (message bus), trace-collector, gateway, Akira (health)
Layer 2  unheaded-daemon (drift detection, reconciliation)
Layer 1  23 eBPF programs (XDP/TC, Rust/Aya, Go/cilium)
Layer 0  LXD / Docker / NixOS / bare metal
```

Ports 16666-26666. gRPC with mTLS default. See `pkg/ports/ports.go`.

## Protocol

Monad wire format: 20-byte register file in IPv6 HbH extension header. Wire format frozen at v0x01. 12 IANA registries in foundation spec draft-06.

- [draft-bellis-unheaded-protocol-foundation-06](docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md)
- [draft-bellis-unheaded-sophia-dictionary-03](docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md)
- [draft-bellis-unheaded-wotan-memory-03](docs/protocol/draft-bellis-unheaded-wotan-memory-03.md)
- [draft-bellis-unheaded-mbc-isa-00](docs/protocol/draft-bellis-unheaded-mbc-isa-00.md)
- [draft-bellis-unheaded-shim-00](docs/protocol/draft-bellis-unheaded-shim-00.md)
- [draft-bellis-unheaded-pqc-authentication-00](docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md)

## Stack

Go services, Rust eBPF (Aya), Wotan message bus (gRPC + HTTP), PostgreSQL (multi-DB "The Well"), llama.cpp + Mistral-7B (ROCm), vanilla JS frontend, .deb packaging + systemd, SLH-DSA / ML-DSA-65 post-quantum crypto, deterministic Sealed Cask builds.

## UPC compute substrate

The Unheaded Protocol Computer (UPC) is a virtual CPU built on the protocol itself: Monad as the transport bus, Wotan as memory, Sophia as microcode, eBPF as the interpreter. One interpreter runs multiple guest workloads, feature-partitioned to fit the eBPF verifier's 1M-instruction budget (ADR-080). The Dream Ladder runs from packet stamping (L1) to running an OS (L6).

- **Doom** — L3 computational-completeness proof; playable in-browser via `doom-runner`.
- **xv6** — L5; interactive shell, fork/exec/wait, per-pid MMU isolation, in-BPF filesystem reader (`ls`/`cat`/`echo`/`wc` over `fs.img`). Phase 1 complete 2026-07-02.
- **Unheaded Linux** — next; an own-built minimal OS evolving from the xv6 substrate, scaling toward the Yggdrasil golden image (ADR-081). Long-term.

Robust documentation:
- [`wiki/UPC-Overview.md`](wiki/UPC-Overview.md) — substrate, BPF maps, MBC ISA, Boot Protocol v2
- [`wiki/Unheaded-Protocol.md`](wiki/Unheaded-Protocol.md) — Monad + Sophia + Wotan + MBC + Shim + PQC
- [`wiki/UPC-Dream-Ladder.md`](wiki/UPC-Dream-Ladder.md) — six-level ascent + gates
- [`wiki/Doom-on-UPC.md`](wiki/Doom-on-UPC.md) — L3 proof
- [`wiki/Linux-on-UPC.md`](wiki/Linux-on-UPC.md) — ASCEND-LINUX, current frontier
- [`wiki/MBC-ISA-Reference.md`](wiki/MBC-ISA-Reference.md) — opcode + encoding reference

Code: `crates/doom-runner/`, `crates/xv6-mbc/`, `cmd/upc-bootctl/`, `ebpf/monad-cpu-ebpf/`. Roadmap: `docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md`.

## Zhen AI

`crates/zhend/` (anti-fragile gossip knowledge substrate, PQC-secured) + `crates/zhenai-forge/` (LoRA fine-tuning on Gemma-4 / Mistral-7B via ROCm) + Flask web UI at port 20103 (1.52M-vector RAG corpus). Currently consolidating WAVE12 Kingdom RAFT LoRA wins; eval Δ −14.32 vs base on held-out Kingdom prefixes.

## License

GPL-3.0-or-later. Protocol specs dual-licensed GPL-3.0-or-later / Apache-2.0.

## Author

Stevie Bellis — stevie@bellis.tech
