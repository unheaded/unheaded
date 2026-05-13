# ADR-074 — ASCEND-LINUX Phase 1.2: Page-Table Mapping Model

**Status**: DRAFT (Scientist + Architect pre-work; awaiting Stevie pair-call)
**Date**: 2026-05-11 (drafted post-Marshal extended-churn shift)
**Deciders pending**: Stevie + unheaded-architect + unheaded-computermancer + unheaded-blackmage + unheaded-developer
**Aligns with**: ADR-067 (MBC ISA v2 + UPC ABI v1), ADR-023 (TLB design), ADR-052 (in-tree source-of-truth policy)
**Triggers**: ASCEND-LINUX Phase 1.2 ("Page tables under MMU"), `references/battle-plan-ascend-linux-2026-05-08.md` §4.2 day-1 pair call.

---

## Context

### What we actually have today (before this ADR)

The kingdom is ALREADY further along on MMU than a fresh reading of the battle plan suggests. Phase 1.1 SHIP landed a working two-level Sv32-shaped page-table walker in `ebpf/monad-cpu-ebpf/src/main.rs::translate_address()`. Specifically:

- `MbcCpuState.mmu_enabled` (u8) — flat addressing when 0, paging when 1. Currently always 0 (Doom mode + xv6 banner path).
- `MbcCpuState.page_dir_base` (u32) — physical address of the per-CPU page directory.
- `MbcCpuState.priv_level` (u8) — 0=M, 1=S, 3=U per RISC-V convention (ADR-067 Decision 1).
- `TLB_MAP` — 64-entry direct-mapped TLB in BPF, keyed on `vpn & 63`, storing `[vpn, pfn, pte]`.
- 10+10+12 page-table layout — Sv32-compatible (4 KiB pages, 1024 entries per pde/pte, 4 MiB pages possible via large-page flag in pte).
- Page-table walk on TLB miss: 2 bounded `mem_read_word()` calls (verifier-safe per the existing budget gate).

**Phase 1.2 is not "build the MMU" — that's already done.** Phase 1.2 is **"wire the MMU to a process model"**: switch `page_dir_base` on context switch, manage per-process address spaces, handle the TLB flush story.

### Why a decision is needed

Three viable architectures for "which page table does the CPU use right now":

- **Option A — Per-task pgd**: each task owns its own page directory at a distinct physical address. Context switch updates `cpu.page_dir_base` and flushes the TLB.
- **Option B — Shared address space + ASID**: a single global page directory, but TLB entries (and walks) are tagged with an Address Space Identifier (ASID) so multiple processes coexist without flushes.
- **Option C — Software-tagged via MBC `priv_level` + process tag**: encode the active tenant in `MbcCpuState` (e.g. add a `current_asid: u8` field, similar to existing `current_pid`), let TLB entries carry it, but the *page table* itself is per-process (mostly Option A but with TLB tagging).

These three options have materially different consequences for: verifier budget, context-switch cost, code volume in xv6 adapters, BlackMage attack surface, and compatibility with future Phase 2 uClinux.

This ADR enumerates the options from first principles, predicts the verifier-budget headroom for each, and proposes a decision for the pair-call.

---

## First-principles analysis

### What MUST be true (axioms)

1. The eBPF program must continue to pass the kernel BPF verifier. Current budget headroom is ~7% / 900K instructions per program; Phase 1.2 must not push it past ~25% (per ASCEND-LINUX cut-point).
2. Two processes MUST NOT see each other's pages. This is a **hard safety property** — proves with `kfork+memprobe` test or similar.
3. The MBC ISA must not require new opcodes beyond what ADR-067 already locked in unless absolutely necessary. (TLB flushes are doable via `SFENCE.VMA`, which is already routed through `translator.rs` as MRET/SRET/WFI/SFENCE.VMA opcode block.)
4. Sv32 layout (10+10+12) must be preserved so the same translator path serves Phase 2 uClinux (`arch/microblaze` cargo-cult) and Phase 3 full Linux (`arch/riscv` cargo-cult).

### What we WANT to be true (goals)

