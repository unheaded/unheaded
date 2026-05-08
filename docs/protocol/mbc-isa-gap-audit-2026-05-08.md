# MBC ISA Gap Audit — 2026-05-08 (Phase 0.2 of ASCEND-LINUX)

**Auditor:** Marshal-led ASCEND-LINUX Phase 0.2 (`references/battle-plan-ascend-linux-2026-05-08.md`).
**Scope:** Inventory what MBC has today vs. what real Linux 6.x needs for L5 → L6.
**Inventory only — no code changes in this audit. Design decisions deferred to the Phase 0.2/0.3 pair call.**

---

## Findings — what MBC already has

The mbc_opcodes module (`ebpf/monad-common/src/lib.rs` lines 1131+) defines **51 opcodes** today. Notable Linux-relevant primitives that ALREADY EXIST:

| Opcode | Hex | Role | Linux use |
|--------|-----|------|-----------|
| `IRET` | 0x18 | Return from interrupt; pop PC+flags, re-enable interrupts | every IRQ exit, every syscall return |
| `CLI` | 0x3B | "Lock half of atomic operations on single-core UPC" — clears IF flag | `local_irq_disable()`, spinlock entry |
| `STI` | 0x3C | "Unlock half" — sets IF flag | `local_irq_enable()`, spinlock exit |
| `XCHG` | 0x3D | `tmp = dst; dst = RAM[src+imm]; RAM[src+imm] = tmp` | `xchg()`, atomic swap |
| `CAS` | 0x3E | Compare-and-swap with Z-flag report | `cmpxchg()`, lockless data structures, refcounts |
| `SYSCALL` | 0x40 | Invoke I/O callback; imm16 = syscall number | every userspace → kernel transition |
| `HALT` | 0xFF | Halt execution | kernel `cpu_idle_loop` HLT |

**This is huge for Linux portability.** The atomic-op set Stevie's plan called out as "decision needed" (Phase 0.2 decision 6) is largely answered: `CLI`/`STI`/`XCHG`/`CAS` cover the lock primitives Linux needs on a single-core target.

---

## Gaps — what MBC is missing (Linux-relevant)

### Gap 1 — Memory barriers (`FENCE`, `FENCE.I`)

**RV32 spec:** `FENCE pred,succ` orders memory ops; `FENCE.I` synchronizes I-cache vs D-cache.
**MBC today:** absent.
**Linux usage:** every `smp_mb()`, `mb()`, `wmb()`, `rmb()`, `dma_wmb()`. Even on single-core builds, the kernel emits these as compiler barriers.
**Disposition (recommended):** translator stubs `FENCE` as NOP + a Rust-side compiler fence on the eBPF interpreter side. No new MBC opcode needed for single-core. **Reserve 0x3F for future SMP work.**

### Gap 2 — CSR access (`CSRRW`, `CSRRS`, `CSRRC`, `CSRRWI`, `CSRRSI`, `CSRRCI`)

**RV32 spec:** Control/Status Register read-modify-write ops for SATP, MTVEC, MSTATUS, MEPC, MCAUSE, MTVAL, MIP, MIE, MIDELEG, MEDELEG, etc.
**MBC today:** absent.
**Linux usage:** `start_kernel` reads + writes ~12 CSRs during early init; `do_trap` reads MCAUSE + MEPC + MTVAL on every exception.
**Two options for the pair call:**

  **(a) Memory-mapped CSR region** — reserve RAM bytes `0x000_F000 - 0x000_F0FF` (256 bytes = 64 × 4-byte CSRs). Translator emits `CSRR* csr, rs1, rd` → `LD rd, F000+csr*4` + `ST F000+csr*4, rs1`. Pro: zero new opcodes, fits existing memory model. Con: not atomic (each CSRRW expands to two MBC instructions; race window if both are interrupted by IRQ).

  **(b) New MBC opcode** — `CSRRW` (0x41), `CSRRS` (0x42), `CSRRC` (0x43), three `*I` variants 0x44-0x46. Translator-direct. Pro: atomic. Con: 6 new opcodes — verifier-budget impact.

  **Marshal recommendation:** (a) for Phases 1-3 (single-core, all CSR access serialized by interrupt-disable), (b) deferred to Phase 4 if SMP becomes necessary.

### Gap 3 — Privilege transitions (`MRET`, `SRET`)

