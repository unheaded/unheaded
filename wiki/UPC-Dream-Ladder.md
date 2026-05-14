# UPC Dream Ladder

The Dream Ladder is the six-level ascent from "packet stamping works" to "Linux runs on the UPC". Each level has a concrete gate criterion. The Computermancer owns level transitions; the Marshal enforces gate verification.

This page is the live status. The deep architecture is at [UPC Overview](UPC-Overview); current frontier work is at [Linux on UPC](Linux-on-UPC).

## Level status

| Level | Goal | Gate criterion | Status |
|---|---|---|---|
| **L1** Basic eBPF | XDP attach, packet parse, flow table | `bpftool prog list` + flow match rate | ✅ shipped (alpha era) |
| **L2** Stamping | Monad HbH write, CRC-16 valid | Wire capture, checksum verify | ✅ shipped |
| **L3** Framebuffer | Scanline render, 320×200 output | Visual verify, ≥35 fps benchmark | ✅ shipped — [Doom on UPC](Doom-on-UPC) |
| **L4** OS primitives | Syscalls, interrupts, scheduler, MMU | Unit tests, scheduling fairness, MMU isolation | ✅ shipped through L4f |
| **L5** xv6 kernel | xv6 boots, init runs | Boot log reaches `init: starting sh`, shell prompt visible | 🟡 Phase 1.6 (2026-05-14): `init: starting sh` shipped. Shell prompt blocked on fork/exec/wait handlers. |
| **L6** Linux | uClinux first, then Linux+MMU | `/bin/sh` prompt over Mode-C SSH session | ⏳ Phase 2+ |

## Per-level detail

### L1 — Basic eBPF (shipped, alpha)

XDP program loads cleanly on kernel 5.15+, parses Ethernet → IPv6, identifies Monad HbH option (type 0x3E), maintains a flow table keyed by 5-tuple. No state changes per packet beyond stats counters.

Gate: `bpftool prog list` shows the program attached; flow-match rate measured on `cmd/trace-collector-go`.

### L2 — Stamping (shipped)

Each hop reads the 20-byte Monad register file, optionally writes a register, recomputes CRC-16, advances the hop counter, and forwards. Validates against the Monad foundation spec (FROZEN at v0x01).

Gate: wire capture shows monotonic hop counter and valid CRC across a multi-namespace ring.

### L3 — Framebuffer (shipped — Doom-on-Monad)

Scanline rendering to SCREEN_MAP at 320×200, palettized. Doom runs as proof of generality.

Gate: visual verify of Doom title screen + ≥35 fps measured benchmark. Both met; the ASCII-art proof is the actual Doom canvas streaming to a browser.

See [Doom on UPC](Doom-on-UPC) for the full story.

### L4 — OS primitives (shipped through L4f)

Sub-levels:

| Sub | Capability |
|---|---|
| L4a | Syscall dispatch (INT 0x80 → handler table) |
| L4b | Linux-compat SYS_WRITE, SYS_BRK, SYS_FORK, SYS_EXIT, SYS_GETPID |
| L4c | Timer interrupt + scheduler round-robin (when `num_processes > 1`) |
| L4d | MMU emulation (TLB_MAP + page directory walk, gated on `cpu.mmu_enabled = 1`) |
| L4e | Block device syscalls (SYS_READ_BLOCK 200, SYS_WRITE_BLOCK 201) backed by RAMDISK_BASE_WORD at byte 0x800000 |
| L4f | TTY MMIO (byte 0xC001), keyboard MMIO (byte 0xFFFF), IVT region (byte 0–0x3FF) |

Gate: each sub-level has its own unit tests in `ebpf/monad-cpu-ebpf/` + integration tests via `cmd/upc-bootctl validate`.

### L5 — xv6 kernel (Phase 1.5 spike, 2026-05-14)

xv6-riscv vendored at `crates/xv6-mbc/upstream/` (MIT). MBC ISA gained five new opcodes (FENCE / MRET / SRET / LR.W / SC.W) gated behind `cfg!(feature = "ascend-linux")`.

**Achieved:** kernel boots end-to-end. consoleinit → printfinit → kinit → kvminit → kvminithart → procinit → trapinit → trapinithart → binit → iinit → fileinit → userinit → started=1 → scheduler → swtch → forkret → fsinit → kexec → SRET → priv=3 (user mode) → init's `main` → `open` → `dup` × 2 → `printf` → `vprintf` body → `write` syscall stub → `ecall`.

**Remaining:** `SYS_write` reads NUL at r9 (Phase 1.6 — suspect translator's spill-shadow on x17). Once unblocked, init prints `init: starting sh\n` and the L5 gate ships.

Detail: [Linux on UPC](Linux-on-UPC).

### L6 — Linux (Phase 2+)

Two sub-phases:

- **L6a** uClinux (no MMU). The Phase-2 stage-1 stub `crates/upc-bootstub/` is scaffolded. Kernel boot will reuse the BootParams + MEPC handoff from xv6.
- **L6b** full Linux with MMU. Requires `cpu.mmu_enabled = 1` + a real page-table walker in the BPF interpreter. The L4d MMU emulation is the substrate; xv6's Phase 1.2 per-pid pgd model is the dry run.

Gate: a `/bin/sh` prompt visible over an SSH session terminating in a network namespace inside the UPC. Demo Mode C from the original ASCEND-LINUX battle plan.

## Gate enforcement

The Marshal enforces gate verification at every level transition. Sequence:

```
MARSHAL — UPC GATE CHECK

Level [N] exit gate: [PASS/FAIL/NOT RUN]
Level [N+1] prerequisites: [MET/NOT MET]

Clearance: [PROCEED / HOLD / HALT]
```

You may NOT ascend until the current level gate passes. Dream Ladder is sequential. No shortcuts.

## Velocity

Empirical level-to-level wall time, for the record:

| Transition | Wall time | Notes |
|---|---|---|
| L1 → L2 | weeks | alpha-era foundation |
| L2 → L3 | months | Doom build pipeline took a year of bug-of-the-day |
| L3 → L4f | weeks | once Doom was stable, the syscall + MMU emulation grew quickly |
| L4f → L5 (substrate) | days | Phase 0 ISA + ABI freeze + Phase 1.1 SHIP gate |
| L5 (substrate) → L5 (user mode) | 1 day | Phase 1.4 + Phase 1.5 (2026-05-13 → 2026-05-14, ~24 hours across two attended + Marshal-led shifts) |

The L4 → L5 acceleration is the substrate-investment payoff. With a working MBC translator + BPF dispatcher + Doom proof in hand, bringing up a fresh kernel was nine architectural fixes and one CALL-target patcher.

## Cross-references

- [UPC Overview](UPC-Overview)
- [Linux on UPC](Linux-on-UPC) — L5 frontier
- [Doom on UPC](Doom-on-UPC) — L3 proof
- [MBC ISA Reference](MBC-ISA-Reference)
- [`references/battle-plan-ascend-linux-2026-05-08.md`](../references/battle-plan-ascend-linux-2026-05-08.md) — the 10-month phased campaign
- ADR-067, ADR-072, ADR-074, ADR-075

---

> **Source:** [docs/doom/](../docs/doom/) · [crates/xv6-mbc/](../crates/xv6-mbc/) · [ebpf/monad-cpu-ebpf/](../ebpf/monad-cpu-ebpf/) · [cmd/upc-bootctl/](../cmd/upc-bootctl/)
