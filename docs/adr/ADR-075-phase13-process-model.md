# ADR-075 — ASCEND-LINUX Phase 1.3: Process Model

**Status**: ACCEPTED (autonomous Marshal-led close per Stevie's continuous-churn authorization)
**Date**: 2026-05-13
**Deciders**: Stevie (via standing autonomous-churn authorization) + unheaded-architect + unheaded-developer + unheaded-blackmage (joint security review with unheaded-sentinel)
**Aligns with**: ADR-067 (MBC ISA v2 + UPC ABI v1), ADR-074 (Phase 1.2 page-table model), ADR-052 (in-tree source-of-truth policy)
**Triggers**: ASCEND-LINUX Phase 1.3 ("Process model"), `references/battle-plan-ascend-linux-2026-05-08.md` §4.3 items 33-36.
**Predecessors**: Phase 1.2 IMPL CLOSED at `b9572c26`; Phase 1.3 AP-2 MRET/SRET RV→MBC translation SHIPPED at `73834054`; PRE-WORK plan at `abb84cfb`.

---

## Context

Phase 1.3 advances xv6 past the boot banner into a real process model: SYS_FORK, SYS_EXECVE, SYS_WAITPID, SYS_EXIT, SYS_SCHED_YIELD on the live BPF interpreter. The PRE-WORK shift (kickoff plan + 6 AP-N artifacts) consolidated 6 open decisions into this ADR.

**Substrate already in place** (do not re-derive):
- Phase 1.2 IMPL substrate: PROC_TABLE `Array<[u32; 21]> x 4`, `pgd_base_for_pid()` at `ebpf/monad-cpu-ebpf/src/phase12.rs`, `uvmcreate(int pid)` in xv6 vm.c, `scheduler_context_switch` saves/loads slot[20]. See [[phase12-impl-closure-2026-05-13]] memory.
- Phase 1.3 AP-2: MRET/SRET RV→MBC translation in BPF interpreter + `upc-bootctl::populate_rv2mbc(bytes, text_rv_word_base)`. xv6 advanced past `main()`, priv 0→1 transition observed in live boot. See [[phase13-ap2-mret-fix]] memory.

---

## Decisions

### D-1 — Process slot count: GROW PROC_TABLE 4 → 8

**Decision**: Widen PROC_TABLE from `Array<[u32; 21]> x 4` to `Array<[u32; 21]> x 8`. Update `MAX_PROCESSES = 8` in `ebpf/monad-cpu-ebpf/src/phase12.rs` and the host emulator mirror.

**Rationale (per AP-1 `references/phase13-ap1-slot-count-budget.md`)**: xv6 needs init + sh + 5 cmds (ls, cat, echo, uname, ps) = 7 slots minimum. 8 is the next power of 2 and gives one slot of headroom for tests/forkbomb. 16 was rejected because the additional verifier insn delta (~+650) is unnecessary for Phase 1 scope. Phase 5 multi-process demos can revisit if 8 isn't enough.

**Cost**: ~+300 BPF insns vs the 4-slot baseline (per AP-1 projection). Map memory: 84B × 8 = 672B (vs 336B at 4) — trivial.

**Implementation**: Phase 1.3 IMPL Phase A.

### D-2 — MRET/SRET fall-through: FIXED at commit `73834054`

**Decision**: Already shipped. No additional work in this ADR. Acknowledged here so future readers see Phase 1.3 reasoned about it before IMPL.

**See**: [[phase13-ap2-mret-fix]] memory for full root cause + fix shape.

### D-3 — Trapframe collapse: KEEP xv6's trampoline pattern

**Decision**: Do NOT redesign xv6's M→S→U trap path. The existing `adapters/swtch_mbc.S`, `trampoline_mbc.S`, and `kernelvec_mbc.S` (with s2-s11 stripped per task #60) work correctly under UPC ABI v1. Phase 1.3 IMPL must NOT introduce a new trap shim.

**Rationale (per AP-3 `references/phase13-ap3-trapframe-decision.md`)**: UPC ABI v1 (ADR-067 §Decision 1) codifies SYSCALL/SRET/MRET with the same semantics xv6 expects. The translator already handles the CSR + sret sequences. A "collapse" would re-derive what already works and burn complexity budget.

**Cost**: zero net (no new code, no new opcodes).

### D-4 — Userland address space layout

**Decision**: Adopt the layout amendment landed at commit `0089a3ff` (`docs/doom/UPC_PAGE_TABLE_LAYOUT.md`):

- **User code/data**: virtual addresses `0x00000000+` (xv6 default user-VA layout). Page tables at `pgd[pid]` map these to physical range `0x01000000-0x02FFFFFF`.
- **Trampoline**: highest user VA (`TRAMPOLINE = MAXVA - PGSIZE`, per xv6 convention). Maps to physical address of `trampoline_mbc.S` (kernel-side, shared).
- **Trapframe**: just below trampoline (`TRAPFRAME = TRAMPOLINE - PGSIZE`). Per-process, allocated by `proc_pagetable()`.

**Rationale (per AP-4 amendment)**: Stays within the existing RAM_MAP layout. No new physical regions reserved. xv6 userland code conventions (high VA trampoline, fixed trapframe slot) drive the choice.

### D-5 — Fork bridge: xv6's `kfork()` is AUTHORITATIVE; eBPF SYS_FORK is a low-level primitive

**Decision**: xv6's `kernel/proc.c::kfork()` (already wired post Phase 1.2 to call `proc_pagetable() → uvmcreate(p->pid)`) is the authoritative caller. The eBPF SYS_FORK handler at `main.rs:1036-1080` is a primitive that xv6 invokes via the standard syscall path: `ecall a7=SYS_FORK_NUM` → translator emits MBC SYSCALL (0x40) → BPF dispatches to SYS_FORK handler → allocates PROC_TABLE slot → returns via SRET → xv6 kernel does bookkeeping (fds, parent pointer, RUNNABLE state).

**Sub-decision on uvmcopy**: `uvmcopy(parent_pgd, child_pgd, sz)` runs as PLAIN RV32 code that the translator emits as MBC LD/ST loops. No new BPF handler. Performance implication: bounded by interpreter throughput per word, acceptable for Phase 1.3 (4 KiB copy = 1024 LD/ST pairs = small fraction of trigger-packet budget).

**Rationale (per AP-5 `references/phase13-ap5-fork-bridge.md`)**: dual authority would create state-consistency bugs. xv6's invariants (RUNNABLE/UNUSED/ZOMBIE state machine, parent pointer wakeup) are non-trivial and well-tested. The BPF primitive only owns CPU/page-table state — exactly what xv6 doesn't track.

### D-6 — Verifier budget projection: 8.67% of 900K, PASSES 12% hard gate

**Decision**: Phase 1.3 IMPL is GO. Projected post-implementation BPF insn count = **78,025 insns = 8.67% of 900K**.

**Breakdown (per AP-6 `references/phase13-ap6-verifier-projection.md`)**:
| Source | Δ insns |
|---|---:|
| Phase 1.2 baseline (post AP-2) | 76,125 (8.46%) |
| PROC_TABLE 4 → 8 (D-1) | +300 |
| sys_execve_handler | +800 |
| sys_waitpid_handler | +400 |
| sys_exit_handler | +300 |
| sys_sched_yield_handler | +100 |
| uvmcopy host-side | 0 (RV32 via existing opcodes) |
| **Phase 1.3 projected total** | **78,025 (8.67%)** |

Hard gate: 12% of 900K = 108K insns. Headroom: 30,000 insns. Comfortable.

---

## Out of scope (explicitly deferred)

- **Live forkbomb test on real booted xv6**: gates on Phase 1.3 IMPL being complete + Phase 1.4 ramdisk providing executable. Phase 1.5 territory.
- **Multi-process shell + 5 commands**: Phase 1.5 final gate (§1.6 of ASCEND-LINUX battle plan).
- **DISTRIBUTED-mode replacement of physical-address page_dir_base** with `(node_id, pgd_id)` handle: flagged in ASCEND-LINUX Phase 3.1 per ADR-074 AP-6. Out of Phase 1 scope entirely.
- **MAX_PROCESSES > 8**: revisit only if Phase 5 multi-process demos require it.
- **BPF-side mirror of uvmcopy**: rejected. Plain RV32 via existing opcodes is the chosen path. If perf becomes a blocker, add a new opcode in a future ADR.

---

## Security review (joint Sentinel + BlackMage)

See [[security-upc-linux-gain-vs-risk]] memory for full brief. Key items affecting Phase 1.3 IMPL:

1. **RV2MBC poisoning**: any `.rv2mbc` loaded by `upc-bootctl` must be integrity-checked against the matching `.mbc`. ACTION: add SHA-256 of `.rv2mbc` to BootParams v2 reserved field, verify in bootctl before `populate_rv2mbc` invocation. **Tracked as a Phase 1.3 IMPL prerequisite, not part of this ADR.**
2. **PROC_TABLE row tampering**: a malicious user-mode write to `PROC_TABLE[i][20]` could redirect `page_dir_base` on next context switch. xv6 user processes have no MMIO access to PROC_TABLE by default (it's a BPF map, host-only). Phase 1.3 IMPL must ensure no MMIO region overlaps PROC_TABLE's BPF-map-accessible range.
3. **0x49 LR.W / 0x4A SC.W reservation tracking**: untested under priv-level transitions. Phase 1.3 IMPL adds falsification tests with deliberate context-switch + priv-transition interleavings.

---

## Authorization note

Stevie's standing directive (`feedback_unattended_churn_with_queued_work` + plan-mode authorization `build-more-extensive-brached-wobbly-pond.md`) authorizes Marshal-led autonomous decisions on Phase 1.3 PRE-WORK. This ADR is ACCEPTED autonomously per that scope. Pair-call retro is welcome but not gating.

---

*Free to use. Free to share.*