**RV32 spec:** Machine-mode-return / Supervisor-mode-return. Restore PC from MEPC/SEPC, restore prior priv level.
**MBC today:** absent. The IRET (0x18) we have only handles flags + PC, no priv-level concept.
**Linux usage:** Linux uses M-mode for boot, transitions to S-mode for kernel ops, and U-mode for userspace. Every syscall = U → S transition; every IRQ from userspace = U → S → handle → SRET to U.
**Two options for the pair call:**

  **(a) Single-ring kingdom** — Linux runs entirely in one privilege level. We add a `priv_level` byte to MbcCpuState but always set it to S-mode equivalent. SYSCALL (0x40) does not transition privilege; it just dispatches via IVT 0x80. MRET / SRET become NOPs in the translator.

  **(b) Multi-ring kingdom** — add `priv_level` to MbcCpuState. Add `MRET` (0x47) + `SRET` (0x48) opcodes. SYSCALL transitions M/S/U. Real privilege isolation enforced at MBC level.

  **Marshal recommendation:** (a) for Phases 1-3. The MMU emulation already in L4d provides isolation via page tables; privilege rings would be extra defense-in-depth that we don't need until SMP. Defer to Phase 4 if Round Table demands stricter isolation.

### Gap 4 — Floating-point (`F` and `D` extensions)

**RV32 spec:** ~30 single-precision F ops (FADD.S, FMUL.S, etc.) + ~30 double-precision D ops.
**MBC today:** absent. Doom's `gcc_runtime.c` provides 25 soft-float functions.
**Linux usage:** kernel is integer-only (CONFIG_FP_DISABLED is the default for embedded/uClinux). glibc/musl userspace optionally uses FP.
**Disposition:** ship soft-float-only userspace (busybox + musl built with `-msoft-float`). NO new MBC opcodes. Existing soft-float runtime in `demos/doom/gcc_runtime.c` already proves this works at scale (Doom's renderer uses it).

### Gap 5 — Reserved opcode space

**Currently defined:** 51 opcodes across 0x00-0xFF.
**Reserved (free for ASCEND-LINUX):**
- 0x11-0x17, 0x19 (8 slots)
- 0x1E, 0x1F (2 slots)
- 0x2B-0x2F (5 slots)
- 0x3F (1 slot)
- 0x41-0xFE (190 slots)

That's **206 free opcodes** before we touch HALT (0xFF). Plenty of room for whatever the pair call decides.

---

## Phase 0.2 → 0.3 pair-call decisions needed

| # | Decision | Recommendation | Risk if wrong |
|---|----------|----------------|---------------|
| 6 | Atomic ops: keep CLI/STI/XCHG/CAS as the lock set, OR add LR.W/SC.W | KEEP existing — translator maps RV32-A patterns to CLI+LD+...+STI sequences | Low |
| 7 | FENCE / FENCE.I | NOP-translate; reserve 0x3F | Low |
| 8 | CSR access strategy | (a) memory-mapped region 0x000_F000-0x000_F0FF | Med — atomicity loss across IRQ |
| 9 | Privilege rings | (a) single-ring; MMU provides isolation | Med — relies on MMU correctness |
| 10 | Floating-point | Soft-float-only userspace; reuse gcc_runtime.c | Low |
| 13 | BootParams v2 layout: 88-byte tight or 256-byte padded | 256-byte padded (forward-compat) | Low |
| 16 | SMP support | Single-CPU through Phase 4; defer SMP | Low — explicit cut point if it becomes blocker |

**These 7 decisions need a 30-min pair call with Stevie + Computermancer + Architect before Phase 0.4 commits ADR-067.**

---

## Phase 0.1 baseline regression numbers

For the record, captured on 2026-05-08 at 05:23 UTC:

| Artifact | Value |
|----------|-------|
| `demos/doom/doom.elf` | 414 236 bytes |
| `demos/doom/doom.mbc` | 345 852 bytes (86 463 instructions) |
| `demos/doom/doom.rv2mbc` | 209 016 bytes (52 254 entries) |
| `os_primitives_test.rs` | 46/46 PASS (was 37 documented; suite has grown) |
| `monad-cpu-ebpf` BPF instructions | 69 863 (~7% of 900K verifier budget) |
| `bpf-verifier-check.sh` gate | PASSED (2 warnings, 0 failures) |

These are the numbers Phases 1-4 must not regress past. Saved at `tmp/bpf-baseline-pre-ascend.txt`.

---

## What lands in this commit

- This document (`docs/protocol/mbc-isa-gap-audit-2026-05-08.md`).
- `tmp/bpf-baseline-pre-ascend.txt` (regression baseline).
- Fresh `demos/doom/doom.{elf,mbc,rv2mbc}` artifacts (rebuilt clean).

**Does NOT land in this commit:** any change to `ebpf/monad-common/src/lib.rs`'s opcode set or `crates/monad-mbc/src/instruction.rs`'s decoder. Those wait on the pair call + ADR-067.
