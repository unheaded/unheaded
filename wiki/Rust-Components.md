# Rust Components

Where the project uses Rust: the eBPF data plane (it must be Rust/C to run in the kernel), the UPC toolchain, and Zhen's training stack. Program-by-program eBPF detail: [[eBPF Programs|eBPF-Programs]].

## `ebpf/` workspace (Aya, GPL-2.0-only)

| Crate | What |
|-------|------|
| `monad-common`, `common`, `pqc-common` | Shared wire types (Foundation draft), PQC auth types |
| `shield-ebpf` | Boundary enforcement: XDP ingress (BIRTH), TC egress (DEATH) |
| `hop-ebpf` | Per-hop XDP: verify, mutate, re-sign the Monad |
| `firewall-ebpf` | Stateful XDP firewall, connection tracking |
| `maglev-ebpf` | Maglev consistent-hash load balancer |
| `qos-ebpf` | Token bucket + CoDel AQM |
| `failover-ebpf` | Health scoring + automatic failover |
| `canary-ebpf` | Canary traffic splitting |
| `nfv-ebpf` | Tail-call network function chain |
| `version-ebpf` | Protocol version translation (XDP + TC) |
| `compliance-ebpf` | Wire-level zero-trust policy checks |
| `anomaly-ebpf` | Quantized decision-tree anomaly detection at XDP |
| `yaldabaoth-ebpf` | Chaos injection on TC egress |
| `monad-cpu-ebpf` | The UPC: MBC fetch-decode-execute CPU. Build with `--features ascend-linux` for the xv6 kernel |
| `packet-marker`, `flow-tracker`, `latency-probe`, `syscall-tracer` | Tracing programs |

Every program is loaded through the kernel verifier in CI (`cmd/ebpf-load-check`, ADR-012).

## `crates/`

| Crate | What |
|-------|------|
| `monad-mbc` | MBC ISA assembler, disassembler, RV32I → MBC translator ([[MBC-ISA-Reference]]) |
| `upc-api` | Host-side contract for loading and running guests on the UPC; both loaders use it (ADR-080) |
| `upc-bootstub` | Stage-1 boot stub for uClinux/Linux on the UPC |
| `xv6-mbc` | The UPC kernel and userland (Unheaded-authored since Phase 2.4) ([[Linux-on-UPC]]) |
| `heimdall-bpf` | kprobe scaffold for Mímir's Law ([[Mimirs-Law]]) |
| `lich-fuzz` | The Lich: fuzzing for the protocol and eBPF programs ([[LICH-Campaigns]]) |
| `zhend` | Zhen Layer 0: single-node gRPC store; gossip and QUIC written, never spawned (DP-209) |

## Rust binaries under `cmd/`

`ebpf-collector`, `ebpf-loader`, `ebpf-load-check`, `upc-bootctl`. The Rust `waf` was deleted 2026-11-04 and is archived in `unheaded-waf`.

---

> **Source:** `ebpf/*/Cargo.toml`, `crates/*/Cargo.toml` · [docs/architecture/RUST_COMPONENTS.md](../docs/architecture/RUST_COMPONENTS.md) (2026-02 roadmap)