5. Context switch should be O(1) wall-time, not O(TLB-size).
6. Stevie shouldn't have to write a new MBC opcode in Phase 1.2 — every new opcode burns ISA-v2 budget (5 of 256 slots already used by ADR-067; we've kept 251 in reserve).
7. The forkbomb test from the battle plan (§3.1 hard gate) must pass cleanly for any chosen option.
8. Phase 3.1's per-task pgd model should require *zero* architectural change — Phase 1.2 should pre-stage that path.

### What we ASSUME (must be challenged)

9. **Assumption**: BPF verifier is unmoved by TLB key-width changes. → Challenge: measure. TLB entry currently `[u32; 3]` (12 bytes); adding ASID widens to `[u32; 4]` (16 bytes). Verifier path-explosion is a known eBPF hazard.
10. **Assumption**: xv6's `kernel/vm.c` will accept any of the three options with minimal patching. → Challenge: read the upstream code. xv6 assumes per-task pgd by default (per `proc_pagetable()` in `kernel/proc.c`); ASID tagging would require restructuring.
11. **Assumption**: 64-entry direct-mapped TLB is enough for 4-slot process table. → Challenge: 64 / 4 = 16 TLB entries per process worst case. xv6's working set on shell+5cmds is probably ~32 unique VPNs. Likely fine, but should be measured.

---

## Three options enumerated

### Option A — Per-task pgd (classic xv6 model)

**Structure**:
- Each process owns a unique `pagetable_t` (root page directory) at a unique physical address in RAM_MAP.
- `MbcCpuState.page_dir_base` is the per-process pgd's physical address.
- Context switch in `proc.c`:
  1. `cpu.page_dir_base = next_proc->pagetable_phys` (1 word write to CPU_MAP)
  2. Flush TLB: zero `TLB_MAP[*]` (or set `vpn=0xFFFFFFFF` sentinel)
  3. Update `cpu.current_pid` (already exists)
- TLB miss → walk through new `page_dir_base` → install entry.

**Translator/eBPF changes**:
- ZERO new MBC opcodes. `SFENCE.VMA` already maps to "zero TLB_MAP" via translator (per Phase 1.1 work).
- ONE new path in eBPF: at context-switch syscall (SYS_SCHED_YIELD or trap exit), zero the TLB.

**Verifier-budget delta predicted**:
- TLB-zero loop: 64 iterations × constant body ≈ 200 verifier instructions (bounded loop, easy for verifier).
- Predicted total delta: **+0.5% to +1.5%** of budget. Well under 25% cut-point.

**xv6 patch surface**:
- `kernel/proc.c::proc_pagetable()` — allocate a fresh pgd per process. Upstream already does this; we just need to surface the `pagetable_phys` field through `cpu.page_dir_base`.
- `kernel/swtch.c` (or our `swtch_mbc.S`) — store/restore `page_dir_base` alongside ra/sp/s0/s1.
- ~40-60 lines of xv6-side change, mostly mechanical.

**BlackMage assessment**:
- **Isolation**: cryptographically clean. Different `page_dir_base` → walker reads disjoint page-table tries → impossible to leak across processes.
- **TLB poisoning**: low risk because we flush on context switch. The 1-cycle window between `page_dir_base` write and TLB flush is invisible to user code (interrupts are off at that point).
- **Attack surface gain**: minimal. The flush IS the boundary.

**Strong-inference test**:
> If Option A is correct, then after context switch from PID 1 → PID 2, the first memory access in PID 2 must produce a TLB miss (we just flushed), and the walk must read from PID 2's `page_dir_base`, not PID 1's. Crucial experiment: `dmesg`-style trace of TLB miss + which pgd was walked. Falsifiable: if walk reads stale pgd → option A is broken (probably forgot to update `page_dir_base` before re-enabling interrupts).

### Option B — Shared address space + ASID

**Structure**:
- ONE global page directory. Every process maps into the SAME pgd at different VPN regions.
- `MbcCpuState.page_dir_base` is constant — set once at boot.
- Each process gets an ASID (0..255) stored in `MbcCpuState.current_asid` (new field, 1 byte).
- TLB entries widen from `[u32; 3]` to `[u32; 4]` to include ASID.
- TLB lookup matches on `(vpn, asid)` instead of just `vpn`.
- Context switch: just update `cpu.current_asid` (1 byte write). No flush.

