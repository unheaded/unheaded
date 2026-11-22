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
| `zhend` | Zhen Layer 0: single-node gRPC store (DP-209) with UDP gossip. Its QUIC server, Monad bridge, topology observer and strata snapshot history were never spawned and were archived in `unheaded-zhend-subsystems` on 2026-11-21 (ADR-152) |
| `objtrust` (`unheaded-objtrust`) | Whether an eBPF object on disk may be loaded: the Rust twin of `pkg/ebpf/objtrust`, called by `ebpf-loader`, `ebpf-load-check`, `upc-bootctl` and the `ebpf-collector` collector before they read an object |
| `svc-token` (`unheaded-svc-token`) | EdDSA service tokens for Rust binaries, byte for byte the ones `pkg/auth` mints (ADR-103); used by the `ebpf-collector` collector ([[Service-Auth]]) |
| `upc-cartridge` | Host side of MBC cartridges: the `.cart` format, the validator, the `cart` tool ([[UPC-Cartridges]]) |
| `monad-mbc/fuzz`, `upc-api/fuzz` | cargo-fuzz targets: MBC decode, execute and round trip, the cartridge step and image; `upc-api`'s object-facts and slice-geometry gates |

The LICH-015 differential harness (`tomb/lich/LICH-015-mbc-differential/harnesses/diff_harness`, crate `lich015-diff-harness`) runs MBC programs on the reference interpreter and the cartridge step in lockstep and reports the first divergence ([[LICH-Campaigns]]).

## Rust binaries under `cmd/`

`ebpf-collector`, `ebpf-loader`, `ebpf-load-check`, `upc-bootctl`. The Rust `waf` was deleted 2026-11-04 and is archived in `unheaded-waf`.

`cmd/ebpf-collector` is its own workspace of three crates: `collector` (userspace: loads the programs, reads their ring buffers, publishes JSON to Wotan with its service token), `common` (crate `ebpf-common`, `#[repr(C)]` types shared with the kernel side) and `ebpf-programs` (an XDP packet counter and a kprobe TCP latency probe).

---

> **Source:** `ebpf/*/Cargo.toml`, `crates/*/Cargo.toml` · [docs/architecture/RUST_COMPONENTS.md](../docs/architecture/RUST_COMPONENTS.md) (2026-02 roadmap)
