# Unheaded Protocol Foundation

**Production-ready infrastructure in hours, not months.**

Unheaded is a configuration management automation platform that delivers the complete "suit of armor" for modern SaaS applications. You bring your application ("the head"), we provide everything else — from kernel tuning to the control plane.

---

## What is Unheaded?

A drop-in infrastructure platform providing:

- **Unheaded Protocol** — 20-byte IPv6 HBH option (the "Monad") as a distributed register file, flowing with every packet
- **eBPF-based observability** — Packet-level tracing from L2–L7 via `aya-rs` XDP/TC programs
- **Immutable infrastructure** — NixOS containers on LXD, BGP/VXLAN/EVPN underlay
- **Service mesh** — Built on [Wotan](https://github.com/unheaded/wotan) message bus
- **Control plane** — Declarative config with drift detection
- **Security baseline** — FEDRAMP, NIST, SOC2, PCI-DSS, HIPAA, ITAR, GDPR
- **Zero user data access** — Architectural isolation at every layer

---

## The Monad: Unheaded Protocol in One Paragraph

Every packet on the Unheaded fabric carries a **20-byte IPv6 Hop-by-Hop option** (option type `0x3E`, change-en-route=1).
This option — the **Monad** — is a compact distributed register file:

| Field | Width | Purpose |
|-------|-------|---------|
| `flow_action` | 4 bits | TRACE / SAMPLE / MIRROR / DROP / FORWARD |
| `hop_count` | 4 bits | Incremented at each hop |
| `flags` | 8 bits | TRACED / SAMPLED / CHAOS / MIRROR / CUSTOM |
| `circuit_state` | 8 bits | CLOSED / OPEN / HALF_OPEN |
| `src_service_id` | 8 bits | Source service identifier |
| `dst_service_id` | 8 bits | Destination service identifier |
| `regs[0..3]` | 4×16b | General-purpose in-flight registers |
| `scratch_{r0,r1}` | 2×8b | Ephemeral scratch (TTL / delay_us) |
| `crc` | 16 bits | CRC-16/CCITT over first 18 bytes |

Peers recompute and validate the CRC at every hop. The eBPF programs read, mutate, and re-sign the Monad in-place — no userspace round-trip.

See [`draft-bellis-unheaded-protocol-foundation-01.md`](draft-bellis-unheaded-protocol-foundation-01.md) for the full spec.

---

## Architecture

```
BARE METAL HOST (NixOS)
├── eBPF programs (attached at XDP / TC hooks)
│   ├── shield-ebpf        — ingress BIRTH / egress DEATH events
│   ├── hop-ebpf           — per-hop: verify CRC, Sophia lookup, circuit breaker
│   ├── yaldabaoth-ebpf    — chaos injection (bit-flip, delay, duplicate, truncate)
│   └── monad-cpu-ebpf     — MBC fetch-decode-execute CPU inside eBPF
│
├── Userspace (Go)
│   ├── wotan             — message bus (gRPC + REST + pub/sub)
│   ├── trace-collector    — eBPF ring buffer → Wotan bridge (Rust)
│   ├── dashboard-backend  — metrics aggregator + WebSocket
│   ├── kanban-app         — the meta moment (Unheaded building itself)
│   └── unheaded-daemon    — control plane agent
│
└── LXD containers (NixOS)
    ├── BGP/VXLAN/EVPN underlay
    └── Per-service immutable containers
```

---

## Project Structure

```
unheaded-protocol/
│
├── ebpf/                         # aya-rs eBPF workspace (Rust, bpfel-unknown-none)
│   ├── Cargo.toml                # Workspace root — monad-common + 4 BPF programs
│   ├── monad-common/             # Shared types: Monad, AnamnesisEvent, MBC ISA, CRC-16
│   │   └── src/lib.rs            # #![no_std] — compiles on host AND bpf target
│   ├── shield-ebpf/              # XDP ingress (BIRTH) + TC egress (DEATH)
│   ├── hop-ebpf/                 # Per-hop XDP processor (HOP events, Sophia, circuit breaker)
│   ├── yaldabaoth-ebpf/          # TC egress chaos injection (Demiurge corrupts)
│   └── monad-cpu-ebpf/           # MBC fetch-decode-execute CPU (computational completeness PoC)
│
├── crates/                       # Host-side Rust crates
│   └── monad-mbc/                # MBC ISA assembler, disassembler, RV32I→MBC translator
│       ├── src/assembler.rs      # Two-pass assembler with label resolution
│       ├── src/disasm.rs         # Disassembler + listing generator
│       └── src/translator.rs     # RV32I binary → MBC binary translator
│
├── cmd/                          # Go service binaries
│   ├── trace-collector/          # eBPF ring buffer → Wotan bridge (Rust binary)
│   ├── dashboard-backend/        # Metrics aggregator
│   ├── kanban-app/               # The meta moment
│   ├── unheaded/                 # Unheaded CLI
│   └── unheaded-daemon/          # Control plane agent
│
├── docs/
│   ├── doom-over-ipv6-plan.md    # Doom-over-IPv6 PoC using MBC CPU in eBPF
│   ├── ARCHITECTURE.md
│   └── SECURITY.md
│
├── nix/                          # NixOS container definitions (flake.nix)
├── scripts/                      # Deployment automation
├── references/                   # Living roadmap (timeline.md / .json / .yaml)
│
├── draft-bellis-unheaded-protocol-foundation-01.md  # Internet-Draft spec
├── Makefile                      # Full build pipeline
└── .github/workflows/ci.yml      # CI: Go tests + Rust stable + eBPF nightly build
```

---

## eBPF Programs

All eBPF programs are written in **Rust** using [aya-rs](https://aya-rs.dev/).
They require `bpfel-unknown-none` target + nightly Rust for `build-std=core`.

### shield-ebpf — XDP ingress + TC egress

Attached at the ingress XDP hook and egress TC hook of the host NIC.

- **BIRTH** event: packet arrives, no Monad → inject one, emit `BIRTH` to ring buffer
- **DEATH** event: packet leaves last hop → emit `DEATH` to ring buffer

### hop-ebpf — Per-hop XDP processor

The ALU of the Unheaded Protocol. At each router/gateway:

1. Parse ETH → IPv6 → HBH options to locate the Monad option (`0x3E`)
2. Verify CRC-16/CCITT; drop if invalid (circuit protection)
3. Increment `hop_count`; enforce `max_hops` limit
4. Consult **Sophia** BPF HashMap for QoS, circuit, and service metadata
5. Update circuit breaker state (`CLOSED/OPEN/HALF_OPEN`)
6. Apply `flow_action` semantics (TRACE/SAMPLE/MIRROR/DROP/FORWARD)
7. Recompute CRC, write Monad back in-place
8. Emit `HOP` event if sampled, traced, or rate-selected

**BPF Maps**: `ANAMNESIS` (8 MiB ring buffer), `SOPHIA` (65536 entries), `CIRCUIT_ERRORS`, `CONFIG`, `STATS`

### yaldabaoth-ebpf — Chaos injection TC classifier

Named for the Demiurge. Intentionally corrupts packets at the TC egress hook.

| Mode | Effect |
|------|--------|
| `BIT_FLIP` | XOR a random byte in the Monad — invalidates CRC |
| `DELAY` | Write `delay_us` to `scratch_r0`, recompute CRC |
| `DUPLICATE` | `bpf_clone_redirect` then mark CHAOS |
| `TRUNCATE` | Zero bytes 8-19 of Monad — no CRC recompute |
| `CHAOS_MARKER` | Set `CHAOS` flag, recompute CRC |

**Chaos targets** are configured via `CHAOS_TARGETS` BPF HashMap keyed by flow label.

### monad-cpu-ebpf — MBC CPU in eBPF

A fetch-decode-execute loop running entirely inside an XDP program. Triggered by packets with `flags::CUSTOM` set.

- **MBC ISA**: 32-bit fixed-width `[opcode:8][dst:4][src:4][imm16:16]`, 16 GP registers, Z/N/C flags
- 512 instructions per packet tick (bounded for BPF verifier)
- **BPF Maps**: `ROM_MAP` (64K words), `RAM_MAP` (256K words sparse), `SCREEN_MAP` (64K bytes, 320×200), `KBD_MAP`, `CPU_MAP`
- **Syscalls**: `SYS_DRAW_FRAME`, `SYS_GET_KEY`, `SYS_GET_TICKS`, `SYS_SLEEP`

See [docs/doom-over-ipv6-plan.md](docs/doom-over-ipv6-plan.md) for the full Doom-over-IPv6 PoC design.

---

## Userspace: trace-collector

The Rust binary that bridges eBPF ring buffers to Wotan.

```
ebpf ring buffer (AnamnesisEvent, 32 bytes)
    └── trace-collector
            ├── decode event
            ├── validate + enrich (CRC check, flag extraction)
            ├── publish to Wotan REST POST /api/v1/publish
            └── expose Prometheus metrics on :9100
```

**AnamnesisEvent** (1 cache line = 32 bytes):
```
[timestamp_ns: u64][event_type: u8][hop_id: u8][flow_label_lo: u16][monad: 20 bytes]
```

Usage:
```bash
sudo ./trace-collector \
  --pin-path /sys/fs/bpf/shield_anamnesis \
  --pin-path /sys/fs/bpf/hop_anamnesis \
  --wotan-url http://localhost:9090 \
  --wotan-topic ebpf.anamnesis \
  --metrics-port 9100 \
  --max-eps 10000
```

---

## monad-mbc: MBC Toolchain

The `crates/monad-mbc` crate provides a full MBC ISA toolchain for the host:

```rust
use monad_mbc::{assembler, disasm, translator};

// Assemble MBC source text → bytecode
let words = assembler::assemble(r#"
    MOVI r0, 42
    MOVI r1, 58
    ADD  r0, r1      ; r0 = 100
    HALT
"#)?;

// Disassemble
let listing = disasm::disasm_listing(&words);

// Translate RV32I binary → MBC binary
let mbc_words = translator::Translator::translate_program(&rv32i_words)?;
```

The `monad-cpu-ebpf` program executes bytecode stored in `ROM_MAP` — load compiled MBC programs into that map to run arbitrary code inside eBPF.

---

## Building

### Prerequisites

```bash
# Rust nightly (for eBPF bpfel-unknown-none target)
rustup toolchain install nightly --component rust-src
cargo install bpf-linker --no-default-features

# Go 1.24+
# NixOS or any modern Linux kernel 5.15+ for loading
```

### Build targets

```bash
# All eBPF programs (requires nightly + bpf-linker)
make ebpf

# Individual programs
make ebpf-shield
make ebpf-hop
make ebpf-yaldabaoth
make ebpf-monad-cpu

# monad-mbc toolchain crate (stable Rust)
make build-monad-mbc

# monad-common + monad-mbc tests (stable Rust, no BPF target needed)
make test-rust

# Verify monad-common compiles against bpfel-unknown-none (compat check)
make test-ebpf-compat

# Pin eBPF maps to /sys/fs/bpf/
make pin-ebpf   IFACE=eth0
make unpin-ebpf

# All Go services
make build

# Full test suite
make test
```

---

## Development

```bash
# Clone
git clone https://github.com/unheaded/unheaded-protocol.git
cd unheaded-protocol

# Run tests
make test        # Go tests (race detection)
make test-rust   # Rust tests (monad-common + monad-mbc, stable)

# Build eBPF
make ebpf

# Build services
make build

# Lint
make lint

# Format
make fmt
```

---

## Technology Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| eBPF Programs | Rust + aya-rs | Memory safety, zero overhead, BPF verifier-friendly |
| MBC Toolchain | Rust (stable) | Same language as eBPF, strong type safety |
| Services | Go 1.24 | Fast compilation, excellent concurrency |
| Message Bus | Wotan (Go + gRPC) | Service communication, pub/sub |
| Containers | NixOS on LXD | Immutable, declarative, reproducible |
| Underlay | BGP + VXLAN + EVPN | L2 overlay over L3, scalable fabric |
| Network Observability | eBPF + Prometheus | Per-packet metrics without sampling |
| Frontend | Vanilla JS | Dashboard, Kanban app |
| Gateway | Nginx (HTTP/3 + QUIC) | Edge termination |

---

## CI

The CI pipeline runs on every push to `main`, `develop`, and `theory/protocol-foundation`:

| Job | Toolchain | What |
|-----|-----------|------|
| `go-lint` | Go 1.24 | golangci-lint |
| `go-test` | Go 1.24 | `go test -race ./...` |
| `go-build` | Go 1.24 | Build all service binaries |
| `rust-check` | Rust stable | `cargo fmt --check` + `cargo clippy` on host-testable crates |
| `rust-test` | Rust stable | `cargo test` on `monad-common` + `monad-mbc` |
| `ebpf-build` | Rust nightly | `cargo build --target=bpfel-unknown-none -Z build-std=core` on all 4 eBPF programs |
| `rust-audit` | Rust stable | `cargo audit` (advisory) |
| `security-scan` | Go 1.24 | `govulncheck` + `gosec` SARIF |
| `ci-gate` | — | All required jobs must pass |

---

## Roadmap

See [references/timeline.md](references/timeline.md) for the living roadmap.

**Current Phase:** Alpha — Protocol Foundation + eBPF Workspace

**Shipped:**
- Unheaded Protocol Internet-Draft (draft-bellis-unheaded-protocol-foundation-01)
- `monad-common`: Monad struct, CRC-16/CCITT, AnamnesisEvent, SophiaEntry, MBC ISA types
- `shield-ebpf`: XDP BIRTH / TC DEATH events
- `hop-ebpf`: Per-hop processor (CRC verify, Sophia, circuit breaker, flow_action)
- `yaldabaoth-ebpf`: Chaos injection (5 modes)
- `monad-cpu-ebpf`: MBC fetch-decode-execute CPU in eBPF
- `trace-collector`: eBPF ring buffer → Wotan bridge
- `monad-mbc`: MBC assembler, disassembler, RV32I translator
- 25 Go microservices (Wotan, dashboard, kanban, timeguru, captain, micromanager, architect…)
- NixOS container configs + BGP/VXLAN/EVPN underlay

**Next:**
1. eBPF dashboard frontend (Campaign 2.3)
2. Monad/Sophia service integration tests
3. Multi-node BGP peering lab (NixOS containers)
4. Production hardening + compliance templates

---

## Contributing

We're in alpha. Not accepting external contributions yet, but feel free to open issues.

## License

GPL-2.0-only (eBPF workspace) / MIT (monad-mbc toolchain) — see individual `Cargo.toml` files.

## Contact

- **Web**: [unheaded.com](https://unheaded.com)
- **Email**: hello@unheaded.com
- **GitHub**: [github.com/unheaded](https://github.com/unheaded)

---

**Self-hosting is proof, not marketing.**

Built by Unheaded, running on Unheaded.