**Translator/eBPF changes**:
- TLB lookup arithmetic gains an ASID compare. Adds ~2-3 BPF instructions per memory access × ~30 accesses per instruction-dispatch round = ~60-90 verifier insns per round.
- Page-table walk needs to filter PTE entries by ASID (or trust kernel to install ASID-scoped PTEs).
- New `MbcCpuState` field. Wire-format compatibility: backward-compat handled by default-zero.

**Verifier-budget delta predicted**:
- Per memory access: +2 instructions for ASID compare. ~30 memory accesses per dispatch loop iter → +60 insns.
- Dispatch loop runs INSNS_PER_TICK times per packet → +60 × INSNS_PER_TICK ≈ **+4-8% of budget**, depending on tick size. Risky.
- TLB widening (`[u32; 3]` → `[u32; 4]`) — verifier sees 4-element arrays; path explosion possible if optimizer doesn't fold.

**xv6 patch surface**:
- xv6 does NOT natively support ASIDs. Significant rewrite of `proc_pagetable()`, `kalloc()`, `mappages()`, and the vmswitch path.
- ~300-500 lines of xv6-side change. Most of `kernel/vm.c` touched.

**BlackMage assessment**:
- **Isolation**: relies on ASID-tag enforcement being CORRECT in EVERY memory access. One missed compare → cross-process leak.
- **Attack surface**: higher. The ASID compare must be re-checked at every TLB hit. Any optimizer pass that elides the compare under "obvious" conditions is a backdoor.
- **TOCTOU**: between `current_asid` write at context switch and the first walker invocation, an interrupt could probe the TLB with a stale ASID. Real concern.

**Strong-inference test**:
> If Option B is correct, then after context switch from ASID 1 → ASID 2, the same VPN in both ASIDs must resolve to *different* PFNs without flushing. Crucial experiment: alloc page in PID 1, alloc page at same VPN in PID 2, write distinct values, context-switch + read. Both reads must show their own values. Falsifiable: if ASID compare missed, both reads see the same value.

### Option C — Software-tagged via priv_level + pid (hybrid)

**Structure**:
- Per-process pgd (same as Option A).
- TLB entries gain a 1-byte tag: `pid` (already in `MbcCpuState.current_pid` for the L4c scheduler).
- TLB lookup matches on `(vpn, pid)`.
- Context switch updates `cpu.page_dir_base` AND `cpu.current_pid`. Optional flush.

**Translator/eBPF changes**:
- TLB widening + pid compare on lookup. Verifier cost similar to Option B.
- Flush optional: kept for security paranoia, skipped for "TLB hot" path.

**Verifier-budget delta predicted**:
- Per memory access: +1-2 instructions (pid compare is cheaper than ASID because pid is already in `MbcCpuState`).
- **+3-6% of budget**, between Options A and B.

**xv6 patch surface**:
- Similar to Option A (per-task pgd), plus translator emits the pid into TLB key.
- xv6 doesn't need to know about the pid-tagging — that's a translator/eBPF detail. ~50-80 lines.

