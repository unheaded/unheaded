# ADR-067 — MBC ISA v2 + UPC ABI v1 (ASCEND-LINUX Phase 0 freeze)

**Status:** Accepted (decisions made 2026-05-08; implementation lands across Phase 0)
**Date:** 2026-05-08
**Deciders:** Stevie + unheaded-architect (BPF / verifier impact) + unheaded-computermancer (UPC ABI / MBC ISA) + unheaded-developer (impl) + unheaded-marshal (gate enforcement)
**Aligns with:** ADR-003 (eBPF Rust + Aya choice), ADR-012 (BPF verifier risk mitigation), ADR-023 (Wotan virtual memory), ADR-052 (in-tree source-of-truth policy), `references/battle-plan-ascend-linux-2026-05-08.md` Phase 0.

**Triggered by:** ASCEND-LINUX Phase 0.2 — running real Linux 6.x on UPC requires ISA capabilities the original MBC didn't ship: privilege transitions, atomic load-reserved/store-conditional, memory barriers, CSR access. The Phase 0.2 gap audit (`docs/protocol/mbc-isa-gap-audit-2026-05-08.md`) inventoried the deltas; this ADR records the seven design decisions made at the pair-call.

---

## Context

The MBC ISA today (`ebpf/monad-common/src/lib.rs::mbc_opcodes`, 51 opcodes) shipped to support Doom + L4 OS primitives. Linux 6.x with full MMU + SMP + IPv6 networking needs five additional capabilities that MBC v1 didn't formalize. Per `references/battle-plan-ascend-linux-2026-05-08.md`, the kingdom's chosen end-state is **maximalist** (full Linux + IPv6 + SSH), so we make the design choices that future-proof the ISA rather than ship a minimum that needs a v3.

## Decisions

### Decision 1 — Multi-ring privilege model

Add `priv_level: u8` to `MbcCpuState` (offset TBD, post-existing-fields). RV32 privilege codes: `M = 0`, `S = 1`, `U = 3`.

New MBC opcodes:
- `0x47 MRET` — Machine-mode return. Pops MEPC into PC, restores prior priv from MSTATUS.MPP.
- `0x48 SRET` — Supervisor-mode return. Pops SEPC into PC, restores prior priv from SSTATUS.SPP.

`SYSCALL (0x40)` semantics extended: transitions U → S, saves PC into SEPC, jumps via STVEC. `IRET (0x18)` continues to handle pure interrupt-return (no priv transition); `MRET`/`SRET` handle priv-transition return.

**Rationale:** real privilege isolation is defense-in-depth on top of MMU; with SMP=yes (decision 7 below) we can't rely on MMU alone for cross-CPU isolation. Linux mainline expects M/S/U; emulating them in MBC keeps the kernel port mechanical.

### Decision 2 — Memory-mapped CSR region

Reserve RAM bytes `0x000_F000 - 0x000_F0FF` (256 bytes = 64 × 4-byte CSRs).

Translator emits `CSRR* csr, rs1, rd` as a 2-instruction MBC sequence:
- read: `LD rd, F000 + csr*4`
- write: `ST F000 + csr*4, rs1`
- read-modify-write `CSRRW`: `LD rd, F000+csr*4` + `ST F000+csr*4, rs1`

Atomicity is preserved by Decision 4 (LR.W/SC.W) for kernel paths that need it; for standard CSR access in IRQ-disabled critical sections (CLI..STI), the 2-instruction sequence is acceptable.

CSR address allocation (RV32 standard):
- 0x000-0x00F: M-mode trap setup (MSTATUS, MISA, MEDELEG, MIDELEG, MIE, MTVEC, MCOUNTEREN)
- 0x040-0x04F: M-mode trap handling (MSCRATCH, MEPC, MCAUSE, MTVAL, MIP)
- 0x100-0x1FF: S-mode trap setup + handling (SSTATUS, SIE, STVEC, SSCRATCH, SEPC, SCAUSE, STVAL, SIP, SATP)
- 0xC00-0xC1F: counters/timers (CYCLE, INSTRET, TIME)

The translator maps RV32 CSR addresses 1:1 into our memory region.

**Rationale:** zero new opcodes for CSR access; minimum verifier-budget impact. Atomicity is delegated to LR.W/SC.W.

### Decision 3 — Single-CPU per UPC instance, SMP-aware ABI

`BootParams.NumCPUs` becomes meaningful. CPU_MAP becomes a **`PerCPU<MbcCpuState>` BPF map** instead of `HashMap<u8, MbcCpuState>` keyed by 0xDE/0xEA/etc.

