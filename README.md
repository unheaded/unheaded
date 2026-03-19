# Unheaded

Unheaded is a configuration management platform that provisions backend infrastructure — service mesh, observability, security, and control plane — from declarative configuration. The Unheaded Protocol enables distributed computation by encoding state in IPv6 headers and processing it at each hop via eBPF.

## Building

Prerequisites: Linux (kernel 5.15+), Go 1.24+, Rust, Docker.

```bash
go build ./...
docker compose up -d
```

See [QUICKSTART.md](QUICKSTART.md) for the full walkthrough.

## The Protocol

The Monad wire format encodes a 20-byte register file in the IPv6 Hop-by-Hop Options header. At each hop, eBPF code reads and writes registers; the packet becomes distributed working memory. Kingdom Mode (EVPN-VXLAN) extends this to 48 bytes per packet with zero wire overhead. Sophia dictionaries (pinned BPF maps) provide field semantics. The Unheaded Protocol Computer (UPC) is a BPF-compliant CPU emulator in XDP. See [docs/UPC_REFERENCE_MANUAL.md](docs/UPC_REFERENCE_MANUAL.md) for details.

Protocol specifications (IETF Experimental track):

- [draft-bellis-unheaded-protocol-foundation-06](docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md) — Monad wire format
- [draft-bellis-unheaded-sophia-dictionary-03](docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md) — BPF map dictionaries
- [draft-bellis-unheaded-wotan-memory-03](docs/protocol/draft-bellis-unheaded-wotan-memory-03.md) — distributed memory model
- [draft-bellis-unheaded-mbc-isa-00](docs/protocol/draft-bellis-unheaded-mbc-isa-00.md) — MBC instruction set architecture
- [draft-bellis-unheaded-shim-00](docs/protocol/draft-bellis-unheaded-shim-00.md) — Shim execution pipeline
- [draft-bellis-unheaded-pqc-authentication-00](docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md) — post-quantum authentication

## Architecture

```
Layer 5  Presentation         Dashboard, Kanban
Layer 4  Application          Service personas (timeguru, captain, architect, etc.)
Layer 3  Infrastructure       Wotan message bus, trace-collector, gateway
Layer 2  Control Plane        unheaded-daemon (drift detection, reconciliation)
Layer 1  Data Plane           eBPF programs (XDP/TC, Rust/Aya)
Layer 0  Host                 LXD / Docker / NixOS / bare metal
```

Network fabric: WireGuard IPv6 overlay, BGP EVPN with VXLAN. Deployment targets: NixOS, Docker Compose, LXD. See [docs](docs/) for security model, compliance tiers, and architecture details.

## License

GPL-3.0-or-later. See [LICENSE](LICENSE).

Protocol specifications dual-licensed GPL-3.0-or-later / Apache-2.0. Suricata integration (GPL-2.0) is process-isolated; see [docs/legal/SURICATA_GPL_ISOLATION.md](docs/legal/SURICATA_GPL_ISOLATION.md).

## AI-Assisted Development

Unheaded is built with AI pair programming across multiple LLM providers:

- [Claude](https://claude.ai) (Anthropic) — Primary development partner. Architecture, implementation, protocol specs, security review.
- [Gemini](https://gemini.google.com) (Google) — Research, analysis, cross-referencing.
- [ChatGPT](https://chatgpt.com) (OpenAI) — Research, drafting, exploration.
- [Zhen AI](https://github.com/unheaded/unheaded/wiki/Zhen-AI) — In-house RAG pipeline. Mistral-7B via llama.cpp (ROCm), 1.52M FAISS vectors, RAFT-trained on Unheaded codebase + 9,739 RFCs.

All code is human-reviewed and human-approved. The humans make the decisions; the AIs accelerate the work.

## Author

Stevie Bellis — stevie@bellis.tech