**BlackMage assessment**:
- **Isolation**: relies on (a) page_dir_base correctness (Option A's guarantee) AND (b) pid-compare correctness in TLB. Double-locked, like a deadbolt + chain.
- **Defense in depth**: if either layer fails, the other catches it. ASID-only (Option B) has no fallback.
- **Subtle**: the pid in TLB is *redundant* if page_dir_base correctly differs. But "redundant" doesn't mean "useless" — it's belt-and-suspenders against a future bug.

**Strong-inference test**:
> Option C makes both A and B testable simultaneously. Run both falsification tests. If Option C is correct, both pass.

---

## Verifier-budget headroom — measurement protocol

Before the pair-call, the Scientist + Computermancer should produce empirical numbers, not just estimates. Proposed measurement:

```bash
# Baseline (current state, mmu_enabled = 0 / 1 toggle, no Phase 1.2 changes)
sudo bash scripts/bpf-verifier-check.sh > tmp/bpf-budget-baseline.txt

# Stub each option (without xv6 process model) by adding the proposed
# eBPF code paths behind a feature flag:
#   cargo build --features ascend-linux,phase12-option-a -p monad-cpu-ebpf
#   cargo build --features ascend-linux,phase12-option-b -p monad-cpu-ebpf
#   cargo build --features ascend-linux,phase12-option-c -p monad-cpu-ebpf

# Re-measure
sudo bash scripts/bpf-verifier-check.sh --features ascend-linux,phase12-option-a > tmp/bpf-budget-A.txt
sudo bash scripts/bpf-verifier-check.sh --features ascend-linux,phase12-option-b > tmp/bpf-budget-B.txt
sudo bash scripts/bpf-verifier-check.sh --features ascend-linux,phase12-option-c > tmp/bpf-budget-C.txt

# Compare
diff tmp/bpf-budget-baseline.txt tmp/bpf-budget-A.txt
```

**Predicted vs actual** (file these as predictions BEFORE running, per Ioannidis):

| Option | Predicted Δ budget | Predicted absolute | 25% cut-point breach risk |
|--------|---------------------|---------------------|--------------------------|
| A (per-task pgd) | +0.5% to +1.5% | ~8% / 900K | LOW |
| B (ASID-tagged TLB) | +4% to +8% | ~12% / 900K | MEDIUM (verifier path-explosion possible) |
| C (priv_level + pid) | +3% to +6% | ~11% / 900K | LOW-MEDIUM |

If the actual numbers diverge from prediction by >2× — the model is wrong, not the experiment. Adjust before pair-call.

---

## Recommendation (Scientist + Architect convergent view)

**Recommend Option A** (per-task pgd) for Phase 1.2, with Option C deferred to Phase 3.

Reasoning:

1. **Minimal axiomatic risk**: Option A reuses the existing translate_address() unchanged. The only delta is "set page_dir_base on context switch + flush TLB." Both are 1-line patches.
2. **xv6 compatibility**: upstream xv6 IS Option A. The pre-existing model in `proc.c` and `vm.c` works as-is. We patch 40-60 lines; we don't rewrite vm.c.
3. **Verifier headroom**: lowest predicted delta. We stay well under the 25% cut-point.
4. **BlackMage clean**: isolation guaranteed by physical page-table separation. No ASID-compare correctness invariant to worry about.
5. **Phase 3 ready**: Option A IS the model that full Linux uses (`arch/riscv/mm/`). Phase 3 will reuse without redesign.
6. **Option C is a future optimization**: once we have a real workload running under Option A and can measure TLB miss rate, THEN consider adding pid-tagging if the flush cost dominates.

**Risks accepted with Option A**:
- Context switch cost = TLB flush cost. At 64 entries × constant body, this is microseconds. Acceptable for xv6+5cmds; revisit at Phase 3 multi-process workload.
- Less defense-in-depth than Option C. Mitigated by: (a) BlackMage forkbomb test at Phase 3.1 hard gate, (b) page-table-walk audit, (c) the eBPF verifier itself rejects out-of-bounds RAM_MAP reads.

---

## Open questions for the pair-call

1. **Question for Stevie**: Are we OK with Phase 1.2 being explicitly Option A (per-task pgd, full TLB flush on context switch)? Or do you want the ASID/pid-tagging optimization NOW for forward-perf?

2. **Question for Computermancer**: Is the proposed feature-flag measurement protocol (3 builds, 3 verifier runs) the right way to gather pre-decision evidence? Or do we have a faster signal?

3. **Question for BlackMage**: Are there attack paths against Option A I haven't enumerated? Specifically: cross-process timing side channels via TLB miss patterns? (xv6 doesn't have timing-sensitive crypto in userspace, so probably not — but worth a 10-minute mental walkthrough.)

4. **Question for Architect**: ADR-023's TLB design assumed flat 64-entry direct-mapped. Does that hold under Option A's per-process pressure, or do we need to widen to set-associative? My prediction: 64 entries is fine for xv6+5cmds (Phase 1.5), revisit at Phase 3 multi-process.

5. **Question for RFC Editor**: Does this need a Boot Protocol v2 amendment, or is "page tables are per-process; flush on context switch" implementation-detail rather than spec-detail? My read: implementation-detail. No spec amendment needed.

---

## Decision (PENDING pair-call)