For Phase 0-3 we run with NumCPUs=1 (single-CPU UPC instance), but the map shape and the kernel build (`CONFIG_SMP=y`) are SMP-aware so Phase 4+ can add CPUs without re-architecting.

Inter-CPU sync requires:
- `0x3F FENCE` (Decision 5) — full memory barrier
- `0x49 LR.W`, `0x4A SC.W` (Decision 4) — atomic load-reserved / store-conditional
- Per-CPU CSR snapshots — each CPU has its own 0x000_F000 region in its per_cpu map

**Rationale:** single-CPU per instance is the simplest mental model and the cheapest verifier path; multi-instance parallelism via networking (Phase 4) gives kingdom-level scaling. The "SMP-aware but single-CPU" stance keeps the door open for adding cores later without rewriting context-switch.

### Decision 4 — LR.W / SC.W as explicit opcodes

New MBC opcodes:
- `0x49 LR.W` — Load-Reserved Word. `rd = RAM[rs1]`; mark `rs1` as reserved on this CPU.
- `0x4A SC.W` — Store-Conditional Word. If reservation on `rs1` still valid, `RAM[rs1] = rs2; rd = 0` (success); else `rd = 1` (failure). Reservation cleared on context switch, IRQ, or any CPU's write to `rs1`.

Translator maps RV32-A `lr.w`/`sc.w` 1:1.

**Rationale:** with SMP=yes (Decision 3), CLI/STI alone don't lock across CPUs. LR/SC is the RV32 standard atomic primitive Linux's spinlock/RCU/cmpxchg expect. Direct opcodes are atomic at the BPF level (the eBPF interpreter holds the reservation in a per-CPU side-band map).

### Decision 5 — FENCE opcode for memory ordering

New MBC opcode:
- `0x3F FENCE` — Full memory barrier. eBPF interpreter implements as a Rust `compiler_fence(Ordering::SeqCst)` + flush of per-CPU write buffers to RAM_MAP.

`FENCE.I` (RV32 instruction-cache sync) translates to `FENCE` in our model — MBC has no separate I-cache. The translator emits `FENCE` for both RV32 ops; Linux's self-modifying code paths (eBPF JIT) work correctly even though the I-cache concept is folded.

**Rationale:** required for SMP correctness. Single opcode keeps the verifier-budget impact minimal.

### Decision 6 — Soft-float-only userspace

No F-extension MBC opcodes. Userspace built with `-msoft-float`. Kernel is integer-only (`CONFIG_FP_DISABLED=y`).

Soft-float runtime: reuse `demos/doom/gcc_runtime.c`'s 25 IEEE 754 functions. musl + busybox both support soft-float compilation; tested in Doom at 35 fps internal with 25 soft-float calls per frame.

**Rationale:** zero new opcodes; ~30 F-extension opcodes would push verifier budget into the danger zone. Real performance need would be userspace ML/games — those aren't on the L6 demo path.

### Decision 7 — BootParams v2: 256-byte padded

```c
struct BootParamsV2 {
    u32 magic;             // 'UNHD' = 0x554E4844
    u32 version;            // 2
    u32 memory_size;        // total RAM bytes
    u32 ramdisk_addr;       // ramdisk byte address
    u32 ramdisk_size;       // ramdisk size in bytes
    u32 kernel_addr;        // kernel load address (default 0x10000)
    u32 kernel_size;        // kernel image size in bytes
    u32 boot_args_addr;     // command line address (default 0x0200)
    u32 boot_args_len;      // command line length
    u32 num_cpus;           // SMP-ready (Decision 3); 1 in Phase 0-3
    u32 tick_rate_hz;       // timer interrupt rate
    u32 bss_start;          // NEW: kernel .bss start (for two-stage boot)
    u32 bss_end;            // NEW: kernel .bss end
    u32 initrd2_addr;       // NEW: optional second initramfs
    u32 cmd_line_args_ptr;  // NEW: pointer to args struct (vs. inline buffer)
    u32 boot_random_seed;   // NEW: high-quality entropy for kernel RNG init
    u32 reserved[48];       // pad to 256 bytes
};
```

**Rationale:** 256 bytes is one full cache line on x86-64 dev hosts; aligns the BootParams region naturally. Forward-compat: adding fields up to 48 × 4 = 192 reserved bytes won't require a v3.

---

## Aggregate impact

### New MBC opcodes (5 total)

| Opcode | Hex | Purpose | Decision |
|--------|-----|---------|----------|
| FENCE | 0x3F | Memory barrier | 5 |
| MRET | 0x47 | Machine-mode return | 1 |
| SRET | 0x48 | Supervisor-mode return | 1 |
| LR.W | 0x49 | Load-reserved word | 4 |
| SC.W | 0x4A | Store-conditional word | 4 |

Five new opcodes is RIGHT AT the cut-point ceiling (per battle plan §3 0.4 cut-point: ">5 new opcodes pushing verifier > 25%"). We pre-flight verifier impact in Phase 0.4 verification. If verifier budget exceeds 25% with all 5, fork the BPF program per the cut-point recipe.

### New CPU state field

`MbcCpuState.priv_level: u8` — 1 byte. Negligible map-shape impact.

### Map shape change

`CPU_MAP` becomes a `PerCPU<MbcCpuState>` BPF map. Touches:
- `crates/doom-runner/src/main.rs` (map definition)
- `ebpf/monad-cpu-ebpf/src/main.rs` (map access wrapper)
- `crates/monad-mbc/src/execute.rs` (single-context interpreter — no change; we still execute one CPU per call)

### CSR memory region

`0x000_F000 - 0x000_F0FF` reserved in `crates/doom-runner/src/memory.rs::validate_layout()`. Existing layout puts SCREEN_BASE at 0x0017_0000 (well above 0xFFF), so this carve-out is in clean low-memory.

### Battle plan timeline impact

The original Phase 0 budget was 2 weeks. Decisions 1+3+4 (multi-ring + SMP-aware + LR/SC) add architectural work:
- Multi-ring `priv_level` plumbing through MBC interpreter + translator: ~1 week
- PerCPU<MbcCpuState> BPF map migration + verifier re-validation: ~1 week
- LR.W/SC.W reservation tracking (per-CPU side-band map): ~3 days
- FENCE eBPF interpreter implementation: ~2 days
- ADR + boot-protocol v2 + ABI doc updates: ~3 days

**Revised Phase 0 budget: ~5 weeks** (vs. original 2). Phase 1+ start dates slip ~3 weeks. The total campaign now runs ~10 months instead of ~9. Stevie acknowledged this scope expansion at the pair call.

---

## Verification gates

### Phase 0.4 — Aggregate ABI gate

1. Verifier check on revised `monad-cpu-ebpf` with all 5 new opcodes implemented: ≤ 25% of 900K budget.
2. All 46 existing OS-primitive tests still PASS unchanged. (Backward compatibility.)
3. New tests: 5 opcodes × at least 3 cases each = 15 new tests covering happy-path + edge cases.
4. Doom regression: rebuild doom.mbc and play E1M1 to first secret room. Must hit 35 fps internal. (Doom shouldn't use any of the new opcodes; this verifies no perf regression in the existing path.)

### Per-decision smoke tests

1. **Multi-ring:** trap from U-mode SYSCALL, handle in S-mode, SRET back to U-mode. PC + priv_level must round-trip.
2. **CSR memory-mapped:** write MSTATUS via CSRRW, read it back via CSRR. Translator must emit the correct LD/ST sequence.
3. **PerCPU map:** boot one UPC instance with NumCPUs=1, verify CPU_MAP[cpu=0] reads work; bring up second instance, verify the two CPU_MAPs don't alias.
4. **LR.W/SC.W:** spinlock harness — two pseudo-threads (interleaved tail-calls) racing on a counter; must produce exact final value with no lost updates.
5. **FENCE:** write before fence + read after fence on a different CPU sees the write.

---

## Cross-references

- Battle plan: `references/battle-plan-ascend-linux-2026-05-08.md` Phase 0.
- Gap audit: `docs/protocol/mbc-isa-gap-audit-2026-05-08.md`.
- ADR-023 (Wotan virtual memory) — Decision 1 (multi-ring) presupposes ADR-023's TLB design works in S-mode.
- `ebpf/monad-common/src/lib.rs::mbc_opcodes` — needs 5 new opcodes added.
- `crates/monad-mbc/src/instruction.rs` — decoder needs 5 new branches.
- `crates/doom-runner/src/main.rs` — CPU_MAP shape change.

---

## Compliance

This ADR satisfies ADR-052 Rule 3: *no Cargo.toml or ABI edit without an in-tree ADR or battle-plan of record.* The MBC ISA v2 implementation lands across Phase 0.4 in a single feature branch, gated on the verifier check above.