To be filled in after the call. Format:

> **DECIDED**: Option [A | B | C] selected.
> **Vote**: Stevie [agrees | overrides]; Architect [seconds | dissents with reasoning]; Computermancer [seconds | dissents]; BlackMage [seconds | dissents]; Developer [seconds | dissents].
> **Implementation**: starts at commit [hash] after pair-call.
> **Verification**: forkbomb test at Phase 3.1 hard gate; falsification experiments per strong-inference protocol above.

---

## References

- `ebpf/monad-cpu-ebpf/src/main.rs::translate_address()` — existing MMU walker
- `ebpf/monad-common/src/lib.rs::MbcCpuState` — CPU state with mmu_enabled, page_dir_base, priv_level, current_pid
- ADR-067 — MBC ISA v2 + UPC ABI v1 (5 new opcodes including SFENCE.VMA via translator)
- ADR-023 — TLB design (64-entry direct-mapped)
- ADR-052 — in-tree source-of-truth for ADR drafts
- `references/battle-plan-ascend-linux-2026-05-08.md` — Phase 1.2 §1.2 + Phase 3.1 §3.1
- xv6 `kernel/proc.c::proc_pagetable()` and `kernel/vm.c` — upstream per-task pgd model

---

## Scientist sign-off

This ADR is pre-work, not a decision. It enumerates options, predicts verifier-budget impact, identifies falsification experiments per option, and recommends Option A on the merits — but the DECISION is the pair-call's. The Scientist's job is to ensure the room walks in with the homework done.

**Falsification experiments per option are documented** so that whichever option is chosen, the implementation comes with a built-in correctness test, not just code.

Per Ioannidis: predictions are pre-registered. Per Popper: each option has a falsification condition. Per Peirce: we generated multiple hypotheses BEFORE picking one (no premature commitment).

Ready for the pair-call.

---

## Architect review addendum (2026-05-11)

**Four-minds verdict: Option A holds up structurally, but the Scientist's analysis surfaced a deeper concept than a deeper infrastructure gap. Three issues to surface before the pair-call.**

### Issue 1 — `PROC_TABLE` slot layout does not include `page_dir_base` (BLOCKER for Option A)

Ground truth from `ebpf/monad-cpu-ebpf/src/main.rs`:

```rust
/// Layout per slot: [r0..r15, PC, flags, SP_copy, program_break]
#[map]
static PROC_TABLE: Array<[u32; 20]> = Array::with_max_entries(4, 0);
```

The 4-slot process table that drives the L4c scheduler stores 20 u32s per slot: 16 GPRs + PC + flags + SP_copy + program_break. **It does NOT store `page_dir_base` per process.** Today `MbcCpuState.page_dir_base` is per-CPU (single value for the whole interpreter), and `SYS_SET_PAGE_DIR` writes that single per-CPU field.

For Option A's "per-task pgd" model to work, the slot must widen. Two options:

- **Widen `PROC_TABLE` to `[u32; 21]`** — add a `page_dir_base` field at index 20 (push `program_break` to 21, or keep `program_break` at 19 and add `page_dir_base` at 20). Minimum-risk. Adds 4 bytes × 4 slots = 16 bytes of BPF map memory. Verifier-neutral.
- **Add a new `PROC_PGDS: Array<u32>` of length 4** — parallel array indexed by pid. Same memory cost. Slightly less cache-friendly (two map lookups on context switch instead of one). Slightly easier verifier proof because the new map is read-only after init.

**Architect's pick: widen `PROC_TABLE` to `[u32; 21]`**. Already touching context-switch path; one map is simpler than two; cache locality matters when fork/exec/yield fire repeatedly.

### Issue 2 — No allocator for new page directories on `SYS_FORK`

The current `SYS_FORK` path in `monad-cpu-ebpf/src/main.rs` copies registers from parent into child slot. **It does not allocate a physical page for the child's pgd.** The Scientist's analysis assumed this allocation exists; it does not.

Three structural options:

- **(A1) Fixed per-pid pgd region** at known addresses. With 4-slot `PROC_TABLE` and 4 KiB pgds, reserve `RAM_MAP[0x00_F0_0000..0x00_F0_1000]` for pid 0, `+0x1000` for pid 1, etc. 16 KiB total, fixed forever. **Simplest. Verifier-safe. Phase 3 will need to revisit when process count goes >4.**
- **(A2) Kernel-side freelist in `RAM_MAP`** with a new `SYS_ALLOC_PAGES` syscall. More general, more complex. Verifier cost: medium (the freelist walker is a bounded loop over ≤N free entries).
- **(A3) Allocator lives entirely in xv6 userspace**, using `SYS_BRK`-style heap growth. Most Linux-like. Highest xv6 patch surface (~150 LOC vs ~30 for A1).

**Architect's pick for Phase 1.2: A1 (fixed per-pid region)**. Phase 3 will need A3 (Linux-style); we'll have learned more by then. A1 unblocks Phase 1.2 in days, not weeks.

### Issue 3 — `SYS_FLUSH_TLB` is already chunked; that's a feature, not a flaw

Reading `ebpf/monad-cpu-ebpf/src/main.rs:1294` confirms: `SYS_FLUSH_TLB` already implements **chunked 8-entries-per-tick** flushing with PC rewind for continuation. This is the verifier-safe pattern. The Scientist's ADR-074 estimated "TLB-zero loop: 64 iterations × constant body ≈ 200 verifier instructions" — that's the WORST case (single pass). The actual deployed code does it in 8 passes of 8 iterations, so per-tick verifier cost is ~25 instructions, not 200. **Verifier-budget delta for Option A is even smaller than the Scientist estimated.** Revised prediction: **+0.2% to +0.8%** of budget.

The chunked-flush pattern means Phase 1.2 context switch will take 8 ticks to fully flush the TLB (with the CPU stalled for the remainder of the dispatch loop on tick 1 and resuming on tick 2). This is fine for xv6's coarse scheduling cadence — but worth measuring at Phase 1.5 when shell+5cmds workload actually exercises it.

### Issue 4 — Cross-tier check: Wotan DISTRIBUTED implications (Phase 3+)

In LOCAL mode (Phase 1, single node), Option A is clean. In DISTRIBUTED mode (Phase 3+, multi-node with Wotan distributed memory), per-task pgd at a node-local physical address breaks the model: another node's CPU cannot resolve `page_dir_base = 0x00F00000` unless that virtual-to-physical mapping is identical kingdom-wide.

**Phase 1.2 doesn't need to solve this.** But the ADR should flag it explicitly so Phase 3 doesn't re-derive the problem from scratch. Solution sketch (for Phase 3 future ADR): replace physical-address `page_dir_base` with a logical handle `(node_id, pgd_id)` that Wotan resolves at access time. This is the same pattern used for cross-node Wotan memory coherence.

### Issue 5 — BPF map inventory pressure

Current map count in `monad-cpu-ebpf/src/main.rs`: **16 maps**. Kernel default `kernel.bpf_stats_enabled` paths can struggle past 20-30 maps per program on older kernels (<5.15). Yggdrasil's anchor kernel is 6.x so this is fine, but verifier-budget regression to watch: each new map adds proof obligations.

**Phase 1.2 net map delta**:
- Option A (widen PROC_TABLE): **0 new maps**. Existing slot extension.
- Option B (ASID-tagged): **0 new maps**, but TLB_MAP value type widens.
- Option C (priv_level + pid hybrid): **0 new maps**.

All three options are map-neutral. Good.

---

## Architect-owned pre-work (before pair-call)

These items can land WITHOUT the pair-call decision because they're structural prerequisites for any of the three options. Marshal-mode safe (no architectural decision is being made, only foundation-laying):

| # | Item | Effort | Owner | Output |
|---|------|--------|-------|--------|
| AP-1 | **Author ADR-074 follow-on note**: add the `PROC_TABLE` layout-extension table and the fixed-per-pid-pgd-region memory map. | 30 min | Architect | This addendum (DONE) |
| AP-2 | **Measure baseline verifier budget** with current code on the Yggdrasil anchor kernel. Capture `tmp/bpf-budget-baseline-2026-05-11.txt`. | 15 min | Architect + Developer | Baseline numbers in `tmp/` |
| AP-3 | **Write a feature-flag scaffold in `ebpf/monad-cpu-ebpf/src/main.rs`** that's a NO-OP today but gates the future Option-A code path behind `#[cfg(feature = "phase12-option-a")]`. Lets the pair-call build all 3 variants for measurement. | 1 hour | Architect + Developer | Feature flags + 3 stub builds compile |
| AP-4 | **Document the per-pid pgd memory map** in `docs/doom/UPC_OS_PRIMITIVES.md` (or a new `docs/doom/UPC_PAGE_TABLE_LAYOUT.md`) so the pair-call has a concrete address space picture. | 45 min | Architect | New doc + diagram |
| AP-5 | **Sample MEM_READ_WORD verifier cost** at the current path-traversal-guarded sites to validate the Scientist's "+0.2% to +0.8%" revised estimate. | 30 min | Architect + Scientist | Numbers in `tmp/` |
| AP-6 | **Flag the DISTRIBUTED-mode follow-on** in the ASCEND-LINUX battle plan as an explicit Phase 3 dependency, so the page-table work in Phase 1.2 + Phase 3.1 isn't surprised by it. | 15 min | Architect | Battle-plan amendment commit |

**Total pre-work: ~3.5 hours of Marshal-safe foundation work.** None of it commits to Option A, B, or C. All of it leaves the pair-call decision-ready.

## Architect endorsement of Option A (conditional)

The four minds align:

- **Systems**: per-task pgd is the kernel-native pattern. Linux, xv6, FreeBSD all use it. Path of least kernel-side surprise.
- **Network**: irrelevant for Phase 1.2 (single-node compute). Will become relevant in Phase 3 DISTRIBUTED — flag it now (AP-6).
- **Infrastructure**: BPF map cost is neutral. Verifier budget delta is the smallest of the three options. Memory cost is 16 KiB fixed for 4-pid pgd region. All numbers fit easily.
- **Security**: BlackMage analysis from Scientist holds. Physical pgd separation is the cleanest isolation primitive. Defense-in-depth (Option C) becomes available later as a non-breaking addition if needed.

**Conditional on**:
1. Pre-work items AP-1 through AP-6 complete before the pair-call.
2. Measurement of actual verifier-budget delta confirms <2% (predicted 0.2-0.8%).
3. Stevie confirms the 4-process cap is acceptable for Phase 1.2-1.5; Phase 3 will widen.

If the measurement comes back >5% or some assumption fails, Architect joins the room with a revised position — but the foundation work is structurally agnostic.

---

## Decision — 2026-05-12 pair-call

> **DECIDED**: **Option A — per-task pgd** selected.
> **Vote**: Stevie (Captain) called Option A. Architect, Scientist, Computermancer, BlackMage, Developer, Marshal all aligned with the recommendation. No dissents.
> **PROC_TABLE layout**: `[u32; 21]` adopted (slot[20] = `page_dir_base`).
> **Page-table allocator**: **A1 — Fixed per-pid region** at `RAM_MAP[0x00F00000 + pid*0x1000]` (16 KiB total, inherits current 4-pid scheduler cap). Per `docs/doom/UPC_PAGE_TABLE_LAYOUT.md`.
> **Execution mode**: Marshal executes pre-work + implementation autonomously per Track C "slow + safe" posture.
> **Verification**: forkbomb test at Phase 3.1 hard gate; falsification experiment per strong-inference protocol above (PID 1 maps page → switch to PID 2 → verify isolation).
> **Implementation kickoff**: this commit lands the decision; subsequent commits execute the pre-work battle plan at `references/battle-plan-phase12-prework-2026-05-11.md` (HOLD lifted) followed by the actual Option A implementation.

### Phase 3 follow-on docket (out-of-scope for Phase 1.2)

- Replace physical-address `page_dir_base` with logical `(node_id, pgd_id)` handle resolved by Wotan DISTRIBUTED memory coherence. Current Phase 1.2 implementation will explicitly assume single-node LOCAL mode.
- Defense-in-depth Option C (pid-tagged TLB on top of per-task pgd) remains a non-breaking extension when/if a future shift wants it.
- Allocator A2/A3 (kernel freelist or xv6 userspace allocator) when process count exceeds the 4-pid `PROC_TABLE` cap.
